package timerrefclosuresimple

import (
	"errors"
	"sort"
	"sync"
	"testing"
	"time"
)

type admissionResult struct {
	id    int
	epoch uint64
	err   error
}

func TestOwnerReferenceSemantics(t *testing.T) {
	loop := newLoop(true)
	if !loop.bindOwner() || !loop.seed(1, true) {
		t.Fatal("owner setup failed")
	}
	if err := loop.refTimer(1); err != nil {
		t.Fatal(err)
	}
	if got := loop.snapshot(1); got.submissionEpoch != 0 || got.refedCount != 1 || !got.refed {
		t.Fatalf("idempotent ref snapshot = %+v", got)
	}
	if err := loop.unrefTimer(1); err != nil {
		t.Fatal(err)
	}
	if got := loop.snapshot(1); got.submissionEpoch != 1 || got.refedCount != 0 || got.refed || got.wakeAttempts != 1 || got.wakeSuccesses != 1 {
		t.Fatalf("unref snapshot = %+v", got)
	}
	if err := loop.refTimer(1); err != nil {
		t.Fatal(err)
	}
	if err := loop.refTimer(99); err != nil {
		t.Fatal(err)
	}
	if got := loop.snapshot(1); got.submissionEpoch != 2 || got.refedCount != 1 || !got.refed || got.wakeAttempts != 2 || got.wakeSuccesses != 2 {
		t.Fatalf("restored snapshot = %+v", got)
	}
}

func TestExternalReferenceRoundTrip(t *testing.T) {
	loop := newLoop(true)
	if !loop.bindOwner() || !loop.seed(1, true) {
		t.Fatal("owner setup failed")
	}
	result := make(chan error, 1)
	go func() { result <- loop.unrefTimer(1) }()
	waitSignal(t, loop.fastWakeupCh, "external admission wake")
	assertErrorBlocked(t, result)
	if drained := loop.drain(); drained != 1 {
		t.Fatalf("drained = %d, want 1", drained)
	}
	if err := receiveError(t, result); err != nil {
		t.Fatal(err)
	}
	if got := loop.snapshot(1); got.refed || got.refedCount != 0 || got.submissionEpoch != 2 || got.wakeAttempts != 1 || got.fastWakePending != 0 {
		t.Fatalf("external snapshot = %+v", got)
	}
}

func TestFinishWinsAfterAdmissionBeforeWake(t *testing.T) {
	value := newLoop(false)
	secondWait := make(chan struct{})
	resumeRun := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRun) })
	waits := 0
	timerValue, runResult := startSeededLoop(t, value, true, lifecycleObserver{runWait: func() {
		waits++
		if waits == 2 {
			close(secondWait)
			<-resumeRun
		}
	}})
	waitSignal(t, secondWait, "post-seed Run wait")

	admitted := make(chan struct{})
	resumeIngress := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeIngress) })
	result := make(chan error, 1)
	go func() {
		result <- value.unrefTimerObserved(1, referenceObserver{queueAdmitted: func(uint64) {
			close(admitted)
			<-resumeIngress
		}})
	}()
	waitSignal(t, admitted, "Unref admission before wake")
	closeWoke := make(chan struct{})
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{closeWake: func() { close(closeWoke) }})
	}()
	waitSignal(t, closeWoke, "started Close wake")
	close(resumeRun)
	if !receiveBool(t, runResult, "Close Run") {
		t.Fatal("Run did not exit for Close")
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
	assertErrorBlocked(t, result)
	close(resumeIngress)
	if err := receiveError(t, result); !errors.Is(err, errTerminated) {
		t.Fatalf("discarded Unref = %v", err)
	}
	if !timerValue.refed.Load() || len(value.queue) != 0 || len(value.fastWakeupCh) != 0 ||
		value.wakeUpSignalPending.Load() != 0 || value.wakeAttempts.Load() != 1 {
		t.Fatal("late ingress wake repopulated terminal state or discarded work executed")
	}
}

