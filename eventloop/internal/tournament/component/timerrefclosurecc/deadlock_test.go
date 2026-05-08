//go:build !js && !wasip1

package timerrefclosurecc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	historicalDeadlockModeEnv = "EVENTLOOP_TIMERREFCLOSURE_DEADLOCK_MODE"
	historicalDeadlockPrefix  = "TIMERREFCLOSURE_DEADLOCK:"
)

type historicalDeadlockCase struct {
	mode      string
	required  []string
	forbidden []string
}

func TestSourceHistoricalTerminalDeadlocks(t *testing.T) {
	if mode := os.Getenv(historicalDeadlockModeEnv); mode != "" {
		runHistoricalDeadlockChild(t, mode)
		return
	}

	cases := []historicalDeadlockCase{
		{mode: "worker-shutdown-awake", required: []string{
			"worker-committed", "worker-started", "shutdown-transitioned", "awake-barrier-state-ok", "worker-wait-started",
		}},
		{mode: "worker-shutdown-started", required: []string{
			"run-paused", "worker-committed", "worker-started", "shutdown-transitioned", "terminal-wake", "run-returned", "before-shutdown-commit", "worker-wait-started",
		}},
		{mode: "worker-close-awake", required: []string{
			"worker-committed", "worker-started", "close-transitioned", "awake-barrier-state-ok", "worker-wait-started",
		}},
		{mode: "worker-close-started", required: []string{
			"run-paused", "worker-committed", "worker-started", "close-transitioned", "terminal-wake", "run-returned", "worker-wait-started",
		}},
		{mode: "worker-close-started-owner-callback", required: []string{
			"worker-committed", "worker-started", "callback-admitted", "callback-entered", "owner-identity-ok", "worker-terminal-entered", "close-transitioned", "terminal-wake", "loop-done-open", "callback-waiting-worker",
		}, forbidden: []string{"run-returned", "callback-returned", "worker-wait-started"}},
		{mode: "worker-shutdown-started-owner-callback-background", required: []string{
			"worker-committed", "worker-started", "callback-admitted", "callback-entered", "owner-identity-ok", "worker-terminal-entered", "shutdown-once-entering", "shutdown-transitioned", "terminal-wake", "loop-done-open", "callback-waiting-worker",
		}, forbidden: []string{"run-returned", "callback-returned", "before-shutdown-commit", "worker-wait-started"}},
		{mode: "external-shutdown-worker-shutdown-awake", required: []string{
			"worker-committed", "worker-started", "shutdown-transitioned", "awake-barrier-state-ok", "worker-wait-started", "losing-shutdown-once-entering",
		}},
		{mode: "external-shutdown-worker-shutdown-started", required: []string{
			"run-paused", "worker-committed", "worker-started", "shutdown-transitioned", "terminal-wake", "run-returned", "before-shutdown-commit", "worker-wait-started", "losing-shutdown-once-entering",
		}},
		{mode: "external-shutdown-worker-close-owner-callback", required: []string{
			"worker-committed", "worker-started", "callback-admitted", "callback-entered", "owner-identity-ok", "shutdown-transitioned", "worker-terminal-entered", "losing-close-entered", "callback-waiting-worker", "terminal-wake", "loop-done-open",
		}, forbidden: []string{"run-returned", "callback-returned", "before-shutdown-commit", "worker-wait-started"}},
		{mode: "external-shutdown-worker-shutdown-owner-callback", required: []string{
			"worker-committed", "worker-started", "callback-admitted", "callback-entered", "owner-identity-ok", "shutdown-transitioned", "worker-terminal-entered", "losing-shutdown-once-entering", "callback-waiting-worker", "terminal-wake", "loop-done-open",
		}, forbidden: []string{"run-returned", "callback-returned", "before-shutdown-commit", "worker-wait-started"}},
		{mode: "external-close-gap-worker-close-owner-callback", required: []string{
			"worker-committed", "worker-started", "callback-admitted", "callback-entered", "owner-identity-ok", "close-transitioned", "worker-terminal-entered", "losing-close-entered", "callback-waiting-worker", "terminal-wake", "loop-done-open",
		}, forbidden: []string{"run-returned", "callback-returned", "worker-wait-started"}},
		{mode: "owner-close-winning-started", required: []string{
			"run-paused", "callback-admitted", "callback-entered", "owner-identity-ok", "close-transitioned", "terminal-wake", "loop-done-open",
		}, forbidden: []string{"run-returned"}},
		{mode: "owner-shutdown-winning-started-background", required: []string{
			"run-paused", "callback-admitted", "callback-entered", "owner-identity-ok", "shutdown-once-entering", "shutdown-transitioned", "terminal-wake", "loop-done-open",
		}, forbidden: []string{"run-returned", "before-shutdown-commit", "worker-wait-started"}},
		{mode: "owner-close-losing-shutdown", required: []string{
			"callback-admitted", "callback-entered", "owner-identity-ok", "shutdown-transitioned", "close-terminating-wait", "terminal-wake", "loop-done-open",
		}, forbidden: []string{"run-returned", "before-shutdown-commit", "worker-wait-started"}},
		{mode: "owner-close-losing-close-gap", required: []string{
			"callback-admitted", "callback-entered", "owner-identity-ok", "close-transitioned", "close-terminating-wait", "terminal-wake", "loop-done-open",
		}, forbidden: []string{"run-returned", "worker-wait-started"}},
		{mode: "owner-shutdown-losing-shutdown-background", required: []string{
			"callback-admitted", "callback-entered", "owner-identity-ok", "shutdown-transitioned", "shutdown-once-entering", "terminal-wake", "loop-done-open",
		}, forbidden: []string{"run-returned", "before-shutdown-commit", "worker-wait-started"}},
	}

	timeout := historicalDeadlockTimeout(t, len(cases))
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		t.Run(test.mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			command := exec.CommandContext(ctx, executable, "-test.run=^TestSourceHistoricalTerminalDeadlocks$", "-test.count=1", "-test.timeout=1h", "-test.v")
			command.Env = historicalDeadlockEnvironment(test.mode)
			command.WaitDelay = 250 * time.Millisecond
			output, err := command.CombinedOutput()
			if ctx.Err() != context.DeadlineExceeded {
				t.Fatalf("child mode %s returned before the historical cycle timeout: %v\n%s", test.mode, err, output)
			}
			if err == nil {
				t.Fatalf("child mode %s timed out without a command error\n%s", test.mode, output)
			}
			text := string(output)
			for _, phase := range test.required {
				if !strings.Contains(text, historicalDeadlockMarker(test.mode, phase)) {
					t.Fatalf("child mode %s did not reach %s\n%s", test.mode, phase, output)
				}
			}
			for _, phase := range []string{
				"invalid", "terminal-returned", "external-terminal-returned", "worker-returned",
				"settlement-published", "before-worker-done", "worker-wait-finished",
			} {
				if strings.Contains(text, historicalDeadlockMarker(test.mode, phase)) {
					t.Fatalf("child mode %s unexpectedly reached %s\n%s", test.mode, phase, output)
				}
			}
			for _, phase := range test.forbidden {
				if strings.Contains(text, historicalDeadlockMarker(test.mode, phase)) {
					t.Fatalf("child mode %s unexpectedly reached %s\n%s", test.mode, phase, output)
				}
			}
		})
	}
}

