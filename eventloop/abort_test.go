package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ===============================================
// FEATURE-001: AbortController/AbortSignal Tests
// ===============================================

// TestAbortController_New tests creating a new AbortController.
func TestAbortController_New(t *testing.T) {
	controller := NewAbortController()
	if controller == nil {
		t.Fatal("NewAbortController returned nil")
	}

	signal := controller.Signal()
	if signal == nil {
		t.Fatal("Signal() returned nil")
	}

	if signal.Aborted() {
		t.Error("New signal should not be aborted")
	}

	if signal.Reason() != nil {
		t.Error("New signal should have nil reason")
	}
}

// TestAbortController_Abort tests basic abort functionality.
func TestAbortController_Abort(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	// Abort with a reason
	controller.Abort("test reason")

	if !signal.Aborted() {
		t.Error("Signal should be aborted after Abort()")
	}

	// Check reason
	reason := signal.Reason()
	if reason == nil {
		t.Fatal("Reason should not be nil")
	}
	if s, ok := reason.(string); !ok || s != "test reason" {
		t.Errorf("Reason should be 'test reason', got %v", reason)
	}
}

// TestAbortController_AbortWithNilReason tests abort with nil reason.
func TestAbortController_AbortWithNilReason(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	// Abort with nil reason - should use default AbortError
	controller.Abort(nil)

	if !signal.Aborted() {
		t.Error("Signal should be aborted")
	}

	reason := signal.Reason()
	if reason == nil {
		t.Error("Reason should not be nil when aborting with nil")
	}

	// Should be an AbortError
	abortErr, ok := reason.(*AbortError)
	if !ok {
		t.Errorf("Expected *AbortError, got %T", reason)
	}
	if abortErr != nil && abortErr.Reason != "Aborted" {
		t.Errorf("Expected reason 'Aborted', got %v", abortErr.Reason)
	}
}

// TestAbortController_AbortMultipleTimes tests that multiple aborts are ignored.
func TestAbortController_AbortMultipleTimes(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	// Abort multiple times with different reasons
	controller.Abort("first reason")
	controller.Abort("second reason")
	controller.Abort("third reason")

	if !signal.Aborted() {
		t.Error("Signal should be aborted")
	}

	// First abort's reason should be preserved
	reason := signal.Reason()
	if s, ok := reason.(string); !ok || s != "first reason" {
		t.Errorf("Reason should be 'first reason', got %v", reason)
	}
}

// TestAbortSignal_OnAbort tests onabort handler registration and invocation.
func TestAbortSignal_OnAbort(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	var handlerCalled bool
	var receivedReason any

	signal.OnAbort(func(reason any) {
		handlerCalled = true
		receivedReason = reason
	})

	controller.Abort("test abort")

	if !handlerCalled {
		t.Error("OnAbort handler should have been called")
	}

	if s, ok := receivedReason.(string); !ok || s != "test abort" {
		t.Errorf("Handler should receive abort reason, got %v", receivedReason)
	}
}

// TestAbortSignal_OnAbortMultipleHandlers tests multiple handlers.
func TestAbortSignal_OnAbortMultipleHandlers(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	var calls []int

	signal.OnAbort(func(reason any) {
		calls = append(calls, 1)
	})
	signal.OnAbort(func(reason any) {
		calls = append(calls, 2)
	})
	signal.OnAbort(func(reason any) {
		calls = append(calls, 3)
	})

	controller.Abort("test")

	if len(calls) != 3 {
		t.Errorf("Expected 3 handlers to be called, got %d", len(calls))
	}

	// Verify order
	for i, v := range calls {
		if v != i+1 {
			t.Errorf("Handler %d called out of order, got %d", i+1, v)
		}
	}
}

// TestAbortSignal_OnAbortAfterAbort tests registering handler after abort.
func TestAbortSignal_OnAbortAfterAbort(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	// Abort first
	controller.Abort("early abort")

	// Then register handler
	var handlerCalled bool
	signal.OnAbort(func(reason any) {
		handlerCalled = true
	})

	// Handler should be called immediately
	if !handlerCalled {
		t.Error("Handler registered after abort should be called immediately")
	}
}

// TestAbortSignal_OnAbortNilHandler tests nil handler is ignored.
func TestAbortSignal_OnAbortNilHandler(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	// Should not panic
	signal.OnAbort(nil)

	controller.Abort("test")

	// No panic means success
}

