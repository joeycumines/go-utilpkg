package eventloop

import (
	"context"
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
			err := loop.Start(ctx)
			if err == nil {
				successCount.Add(1)
			} else {
				errorCount.Add(1)
			}
		}()
	}

	close(ready)
	wg.Wait()

	if successCount.Load() != 1 {
		t.Fatalf("RACE DETECTED: %d successful Start() calls (expected 1)", successCount.Load())
	}
	if errorCount.Load() != goroutines-1 {
		t.Fatalf("Expected %d errors, got %d", goroutines-1, errorCount.Load())
	}
}

// TestStart_SecondCallReturnsError verifies that calling Start() twice returns
// ErrLoopAlreadyRunning on the second call.
func TestStart_SecondCallReturnsError(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	err1 := loop.Start(ctx)
	if err1 != nil {
		t.Fatalf("First Start() failed: %v", err1)
	}

	time.Sleep(10 * time.Millisecond)

	err2 := loop.Start(ctx)
	if err2 == nil {
		t.Fatal("Second Start() should return error, got nil")
	}
	if err2 != ErrLoopAlreadyRunning {
		t.Fatalf("Expected ErrLoopAlreadyRunning, got: %v", err2)
	}
}

// TestLoop_Stop_Race_Torture is a stress test that verifies the loop can be
// stopped without deadlock. It catches the "Zombie Loop" bug where Stop()
// hangs forever due to state overwrite in poll().
func TestLoop_Stop_Race_Torture(t *testing.T) {
	for i := 0; i < 100; i++ {
		l, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		startErr := l.Start(context.Background())
		if startErr != nil {
			t.Fatalf("Failed to start loop: %v", startErr)
		}

		time.Sleep(1 * time.Millisecond)

		doneCh := make(chan error)
		go func() {
			doneCh <- l.Stop(context.Background())
		}()

		select {
		case err := <-doneCh:
			if err != nil {
				t.Errorf("Iteration %d: Stop returned error: %v", i, err)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Iteration %d: DEADLOCK! Loop failed to shut down. The Zombie State bug is present.", i)
		}
	}
}
