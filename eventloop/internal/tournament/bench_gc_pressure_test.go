package tournament

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkGCPressure measures performance under sustained load with aggressive GC.
// This is T6: Memory - GC Pressure Benchmark
func BenchmarkGCPressure(b *testing.B) {
	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkGCPressure(b, impl)
		})
	}
}

func benchmarkGCPressure(b *testing.B, impl Implementation) {
	loop, err := impl.Factory()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Go(func() {
		loop.Run(ctx)
	})

	var wg sync.WaitGroup
	var counter atomic.Int64

	// Force aggressive GC during benchmark
	originalGOGC := runtime.GOMAXPROCS(0)
	runtime.GC()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		wg.Add(1)
		err := loop.Submit(func() {
			counter.Add(1)
			wg.Done()
		})
		if err != nil {
			wg.Done()
		}

		// Trigger GC periodically
		if i%1000 == 0 {
			runtime.GC()
		}
	}

	wg.Wait()
	b.StopTimer()

	// Restore
	runtime.GOMAXPROCS(originalGOGC)

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	result := BenchmarkResult{
		BenchmarkName:  "GCPressure",
		Implementation: impl.Name,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(b.N),
		Iterations:     b.N,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}

// TestGCPressure_Correctness tests correctness under GC pressure.
func TestGCPressure_Correctness(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping GC pressure test in short mode")
	}

	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			testGCPressureCorrectness(t, impl)
		})
	}
}

func testGCPressureCorrectness(t *testing.T, impl Implementation) {
	const numTasks = 10000

	start := time.Now()

	loop, err := impl.Factory()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Go(func() {
		loop.Run(ctx)
	})

	var executed atomic.Int64
	var rejected atomic.Int64
	var wg sync.WaitGroup

	// Aggressive GC goroutine
	gcStop := make(chan struct{})
	go func() {
		for {
			select {
			case <-gcStop:
				return
			default:
				runtime.GC()
				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	// Submit tasks
	for range numTasks {
		wg.Add(1)
		err := loop.Submit(func() {
			executed.Add(1)
			wg.Done()
		})
		if err != nil {
			rejected.Add(1)
			wg.Done()
		}
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Error("Timeout waiting for tasks under GC pressure")
	}

	close(gcStop)

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	exec := executed.Load()
	rej := rejected.Load()
	passed := exec+rej == numTasks

	result := TestResult{
		TestName:       "GCPressure_Correctness",
		Implementation: impl.Name,
		Passed:         passed,
		Duration:       time.Since(start),
		Metrics: map[string]any{
			"total_tasks": numTasks,
			"executed":    exec,
			"rejected":    rej,
		},
	}
	if !passed {
		result.Error = "not all tasks accounted for"
	}
	GetResults().RecordTest(result)

	if !passed {
		t.Errorf("%s: Task accounting failed: executed=%d, rejected=%d, expected=%d",
			impl.Name, exec, rej, numTasks)
	}
}

// BenchmarkGCPressure_Allocations tracks allocations under load.
func BenchmarkGCPressure_Allocations(b *testing.B) {
	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkGCPressureAllocations(b, impl)
		})
	}
}

func benchmarkGCPressureAllocations(b *testing.B, impl Implementation) {
	loop, err := impl.Factory()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Go(func() {
		loop.Run(ctx)
	})

	// Warm up
	warmupDone := make(chan struct{})
	_ = loop.Submit(func() { close(warmupDone) })
	<-warmupDone

	var wg sync.WaitGroup
	var counter atomic.Int64

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		wg.Add(1)
		err := loop.Submit(func() {
			counter.Add(1)
			wg.Done()
		})
		if err != nil {
			wg.Done()
		}
	}

	wg.Wait()
	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	// Note: ReportAllocs will show allocs/op in benchmark output
	result := BenchmarkResult{
		BenchmarkName:  "GCPressure_Allocations",
		Implementation: impl.Name,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(b.N),
		Iterations:     b.N,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}

// TestMemoryLeak tests for memory leaks under sustained load.
func TestMemoryLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak test in short mode")
	}

	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			testMemoryLeak(t, impl)
		})
	}
}

func testMemoryLeak(t *testing.T, impl Implementation) {
	const iterations = 3
	const tasksPerIteration = 10000

	start := time.Now()

	var memStats []uint64

	for range iterations {
		loop, err := impl.Factory()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx := context.Background()
		var runWg sync.WaitGroup
		runWg.Go(func() {
			loop.Run(ctx)
		})

		var wg sync.WaitGroup
		for range tasksPerIteration {
			wg.Add(1)
			_ = loop.Submit(func() {
				wg.Done()
			})
		}
		wg.Wait()

		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_ = loop.Shutdown(stopCtx)
		runWg.Wait()
		cancel()

		// Force GC and measure memory
		runtime.GC()
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		memStats = append(memStats, m.Alloc)
	}

	// Check for memory growth
	// Allow for some variance, but growth should be bounded
	passed := true
	if len(memStats) >= 2 {
		growth := float64(memStats[len(memStats)-1]) / float64(memStats[0])
		if growth > 2.0 {
			// More than 2x growth indicates potential leak
			passed = false
		}
	}

	result := TestResult{
		TestName:       "MemoryLeak",
		Implementation: impl.Name,
		Passed:         passed,
		Duration:       time.Since(start),
		Metrics: map[string]any{
			"iterations": iterations,
			"memory_kb":  memStats,
		},
	}
	if !passed {
		result.Error = "potential memory leak detected"
	}
	GetResults().RecordTest(result)

	t.Logf("%s: Memory samples (bytes): %v", impl.Name, memStats)
}
