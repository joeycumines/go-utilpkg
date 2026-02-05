// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// COVERAGE-001: thenStandalone Function 100% Coverage Tests
//
// These tests target specific code paths in thenStandalone() that were
// previously uncovered. The function handles promises without a JS adapter.
// ============================================================================

// ---------------------------------------------------------------------------
// Path 1: Pending promise, h0.target is nil (first handler storage)
// ---------------------------------------------------------------------------

func TestThenStandalone_Pending_FirstHandler_H0TargetNil(t *testing.T) {
	// Create a pending promise with js=nil
	p := &ChainedPromise{
		id: 100,
		js: nil,
		// h0 is zero-value, so h0.target is nil
	}
	p.state.Store(int32(Pending))

	var handlerCalled atomic.Bool
	var receivedValue Result

	// Call Then - this should store handler in h0 (first handler slot)
	child := p.Then(
		func(v Result) Result {
			handlerCalled.Store(true)
			receivedValue = v
			return "transformed"
		},
		nil,
	)

	// Verify child promise was created with correct ID
	if child.id != 101 {
		t.Errorf("Child ID should be p.id+1=101, got: %d", child.id)
	}

	// Verify child is pending (handler not called yet)
	if child.State() != Pending {
		t.Errorf("Child should be Pending, got: %v", child.State())
	}

	// Verify handler was stored in h0
	p.mu.Lock()
	hasH0 := p.h0.target != nil
	p.mu.Unlock()
	if !hasH0 {
		t.Error("Handler should be stored in h0.target")
	}

	// Verify handler was NOT called yet
	if handlerCalled.Load() {
		t.Error("Handler should not be called for pending promise")
	}

	// Now manually resolve the promise to trigger the handler
	p.resolve("test value", nil)

	// Allow time for synchronous execution
	time.Sleep(10 * time.Millisecond)

	// Verify handler was called
	if !handlerCalled.Load() {
		t.Error("Handler should be called after resolve")
	}

	if receivedValue != "test value" {
		t.Errorf("Expected 'test value', got: %v", receivedValue)
	}

	// Verify child is now fulfilled
	if child.State() != Fulfilled {
		t.Errorf("Child should be Fulfilled, got: %v", child.State())
	}

	if child.Value() != "transformed" {
		t.Errorf("Child value should be 'transformed', got: %v", child.Value())
	}
}

// ---------------------------------------------------------------------------
// Path 2: Pending promise, h0.target is NOT nil (second+ handler storage)
// ---------------------------------------------------------------------------

func TestThenStandalone_Pending_SecondHandler_H0TargetNotNil(t *testing.T) {
	// Create a pending promise with js=nil
	p := &ChainedPromise{
		id: 200,
		js: nil,
	}
	p.state.Store(int32(Pending))

	var handler1Called, handler2Called atomic.Bool

	// First Then - stores in h0
	child1 := p.Then(
		func(v Result) Result {
			handler1Called.Store(true)
			return "from handler1"
		},
		nil,
	)

	// Second Then - should store in handlers slice (p.result is nil initially)
	child2 := p.Then(
		func(v Result) Result {
			handler2Called.Store(true)
			return "from handler2"
		},
		nil,
	)

	// Verify both children created
	// Note: thenStandalone uses simple p.id+1 for all children, so both get 201
	if child1.id != 201 {
		t.Errorf("Child1 ID should be 201, got: %d", child1.id)
	}
	// Both children get the same ID (p.id+1=201) - this is expected for thenStandalone
	if child2.id != 201 {
		t.Errorf("Child2 ID should be 201 (same as child1), got: %d", child2.id)
	}

	// Verify handlers slice was created
	p.mu.Lock()
	handlers, ok := p.result.([]handler)
	p.mu.Unlock()
	if !ok || len(handlers) != 1 {
		t.Errorf("Expected handlers slice with 1 element, got: %v", p.result)
	}

	// Resolve and verify both handlers called
	p.resolve("value", nil)
	time.Sleep(10 * time.Millisecond)

	if !handler1Called.Load() {
		t.Error("Handler 1 should be called")
	}
	if !handler2Called.Load() {
		t.Error("Handler 2 should be called")
	}
}

// ---------------------------------------------------------------------------
// Path 3: Pending + p.result already has handlers (append to existing slice)
// ---------------------------------------------------------------------------

