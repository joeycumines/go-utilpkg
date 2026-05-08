// Promise Finally Full Coverage Tests
//
// Tests comprehensive coverage of ChainedPromise.Finally including:
// - Handler execution on fulfilled promise
// - Handler execution on rejected promise
// - Result propagation (not transformation)
// - Nil onFinally handler
// - Concurrent Finally calls

package eventloop

import (
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
)

// TestPromiseFinally_FulfilledPromisePreservesValue verifies value is preserved (not transformed).
func TestPromiseFinally_FulfilledPromisePreservesValue(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	p, resolve, _ := js.NewChainedPromise()

	finallyCalls := 0
	result := p.Finally(func() {
		finallyCalls++
		// Note: Finally cannot transform the value
	})

	resolve("original-value")
	loop.tick()

	if finallyCalls != 1 {
		t.Fatalf("Finally handler calls = %d, want 1", finallyCalls)
	}

	// Verify original value is preserved
	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got %v", result.State())
	}

	if result.Value() != "original-value" {
		t.Errorf("Expected 'original-value', got %v", result.Value())
	}
}

// TestPromiseFinally_RejectedPromisePreservesReason verifies rejection reason is preserved.
func TestPromiseFinally_RejectedPromisePreservesReason(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	p, _, reject := js.NewChainedPromise()

	finallyCalled := false
	result := p.Finally(func() {
		finallyCalled = true
	})

	originalErr := errors.New("original-error")
	reject(originalErr)
	loop.tick()

	if !finallyCalled {
		t.Error("Finally handler should be called")
	}

	if result.State() != Rejected {
		t.Errorf("Expected Rejected, got %v", result.State())
	}

	// Verify original rejection reason is preserved
	if result.Reason() != originalErr {
		t.Errorf("Expected original error, got %v", result.Reason())
	}
}

// TestPromiseFinally_NilHandlerOnFulfilled tests nil onFinally on fulfilled promise.
func TestPromiseFinally_NilHandlerOnFulfilled(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	p, resolve, _ := js.NewChainedPromise()

	result := p.Finally(nil)

	resolve("value")
	loop.tick()

	// Should still propagate value
	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got %v", result.State())
	}

	if result.Value() != "value" {
		t.Errorf("Expected 'value', got %v", result.Value())
	}
}

// TestPromiseFinally_NilHandlerOnRejected tests nil onFinally on rejected promise.
func TestPromiseFinally_NilHandlerOnRejected(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	p, _, reject := js.NewChainedPromise()

	result := p.Finally(nil)

	originalErr := errors.New("error")
	reject(originalErr)
	loop.tick()

	// Should still propagate rejection
	if result.State() != Rejected {
		t.Errorf("Expected Rejected, got %v", result.State())
	}
	if result.Reason() != originalErr {
		t.Fatalf("rejection reason = %v, want original error", result.Reason())
	}
}

// TestPromiseFinally_ConcurrentMultiple tests multiple concurrent Finally calls.
func TestPromiseFinally_ConcurrentMultiple(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	p, resolve, _ := js.NewChainedPromise()

	var counter atomic.Int32
	results := make([]*ChainedPromise, 5)
	start := make(chan struct{})
	done := make(chan struct{}, len(results))
	for i := range 5 {
		go func() {
			<-start
			results[i] = p.Finally(func() { counter.Add(1) })
			done <- struct{}{}
		}()
	}
	close(start)
	for range results {
		waitContractSignal(t, done, "concurrent Finally registration")
	}

	resolve("value")
	loop.tick()

	if got := counter.Load(); got != int32(len(results)) {
		t.Fatalf("Finally calls = %d, want %d", got, len(results))
	}

	// All results should be fulfilled
	for i, r := range results {
		if r.State() != Fulfilled {
			t.Errorf("Result %d: expected Fulfilled, got %v", i, r.State())
		}
		if r.Value() != "value" {
			t.Errorf("Result %d: expected 'value', got %v", i, r.Value())
		}
	}
}

// TestPromiseFinally_AlreadyFulfilled tests Finally on already-settled fulfilled promise.
func TestPromiseFinally_AlreadyFulfilled(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	p, resolve, _ := js.NewChainedPromise()
	resolve("pre-resolved")
	loop.tick()

	// Attach Finally after settlement
	finallyCalled := false
	result := p.Finally(func() {
		finallyCalled = true
	})

	if finallyCalled {
		t.Fatal("late Finally handler executed synchronously")
	}
	if state := result.State(); state != Pending {
		t.Fatalf("late Finally child before tick = %v, want Pending", state)
	}

	// Settled JS-backed promises schedule late handlers as microtasks.
	loop.tick()

	if !finallyCalled {
		t.Error("Finally should be called on already-fulfilled promise")
	}

	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got %v", result.State())
	}

	if result.Value() != "pre-resolved" {
		t.Errorf("Expected 'pre-resolved', got %v", result.Value())
	}
}

