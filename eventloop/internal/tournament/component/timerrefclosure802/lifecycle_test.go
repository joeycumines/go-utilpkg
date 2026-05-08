package timerrefclosure802

import (
	"errors"
	"testing"
	"time"
)

func TestSourceTerminalWinsRunClaim(t *testing.T) {
	for _, immediate := range []bool{false, true} {
		name := "Shutdown"
		if immediate {
			name = "Close"
		}
		t.Run(name, func(t *testing.T) {
			value := newLoop(false)
			sentinel := make(chan struct{})
			if err := value.submitToQueue(func() { close(sentinel) }); err != nil {
				t.Fatal(err)
			}
			runStarted := make(chan struct{})
			resumeRun, releaseRun := newSourceRelease(t)
			runResult := make(chan bool, 1)
			go func() {
				runResult <- value.runObserved(lifecycleObserver{
					runStarted: func() {
						close(runStarted)
						<-resumeRun
					},
				})
			}()
			waitSignal(t, runStarted, "Run pre-claim publication")
			var err error
			if immediate {
				err = value.closeLoop()
				assertChannelOpen(t, sentinel, "Close-discarded pre-Run sentinel")
			} else {
				err = value.shutdown()
				assertChannelClosed(t, sentinel, "Shutdown-drained pre-Run sentinel")
			}
			if err != nil {
				t.Fatal(err)
			}
			releaseRun()
			if receiveBool(t, runResult, "802 terminal-first Run") {
				t.Fatal("Run claimed ownership after the terminal winner")
			}
			if value.ownerID.Load() != 0 {
				t.Fatal("terminal-first Run published an owner")
			}
			assertSourceCleanupWake(t, value, nil, 1)
		})
	}
}

func TestSourceRunClaimPrecedesOwnerPublication(t *testing.T) {
	for _, immediate := range []bool{false, true} {
		name := "Shutdown"
		if immediate {
			name = "Close"
		}
		t.Run(name, func(t *testing.T) {
			value := newLoop(false)
			claimed := make(chan struct{})
			resumeRun, releaseRun := newSourceRelease(t)
			runResult := make(chan bool, 1)
			go func() {
				runResult <- value.runObserved(lifecycleObserver{
					runClaimed: func() {
						close(claimed)
						<-resumeRun
					},
				})
			}()
			waitSignal(t, claimed, "Run claim before owner publication")
			if state(value.state.Load()) != stateRunning || value.ownerID.Load() != 0 {
				t.Fatal("Run claim phase differs before owner publication")
			}
			boundary := make(chan struct{})
			terminalResult := make(chan error, 1)
			if immediate {
				go func() {
					terminalResult <- value.closeLoopObserved(lifecycleObserver{
						closeWait: func(stage closeWaitStage) {
							if stage == closeWaitWinningLoop {
								close(boundary)
							}
						},
					})
				}()
			} else {
				go func() {
					terminalResult <- value.shutdownObserved(lifecycleObserver{
						shutdownPublished: func(<-chan struct{}) { close(boundary) },
					})
				}()
			}
			waitSignal(t, boundary, name+" boundary before owner publication")
			assertErrorBlocked(t, terminalResult)
			assertBoolBlocked(t, runResult)
			if value.ownerID.Load() != 0 {
				t.Fatal("terminal winner fabricated owner publication")
			}
			releaseRun()
			if !receiveBool(t, runResult, "802 claimed Run terminal exit") {
				t.Fatal("Run did not retain its successful claim")
			}
			if err := receiveError(t, terminalResult); err != nil {
				t.Fatal(err)
			}
			assertSourceCleanupWake(t, value, nil, 1)
		})
	}
}

