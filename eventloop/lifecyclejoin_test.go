package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLifecycleExternalCloseJoinsGracefulCompletion(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	releaseWorker := make(chan struct{})
	releaseWorkerFn := releaseSignalT(t, releaseWorker)
	workerStarted := make(chan struct{})
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-releaseWorker
		return "released", nil
	})
	waitContractSignal(t, workerStarted, "Promisify worker entry")

	terminalPublished := make(chan struct{})
	releaseTransition := make(chan struct{})
	releaseTransitionFn := releaseSignalT(t, releaseTransition)
	terminalJoined := make(chan struct{})
	var joinedOnce sync.Once
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			close(terminalPublished)
			<-releaseTransition
		},
		BeforeTerminalJoin: func() {
			joinedOnce.Do(func() { close(terminalJoined) })
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
	waitContractSignal(t, terminalPublished, "graceful terminal publication")

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, terminalJoined, "external Close terminal join")
	select {
	case err := <-closeDone:
		t.Fatalf("non-owning Close returned before graceful completion: %v", err)
	default:
	}

	releaseTransitionFn()
	select {
	case err := <-closeDone:
		t.Fatalf("non-owning Close returned before admitted worker completion: %v", err)
	default:
	}
	releaseWorkerFn()

	if err := waitContractValue(t, shutdownDone, "winning Shutdown completion"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, closeDone, "joined Close completion"); err != nil {
		t.Fatalf("joined Close: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result := waitContractValue(t, promise.ToChannel(), "Promisify settlement"); result != "released" {
		t.Fatalf("Promisify result = %v, want released", result)
	}
}

func TestLifecycleExternalShutdownJoinsImmediateCompletion(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	terminalPublished := make(chan struct{})
	releaseTransition := make(chan struct{})
	releaseTransitionFn := releaseSignalT(t, releaseTransition)
	terminalJoined := make(chan struct{})
	var joinedOnce sync.Once
	loop.testHooks = &loopTestHooks{
		AfterCloseStateTerminating: func() {
			close(terminalPublished)
			<-releaseTransition
		},
		BeforeTerminalJoin: func() {
			joinedOnce.Do(func() { close(terminalJoined) })
		},
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, terminalPublished, "immediate terminal publication")

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
	waitContractSignal(t, terminalJoined, "external Shutdown terminal join")
	select {
	case err := <-shutdownDone:
		t.Fatalf("non-owning Shutdown returned before immediate completion: %v", err)
	default:
	}

	releaseTransitionFn()
	if err := waitContractValue(t, closeDone, "winning Close completion"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := waitContractValue(t, shutdownDone, "joined Shutdown completion"); err != nil {
		t.Fatalf("joined Shutdown: %v", err)
	}
	assertCloseSignals(t, loop)

	if err := loop.Close(); !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("repeated Close = %v, want ErrLoopTerminated", err)
	}
	if err := loop.Shutdown(context.Background()); !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("repeated Shutdown = %v, want ErrLoopTerminated", err)
	}
}

