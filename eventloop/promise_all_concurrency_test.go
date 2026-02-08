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
	"testing"
	"time"
)

// TestPromiseAll_ConcurrentRejections tests All with concurrent rejections
func TestPromiseAll_ConcurrentRejections(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	const numPromises = 10
	promises := make([]*ChainedPromise, numPromises)
	rejectors := make([]func(Result), numPromises)

	for i := 0; i < numPromises; i++ {
		p, _, r := js.NewChainedPromise()
		promises[i] = p
		rejectors[i] = r
	}

	result := js.All(promises)

	var wg sync.WaitGroup
	var rejectionOrder []int
	var mu sync.Mutex

	// Concurrently reject all promises
	for i := 0; i < numPromises; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := errors.New("error from " + string(rune('a'+idx)))
			rejectors[idx](err)
			mu.Lock()
			rejectionOrder = append(rejectionOrder, idx)
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	loop.tick()

	// All should reject with first rejection reason
	rejected := false
	var rejectionReason string
	handlerDone := make(chan struct{})
	result.Then(nil, func(r Result) Result {
		rejected = true
		rejectionReason = r.(error).Error()
		close(handlerDone)
		return nil
	})
	loop.tick()   // Process microtasks to execute the handler
	<-handlerDone // Wait for handler to complete

	if !rejected {
		t.Error("All should have rejected")
	}

	// Rejection reason should match one of the concurrent rejections
	found := false
	for i := 0; i < numPromises; i++ {
		expected := "error from " + string(rune('a'+i))
		if rejectionReason == expected {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected rejection reason to be one of concurrent errors, got: %s", rejectionReason)
	}
}

// TestPromiseAll_ConcurrentResolutions tests All with concurrent resolutions
func TestPromiseAll_ConcurrentResolutions(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	const numPromises = 10
	promises := make([]*ChainedPromise, numPromises)
	resolvers := make([]func(Result), numPromises)

	for i := 0; i < numPromises; i++ {
		p, r, _ := js.NewChainedPromise()
		promises[i] = p
		resolvers[i] = r
	}

	result := js.All(promises)

	var wg sync.WaitGroup

	// Concurrently resolve all promises
	for i := 0; i < numPromises; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resolvers[idx]("value" + string(rune('a'+idx)))
		}(i)
	}
	wg.Wait()
	loop.tick()

	// All should resolve with all values
	var resolvedValues []Result
	handlerDone := make(chan struct{})
	result.Then(func(v Result) Result {
		resolvedValues = v.([]Result)
		close(handlerDone)
		return nil
	}, nil)
	loop.tick()   // Process microtasks to execute the handler
	<-handlerDone // Wait for handler to complete

	if len(resolvedValues) != numPromises {
		t.Errorf("Expected %d values, got %d", numPromises, len(resolvedValues))
	}

	// Verify all values are present (order preserved)
	for i := 0; i < numPromises; i++ {
		expected := "value" + string(rune('a'+i))
		if resolvedValues[i] != expected {
			t.Errorf("Expected %s at index %d, got %v", expected, i, resolvedValues[i])
		}
	}
}

// TestPromiseAll_RejectionWinsOverResolution tests that rejection wins when mixed
func TestPromiseAll_RejectionWinsOverResolution(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, _, reject := js.NewChainedPromise()
	p2, resolve, _ := js.NewChainedPromise()

	result := js.All([]*ChainedPromise{p1, p2})

	// Reject and resolve concurrently
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		reject(errors.New("rejection wins"))
	}()

	go func() {
		defer wg.Done()
		resolve("resolution")
	}()

	wg.Wait()
	loop.tick()

	// Should reject
	rejected := false
	handlerDone := make(chan struct{})
	result.Then(nil, func(r Result) Result {
		rejected = true
		close(handlerDone)
		return nil
	})
	loop.tick()   // Process microtasks to execute the handler
	<-handlerDone // Wait for handler to complete

	if !rejected {
		t.Error("All should have rejected when one promise rejects")
	}
}

