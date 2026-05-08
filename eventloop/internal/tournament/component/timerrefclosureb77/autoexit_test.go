package timerrefclosureb77

import (
	"errors"
	"testing"
)

func TestSourceAutoExitStoreOvertakesShutdownTransition(t *testing.T) {
	value := newLoop(true)
	autoExitPrepared := make(chan struct{})
	resumeAutoExit := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-resumeAutoExit:
		default:
			close(resumeAutoExit)
		}
	})
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			autoExitPrepared: func() {
				close(autoExitPrepared)
				<-resumeAutoExit
			},
		})
	}()
	waitSignal(t, autoExitPrepared, "final auto-exit observation")
	if state(value.state.Load()) != stateRunning || value.ownerID.Load() == 0 || !value.quiescing.Load() {
		t.Fatal("auto-exit continuation was not captured from Running quiescence")
	}

	shutdownTransitioned := make(chan struct{})
	shutdownWoke := make(chan struct{})
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			shutdownTransitioned: func() { close(shutdownTransitioned) },
			shutdownWake:         func() { close(shutdownWoke) },
		})
	}()
	waitSignal(t, shutdownTransitioned, "Shutdown transition")
	waitSignal(t, shutdownWoke, "Shutdown wake")
	if state(value.state.Load()) != stateTerminating || !value.quiescing.Load() ||
		len(value.fastWakeupCh) != 1 || value.wakeAttempts.Load() != 1 ||
		value.wakeSuccesses.Load() != 1 || value.wakeRejections.Load() != 0 {
		t.Fatal("Shutdown did not occupy the historical auto-exit Store gap")
	}
	select {
	case result := <-runResult:
		t.Fatalf("Run returned before auto-exit release: %v", result)
	default:
	}
	assertErrorBlocked(t, shutdownResult)

	close(resumeAutoExit)
	if !receiveBool(t, runResult, "auto-exit overlap Run") {
		t.Fatal("Run did not complete its captured auto-exit continuation")
	}
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if state(value.state.Load()) != stateTerminated || value.ownerID.Load() != 0 ||
		value.quiescing.Load() || value.activeRun != nil || value.activeTerminal != nil ||
		len(value.fastWakeupCh) != 1 || value.wakeUpSignalPending.Load() != 0 ||
		value.wakeAttempts.Load() != 1 || value.wakeSuccesses.Load() != 1 || value.wakeRejections.Load() != 0 {
		t.Fatal("auto-exit and Shutdown overlap did not settle to the source residue")
	}
	assertChannelClosed(t, value.loopDone, "auto-exit overlap loopDone")
}

