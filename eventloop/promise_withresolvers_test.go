package eventloop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestWithResolvers_Basic tests basic WithResolvers usage.
func TestWithResolvers_Basic(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	resolvers := js.WithResolvers()

	if resolvers.Promise == nil {
		t.Fatal("expected promise to be non-nil")
	}
	if resolvers.Resolve == nil {
		t.Fatal("expected resolve to be non-nil")
	}
	if resolvers.Reject == nil {
		t.Fatal("expected reject to be non-nil")
	}
}

// TestWithResolvers_Resolve tests resolving via WithResolvers.
func TestWithResolvers_Resolve(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	resolvers := js.WithResolvers()

	ch := resolvers.Promise.ToChannel()

	resolvers.Resolve("test value")

	select {
	case val := <-ch:
		if val != "test value" {
			t.Errorf("expected 'test value', got: %v", val)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for resolution")
	}
}

// TestWithResolvers_Reject tests rejecting via WithResolvers.
func TestWithResolvers_Reject(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	resolvers := js.WithResolvers()

	testErr := errors.New("test error")
	ch := resolvers.Promise.ToChannel()

	resolvers.Reject(testErr)

	select {
	case got := <-ch:
		if got != testErr {
			t.Errorf("expected test error, got: %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for rejection")
	}
}

// TestWithResolvers_PendingInitially tests that promise starts pending.
func TestWithResolvers_PendingInitially(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	resolvers := js.WithResolvers()

	if resolvers.Promise.State() != Pending {
		t.Errorf("expected pending state, got: %v", resolvers.Promise.State())
	}
}

// TestWithResolvers_ResolveOnlyOnce tests that resolve is idempotent.
func TestWithResolvers_ResolveOnlyOnce(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	resolvers := js.WithResolvers()

	resolvers.Resolve("first")
	resolvers.Resolve("second") // Should be ignored

	if resolvers.Promise.State() != Fulfilled {
		t.Errorf("expected Fulfilled state, got: %v", resolvers.Promise.State())
	}

	if resolvers.Promise.Value() != "first" {
		t.Errorf("expected 'first', got: %v", resolvers.Promise.Value())
	}
}

// TestWithResolvers_RejectOnlyOnce tests that reject is idempotent.
func TestWithResolvers_RejectOnlyOnce(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	resolvers := js.WithResolvers()

	err1 := errors.New("first")
	err2 := errors.New("second")

	resolvers.Reject(err1)
	resolvers.Reject(err2) // Should be ignored

	if resolvers.Promise.State() != Rejected {
		t.Errorf("expected Rejected state, got: %v", resolvers.Promise.State())
	}

	if resolvers.Promise.Reason() != err1 {
		t.Errorf("expected first error, got: %v", resolvers.Promise.Reason())
	}
}

// TestWithResolvers_ResolveAfterReject tests that resolve after reject is ignored.
func TestWithResolvers_ResolveAfterReject(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	resolvers := js.WithResolvers()

	testErr := errors.New("error")
	resolvers.Reject(testErr)
	resolvers.Resolve("value") // Should be ignored

	if resolvers.Promise.State() != Rejected {
		t.Errorf("expected Rejected state, got: %v", resolvers.Promise.State())
	}

	if resolvers.Promise.Reason() != testErr {
		t.Errorf("expected test error, got: %v", resolvers.Promise.Reason())
	}
}

// TestWithResolvers_RejectAfterResolve tests that reject after resolve is ignored.
func TestWithResolvers_RejectAfterResolve(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	resolvers := js.WithResolvers()

	resolvers.Resolve("value")
	resolvers.Reject(errors.New("error")) // Should be ignored

	if resolvers.Promise.State() != Fulfilled {
		t.Errorf("expected Fulfilled state, got: %v", resolvers.Promise.State())
	}

	if resolvers.Promise.Value() != "value" {
		t.Errorf("expected 'value', got: %v", resolvers.Promise.Value())
	}
}

// TestWithResolvers_ConcurrentResolve tests concurrent resolve calls.
func TestWithResolvers_ConcurrentResolve(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	resolvers := js.WithResolvers()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			resolvers.Resolve(val)
		}(i)
	}

	wg.Wait()

	// Should be fulfilled with exactly one value
	if resolvers.Promise.State() != Fulfilled {
		t.Errorf("expected Fulfilled state, got: %v", resolvers.Promise.State())
	}

	// Value should be one of 0-99
	val, ok := resolvers.Promise.Value().(int)
	if !ok {
		t.Errorf("expected int value, got: %T", resolvers.Promise.Value())
	}
	if val < 0 || val >= 100 {
		t.Errorf("expected value 0-99, got: %d", val)
	}
}

