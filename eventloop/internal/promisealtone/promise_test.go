package promisealtone_test

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-eventloop/internal/promisealtone"
)

// TestPromiseBasicResolveThen verifies basic promise resolution.
func TestPromiseBasicResolveThen(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := eventloop.NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := promisealtone.New(js)
	result := 0

	p.Then(func(v any) any {
		result = v.(int)
		return v
	}, nil)

	resolve(1)

	// Run the event loop to process microtasks
	runLoopFor(loop, time.Millisecond*10)

	if result != 1 {
		t.Errorf("Expected result 1, got %d", result)
	}
}

// TestPromiseChaining verifies chaining.
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

	p, resolve, _ := promisealtone.New(js)

	finalVal := 0

	p.Then(func(v any) any {
		return v.(int) + 1
	}, nil).Then(func(v any) any {
		return v.(int) * 2
	}, nil).Then(func(v any) any {
		finalVal = v.(int)
		return nil
	}, nil)

	resolve(1)

	runLoopFor(loop, time.Millisecond*50)

	if finalVal != 4 {
		t.Errorf("Expected 4, got %d", finalVal)
	}
}

// TestPromiseFinally verifies finally execution.
func TestPromiseFinally(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := eventloop.NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, _, reject := promisealtone.New(js)
	finallyCalled := false

	p.Finally(func() {
		finallyCalled = true
	}).Catch(func(r any) any {
		return "caught"
	})

	reject("error")

	runLoopFor(loop, time.Millisecond*10)

	if !finallyCalled {
		t.Error("Finally not called")
	}
}

func runLoopFor(loop *eventloop.Loop, d time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	loop.Run(ctx)
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkPromiseAltOne_Chain(b *testing.B) {
	loop, _ := eventloop.New()
	defer loop.Shutdown(context.Background())
	js, _ := eventloop.NewJS(loop)

	// We run the benchmark logic without actually running the loop logic fully,
	// or we mock the loop if possible to isolate allocation cost?
	// Actually, Promise relies on js.QueueMicrotask.
	// If we want to benchmark creation and chaining overhead, we can do it.

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p, resolve, _ := promisealtone.New(js)
		p.Then(func(v any) any {
			return v
		}, nil)
		resolve(1)
	}
}

func BenchmarkStandardPromise_Chain(b *testing.B) {
	loop, _ := eventloop.New()
	defer loop.Shutdown(context.Background())
	js, _ := eventloop.NewJS(loop)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p, resolve, _ := js.NewChainedPromise()
		p.Then(func(v any) any {
			return v
		}, nil)
		resolve(1)
	}
}

func BenchmarkPromiseAltOne_DeepChain(b *testing.B) {
	loop, _ := eventloop.New()
	defer loop.Shutdown(context.Background())
	js, _ := eventloop.NewJS(loop)

	p, _, _ := promisealtone.New(js)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p = p.Then(func(v any) any {
			return v
		}, nil)
	}
}

func BenchmarkStandardPromise_DeepChain(b *testing.B) {
	loop, _ := eventloop.New()
	defer loop.Shutdown(context.Background())
	js, _ := eventloop.NewJS(loop)

	p, _, _ := js.NewChainedPromise()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p = p.Then(func(v any) any {
			return v
		}, nil)
	}
}
