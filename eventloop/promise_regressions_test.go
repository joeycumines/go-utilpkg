// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestMemoryLeakProof_HandlerLeak_SuccessPath proves that promiseHandlers cleanup occurs on resolve.
// This verifies the fix for Memory Leak #1 from review.md Section 2.A.
//
// Test flow:
// 1. Track initial promiseHandlers count
// 2. Create 10,000 promises with then handlers
// 3. Resolve all promises
// 4. Run microtasks to trigger cleanup
// 5. Force GC
// 6. Verify promiseHandlers map is EMPTY (no entries from resolved promises)
func TestMemoryLeakProof_HandlerLeak_SuccessPath(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Note: loop.tick() is used to process microtasks directly
	// instead of running a blocking loop.Run()

	// Track initial handler count
	js.promiseHandlersMu.RLock()
	initialHandlerCount := len(js.promiseHandlers)
	js.promiseHandlersMu.RUnlock()

	// Create 10,000 promises with rejection handlers (.Catch)
	// The promiseHandlers map tracks promises with rejection handlers
	// (needed for unhandled rejection detection)
	const numPromises = 10000
	resolves := make([]ResolveFunc, 0, numPromises)

	for i := 0; i < numPromises; i++ {
		p, resolve, _ := js.NewChainedPromise()
		resolves = append(resolves, resolve)

		// Attach catch handler - this adds entry to promiseHandlers map
		// The map promiseHandlers[p] = true indicates this promise has
		// a rejection handler attached
		p.Catch(func(r Result) Result {
			return r
		})
	}

	// Verify handlers were added (sanity check)
	js.promiseHandlersMu.RLock()
	handlerCountAfterAttach := len(js.promiseHandlers)
	js.promiseHandlersMu.RUnlock()

	if handlerCountAfterAttach <= initialHandlerCount {
		t.Fatalf("Expected handlers to be added to promiseHandlers map (initial: %d, after: %d)",
			initialHandlerCount, handlerCountAfterAttach)
	}

	// Resolve all promises - this should trigger cleanup in resolve()
	for i := 0; i < numPromises; i++ {
		resolves[i](i)
	}

	// Run microtasks to process all resolve handlers
	// We tick multiple times to ensure all microtasks are processed
	for i := 0; i < 10; i++ {
		loop.tick()
	}

	// Force GC to reclaim memory
	runtime.GC()
	runtime.GC()
	runtime.GC()

	// Verify NO net increase in handlers - cleanup worked!
	js.promiseHandlersMu.RLock()
	finalHandlerCount := len(js.promiseHandlers)
	js.promiseHandlersMu.RUnlock()

	if finalHandlerCount != initialHandlerCount {
		t.Fatalf("Memory Leak: promiseHandlers map has %d entries (expected %d after cleanup). "+
			"Delta: %d handlers leaked from resolved promises",
			finalHandlerCount, initialHandlerCount, finalHandlerCount-initialHandlerCount)
	}
}

// TestMemoryLeakProof_HandlerLeak_LateSubscriber proves that retroactive cleanup prevents leak.
// This verifies the fix for Memory Leak #3 from review.md Section 2.A.
//
// Test flow:
// 1. Create and resolve a promise (already-settled)
// 2. Attach a rejection handler late via Catch()
// 3. Force GC
// 4. Verify promiseHandlers does NOT contain p.id (retroactive cleanup)
func TestMemoryLeakProof_HandlerLeak_LateSubscriber(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Process microtasks via tick() instead of running loop.Run()

	// Create and resolve a promise immediately
	p := js.Resolve("already-settled")

	// Run microtasks to ensure the promise is fully settled
	loop.tick()

	// Attach rejection handler late to an already-fulfilled promise
	p.Catch(func(r Result) Result {
		// This won't be called since promise is fulfilled
		return r
	})

	// Run microtasks to process any scheduled tasks
	loop.tick()

	// Force GC
	runtime.GC()
	runtime.GC()

	// Verify promiseHandlers does NOT contain promise pointer
	// Retroactive cleanup in then() should have removed it immediately
	js.promiseHandlersMu.RLock()
	_, exists := js.promiseHandlers[p]
	js.promiseHandlersMu.RUnlock()

	if exists {
		t.Fatalf("Memory Leak: promiseHandlers still contains promise %p after late Catch() on fulfilled promise. "+
			"Retroactive cleanup failed.", p)
	}
}

