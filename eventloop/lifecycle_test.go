package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestStart_ConcurrentCallsOnlyOneSucceeds verifies that concurrent Start() calls
// result in exactly one success, preventing the race condition where multiple
// goroutines execute run() concurrently.
func TestStart_ConcurrentCallsOnlyOneSucceeds(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 100
	var (
		successCount atomic.Int32
		errorCount   atomic.Int32
		wg           sync.WaitGroup
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ready := make(chan struct{})

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			<-ready
			err := loop.Run(ctx)
			if err == nil {
				successCount.Add(1)
			} else {
				errorCount.Add(1)
			}
		}()
	}

	close(ready)

	// Shutdown will unblock the successful Run() call
	shutdownDone := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(shutdownDone)
	}()

	wg.Wait()
	<-shutdownDone

	if successCount.Load() != 1 {
		t.Fatalf("RACE DETECTED: %d successful Start() calls (expected 1)", successCount.Load())
	}
	if errorCount.Load() != goroutines-1 {
		t.Fatalf("Expected %d errors, got %d", goroutines-1, errorCount.Load())
	}
}

// TestStart_SecondCallReturnsError verifies that calling Run() twice returns
// ErrLoopAlreadyRunning on the second call.
func TestStart_SecondCallReturnsError(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	runDone := make(chan struct{})
	go func() {
		if err := loop.Run(ctx); err != nil {
			t.Errorf("Run() unexpected error: %v", err)
		}
		close(runDone)
	}()

	time.Sleep(10 * time.Millisecond)

	err2 := loop.Run(ctx)
	if err2 == nil {
		t.Fatal("Second Run() should return error, got nil")
	}
	if err2 != ErrLoopAlreadyRunning {
		t.Fatalf("Expected ErrLoopAlreadyRunning, got: %v", err2)
	}

	loop.Shutdown(context.Background())
	<-runDone
}

// TestLoop_Stop_Race_Torture is a stress test that verifies the loop can be
// stopped without deadlock. It catches the "Zombie Loop" bug where Shutdown()
// hangs forever due to state overwrite in poll().
func TestLoop_Stop_Race_Torture(t *testing.T) {
	for i := 0; i < 100; i++ {
		l, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		go func() {
			if err := l.Run(context.Background()); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrLoopTerminated) {
			}
		}()

		time.Sleep(1 * time.Millisecond)

		doneCh := make(chan error)
		go func() {
			doneCh <- l.Shutdown(context.Background())
		}()

		select {
		case err := <-doneCh:
			if err != nil {
				t.Errorf("Iteration %d: Shutdown returned error: %v", i, err)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Iteration %d: DEADLOCK! Loop failed to shut down. The Zombie State bug is present.", i)
		}
	}
}
