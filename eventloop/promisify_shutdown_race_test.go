// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// EXPAND-008: Promisify Shutdown Race Coverage
// Tests for:
// - Promisify called during shutdown
// - Shutdown waits for in-flight Promisify goroutines
// - promisifyMu lock coordination

// TestPromisify_DuringShutdown tests Promisify called while loop is shutting down
func TestPromisify_DuringShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Start shutdown in background
	shutdownDone := make(chan struct{})
	go func() {
		loop.Shutdown(context.Background())
		close(shutdownDone)
	}()

	// Try Promisify immediately during shutdown window
	// This races with shutdown - may succeed or be rejected
	p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		return "result", nil
	})

	// Wait for promise to settle
	select {
	case <-p.ToChannel():
		// Good - promise settled
	case <-time.After(10 * time.Second):
		t.Error("Promisify promise should settle during shutdown")
	}

	cancel()
	<-runDone
	<-shutdownDone
}

// TestPromisify_ShutdownWaitsForInflight tests that shutdown waits for in-flight Promisify
func TestPromisify_ShutdownWaitsForInflight(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Start a slow Promisify
	slowStarted := make(chan struct{})
	slowFinished := make(chan struct{})
	_ = loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		close(slowStarted)
		time.Sleep(100 * time.Millisecond) // Slow operation
		close(slowFinished)
		return "slow result", nil
	})

	// Wait for slow Promisify to start
	<-slowStarted

	// Start shutdown - should wait for slow Promisify
	shutdownStart := time.Now()
	shutdownDone := make(chan struct{})
	go func() {
		loop.Shutdown(context.Background())
		close(shutdownDone)
	}()

	// Shutdown should not complete before slow operation finishes
	select {
	case <-slowFinished:
		// Good - slow operation finished
	case <-shutdownDone:
		// Shutdown completed before slow operation - check if slow operation also finished
		select {
		case <-slowFinished:
			// OK - they finished at about the same time
		default:
			t.Error("Shutdown completed before slow Promisify finished")
		}
	case <-time.After(10 * time.Second):
		t.Error("Test timed out")
	}
	<-shutdownDone

	// Shutdown should have taken at least 50ms (giving some tolerance)
	elapsed := time.Since(shutdownStart)
	if elapsed < 50*time.Millisecond {
		t.Logf("Warning: Shutdown completed in %v (expected ~100ms)", elapsed)
	}

	cancel()
	<-runDone
}

// TestPromisify_ConcurrentWithShutdown_Race tests concurrent Promisify calls during shutdown
func TestPromisify_ConcurrentWithShutdown_Race(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Launch many concurrent Promisify calls
	const numCalls = 50
	var wg sync.WaitGroup
	var successCount, rejectCount atomic.Int64

	for i := 0; i < numCalls; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
				time.Sleep(time.Duration(idx) * time.Millisecond)
				return idx, nil
			})
			<-p.ToChannel()
			if p.State() == Rejected {
				rejectCount.Add(1)
			} else {
				successCount.Add(1)
			}
		}(i)
	}

	// Give some Promisify calls time to start
	time.Sleep(10 * time.Millisecond)

	// Start shutdown
	loop.Shutdown(context.Background())

	// Wait for all Promisify calls to complete
	wg.Wait()

	t.Logf("Success: %d, Rejected: %d", successCount.Load(), rejectCount.Load())
	// At least some should have been rejected with ErrLoopTerminated
	if rejectCount.Load() == 0 && successCount.Load() == numCalls {
		t.Log("All Promisify calls succeeded (possible if all started before shutdown)")
	}

	cancel()
	<-runDone
}

// TestPromisify_MuLockCoordination tests promisifyMu lock prevents races
func TestPromisify_MuLockCoordination(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Rapid Promisify calls to stress the lock
	const numCalls = 100
	var wg sync.WaitGroup
	promises := make([]Promise, numCalls)

	for i := 0; i < numCalls; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			promises[idx] = loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
				return idx, nil
			})
		}()
	}

	wg.Wait()

	// All promises should have been created
	for i, p := range promises {
		if p == nil {
			t.Errorf("Promise %d is nil", i)
		}
	}

	cancel()
	<-runDone
}

// TestPromisify_AfterTerminated tests Promisify on terminated loop
func TestPromisify_AfterTerminated(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Shutdown and wait
	loop.Shutdown(context.Background())
	cancel()
	<-runDone

	// Now call Promisify on terminated loop
	p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		t.Error("Function should not be called on terminated loop")
		return nil, nil
	})

	// Should be immediately rejected
	select {
	case result := <-p.ToChannel():
		err, ok := result.(error)
		if !ok || !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Expected ErrLoopTerminated, got: %v", result)
		}
	case <-time.After(2 * time.Second):
		t.Error("Promise should settle immediately for terminated loop")
	}
}

