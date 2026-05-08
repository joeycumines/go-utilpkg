package timerrefclosuresimple

import (
	"errors"
	"testing"
	"time"
)

func TestSourceTerminalLosersReturnDuringWinnerPause(t *testing.T) {
	value := newLoop(false)
	callbackEntered := make(chan struct{})
	ownerReference := make(chan struct{})
	ownerReferenceResult := make(chan error, 1)
	releaseCallback := make(chan struct{})
	winnerPaused := make(chan struct{})
	resumeWinner := make(chan struct{})
	winnerWaiting := make(chan struct{})
	t.Cleanup(func() {
		releaseSignal(resumeWinner)
		releaseSignal(ownerReference)
		releaseSignal(releaseCallback)
		if state(value.state.Load()) != stateTerminated {
			_ = value.closeLoop()
		}
	})
	if err := value.submitToQueue(func() {
		close(callbackEntered)
		<-ownerReference
		ownerReferenceResult <- value.refTimer(1)
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
	close(ownerReference)
	if err := receiveError(t, ownerReferenceResult); err != nil {
		t.Fatalf("Run-owner reference during Terminating = %v", err)
	}

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
	if state(value.state.Load()) != stateTerminating {
		t.Fatal("terminal loser disturbed the winning operation")
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
	if value.ownerID.Load() != 0 || value.activeRun != nil || value.activeTerminal != nil ||
		state(value.state.Load()) != stateTerminated {
		t.Fatal("winning terminal operation did not settle")
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

func TestStartedCloseDiscardsAcceptedWork(t *testing.T) {
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
	waitSignal(t, secondWait, "started Close Run wait")
	closeWon := make(chan struct{})
	resumeCloseWin := make(chan struct{})
	closeWoke := make(chan struct{})
	resumeClose := make(chan struct{})
	completionPending := make(chan struct{})
	resumeCompletion := make(chan struct{})
	t.Cleanup(func() {
		releaseSignal(resumeCloseWin)
		releaseSignal(resumeClose)
		releaseSignal(resumeCompletion)
	})
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{
			closeWon: func() {
				close(closeWon)
				<-resumeCloseWin
			},
			closeWake: func() {
				close(closeWoke)
				<-resumeClose
			},
			completionPending: func() {
				close(completionPending)
				<-resumeCompletion
			},
		})
	}()
	waitSignal(t, closeWon, "started Close ownership")
	admitted := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- value.unrefTimerObserved(1, referenceObserver{queueAdmitted: func(uint64) { close(admitted) }})
	}()
	waitSignal(t, admitted, "Terminating Close Unref admission")
	assertErrorBlocked(t, result)
	close(resumeCloseWin)
	waitSignal(t, closeWoke, "started Close wake")
	close(resumeRun)
	if !receiveBool(t, runResult, "started Close Run") {
		t.Fatal("Run did not exit for Close")
	}
	assertChannelClosed(t, value.loopDone, "started Close loopDone")
	assertErrorBlocked(t, result)
	if state(value.state.Load()) != stateTerminated || !timerValue.refed.Load() ||
		len(value.queue) != 1 || value.activeTerminal == nil {
		t.Fatal("Close discarded accepted work before terminal completion")
	}
	close(resumeClose)
	waitSignal(t, completionPending, "Close cleanup before terminal completion")
	assertErrorBlocked(t, result)
	assertErrorBlocked(t, closeResult)
	close(resumeCompletion)
	if err := receiveError(t, result); !errors.Is(err, errTerminated) {
		t.Fatalf("started Close result = %v", err)
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
	if !timerValue.refed.Load() {
		t.Fatal("Close executed an accepted closure")
	}
	if len(value.timerMap) != 0 || len(value.queue) != 0 {
		t.Fatal("started Close cleanup failed")
	}
}

func TestSourcePreRunReferenceOutcomes(t *testing.T) {
	t.Run("RunStarts", func(t *testing.T) {
		value := newLoop(false)
		waiting := make(chan struct{})
		result := make(chan error, 1)
		go func() {
			result <- value.unrefTimerObserved(1, referenceObserver{
				runWaiting: func() { close(waiting) },
			})
		}()
		waitSignal(t, waiting, "pre-Run reference wait")
		runResult := make(chan bool, 1)
		go func() { runResult <- value.run() }()
		if err := receiveError(t, result); err != nil {
			t.Fatal(err)
		}
		if err := value.closeLoop(); err != nil {
			t.Fatal(err)
		}
		if !receiveBool(t, runResult, "pre-Run reference Run") {
			t.Fatal("Run did not complete Close")
		}
	})

	t.Run("CloseWins", func(t *testing.T) {
		value := newLoop(false)
		waiting := make(chan struct{})
		result := make(chan error, 1)
		go func() {
			result <- value.unrefTimerObserved(1, referenceObserver{
				runWaiting: func() { close(waiting) },
			})
		}()
		waitSignal(t, waiting, "pre-Run Close wait")
		if err := value.closeLoop(); err != nil {
			t.Fatal(err)
		}
		if err := receiveError(t, result); !errors.Is(err, errTerminated) {
			t.Fatalf("pre-Run reference after Close = %v", err)
		}
	})

	t.Run("Deadline", func(t *testing.T) {
		value := newLoop(false)
		deadline := make(chan time.Time)
		close(deadline)
		err := value.unrefTimerObserved(1, referenceObserver{runDeadline: deadline})
		if !errors.Is(err, errNotRunning) {
			t.Fatalf("pre-Run reference deadline = %v", err)
		}
		if err := value.closeLoop(); err != nil {
			t.Fatal(err)
		}
	})
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
		value.wakeUpSignalPending.Load() != 0 || value.wakeAttempts.Load() != 1 ||
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
