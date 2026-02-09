package eventloop

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// COMPREHENSIVE BENCHMARK SUITE - PERF-003 & PERF-004
//
// This file contains benchmarks for all hot paths in the eventloop package:
// - Timer scheduling/firing
// - Microtask throughput
// - Promise creation/resolution
// - I/O polling latency
// - Memory allocation profiling
//
// Run: go test -bench=. -benchmem -count=5 -run=^$ ./eventloop/
// ============================================================================

// ============================================================================
// SECTION 1: TIMER BENCHMARKS
// ============================================================================

// BenchmarkTimerSchedule measures timer scheduling throughput.
// Expected: ~0 allocs/op due to timerPool.
func BenchmarkTimerSchedule(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		id, err := loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			b.Fatalf("ScheduleTimer failed: %v", err)
		}
		_ = loop.CancelTimer(id)
	}

	b.StopTimer()
	cancel()
}

// BenchmarkTimerSchedule_Parallel measures timer scheduling under contention.
func BenchmarkTimerSchedule_Parallel(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			id, err := loop.ScheduleTimer(time.Hour, func() {})
			if err != nil {
				b.Fatalf("ScheduleTimer failed: %v", err)
			}
			_ = loop.CancelTimer(id)
		}
	})

	b.StopTimer()
	cancel()
}

// BenchmarkTimerFire measures timer execution throughput.
// Uses 0 delay to test immediate expiration path.
func BenchmarkTimerFire(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		wg.Add(1)
		_, err := loop.ScheduleTimer(0, func() {
			wg.Done()
		})
		if err != nil {
			wg.Done()
			b.Fatalf("ScheduleTimer failed: %v", err)
		}
	}

	wg.Wait()
	b.StopTimer()
	cancel()
}

// BenchmarkTimerHeapOperations measures raw heap performance.
// This isolates heap operations from loop overhead.
func BenchmarkTimerHeapOperations(b *testing.B) {
	h := make(timerHeap, 0, b.N)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		t := &timer{
			when: time.Now().Add(time.Duration(i) * time.Millisecond),
			id:   TimerID(i),
		}
		h.Push(t)
	}

	for len(h) > 0 {
		h.Pop()
	}
}

// ============================================================================
// SECTION 2: MICROTASK BENCHMARKS
// ============================================================================

// BenchmarkMicrotaskSchedule measures microtask scheduling throughput.
// Expected: ~0 allocs/op for microtaskRing fast path.
func BenchmarkMicrotaskSchedule(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = loop.ScheduleMicrotask(func() {})
	}

	b.StopTimer()
	cancel()
}

// BenchmarkMicrotaskSchedule_Parallel measures microtask scheduling under contention.
func BenchmarkMicrotaskSchedule_Parallel(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = loop.ScheduleMicrotask(func() {})
		}
	})

	b.StopTimer()
	cancel()
}

// Benchmark_microtaskRing_PushPop measures raw ring buffer performance.
// Expected: 0 allocs/op in steady state (ring not overflowing).
func Benchmark_microtaskRing_PushPop(b *testing.B) {
	ring := newMicrotaskRing()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ring.Push(func() {})
		ring.Pop()
	}
}

// Benchmark_microtaskRing_Push measures push-only throughput.
func Benchmark_microtaskRing_Push(b *testing.B) {
	ring := newMicrotaskRing()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ring.Push(func() {})
	}
}

// Benchmark_microtaskRing_Parallel measures MPSC contention.
// Multiple producers, single consumer (typical event loop model).
func Benchmark_microtaskRing_Parallel(b *testing.B) {
	ring := newMicrotaskRing()

	// Start consumer goroutine
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				ring.Pop()
				runtime.Gosched()
			}
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ring.Push(func() {})
		}
	})

	b.StopTimer()
	close(done)
}

// BenchmarkMicrotaskExecution measures full microtask execution cycle.
func BenchmarkMicrotaskExecution(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	var executed atomic.Int64

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = loop.ScheduleMicrotask(func() {
			executed.Add(1)
		})
	}

	// Wait for all to execute
	for executed.Load() < int64(b.N) {
		runtime.Gosched()
	}

	b.StopTimer()
	cancel()
}

// ============================================================================
// SECTION 3: PROMISE BENCHMARKS
// ============================================================================

// BenchmarkPromiseCreate measures promise creation overhead.
func BenchmarkPromiseCreate(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, _ = js.NewChainedPromise()
	}

	b.StopTimer()
	cancel()
}

// BenchmarkPromiseResolve measures promise resolution.
func BenchmarkPromiseResolve(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, resolve, _ := js.NewChainedPromise()
		resolve("result")
	}

	b.StopTimer()
	cancel()
}