func TestThenStandalone_Pending_ThirdHandler_ExistingSlice(t *testing.T) {
	// Create a pending promise with js=nil
	p := &ChainedPromise{
		id: 300,
		js: nil,
	}
	p.state.Store(int32(Pending))

	var h1, h2, h3 atomic.Bool

	// Handler 1 - goes into h0
	p.Then(func(v Result) Result {
		h1.Store(true)
		return nil
	}, nil)

	// Handler 2 - creates handlers slice with 1 element
	p.Then(func(v Result) Result {
		h2.Store(true)
		return nil
	}, nil)

	// Handler 3 - appends to existing slice
	p.Then(func(v Result) Result {
		h3.Store(true)
		return nil
	}, nil)

	// Verify handlers slice has 2 elements (h2 and h3)
	p.mu.Lock()
	handlers, ok := p.result.([]handler)
	p.mu.Unlock()
	if !ok || len(handlers) != 2 {
		t.Errorf("Expected handlers slice with 2 elements, got: %v (ok=%v)", handlers, ok)
	}

	// Resolve and verify all handlers called
	p.resolve("value", nil)
	time.Sleep(10 * time.Millisecond)

	if !h1.Load() || !h2.Load() || !h3.Load() {
		t.Errorf("All handlers should be called: h1=%v, h2=%v, h3=%v",
			h1.Load(), h2.Load(), h3.Load())
	}
}

// ---------------------------------------------------------------------------
// Path 4: Fulfilled + nil onFulfilled handler (pass-through resolution)
// ---------------------------------------------------------------------------

func TestThenStandalone_Fulfilled_NilOnFulfilled_PassThrough(t *testing.T) {
	// Create an already-fulfilled promise with js=nil
	p := &ChainedPromise{
		id:     400,
		js:     nil,
		result: "original value",
	}
	p.state.Store(int32(Fulfilled))

	// Call Then with nil onFulfilled - should pass through value
	child := p.Then(nil, nil)

	// Child should be fulfilled with same value (pass-through)
	if child.State() != Fulfilled {
		t.Errorf("Child should be Fulfilled, got: %v", child.State())
	}

	if child.Value() != "original value" {
		t.Errorf("Child value should be 'original value', got: %v", child.Value())
	}
}

// ---------------------------------------------------------------------------
// Path 5: Rejected + nil onRejected handler (pass-through rejection)
// ---------------------------------------------------------------------------

func TestThenStandalone_Rejected_NilOnRejected_PassThrough(t *testing.T) {
	// Create an already-rejected promise with js=nil
	testErr := errors.New("test rejection")
	p := &ChainedPromise{
		id:     500,
		js:     nil,
		result: testErr,
	}
	p.state.Store(int32(Rejected))

	// Call Then with nil onRejected - should pass through rejection
	child := p.Then(func(v Result) Result {
		// This should NOT be called
		t.Error("onFulfilled should not be called for rejected promise")
		return nil
	}, nil) // nil onRejected

	// Child should be rejected with same reason
	if child.State() != Rejected {
		t.Errorf("Child should be Rejected, got: %v", child.State())
	}

	if child.Reason() != testErr {
		t.Errorf("Child reason should be same error, got: %v", child.Reason())
	}
}

// ---------------------------------------------------------------------------
// Path 6: Fulfilled + onFulfilled handler (synchronous call)
// Already covered but adding explicit test for completeness
// ---------------------------------------------------------------------------

func TestThenStandalone_Fulfilled_WithHandler_Synchronous(t *testing.T) {
	p := &ChainedPromise{
		id:     600,
		js:     nil,
		result: 42,
	}
	p.state.Store(int32(Fulfilled))

	var handlerCalled atomic.Bool

	child := p.Then(func(v Result) Result {
		handlerCalled.Store(true)
		return v.(int) * 2
	}, nil)

	// Handler should be called synchronously (not via microtask)
	if !handlerCalled.Load() {
		t.Error("Handler should be called immediately for already-fulfilled promise")
	}

	if child.State() != Fulfilled {
		t.Errorf("Child should be Fulfilled, got: %v", child.State())
	}

	if child.Value() != 84 {
		t.Errorf("Child value should be 84, got: %v", child.Value())
	}
}

// ---------------------------------------------------------------------------
// Path 7: Rejected + onRejected handler (synchronous call)
// Already covered but adding explicit test for completeness
// ---------------------------------------------------------------------------

