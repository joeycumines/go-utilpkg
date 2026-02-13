// Tests for Promise combinators (All, Race, AllSettled, Any)

package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
)

// ============================================================================
// Promise.all Tests (Task 3.1)
// ============================================================================

// Test 3.1.5: All with all resolved
func TestAdapterAllWithAllResolved(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	jsAdapter, err := goeventloop.NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS adapter: %v", err)
	}

	// Create test promises
	p1, r1, _ := jsAdapter.NewChainedPromise()
	p2, r2, _ := jsAdapter.NewChainedPromise()
	p3, r3, _ := jsAdapter.NewChainedPromise()

	promises := []*goeventloop.ChainedPromise{p1, p2, p3}
	resultPromise := jsAdapter.All(promises)

	go func() { _ = loop.Run(ctx) }()

	done := make(chan struct{})
	var values any
	resultPromise.Then(func(v any) any {
		t.Logf("All() resolved with: %v (type: %T)", v, v)
		values = v
		close(done)
		return nil
	}, nil)

	// Resolve all promises
	r1(42)
	r2("hello")
	r3(true)

	// Wait for microtasks to process
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}

	if values == nil {
		t.Error("Expected promise to resolve with values, got nil")
	}
}

// Test 3.1.2: Handle empty array
func TestAdapterAllWithEmptyArray(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	jsAdapter, err := goeventloop.NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS adapter: %v", err)
	}

	promises := []*goeventloop.ChainedPromise{}
	resultPromise := jsAdapter.All(promises)

	go func() { _ = loop.Run(ctx) }()

	done := make(chan struct{})
	var resolved bool
	resultPromise.Then(func(v any) any {
		t.Logf("All(empty) resolved with: %v (type: %T)", v, v)
		resolved = true
		close(done)
		return nil
	}, nil)

	// Wait for microtasks to process
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}

	if !resolved {
		t.Error("Expected promise to resolve with empty array")
	}
}

// Test 3.1.6: All with one rejected
func TestAdapterAllWithOneRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	jsAdapter, err := goeventloop.NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS adapter: %v", err)
	}

	p1, r1, _ := jsAdapter.NewChainedPromise()
	p2, _, rej2 := jsAdapter.NewChainedPromise()
	p3, r3, _ := jsAdapter.NewChainedPromise()

	promises := []*goeventloop.ChainedPromise{p1, p2, p3}
	resultPromise := jsAdapter.All(promises)

	go func() { _ = loop.Run(ctx) }()

	done := make(chan struct{})
	var reason any
	resultPromise.Then(nil, func(r any) any {
		if r != "error from p2" {
			t.Errorf("Expected rejection reason 'error from p2', got: %v", r)
		}
		reason = r
		close(done)
		return r
	})

	r1(42)
	rej2("error from p2")
	r3(99)

	// Wait for microtasks
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}

	if reason != "error from p2" {
		t.Errorf("Expected promise to reject, got: %v", reason)
	}
}

// ============================================================================
// Promise.race Tests (Task 3.2)
// ============================================================================

// Test 3.2.5: Race timing
func TestAdapterRaceTiming(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	jsAdapter, err := goeventloop.NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS adapter: %v", err)
	}

	p1, r1, _ := jsAdapter.NewChainedPromise()
	p2, _, _ := jsAdapter.NewChainedPromise()

	promises := []*goeventloop.ChainedPromise{p1, p2}
	resultPromise := jsAdapter.Race(promises)

	go func() { _ = loop.Run(ctx) }()

	done := make(chan struct{})
	var winner any
	resultPromise.Then(func(v any) any {
		winner = v
		close(done)
		return nil
	}, nil)

	// Resolve p1 first - it should win
	r1("winner")

	// Wait for microtasks
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}

	if winner != "winner" {
		t.Errorf("Expected 'winner', got: %v", winner)
	}
}