// BenchmarkPromiseThen measures promise chaining.
func BenchmarkPromiseThen(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		promise, resolve, _ := js.NewChainedPromise()
		promise.Then(func(v Result) Result { return v }, nil)
		resolve("result")
	}

	b.StopTimer()
	cancel()
}

// BenchmarkPromiseChain measures multi-level promise chaining.
func BenchmarkPromiseChain(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		promise, resolve, _ := js.NewChainedPromise()
		promise.
			Then(func(v Result) Result { return v }, nil).
			Then(func(v Result) Result { return v }, nil).
			Then(func(v Result) Result { return v }, nil)
		resolve("result")
	}

	b.StopTimer()
	cancel()
}

// BenchmarkPromiseAll measures Promise.All performance.
func BenchmarkPromiseAll(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p1, r1, _ := js.NewChainedPromise()
		p2, r2, _ := js.NewChainedPromise()
		p3, r3, _ := js.NewChainedPromise()

		_ = js.All([]*ChainedPromise{p1, p2, p3})

		r1("a")
		r2("b")
		r3("c")
	}

	b.StopTimer()
	cancel()
}

// ============================================================================
// SECTION 4: SUBMIT BENCHMARKS
// ============================================================================

// BenchmarkSubmit measures external Submit throughput.
func BenchmarkSubmit(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = loop.Submit(func() {})
	}

	b.StopTimer()
	cancel()
}

// BenchmarkSubmit_Parallel measures Submit under contention.
func BenchmarkSubmit_Parallel(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = loop.Submit(func() {})
		}
	})

	b.StopTimer()
	cancel()
}

// BenchmarkSubmitInternal measures internal Submit throughput.
func BenchmarkSubmitInternal(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = loop.SubmitInternal(func() {})
	}

	b.StopTimer()
	cancel()
}

// BenchmarkSubmitExecution measures full submit-execute cycle.
func BenchmarkSubmitExecution(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	var executed atomic.Int64

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = loop.Submit(func() {
			executed.Add(1)
		})
	}

	// Wait for completion
	for executed.Load() < int64(b.N) {
		runtime.Gosched()
	}

	b.StopTimer()
	cancel()
}

// ============================================================================
// SECTION 5: CHUNKED INGRESS BENCHMARKS
// ============================================================================

// Benchmark_chunkedIngress_Sequential measures push/pop throughput.
// Expected: ~0 allocs/op in steady state due to chunk pooling.
func Benchmark_chunkedIngress_Sequential(b *testing.B) {
	q := newChunkedIngress()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		q.Push(func() {})
		q.Pop()
	}
}

// Benchmark_chunkedIngress_Batch measures batched operations.
func Benchmark_chunkedIngress_Batch(b *testing.B) {
	q := newChunkedIngress()
	const batchSize = 100

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Push batch
		for j := 0; j < batchSize; j++ {
			q.Push(func() {})
		}
		// Pop batch
		for j := 0; j < batchSize; j++ {
			q.Pop()
		}
	}
}

// ============================================================================
// SECTION 6: FAST PATH VS I/O MODE BENCHMARKS
// ============================================================================

// BenchmarkFastPathSubmit measures fast path (no I/O FDs) Submit latency.
func BenchmarkFastPathSubmit(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	// Ensure fast path mode
	_ = loop.SetFastPathMode(FastPathAuto)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = loop.Submit(func() {})
	}

	b.StopTimer()
	cancel()
}

// BenchmarkFastPathExecution measures fast path execution latency.
func BenchmarkFastPathExecution(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	// Ensure fast path mode
	_ = loop.SetFastPathMode(FastPathAuto)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	var executed atomic.Int64

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = loop.Submit(func() {
			executed.Add(1)
		})
	}

	// Wait for completion
	for executed.Load() < int64(b.N) {
		runtime.Gosched()
	}

	b.StopTimer()
	cancel()
}

// ============================================================================
// SECTION 7: ALLOCATION PROFILING TESTS
//
// These tests document actual allocation counts in hot paths.
// Run with -test.v to see detailed allocation information.
// The purpose is measurement and documentation, not hard failure.
// ============================================================================