func TestThenStandalone_Rejected_WithHandler_Synchronous(t *testing.T) {
	testErr := errors.New("rejected reason")
	p := &ChainedPromise{
		id:     700,
		js:     nil,
		result: testErr,
	}
	p.state.Store(int32(Rejected))

	var handlerCalled atomic.Bool
	var receivedReason Result

	child := p.Then(nil, func(r Result) Result {
		handlerCalled.Store(true)
		receivedReason = r
		return "recovered"
	})

	// Handler should be called synchronously
	if !handlerCalled.Load() {
		t.Error("Handler should be called immediately for already-rejected promise")
	}

	if receivedReason != testErr {
		t.Errorf("Received reason should be error, got: %v", receivedReason)
	}

	// Child should be fulfilled (recovered from rejection)
	if child.State() != Fulfilled {
		t.Errorf("Child should be Fulfilled (recovered), got: %v", child.State())
	}

	if child.Value() != "recovered" {
		t.Errorf("Child value should be 'recovered', got: %v", child.Value())
	}
}

// ---------------------------------------------------------------------------
// Path 8: Child promise ID generation edge case (ID overflow behavior)
// ---------------------------------------------------------------------------

func TestThenStandalone_ChildIDGeneration(t *testing.T) {
	// Test ID generation with various parent IDs
	testCases := []struct {
		parentID   uint64
		expectedID uint64
	}{
		{0, 1},
		{1, 2},
		{100, 101},
		{^uint64(0) - 1, ^uint64(0)}, // Max uint64 - 1 -> Max uint64
		// Note: ^uint64(0) would overflow to 0, but that's acceptable
	}

	for _, tc := range testCases {
		p := &ChainedPromise{
			id:     tc.parentID,
			js:     nil,
			result: "value",
		}
		p.state.Store(int32(Fulfilled))

		child := p.Then(nil, nil)

		if child.id != tc.expectedID {
			t.Errorf("Parent ID %d: expected child ID %d, got %d",
				tc.parentID, tc.expectedID, child.id)
		}
	}
}

// ---------------------------------------------------------------------------
// Path 9: Handler panic recovery
// ---------------------------------------------------------------------------

func TestThenStandalone_HandlerPanic_Recovery(t *testing.T) {
	p := &ChainedPromise{
		id:     900,
		js:     nil,
		result: "trigger",
	}
	p.state.Store(int32(Fulfilled))

	child := p.Then(func(v Result) Result {
		panic("intentional panic")
	}, nil)

	// Child should be rejected with PanicError
	if child.State() != Rejected {
		t.Errorf("Child should be Rejected due to panic, got: %v", child.State())
	}

	reason := child.Reason()
	panicErr, ok := reason.(PanicError)
	if !ok {
		t.Errorf("Rejection should be PanicError, got: %T", reason)
	}

	if panicErr.Value != "intentional panic" {
		t.Errorf("Panic value should be 'intentional panic', got: %v", panicErr.Value)
	}
}

// ---------------------------------------------------------------------------
// Path 10: Concurrent Then calls on pending promise
// ---------------------------------------------------------------------------

func TestThenStandalone_Concurrent_PendingPromise(t *testing.T) {
	p := &ChainedPromise{
		id: 1000,
		js: nil,
	}
	p.state.Store(int32(Pending))

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	var handlersCalled atomic.Int32
	children := make([]*ChainedPromise, numGoroutines)

	// Attach handlers concurrently
	for i := 0; i < numGoroutines; i++ {
		idx := i
		go func() {
			defer wg.Done()
			children[idx] = p.Then(func(v Result) Result {
				handlersCalled.Add(1)
				return v
			}, nil)
		}()
	}

	wg.Wait()

	// Resolve the promise
	p.resolve("concurrent value", nil)

	// Wait for handlers to execute
	time.Sleep(50 * time.Millisecond)

	// All handlers should be called
	if handlersCalled.Load() != int32(numGoroutines) {
		t.Errorf("Expected %d handlers called, got %d",
			numGoroutines, handlersCalled.Load())
	}

	// All children should be fulfilled
	for i, child := range children {
		if child.State() != Fulfilled {
			t.Errorf("Child %d should be Fulfilled, got: %v", i, child.State())
		}
	}
}

// ---------------------------------------------------------------------------
// Path 11: Rejection handler on pending promise
// ---------------------------------------------------------------------------

