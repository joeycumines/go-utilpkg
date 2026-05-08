package timerrefclosure0def

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type sourceTimerSetup struct {
	value *timer
}

func TestSourceStartedShutdownWrapperCleansAfterGeneration(t *testing.T) {
	value := newLoop(false)
	runResult := startSourceRun(t, value)
	timerValue := seedSourceTimer(t, value, true)
	callbackStarted := make(chan struct{})
	callbackRelease, releaseCallback := newSourceRelease(t)
	if err := value.submitToQueue(func() {
		close(callbackStarted)
		<-callbackRelease
	}); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, callbackStarted, "blocking callback")
	baselineAttempts := value.wakeAttempts.Load()
	baselineSuccesses := value.wakeSuccesses.Load()
	baselineRejections := value.wakeRejections.Load()
	wakePublished := make(chan struct{})
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			shutdownWake: func() { close(wakePublished) },
		})
	}()
	waitSignal(t, wakePublished, "started Shutdown wake")
	if value.wakeAttempts.Load() != baselineAttempts+1 || value.wakeSuccesses.Load() != baselineSuccesses+1 || value.wakeRejections.Load() != baselineRejections {
		t.Fatal("started Shutdown physical wake classification differs")
	}
	releaseCallback()
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "0def Run exit") {
		t.Fatal("0def Run did not own its terminal exit")
	}
	assertSourceCleanupWake(t, value, timerValue, 1)
}

func TestSourceSleepingShutdownPublishesPhysicalWake(t *testing.T) {
	value := newLoop(false)
	sleepWait := make(chan struct{}, 1)
	terminalTransition := make(chan struct{})
	resumeTransition, releaseTransition := newSourceRelease(t)
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			runWait: func() {
				if state(value.state.Load()) == stateSleeping {
					select {
					case sleepWait <- struct{}{}:
					default:
					}
				}
			},
			terminalTransition: func() {
				close(terminalTransition)
				<-resumeTransition
			},
		})
	}()
	waitSignal(t, value.runCh, "0def Run start")
	sleepResult := make(chan bool, 1)
	if err := value.submitToQueue(func() {
		sleepResult <- value.transition(stateSleeping)
	}); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, sleepResult, "owner Sleeping transition") {
		t.Fatal("owner did not enter Sleeping")
	}
	waitSignal(t, sleepWait, "Run Sleeping wait")
	wakePublished := make(chan struct{})
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			shutdownWake: func() { close(wakePublished) },
		})
	}()
	waitSignal(t, terminalTransition, "Run terminal transition")
	waitSignal(t, wakePublished, "Sleeping Shutdown wake")
	if value.wakeAttempts.Load() != 1 || value.wakeSuccesses.Load() != 1 || value.wakeRejections.Load() != 0 {
		t.Fatal("Sleeping Shutdown did not publish one successful physical wake")
	}
	releaseTransition()
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "0def Sleeping Shutdown Run") {
		t.Fatal("Run did not complete")
	}
	assertSourceCleanup(t, value, nil)
}

func TestSourceStartedCloseWrapperCleansAfterRunExit(t *testing.T) {
	value := newLoop(false)
	runResult, runWaits := startSourceWaitingRun(t, value, lifecycleObserver{})
	timerValue := seedSourceTimer(t, value, true)
	waitSignal(t, runWaits, "Run wait after timer seed")
	if err := value.closeLoop(); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "0def Run exit") {
		t.Fatal("0def Run did not own its Close exit")
	}
	if value.wakeAttempts.Load() != 1 || value.wakeSuccesses.Load() != 1 || value.wakeRejections.Load() != 0 {
		t.Fatal("started Close did not preserve its forced physical wake")
	}
	assertSourceCleanup(t, value, timerValue)
}

