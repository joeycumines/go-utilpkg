package eventloop

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestPromiseAny_EmptyArray tests Promise.Any with empty array
func TestPromiseAny_EmptyArray(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	result := js.Any([]*ChainedPromise{})

	// Should reject with AggregateError
	handlerDone := make(chan struct{})
	rejected := false
	result.Then(nil, func(r any) any {
		rejected = true
		close(handlerDone)
		return nil
	})
	loop.tick()   // Process the handler microtask
	<-handlerDone // Wait for handler to complete

	if !rejected {
		t.Error("Any with empty array should reject")
	}
}

// TestPromiseAny_OnePromiseFulfills tests Promise.Any with one fulfilling promise
func TestPromiseAny_OnePromiseFulfills(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, _, _ := js.NewChainedPromise()
	p2, resolve2, _ := js.NewChainedPromise()

	result := js.Any([]*ChainedPromise{p1, p2})

	var fulfilledValue string
	result.Then(func(v any) any {
		fulfilledValue = v.(string)
		return nil
	}, nil)

	resolve2("winner")
	loop.tick()

	if fulfilledValue != "winner" {
		t.Errorf("Expected 'winner', got '%s'", fulfilledValue)
	}
}

// TestPromiseAny_FirstToFulfillWins tests Promise.Any with multiple fulfilling promises
func TestPromiseAny_FirstToFulfillWins(t *testing.T) {
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
	p3, resolve3, _ := js.NewChainedPromise()

	result := js.Any([]*ChainedPromise{p1, p2, p3})

	var winner string
	result.Then(func(v any) any {
		winner = v.(string)
		return nil
	}, nil)

	resolve2("second wins")
	loop.tick()

	if winner != "second wins" {
		t.Errorf("Expected 'second wins', got '%s'", winner)
	}

	// Others resolve - should be ignored
	resolve1("first")
	resolve3("third")
	loop.tick()

	if winner != "second wins" {
		t.Errorf("Expected 'second wins' (unchanged), got '%s'", winner)
	}
}

// TestPromiseAny_AllReject tests Promise.Any when all promises reject
func TestPromiseAny_AllReject(t *testing.T) {
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

	result := js.Any([]*ChainedPromise{p1, p2, p3})

	// All reject
	reject1(errors.New("error1"))
	reject2(errors.New("error2"))
	reject3(errors.New("error3"))
	loop.tick()

	// Should reject with AggregateError
	handlerDone := make(chan struct{})
	rejected := false
	result.Then(nil, func(r any) any {
		rejected = true
		close(handlerDone)
		return nil
	})
	loop.tick()   // Process the handler microtask
	<-handlerDone // Wait for handler to complete

	if !rejected {
		t.Error("Any with all rejections should reject")
	}
}

// TestPromiseAny_OneRejectsOthersFulfill tests Promise.Any with mix of rejection and fulfillment
func TestPromiseAny_OneRejectsOthersFulfill(t *testing.T) {
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
	p3, resolve3, _ := js.NewChainedPromise()

	result := js.Any([]*ChainedPromise{p1, p2, p3})

	var winner string
	result.Then(func(v any) any {
		winner = v.(string)
		return nil
	}, nil)

	// Reject and resolve concurrently
	reject1(errors.New("error"))
	resolve2("fulfillment wins")
	loop.tick()

	if winner != "fulfillment wins" {
		t.Errorf("Expected 'fulfillment wins', got '%s'", winner)
	}

	// Resolve p3 - should be ignored
	resolve3("ignored")
	loop.tick()

	if winner != "fulfillment wins" {
		t.Errorf("Expected 'fulfillment wins' (unchanged), got '%s'", winner)
	}
}

// TestPromiseAny_ConcurrentRejections tests Promise.Any with concurrent rejections
func TestPromiseAny_ConcurrentRejections(t *testing.T) {
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
	rejectors := make([]func(any), numPromises)

	for i := range numPromises {
		p, _, r := js.NewChainedPromise()
		promises[i] = p
		rejectors[i] = r
	}

	result := js.Any(promises)

	// All reject concurrently
	for i := range numPromises {
		rejectors[i](errors.New("error" + string(rune('a'+i))))
	}
	loop.tick()

	// Should reject with AggregateError
	handlerDone := make(chan struct{})
	rejected := false
	result.Then(nil, func(r any) any {
		rejected = true
		close(handlerDone)
		return nil
	})
	loop.tick()   // Process the handler microtask
	<-handlerDone // Wait for handler to complete

	if !rejected {
		t.Error("Any with all concurrent rejections should reject")
	}
}

// TestPromiseAny_TerminatedLoop tests Promise.Any on terminated loop
func TestPromiseAny_TerminatedLoop(t *testing.T) {
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

	result := js.Any([]*ChainedPromise{p1})

	// Should handle gracefully
	if result.State() == Resolved {
		t.Error("Any should not resolve on terminated loop")
	}

	loop.Shutdown(context.Background())
}

