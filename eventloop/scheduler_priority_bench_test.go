package eventloop

import (
	"sync/atomic"
	"testing"
	"time"
)

const schedulerPriorityExternalBacklog = 64

// BenchmarkSchedulerPriorityLatency measures final-Loop internal priority end to end.
func BenchmarkSchedulerPriorityLatency(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	var totalExternalBeforeInternal int64
	var totalPriorityLatency time.Duration
	var blockedRelease chan struct{}
	defer func() {
		if blockedRelease != nil {
			close(blockedRelease)
		}
	}()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		started := make(chan struct{})
		release := make(chan struct{})
		blockedRelease = release
		internalDone := make(chan time.Duration, 1)
		externalDone := make(chan struct{}, schedulerPriorityExternalBacklog)
		var externalBeforeInternal atomic.Int64

		if err := loop.Submit(func() {
			close(started)
			<-release
		}); err != nil {
			b.Fatalf("blocking Submit: %v", err)
		}
		waitBenchmarkSignalDeadline(b, started, deadline.C, "priority blocker entry")
		for range schedulerPriorityExternalBacklog {
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
			latency := time.Since(admitted)
			totalPriorityLatency += latency
			internalDone <- latency
		}); err != nil {
			b.Fatalf("queued SubmitInternal: %v", err)
		}
		close(release)
		blockedRelease = nil
		select {
		case <-internalDone:
		case <-deadline.C:
			b.Fatal("timed out waiting for priority callback")
		}
		for range schedulerPriorityExternalBacklog {
			waitBenchmarkSignalDeadline(b, externalDone, deadline.C, "external backlog callback")
		}
	}
	b.StopTimer()
	if b.N > 0 {
		b.ReportMetric(float64(totalExternalBeforeInternal)/float64(b.N), "externals_before_internal/op")
		b.ReportMetric(float64(totalPriorityLatency.Nanoseconds())/float64(b.N), "priority_latency_ns/op")
	}
}

// BenchmarkSubmitInternalChainHandoff measures a completed internal handoff chain.
func BenchmarkSubmitInternalChainHandoff(b *testing.B) {
	const chainDepth = 32
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	done := make(chan struct{}, 1)
	errs := make(chan error, 1)
	var submitNext func(int)
	submitNext = func(depth int) {
		if depth >= chainDepth {
			done <- struct{}{}
			return
		}
		if err := loop.SubmitInternal(func() { submitNext(depth + 1) }); err != nil {
			select {
			case errs <- err:
			default:
			}
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := loop.Submit(func() { submitNext(0) }); err != nil {
			b.Fatalf("Submit chain root: %v", err)
		}
		select {
		case <-done:
		case err := <-errs:
			b.Fatalf("SubmitInternal chain handoff: %v", err)
		case <-deadline.C:
			b.Fatal("timed out waiting for internal chain handoff")
		}
	}
	b.StopTimer()
	b.ReportMetric(chainDepth, "handoffs/op")
}

// BenchmarkSchedulerInternalExternalBurst measures paired callback completion.
func BenchmarkSchedulerInternalExternalBurst(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	completed := make(chan struct{}, 2)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := loop.Submit(func() { completed <- struct{}{} }); err != nil {
			b.Fatalf("Submit: %v", err)
		}
		if err := loop.SubmitInternal(func() { completed <- struct{}{} }); err != nil {
			b.Fatalf("SubmitInternal: %v", err)
		}
		waitBenchmarkSignalDeadline(b, completed, deadline.C, "first burst callback")
		waitBenchmarkSignalDeadline(b, completed, deadline.C, "second burst callback")
	}
}

func waitBenchmarkSignalDeadline(b *testing.B, signal <-chan struct{}, deadline <-chan time.Time, label string) {
	b.Helper()
	select {
	case <-signal:
	case <-deadline:
		b.Fatalf("timed out waiting for %s", label)
	}
}
