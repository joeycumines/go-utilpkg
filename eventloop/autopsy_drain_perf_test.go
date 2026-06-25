package eventloop

import (
	"context"
	"testing"
	"time"
)

// drainMicrotasks() Performance Benchmarks
//
// These benchmarks measure the overhead of the unconditional drainMicrotasks()
// call that occurs after every internal task execution in processInternalQueue
// (loop.go: l.drainMicrotasks() after each safeExecute(task)).
//
// The benchmarks use WithFastPathMode(FastPathDisabled) to force the poll path
// (tick() → processInternalQueue()), ensuring we measure the processInternalQueue
// code path, not the runAux fast path.
//
// The fast-path early-return in drainMicrotasks() is:
//
//	if l.nextTickQueue.IsEmpty() && l.microtasks.IsEmpty() {
//	    return
//	}
//
// When no microtasks or nextTicks are pending, this evaluates two IsEmpty()
// checks and returns immediately. BenchmarkDrainPerf_NoMicrotasks quantifies
// the cost of this fast-path on every internal task.
//
// Run: go test -bench=BenchmarkDrainPerf -benchmem -count=5 -run=^$ ./eventloop/

// setupDrainBenchLoop creates a running loop with FastPathDisabled to force
// the poll path (tick() → processInternalQueue). The per-task draining in
// processInternalQueue is unconditional. Returns the loop and a cleanup func.
func setupDrainBenchLoop(b *testing.B) (*Loop, func()) {
	b.Helper()
	ctx := context.Background()

	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	time.Sleep(10 * time.Millisecond) // let loop start

	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = loop.Shutdown(shutdownCtx)
	}

	return loop, cleanup
}

// BenchmarkDrainPerf_NoMicrotasks measures the overhead of the unconditional
// drainMicrotasks() calls when no microtasks are ever scheduled. Each internal
// task is a no-op, so drainMicrotasks() hits the fast-path early-return every
// time: two IsEmpty() checks and a return. This is the baseline overhead.
//
// N = b.N tasks are submitted in a batch, then a sentinel task closes a channel
// to signal completion. The benchmark measures total wall time for the batch.
func BenchmarkDrainPerf_NoMicrotasks(b *testing.B) {
	loop, cleanup := setupDrainBenchLoop(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})

		for j := 0; j < 10000; j++ {
			if err := loop.SubmitInternal(func() {}); err != nil {
				b.Fatalf("SubmitInternal: %v", err)
			}
		}

		// Sentinel: signals all prior tasks have been processed.
		if err := loop.SubmitInternal(func() { close(done) }); err != nil {
			b.Fatalf("sentinel SubmitInternal: %v", err)
		}
		<-done
	}
}

// BenchmarkDrainPerf_WithMicrotasks measures throughput when each internal task
// schedules exactly 1 microtask via ScheduleMicrotask. This exercises the full
// drainMicrotasks() loop: the fast-path check fails, one microtask is popped
// and executed, then the next Pop() returns nil and the loop breaks.
func BenchmarkDrainPerf_WithMicrotasks(b *testing.B) {
	loop, cleanup := setupDrainBenchLoop(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})

		for j := 0; j < 10000; j++ {
			if err := loop.SubmitInternal(func() {
				_ = loop.ScheduleMicrotask(func() {})
			}); err != nil {
				b.Fatalf("SubmitInternal: %v", err)
			}
		}

		if err := loop.SubmitInternal(func() { close(done) }); err != nil {
			b.Fatalf("sentinel SubmitInternal: %v", err)
		}
		<-done
	}
}

// BenchmarkDrainPerf_WithNextTick measures throughput when each internal task
// schedules exactly 1 nextTick via ScheduleNextTick. nextTick callbacks have
// higher priority than regular microtasks and run first in drainMicrotasks().
func BenchmarkDrainPerf_WithNextTick(b *testing.B) {
	loop, cleanup := setupDrainBenchLoop(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})

		for j := 0; j < 10000; j++ {
			if err := loop.SubmitInternal(func() {
				_ = loop.ScheduleNextTick(func() {})
			}); err != nil {
				b.Fatalf("SubmitInternal: %v", err)
			}
		}

		if err := loop.SubmitInternal(func() { close(done) }); err != nil {
			b.Fatalf("sentinel SubmitInternal: %v", err)
		}
		<-done
	}
}

// BenchmarkDrainPerf_MixedWorkload measures throughput with a realistic mix:
//   - 50% internal tasks that each schedule 1 microtask
//   - 30% internal tasks with no microtasks (fast-path drain)
//   - 20% internal tasks that each schedule 1 nextTick
//
// This exercises the drainMicrotasks() code paths with both microtask and
// nextTick scheduling, without timers (which fire asynchronously and would
// not complete before the sentinel).
func BenchmarkDrainPerf_MixedWorkload(b *testing.B) {
	loop, cleanup := setupDrainBenchLoop(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})

		for j := 0; j < 10000; j++ {
			switch j % 10 {
			case 0, 1, 2, 3, 4: // 50% internal + microtask
				if err := loop.SubmitInternal(func() {
					_ = loop.ScheduleMicrotask(func() {})
				}); err != nil {
					b.Fatalf("SubmitInternal: %v", err)
				}
			case 5, 6, 7: // 30% internal, no microtask
				if err := loop.SubmitInternal(func() {}); err != nil {
					b.Fatalf("SubmitInternal: %v", err)
				}
			default: // 20% internal + nextTick
				if err := loop.SubmitInternal(func() {
					_ = loop.ScheduleNextTick(func() {})
				}); err != nil {
					b.Fatalf("SubmitInternal: %v", err)
				}
			}
		}

		if err := loop.SubmitInternal(func() { close(done) }); err != nil {
			b.Fatalf("sentinel SubmitInternal: %v", err)
		}
		<-done
	}
}