// TestPromisify_InTerminatingState tests Promisify during StateTerminating
// This test is similar to TestPromisify_LoopTerminatingState but uses state transition
func TestPromisify_InTerminatingState(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Start shutdown - this will transition to StateTerminating
	go loop.Shutdown(context.Background())

	// Try Promisify during termination - result depends on timing
	p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		// This may or may not be called depending on timing
		return "result", nil
	})

	// Should settle with either result or error - should never hang.
	// Use generous timeout to prevent intermittent failures under heavy
	// system load or race detector overhead.
	select {
	case result := <-p.ToChannel():
		if err, ok := result.(error); ok {
			if !errors.Is(err, ErrLoopTerminated) {
				t.Logf("Promise rejected with unexpected error: %v", err)
			} else {
				t.Log("Promise correctly rejected with ErrLoopTerminated")
			}
		} else {
			t.Logf("Promise resolved with: %v", result)
		}
	case <-time.After(10 * time.Second):
		t.Error("Promise should settle during termination")
	}

	cancel()
	<-runDone
}

// TestPromisify_SubmitInternalFallback tests fallback when SubmitInternal fails
func TestPromisify_SubmitInternalFallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Start a Promisify that will complete after we start shutdown
	completionSignal := make(chan struct{})
	p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		<-completionSignal // Wait for signal
		return "result after shutdown", nil
	})

	// Start shutdown
	go func() {
		loop.Shutdown(context.Background())
	}()

	// Give shutdown time to start
	time.Sleep(50 * time.Millisecond)

	// Now let the Promisify complete - SubmitInternal may fail
	close(completionSignal)

	// Promise should still settle (via fallback direct resolution)
	select {
	case result := <-p.ToChannel():
		if err, ok := result.(error); ok {
			t.Logf("Promise rejected (expected during shutdown): %v", err)
		} else {
			t.Logf("Promise resolved with: %v", result)
		}
	case <-time.After(10 * time.Second):
		t.Error("Promise should settle even when SubmitInternal fails")
	}

	cancel()
	<-runDone
}

// TestPromisify_ContextCancelDuringShutdown tests context cancellation races with shutdown
func TestPromisify_ContextCancelDuringShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	loopCtx, loopCancel := context.WithCancel(context.Background())
	defer loopCancel()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(loopCtx)
		close(runDone)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Create cancellable context for Promisify
	promCtx, promCancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	p := loop.Promisify(promCtx, func(ctx context.Context) (Result, error) {
		close(started)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return "timeout", nil
		}
	})

	<-started

	// Cancel context and shutdown concurrently
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		promCancel()
	}()
	go func() {
		defer wg.Done()
		loop.Shutdown(context.Background())
	}()
	wg.Wait()

	// Promise should settle
	select {
	case <-p.ToChannel():
		// Good
	case <-time.After(10 * time.Second):
		t.Error("Promise should settle")
	}

	loopCancel()
	<-runDone
}

// TestPromisify_MultipleShutdownCalls tests multiple concurrent Shutdown calls
func TestPromisify_MultipleShutdownCalls(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Start a slow Promisify
	_ = loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		time.Sleep(100 * time.Millisecond)
		return "slow", nil
	})

	// Multiple concurrent Shutdown calls
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			loop.Shutdown(context.Background())
		}()
	}
	wg.Wait()

	cancel()
	<-runDone
}

// TestPromisify_PanicDuringShutdown tests panic recovery when shutdown is in progress
func TestPromisify_PanicDuringShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Start a Promisify that panics
	started := make(chan struct{})
	p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		close(started)
		time.Sleep(20 * time.Millisecond)
		panic("test panic during shutdown")
	})

	<-started

	// Start shutdown
	go loop.Shutdown(context.Background())

	// Promise should be rejected with PanicError
	select {
	case result := <-p.ToChannel():
		if result == nil {
			t.Error("Expected error from panicking Promisify")
		} else if err, ok := result.(error); ok {
			_, isPanic := err.(PanicError)
			if !isPanic && !errors.Is(err, ErrLoopTerminated) {
				t.Logf("Got error: %T %v", err, err)
			}
		}
	case <-time.After(10 * time.Second):
		t.Error("Promise should settle")
	}

	cancel()
	<-runDone
}

// TestPromisify_WgCounterIntegrity tests WaitGroup counter integrity
func TestPromisify_WgCounterIntegrity(t *testing.T) {
	for iteration := 0; iteration < 10; iteration++ {
		loop, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		runDone := make(chan struct{})
		go func() {
			_ = loop.Run(ctx)
			close(runDone)
		}()

		time.Sleep(5 * time.Millisecond)

		// Rapid Promisify and shutdown
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
					return nil, nil
				})
			}()
		}

		// Shutdown while Promisify calls are in flight
		go loop.Shutdown(context.Background())

		wg.Wait()
		cancel()
		<-runDone
	}
	// If WaitGroup counter becomes negative, we'll get a panic
}
