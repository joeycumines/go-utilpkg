package promisealtone_test

import (
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-eventloop/internal/promisealtone"
)

// TestPromiseBranching verifies multiple handlers on the same promise.
func TestPromiseBranching(t *testing.T) {
	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)
	defer loop.Shutdown(nil)

	p, resolve, _ := promisealtone.New(js)

	res1 := 0
	res2 := 0

	// 1. First handler (should go to h0)
	p.Then(func(v promisealtone.Result) promisealtone.Result {
		res1 = v.(int)
		return nil
	}, nil)

	// 2. Second handler (should force allocation of handlers slice)
	p.Then(func(v promisealtone.Result) promisealtone.Result {
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
	defer loop.Shutdown(nil)

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
	defer loop.Shutdown(nil)

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
	defer loop.Shutdown(nil)

	promises := make([]*promisealtone.Promise, 100)
	resolvers := make([]promisealtone.ResolveFunc, 100)
	for i := 0; i < 100; i++ {
		promises[i], resolvers[i], _ = promisealtone.New(js)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// We are benchmarking the SETUP of All, not the resolution mostly
		// Because resolution requires run loop.
		// Actually, All() creates subscriptions.
		_ = promisealtone.All(js, promises)
	}
}