func TestThenStandalone_Pending_RejectionHandler(t *testing.T) {
	p := &ChainedPromise{
		id: 1100,
		js: nil,
	}
	p.state.Store(int32(Pending))

	var onRejectedCalled atomic.Bool
	var receivedReason Result

	child := p.Then(
		nil, // no fulfillment handler
		func(r Result) Result {
			onRejectedCalled.Store(true)
			receivedReason = r
			return "recovered"
		},
	)

	// Verify child is pending
	if child.State() != Pending {
		t.Errorf("Child should be Pending, got: %v", child.State())
	}

	// Reject the parent promise
	testErr := errors.New("rejection reason")
	p.reject(testErr, nil)

	// Wait for handler execution
	time.Sleep(10 * time.Millisecond)

	// Verify rejection handler was called
	if !onRejectedCalled.Load() {
		t.Error("onRejected handler should be called after rejection")
	}

	if receivedReason != testErr {
		t.Errorf("Expected error '%v', got: %v", testErr, receivedReason)
	}

	// Child should be fulfilled (recovered)
	if child.State() != Fulfilled {
		t.Errorf("Child should be Fulfilled (recovered), got: %v", child.State())
	}

	if child.Value() != "recovered" {
		t.Errorf("Child value should be 'recovered', got: %v", child.Value())
	}
}

// ---------------------------------------------------------------------------
// Path 12: Both handlers nil on pending promise - should still work
// ---------------------------------------------------------------------------

func TestThenStandalone_Pending_BothHandlersNil(t *testing.T) {
	p := &ChainedPromise{
		id: 1200,
		js: nil,
	}
	p.state.Store(int32(Pending))

	// Both handlers nil
	child := p.Then(nil, nil)

	// Child should be pending initially
	if child.State() != Pending {
		t.Errorf("Child should be Pending, got: %v", child.State())
	}

	// Resolve parent - should propagate value to child
	p.resolve("propagated value", nil)
	time.Sleep(10 * time.Millisecond)

	// Child should be fulfilled with same value (pass-through)
	if child.State() != Fulfilled {
		t.Errorf("Child should be Fulfilled, got: %v", child.State())
	}

	if child.Value() != "propagated value" {
		t.Errorf("Child value should be 'propagated value', got: %v", child.Value())
	}
}

// ---------------------------------------------------------------------------
// Path 13: Reject pending with both handlers nil - rejection pass-through
// ---------------------------------------------------------------------------

func TestThenStandalone_Pending_BothHandlersNil_Rejection(t *testing.T) {
	p := &ChainedPromise{
		id: 1300,
		js: nil,
	}
	p.state.Store(int32(Pending))

	// Both handlers nil
	child := p.Then(nil, nil)

	// Reject parent - should propagate rejection to child
	testErr := errors.New("propagated rejection")
	p.reject(testErr, nil)
	time.Sleep(10 * time.Millisecond)

	// Child should be rejected with same reason
	if child.State() != Rejected {
		t.Errorf("Child should be Rejected, got: %v", child.State())
	}

	if child.Reason() != testErr {
		t.Errorf("Child reason should be same error, got: %v", child.Reason())
	}
}

// ---------------------------------------------------------------------------
// Path 14: Chaining through thenStandalone
// ---------------------------------------------------------------------------

func TestThenStandalone_Chaining_Multiple(t *testing.T) {
	p := &ChainedPromise{
		id:     1400,
		js:     nil,
		result: 1,
	}
	p.state.Store(int32(Fulfilled))

	// Chain 1
	c1 := p.Then(func(v Result) Result {
		return v.(int) + 1
	}, nil)

	// Chain 2
	c2 := c1.Then(func(v Result) Result {
		return v.(int) * 2
	}, nil)

	// Chain 3
	c3 := c2.Then(func(v Result) Result {
		return v.(int) + 10
	}, nil)

	// Verify chain: (1+1)*2+10 = 14
	if c3.State() != Fulfilled {
		t.Errorf("c3 should be Fulfilled, got: %v", c3.State())
	}

	if c3.Value() != 14 {
		t.Errorf("c3 value should be 14, got: %v", c3.Value())
	}
}

// ---------------------------------------------------------------------------
// Path 15: Mixed handlers (onFulfilled for some, onRejected for others)
// ---------------------------------------------------------------------------