func TestDrainBeforePostAdmissionWake(t *testing.T) {
	value := newLoop(false)
	secondWait := make(chan struct{})
	admissionReady := make(chan struct{})
	drained := make(chan int, 1)
	permitReceive := make(chan struct{})
	postWakeWait := make(chan struct{})
	t.Cleanup(func() {
		releaseSignal(admissionReady)
		releaseSignal(permitReceive)
	})
	waits := 0
	_, runResult := startSeededLoop(t, value, false, lifecycleObserver{runWait: func() {
		waits++
		switch waits {
		case 2:
			close(secondWait)
			<-admissionReady
			drained <- value.drainQueues()
			<-permitReceive
		case 3:
			close(postWakeWait)
		}
	}})
	waitSignal(t, secondWait, "owner wait before observed admission")

	admitted := make(chan struct{})
	resumeIngress := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeIngress) })
	executed := 0
	submitResult := make(chan error, 1)
	go func() {
		submitResult <- value.submitToQueueObserved(func() { executed++ }, referenceObserver{
			queueAdmitted: func(uint64) {
				close(admitted)
				<-resumeIngress
			},
		})
	}()
	waitSignal(t, admitted, "task admission before wake")
	close(admissionReady)
	select {
	case count := <-drained:
		if count != 1 || executed != 1 {
			t.Fatalf("owner drain = %d, executed = %d", count, executed)
		}
	case <-time.After(time.Second):
		t.Fatal("owner did not drain admitted work")
	}
	assertErrorBlocked(t, submitResult)
	close(resumeIngress)
	if err := receiveError(t, submitResult); err != nil {
		t.Fatal(err)
	}
	if len(value.fastWakeupCh) != 1 {
		t.Fatal("repaired control did not retain the post-drain wake turn")
	}
	close(permitReceive)
	waitSignal(t, postWakeWait, "empty normalized wake turn")
	if len(value.fastWakeupCh) != 0 {
		t.Fatal("empty wake turn did not restore the control baseline")
	}
	closeResult := make(chan error, 1)
	go func() { closeResult <- value.closeLoop() }()
	if !receiveBool(t, runResult, "post-admission Run") {
		t.Fatal("Run did not exit for Close")
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
}

func TestQuiescingAndTerminatingAsymmetry(t *testing.T) {
	loop := newLoop(true)
	if !loop.bindOwner() || !loop.seed(1, false) {
		t.Fatal("owner setup failed")
	}
	if !loop.beginQuiescing(loop.snapshot(1).submissionEpoch) {
		t.Fatal("BeginQuiescing failed")
	}
	if err := loop.refTimer(1); !errors.Is(err, errTerminated) {
		t.Fatalf("quiescing refTimer = %v", err)
	}
	if err := loop.unrefTimer(1); err != nil {
		t.Fatalf("owner unrefTimer during quiescing = %v", err)
	}
	result := make(chan error, 1)
	go func() { result <- loop.unrefTimer(1) }()
	if err := receiveError(t, result); !errors.Is(err, errTerminated) {
		t.Fatalf("external unrefTimer during quiescing = %v", err)
	}
	if !loop.endQuiescing() {
		t.Fatal("EndQuiescing failed")
	}
}

func TestQuiescenceRequiresNoSupportedLiveness(t *testing.T) {
	refed := newLoop(true)
	if !refed.bindOwner() || !refed.seed(1, true) {
		t.Fatal("refed setup failed")
	}
	if refed.beginQuiescing(refed.snapshot(1).submissionEpoch) {
		t.Fatal("quiescing began with a refed timer")
	}
	withFD := newLoop(true)
	if !withFD.bindOwner() || !withFD.seed(1, false) || !withFD.configureUserFDCount(1) {
		t.Fatal("FD setup failed")
	}
	if withFD.beginQuiescing(withFD.snapshot(1).submissionEpoch) {
		t.Fatal("quiescing began with a live user FD")
	}
	quiescing := newLoop(true)
	if !quiescing.bindOwner() || !quiescing.seed(1, false) || !quiescing.beginQuiescing(0) {
		t.Fatal("empty quiescence setup failed")
	}
	if quiescing.configureUserFDCount(1) || quiescing.transition(stateSleeping) {
		t.Fatal("active quiescence admitted new liveness or sleep")
	}
}

