package eventloop

import (
	"context"
	"testing"
	"time"
)

// TestChainedPromiseBasicResolveThen verifies basic promise resolution.
func TestChainedPromiseBasicResolveThen(t *testing.T) {
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
	result := 0

	p.Then(func(v Result) Result {
		result = v.(int)
		return v
	}, nil)

	resolve(1)

	// Run the event loop to process microtasks
	loop.tick()

	if result != 1 {
		t.Errorf("Expected result 1, got %d", result)
	}
}

// TestChainedPromiseThenAfterResolve verifies Then called after resolve works.
func TestChainedPromiseThenAfterResolve(t *testing.T) {
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
	result := 0

	resolve(2)
	loop.tick()

	p.Then(func(v Result) Result {
		result = v.(int)
		return v
	}, nil)

	// Run the event loop to process the Since-Then settlement handler
	loop.tick()

	if result != 2 {
		t.Errorf("Expected result 2, got %d", result)
	}
}

// TestChainedPromiseMultipleThen verifies multiple Then handlers.
func TestChainedPromiseMultipleThen(t *testing.T) {
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
	count := 0
	mu := make(chan int, 2)

	p.Then(func(v Result) Result {
		mu <- 1
		return v
	}, nil)

	p.Then(func(v Result) Result {
		mu <- 2
		return v
	}, nil)

	resolve(1)

	// Run the event loop to process microtasks
	loop.tick()

	for i := 0; i < 2; i++ {
		select {
		case <-mu:
			count++
		case <-time.After(2 * time.Second):
			t.Error("Timeout waiting for handlers")
		}
	}

	if count != 2 {
		t.Errorf("Expected 2 handlers, got %d", count)
	}
}

// TestChainedPromiseFinallyAfterResolve verifies Finally runs after resolution.
func TestChainedPromiseFinallyAfterResolve(t *testing.T) {
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

	p.Then(func(v Result) Result {
		return v
	}, nil).Finally(func() {
		finallyCalled = true
	})

	resolve(1)

	// Run the event loop to process microtasks
	loop.tick()

	if !finallyCalled {
		t.Error("Finally was not called")
	}
}

// TestChainedPromiseBasicRejectCatch verifies basic rejection and catching.
func TestChainedPromiseBasicRejectCatch(t *testing.T) {
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
	caught := false
	catchMsg := ""

	p.Catch(func(v Result) Result {
		caught = true
		catchMsg = v.(string)
		return v
	})

	reject("test error")

	// Run to process microtasks
	loop.tick()

	if !caught {
		t.Error("Catch was not called")
	}
	if catchMsg != "test error" {
		t.Errorf("Expected 'test error', got '%s'", catchMsg)
	}
}

// TestChainedPromiseThreeLevelChaining verifies 3-level promise chaining.
func TestChainedPromiseThreeLevelChaining(t *testing.T) {
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
	results := make([]int, 3)
	mu := make(chan struct{}, 3)

	p.Then(func(v Result) Result {
		results[0] = v.(int)
		mu <- struct{}{}
		return v.(int) + 1
	}, nil).Then(func(v Result) Result {
		results[1] = v.(int)
		mu <- struct{}{}
		return v.(int) + 1
	}, nil).Then(func(v Result) Result {
		results[2] = v.(int)
		mu <- struct{}{}
		return v
	}, nil)

	resolve(1)

	// Run to process microtasks
	loop.tick()

	// Wait for all handlers
	for i := 0; i < 3; i++ {
		select {
		case <-mu:
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for handlers")
		}
	}

	// Verify results: 1 -> 2 -> 3
	if results[0] != 1 {
		t.Errorf("Expected results[0]=1, got %d", results[0])
	}
	if results[1] != 2 {
		t.Errorf("Expected results[1]=2, got %d", results[1])
	}
	if results[2] != 3 {
		t.Errorf("Expected results[2]=3, got %d", results[2])
	}
}