func TestSourceShutdownGenerationPublicationGapCompletes(t *testing.T) {
	value := newLoop(false)
	callbackStarted := make(chan struct{})
	callbackRelease, releaseCallback := newSourceRelease(t)
	if err := value.submitToQueue(func() {
		close(callbackStarted)
		<-callbackRelease
	}); err != nil {
		t.Fatal(err)
	}
	terminalClaimAttempted := make(chan struct{})
	terminalGenerationObserved := make(chan struct{})
	terminalCommitRelease, releaseTerminalCommit := newSourceRelease(t)
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			terminalClaimAttempted: func() { close(terminalClaimAttempted) },
			terminalTransition: func() {
				close(terminalGenerationObserved)
				<-terminalCommitRelease
			},
		})
	}()
	waitSignal(t, callbackStarted, "callback in acquired startup turn")

	stateTransitioned := make(chan struct{})
	shutdownWake := make(chan struct{})
	publicationRelease, releasePublication := newSourceRelease(t)
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			shutdownStateTransitioned: func() {
				close(stateTransitioned)
				<-publicationRelease
			},
			shutdownWake: func() { close(shutdownWake) },
		})
	}()
	waitSignal(t, stateTransitioned, "Shutdown state transition")
	if state(value.state.Load()) != stateTerminating || value.terminalDraining.Load() ||
		value.terminalDrainDone != nil || value.terminalOwnerID.Load() != 0 {
		t.Fatal("Shutdown published generation authority before the source gap")
	}

	releaseCallback()
	waitSignal(t, terminalClaimAttempted, "Run terminal-owner claim")
	assertBoolBlocked(t, runResult)
	assertErrorBlocked(t, shutdownResult)
	releasePublication()
	waitSignal(t, terminalGenerationObserved, "Run generation observation")
	waitSignal(t, shutdownWake, "Shutdown wake before terminal commit")
	releaseTerminalCommit()
	if !receiveBool(t, runResult, "generation-publication-gap Run") {
		t.Fatal("Run did not complete the published graceful generation")
	}
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if value.wakeAttempts.Load() != 1 || value.wakeSuccesses.Load() != 1 || value.wakeRejections.Load() != 0 {
		t.Fatalf("Shutdown wake counters = attempts %d successes %d rejections %d",
			value.wakeAttempts.Load(), value.wakeSuccesses.Load(), value.wakeRejections.Load())
	}
	if value.submissionEpoch.Load() != 1 {
		t.Fatalf("generation-publication-gap admission epoch = %d", value.submissionEpoch.Load())
	}
	assertSourceCleanupWake(t, value, nil, 1)
}

func TestSourceLateStartedShutdownWakeRemainsVisible(t *testing.T) {
	value := newLoop(false)
	terminalCommitted := make(chan struct{})
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			terminalCommit: func() { close(terminalCommitted) },
		})
	}()
	waitSignal(t, value.runCh, "802 Run start")
	timerValue := seedSourceTimer(t, value, true)
	callbackStarted := make(chan bool, 1)
	callbackRelease, releaseCallback := newSourceRelease(t)
	if err := value.submitToQueue(func() {
		callbackStarted <- value.configureUserFDCount(1)
		<-callbackRelease
	}); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, callbackStarted, "busy Run callback with user FD") {
		t.Fatal("busy Run callback did not configure user FD liveness")
	}
	baselineAttempts := value.wakeAttempts.Load()
	baselineSuccesses := value.wakeSuccesses.Load()
	baselineRejections := value.wakeRejections.Load()
	fastWakePublished := make(chan struct{})
	resumePhysicalWake, releasePhysicalWake := newSourceRelease(t)
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			shutdownFastWake: func() {
				close(fastWakePublished)
				<-resumePhysicalWake
			},
		})
	}()
	waitSignal(t, fastWakePublished, "Shutdown fast wake")
	releaseCallback()
	waitSignal(t, terminalCommitted, "Run terminal commit")
	if state(value.state.Load()) != stateTerminated {
		t.Fatal("Run did not reach Terminated before the delayed physical wake")
	}
	if !receiveBool(t, runResult, "802 Run before delayed physical wake") {
		t.Fatal("Run did not finish before the delayed physical wake")
	}
	assertChannelClosed(t, value.loopDone, "loopDone before delayed physical wake")
	assertErrorBlocked(t, shutdownResult)
	releasePhysicalWake()
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if value.wakeAttempts.Load() != baselineAttempts+1 || value.wakeSuccesses.Load() != baselineSuccesses || value.wakeRejections.Load() != baselineRejections+1 {
		t.Fatal("late Shutdown wake did not preserve its rejected physical attempt")
	}
	assertSourceCleanupWake(t, value, timerValue, 1)
}