func TestRunPublicationPhasesAndReset(t *testing.T) {
	value := newLoop(false)
	waiting := make(chan struct{})
	resumeRun := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRun) })
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{runWait: func() {
			close(waiting)
			<-resumeRun
		}})
	}()
	waitSignal(t, waiting, "Run owner publication")
	if state(value.state.Load()) != stateRunning || value.ownerID.Load() == 0 || value.activeRun == nil {
		t.Fatal("Run did not publish its active owner")
	}
	closeResult := make(chan error, 1)
	closeWoke := make(chan struct{})
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{closeWake: func() { close(closeWoke) }})
	}()
	waitSignal(t, closeWoke, "Close wake")
	close(resumeRun)
	if !receiveBool(t, runResult, "published-owner Run") {
		t.Fatal("Run did not observe Close")
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
	if value.ownerID.Load() != 0 || value.activeRun != nil {
		t.Fatal("Close did not publish Run exit and clear owner")
	}
	assertChannelClosed(t, value.loopDone, "loopDone after Close")
}

func TestTerminalCanInterveneBeforeOwnerPublication(t *testing.T) {
	value := newLoop(false)
	claimed := make(chan struct{})
	resumeRun := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRun) })
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{runClaimed: func() {
			close(claimed)
			<-resumeRun
		}})
	}()
	waitSignal(t, claimed, "Running claim before owner publication")
	if state(value.state.Load()) != stateRunning || value.ownerID.Load() != 0 || value.activeRun == nil {
		t.Fatal("Running-with-owner-zero publication window missing")
	}
	closeWoke := make(chan struct{})
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{closeWake: func() { close(closeWoke) }})
	}()
	waitSignal(t, closeWoke, "intervening Close wake")
	close(resumeRun)
	if !receiveBool(t, runResult, "intervened Run") {
		t.Fatal("Run did not publish its owner and observe Close")
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
	if value.ownerID.Load() != 0 || value.activeRun != nil || state(value.state.Load()) != stateTerminated {
		t.Fatal("intervening Close did not complete")
	}
}

func TestBindOwnerPublishesRunStartOnLostCAS(t *testing.T) {
	value := newLoop(false)
	if err := value.closeLoop(); err != nil {
		t.Fatal(err)
	}
	if value.run() || value.ownerID.Load() != 0 {
		t.Fatal("lost Running CAS published an owner")
	}
	assertChannelClosed(t, value.runCh, "runCh after lost Running CAS")
}

func TestShutdownWinsRunCAS(t *testing.T) {
	value := newLoop(false)
	started := make(chan struct{})
	resumeRun := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRun) })
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{runStarted: func() {
			close(started)
			<-resumeRun
		}})
	}()
	waitSignal(t, started, "Run start publication")
	if err := value.shutdown(); err != nil {
		t.Fatal(err)
	}
	close(resumeRun)
	if receiveBool(t, runResult, "Awake Shutdown Run") || value.ownerID.Load() != 0 || value.activeRun != nil {
		t.Fatal("Awake Shutdown winner permitted a Run generation")
	}
	assertChannelClosed(t, value.loopDone, "Awake Shutdown loopDone")
}

func TestBinaryWakeFailureResetAndRetry(t *testing.T) {
	loop := newLoop(false)
	if !loop.bindOwner() || !loop.seed(1, true) || !loop.configureUserFDCount(1) || !loop.transition(stateSleeping) || !loop.configureWakeFailure(true) {
		t.Fatal("sleeping failure setup failed")
	}
	if err := loop.submitToQueue(func() {}); err != nil {
		t.Fatal(err)
	}
	if err := loop.submitToQueue(func() {}); err != nil {
		t.Fatal(err)
	}
	if got := loop.snapshot(1); got.queued != 2 || got.wakeAttempts != 1 || got.wakeSuccesses != 0 || !got.wakePending {
		t.Fatalf("sticky failure snapshot = %+v", got)
	}
	if !loop.transition(stateRunning) || loop.drain() != 2 {
		t.Fatal("failed wake queue did not normalize")
	}
	if !loop.configureWakeFailure(false) || !loop.transition(stateSleeping) {
		t.Fatal("wake retry setup failed")
	}
	if err := loop.submitToQueue(func() {}); err != nil {
		t.Fatal(err)
	}
	if got := loop.snapshot(1); got.wakeAttempts != 2 || got.wakeSuccesses != 1 || !got.wakePending {
		t.Fatalf("post-reset retry snapshot = %+v", got)
	}
	if !loop.transition(stateRunning) || loop.drain() != 1 {
		t.Fatal("successful retry did not normalize")
	}
}

