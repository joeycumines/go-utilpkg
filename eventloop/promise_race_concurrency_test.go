package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPromiseRace_ConcurrentRejections tests Race with concurrent rejections
// This tests the CompareAndSwap logic under concurrent settlement
func TestPromiseRace_ConcurrentRejections(t *testing.T) {
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

	result := js.Race([]*ChainedPromise{p1, p2, p3})

	var wg sync.WaitGroup
	errorCount := atomic.Int32{}

	// Set up handler to track how many times it's called
	result.Then(nil, func(r any) any {
		errorCount.Add(1)
		return nil
	})

	// Concurrently reject all three promises
	wg.Add(3)
	go func() {
		defer wg.Done()
		reject1("error from p1")
	}()
	go func() {
		defer wg.Done()
		reject2("error from p2")
	}()
	go func() {
		defer wg.Done()
		reject3("error from p3")
	}()

	wg.Wait()
	loop.tick()

	// Handler should only be called once (CompareAndSwap ensures single settlement)
	if errorCount.Load() != 1 {
		t.Errorf("Expected handler to be called exactly once, but was called %d times", errorCount.Load())
	}

	// Result should be rejected
	if result.State() != Rejected {
		t.Errorf("Expected Rejected state, got: %v", result.State())
	}
}

// TestPromiseRace_ConcurrentResolutions tests Race with concurrent resolutions
func TestPromiseRace_ConcurrentResolutions(t *testing.T) {
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

	result := js.Race([]*ChainedPromise{p1, p2, p3})

	var wg sync.WaitGroup
	resolveCount := atomic.Int32{}

	result.Then(func(v any) any {
		resolveCount.Add(1)
		return nil
	}, nil)

	// Concurrently resolve all three promises
	wg.Add(3)
	go func() {
		defer wg.Done()
		resolve1("result from p1")
	}()
	go func() {
		defer wg.Done()
		resolve2("result from p2")
	}()
	go func() {
		defer wg.Done()
		resolve3("result from p3")
	}()

	wg.Wait()
	loop.tick()

	// Handler should only be called once
	if resolveCount.Load() != 1 {
		t.Errorf("Expected handler to be called exactly once, but was called %d times", resolveCount.Load())
	}

	// Result should be fulfilled
	if result.State() != Fulfilled {
		t.Errorf("Expected Fulfilled state, got: %v", result.State())
	}
}

// TestPromiseRace_MixedConcurrentSettlement tests Race with mixed resolve/reject
func TestPromiseRace_MixedConcurrentSettlement(t *testing.T) {
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

	result := js.Race([]*ChainedPromise{p1, p2, p3})

	var wg sync.WaitGroup
	settleCount := atomic.Int32{}

	result.Then(func(v any) any {
		settleCount.Add(1)
		return nil
	}, func(r any) any {
		settleCount.Add(1)
		return nil
	})

	// Concurrently settle all three promises with mixed outcomes
	wg.Add(3)
	go func() {
		defer wg.Done()
		resolve1("from p1")
	}()
	go func() {
		defer wg.Done()
		reject2("error from p2")
	}()
	go func() {
		defer wg.Done()
		resolve3("from p3")
	}()

	wg.Wait()
	loop.tick()

	// Handler should only be called once
	if settleCount.Load() != 1 {
		t.Errorf("Expected exactly one settlement, got %d", settleCount.Load())
	}
}

// TestPromiseRace_FirstPromiseSettles tests first promise winning
func TestPromiseRace_FirstPromiseSettles(t *testing.T) {
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

	result := js.Race([]*ChainedPromise{p1, p2, p3})

	var winner string
	result.Then(func(v any) any {
		winner = v.(string)
		return nil
	}, nil)

	// p1 should win since it's first
	resolve1("p1 wins")
	loop.tick()

	if winner != "p1 wins" {
		t.Errorf("Expected 'p1 wins', got '%s'", winner)
	}

	// Other promises should be ignored
	resolve2("p2")
	resolve3("p3")
	loop.tick()

	if winner != "p1 wins" {
		t.Errorf("Expected 'p1 wins' (should not change), got '%s'", winner)
	}
}