// TestPromiseAll_FirstRejectionWins tests that first rejection wins
func TestPromiseAll_FirstRejectionWins(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, resolve1, _ := js.NewChainedPromise()
	p2, _, reject2 := js.NewChainedPromise()
	p3, resolve3, _ := js.NewChainedPromise()

	result := js.All([]*ChainedPromise{p1, p2, p3})

	// Reject p2 first, then resolve p3
	reject2("second promise rejects")
	resolve3("third promise resolves")

	// Resolve p1 - should be ignored since p2 already rejected
	resolve1("first promise resolves")

	// Process all pending microtasks before attaching handler
	loop.tick()

	// Should reject with p2's error
	var rejectionReason string
	handlerDone := make(chan struct{})
	result.Then(nil, func(r Result) Result {
		// Handle both error and string rejection reasons
		if err, ok := r.(error); ok {
			rejectionReason = err.Error()
		} else if str, ok := r.(string); ok {
			rejectionReason = str
		} else {
			// Fallback: use direct string conversion
			rejectionReason = r.(string)
		}
		close(handlerDone)
		return nil
	})

	// Process the microtask queue to run the handler
	loop.tick()

	// Give the handler microtask time to be scheduled and executed
	// Use select with timeout to avoid infinite deadlock
	select {
	case <-handlerDone:
		// Handler completed successfully
	case <-time.After(2 * time.Second):
		t.Fatal("Handler was not called within timeout - possible deadlock")
	}

	if rejectionReason != "second promise rejects" {
		t.Errorf("Expected 'second promise rejects', got: %s", rejectionReason)
	}
}

// TestPromiseAll_AlreadySettledPromises tests All with already-settled promises.
// This tests the fix for the race condition bug where fulfillment could win
// over rejection when promises were already settled before being passed to All().
func TestPromiseAll_AlreadySettledPromises(t *testing.T) {
	t.Run("one_rejected_among_fulfilled", func(t *testing.T) {
		loop, err := New()
		if err != nil {
			t.Fatal(err)
		}
		defer loop.Shutdown(context.Background())

		js, err := NewJS(loop)
		if err != nil {
			t.Fatal(err)
		}

		// Create pre-settled promises
		p1, resolve1, _ := js.NewChainedPromise()
		p2, _, reject2 := js.NewChainedPromise()
		p3, resolve3, _ := js.NewChainedPromise()

		// Settle them before passing to All
		resolve1("value1")
		reject2(errors.New("rejection reason"))
		resolve3("value3")
		loop.tick() // Process settlements

		result := js.All([]*ChainedPromise{p1, p2, p3})

		var settled bool
		var settledState PromiseState
		var settledReason Result
		done := make(chan struct{})

		result.Then(
			func(v Result) Result {
				settled = true
				settledState = Fulfilled
				close(done)
				return nil
			},
			func(r Result) Result {
				settled = true
				settledState = Rejected
				settledReason = r
				close(done)
				return nil
			},
		)

		// Process microtasks to settle
		loop.tick()
		loop.tick()
		<-done

		if !settled {
			t.Fatal("result promise was not settled")
		}
		if settledState != Rejected {
			t.Errorf("expected Rejected, got %v (fulfillment won over rejection)", settledState)
		}
		if settledReason == nil {
			t.Error("expected rejection reason, got nil")
		}
	})

	t.Run("all_fulfilled", func(t *testing.T) {
		loop, err := New()
		if err != nil {
			t.Fatal(err)
		}
		defer loop.Shutdown(context.Background())

		js, err := NewJS(loop)
		if err != nil {
			t.Fatal(err)
		}

		p1, resolve1, _ := js.NewChainedPromise()
		p2, resolve2, _ := js.NewChainedPromise()

		resolve1("a")
		resolve2("b")
		loop.tick()

		result := js.All([]*ChainedPromise{p1, p2})

		var settled bool
		var settledState PromiseState
		var settledValue Result
		done := make(chan struct{})

		result.Then(
			func(v Result) Result {
				settled = true
				settledState = Fulfilled
				settledValue = v
				close(done)
				return nil
			},
			func(r Result) Result {
				settled = true
				settledState = Rejected
				close(done)
				return nil
			},
		)

		loop.tick()
		loop.tick()
		<-done

		if !settled {
			t.Fatal("result promise was not settled")
		}
		if settledState != Fulfilled {
			t.Errorf("expected Fulfilled, got %v", settledState)
		}
		values, ok := settledValue.([]Result)
		if !ok {
			t.Fatalf("expected []Result, got %T", settledValue)
		}
		if len(values) != 2 || values[0] != Result("a") || values[1] != Result("b") {
			t.Errorf("expected [a b], got %v", values)
		}
	})

	t.Run("first_rejected", func(t *testing.T) {
		loop, err := New()
		if err != nil {
			t.Fatal(err)
		}
		defer loop.Shutdown(context.Background())

		js, err := NewJS(loop)
		if err != nil {
			t.Fatal(err)
		}

		p1, _, reject1 := js.NewChainedPromise()
		p2, resolve2, _ := js.NewChainedPromise()

		reject1(errors.New("first rejects"))
		resolve2("second fulfills")
		loop.tick()

		result := js.All([]*ChainedPromise{p1, p2})

		var settledState PromiseState
		done := make(chan struct{})

		result.Then(
			func(v Result) Result {
				settledState = Fulfilled
				close(done)
				return nil
			},
			func(r Result) Result {
				settledState = Rejected
				close(done)
				return nil
			},
		)

		loop.tick()
		loop.tick()
		<-done

		if settledState != Rejected {
			t.Errorf("expected Rejected when first promise is rejected, got %v", settledState)
		}
	})

	t.Run("all_rejected", func(t *testing.T) {
		loop, err := New()
		if err != nil {
			t.Fatal(err)
		}
		defer loop.Shutdown(context.Background())

		js, err := NewJS(loop)
		if err != nil {
			t.Fatal(err)
		}

		p1, _, reject1 := js.NewChainedPromise()
		p2, _, reject2 := js.NewChainedPromise()

		reject1(errors.New("r1"))
		reject2(errors.New("r2"))
		loop.tick()

		result := js.All([]*ChainedPromise{p1, p2})

		var settledState PromiseState
		done := make(chan struct{})

		result.Then(
			func(v Result) Result {
				settledState = Fulfilled
				close(done)
				return nil
			},
			func(r Result) Result {
				settledState = Rejected
				close(done)
				return nil
			},
		)

		loop.tick()
		loop.tick()
		<-done

		if settledState != Rejected {
			t.Errorf("expected Rejected when all promises rejected, got %v", settledState)
		}
	})
}

