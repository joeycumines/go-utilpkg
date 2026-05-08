package tournament

import (
	"fmt"
	"testing"
	"time"
)

var batchSizes = []int{64, 128, 256, 512, 1024, 2048, 4096}

// BenchmarkMicroBatchBudget_Throughput measures admission throughput for
// sustained fixed-size batches. Accepted callback drain is verified untimed.
func BenchmarkMicroBatchBudget_Throughput(b *testing.B) {
	for _, impl := range Implementations() {
		for _, batchSize := range batchSizes {
			b.Run(fmt.Sprintf("%s/Burst=%d", impl.Name, batchSize), func(b *testing.B) {
				benchmarkBatchThroughput(b, impl, batchSize)
			})
		}
	}
}

func benchmarkBatchThroughput(b *testing.B, impl Implementation, batchSize int) {
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()

	tasks := newBenchmarkBarrier(b.N * batchSize)
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	b.ReportAllocs()
	b.ReportMetric(float64(batchSize), "tasks/op")
	b.ResetTimer()
	for range b.N {
		for range batchSize {
			if err := loop.Submit(tasks.Done); err != nil {
				tasks.Done()
				b.Fatalf("Submit: %v", err)
			}
		}
	}
	b.StopTimer()
	waitBenchmarkBarrier(b, tasks, deadline.C, "batch-throughput callback drain")
}

// BenchmarkMicroBatchBudget_Latency measures complete fixed-size batch
// admission and execution. Each reported operation is one entire batch.
func BenchmarkMicroBatchBudget_Latency(b *testing.B) {
	for _, impl := range Implementations() {
		for _, batchSize := range batchSizes {
			b.Run(fmt.Sprintf("%s/Burst=%d", impl.Name, batchSize), func(b *testing.B) {
				benchmarkBatchLatency(b, impl, batchSize)
			})
		}
	}
}

func benchmarkBatchLatency(b *testing.B, impl Implementation, batchSize int) {
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	b.ReportAllocs()
	b.ReportMetric(float64(batchSize), "tasks/op")
	b.ResetTimer()
	for range b.N {
		tasks := newBenchmarkBarrier(batchSize)
		for range batchSize {
			if err := loop.Submit(tasks.Done); err != nil {
				tasks.Done()
				b.Fatalf("Submit: %v", err)
			}
		}
		waitBenchmarkBarrier(b, tasks, deadline.C, "batch-latency callback drain")
	}
}

// BenchmarkMicroBatchBudget_Continuous measures exact end-to-end throughput
// with four concurrent producers and no timing guesses or polling loops.
func BenchmarkMicroBatchBudget_Continuous(b *testing.B) {
	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkContinuousSubmission(b, impl)
		})
	}
}

func benchmarkContinuousSubmission(b *testing.B, impl Implementation) {
	const producerCount = 4
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
	waitBenchmarkDeadline(b, producers.Done(), deadline.C, "continuous producer exit")
	waitBenchmarkBarrier(b, tasks, deadline.C, "continuous callback drain")
	b.StopTimer()
	select {
	case err := <-submitErrors:
		b.Fatalf("Submit: %v", err)
	default:
	}
}

// BenchmarkMicroBatchBudget_Mixed measures an equal mix of queued burst work
// and sequential round trips. One operation contains batchSize total tasks.
func BenchmarkMicroBatchBudget_Mixed(b *testing.B) {
	mixedSizes := []int{100, 500, 1000, 2000, 5000}
	for _, impl := range Implementations() {
		for _, batchSize := range mixedSizes {
			b.Run(fmt.Sprintf("%s/Burst=%d", impl.Name, batchSize), func(b *testing.B) {
				benchmarkMixedWorkload(b, impl, batchSize)
			})
		}
	}
}

func benchmarkMixedWorkload(b *testing.B, impl Implementation, batchSize int) {
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()
	timer := time.NewTimer(30 * time.Minute)
	defer timer.Stop()

	burstCount := batchSize / 2
	steadyCount := batchSize - burstCount
	b.ReportAllocs()
	b.ReportMetric(float64(batchSize), "tasks/op")
	b.ResetTimer()
	for range b.N {
		burst := newBenchmarkBarrier(burstCount)
		for range burstCount {
			if err := loop.Submit(burst.Done); err != nil {
				burst.Done()
				b.Fatalf("burst Submit: %v", err)
			}
		}
		waitBenchmarkBarrier(b, burst, timer.C, "mixed burst callback drain")

		for range steadyCount {
			done := make(chan struct{})
			if err := loop.Submit(func() { close(done) }); err != nil {
				b.Fatalf("steady Submit: %v", err)
			}
			waitBenchmarkDeadline(b, done, timer.C, "mixed steady callback")
		}
	}
}
