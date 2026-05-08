package tournament

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// BenchmarkMicroCASContention measures exact submission cost as producer count
// increases. Callback drain is verified after timing so a scheduler cannot look
// fast by rejecting or losing accepted work.
func BenchmarkMicroCASContention(b *testing.B) {
	producerCounts := []int{1, 2, 4, 8, 16, 32}
	for _, impl := range Implementations() {
		for _, producerCount := range producerCounts {
			b.Run(fmt.Sprintf("%s/N=%02d", impl.Name, producerCount), func(b *testing.B) {
				benchmarkCASContention(b, impl, producerCount)
			})
		}
	}
}

func benchmarkCASContention(b *testing.B, impl Implementation, producerCount int) {
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()

	producers := newBenchmarkProducerGroup(b, producerCount)
	tasks := newBenchmarkBarrier(b.N)
	submitErrors := make(chan error, 1)
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	b.ReportAllocs()
	b.ResetTimer()
	for producer := range producerCount {
		taskCount := b.N / producerCount
		if producer < b.N%producerCount {
			taskCount++
		}
		producers.Go(func(stop <-chan struct{}) {
			for range taskCount {
				select {
				case <-stop:
					return
				default:
				}
				if err := loop.Submit(tasks.Done); err != nil {
					tasks.Done()
					select {
					case submitErrors <- err:
					default:
					}
				}
			}
		})
	}
	waitBenchmarkDeadline(b, producers.Done(), deadline.C, "CAS-contention producer exit")
	b.StopTimer()
	waitBenchmarkBarrier(b, tasks, deadline.C, "CAS-contention callback drain")
	select {
	case err := <-submitErrors:
		b.Fatalf("Submit: %v", err)
	default:
	}
}

// BenchmarkMicroCASContention_Latency measures end-to-end task latency and
// reports sorted percentile observations for every scheduler variant.
func BenchmarkMicroCASContention_Latency(b *testing.B) {
	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkCASLatency(b, impl)
		})
	}
}

func benchmarkCASLatency(b *testing.B, impl Implementation) {
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()

	latencies := make([]time.Duration, 0, b.N)
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		started := time.Now()
		done := make(chan struct{})
		if err := loop.Submit(func() { close(done) }); err != nil {
			b.Fatalf("Submit: %v", err)
		}
		waitBenchmarkDeadline(b, done, deadline.C, "CAS-latency callback")
		latencies = append(latencies, time.Since(started))
	}
	b.StopTimer()

	slices.Sort(latencies)
	if len(latencies) == 0 {
		return
	}
	b.ReportMetric(float64(latencies[percentileIndex(len(latencies), 50)].Nanoseconds()), "p50_ns")
	b.ReportMetric(float64(latencies[percentileIndex(len(latencies), 95)].Nanoseconds()), "p95_ns")
	b.ReportMetric(float64(latencies[percentileIndex(len(latencies), 99)].Nanoseconds()), "p99_ns")
}

func percentileIndex(length, percentile int) int {
	if length <= 0 {
		return 0
	}
	index := (length*percentile+99)/100 - 1
	return min(max(index, 0), length-1)
}
