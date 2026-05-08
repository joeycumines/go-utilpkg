package promisealtone_test

import (
	"testing"

	"github.com/joeycumines/go-eventloop/internal/promisealtone"
)

// TestPromiseBasicResolveThen verifies basic promise resolution.
func TestPromiseBasicResolveThen(t *testing.T) {
	loop, js := newPromiseAltOneAutoLoop(t)

	p, resolve, _ := promisealtone.New(js)
	result := 0

	done := make(chan struct{}, 1)
	p.Then(func(v any) any {
		result = v.(int)
		done <- struct{}{}
		return v
	}, nil)

	resolve(1)

	runPromiseAltOneAutoLoop(t, loop)
	select {
	case <-done:
	default:
		t.Fatal("resolution handler did not execute before auto-exit")
	}

	if result != 1 {
		t.Errorf("Expected result 1, got %d", result)
	}
}

// TestPromiseChaining verifies chaining.
func TestPromiseChaining(t *testing.T) {
	loop, js := newPromiseAltOneAutoLoop(t)

	p, resolve, _ := promisealtone.New(js)

	finalVal := 0

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

	runPromiseAltOneAutoLoop(t, loop)
	select {
	case <-done:
	default:
		t.Fatal("chain did not complete before auto-exit")
	}

	if finalVal != 4 {
		t.Errorf("Expected 4, got %d", finalVal)
	}
}

// TestPromiseFinally verifies finally execution.
func TestPromiseFinally(t *testing.T) {
	loop, js := newPromiseAltOneAutoLoop(t)

	p, _, reject := promisealtone.New(js)
	finallyCalled := false

	done := make(chan struct{}, 1)
	p.Finally(func() {
		finallyCalled = true
		done <- struct{}{}
	}).Catch(func(r any) any {
		return "caught"
	})

	reject("error")

	runPromiseAltOneAutoLoop(t, loop)
	select {
	case <-done:
	default:
		t.Fatal("Finally handler did not execute before auto-exit")
	}

	if !finallyCalled {
		t.Error("Finally not called")
	}
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkPromiseAltOne_Chain(b *testing.B) {
	harness, js := startPromiseAltOneRunningLoop(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p, resolve, _ := promisealtone.New(js)
		done := make(chan struct{}, 1)
		p.Then(func(v any) any {
			done <- struct{}{}
			return v
		}, nil)
		resolve(1)
		harness.wait(b, done, "PromiseAltOne chain")
	}
}

func BenchmarkStandardPromise_Chain(b *testing.B) {
	harness, js := startPromiseAltOneRunningLoop(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p, resolve, _ := js.NewChainedPromise()
		done := make(chan struct{}, 1)
		p.Then(func(v any) any {
			done <- struct{}{}
			return v
		}, nil)
		resolve(1)
		harness.wait(b, done, "standard Promise chain")
	}
}

func BenchmarkPromiseAltOne_DeepChain(b *testing.B) {
	_, js := startPromiseAltOneRunningLoop(b)

	p, _, _ := promisealtone.New(js)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p = p.Then(func(v any) any {
			return v
		}, nil)
	}
}

func BenchmarkStandardPromise_DeepChain(b *testing.B) {
	_, js := startPromiseAltOneRunningLoop(b)

	p, _, _ := js.NewChainedPromise()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p = p.Then(func(v any) any {
			return v
		}, nil)
	}
}