func TestDrainDoubleBufferFIFOAndRelease(t *testing.T) {
	loop := newLoop(false)
	if !loop.bindOwner() {
		t.Fatal("BindOwner failed")
	}
	order := make([]int, 0, 3)
	if err := loop.submitToQueue(func() {
		order = append(order, 1)
		if err := loop.submitToQueue(func() { order = append(order, 3) }); err != nil {
			t.Errorf("nested admission: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := loop.submitToQueue(func() { order = append(order, 2) }); err != nil {
		t.Fatal(err)
	}
	if loop.drain() != 2 {
		t.Fatal("first detached batch mismatch")
	}
	if got := loop.snapshot(0); got.queued != 1 || got.fastWakePending != 1 {
		t.Fatalf("next-batch wake = %+v", got)
	}
	assertNilSlots(t, loop.spare)
	if loop.drain() != 1 {
		t.Fatal("second detached batch mismatch")
	}
	assertNilSlots(t, loop.spare)
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("order = %v, want [1 2 3]", order)
	}
}

func TestMultiProducerFIFOByAdmissionEpoch(t *testing.T) {
	loop := newLoop(false)
	if !loop.bindOwner() {
		t.Fatal("owner setup failed")
	}
	const producers = 32
	records := make(chan admissionResult, producers)
	executed := make([]int, 0, producers)
	var start sync.WaitGroup
	start.Add(1)
	for id := range producers {
		go func() {
			start.Wait()
			record := admissionResult{id: id}
			record.err = loop.submitToQueueObserved(
				func() { executed = append(executed, id) },
				referenceObserver{queueAdmitted: func(epoch uint64) { record.epoch = epoch }},
			)
			records <- record
		}()
	}
	start.Done()
	want := make([]admissionResult, 0, producers)
	for range producers {
		value := receiveAdmission(t, records)
		if value.err != nil {
			t.Fatal(value.err)
		}
		want = append(want, value)
	}
	sort.Slice(want, func(left, right int) bool { return want[left].epoch < want[right].epoch })
	if loop.drain() != producers || len(executed) != producers {
		t.Fatal("multi-producer batch was not executed exactly once")
	}
	for index, value := range want {
		if value.epoch != uint64(index+1) || executed[index] != value.id {
			t.Fatalf("position %d: admission=%+v execution=%d", index, value, executed[index])
		}
	}
	assertNilSlots(t, loop.spare)
}

func TestAwakeCloseDiscardsRunWindowAdmission(t *testing.T) {
	value := newLoop(false)
	started := make(chan struct{})
	resumeRun := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRun) })
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{runStarted: func() {
			close(started)
			<-resumeRun
		}})
	}()
	waitSignal(t, started, "Awake Run publication")
	admitted := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- value.unrefTimerObserved(1, referenceObserver{queueAdmitted: func(uint64) { close(admitted) }})
	}()
	waitSignal(t, admitted, "Awake run-window admission")
	if err := value.closeLoop(); err != nil {
		t.Fatal(err)
	}
	if err := receiveError(t, result); !errors.Is(err, errTerminated) {
		t.Fatalf("Awake admitted call = %v", err)
	}
	close(resumeRun)
	if receiveBool(t, runResult, "Awake Close Run") || value.activeRun != nil || len(value.queue) != 0 {
		t.Fatal("Awake Close did not permanently discard admission")
	}
}

