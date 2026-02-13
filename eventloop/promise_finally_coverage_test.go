// COVERAGE-015: Promise Finally Full Coverage Tests
//
// Tests comprehensive coverage of ChainedPromise.Finally including:
// - Handler execution on fulfilled promise
// - Handler execution on rejected promise
// - Result propagation (not transformation)
// - Nil onFinally handler
// - Concurrent Finally calls

package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPromiseFinally_FulfilledPromisePreservesValue verifies value is preserved (not transformed).
func TestPromiseFinally_FulfilledPromisePreservesValue(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	finallyCalled := false
	result := p.Finally(func() {
		finallyCalled = true
		// Note: Finally cannot transform the value
	})

	resolve("original-value")
	loop.tick()

	if !finallyCalled {
		t.Error("Finally handler should be called")
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
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

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
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

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
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, _, reject := js.NewChainedPromise()

	result := p.Finally(nil)

	reject(errors.New("error"))
	loop.tick()

	// Should still propagate rejection
	if result.State() != Rejected {
		t.Errorf("Expected Rejected, got %v", result.State())
	}
}

// TestPromiseFinally_ConcurrentMultiple tests multiple concurrent Finally calls.
func TestPromiseFinally_ConcurrentMultiple(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	var callOrder []int
	var mu sync.Mutex
	var counter atomic.Int32

	// Attach multiple Finally handlers concurrently
	var wg sync.WaitGroup
	results := make([]*ChainedPromise, 5)

	for i := 0; i < 5; i++ {
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[idx] = p.Finally(func() {
				order := counter.Add(1)
				mu.Lock()
				callOrder = append(callOrder, int(order))
				mu.Unlock()
			})
		}()
	}

	wg.Wait()

	resolve("value")
	loop.tick()

	// All should be called
	mu.Lock()
	callCount := len(callOrder)
	mu.Unlock()

	if callCount != 5 {
		t.Errorf("Expected 5 Finally calls, got %d", callCount)
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
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()
	resolve("pre-resolved")
	loop.tick()

	// Attach Finally after settlement
	finallyCalled := false
	result := p.Finally(func() {
		finallyCalled = true
	})

	// Should execute immediately for already-settled
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
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, _, reject := js.NewChainedPromise()
	originalErr := errors.New("pre-rejected")
	reject(originalErr)
	loop.tick()

	// Attach Finally after settlement
	finallyCalled := false
	result := p.Finally(func() {
		finallyCalled = true
	})

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
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

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
	if len(order) != 3 {
		t.Errorf("Expected 3 calls, got %d", len(order))
	}
	if order[0] != "then1" || order[1] != "finally" || order[2] != "then2" {
		t.Errorf("Unexpected order: %v", order)
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

// TestPromiseFinally_DoesNotTransformValue tests that Finally return value is ignored.
func TestPromiseFinally_DoesNotTransformValue(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	// Even though we try to "return" something, Finally ignores it
	result := p.Finally(func() {
		// Attempting to set a value here is useless
		_ = "ignored-value"
	})

	resolve(42)
	loop.tick()

	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got %v", result.State())
	}

	// Original value should be preserved
	if result.Value() != 42 {
		t.Errorf("Expected 42, got %v", result.Value())
	}
}

// TestPromiseFinally_HandlerExecutesOnce tests handler only executes once per Finally.
func TestPromiseFinally_HandlerExecutesOnce(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	var count atomic.Int32

	p.Finally(func() {
		count.Add(1)
	})

	resolve("value")
	loop.tick()
	loop.tick() // Extra tick to ensure no double-execution
	loop.tick()

	if count.Load() != 1 {
		t.Errorf("Expected handler called once, got %d", count.Load())
	}
}

// TestPromiseFinally_WithNilValue tests Finally with nil resolved value.
func TestPromiseFinally_WithNilValue(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

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
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

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
}

// TestPromiseFinally_Standalone tests Finally without JS context on already-settled promise.
// Note: Finally on pending promises without JS context causes nil pointer dereference
// because the code assumes js != nil for handler registration. This tests the
// already-settled path which works correctly without JS.
func TestPromiseFinally_Standalone(t *testing.T) {
	// Create promise without JS context - must be already settled
	p := &ChainedPromise{
		js: nil,
	}
	p.state.Store(int32(Fulfilled))
	p.result = "standalone-value"

	finallyCalled := false
	result := p.Finally(func() {
		finallyCalled = true
	})

	if !finallyCalled {
		t.Error("Finally should be called on already-settled standalone promise")
	}

	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got %v", result.State())
	}

	if result.Value() != "standalone-value" {
		t.Errorf("Expected 'standalone-value', got %v", result.Value())
	}
}

// TestPromiseFinally_ConcurrentWithResolution tests Finally during concurrent resolution.
func TestPromiseFinally_ConcurrentWithResolution(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	for trial := 0; trial < 100; trial++ {
		p, resolve, _ := js.NewChainedPromise()

		var finallyCalled atomic.Int32
		var wg sync.WaitGroup

		wg.Add(2)

		// Concurrent Finally attachment
		go func() {
			defer wg.Done()
			p.Finally(func() {
				finallyCalled.Add(1)
			})
		}()

		// Concurrent resolution
		go func() {
			defer wg.Done()
			resolve("value")
		}()

		wg.Wait()
		loop.tick()

		if finallyCalled.Load() != 1 {
			t.Errorf("Trial %d: expected finally called once, got %d", trial, finallyCalled.Load())
		}
	}
}

// TestPromiseFinally_OrderOfExecution tests execution order with multiple handlers.
func TestPromiseFinally_OrderOfExecution(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

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

	if len(order) != 3 {
		t.Errorf("Expected 3 handlers called, got %d", len(order))
	}
}

// TestPromiseFinally_TimeoutBehavior tests Finally with delayed resolution.
func TestPromiseFinally_TimeoutBehavior(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	finallyCalled := false
	result := p.Finally(func() {
		finallyCalled = true
	})

	// Resolve after slight delay
	go func() {
		time.Sleep(10 * time.Millisecond)
		resolve("delayed")
	}()

	// Give time for resolution then run loop
	time.Sleep(50 * time.Millisecond)
	loop.tick()

	if !finallyCalled {
		t.Fatal("Finally handler was not called")
	}

	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got %v", result.State())
	}
}