func historicalDeadlockTimeout(t *testing.T, count int) time.Duration {
	t.Helper()
	timeout := 5 * time.Second
	if deadline, ok := t.Deadline(); ok {
		remaining := time.Until(deadline) - time.Second
		if remaining <= 0 {
			t.Fatal("historical deadlock subprocesses have no time before the test deadline")
		}
		perChild := remaining / time.Duration(count+1)
		if perChild < timeout {
			timeout = perChild
		}
	}
	if timeout < time.Second {
		t.Fatalf("historical deadlock subprocess timeout %s is too short", timeout)
	}
	return timeout
}

func historicalDeadlockEnvironment(mode string) []string {
	prefix := historicalDeadlockModeEnv + "="
	environment := make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, prefix) {
			environment = append(environment, value)
		}
	}
	return append(environment, prefix+mode)
}

func historicalDeadlockMarker(mode, phase string) string {
	return historicalDeadlockPrefix + mode + ":" + phase
}

func runHistoricalDeadlockChild(t *testing.T, mode string) {
	t.Helper()
	emit := func(phase string) {
		_, _ = fmt.Fprintln(os.Stdout, historicalDeadlockMarker(mode, phase))
	}
	var unexpected <-chan string
	switch mode {
	case "worker-shutdown-awake":
		unexpected = startWinningWorkerDeadlock(t, false, false, emit)
	case "worker-shutdown-started":
		unexpected = startWinningWorkerDeadlock(t, false, true, emit)
	case "worker-close-awake":
		unexpected = startWinningWorkerDeadlock(t, true, false, emit)
	case "worker-close-started":
		unexpected = startWinningWorkerDeadlock(t, true, true, emit)
	case "worker-close-started-owner-callback":
		unexpected = startWorkerOwnerCallbackDeadlock(t, true, emit)
	case "worker-shutdown-started-owner-callback-background":
		unexpected = startWorkerOwnerCallbackDeadlock(t, false, emit)
	case "external-shutdown-worker-shutdown-awake":
		unexpected = startLosingWorkerShutdownDeadlock(t, false, emit)
	case "external-shutdown-worker-shutdown-started":
		unexpected = startLosingWorkerShutdownDeadlock(t, true, emit)
	case "external-shutdown-worker-close-owner-callback":
		unexpected = startExternalWinnerCallbackWorkerDeadlock(t, terminalShutdown, terminalClose, emit)
	case "external-shutdown-worker-shutdown-owner-callback":
		unexpected = startExternalWinnerCallbackWorkerDeadlock(t, terminalShutdown, terminalShutdown, emit)
	case "external-close-gap-worker-close-owner-callback":
		unexpected = startExternalWinnerCallbackWorkerDeadlock(t, terminalClose, terminalClose, emit)
	case "owner-close-winning-started":
		unexpected = startOwnerWinningDeadlock(t, true, emit)
	case "owner-shutdown-winning-started-background":
		unexpected = startOwnerWinningDeadlock(t, false, emit)
	case "owner-close-losing-shutdown":
		unexpected = startOwnerLosingDeadlock(t, terminalShutdown, terminalClose, emit)
	case "owner-close-losing-close-gap":
		unexpected = startOwnerLosingDeadlock(t, terminalClose, terminalClose, emit)
	case "owner-shutdown-losing-shutdown-background":
		unexpected = startOwnerLosingDeadlock(t, terminalShutdown, terminalShutdown, emit)
	default:
		emit("invalid")
		t.Fatalf("unknown historical deadlock mode %q", mode)
	}
	phase := <-unexpected
	emit(phase)
	t.Fatalf("historical cycle %s unexpectedly completed at %s", mode, phase)
}

