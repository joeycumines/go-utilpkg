package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

type asyncLoopRun struct {
	done chan struct{}
	mu   sync.Mutex
	err  error
}

func startAsyncLoopRun(loop *Loop, ctx context.Context) *asyncLoopRun {
	run := &asyncLoopRun{done: make(chan struct{})}
	go func() {
		err := loop.Run(ctx)
		run.mu.Lock()
		run.err = err
		run.mu.Unlock()
		close(run.done)
	}()
	return run
}

func (run *asyncLoopRun) Err() error {
	run.mu.Lock()
	defer run.mu.Unlock()
	return run.err
}

// TestFastPathAutoExit_StaleQuiescingClearedBeforeEphemeralSubmitRuns verifies
// that a fast-path auto-exit decision does not leave the quiescing gate stale
// when ephemeral work arrives after runFastPath's final Alive() re-check but
// before run() commits termination.
func TestFastPathAutoExit_StaleQuiescingClearedBeforeEphemeralSubmitRuns(t *testing.T) {
	loop := New(WithAutoExit(true), WithFastPathMode(FastPathForced))

	hookEntered := make(chan struct{})
	submitErrCh := make(chan error, 1)
	workRan := make(chan struct{})
	var hookOnce sync.Once
	var timerID TimerID
	var timerErr error
	var quiescingDuringTask bool

	loop.testHooks = &loopTestHooks{
		BeforeAutoExitCommit: func() {
			hookOnce.Do(func() {
				close(hookEntered)
				submitErrCh <- loop.Submit(func() {
					quiescingDuringTask = loop.quiescing.Load()
					timerID, timerErr = loop.ScheduleTimer(time.Hour, func() {})
					close(workRan)
				})
			})
		},
	}

	// Seed one ephemeral task before Run starts. Without this, run() observes
	// !Alive() at the top-level auto-exit check and terminates before entering
	// runFastPath(), so the fast-path hook never fires.
	if err := loop.Submit(func() {}); err != nil {
		t.Fatalf("seed Submit: %v", err)
	}

	run := startAsyncLoopRun(loop, context.Background())

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := loop.Shutdown(shutdownCtx); err != nil && err != ErrLoopTerminated {
			t.Errorf("Shutdown cleanup: %v", err)
		}
		select {
		case <-run.done:
			err := run.Err()
			if err != nil && err != ErrLoopTerminated {
				t.Errorf("Run returned during cleanup: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Run did not exit during cleanup")
		}
	}()

	select {
	case <-hookEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("fast-path auto-exit hook did not fire")
	}

	select {
	case err := <-submitErrCh:
		if err != nil {
			t.Fatalf("Submit during fast-path quiescing: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hook did not finish Submit")
	}

	select {
	case <-workRan:
	case <-time.After(5 * time.Second):
		select {
		case <-run.done:
			t.Fatalf("loop exited before submitted work ran: %v", run.Err())
		default:
			t.Fatal("submitted work did not run")
		}
	}

	if quiescingDuringTask {
		t.Fatal("quiescing stayed stale while accepted ephemeral task ran")
	}
	if timerErr != nil {
		t.Fatalf("ScheduleTimer from accepted ephemeral task: %v", timerErr)
	}
	if timerID == 0 {
		t.Fatal("ScheduleTimer returned timer ID 0 without error")
	}
}

func TestFastPathAutoExit_EphemeralSubmitBeforeQuiescingClearRuns(t *testing.T) {
	loop := New(WithAutoExit(true), WithFastPathMode(FastPathForced))

	hookEntered := make(chan struct{})
	submitErrCh := make(chan error, 1)
	workRan := make(chan struct{})
	var hookOnce sync.Once

	loop.testHooks = &loopTestHooks{
		BeforeAutoExitCommit: func() {
			hookOnce.Do(func() {
				close(hookEntered)
				submitErrCh <- loop.Submit(func() { close(workRan) })
			})
		},
	}

	if err := loop.Submit(func() {}); err != nil {
		t.Fatalf("seed Submit: %v", err)
	}

	run := startAsyncLoopRun(loop, context.Background())

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := loop.Shutdown(shutdownCtx); err != nil && err != ErrLoopTerminated {
			t.Errorf("Shutdown cleanup: %v", err)
		}
		select {
		case <-run.done:
			err := run.Err()
			if err != nil && err != ErrLoopTerminated {
				t.Errorf("Run returned during cleanup: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Run did not exit during cleanup")
		}
	}()

	select {
	case <-hookEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("fast-path auto-exit hook did not fire")
	}
	select {
	case err := <-submitErrCh:
		if err != nil {
			t.Fatalf("Submit during stale fast-path quiescing: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hook did not finish Submit")
	}

	select {
	case <-workRan:
	case <-time.After(5 * time.Second):
		select {
		case <-run.done:
			t.Fatalf("loop exited before accepted Submit ran: %v", run.Err())
		default:
			t.Fatal("accepted Submit did not run")
		}
	}
}
