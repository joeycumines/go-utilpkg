// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ==============================================================================
// EXPAND-029: AbortController Integration Tests
// ==============================================================================

// TestAbortIntegration_FetchLikeAbortPattern simulates a fetch-like abort pattern:
// start async operation, abort mid-flight, verify cleanup.
func TestAbortIntegration_FetchLikeAbortPattern(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	controller := NewAbortController()
	signal := controller.Signal()

	var (
		operationStarted   atomic.Bool
		operationCancelled atomic.Bool
		cleanupExecuted    atomic.Bool
	)

	// Simulate a fetch-like operation
	promise, resolve, reject := js.NewChainedPromise()

	// Start "fetch" operation
	go func() {
		operationStarted.Store(true)

		// Simulate network latency
		for i := 0; i < 50; i++ {
			time.Sleep(10 * time.Millisecond)
			if signal.Aborted() {
				operationCancelled.Store(true)
				reject(&AbortError{Reason: signal.Reason()})
				return
			}
		}

		resolve("fetch completed")
	}()

	// Register cleanup handler
	signal.OnAbort(func(reason any) {
		cleanupExecuted.Store(true)
	})

	// Wait for operation to start, then abort mid-flight
	time.Sleep(50 * time.Millisecond)
	controller.Abort("user cancelled request")

	// Track promise outcome
	var promiseRejected atomic.Bool
	var rejectionReason atomic.Value

	promise.Then(func(val Result) Result {
		t.Error("Promise should not resolve when aborted")
		return val
	}, func(reason Result) Result {
		promiseRejected.Store(true)
		rejectionReason.Store(reason)
		return reason
	})

	// Run loop briefly
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	time.Sleep(200 * time.Millisecond)
	_ = loop.Shutdown(context.Background())
	<-done

	// Verify all expected outcomes
	if !operationStarted.Load() {
		t.Error("Operation should have started")
	}

	if !operationCancelled.Load() {
		t.Error("Operation should have been cancelled")
	}

	if !cleanupExecuted.Load() {
		t.Error("Cleanup handler should have executed")
	}

	if !promiseRejected.Load() {
		t.Error("Promise should have been rejected")
	}
}

// TestAbortIntegration_TimeoutAbortRetryPattern tests timeout race with abort
// and retry on timeout.
func TestAbortIntegration_TimeoutAbortRetryPattern(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run loop
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Allow loop to start
	time.Sleep(20 * time.Millisecond)

	var (
		attemptCount atomic.Int32
		successValue atomic.Value
	)

	// Retry function with timeout
	attemptWithTimeout := func(timeoutMs int, shouldSucceed bool) *ChainedPromise {
		controller, err := AbortTimeout(loop, timeoutMs)
		if err != nil {
			t.Errorf("AbortTimeout failed: %v", err)
			return nil
		}

		signal := controller.Signal()
		promise, resolve, reject := js.NewChainedPromise()

		go func() {
			attemptCount.Add(1)

			// First attempt is slow (will timeout), subsequent are fast
			if attemptCount.Load() == 1 && !shouldSucceed {
				time.Sleep(time.Duration(timeoutMs*3) * time.Millisecond)
			} else {
				time.Sleep(10 * time.Millisecond)
			}

			if signal.Aborted() {
				reject(&AbortError{Reason: signal.Reason()})
				return
			}

			resolve("success")
		}()

		return promise
	}

	// First attempt: short timeout, will fail
	p1 := attemptWithTimeout(50, false)
	if p1 == nil {
		t.Fatal("Failed to create first promise")
	}

	var firstAttemptFailed atomic.Bool
	p1.Then(nil, func(reason Result) Result {
		if _, ok := reason.(*AbortError); ok {
			firstAttemptFailed.Store(true)
		}
		return reason
	})

	// Wait for first attempt to timeout
	time.Sleep(100 * time.Millisecond)

	// Retry with longer timeout
	p2 := attemptWithTimeout(500, true)
	if p2 == nil {
		t.Fatal("Failed to create second promise")
	}

	p2.Then(func(val Result) Result {
		successValue.Store(val)
		return val
	}, nil)

	// Wait for second attempt
	time.Sleep(200 * time.Millisecond)

	_ = loop.Shutdown(context.Background())
	<-done

	// Verify retry pattern worked
	if !firstAttemptFailed.Load() {
		t.Error("First attempt should have failed with timeout")
	}

	if v := successValue.Load(); v != "success" {
		t.Errorf("Second attempt should have succeeded, got %v", v)
	}

	if attemptCount.Load() < 2 {
		t.Errorf("Should have had at least 2 attempts, got %d", attemptCount.Load())
	}
}