// These rows distinguish three owner-sensitive cycles. The external winner
// waits for Run exit; Run's owner callback waits for the counted worker; and
// that worker waits for loopDone or for the winner-held Shutdown once.
func startExternalWinnerCallbackWorkerDeadlock(
	t *testing.T,
	externalKind,
	workerKind terminalKind,
	emit func(string),
) <-chan string {
	t.Helper()
	value := newLoop(false)
	unexpected := make(chan string, 16)
	workerGate := make(chan struct{})
	workerResult := make(chan error, 1)
	workerStarted := make(chan struct{})
	losingEntered := make(chan struct{})
	if err := value.promisifyObserved(func() {
		<-workerGate
		emit("worker-terminal-entered")
		if workerKind == terminalClose {
			workerResult <- value.closeLoopObserved(lifecycleObserver{closeTerminatingWait: func() {
				emit("losing-close-entered")
				close(losingEntered)
			}})
		} else {
			workerResult <- value.shutdownObserved(lifecycleObserver{shutdownOnceEntering: func() {
				emit("losing-shutdown-once-entering")
				close(losingEntered)
			}})
		}
		unexpected <- "terminal-returned"
	}, workerObserver{
		workerCommitted: func() { emit("worker-committed") },
		workerStarted: func() {
			emit("worker-started")
			close(workerStarted)
		},
		workerReturned:      func() { unexpected <- "worker-returned" },
		settlementPublished: func(error) { unexpected <- "settlement-published" },
		beforeWorkerDone:    func() { unexpected <- "before-worker-done" },
	}); err != nil {
		unexpected <- "invalid"
		return unexpected
	}
	<-workerStarted
	callbackEntered := make(chan struct{})
	callbackGate := make(chan struct{})
	if err := value.submitToQueue(func() {
		emit("callback-entered")
		if value.isOwner() {
			emit("owner-identity-ok")
		} else {
			unexpected <- "invalid"
		}
		close(callbackEntered)
		<-callbackGate
		close(workerGate)
		emit("callback-waiting-worker")
		<-workerResult
		unexpected <- "callback-returned"
	}); err != nil {
		unexpected <- "invalid"
		return unexpected
	}
	emit("callback-admitted")
	go func() {
		value.run()
		unexpected <- "run-returned"
	}()
	<-callbackEntered
	observer := lifecycleObserver{
		shutdownTransitioned: func() {
			emit("shutdown-transitioned")
			close(callbackGate)
			<-losingEntered
		},
		closeTransitioned: func() {
			emit("close-transitioned")
			close(callbackGate)
			<-losingEntered
		},
		shutdownWake: func() {
			emit("terminal-wake")
			emitHistoricalLoopDoneOpen(value, emit, unexpected)
		},
		closeWake: func() {
			emit("terminal-wake")
			emitHistoricalLoopDoneOpen(value, emit, unexpected)
		},
		beforeShutdownCommit: func() { unexpected <- "before-shutdown-commit" },
		workerWaitStarted:    func() { unexpected <- "worker-wait-started" },
	}
	go func() {
		if externalKind == terminalClose {
			_ = value.closeLoopObserved(observer)
		} else {
			_ = value.shutdownObserved(observer)
		}
		unexpected <- "external-terminal-returned"
	}()
	return unexpected
}