// TestPromiseFinally_AlreadyRejected tests Finally on already-settled rejected promise.
func TestPromiseFinally_AlreadyRejected(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	p, _, reject := js.NewChainedPromise()
	originalErr := errors.New("pre-rejected")
	reject(originalErr)
	loop.tick()

	// Attach Finally after settlement
	finallyCalled := false
	result := p.Finally(func() {
		finallyCalled = true
	})

	if finallyCalled {
		t.Fatal("late Finally handler executed synchronously")
	}
	if state := result.State(); state != Pending {
		t.Fatalf("late Finally child before tick = %v, want Pending", state)
	}

	loop.tick()

	if !finallyCalled {
		t.Error("Finally should be called on already-rejected promise")
	}

	if result.State() != Rejected {
		t.Errorf("Expected Rejected, got %v", result.State())
	}

	if result.Reason() != originalErr {
		t.Errorf("Expected original error, got %v", result.Reason())
	}
}

// TestPromiseFinally_ChainedWithThen tests Finally in a Then chain.
func TestPromiseFinally_ChainedWithThen(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	p, resolve, _ := js.NewChainedPromise()

	var order []string
	var mu sync.Mutex

	result := p.
		Then(func(v any) any {
			mu.Lock()
			order = append(order, "then1")
			mu.Unlock()
			return v.(string) + "-transformed"
		}, nil).
		Finally(func() {
			mu.Lock()
			order = append(order, "finally")
			mu.Unlock()
		}).
		Then(func(v any) any {
			mu.Lock()
			order = append(order, "then2")
			mu.Unlock()
			return v
		}, nil)

	resolve("start")
	loop.tick()

	mu.Lock()
	if want := []string{"then1", "finally", "then2"}; !slices.Equal(order, want) {
		mu.Unlock()
		t.Fatalf("handler order = %v, want %v", order, want)
	}
	mu.Unlock()

	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got %v", result.State())
	}

	// Value should pass through finally unchanged
	if result.Value() != "start-transformed" {
		t.Errorf("Expected 'start-transformed', got %v", result.Value())
	}
}

// TestPromiseFinally_WithNilValue tests Finally with nil resolved value.
func TestPromiseFinally_WithNilValue(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	p, resolve, _ := js.NewChainedPromise()

	finallyCalled := false
	result := p.Finally(func() {
		finallyCalled = true
	})

	resolve(nil) // Resolve with nil value
	loop.tick()

	if !finallyCalled {
		t.Error("Finally should be called")
	}

	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got %v", result.State())
	}

	if result.Value() != nil {
		t.Errorf("Expected nil value, got %v", result.Value())
	}
}

// TestPromiseFinally_WithNilRejection tests Finally with nil rejection reason.
func TestPromiseFinally_WithNilRejection(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	p, _, reject := js.NewChainedPromise()

	finallyCalled := false
	result := p.Finally(func() {
		finallyCalled = true
	})

	reject(nil) // Reject with nil reason
	loop.tick()

	if !finallyCalled {
		t.Error("Finally should be called")
	}

	if result.State() != Rejected {
		t.Errorf("Expected Rejected, got %v", result.State())
	}
	if result.Reason() != nil {
		t.Fatalf("nil rejection reason = %v, want nil", result.Reason())
	}
}

// TestPromiseFinally_Standalone tests synchronous Finally settlement without a JS scheduler.
func TestPromiseFinally_Standalone(t *testing.T) {
	p := &ChainedPromise{}

	finallyCalled := false
	result := p.Finally(func() {
		finallyCalled = true
	})
	if state := result.State(); state != Pending {
		t.Fatalf("standalone Finally child before settlement = %v, want Pending", state)
	}
	p.resolve("standalone-value")

	if !finallyCalled {
		t.Error("Finally should be called when a standalone promise settles")
	}

	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got %v", result.State())
	}

	if result.Value() != "standalone-value" {
		t.Errorf("Expected 'standalone-value', got %v", result.Value())
	}
}

// TestPromiseFinally_OrderOfExecution tests execution order with multiple handlers.
func TestPromiseFinally_OrderOfExecution(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	p, resolve, _ := js.NewChainedPromise()

	var order []int
	var mu sync.Mutex

	// Attach both Then and Finally
	p.Then(func(v any) any {
		mu.Lock()
		order = append(order, 1)
		mu.Unlock()
		return v
	}, nil)

	p.Finally(func() {
		mu.Lock()
		order = append(order, 2)
		mu.Unlock()
	})

	p.Then(func(v any) any {
		mu.Lock()
		order = append(order, 3)
		mu.Unlock()
		return v
	}, nil)

	resolve("value")
	loop.tick()

	mu.Lock()
	defer mu.Unlock()

	if want := []int{1, 2, 3}; !slices.Equal(order, want) {
		t.Fatalf("handler order = %v, want %v", order, want)
	}
}
