package promisealtthree_test

import (
	"errors"
	"sync/atomic"
	"testing"

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
	loop, js := newPromiseAltThreeAutoLoop(t)

	p, resolve, _ := promisealtthree.New(js)

	var result string

	done := make(chan struct{}, 1)
	p.Then(func(v any) any {
		return v.(string) + " transformed"
	}, nil).Then(func(v any) any {
		result = v.(string)
		done <- struct{}{}
		return nil
	}, nil)

	resolve("original")

	runPromiseAltThreeAutoLoop(t, loop)
	select {
	case <-done:
	default:
		t.Fatal("Then chain did not complete before auto-exit")
	}

	if result != "original transformed" {
		t.Errorf("Expected 'original transformed', got: %v", result)
	}
}

// TestCatch tests promise rejection handling
func TestCatch(t *testing.T) {
	loop, js := newPromiseAltThreeAutoLoop(t)

	p, _, reject := promisealtthree.New(js)

	var recovered bool

	done := make(chan struct{}, 1)
	p.Catch(func(r any) any {
		recovered = true
		done <- struct{}{}
		return "caught"
	})

	reject("error")

	runPromiseAltThreeAutoLoop(t, loop)
	select {
	case <-done:
	default:
		t.Fatal("Catch handler did not complete before auto-exit")
	}

	if !recovered {
		t.Error("Catch handler should have been called")
	}
}

// TestFinally tests finally execution
func TestFinally(t *testing.T) {
	loop, js := newPromiseAltThreeAutoLoop(t)

	p, resolve, _ := promisealtthree.New(js)

	var finallyCalled bool

	done := make(chan struct{}, 1)
	p.Finally(func() {
		finallyCalled = true
		done <- struct{}{}
	})

	resolve("value")

	runPromiseAltThreeAutoLoop(t, loop)
	select {
	case <-done:
	default:
		t.Fatal("Finally handler did not complete before auto-exit")
	}

	if !finallyCalled {
		t.Error("Finally handler should have been called")
	}
}

// TestMultipleThen tests multiple then calls
func TestMultipleThen(t *testing.T) {
	loop, js := newPromiseAltThreeAutoLoop(t)

	p, resolve, _ := promisealtthree.New(js)

	chain := p

	for range 5 {
		chain = chain.Then(func(v any) any {
			return v
		}, nil)
	}
	done := make(chan any, 1)
	chain = chain.Then(func(value any) any {
		done <- value
		return value
	}, nil)

	resolve("value")

	runPromiseAltThreeAutoLoop(t, loop)
	select {
	case value := <-done:
		if value != "value" {
			t.Errorf("final value = %v, want value", value)
		}
	default:
		t.Fatal("multiple-Then chain did not complete before auto-exit")
	}
	if chain.State() != promisealtthree.Fulfilled || chain.Result() != "value" {
		t.Errorf("final chain = (%v, %v), want (Fulfilled, value)", chain.State(), chain.Result())
	}
}

// TestStateConstants tests state constants
func TestStateConstants(t *testing.T) {
	if promisealtthree.Pending != eventloop.Pending {
		t.Errorf("Expected Pending=%d, got: %d", eventloop.Pending, promisealtthree.Pending)
	}

	if promisealtthree.Resolved != eventloop.Fulfilled {
		t.Errorf("Expected Resolved=%d, got: %d", eventloop.Fulfilled, promisealtthree.Resolved)
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
	loop, js := newPromiseAltThreeAutoLoop(t)

	p, resolve, reject := promisealtthree.New(js)

	if p.State() != promisealtthree.Pending {
		t.Errorf("Expected Pending, got: %v", p.State())
	}

	if resolve == nil || reject == nil {
		t.Fatal("New() returned a nil settlement function")
	}
	done := make(chan any, 1)
	child := p.Then(func(value any) any {
		done <- value
		return value
	}, nil)
	resolve("value")
	runPromiseAltThreeAutoLoop(t, loop)
	select {
	case value := <-done:
		if value != "value" {
			t.Errorf("observed value = %v, want value", value)
		}
	default:
		t.Fatal("JS reaction did not complete before auto-exit")
	}
	if child.State() != promisealtthree.Fulfilled || child.Result() != "value" {
		t.Errorf("JS child = (%v, %v), want (Fulfilled, value)", child.State(), child.Result())
	}
}

// TestResultTypes tests promise with different result types
func TestResultTypes(t *testing.T) {
	testCases := []struct {
		name     string
		input    any
		isSlice  bool
		expected any
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

	var completed atomic.Int32
	registered := make(chan struct{}, 10)
	for range 10 {
		go func() {
			p.Then(func(v any) any {
				completed.Add(1)
				return v
			}, nil)
			registered <- struct{}{}
		}()
	}
	for range 10 {
		waitPromiseAltThreeSignal(t, registered, "concurrent handler registration")
	}
	resolve("value")
	if completed.Load() != 10 {
		t.Errorf("handler executions = %d, want 10", completed.Load())
	}
}

// TestPromiseChaining verifies chaining
func TestPromiseChaining(t *testing.T) {
	loop, js := newPromiseAltThreeAutoLoop(t)

	p, resolve, _ := promisealtthree.New(js)

	var finalVal int

	done := make(chan struct{}, 1)
	p.Then(func(v any) any {
		return v.(int) + 1
	}, nil).Then(func(v any) any {
		return v.(int) * 2
	}, nil).Then(func(v any) any {
		finalVal = v.(int)
		done <- struct{}{}
		return nil
	}, nil)

	resolve(1)

	runPromiseAltThreeAutoLoop(t, loop)
	select {
	case <-done:
	default:
		t.Fatal("Promise chain did not complete before auto-exit")
	}

	if finalVal != 4 {
		t.Errorf("Expected 4, got %d", finalVal)
	}
}
