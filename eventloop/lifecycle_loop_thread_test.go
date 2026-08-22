package eventloop

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestShutdownFromLoopCallbacksDoesNotDeadlock(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Loop, chan<- error) error
	}{
		{
			name: "timer",
			setup: func(loop *Loop, shutdownErr chan<- error) error {
				_, err := loop.ScheduleTimer(0, func() {
					shutdownErr <- loop.Shutdown(context.Background())
				})
				return err
			},
		},
		{
			name: "submit",
			setup: func(loop *Loop, shutdownErr chan<- error) error {
				return loop.Submit(func() {
					shutdownErr <- loop.Shutdown(context.Background())
				})
			},
		},
		{
			name: "microtask",
			setup: func(loop *Loop, shutdownErr chan<- error) error {
				return loop.Submit(func() {
					if err := loop.ScheduleMicrotask(func() {
						shutdownErr <- loop.Shutdown(context.Background())
					}); err != nil {
						shutdownErr <- err
					}
				})
			},
		},
		{
			name: "nextTick",
			setup: func(loop *Loop, shutdownErr chan<- error) error {
				return loop.Submit(func() {
					if err := loop.ScheduleNextTick(func() {
						shutdownErr <- loop.Shutdown(context.Background())
					}); err != nil {
						shutdownErr <- err
					}
				})
			},
		},
		{
			name: "setImmediate",
			setup: func(loop *Loop, shutdownErr chan<- error) error {
				js, err := NewJS(loop)
				if err != nil {
					t.Fatal(err)
				}
				_, err = js.SetImmediate(func() {
					shutdownErr <- loop.Shutdown(context.Background())
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}

			shutdownErr := make(chan error, 1)
			if err := tt.setup(loop, shutdownErr); err != nil {
				t.Fatalf("setup: %v", err)
			}

			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()

			select {
			case err := <-shutdownErr:
				if err != nil {
					t.Fatalf("Shutdown from loop callback returned %v, want nil", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Shutdown from loop callback did not return")
			}

			select {
			case err := <-runDone:
				if err != nil {
					t.Fatalf("Run returned %v, want nil", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Run did not exit after callback-local Shutdown")
			}
		})
	}
}

func TestCloseFromLoopCallbackRejectedWithoutDeadlock(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	closeErr := make(chan error, 1)
	shutdownErr := make(chan error, 1)
	if err := loop.Submit(func() {
		closeErr <- loop.Close()
		shutdownErr <- loop.Shutdown(context.Background())
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	select {
	case err := <-closeErr:
		if !errors.Is(err, ErrReentrantClose) {
			t.Fatalf("Close from loop callback err = %v, want ErrReentrantClose", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close from loop callback did not return")
	}

	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Fatalf("Shutdown after rejected Close returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown after rejected Close did not return")
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit after callback-local Shutdown")
	}
}

func TestShutdownFromLoopCallbackRunsTerminationCleanup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	timerErr := make(chan error, 1)
	shutdownErr := make(chan error, 1)
	if err := loop.Submit(func() {
		_, err := loop.ScheduleTimer(time.Hour, func() {})
		timerErr <- err
		shutdownErr <- loop.Shutdown(context.Background())
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	select {
	case err := <-timerErr:
		if err != nil {
			t.Fatalf("ScheduleTimer from loop callback: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timer scheduling did not run")
	}
	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Fatalf("Shutdown from loop callback returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown from loop callback did not return")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit after callback-local Shutdown")
	}

	if loop.Alive() {
		t.Fatal("loop is still alive after callback-local Shutdown completed")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount after callback-local Shutdown = %d, want 0", got)
	}
}

func TestTimedOutShutdownStillRunsLoopOwnedCleanup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	timerErr := make(chan error, 1)
	promiseMade := make(chan *promise, 1)
	if err := loop.Submit(func() {
		_, err := loop.ScheduleTimer(time.Hour, func() {})
		timerErr <- err
		p := loop.registry.NewPromise()
		promiseMade <- p
		close(callbackStarted)
		<-releaseCallback
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	select {
	case <-callbackStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("blocking callback did not start")
	}
	select {
	case err := <-timerErr:
		if err != nil {
			close(releaseCallback)
			t.Fatalf("ScheduleTimer from blocking callback: %v", err)
		}
	case <-time.After(time.Second):
		close(releaseCallback)
		t.Fatal("timer scheduling did not complete")
	}

	var pending *promise
	select {
	case pending = <-promiseMade:
	case <-time.After(time.Second):
		close(releaseCallback)
		t.Fatal("registry promise was not created")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	shutdownErr := loop.Shutdown(shutdownCtx)
	cancel()
	if !errors.Is(shutdownErr, context.DeadlineExceeded) {
		close(releaseCallback)
		t.Fatalf("Shutdown err = %v, want context deadline exceeded", shutdownErr)
	}

	close(releaseCallback)

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit after timed-out Shutdown released terminal drain")
	}

	if loop.Alive() {
		t.Fatal("loop is still alive after timed-out Shutdown's loop-owned cleanup")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount after timed-out Shutdown = %d, want 0", got)
	}
	waitContractSignal(t, loop.terminalDone, "timed-out Shutdown terminal cleanup")
	if pending.State() != Rejected {
		t.Fatalf("registry promise state after timed-out Shutdown = %v, want Rejected", pending.State())
	}
	if !errors.Is(pending.Result().(error), ErrLoopTerminated) {
		t.Fatalf("registry promise reason = %v, want ErrLoopTerminated", pending.Result())
	}
}

func TestShutdownFromLoopCallbackThenContextCancelRunsCleanup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	timerErr := make(chan error, 1)
	shutdownErr := make(chan error, 1)
	promiseMade := make(chan *promise, 1)
	if err := loop.Submit(func() {
		_, err := loop.ScheduleTimer(time.Hour, func() {})
		timerErr <- err
		p := loop.registry.NewPromise()
		promiseMade <- p
		shutdownErr <- loop.Shutdown(context.Background())
		cancelRun()
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(runCtx) }()

	select {
	case err := <-timerErr:
		if err != nil {
			t.Fatalf("ScheduleTimer from loop callback: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timer scheduling did not run")
	}
	var pending *promise
	select {
	case pending = <-promiseMade:
	case <-time.After(time.Second):
		t.Fatal("registry promise was not created")
	}
	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Fatalf("Shutdown from loop callback returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown from loop callback did not return")
	}
	select {
	case err := <-runDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit after callback-local Shutdown and context cancellation")
	}

	requireTerminatedCleanup(t, loop, pending)
}

func TestTimedOutShutdownThenContextCancelRunsCleanup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	timerErr := make(chan error, 1)
	promiseMade := make(chan *promise, 1)
	if err := loop.Submit(func() {
		_, err := loop.ScheduleTimer(time.Hour, func() {})
		timerErr <- err
		p := loop.registry.NewPromise()
		promiseMade <- p
		close(callbackStarted)
		<-releaseCallback
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(runCtx) }()

	select {
	case <-callbackStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("blocking callback did not start")
	}
	select {
	case err := <-timerErr:
		if err != nil {
			close(releaseCallback)
			t.Fatalf("ScheduleTimer from blocking callback: %v", err)
		}
	case <-time.After(time.Second):
		close(releaseCallback)
		t.Fatal("timer scheduling did not complete")
	}
	var pending *promise
	select {
	case pending = <-promiseMade:
	case <-time.After(time.Second):
		close(releaseCallback)
		t.Fatal("registry promise was not created")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 20*time.Millisecond)
	shutdownErr := loop.Shutdown(shutdownCtx)
	cancelShutdown()
	if !errors.Is(shutdownErr, context.DeadlineExceeded) {
		close(releaseCallback)
		t.Fatalf("Shutdown err = %v, want context deadline exceeded", shutdownErr)
	}
	cancelRun()
	close(releaseCallback)

	select {
	case err := <-runDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit after timed-out Shutdown and context cancellation")
	}

	requireTerminatedCleanup(t, loop, pending)
}

func requireTerminatedCleanup(t *testing.T, loop *Loop, pending *promise) {
	t.Helper()
	waitContractSignal(t, loop.terminalDone, "loop-owned terminal cleanup")
	if loop.Alive() {
		t.Fatal("loop is still alive after loop-owned cleanup")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount after loop-owned cleanup = %d, want 0", got)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after loop-owned cleanup = %d, want 0", got)
	}
	if pending.State() != Rejected {
		t.Fatalf("registry promise state after loop-owned cleanup = %v, want Rejected", pending.State())
	}
	if !errors.Is(pending.Result().(error), ErrLoopTerminated) {
		t.Fatalf("registry promise reason = %v, want ErrLoopTerminated", pending.Result())
	}
}