func TestSourceReferenceAdmissionPrecedesShutdownBoundary(t *testing.T) {
	for _, test := range []struct {
		name      string
		initial   bool
		refed     bool
		wantCount int32
	}{
		{name: "Ref", initial: false, refed: true, wantCount: 1},
		{name: "Unref", initial: true, refed: false, wantCount: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := newLoop(false)
			runResult := startSourceRun(t, value)
			timerValue := seedSourceTimer(t, value, test.initial)
			callbackStarted := make(chan struct{})
			callbackRelease, releaseCallback := newSourceRelease(t)
			if err := value.submitToQueue(func() {
				close(callbackStarted)
				<-callbackRelease
			}); err != nil {
				t.Fatal(err)
			}
			waitSignal(t, callbackStarted, "blocking callback")
			result := make(chan error, 1)
			go func() {
				if test.refed {
					result <- value.refTimer(1)
					return
				}
				result <- value.unrefTimer(1)
			}()
			waitSignal(t, value.fastWakeupCh, "reference admission wake")
			assertErrorBlocked(t, result)
			observed := make(chan qualificationSnapshot, 1)
			if err := value.submitToQueue(func() { observed <- value.snapshot(1) }); err != nil {
				t.Fatal(err)
			}
			shutdownPublished := make(chan struct{})
			shutdownResult := make(chan error, 1)
			go func() {
				shutdownResult <- value.shutdownObserved(lifecycleObserver{
					shutdownPublished: func(<-chan struct{}) { close(shutdownPublished) },
				})
			}()
			waitSignal(t, shutdownPublished, "Shutdown boundary")
			releaseCallback()
			var snapshot qualificationSnapshot
			select {
			case snapshot = <-observed:
			case <-time.After(time.Second):
				t.Fatal("terminal sentinel did not observe the reference effect")
			}
			if snapshot.refed != test.refed || snapshot.refedCount != int64(test.wantCount) {
				t.Fatalf("terminal reference snapshot = %+v", snapshot)
			}
			if err := receiveError(t, result); err != nil {
				t.Fatalf("pre-boundary reference result = %v", err)
			}
			if err := receiveError(t, shutdownResult); err != nil {
				t.Fatal(err)
			}
			if !receiveBool(t, runResult, "802 reference-boundary Run") {
				t.Fatal("Run did not complete")
			}
			assertSourceCleanupWake(t, value, timerValue, 1)
		})
	}
}