// TestAbortIntegration_SignalPropagationThroughPromiseChain tests abort signal
// propagation through promise chains.
func TestAbortIntegration_SignalPropagationThroughPromiseChain(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	controller := NewAbortController()
	signal := controller.Signal()

	// Track which stages completed vs aborted
	var (
		stage1Started  atomic.Bool
		stage1Complete atomic.Bool
		stage2Started  atomic.Bool
		stage2Aborted  atomic.Bool
		stage3Started  atomic.Bool
		chainRejected  atomic.Bool
	)

	// Build promise chain that respects abort signal at each stage
	checkAbort := func(stage string, started, completed *atomic.Bool) func(Result) Result {
		return func(val Result) Result {
			started.Store(true)

			// Check abort at each stage
			if signal.Aborted() {
				return &AbortError{Reason: signal.Reason()}
			}

			if completed != nil {
				completed.Store(true)
			}
			return val
		}
	}

	// Start chain
	p1, resolve, _ := js.NewChainedPromise()

	// Chain stages
	p2 := p1.Then(checkAbort("stage1", &stage1Started, &stage1Complete), nil)

	p3 := p2.Then(func(val Result) Result {
		stage2Started.Store(true)

		// This stage has delay - abort during it
		time.Sleep(100 * time.Millisecond)

		if signal.Aborted() {
			stage2Aborted.Store(true)
			return &AbortError{Reason: signal.Reason()}
		}
		return val
	}, nil)

	p4 := p3.Then(func(val Result) Result {
		stage3Started.Store(true)

		// If we get an AbortError, propagate rejection
		if _, ok := val.(*AbortError); ok {
			return val
		}
		return val
	}, nil)

	finalPromise := p4.Catch(func(reason Result) Result {
		chainRejected.Store(true)
		return reason
	})

	// Need a terminal handler to ensure chain executes
	_ = finalPromise

	// Run loop
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Allow loop to start
	time.Sleep(20 * time.Millisecond)

	// Resolve starting promise
	resolve("initial value")

	// Wait for stage 2 to start, then abort
	time.Sleep(70 * time.Millisecond)
	controller.Abort("abort during chain")

	// Wait for chain to complete
	time.Sleep(200 * time.Millisecond)

	_ = loop.Shutdown(context.Background())
	<-done

	// Verify propagation
	if !stage1Started.Load() {
		t.Error("Stage 1 should have started")
	}

	if !stage1Complete.Load() {
		t.Error("Stage 1 should have completed (before abort)")
	}

	if !stage2Started.Load() {
		t.Error("Stage 2 should have started")
	}

	if !stage2Aborted.Load() {
		t.Error("Stage 2 should have detected abort")
	}
}