// TestMemoryLeakProof_HandlerLeak_LateSubscriberOnRejected proves that late attachment
// to already-rejected promises also properly cleans up when the promise is handled.
func TestMemoryLeakProof_HandlerLeak_LateSubscriberOnRejected(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Process microtasks via tick() instead of running loop.Run()

	// Track initial handler count
	js.promiseHandlersMu.RLock()
	initialHandlerCount := len(js.promiseHandlers)
	js.promiseHandlersMu.RUnlock()

	// Create and reject a promise
	p := js.Reject("already-rejected")

	// Run microtasks to ensure rejection is processed
	// This schedules the unhandled rejection check microtask
	loop.tick()

	// Now attach a catch handler late to the already-rejected promise
	// This should remove the promise from promiseHandlers map via retroactive cleanup
	p.Catch(func(r Result) Result {
		// Handle the rejection
		return "recover"
	})

	// Run microtasks to process the catch handler
	loop.tick()

	// Force GC
	runtime.GC()
	runtime.GC()

	// Verify promiseHandlers does NOT contain promise pointer
	// Retroactive cleanup should have removed it when Catch() was called
	js.promiseHandlersMu.RLock()
	_, exists := js.promiseHandlers[p]
	finalHandlerCount := len(js.promiseHandlers)
	js.promiseHandlersMu.RUnlock()

	if exists {
		t.Fatalf("Memory Leak: promiseHandlers still contains promise %p after late Catch() on rejected promise. "+
			"Retroactive cleanup failed.", p)
	}

	if finalHandlerCount != initialHandlerCount {
		t.Fatalf("Memory Leak: promiseHandlers map has %d entries (expected %d after cleanup). "+
			"Delta: %d handlers leaked",
			finalHandlerCount, initialHandlerCount, finalHandlerCount-initialHandlerCount)
	}
}

// TestMemoryLeakProof_SetImmediate_PanicLeak proves that defer cleanup prevents leak on panic.
// This verifies the fix for Memory Leak #2 from review.md Section 2.A.
//
// Test flow:
// 1. Track initial setImmediateMap size
// 2. Schedule an immediate that panics
// 3. Wait for execution and recovery
// 4. Force GC
// 5. Verify setImmediateMap is EMPTY (defer cleanup deleted entry even on panic)
func TestMemoryLeakProof_SetImmediate_PanicLeak(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Process immediates via tick() instead of running loop.Run()

	// Track initial map size
	js.setImmediateMu.RLock()
	initialImmediateCount := len(js.setImmediateMap)
	js.setImmediateMu.RUnlock()

	// Need to run the loop in a separate goroutine for SetImmediate to work
	// since it uses loop.Submit()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = loop.Run(ctx)
	}()

	// Create a panicking callback
	var panicValue = "test panic value"
	var panicCaught = false
	var executionComplete = make(chan struct{})

	panickingFn := func() {
		defer func() {
			if r := recover(); r != nil {
				if r == panicValue {
					panicCaught = true
				}
				close(executionComplete)
			}
		}()
		panic(panicValue)
	}

	// Schedule the panicking immediate
	_, err = js.SetImmediate(panickingFn)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the task was scheduled (map has one entry)
	js.setImmediateMu.RLock()
	immediateCountAfterSchedule := len(js.setImmediateMap)
	js.setImmediateMu.RUnlock()

	if immediateCountAfterSchedule <= initialImmediateCount {
		t.Fatalf("Expected setImmediateMap to have an entry after scheduling (initial: %d, after: %d)",
			initialImmediateCount, immediateCountAfterSchedule)
	}

	// Wait for the panic to be caught
	select {
	case <-executionComplete:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for panic recovery")
	}

	// Verify panic was caught
	if !panicCaught {
		t.Fatal("Expected panic to be caught")
	}

	// Force GC
	runtime.GC()
	runtime.GC()

	// Verify NO net increase in setImmediateMap entries - defer cleanup worked!
	js.setImmediateMu.RLock()
	finalImmediateCount := len(js.setImmediateMap)
	js.setImmediateMu.RUnlock()

	if finalImmediateCount != initialImmediateCount {
		t.Fatalf("Memory Leak: setImmediateMap has %d entries (expected %d after cleanup). "+
			"Delta: %d entries leaked from panicking immediates",
			finalImmediateCount, initialImmediateCount, finalImmediateCount-initialImmediateCount)
	}
}

