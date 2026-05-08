package timerrefclosureb77

import (
	"errors"
	"testing"
	"time"
)

type referenceState struct {
	refed         bool
	refedCount    int32
	epoch         uint64
	wakeAttempts  uint64
	wakeSuccesses uint64
}

type referenceSequence struct {
	idempotent referenceState
	unrefed    referenceState
	restored   referenceState
}

func TestOwnerReferenceSemantics(t *testing.T) {
	value := newLoop(true)
	result := make(chan referenceSequence, 1)
	if err := value.submitToQueue(func() {
		id, err := value.scheduleTimer(time.Hour, func() {})
		if err != nil || id != 1 {
			t.Errorf("owner ScheduleTimer = (%d, %v)", id, err)
			return
		}
		timerValue := value.timerMap[id]
		capture := func() referenceState {
			return referenceState{
				refed: timerValue.refed.Load(), refedCount: value.refedTimerCount.Load(),
				epoch: value.submissionEpoch.Load(), wakeAttempts: value.wakeAttempts.Load(),
				wakeSuccesses: value.wakeSuccesses.Load(),
			}
		}
		if err := value.refTimer(id); err != nil {
			t.Error(err)
		}
		sequence := referenceSequence{idempotent: capture()}
		if err := value.unrefTimer(id); err != nil {
			t.Error(err)
		}
		sequence.unrefed = capture()
		if err := value.refTimer(id); err != nil {
			t.Error(err)
		}
		if err := value.refTimer(99); err != nil {
			t.Error(err)
		}
		sequence.restored = capture()
		result <- sequence
	}); err != nil {
		t.Fatal(err)
	}
	runResult := startSourceRun(t, value)
	var got referenceSequence
	select {
	case got = <-result:
	case <-time.After(time.Second):
		t.Fatal("owner reference sequence did not return")
	}
	if got.idempotent != (referenceState{refed: true, refedCount: 1, epoch: 2}) {
		t.Fatalf("idempotent Ref = %+v", got.idempotent)
	}
	if got.unrefed != (referenceState{epoch: 3, wakeAttempts: 1, wakeSuccesses: 1}) {
		t.Fatalf("Unref = %+v", got.unrefed)
	}
	if got.restored != (referenceState{refed: true, refedCount: 1, epoch: 4, wakeAttempts: 2, wakeSuccesses: 2}) {
		t.Fatalf("restored Ref = %+v", got.restored)
	}
	if err := value.closeLoop(); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "owner reference Run") {
		t.Fatal("Run did not exit for Close")
	}
}

func TestExternalReferenceRoundTrip(t *testing.T) {
	value := newLoop(false)
	id, err := value.scheduleTimer(time.Hour, func() {})
	if err != nil || id != 1 {
		t.Fatalf("ScheduleTimer = (%d, %v)", id, err)
	}
	registered := make(chan struct{})
	stationary := make(chan struct{})
	resumeRegistration := make(chan struct{})
	resumeStationary := make(chan struct{})
	t.Cleanup(func() {
		releaseSignal(resumeRegistration)
		releaseSignal(resumeStationary)
	})
	waits := 0
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{runWait: func() {
			waits++
			switch waits {
			case 2:
				close(registered)
				<-resumeRegistration
			case 3:
				close(stationary)
				<-resumeStationary
			}
		}})
	}()
	waitSignal(t, registered, "registered timer Run wait")
	admitted := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- value.unrefTimerObserved(id, referenceObserver{
			queueAdmitted: func(uint64) { close(admitted) },
		})
	}()
	waitSignal(t, admitted, "external Unref admission")
	assertErrorBlocked(t, result)
	close(resumeRegistration)
	if err := receiveError(t, result); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, stationary, "post-Unref stationary Run wait")
	timerValue := value.timerMap[id]
	if timerValue == nil || timerValue.refed.Load() || value.refedTimerCount.Load() != 0 ||
		value.submissionEpoch.Load() != 4 || len(value.queue) != 0 || len(value.fastWakeupCh) != 0 {
		t.Fatal("external Unref did not reach a stationary source baseline")
	}
	closeResult := make(chan error, 1)
	go func() { closeResult <- value.closeLoop() }()
	close(resumeStationary)
	if !receiveBool(t, runResult, "external reference Run") {
		t.Fatal("Run did not exit for Close")
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
}

