package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// BEFORE Benchmarks - baseline performance measurements
// Run: go test -bench=. -benchmem -count=5 -run=^$ | tee bench_before.txt
// ============================================================================

// BenchmarkSetInterval_Current benchmarks current SetInterval implementation.
func BenchmarkSetInterval_Current(b *testing.B) {
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

	// Give the loop time to start
	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id, err := js.SetInterval(func() {}, 1000)
		if err != nil {
			b.Fatalf("SetInterval failed: %v", err)
		}
		_ = js.ClearInterval(id)
	}
	b.StopTimer()

	cancel()
}

// BenchmarkSetTimeout_Current benchmarks current SetTimeout implementation.
func BenchmarkSetTimeout_Current(b *testing.B) {
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id, err := js.SetTimeout(func() {}, 0)
		if err != nil {
			b.Fatalf("SetTimeout failed: %v", err)
		}
		_ = js.ClearTimeout(id)
	}
	b.StopTimer()

	cancel()
}

// BenchmarkSetInterval_Parallel benchmarks SetInterval under contention.
func BenchmarkSetInterval_Parallel(b *testing.B) {
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

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			id, err := js.SetInterval(func() {}, 1000)
			if err != nil {
				b.Fatalf("SetInterval failed: %v", err)
			}
			_ = js.ClearInterval(id)
		}
	})
	b.StopTimer()

	cancel()
}

// BenchmarkPromiseHandlerTracking benchmarks the promise handler tracking pattern.
func BenchmarkPromiseHandlerTracking(b *testing.B) {
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate promise handler store/load/delete cycle
		js.promiseHandlers.Store(uint64(i), true)
		_, _ = js.promiseHandlers.Load(uint64(i))
		js.promiseHandlers.Delete(uint64(i))
	}
	b.StopTimer()

	cancel()
}

// BenchmarkPromiseHandlerTracking_Parallel benchmarks promise handler tracking under contention.
func BenchmarkPromiseHandlerTracking_Parallel(b *testing.B) {
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

	var counter uint64
	var mu sync.Mutex

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			id := counter
			counter++
			mu.Unlock()

			js.promiseHandlers.Store(id, true)
			_, _ = js.promiseHandlers.Load(id)
			js.promiseHandlers.Delete(id)
		}
	})
	b.StopTimer()

	cancel()
}

// BenchmarkRWMutexMap_Baseline provides a baseline for map+RWMutex performance.
// This simulates what our replacement will look like.
func BenchmarkRWMutexMap_Baseline(b *testing.B) {
	m := make(map[uint64]bool)
	var mu sync.RWMutex

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := uint64(i)

		mu.Lock()
		m[id] = true
		mu.Unlock()

		mu.RLock()
		_ = m[id]
		mu.RUnlock()

		mu.Lock()
		delete(m, id)
		mu.Unlock()
	}
}

// BenchmarkRWMutexMap_Parallel provides parallel baseline for map+RWMutex.
func BenchmarkRWMutexMap_Parallel(b *testing.B) {
	m := make(map[uint64]bool)
	var mu sync.RWMutex
	var counter uint64
	var counterMu sync.Mutex

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			counterMu.Lock()
			id := counter
			counter++
			counterMu.Unlock()

			mu.Lock()
			m[id] = true
			mu.Unlock()

			mu.RLock()
			_ = m[id]
			mu.RUnlock()

			mu.Lock()
			delete(m, id)
			mu.Unlock()
		}
	})
}