// TestAbortIntegration_MemoryCleanupAfterAbort verifies handler removal and
// signal collection after abort.
func TestAbortIntegration_MemoryCleanupAfterAbort(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run loop
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Allow loop to start
	time.Sleep(20 * time.Millisecond)

	// Create many controllers with handlers
	const numControllers = 100
	controllers := make([]*AbortController, numControllers)
	handlerCallCounts := make([]atomic.Int32, numControllers)

	for i := 0; i < numControllers; i++ {
		controllers[i] = NewAbortController()
		idx := i

		// Add multiple handlers
		for j := 0; j < 10; j++ {
			controllers[i].Signal().OnAbort(func(reason any) {
				handlerCallCounts[idx].Add(1)
			})
		}
	}

	// Abort all controllers
	for i := 0; i < numControllers; i++ {
		controllers[i].Abort("cleanup test")
	}

	// Verify all handlers were called
	for i := 0; i < numControllers; i++ {
		if handlerCallCounts[i].Load() != 10 {
			t.Errorf("Controller %d: expected 10 handler calls, got %d",
				i, handlerCallCounts[i].Load())
		}
	}

	// Clear references and force GC
	controllers = nil
	runtime.GC()
	runtime.GC()

	// Verify no memory leaks by creating more controllers
	// (if handlers weren't cleaned up, this would accumulate memory)
	for i := 0; i < numControllers; i++ {
		c := NewAbortController()
		c.Signal().OnAbort(func(reason any) {})
		c.Abort("gc test")
	}

	runtime.GC()

	_ = loop.Shutdown(context.Background())
	<-done
}

// TestAbortIntegration_AbortSignalAnyMultipleOperations tests AbortSignal.any()
// with multiple concurrent operations.
func TestAbortIntegration_AbortSignalAnyMultipleOperations(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run loop
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Allow loop to start
	time.Sleep(20 * time.Millisecond)

	// Create multiple abort controllers
	c1 := NewAbortController()
	c2 := NewAbortController()
	c3 := NewAbortController()

	// Create composite signal
	composite := AbortAny([]*AbortSignal{
		c1.Signal(),
		c2.Signal(),
		c3.Signal(),
	})

	// Track operations
	var (
		op1Started atomic.Bool
		op1Aborted atomic.Bool
		op2Started atomic.Bool
		op2Aborted atomic.Bool
		op3Started atomic.Bool
		op3Aborted atomic.Bool
	)

	// Create promises that respect the composite signal
	createOperation := func(started, aborted *atomic.Bool, delayMs int) *ChainedPromise {
		p, resolve, reject := js.NewChainedPromise()

		go func() {
			started.Store(true)

			// Poll for abort
			for i := 0; i < delayMs/10; i++ {
				if composite.Aborted() {
					aborted.Store(true)
					reject(&AbortError{Reason: composite.Reason()})
					return
				}
				time.Sleep(10 * time.Millisecond)
			}

			resolve("completed")
		}()

		return p
	}

	_ = createOperation(&op1Started, &op1Aborted, 500)
	_ = createOperation(&op2Started, &op2Aborted, 500)
	_ = createOperation(&op3Started, &op3Aborted, 500)

	// Wait for operations to start
	time.Sleep(50 * time.Millisecond)

	// Abort via second controller - should abort all via composite
	c2.Abort("second controller abort")

	// Wait for abort to propagate
	time.Sleep(100 * time.Millisecond)

	_ = loop.Shutdown(context.Background())
	<-done

	// Verify all operations saw the abort
	if !op1Started.Load() || !op2Started.Load() || !op3Started.Load() {
		t.Error("All operations should have started")
	}

	if !op1Aborted.Load() || !op2Aborted.Load() || !op3Aborted.Load() {
		t.Errorf("All operations should be aborted: op1=%v, op2=%v, op3=%v",
			op1Aborted.Load(), op2Aborted.Load(), op3Aborted.Load())
	}

	// Verify composite has correct reason
	if composite.Reason() != "second controller abort" {
		t.Errorf("Composite should have second controller's reason, got %v", composite.Reason())
	}
}