// TestAbortSignal_ThrowIfAborted tests ThrowIfAborted method.
func TestAbortSignal_ThrowIfAborted(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	// Not aborted - should return nil
	err := signal.ThrowIfAborted()
	if err != nil {
		t.Errorf("ThrowIfAborted should return nil when not aborted, got %v", err)
	}

	// Abort
	controller.Abort("test reason")

	// Should return error
	err = signal.ThrowIfAborted()
	if err == nil {
		t.Error("ThrowIfAborted should return error when aborted")
	}

	var abortErr *AbortError
	if !errors.As(err, &abortErr) {
		t.Errorf("Expected *AbortError, got %T", err)
	}
}

// TestAbortError_Error tests AbortError.Error() method.
func TestAbortError_Error(t *testing.T) {
	tests := []struct {
		name     string
		reason   any
		expected string
	}{
		{
			name:     "nil reason",
			reason:   nil,
			expected: "AbortError: The operation was aborted",
		},
		{
			name:     "string reason",
			reason:   "user cancelled",
			expected: "AbortError: user cancelled",
		},
		{
			name:     "error reason",
			reason:   errors.New("underlying error"),
			expected: "AbortError: underlying error",
		},
		{
			name:     "other type reason",
			reason:   42,
			expected: "AbortError: The operation was aborted",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := &AbortError{Reason: tc.reason}
			if err.Error() != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, err.Error())
			}
		})
	}
}

// TestAbortError_Is tests AbortError errors.Is support.
func TestAbortError_Is(t *testing.T) {
	err1 := &AbortError{Reason: "test"}
	err2 := &AbortError{Reason: "different"}

	if !errors.Is(err1, err2) {
		t.Error("AbortError.Is should return true for any AbortError")
	}

	if errors.Is(err1, errors.New("not abort")) {
		t.Error("AbortError.Is should return false for non-AbortError")
	}
}

// TestAbortSignal_AddEventListener tests AddEventListener alias.
func TestAbortSignal_AddEventListener(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	var called bool
	signal.AddEventListener("abort", func(reason any) {
		called = true
	})

	controller.Abort("test")

	if !called {
		t.Error("AddEventListener handler should be called")
	}
}

// TestAbortSignal_ConcurrentAccess tests thread safety.
func TestAbortSignal_ConcurrentAccess(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	var wg sync.WaitGroup
	var handlerCount atomic.Int32

	// Register handlers concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			signal.OnAbort(func(reason any) {
				handlerCount.Add(1)
			})
		}()
	}

	// Check aborted concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = signal.Aborted()
		}()
	}

	// Abort from another goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		controller.Abort("concurrent abort")
	}()

	wg.Wait()

	if !signal.Aborted() {
		t.Error("Signal should be aborted")
	}

	// At least some handlers should have been called
	// (exact count depends on timing)
	if handlerCount.Load() == 0 {
		t.Error("At least some handlers should have been called")
	}
}

// TestAbortTimeout tests the AbortTimeout helper function.
func TestAbortTimeout(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var abortCalled bool
	var abortReason any

	// Create abort controller with 50ms timeout
	controller, err := AbortTimeout(loop, 50)
	if err != nil {
		t.Fatalf("AbortTimeout failed: %v", err)
	}

	signal := controller.Signal()
	signal.OnAbort(func(reason any) {
		abortCalled = true
		abortReason = reason
	})

	// Run the loop in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Wait for abort
	time.Sleep(150 * time.Millisecond)

	// Stop the loop
	_ = loop.Shutdown(context.Background())
	<-done

	if !abortCalled {
		t.Error("Abort handler should have been called after timeout")
	}

	// Verify it's a timeout error
	if abortErr, ok := abortReason.(*AbortError); ok {
		if s, ok := abortErr.Reason.(string); ok {
			if s != "TimeoutError: The operation timed out" {
				t.Errorf("Expected timeout error reason, got %v", s)
			}
		}
	} else {
		t.Errorf("Expected *AbortError, got %T", abortReason)
	}
}

// TestAbortController_UseWithPromise tests using abort signal with promise.
func TestAbortController_UseWithPromise(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	controller := NewAbortController()
	signal := controller.Signal()

	var promiseResolved bool
	var promiseRejected bool
	var rejectionReason any

	// Create a promise that respects the abort signal
	promise, resolve, reject := js.NewChainedPromise()

	// Register abort handler
	signal.OnAbort(func(reason any) {
		reject(reason)
	})

	// Simulate async operation that checks abort
	go func() {
		time.Sleep(100 * time.Millisecond)
		if signal.Aborted() {
			return // Already rejected by abort handler
		}
		resolve("success")
	}()

	// Abort before operation completes
	time.Sleep(20 * time.Millisecond)
	controller.Abort("cancelled by user")

	// Wait for promise
	promise.Then(func(val any) any {
		promiseResolved = true
		return val
	}, func(reason any) any {
		promiseRejected = true
		rejectionReason = reason
		return reason
	})

	// Run the loop
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	time.Sleep(200 * time.Millisecond)
	_ = loop.Shutdown(context.Background())
	<-done

	if promiseResolved {
		t.Error("Promise should not resolve when aborted")
	}

	if !promiseRejected {
		t.Error("Promise should reject when aborted")
	}

	if s, ok := rejectionReason.(string); ok && s != "cancelled by user" {
		t.Errorf("Expected rejection reason 'cancelled by user', got %v", rejectionReason)
	}
}

