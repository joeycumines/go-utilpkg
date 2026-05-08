package promisealtfour

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
)

func changeLoop(t *testing.T) (*eventloop.Loop, *eventloop.JS) {
	loop := eventloop.New(eventloop.WithAutoExit(true))
	return loop, eventloop.NewJS(loop)
}

func runChangedLoop(t *testing.T, loop *eventloop.Loop) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run() failed: %v", err)
	}
}

func receiveChangedLoop[T any](t *testing.T, values <-chan T, description string) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	default:
		t.Fatalf("%s did not complete before auto-exit", description)
		var zero T
		return zero
	}
}

// TestPromiseBasicResolveThen verifies basic promise resolution.
func TestPromiseBasicResolveThen(t *testing.T) {
	loop, js := changeLoop(t)

	p, resolve, _ := New(js)

	done := make(chan int, 1)

	p.Then(func(v any) any {
		done <- v.(int)
		return v
	}, nil)

	resolve(1)
	runChangedLoop(t, loop)
	if result := receiveChangedLoop(t, done, "resolve handler"); result != 1 {
		t.Errorf("Expected result 1, got %d", result)
	}
}

// TestPromiseThenAfterResolve verifies Then called after resolve works.
func TestPromiseThenAfterResolve(t *testing.T) {
	loop, js := changeLoop(t)

	p, resolve, _ := New(js)

	resolve(2)

	done := make(chan int, 1)

	p.Then(func(v any) any {
		done <- v.(int)
		return v
	}, nil)

	runChangedLoop(t, loop)
	if result := receiveChangedLoop(t, done, "late Then handler"); result != 2 {
		t.Errorf("Expected result 2, got %d", result)
	}
}

// TestPromiseMultipleThen verifies multiple Then handlers.
func TestPromiseMultipleThen(t *testing.T) {
	loop, js := changeLoop(t)

	p, resolve, _ := New(js)
	count := 0
	mu := make(chan int, 2)

	p.Then(func(v any) any {
		mu <- 1
		return v
	}, nil)

	p.Then(func(v any) any {
		mu <- 2
		return v
	}, nil)

	resolve(1)
	runChangedLoop(t, loop)
	for range 2 {
		receiveChangedLoop(t, mu, "multiple Then handler")
		count++
	}

	if count != 2 {
		t.Errorf("Expected 2 handlers, got %d", count)
	}
}

// TestPromiseFinallyAfterResolve verifies Finally runs after resolution.
func TestPromiseFinallyAfterResolve(t *testing.T) {
	loop, js := changeLoop(t)

	p, resolve, _ := New(js)
	done := make(chan bool, 1)

	p.Then(func(v any) any {
		return v
	}, nil).Finally(func() {
		done <- true
	})

	resolve(1)
	runChangedLoop(t, loop)
	if !receiveChangedLoop(t, done, "Finally handler") {
		t.Error("Finally handler returned false")
	}
}

// TestPromiseBasicRejectCatch verifies basic rejection and catching.
func TestPromiseBasicRejectCatch(t *testing.T) {
	loop, js := changeLoop(t)

	p, _, reject := New(js)
	done := make(chan string, 1)

	p.Catch(func(v any) any {
		done <- v.(string)
		return v
	})

	reject("test error")
	runChangedLoop(t, loop)
	if result := receiveChangedLoop(t, done, "Catch handler"); result != "test error" {
		t.Errorf("Expected 'test error', got '%s'", result)
	}
}

// TestPromiseThreeLevelChaining verifies 3-level promise chaining.
func TestPromiseThreeLevelChaining(t *testing.T) {
	loop, js := changeLoop(t)

	p, resolve, _ := New(js)
	results := make([]int, 3)
	mu := make(chan struct{}, 3)

	p.Then(func(v any) any {
		results[0] = v.(int)
		mu <- struct{}{}
		return v.(int) + 1
	}, nil).Then(func(v any) any {
		results[1] = v.(int)
		mu <- struct{}{}
		return v.(int) + 1
	}, nil).Then(func(v any) any {
		results[2] = v.(int)
		mu <- struct{}{}
		return v
	}, nil)

	resolve(1)
	runChangedLoop(t, loop)
	for range 3 {
		receiveChangedLoop(t, mu, "chain handler")
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

// TestPromiseErrorPropagation verifies error recovery with Catch.
func TestPromiseErrorPropagation(t *testing.T) {
	t.Run("CatchRecoversFromRejection", func(t *testing.T) {
		loop, js := changeLoop(t)

		p, _, reject := New(js)
		done := make(chan string, 1)

		// Catch should be called when promise rejects
		p.Catch(func(v any) any {
			done <- v.(string)
			return "recovery complete"
		})

		reject("original error")
		runChangedLoop(t, loop)
		if reason := receiveChangedLoop(t, done, "Catch recovery"); reason != "original error" {
			t.Errorf("Expected 'original error', got '%s'", reason)
		}
	})

	t.Run("ThenAfterCatchReceivesRecovery", func(t *testing.T) {
		loop, js := changeLoop(t)

		p, _, reject := New(js)
		done := make(chan string, 1)

		// Catch recovers, then receives recovery value
		p.Catch(func(v any) any {
			return "recovery complete"
		}).Then(func(v any) any {
			// This final Then should receive "recovery complete"
			done <- v.(string)
			return v
		}, nil)

		reject("original error")
		runChangedLoop(t, loop)
		if result := receiveChangedLoop(t, done, "post-Catch Then"); result != "recovery complete" {
			t.Errorf("Expected 'recovery complete', got '%s'", result)
		}
	})
}
