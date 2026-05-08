package tournament

import (
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
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()

	tasks := newBenchmarkBarrier(b.N)
	var counter atomic.Int64
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	// Establish a collected starting point before the timed workload.
	runtime.GC()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := loop.Submit(func() {
			counter.Add(1)
			tasks.Done()
		}); err != nil {
			tasks.Done()
			b.Fatalf("Submit: %v", err)
		}

		// Trigger GC periodically
		if i%1000 == 0 {
			runtime.GC()
		}
	}

	waitBenchmarkBarrier(b, tasks, deadline.C, "GC-pressure callback drain")
	b.StopTimer()
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

	loop, _ := startTournamentTestLoop(t, impl)

	var executed atomic.Int64
	var rejected atomic.Int64
	tasks := newTournamentTestBarrier(numTasks)

	// Aggressive GC goroutine
	gcStop := make(chan struct{})
	gcDone := make(chan struct{})
	var gcStopOnce sync.Once
	stopGC := func() {
		gcStopOnce.Do(func() {
			close(gcStop)
			waitTournamentSignal(t, gcDone, "GC worker exit")
		})
	}
	t.Cleanup(stopGC)
	go func() {
		defer close(gcDone)
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-gcStop:
				return
			case <-ticker.C:
				runtime.GC()
			}
		}
	}()

	// Submit tasks
	for range numTasks {
		err := loop.Submit(func() {
			executed.Add(1)
			tasks.Done()
		})
		if err != nil {
			rejected.Add(1)
			tasks.Done()
		}
	}

	waitTournamentSignal(t, tasks.done, "tasks under GC pressure")
	stopGC()

	exec := executed.Load()
	rej := rejected.Load()
	passed := exec == numTasks && rej == 0

	result := TestResult{
		TestName:       "GCPressure_Correctness",
		VariantID:      impl.VariantID,
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
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()

	tasks := newBenchmarkBarrier(b.N)
	var counter atomic.Int64
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := loop.Submit(func() {
			counter.Add(1)
			tasks.Done()
		}); err != nil {
			tasks.Done()
			b.Fatalf("Submit: %v", err)
		}
		if i%1000 == 0 {
			runtime.GC()
		}
	}

	waitBenchmarkBarrier(b, tasks, deadline.C, "allocation callback drain")
	b.StopTimer()

}

// TestHeapGrowthDiagnostic records process heap samples after repeated loop
// workloads. Runtime-wide MemStats are diagnostic evidence, not a leak oracle.
func TestHeapGrowthDiagnostic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak test in short mode")
	}

	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			testHeapGrowthDiagnostic(t, impl)
		})
	}
}

func testHeapGrowthDiagnostic(t *testing.T, impl Implementation) {
	const iterations = 3
	const tasksPerIteration = 10000

	start := time.Now()

	var memStats []uint64

	for range iterations {
		loop, cleanup := startTournamentTestLoop(t, impl)

		tasks := newTournamentTestBarrier(tasksPerIteration)
		submitErrors := make(chan error, 1)
		for range tasksPerIteration {
			if err := loop.Submit(tasks.Done); err != nil {
				tasks.Done()
				select {
				case submitErrors <- err:
				default:
				}
			}
		}
		waitTournamentSignal(t, tasks.done, "heap-growth iteration callback drain")
		select {
		case err := <-submitErrors:
			t.Fatalf("Submit: %v", err)
		default:
		}
		cleanup()

		// Force GC and measure memory
		runtime.GC()
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		memStats = append(memStats, m.Alloc)
	}

	result := TestResult{
		TestName:       "HeapGrowthDiagnostic",
		VariantID:      impl.VariantID,
		Implementation: impl.Name,
		Status:         TestStatusDiagnostic,
		Duration:       time.Since(start),
		Metrics: map[string]any{
			"iterations":   iterations,
			"memory_bytes": memStats,
		},
	}
	GetResults().RecordTest(result)

	t.Logf("%s: Memory samples (bytes): %v", impl.Name, memStats)
}