func TestSourceWakeTurnDrainsGracefulTerminatingCallback(t *testing.T) {
	value := newLoop(false)
	id, err := value.prepareTimerRegistration()
	if err != nil || id != 1 {
		t.Fatalf("pre-Run registration = %d, %v", id, err)
	}
	waiting := make(chan struct{})
	resumeRun, releaseRun := newSourceRelease(t)
	firstWait := true
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			runWait: func() {
				if firstWait {
					firstWait = false
					close(waiting)
					<-resumeRun
				}
			},
		})
	}()
	waitSignal(t, waiting, "Run before wake receive")
	timerValue := value.timerMap[id]
	if timerValue == nil || !timerValue.refed.Load() || value.refedTimerCount.Load() != 1 {
		t.Fatal("startup registration did not materialize before the wait")
	}
	baselineEpoch := value.submissionEpoch.Load()
	type callbackObservation struct {
		state      state
		refErr     error
		unrefErr   error
		refed      bool
		refedCount int32
		epoch      uint64
	}
	callbackResult := make(chan callbackObservation, 1)
	if err := value.submitToQueue(func() {
		observation := callbackObservation{
			state:  state(value.state.Load()),
			refErr: value.refTimer(id),
		}
		observation.unrefErr = value.unrefTimer(id)
		observation.refed = timerValue.refed.Load()
		observation.refedCount = value.refedTimerCount.Load()
		observation.epoch = value.submissionEpoch.Load()
		callbackResult <- observation
	}); err != nil {
		t.Fatal(err)
	}
	shutdownPublished := make(chan struct{})
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			shutdownPublished: func(<-chan struct{}) { close(shutdownPublished) },
		})
	}()
	waitSignal(t, shutdownPublished, "Shutdown before wake-turn drain")
	if state(value.state.Load()) != stateTerminating {
		t.Fatal("Shutdown did not publish the graceful terminal boundary")
	}
	select {
	case observation := <-callbackResult:
		t.Fatalf("callback executed before wake-turn release: %+v", observation)
	default:
	}
	releaseRun()
	var observation callbackObservation
	select {
	case observation = <-callbackResult:
	case <-time.After(time.Second):
		t.Fatal("graceful wake-turn callback did not execute")
	}
	if observation.state != stateTerminating || !errors.Is(observation.refErr, errTerminated) || observation.unrefErr != nil ||
		observation.refed || observation.refedCount != 0 || observation.epoch != baselineEpoch+2 {
		t.Fatalf("graceful wake-turn callback = %+v, baseline epoch %d", observation, baselineEpoch)
	}
	if !receiveBool(t, runResult, "802 graceful wake-turn Run") {
		t.Fatal("Run did not complete graceful wake-turn termination")
	}
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	assertSourceCleanupWake(t, value, timerValue, 0)
}

func TestSourceExternalReferencesRejectAfterShutdownBoundary(t *testing.T) {
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
	epoch := value.submissionEpoch.Load()
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
	waitSignal(t, shutdownPublished, "Shutdown boundary")
	if err := value.refTimer(1); !errors.Is(err, errTerminated) {
		t.Fatalf("external Ref after Shutdown boundary = %v", err)
	}
	if err := value.unrefTimer(1); !errors.Is(err, errTerminated) {
		t.Fatalf("external Unref after Shutdown boundary = %v", err)
	}
	if finish, ok := value.beginPromisifyWorker(); ok || finish != nil {
		t.Fatal("Promisify worker admitted after Shutdown boundary")
	}
	if value.submissionEpoch.Load() != epoch || !timerValue.refed.Load() || value.refedTimerCount.Load() != 1 || len(value.queue) != 0 || len(value.externalQueue) != 0 {
		t.Fatal("rejected external references mutated terminal admission state")
	}
	releaseShutdown()
	releaseCallback()
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "802 external-rejection Run") {
		t.Fatal("Run did not complete")
	}
	if err := value.refTimer(1); !errors.Is(err, errTerminated) {
		t.Fatalf("post-terminal Ref = %v", err)
	}
	if err := value.unrefTimer(1); !errors.Is(err, errTerminated) {
		t.Fatalf("post-terminal Unref = %v", err)
	}
	if finish, ok := value.beginPromisifyWorker(); ok || finish != nil {
		t.Fatal("Promisify worker admitted after terminal completion")
	}
	if value.submissionEpoch.Load() != epoch {
		t.Fatal("post-terminal operations changed the admission epoch")
	}
	assertSourceCleanupWake(t, value, timerValue, 1)
}