func TestQueueDrainPreservesLateFastWake(t *testing.T) {
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
	_, runResult := startSeededRun(t, value, false, lifecycleObserver{runWait: func() {
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
	if len(value.fastWakeupCh) != 1 || value.drainQueues() != 0 {
		t.Fatal("empty queue drain consumed the source-ordered late wake")
	}
	close(permitReceive)
	waitSignal(t, postWakeWait, "empty source wake turn")
	if len(value.fastWakeupCh) != 0 {
		t.Fatal("Run wake acquisition did not consume the late token")
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
	value := newLoop(true)
	var id timerID
	if err := value.submitToQueue(func() {
		var err error
		id, err = value.scheduleTimer(time.Hour, func() {})
		if err != nil || id != 1 {
			t.Errorf("owner ScheduleTimer = (%d, %v)", id, err)
			return
		}
		if err := value.unrefTimer(id); err != nil {
			t.Error(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	quiescing := make(chan struct{})
	resume := make(chan struct{})
	ownerUnref := make(chan error, 1)
	t.Cleanup(func() { releaseSignal(resume) })
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{autoExitQuiescing: func() {
			ownerUnref <- value.unrefTimer(id)
			close(quiescing)
			<-resume
		}})
	}()
	waitSignal(t, quiescing, "source auto-exit quiescing")
	if err := receiveError(t, ownerUnref); err != nil {
		t.Fatalf("owner Unref during quiescing = %v", err)
	}
	if err := value.refTimer(id); !errors.Is(err, errTerminated) {
		t.Fatalf("quiescing refTimer = %v", err)
	}
	if err := value.unrefTimer(id); !errors.Is(err, errTerminated) {
		t.Fatalf("external unrefTimer during quiescing = %v", err)
	}
	if claimed, err := value.scheduleTimer(time.Hour, func() {}); claimed != 0 || !errors.Is(err, errTerminated) {
		t.Fatalf("quiescing ScheduleTimer = (%d, %v)", claimed, err)
	}
	if err := value.registerFD(1); !errors.Is(err, errTerminated) {
		t.Fatalf("quiescing RegisterFD = %v", err)
	}
	if err := value.promisify(func() {}); !errors.Is(err, errTerminated) {
		t.Fatalf("quiescing Promisify = %v", err)
	}
	close(resume)
	if !receiveBool(t, runResult, "quiescing auto-exit Run") {
		t.Fatal("Run did not commit source auto-exit")
	}
}

func TestQuiescenceRequiresNoSupportedLiveness(t *testing.T) {
	t.Run("RefedTimer", func(t *testing.T) {
		value := newLoop(true)
		committed := make(chan struct{})
		id, err := value.scheduleTimerObserved(time.Hour, func() {}, registrationObserver{
			registrationCommitted: func() { close(committed) },
		})
		if err != nil || id != 1 {
			t.Fatalf("ScheduleTimer = (%d, %v)", id, err)
		}
		runResult := startSourceRun(t, value)
		waitSignal(t, committed, "refed timer registration")
		assertBoolBlocked(t, runResult, "refed-timer Run")
		if err := value.unrefTimer(id); err != nil {
			t.Fatal(err)
		}
		if !receiveBool(t, runResult, "unrefed-timer Run") {
			t.Fatal("Run did not auto-exit after Unref")
		}
	})

	t.Run("UserFD", func(t *testing.T) {
		value := newLoop(true)
		registered := make(chan error, 1)
		if err := value.submitToQueue(func() { registered <- value.registerFD(1) }); err != nil {
			t.Fatal(err)
		}
		runResult := startSourceRun(t, value)
		if err := receiveError(t, registered); err != nil {
			t.Fatal(err)
		}
		assertBoolBlocked(t, runResult, "user-FD Run")
		if err := value.unregisterFD(1); err != nil {
			t.Fatal(err)
		}
		if !receiveBool(t, runResult, "unregistered-FD Run") {
			t.Fatal("Run did not auto-exit after UnregisterFD")
		}
	})
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
	closeWoke := make(chan struct{})
	closeResult := make(chan error, 1)
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

func TestPhysicalWakeFailureRemainsPendingAcrossQueueDrain(t *testing.T) {
	value := newLoopWithWakeBackend(false, func() bool { return false })
	registered := make(chan error, 1)
	if err := value.submitToQueue(func() { registered <- value.registerFD(1) }); err != nil {
		t.Fatal(err)
	}
	sleeping := make(chan bool, 1)
	resumeSleeping := make(chan struct{})
	postDrain := make(chan struct{})
	resumePostDrain := make(chan struct{})
	t.Cleanup(func() {
		releaseSignal(resumeSleeping)
		releaseSignal(resumePostDrain)
	})
	wakeAcquired := 0
	waits := 0
	executed := 0
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			runWakeAcquired: func() {
				wakeAcquired++
				if wakeAcquired == 2 {
					sleeping <- value.transition(stateSleeping)
					<-resumeSleeping
				}
			},
			runWait: func() {
				waits++
				if waits == 3 {
					close(postDrain)
					<-resumePostDrain
				}
			},
		})
	}()
	if err := receiveError(t, registered); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, sleeping, "source Sleeping publication") {
		t.Fatal("Run owner did not publish Sleeping")
	}
	if err := value.submitToQueue(func() { executed++ }); err != nil {
		t.Fatal(err)
	}
	if err := value.submitToQueue(func() { executed++ }); err != nil {
		t.Fatal(err)
	}
	if len(value.queue) != 2 || value.wakeAttempts.Load() != 1 || value.wakeSuccesses.Load() != 0 ||
		value.wakeUpSignalPending.Load() != 1 || len(value.fastWakeupCh) != 1 {
		t.Fatal("failed physical wake did not remain sticky before Run drain")
	}
	close(resumeSleeping)
	waitSignal(t, postDrain, "post-failed-wake queue drain")
	if executed != 2 || len(value.queue) != 0 || value.wakeAttempts.Load() != 1 ||
		value.wakeSuccesses.Load() != 0 || value.wakeUpSignalPending.Load() != 1 || len(value.fastWakeupCh) != 1 {
		t.Fatal("queue drain repaired or lost the sticky physical wake")
	}
	assertNilSlots(t, value.spare)
	closeResult := make(chan error, 1)
	go func() { closeResult <- value.closeLoop() }()
	close(resumePostDrain)
	if !receiveBool(t, runResult, "failed-wake Close Run") {
		t.Fatal("Run did not exit for Close")
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
}

func TestDrainExhaustsNestedFIFOAndReleasesBuffers(t *testing.T) {
	value := newLoop(false)
	order := make([]int, 0, 3)
	if err := value.submitToQueue(func() {
		order = append(order, 1)
		if err := value.submitToQueue(func() { order = append(order, 3) }); err != nil {
			t.Errorf("nested admission: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := value.submitToQueue(func() { order = append(order, 2) }); err != nil {
		t.Fatal(err)
	}
	drained := make(chan struct{})
	resumeDrained := make(chan struct{})
	emptyTurn := make(chan struct{})
	resumeEmptyTurn := make(chan struct{})
	t.Cleanup(func() {
		releaseSignal(resumeDrained)
		releaseSignal(resumeEmptyTurn)
	})
	waits := 0
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{runWait: func() {
			waits++
			switch waits {
			case 2:
				close(drained)
				<-resumeDrained
			case 3:
				close(emptyTurn)
				<-resumeEmptyTurn
			}
		}})
	}()
	waitSignal(t, drained, "nested exhaustive Run drain")
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("order = %v, want [1 2 3]", order)
	}
	if len(value.queue) != 0 || len(value.fastWakeupCh) != 1 {
		t.Fatal("empty queue drain consumed nested admission wake")
	}
	assertNilSlots(t, value.spare)
	close(resumeDrained)
	waitSignal(t, emptyTurn, "nested empty source turn")
	if len(value.fastWakeupCh) != 0 {
		t.Fatal("Run did not acquire the nested admission wake")
	}
	assertNilSlots(t, value.spare)
	closeResult := make(chan error, 1)
	go func() { closeResult <- value.closeLoop() }()
	close(resumeEmptyTurn)
	if !receiveBool(t, runResult, "nested drain Close Run") {
		t.Fatal("Run did not exit for Close")
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
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

func TestStartedCloseDiscardsAcceptedWork(t *testing.T) {
	value := newLoop(false)
	secondWait := make(chan struct{})
	resumeRun := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRun) })
	waits := 0
	timerValue, runResult := startSeededRun(t, value, true, lifecycleObserver{runWait: func() {
		waits++
		if waits == 2 {
			close(secondWait)
			<-resumeRun
		}
	}})
	waitSignal(t, secondWait, "started Close Run wait")
	admitted := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- value.unrefTimerObserved(1, referenceObserver{queueAdmitted: func(uint64) { close(admitted) }})
	}()
	waitSignal(t, admitted, "accepted Unref admission")
	closeWoke := make(chan struct{})
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{closeWake: func() { close(closeWoke) }})
	}()
	waitSignal(t, closeWoke, "started Close wake")
	close(resumeRun)
	if !receiveBool(t, runResult, "started Close Run") {
		t.Fatal("Run did not exit for Close")
	}
	if err := receiveError(t, result); !errors.Is(err, errTerminated) {
		t.Fatalf("started Close result = %v", err)
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
	if value.submissionEpoch.Load() != 3 {
		t.Fatal("Close executed an accepted closure")
	}
	if len(value.timerMap) != 0 || len(value.queue) != 0 {
		t.Fatal("started Close cleanup failed")
	}
	assertSourceCleanup(t, value, timerValue)
}

func TestStartedShutdownPublishesDoneBeforeEffect(t *testing.T) {
	value := newLoop(false)
	secondWait := make(chan struct{})
	resumeRun := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRun) })
	waits := 0
	timerValue, runResult := startSeededRun(t, value, true, lifecycleObserver{runWait: func() {
		waits++
		if waits == 2 {
			close(secondWait)
			<-resumeRun
		}
	}})
	waitSignal(t, secondWait, "started Shutdown Run wait")
	admitted := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- value.unrefTimerObserved(1, referenceObserver{queueAdmitted: func(uint64) { close(admitted) }})
	}()
	waitSignal(t, admitted, "accepted Shutdown Unref admission")
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
	waitSignal(t, shutdownWoke, "started Shutdown wake")
	close(resumeRun)
	if !receiveBool(t, runResult, "started Shutdown Run") {
		t.Fatal("Run did not exit for Shutdown")
	}
	waitSignal(t, beforeCommit, "Shutdown before commit")
	if err := receiveError(t, result); !errors.Is(err, errTerminated) {
		t.Fatalf("done-before-effect result = %v", err)
	}
	if state(value.state.Load()) != stateTerminating || !timerValue.refed.Load() || len(value.queue) != 1 {
		t.Fatal("effect occurred before Shutdown terminal commit and drain")
	}
	close(resumeCommit)
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if timerValue.refed.Load() || value.submissionEpoch.Load() != 4 || len(value.timerMap) != 0 {
		t.Fatal("started Shutdown cleanup failed")
	}
}