func TestLifecycleStateTerminatingModePublicationBarrier(t *testing.T) {
	tests := []struct {
		name           string
		winner         func(*Loop) error
		external       func(*Loop) error
		worker         func(*Loop) error
		request        func(LoopRequests) error
		wantRequestErr error
		immediate      bool
	}{
		{
			name:           "graceful",
			winner:         func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			external:       func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			worker:         func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			request:        func(requests LoopRequests) error { return requests.Close() },
			wantRequestErr: ErrLoopTerminated,
		},
		{
			name:           "immediate",
			winner:         func(loop *Loop) error { return loop.Close() },
			external:       func(loop *Loop) error { return loop.Close() },
			worker:         func(loop *Loop) error { return loop.Close() },
			request:        func(requests LoopRequests) error { return requests.Shutdown() },
			wantRequestErr: ErrLoopTerminated,
			immediate:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			sentinel := errors.New("injected terminal failure")
			loop.storeTerminalError(sentinel)

			callWorker := make(chan struct{})
			callWorkerFn := releaseSignalT(t, callWorker)
			workerStarted := make(chan struct{})
			workerDone := make(chan error, 1)
			promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
				close(workerStarted)
				<-callWorker
				workerDone <- test.worker(loop)
				return "worker-returned", nil
			})
			waitContractSignal(t, workerStarted, "Promisify worker entry")

			stateCommitted := make(chan struct{})
			releaseMode := make(chan struct{})
			releaseModeFn := releaseSignalT(t, releaseMode)
			terminalJoined := make(chan struct{})
			modeReaders := make(chan struct{})
			var joinedOnce sync.Once
			var modeReaderCount atomic.Int32
			loop.testHooks = &loopTestHooks{
				TerminalStateCAS: func() {
					close(stateCommitted)
					<-releaseMode
				},
				BeforeTerminalJoin: func() {
					joinedOnce.Do(func() { close(terminalJoined) })
				},
				BeforeTerminalModeLock: func() {
					if modeReaderCount.Add(1) == 2 {
						close(modeReaders)
					}
				},
			}

			winnerDone := make(chan error, 1)
			go func() { winnerDone <- test.winner(loop) }()
			waitContractSignal(t, stateCommitted, "StateTerminating CAS")
			if state := loop.State(); state != StateTerminating {
				t.Fatalf("state = %v, want StateTerminating", state)
			}
			if err := loop.Run(context.Background()); !errors.Is(err, ErrLoopTerminated) {
				t.Fatalf("Run during pre-Run terminal transition = %v, want ErrLoopTerminated", err)
			}

			externalDone := make(chan error, 1)
			go func() { externalDone <- test.external(loop) }()
			waitContractSignal(t, terminalJoined, "ordinary external terminal join")
			select {
			case err := <-externalDone:
				t.Fatalf("external call returned before terminal mode publication: %v", err)
			default:
			}

			callWorkerFn()
			requestDone := make(chan error, 1)
			go func() {
				requestDone <- test.request(loop.Requests())
			}()
			waitContractSignal(t, modeReaders, "blocked terminal mode readers")
			select {
			case err := <-workerDone:
				t.Fatalf("Promisify worker bypassed terminal mode publication: %v", err)
			default:
			}
			select {
			case err := <-requestDone:
				t.Fatalf("LoopRequests bypassed terminal mode publication: %v", err)
			default:
			}

			releaseModeFn()
			if err := waitContractValue(t, workerDone, "Promisify mode-sensitive result"); err != nil {
				t.Fatalf("Promisify lifecycle call = %v, want nil", err)
			}
			if err := waitContractValue(t, requestDone, "LoopRequests mode-sensitive result"); !errors.Is(err, test.wantRequestErr) {
				t.Fatalf("LoopRequests lifecycle call = %v, want %v", err, test.wantRequestErr)
			}
			if err := waitContractValue(t, winnerDone, "winning terminal result"); !errors.Is(err, sentinel) {
				t.Fatalf("winning terminal operation = %v, want injected failure", err)
			}
			if err := waitContractValue(t, externalDone, "joined external terminal result"); !errors.Is(err, sentinel) {
				t.Fatalf("joined external operation = %v, want injected failure", err)
			}
			if test.immediate {
				assertPromiseRejected(t, promise, ErrLoopTerminated)
			} else if result := waitContractValue(t, promise.ToChannel(), "graceful Promisify settlement"); result != "worker-returned" {
				t.Fatalf("Promisify result = %v, want worker-returned", result)
			}
		})
	}
}

func TestLifecycleJoinedShutdownRetainsContextBound(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	terminalPublished := make(chan struct{})
	releaseTransition := make(chan struct{})
	releaseTransitionFn := releaseSignalT(t, releaseTransition)
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			close(terminalPublished)
			<-releaseTransition
		},
	}

	winnerDone := make(chan error, 1)
	go func() { winnerDone <- loop.Shutdown(context.Background()) }()
	waitContractSignal(t, terminalPublished, "graceful terminal publication")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := loop.Shutdown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("joined Shutdown = %v, want context.Canceled", err)
	}

	releaseTransitionFn()
	if err := waitContractValue(t, winnerDone, "winning Shutdown completion"); err != nil {
		t.Fatalf("winning Shutdown: %v", err)
	}
}

func TestLifecycleExternalCallJoinsTerminatedOpenBarrier(t *testing.T) {
	tests := []struct {
		name      string
		winner    func(*Loop) error
		external  func(*Loop) error
		immediate bool
	}{
		{
			name:     "graceful_close",
			winner:   func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			external: func(loop *Loop) error { return loop.Close() },
		},
		{
			name:     "graceful_shutdown",
			winner:   func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			external: func(loop *Loop) error { return loop.Shutdown(context.Background()) },
		},
		{
			name:      "immediate_close",
			winner:    func(loop *Loop) error { return loop.Close() },
			external:  func(loop *Loop) error { return loop.Close() },
			immediate: true,
		},
		{
			name:      "immediate_shutdown",
			winner:    func(loop *Loop) error { return loop.Close() },
			external:  func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			immediate: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			sentinel := errors.New("injected terminal failure")
			loop.storeTerminalError(sentinel)

			terminatedPublished := make(chan struct{})
			releaseCompletion := make(chan struct{})
			releaseCompletionFn := releaseSignalT(t, releaseCompletion)
			terminalJoined := make(chan struct{})
			var joinedOnce sync.Once
			hooks := &loopTestHooks{
				BeforeTerminalJoin: func() {
					joinedOnce.Do(func() { close(terminalJoined) })
				},
			}
			if test.immediate {
				hooks.BeforeClosePromiseRejection = func() {
					close(terminatedPublished)
					<-releaseCompletion
				}
			} else {
				hooks.AfterTerminateStateBeforeDrain = func() {
					close(terminatedPublished)
					<-releaseCompletion
				}
			}
			loop.testHooks = hooks

			winnerDone := make(chan error, 1)
			go func() { winnerDone <- test.winner(loop) }()
			waitContractSignal(t, terminatedPublished, "StateTerminated publication before cleanup")
			if state := loop.State(); state != StateTerminated {
				t.Fatalf("state = %v, want StateTerminated", state)
			}
			select {
			case <-loop.terminalDone:
				t.Fatal("terminalDone closed while completion was blocked")
			default:
			}

			externalDone := make(chan error, 1)
			go func() { externalDone <- test.external(loop) }()
			waitContractSignal(t, terminalJoined, "external join after StateTerminated publication")
			select {
			case err := <-externalDone:
				t.Fatalf("external call returned before terminalDone: %v", err)
			default:
			}

			releaseCompletionFn()
			if err := waitContractValue(t, winnerDone, "winning terminal completion"); !errors.Is(err, sentinel) {
				t.Fatalf("winning terminal operation = %v, want injected failure", err)
			}
			if err := waitContractValue(t, externalDone, "joined terminal completion"); !errors.Is(err, sentinel) {
				t.Fatalf("joined terminal operation = %v, want injected failure", err)
			}
		})
	}
}