func TestSourceAwakeShutdownDrainsButCloseDiscards(t *testing.T) {
	t.Run("Shutdown", func(t *testing.T) {
		value := newLoop(false)
		executed := make(chan struct{})
		id, err := value.prepareTimerRegistration()
		if err != nil || id != 1 {
			t.Fatalf("Awake registration = (%d, %v)", id, err)
		}
		if err := value.submitToQueue(func() { close(executed) }); err != nil {
			t.Fatal(err)
		}
		if err := value.shutdown(); err != nil {
			t.Fatal(err)
		}
		assertChannelClosed(t, executed, "Awake Shutdown sentinel")
		assertSourceCleanupWake(t, value, nil, 1)
	})

	t.Run("Close", func(t *testing.T) {
		value := newLoop(false)
		executed := make(chan struct{})
		id, err := value.prepareTimerRegistration()
		if err != nil || id != 1 {
			t.Fatalf("Awake registration = (%d, %v)", id, err)
		}
		if err := value.submitToQueue(func() { close(executed) }); err != nil {
			t.Fatal(err)
		}
		if err := value.closeLoop(); err != nil {
			t.Fatal(err)
		}
		assertChannelOpen(t, executed, "Awake Close sentinel")
		assertSourceCleanupWake(t, value, nil, 1)
	})
}

func TestSourceAwakeShutdownWaitsWorkerBeforeDrain(t *testing.T) {
	value := newLoop(false)
	finishWorker, ok := value.beginPromisifyWorker()
	if !ok {
		t.Fatal("Awake worker was not admitted")
	}
	t.Cleanup(finishWorker)
	executed := make(chan struct{})
	if err := value.submitToQueue(func() { close(executed) }); err != nil {
		t.Fatal(err)
	}
	waiting := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- value.shutdownObserved(lifecycleObserver{
			workerWait: func() { close(waiting) },
		})
	}()
	waitSignal(t, waiting, "Awake Shutdown worker barrier")
	assertChannelOpen(t, executed, "Awake queue before worker release")
	assertChannelOpen(t, value.terminalDone, "terminalDone before Awake worker release")
	assertChannelOpen(t, value.loopDone, "loopDone before Awake worker release")
	assertErrorBlocked(t, result)
	finishWorker()
	waitSignal(t, executed, "Awake queue after worker release")
	if err := receiveError(t, result); err != nil {
		t.Fatal(err)
	}
	assertSourceCleanupWake(t, value, nil, 1)
}

func TestSourceStartedShutdownWorkerBarrierOrder(t *testing.T) {
	value := newLoop(false)
	runResult, runWaits := startSourceWaitingRun(t, value, lifecycleObserver{})
	timerValue := seedSourceTimer(t, value, true)
	waitSignal(t, runWaits, "Run wait after timer seed")
	finishWorker, ok := value.beginPromisifyWorker()
	if !ok {
		t.Fatal("valid worker was not admitted")
	}
	t.Cleanup(finishWorker)
	shutdownResult := make(chan error, 1)
	go func() { shutdownResult <- value.shutdown() }()
	waitSignal(t, value.loopDone, "Run completion before worker barrier")
	assertChannelOpen(t, value.terminalDone, "terminalDone before worker release")
	assertErrorBlocked(t, shutdownResult)
	if len(value.timerMap) != 0 || timerValue.task != nil || timerValue.refed.Load() || !timerValue.canceled.Load() || value.refedTimerCount.Load() != 0 || value.userIOFDCount.Load() != 0 {
		t.Fatal("started Shutdown did not clean before its worker barrier")
	}
	finishWorker()
	waitSignal(t, value.terminalDone, "terminalDone after worker release")
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "0def worker-barrier Run") {
		t.Fatal("Run did not own graceful termination")
	}
	assertSourceCleanup(t, value, timerValue)
}

