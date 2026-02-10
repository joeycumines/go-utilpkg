package eventloop

import (
	"sync/atomic"
	"testing"
)

// TestPromise_ThenStandalone_Basic tests the thenStandalone path
// for promises without a JS runtime (js field is nil).
// This path is used in special cases and needs test coverage.
// Priority: HIGH - thenStandalone currently at 0.0% coverage.
func TestPromise_ThenStandalone_Basic(t *testing.T) {
	// Create a standalone promise (nil js field) that is already fulfilled
	p := &ChainedPromise{
		js:     nil,
		result: "test value",
		h0:     handler{}, // Initialize handler slot
	}
	p.state.Store(int32(Fulfilled))

	var handlerCalled atomic.Bool
	result := p.Then(
		func(v Result) Result {
			handlerCalled.Store(true)
			return "result: " + v.(string)
		},
		nil,
	)

	// thenStandalone path executes handler synchronously
	if !handlerCalled.Load() {
		t.Error("Handler was not called via thenStandalone")
	}

	if result.State() != Fulfilled {
		t.Errorf("Result should be Fulfilled, got: %v", result.State())
	}

	if result.Value() != "result: test value" {
		t.Errorf("Expected 'result: test value', got: %v", result.Value())
	}
}

// TestPromise_ThenStandalone_MultipleChains tests chaining promises
// without a JS runtime.
// Priority: HIGH - thenStandalone chaining edge case.
func TestPromise_ThenStandalone_MultipleChains(t *testing.T) {
	p1 := &ChainedPromise{
		js:     nil,
		result: 5,
		h0:     handler{}, // Initialize handler slot
	}
	p1.state.Store(int32(Fulfilled))

	// chain 1 - this should use thenStandalone path
	p2 := p1.Then(
		func(v Result) Result {
			return v.(int) * 2
		},
		nil,
	)

	// chain 2 - this should also use thenStandalone path
	p3 := p2.Then(
		func(v Result) Result {
			return v.(int) + 10
		},
		nil,
	)

	if p1.Value() != 5 {
		t.Errorf("p1 should have value 5, got: %v", p1.Value())
	}

	if p2.Value() != 10 {
		t.Errorf("p2 should have value 10, got: %v", p2.Value())
	}

	if p3.Value() != 20 {
		t.Errorf("p3 should have value 20, got: %v", p3.Value())
	}
}

// TestPromise_ThenStandalone_Rejection tests rejection handling
// in thenStandalone path.
// Priority: HIGH - thenStandalone rejection handling.
func TestPromise_ThenStandalone_Rejection(t *testing.T) {
	p := &ChainedPromise{
		js:     nil,
		result: "test error",
		h0:     handler{}, // Initialize handler slot
	}
	p.state.Store(int32(Rejected))

	var rejectionHandled atomic.Bool
	var rejectionReason Result

	result := p.Then(
		nil,
		func(r Result) Result {
			rejectionHandled.Store(true)
			rejectionReason = r
			return "recovered"
		},
	)

	if !rejectionHandled.Load() {
		t.Error("Rejection handler was not called via thenStandalone")
	}

	reason := "test error"
	if rejectionReason != reason {
		t.Errorf("Expected reason '%s', got: %v", reason, rejectionReason)
	}

	if result.State() != Fulfilled {
		t.Errorf("Result should be Fulfilled (recovered), got: %v", result.State())
	}

	if result.Value() != "recovered" {
		t.Errorf("Expected 'recovered', got: %v", result.Value())
	}
}

// TestPromise_ThenStandalone_AlreadySettled tests thenStandalone
// when the promise is already fulfilled/rejected.
// Priority: MEDIUM - Edge case already-settled promises.
func TestPromise_ThenStandalone_AlreadySettled(t *testing.T) {
	// Test with already fulfilled promise
	p1 := &ChainedPromise{
		js:     nil,
		result: "pre-fulfilled",
		h0:     handler{}, // Initialize handler slot
	}
	p1.state.Store(int32(Fulfilled))

	var handlerCalled atomic.Bool
	result1 := p1.Then(
		func(v Result) Result {
			handlerCalled.Store(true)
			return v.(string) + " extended"
		},
		nil,
	)

	if !handlerCalled.Load() {
		t.Error("Handler should be called immediately for already-fulfilled promise")
	}

	if result1.Value() != "pre-fulfilled extended" {
		t.Errorf("Expected 'pre-fulfilled extended', got: %v", result1.Value())
	}

	// Test with already rejected promise
	p2 := &ChainedPromise{
		js:     nil,
		result: "pre-rejected",
		h0:     handler{}, // Initialize handler slot
	}
	p2.state.Store(int32(Rejected))

	var rejectionHandled atomic.Bool
	result2 := p2.Then(
		nil,
		func(r Result) Result {
			rejectionHandled.Store(true)
			return "recovered from pre-rejection"
		},
	)

	if !rejectionHandled.Load() {
		t.Error("Rejection handler should be called for already-rejected promise")
	}

	if result2.Value() != "recovered from pre-rejection" {
		t.Errorf("Expected 'recovered from pre-rejection', got: %v", result2.Value())
	}
}

// TestPromise_ThenStandalone_MultipleHandlers tests multiple
// handlers attached to a standalone promise that is already settled.
// Priority: MEDIUM - thenStandalone multiple handlers edge case.
func TestPromise_ThenStandalone_MultipleHandlers(t *testing.T) {
	p := &ChainedPromise{
		js:     nil,
		result: 10,
		h0:     handler{}, // Initialize handler slot
	}
	p.state.Store(int32(Fulfilled))

	var handler1Called, handler2Called atomic.Bool

	r1 := p.Then(
		func(v Result) Result {
			handler1Called.Store(true)
			return v.(int) * 2
		},
		nil,
	)

	r2 := p.Then(
		func(v Result) Result {
			handler2Called.Store(true)
			return v.(int) + 100
		},
		nil,
	)

	if !handler1Called.Load() {
		t.Error("Handler 1 was not called")
	}

	if !handler2Called.Load() {
		t.Error("Handler 2 was not called")
	}

	if r1.Value() != 20 {
		t.Errorf("r1 expected 20, got: %v", r1.Value())
	}

	if r2.Value() != 110 {
		t.Errorf("r2 expected 110, got: %v", r2.Value())
	}
}