// The Shutdown row models the non-canceling/background source branch. Both
// rows stop before the later self-WaitGroup join: the owner callback is already
// waiting for the counted worker, while the worker waits for owner Run exit.
func startWorkerOwnerCallbackDeadlock(t *testing.T, immediate bool, emit func(string)) <-chan string {
	t.Helper()
	value := newLoop(false)
	unexpected := make(chan string, 16)
	workerGate := make(chan struct{})
	workerResult := make(chan error, 1)
	workerStarted := make(chan struct{})
	observer := lifecycleObserver{
		shutdownOnceEntering: func() { emit("shutdown-once-entering") },
		shutdownTransitioned: func() { emit("shutdown-transitioned") },
		closeTransitioned:    func() { emit("close-transitioned") },
		shutdownWake: func() {
			emit("terminal-wake")
			emitHistoricalLoopDoneOpen(value, emit, unexpected)
		},
		closeWake: func() {
			emit("terminal-wake")
			emitHistoricalLoopDoneOpen(value, emit, unexpected)
		},
		beforeShutdownCommit: func() { unexpected <- "before-shutdown-commit" },
		workerWaitStarted:    func() { unexpected <- "worker-wait-started" },
	}
	if err := value.promisifyObserved(func() {
		<-workerGate
		emit("worker-terminal-entered")
		if immediate {
			workerResult <- value.closeLoopObserved(observer)
		} else {
			workerResult <- value.shutdownObserved(observer)
		}
		unexpected <- "terminal-returned"
	}, workerObserver{
		workerCommitted: func() { emit("worker-committed") },
		workerStarted: func() {
			emit("worker-started")
			close(workerStarted)
		},
		workerReturned:      func() { unexpected <- "worker-returned" },
		settlementPublished: func(error) { unexpected <- "settlement-published" },
		beforeWorkerDone:    func() { unexpected <- "before-worker-done" },
	}); err != nil {
		unexpected <- "invalid"
		return unexpected
	}
	<-workerStarted
	if err := value.submitToQueue(func() {
		emit("callback-entered")
		if value.isOwner() {
			emit("owner-identity-ok")
		} else {
			unexpected <- "invalid"
		}
		close(workerGate)
		emit("callback-waiting-worker")
		<-workerResult
		unexpected <- "callback-returned"
	}); err != nil {
		unexpected <- "invalid"
		return unexpected
	}
	emit("callback-admitted")
	go func() {
		value.run()
		unexpected <- "run-returned"
	}()
	return unexpected
}

