package eventloop

import (
	"errors"
	"testing"
)

// ============================================================================
// Coverage Improvement Tests (Task COVERAGE_1.2)
// ============================================================================

// Test ChainedPromise.State() method - covers promise.go:265
func TestChainedPromise_State_Lifecycle(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	t.Run("Pending state", func(t *testing.T) {
		p, _, _ := js.NewChainedPromise()
		if p.State() != Pending {
			t.Errorf("Initial state should be Pending, got %v", p.State())
		}
	})

	t.Run("Fulfilled state", func(t *testing.T) {
		p, resolve, _ := js.NewChainedPromise()
		resolve("value")
		loop.tick()
		if p.State() != Fulfilled {
			t.Errorf("State should be Fulfilled, got %v", p.State())
		}
	})

	t.Run("Rejected state", func(t *testing.T) {
		p, _, reject := js.NewChainedPromise()
		reject("error")
		loop.tick()
		if p.State() != Rejected {
			t.Errorf("State should be Rejected, got %v", p.State())
		}
	})
}

// Test ChainedPromise.Value() and Reason() methods - covers promise.go:272,284
func TestChainedPromise_ValueAndReason_Accessors(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	t.Run("Value() returns fulfillment value", func(t *testing.T) {
		p, resolve, _ := js.NewChainedPromise()
		resolve("test value")
		loop.tick()

		val := p.Value()
		if val != "test value" {
			t.Errorf("Expected 'test value', got %v", val)
		}
	})

	t.Run("Value() returns nil for pending", func(t *testing.T) {
		p, _, _ := js.NewChainedPromise()
		if p.Value() != nil {
			t.Errorf("Pending promise Value() should return nil, got %v", p.Value())
		}
	})

	t.Run("Value() returns nil for rejected", func(t *testing.T) {
		p, _, reject := js.NewChainedPromise()
		reject("error")
		loop.tick()

		val := p.Value()
		if val != nil {
			t.Errorf("Rejected promise Value() should return nil, got %v", val)
		}
	})

	t.Run("Reason() returns rejection reason", func(t *testing.T) {
		p, _, reject := js.NewChainedPromise()
		reject("reason value")
		loop.tick()

		reason := p.Reason()
		if reason != "reason value" {
			t.Errorf("Expected 'reason value', got %v", reason)
		}
	})

	t.Run("Reason() returns nil for pending", func(t *testing.T) {
		p, _, _ := js.NewChainedPromise()
		if p.Reason() != nil {
			t.Errorf("Pending promise Reason() should return nil, got %v", p.Reason())
		}
	})

	t.Run("Reason() returns nil for fulfilled", func(t *testing.T) {
		p, resolve, _ := js.NewChainedPromise()
		resolve("value")
		loop.tick()

		reason := p.Reason()
		if reason != nil {
			t.Errorf("Fulfilled promise Reason() should return nil, got %v", reason)
		}
	})
}

// Test Chaining Cycle Detection - covers promise.go:296-299
func TestChainedPromise_CycleDetection(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	p, resolve, _ := js.NewChainedPromise()

	// Attempt to resolve promise with itself
	resolve(p)
	loop.tick()

	// Self-resolution rejects with the stable Go-native identity. JavaScript
	// adapters translate this identity to their native TypeError.
	if p.State() != Rejected {
		t.Errorf("Self-resolution should reject, got state %v", p.State())
	}
	if reason, ok := p.Reason().(error); !ok || !errors.Is(reason, ErrPromiseSelfResolution) {
		t.Errorf("self-resolution reason = %T %v, want ErrPromiseSelfResolution", p.Reason(), p.Reason())
	}
}