// TestPromiseAny_PanicInHandler tests Promise.Any with panic in handler
func TestPromiseAny_PanicInHandler(t *testing.T) {
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

	result := js.Any([]*ChainedPromise{p1})

	// Handler that panics
	result.Then(func(v any) any {
		panic("handler panic")
	}, nil)

	// Should not crash
	resolve1("value")
	loop.tick()
}

// TestPromiseAny_SinglePromise tests Promise.Any with single promise
func TestPromiseAny_SinglePromise(t *testing.T) {
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

	result := js.Any([]*ChainedPromise{p1})

	var value string
	result.Then(func(v any) any {
		value = v.(string)
		return nil
	}, nil)

	resolve1("single")
	loop.tick()

	if value != "single" {
		t.Errorf("Expected 'single', got '%s'", value)
	}
}

// TestPromiseAny_AlreadyFulfilledPromise tests Any with already-fulfilled promise
func TestPromiseAny_AlreadyFulfilledPromise(t *testing.T) {
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
	resolve1("already fulfilled")

	p2, _, _ := js.NewChainedPromise()

	result := js.Any([]*ChainedPromise{p1, p2})

	var value string
	result.Then(func(v any) any {
		value = v.(string)
		return nil
	}, nil)

	loop.tick()

	if value != "already fulfilled" {
		t.Errorf("Expected 'already fulfilled', got '%s'", value)
	}
}

// TestPromiseAny_WithThenHandler tests Any with immediate then handler attachment
func TestPromiseAny_WithThenHandler(t *testing.T) {
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

	result := js.Any([]*ChainedPromise{p1})

	var gotResult bool
	var value string

	result.Then(func(v any) any {
		gotResult = true
		value = v.(string)
		return nil
	}, nil)

	resolve1("winner")
	loop.tick()

	if !gotResult {
		t.Error("Then handler should have been called")
	}
	if value != "winner" {
		t.Errorf("Expected 'winner', got '%s'", value)
	}
}

// TestPromiseAny_ChainedPromises tests Any with chained promises
func TestPromiseAny_ChainedPromises(t *testing.T) {
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
	p2, _, _ := js.NewChainedPromise()

	// Create chains
	chained1 := p1.Then(func(v any) any {
		return v.(string) + "-chained1"
	}, nil)
	chained2 := p2.Then(nil, func(r any) any {
		return r.(error).Error() + "-chained2"
	})

	result := js.Any([]*ChainedPromise{chained1, chained2})

	var value string
	result.Then(func(v any) any {
		value = v.(string)
		return nil
	}, nil)

	resolve1("value")
	loop.tick()

	if value != "value-chained1" {
		t.Errorf("Expected 'value-chained1', got '%s'", value)
	}
}

// TestPromiseAny_AlreadyRejectedPromises tests Any with already-rejected promises
func TestPromiseAny_AlreadyRejectedPromises(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Create and reject promises
	p1, _, reject1 := js.NewChainedPromise()
	p2, _, reject2 := js.NewChainedPromise()

	reject1(errors.New("error1"))
	reject2(errors.New("error2"))

	// Any with already-rejected promises
	result := js.Any([]*ChainedPromise{p1, p2})

	// Should reject with AggregateError
	rejected := false
	result.Then(nil, func(r any) any {
		rejected = true
		return nil
	})

	loop.tick()

	if !rejected {
		t.Error("Any with all rejected promises should reject")
	}
}

// TestPromiseAny_PromiseAlreadySettled tests Any with mixed settled promises
func TestPromiseAny_PromiseAlreadySettled(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Create already-fulfilled promise
	p1, resolve1, _ := js.NewChainedPromise()
	resolve1("fulfilled")

	// Create already-rejected promise
	p2, _, reject2 := js.NewChainedPromise()
	reject2(errors.New("error"))

	// Create pending promise
	p3, _, _ := js.NewChainedPromise()

	result := js.Any([]*ChainedPromise{p1, p2, p3})

	// Should resolve with p1 immediately
	var value string
	result.Then(func(v any) any {
		value = v.(string)
		return nil
	}, nil)

	loop.tick()

	if value != "fulfilled" {
		t.Errorf("Expected 'fulfilled', got '%s'", value)
	}
}

// TestPromiseAny_LargeNumberOfPromises tests Any with many promises
func TestPromiseAny_LargeNumberOfPromises(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	const numPromises = 50
	promises := make([]*ChainedPromise, numPromises)
	resolvers := make([]func(any), numPromises)

	for i := range numPromises {
		p, r, _ := js.NewChainedPromise()
		promises[i] = p
		resolvers[i] = r
	}

	result := js.Any(promises)

	var winner string
	result.Then(func(v any) any {
		winner = v.(string)
		return nil
	}, nil)

	// Resolve the last promise
	resolvers[numPromises-1]("last one wins")
	loop.tick()

	if winner != "last one wins" {
		t.Errorf("Expected 'last one wins', got '%s'", winner)
	}
}
