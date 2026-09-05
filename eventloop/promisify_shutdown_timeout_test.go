package eventloop

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

var errShutdownCompletionWaitObserved = errors.New("shutdown completion wait observed")

type shutdownCompletionProbeContext struct {
	done     chan struct{}
	observed chan struct{}
	once     sync.Once
}

func newShutdownCompletionProbeContext() *shutdownCompletionProbeContext {
	return &shutdownCompletionProbeContext{
		done:     make(chan struct{}),
		observed: make(chan struct{}),
	}
}

func (*shutdownCompletionProbeContext) Deadline() (time.Time, bool) { return time.Time{}, false }

func (c *shutdownCompletionProbeContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.done
}

func (c *shutdownCompletionProbeContext) Err() error {
	select {
	case <-c.done:
		return errShutdownCompletionWaitObserved
	default:
		return nil
	}
}

func (*shutdownCompletionProbeContext) Value(any) any { return nil }

func (c *shutdownCompletionProbeContext) release() {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}

func TestPromisifyWorkerWinningShutdownDoesNotJoinSelf(t *testing.T) {
	for _, running := range []bool{false, true} {
		t.Run(map[bool]string{false: "pre-run", true: "running"}[running], func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)

			var runDone chan error
			if running {
				runDone = make(chan error, 1)
				go func() { runDone <- loop.Run(context.Background()) }()
				waitLoopOwnerTurnT(t, loop)
			}

			probe := newShutdownCompletionProbeContext()
			shutdownResult := make(chan error, 1)
			promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
				shutdownResult <- loop.Shutdown(probe)
				return "worker-returned", nil
			})

			select {
			case err := <-shutdownResult:
				if err != nil {
					t.Fatalf("worker-winning Shutdown = %v, want nil request acknowledgement", err)
				}
			case <-probe.observed:
				probe.release()
				err := <-shutdownResult
				t.Fatalf("worker-winning Shutdown joined terminal completion: %v", err)
			case <-time.After(5 * time.Second):
				probe.release()
				t.Fatal("worker-winning Shutdown neither returned nor observed its completion context")
			}

			select {
			case <-loop.terminalDone:
			case <-time.After(5 * time.Second):
				t.Fatal("terminal cleanup did not complete after the winning worker returned")
			}
			select {
			case result := <-promise.ToChannel():
				if result != "worker-returned" {
					t.Fatalf("Promisify result = %v, want worker-returned", result)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Promisify promise did not settle")
			}
			if running {
				select {
				case err := <-runDone:
					if err != nil {
						t.Fatalf("Run = %v, want nil", err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("Run did not return after worker-requested Shutdown")
				}
			}
		})
	}
}

