package promisealttwo

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
)

// TestNew tests creating a new promise
func TestNew(t *testing.T) {
	p, resolve, reject := New(nil)

	if p.State() != Pending {
		t.Errorf("Expected Pending, got: %v", p.State())
	}

	if p.Result() != nil {
		t.Errorf("Expected nil result for pending promise, got: %v", p.Result())
	}

	if resolve == nil || reject == nil {
		t.Fatal("New() returned a nil settlement function")
	}
}

// TestResolve tests promise resolution
func TestResolve(t *testing.T) {
	p, resolve, _ := New(nil)

	if p.State() != Pending {
		t.Errorf("Expected Pending, got: %v", p.State())
	}

	resolve("value")

	if p.State() != Resolved {
		t.Errorf("Expected Resolved, got: %v", p.State())
	}

	if p.Result() != "value" {
		t.Errorf("Expected 'value', got: %v", p.Result())
	}
}

// TestReject tests promise rejection
func TestReject(t *testing.T) {
	p, _, reject := New(nil)

	if p.State() != Pending {
		t.Errorf("Expected Pending, got: %v", p.State())
	}

	reject(errors.New("error"))

	if p.State() != Rejected {
		t.Errorf("Expected Rejected, got: %v", p.State())
	}

	if p.Result().(error).Error() != "error" {
		t.Errorf("Expected 'error', got: %v", p.Result())
	}
}

// TestThen tests promise chaining
func TestThen(t *testing.T) {
	p, resolve, _ := New(nil)

	child := p.Then(func(v any) any {
		return v.(string) + " transformed"
	}, nil)
	resolve("original")
	if child.State() != Fulfilled || child.Result() != "original transformed" {
		t.Errorf("Then child = (%v, %v), want (Fulfilled, original transformed)", child.State(), child.Result())
	}
}

// TestCatch tests promise rejection handling
func TestCatch(t *testing.T) {
	p, _, reject := New(nil)

	recovered := false
	p.Catch(func(r any) any {
		recovered = true
		return r
	})

	reject(errors.New("original error"))

	if !recovered {
		t.Error("Catch handler should have been called")
	}
}

// TestFinally tests finally handler
func TestFinally(t *testing.T) {
	p, resolve, _ := New(nil)

	finallyCalled := false
	p.Finally(func() {
		finallyCalled = true
	})

	resolve("value")

	if !finallyCalled {
		t.Error("Finally handler should have been called")
	}
}

// TestMultipleThen tests multiple then calls
func TestMultipleThen(t *testing.T) {
	p, resolve, _ := New(nil)

	chain := p
	for range 5 {
		chain = chain.Then(func(v any) any {
			return v
		}, nil)
	}

	resolve("value")
	if chain.State() != Fulfilled || chain.Result() != "value" {
		t.Errorf("final chain = (%v, %v), want (Fulfilled, value)", chain.State(), chain.Result())
	}
}

// TestStateConstants tests state constants
func TestStateConstants(t *testing.T) {
	if Pending != eventloop.Pending {
		t.Errorf("Expected Pending=%d, got: %d", eventloop.Pending, Pending)
	}
	if Resolved != eventloop.Fulfilled {
		t.Errorf("Expected Resolved=%d, got: %d", eventloop.Fulfilled, Resolved)
	}
	if Rejected != eventloop.Rejected {
		t.Errorf("Expected Rejected=%d, got: %d", eventloop.Rejected, Rejected)
	}
	if Fulfilled != eventloop.Fulfilled {
		t.Errorf("Expected Fulfilled=%d, got: %d", eventloop.Fulfilled, Fulfilled)
	}
}

