package inprocgrpc

import (
	"reflect"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRPCControlReducerOrdinarySelectionIsStable(t *testing.T) {
	state := newRPCControlState(false)
	selected := status.Error(codes.Aborted, "selected")
	state, claim := reduceRPCControl(state, rpcControlEvent{
		kind:   rpcControlClaim,
		mode:   terminalAbort,
		origin: terminalServer,
		err:    selected,
	})
	if !claim.admitted || !claim.published || !state.stable {
		t.Fatalf("ordinary claim = %+v, %+v", state, claim)
	}
	state, observed := reduceRPCControl(state, rpcControlEvent{
		kind: rpcControlObserve,
	})
	if !observed.observe || observed.wait ||
		status.Code(observed.observation.err) != codes.Aborted {
		t.Fatalf("ordinary observation = %+v", observed)
	}
}

func TestRPCControlReducerPreparedCompletionWaitsFence(t *testing.T) {
	state := newRPCControlState(false)
	state, claim := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlClaim,
		mode:      terminalGraceful,
		origin:    terminalServer,
		prepareID: 1,
	})
	state, start := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerStart,
		ownerTurn: claim.ownerTurn,
	})
	if !start.ownerStarted {
		t.Fatalf("inline start = %+v", start)
	}
	prepared := status.Error(codes.DataLoss, "prepare failed")
	state, completion := reduceRPCControl(state, rpcControlEvent{
		kind:       rpcControlOwnerComplete,
		terminalID: claim.terminalID,
		ownerTurn:  claim.ownerTurn,
		drained:    true,
	})
	if !completion.ownerCompleted || completion.published || state.stable {
		t.Fatalf("pre-fence completion = %+v, %+v", state, completion)
	}
	state, preparedCompletion := reduceRPCControl(state, rpcControlEvent{
		kind:       rpcControlPreparedComplete,
		terminalID: claim.terminalID,
		prepareID:  1,
		err:        prepared,
	})
	if preparedCompletion.published || state.stable {
		t.Fatalf("prepared completion = %+v, %+v",
			state,
			preparedCompletion,
		)
	}
	state, fence := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerFence,
		ownerTurn: claim.ownerTurn,
		accepted:  true,
	})
	if !fence.published || !state.stable ||
		status.Code(fence.observation.err) != codes.DataLoss {
		t.Fatalf("fenced completion = %+v, %+v", state, fence)
	}
	if !fence.released || !state.released {
		t.Fatalf("derived release = %+v, %+v", state, fence)
	}
}

func TestRPCControlReducerRejectsStaleDrainSnapshot(t *testing.T) {
	state := newRPCControlState(false)
	state, ownerA := reduceRPCControl(state, rpcControlEvent{
		kind: rpcControlOwnerReserve,
	})
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerFence,
		ownerTurn: ownerA.ownerTurn,
		accepted:  true,
	})
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerStart,
		ownerTurn: ownerA.ownerTurn,
	})
	state, completedA := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerComplete,
		ownerTurn: ownerA.ownerTurn,
		drained:   true,
	})
	if !completedA.ownerCompleted || completedA.dataReleased {
		t.Fatalf("owner A completion = %+v, %+v", state, completedA)
	}

	state, terminalB := reduceRPCControl(state, rpcControlEvent{
		kind:   rpcControlClaim,
		mode:   terminalGraceful,
		origin: terminalServer,
	})
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerFence,
		ownerTurn: terminalB.ownerTurn,
		accepted:  true,
	})
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerStart,
		ownerTurn: terminalB.ownerTurn,
	})
	state, completedB := reduceRPCControl(state, rpcControlEvent{
		kind:       rpcControlOwnerComplete,
		terminalID: terminalB.terminalID,
		ownerTurn:  terminalB.ownerTurn,
		drained:    false,
	})
	if !completedB.ownerCompleted || completedB.dataReleased ||
		state.dataReleased {
		t.Fatalf("terminal B completion = %+v, %+v", state, completedB)
	}

	state, staleA := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerComplete,
		ownerTurn: ownerA.ownerTurn,
		drained:   true,
	})
	if staleA.ownerCompleted || staleA.dataReleased || state.dataReleased {
		t.Fatalf("stale owner A snapshot released B data = %+v, %+v",
			state,
			staleA,
		)
	}

	state, ownerC := reduceRPCControl(state, rpcControlEvent{
		kind:       rpcControlDataOwnerReserve,
		terminalID: terminalB.terminalID,
	})
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerFence,
		ownerTurn: ownerC.ownerTurn,
		accepted:  true,
	})
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlDataOwnerStart,
		ownerTurn: ownerC.ownerTurn,
	})
	state, completedC := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerComplete,
		ownerTurn: ownerC.ownerTurn,
		drained:   true,
	})
	if !completedC.ownerCompleted || !completedC.dataReleased ||
		!state.dataReleased {
		t.Fatalf("owner C completion = %+v, %+v", state, completedC)
	}
}