func startWinningWorkerDeadlock(t *testing.T, immediate, started bool, emit func(string)) <-chan string {
	t.Helper()
	value := newLoop(false)
	unexpected := make(chan string, 16)
	var resumeRun chan struct{}
	if started {
		paused := make(chan struct{})
		resumeRun = make(chan struct{})
		go func() {
			value.runObserved(lifecycleObserver{runWait: func() {
				close(paused)
				<-resumeRun
			}})
			emit("run-returned")
		}()
		<-paused
		emit("run-paused")
	}
	observer := lifecycleObserver{
		shutdownTransitioned: func() { emit("shutdown-transitioned") },
		closeTransitioned:    func() { emit("close-transitioned") },
		shutdownWake: func() {
			emit("terminal-wake")
			close(resumeRun)
		},
		closeWake: func() {
			emit("terminal-wake")
			close(resumeRun)
		},
		beforeShutdownCommit: func() { emit("before-shutdown-commit") },
		workerWaitStarted: func() {
			if !started {
				select {
				case <-value.loopDone:
					if state(value.state.Load()) == stateTerminated && value.promisifyCount.Load() == 1 {
						emit("awake-barrier-state-ok")
					} else {
						unexpected <- "invalid"
					}
				default:
					unexpected <- "invalid"
				}
			}
			emit("worker-wait-started")
		},
		workerWaitFinished: func() { unexpected <- "worker-wait-finished" },
	}
	err := value.promisifyObserved(func() {
		if immediate {
			_ = value.closeLoopObserved(observer)
		} else {
			_ = value.shutdownObserved(observer)
		}
		unexpected <- "terminal-returned"
	}, workerObserver{
		workerCommitted:     func() { emit("worker-committed") },
		workerStarted:       func() { emit("worker-started") },
		workerReturned:      func() { unexpected <- "worker-returned" },
		settlementPublished: func(error) { unexpected <- "settlement-published" },
		beforeWorkerDone:    func() { unexpected <- "before-worker-done" },
	})
	if err != nil {
		unexpected <- "invalid"
	}
	return unexpected
}

func startLosingWorkerShutdownDeadlock(t *testing.T, started bool, emit func(string)) <-chan string {
	t.Helper()
	value := newLoop(false)
	unexpected := make(chan string, 16)
	var resumeRun chan struct{}
	if started {
		paused := make(chan struct{})
		resumeRun = make(chan struct{})
		go func() {
			value.runObserved(lifecycleObserver{runWait: func() {
				close(paused)
				<-resumeRun
			}})
			emit("run-returned")
		}()
		<-paused
		emit("run-paused")
	}
	workerStarted := make(chan struct{})
	workerGate := make(chan struct{})
	if err := value.promisifyObserved(func() {
		<-workerGate
		_ = value.shutdownObserved(lifecycleObserver{shutdownOnceEntering: func() {
			emit("losing-shutdown-once-entering")
		}})
		unexpected <- "terminal-returned"
	}, workerObserver{
		workerCommitted: func() { emit("worker-committed") },
		workerStarted: func() {
			emit("worker-started")
			close(workerStarted)
		},
		workerReturned:      func() { unexpected <- "worker-returned" },
		settlementPublished: func(error) { unexpected <- "settlement-published" },
		beforeWorkerDone:    func() { unexpected <- "before-worker-done" },
	}); err != nil {
		unexpected <- "invalid"
		return unexpected
	}
	<-workerStarted
	go func() {
		_ = value.shutdownObserved(lifecycleObserver{
			shutdownTransitioned: func() { emit("shutdown-transitioned") },
			shutdownWake: func() {
				emit("terminal-wake")
				close(resumeRun)
			},
			beforeShutdownCommit: func() { emit("before-shutdown-commit") },
			workerWaitStarted: func() {
				if !started {
					select {
					case <-value.loopDone:
						if state(value.state.Load()) == stateTerminated && value.promisifyCount.Load() == 1 {
							emit("awake-barrier-state-ok")
						} else {
							unexpected <- "invalid"
						}
					default:
						unexpected <- "invalid"
					}
				}
				emit("worker-wait-started")
				close(workerGate)
			},
			workerWaitFinished: func() { unexpected <- "worker-wait-finished" },
		})
		unexpected <- "external-terminal-returned"
	}()
	return unexpected
}