func TestSourceStartedCloseWaitsWorkerBeforeCleanup(t *testing.T) {
	value := newLoop(false)
	runResult, runWaits := startSourceWaitingRun(t, value, lifecycleObserver{})
	timerValue := seedSourceTimer(t, value, true)
	waitSignal(t, runWaits, "Run wait after timer seed")
	finishWorker, ok := value.beginPromisifyWorker()
	if !ok {
		t.Fatal("valid worker was not admitted")
	}
	t.Cleanup(finishWorker)
	closeResult := make(chan error, 1)
	go func() { closeResult <- value.closeLoop() }()
	waitSignal(t, value.loopDone, "Run completion before Close worker barrier")
	assertChannelOpen(t, value.terminalDone, "terminalDone before Close worker release")
	assertErrorBlocked(t, closeResult)
	if len(value.timerMap) != 1 || timerValue.canceled.Load() || !timerValue.refed.Load() || timerValue.task == nil {
		t.Fatal("started Close cleaned the timer before its worker barrier")
	}
	finishWorker()
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "0def Close worker-barrier Run") {
		t.Fatal("Run did not complete")
	}
	assertSourceCleanup(t, value, timerValue)
}

func TestSourceLosingCloseWaitsTerminalOnly(t *testing.T) {
	value := newLoop(false)
	beforeRunReturn := make(chan struct{}, 1)
	terminalComplete := make(chan struct{})
	resumeRunReturn, releaseRunReturn := newSourceRelease(t)
	runResult, runWaits := startSourceWaitingRun(t, value, lifecycleObserver{
		beforeRunReturn: func() {
			beforeRunReturn <- struct{}{}
			<-resumeRunReturn
		},
		terminalComplete: func() { close(terminalComplete) },
	})
	timerValue := seedSourceTimer(t, value, true)
	waitSignal(t, runWaits, "Run wait after timer seed")
	shutdownPublished := make(chan struct{})
	resumeShutdown, releaseShutdown := newSourceRelease(t)
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			shutdownPublished: func(<-chan struct{}) {
				close(shutdownPublished)
				<-resumeShutdown
			},
		})
	}()
	waitSignal(t, shutdownPublished, "Shutdown transition")
	stages := make(chan closeWaitStage, 1)
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{
			closeWait: func(stage closeWaitStage) { stages <- stage },
		})
	}()
	select {
	case stage := <-stages:
		if stage != closeWaitLosingTerminal {
			t.Fatalf("first losing Close stage = %d", stage)
		}
	case <-time.After(time.Second):
		t.Fatal("losing Close did not reach terminalDone")
	}
	releaseShutdown()
	waitSignal(t, beforeRunReturn, "Run return boundary")
	waitSignal(t, terminalComplete, "terminal completion before Run return")
	if err := receiveError(t, closeResult); !errors.Is(err, errTerminated) {
		t.Fatalf("losing Close = %v", err)
	}
	assertChannelOpen(t, value.loopDone, "loopDone after losing Close return")
	releaseRunReturn()
	if !receiveBool(t, runResult, "0def losing-Close Run") {
		t.Fatal("Run did not complete")
	}
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	assertSourceCleanup(t, value, timerValue)
}

