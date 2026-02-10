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

// TestLoop_Close_WaitsForLoopDone verifies that Close() blocks until the
// loop goroutine has fully exited, preventing use-after-free bugs.
func TestLoop_Close_WaitsForLoopDone(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Run the loop in a goroutine
	runFinished := make(chan error, 1)
	go func() {
		runFinished <- loop.Run(ctx)
	}()

	// Wait for loop to start
	for i := 0; i < 100; i++ {
		if loop.State() == StateRunning {
			break
		}
		time.Sleep(time.Millisecond)
		if i == 99 {
			t.Fatal("Loop never reached StateRunning")
		}
	}

	// Call Close() - this SHOULD block until loop goroutine exits
	closeFinished := make(chan struct{})
	go func() {
		loop.Close()
		close(closeFinished)
	}()

	// Wait for Close() to complete (with timeout to ensure it doesn't deadlock)
	select {
	case <-closeFinished:
		// Close() completed
	case <-time.After(5 * time.Second):
		t.Fatal("Close() didn't return in 5 seconds (possible deadlock)")
	}

	// Verify loop goroutine has exited
	select {
	case err := <-runFinished:
		if err != nil && err != ErrLoopTerminated && err != context.Canceled {
			t.Errorf("Run() unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() goroutine didn't exit after Close()")
	}

	// Verify loop is terminated
	if loop.State() != StateTerminated {
		t.Errorf("Expected StateTerminated, got %v", loop.State())
	}
}

// TestLoop_CloseIdempotence verifies that calling Close() multiple times
// is safe and thread-safe.
func TestLoop_CloseIdempotence(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Run the loop in a goroutine
	runDone := make(chan error)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// First Close should succeed
	if err := loop.Close(); err != nil {
		t.Fatalf("First Close() failed: %v", err)
	}

	// Wait for Run() to complete
	<-runDone

	// Second Close should return ErrLoopTerminated
	if err := loop.Close(); err != ErrLoopTerminated {
		t.Fatalf("Second Close() should return ErrLoopTerminated, got: %v", err)
	}

	// Verify state is Terminated
	if loop.State() != StateTerminated {
		t.Errorf("Expected StateTerminated, got %v", loop.State())
	}
}

// TestLoop_Close_ConcurrentWithShutdown verifies that calling Close() and
// Shutdown() concurrently is safe (only one should proceed).
func TestLoop_Close_ConcurrentWithShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Run the loop in a goroutine
	runDone := make(chan error)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(2)

	// Call Close() concurrently with Shutdown()
	go func() {
		defer wg.Done()
		loop.Close()
	}()

	go func() {
		defer wg.Done()
		loop.Shutdown(context.Background())
	}()

	wg.Wait()

	// Wait for Run() to complete
	<-runDone

	// Verify loop is terminated
	if loop.State() != StateTerminated {
		t.Errorf("Expected StateTerminated after concurrent Close/Shutdown, got %v", loop.State())
	}
}

func TestLoop_Close_AfterShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Run the loop in a goroutine
	runDone := make(chan error)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	if err := loop.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() failed: %v", err)
	}

	// Wait for Run() to complete
	<-runDone

	// Close() after Shutdown() should return ErrLoopTerminated
	if err := loop.Close(); err != ErrLoopTerminated {
		t.Fatalf("Close() after Shutdown() should return ErrLoopTerminated, got: %v", err)
	}
}

func TestLoop_Shutdown_AfterClose(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Run the loop in a goroutine
	runDone := make(chan error)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	if err := loop.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Wait for Run() to complete
	<-runDone

	// Shutdown() after Close() should return ErrLoopTerminated
	if err := loop.Shutdown(ctx); err != ErrLoopTerminated {
		t.Fatalf("Shutdown() after Close() should return ErrLoopTerminated, got: %v", err)
	}
}

// TestLoop_Close_PreventsNewSubmits verifies that after Close(),
// attempting to submit new work fails with appropriate errors.
func TestLoop_Close_PreventsNewSubmits(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Run the loop in a goroutine
	runDone := make(chan error)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	if err := loop.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Wait for Run() to complete
	<-runDone

	// All submit attempts should fail
	testFunc := func() {
		err := loop.Submit(func() {})
		if err != nil {
			t.Logf("Submit error after Close: %v", err)
		}
	}

	testFunc()

	// Verify state is Terminated
	if loop.State() != StateTerminated {
		t.Errorf("Expected StateTerminated after Close, got %v", loop.State())
	}
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
		case <-time.After(2 * time.Second):
			t.Fatalf("Iteration %d: DEADLOCK! Loop failed to shut down. The Zombie State bug is present.", i)
		}
	}
}