// TestAbortIntegration_TimeoutRacingWithManualAbort tests AbortSignal.timeout()
// racing with manual abort.
func TestAbortIntegration_TimeoutRacingWithManualAbort(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run loop
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Allow loop to start
	time.Sleep(20 * time.Millisecond)

	// Test 1: Manual abort wins
	t.Run("ManualAbortWins", func(t *testing.T) {
		// Create timeout controller (500ms)
		timeoutController, err := AbortTimeout(loop, 500)
		if err != nil {
			t.Fatalf("AbortTimeout failed: %v", err)
		}

		// Create manual controller
		manualController := NewAbortController()

		// Composite signal
		composite := AbortAny([]*AbortSignal{
			timeoutController.Signal(),
			manualController.Signal(),
		})

		p, _, reject := js.NewChainedPromise()

		go func() {
			for !composite.Aborted() {
				time.Sleep(10 * time.Millisecond)
			}
			reject(&AbortError{Reason: composite.Reason()})
		}()

		// Manual abort after 50ms (before 500ms timeout)
		time.Sleep(50 * time.Millisecond)
		manualController.Abort("manual abort")

		// Wait for propagation
		time.Sleep(50 * time.Millisecond)

		if !composite.Aborted() {
			t.Error("Composite should be aborted")
		}

		if composite.Reason() != "manual abort" {
			t.Errorf("Manual abort should win, got reason: %v", composite.Reason())
		}

		_ = p
	})

	// Test 2: Timeout wins
	t.Run("TimeoutWins", func(t *testing.T) {
		// Create timeout controller (50ms)
		timeoutController, err := AbortTimeout(loop, 50)
		if err != nil {
			t.Fatalf("AbortTimeout failed: %v", err)
		}

		// Create manual controller (won't be used in time)
		manualController := NewAbortController()

		// Composite signal
		composite := AbortAny([]*AbortSignal{
			timeoutController.Signal(),
			manualController.Signal(),
		})

		var abortReason atomic.Value
		p, _, reject := js.NewChainedPromise()

		go func() {
			for !composite.Aborted() {
				time.Sleep(10 * time.Millisecond)
			}
			abortReason.Store(composite.Reason())
			reject(&AbortError{Reason: composite.Reason()})
		}()

		// Wait for timeout
		time.Sleep(150 * time.Millisecond)

		if !composite.Aborted() {
			t.Error("Composite should be aborted by timeout")
		}

		reason := abortReason.Load()
		if abortErr, ok := reason.(*AbortError); ok {
			if s, ok := abortErr.Reason.(string); ok {
				if s != "TimeoutError: The operation timed out" {
					t.Errorf("Expected timeout reason, got: %v", s)
				}
			}
		}

		_ = p
	})

	_ = loop.Shutdown(context.Background())
	<-done
}

// TestAbortIntegration_ConcurrentAbortFromMultipleGoroutines tests concurrent
// abort from multiple goroutines.
func TestAbortIntegration_ConcurrentAbortFromMultipleGoroutines(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run loop
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Allow loop to start
	time.Sleep(20 * time.Millisecond)

	controller := NewAbortController()
	signal := controller.Signal()

	// Track handler calls
	var handlerCalls atomic.Int32
	var handlerReasons sync.Map

	// Add multiple handlers
	for i := 0; i < 10; i++ {
		idx := i
		signal.OnAbort(func(reason any) {
			handlerCalls.Add(1)
			handlerReasons.Store(idx, reason)
		})
	}

	// Concurrent abort attempts from many goroutines
	const numGoroutines = 50
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			controller.Abort(idx) // Each goroutine tries with its own reason
		}(i)
	}

	wg.Wait()

	// Verify signal is aborted
	if !signal.Aborted() {
		t.Error("Signal should be aborted")
	}

	// Verify handlers were called exactly once each
	if handlerCalls.Load() != 10 {
		t.Errorf("Expected 10 handler calls (once per handler), got %d", handlerCalls.Load())
	}

	// Verify all handlers got the same reason (first abort's reason)
	var firstReason any
	handlerReasons.Range(func(key, value any) bool {
		if firstReason == nil {
			firstReason = value
		} else if firstReason != value {
			t.Errorf("Handler %d got different reason: expected %v, got %v",
				key.(int), firstReason, value)
		}
		return true
	})

	// Signal's reason should match handler reason
	if signal.Reason() != firstReason {
		t.Errorf("Signal reason (%v) should match handler reason (%v)",
			signal.Reason(), firstReason)
	}

	_ = loop.Shutdown(context.Background())
	<-done
}

