package promisealtone_test

import (
	"testing"

	"github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-eventloop/internal/promisealtone"
)

// TestPromiseBranching verifies multiple handlers on the same promise.
func TestPromiseBranching(t *testing.T) {
	loop, js := newPromiseAltOneAutoLoop(t)

	p, resolve, _ := promisealtone.New(js)

	res1 := 0
	res2 := 0
	done := make(chan struct{}, 2)

	// 1. First handler (should go to h0)
	p.Then(func(v any) any {
		res1 = v.(int)
		done <- struct{}{}
		return nil
	}, nil)

	// 2. Second handler (should force allocation of handlers slice)
	p.Then(func(v any) any {
		res2 = v.(int) * 2
		done <- struct{}{}
		return nil
	}, nil)

	resolve(10)
	runPromiseAltOneAutoLoop(t, loop)
	for range 2 {
		select {
		case <-done:
		default:
			t.Fatal("branch handler did not execute before auto-exit")
		}
	}

	if res1 != 10 {
		t.Errorf("Handler 1 failed: got %d, want 10", res1)
	}
	if res2 != 20 {
		t.Errorf("Handler 2 failed: got %d, want 20", res2)
	}
}

// TestPromiseCycle checks for self-resolution cycles.
func TestPromiseCycle(t *testing.T) {
	p, resolve, _ := promisealtone.New(nil)

	// Resolve with itself
	resolve(p)

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
	p1, resolve1, _ := promisealtone.New(nil)
	p2, resolve2, _ := promisealtone.New(nil)

	resolve1(p2)
	resolve2(p1)

	if p1.State() != promisealtone.Pending || p2.State() != promisealtone.Pending {
		t.Errorf("indirect cycle states = (%v, %v), want both Pending", p1.State(), p2.State())
	}
}

func BenchmarkPromiseAltOne_All(b *testing.B) {
	_, js := newPromiseAltOneUnstartedLoop(b)

	promises := make([]*promisealtone.Promise, 100)
	for i := range 100 {
		promises[i] = promisealtone.NewPromiseForTesting(js)
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
	_, js := newPromiseAltOneUnstartedLoop(b)

	promises := make([]*eventloop.ChainedPromise, 100)
	for i := range 100 {
		promises[i], _, _ = js.NewChainedPromise()
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		for j := range 100 {
			promises[j], _, _ = js.NewChainedPromise()
		}
		b.StartTimer()
		_ = js.All(promises)
	}
}

// FuzzPromiseChains performs random operations on promises to detect crashes
func FuzzPromiseChains(f *testing.F) {
	f.Add(uint8(1), uint8(1))
	f.Add(uint8(2), uint8(10))

	f.Fuzz(func(t *testing.T, op uint8, depth uint8) {
		p, resolve, reject := promisealtone.New(nil)

		// Limit depth to avoid stack overflow or timeout in fuzz
		if depth > 50 {
			depth = 50
		}

		last := p

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

		expected := promisealtone.Rejected
		if op%2 == 0 {
			expected = promisealtone.Fulfilled
		}
		if p.State() != expected {
			t.Fatalf("root state = %v, want %v", p.State(), expected)
		}
	})
}
