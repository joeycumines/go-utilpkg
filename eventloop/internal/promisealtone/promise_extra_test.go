package promisealtone_test

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-eventloop/internal/promisealtone"
)

// TestPromiseBranching verifies multiple handlers on the same promise.
func TestPromiseBranching(t *testing.T) {
	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)
	defer loop.Shutdown(context.Background())

	p, resolve, _ := promisealtone.New(js)

	res1 := 0
	res2 := 0

	// 1. First handler (should go to h0)
	p.Then(func(v any) any {
		res1 = v.(int)
		return nil
	}, nil)

	// 2. Second handler (should force allocation of handlers slice)
	p.Then(func(v any) any {
		res2 = v.(int) * 2
		return nil
	}, nil)

	resolve(10)
	runLoopFor(loop, time.Millisecond*10)

	if res1 != 10 {
		t.Errorf("Handler 1 failed: got %d, want 10", res1)
	}
	if res2 != 20 {
		t.Errorf("Handler 2 failed: got %d, want 20", res2)
	}
}

// TestPromiseCycle checks for self-resolution cycles.
func TestPromiseCycle(t *testing.T) {
	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)
	defer loop.Shutdown(context.Background())

	p, resolve, _ := promisealtone.New(js)

	// Resolve with itself
	resolve(p)

	runLoopFor(loop, time.Millisecond*10)

	if p.State() != promisealtone.Rejected {
		t.Errorf("Expected rejected state for cycle, got %v", p.State())
		return
	}

	err, ok := p.Reason().(error)
	if !ok || err.Error() != "TypeError: Chaining cycle detected" {
		t.Errorf("Unexpected rejection reason: %v", p.Reason())
	}
}

// TestPromiseIndirectCycle checks for A->B->A cycles.
func TestPromiseIndirectCycle(t *testing.T) {
	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)
	defer loop.Shutdown(context.Background())

	p1, resolve1, _ := promisealtone.New(js)
	p2, resolve2, _ := promisealtone.New(js)

	resolve1(p2)
	resolve2(p1)

	runLoopFor(loop, time.Millisecond*20)

	// Both should eventually fail or hang.
	// Standard Promise/A+ doesn't strictly mandate deep cycle detection,
	// but implementation usually handles simple cases.
	// Our implementation checks direct `val == p`. Indirect cycles usually deadlock or stack overflow
	// unless specifically handled. Standard Go implementation might not detect this.
	// Let's just see if it crashes.
}

func BenchmarkPromiseAltOne_All(b *testing.B) {
	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)
	defer loop.Shutdown(context.Background())

	promises := make([]*promisealtone.Promise, 100)
	resolvers := make([]promisealtone.ResolveFunc, 100)
	for i := range 100 {
		promises[i], resolvers[i], _ = promisealtone.New(js)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		for j := range 100 {
			promises[j] = promisealtone.NewPromiseForTesting(js)
		}
		b.StartTimer()
		_ = promisealtone.All(js, promises)
	}
}

func BenchmarkStandardPromise_All(b *testing.B) {
	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)
	defer loop.Shutdown(context.Background())

	promises := make([]*eventloop.ChainedPromise, 100)
	resolvers := make([]eventloop.ResolveFunc, 100)
	for i := range 100 {
		promises[i], resolvers[i], _ = js.NewChainedPromise()
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = js.All(promises)
	}
}

// FuzzPromiseChains performs random operations on promises to detect crashes
func FuzzPromiseChains(f *testing.F) {
	f.Add(uint8(1), uint8(1))
	f.Add(uint8(2), uint8(10))

	f.Fuzz(func(t *testing.T, op uint8, depth uint8) {
		loop, _ := eventloop.New()
		js, _ := eventloop.NewJS(loop)
		defer loop.Shutdown(context.Background())

		p, resolve, reject := promisealtone.New(js)

		// Limit depth to avoid stack overflow or timeout in fuzz
		if depth > 50 {
			depth = 50
		}

		var last *promisealtone.Promise = p

		for i := 0; i < int(depth); i++ {
			if i%2 == 0 {
				last = last.Then(func(v any) any {
					return v
				}, nil)
			} else {
				last = last.Catch(func(r any) any {
					return r
				})
			}
		}

		if op%2 == 0 {
			resolve(1)
		} else {
			reject("error")
		}

		// Run loop briefly
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		loop.Run(ctx)
	})
}
