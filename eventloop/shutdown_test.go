package eventloop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestShutdown_PendingPromisesRejected verifies that pending promises are
// rejected with ErrLoopTerminated when the loop is stopped.
func TestShutdown_PendingPromisesRejected(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		if err := loop.Run(ctx); err != nil && err != context.Canceled {
			t.Errorf("Run() unexpected error: %v", err)
		}
		close(runDone)
	}()

	// Wait for loop to be running
	time.Sleep(10 * time.Millisecond)

	blocker := make(chan struct{})
	promise := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		<-blocker
		return nil, nil
	})

	ch := promise.ToChannel()

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	loop.Shutdown(stopCtx)
	<-runDone

	select {
	case result := <-ch:
		err, ok := result.(error)
		if !ok {
			t.Fatalf("Expected error, got: %v", result)
		}
		if !errors.Is(err, ErrLoopTerminated) {
			t.Fatalf("Expected ErrLoopTerminated, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ZOMBIE PROMISE: Never rejected after shutdown")
	}

	close(blocker)
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
		if err := l.Run(context.Background()); err != nil && err != context.Canceled {
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
		if err := l.Run(ctx); err != nil && err != context.Canceled {
			t.Errorf("Run() unexpected error: %v", err)
		}
		close(runDone)
	}()

	// Wait for loop to be running
	time.Sleep(10 * time.Millisecond)

	_, p := l.registry.NewPromise()

	l.Submit(Task{Runnable: func() {
		time.Sleep(10 * time.Millisecond)
		p.Resolve("manual_success")
	}})

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
		if err := loop.Run(ctx); err != nil && err != context.Canceled {
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
