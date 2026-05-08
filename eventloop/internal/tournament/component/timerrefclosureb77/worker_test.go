package timerrefclosureb77

import (
	"errors"
	"sync"
	"testing"
)

func TestSourceAwakeTerminalWaitsPromisifyWorker(t *testing.T) {
	for _, immediate := range []bool{false, true} {
		name := "Shutdown"
		if immediate {
			name = "Close"
		}
		t.Run(name, func(t *testing.T) {
			value := newLoop(false)
			workerStarted := make(chan struct{})
			releaseWorker := make(chan struct{})
			settlement := make(chan error, 1)
			t.Cleanup(func() { releaseSignal(releaseWorker) })
			if err := value.promisifyObserved(func() {
				close(workerStarted)
				<-releaseWorker
			}, workerObserver{settlementPublished: func(err error) { settlement <- err }}); err != nil {
				t.Fatal(err)
			}
			waitSignal(t, workerStarted, "Awake Promisify worker")
			workerWait := make(chan struct{})
			terminalResult := make(chan error, 1)
			observer := lifecycleObserver{workerWaitStarted: func() { close(workerWait) }}
			if immediate {
				go func() { terminalResult <- value.closeLoopObserved(observer) }()
			} else {
				go func() { terminalResult <- value.shutdownObserved(observer) }()
			}
			waitSignal(t, workerWait, "Awake terminal worker barrier")
			assertErrorBlocked(t, terminalResult)
			if state(value.state.Load()) != stateTerminated || value.promisifyCount.Load() != 1 {
				t.Fatal("Awake terminal did not publish its worker barrier state")
			}
			assertChannelClosed(t, value.loopDone, "Awake terminal loopDone")
			close(releaseWorker)
			if err := receiveError(t, settlement); !errors.Is(err, errTerminated) {
				t.Fatalf("worker settlement = %v", err)
			}
			if err := receiveError(t, terminalResult); err != nil {
				t.Fatal(err)
			}
			if value.promisifyCount.Load() != 0 || len(value.queue) != 0 {
				t.Fatal("Awake terminal worker cleanup differs")
			}
			if err := value.promisify(func() {}); !errors.Is(err, errTerminated) {
				t.Fatalf("post-terminal Promisify = %v", err)
			}
		})
	}
}

func TestSourceStartedTerminalWaitsPromisifyWorker(t *testing.T) {
	for _, immediate := range []bool{false, true} {
		name := "Shutdown"
		if immediate {
			name = "Close"
		}
		t.Run(name, func(t *testing.T) {
			value := newLoop(false)
			runWaiting := make(chan struct{})
			resumeRun := make(chan struct{})
			t.Cleanup(func() { releaseSignal(resumeRun) })
			runResult := make(chan bool, 1)
			go func() {
				runResult <- value.runObserved(lifecycleObserver{runWait: func() {
					close(runWaiting)
					<-resumeRun
				}})
			}()
			waitSignal(t, runWaiting, "started terminal Run wait")
			workerStarted := make(chan struct{})
			releaseWorker := make(chan struct{})
			settlement := make(chan error, 1)
			t.Cleanup(func() { releaseSignal(releaseWorker) })
			if err := value.promisifyObserved(func() {
				close(workerStarted)
				<-releaseWorker
			}, workerObserver{settlementPublished: func(err error) { settlement <- err }}); err != nil {
				t.Fatal(err)
			}
			waitSignal(t, workerStarted, "started Promisify worker")
			terminalWoke := make(chan struct{})
			workerWait := make(chan struct{})
			terminalResult := make(chan error, 1)
			observer := lifecycleObserver{workerWaitStarted: func() { close(workerWait) }}
			if immediate {
				observer.closeWake = func() { close(terminalWoke) }
				go func() { terminalResult <- value.closeLoopObserved(observer) }()
			} else {
				observer.shutdownWake = func() { close(terminalWoke) }
				go func() { terminalResult <- value.shutdownObserved(observer) }()
			}
			waitSignal(t, terminalWoke, "started terminal wake")
			close(resumeRun)
			if !receiveBool(t, runResult, "started terminal Run") {
				t.Fatal("Run did not exit for terminal operation")
			}
			waitSignal(t, workerWait, "started terminal worker barrier")
			assertErrorBlocked(t, terminalResult)
			close(releaseWorker)
			if err := receiveError(t, settlement); !errors.Is(err, errTerminated) {
				t.Fatalf("worker settlement = %v", err)
			}
			if err := receiveError(t, terminalResult); err != nil {
				t.Fatal(err)
			}
			if value.promisifyCount.Load() != 0 || len(value.queue) != 0 {
				t.Fatal("started terminal worker cleanup differs")
			}
		})
	}
}