func TestRPCControlReducerDoneSelectsUnavailable(t *testing.T) {
	state := newRPCControlState(false)
	state, done := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlSchedulerDone,
		doneProof: true,
	})
	if !state.selected || !state.stable || !done.published ||
		status.Code(state.err) != codes.Unavailable {
		t.Fatalf("Done selection = %+v, %+v", state, done)
	}
	if !done.transferGranted {
		t.Fatalf("Done recovery = %+v", done)
	}
	unchanged, claim := reduceRPCControl(state, rpcControlEvent{
		kind:   rpcControlClaim,
		mode:   terminalAbort,
		origin: terminalCaller,
		err:    status.Error(codes.Canceled, "late"),
	})
	if claim.admitted || !sameRPCControlState(unchanged, state) {
		t.Fatal("late claim changed Done selection")
	}
}

func TestRPCControlReducerDoneClosesUnfencedReservation(t *testing.T) {
	state := newRPCControlState(false)
	selected := status.Error(codes.PermissionDenied, "selected")
	state, claim := reduceRPCControl(state, rpcControlEvent{
		kind:   rpcControlClaim,
		mode:   terminalAbort,
		origin: terminalCaller,
		err:    selected,
	})
	state, done := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlSchedulerDone,
		doneProof: true,
	})
	if !done.transferGranted || !state.stable ||
		status.Code(state.err) != codes.PermissionDenied {
		t.Fatalf("Done transfer = %+v, %+v", state, done)
	}
	_, fence := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerFence,
		ownerTurn: claim.ownerTurn,
		accepted:  true,
	})
	if fence.ownerFenced {
		t.Fatal("late ingress fence mutated closed reservation")
	}
}

func TestRPCControlReducerPreparedLossRemapsUnavailable(t *testing.T) {
	state := newRPCControlState(false)
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlClaim,
		mode:      terminalGraceful,
		origin:    terminalServer,
		prepareID: 1,
	})
	state, done := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlSchedulerDone,
		doneProof: true,
	})
	if !done.transferGranted || done.published || state.stable {
		t.Fatalf("prepared loss = %+v, %+v", state, done)
	}
	if state.origin != terminalScheduler ||
		state.mode != terminalAbort ||
		status.Code(state.err) != codes.Unavailable {
		t.Fatalf("prepared loss outcome = %+v", state)
	}
	state, missing := reduceRPCControl(state, rpcControlEvent{
		kind:       rpcControlPreparedComplete,
		terminalID: state.terminalID,
		prepareID:  1,
		err:        unavailableError(),
	})
	if !missing.published || !state.stable ||
		status.Code(state.err) != codes.Unavailable {
		t.Fatalf("missing preparation = %+v, %+v", state, missing)
	}
}

func TestRPCControlReducerPreparedTransferPreservesResult(t *testing.T) {
	state := newRPCControlState(false)
	state, claim := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlClaim,
		mode:      terminalGraceful,
		origin:    terminalServer,
		prepareID: 7,
	})
	preparedErr := status.Error(codes.DataLoss, "prepared")
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerFence,
		ownerTurn: claim.ownerTurn,
		accepted:  true,
	})
	state, done := reduceRPCControl(state, rpcControlEvent{
		kind:       rpcControlSchedulerDone,
		doneProof:  true,
		terminalID: claim.terminalID,
		prepareID:  7,
	})
	if !done.transferGranted || done.published || state.stable {
		t.Fatalf("prepared transfer = %+v, %+v", state, done)
	}
	state, completed := reduceRPCControl(state, rpcControlEvent{
		kind:       rpcControlPreparedComplete,
		terminalID: claim.terminalID,
		prepareID:  7,
		err:        preparedErr,
	})
	if !completed.published || !state.stable ||
		status.Code(state.err) != codes.DataLoss {
		t.Fatalf("prepared recovery = %+v, %+v", state, completed)
	}
}

func TestRPCControlReducerCallbackBoundaryOrdersTerminal(t *testing.T) {
	state := newRPCControlState(false)
	state, callback := reduceRPCControl(state, rpcControlEvent{
		kind: rpcControlOwnerReserve,
	})
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerFence,
		ownerTurn: callback.ownerTurn,
		accepted:  true,
	})
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerStart,
		ownerTurn: callback.ownerTurn,
	})
	state, claim := reduceRPCControl(state, rpcControlEvent{
		kind:   rpcControlClaim,
		mode:   terminalGraceful,
		origin: terminalServer,
	})
	state, blocked := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerStart,
		ownerTurn: claim.ownerTurn,
	})
	if blocked.ownerStarted {
		t.Fatal("terminal owner overtook callback")
	}
	state, completion := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerComplete,
		ownerTurn: callback.ownerTurn,
	})
	if !completion.terminalRunnable {
		t.Fatalf("callback completion = %+v", completion)
	}
	state, started := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerStart,
		ownerTurn: claim.ownerTurn,
	})
	if !started.ownerStarted {
		t.Fatalf("terminal start = %+v", started)
	}
	_, late := reduceRPCControl(state, rpcControlEvent{
		kind: rpcControlOwnerReserve,
	})
	if late.ownerAdmitted {
		t.Fatal("callback admitted after terminal selection")
	}
}