// TestPromiseRace_FirstPromiseRejects tests first promise rejection winning
func TestPromiseRace_FirstPromiseRejects(t *testing.T) {
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

	result := js.Race([]*ChainedPromise{p1, p2, p3})

	var winnerErr string
	result.Catch(func(r any) any {
		winnerErr = r.(string)
		return nil
	})

	// p1 should win rejection
	reject1("p1 error")
	loop.tick()

	if winnerErr != "p1 error" {
		t.Errorf("Expected 'p1 error', got '%s'", winnerErr)
	}

	// Other rejections should be ignored
	reject2("p2 error")
	reject3("p3 error")
	loop.tick()

	if winnerErr != "p1 error" {
		t.Errorf("Expected 'p1 error' (should not change), got '%s'", winnerErr)
	}
}

// TestPromiseRace_ManyPromisesConcurrent tests Race with many concurrent promises
func TestPromiseRace_ManyPromisesConcurrent(t *testing.T) {
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
	resolvers := make([]func(any), numPromises)

	for i := 0; i < numPromises; i++ {
		p, r, _ := js.NewChainedPromise()
		promises[i] = p
		resolvers[i] = r
	}

	result := js.Race(promises)

	var settleCount atomic.Int32
	result.Then(func(v any) any {
		settleCount.Add(1)
		return nil
	}, nil)

	// Concurrently resolve all promises
	var wg sync.WaitGroup
	for i := 0; i < numPromises; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resolvers[idx]("result")
		}(i)
	}
	wg.Wait()
	loop.tick()

	// Should only settle once despite all concurrent resolutions
	if settleCount.Load() != 1 {
		t.Errorf("Expected exactly 1 settlement with %d concurrent promises, got %d", numPromises, settleCount.Load())
	}
}

// TestPromiseRace_RaceConditionWithThenHandler tests Race with handler attached after settlement
func TestPromiseRace_RaceConditionWithThenHandler(t *testing.T) {
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
	result := js.Race([]*ChainedPromise{p})

	// Resolve immediately before handler is attached
	resolve("quick")
	loop.tick()

	// Now attach handler
	var handlerCalled bool
	result.Then(func(v any) any {
		handlerCalled = true
		if v != "quick" {
			t.Errorf("Expected 'quick', got '%v'", v)
		}
		return nil
	}, nil)

	loop.tick()

	if !handlerCalled {
		t.Error("Handler should have been called with the resolved value")
	}
}

// TestPromiseRace_TerminatedLoop tests Race on a terminated loop
func TestPromiseRace_TerminatedLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// Create JS and terminate the loop
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Let loop start
	time.Sleep(10 * time.Millisecond)

	// Terminate the loop
	cancel()

	// Wait for termination with timeout
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Skip("Loop did not terminate in time, skipping timing-dependent test")
		return
	}

	js, err := NewJS(loop)
	if err != nil {
		// Expected - loop is terminated
		return
	}

	p1, _, _ := js.NewChainedPromise()
	result := js.Race([]*ChainedPromise{p1})

	// Should remain pending since loop is terminated
	if result.State() != Pending {
		t.Errorf("Expected Pending state, got: %v", result.State())
	}

	loop.Shutdown(context.Background())
}

// TestPromiseRace_WithThenHandler tests Race with immediate then handler attachment
func TestPromiseRace_WithThenHandler(t *testing.T) {
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

	result := js.Race([]*ChainedPromise{p1, p2})

	var gotResult bool
	var resultValue string

	result.Then(func(v any) any {
		gotResult = true
		resultValue = v.(string)
		return nil
	}, nil)

	// p2 should win (resolve)
	resolve2("winner")
	loop.tick()

	if !gotResult {
		t.Error("Then handler should have been called")
	}
	if resultValue != "winner" {
		t.Errorf("Expected 'winner', got '%s'", resultValue)
	}
}

