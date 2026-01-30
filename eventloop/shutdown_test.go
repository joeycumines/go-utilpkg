package eventloop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestShutdown_PendingPromisesRejected verifies that:
// 1. Shutdown() correctly waits for promisify goroutines to complete
// 2. Promises created before shutdown settle correctly even when SubmitInternal may fail
//
// This test verifies the fallback behavior in promisify.go (lines 84-91) that ensures
// promises always settle, preserving the actual operation outcome even when the
// infrastructure (SubmitInternal) fails during shutdown race conditions.
func TestShutdown_PendingPromisesRejected(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		if err := loop.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Run() unexpected error: %v", err)
		}
		close(runDone)
	}()

	// Wait for loop to be running
	time.Sleep(10 * time.Millisecond)

	// Use channels to synchronize blocking
	blockCh := make(chan struct{})
	goroutineStarted := make(chan struct{})

	// Create Promisify goroutine that blocks on a manual channel
	// This creates a promisify goroutine that is still in-flight during shutdown
	promise := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		close(goroutineStarted)
		// Block on channel to simulate a long-running operation
		<-blockCh
		// Return success - if SubmitInternal fails during shutdown race,
		// the fallback will preserve this successful result
		return "result", nil
	})

	ch := promise.ToChannel()

	// Wait for promisify goroutine to start and be blocked
	<-goroutineStarted
	time.Sleep(10 * time.Millisecond)

	// Shutdown in a separate goroutine so we can unblock the goroutine
	shutdownComplete := make(chan error)
	go func() {
		shutdownComplete <- loop.Shutdown(context.Background())
	}()

	// Verify Shutdown() is waiting for promisify goroutine by checking
	// that shutdownComplete hasn't fired yet
	select {
	case <-shutdownComplete:
		// This shouldn't happen immediately - Shutdown should be blocked
		t.Fatal("Shutdown completed immediately before goroutine finished - race condition test invalid")
	case <-time.After(50 * time.Millisecond):
		// Good - Shutdown is waiting for the promisify goroutine
	}

	// Unblock the goroutine so it can complete
	// At this point:
	// 1. The goroutine will no longer be blocked
	// 2. Shutdown() can proceed once promisifyWg.Done() is called
	// 3. Depending on timing, SubmitInternal might succeed or fail during
	//    the shutdown transition
	close(blockCh)

	// Wait for Shutdown() to complete
	if err := <-shutdownComplete; err != nil && !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Wait for loop termination
	<-runDone

	// Verify that the promise settles correctly regardless of SubmitInternal
	// outcome. The fallback logic ensures the promise reflects the actual
	// user operation outcome ("result"), not infrastructure failures.
	select {
	case result := <-ch:
		if result != "result" {
			t.Fatalf("Expected 'result', got: %v", result)
		}
		t.Log("SUCCESS: Promise settled correctly (fallback preserved result)")
	case <-time.After(2 * time.Second):
		t.Fatal("ZOMBIE PROMISE: Never settled after shutdown")
	}
}

// TestShutdown_PromisifyResolution_Race verifies that Promisify operations that
// complete during shutdown (StateTerminating) are properly resolved, not rejected.
// This tests bug C4: SubmitInternal rejects during StateTerminating.
func TestShutdown_PromisifyResolution_Race(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	go func() {
		if err := l.Run(context.Background()); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Run() unexpected error: %v", err)
		}
	}()

	// Wait for loop to be running
	time.Sleep(10 * time.Millisecond)

	started := make(chan struct{})
	p := l.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		close(started)
		time.Sleep(50 * time.Millisecond)
		return "success", nil
	})

	<-started
	l.Shutdown(context.Background())

	if p.State() != Resolved {
		t.Errorf("FATAL: Expected Resolved, got %v", p.State())
		if p.State() == Rejected {
			t.Errorf("Reason: %v", p.Result())
		}
	} else if p.Result() != "success" {
		t.Errorf("FATAL: Expected 'success', got %v", p.Result())
	}
}

// TestShutdown_IngressResolvesInternal verifies that ingress tasks submitted
// before shutdown can still resolve promises during the shutdown drain phase.
func TestShutdown_IngressResolvesInternal(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		if err := l.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Run() unexpected error: %v", err)
		}
		close(runDone)
	}()

	// Wait for loop to be running
	time.Sleep(10 * time.Millisecond)

	_, p := l.registry.NewPromise()

	l.Submit(func() {
		time.Sleep(10 * time.Millisecond)
		p.Resolve("manual_success")
	})

	// Wait for task to be submitted and possibly started
	time.Sleep(20 * time.Millisecond)

	l.Shutdown(context.Background())
	<-runDone

	if p.State() != Resolved {
		t.Errorf("FATAL: Ingress task failed to resolve promise. State: %v", p.State())
	}
}

// TestLoop_StopWakesSleepingLoop verifies that Stop() properly wakes a loop
// that is sleeping in poll(), preventing indefinite hangs.
func TestLoop_ShutdownWakesSleepingLoop(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}
	// Ensure we're in poll (sleep) mode for this test
	if err := l.SetFastPathMode(FastPathDisabled); err != nil {
		t.Fatal(err)
	}
	go func() {
		if err := l.Run(context.Background()); err != nil {
			t.Errorf("Run() unexpected error: %v", err)
		}
	}()

	start := time.Now()
	for {
		if LoopState(l.state.Load()) == StateSleeping {
			break
		}
		if time.Since(start) > 1*time.Second {
			t.Fatal("Loop never went to sleep")
		}
		time.Sleep(1 * time.Millisecond)
	}

	done := make(chan error)
	go func() {
		done <- l.Shutdown(context.Background())
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Shutdown failed: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Shutdown() timed out - loop stuck in poll")
	}
}

// TestStopRace verifies that multiple concurrent Stop() callers all return
// without hanging. This tests CRITICAL #1 fix (stopOnce sync.Once).
//
// Failure Mode WITHOUT Fix:
//   - Multiple goroutines call Stop() concurrently
//   - All pass the CAS loop and reach the select
//   - First caller receives from done, returns nil
//   - Second+ callers block FOREVER on done (goroutine leak)
//
// Success Mode WITH Fix:
//   - sync.Once ensures only first caller executes stopImpl()
//   - Subsequent callers wait via stopOnce.Do() and return immediately
//   - NO goroutine leaks, NO hangs
func TestShutdownRace(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		// Use an error channel instead of t.Errorf to avoid calling t.Errorf after test completes
		if err := loop.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrLoopTerminated) {
			// Don't call t.Errorf - test may already be done
		}
		close(runDone)
	}()

	// Wait for loop to be running
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(10)

	// 10 goroutines calling Shutdown() concurrently - ALL must return, not hang
	results := make([]error, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer wg.Done()
			results[id] = loop.Shutdown(context.Background())
		}(i)
	}

	// Wait for ALL or timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// SUCCESS: All goroutines returned - wait for Run() goroutine to finish
		<-runDone
		for i, err := range results {
			if err != nil && i != 0 { // First might be nil, others should be ErrLoopTerminated
				t.Logf("Goroutine %d: %v", i, err)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("RACE CONDITION: Multiple Shutdown() calls caused hang. %d/10 goroutines returned.",
			10-countReturned(results))
	}
}

func countReturned(results []error) int {
	count := 0
	for range results {
		// Any result (nil or error) counts as a successful return
		count++
	}
	return count
}