func TestRPCControlReducerDirectAdmissionIsAtomic(t *testing.T) {
	state := newRPCControlState(false)
	state, direct := reduceRPCControl(state, rpcControlEvent{
		kind: rpcControlOwnerDirectAdmit,
	})
	if !direct.ownerAdmitted || !direct.ownerStarted ||
		!direct.ownerFenced {
		t.Fatalf("direct admission = %+v", direct)
	}
	turn, ok := state.ownerTurns.get(direct.ownerTurn)
	if !ok || !turn.started || !turn.fenced || !turn.accepted {
		t.Fatalf("direct owner state = %+v, %t", turn, ok)
	}
	_, nested := reduceRPCControl(state, rpcControlEvent{
		kind: rpcControlOwnerDirectAdmit,
	})
	if nested.ownerAdmitted {
		t.Fatalf("fresh nested direct admission = %+v", nested)
	}
}

func TestRPCControlReducerOutOfOrderDeliveries(t *testing.T) {
	state := newRPCControlState(false)
	state, owner := reduceRPCControl(state, rpcControlEvent{
		kind: rpcControlOwnerReserve,
	})
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerFence,
		ownerTurn: owner.ownerTurn,
		accepted:  true,
	})
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerStart,
		ownerTurn: owner.ownerTurn,
	})
	state, delivery1 := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlDeliveryBegin,
		ownerTurn: owner.ownerTurn,
	})
	state, delivery2 := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlDeliveryBegin,
		ownerTurn: owner.ownerTurn,
	})
	state, end2 := reduceRPCControl(state, rpcControlEvent{
		kind:       rpcControlDeliveryEnd,
		deliveryID: delivery2.deliveryID,
	})
	if !end2.deliveryChanged || state.deliveriesDone != 0 {
		t.Fatalf("out-of-order delivery end = %+v, %+v", state, end2)
	}
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:       rpcControlDeliveryEnd,
		deliveryID: delivery1.deliveryID,
	})
	if state.deliveriesDone != delivery2.deliveryID ||
		state.deliveries.len != 0 {
		t.Fatalf("delivery compaction = %+v", state)
	}
}

func TestRPCControlReducerImmutableFinalizationRequirement(t *testing.T) {
	state := newRPCControlState(true)
	state, claim := reduceRPCControl(state, rpcControlEvent{
		kind:   rpcControlClaim,
		mode:   terminalGraceful,
		origin: terminalServer,
	})
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerStart,
		ownerTurn: claim.ownerTurn,
	})
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:       rpcControlOwnerComplete,
		terminalID: claim.terminalID,
		ownerTurn:  claim.ownerTurn,
		drained:    true,
	})
	state, _ = reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlOwnerFence,
		ownerTurn: claim.ownerTurn,
		accepted:  true,
	})
	if state.released {
		t.Fatal("released before required client finalization")
	}
	state, finalized := reduceRPCControl(state, rpcControlEvent{
		kind: rpcControlClientFinalized,
	})
	if !finalized.released || !state.released {
		t.Fatalf("finalization release = %+v, %+v", state, finalized)
	}
}

func TestRPCControlReducerOwnerWatermarksSaturate(t *testing.T) {
	maximum := ^uint64(0)
	state := newRPCControlState(false)
	state.nextOwnerTurn = maximum
	state.ownerFenced = maximum - 1
	state.ownerSettled = maximum - 1
	state.ownerCompacted = maximum - 1
	state.ownerTurns = state.ownerTurns.set(maximum, rpcOwnerTurnState{
		fenced:   true,
		complete: true,
	})
	state = state.advanceOwnerWatermarks()
	if state.ownerFenced != maximum ||
		state.ownerSettled != maximum ||
		state.ownerCompacted != maximum ||
		state.ownerTurns.len != 0 {
		t.Fatalf("saturated owner watermarks = %+v", state)
	}
}