// TestPromiseWithJS tests promise with JS adapter
func TestPromiseWithJS(t *testing.T) {
	loop, err := eventloop.New(eventloop.WithAutoExit(true))
	if err != nil { t.Fatal(err) }
	js, err := eventloop.NewJS(loop)
	if err != nil { t.Fatal(err) }
	p, resolve, _ := New(js)

	if p.State() != Pending {
		t.Errorf("Expected Pending, got: %v", p.State())
	}

	done := make(chan any, 1)
	child := p.Then(func(value any) any {
		done <- value
		return value
	}, nil)
	resolve("value")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run() failed: %v", err)
	}
	select {
	case value := <-done:
		if value != "value" {
			t.Errorf("observed value = %v, want value", value)
		}
	default:
		t.Fatal("JS reaction did not execute before auto-exit")
	}
	if child.State() != Fulfilled || child.Result() != "value" {
		t.Errorf("JS child = (%v, %v), want (Fulfilled, value)", child.State(), child.Result())
	}
}

// TestConcurrentPromises tests concurrent promise operations
func TestConcurrentPromises(t *testing.T) {
	results := make(chan any, 10)
	for range 10 {
		go func() {
			p, resolve, _ := New(nil)
			child := p.Then(func(v any) any {
				return v
			}, nil)
			resolve("value")
			results <- child.Result()
		}()
	}
	for range 10 {
		select {
		case result := <-results:
			if result != "value" {
				t.Errorf("concurrent result = %v, want value", result)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent promise operations did not complete")
		}
	}
}

// TestNilHandlers tests promise with nil handlers
func TestNilHandlers(t *testing.T) {
	p, resolve, _ := New(nil)

	// Then with nil handlers should not panic
	thenChild := p.Then(nil, nil)
	catchChild := p.Catch(nil)
	finallyChild := p.Finally(nil)

	resolve("value")
	for name, child := range map[string]*Promise{
		"Then": thenChild, "Catch": catchChild, "Finally": finallyChild,
	} {
		if child.State() != Fulfilled || child.Result() != "value" {
			t.Errorf("%s nil-handler child = (%v, %v), want (Fulfilled, value)", name, child.State(), child.Result())
		}
	}
}

// TestResultTypes tests promise with different result types
func TestResultTypes(t *testing.T) {
	testCases := []any{
		nil,
		"string",
		42,
		3.14,
		true,
		false,
		errors.New("error"),
	}

	for _, tc := range testCases {
		p, resolve, _ := New(nil)
		resolve(tc)

		if p.Result() != tc {
			t.Errorf("Expected %v, got: %v", tc, p.Result())
		}
	}
}

// TestChainedPromises tests promise chaining with multiple handlers
func TestChainedPromises(t *testing.T) {
	p, resolve, _ := New(nil)

	results := []string{}
	var mu sync.Mutex

	for range 5 {
		p.Then(func(v any) any {
			mu.Lock()
			results = append(results, "handled")
			mu.Unlock()
			return v
		}, nil)
	}

	resolve("value")
	if len(results) != 5 {
		t.Errorf("handler executions = %d, want 5", len(results))
	}
}

// TestRejectChain tests rejection propagation through chain
func TestRejectChain(t *testing.T) {
	p, _, reject := New(nil)

	child := p.Catch(func(r any) any {
		return errors.New("handled")
	})

	// Note: In this implementation, handlers are processed asynchronously
	reject(errors.New("original error"))
	result, ok := child.Result().(error)
	if child.State() != Fulfilled || !ok || result.Error() != "handled" {
		t.Errorf("Catch child = (%v, %v), want fulfilled handled error", child.State(), child.Result())
	}
}

// TestNilResult tests promise with nil result
func TestNilResult(t *testing.T) {
	p, resolve, _ := New(nil)

	resolve(nil)

	if p.State() != Resolved {
		t.Errorf("Expected Resolved, got: %v", p.State())
	}

	if p.Result() != nil {
		t.Errorf("Expected nil result, got: %v", p.Result())
	}
}

// TestErrorTypes tests different error types
func TestErrorTypes(t *testing.T) {
	errorsToTest := []error{
		nil,
		errors.New(""),
		errors.New("simple error"),
	}

	for _, err := range errorsToTest {
		p, _, reject := New(nil)
		reject(err)

		if p.State() != Rejected {
			t.Errorf("Expected Rejected for error '%v', got: %v", err, p.State())
		}
	}
}
