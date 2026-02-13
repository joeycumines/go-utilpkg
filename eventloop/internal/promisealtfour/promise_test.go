package promisealtfour

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
)

func changeLoop(t *testing.T) (*eventloop.Loop, *eventloop.JS) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	js, err := eventloop.NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	// Start loop in background
	go loop.Run(context.Background())
	return loop, js
}

// TestPromiseBasicResolveThen verifies basic promise resolution.
func TestPromiseBasicResolveThen(t *testing.T) {
	loop, js := changeLoop(t)
	defer loop.Shutdown(context.Background())

	p, resolve, _ := New(js)

	done := make(chan int, 1)

	p.Then(func(v any) any {
		done <- v.(int)
		return v
	}, nil)

	resolve(1)

	select {
	case res := <-done:
		if res != 1 {
			t.Errorf("Expected result 1, got %d", res)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout")
	}
}

// TestPromiseThenAfterResolve verifies Then called after resolve works.
func TestPromiseThenAfterResolve(t *testing.T) {
	loop, js := changeLoop(t)
	defer loop.Shutdown(context.Background())

	p, resolve, _ := New(js)

	resolve(2)

	// Wait a bit to ensure settled (though strictly not required for thread safety)
	time.Sleep(10 * time.Millisecond)

	done := make(chan int, 1)

	p.Then(func(v any) any {
		done <- v.(int)
		return v
	}, nil)

	select {
	case res := <-done:
		if res != 2 {
			t.Errorf("Expected result 2, got %d", res)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout")
	}
}

// TestPromiseMultipleThen verifies multiple Then handlers.
func TestPromiseMultipleThen(t *testing.T) {
	loop, js := changeLoop(t)
	defer loop.Shutdown(context.Background())

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

	for i := 0; i < 2; i++ {
		select {
		case <-mu:
			count++
		case <-time.After(time.Millisecond * 100):
			t.Error("Timeout waiting for handlers")
		}
	}

	if count != 2 {
		t.Errorf("Expected 2 handlers, got %d", count)
	}
}

// TestPromiseFinallyAfterResolve verifies Finally runs after resolution.
func TestPromiseFinallyAfterResolve(t *testing.T) {
	loop, js := changeLoop(t)
	defer loop.Shutdown(context.Background())

	p, resolve, _ := New(js)
	done := make(chan bool)

	p.Then(func(v any) any {
		return v
	}, nil).Finally(func() {
		done <- true
	})

	resolve(1)

	select {
	case <-done:
		// success
	case <-time.After(100 * time.Millisecond):
		t.Error("Finally was not called")
	}
}

// TestPromiseBasicRejectCatch verifies basic rejection and catching.
func TestPromiseBasicRejectCatch(t *testing.T) {
	loop, js := changeLoop(t)
	defer loop.Shutdown(context.Background())

	p, _, reject := New(js)
	done := make(chan string)

	p.Catch(func(v any) any {
		done <- v.(string)
		return v
	})

	reject("test error")

	select {
	case res := <-done:
		if res != "test error" {
			t.Errorf("Expected 'test error', got '%s'", res)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Catch was not called")
	}
}

// TestPromiseThreeLevelChaining verifies 3-level promise chaining.
func TestPromiseThreeLevelChaining(t *testing.T) {
	loop, js := changeLoop(t)
	defer loop.Shutdown(context.Background())

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

	// Wait for all handlers
	for i := 0; i < 3; i++ {
		select {
		case <-mu:
		case <-time.After(time.Millisecond * 100):
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

// TestPromiseErrorPropagation verifies error recovery with Catch.
func TestPromiseErrorPropagation(t *testing.T) {
	t.Run("CatchRecoversFromRejection", func(t *testing.T) {
		loop, js := changeLoop(t)
		defer loop.Shutdown(context.Background())

		p, _, reject := New(js)
		done := make(chan string)

		// Catch should be called when promise rejects
		p.Catch(func(v any) any {
			if v.(string) != "original error" {
				t.Errorf("Expected 'original error', got '%s'", v)
			}
			done <- "recovery complete"
			return "recovery complete"
		})

		reject("original error")

		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
			t.Error("Catch was not called")
		}
	})

	t.Run("ThenAfterCatchReceivesRecovery", func(t *testing.T) {
		loop, js := changeLoop(t)
		defer loop.Shutdown(context.Background())

		p, _, reject := New(js)
		done := make(chan string)

		// Catch recovers, then receives recovery value
		p.Catch(func(v any) any {
			return "recovery complete"
		}).Then(func(v any) any {
			// This final Then should receive "recovery complete"
			done <- v.(string)
			return v
		}, nil)

		reject("original error")

		select {
		case res := <-done:
			if res != "recovery complete" {
				t.Errorf("Expected 'recovery complete', got '%s'", res)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("Timeout")
		}
	})
}
