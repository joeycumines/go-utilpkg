package timerrefclosureawake

import (
	"errors"
	"testing"
	"time"
)

func TestSourceTerminalLosersReturnDuringWinnerPause(t *testing.T) {
	value := newLoop(false)
	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	winnerPaused := make(chan struct{})
	resumeWinner := make(chan struct{})
	winnerWaiting := make(chan struct{})
	t.Cleanup(func() {
		releaseSignal(resumeWinner)
		releaseSignal(releaseCallback)
		if state(value.state.Load()) != stateTerminated {
			_ = value.closeLoop()
		}
	})
	if err := value.submitToQueue(func() {
		close(callbackEntered)
		<-releaseCallback
	}); err != nil {
		t.Fatal(err)
	}
	runResult := make(chan bool, 1)
	go func() { runResult <- value.run() }()
	waitSignal(t, callbackEntered, "blocking accepted callback")

	winnerResult := make(chan error, 1)
	go func() {
		winnerResult <- value.shutdownObserved(lifecycleObserver{
			shutdownWon: func() {
				close(winnerPaused)
				<-resumeWinner
			},
			shutdownWake: func() { close(winnerWaiting) },
		})
	}()
	waitSignal(t, winnerPaused, "terminal winner")

	shutdownResult := make(chan error, 1)
	closeResult := make(chan error, 1)
	go func() { shutdownResult <- value.shutdown() }()
	go func() { closeResult <- value.closeLoop() }()
	if err := receiveError(t, shutdownResult); !errors.Is(err, errTerminated) {
		t.Fatalf("losing Shutdown = %v", err)
	}
	if err := receiveError(t, closeResult); !errors.Is(err, errTerminated) {
		t.Fatalf("losing Close = %v", err)
	}
	if state(value.state.Load()) != stateTerminating || !value.terminalDraining.Load() {
		t.Fatal("terminal loser disturbed the winning generation")
	}

	close(resumeWinner)
	waitSignal(t, winnerWaiting, "started Shutdown wake")
	assertErrorBlocked(t, winnerResult)
	close(releaseCallback)
	if !receiveBool(t, runResult, "terminal winner Run") {
		t.Fatal("Run did not complete winning Shutdown")
	}
	if err := receiveError(t, winnerResult); err != nil {
		t.Fatal(err)
	}
	if value.ownerID.Load() != 0 || value.terminalDraining.Load() ||
		value.terminalGeneration != nil || state(value.state.Load()) != stateTerminated {
		t.Fatal("winning terminal generation did not settle")
	}
	for name, operation := range map[string]func() error{
		"Close":    value.closeLoop,
		"Ref":      func() error { return value.refTimer(1) },
		"Shutdown": value.shutdown,
		"Unref":    func() error { return value.unrefTimer(1) },
	} {
		if err := operation(); !errors.Is(err, errTerminated) {
			t.Fatalf("post-terminal %s = %v", name, err)
		}
	}
}

func TestSourceTerminalIntervenesAfterRunClaim(t *testing.T) {
	value := newLoop(false)
	runStarted := make(chan struct{})
	resumeRunStart := make(chan struct{})
	runClaimed := make(chan struct{})
	resumeRunClaim := make(chan struct{})
	t.Cleanup(func() {
		releaseSignal(resumeRunStart)
		releaseSignal(resumeRunClaim)
	})
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			runStarted: func() {
				close(runStarted)
				<-resumeRunStart
			},
			runClaimed: func() {
				close(runClaimed)
				<-resumeRunClaim
			},
		})
	}()
	waitSignal(t, runStarted, "Run start")

	shutdownObserved := make(chan state, 1)
	resumeShutdownCAS := make(chan struct{})
	shutdownWon := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeShutdownCAS) })
	firstObservation := true
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			shutdownStateObserved: func(current state) {
				if firstObservation {
					firstObservation = false
					shutdownObserved <- current
					<-resumeShutdownCAS
				}
			},
			shutdownWon: func() { close(shutdownWon) },
		})
	}()
	if current := <-shutdownObserved; current != stateAwake {
		t.Fatalf("first Shutdown observation = %v", current)
	}
	close(resumeRunStart)
	waitSignal(t, runClaimed, "Run claim")
	close(resumeShutdownCAS)
	waitSignal(t, shutdownWon, "Shutdown claim")
	if state(value.state.Load()) != stateTerminating || value.ownerID.Load() != 0 {
		t.Fatal("Shutdown did not win after retrying the stale Awake observation")
	}

	close(resumeRunClaim)
	if !receiveBool(t, runResult, "claimed Run") {
		t.Fatal("Run did not complete the intervening generation")
	}
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if value.ownerID.Load() != 0 || value.terminalDraining.Load() ||
		value.terminalGeneration != nil || state(value.state.Load()) != stateTerminated {
		t.Fatal("intervening terminal generation did not settle")
	}
}

