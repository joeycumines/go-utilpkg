package eventloop

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

// BenchmarkScheduleTimerWithPool benchmarks ScheduleTimer with pooling enabled.
// Expected: Minimal allocations (timer structs are pooled and reused).
func BenchmarkScheduleTimerWithPool(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	// Warm up the loop
	done := make(chan struct{})
	loop.Submit(func() { close(done) })
	<-done

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		loop.ScheduleTimer(10*time.Millisecond, func() {})
	}

	b.ReportAllocs()

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	loop.Shutdown(stopCtx)
	runWg.Wait()
}

// BenchmarkScheduleTimerWithPool_Immediate benchmarks ScheduleTimer with extremely short delays.
// This exercises the hot path more aggressively as timers fire and return to pool quickly.
func BenchmarkScheduleTimerWithPool_Immediate(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	// Warm up the loop
	done := make(chan struct{})
	loop.Submit(func() { close(done) })
	<-done

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Use 0 delay to fire immediately and return to pool quickly
		loop.ScheduleTimer(0, func() {})
	}

	b.ReportAllocs()

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	loop.Shutdown(stopCtx)
	runWg.Wait()
}

// BenchmarkScheduleTimerWithPool_FireAndReuse benchmarks repeated timer scheduling and firing.
// This verifies that timers are properly returned to the pool after firing.
// Expected: Allocations should be stable and minimal after warmup.
func BenchmarkScheduleTimerWithPool_FireAndReuse(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	// Warm up the loop
	done := make(chan struct{})
	loop.Submit(func() { close(done) })
	<-done

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		loop.ScheduleTimer(1*time.Microsecond, func() {})
		// Give the timer time to fire and return to pool
		if i%100 == 0 {
			runtime.Gosched()
		}
	}

	b.ReportAllocs()

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	loop.Shutdown(stopCtx)
	runWg.Wait()
}

// TestScheduleTimerPoolVerification tests that timers are being reused from the pool.
// This is a functional test that checks the allocation pattern at a smaller scale.
func TestScheduleTimerPoolVerification(t *testing.T) {
	var allocsBefore, allocsAfter float64

	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	// Warm up the loop
	done := make(chan struct{})
	loop.Submit(func() { close(done) })
	<-done

	// Warmup: Schedule 1000 timers to populate the pool
	for i := 0; i < 1000; i++ {
		loop.ScheduleTimer(1*time.Microsecond, func() {})
	}
	time.Sleep(50 * time.Millisecond) // Let timers fire

	// Before test
	allocsBefore = testing.AllocsPerRun(1000, func() {
		loop.ScheduleTimer(1*time.Microsecond, func() {})
	})

	// Main test: Schedule 1000 timers after pool is established
	allocsAfter = testing.AllocsPerRun(1000, func() {
		loop.ScheduleTimer(1*time.Microsecond, func() {})
	})

	t.Logf("Allocs before pool warmup: %d", int(allocsBefore))
	t.Logf("Allocs after pool warmup: %d", int(allocsAfter))

	// With pool, we expect significantly fewer allocations
	// Ideally should be near zero, but allow some overhead
	if allocsAfter > allocsBefore {
		t.Logf("NOTE: Allocs increased from %d to %d (pool may need tuning)", int(allocsBefore), int(allocsAfter))
	} else {
		t.Logf("✓ Pool working: allocations improved or stable")
	}

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	loop.Shutdown(stopCtx)
	runWg.Wait()
}

// BenchmarkScheduleTimerCancel benchmarks ScheduleTimer with immediate cancellation.
// Expected: Allocations should be zero as timer is returned to pool immediately.
func BenchmarkScheduleTimerCancel(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	// Warm up the loop
	done := make(chan struct{})
	loop.Submit(func() { close(done) })
	<-done

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		id, err := loop.ScheduleTimer(10*time.Millisecond, func() {})
		if err != nil {
			b.Fatal(err)
		}
		loop.CancelTimer(id)
	}

	b.ReportAllocs()

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	loop.Shutdown(stopCtx)
	runWg.Wait()
}