func TestSourceOwnerRegistrationFirstGateTerminalOvertake(t *testing.T) {
	for _, immediate := range []bool{false, true} {
		name := "Shutdown"
		if immediate {
			name = "Close"
		}
		t.Run(name, func(t *testing.T) {
			value := newLoop(false)
			runResult, _ := startSourceWaitingRun(t, value, lifecycleObserver{})
			firstGatePassed := make(chan struct{})
			resumeRegistration, releaseRegistration := newSourceRelease(t)
			registrationResults := make(chan registrationResult, 1)
			if err := value.submitToQueue(func() {
				id, err := value.prepareTimerRegistrationObserved(registrationObserver{
					firstGatePassed: func() {
						close(firstGatePassed)
						<-resumeRegistration
					},
				})
				registrationResults <- registrationResult{id: id, err: err}
			}); err != nil {
				t.Fatal(err)
			}
			waitSignal(t, firstGatePassed, name+" owner registration first gate")
			epoch := value.submissionEpoch.Load()

			terminalBoundary := make(chan struct{})
			resumeTerminal, releaseTerminal := newSourceRelease(t)
			terminalResult := make(chan error, 1)
			if immediate {
				go func() {
					terminalResult <- value.closeLoopObserved(lifecycleObserver{
						closeWait: func(stage closeWaitStage) {
							if stage == closeWaitWinningLoop {
								close(terminalBoundary)
							}
						},
					})
				}()
			} else {
				go func() {
					terminalResult <- value.shutdownObserved(lifecycleObserver{
						shutdownPublished: func(<-chan struct{}) {
							close(terminalBoundary)
							<-resumeTerminal
						},
					})
				}()
			}
			waitSignal(t, terminalBoundary, name+" owner registration terminal boundary")
			releaseRegistration()
			registration := receiveRegistration(t, registrationResults)
			if registration.id != 0 || !errors.Is(registration.err, errTerminated) {
				t.Fatalf("owner registration after %s = (%d, %v)", name, registration.id, registration.err)
			}
			if value.nextTimerID.Load() != 1 ||
				value.submissionEpoch.Load() != epoch ||
				len(value.timerMap) != 0 {
				t.Fatal("owner registration overtake did not preserve consumed-ID-only state")
			}
			releaseTerminal()
			if err := receiveError(t, terminalResult); err != nil {
				t.Fatal(err)
			}
			if !receiveBool(t, runResult, name+" owner registration Run") {
				t.Fatal("Run did not complete owner registration overtake")
			}
			assertSourceCleanupWake(t, value, nil, 1)
		})
	}
}