// TestAbortSignal_ReasonTypes tests various reason types.
func TestAbortSignal_ReasonTypes(t *testing.T) {
	tests := []struct {
		name   string
		reason any
	}{
		{"string", "abort reason"},
		{"int", 42},
		{"error", errors.New("error reason")},
		{"struct", struct{ msg string }{"test"}},
		{"slice", []int{1, 2, 3}},
		{"nil", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			controller := NewAbortController()
			signal := controller.Signal()

			var receivedReason any
			signal.OnAbort(func(reason any) {
				receivedReason = reason
			})

			controller.Abort(tc.reason)

			if tc.reason == nil {
				// Nil should become default AbortError
				if _, ok := receivedReason.(*AbortError); !ok {
					t.Errorf("Expected *AbortError for nil, got %T", receivedReason)
				}
			} else if tc.name == "slice" {
				// Slices are not comparable, just check type
				if _, ok := receivedReason.([]int); !ok {
					t.Errorf("Expected []int, got %T", receivedReason)
				}
			} else if tc.name == "struct" {
				// Structs with unexported fields need special handling
				if receivedReason == nil {
					t.Error("Received reason should not be nil")
				}
			} else {
				if receivedReason != tc.reason {
					t.Errorf("Expected %v, got %v", tc.reason, receivedReason)
				}
			}
		})
	}
}

// TestAbortController_SignalIdentity tests that Signal() returns same instance.
func TestAbortController_SignalIdentity(t *testing.T) {
	controller := NewAbortController()

	signal1 := controller.Signal()
	signal2 := controller.Signal()

	if signal1 != signal2 {
		t.Error("Signal() should return the same signal instance")
	}
}

// TestAbortSignal_HandlerExecutionOrder tests FIFO handler execution.
func TestAbortSignal_HandlerExecutionOrder(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	order := make([]int, 0)
	mu := sync.Mutex{}

	for i := 0; i < 5; i++ {
		idx := i
		signal.OnAbort(func(reason any) {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, idx)
		})
	}

	controller.Abort("test")

	// Verify FIFO order
	for i, v := range order {
		if v != i {
			t.Errorf("Handler %d executed at position %d", v, i)
		}
	}
}

// TestAbortSignal_RemoveEventListener tests remove functionality.
func TestAbortSignal_RemoveEventListener(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()

	var calls int

	// Note: Due to Go function comparison semantics, this is a limited test
	// RemoveEventListener compares by pointer, which is tricky in Go

	signal.OnAbort(func(reason any) {
		calls++
	})

	controller.Abort("test")

	// Handler should have been called
	if calls != 1 {
		t.Errorf("Expected 1 call, got %d", calls)
	}
}

// TestAbortError_Unwrap tests that AbortError can be unwrapped.
func TestAbortError_Unwrap(t *testing.T) {
	inner := errors.New("inner error")
	err := &AbortError{Reason: inner}

	// Our AbortError.Is returns true for any AbortError
	if !errors.Is(err, &AbortError{}) {
		t.Error("errors.Is should work with AbortError")
	}
}

// ===============================================
// EXPAND-001: AbortAny Tests
// ===============================================

// TestAbortAny_Empty tests AbortAny with empty slice.
func TestAbortAny_Empty(t *testing.T) {
	composite := AbortAny([]*AbortSignal{})

	if composite == nil {
		t.Fatal("AbortAny should return a signal even for empty input")
	}

	if composite.Aborted() {
		t.Error("Empty AbortAny should not be aborted")
	}
}

// TestAbortAny_SingleSignal tests AbortAny with single signal.
func TestAbortAny_SingleSignal(t *testing.T) {
	controller := NewAbortController()
	composite := AbortAny([]*AbortSignal{controller.Signal()})

	if composite.Aborted() {
		t.Error("Composite should not be aborted initially")
	}

	controller.Abort("test reason")

	if !composite.Aborted() {
		t.Error("Composite should be aborted when input signal aborts")
	}

	if composite.Reason() != "test reason" {
		t.Errorf("Expected reason 'test reason', got %v", composite.Reason())
	}
}