// TestPromisify_SlowOperation_ShutdownWaits verifies that Shutdown waits for
// all Promisify goroutines to complete while they remain behind an explicit
// release barrier. This validates the fix for CRITICAL-5 without relying on the
// old 100ms timeout or sleep-based goroutine-lifetime assumptions.
func TestPromisify_SlowOperation_ShutdownWaits(t *testing.T) {
	const numGoroutines = 10

	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	release := make(chan struct{})
	releaseFn := releaseSignalT(t, release)

	transitioned := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() { close(transitioned) },
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)

	started := make(chan int, numGoroutines)
	completed := make(chan int, numGoroutines)
	promises := make([]Future, numGoroutines)
	for i := range numGoroutines {
		idx := i
		promises[i] = loop.Promisify(context.Background(), func(context.Context) (any, error) {
			started <- idx
			<-release
			completed <- idx
			return fmt.Sprintf("result-%d", idx), nil
		})
	}
	for range numGoroutines {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("Promisify worker did not start")
		}
	}

	// Shutdown should wait for ALL Promisify goroutines
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
	select {
	case <-transitioned:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not commit StateTerminating")
	}
	select {
	case err := <-shutdownDone:
		t.Fatalf("Shutdown returned before Promisify workers were released: %v", err)
	default:
	}

	releaseFn()
	for range numGoroutines {
		select {
		case <-completed:
		case <-time.After(5 * time.Second):
			t.Fatal("Promisify worker did not complete after release")
		}
	}
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not return after every Promisify worker completed")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown completed")
	}
	for i, promise := range promises {
		select {
		case result := <-promise.ToChannel():
			want := fmt.Sprintf("result-%d", i)
			if result != want {
				t.Fatalf("promise %d result = %v, want %q", i, result, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("promise %d did not settle", i)
		}
	}
}

// TestPromisify_MultipleShutdowns verifies that multiple concurrent Shutdown calls
// all work correctly and wait for Promisify goroutines.
func TestPromisify_MultipleShutdowns(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	release := make(chan struct{})
	releaseFn := releaseSignalT(t, release)
	started := make(chan struct{})
	completed := make(chan struct{})
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(started)
		<-release
		close(completed)
		return "done", nil
	})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not start")
	}

	const numShutdowns = 5
	var transitionedOnce sync.Once
	transitioned := make(chan struct{})
	terminalJoined := make(chan struct{}, numShutdowns-1)
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			transitionedOnce.Do(func() { close(transitioned) })
		},
		BeforeTerminalJoin: func() { terminalJoined <- struct{}{} },
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)

	startShutdowns := make(chan struct{})
	startShutdownsFn := releaseSignalT(t, startShutdowns)
	shutdownErrors := make(chan error, numShutdowns)
	for range numShutdowns {
		go func() {
			<-startShutdowns
			shutdownErrors <- loop.Shutdown(context.Background())
		}()
	}
	startShutdownsFn()

	select {
	case <-transitioned:
	case <-time.After(5 * time.Second):
		t.Fatal("no Shutdown caller committed StateTerminating")
	}
	for i := range numShutdowns - 1 {
		waitContractSignal(t, terminalJoined, fmt.Sprintf("concurrent Shutdown join %d", i))
	}
	select {
	case err := <-shutdownErrors:
		t.Fatalf("Shutdown returned before worker release: %v", err)
	default:
	}

	releaseFn()
	select {
	case <-completed:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not complete after release")
	}
	for i := range numShutdowns {
		if err := waitContractValue(t, shutdownErrors, fmt.Sprintf("concurrent Shutdown completion %d", i)); err != nil {
			t.Fatalf("Shutdown %d = %v, want nil", i, err)
		}
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after concurrent Shutdowns")
	}
	select {
	case result := <-promise.ToChannel():
		if result != "done" {
			t.Fatalf("Promisify result = %v, want done", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify promise did not settle")
	}
}

func TestGracefulShutdownPreservesPromisifyWorkerOutcome(t *testing.T) {
	returnedError := errors.New("Promisify worker error")
	panicValue := &struct{ label string }{label: "Promisify worker panic"}
	tests := []struct {
		name                 string
		cancelBeforeRelease  bool
		outcome              func(context.Context) (any, error)
		assertPromiseOutcome func(*testing.T, Future)
	}{
		{
			name:    "returned error",
			outcome: func(context.Context) (any, error) { return nil, returnedError },
			assertPromiseOutcome: func(t *testing.T, promise Future) {
				assertPromisifyExactRejection(t, promise, returnedError)
			},
		},
		{
			name:    "panic",
			outcome: func(context.Context) (any, error) { panic(panicValue) },
			assertPromiseOutcome: func(t *testing.T, promise Future) {
				t.Helper()
				if state := promise.State(); state != Rejected {
					t.Fatalf("Promisify state = %v, want Rejected", state)
				}
				panicError, ok := promise.Result().(PanicError)
				if !ok || panicError.Value != panicValue {
					t.Fatalf("Promisify result = %T %#v, want PanicError with value identity %p", promise.Result(), promise.Result(), panicValue)
				}
			},
		},
		{
			name: "runtime Goexit",
			outcome: func(context.Context) (any, error) {
				runtime.Goexit()
				return nil, errors.New("unreachable after runtime.Goexit")
			},
			assertPromiseOutcome: func(t *testing.T, promise Future) {
				assertPromisifyExactRejection(t, promise, ErrGoexit)
			},
		},
		{
			name:                "context cancellation",
			cancelBeforeRelease: true,
			outcome: func(ctx context.Context) (any, error) {
				return nil, ctx.Err()
			},
			assertPromiseOutcome: func(t *testing.T, promise Future) {
				assertPromisifyExactRejection(t, promise, context.Canceled)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)

			var workerContext context.Context = context.Background()
			cancelWorker := func() {}
			if test.cancelBeforeRelease {
				workerContext, cancelWorker = context.WithCancel(context.Background())
				t.Cleanup(cancelWorker)
			}
			workerStarted := make(chan struct{})
			releaseWorker := make(chan struct{})
			releaseWorkerFn := releaseSignalT(t, releaseWorker)
			promise := loop.Promisify(workerContext, func(ctx context.Context) (any, error) {
				close(workerStarted)
				<-releaseWorker
				return test.outcome(ctx)
			})
			waitContractSignal(t, workerStarted, "Promisify worker entry")

			transitioned := make(chan struct{})
			loop.testHooks = &loopTestHooks{
				AfterShutdownStateTerminating: func() { close(transitioned) },
			}
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			waitLoopOwnerTurnT(t, loop)
			shutdownDone := make(chan error, 1)
			go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
			waitContractSignal(t, transitioned, "graceful Shutdown transition")
			select {
			case err := <-shutdownDone:
				t.Fatalf("Shutdown returned before Promisify worker release: %v", err)
			default:
			}

			cancelWorker()
			releaseWorkerFn()
			if err := waitContractValue(t, shutdownDone, "graceful Shutdown completion"); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}
			if err := waitContractValue(t, runDone, "graceful Shutdown Run completion"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := loop.promisifyCount.Load(); got != 0 {
				t.Fatalf("promisifyCount = %d, want 0", got)
			}
			test.assertPromiseOutcome(t, promise)
		})
	}
}