// TestPromiseAll_OnePromise tests All with single promise
func TestPromiseAll_OnePromise(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, resolve1, _ := js.NewChainedPromise()

	result := js.All([]*ChainedPromise{p1})

	var values []Result
	result.Then(func(v Result) Result {
		values = v.([]Result)
		return nil
	}, nil)

	resolve1("single value")
	loop.tick()

	if len(values) != 1 {
		t.Errorf("Expected 1 value, got %d", len(values))
	}
	if values[0] != "single value" {
		t.Errorf("Expected 'single value', got %v", values[0])
	}
}

// TestPromiseAll_LargeNumberOfPromises tests All with many promises
func TestPromiseAll_LargeNumberOfPromises(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	const numPromises = 100
	promises := make([]*ChainedPromise, numPromises)
	resolvers := make([]func(Result), numPromises)

	for i := 0; i < numPromises; i++ {
		p, r, _ := js.NewChainedPromise()
		promises[i] = p
		resolvers[i] = r
	}

	result := js.All(promises)

	// Resolve all
	for i := 0; i < numPromises; i++ {
		resolvers[i]("value" + string(rune('a'+i%26)))
	}
	loop.tick()

	// Verify all resolved
	var values []Result
	handlerDone := make(chan struct{})
	result.Then(func(v Result) Result {
		values = v.([]Result)
		close(handlerDone)
		return nil
	}, nil)

	// Process the handler microtask
	loop.tick()

	// Wait for handler with timeout
	select {
	case <-handlerDone:
		// Handler completed
	case <-time.After(2 * time.Second):
		t.Fatal("Handler was not called within timeout")
	}

	if len(values) != numPromises {
		t.Errorf("Expected %d values, got %d", numPromises, len(values))
	}
}

// TestPromiseAll_ResolveAfterRejection tests that resolution after rejection is ignored
func TestPromiseAll_ResolveAfterRejection(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, resolve1, _ := js.NewChainedPromise()
	p2, _, reject2 := js.NewChainedPromise()

	result := js.All([]*ChainedPromise{p1, p2})

	// Reject first
	reject2(errors.New("rejection"))
	loop.tick()

	// Resolve after rejection - should be ignored
	resolve1("late resolution")
	loop.tick()

	// Should still be rejected
	rejected := false
	handlerDone := make(chan struct{})
	result.Then(nil, func(r Result) Result {
		rejected = true
		close(handlerDone)
		return nil
	})

	// Process the handler microtask
	loop.tick()

	// Wait for handler with timeout
	select {
	case <-handlerDone:
		// Handler completed
	case <-time.After(2 * time.Second):
		t.Fatal("Handler was not called within timeout")
	}

	if !rejected {
		t.Error("All should remain rejected after resolution attempt")
	}
}

// TestPromiseAll_TerminatedLoop tests All on terminated loop
func TestPromiseAll_TerminatedLoop(t *testing.T) {
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
	p1, _, _ := js.NewChainedPromise()

	result := js.All([]*ChainedPromise{p1})

	// Should remain pending or reject gracefully
	if result.State() == Resolved {
		t.Error("All should not resolve on terminated loop")
	}

	loop.Shutdown(context.Background())
}

// TestPromiseAll_PanicInHandler tests All with panic in resolve handler
func TestPromiseAll_PanicInHandler(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, resolve1, _ := js.NewChainedPromise()

	result := js.All([]*ChainedPromise{p1})

	// Handler that panics
	result.Then(func(v Result) Result {
		panic("handler panic")
	}, nil)

	// Resolve should not crash
	resolve1("value")
	loop.tick()
}

