package eventloop

import (
	"context"
	"runtime"
	"testing"
	"time"
)

// Promise Memory Profiling Benchmarks (EXPAND-012)
//
// This file contains benchmarks to profile memory allocation patterns in
// promise creation, resolution, and garbage collection. Use with -benchmem
// to see allocation counts and bytes.
//
// Run with: go test -bench=BenchmarkPromise -benchmem ./eventloop/

// BenchmarkPromiseCreation measures allocations for creating a new promise.
func BenchmarkPromiseCreation(b *testing.B) {
	ctx := b.Context()

	loop, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("NewJS() failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		js.NewChainedPromise()
	}
}

// BenchmarkPromiseResolution measures allocations for resolving a promise.
func BenchmarkPromiseResolution(b *testing.B) {
	ctx := b.Context()

	loop, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("NewJS() failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, resolve, _ := js.NewChainedPromise()
		resolve(i)
	}
}

// BenchmarkPromiseRejection measures allocations for rejecting a promise.
func BenchmarkPromiseRejection(b *testing.B) {
	ctx := b.Context()

	loop, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	// Use unhandled rejection callback to suppress warnings
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {}))
	if err != nil {
		b.Fatalf("NewJS() failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, reject := js.NewChainedPromise()
		reject("error")
	}
}

// BenchmarkPromiseThenChain measures allocations for building a promise chain.
func BenchmarkPromiseThenChain(b *testing.B) {
	ctx := b.Context()

	loop, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("NewJS() failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p, resolve, _ := js.NewChainedPromise()

		// Chain 3 .then() handlers
		p.Then(func(v any) any { return v }, nil).
			Then(func(v any) any { return v }, nil).
			Then(func(v any) any { return v }, nil)

		resolve(i)
	}
}

// BenchmarkPromiseResolve_Memory measures allocations for Promise.resolve().
func BenchmarkPromiseResolve_Memory(b *testing.B) {
	ctx := b.Context()

	loop, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("NewJS() failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		js.Resolve(i)
	}
}

// BenchmarkPromiseReject measures allocations for Promise.reject().
func BenchmarkPromiseReject(b *testing.B) {
	ctx := b.Context()

	loop, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	// Suppress unhandled rejection warnings
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {}))
	if err != nil {
		b.Fatalf("NewJS() failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		js.Reject("error")
	}
}

// BenchmarkPromiseAll_Memory measures allocations for Promise.all().
func BenchmarkPromiseAll_Memory(b *testing.B) {
	ctx := b.Context()

	loop, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("NewJS() failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p1 := js.Resolve(1)
		p2 := js.Resolve(2)
		p3 := js.Resolve(3)
		js.All([]*ChainedPromise{p1, p2, p3})
	}
}

// BenchmarkPromiseRace measures allocations for Promise.race().
func BenchmarkPromiseRace(b *testing.B) {
	ctx := b.Context()

	loop, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("NewJS() failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p1 := js.Resolve(1)
		p2 := js.Resolve(2)
		p3 := js.Resolve(3)
		js.Race([]*ChainedPromise{p1, p2, p3})
	}
}

// BenchmarkPromiseWithResolvers measures allocations for Promise.withResolvers().
func BenchmarkPromiseWithResolvers(b *testing.B) {
	ctx := b.Context()

	loop, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("NewJS() failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pwr := js.WithResolvers()
		pwr.Resolve(i)
	}
}

// BenchmarkPromiseTry measures allocations for Promise.try().
func BenchmarkPromiseTry(b *testing.B) {
	ctx := b.Context()

	loop, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("NewJS() failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		js.Try(func() any { return i })
	}
}

// BenchmarkPromiseGC measures how quickly promises are garbage collected.
// This is an allocation hotspot detection benchmark.
func BenchmarkPromiseGC(b *testing.B) {
	ctx := b.Context()

	loop, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		b.Fatalf("NewJS() failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create and immediately resolve 100 promises, then force GC
		for j := range 100 {
			p, resolve, _ := js.NewChainedPromise()
			resolve(j)
			_ = p
		}

		if i%10 == 0 {
			runtime.GC()
		}
	}
}

