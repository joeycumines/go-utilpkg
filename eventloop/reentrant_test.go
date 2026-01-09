package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestReentrancy_StartFromCallback verifies that calling Start() from within
// a callback returns ErrReentrantStart, preventing recursive loop starts.
func TestReentrancy_StartFromCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := loop.Start(ctx); err != nil {
		t.Fatal(err)
	}

	var reentrantErr atomic.Value
	done := make(chan struct{})

	loop.Submit(Task{Runnable: func() {
		err := loop.Start(ctx)
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
		t.Fatal("REENTRANCY CHECK BROKEN: Start() from callback should return error")
	}
	if err != ErrReentrantStart {
		t.Fatalf("Expected ErrReentrantStart, got: %v", err)
	}
}

// TestLoop_StartRace verifies that only one concurrent Start() call succeeds.
// All others should receive ErrLoopAlreadyRunning.
func TestLoop_StartRace(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}

	var successes atomic.Int32
	var failures atomic.Int32
	var wg sync.WaitGroup

	count := 100
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			err := l.Start(context.Background())
			if err == nil {
				successes.Add(1)
			} else if err == ErrLoopAlreadyRunning {
				failures.Add(1)
			} else {
				t.Errorf("Unexpected error: %v", err)
			}
		}()
	}

	wg.Wait()
	l.Stop(context.Background())

	if successes.Load() != 1 {
		t.Errorf("Expected 1 successful Start, got %d", successes.Load())
	}
	if failures.Load() != 99 {
		t.Errorf("Expected 99 failures, got %d", failures.Load())
	}
}
