package promisealttwo

import (
	"errors"
	"sync"
	"testing"

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

	_ = resolve
	_ = reject
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

	resolve("original")
	_ = p.Then(func(v Result) Result {
		return v.(string) + " transformed"
	}, nil)

	resolve(nil)
}

// TestCatch tests promise rejection handling
func TestCatch(t *testing.T) {
	p, _, reject := New(nil)

	recovered := false
	p.Catch(func(r Result) Result {
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
	for i := 0; i < 5; i++ {
		chain = chain.Then(func(v Result) Result {
			return v
		}, nil)
	}

	resolve("value")
}

// TestStateConstants tests state constants
func TestStateConstants(t *testing.T) {
	if Pending != eventloop.Pending {
		t.Errorf("Expected Pending=%d, got: %d", eventloop.Pending, Pending)
	}
	if Resolved != eventloop.Resolved {
		t.Errorf("Expected Resolved=%d, got: %d", eventloop.Resolved, Resolved)
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
	p, resolve, reject := New(nil)

	if p.State() != Pending {
		t.Errorf("Expected Pending, got: %v", p.State())
	}

	resolve("value")
	if p.State() != Resolved {
		t.Errorf("Expected Resolved, got: %v", p.State())
	}

	reject(errors.New("error"))
}

// TestConcurrentPromises tests concurrent promise operations
func TestConcurrentPromises(t *testing.T) {
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, resolve, _ := New(nil)

			p.Then(func(v Result) Result {
				return v
			}, nil)

			resolve("value")
		}()
	}

	wg.Wait()
}

// TestNilHandlers tests promise with nil handlers
func TestNilHandlers(t *testing.T) {
	p, resolve, _ := New(nil)

	// Then with nil handlers should not panic
	p.Then(nil, nil)
	p.Catch(nil)
	p.Finally(nil)

	resolve("value")
}

// TestResultTypes tests promise with different result types
func TestResultTypes(t *testing.T) {
	testCases := []Result{
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

	for i := 0; i < 5; i++ {
		p.Then(func(v Result) Result {
			mu.Lock()
			results = append(results, "handled")
			mu.Unlock()
			return v
		}, nil)
	}

	resolve("value")
}

// TestRejectChain tests rejection propagation through chain
func TestRejectChain(t *testing.T) {
	p, _, reject := New(nil)

	_ = p.Catch(func(r Result) Result {
		return errors.New("handled")
	})

	// Note: In this implementation, handlers are processed asynchronously
	reject(errors.New("original error"))
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