// TestChainedPromiseErrorPropagation verifies error recovery with Catch.
func TestChainedPromiseErrorPropagation(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("CatchRecoversFromRejection", func(t *testing.T) {
		p, _, reject := js.NewChainedPromise()
		catchCalled := false

		// Catch should be called when promise rejects
		p.Catch(func(v Result) Result {
			catchCalled = true
			if v.(string) != "original error" {
				t.Errorf("Expected 'original error', got '%s'", v)
			}
			return "recovery complete"
		})

		reject("original error")
		loop.tick()

		if !catchCalled {
			t.Error("Catch was not called")
		}
	})

	t.Run("ThenAfterCatchReceivesRecovery", func(t *testing.T) {
		p, _, reject := js.NewChainedPromise()
		thenReceived := ""

		// Catch recovers, then receives recovery value
		p.Catch(func(v Result) Result {
			return "recovery complete"
		}).Then(func(v Result) Result {
			// This final Then should receive "recovery complete"
			thenReceived = v.(string)
			return v
		}, nil)

		reject("original error")
		loop.tick()

		if thenReceived != "recovery complete" {
			t.Errorf("Expected 'recovery complete', got '%s'", thenReceived)
		}
	})
}

// TestUnhandledRejectionDetection verifies unhandled callbacks are invoked.
func TestUnhandledRejectionDetection(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	t.Run("UnhandledRejectionCallbackInvoked", func(t *testing.T) {
		unhandledReason := ""
		js, err := NewJS(loop, WithUnhandledRejection(func(r Result) {
			unhandledReason = r.(string)
		}))
		if err != nil {
			t.Fatal(err)
		}

		_, _, reject := js.NewChainedPromise()
		// Reject without catch handler
		reject("test error without handler")

		// Process microtasks
		loop.tick()

		if unhandledReason != "test error without handler" {
			t.Errorf("Expected unhandled callback to be called with 'test error without handler', got '%s'", unhandledReason)
		}
	})

	t.Run("HandledRejectionNotReported", func(t *testing.T) {
		unhandledCalled := false
		js, err := NewJS(loop, WithUnhandledRejection(func(r Result) {
			unhandledCalled = true
		}))
		if err != nil {
			t.Fatal(err)
		}

		p, _, reject := js.NewChainedPromise()
		// Attach catch handler BEFORE rejection
		p.Catch(func(v Result) Result {
			return "handled"
		})

		reject("test error with handler")

		// Process microtasks
		loop.tick()

		if unhandledCalled {
			t.Error("Unhandled callback should NOT be called when promise has catch handler")
		}
	})

	t.Run("MultipleUnhandledRejectionsDetected", func(t *testing.T) {
		unhandledCount := 0
		var reasons []string
		js, err := NewJS(loop, WithUnhandledRejection(func(r Result) {
			unhandledCount++
			reasons = append(reasons, r.(string))
		}))
		if err != nil {
			t.Fatal(err)
		}

		// Create 3 rejected promises without catch handlers
		for i := 0; i < 3; i++ {
			_, _, reject := js.NewChainedPromise()
			reject("error" + string(rune('A'+i)))
		}

		// Process microtasks
		loop.tick()

		if unhandledCount != 3 {
			t.Errorf("Expected 3 unhandled rejections, got %d", unhandledCount)
		}

		// Verify all expected errors were received (unordered, since microtask order is not guaranteed)
		expectedErrors := map[string]bool{
			"errorA": false,
			"errorB": false,
			"errorC": false,
		}
		for _, reason := range reasons {
			if _, exists := expectedErrors[reason]; !exists {
				t.Errorf("Unexpected reason: %s", reason)
			} else {
				expectedErrors[reason] = true
			}
		}
		for errorName, found := range expectedErrors {
			if !found {
				t.Errorf("Missing expected rejection: %s", errorName)
			}
		}
	})
}