// Test 3.2.4: First rejected wins
func TestAdapterRaceFirstRejectedWins(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	jsAdapter, err := goeventloop.NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS adapter: %v", err)
	}

	p1, _, rej1 := jsAdapter.NewChainedPromise()
	p2, r2, _ := jsAdapter.NewChainedPromise()

	promises := []*goeventloop.ChainedPromise{p1, p2}
	resultPromise := jsAdapter.Race(promises)

	go func() { _ = loop.Run(ctx) }()

	done := make(chan struct{})
	var reason any
	resultPromise.Then(nil, func(r any) any {
		reason = r
		close(done)
		return r
	})

	// Reject first
	rej1("rejected")
	r2("resolved")

	// Wait for microtasks
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}

	if reason != "rejected" {
		t.Errorf("Expected rejection, got: %v", reason)
	}
}

// ============================================================================
// Promise.allSettled Tests (Task 3.3)
// ============================================================================

// Test 3.3.3: Mixed results
func TestAdapterAllSettledMixedResults(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	jsAdapter, err := goeventloop.NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS adapter: %v", err)
	}

	p1, r1, _ := jsAdapter.NewChainedPromise()
	p2, _, rej2 := jsAdapter.NewChainedPromise()
	p3, r3, _ := jsAdapter.NewChainedPromise()

	promises := []*goeventloop.ChainedPromise{p1, p2, p3}
	resultPromise := jsAdapter.AllSettled(promises)

	go func() { _ = loop.Run(ctx) }()

	done := make(chan struct{})
	var results any
	resultPromise.Then(func(v any) any {
		t.Logf("AllSettled resolved with: %v (type: %T)", v, v)
		results = v
		close(done)
		return nil
	}, nil)

	// Resolve p1, reject p2, resolve p3
	r1(42)
	rej2("error from p2")
	r3("resolved")

	// Wait for microtasks
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}

	if results == nil {
		t.Fatal("Expected AllSettled to resolve")
	}

	t.Logf("AllSettled results: %v", results)
}

// ============================================================================
// Promise.any Tests (Task 3.4)
// ============================================================================

// Test 3.4.3: First resolved wins
func TestAdapterAnyFirstResolvedWins(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	jsAdapter, err := goeventloop.NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS adapter: %v", err)
	}

	p1, _, rej1 := jsAdapter.NewChainedPromise()
	p2, r2, _ := jsAdapter.NewChainedPromise()
	p3, _, rej3 := jsAdapter.NewChainedPromise()

	promises := []*goeventloop.ChainedPromise{p1, p2, p3}
	resultPromise := jsAdapter.Any(promises)

	go func() { _ = loop.Run(ctx) }()

	valCh := make(chan any, 1)
	resultPromise.Then(func(v any) any {
		valCh <- v
		return nil
	}, nil)

	// Reject p1, resolve p2, reject p3
	rej1("err1")
	r2("winner")
	rej3("err3")

	// Wait for microtasks
	select {
	case val := <-valCh:
		if val != "winner" {
			t.Errorf("Expected 'winner', got: %v", val)
		}
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}
}

// Test 3.4.4: All rejected
func TestAdapterAnyAllRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	jsAdapter, err := goeventloop.NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS adapter: %v", err)
	}

	p1, _, rej1 := jsAdapter.NewChainedPromise()
	p2, _, rej2 := jsAdapter.NewChainedPromise()
	p3, _, rej3 := jsAdapter.NewChainedPromise()

	promises := []*goeventloop.ChainedPromise{p1, p2, p3}
	resultPromise := jsAdapter.Any(promises)

	go func() { _ = loop.Run(ctx) }()

	done := make(chan struct{})
	var rejected bool
	resultPromise.Then(nil, func(r any) any {
		// Expect AggregateError when all promises reject
		rejected = true
		t.Logf("Rejected with: %v (type: %T)", r, r)
		close(done)
		return r
	})

	// Reject all promises
	rej1("error 1")
	rej2("error 2")
	rej3("error 3")

	// Wait for microtasks
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for rejection")
	}

	if !rejected {
		t.Error("Expected rejection when all promises reject")
	}
}

// NOTE: JavaScript integration tests require Promise binding which is handled separately.
// These Go-level tests verify the combinator implementations directly.