// The Shutdown path models the exact non-canceling/background source branch;
// this reduction deliberately has no context or cancellation input.
func startOwnerWinningDeadlock(t *testing.T, immediate bool, emit func(string)) <-chan string {
	t.Helper()
	value := newLoop(false)
	unexpected := make(chan string, 16)
	paused := make(chan struct{})
	resumeRun := make(chan struct{})
	go func() {
		value.runObserved(lifecycleObserver{runWait: func() {
			close(paused)
			<-resumeRun
		}})
		unexpected <- "run-returned"
	}()
	<-paused
	emit("run-paused")
	if err := value.submitToQueue(func() {
		emit("callback-entered")
		if value.isOwner() {
			emit("owner-identity-ok")
		} else {
			unexpected <- "invalid"
		}
		observer := lifecycleObserver{
			shutdownOnceEntering: func() { emit("shutdown-once-entering") },
			shutdownTransitioned: func() { emit("shutdown-transitioned") },
			closeTransitioned:    func() { emit("close-transitioned") },
			shutdownWake: func() {
				emit("terminal-wake")
				emitHistoricalLoopDoneOpen(value, emit, unexpected)
			},
			closeWake: func() {
				emit("terminal-wake")
				emitHistoricalLoopDoneOpen(value, emit, unexpected)
			},
			beforeShutdownCommit: func() { unexpected <- "before-shutdown-commit" },
			workerWaitStarted:    func() { unexpected <- "worker-wait-started" },
		}
		if immediate {
			_ = value.closeLoopObserved(observer)
		} else {
			_ = value.shutdownObserved(observer)
		}
		unexpected <- "terminal-returned"
	}); err != nil {
		unexpected <- "invalid"
		return unexpected
	}
	emit("callback-admitted")
	close(resumeRun)
	return unexpected
}

// Shutdown-versus-Shutdown likewise models a non-canceling/background winner.
func startOwnerLosingDeadlock(t *testing.T, externalKind, ownerKind terminalKind, emit func(string)) <-chan string {
	t.Helper()
	value := newLoop(false)
	unexpected := make(chan string, 16)
	callbackEntered := make(chan struct{})
	callbackGate := make(chan struct{})
	closeWait := make(chan struct{})
	shutdownOnceWait := make(chan struct{})
	if err := value.submitToQueue(func() {
		emit("callback-entered")
		if value.isOwner() {
			emit("owner-identity-ok")
		} else {
			unexpected <- "invalid"
		}
		close(callbackEntered)
		<-callbackGate
		if ownerKind == terminalClose {
			_ = value.closeLoopObserved(lifecycleObserver{closeTerminatingWait: func() {
				emit("close-terminating-wait")
				close(closeWait)
			}})
		} else {
			_ = value.shutdownObserved(lifecycleObserver{shutdownOnceEntering: func() {
				emit("shutdown-once-entering")
				close(shutdownOnceWait)
			}})
		}
		unexpected <- "terminal-returned"
	}); err != nil {
		unexpected <- "invalid"
		return unexpected
	}
	emit("callback-admitted")
	go func() {
		value.run()
		unexpected <- "run-returned"
	}()
	<-callbackEntered
	observer := lifecycleObserver{
		shutdownTransitioned: func() {
			emit("shutdown-transitioned")
			close(callbackGate)
			if ownerKind == terminalClose {
				<-closeWait
			} else {
				<-shutdownOnceWait
			}
		},
		closeTransitioned: func() {
			emit("close-transitioned")
			close(callbackGate)
			<-closeWait
		},
		shutdownWake: func() {
			emit("terminal-wake")
			emitHistoricalLoopDoneOpen(value, emit, unexpected)
		},
		closeWake: func() {
			emit("terminal-wake")
			emitHistoricalLoopDoneOpen(value, emit, unexpected)
		},
		beforeShutdownCommit: func() { unexpected <- "before-shutdown-commit" },
		workerWaitStarted:    func() { unexpected <- "worker-wait-started" },
	}
	go func() {
		if externalKind == terminalClose {
			_ = value.closeLoopObserved(observer)
		} else {
			_ = value.shutdownObserved(observer)
		}
		unexpected <- "external-terminal-returned"
	}()
	return unexpected
}

func emitHistoricalLoopDoneOpen(value *loop, emit func(string), unexpected chan<- string) {
	select {
	case <-value.loopDone:
		unexpected <- "invalid"
	default:
		emit("loop-done-open")
	}
}