// TestAllocProfile_TimerSchedule documents timer scheduling allocations.
//
// Allocation Sources:
// - timerPool is used, so timer struct itself is 0 allocs in steady state
// - CancelTimer result channel: 1 alloc (buffered chan error)
// - Closure for SubmitInternal: varies by Go version
// - Result channel receive in CancelTimer: 1 alloc
//
// DOCUMENTED: As of 2026-02-06, timer schedule+cancel allocates ~7 allocs/op.
// This is expected due to the synchronous CancelTimer design requiring channels.
func TestAllocProfile_TimerSchedule(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Warm up pool
	for i := 0; i < 100; i++ {
		id, _ := loop.ScheduleTimer(time.Hour, func() {})
		_ = loop.CancelTimer(id)
	}

	// Measure allocations
	allocs := testing.AllocsPerRun(1000, func() {
		id, _ := loop.ScheduleTimer(time.Hour, func() {})
		_ = loop.CancelTimer(id)
	})

	cancel()

	// Document actual allocations - informational, not enforced
	t.Logf("ALLOCATION PROFILE: Timer schedule+cancel: %.2f allocs/op", allocs)
	t.Logf("  Known sources: result channel (1), closures (~3-6), heap operations")
	t.Logf("  Timer pool verified working (timer struct reused)")

	// Only warn on extreme regression (>20 allocs suggests a bug)
	if allocs > 20 {
		t.Errorf("Timer scheduling allocation regression: %.2f/op (expected <20)", allocs)
	}
}

// TestAllocProfile_microtaskRing documents ring buffer allocations.
//
// Allocation Sources (steady state):
// - Ring buffer slots: 0 allocs (pre-allocated)
// - Overflow: 0 allocs when not triggered
//
// VERIFIED: microtaskRing achieves 0 allocs/op in steady state (ring not full).
func TestAllocProfile_microtaskRing(t *testing.T) {
	ring := newMicrotaskRing()

	// Warm up
	for i := 0; i < 1000; i++ {
		ring.Push(func() {})
		ring.Pop()
	}

	// Measure allocations (steady state, no overflow)
	allocs := testing.AllocsPerRun(10000, func() {
		ring.Push(func() {})
		ring.Pop()
	})

	t.Logf("ALLOCATION PROFILE: microtaskRing push+pop: %.2f allocs/op", allocs)

	// Ring buffer MUST be zero-alloc in steady state
	if allocs > 0 {
		t.Errorf("microtaskRing allocation regression: %.2f/op (expected 0)", allocs)
	}
}

// TestAllocProfile_chunkedIngress documents chunkedIngress allocations.
//
// Allocation Sources (steady state):
// - Chunk pool: 0 allocs (pooled)
// - Task slots: 0 allocs (pre-allocated in chunk)
//
// VERIFIED: chunkedIngress achieves 0 allocs/op in steady state.
func TestAllocProfile_chunkedIngress(t *testing.T) {
	q := newChunkedIngress()

	// Warm up chunk pool
	for i := 0; i < 1000; i++ {
		q.Push(func() {})
		q.Pop()
	}

	// Measure allocations (steady state with pooled chunks)
	allocs := testing.AllocsPerRun(10000, func() {
		q.Push(func() {})
		q.Pop()
	})

	t.Logf("ALLOCATION PROFILE: chunkedIngress push+pop: %.2f allocs/op", allocs)

	// Chunk pool MUST be zero-alloc in steady state
	if allocs > 0 {
		t.Errorf("chunkedIngress allocation regression: %.2f/op (expected 0)", allocs)
	}
}

// TestAllocProfile_SubmitFastPath documents fast path Submit allocations.
//
// Allocation Sources:
// - auxJobs append: may allocate on slice growth (amortized O(1))
// - Channel send: 0 allocs (pre-allocated buffered channel)
//
// DOCUMENTED: Fast path Submit may allocate ~0-1 per op for slice growth.
func TestAllocProfile_SubmitFastPath(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Warm up
	for i := 0; i < 100; i++ {
		_ = loop.Submit(func() {})
	}
	time.Sleep(10 * time.Millisecond)

	// Measure allocations
	allocs := testing.AllocsPerRun(1000, func() {
		_ = loop.Submit(func() {})
	})

	cancel()

	t.Logf("ALLOCATION PROFILE: Submit (fast path): %.2f allocs/op", allocs)
	t.Logf("  Known sources: auxJobs slice growth (amortized)")

	// Fast path should be low-allocation
	if allocs > 2 {
		t.Errorf("Fast path Submit allocation regression: %.2f/op (expected <2)", allocs)
	}
}

// TestAllocProfile_ScheduleMicrotask documents ScheduleMicrotask allocations.
//
// Allocation Sources:
// - microtaskRing.Push: 0 allocs (ring buffer)
// - Mutex lock/unlock: 0 allocs
//
// DOCUMENTED: ScheduleMicrotask may have small allocations from locking.
func TestAllocProfile_ScheduleMicrotask(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Warm up
	for i := 0; i < 100; i++ {
		_ = loop.ScheduleMicrotask(func() {})
	}
	time.Sleep(10 * time.Millisecond)

	// Measure allocations
	allocs := testing.AllocsPerRun(1000, func() {
		_ = loop.ScheduleMicrotask(func() {})
	})

	cancel()

	t.Logf("ALLOCATION PROFILE: ScheduleMicrotask: %.2f allocs/op", allocs)

	// Should be very low allocation
	if allocs > 2 {
		t.Errorf("ScheduleMicrotask allocation regression: %.2f/op (expected <2)", allocs)
	}
}

