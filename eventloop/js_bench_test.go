package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// AFTER Benchmarks - performance measurements with map+RWMutex and optimized setImmediate
// Run: go test -bench=. -benchmem -count=5 -run=^$ | tee bench_after.txt
// ============================================================================

// BenchmarkSetInterval_Optimized benchmarks the new map+RWMutex SetInterval implementation.
func BenchmarkSetInterval_Optimized(b *testing.B) {
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
		id, err := js.SetInterval(func() {}, 1000)
		if err != nil {
			b.Fatalf("SetInterval failed: %v", err)
		}
		_ = js.ClearInterval(id)
	}
	b.StopTimer()

	cancel()
}

// BenchmarkSetTimeout_Optimized benchmarks the new map+RWMutex SetTimeout implementation.
// Note: SetTimeout (with delay) still uses the timer heap.
func BenchmarkSetTimeout_Optimized(b *testing.B) {
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
		id, err := js.SetTimeout(func() {}, 0) // still uses timer heap despite 0 delay
		if err != nil {
			b.Fatalf("SetTimeout failed: %v", err)
		}
		_ = js.ClearTimeout(id)
	}
	b.StopTimer()

	cancel()
}

// BenchmarkSetImmediate_Optimized benchmarks the new direct SetImmediate implementation.
// This bypasses the timer heap and should be significantly faster/leaner than SetTimeout(0).
func BenchmarkSetImmediate_Optimized(b *testing.B) {
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
		id, err := js.SetImmediate(func() {})
		if err != nil {
			b.Fatalf("SetImmediate failed: %v", err)
		}
		_ = js.ClearImmediate(id)
	}
	b.StopTimer()

	cancel()
}

// BenchmarkSetInterval_Parallel_Optimized benchmarks SetInterval (map+RWMutex) under contention.
func BenchmarkSetInterval_Parallel_Optimized(b *testing.B) {
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

// BenchmarkPromiseHandlerTracking_Optimized benchmarks promise handler tracking with map+RWMutex.
func BenchmarkPromiseHandlerTracking_Optimized(b *testing.B) {
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
		id := uint64(i)

		js.promiseHandlersMu.Lock()
		js.promiseHandlers[id] = true
		js.promiseHandlersMu.Unlock()

		js.promiseHandlersMu.RLock()
		_ = js.promiseHandlers[id]
		js.promiseHandlersMu.RUnlock()

		js.promiseHandlersMu.Lock()
		delete(js.promiseHandlers, id)
		js.promiseHandlersMu.Unlock()
	}
	b.StopTimer()

	cancel()
}

// BenchmarkPromiseHandlerTracking_Parallel_Optimized benchmarks map+RWMutex under contention.
func BenchmarkPromiseHandlerTracking_Parallel_Optimized(b *testing.B) {
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

			js.promiseHandlersMu.Lock()
			js.promiseHandlers[id] = true
			js.promiseHandlersMu.Unlock()

			js.promiseHandlersMu.RLock()
			_ = js.promiseHandlers[id]
			js.promiseHandlersMu.RUnlock()

			js.promiseHandlersMu.Lock()
			delete(js.promiseHandlers, id)
			js.promiseHandlersMu.Unlock()
		}
	})
	b.StopTimer()

	cancel()
}