func TestSourceStartupFIFOConsumesRegistrationBeforeAutoExit(t *testing.T) {
	value := newLoop(true)
	id, err := value.prepareTimerRegistration()
	if err != nil || id != 1 {
		t.Fatalf("startup registration = (%d, %v)", id, err)
	}
	unrefWaiting := make(chan struct{})
	unrefPublished := make(chan struct{})
	unrefTimeout := make(chan time.Time)
	defer close(unrefTimeout)
	unrefResult := make(chan error, 1)
	go func() {
		unrefResult <- value.unrefTimerObserved(id, referenceObserver{
			runWaitEntered: func() { close(unrefWaiting) },
			runWaitTimeout: unrefTimeout,
			wakePublished:  func() { close(unrefPublished) },
		})
	}()
	waitSignal(t, unrefWaiting, "startup Unref pre-Run wait")
	assertErrorBlocked(t, unrefResult)

	runStarted := make(chan struct{})
	runStartRelease, releaseRunStart := newSourceRelease(t)
	runWait := make(chan struct{})
	runWaitRelease, releaseRunWait := newSourceRelease(t)
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			runStarted: func() {
				close(runStarted)
				<-runStartRelease
			},
			runWait: func() {
				close(runWait)
				<-runWaitRelease
			},
		})
	}()
	waitSignal(t, runStarted, "startup Run publication")
	waitSignal(t, unrefPublished, "startup Unref admission")
	if len(value.queue) != 2 || len(value.timerMap) != 0 || value.submissionEpoch.Load() != 2 ||
		len(value.fastWakeupCh) != 1 || value.ownerID.Load() != 0 {
		t.Fatal("startup registration and Unref did not remain FIFO before Run claim")
	}
	releaseRunStart()
	waitSignal(t, runWait, "startup ordinary wake wait")
	if len(value.queue) != 2 || len(value.timerMap) != 0 || value.refedTimerCount.Load() != 0 ||
		value.submissionEpoch.Load() != 2 || len(value.fastWakeupCh) != 1 {
		t.Fatal("Run performed a non-source startup drain before acquiring the wake")
	}
	assertErrorBlocked(t, unrefResult)
	releaseRunWait()
	if err := receiveError(t, unrefResult); err != nil {
		t.Fatalf("startup Unref = %v", err)
	}
	if !receiveBool(t, runResult, "0def startup auto-exit Run") {
		t.Fatal("startup FIFO did not reach auto-exit")
	}
	if value.submissionEpoch.Load() != 4 {
		t.Fatalf("startup FIFO epoch = %d", value.submissionEpoch.Load())
	}
	if value.wakeAttempts.Load() != 1 || value.wakeSuccesses.Load() != 1 || value.wakeRejections.Load() != 0 {
		t.Fatalf("startup Unref wake = attempts %d successes %d rejections %d",
			value.wakeAttempts.Load(), value.wakeSuccesses.Load(), value.wakeRejections.Load())
	}
	assertSourceCleanupWake(t, value, nil, 1)
}

func TestSourceAutoExitIngressInvalidatesDecision(t *testing.T) {
	value := newLoop(true)
	decisionReached := make(chan struct{})
	resumeDecision, releaseDecision := newSourceRelease(t)
	var once sync.Once
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{autoExitDecision: func() {
			once.Do(func() {
				close(decisionReached)
				<-resumeDecision
			})
		}})
	}()
	waitSignal(t, decisionReached, "auto-exit decision")
	if state(value.state.Load()) != stateRunning || !value.quiescing.Load() ||
		value.terminalDraining.Load() || value.terminalDrainDone != nil {
		t.Fatal("auto-exit decision did not expose valid quiescence")
	}

	epoch := value.submissionEpoch.Load()
	if finish, ok := value.beginPromisifyWorker(); ok || finish != nil ||
		value.submissionEpoch.Load() != epoch {
		t.Fatal("quiescence admitted Promisify liveness")
	}
	wakePublished := make(chan struct{})
	unrefResult := make(chan error, 1)
	go func() {
		unrefResult <- value.unrefTimerObserved(1, referenceObserver{
			wakePublished: func() { close(wakePublished) },
		})
	}()
	waitSignal(t, wakePublished, "auto-exit invalidating Unref wake")
	if value.submissionEpoch.Load() != epoch+1 || len(value.queue) != 1 ||
		!value.quiescing.Load() || len(value.fastWakeupCh) != 1 {
		t.Fatal("Unref did not occupy the auto-exit invalidation window")
	}
	assertErrorBlocked(t, unrefResult)

	releaseDecision()
	if err := receiveError(t, unrefResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "invalidated auto-exit Run") {
		t.Fatal("Run did not retry auto-exit after admitted Unref")
	}
	assertSourceCleanup(t, value, nil)
}