func TestSourceTerminalOvertakesAutoExitQuiescing(t *testing.T) {
	for _, immediate := range []bool{false, true} {
		name := "Shutdown"
		if immediate {
			name = "Close"
		}
		t.Run(name, func(t *testing.T) {
			value := newLoop(true)
			quiescing := make(chan struct{})
			resumeAutoExit := make(chan struct{})
			resumeAutoExitClose := closeSignalOnce(resumeAutoExit)
			t.Cleanup(resumeAutoExitClose)

			runResult := make(chan bool, 1)
			go func() {
				runResult <- value.runObserved(lifecycleObserver{
					autoExitQuiescing: func() {
						close(quiescing)
						<-resumeAutoExit
					},
				})
			}()
			waitSignal(t, quiescing, "auto-exit quiescing")

			transitioned := make(chan struct{})
			woke := make(chan struct{})
			terminalResult := make(chan error, 1)
			if immediate {
				go func() {
					terminalResult <- value.closeLoopObserved(lifecycleObserver{
						closeTransitioned: func() { close(transitioned) },
						closeWake:         func() { close(woke) },
					})
				}()
			} else {
				go func() {
					terminalResult <- value.shutdownObserved(lifecycleObserver{
						shutdownTransitioned: func() { close(transitioned) },
						shutdownWake:         func() { close(woke) },
					})
				}()
			}

			waitSignal(t, transitioned, name+" transition")
			waitSignal(t, woke, name+" wake")
			wantState := stateTerminating
			wantQuiescing := true
			if immediate {
				wantState = stateTerminated
				wantQuiescing = false
			}
			if state(value.state.Load()) != wantState ||
				value.quiescing.Load() != wantQuiescing ||
				value.activeTerminal == nil {
				t.Fatal("terminal operation did not occupy the auto-exit revalidation window")
			}
			assertBoolBlocked(t, runResult, "quiescing Run")
			assertErrorBlocked(t, terminalResult)

			resumeAutoExitClose()
			if !receiveBool(t, runResult, "terminal-overrun auto-exit Run") {
				t.Fatal("Run did not abandon invalidated auto-exit")
			}
			if err := receiveError(t, terminalResult); err != nil {
				t.Fatal(err)
			}

			wantSuccesses := uint64(1)
			wantRejections := uint64(0)
			if immediate {
				wantSuccesses = 0
				wantRejections = 1
			}
			if value.wakeAttempts.Load() != 1 ||
				value.wakeSuccesses.Load() != wantSuccesses ||
				value.wakeRejections.Load() != wantRejections {
				t.Fatal("terminal wake classification differs")
			}
			assertSourceTerminalCleanup(
				t,
				value,
				sourceTerminalCleanupExpectation{},
				"terminal-overrun auto-exit",
			)
		})
	}
}

func TestSourceAutoExitStoreOvertakesCloseTransition(t *testing.T) {
	value := newLoop(true)
	autoExitPrepared := make(chan struct{})
	resumeAutoExit := make(chan struct{})
	resumeAutoExitClose := closeSignalOnce(resumeAutoExit)
	t.Cleanup(resumeAutoExitClose)

	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			autoExitPrepared: func() {
				close(autoExitPrepared)
				<-resumeAutoExit
			},
		})
	}()
	waitSignal(t, autoExitPrepared, "captured auto-exit continuation")

	closeTransitioned := make(chan struct{})
	resumeClose := make(chan struct{})
	resumeCloseCall := closeSignalOnce(resumeClose)
	t.Cleanup(resumeCloseCall)
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{
			closeTransitioned: func() {
				close(closeTransitioned)
				<-resumeClose
			},
		})
	}()
	waitSignal(t, closeTransitioned, "Close transition")
	if state(value.state.Load()) != stateTerminating ||
		!value.quiescing.Load() ||
		value.activeTerminal == nil {
		t.Fatal("Close did not occupy the captured auto-exit Store window")
	}
	assertBoolBlocked(t, runResult, "captured auto-exit Run")
	assertErrorBlocked(t, closeResult)

	resumeAutoExitClose()
	if !receiveBool(t, runResult, "Close-overrun auto-exit Run") {
		t.Fatal("Run did not commit its captured auto-exit continuation")
	}
	assertChannelClosed(t, value.loopDone, "Close-overrun loopDone")
	resumeCloseCall()
	if err := receiveError(t, closeResult); !errors.Is(err, errTerminated) {
		t.Fatalf("overtaken Close = %v", err)
	}

	operation := value.activeTerminal
	if operation == nil ||
		operation.kind != terminalClose ||
		!operation.started ||
		operation.run == nil ||
		!operation.run.exited.Load() {
		t.Fatal("overtaken Close did not retain its historical terminal operation")
	}
	if state(value.state.Load()) != stateTerminated ||
		value.ownerID.Load() != 0 ||
		value.activeRun != nil ||
		value.quiescing.Load() ||
		value.queue != nil ||
		value.spare != nil ||
		len(value.timerMap) != 0 ||
		len(value.fds) != 0 ||
		value.refedTimerCount.Load() != 0 ||
		value.userIOFDCount.Load() != 0 ||
		len(value.fastWakeupCh) != 0 ||
		value.wakeAttempts.Load() != 0 {
		t.Fatal("captured auto-exit and Close residue differs")
	}
}
