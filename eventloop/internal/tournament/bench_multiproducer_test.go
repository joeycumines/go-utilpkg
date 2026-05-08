package tournament

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkMultiProducer measures throughput with 10 producers submitting 1M total tasks.
// This is T4: Performance - Multi-Producer Stress Benchmark
func BenchmarkMultiProducer(b *testing.B) {
	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkMultiProducer(b, impl)
		})
	}
}

func benchmarkMultiProducer(b *testing.B, impl Implementation) {
	const numProducers = 10
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()

	producers := newBenchmarkProducerGroup(b, numProducers)
	tasks := newBenchmarkBarrier(b.N)
	submitErrors := make(chan error, 1)
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	b.ResetTimer()

	for producer := range numProducers {
		taskCount := b.N / numProducers
		if producer < b.N%numProducers {
			taskCount++
		}
		producers.Go(func(stop <-chan struct{}) {
			for range taskCount {
				select {
				case <-stop:
					return
				default:
				}
				err := loop.Submit(tasks.Done)
				if err != nil {
					tasks.Done()
					select {
					case submitErrors <- err:
					default:
					}
				}
			}
		})
	}

	waitBenchmarkDeadline(b, producers.Done(), deadline.C, "multi-producer exit")
	waitBenchmarkBarrier(b, tasks, deadline.C, "multi-producer callback drain")

	b.StopTimer()
	select {
	case err := <-submitErrors:
		b.Fatalf("Submit: %v", err)
	default:
	}

}

// TestMultiProducerStress is a test variant that measures latency distribution.
func TestMultiProducerStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			testMultiProducerStress(t, impl)
		})
	}
}

func testMultiProducerStress(t *testing.T, impl Implementation) {
	const numProducers = 10
	const totalTasks = 100000 // 100K for test mode
	const tasksPerProducer = totalTasks / numProducers

	loop, cleanup := startTournamentTestLoop(t, impl)

	producerDone := make(chan struct{}, numProducers)
	tasks := newTournamentTestBarrier(totalTasks)
	var counter atomic.Int64
	var rejected atomic.Int64
	submitErrors := make(chan error, 1)

	// Track latencies (approximate P99)
	latencies := make([]time.Duration, 0, 1000) // Sample
	var latMu sync.Mutex
	sampleRate := totalTasks / 1000
	start := time.Now()

	for p := range numProducers {
		go func(pid int) {
			defer func() { producerDone <- struct{}{} }()
			for i := range tasksPerProducer {
				submitTime := time.Now()
				taskID := pid*tasksPerProducer + i

				err := loop.Submit(func() {
					defer tasks.Done()
					counter.Add(1)
					if taskID%sampleRate == 0 {
						lat := time.Since(submitTime)
						latMu.Lock()
						latencies = append(latencies, lat)
						latMu.Unlock()
					}
				})
				if err != nil {
					tasks.Done()
					rejected.Add(1)
					select {
					case submitErrors <- err:
					default:
					}
				}
			}
		}(p)
	}

	waitTournamentCount(t, producerDone, numProducers, "multi-producer exit")
	waitTournamentSignal(t, tasks.done, "multi-producer callback drain")
	duration := time.Since(start)
	cleanup()
	exec := counter.Load()
	rej := rejected.Load()
	throughput := float64(exec) / duration.Seconds()

	// Calculate the sampled P99 latency.
	var p99 time.Duration
	if len(latencies) > 0 {
		slices.Sort(latencies)
		p99 = latencies[percentileIndex(len(latencies), 99)]
	}

	result := TestResult{
		TestName:       "MultiProducerStress",
		VariantID:      impl.VariantID,
		Implementation: impl.Name,
		Passed:         exec == totalTasks && rej == 0,
		Duration:       duration,
		Metrics: map[string]any{
			"total_tasks":      totalTasks,
			"executed":         exec,
			"rejected":         rej,
			"throughput_ops_s": throughput,
			"p99_latency_us":   float64(p99.Microseconds()),
			"sample_count":     len(latencies),
		},
	}
	GetResults().RecordTest(result)
	select {
	case err := <-submitErrors:
		t.Errorf("Submit: %v", err)
	default:
	}

	t.Logf("%s: Throughput=%.0f ops/s, Executed=%d, Rejected=%d, P99≈%v",
		impl.Name, throughput, exec, rej, p99)
}

// BenchmarkMultiProducerContention measures performance under high contention.
func BenchmarkMultiProducerContention(b *testing.B) {
	const numProducers = 100 // High contention

	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkMultiProducerContention(b, impl, numProducers)
		})
	}
}

func benchmarkMultiProducerContention(b *testing.B, impl Implementation, numProducers int) {
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()

	producers := newBenchmarkProducerGroup(b, numProducers)
	tasks := newBenchmarkBarrier(b.N)
	submitErrors := make(chan error, 1)
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	b.ResetTimer()

	for producer := range numProducers {
		taskCount := b.N / numProducers
		if producer < b.N%numProducers {
			taskCount++
		}
		producers.Go(func(stop <-chan struct{}) {
			for range taskCount {
				select {
				case <-stop:
					return
				default:
				}
				err := loop.Submit(tasks.Done)
				if err != nil {
					tasks.Done()
					select {
					case submitErrors <- err:
					default:
					}
				}
			}
		})
	}

	waitBenchmarkDeadline(b, producers.Done(), deadline.C, "contention producer exit")
	waitBenchmarkBarrier(b, tasks, deadline.C, "contention callback drain")

	b.StopTimer()
	select {
	case err := <-submitErrors:
		b.Fatalf("Submit: %v", err)
	default:
	}

}