// BenchmarkPromisifyAllocation measures allocations for Promisify.
func BenchmarkPromisifyAllocation(b *testing.B) {
	ctx := b.Context()

	loop, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	done := make(chan struct{}, b.N)

	for i := 0; i < b.N; i++ {
		p := loop.Promisify(ctx, func(ctx context.Context) (any, error) {
			return i, nil
		})
		go func() {
			<-p.ToChannel()
			done <- struct{}{}
		}()
	}

	// Wait for all promises to resolve
	for i := 0; i < b.N; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			b.Fatalf("Timeout waiting for promises")
		}
	}
}

// === Allocation Hotspot Tests ===
// These tests verify allocation counts and identify memory hotspots.

// TestPromiseAllocationHotspots identifies allocation hotspots in promise operations.
func TestPromiseAllocationHotspots(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	js, _ := NewJS(loop)

	// Baseline memory
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// Create 1000 promises
	const N = 1000
	for i := range N {
		p, resolve, _ := js.NewChainedPromise()
		resolve(i)
		_ = p
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	allocatedBytes := m2.TotalAlloc - m1.TotalAlloc
	bytesPerPromise := allocatedBytes / N

	t.Logf("Promise Memory Profile:")
	t.Logf("  Total allocated: %d bytes", allocatedBytes)
	t.Logf("  Bytes per promise (create+resolve): ~%d bytes", bytesPerPromise)
	t.Logf("  Heap objects created: %d", m2.Mallocs-m1.Mallocs)

	// Document expected allocation pattern
	// This helps identify regressions in memory usage
	if bytesPerPromise > 2000 {
		t.Logf("WARNING: High memory per promise. Consider investigating:")
		t.Logf("  - promise struct size")
		t.Logf("  - closure allocations in handlers")
		t.Logf("  - registry overhead")
	}
}

// TestPromiseChainAllocationHotspots profiles .then() chain allocations.
func TestPromiseChainAllocationHotspots(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	js, _ := NewJS(loop)

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// Create 100 promise chains of depth 10
	const chains = 100
	const depth = 10
	for i := range chains {
		p, resolve, _ := js.NewChainedPromise()

		current := p
		for range depth {
			current = current.Then(func(v any) any { return v }, nil)
		}

		resolve(i)
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	allocatedBytes := m2.TotalAlloc - m1.TotalAlloc
	bytesPerChain := allocatedBytes / chains

	t.Logf("Promise Chain Memory Profile (depth=%d):", depth)
	t.Logf("  Total allocated: %d bytes", allocatedBytes)
	t.Logf("  Bytes per chain: ~%d bytes", bytesPerChain)
	t.Logf("  Bytes per .then(): ~%d bytes", bytesPerChain/depth)
}

// TestPromiseCombinatorAllocationHotspots profiles Promise.all/race allocations.
func TestPromiseCombinatorAllocationHotspots(t *testing.T) {
	ctx := t.Context()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	js, _ := NewJS(loop)

	var m1, m2 runtime.MemStats

	// Profile Promise.all with 10 promises
	runtime.GC()
	runtime.ReadMemStats(&m1)

	const N = 100
	for range N {
		promises := make([]*ChainedPromise, 10)
		for j := range promises {
			promises[j] = js.Resolve(j)
		}
		js.All(promises)
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	allocatedBytes := m2.TotalAlloc - m1.TotalAlloc
	bytesPerAll := allocatedBytes / N

	t.Logf("Promise.all Memory Profile (10 promises):")
	t.Logf("  Bytes per Promise.all(): ~%d bytes", bytesPerAll)

	// Profile Promise.race with 10 promises
	runtime.GC()
	runtime.ReadMemStats(&m1)

	for range N {
		promises := make([]*ChainedPromise, 10)
		for j := range promises {
			promises[j] = js.Resolve(j)
		}
		js.Race(promises)
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	allocatedBytes = m2.TotalAlloc - m1.TotalAlloc
	bytesPerRace := allocatedBytes / N

	t.Logf("Promise.race Memory Profile (10 promises):")
	t.Logf("  Bytes per Promise.race(): ~%d bytes", bytesPerRace)
}