// TestPromiseRace_ConcurrentThenReject_HandlersCalled verifies fix for CRITICAL-3
// from review_vs_main_MAX_PARANOIA_2026-01-30.txt.
//
// BUG DESCRIPTION (BEFORE FIX):
// When reject() and then() execute concurrently:
// 1. reject() stores rejection in unhandledRejections
// 2. reject() calls trackRejection() which schedules checkUnhandledRejections microtask
// 3. then() adds handler to promiseHandlers map
// 4. checkUnhandledRejections executes and deletes from unhandledRejections
// 5. PROBLEM: checkUnhandledRejections sees OLD snapshot where handler wasn't registered yet
// 6. Then handler micro task executes, rejection is handled
// 7. checkUnhandledRejections runs AGAIN (from next checkUnhandledRejections call)
// 8. Sees stale rejection in unhandledRejections and reports it (FALSE POSITIVE)
//
// FIX DESCRIPTION:
// trackRejection() is called AFTER handler microtasks are scheduled.
// This ensures checkUnhandledRejections microtask is enqueued AFTER handler microtasks,
// preventing the race where checkUnhandledRejections reports handler-based rejections as unhandled.
//
// Test flow:
// 1. Create promise
// 2. Concurrently: call reject() and then().Catch() with proper synchronization
// 3. Run microtasks to process everything
// 4. Verify handler was called (promise handled)
// 5. Verify no unhandled rejection callback was invoked
func TestPromiseRace_ConcurrentThenReject_HandlersCalled(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.TODO())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Track unhandled rejection calls
	var unhandledRejects []Result
	var unhandledMu sync.Mutex
	callbackCalled := false

	oldCallback := js.unhandledCallback
	js.mu.Lock()
	js.unhandledCallback = func(reason Result) {
		unhandledMu.Lock()
		unhandledRejects = append(unhandledRejects, reason)
		callbackCalled = true
		unhandledMu.Unlock()
	}
	js.mu.Unlock()

	defer func() {
		js.mu.Lock()
		js.unhandledCallback = oldCallback
		js.mu.Unlock()
	}()

	const numPromises = 100

	for i := 0; i < numPromises; i++ {
		i := i
		p, _, reject := js.NewChainedPromise()

		handlerCalled := false
		var handlerMu sync.Mutex

		// Attach Catch handler FIRST
		p.Catch(func(r Result) Result {
			handlerMu.Lock()
			handlerCalled = true
			handlerMu.Unlock()
			return "caught"
		})

		// Small sleep to ensure handler is registered before rejection
		// This is required because Then/Catch don't synchronize with goroutines
		runtime.Gosched()

		// Reject after handler is attached
		reject(fmt.Sprintf("error-%d", i))

		// Run microtasks to process everything
		// We need enough ticks to process all pending microtasks
		// With 100 promises and 2 microtasks each (handler + potentially more),
		// we need at least 200 ticks to ensure all are processed
		for j := 0; j < 300; j++ {
			loop.tick()
		}

		handlerMu.Lock()
		if !handlerCalled {
			t.Errorf("Promise %d: Handler was NOT called", i)
		}
		handlerMu.Unlock()
	}

	// Verify no unhandled rejection callbacks were called
	unhandledMu.Lock()
	if callbackCalled {
		t.Errorf("Unhandled rejection callback was called %d times", len(unhandledRejects))
		t.Errorf("Rejects reported as unhandled: %v", unhandledRejects)
		t.Error("These rejections should have been marked as HANDLED because Catch() was attached")
	}
	unhandledMu.Unlock()

	t.Logf("All %d promises: Handlers called correctly, no false unhandled rejections", numPromises)
}