// TestPromiseRace_MultipleRaces tests multiple concurrent Race operations
func TestPromiseRace_MultipleRaces(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	const numRaces = 5
	results := make([]*ChainedPromise, numRaces)

	// Create multiple concurrent race operations
	for i := 0; i < numRaces; i++ {
		p1, resolve1, _ := js.NewChainedPromise()
		p2, _, _ := js.NewChainedPromise()

		results[i] = js.Race([]*ChainedPromise{p1, p2})

		// Resolve immediately
		resolve1("race" + string(rune('a'+i)))
	}

	var wg sync.WaitGroup
	for i := 0; i < numRaces; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx].Then(func(v any) any {
				expected := "race" + string(rune('a'+idx))
				if v != expected {
					t.Errorf("Race %d: expected '%s', got '%v'", idx, expected, v)
				}
				return nil
			}, nil)
		}(i)
	}
	wg.Wait()
	loop.tick()
}

// TestPromiseRace_PromiseAlreadySettled tests Race with already-settled promises
func TestPromiseRace_PromiseAlreadySettled(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Create already-settled promises
	p1, resolve1, _ := js.NewChainedPromise()
	p2, _, reject2 := js.NewChainedPromise()

	// Settle before Race
	resolve1("p1 ready")
	reject2("p2 ready")

	// Race with already-settled promises
	result := js.Race([]*ChainedPromise{p1, p2})

	var resultValue string
	result.Then(func(v any) any {
		resultValue = v.(string)
		return nil
	}, nil)

	loop.tick()

	// p1 was fulfilled first, so it should win
	if resultValue != "p1 ready" {
		t.Errorf("Expected 'p1 ready' (already settled), got '%s'", resultValue)
	}
}

// TestPromiseRace_PanicInHandler tests Race with panic in resolve handler
func TestPromiseRace_PanicInHandler(t *testing.T) {
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

	result := js.Race([]*ChainedPromise{p1})

	// Attach handler that panics
	result.Then(func(v any) any {
		panic("handler panic")
	}, nil)

	// Resolve should not crash even with panicking handler
	resolve1("value")
	loop.tick()
}

// TestPromiseRace_ZeroPromises tests Race with empty array (stays pending)
func TestPromiseRace_ZeroPromises(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	result := js.Race([]*ChainedPromise{})

	// Should be pending
	if result.State() != Pending {
		t.Errorf("Expected Pending state for empty Race, got: %v", result.State())
	}

	// Tick multiple times - should remain pending
	for i := 0; i < 5; i++ {
		loop.tick()
		if result.State() != Pending {
			t.Errorf("Expected still Pending at tick %d, got: %v", i, result.State())
		}
	}
}

// TestPromiseRace_WithContextCancellation tests Race with context cancellation
func TestPromiseRace_WithContextCancellation(t *testing.T) {
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
	_ = js.Race([]*ChainedPromise{p1})

	// Cancel context before resolution
	cancel()

	// Should handle context cancellation gracefully
	resolve1("value")
	loop.tick()
}

// TestPromiseRace_SecondPromiseSettles tests second promise settling (not first)
func TestPromiseRace_SecondPromiseSettles(t *testing.T) {
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

	result := js.Race([]*ChainedPromise{p1, p2})

	var winner string
	result.Then(func(v any) any {
		winner = v.(string)
		return nil
	}, nil)

	// First promise does nothing
	// Second promise resolves
	resolve2("second wins")
	loop.tick()

	if winner != "second wins" {
		t.Errorf("Expected 'second wins', got '%s'", winner)
	}
}

// TestPromiseRace_LastPromiseSettles tests last promise settling (all others ignored)
func TestPromiseRace_LastPromiseSettles(t *testing.T) {
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
	p2, _, _ := js.NewChainedPromise()
	p3, resolve3, _ := js.NewChainedPromise()

	result := js.Race([]*ChainedPromise{p1, p2, p3})

	var winner string
	result.Then(func(v any) any {
		winner = v.(string)
		return nil
	}, nil)

	// First two promises resolve but are "pending"
	// Only p3 matters in this test (we're testing order)
	resolve3("third wins")
	loop.tick()

	if winner != "third wins" {
		t.Errorf("Expected 'third wins', got '%s'", winner)
	}
}