func TestStartedShutdownLoopDoneRechecksAcceptedResult(t *testing.T) {
	value := newLoop(false)
	secondWait := make(chan struct{})
	resumeRun := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRun) })
	waits := 0
	timerValue, runResult := startSeededLoop(t, value, true, lifecycleObserver{runWait: func() {
		waits++
		if waits == 2 {
			close(secondWait)
			<-resumeRun
		}
	}})
	waitSignal(t, secondWait, "started Shutdown Run wait")
	admitted := make(chan struct{})
	loopDoneSelected := make(chan struct{})
	resumeReference := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeReference) })
	result := make(chan error, 1)
	go func() {
		result <- value.unrefTimerObserved(1, referenceObserver{
			queueAdmitted: func(uint64) { close(admitted) },
			loopDoneSelected: func() {
				close(loopDoneSelected)
				<-resumeReference
			},
		})
	}()
	waitSignal(t, admitted, "accepted Shutdown Unref admission")
	beforeCommit := make(chan struct{})
	resumeCommit := make(chan struct{})
	afterDrain := make(chan struct{})
	resumeShutdown := make(chan struct{})
	shutdownWoke := make(chan struct{})
	t.Cleanup(func() {
		releaseSignal(resumeCommit)
		releaseSignal(resumeShutdown)
	})
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			shutdownWake: func() { close(shutdownWoke) },
			beforeShutdownCommit: func() {
				close(beforeCommit)
				<-resumeCommit
			},
			afterShutdownDrain: func() {
				close(afterDrain)
				<-resumeShutdown
			},
		})
	}()
	waitSignal(t, shutdownWoke, "started Shutdown wake")
	close(resumeRun)
	if !receiveBool(t, runResult, "started Shutdown Run") {
		t.Fatal("Run did not exit for Shutdown")
	}
	waitSignal(t, loopDoneSelected, "accepted reference loopDone selection")
	waitSignal(t, beforeCommit, "Shutdown before commit")
	assertErrorBlocked(t, result)
	if state(value.state.Load()) != stateTerminating || !timerValue.refed.Load() || len(value.queue) != 1 {
		t.Fatal("effect occurred before Shutdown terminal commit and drain")
	}
	close(resumeCommit)
	waitSignal(t, afterDrain, "Shutdown accepted-work drain")
	if timerValue.refed.Load() || value.submissionEpoch.Load() != 3 {
		t.Fatal("Shutdown drain did not execute accepted Unref")
	}
	close(resumeReference)
	if err := receiveError(t, result); err != nil {
		t.Fatalf("accepted Shutdown Unref = %v", err)
	}
	assertErrorBlocked(t, shutdownResult)
	close(resumeShutdown)
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if len(value.timerMap) != 0 {
		t.Fatal("started Shutdown cleanup failed")
	}
}

func TestStartedShutdownAdmitsAfterLoopDone(t *testing.T) {
	value := newLoop(false)
	secondWait := make(chan struct{})
	resumeRun := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRun) })
	waits := 0
	timerValue, runResult := startSeededLoop(t, value, true, lifecycleObserver{runWait: func() {
		waits++
		if waits == 2 {
			close(secondWait)
			<-resumeRun
		}
	}})
	waitSignal(t, secondWait, "post-loopDone Run wait")
	beforeCommit := make(chan struct{})
	resumeCommit := make(chan struct{})
	shutdownWoke := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeCommit) })
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			shutdownWake: func() { close(shutdownWoke) },
			beforeShutdownCommit: func() {
				close(beforeCommit)
				<-resumeCommit
			},
		})
	}()
	waitSignal(t, shutdownWoke, "post-loopDone Shutdown wake")
	close(resumeRun)
	if !receiveBool(t, runResult, "post-loopDone Shutdown Run") {
		t.Fatal("Run did not exit for Shutdown")
	}
	waitSignal(t, beforeCommit, "post-loopDone Shutdown commit boundary")
	assertChannelClosed(t, value.loopDone, "started Shutdown loopDone")
	terminalOwnerResult := make(chan error, 1)
	if err := value.submitToQueue(func() { terminalOwnerResult <- value.refTimer(1) }); err != nil {
		t.Fatal(err)
	}
	admitted := make(chan struct{})
	terminalWait := make(chan struct{})
	resumeReference := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeReference) })
	result := make(chan error, 1)
	go func() {
		result <- value.unrefTimerObserved(1, referenceObserver{
			queueAdmitted: func(uint64) { close(admitted) },
			terminalWait: func() {
				close(terminalWait)
				<-resumeReference
			},
		})
	}()
	waitSignal(t, admitted, "post-loopDone Unref admission")
	waitSignal(t, terminalWait, "post-loopDone terminal wait")
	assertErrorBlocked(t, result)
	if len(value.queue) != 2 || state(value.state.Load()) != stateTerminating {
		t.Fatal("repaired control rejected accepted post-loopDone work")
	}
	close(resumeCommit)
	if err := receiveError(t, terminalOwnerResult); !errors.Is(err, errTerminated) {
		t.Fatalf("Shutdown-drain owner reference = %v", err)
	}
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if timerValue.refed.Load() || value.submissionEpoch.Load() != 4 || value.activeTerminal != nil {
		t.Fatal("post-loopDone admission was not drained")
	}
	close(resumeReference)
	if err := receiveError(t, result); err != nil {
		t.Fatalf("post-loopDone accepted Unref = %v", err)
	}
}