func TestSourceRunDrivesAutoExit(t *testing.T) {
	value := newLoop(true)
	if !value.run() {
		t.Fatal("Run did not complete auto-exit")
	}
	if value.ownerID.Load() != 0 || value.terminalDraining.Load() ||
		state(value.state.Load()) != stateTerminated {
		t.Fatal("Run auto-exit did not settle")
	}
	assertChannelClosed(t, value.loopDone, "auto-exit loopDone")
}

func TestSourceTerminatingReferenceWaitOutcomes(t *testing.T) {
	for _, terminalCompletion := range []bool{false, true} {
		name := "Deadline"
		if terminalCompletion {
			name = "TerminalCompletion"
		}
		t.Run(name, func(t *testing.T) {
			value := newLoop(false)
			winnerPaused := make(chan struct{})
			resumeWinner := make(chan struct{})
			t.Cleanup(func() { releaseSignal(resumeWinner) })
			winnerResult := make(chan error, 1)
			go func() {
				winnerResult <- value.shutdownObserved(lifecycleObserver{
					shutdownWon: func() {
						close(winnerPaused)
						<-resumeWinner
					},
				})
			}()
			waitSignal(t, winnerPaused, "unstarted Shutdown winner")

			waiting := make(chan struct{})
			var deadline chan time.Time
			if !terminalCompletion {
				deadline = make(chan time.Time)
			}
			runWaiting := func() { close(waiting) }
			if terminalCompletion {
				runWaiting = func() {
					close(waiting)
					<-value.loopDone
				}
			}
			result := make(chan error, 1)
			go func() {
				result <- value.unrefTimerObserved(1, referenceObserver{
					runWaiting:  runWaiting,
					runDeadline: deadline,
				})
			}()
			waitSignal(t, waiting, "terminating reference wait")
			assertErrorBlocked(t, result)
			if terminalCompletion {
				close(resumeWinner)
				if err := receiveError(t, result); !errors.Is(err, errTerminated) {
					t.Fatalf("reference after terminal completion = %v", err)
				}
			} else {
				close(deadline)
				if err := receiveError(t, result); !errors.Is(err, errNotRunning) {
					t.Fatalf("terminating reference deadline = %v", err)
				}
				close(resumeWinner)
			}
			if err := receiveError(t, winnerResult); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSourceSleepingReferenceIngressResumesRun(t *testing.T) {
	value := newLoop(false)
	sleeping := make(chan struct{}, 1)
	resumed := make(chan struct{}, 1)
	sawSleeping := false
	timerValue, runResult := startSeededLoop(t, value, false, lifecycleObserver{
		runWait: func() {
			switch state(value.state.Load()) {
			case stateSleeping:
				sawSleeping = true
				select {
				case sleeping <- struct{}{}:
				default:
				}
			case stateRunning:
				if sawSleeping {
					select {
					case resumed <- struct{}{}:
					default:
					}
				}
			}
		},
	})
	configured := make(chan bool, 1)
	if err := value.submitToQueue(func() {
		configured <- value.configureUserFDCount(1) && value.transition(stateSleeping)
	}); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, configured, "Sleeping reference configuration") {
		t.Fatal("owner did not enter Sleeping")
	}
	waitSignal(t, sleeping, "Sleeping reference Run wait")

	refResult := make(chan error, 1)
	go func() { refResult <- value.refTimer(1) }()
	if err := receiveError(t, refResult); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, resumed, "resumed reference Run wait")
	if !timerValue.refed.Load() || value.refedTimerCount.Load() != 1 ||
		state(value.state.Load()) != stateRunning || len(value.queue) != 0 ||
		value.wakePending.Load() != 0 || value.wakeAttempts.Load() != 1 ||
		value.wakeSuccesses.Load() != 1 {
		t.Fatal("Sleeping reference ingress did not resume and settle")
	}
	if err := value.closeLoop(); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "Sleeping reference Close Run") {
		t.Fatal("Run did not complete Close")
	}
}

func startSeededLoop(
	t *testing.T,
	value *loop,
	refed bool,
	observer lifecycleObserver,
) (*timer, <-chan bool) {
	t.Helper()
	seeded := make(chan *timer, 1)
	if err := value.submitToQueue(func() {
		if !value.seed(1, refed) {
			seeded <- nil
			return
		}
		seeded <- value.timerMap[1]
	}); err != nil {
		t.Fatal(err)
	}
	runResult := make(chan bool, 1)
	go func() { runResult <- value.runObserved(observer) }()
	select {
	case timerValue := <-seeded:
		if timerValue == nil {
			t.Fatal("Run did not seed the control timer")
		}
		return timerValue, runResult
	case <-value.loopDone:
		t.Fatal("Run exited before seeding the control timer")
		return nil, runResult
	}
}

func releaseSignal(signal chan struct{}) {
	select {
	case <-signal:
	default:
		close(signal)
	}
}

func TestStartedShutdownModelsMandatoryWake(t *testing.T) {
	t.Run("Running", func(t *testing.T) {
		value := newLoop(false)
		_, runResult := startSeededLoop(t, value, false, lifecycleObserver{})
		shutdownResult := make(chan error, 1)
		go func() { shutdownResult <- value.shutdown() }()
		if !receiveBool(t, runResult, "Running Shutdown Run") {
			t.Fatal("Run did not complete Shutdown")
		}
		if err := receiveError(t, shutdownResult); err != nil {
			t.Fatal(err)
		}
		if value.wakeAttempts.Load() != 1 || value.wakeSuccesses.Load() != 1 ||
			value.ownerID.Load() != 0 || state(value.state.Load()) != stateTerminated {
			t.Fatal("Running Shutdown did not publish and settle one mandatory wake")
		}
	})
	t.Run("Sleeping", func(t *testing.T) {
		value := newLoop(false)
		runningWait := make(chan struct{}, 1)
		sleepingWait := make(chan struct{}, 1)
		runResult := make(chan bool, 1)
		go func() {
			runResult <- value.runObserved(lifecycleObserver{runWait: func() {
				switch state(value.state.Load()) {
				case stateRunning:
					select {
					case runningWait <- struct{}{}:
					default:
					}
				case stateSleeping:
					select {
					case sleepingWait <- struct{}{}:
					default:
					}
				}
			}})
		}()
		waitSignal(t, runningWait, "ordinary Running wait")
		configured := make(chan bool, 1)
		if err := value.submitToQueue(func() {
			configured <- value.configureUserFDCount(1) && value.transition(stateSleeping)
		}); err != nil {
			t.Fatal(err)
		}
		if !receiveBool(t, configured, "Sleeping configuration") {
			t.Fatal("owner did not enter Sleeping")
		}
		waitSignal(t, sleepingWait, "Sleeping Run wait")
		shutdownResult := make(chan error, 1)
		go func() { shutdownResult <- value.shutdown() }()
		if !receiveBool(t, runResult, "Sleeping Shutdown Run") {
			t.Fatal("Run did not complete Shutdown")
		}
		if err := receiveError(t, shutdownResult); err != nil {
			t.Fatal(err)
		}
		if value.wakeAttempts.Load() != 1 || value.wakeSuccesses.Load() != 1 ||
			value.ownerID.Load() != 0 || state(value.state.Load()) != stateTerminated {
			t.Fatal("Sleeping Shutdown did not publish and settle one mandatory wake")
		}
	})
}
func TestRepairedControlSuppressesLateStartedShutdownWake(t *testing.T) {
	value := newLoop(false)
	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	t.Cleanup(func() { releaseSignal(releaseCallback) })
	if err := value.submitToQueue(func() {
		close(callbackEntered)
		<-releaseCallback
	}); err != nil {
		t.Fatal(err)
	}
	runResult := make(chan bool, 1)
	go func() { runResult <- value.run() }()
	waitSignal(t, callbackEntered, "blocking callback")
	boundary := make(chan struct{})
	resumeShutdown := make(chan struct{})
	wakeReturned := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeShutdown) })
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			shutdownWon: func() {
				close(boundary)
				<-resumeShutdown
			},
			shutdownWake: func() { close(wakeReturned) },
		})
	}()
	waitSignal(t, boundary, "Shutdown transition")
	close(releaseCallback)
	if !receiveBool(t, runResult, "late-wake Shutdown Run") {
		t.Fatal("Run did not complete the generation")
	}
	close(resumeShutdown)
	waitSignal(t, wakeReturned, "delayed Shutdown wake return")
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if len(value.fastWakeupCh) != 0 || value.wakePending.Load() != 0 ||
		value.wakeAttempts.Load() != 0 || state(value.state.Load()) != stateTerminated {
		t.Fatal("repaired control did not suppress the late started Shutdown wake")
	}
}
func TestAwakeShutdownOwnsAcceptedDrain(t *testing.T) {
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
	id, err := value.prepareTimerRegistration()
	if err != nil || id != 1 {
		t.Fatalf("Awake registration = (%d, %v)", id, err)
	}
	admitted := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- value.unrefTimerObserved(id, referenceObserver{queueAdmitted: func(uint64) { close(admitted) }})
	}()
	waitSignal(t, admitted, "Awake accepted admission")
	if err := value.shutdown(); err != nil {
		t.Fatal(err)
	}
	if err := receiveError(t, result); err != nil {
		t.Fatalf("Awake accepted result = %v", err)
	}
	close(resumeRun)
	timerValue := value.timerMap[id]
	if receiveBool(t, runResult, "Awake Shutdown Run") || timerValue == nil || timerValue.refed.Load() ||
		len(value.queue) != 0 ||
		value.ownerID.Load() != 0 || state(value.state.Load()) != stateTerminated {
		t.Fatal("Awake Shutdown did not own and complete its accepted drain")
	}
}
func TestAcceptedTerminalDrainWorkCompletes(t *testing.T) {
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
	waitSignal(t, secondWait, "accepted-drain Run wait")
	admitted := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- value.unrefTimerObserved(1, referenceObserver{queueAdmitted: func(uint64) { close(admitted) }})
	}()
	waitSignal(t, admitted, "accepted Unref admission")
	shutdownWoke := make(chan struct{})
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{shutdownWake: func() { close(shutdownWoke) }})
	}()
	waitSignal(t, shutdownWoke, "accepted-drain Shutdown wake")
	close(resumeRun)
	if err := receiveError(t, result); err != nil {
		t.Fatalf("accepted terminal-drain call = %v", err)
	}
	if !receiveBool(t, runResult, "accepted-drain Run") {
		t.Fatal("Run did not complete Shutdown")
	}
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if timerValue.refed.Load() || value.terminalDraining.Load() ||
		value.ownerID.Load() != 0 || state(value.state.Load()) != stateTerminated {
		t.Fatal("accepted work or terminal generation did not settle")
	}
}
func TestRefAndUnrefAdmissionPrecedeTerminalBoundary(t *testing.T) {
	tests := []struct {
		name  string
		start bool
		refed bool
	}{
		{name: "Ref", start: false, refed: true},
		{name: "Unref", start: true, refed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := newLoop(false)
			secondWait := make(chan struct{})
			resumeRun := make(chan struct{})
			t.Cleanup(func() { releaseSignal(resumeRun) })
			waits := 0
			timerValue, runResult := startSeededLoop(t, value, test.start, lifecycleObserver{runWait: func() {
				waits++
				if waits == 2 {
					close(secondWait)
					<-resumeRun
				}
			}})
			waitSignal(t, secondWait, "reference-boundary Run wait")
			admitted := make(chan struct{})
			result := make(chan error, 1)
			go func() {
				observer := referenceObserver{queueAdmitted: func(uint64) { close(admitted) }}
				if test.refed {
					result <- value.refTimerObserved(1, observer)
				} else {
					result <- value.unrefTimerObserved(1, observer)
				}
			}()
			waitSignal(t, admitted, "reference admission")
			shutdownWoke := make(chan struct{})
			shutdownResult := make(chan error, 1)
			go func() {
				shutdownResult <- value.shutdownObserved(lifecycleObserver{shutdownWake: func() { close(shutdownWoke) }})
			}()
			waitSignal(t, shutdownWoke, "reference-boundary Shutdown wake")
			close(resumeRun)
			if err := receiveError(t, result); err != nil {
				t.Fatalf("pre-boundary call = %v", err)
			}
			if !receiveBool(t, runResult, "reference-boundary Run") {
				t.Fatal("Run did not complete Shutdown")
			}
			if err := receiveError(t, shutdownResult); err != nil {
				t.Fatal(err)
			}
			if timerValue.refed.Load() != test.refed {
				t.Fatal("pre-boundary reference change was lost")
			}
		})
	}
}
func TestImmediateCloseDiscardsAcceptedWork(t *testing.T) {
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
	waitSignal(t, secondWait, "immediate Close Run wait")
	admitted := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- value.unrefTimerObserved(1, referenceObserver{queueAdmitted: func(uint64) { close(admitted) }})
	}()
	waitSignal(t, admitted, "accepted Close admission")
	closeWoke := make(chan struct{})
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{closeWake: func() { close(closeWoke) }})
	}()
	waitSignal(t, closeWoke, "immediate Close wake")
	close(resumeRun)
	if !receiveBool(t, runResult, "immediate Close Run") {
		t.Fatal("Run did not complete Close")
	}
	if err := receiveError(t, result); !errors.Is(err, errTerminated) {
		t.Fatalf("accepted call result = %v", err)
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
	if !timerValue.refed.Load() || len(value.queue) != 0 || value.ownerID.Load() != 0 {
		t.Fatal("Close executed accepted work or retained owner state")
	}
}
func TestImmediateCloseRejectsActiveGeneration(t *testing.T) {
	value := newLoop(false)
	secondWait := make(chan struct{})
	resumeRun := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRun) })
	waits := 0
	_, runResult := startSeededLoop(t, value, false, lifecycleObserver{runWait: func() {
		waits++
		if waits == 2 {
			close(secondWait)
			<-resumeRun
		}
	}})
	waitSignal(t, secondWait, "terminal competition Run wait")
	boundary := make(chan struct{})
	resumeShutdown := make(chan struct{})
	shutdownWoke := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeShutdown) })
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			shutdownWon: func() {
				close(boundary)
				<-resumeShutdown
			},
			shutdownWake: func() { close(shutdownWoke) },
		})
	}()
	waitSignal(t, boundary, "active Shutdown generation")
	if err := value.closeLoop(); !errors.Is(err, errTerminated) {
		t.Fatalf("Close during active Shutdown = %v", err)
	}
	close(resumeShutdown)
	waitSignal(t, shutdownWoke, "active Shutdown wake")
	close(resumeRun)
	if !receiveBool(t, runResult, "terminal competition Run") {
		t.Fatal("Run did not complete Shutdown")
	}
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
}
func TestTerminalEndFollowsAcceptedCallback(t *testing.T) {
	value := newLoop(false)
	entered := make(chan struct{})
	releaseCallback := make(chan struct{})
	callbackTwo := make(chan struct{})
	t.Cleanup(func() { releaseSignal(releaseCallback) })
	if err := value.submitToQueue(func() {
		close(entered)
		<-releaseCallback
	}); err != nil {
		t.Fatal(err)
	}
	if err := value.submitToQueue(func() { close(callbackTwo) }); err != nil {
		t.Fatal(err)
	}
	shutdownResult := make(chan error, 1)
	go func() { shutdownResult <- value.shutdown() }()
	waitSignal(t, entered, "accepted terminal callback")
	assertChannelOpen(t, value.loopDone, "loopDone during accepted callback")
	assertErrorBlocked(t, shutdownResult)
	close(releaseCallback)
	waitSignal(t, callbackTwo, "second accepted terminal callback")
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	assertChannelClosed(t, value.loopDone, "loopDone after accepted callbacks")
}