// TestPromiseAll_ChainedPromises tests All with chained promises
func TestPromiseAll_ChainedPromises(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, resolve1, _ := js.NewChainedPromise()
	p2, resolve2, _ := js.NewChainedPromise()

	// Create chains
	chained1 := p1.Then(func(v Result) Result {
		return v.(string) + "-chained1"
	}, nil)
	chained2 := p2.Then(func(v Result) Result {
		return v.(string) + "-chained2"
	}, nil)

	result := js.All([]*ChainedPromise{chained1, chained2})

	var values []Result
	result.Then(func(v Result) Result {
		values = v.([]Result)
		return nil
	}, nil)

	resolve1("value1")
	resolve2("value2")
	loop.tick()

	if len(values) != 2 {
		t.Errorf("Expected 2 values, got %d", len(values))
	}

	if values[0] != "value1-chained1" {
		t.Errorf("Expected 'value1-chained1', got %v", values[0])
	}
	if values[1] != "value2-chained2" {
		t.Errorf("Expected 'value2-chained2', got %v", values[1])
	}
}

// TestPromiseAll_MixedImmediateAndPending tests All with mixed immediate/pending
func TestPromiseAll_MixedImmediateAndPending(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Create immediate resolution
	p1, resolve1, _ := js.NewChainedPromise()
	resolve1("immediate")

	// Create pending promise
	p2, resolve2, _ := js.NewChainedPromise()

	result := js.All([]*ChainedPromise{p1, p2})

	var values []Result
	result.Then(func(v Result) Result {
		values = v.([]Result)
		return nil
	}, nil)

	// p1 already resolved, p2 pending
	loop.tick()

	if len(values) != 0 {
		t.Error("Should not resolve until all complete")
	}

	// Resolve p2
	resolve2("pending")
	loop.tick()

	if len(values) != 2 {
		t.Errorf("Expected 2 values after p2 resolves, got %d", len(values))
	}
}

// TestPromiseAll_WithContextCancellation tests All with context cancellation
func TestPromiseAll_WithContextCancellation(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	_, cancel := context.WithCancel(context.Background())

	p1, resolve1, _ := js.NewChainedPromise()
	p2, resolve2, _ := js.NewChainedPromise()

	result := js.All([]*ChainedPromise{p1, p2})

	// Cancel before resolution
	cancel()

	// Resolve after cancellation
	resolve1("value1")
	resolve2("value2")
	loop.tick()

	// All may reject or remain pending - behavior depends on implementation
	// Just verify it doesn't crash
	_ = result
}

// TestPromiseAll_ConcurrentResolveAndReject tests race between resolve and reject
func TestPromiseAll_ConcurrentResolveAndReject(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, _, reject := js.NewChainedPromise()
	p2, resolve, _ := js.NewChainedPromise()

	result := js.All([]*ChainedPromise{p1, p2})

	// Attempt to resolve and reject concurrently
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		reject(errors.New("rejection"))
	}()

	go func() {
		defer wg.Done()
		resolve("resolution")
	}()

	wg.Wait()
	loop.tick()

	// Should reject (rejection wins)
	rejected := false
	handlerDone := make(chan struct{})
	result.Then(nil, func(r Result) Result {
		rejected = true
		close(handlerDone)
		return nil
	})

	// Process the handler microtask
	loop.tick()

	// Wait for handler with timeout
	select {
	case <-handlerDone:
		// Handler completed
	case <-time.After(2 * time.Second):
		t.Fatal("Handler was not called within timeout")
	}

	if !rejected {
		t.Error("All should reject when concurrent rejection occurs")
	}
}

// TestPromiseAll_AllReject tests All where all promises reject
func TestPromiseAll_AllReject(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, _, reject1 := js.NewChainedPromise()
	p2, _, reject2 := js.NewChainedPromise()
	p3, _, reject3 := js.NewChainedPromise()

	result := js.All([]*ChainedPromise{p1, p2, p3})

	// All reject
	reject1(errors.New("error1"))
	reject2(errors.New("error2"))
	reject3(errors.New("error3"))
	loop.tick()

	// Should reject with first error
	var rejectionReason string
	handlerDone := make(chan struct{})
	result.Then(nil, func(r Result) Result {
		if err, ok := r.(error); ok {
			rejectionReason = err.Error()
		} else if str, ok := r.(string); ok {
			rejectionReason = str
		}
		close(handlerDone)
		return nil
	})

	// Process the handler microtask
	loop.tick()

	// Wait for handler with timeout
	select {
	case <-handlerDone:
		// Handler completed
	case <-time.After(2 * time.Second):
		t.Fatal("Handler was not called within timeout")
	}

	if rejectionReason != "error1" {
		t.Errorf("Expected 'error1' (first rejection), got: %s", rejectionReason)
	}
}