func TestThenStandalone_MixedHandlers_Fulfilled(t *testing.T) {
	p := &ChainedPromise{
		id:     1500,
		js:     nil,
		result: "fulfilled",
	}
	p.state.Store(int32(Fulfilled))

	var fulfillCalled, rejectCalled atomic.Bool

	child := p.Then(
		func(v Result) Result {
			fulfillCalled.Store(true)
			return v
		},
		func(r Result) Result {
			rejectCalled.Store(true)
			return r
		},
	)

	// Only fulfillment handler should be called
	if !fulfillCalled.Load() {
		t.Error("Fulfillment handler should be called")
	}
	if rejectCalled.Load() {
		t.Error("Rejection handler should NOT be called for fulfilled promise")
	}

	if child.State() != Fulfilled {
		t.Errorf("Child should be Fulfilled, got: %v", child.State())
	}
}

func TestThenStandalone_MixedHandlers_Rejected(t *testing.T) {
	testErr := errors.New("rejected")
	p := &ChainedPromise{
		id:     1501,
		js:     nil,
		result: testErr,
	}
	p.state.Store(int32(Rejected))

	var fulfillCalled, rejectCalled atomic.Bool

	child := p.Then(
		func(v Result) Result {
			fulfillCalled.Store(true)
			return v
		},
		func(r Result) Result {
			rejectCalled.Store(true)
			return "recovered"
		},
	)

	// Only rejection handler should be called
	if fulfillCalled.Load() {
		t.Error("Fulfillment handler should NOT be called for rejected promise")
	}
	if !rejectCalled.Load() {
		t.Error("Rejection handler should be called")
	}

	// Child should be fulfilled (recovered)
	if child.State() != Fulfilled {
		t.Errorf("Child should be Fulfilled, got: %v", child.State())
	}
	if child.Value() != "recovered" {
		t.Errorf("Child value should be 'recovered', got: %v", child.Value())
	}
}

// ---------------------------------------------------------------------------
// Path 16: Handler returns nil (valid result)
// ---------------------------------------------------------------------------

func TestThenStandalone_HandlerReturnsNil(t *testing.T) {
	p := &ChainedPromise{
		id:     1600,
		js:     nil,
		result: "original",
	}
	p.state.Store(int32(Fulfilled))

	child := p.Then(func(v Result) Result {
		return nil // Explicit nil return
	}, nil)

	if child.State() != Fulfilled {
		t.Errorf("Child should be Fulfilled, got: %v", child.State())
	}

	// Value should be nil (valid fulfilled value)
	if child.Value() != nil {
		t.Errorf("Child value should be nil, got: %v", child.Value())
	}
}

// ---------------------------------------------------------------------------
// Path 17: Zero-value promise (edge case)
// ---------------------------------------------------------------------------

func TestThenStandalone_ZeroValuePromise(t *testing.T) {
	// Zero-value promise (all fields at default)
	p := &ChainedPromise{}
	// id=0, js=nil, result=nil, h0 is zero handler

	// State is 0 which is Pending
	if p.State() != Pending {
		t.Errorf("Zero-value promise should be Pending, got: %v", p.State())
	}

	child := p.Then(func(v Result) Result {
		return "handled"
	}, nil)

	// Child ID should be 0+1=1
	if child.id != 1 {
		t.Errorf("Child ID should be 1, got: %d", child.id)
	}

	// Resolve zero-value promise
	p.resolve(nil, nil)
	time.Sleep(10 * time.Millisecond)

	if child.State() != Fulfilled {
		t.Errorf("Child should be Fulfilled, got: %v", child.State())
	}

	if child.Value() != "handled" {
		t.Errorf("Child value should be 'handled', got: %v", child.Value())
	}
}

// ---------------------------------------------------------------------------
// Path 18: Rejection panic recovery
// ---------------------------------------------------------------------------

func TestThenStandalone_RejectionHandler_Panic(t *testing.T) {
	testErr := errors.New("original error")
	p := &ChainedPromise{
		id:     1800,
		js:     nil,
		result: testErr,
	}
	p.state.Store(int32(Rejected))

	child := p.Then(nil, func(r Result) Result {
		panic("panic in rejection handler")
	})

	// Child should be rejected with PanicError
	if child.State() != Rejected {
		t.Errorf("Child should be Rejected due to panic, got: %v", child.State())
	}

	reason := child.Reason()
	panicErr, ok := reason.(PanicError)
	if !ok {
		t.Errorf("Rejection should be PanicError, got: %T", reason)
	}

	if panicErr.Value != "panic in rejection handler" {
		t.Errorf("Panic value mismatch, got: %v", panicErr.Value)
	}
}
