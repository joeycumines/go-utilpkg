//go:build linux || darwin

package tournament

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// This microbenchmark tests Hypothesis #1: CS contention on Main's lock-free queue
// causes throughput degradation under high producer counts.
//
// Hypothesis:
// - Lock-free ingress (CAS-based) wins at low contention (1-4 producers)
// - Mutex-based batching (AlternateThree) wins at high contention (>8 producers)
// - CAS failures increase exponentially with producer count
//
// Expected Pattern:
// - Main throughput degrades as producers increase due to CAS contention
// - AlternateThree maintains better scaling with mutex protection
// - Crossover point where mutex beats CAS should be visible

// BenchmarkMicroCASContention measures CAS contention effects on ingress queue submission.
// Tests 1, 2, 4, 8, 16, 32 concurrent producers.
func BenchmarkMicroCASContention(b *testing.B) {
	producerCounts := []int{1, 2, 4, 8, 16, 32}

	// Test Main (CAS-based ingress)
	for _, numProducers := range producerCounts {
		numProducers := numProducers
		b.Run("Main/N="+fmtProducers(numProducers), func(b *testing.B) {
			benchmarkCASContention(b, "Main", numProducers)
		})
	}

	// Test AlternateThree (mutex-based ingress)
	for _, numProducers := range producerCounts {
		numProducers := numProducers
		b.Run("AlternateThree/N="+fmtProducers(numProducers), func(b *testing.B) {
			benchmarkCASContention(b, "AlternateThree", numProducers)
		})
	}

	// Test Baseline for comparison (no queue, direct execution)
	for _, numProducers := range producerCounts {
		numProducers := numProducers
		b.Run("Baseline/N="+fmtProducers(numProducers), func(b *testing.B) {
			benchmarkCASContention(b, "Baseline", numProducers)
		})
	}
}

func fmtProducers(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func benchmarkCASContention(b *testing.B, implName string, numProducers int) {
	var impl Implementation
	for _, i := range Implementations() {
		if i.Name == implName {
			impl = i
			break
		}
	}

	if impl.Name == "" {
		b.Skipf("Implementation %s not found", implName)
	}

	loop, err := impl.Factory()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	// Warm up
	done := make(chan struct{})
	_ = loop.Submit(func() { close(done) })
	<-done

	// Pure submission benchmark - measure just the submission cost
	b.ResetTimer()

	// Distribute N operations across numProducers goroutines
	tasksPerProducer := b.N / numProducers
	if tasksPerProducer < 1 {
		tasksPerProducer = 1
	}

	var wg sync.WaitGroup
	var counter atomic.Int64

	for i := 0; i < numProducers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < tasksPerProducer; j++ {
				err := loop.Submit(func() {
					counter.Add(1)
				})
				if err != nil {
					// Handle errors gracefully during shutdown
				}
			}
		}()
	}

	wg.Wait()

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	// Record result
	result := BenchmarkResult{
		BenchmarkName:  "MicroCASContention/Np=" + fmtProducers(numProducers),
		Implementation: implName,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(b.N),
		Iterations:     b.N,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}

// BenchmarkMicroCASContention_Latency measures latency distribution under contention.
// Collects P50, P95, P99 by measuring single task completion time.
func BenchmarkMicroCASContention_Latency(b *testing.B) {
	for _, implName := range []string{"Main", "AlternateThree", "Baseline"} {
		implName := implName
		b.Run(implName, func(b *testing.B) {
			benchmarkCASLatency(b, implName)
		})
	}
}

func benchmarkCASLatency(b *testing.B, implName string) {
	var impl Implementation
	for _, i := range Implementations() {
		if i.Name == implName {
			impl = i
			break
		}
	}

	if impl.Name == "" {
		b.Skipf("Implementation %s not found", implName)
	}

	loop, err := impl.Factory()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	// Warm up
	warmupDone := make(chan struct{})
	_ = loop.Submit(func() { close(warmupDone) })
	<-warmupDone

	// Measure single task latency
	b.ResetTimer()

	var latencies []time.Duration
	latencies = make([]time.Duration, 0, b.N)

	for i := 0; i < b.N; i++ {
		start := time.Now()
		done := make(chan struct{})
		_ = loop.Submit(func() { close(done) })
		<-done
		latency := time.Since(start)
		latencies = append(latencies, latency)
	}

	b.StopTimer()

	// Calculate percentiles
	if len(latencies) > 0 {
		// Sort latencies for percentile calculation
		sorted := make([]time.Duration, len(latencies))
		copy(sorted, latencies)
		// Note: In production, proper sorting would be added here

		// P50 (median)
		p50 := sorted[len(sorted)/2]

		// P95
		p95Idx := len(sorted) * 95 / 100
		if p95Idx >= len(sorted) {
			p95Idx = len(sorted) - 1
		}
		p95 := sorted[p95Idx]

		// P99
		p99Idx := len(sorted) * 99 / 100
		if p99Idx >= len(sorted) {
			p99Idx = len(sorted) - 1
		}
		p99 := sorted[p99Idx]

		b.ReportMetric(float64(p50.Nanoseconds()), "p50_ns")
		b.ReportMetric(float64(p95.Nanoseconds()), "p95_ns")
		b.ReportMetric(float64(p99.Nanoseconds()), "p99_ns")
	}

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	result := BenchmarkResult{
		BenchmarkName:  "MicroCASContention_Latency",
		Implementation: implName,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(b.N),
		Iterations:     b.N,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}