// TestWithResolvers_ConcurrentReject tests concurrent reject calls.
func TestWithResolvers_ConcurrentReject(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	resolvers := js.WithResolvers()

	var wg sync.WaitGroup
	errs := make([]error, 100)
	for i := 0; i < 100; i++ {
		errs[i] = errors.New("error")
		wg.Add(1)
		go func(e error) {
			defer wg.Done()
			resolvers.Reject(e)
		}(errs[i])
	}

	wg.Wait()

	// Should be rejected with exactly one reason
	if resolvers.Promise.State() != Rejected {
		t.Errorf("expected Rejected state, got: %v", resolvers.Promise.State())
	}
}

// TestWithResolvers_NilValue tests resolving with nil.
func TestWithResolvers_NilValue(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	resolvers := js.WithResolvers()
	resolvers.Resolve(nil)

	if resolvers.Promise.State() != Fulfilled {
		t.Errorf("expected Fulfilled state, got: %v", resolvers.Promise.State())
	}

	if resolvers.Promise.Value() != nil {
		t.Errorf("expected nil value, got: %v", resolvers.Promise.Value())
	}
}

// TestWithResolvers_NilReason tests rejecting with nil.
func TestWithResolvers_NilReason(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	resolvers := js.WithResolvers()
	resolvers.Reject(nil)

	if resolvers.Promise.State() != Rejected {
		t.Errorf("expected Rejected state, got: %v", resolvers.Promise.State())
	}

	if resolvers.Promise.Reason() != nil {
		t.Errorf("expected nil reason, got: %v", resolvers.Promise.Reason())
	}
}

// TestWithResolvers_Chaining tests chaining from WithResolvers promise.
func TestWithResolvers_Chaining(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	resolvers := js.WithResolvers()

	// Chain transformers and get final promise
	finalPromise := resolvers.Promise.
		Then(func(v Result) Result {
			return v.(int) * 2
		}, nil)

	ch := finalPromise.ToChannel()

	resolvers.Resolve(21)

	// tick() processes microtasks (required for Then handlers to run)
	loop.tick()

	select {
	case val := <-ch:
		if val != 42 {
			t.Errorf("expected 42, got: %v", val)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for chained result")
	}
}

// TestWithResolvers_ToChannel tests ToChannel integration.
func TestWithResolvers_ToChannel(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	resolvers := js.WithResolvers()

	ch := resolvers.Promise.ToChannel()

	go func() {
		time.Sleep(10 * time.Millisecond)
		resolvers.Resolve("channel value")
	}()

	select {
	case val := <-ch:
		if val != "channel value" {
			t.Errorf("expected 'channel value', got: %v", val)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for channel")
	}
}

// TestWithResolvers_AsyncFromGoroutine tests resolving from another goroutine.
func TestWithResolvers_AsyncFromGoroutine(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	resolvers := js.WithResolvers()

	go func() {
		time.Sleep(10 * time.Millisecond)
		resolvers.Resolve("async value")
	}()

	ch := resolvers.Promise.ToChannel()

	select {
	case val := <-ch:
		if val != "async value" {
			t.Errorf("expected 'async value', got: %v", val)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for resolution")
	}
}

// TestWithResolvers_MultiplePromises tests creating multiple WithResolvers.
func TestWithResolvers_MultiplePromises(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	r1 := js.WithResolvers()
	r2 := js.WithResolvers()
	r3 := js.WithResolvers()

	// Each should be independent
	if r1.Promise == r2.Promise || r2.Promise == r3.Promise {
		t.Error("expected different promise instances")
	}

	r1.Resolve("one")
	r2.Reject(errors.New("two"))
	// r3 left pending

	if r1.Promise.State() != Fulfilled {
		t.Errorf("r1: expected Fulfilled, got: %v", r1.Promise.State())
	}
	if r2.Promise.State() != Rejected {
		t.Errorf("r2: expected Rejected, got: %v", r2.Promise.State())
	}
	if r3.Promise.State() != Pending {
		t.Errorf("r3: expected Pending, got: %v", r3.Promise.State())
	}
}

// TestWithResolvers_UseCase_RequestCorrelation demonstrates request/response correlation.
func TestWithResolvers_UseCase_RequestCorrelation(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("failed to create JS: %v", err)
	}

	// Simulate request/response correlation pattern
	pending := make(map[string]*PromiseWithResolvers)
	var mu sync.Mutex

	sendRequest := func(id string) *ChainedPromise {
		r := js.WithResolvers()
		mu.Lock()
		pending[id] = r
		mu.Unlock()
		return r.Promise
	}

	onResponse := func(id string, result any) {
		mu.Lock()
		r, ok := pending[id]
		if ok {
			delete(pending, id)
		}
		mu.Unlock()
		if ok {
			r.Resolve(result)
		}
	}

	// Simulate sending request and receiving response
	p := sendRequest("req-1")

	go func() {
		time.Sleep(10 * time.Millisecond)
		onResponse("req-1", "response data")
	}()

	ch := p.ToChannel()
	select {
	case val := <-ch:
		if val != "response data" {
			t.Errorf("expected 'response data', got: %v", val)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for response")
	}
}