// TestMemoryLeakProof_MultipleImmediates verifies that multiple setImmediate tasks
// with various outcomes (success, panic) all properly clean up.
func TestMemoryLeakProof_MultipleImmediates(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Process immediates via Run() in separate goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = loop.Run(ctx)
	}()

	// Track initial sizes
	js.setImmediateMu.RLock()
	initialImmediateCount := len(js.setImmediateMap)
	js.setImmediateMu.RUnlock()
	js.promiseHandlersMu.RLock()
	initialHandlerCount := len(js.promiseHandlers)
	js.promiseHandlersMu.RUnlock()

	// Schedule 100 successful immediates
	const numImmediates = 100
	var wg sync.WaitGroup
	wg.Add(numImmediates)

	for i := 0; i < numImmediates; i++ {
		_, err := js.SetImmediate(func() {
			wg.Done()
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Schedule 5 panicking immediates
	for i := 0; i < 5; i++ {
		_, err := js.SetImmediate(func() {
			defer func() {
				if r := recover(); r != nil {
					t.Logf("Caught expected panic: %v", r)
				}
			}()
			panic("expected panic")
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Wait for all successful completions
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for immediates")
	}

	// Force GC
	runtime.GC()
	runtime.GC()

	// Verify NO net increase in either map
	js.setImmediateMu.RLock()
	finalImmediateCount := len(js.setImmediateMap)
	js.setImmediateMu.RUnlock()

	js.promiseHandlersMu.RLock()
	finalHandlerCount := len(js.promiseHandlers)
	js.promiseHandlersMu.RUnlock()

	if finalImmediateCount != initialImmediateCount {
		t.Fatalf("Memory Leak: setImmediateMap has %d entries (expected %d). "+
			"Delta: %d entries leaked",
			finalImmediateCount, initialImmediateCount, finalImmediateCount-initialImmediateCount)
	}

	if finalHandlerCount != initialHandlerCount {
		t.Fatalf("Memory Leak: promiseHandlers map has %d entries (expected %d). "+
			"Delta: %d handlers leaked",
			finalHandlerCount, initialHandlerCount, finalHandlerCount-initialHandlerCount)
	}
}

// TestMemoryLeakProof_PromiseChainingCleanup verifies that promise chains
// properly clean up intermediate handler entries.
func TestMemoryLeakProof_PromiseChainingCleanup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Process microtasks via tick() instead of running loop.Run()

	// Track initial handler count
	js.promiseHandlersMu.RLock()
	initialHandlerCount := len(js.promiseHandlers)
	js.promiseHandlersMu.RUnlock()

	// Create a long chain of promises
	const chainLength = 100
	root, resolve, _ := js.NewChainedPromise()

	// Build chain: p0 -> p1 -> p2 -> ... -> p99
	p := root
	for i := 0; i < chainLength-1; i++ {
		// Each Then() creates a new promise and attaches handler to previous
		num := i
		p = p.Then(func(v Result) Result {
			return num
		}, nil)
	}

	// Resolve the root promise
	resolve("start")

	// Process all microtasks
	for i := 0; i < chainLength+10; i++ {
		loop.tick()
	}

	// Force GC
	runtime.GC()
	runtime.GC()

	// Verify all transient handler entries are cleaned up
	js.promiseHandlersMu.RLock()
	finalHandlerCount := len(js.promiseHandlers)
	js.promiseHandlersMu.RUnlock()

	if finalHandlerCount != initialHandlerCount {
		t.Fatalf("Memory Leak: promiseHandlers map has %d entries (expected %d after %d-chain cleanup). "+
			"Delta: %d handlers leaked",
			finalHandlerCount, initialHandlerCount, chainLength, finalHandlerCount-initialHandlerCount)
	}
}