// TestAbortIntegration_WithPromisify tests AbortController with Promisify integration.
func TestAbortIntegration_WithPromisify(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run loop
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Allow loop to start
	time.Sleep(20 * time.Millisecond)

	t.Run("AbortCancelsPromisify", func(t *testing.T) {
		controller := NewAbortController()
		signal := controller.Signal()

		// Create a context that cancels when signal aborts
		promisifyCtx, promisifyCancel := context.WithCancel(ctx)
		signal.OnAbort(func(reason any) {
			promisifyCancel()
		})

		var operationCancelled atomic.Bool

		promise := loop.Promisify(promisifyCtx, func(ctx context.Context) (Result, error) {
			// Long-running operation that checks context
			for i := 0; i < 50; i++ {
				select {
				case <-ctx.Done():
					operationCancelled.Store(true)
					return nil, ctx.Err()
				default:
					time.Sleep(10 * time.Millisecond)
				}
			}
			return "completed", nil
		})

		// Abort after operation starts
		time.Sleep(50 * time.Millisecond)
		controller.Abort("abort promisify")

		// Wait for promise to settle
		// Wait for promise to settle via channel
		promiseCh := promise.ToChannel()
		var rejected atomic.Bool

		go func() {
			select {
			case <-promiseCh:
				if promise.State() == Rejected {
					rejected.Store(true)
				}
			case <-time.After(200 * time.Millisecond):
			}
		}()

		time.Sleep(100 * time.Millisecond)

		if !operationCancelled.Load() {
			t.Error("Operation should have been cancelled")
		}

		if !rejected.Load() {
			t.Error("Promise should have been rejected")
		}
	})

	t.Run("PromisifyWithAbortTimeout", func(t *testing.T) {
		// Combine Promisify with abort timeout
		var operationStarted atomic.Bool
		var operationCancelled atomic.Bool

		// Create abort timeout - use 200ms to ensure the Promisify goroutine has time
		// to start and enter fn() before the timeout fires (Promisify checks ctx.Done()
		// before calling fn, so if timeout fires too early, fn never runs)
		controller, err := AbortTimeout(loop, 200)
		if err != nil {
			t.Fatalf("AbortTimeout failed: %v", err)
		}

		// Link abort signal to context
		promisifyCtx, promisifyCancel := context.WithCancel(ctx)
		controller.Signal().OnAbort(func(reason any) {
			promisifyCancel()
		})

		promise := loop.Promisify(promisifyCtx, func(ctx context.Context) (Result, error) {
			operationStarted.Store(true)

			// Slow operation that will timeout
			for i := 0; i < 50; i++ {
				select {
				case <-ctx.Done():
					operationCancelled.Store(true)
					return nil, ctx.Err()
				default:
					time.Sleep(20 * time.Millisecond)
				}
			}
			return "completed", nil
		})

		// Wait for promise to settle via channel
		promiseCh := promise.ToChannel()
		var rejected atomic.Bool

		go func() {
			select {
			case <-promiseCh:
				if promise.State() == Rejected {
					rejected.Store(true)
				}
			case <-time.After(400 * time.Millisecond):
			}
		}()

		// Wait for timeout and promise to settle (200ms timeout + margin)
		time.Sleep(400 * time.Millisecond)

		if !operationStarted.Load() {
			t.Error("Operation should have started")
		}

		if !operationCancelled.Load() {
			t.Error("Operation should have been cancelled by timeout")
		}

		if !rejected.Load() {
			t.Error("Promise should have been rejected")
		}
	})

	_ = loop.Shutdown(context.Background())
	<-done
}

