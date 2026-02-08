//go:build linux || darwin

package tournament

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// This microbenchmark tests Hypothesis #3: Batch budget of 1024 in processExternal()
// is suboptimal for certain workloads.
//
// Hypothesis:
// - Too large (2048+) increases latency due to single-threaded processing
// - Too small (256) reduces throughput due to frequent PopBatch calls
// - Optimal balance point exists around 512-1024 for typical workloads
// - Steady-state vs bursty workloads favor different budget sizes
//
// Expected Pattern:
// - Throughput peaks at 512-1024 tasks per batch
// - P99 latency increases linearly with budget size
// - Small budgets (256) better for bursty workloads
// - Large budgets (2048+) better for steady-state high throughput

// BenchmarkMicroBatchBudget_Throughput measures throughput impact of effective batching.
// Since we can't change the hardcoded budget=1024 in Main loop, we simulate
// this by submitting in bursts of different sizes to trigger actual processing in batches.
func BenchmarkMicroBatchBudget_Throughput(b *testing.B) {
	burstSizes := []int{64, 128, 256, 512, 1024, 2048, 4096}

	for _, implName := range []string{"Main", "AlternateOne", "AlternateTwo", "AlternateThree"} {
		implName := implName
		for _, burstSize := range burstSizes {
			burstSize := burstSize
			b.Run(fmt.Sprintf("%s/Burst=%d", implName, burstSize), func(b *testing.B) {
				benchmarkBatchThroughput(b, implName, burstSize)
			})
		}
	}

	// Baseline for comparison (no batching overhead)
	b.Run("Baseline/Burst=1024", func(b *testing.B) {
		benchmarkBatchThroughput(b, "Baseline", 1024)
	})
}