func TestSourceAutoExitTerminalOvertakes(t *testing.T) {
	t.Run("Close completes captured generation", func(t *testing.T) {
		value := newLoop(true)
		finishWorker, ok := value.beginPromisifyWorker()
		if !ok {
			t.Fatal("Awake worker was not admitted")
		}
		t.Cleanup(finishWorker)
		published := make(chan (<-chan struct{}), 1)
		resume, releaseContinuation := newSourceRelease(t)
		runWaiting := make(chan struct{}, 1)
		runResult := make(chan bool, 1)
		go func() {
			runResult <- value.runObserved(lifecycleObserver{
				runWait: func() { runWaiting <- struct{}{} },
				autoExitPublished: func(done <-chan struct{}) {
					published <- done
					<-resume
				},
			})
		}()
		waitSignal(t, runWaiting, "worker-backed auto-exit wait")
		finishWorker()
		var generationDone <-chan struct{}
		select {
		case generationDone = <-published:
		case <-time.After(time.Second):
			t.Fatal("auto-exit generation was not published")
		}
		closeWoke := make(chan struct{})
		closeResult := make(chan error, 1)
		go func() {
			closeResult <- value.closeLoopObserved(lifecycleObserver{
				closeWait: func(stage closeWaitStage) {
					if stage == closeWaitWinningLoop {
						close(closeWoke)
					}
				},
			})
		}()
		waitSignal(t, closeWoke, "Close auto-exit overtake")
		assertChannelOpen(t, generationDone, "captured generation before continuation")
		if value.terminalDrainDone != generationDone || !value.terminalDraining.Load() || state(value.state.Load()) != stateTerminated {
			t.Fatal("Close changed captured generation authority")
		}
		releaseContinuation()
		if !receiveBool(t, runResult, "0def auto-exit Close Run") {
			t.Fatal("Run did not complete")
		}
		if err := receiveError(t, closeResult); err != nil {
			t.Fatal(err)
		}
		assertChannelClosed(t, generationDone, "captured generation after continuation")
		assertSourceCleanupWake(t, value, nil, 1)
	})

	t.Run("Shutdown overwrites and strands generations", func(t *testing.T) {
		value := newLoop(true)
		finishWorker, ok := value.beginPromisifyWorker()
		if !ok {
			t.Fatal("Awake worker was not admitted")
		}
		t.Cleanup(finishWorker)
		published := make(chan (<-chan struct{}), 1)
		resume, releaseContinuation := newSourceRelease(t)
		runWaiting := make(chan struct{}, 1)
		runResult := make(chan bool, 1)
		go func() {
			runResult <- value.runObserved(lifecycleObserver{
				runWait: func() { runWaiting <- struct{}{} },
				autoExitPublished: func(done <-chan struct{}) {
					published <- done
					<-resume
				},
			})
		}()
		waitSignal(t, runWaiting, "worker-backed auto-exit wait")
		finishWorker()
		var oldDone <-chan struct{}
		select {
		case oldDone = <-published:
		case <-time.After(time.Second):
			t.Fatal("auto-exit generation was not published")
		}
		replacementPublished := make(chan (<-chan struct{}), 1)
		shutdownWoke := make(chan struct{})
		shutdownResult := make(chan error, 1)
		go func() {
			shutdownResult <- value.shutdownObserved(lifecycleObserver{
				shutdownPublished: func(done <-chan struct{}) { replacementPublished <- done },
				shutdownWake:      func() { close(shutdownWoke) },
			})
		}()
		waitSignal(t, shutdownWoke, "Shutdown auto-exit overtake")
		var replacementDone <-chan struct{}
		select {
		case replacementDone = <-replacementPublished:
		case <-time.After(time.Second):
			t.Fatal("replacement generation was not published")
		}
		if oldDone == replacementDone || value.terminalDrainDone != replacementDone || !value.terminalDraining.Load() ||
			state(value.state.Load()) != stateTerminating {
			t.Fatal("Shutdown did not overwrite the captured generation")
		}
		assertChannelOpen(t, oldDone, "overwritten auto-exit generation")
		assertChannelOpen(t, replacementDone, "replacement Shutdown generation")
		releaseContinuation()
		if !receiveBool(t, runResult, "0def auto-exit Shutdown Run") {
			t.Fatal("Run did not complete")
		}
		if err := receiveError(t, shutdownResult); err != nil {
			t.Fatal(err)
		}
		assertChannelOpen(t, oldDone, "stranded old generation")
		assertChannelOpen(t, replacementDone, "stranded replacement generation")
		if value.terminalDrainDone != replacementDone || !value.terminalDraining.Load() || value.terminalOwnerID.Load() == 0 ||
			len(value.timerMap) != 0 || value.queue != nil || value.externalQueue != nil || value.quiescing.Load() ||
			len(value.fastWakeupCh) != 1 || value.wakePending.Load() != 0 || state(value.state.Load()) != stateTerminated {
			t.Fatal("historical generation replacement defect did not remain classified")
		}
		assertChannelClosed(t, value.terminalDone, "terminalDone after generation defect")
		assertChannelClosed(t, value.loopDone, "loopDone after generation defect")
	})
}