func TestSourceTerminalOperationPairs(t *testing.T) {
	t.Run("ShutdownThenShutdown", func(t *testing.T) {
		value := newLoop(false)
		runResult, runWaits := startSourceWaitingRun(t, value, lifecycleObserver{})
		timerValue := seedSourceTimer(t, value, true)
		waitSignal(t, runWaits, "Run wait after timer seed")
		published := make(chan struct{})
		resumeWinner, releaseWinner := newSourceRelease(t)
		winner := make(chan error, 1)
		go func() {
			winner <- value.shutdownObserved(lifecycleObserver{
				shutdownPublished: func(<-chan struct{}) {
					close(published)
					<-resumeWinner
				},
			})
		}()
		waitSignal(t, published, "winning Shutdown boundary")
		loserEntered := make(chan struct{})
		loser := make(chan error, 1)
		go func() {
			loser <- value.shutdownObserved(lifecycleObserver{
				shutdownEntered: func() { close(loserEntered) },
			})
		}()
		waitSignal(t, loserEntered, "losing Shutdown sync.Once entry")
		assertErrorBlocked(t, loser)
		releaseWinner()
		if err := receiveError(t, winner); err != nil {
			t.Fatalf("winning Shutdown = %v", err)
		}
		if err := receiveError(t, loser); !errors.Is(err, errTerminated) {
			t.Fatalf("losing Shutdown = %v", err)
		}
		if !receiveBool(t, runResult, "802 Shutdown-then-Shutdown Run") {
			t.Fatal("Run did not complete")
		}
		assertSourceCleanup(t, value, timerValue)
	})

	t.Run("CloseThenShutdown", func(t *testing.T) {
		value := newLoop(false)
		runResult, runWaits := startSourceWaitingRun(t, value, lifecycleObserver{})
		timerValue := seedSourceTimer(t, value, true)
		waitSignal(t, runWaits, "Run wait after timer seed")
		won := make(chan struct{})
		resumeWinner, releaseWinner := newSourceRelease(t)
		winner := make(chan error, 1)
		go func() {
			winner <- value.closeLoopObserved(lifecycleObserver{
				closeWon: func() {
					close(won)
					<-resumeWinner
				},
			})
		}()
		waitSignal(t, won, "winning Close boundary")
		if err := value.shutdown(); !errors.Is(err, errTerminated) {
			t.Fatalf("losing Shutdown = %v", err)
		}
		assertErrorBlocked(t, winner)
		releaseWinner()
		if err := receiveError(t, winner); err != nil {
			t.Fatalf("winning Close = %v", err)
		}
		if !receiveBool(t, runResult, "802 Close-then-Shutdown Run") {
			t.Fatal("Run did not complete")
		}
		assertSourceCleanup(t, value, timerValue)
	})

	t.Run("CloseThenClose", func(t *testing.T) {
		value := newLoop(false)
		runResult, runWaits := startSourceWaitingRun(t, value, lifecycleObserver{})
		timerValue := seedSourceTimer(t, value, true)
		waitSignal(t, runWaits, "Run wait after timer seed")
		won := make(chan struct{})
		resumeWinner, releaseWinner := newSourceRelease(t)
		winner := make(chan error, 1)
		go func() {
			winner <- value.closeLoopObserved(lifecycleObserver{
				closeWon: func() {
					close(won)
					<-resumeWinner
				},
			})
		}()
		waitSignal(t, won, "winning Close boundary")
		loserStage := make(chan closeWaitStage, 2)
		loser := make(chan error, 1)
		go func() {
			loser <- value.closeLoopObserved(lifecycleObserver{
				closeWait: func(stage closeWaitStage) { loserStage <- stage },
			})
		}()
		select {
		case stage := <-loserStage:
			if stage != closeWaitLosingTerminal {
				t.Fatalf("losing Close first stage = %d", stage)
			}
		case <-time.After(time.Second):
			t.Fatal("losing Close did not wait for terminalDone")
		}
		assertErrorBlocked(t, loser)
		releaseWinner()
		if err := receiveError(t, winner); err != nil {
			t.Fatalf("winning Close = %v", err)
		}
		if err := receiveError(t, loser); !errors.Is(err, errTerminated) {
			t.Fatalf("losing Close = %v", err)
		}
		if !receiveBool(t, runResult, "802 Close-then-Close Run") {
			t.Fatal("Run did not complete")
		}
		assertSourceCleanup(t, value, timerValue)
	})
}

func TestSourceLateIngressWakeAfterClose(t *testing.T) {
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
	admitted := make(chan struct{})
	wakePublished := make(chan struct{})
	resumeIngress, releaseIngress := newSourceRelease(t)
	unrefResult := make(chan error, 1)
	go func() {
		unrefResult <- value.unrefTimerObserved(1, referenceObserver{
			queueAdmitted: func() {
				close(admitted)
				<-resumeIngress
			},
			wakePublished: func() { close(wakePublished) },
		})
	}()
	waitSignal(t, admitted, "Unref queue admission")
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
	releaseCallback()
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "802 late-ingress Close Run") {
		t.Fatal("Run did not complete")
	}
	select {
	case <-value.fastWakeupCh:
	default:
		t.Fatal("Close did not leave its forced fast token")
	}
	if len(value.fastWakeupCh) != 0 {
		t.Fatal("Close fast token did not drain before late ingress resumed")
	}
	releaseIngress()
	waitSignal(t, wakePublished, "post-Close ingress wake publication")
	if err := receiveError(t, unrefResult); !errors.Is(err, errTerminated) {
		t.Fatalf("late-wake Unref = %v", err)
	}
	if len(value.fastWakeupCh) != 1 || value.wakePending.Load() != 0 {
		t.Fatal("post-Close ingress did not preserve its historical late fast token")
	}
	assertSourceCleanupWake(t, value, timerValue, 1)
}

