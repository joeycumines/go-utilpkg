package tournament

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkMultiProducer measures throughput with 10 producers submitting 1M total tasks.
// This is T4: Performance - Multi-Producer Stress Benchmark
func BenchmarkMultiProducer(b *testing.B) {
	for _, impl := range Implementations() {
		impl := impl
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkMultiProducer(b, impl)
		})
	}
}

func benchmarkMultiProducer(b *testing.B, impl Implementation) {
	const numProducers = 10
	tasksPerProducer := b.N / numProducers
	if tasksPerProducer == 0 {
		tasksPerProducer = 1
	}

	loop, err := impl.Factory()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	if err := loop.Start(ctx); err != nil {
		b.Fatalf("Failed to start loop: %v", err)
	}

	var wg sync.WaitGroup
	var counter atomic.Int64
	var rejected atomic.Int64

	b.ResetTimer()

	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < tasksPerProducer; i++ {
				err := loop.Submit(func() {
					counter.Add(1)
				})
				if err != nil {
					rejected.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	// Wait for all tasks to complete
	for {
		c := counter.Load()
		r := rejected.Load()
		total := int64(numProducers * tasksPerProducer)
		if c+r >= total {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Stop(stopCtx)

	totalTasks := numProducers * tasksPerProducer
	result := BenchmarkResult{
		BenchmarkName:  "MultiProducer",
		Implementation: impl.Name,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(totalTasks),
		Iterations:     totalTasks,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}

// TestMultiProducerStress is a test variant that measures latency distribution.
func TestMultiProducerStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	for _, impl := range Implementations() {
		impl := impl
		t.Run(impl.Name, func(t *testing.T) {
			testMultiProducerStress(t, impl)
		})
	}
}

func testMultiProducerStress(t *testing.T, impl Implementation) {
	const numProducers = 10
	const totalTasks = 100000 // 100K for test mode
	const tasksPerProducer = totalTasks / numProducers

	start := time.Now()

	loop, err := impl.Factory()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	if err := loop.Start(ctx); err != nil {
		t.Fatalf("Failed to start loop: %v", err)
	}

	var wg sync.WaitGroup
	var counter atomic.Int64
	var rejected atomic.Int64

	// Track latencies (approximate P99)
	latencies := make([]time.Duration, 0, 1000) // Sample
	var latMu sync.Mutex
	sampleRate := totalTasks / 1000

	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func(pid int) {
			defer wg.Done()
			for i := 0; i < tasksPerProducer; i++ {
				submitTime := time.Now()
				taskID := pid*tasksPerProducer + i

				err := loop.Submit(func() {
					counter.Add(1)
					if taskID%sampleRate == 0 {
						lat := time.Since(submitTime)
						latMu.Lock()
						latencies = append(latencies, lat)
						latMu.Unlock()
					}
				})
				if err != nil {
					rejected.Add(1)
				}
			}
		}(p)
	}

	wg.Wait()

	// Wait for all tasks to complete
	timeout := time.After(30 * time.Second)
	for {
		c := counter.Load()
		r := rejected.Load()
		if c+r >= totalTasks {
			break
		}
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for tasks: completed=%d, rejected=%d, expected=%d",
				c, r, totalTasks)
		case <-time.After(10 * time.Millisecond):
		}
	}

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Stop(stopCtx)

	duration := time.Since(start)
	exec := counter.Load()
	rej := rejected.Load()
	throughput := float64(exec) / duration.Seconds()

	// Calculate P99 latency (approximate - find max in top 1%)
	var p99 time.Duration
	if len(latencies) > 0 {
		// Find max latency as approximate P99
		for _, l := range latencies {
			if l > p99 {
				p99 = l
			}
		}
	}

	result := TestResult{
		TestName:       "MultiProducerStress",
		Implementation: impl.Name,
		Passed:         exec+rej == totalTasks,
		Duration:       duration,
		Metrics: map[string]interface{}{
			"total_tasks":      totalTasks,
			"executed":         exec,
			"rejected":         rej,
			"throughput_ops_s": throughput,
			"p99_latency_us":   float64(p99.Microseconds()),
			"sample_count":     len(latencies),
		},
	}
	GetResults().RecordTest(result)

	t.Logf("%s: Throughput=%.0f ops/s, Executed=%d, Rejected=%d, P99≈%v",
		impl.Name, throughput, exec, rej, p99)
}

// BenchmarkMultiProducerContention measures performance under high contention.
func BenchmarkMultiProducerContention(b *testing.B) {
	const numProducers = 100 // High contention

	for _, impl := range Implementations() {
		impl := impl
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkMultiProducerContention(b, impl, numProducers)
		})
	}
}

func benchmarkMultiProducerContention(b *testing.B, impl Implementation, numProducers int) {
	tasksPerProducer := b.N / numProducers
	if tasksPerProducer == 0 {
		tasksPerProducer = 1
	}

	loop, err := impl.Factory()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	if err := loop.Start(ctx); err != nil {
		b.Fatalf("Failed to start loop: %v", err)
	}

	var wg sync.WaitGroup
	var counter atomic.Int64

	b.ResetTimer()

	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < tasksPerProducer; i++ {
				_ = loop.Submit(func() {
					counter.Add(1)
				})
			}
		}()
	}

	wg.Wait()

	// Brief wait for execution
	time.Sleep(10 * time.Millisecond)

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Stop(stopCtx)

	totalTasks := numProducers * tasksPerProducer
	result := BenchmarkResult{
		BenchmarkName:  "MultiProducerContention",
		Implementation: impl.Name,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(totalTasks),
		Iterations:     totalTasks,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}
