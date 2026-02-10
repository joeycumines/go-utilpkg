package eventloop

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestPromiseCatch_OnRejectedPromise tests Catch on a rejected promise
func TestPromiseCatch_OnRejectedPromise(t *testing.T) {
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

	result := p.Catch(func(r Result) Result {
		return errors.New("recovered: " + r.(error).Error())
	})

	// Reject the promise
	reject(errors.New("original error"))
	loop.tick()

	// Catch should have recovered
	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got: %v", result.State())
	}

	reason, ok := result.Value().(error)
	if !ok {
		t.Fatal("Expected error result")
	}
	if reason.Error() != "recovered: original error" {
		t.Errorf("Expected 'recovered: original error', got: %s", reason.Error())
	}
}

// TestPromiseCatch_OnFulfilledPromise tests Catch on a fulfilled promise (no-op)
func TestPromiseCatch_OnFulfilledPromise(t *testing.T) {
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

	result := p.Catch(func(r Result) Result {
		return errors.New("should not be called")
	})

	// Resolve the promise
	resolve("value")
	loop.tick()

	// Catch should be no-op, promise remains fulfilled
	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got: %v", result.State())
	}
	if result.Value() != "value" {
		t.Errorf("Expected 'value', got: %v", result.Value())
	}
}

// TestPromiseCatch_HandlerReturningValue tests Catch handler returning a value
func TestPromiseCatch_HandlerReturningValue(t *testing.T) {
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

	result := p.Catch(func(r Result) Result {
		return "recovered value"
	})

	reject(errors.New("error"))
	loop.tick()

	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got: %v", result.State())
	}
	if result.Value() != "recovered value" {
		t.Errorf("Expected 'recovered value', got: %v", result.Value())
	}
}

// TestPromiseCatch_HandlerReThrowing tests Catch handler re-throwing
func TestPromiseCatch_HandlerReThrowing(t *testing.T) {
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

	result := p.Catch(func(r Result) Result {
		panic("re-throwing")
	})

	reject(errors.New("original"))
	loop.tick()

	// Should remain rejected
	if result.State() != Rejected {
		t.Errorf("Expected Rejected, got: %v", result.State())
	}
}

// TestPromiseCatch_InPromiseChain tests Catch in promise chain
func TestPromiseCatch_InPromiseChain(t *testing.T) {
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

	// Chain: resolve -> then -> then (rejects) -> catch
	result := p.
		Then(func(v Result) Result {
			return v.(string) + "-1"
		}, nil).
		Then(func(v Result) Result {
			panic("error in chain")
		}, nil).
		Catch(func(r Result) Result {
			// Panic is wrapped in PanicError
			panicErr, ok := r.(PanicError)
			if !ok {
				return "error: expected PanicError"
			}
			return "recovered: " + panicErr.Value.(string)
		})

	resolve("start")
	loop.tick()

	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got: %v", result.State())
	}
	if result.Value() != "recovered: error in chain" {
		t.Errorf("Expected 'recovered: error in chain', got: %v", result.Value())
	}
}

// TestPromiseCatch_WithNilHandler tests Catch with nil handler
func TestPromiseCatch_WithNilHandler(t *testing.T) {
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

	// Catch with nil handler - should pass through
	result := p.Catch(nil)

	reject(errors.New("error"))
	loop.tick()

	// Should still be rejected
	if result.State() != Rejected {
		t.Errorf("Expected Rejected, got: %v", result.State())
	}
}

// TestPromiseCatch_TerminatedLoop tests Catch on terminated loop
func TestPromiseCatch_TerminatedLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done

	js, _ := NewJS(loop)
	p, _, _ := js.NewChainedPromise()

	result := p.Catch(func(r Result) Result {
		return r
	})

	// Should handle gracefully
	if result.State() == Fulfilled {
		t.Error("Catch should not fulfill on terminated loop")
	}

	loop.Shutdown(context.Background())
}

// TestPromiseCatch_ChainedCatch tests multiple Catch in chain
func TestPromiseCatch_ChainedCatch(t *testing.T) {
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

	// Chain of catches
	result := p.
		Catch(func(r Result) Result {
			return "first catch: " + r.(error).Error()
		}).
		Catch(func(r Result) Result {
			return "second catch: " + r.(error).Error()
		})

	reject(errors.New("error"))
	loop.tick()

	// Only first catch should be called
	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got: %v", result.State())
	}
	if result.Value() != "first catch: error" {
		t.Errorf("Expected 'first catch: error', got: %v", result.Value())
	}
}

// TestPromiseCatch_ConcurrentCatch tests concurrent Catch attachments
func TestPromiseCatch_ConcurrentCatch(t *testing.T) {
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

	// Attach multiple catches concurrently
	result1 := p.Catch(func(r Result) Result {
		return "catch1"
	})
	result2 := p.Catch(func(r Result) Result {
		return "catch2"
	})

	reject(errors.New("error"))
	loop.tick()

	// Both should recover (independent promises)
	if result1.State() != Fulfilled {
		t.Errorf("result1: Expected Fulfilled, got: %v", result1.State())
	}
	if result2.State() != Fulfilled {
		t.Errorf("result2: Expected Fulfilled, got: %v", result2.State())
	}
}

