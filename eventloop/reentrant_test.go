package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestReentrancy_RunFromCallback verifies that calling Run() from within
// a callback returns ErrReentrantRun, preventing recursive loop starts.
func TestReentrancy_RunFromCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run() is blocking, so run it in a goroutine
	runDone := make(chan struct{})
	go func() {
		if err := loop.Run(ctx); err != nil {
			t.Errorf("Run() unexpected error: %v", err)
		}
		close(runDone)
	}()

	var reentrantErr atomic.Value
	done := make(chan struct{})

	loop.Submit(Task{Runnable: func() {
		err := loop.Run(ctx)
		reentrantErr.Store(err)
		close(done)
	}})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Callback never executed")
	}

	err, ok := reentrantErr.Load().(error)
	if !ok || err == nil {
		t.Fatal("REENTRANCY CHECK BROKEN: Run() from callback should return error")
	}
	if err != ErrReentrantRun {
		t.Fatalf("Expected ErrReentrantRun, got: %v", err)
	}

	// Cleanup
	loop.Shutdown(context.Background())
	<-runDone
}

// TestLoop_RunRace verifies that only one concurrent Run() call succeeds.
// All others should receive ErrLoopAlreadyRunning.
func TestLoop_RunRace(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// Add a timeout context for all Run() calls
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel() // Ensure cleanup happens in all cases

	var successes atomic.Int32
	var failures atomic.Int32
	var wg sync.WaitGroup

	count := 100
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(id int) {
			defer wg.Done()
			err := l.Run(ctx) // Use timeout context instead of context.Background()
			if err == nil {
				successes.Add(1)
			} else if err == ErrLoopAlreadyRunning {
				failures.Add(1)
			} else {
				t.Errorf("[%d] Unexpected error: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	if successes.Load() != 1 {
		t.Errorf("Expected 1 successful Run, got %d", successes.Load())
	}
	if failures.Load() != 99 {
		t.Errorf("Expected 99 failures, got %d", failures.Load())
	}

	// Add cleanup to ensure the loop is shut down
	l.Shutdown(context.Background())
}