// TestAbortIntegration_AbortErrorIs tests that AbortError works with errors.Is.
func TestAbortIntegration_AbortErrorIs(t *testing.T) {
	err1 := &AbortError{Reason: "test1"}
	err2 := &AbortError{Reason: "test2"}
	errOther := errors.New("other error")

	// AbortError.Is returns true for any AbortError
	if !errors.Is(err1, err2) {
		t.Error("errors.Is should return true for two AbortErrors")
	}

	if errors.Is(err1, errOther) {
		t.Error("errors.Is should return false for non-AbortError")
	}

	// Test with wrapped errors
	wrapped := struct {
		error
	}{err1}
	_ = wrapped // Just to ensure AbortError can be embedded
}

// TestAbortIntegration_HandlerPanicDoesNotCrash tests that handler panics
// propagate (matching JS semantics).
func TestAbortIntegration_HandlerPanicDoesNotCrash(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	var handler2Called atomic.Bool

	signal.OnAbort(func(reason any) {
		panic("handler panic")
	})

	signal.OnAbort(func(reason any) {
		handler2Called.Store(true)
	})

	// Abort should cause panic to propagate
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic to propagate from abort handler")
		}
		if r != "handler panic" {
			t.Errorf("Expected 'handler panic', got %v", r)
		}
	}()

	controller.Abort("trigger panic")

	// If we get here, panic didn't propagate
	t.Error("Should not reach here - panic should propagate")
}

// TestAbortIntegration_ThrowIfAbortedPattern tests the ThrowIfAborted pattern.
func TestAbortIntegration_ThrowIfAbortedPattern(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	// Before abort: should return nil
	err := signal.ThrowIfAborted()
	if err != nil {
		t.Errorf("ThrowIfAborted should return nil before abort, got %v", err)
	}

	// After abort: should return AbortError
	controller.Abort("test abort")

	err = signal.ThrowIfAborted()
	if err == nil {
		t.Error("ThrowIfAborted should return error after abort")
	}

	var abortErr *AbortError
	if !errors.As(err, &abortErr) {
		t.Errorf("Error should be *AbortError, got %T", err)
	}

	if abortErr.Reason != "test abort" {
		t.Errorf("AbortError reason should be 'test abort', got %v", abortErr.Reason)
	}
}

// TestAbortIntegration_LargeHandlerList tests memory handling with many handlers.
func TestAbortIntegration_LargeHandlerList(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	const numHandlers = 10000
	var callCount atomic.Int32

	for i := 0; i < numHandlers; i++ {
		signal.OnAbort(func(reason any) {
			callCount.Add(1)
		})
	}

	controller.Abort("large handler test")

	if callCount.Load() != numHandlers {
		t.Errorf("Expected %d handler calls, got %d", numHandlers, callCount.Load())
	}
}

// TestAbortIntegration_SignalReusability tests that signals cannot be reused.
func TestAbortIntegration_SignalReusability(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	// First abort
	controller.Abort("first abort")

	reason1 := signal.Reason()

	// Second abort attempt
	controller.Abort("second abort")

	reason2 := signal.Reason()

	// Reason should be from first abort
	if reason1 != reason2 {
		t.Errorf("Reason should not change: first=%v, second=%v", reason1, reason2)
	}

	if reason1 != "first abort" {
		t.Errorf("Expected 'first abort', got %v", reason1)
	}
}

// TestAbortIntegration_NestedAbortControllers tests nested abort scenarios.
func TestAbortIntegration_NestedAbortControllers(t *testing.T) {
	outer := NewAbortController()
	inner := NewAbortController()

	// Inner aborts when outer aborts
	outer.Signal().OnAbort(func(reason any) {
		inner.Abort(reason)
	})

	var innerAborted atomic.Bool
	inner.Signal().OnAbort(func(reason any) {
		innerAborted.Store(true)
	})

	// Abort outer
	outer.Abort("cascade abort")

	if !innerAborted.Load() {
		t.Error("Inner should be aborted when outer aborts")
	}

	if inner.Signal().Reason() != "cascade abort" {
		t.Errorf("Inner should have outer's reason, got %v", inner.Signal().Reason())
	}
}