func TestStartedShutdownAdmitsAfterLoopDone(t *testing.T) {
	value := newLoop(false)
	secondWait := make(chan struct{})
	resumeRun := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRun) })
	waits := 0
	timerValue, runResult := startSeededRun(t, value, true, lifecycleObserver{runWait: func() {
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
	result := make(chan error, 1)
	go func() { result <- value.unrefTimer(1) }()
	if err := receiveError(t, result); !errors.Is(err, errTerminated) {
		t.Fatalf("post-loopDone admission result = %v", err)
	}
	if len(value.queue) != 1 || state(value.state.Load()) != stateTerminating {
		t.Fatal("Terminating queue rejected source-real post-loopDone admission")
	}
	close(resumeCommit)
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if timerValue.refed.Load() || value.submissionEpoch.Load() != 4 {
		t.Fatal("post-loopDone admission was not drained")
	}
}

func TestExternalTerminatingWindow(t *testing.T) {
	value := newLoop(false)
	secondWait := make(chan struct{})
	resumeRun := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRun) })
	waits := 0
	timerValue, runResult := startSeededRun(t, value, true, lifecycleObserver{runWait: func() {
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
	refResult := make(chan error, 1)
	unrefResult := make(chan error, 1)
	go func() { refResult <- value.refTimer(1) }()
	if err := receiveError(t, refResult); !errors.Is(err, errTerminated) {
		t.Fatalf("external terminating refTimer = %v", err)
	}
	go func() { unrefResult <- value.unrefTimer(1) }()
	if err := receiveError(t, unrefResult); !errors.Is(err, errTerminated) {
		t.Fatalf("external terminating unrefTimer = %v", err)
	}
	if len(value.queue) != 2 || state(value.state.Load()) != stateTerminating || !timerValue.refed.Load() {
		t.Fatal("real terminating window did not retain accepted FIFO work")
	}
	close(resumeCommit)
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if timerValue.refed.Load() || value.submissionEpoch.Load() != 5 || len(value.queue) != 0 {
		t.Fatal("Shutdown did not drain terminating-window work in order")
	}
}

func TestQuiescingDoesNotAbsorbPriorIngress(t *testing.T) {
	value := newLoop(true)
	observed := make(chan uint64, 1)
	resumeObservation := make(chan struct{})
	executed := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeObservation) })
	observations := 0
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{autoExitObserved: func(epoch uint64) {
			observations++
			if observations == 1 {
				observed <- epoch
				<-resumeObservation
			}
		}})
	}()
	select {
	case epoch := <-observed:
		if epoch != 0 {
			t.Fatalf("first auto-exit observation = %d, want 0", epoch)
		}
	case <-time.After(time.Second):
		t.Fatal("auto-exit observation did not occur")
	}
	if err := value.submitToQueue(func() { close(executed) }); err != nil {
		t.Fatal(err)
	}
	if value.submissionEpoch.Load() != 1 || len(value.queue) != 1 {
		t.Fatal("prior ingress was not admitted after the auto-exit observation")
	}
	close(resumeObservation)
	waitSignal(t, executed, "prior ingress execution")
	if !receiveBool(t, runResult, "prior-ingress auto-exit Run") {
		t.Fatal("Run did not retry and commit auto-exit")
	}
	if observations < 2 || value.submissionEpoch.Load() != 1 || len(value.queue) != 0 {
		t.Fatal("quiescing absorbed or lost previously admitted ingress")
	}
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