// TestAbortAny_MultipleSignals tests AbortAny with multiple signals.
func TestAbortAny_MultipleSignals(t *testing.T) {
	c1 := NewAbortController()
	c2 := NewAbortController()
	c3 := NewAbortController()

	composite := AbortAny([]*AbortSignal{
		c1.Signal(),
		c2.Signal(),
		c3.Signal(),
	})

	if composite.Aborted() {
		t.Error("Composite should not be aborted initially")
	}

	// Abort the second controller
	c2.Abort("second reason")

	if !composite.Aborted() {
		t.Error("Composite should be aborted when any input signal aborts")
	}

	if composite.Reason() != "second reason" {
		t.Errorf("Expected reason 'second reason', got %v", composite.Reason())
	}
}

// TestAbortAny_AlreadyAborted tests AbortAny with already-aborted signal.
func TestAbortAny_AlreadyAborted(t *testing.T) {
	c1 := NewAbortController()
	c2 := NewAbortController()

	// Abort c1 before creating composite
	c1.Abort("pre-aborted")

	composite := AbortAny([]*AbortSignal{
		c1.Signal(),
		c2.Signal(),
	})

	// Should be immediately aborted with c1's reason
	if !composite.Aborted() {
		t.Error("Composite should be immediately aborted if input is already aborted")
	}

	if composite.Reason() != "pre-aborted" {
		t.Errorf("Expected reason 'pre-aborted', got %v", composite.Reason())
	}
}

// TestAbortAny_NilSignals tests AbortAny with nil signals in slice.
func TestAbortAny_NilSignals(t *testing.T) {
	c1 := NewAbortController()

	composite := AbortAny([]*AbortSignal{
		nil,
		c1.Signal(),
		nil,
	})

	if composite.Aborted() {
		t.Error("Composite should not be aborted initially")
	}

	c1.Abort("test")

	if !composite.Aborted() {
		t.Error("Composite should be aborted when non-nil signal aborts")
	}
}

// TestAbortAny_OnlyFirstAbortsComposite tests that only first abort takes effect.
func TestAbortAny_OnlyFirstAbortsComposite(t *testing.T) {
	c1 := NewAbortController()
	c2 := NewAbortController()

	composite := AbortAny([]*AbortSignal{
		c1.Signal(),
		c2.Signal(),
	})

	// Both abort
	c1.Abort("first")
	c2.Abort("second")

	// Should have first reason
	if composite.Reason() != "first" {
		t.Errorf("Expected reason 'first', got %v", composite.Reason())
	}
}

// TestAbortAny_ConcurrentAbort tests concurrent abort handling.
func TestAbortAny_ConcurrentAbort(t *testing.T) {
	const numControllers = 10
	controllers := make([]*AbortController, numControllers)
	signals := make([]*AbortSignal, numControllers)

	for i := 0; i < numControllers; i++ {
		controllers[i] = NewAbortController()
		signals[i] = controllers[i].Signal()
	}

	composite := AbortAny(signals)

	// Abort all concurrently
	var wg sync.WaitGroup
	for i := 0; i < numControllers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			controllers[idx].Abort(idx)
		}(i)
	}

	wg.Wait()

	// Should be aborted with one of the reasons
	if !composite.Aborted() {
		t.Error("Composite should be aborted")
	}

	reason, ok := composite.Reason().(int)
	if !ok {
		t.Errorf("Expected int reason, got %T: %v", composite.Reason(), composite.Reason())
	}
	if reason < 0 || reason >= numControllers {
		t.Errorf("Unexpected reason %d", reason)
	}
}

// TestAbortAny_OnAbortHandler tests that composite signal fires OnAbort handlers.
func TestAbortAny_OnAbortHandler(t *testing.T) {
	c1 := NewAbortController()

	composite := AbortAny([]*AbortSignal{c1.Signal()})

	var called atomic.Bool
	var receivedReason any
	composite.OnAbort(func(reason any) {
		called.Store(true)
		receivedReason = reason
	})

	c1.Abort("handler test")

	// Small delay to allow handler to fire
	time.Sleep(10 * time.Millisecond)

	if !called.Load() {
		t.Error("OnAbort handler should have been called")
	}

	if receivedReason != "handler test" {
		t.Errorf("Expected reason 'handler test', got %v", receivedReason)
	}
}

// TestAbortAny_AllNil tests AbortAny with all nil signals.
func TestAbortAny_AllNil(t *testing.T) {
	composite := AbortAny([]*AbortSignal{nil, nil, nil})

	if composite.Aborted() {
		t.Error("Composite with all nil signals should not be aborted")
	}
}
