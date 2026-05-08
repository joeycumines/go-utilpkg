package inprocgrpc

import (
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRPCCoordinatorMaxOwnerWatermarkSignals(t *testing.T) {
	coordinator := newRPCCoordinator()
	maximum := ^uint64(0)
	state := rpcControlState{
		selected:      true,
		stable:        true,
		terminalOwner: maximum,
		ownerFenced:   maximum,
		ownerSettled:  maximum,
	}
	coordinator.publishSignals(state)
	select {
	case <-coordinator.boundary:
	default:
		t.Fatal("max owner fence did not publish boundary")
	}
	select {
	case <-coordinator.runnable:
	default:
		t.Fatal("max owner settlement did not publish runnable")
	}

	coordinator.mu.Lock()
	coordinator.state.ownerSettled = maximum
	coordinator.mu.Unlock()
	result := make(chan bool, 1)
	go func() {
		result <- coordinator.waitOwnerRunnable(maximum)
	}()
	select {
	case runnable := <-result:
		if !runnable {
			t.Fatal("max settled owner reported unavailable")
		}
	case <-time.After(time.Second):
		t.Fatal("max settled owner wait did not return")
	}
}

func TestRPCCoordinatorOrdinarySelectionStableAtReturn(t *testing.T) {
	control := newRPCCoordinator(false)
	selected := status.Error(codes.Aborted, "selected")
	claim := control.claim(
		selected,
		terminalAbort,
		terminalServer,
		0,
	)
	if !claim.admitted {
		t.Fatalf("claim = %+v", claim)
	}
	select {
	case <-control.selected:
	default:
		t.Fatal("selection was not published before claim returned")
	}
	select {
	case <-control.stable:
	default:
		t.Fatal("ordinary result was not stable before claim returned")
	}
	result := control.observe()
	if !result.terminal ||
		status.Code(result.observation.err) != codes.Aborted {
		t.Fatalf("observation = %+v", result)
	}
}

func TestRPCCoordinatorPreparedResultJoinsCompletion(t *testing.T) {
	control := newRPCCoordinator(false)
	claim := control.claim(
		nil,
		terminalGraceful,
		terminalServer,
		1,
	)
	capability, started := control.startOwner(claim.ownerTurn)
	if !started {
		t.Fatal("terminal owner did not start")
	}
	observed := make(chan rpcControlObserveReply, 1)
	go func() { observed <- control.observe() }()
	select {
	case result := <-observed:
		t.Fatalf("prepared result returned early: %+v", result)
	default:
	}
	prepared := status.Error(codes.DataLoss, "prepare failed")
	if !control.completeTerminal(
		capability,
		claim.terminalID,
		true,
	) {
		t.Fatal("terminal completion was rejected")
	}
	if !control.completePrepared(claim.terminalID, 1, prepared) {
		t.Fatal("prepared completion was rejected")
	}
	select {
	case result := <-observed:
		t.Fatalf("prepared result crossed ingress fence: %+v", result)
	default:
	}
	control.ownerFence(claim.ownerTurn, true)
	result := <-observed
	if status.Code(result.observation.err) != codes.DataLoss {
		t.Fatalf("prepared observation = %+v", result)
	}
	<-control.released
}

func TestRPCCoordinatorCallbackCapabilityOrdersTerminal(t *testing.T) {
	control := newRPCCoordinator(false)
	callbackTurn, admitted := control.reserveCallback()
	if !admitted {
		t.Fatal("callback was not admitted")
	}
	control.ownerFence(callbackTurn, true)
	callback, started := control.startOwner(callbackTurn)
	if !started {
		t.Fatal("callback did not start")
	}
	claim := control.claim(nil, terminalGraceful, terminalServer, 0)
	if _, started := control.startOwner(claim.ownerTurn); started {
		t.Fatal("terminal owner crossed running callback")
	}
	if !control.completeCallback(callback, false, false) {
		t.Fatal("callback completion was rejected")
	}
	capability, started := control.startOwner(claim.ownerTurn)
	if !started {
		t.Fatal("terminal owner did not start after callback")
	}
	if _, admitted := control.reserveCallback(); admitted {
		t.Fatal("callback admitted after terminal selection")
	}
	control.completeTerminal(
		capability,
		claim.terminalID,
		true,
	)
	control.ownerFence(claim.ownerTurn, true)
}

func TestRPCCoordinatorDirectCapabilitySettles(t *testing.T) {
	control := newRPCCoordinator(false)
	capability, admitted := control.admitDirect()
	if !admitted {
		t.Fatal("direct callback was not admitted")
	}
	if _, admitted := control.admitDirect(); admitted {
		t.Fatal("fresh nested direct callback was admitted")
	}
	if !control.completeCallback(capability, false, false) {
		t.Fatal("direct callback did not settle")
	}
	if _, admitted := control.admitDirect(); !admitted {
		t.Fatal("next direct callback was not admitted")
	}
}

func TestRPCCoordinatorConcurrentClaimHasOneWinner(t *testing.T) {
	const callers = 256
	control := newRPCCoordinator(false)
	start := make(chan struct{})
	results := make(chan rpcControlClaimReply, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	for range callers {
		go func() {
			ready.Done()
			<-start
			results <- control.claim(
				nil,
				terminalAbort,
				terminalServer,
				0,
			)
		}()
	}
	ready.Wait()
	close(start)
	winners := 0
	for range callers {
		if (<-results).admitted {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("winning claims = %d, want 1", winners)
	}
}

func TestRPCCoordinatorRecoveryProofSelectsRelease(t *testing.T) {
	control := newRPCCoordinator(false)
	claim := control.claim(nil, terminalGraceful, terminalServer, 0)
	capability, _ := control.startOwner(claim.ownerTurn)
	control.completeTerminal(
		capability,
		claim.terminalID,
		true,
	)
	control.ownerFence(claim.ownerTurn, true)
	if _, transfer := control.recoveryProof(); transfer {
		t.Fatal("normal owner release returned a recovery transfer")
	}
}

func TestRPCCoordinatorRejectsForeignRecoveryProof(t *testing.T) {
	control := newRPCCoordinator(false)
	foreign := newRPCCoordinator(false)
	control.schedulerDone(rpcSchedulerDoneProof{coordinator: control})
	proof, transfer := control.recoveryProof()
	if !transfer {
		t.Fatal("scheduler recovery was not granted")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("foreign recovery proof did not panic")
		}
	}()
	foreign.recoveryRelease(proof)
}