// Test Promise Adopts State from Another Promise - covers promise.go:304-318
func TestChainedPromise_AdoptsState(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	t.Run("Adopts fulfilled state", func(t *testing.T) {
		p1, resolve1, _ := js.NewChainedPromise()
		p2, resolve2, _ := js.NewChainedPromise()

		// Resolve p2 with p1
		resolve2(p1)

		// Now settle p1
		resolve1("adopted value")
		loop.tick()

		// Both should be fulfilled with same value
		if p1.State() != Fulfilled {
			t.Errorf("p1 should be fulfilled, got state %v", p1.State())
		}
		if p2.State() != Fulfilled {
			t.Errorf("p2 should be fulfilled (adopted), got state %v", p2.State())
		}
		if p1.Value() != "adopted value" {
			t.Errorf("p1 value mismatch, got: %v", p1.Value())
		}
		if p2.Value() != "adopted value" {
			t.Errorf("p2 should adopt p1's value, got: %v", p2.Value())
		}
	})

	t.Run("Adopts rejected state", func(t *testing.T) {
		p1, _, reject1 := js.NewChainedPromise()
		p2, resolve2, _ := js.NewChainedPromise()

		resolve2(p1)
		reject1("adopted error")
		loop.tick()

		// Both should be rejected with same reason
		if p1.State() != Rejected {
			t.Errorf("p1 should be rejected, got state %v", p1.State())
		}
		if p2.State() != Rejected {
			t.Errorf("p2 should be rejected (adopted), got state %v", p2.State())
		}
		if p2.Reason() != "adopted error" {
			t.Errorf("p2 should adopt p1's reason, got: %v", p2.Reason())
		}
	})
}

// Test Nil Handler Pass-Through - covers tryCall promise.go:680-684
func TestChainedPromise_NilHandlerPassThrough(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	t.Run("Then with nil handlers passes value through", func(t *testing.T) {
		p, resolve, _ := js.NewChainedPromise()
		result := p.Then(nil, nil) // Both handlers nil
		resolve("original value")
		loop.tick()

		// Should pass through without modification
		val := result.Value()
		if val != "original value" {
			t.Errorf("Nil handler should pass-through value, got: %v", val)
		}
	})

	t.Run("Catch with nil handler passes reason through", func(t *testing.T) {
		p, _, reject := js.NewChainedPromise()
		result := p.Catch(nil) // nil handler
		reject("original error")
		loop.tick()

		reason := result.Reason()
		if reason != "original error" {
			t.Errorf("Nil Catch handler should pass-through reason, got: %v", reason)
		}
	})
}

// Test Resolve/Reject Idempotency - covers promise.go:322,363
func TestChainedPromise_ResolveRejectIdempotency(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	t.Run("Resolve only accepts first call", func(t *testing.T) {
		p1, resolve1, _ := js.NewChainedPromise()

		resolve1("first")
		resolve1("second") // Should be ignored
		resolve1("third")  // Should be ignored

		loop.tick()

		if p1.Value() != "first" {
			t.Errorf("Should only use first resolve, got: %v", p1.Value())
		}
	})

	t.Run("Reject only accepts first call", func(t *testing.T) {
		p2, _, reject2 := js.NewChainedPromise()

		reject2("first error")
		reject2("second error") // Should be ignored
		reject2("third error")  // Should be ignored

		loop.tick()

		if p2.Reason() != "first error" {
			t.Errorf("Should only use first reject, got: %v", p2.Reason())
		}
	})

	t.Run("Resolve after reject has no effect", func(t *testing.T) {
		p3, resolve3, reject3 := js.NewChainedPromise()

		reject3("rejected")
		resolve3("resolved") // Should be ignored

		loop.tick()

		if p3.State() != Rejected {
			t.Errorf("State should remain Rejected, got: %v", p3.State())
		}
	})

	t.Run("Reject after resolve has no effect", func(t *testing.T) {
		p4, resolve4, reject4 := js.NewChainedPromise()

		resolve4("resolved")
		reject4("rejected") // Should be ignored

		loop.tick()

		if p4.State() != Fulfilled {
			t.Errorf("State should remain Fulfilled, got: %v", p4.State())
		}
	})
}

// Test Then method - covers promise.go Then