// TestPromiseCatch_PanicInHandler tests Catch with panic in handler
func TestPromiseCatch_PanicInHandler(t *testing.T) {
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

	result := p.Catch(func(r Result) Result {
		panic("catch panic")
	})

	reject(errors.New("original"))
	loop.tick()

	// Should remain rejected due to panic
	if result.State() != Rejected {
		t.Errorf("Expected Rejected after panic, got: %v", result.State())
	}
}

// ============================================================================
// Finally Tests
// ============================================================================

// TestPromiseFinally_OnResolvedPromise tests Finally on a resolved promise
func TestPromiseFinally_OnResolvedPromise(t *testing.T) {
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

	// Resolve the promise
	resolve("value")
	loop.tick()

	// Finally should be called
	if !finallyCalled {
		t.Error("Finally callback should have been called")
	}

	// Promise should remain resolved
	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got: %v", result.State())
	}
	if result.Value() != "value" {
		t.Errorf("Expected 'value', got: %v", result.Value())
	}
}

// TestPromiseFinally_OnRejectedPromise tests Finally on a rejected promise
func TestPromiseFinally_OnRejectedPromise(t *testing.T) {
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

	// Reject the promise
	reject(errors.New("error"))
	loop.tick()

	// Finally should be called
	if !finallyCalled {
		t.Error("Finally callback should have been called")
	}

	// Promise should remain rejected
	if result.State() != Rejected {
		t.Errorf("Expected Rejected, got: %v", result.State())
	}
}

// TestPromiseFinally_HandlerReturnValueIgnored tests Finally handler return value
func TestPromiseFinally_HandlerReturnValueIgnored(t *testing.T) {
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

	// Finally returning a value (should be ignored)
	result := p.Finally(func() {
		_ = "ignored return"
	})

	resolve("original")
	loop.tick()

	// Should keep original value
	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got: %v", result.State())
	}
	if result.Value() != "original" {
		t.Errorf("Expected 'original', got: %v", result.Value())
	}
}

// TestPromiseFinally_InPromiseChain tests Finally in promise chain
func TestPromiseFinally_InPromiseChain(t *testing.T) {
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

	finallyOrder := []string{}

	result := p.
		Then(func(v Result) Result {
			return v.(string) + "-1"
		}, nil).
		Finally(func() {
			finallyOrder = append(finallyOrder, "finally")
		}).
		Then(func(v Result) Result {
			return v.(string) + "-2"
		}, nil)

	resolve("start")
	loop.tick()

	// Finally should have been called
	if len(finallyOrder) != 1 {
		t.Errorf("Expected finally to be called once, got: %d", len(finallyOrder))
	}

	// Result should have both transformations
	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got: %v", result.State())
	}
	if result.Value() != "start-1-2" {
		t.Errorf("Expected 'start-1-2', got: %v", result.Value())
	}
}

// TestPromiseFinally_WithNilHandler tests Finally with nil handler
func TestPromiseFinally_WithNilHandler(t *testing.T) {
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

	// Finally with nil handler - should pass through
	result := p.Finally(nil)

	resolve("value")
	loop.tick()

	// Should remain resolved
	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got: %v", result.State())
	}
}

// TestPromiseFinally_TerminatedLoop tests Finally on terminated loop
func TestPromiseFinally_TerminatedLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done

	js, _ := NewJS(loop)
	p, _, _ := js.NewChainedPromise()

	result := p.Finally(func() {
		// This won't be called since loop is terminated
	})

	// Should handle gracefully
	if result.State() == Rejected {
		t.Error("Finally should not reject on terminated loop")
	}

	loop.Shutdown(context.Background())
}

// TestPromiseFinally_ConcurrentFinally tests concurrent Finally attachments
func TestPromiseFinally_ConcurrentFinally(t *testing.T) {
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

	finally1Called := false
	finally2Called := false

	result1 := p.Finally(func() {
		finally1Called = true
	})
	result2 := p.Finally(func() {
		finally2Called = true
	})

	resolve("value")
	loop.tick()

	// Both should be called
	if !finally1Called {
		t.Error("First finally should be called")
	}
	if !finally2Called {
		t.Error("Second finally should be called")
	}

	// Both should pass through
	if result1.State() != Fulfilled || result2.State() != Fulfilled {
		t.Errorf("Both should remain Fulfilled")
	}
}

// TestPromiseFinally_PanicInHandler tests Finally with panic in handler
func TestPromiseFinally_PanicInHandler(t *testing.T) {
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

	result := p.Finally(func() {
		panic("finally panic")
	})

	resolve("value")
	loop.tick()

	// Should remain resolved (panic in finally doesn't affect settlement)
	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled after finally panic, got: %v", result.State())
	}
}

// TestPromiseFinally_CatchAndFinally tests Catch followed by Finally
func TestPromiseFinally_CatchAndFinally(t *testing.T) {
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

	catchCalled := false
	finallyCalled := false

	result := p.
		Catch(func(r Result) Result {
			catchCalled = true
			return "recovered"
		}).
		Finally(func() {
			finallyCalled = true
		})

	reject(errors.New("error"))
	loop.tick()

	// Both should be called
	if !catchCalled {
		t.Error("Catch should be called")
	}
	if !finallyCalled {
		t.Error("Finally should be called")
	}

	// Result should be fulfilled from catch
	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got: %v", result.State())
	}
}