func TestRPCControlReducerDoneAtIdentifierExhaustion(t *testing.T) {
	maximum := ^uint64(0)
	state := newRPCControlState(false)
	state.nextTerminalID = maximum
	state.nextOwnerTurn = maximum
	state.ownerFenced = maximum
	state.ownerSettled = maximum
	state.ownerCompacted = maximum
	state, done := reduceRPCControl(state, rpcControlEvent{
		kind:      rpcControlSchedulerDone,
		doneProof: true,
	})
	if !state.selected || !state.stable || state.released ||
		!state.transferGranted || !done.published ||
		!done.transferGranted || status.Code(state.err) != codes.Unavailable {
		t.Fatalf("exhausted Done = %+v, %+v", state, done)
	}
	state, release := reduceRPCControl(state, rpcControlEvent{
		kind:       rpcControlRecoveryRelease,
		terminalID: state.terminalID,
		transferID: state.transferID,
	})
	if !release.dataReleased || !state.released {
		t.Fatalf("exhausted recovery release = %+v, %+v", state, release)
	}
}

func FuzzRPCControlReducer(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6})
	f.Add([]byte{1, 1, 4, 2, 5, 3, 0})
	f.Fuzz(func(t *testing.T, events []byte) {
		state := newRPCControlState(len(events)&1 != 0)
		for _, value := range events {
			next, _ := reduceRPCControl(state, rpcControlEvent{
				kind:       rpcControlEventKind(value % byte(rpcControlEventLimit)),
				terminalID: uint64(value%3 + 1),
				ownerTurn:  uint64(value%4 + 1),
				deliveryID: uint64(value%5 + 1),
				statsID:    uint64(value%5 + 1),
				transferID: uint64(value%3 + 1),
				mode:       terminalMode(value % 2),
				origin:     terminalOrigin(value % 4),
				prepareID:  uint64(value & 1),
				accepted:   value&2 != 0,
				doneProof:  value&4 != 0,
				drained:    value&8 != 0,
				err: status.Error(
					codes.Code(value%byte(codes.Unauthenticated+1)),
					"fuzz",
				),
			})
			if state.selected && !next.selected {
				t.Fatal("terminal selection regressed")
			}
			if state.stable && !next.stable {
				t.Fatal("stable result regressed")
			}
			if state.schedulerDone && !next.schedulerDone {
				t.Fatal("scheduler proof regressed")
			}
			if state.dataReleased && !next.dataReleased {
				t.Fatal("data release regressed")
			}
			if state.released && !next.released {
				t.Fatal("release regressed")
			}
			if next.ownerFenced > next.nextOwnerTurn ||
				next.ownerSettled > next.nextOwnerTurn ||
				next.ownerCompacted > min(next.ownerFenced, next.ownerSettled) {
				t.Fatalf("invalid owner watermarks: %+v", next)
			}
			if next.deliveriesDone > next.nextDeliveryID {
				t.Fatalf("invalid obligation watermarks: %+v", next)
			}
			if next.statsDone > next.nextStatsID {
				t.Fatalf("invalid stats watermarks: %+v", next)
			}
			state = next
		}
	})
}

func sameRPCControlState(a, b rpcControlState) bool {
	return a.selected == b.selected &&
		a.preparedSelection == b.preparedSelection &&
		a.preparedPending == b.preparedPending &&
		a.stable == b.stable &&
		a.schedulerDone == b.schedulerDone &&
		a.transferGranted == b.transferGranted &&
		a.terminalFenced == b.terminalFenced &&
		a.terminalAccepted == b.terminalAccepted &&
		a.terminalCompleted == b.terminalCompleted &&
		a.dataReleased == b.dataReleased &&
		a.finalizationRequired == b.finalizationRequired &&
		a.clientFinalized == b.clientFinalized &&
		a.serverFinalizationRequired == b.serverFinalizationRequired &&
		a.serverFinalized == b.serverFinalized &&
		a.released == b.released &&
		a.nextTerminalID == b.nextTerminalID &&
		a.terminalID == b.terminalID &&
		a.nextOwnerTurn == b.nextOwnerTurn &&
		a.ownerBoundary == b.ownerBoundary &&
		a.ownerFenced == b.ownerFenced &&
		a.ownerSettled == b.ownerSettled &&
		a.ownerCompacted == b.ownerCompacted &&
		a.terminalOwner == b.terminalOwner &&
		a.nextTransferID == b.nextTransferID &&
		a.transferID == b.transferID &&
		a.prepareID == b.prepareID &&
		a.mode == b.mode &&
		a.origin == b.origin &&
		reflect.DeepEqual(a.err, b.err) &&
		a.nextDeliveryID == b.nextDeliveryID &&
		a.deliveriesDone == b.deliveriesDone &&
		a.nextStatsID == b.nextStatsID &&
		a.statsDone == b.statsDone &&
		a.ownerTurns.root == b.ownerTurns.root &&
		a.deliveries.root == b.deliveries.root &&
		a.stats.root == b.stats.root
}
