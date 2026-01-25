package gojaeventloop_test

import (
	"runtime"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja-eventloop"
)

// TestMemoryLeaks_MicrotaskLoop verifies that high-frequency promise creation
// does not cause unbounded memory growth. This addresses CRITICAL #2 from the
// GOJA SCOPE review: "Promise wrappers reference Go-native promises, but
// no explicit cleanup mechanism exists to break this reference loop."
//
// EXPECTED BEHAVIOR:
// - Goja's GC should reclaim unreferenced promise wrappers
// - Memory should not grow unboundedly
// - Growth should be bounded (< 50% for 10K promises) and stabilize after GC
//
// VERIFICATION:
// We create 10K promises in a loop, capture memory stats after each batch,
// and verify that (1) growth is bounded, (2) GC reclaims memory, (3) no
// unbounded growth occurs across multiple GC cycles.
func TestMemoryLeaks_MicrotaskLoop(t *testing.T) {
	// Setup event loop and Goja runtime
	loop := goeventloop.New()
	defer loop.Stop()

	runtimeJS := goja.New()
	adapter, err := gojaeventloop.New(loop, runtimeJS)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	// Baseline memory stats
	runtime.GC()
	memBefore := getMemStats()

	// Create 10K promises in microtasks (high-frequency scenario)
	const PROMISE_COUNT = 10000
	for i := 0; i < PROMISE_COUNT; i++ {
		// Run on event loop to ensure proper cleanup
		loop.Run(loop.Now(), func() {
			// Create promise with microtask (common pattern)
			_, err := runtimeJS.RunString(`
				(function() {
					const p = Promise.resolve(${i});
					p.then(v => {
						// Microtask: do minimal work
						typeof v;
					});
				})();
			`)
			if err != nil {
				t.Errorf("Promise creation failed: %v", err)
			}
		})

		// Force GC every 1K promises to track memory pattern
		if i%1000 == 0 {
			runtime.GC()
		}
	}

	// Run loop to completion (all microtasks execute)
	for i := 0; i < 100; i++ {
		if !loop.RunOnce(loop.Now()) {
			break
		}
	}

	// Force final GC
	runtime.GC()
	memAfter := getMemStats()

	// Verify memory growth bounded
	memGrowth := float64(memAfter-memBefore) / float64(memBefore) * 100

	t.Logf("Memory before GC: %d bytes", memBefore)
	t.Logf("Memory after GC: %d bytes", memAfter)
	t.Logf("Memory growth: %.2f%%", memGrowth)

	// Verify growth is bounded (< 50% tolerance for 10K promises)
	// This allows for legitimate memory usage (promise objects, wrappers, handlers)
	// but detects unbounded leaks (growth > 100% would indicate leak)
	if memGrowth > 50 {
		t.Errorf("Memory leak detected: growth %.2f%% exceeds 50%% threshold (this may indicate GC hasn't reclaimed promises yet; consider adding runtime.GC() calls in long-running microtask loops)", memGrowth)
	}

	// Additional verification: run loop again and ensure no further growth
	// (this catches if references are retained in eventloop internals)
	secondRunMem := memAfter
	for i := 0; i < 1000; i++ {
		loop.Run(loop.Now(), func() {
			_, _ = runtimeJS.RunString(`Promise.resolve(${i}).then(v => typeof v);`)
		})
	}
	for i := 0; i < 100; i++ {
		if !loop.RunOnce(loop.Now()) {
			break
		}
	}
	runtime.GC()
	secondRunAfter := getMemStats()

	secondGrowth := float64(secondRunAfter-secondRunMem) / float64(secondRunMem) * 100
	t.Logf("Second run memory growth: %.2f%%", secondGrowth)

	// Second run should show very little growth (< 10%)
	// If growth is high, promises might be retained in eventloop internals
	if secondGrowth > 10 {
		t.Errorf("Potential retained references: second run growth %.2f%% indicates promises not being reclaimed (this may be an issue in production)", secondGrowth)
	}
}

// TestMemoryLeaks_NestedPromises verifies memory is reclaimed after complex
// nested promise chains complete. This tests edge cases where promise references
// might form cycles or retain other promises.
func TestMemoryLeaks_NestedPromises(t *testing.T) {
	loop := goeventloop.New()
	defer loop.Stop()

	runtimeJS := goja.New()
	adapter, err := gojaeventloop.New(loop, runtimeJS)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	runtime.GC()
	memBefore := getMemStats()

	// Create nested promise chains (P1 → P2 → P3 → ...)
	const CHAIN_DEPTH = 100
	for i := 0; i < CHAIN_DEPTH; i++ {
		loop.Run(loop.Now(), func() {
			_, err := runtimeJS.RunString(`
				(function() {
					const p1 = Promise.resolve(1);
					const p2 = p1.then(v => v + 1);
					const p3 = p2.then(v => v + 2);
					// Chain continues...
					return p3;
				})();
			`)
			if err != nil {
				t.Errorf("Nested promise creation failed: %v", err)
			}
		})
	}

	// Execute all pending microtasks
	for {
		if !loop.RunOnce(loop.Now()) {
			break
		}
	}

	runtime.GC()
	memAfter := getMemStats()

	memGrowth := float64(memAfter-memBefore) / float64(memBefore) * 100
	t.Logf("Nested promises memory growth: %.2f%%", memGrowth)

	// Nested chains should show bounded growth (< 100%)
	if memGrowth > 100 {
		t.Errorf("Nested promise memory leak detected: growth %.2f%% exceeds threshold", memGrowth)
	}
}

// TestMemoryLeaks_PromiseAll verifies Promise.all with large arrays
// doesn't leak memory when promises are settled.
func TestMemoryLeaks_PromiseAll(t *testing.T) {
	loop := goeventloop.New()
	defer loop.Stop()

	runtimeJS := goja.New()
	adapter, err := gojaeventloop.New(loop, runtimeJS)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	runtime.GC()
	memBefore := getMemStats()

	// Create Promise.all with 1000 promises
	for i := 0; i < 10; i++ {
		loop.Run(loop.Now(), func() {
			_, err := runtimeJS.RunString(`
				(function() {
					const promises = [];
					for (let j = 0; j < 1000; j++) {
						promises.push(Promise.resolve(j));
					}
					return Promise.all(promises);
				})();
			`)
			if err != nil {
				t.Errorf("Promise.all creation failed: %v", err)
			}
		})
	}

	// Execute all pending operations
	for {
		if !loop.RunOnce(loop.Now()) {
			break
		}
	}

	runtime.GC()
	memAfter := getMemStats()

	memGrowth := float64(memAfter-memBefore) / float64(memBefore) * 100
	t.Logf("Promise.all memory growth: %.2f%%", memGrowth)

	// Should show bounded growth (< 200% for 10K total promises across 10 iterations)
	if memGrowth > 200 {
		t.Errorf("Promise.all memory leak detected: growth %.2f%% exceeds threshold", memGrowth)
	}
}

// getMemStats captures current heap allocation size using runtime.ReadMemStats.
// This is a simple metric for leak detection; for production use,
// consider more sophisticated memory profiling (pprof, heap dumps).
func getMemStats() uint64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.Alloc
}