func TestSourceTerminalBoundaryControlsQueuedUnref(t *testing.T) {
	for _, immediate := range []bool{false, true} {
		name := "Shutdown"
		if immediate {
			name = "Close"
		}
		t.Run(name, func(t *testing.T) {
			value := newLoop(false)
			runResult := startSourceRun(t, value)
			timerValue := seedSourceTimer(t, value, true)
			callbackStarted := make(chan struct{})
			callbackRelease, releaseCallback := newSourceRelease(t)
			if err := value.submitToQueue(func() {
				close(callbackStarted)
				<-callbackRelease
			}); err != nil {
				t.Fatal(err)
			}
			waitSignal(t, callbackStarted, "blocking callback")
			unrefResult := make(chan error, 1)
			go func() { unrefResult <- value.unrefTimer(1) }()
			waitSignal(t, value.fastWakeupCh, "queued Unref")
			assertErrorBlocked(t, unrefResult)
			terminalPublished := make(chan struct{})
			terminalResult := make(chan error, 1)
			if immediate {
				go func() {
					terminalResult <- value.closeLoopObserved(lifecycleObserver{
						closeWait: func(stage closeWaitStage) {
							if stage == closeWaitWinningLoop {
								close(terminalPublished)
							}
						},
					})
				}()
			} else {
				go func() {
					terminalResult <- value.shutdownObserved(lifecycleObserver{
						shutdownPublished: func(<-chan struct{}) { close(terminalPublished) },
					})
				}()
			}
			waitSignal(t, terminalPublished, name+" boundary")
			releaseCallback()
			unrefErr := receiveError(t, unrefResult)
			if immediate && !errors.Is(unrefErr, errTerminated) {
				t.Fatalf("Close-queued Unref = %v", unrefErr)
			}
			if !immediate && unrefErr != nil {
				t.Fatalf("Shutdown-queued Unref = %v", unrefErr)
			}
			if err := receiveError(t, terminalResult); err != nil {
				t.Fatal(err)
			}
			if !receiveBool(t, runResult, name+" Run") {
				t.Fatal("Run did not complete")
			}
			assertSourceCleanupWake(t, value, timerValue, 1)
		})
	}
}

func TestSourceTerminalBoundaryControlsDetachedBatch(t *testing.T) {
	for _, immediate := range []bool{false, true} {
		name := "Shutdown"
		if immediate {
			name = "Close"
		}
		t.Run(name, func(t *testing.T) {
			value := newLoop(false)
			callbackStarted := make(chan struct{})
			callbackRelease, releaseCallback := newSourceRelease(t)
			callbackTwo := make(chan struct{})
			if err := value.submitToQueue(func() {
				close(callbackStarted)
				<-callbackRelease
			}); err != nil {
				t.Fatal(err)
			}
			if err := value.submitToQueue(func() { close(callbackTwo) }); err != nil {
				t.Fatal(err)
			}
			runResult := startSourceRun(t, value)
			waitSignal(t, callbackStarted, "detached callback one")
			terminalPublished := make(chan struct{})
			terminalResult := make(chan error, 1)
			if immediate {
				go func() {
					terminalResult <- value.closeLoopObserved(lifecycleObserver{
						closeWait: func(stage closeWaitStage) {
							if stage == closeWaitWinningLoop {
								close(terminalPublished)
							}
						},
					})
				}()
			} else {
				go func() {
					terminalResult <- value.shutdownObserved(lifecycleObserver{
						shutdownPublished: func(<-chan struct{}) { close(terminalPublished) },
					})
				}()
			}
			waitSignal(t, terminalPublished, name+" boundary")
			releaseCallback()
			waitSignal(t, callbackTwo, name+" acquired-turn detached callback")
			if err := receiveError(t, terminalResult); err != nil {
				t.Fatal(err)
			}
			if !receiveBool(t, runResult, name+" detached-batch Run") {
				t.Fatal("Run did not complete")
			}
			assertSourceCleanupWake(t, value, nil, 1)
		})
	}
}