func TestLifecycleExternalCallJoinsInternalTermination(t *testing.T) {
	pollFailure := errors.New("injected poll failure")
	origins := []struct {
		name         string
		newLoop      func() *Loop
		start        func(*testing.T, *Loop) <-chan error
		wantRun      error
		wantTerminal error
		skip         func() bool
	}{
		{
			name: "auto_exit",
			newLoop: func() *Loop {
				loop, err := New(WithAutoExit(true))
				if err != nil {
					panic(err)
				}
				return loop
			},
			start: func(_ *testing.T, loop *Loop) <-chan error {
				done := make(chan error, 1)
				go func() { done <- loop.Run(context.Background()) }()
				return done
			},
		},
		{
			name: "context_cancellation",
			newLoop: func() *Loop {
				loop, err := New()
				if err != nil {
					panic(err)
				}
				return loop
			},
			start: func(t *testing.T, loop *Loop) <-chan error {
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan error, 1)
				go func() { done <- loop.Run(ctx) }()
				waitLoopOwnerTurnT(t, loop)
				cancel()
				return done
			},
			wantRun: context.Canceled,
		},
		{
			name: "poll_failure",
			newLoop: func() *Loop {
				loop, err := New(WithFastPathMode(FastPathDisabled))
				if err != nil {
					panic(err)
				}
				return loop
			},
			start: func(_ *testing.T, loop *Loop) <-chan error {
				loop.testHooks.PollIO = func(int) (int, error) { return 0, pollFailure }
				done := make(chan error, 1)
				go func() { done <- loop.Run(context.Background()) }()
				return done
			},
			wantRun:      pollFailure,
			wantTerminal: pollFailure,
			skip:         func() bool { return !fdPollingSupported },
		},
	}
	externalCalls := []struct {
		name string
		call func(*Loop) error
	}{
		{name: "close", call: func(loop *Loop) error { return loop.Close() }},
		{name: "shutdown", call: func(loop *Loop) error { return loop.Shutdown(context.Background()) }},
	}

	for _, origin := range origins {
		for _, external := range externalCalls {
			t.Run(origin.name+"_"+external.name, func(t *testing.T) {
				if origin.skip != nil && origin.skip() {
					t.Skip("native poll failure origin is unavailable on this target")
				}
				loop := origin.newLoop()
				registerLoopCleanupT(t, loop)

				terminatedPublished := make(chan struct{})
				releaseCompletion := make(chan struct{})
				releaseCompletionFn := releaseSignalT(t, releaseCompletion)
				terminalJoined := make(chan struct{})
				var joinedOnce sync.Once
				loop.testHooks = &loopTestHooks{
					AfterTerminateStateBeforeDrain: func() {
						close(terminatedPublished)
						<-releaseCompletion
					},
					BeforeTerminalJoin: func() {
						joinedOnce.Do(func() { close(terminalJoined) })
					},
				}

				runDone := origin.start(t, loop)
				waitContractSignal(t, terminatedPublished, "internal StateTerminated publication")
				if state := loop.State(); state != StateTerminated {
					t.Fatalf("state = %v, want StateTerminated", state)
				}
				select {
				case <-loop.terminalDone:
					t.Fatal("terminalDone closed while internal termination was blocked")
				default:
				}

				externalDone := make(chan error, 1)
				go func() { externalDone <- external.call(loop) }()
				waitContractSignal(t, terminalJoined, "external internal-origin terminal join")
				select {
				case err := <-externalDone:
					t.Fatalf("external call returned before internal terminal completion: %v", err)
				default:
				}

				releaseCompletionFn()
				runErr := waitContractValue(t, runDone, "internal-origin Run completion")
				if origin.wantRun == nil {
					if runErr != nil {
						t.Fatalf("Run = %v, want nil", runErr)
					}
				} else if !errors.Is(runErr, origin.wantRun) {
					t.Fatalf("Run = %v, want %v", runErr, origin.wantRun)
				}
				externalErr := waitContractValue(t, externalDone, "joined internal terminal completion")
				if origin.wantTerminal == nil {
					if externalErr != nil {
						t.Fatalf("joined external operation = %v, want nil", externalErr)
					}
				} else if !errors.Is(externalErr, origin.wantTerminal) {
					t.Fatalf("joined external operation = %v, want %v", externalErr, origin.wantTerminal)
				}
			})
		}
	}
}