func TestSourceCloseCASGapExecutesDetachedCallback(t *testing.T) {
	value := newLoop(false)
	id, err := value.prepareTimerRegistration()
	if err != nil || id != 1 {
		t.Fatalf("pre-Run registration = (%d, %v)", id, err)
	}
	type gapObservation struct {
		closeErr         error
		shutdownErr      error
		shutdownAgainErr error
		unrefErr         error
		timerValue       *timer
		refed            bool
		refedCount       int32
	}
	callbackStarted := make(chan struct{})
	callbackRelease, releaseCallback := newSourceRelease(t)
	callbackTwo := make(chan gapObservation, 1)
	refResult := make(chan error, 1)
	if err := value.submitToQueue(func() {
		close(callbackStarted)
		<-callbackRelease
	}); err != nil {
		t.Fatal(err)
	}
	if err := value.submitToQueue(func() {
		observation := gapObservation{
			closeErr:         value.closeLoop(),
			shutdownErr:      value.shutdown(),
			shutdownAgainErr: value.shutdown(),
			unrefErr:         value.unrefTimer(id),
			timerValue:       value.timerMap[id],
			refed:            value.timerMap[id].refed.Load(),
			refedCount:       value.refedTimerCount.Load(),
		}
		callbackTwo <- observation
		refResult <- value.refTimer(id)
	}); err != nil {
		t.Fatal(err)
	}
	runResult := startSourceRun(t, value)
	waitSignal(t, callbackStarted, "detached callback one")
	closeWon := make(chan struct{})
	resumeClose, releaseClose := newSourceRelease(t)
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{
			closeWon: func() {
				close(closeWon)
				<-resumeClose
			},
		})
	}()
	waitSignal(t, closeWon, "Close CAS boundary")
	releaseCallback()
	var observation gapObservation
	select {
	case observation = <-callbackTwo:
	case <-time.After(time.Second):
		t.Fatal("detached callback did not execute in Close CAS gap")
	}
	if !errors.Is(observation.closeErr, errReentrant) || observation.shutdownErr != nil || !errors.Is(observation.shutdownAgainErr, errTerminated) ||
		observation.unrefErr != nil || observation.timerValue == nil || observation.refed || observation.refedCount != 0 {
		t.Fatalf("Close CAS-gap callback results = %+v", observation)
	}
	assertErrorBlocked(t, refResult)
	releaseClose()
	if err := receiveError(t, refResult); !errors.Is(err, errTerminated) {
		t.Fatalf("Close CAS-gap Ref = %v", err)
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "802 Close CAS-gap Run") {
		t.Fatal("Run did not complete")
	}
	assertSourceCleanupWake(t, value, observation.timerValue, 1)
}

func TestSourceCloseCASGapDrainsQueuedUnref(t *testing.T) {
	value := newLoop(false)
	terminalTransition := make(chan struct{})
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			terminalTransition: func() { close(terminalTransition) },
		})
	}()
	waitSignal(t, value.runCh, "802 Run start")
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
	waitSignal(t, value.fastWakeupCh, "queued Unref wake")
	closeWon := make(chan struct{})
	resumeClose, releaseClose := newSourceRelease(t)
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{
			closeWon: func() {
				close(closeWon)
				<-resumeClose
			},
		})
	}()
	waitSignal(t, closeWon, "Close CAS boundary")
	releaseCallback()
	waitSignal(t, terminalTransition, "Run no-generation terminal transition")
	assertErrorBlocked(t, unrefResult)
	releaseClose()
	if err := receiveError(t, unrefResult); err != nil {
		t.Fatalf("Close-gap queued Unref = %v", err)
	}
	if timerValue.refed.Load() || value.refedTimerCount.Load() != 0 {
		t.Fatal("Run did not drain the queued Unref in Close's Terminating window")
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "802 Close-gap Unref Run") {
		t.Fatal("Run did not complete")
	}
	assertSourceCleanupWake(t, value, timerValue, 1)
}
