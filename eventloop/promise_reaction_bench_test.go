package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Promise Reaction Scheduling Benchmarks
//
// These benchmarks measure the throughput and allocation profile of the
// promise reaction scheduling paths (resolve, reject, Then on settled,
// Then on pending). They are used to quantify the performance impact of
// the FIFO ordering fix (scheduling handlers before state.Store).
//
// Run: go test -bench=BenchmarkReaction -benchmem -count=5 -run=^$ ./eventloop/

// helper: create a running loop + JS adapter, returning cleanup.
func setupBenchLoop(b *testing.B) (*Loop, *JS, func()) {
	b.Helper()
	ctx := b.Context()

	loop, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	waitForRunningBench(b, loop)

	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {}))
	if err != nil {
		b.Fatalf("NewJS() failed: %v", err)
	}

	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		loop.Shutdown(shutdownCtx)
	}

	return loop, js, cleanup
}

// noopHandler is a reusable handler closure that does nothing.
// Defined at package level to avoid per-iteration closure allocation.
var noopHandler = func(v any) any { return nil }

// --- resolve benchmarks ---

// BenchmarkReaction_Resolve_NoHandler measures resolve() with zero handlers
// attached (baseline overhead of the resolve path itself).
func BenchmarkReaction_Resolve_NoHandler(b *testing.B) {
	_, js, cleanup := setupBenchLoop(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, resolve, _ := js.NewChainedPromise()
		resolve(i)
	}
}

// BenchmarkReaction_Resolve_1Handler measures resolve() with one pre-attached
// handler (the most common case). This is the path most affected by the fix:
// handler scheduling moves before state.Store.
func BenchmarkReaction_Resolve_1Handler(b *testing.B) {
	_, js, cleanup := setupBenchLoop(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p, resolve, _ := js.NewChainedPromise()
		p.Then(noopHandler, nil)
		resolve(i)
	}
}

// BenchmarkReaction_Resolve_10Handlers measures resolve() with 10 pre-attached
// handlers. Exercises the handlers-slice path and the scheduling loop.
func BenchmarkReaction_Resolve_10Handlers(b *testing.B) {
	_, js, cleanup := setupBenchLoop(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p, resolve, _ := js.NewChainedPromise()
		for range 10 {
			p.Then(noopHandler, nil)
		}
		resolve(i)
	}
}

// BenchmarkReaction_Resolve_3Chain measures resolve() with a 3-level Then
// chain (p.Then(f).Then(g).Then(h)). Measures chained promise scheduling.
func BenchmarkReaction_Resolve_3Chain(b *testing.B) {
	_, js, cleanup := setupBenchLoop(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p, resolve, _ := js.NewChainedPromise()
		p.Then(noopHandler, nil).
			Then(noopHandler, nil).
			Then(noopHandler, nil)
		resolve(i)
	}
}

// --- reject benchmarks ---

// BenchmarkReaction_Reject_1Handler measures reject() with one pre-attached
// rejection handler (Catch).
func BenchmarkReaction_Reject_1Handler(b *testing.B) {
	_, js, cleanup := setupBenchLoop(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p, _, reject := js.NewChainedPromise()
		p.Catch(noopHandler)
		reject("err")
	}
}

// BenchmarkReaction_Reject_10Handlers measures reject() with 10 pre-attached
// handlers (mix of Then and Catch).
func BenchmarkReaction_Reject_10Handlers(b *testing.B) {
	_, js, cleanup := setupBenchLoop(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p, _, reject := js.NewChainedPromise()
		for range 10 {
			p.Then(nil, noopHandler)
		}
		reject("err")
	}
}

// --- Then on settled (optimistic path) ---

// BenchmarkReaction_ThenOnSettled measures Then() called on an already-resolved
// promise. This exercises the optimistic lock-free path in addHandler.
func BenchmarkReaction_ThenOnSettled(b *testing.B) {
	_, js, cleanup := setupBenchLoop(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p := js.Resolve(i)
		p.Then(noopHandler, nil)
	}
}

// BenchmarkReaction_ThenOnRejected measures Then() called on an already-rejected
// promise. Exercises the optimistic path with Rejected state.
func BenchmarkReaction_ThenOnRejected(b *testing.B) {
	_, js, cleanup := setupBenchLoop(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p := js.Reject("err")
		p.Then(nil, noopHandler)
	}
}

// --- Then on pending (locked path) ---

// BenchmarkReaction_ThenOnPending measures Then() called on a pending promise.
// This exercises the locked storage path in addHandler (no scheduling).
func BenchmarkReaction_ThenOnPending(b *testing.B) {
	_, js, cleanup := setupBenchLoop(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p, _, _ := js.NewChainedPromise()
		p.Then(noopHandler, nil)
	}
}

// --- concurrent resolve benchmark ---

// BenchmarkReaction_ConcurrentResolve measures resolve + concurrent Then
// under parallel load. This is the race scenario, but
// measured for throughput rather than correctness.
func BenchmarkReaction_ConcurrentResolve(b *testing.B) {
	_, js, cleanup := setupBenchLoop(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			p, resolve, _ := js.NewChainedPromise()
			p.Then(noopHandler, nil)
			resolve(0)
		}
	})
}

// BenchmarkReaction_ConcurrentResolveAndThen measures the exact race scenario:
// resolve from one goroutine, Then from another. This stresses the addHandler
// optimistic path racing with resolve's state.Store.
func BenchmarkReaction_ConcurrentResolveAndThen(b *testing.B) {
	_, js, cleanup := setupBenchLoop(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			p, resolve, _ := js.NewChainedPromise()
			p.Then(noopHandler, nil) // pre-attach one handler

			var wg sync.WaitGroup
			wg.Go(func() {
				resolve(0)
			})
			wg.Go(func() {
				p.Then(noopHandler, nil)
			})
			wg.Wait()
		}
	})
}