func TestSourceLoopThreadTerminalRules(t *testing.T) {
	value := newLoop(false)
	runResult := startSourceRun(t, value)
	timerValue := seedSourceTimer(t, value, true)
	type callbackObservation struct {
		closeErr         error
		shutdownErr      error
		shutdownAgainErr error
		refErr           error
		unrefErr         error
		refed            bool
		refedCount       int32
	}
	callbackResult := make(chan callbackObservation, 1)
	callbackRelease, releaseCallback := newSourceRelease(t)
	if err := value.submitToQueue(func() {
		observation := callbackObservation{
			closeErr:         value.closeLoop(),
			shutdownErr:      value.shutdown(),
			shutdownAgainErr: value.shutdown(),
			refErr:           value.refTimer(1),
			unrefErr:         value.unrefTimer(1),
			refed:            value.timerMap[1].refed.Load(),
			refedCount:       value.refedTimerCount.Load(),
		}
		callbackResult <- observation
		<-callbackRelease
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-callbackResult:
		if !errors.Is(result.closeErr, errReentrant) || result.shutdownErr != nil || !errors.Is(result.shutdownAgainErr, errTerminated) ||
			!errors.Is(result.refErr, errTerminated) || result.unrefErr != nil || result.refed || result.refedCount != 0 {
			t.Fatalf("callback terminal results = %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("loop callback did not execute terminal operations")
	}
	if err := value.shutdown(); !errors.Is(err, errTerminated) {
		t.Fatalf("post-callback-local Shutdown = %v", err)
	}
	releaseCallback()
	if !receiveBool(t, runResult, "0def callback-local Shutdown Run") {
		t.Fatal("Run did not complete callback-local Shutdown")
	}
	if value.wakeAttempts.Load() != 0 {
		t.Fatal("callback-local Shutdown published an external wake")
	}
	assertSourceCleanup(t, value, timerValue)
}

func TestSourceOwnerShutdownRejectsAfterClosePublication(t *testing.T) {
	value := newLoop(false)
	runResult, _ := startSourceWaitingRun(t, value, lifecycleObserver{})
	callbackStarted := make(chan struct{})
	resumeCallback, releaseCallback := newSourceRelease(t)
	ownerResult := make(chan error, 1)
	if err := value.submitToQueue(func() {
		close(callbackStarted)
		<-resumeCallback
		ownerResult <- value.shutdown()
	}); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, callbackStarted, "owner terminal callback")

	closePublished := make(chan struct{})
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{
			closeWait: func(stage closeWaitStage) {
				if stage == closeWaitWinningLoop {
					close(closePublished)
				}
			},
		})
	}()
	waitSignal(t, closePublished, "Close terminal publication")
	if state(value.state.Load()) != stateTerminated {
		t.Fatal("Close did not publish Terminated before owner callback resumed")
	}
	assertErrorBlocked(t, ownerResult)
	releaseCallback()
	if err := receiveError(t, ownerResult); !errors.Is(err, errTerminated) {
		t.Fatalf("owner Shutdown after Close = %v", err)
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "owner Shutdown after Close Run") {
		t.Fatal("Run did not complete Close")
	}
	assertSourceCleanupWake(t, value, nil, 1)
}