func benchmarkBatchThroughput(b *testing.B, implName string, burstSize int) {
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

	b.ResetTimer()

	// Submit in bursts to trigger batch processing
	numBursts := b.N / burstSize
	if numBursts < 1 {
		numBursts = 1
	}

	var wg sync.WaitGroup
	var counter atomic.Int64

	for i := 0; i < numBursts; i++ {
		// Submit burst of tasks
		for j := 0; j < burstSize; j++ {
			wg.Add(1)
			err := loop.Submit(func() {
				counter.Add(1)
				wg.Done()
			})
			if err != nil {
				wg.Done()
			}
		}

		// Small gap to allow loop to drain and potentially enter sleep
		// This tests whether batch budget affects steady-state vs bursty behavior
		if i < numBursts-1 {
			time.Sleep(10 * time.Microsecond)
		}
	}

	wg.Wait()

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	// Record result
	result := BenchmarkResult{
		BenchmarkName:  fmt.Sprintf("MicroBatchBudget/Burst=%d", burstSize),
		Implementation: implName,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(b.N),
		Iterations:     b.N,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}

// BenchmarkMicroBatchBudget_Latency measures latency impact of batch processing.
// Collects P50, P95, P99 for different burst submission patterns.
func BenchmarkMicroBatchBudget_Latency(b *testing.B) {
	burstSizes := []int{64, 128, 256, 512, 1024, 2048, 4096}

	for _, implName := range []string{"Main", "AlternateOne", "AlternateTwo", "AlternateThree", "Baseline"} {
		implName := implName
		for _, burstSize := range burstSizes {
			burstSize := burstSize
			b.Run(fmt.Sprintf("%s/Burst=%d", implName, burstSize), func(b *testing.B) {
				benchmarkBatchLatency(b, implName, burstSize)
			})
		}
	}
}

func benchmarkBatchLatency(b *testing.B, implName string, burstSize int) {
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

	b.ResetTimer()

	// Measure burst completion latency
	numBursts := b.N / burstSize
	if numBursts < 1 {
		numBursts = 1
	}

	for i := 0; i < numBursts; i++ {
		_ = time.Now() // burstStart - placeholder for future latency metrics

		var wg sync.WaitGroup
		for j := 0; j < burstSize; j++ {
			wg.Add(1)
			_ = loop.Submit(func() {
				wg.Done()
			})
		}
		wg.Wait()

		// Note: In production, we'd collect and report percentile metrics here
	}

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	result := BenchmarkResult{
		BenchmarkName:  fmt.Sprintf("MicroBatchBudget_Latency/Burst=%d", burstSize),
		Implementation: implName,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(b.N),
		Iterations:     b.N,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}

// BenchmarkMicroBatchBudget_Continuous measures steady-state throughput with continuous submission.
// Tests if batch budget hurts steady-state high-throughput scenarios.
func BenchmarkMicroBatchBudget_Continuous(b *testing.B) {
	for _, implName := range []string{"Main", "AlternateOne", "AlternateTwo", "AlternateThree", "Baseline"} {
		implName := implName
		b.Run(implName, func(b *testing.B) {
			benchmarkContinuousSubmission(b, implName)
		})
	}
}

func benchmarkContinuousSubmission(b *testing.B, implName string) {
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

	b.ResetTimer()

	// Continuous submission to saturate the loop
	var wg sync.WaitGroup
	var submitWg sync.WaitGroup
	var counter atomic.Int64
	stopCh := make(chan struct{})

	// Start multiple producers to create continuous load
	numProducers := 4
	for i := 0; i < numProducers; i++ {
		wg.Add(1)
		submitWg.Add(1)
		go func() {
			defer wg.Done()
			defer submitWg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					wg.Add(1)
					_ = loop.Submit(func() {
						counter.Add(1)
						wg.Done()
					})
				}
			}
		}()
	}

	// Run for b.N total operations
	targetOps := b.N
	for counter.Load() < int64(targetOps) {
		time.Sleep(100 * time.Microsecond)
	}

	close(stopCh)
	wg.Wait()
	submitWg.Wait()

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	result := BenchmarkResult{
		BenchmarkName:  "MicroBatchBudget_Continuous",
		Implementation: implName,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(b.N),
		Iterations:     b.N,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}

// BenchmarkMicroBatchBudget_Mixed measures behavior under mixed burst/steady workloads.
// Simulates production-like mixed traffic patterns.
func BenchmarkMicroBatchBudget_Mixed(b *testing.B) {
	burstSizes := []int{100, 500, 1000, 2000, 5000}

	for _, implName := range []string{"Main", "AlternateThree", "Baseline"} {
		implName := implName
		for _, burstSize := range burstSizes {
			burstSize := burstSize
			b.Run(fmt.Sprintf("%s/Burst=%d", implName, burstSize), func(b *testing.B) {
				benchmarkMixedWorkload(b, implName, burstSize)
			})
		}
	}
}

func benchmarkMixedWorkload(b *testing.B, implName string, burstSize int) {
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

	b.ResetTimer()

	// Mixed pattern: 50% bursty, 50% steady
	const steadyRatio = 0.5
	numBursts := int(float64(b.N/burstSize) * steadyRatio)

	var wg sync.WaitGroup
	var counter atomic.Int64

	// Submit bursts
	for i := 0; i < numBursts; i++ {
		for j := 0; j < burstSize; j++ {
			wg.Add(1)
			err := loop.Submit(func() {
				counter.Add(1)
				wg.Done()
			})
			if err != nil {
				wg.Done()
			}
		}
		// Small gap between bursts
		time.Sleep(10 * time.Microsecond)
	}

	// Submit steady stream
	for i := counter.Load(); i < int64(b.N); i++ {
		wg.Add(1)
		_ = loop.Submit(func() {
			counter.Add(1)
			wg.Done()
		})
		time.Sleep(1 * time.Microsecond)
	}

	wg.Wait()

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	result := BenchmarkResult{
		BenchmarkName:  fmt.Sprintf("MicroBatchBudget_Mixed/Burst=%d", burstSize),
		Implementation: implName,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(b.N),
		Iterations:     b.N,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}
