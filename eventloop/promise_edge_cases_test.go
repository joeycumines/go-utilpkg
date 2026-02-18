package eventloop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestPromise_Then_EdgeCases tests edge cases in the Then method
func TestPromise_Then_EdgeCases(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Test Then with nil handlers
	p1, _, _ := js.NewChainedPromise()
	p1.Then(nil, nil)

	// Test Then after promise is already fulfilled
	p2, resolve2, _ := js.NewChainedPromise()
	resolve2("value")
	p2.Then(func(v any) any {
		return v
	}, nil)

	// Test Then after promise is already rejected
	p3, _, reject3 := js.NewChainedPromise()
	reject3("error")
	p3.Then(nil, func(r any) any {
		return r
	})

	loop.Shutdown(context.Background())
}

// TestPromise_Then_MultipleChaining tests chaining multiple Then calls
func TestPromise_Then_MultipleChaining(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	done1 := make(chan struct{})
	go func() {
		loop.Run(ctx1)
		close(done1)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()
	var wg sync.WaitGroup
	wg.Add(1)

	// Chain 10 Then calls
	chain := p
	for range 10 {
		chain = chain.Then(func(v any) any {
			return v.(string) + "-"
		}, nil)
	}

	// Add a final handler to wait for completion
	chain.Then(func(v any) any {
		wg.Done()
		return v
	}, nil)

	resolve("start")

	// Wait for chain to execute with timeout
	chainDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(chainDone)
	}()

	select {
	case <-chainDone:
		// Check final value
		if chain.Value() != "start----------" {
			t.Errorf("Expected 'start----------', got: %v", chain.Value())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for promise chain to complete")
	}

	loop.Shutdown(context.Background())
	cancel1()
	<-done1
}

// TestPromise_Then_RejectionRecovery tests Then catching rejection
func TestPromise_Then_RejectionRecovery(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan struct{})
	go func() {
		loop.Run(ctx2)
		close(done2)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, _, reject := js.NewChainedPromise()

	// Add rejection handler with synchronization
	recovered := false
	var mu sync.Mutex
	recoveryComplete := make(chan struct{})

	p.Then(nil, func(r any) any {
		mu.Lock()
		recovered = true
		mu.Unlock()
		close(recoveryComplete)
		return errors.New("recovered")
	})

	// Reject the promise
	reject("error")

	// Wait for handler with timeout
	select {
	case <-recoveryComplete:
		// Success - handler was called
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for rejection handler")
	}

	if !recovered {
		t.Error("Rejection handler should have been called")
	}

	loop.Shutdown(context.Background())
	cancel2()
	<-done2
}

// TestPromise_Then_PanicRecovery tests Then with panic in handler
func TestPromise_Then_PanicRecovery(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	// Add handler that panics
	panicValue := "test panic"
	p.Then(func(v any) any {
		panic(panicValue)
	}, nil).Then(func(v any) any {
		return v
	}, func(r any) any {
		// Catch the panic
		return r
	})

	// Resolve the promise
	resolve("value")

	// Wait for chain
	time.Sleep(50 * time.Millisecond)

	loop.Shutdown(context.Background())
}

// TestPromise_Finally_EdgeCases tests Finally edge cases
func TestPromise_Finally_EdgeCases(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx3, cancel3 := context.WithCancel(context.Background())
	done3 := make(chan struct{})
	go func() {
		loop.Run(ctx3)
		close(done3)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Test Finally on fulfilled promise with synchronization
	p1, resolve1, _ := js.NewChainedPromise()
	finallyCalled := false
	var mu sync.Mutex
	finallyComplete := make(chan struct{})
	p1.Finally(func() {
		mu.Lock()
		finallyCalled = true
		mu.Unlock()
		close(finallyComplete)
	})
	resolve1("value")

	// Wait for Finally with timeout
	select {
	case <-finallyComplete:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for Finally handler")
	}

	// Test Finally on rejected promise
	p2, _, reject2 := js.NewChainedPromise()
	finallyCalled2 := false
	var mu2 sync.Mutex
	finallyComplete2 := make(chan struct{})
	p2.Finally(func() {
		mu2.Lock()
		finallyCalled2 = true
		mu2.Unlock()
		close(finallyComplete2)
	})
	reject2("error")

	// Wait for Finally with timeout
	select {
	case <-finallyComplete2:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for Finally handler on rejection")
	}

	if !finallyCalled {
		t.Error("Finally should have been called on fulfilled promise")
	}
	if !finallyCalled2 {
		t.Error("Finally should have been called on rejected promise")
	}

	loop.Shutdown(context.Background())
	cancel3()
	<-done3
}

// TestPromise_Catch_EdgeCases tests Catch edge cases
func TestPromise_Catch_EdgeCases(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Test Catch on fulfilled promise (should not be called)
	p1, resolve1, _ := js.NewChainedPromise()
	catchCalled := false
	p1.Catch(func(r any) any {
		catchCalled = true
		return r
	})
	resolve1("value")

	// Test Catch on rejected promise
	p2, _, reject2 := js.NewChainedPromise()
	p2.Catch(func(r any) any {
		return errors.New("handled: " + r.(error).Error())
	})
	reject2("error")

	// Wait
	time.Sleep(20 * time.Millisecond)

	if catchCalled {
		t.Error("Catch should not have been called on fulfilled promise")
	}

	loop.Shutdown(context.Background())
}

// TestPromise_Resolve_MultipleTimes tests resolving promise multiple times
func TestPromise_Resolve_MultipleTimes(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	// Resolve multiple times
	resolve("first")
	resolve("second")
	resolve("third")

	// Only first resolve should take effect
	time.Sleep(20 * time.Millisecond)

	if p.Value() != "first" {
		t.Errorf("Expected 'first', got: %v", p.Value())
	}

	loop.Shutdown(context.Background())
}

// TestPromise_Reject_MultipleTimes tests rejecting promise multiple times
func TestPromise_Reject_MultipleTimes(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, _, reject := js.NewChainedPromise()

	// Reject multiple times
	reject("first")
	reject("second")
	reject("third")

	// Only first reject should take effect
	time.Sleep(20 * time.Millisecond)

	if p.Reason() != "first" {
		t.Errorf("Expected 'first', got: %v", p.Reason())
	}

	loop.Shutdown(context.Background())
}

// TestPromise_ResolveAfterReject tests resolving after rejection
func TestPromise_ResolveAfterReject(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, reject := js.NewChainedPromise()

	// Reject first
	reject("error")

	// Try to resolve after rejection (should have no effect)
	resolve("value")

	// Promise should still be rejected
	time.Sleep(20 * time.Millisecond)

	if p.State() != Rejected {
		t.Errorf("Expected Rejected, got: %v", p.State())
	}

	loop.Shutdown(context.Background())
}

// TestPromise_RejectAfterResolve tests rejecting after resolution
func TestPromise_RejectAfterResolve(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, reject := js.NewChainedPromise()

	// Resolve first
	resolve("value")

	// Try to reject after resolution (should have no effect)
	reject("error")

	// Promise should still be fulfilled
	time.Sleep(20 * time.Millisecond)

	if p.State() != Fulfilled {
		t.Errorf("Expected Fulfilled, got: %v", p.State())
	}

	loop.Shutdown(context.Background())
}

// TestPromise_Then_Concurrent tests concurrent Then calls
func TestPromise_Then_Concurrent(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	// Add handlers concurrently
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			p.Then(func(v any) any {
				return v
			}, nil)
		})
	}
	wg.Wait()

	// Resolve the promise
	resolve("value")

	// Wait for handlers
	time.Sleep(50 * time.Millisecond)

	loop.Shutdown(context.Background())
}

// TestPromise_ChainedRace tests race conditions in promise chaining
func TestPromise_ChainedRace(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, reject := js.NewChainedPromise()

	// Add handlers
	results := []string{}
	var mu sync.Mutex

	for range 5 {
		p.Then(func(v any) any {
			mu.Lock()
			results = append(results, "fulfilled")
			mu.Unlock()
			return v
		}, func(r any) any {
			mu.Lock()
			results = append(results, "rejected")
			mu.Unlock()
			return r
		})
	}

	// Resolve and reject concurrently
	go func() {
		resolve("value")
	}()
	go func() {
		reject("error")
	}()

	// Wait
	time.Sleep(50 * time.Millisecond)

	loop.Shutdown(context.Background())
}