func startSourceRun(t *testing.T, value *loop) <-chan bool {
	t.Helper()
	result := make(chan bool, 1)
	go func() { result <- value.run() }()
	waitSignal(t, value.runCh, "0def Run start")
	return result
}

func startSourceWaitingRun(t *testing.T, value *loop, observer lifecycleObserver) (<-chan bool, <-chan struct{}) {
	t.Helper()
	waits := make(chan struct{}, 8)
	priorWait := observer.runWait
	observer.runWait = func() {
		if priorWait != nil {
			priorWait()
		}
		waits <- struct{}{}
	}
	result := make(chan bool, 1)
	go func() { result <- value.runObserved(observer) }()
	waitSignal(t, value.runCh, "0def Run start")
	waitSignal(t, waits, "0def initial Run wait")
	return result, waits
}

func seedSourceTimer(t *testing.T, value *loop, refed bool) *timer {
	t.Helper()
	result := make(chan sourceTimerSetup, 1)
	if err := value.submitToQueue(func() {
		if !value.seed(1, refed) {
			result <- sourceTimerSetup{}
			return
		}
		timerValue := value.timerMap[1]
		timerValue.earliestTick = 9
		timerValue.heapIndex = 7
		timerValue.nestingLevel = 3
		result <- sourceTimerSetup{value: timerValue}
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case setup := <-result:
		if setup.value == nil {
			t.Fatal("source timer setup failed")
		}
		return setup.value
	case <-time.After(time.Second):
		t.Fatal("source timer setup did not complete")
		return nil
	}
}

func assertSourceCleanup(t *testing.T, value *loop, timerValue *timer) {
	assertSourceCleanupWake(t, value, timerValue, 0)
}

func assertSourceCleanupWake(t *testing.T, value *loop, timerValue *timer, fastWake int) {
	t.Helper()
	if timerValue != nil && (timerValue.task != nil || timerValue.refed.Load() || !timerValue.canceled.Load() ||
		timerValue.earliestTick != 0 || timerValue.heapIndex != -1 || timerValue.nestingLevel != 0) {
		t.Fatal("timer cleanup differs")
	}
	if len(value.timerMap) != 0 || value.refedTimerCount.Load() != 0 || value.promisifyCount.Load() != 0 || value.userIOFDCount.Load() != 0 ||
		value.queue != nil || value.spare != nil || value.externalQueue != nil || value.externalSpare != nil ||
		value.quiescing.Load() || value.terminalDraining.Load() || value.terminalDrainDone != nil ||
		value.terminalOwnerID.Load() != 0 || len(value.fastWakeupCh) != fastWake || value.wakePending.Load() != 0 {
		t.Fatalf("loop cleanup differs: timers=%d refed=%d workers=%d fds=%d queueNil=%t spareNil=%t externalQueueNil=%t externalSpareNil=%t quiescing=%t draining=%t drainDoneNil=%t terminalOwner=%d fastWake=%d wantFastWake=%d wakePending=%d",
			len(value.timerMap), value.refedTimerCount.Load(), value.promisifyCount.Load(), value.userIOFDCount.Load(),
			value.queue == nil, value.spare == nil, value.externalQueue == nil, value.externalSpare == nil,
			value.quiescing.Load(), value.terminalDraining.Load(), value.terminalDrainDone == nil,
			value.terminalOwnerID.Load(), len(value.fastWakeupCh), fastWake, value.wakePending.Load())
	}
	if state(value.state.Load()) != stateTerminated || value.ownerID.Load() != 0 {
		t.Fatal("terminal Run owner/state did not settle")
	}
	assertChannelClosed(t, value.terminalDone, "terminalDone")
	assertChannelClosed(t, value.loopDone, "loopDone")
}

func newSourceRelease(t *testing.T) (<-chan struct{}, func()) {
	t.Helper()
	release := make(chan struct{})
	var once sync.Once
	closeRelease := func() { once.Do(func() { close(release) }) }
	t.Cleanup(closeRelease)
	return release, closeRelease
}