func TestSourcePromisifyPreventsAutoExitUntilWorkerCompletion(t *testing.T) {
	value := newLoop(true)
	workerStarted := make(chan struct{})
	releaseWorker := make(chan struct{})
	t.Cleanup(func() { releaseSignal(releaseWorker) })
	if err := value.promisify(func() {
		close(workerStarted)
		<-releaseWorker
	}); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, workerStarted, "auto-exit Promisify worker")
	runResult := startSourceRun(t, value)
	assertBoolBlocked(t, runResult, "worker-backed auto-exit Run")
	close(releaseWorker)
	if !receiveBool(t, runResult, "worker-backed auto-exit Run") {
		t.Fatal("Run did not auto-exit after worker completion")
	}
	if value.promisifyCount.Load() != 0 || state(value.state.Load()) != stateTerminated || len(value.queue) != 0 {
		t.Fatal("worker-backed auto-exit cleanup differs")
	}
}

func TestSourceTerminalPairsRespectWorkerBarrier(t *testing.T) {
	t.Run("ShutdownThenShutdown", func(t *testing.T) {
		value, releaseWorker := startBlockedWorker(t)
		workerWait := make(chan struct{})
		first := make(chan error, 1)
		go func() {
			first <- value.shutdownObserved(lifecycleObserver{workerWaitStarted: func() { close(workerWait) }})
		}()
		waitSignal(t, workerWait, "first Shutdown worker barrier")
		second := make(chan error, 1)
		go func() { second <- value.shutdown() }()
		assertErrorBlocked(t, first)
		assertErrorBlocked(t, second)
		close(releaseWorker)
		if err := receiveError(t, first); err != nil {
			t.Fatal(err)
		}
		if err := receiveError(t, second); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ShutdownThenClose", func(t *testing.T) {
		value, releaseWorker := startBlockedWorker(t)
		workerWait := make(chan struct{})
		shutdownResult := make(chan error, 1)
		go func() {
			shutdownResult <- value.shutdownObserved(lifecycleObserver{workerWaitStarted: func() { close(workerWait) }})
		}()
		waitSignal(t, workerWait, "Shutdown worker barrier")
		if err := value.closeLoop(); !errors.Is(err, errTerminated) {
			t.Fatalf("losing Close = %v", err)
		}
		assertErrorBlocked(t, shutdownResult)
		close(releaseWorker)
		if err := receiveError(t, shutdownResult); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("CloseThenShutdown", func(t *testing.T) {
		value, releaseWorker := startBlockedWorker(t)
		workerWait := make(chan struct{})
		closeResult := make(chan error, 1)
		go func() {
			closeResult <- value.closeLoopObserved(lifecycleObserver{workerWaitStarted: func() { close(workerWait) }})
		}()
		waitSignal(t, workerWait, "Close worker barrier")
		if err := value.shutdown(); !errors.Is(err, errTerminated) {
			t.Fatalf("losing Shutdown = %v", err)
		}
		if err := value.shutdown(); err != nil {
			t.Fatalf("post-loss Shutdown = %v", err)
		}
		assertErrorBlocked(t, closeResult)
		close(releaseWorker)
		if err := receiveError(t, closeResult); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("CloseThenClose", func(t *testing.T) {
		value, releaseWorker := startBlockedWorker(t)
		transitioned := make(chan struct{})
		resumeClose := make(chan struct{})
		t.Cleanup(func() { releaseSignal(resumeClose) })
		first := make(chan error, 1)
		go func() {
			first <- value.closeLoopObserved(lifecycleObserver{closeTransitioned: func() {
				close(transitioned)
				<-resumeClose
			}})
		}()
		waitSignal(t, transitioned, "first Close transition")
		second := make(chan error, 1)
		go func() { second <- value.closeLoop() }()
		assertErrorBlocked(t, second)
		close(resumeClose)
		if err := receiveError(t, second); !errors.Is(err, errTerminated) {
			t.Fatalf("losing Close = %v", err)
		}
		assertErrorBlocked(t, first)
		close(releaseWorker)
		if err := receiveError(t, first); err != nil {
			t.Fatal(err)
		}
	})
}

func TestSourceStartedTerminalPairsRespectWorkerBarrier(t *testing.T) {
	t.Run("ShutdownThenShutdown", func(t *testing.T) {
		fixture := startStartedWorkerTerminalFixture(t)
		terminalWake := make(chan struct{})
		workerWait := make(chan struct{})
		first := make(chan error, 1)
		go func() {
			first <- fixture.value.shutdownObserved(lifecycleObserver{
				shutdownWake: func() {
					close(terminalWake)
					fixture.resumeRun()
				},
				workerWaitStarted: func() { close(workerWait) },
			})
		}()
		waitSignal(t, terminalWake, "started first Shutdown wake")
		if !receiveBool(t, fixture.runResult, "started Shutdown pair Run") {
			t.Fatal("Run did not exit for started Shutdown pair")
		}
		waitSignal(t, workerWait, "started first Shutdown worker barrier")

		secondEntered := make(chan struct{})
		second := make(chan error, 1)
		go func() {
			second <- fixture.value.shutdownObserved(lifecycleObserver{
				shutdownOnceEntering: func() { close(secondEntered) },
			})
		}()
		waitSignal(t, secondEntered, "started second Shutdown entry")
		assertErrorBlocked(t, first)
		assertErrorBlocked(t, second)
		fixture.releaseWorker()
		if err := receiveError(t, first); err != nil {
			t.Fatalf("winning Shutdown = %v", err)
		}
		if err := receiveError(t, second); err != nil {
			t.Fatalf("joining Shutdown = %v", err)
		}
		assertStartedTerminalPairCleanup(t, fixture.value)
	})

	t.Run("ShutdownThenClose", func(t *testing.T) {
		fixture := startStartedWorkerTerminalFixture(t)
		terminalWake := make(chan struct{})
		workerWait := make(chan struct{})
		first := make(chan error, 1)
		go func() {
			first <- fixture.value.shutdownObserved(lifecycleObserver{
				shutdownWake: func() {
					close(terminalWake)
					fixture.resumeRun()
				},
				workerWaitStarted: func() { close(workerWait) },
			})
		}()
		waitSignal(t, terminalWake, "started Shutdown/Close wake")
		if !receiveBool(t, fixture.runResult, "started Shutdown/Close Run") {
			t.Fatal("Run did not exit for started Shutdown/Close pair")
		}
		waitSignal(t, workerWait, "started Shutdown/Close worker barrier")
		if err := fixture.value.closeLoop(); !errors.Is(err, errTerminated) {
			t.Fatalf("losing Close = %v", err)
		}
		assertErrorBlocked(t, first)
		fixture.releaseWorker()
		if err := receiveError(t, first); err != nil {
			t.Fatalf("winning Shutdown = %v", err)
		}
		assertStartedTerminalPairCleanup(t, fixture.value)
	})

	t.Run("CloseThenShutdown", func(t *testing.T) {
		fixture := startStartedWorkerTerminalFixture(t)
		terminalWake := make(chan struct{})
		workerWait := make(chan struct{})
		first := make(chan error, 1)
		go func() {
			first <- fixture.value.closeLoopObserved(lifecycleObserver{
				closeWake: func() {
					close(terminalWake)
					fixture.resumeRun()
				},
				workerWaitStarted: func() { close(workerWait) },
			})
		}()
		waitSignal(t, terminalWake, "started Close/Shutdown wake")
		if !receiveBool(t, fixture.runResult, "started Close/Shutdown Run") {
			t.Fatal("Run did not exit for started Close/Shutdown pair")
		}
		waitSignal(t, workerWait, "started Close/Shutdown worker barrier")
		if err := fixture.value.shutdown(); !errors.Is(err, errTerminated) {
			t.Fatalf("losing Shutdown = %v", err)
		}
		assertErrorBlocked(t, first)
		fixture.releaseWorker()
		if err := receiveError(t, first); err != nil {
			t.Fatalf("winning Close = %v", err)
		}
		if err := fixture.value.shutdown(); err != nil {
			t.Fatalf("post-loss Shutdown = %v", err)
		}
		assertStartedTerminalPairCleanup(t, fixture.value)
	})

	t.Run("CloseThenCloseTransitionGap", func(t *testing.T) {
		fixture := startStartedWorkerTerminalFixture(t)
		transitioned := make(chan struct{})
		resumeTransition := make(chan struct{})
		resumeTransitionClose := closeSignalOnce(resumeTransition)
		t.Cleanup(resumeTransitionClose)
		terminalWake := make(chan struct{})
		workerWait := make(chan struct{})
		first := make(chan error, 1)
		go func() {
			first <- fixture.value.closeLoopObserved(lifecycleObserver{
				closeTransitioned: func() {
					close(transitioned)
					<-resumeTransition
				},
				closeWake: func() {
					close(terminalWake)
					fixture.resumeRun()
				},
				workerWaitStarted: func() { close(workerWait) },
			})
		}()
		waitSignal(t, transitioned, "started first Close transition")
		secondWait := make(chan struct{})
		second := make(chan error, 1)
		go func() {
			second <- fixture.value.closeLoopObserved(lifecycleObserver{
				closeTerminatingWait: func() { close(secondWait) },
			})
		}()
		waitSignal(t, secondWait, "started second Close terminating wait")
		assertErrorBlocked(t, first)
		assertErrorBlocked(t, second)
		assertBoolBlocked(t, fixture.runResult, "pre-commit started Close pair Run")
		resumeTransitionClose()
		waitSignal(t, terminalWake, "started Close pair wake")
		if !receiveBool(t, fixture.runResult, "started Close pair Run") {
			t.Fatal("Run did not exit for started Close pair")
		}
		if err := receiveError(t, second); !errors.Is(err, errTerminated) {
			t.Fatalf("losing Close = %v", err)
		}
		waitSignal(t, workerWait, "started Close pair worker barrier")
		assertErrorBlocked(t, first)
		fixture.releaseWorker()
		if err := receiveError(t, first); err != nil {
			t.Fatalf("winning Close = %v", err)
		}
		assertStartedTerminalPairCleanup(t, fixture.value)
	})
}

type startedWorkerTerminalFixture struct {
	value         *loop
	runResult     <-chan bool
	resumeRun     func()
	releaseWorker func()
}

func startStartedWorkerTerminalFixture(t *testing.T) startedWorkerTerminalFixture {
	t.Helper()
	value := newLoop(false)
	runWaiting := make(chan struct{})
	runWaitingClose := closeSignalOnce(runWaiting)
	resumeRun := make(chan struct{})
	resumeRunClose := closeSignalOnce(resumeRun)
	runResult := make(chan bool, 1)
	runDone := make(chan struct{})
	workerStarted := make(chan struct{})
	releaseWorker := make(chan struct{})
	releaseWorkerClose := closeSignalOnce(releaseWorker)
	t.Cleanup(func() {
		resumeRunClose()
		closeResult := make(chan error, 1)
		go func() { closeResult <- value.closeLoop() }()
		releaseWorkerClose()
		err := receiveError(t, closeResult)
		waitSignal(t, runDone, "terminal-pair cleanup Run")
		if err != nil && !errors.Is(err, errTerminated) {
			t.Errorf("terminal-pair cleanup Close = %v", err)
		}
	})
	go func() {
		defer close(runDone)
		runResult <- value.runObserved(lifecycleObserver{runWait: func() {
			runWaitingClose()
			<-resumeRun
		}})
	}()
	waitSignal(t, runWaiting, "started terminal-pair Run wait")

	if err := value.promisify(func() {
		close(workerStarted)
		<-releaseWorker
	}); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, workerStarted, "started terminal-pair worker")
	return startedWorkerTerminalFixture{
		value:         value,
		runResult:     runResult,
		resumeRun:     resumeRunClose,
		releaseWorker: releaseWorkerClose,
	}
}

func assertStartedTerminalPairCleanup(t *testing.T, value *loop) {
	t.Helper()
	assertSourceTerminalCleanup(t, value, sourceTerminalCleanupExpectation{
		submissionEpoch: 1,
	}, "started terminal pair")
}

func TestSourceTerminalWorkerNonJoiningContrastsComplete(t *testing.T) {
	tests := []terminalWorkerContrast{
		{name: "AwakeShutdownThenWorkerClose", externalKind: terminalShutdown, workerKind: terminalClose},
		{name: "StartedShutdownThenWorkerClose", started: true, externalKind: terminalShutdown, workerKind: terminalClose},
		{name: "AwakeCloseThenWorkerShutdown", externalKind: terminalClose, workerKind: terminalShutdown},
		{name: "StartedCloseThenWorkerShutdown", started: true, externalKind: terminalClose, workerKind: terminalShutdown},
		{name: "AwakeCloseThenWorkerClose", externalKind: terminalClose, workerKind: terminalClose},
		{name: "StartedCloseThenWorkerClose", started: true, externalKind: terminalClose, workerKind: terminalClose},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { runTerminalWorkerContrast(t, test) })
	}
}

type terminalWorkerContrast struct {
	name         string
	started      bool
	externalKind terminalKind
	workerKind   terminalKind
}

func runTerminalWorkerContrast(t *testing.T, test terminalWorkerContrast) {
	t.Helper()
	value := newLoop(false)
	closeGap := test.started && test.externalKind == terminalClose && test.workerKind == terminalClose
	var runResult chan bool
	var resumeRun chan struct{}
	var resumeRunClose func()
	if test.started {
		runWaiting := make(chan struct{})
		resumeRun = make(chan struct{})
		resumeRunClose = closeSignalOnce(resumeRun)
		t.Cleanup(resumeRunClose)
		runResult = make(chan bool, 1)
		go func() {
			runResult <- value.runObserved(lifecycleObserver{runWait: func() {
				close(runWaiting)
				<-resumeRun
			}})
		}()
		waitSignal(t, runWaiting, "non-joining contrast Run wait")
	}

	workerStarted := make(chan struct{})
	workerGate := make(chan struct{})
	workerEnteredTerminal := make(chan struct{})
	closeTerminatingWait := make(chan struct{})
	workerResult := make(chan error, 1)
	settlement := make(chan error, 1)
	settlementExecuted := make(chan struct{})
	beforeWorkerDone := make(chan struct{})
	workerGateClose := closeSignalOnce(workerGate)
	t.Cleanup(workerGateClose)
	if err := value.promisifyObserved(func() {
		<-workerGate
		close(workerEnteredTerminal)
		if test.workerKind == terminalClose {
			workerResult <- value.closeLoopObserved(lifecycleObserver{closeTerminatingWait: func() {
				close(closeTerminatingWait)
			}})
			return
		}
		workerResult <- value.shutdown()
	}, workerObserver{
		workerStarted:       func() { close(workerStarted) },
		settlementPublished: func(err error) { settlement <- err },
		settlementExecuted:  func() { close(settlementExecuted) },
		beforeWorkerDone:    func() { close(beforeWorkerDone) },
	}); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, workerStarted, "non-joining contrast worker")

	terminalWake := make(chan struct{})
	beforeCommit := make(chan struct{})
	allowCommit := make(chan struct{})
	workerWait := make(chan struct{})
	workerWaitFinished := make(chan struct{})
	allowCommitClose := closeSignalOnce(allowCommit)
	t.Cleanup(allowCommitClose)
	observer := lifecycleObserver{
		shutdownTransitioned: func() {
			if test.started {
				workerGateClose()
				<-closeTerminatingWait
			}
		},
		shutdownWake: func() {
			close(terminalWake)
			resumeRunClose()
		},
		beforeShutdownCommit: func() {
			close(beforeCommit)
			<-allowCommit
		},
		closeTransitioned: func() {
			if closeGap {
				workerGateClose()
				<-closeTerminatingWait
			}
		},
		closeWake: func() {
			close(terminalWake)
			if !closeGap {
				workerGateClose()
			}
		},
		workerWaitStarted: func() {
			if !test.started {
				workerGateClose()
			}
			close(workerWait)
		},
		workerWaitFinished: func() { close(workerWaitFinished) },
	}
	terminalResult := make(chan error, 1)
	if test.externalKind == terminalClose {
		go func() { terminalResult <- value.closeLoopObserved(observer) }()
	} else {
		go func() { terminalResult <- value.shutdownObserved(observer) }()
	}

	runCompleted := false
	if !test.started {
		waitSignal(t, workerWait, "Awake non-joining worker barrier")
		assertChannelClosed(t, value.loopDone, "Awake non-joining loopDone")
	} else if test.externalKind == terminalShutdown {
		waitSignal(t, terminalWake, "started Shutdown wake")
		if !receiveBool(t, runResult, "started Shutdown contrast Run") {
			t.Fatal("Run did not exit for started Shutdown contrast")
		}
		runCompleted = true
		waitSignal(t, beforeCommit, "started Shutdown commit boundary")
	} else {
		waitSignal(t, terminalWake, "started Close wake")
	}
	waitSignal(t, workerEnteredTerminal, "non-joining contrast terminal call")
	if closeGap {
		assertChannelClosed(t, closeTerminatingWait, "started Close transition-gap worker wait")
		assertChannelOpen(t, value.loopDone, "started Close transition-gap loopDone")
		assertErrorBlocked(t, workerResult)
		assertErrorBlocked(t, terminalResult)
		assertBoolBlocked(t, runResult, "started Close transition-gap Run")
		resumeRunClose()
		if !receiveBool(t, runResult, "started Close transition-gap Run") {
			t.Fatal("Run did not independently release the worker Close")
		}
		runCompleted = true
	}
	if err := receiveError(t, workerResult); !errors.Is(err, errTerminated) {
		t.Fatalf("worker terminal result = %v", err)
	}
	wantSettlement := errTerminated
	if test.started && test.externalKind == terminalShutdown {
		wantSettlement = nil
	}
	if err := receiveError(t, settlement); !errors.Is(err, wantSettlement) || err != nil && wantSettlement == nil {
		t.Fatalf("worker settlement = %v, want %v", err, wantSettlement)
	}
	waitSignal(t, beforeWorkerDone, "non-joining worker completion")

	if test.started && test.externalKind == terminalShutdown {
		if state(value.state.Load()) != stateTerminating || value.promisifyCount.Load() != 0 ||
			len(value.queue) != 1 || value.submissionEpoch.Load() != 2 {
			t.Fatal("started Shutdown did not retain the Terminating settlement for drain")
		}
		assertChannelOpen(t, settlementExecuted, "pre-commit worker settlement execution")
		assertErrorBlocked(t, terminalResult)
		allowCommitClose()
	} else if test.started && !runCompleted {
		assertBoolBlocked(t, runResult, "started Close contrast Run")
		resumeRunClose()
		if !receiveBool(t, runResult, "started Close contrast Run") {
			t.Fatal("Run did not exit for started Close contrast")
		}
	}
	waitSignal(t, workerWait, "non-joining worker barrier")
	waitSignal(t, workerWaitFinished, "non-joining worker barrier completion")
	if err := receiveError(t, terminalResult); err != nil {
		t.Fatalf("external terminal result = %v", err)
	}
	wantEpoch := uint64(1)
	if test.started && test.externalKind == terminalShutdown {
		wantEpoch = 2
		waitSignal(t, settlementExecuted, "drained worker settlement")
	} else {
		assertChannelOpen(t, settlementExecuted, "rejected worker settlement execution")
	}
	if test.externalKind == terminalClose && test.workerKind == terminalShutdown {
		if err := value.shutdown(); err != nil {
			t.Fatalf("post-loss Shutdown = %v", err)
		}
	}
	expectation := sourceTerminalCleanupExpectation{submissionEpoch: wantEpoch}
	if test.started && test.externalKind == terminalShutdown {
		expectation.fastWake = 1
		expectation.retainedQueue = true
	}
	assertSourceTerminalCleanup(t, value, expectation, "non-joining contrast")
}

func TestSourceOwnerTerminalCallsAfterCloseCommitComplete(t *testing.T) {
	for _, ownerKind := range []terminalKind{terminalClose, terminalShutdown} {
		name := "OwnerClose"
		if ownerKind == terminalShutdown {
			name = "OwnerShutdown"
		}
		t.Run(name, func(t *testing.T) {
			value := newLoop(false)
			callbackEntered := make(chan struct{})
			ownerIdentity := make(chan struct{})
			callbackGate := make(chan struct{})
			callbackResult := make(chan error, 1)
			callbackGateClose := closeSignalOnce(callbackGate)
			t.Cleanup(callbackGateClose)
			if err := value.submitToQueue(func() {
				close(callbackEntered)
				if value.isOwner() {
					close(ownerIdentity)
				}
				<-callbackGate
				if ownerKind == terminalClose {
					callbackResult <- value.closeLoop()
					return
				}
				callbackResult <- value.shutdown()
			}); err != nil {
				t.Fatal(err)
			}
			runResult := make(chan bool, 1)
			go func() { runResult <- value.run() }()
			waitSignal(t, callbackEntered, "owner completion contrast callback")
			waitSignal(t, ownerIdentity, "owner completion contrast identity")
			terminalWake := make(chan struct{})
			closeResult := make(chan error, 1)
			go func() {
				closeResult <- value.closeLoopObserved(lifecycleObserver{closeWake: func() {
					callbackGateClose()
					close(terminalWake)
				}})
			}()
			waitSignal(t, terminalWake, "owner completion contrast Close wake")
			if err := receiveError(t, callbackResult); !errors.Is(err, errTerminated) {
				t.Fatalf("owner terminal result = %v", err)
			}
			if !receiveBool(t, runResult, "owner completion contrast Run") {
				t.Fatal("Run did not exit after owner terminal contrast")
			}
			if err := receiveError(t, closeResult); err != nil {
				t.Fatalf("winning Close = %v", err)
			}
			if ownerKind == terminalShutdown {
				if err := value.shutdown(); err != nil {
					t.Fatalf("post-loss Shutdown = %v", err)
				}
			}
			assertSourceTerminalCleanup(t, value, sourceTerminalCleanupExpectation{
				submissionEpoch: 1,
				fastWake:        1,
			}, "owner completion contrast")
		})
	}
}

type sourceTerminalCleanupExpectation struct {
	submissionEpoch uint64
	fastWake        int
	retainedQueue   bool
}

func assertSourceTerminalCleanup(
	t *testing.T,
	value *loop,
	expectation sourceTerminalCleanupExpectation,
	name string,
) {
	t.Helper()
	assertNilSlots(t, value.queue)
	assertNilSlots(t, value.spare)
	queueShapeMatches := value.queue == nil
	if expectation.retainedQueue {
		queueShapeMatches = value.queue != nil && len(value.queue) == 0 && cap(value.queue) != 0
	}
	if state(value.state.Load()) != stateTerminated || value.promisifyCount.Load() != 0 ||
		value.activeRun != nil || value.activeTerminal != nil || value.ownerID.Load() != 0 ||
		!queueShapeMatches || value.spare != nil || len(value.fastWakeupCh) != expectation.fastWake ||
		value.wakeUpSignalPending.Load() != 0 || value.quiescing.Load() ||
		value.submissionEpoch.Load() != expectation.submissionEpoch {
		t.Fatalf(
			"%s cleanup differs: state=%d workers=%d activeRun=%p activeTerminal=%p owner=%d queueNil=%t queueLen=%d queueCap=%d spareNil=%t fastWake=%d wakePending=%d quiescing=%t epoch=%d",
			name,
			value.state.Load(),
			value.promisifyCount.Load(),
			value.activeRun,
			value.activeTerminal,
			value.ownerID.Load(),
			value.queue == nil,
			len(value.queue),
			cap(value.queue),
			value.spare == nil,
			len(value.fastWakeupCh),
			value.wakeUpSignalPending.Load(),
			value.quiescing.Load(),
			value.submissionEpoch.Load(),
		)
	}
	assertChannelClosed(t, value.loopDone, name+" loopDone")
}

func closeSignalOnce(signal chan struct{}) func() {
	var once sync.Once
	return func() { once.Do(func() { close(signal) }) }
}

func startBlockedWorker(t *testing.T) (*loop, chan struct{}) {
	t.Helper()
	value := newLoop(false)
	started := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { releaseSignal(release) })
	if err := value.promisify(func() {
		close(started)
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, started, "blocked Promisify worker")
	return value, release
}

func assertBoolBlocked(t *testing.T, result <-chan bool, name string) {
	t.Helper()
	select {
	case value := <-result:
		t.Fatalf("%s returned early: %t", name, value)
	default:
	}
}
