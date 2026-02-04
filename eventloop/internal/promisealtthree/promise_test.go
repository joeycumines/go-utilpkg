package promisealtthree_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-eventloop/internal/promisealtthree"
)

// TestNew tests creating a new promise
func TestNew(t *testing.T) {
	p, resolve, reject := promisealtthree.New(nil)

	if p.State() != promisealtthree.Pending {
		t.Errorf("Expected Pending, got: %v", p.State())
	}

	if p.Result() != nil {
		t.Errorf("Expected nil result for pending promise, got: %v", p.Result())
	}

	_ = reject
	_ = resolve
}

// TestResolve tests promise resolution
func TestResolve(t *testing.T) {
	p, resolve, _ := promisealtthree.New(nil)

	if p.State() != promisealtthree.Pending {
		t.Errorf("Expected Pending, got: %v", p.State())
	}

	resolve("value")

	if p.State() != promisealtthree.Resolved {
		t.Errorf("Expected Resolved, got: %v", p.State())
	}

	if p.Result() != "value" {
		t.Errorf("Expected 'value', got: %v", p.Result())
	}
}

// TestReject tests promise rejection
func TestReject(t *testing.T) {
	p, _, reject := promisealtthree.New(nil)

	reject(errors.New("error"))

	if p.State() != promisealtthree.Rejected {
		t.Errorf("Expected Rejected, got: %v", p.State())
	}

	if p.Result().(error).Error() != "error" {
		t.Errorf("Expected 'error', got: %v", p.Result())
	}
}

// TestThen tests promise chaining
func TestThen(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := eventloop.NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := promisealtthree.New(js)

	var result string

	p.Then(func(v promisealtthree.Result) promisealtthree.Result {
		return v.(string) + " transformed"
	}, nil).Then(func(v promisealtthree.Result) promisealtthree.Result {
		result = v.(string)
		return nil
	}, nil)

	resolve("original")

	runLoopFor(loop, time.Millisecond*10)

	if result != "original transformed" {
		t.Errorf("Expected 'original transformed', got: %v", result)
	}
}

// TestCatch tests promise rejection handling
func TestCatch(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := eventloop.NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, _, reject := promisealtthree.New(js)

	var recovered bool

	p.Catch(func(r promisealtthree.Result) promisealtthree.Result {
		recovered = true
		return "caught"
	})

	reject("error")

	runLoopFor(loop, time.Millisecond*10)

	if !recovered {
		t.Error("Catch handler should have been called")
	}
}

// TestFinally tests finally execution
func TestFinally(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := eventloop.NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := promisealtthree.New(js)

	var finallyCalled bool

	p.Finally(func() {
		finallyCalled = true
	})

	resolve("value")

	runLoopFor(loop, time.Millisecond*10)

	if !finallyCalled {
		t.Error("Finally handler should have been called")
	}
}

// TestMultipleThen tests multiple then calls
func TestMultipleThen(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := eventloop.NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := promisealtthree.New(js)

	chain := p

	for i := 0; i < 5; i++ {
		chain = chain.Then(func(v promisealtthree.Result) promisealtthree.Result {
			return v
		}, nil)
	}

	resolve("value")

	runLoopFor(loop, time.Millisecond*10)
}

// TestStateConstants tests state constants
func TestStateConstants(t *testing.T) {
	if promisealtthree.Pending != eventloop.Pending {
		t.Errorf("Expected Pending=%d, got: %d", eventloop.Pending, promisealtthree.Pending)
	}

	if promisealtthree.Resolved != eventloop.Resolved {
		t.Errorf("Expected Resolved=%d, got: %d", eventloop.Resolved, promisealtthree.Resolved)
	}

	if promisealtthree.Fulfilled != eventloop.Fulfilled {
		t.Errorf("Expected Fulfilled=%d, got: %d", eventloop.Fulfilled, promisealtthree.Fulfilled)
	}

	if promisealtthree.Rejected != eventloop.Rejected {
		t.Errorf("Expected Rejected=%d, got: %d", eventloop.Rejected, promisealtthree.Rejected)
	}
}

// TestPromiseWithJS tests promise with JS adapter
func TestPromiseWithJS(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := eventloop.NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, reject := promisealtthree.New(js)

	if p.State() != promisealtthree.Pending {
		t.Errorf("Expected Pending, got: %v", p.State())
	}

	_ = resolve
	_ = reject
}

// TestResultTypes tests promise with different result types
func TestResultTypes(t *testing.T) {
	testCases := []struct {
		name     string
		input    promisealtthree.Result
		isSlice  bool
		expected interface{}
	}{
		{"nil", nil, false, nil},
		{"string", "string", false, "string"},
		{"int", 42, false, 42},
		{"float", 3.14, false, 3.14},
		{"bool_true", true, false, true},
		{"bool_false", false, false, false},
		{"error", errors.New("error"), false, "error"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p, resolve, _ := promisealtthree.New(nil)

			if p.Result() != nil {
				t.Errorf("Expected nil result, got: %v", p.Result())
			}

			resolve(tc.input)

			if p.Result() != tc.input {
				if tc.isSlice {
					// Slices can't be compared directly
					t.Logf("Result set correctly (slice comparison skipped)")
				} else {
					t.Errorf("Expected %v, got: %v", tc.input, p.Result())
				}
			}
		})
	}
}

// TestNilHandlers tests promise with nil handlers
func TestNilHandlers(t *testing.T) {
	p, resolve, _ := promisealtthree.New(nil)

	// Then with nil handlers should not panic
	p.Then(nil, nil)
	p.Catch(nil)
	p.Finally(nil)

	resolve("value")

	if p.State() != promisealtthree.Resolved {
		t.Errorf("Expected Resolved, got: %v", p.State())
	}
}

// TestConcurrentPromises tests concurrent promise operations
func TestConcurrentPromises(t *testing.T) {
	p, resolve, _ := promisealtthree.New(nil)

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.Then(func(v promisealtthree.Result) promisealtthree.Result {
				return v
			}, nil)
		}()
	}

	resolve("value")

	wg.Wait()
}

// TestPromiseChaining verifies chaining
func TestPromiseChaining(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := eventloop.NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := promisealtthree.New(js)

	var finalVal int

	p.Then(func(v promisealtthree.Result) promisealtthree.Result {
		return v.(int) + 1
	}, nil).Then(func(v promisealtthree.Result) promisealtthree.Result {
		return v.(int) * 2
	}, nil).Then(func(v promisealtthree.Result) promisealtthree.Result {
		finalVal = v.(int)
		return nil
	}, nil)

	resolve(1)

	runLoopFor(loop, time.Millisecond*50)

	if finalVal != 4 {
		t.Errorf("Expected 4, got %d", finalVal)
	}
}

func runLoopFor(loop *eventloop.Loop, d time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	loop.Run(ctx)
}
