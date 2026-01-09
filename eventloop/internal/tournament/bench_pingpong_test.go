package tournament

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkPingPong measures single producer, single consumer throughput.
// This is T3: Performance - Ping-Pong Throughput Benchmark
func BenchmarkPingPong(b *testing.B) {
	for _, impl := range Implementations() {
		impl := impl
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkPingPong(b, impl)
		})
	}
}

func benchmarkPingPong(b *testing.B, impl Implementation) {
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

	for i := 0; i < b.N; i++ {
		wg.Add(1)
		err := loop.Submit(func() {
			counter.Add(1)
			wg.Done()
		})
		if err != nil {
			wg.Done()
			continue
		}
	}

	wg.Wait()
	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Stop(stopCtx)

	// Record benchmark result
	result := BenchmarkResult{
		BenchmarkName:  "PingPong",
		Implementation: impl.Name,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(b.N),
		Iterations:     b.N,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}

// BenchmarkPingPongLatency measures end-to-end latency for single tasks.
func BenchmarkPingPongLatency(b *testing.B) {
	for _, impl := range Implementations() {
		impl := impl
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkPingPongLatency(b, impl)
		})
	}
}

func benchmarkPingPongLatency(b *testing.B, impl Implementation) {
	loop, err := impl.Factory()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	if err := loop.Start(ctx); err != nil {
		b.Fatalf("Failed to start loop: %v", err)
	}

	// Warm up
	done := make(chan struct{})
	_ = loop.Submit(func() { close(done) })
	<-done

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		_ = loop.Submit(func() { close(done) })
		<-done
	}

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Stop(stopCtx)

	result := BenchmarkResult{
		BenchmarkName:  "PingPongLatency",
		Implementation: impl.Name,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(b.N),
		Iterations:     b.N,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}

// BenchmarkBurstSubmit measures throughput when submitting in bursts.
func BenchmarkBurstSubmit(b *testing.B) {
	const burstSize = 1000

	for _, impl := range Implementations() {
		impl := impl
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkBurstSubmit(b, impl, burstSize)
		})
	}
}

func benchmarkBurstSubmit(b *testing.B, impl Implementation, burstSize int) {
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

	bursts := b.N / burstSize
	if bursts == 0 {
		bursts = 1
	}

	for burst := 0; burst < bursts; burst++ {
		wg.Add(burstSize)
		for i := 0; i < burstSize; i++ {
			err := loop.Submit(func() {
				counter.Add(1)
				wg.Done()
			})
			if err != nil {
				wg.Done()
			}
		}
		wg.Wait()
	}

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Stop(stopCtx)

	totalOps := bursts * burstSize
	result := BenchmarkResult{
		BenchmarkName:  "BurstSubmit",
		Implementation: impl.Name,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(totalOps),
		Iterations:     totalOps,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}