func TestExternalTerminatingWindowJoinsAcceptedFIFO(t *testing.T) {
	value := newLoop(false)
	secondWait := make(chan struct{})
	resumeRun := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRun) })
	waits := 0
	timerValue, runResult := startSeededLoop(t, value, true, lifecycleObserver{runWait: func() {
		waits++
		if waits == 2 {
			close(secondWait)
			<-resumeRun
		}
	}})
	waitSignal(t, secondWait, "terminating-window Run wait")
	shutdownWoke := make(chan struct{})
	beforeCommit := make(chan struct{})
	resumeCommit := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeCommit) })
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			shutdownWake: func() { close(shutdownWoke) },
			beforeShutdownCommit: func() {
				close(beforeCommit)
				<-resumeCommit
			},
		})
	}()
	waitSignal(t, shutdownWoke, "terminating-window Shutdown wake")
	close(resumeRun)
	if !receiveBool(t, runResult, "terminating-window Run") {
		t.Fatal("Run did not exit for Shutdown")
	}
	waitSignal(t, beforeCommit, "terminating-window commit boundary")
	unrefResult := make(chan error, 1)
	unrefAdmitted := make(chan struct{})
	go func() {
		unrefResult <- value.unrefTimerObserved(1, referenceObserver{
			queueAdmitted: func(uint64) { close(unrefAdmitted) },
		})
	}()
	waitSignal(t, unrefAdmitted, "terminating-window Unref admission")
	assertErrorBlocked(t, unrefResult)
	refResult := make(chan error, 1)
	refAdmitted := make(chan struct{})
	go func() {
		refResult <- value.refTimerObserved(1, referenceObserver{
			queueAdmitted: func(uint64) { close(refAdmitted) },
		})
	}()
	waitSignal(t, refAdmitted, "terminating-window Ref admission")
	assertErrorBlocked(t, refResult)
	if len(value.queue) != 2 || state(value.state.Load()) != stateTerminating || !timerValue.refed.Load() {
		t.Fatal("terminating window did not retain accepted FIFO work")
	}
	close(resumeCommit)
	if err := receiveError(t, unrefResult); err != nil {
		t.Fatalf("accepted terminating Unref = %v", err)
	}
	if err := receiveError(t, refResult); err != nil {
		t.Fatalf("accepted terminating Ref = %v", err)
	}
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if !timerValue.refed.Load() || value.submissionEpoch.Load() != 5 || len(value.queue) != 0 {
		t.Fatal("Shutdown did not drain terminating-window work in order")
	}
}