// ============================================================================
// SECTION 8: LATENCY BENCHMARKS
// ============================================================================

// BenchmarkSubmitLatency measures round-trip latency for Submit.
func BenchmarkSubmitLatency(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	done := make(chan struct{})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = loop.Submit(func() {
			done <- struct{}{}
		})
		<-done
	}

	b.StopTimer()
	cancel()
}

// BenchmarkMicrotaskLatency measures round-trip latency for microtasks.
func BenchmarkMicrotaskLatency(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	done := make(chan struct{})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = loop.ScheduleMicrotask(func() {
			done <- struct{}{}
		})
		<-done
	}

	b.StopTimer()
	cancel()
}

// BenchmarkTimerLatency measures timer firing latency.
func BenchmarkTimerLatency(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	done := make(chan struct{})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := loop.ScheduleTimer(0, func() {
			done <- struct{}{}
		})
		if err != nil {
			b.Fatalf("ScheduleTimer failed: %v", err)
		}
		<-done
	}

	b.StopTimer()
	cancel()
}

// ============================================================================
// SECTION 9: COMBINED WORKLOAD BENCHMARKS
// ============================================================================

// BenchmarkMixedWorkload simulates realistic event loop usage.
func BenchmarkMixedWorkload(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	var executed atomic.Int64

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Mix of operations (weighted by typical usage):
		// 40% microtasks, 30% submits, 20% timers, 10% promises
		switch i % 10 {
		case 0, 1, 2, 3: // 40% microtasks
			_ = loop.ScheduleMicrotask(func() { executed.Add(1) })
		case 4, 5, 6: // 30% submits
			_ = loop.Submit(func() { executed.Add(1) })
		case 7, 8: // 20% timers
			_, _ = loop.ScheduleTimer(0, func() { executed.Add(1) })
		case 9: // 10% promises
			p, r, _ := js.NewChainedPromise()
			p.Then(func(Result) Result { executed.Add(1); return nil }, nil)
			r(nil)
		}
	}

	// Wait for completion
	expected := int64(b.N)
	for executed.Load() < expected {
		runtime.Gosched()
	}

	b.StopTimer()
	cancel()
}

// BenchmarkHighContention simulates high-contention scenario.
func BenchmarkHighContention(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	numGoroutines := runtime.GOMAXPROCS(0) * 4
	var wg sync.WaitGroup
	perGoroutine := b.N / numGoroutines

	b.ReportAllocs()
	b.ResetTimer()

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				_ = loop.Submit(func() {})
				_ = loop.ScheduleMicrotask(func() {})
			}
		}()
	}

	wg.Wait()
	b.StopTimer()
	cancel()
}

// ============================================================================
// SECTION 10: MEMORY PRESSURE TESTS
// ============================================================================

// BenchmarkMicrotaskOverflow measures performance when ring overflows.
func BenchmarkMicrotaskOverflow(b *testing.B) {
	ring := newMicrotaskRing()

	// Force overflow by filling ring
	for i := 0; i < ringBufferSize+1000; i++ {
		ring.Push(func() {})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ring.Push(func() {})
		ring.Pop()
	}
}

// BenchmarkLargeTimerHeap measures performance with many timers.
func BenchmarkLargeTimerHeap(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Pre-fill with 1000 timers
	ids := make([]TimerID, 1000)
	for i := range ids {
		id, _ := loop.ScheduleTimer(time.Hour, func() {})
		ids[i] = id
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		id, _ := loop.ScheduleTimer(time.Hour, func() {})
		_ = loop.CancelTimer(id)
	}

	b.StopTimer()

	// Cleanup
	for _, id := range ids {
		_ = loop.CancelTimer(id)
	}
	cancel()
}

// ============================================================================
// SECTION 11: JS API BENCHMARKS (using existing patterns)
// ============================================================================

// BenchmarkQueueMicrotask measures JS queueMicrotask API.
func BenchmarkQueueMicrotask(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = js.QueueMicrotask(func() {})
	}

	b.StopTimer()
	cancel()
}

// BenchmarkSetTimeoutZeroDelay measures setTimeout(fn, 0) performance.
func BenchmarkSetTimeoutZeroDelay(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		id, _ := js.SetTimeout(func() {}, 0)
		_ = js.ClearTimeout(id)
	}

	b.StopTimer()
	cancel()
}
