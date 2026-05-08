package tournament

import (
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkPingPong measures single producer, single consumer throughput.
// This is T3: Performance - Ping-Pong Throughput Benchmark
func BenchmarkPingPong(b *testing.B) {
	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkPingPong(b, impl)
		})
	}
}

func benchmarkPingPong(b *testing.B, impl Implementation) {
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()

	tasks := newBenchmarkBarrier(b.N)
	var counter atomic.Int64
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := loop.Submit(func() {
			counter.Add(1)
			tasks.Done()
		}); err != nil {
			tasks.Done()
			b.Fatalf("Submit: %v", err)
		}
	}

	waitBenchmarkBarrier(b, tasks, deadline.C, "ping-pong callback drain")
	b.StopTimer()
	if got := counter.Load(); got != int64(b.N) {
		b.Fatalf("executed callbacks = %d, want %d", got, b.N)
	}
}

// BenchmarkPingPongLatency measures end-to-end latency for single tasks.
func BenchmarkPingPongLatency(b *testing.B) {
	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkPingPongLatency(b, impl)
		})
	}
}

func benchmarkPingPongLatency(b *testing.B, impl Implementation) {
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		if err := loop.Submit(func() { close(done) }); err != nil {
			b.Fatalf("Submit: %v", err)
		}
		waitBenchmarkDeadline(b, done, deadline.C, "ping-pong completion")
	}

	b.StopTimer()

}

// BenchmarkBurstSubmit measures throughput when submitting in bursts.
func BenchmarkBurstSubmit(b *testing.B) {
	const burstSize = 1000

	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkBurstSubmit(b, impl, burstSize)
		})
	}
}

func benchmarkBurstSubmit(b *testing.B, impl Implementation, burstSize int) {
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()

	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	var counter atomic.Int64
	b.ReportMetric(float64(burstSize), "max_tasks/burst")

	b.ResetTimer()

	remaining := b.N
	for remaining > 0 {
		count := min(remaining, burstSize)
		tasks := newBenchmarkBarrier(count)
		for range count {
			if err := loop.Submit(func() {
				counter.Add(1)
				tasks.Done()
			}); err != nil {
				tasks.Done()
				b.Fatalf("Submit: %v", err)
			}
		}
		waitBenchmarkBarrier(b, tasks, deadline.C, "burst callback drain")
		remaining -= count
	}

	b.StopTimer()
	if got := counter.Load(); got != int64(b.N) {
		b.Fatalf("executed callbacks = %d, want %d", got, b.N)
	}
}