func TestQuiescingDoesNotAbsorbPriorIngress(t *testing.T) {
	loop := newLoop(true)
	if !loop.bindOwner() || !loop.seed(1, false) {
		t.Fatal("owner setup failed")
	}
	observed := loop.snapshot(1).submissionEpoch
	result := make(chan error, 1)
	go func() { result <- loop.unrefTimer(1) }()
	waitSignal(t, loop.fastWakeupCh, "prior ingress admission")
	if loop.beginQuiescing(observed) {
		t.Fatal("quiescing absorbed already-admitted work")
	}
	if loop.drain() != 1 {
		t.Fatal("prior ingress was not drained")
	}
	if err := receiveError(t, result); err != nil {
		t.Fatal(err)
	}
}

func TestRepeatedDrainReturnsStationaryBaseline(t *testing.T) {
	loop := newLoop(false)
	if !loop.bindOwner() || !loop.seed(1, true) {
		t.Fatal("owner setup failed")
	}
	wantCapacity := -1
	for iteration := range 6 {
		result := make(chan error, 1)
		if iteration%2 == 0 {
			go func() { result <- loop.unrefTimer(1) }()
		} else {
			go func() { result <- loop.refTimer(1) }()
		}
		waitSignal(t, loop.fastWakeupCh, "stationary admission")
		if loop.drain() != 1 {
			t.Fatalf("iteration %d drain mismatch", iteration)
		}
		if err := receiveError(t, result); err != nil {
			t.Fatal(err)
		}
		if got := loop.snapshot(1); got.queued != 0 || got.fastWakePending != 0 || got.wakePending {
			t.Fatalf("iteration %d baseline = %+v", iteration, got)
		}
		capacity := cap(loop.queue) + cap(loop.spare)
		if iteration == 1 {
			wantCapacity = capacity
		} else if iteration > 1 && capacity != wantCapacity {
			t.Fatalf("iteration %d queue capacity = %d, want %d", iteration, capacity, wantCapacity)
		}
	}
}

func FuzzOwnerReferenceState(f *testing.F) {
	f.Add([]byte{0, 1, 0, 1, 2, 3})
	f.Add([]byte{1, 1, 0, 0, 3, 2})
	f.Fuzz(func(t *testing.T, operations []byte) {
		if len(operations) > 64 {
			operations = operations[:64]
		}
		loop := newLoop(true)
		if !loop.bindOwner() || !loop.seed(1, false) {
			t.Fatal("owner setup failed")
		}
		lastEpoch := uint64(0)
		for _, operation := range operations {
			switch operation % 4 {
			case 0:
				if err := loop.refTimer(1); err != nil {
					t.Fatal(err)
				}
			case 1:
				if err := loop.unrefTimer(1); err != nil {
					t.Fatal(err)
				}
			case 2:
				if err := loop.refTimer(99); err != nil {
					t.Fatal(err)
				}
			case 3:
				if err := loop.unrefTimer(99); err != nil {
					t.Fatal(err)
				}
			}
			snapshot := loop.snapshot(1)
			if snapshot.submissionEpoch < lastEpoch {
				t.Fatal("submission epoch regressed")
			}
			lastEpoch = snapshot.submissionEpoch
			wantCount := int64(0)
			if snapshot.refed {
				wantCount = 1
			}
			if !snapshot.present || snapshot.refedCount != wantCount {
				t.Fatalf("membership/count mismatch: %+v", snapshot)
			}
		}
	})
}

func receiveAdmission(t *testing.T, result <-chan admissionResult) admissionResult {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(time.Second):
		t.Fatal("admission result did not return")
		return admissionResult{}
	}
}

func waitSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("%s did not fire", name)
	}
}

func assertErrorBlocked(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		t.Fatalf("call returned before owner drain: %v", err)
	default:
	}
}

func receiveError(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("call did not return")
		return nil
	}
}

func receiveBool(t *testing.T, result <-chan bool, name string) bool {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(time.Second):
		t.Fatalf("%s did not return", name)
		return false
	}
}

func assertChannelClosed(t *testing.T, channel <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-channel:
	case <-time.After(time.Second):
		t.Fatalf("%s did not close", name)
	}
}

func assertNilSlots(t *testing.T, values []func()) {
	t.Helper()
	for index, value := range values[:cap(values)] {
		if value != nil {
			t.Fatalf("retained closure at slot %d", index)
		}
	}
}
