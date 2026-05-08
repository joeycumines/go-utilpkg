package tournament

import (
	"sync/atomic"
	"testing"
	"time"
)

func BenchmarkSchedulerPriorityLatency(b *testing.B) {
	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkSchedulerPriorityLatency(b, impl)
		})
	}
}

func benchmarkSchedulerPriorityLatency(b *testing.B, impl Implementation) {
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()

	b.ReportAllocs()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	var blockedRelease chan struct{}
	defer func() {
		if blockedRelease != nil {
			close(blockedRelease)
		}
	}()
	const externalBacklog = 64
	var totalExternalBeforeInternal int64
	var totalPriorityLatencyNs int64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		started := make(chan struct{})
		release := make(chan struct{})
		blockedRelease = release
		latency := make(chan time.Duration, 1)
		externalDone := make(chan struct{}, externalBacklog)
		var externalBeforeInternal atomic.Int64

		if err := loop.Submit(func() {
			close(started)
			<-release
		}); err != nil {
			b.Fatalf("blocking Submit: %v", err)
		}
		waitBenchmarkDeadline(b, started, deadline.C, "priority owner callback entry")
		for range externalBacklog {
			if err := loop.Submit(func() {
				externalBeforeInternal.Add(1)
				externalDone <- struct{}{}
			}); err != nil {
				b.Fatalf("queued external Submit: %v", err)
			}
		}
		admitted := time.Now()
		if err := loop.SubmitInternal(func() {
			totalExternalBeforeInternal += externalBeforeInternal.Load()
			priorityLatency := time.Since(admitted)
			totalPriorityLatencyNs += priorityLatency.Nanoseconds()
			latency <- priorityLatency
		}); err != nil {
			b.Fatalf("queued SubmitInternal: %v", err)
		}
		close(release)
		blockedRelease = nil
		waitBenchmarkDeadline(b, latency, deadline.C, "priority callback completion")
		for range externalBacklog {
			waitBenchmarkDeadline(b, externalDone, deadline.C, "external backlog drain")
		}
	}
	b.StopTimer()
	if b.N > 0 {
		b.ReportMetric(float64(totalExternalBeforeInternal)/float64(b.N), "externals_before_internal/op")
		b.ReportMetric(float64(totalPriorityLatencyNs)/float64(b.N), "priority_latency_ns/op")
	}
}

func BenchmarkSubmitInternalChainHandoff(b *testing.B) {
	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkSubmitInternalChainHandoff(b, impl)
		})
	}
}

func benchmarkSubmitInternalChainHandoff(b *testing.B, impl Implementation) {
	const chainDepth = 32
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()

	done := make(chan struct{}, 1)
	errs := make(chan error, 1)
	var submitNext func(int)
	submitNext = func(n int) {
		if n >= chainDepth {
			done <- struct{}{}
			return
		}
		if err := loop.SubmitInternal(func() { submitNext(n + 1) }); err != nil {
			select {
			case errs <- err:
			default:
			}
		}
	}

	b.ReportAllocs()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := loop.Submit(func() { submitNext(0) }); err != nil {
			b.Fatalf("Submit chain root: %v", err)
		}
		select {
		case <-done:
		case err := <-errs:
			b.Fatalf("SubmitInternal chain handoff: %v", err)
		case <-deadline.C:
			b.Fatal("SubmitInternal chain handoff timed out")
		}
	}
	b.StopTimer()
	b.ReportMetric(chainDepth, "handoffs/op")

}

func BenchmarkSchedulerInternalExternalBurst(b *testing.B) {
	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			loop, cleanup := startBenchmarkEventLoop(b, impl)
			defer cleanup()

			deadline := time.NewTimer(30 * time.Minute)
			defer deadline.Stop()
			tasks := newBenchmarkBarrier(b.N * 2)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := loop.Submit(tasks.Done); err != nil {
					tasks.Done()
					b.Fatalf("Submit: %v", err)
				}
				if err := loop.SubmitInternal(tasks.Done); err != nil {
					tasks.Done()
					b.Fatalf("SubmitInternal: %v", err)
				}
			}
			waitBenchmarkBarrier(b, tasks, deadline.C, "internal/external burst drain")
			b.StopTimer()
		})
	}
}
