package eventloop

import (
	"math"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoopMetricsConfiguration(t *testing.T) {
	for _, option := range []LoopOption{nil, WithMetrics(false)} {
		var options []LoopOption
		if option != nil {
			options = append(options, option)
		}
		loop := New(options...)
		if got := loop.Metrics(); got != nil {
			t.Fatalf("disabled Metrics = %+v, want nil", got)
		}
		if err := loop.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	loop := New(WithMetrics(true))
	registerLoopCleanupT(t, loop)
	if got := loop.Metrics(); got == nil || *got != (Metrics{}) {
		t.Fatalf("enabled initial Metrics = %+v, want zero snapshot", got)
	}
}

func TestRuntimeMetricsSnapshotRetriesAfterConcurrentCommit(t *testing.T) {
	metrics := newRuntimeMetrics()
	firstRead := make(chan struct{})
	releaseRead := make(chan struct{})
	snapshotDone := make(chan *Metrics, 1)
	var reads atomic.Int32

	go func() {
		snapshotDone <- metrics.snapshotWith(func(metrics *runtimeMetrics) *Metrics {
			if reads.Add(1) == 1 {
				close(firstRead)
				<-releaseRead
			}
			return copyRuntimeMetrics(metrics)
		})
	}()
	select {
	case <-firstRead:
	case <-time.After(time.Second):
		t.Fatal("metric snapshot did not begin its first even-epoch read")
	}
	metrics.recordCallback(time.Nanosecond, time.Now(), true)
	close(releaseRead)

	select {
	case snapshot := <-snapshotDone:
		if reads.Load() < 2 {
			t.Fatalf("snapshot accepted stale epoch after %d read", reads.Load())
		}
		if snapshot.Latency.Count != 1 || snapshot.TPS <= 0 {
			t.Fatalf("snapshot mixed callback and throughput epochs: %+v", snapshot)
		}
	case <-time.After(time.Second):
		t.Fatal("metric snapshot did not retry after concurrent commit")
	}
}

func TestRuntimeMetricsWritePanicReleasesEpoch(t *testing.T) {
	metrics := newRuntimeMetrics()
	panicValue := func() (recovered any) {
		defer func() { recovered = recover() }()
		metrics.write(func() {
			metrics.queue.ingressCurrent.Store(1)
			panic("metric update panic")
		})
		return nil
	}()
	if panicValue != "metric update panic" {
		t.Fatalf("metric update panic = %#v", panicValue)
	}
	if seq := metrics.snapshotSeq.Load(); seq&1 != 0 {
		t.Fatalf("metric epoch remained odd after panic: %d", seq)
	}
	if got := metrics.snapshot().Queue.IngressCurrent; got != 1 {
		t.Fatalf("snapshot after released panic epoch = %d, want 1", got)
	}
}

func TestRuntimeLatencyCountSaturates(t *testing.T) {
	metrics := newRuntimeMetrics()
	metrics.latency.count.Store(^uint64(0))
	metrics.recordCallback(-time.Second, time.Time{}, false)

	snapshot := metrics.snapshot()
	if snapshot.Latency.Count != ^uint64(0) {
		t.Fatalf("saturated observation count = %d", snapshot.Latency.Count)
	}
	if snapshot.Latency.Mean != 0 || snapshot.Latency.P50 != 0 || snapshot.Latency.Max != 0 {
		t.Fatalf("saturated sampler changed after another observation: %+v", snapshot.Latency)
	}
}

func TestRuntimeLatencyNegativeDurationClamps(t *testing.T) {
	metrics := newRuntimeMetrics()
	metrics.recordCallback(-time.Second, time.Time{}, false)

	latency := metrics.snapshot().Latency
	if latency.Count != 1 || latency.Mean != 0 || latency.P50 != 0 || latency.Max != 0 {
		t.Fatalf("negative duration escaped clamp: %+v", latency)
	}
}

func TestRuntimeLatencySamplerPreservesAllObservationMax(t *testing.T) {
	loop := New(WithMetrics(true))
	registerLoopCleanupT(t, loop)

	const slow = 10 * time.Second
	loop.metrics.recordCallback(slow, time.Now(), false)
	for range 1024 {
		loop.metrics.recordCallback(time.Nanosecond, time.Now(), false)
	}

	stats := loop.Metrics()
	if stats == nil {
		t.Fatal("expected metrics snapshot")
	}
	if stats.Latency.Count != 1025 {
		t.Fatalf("all-observation count after legacy ring size = %d, want 1025", stats.Latency.Count)
	}
	if stats.Latency.Max != slow {
		t.Fatalf("expected all-observation max %v after ring wrap, got %v", slow, stats.Latency.Max)
	}
}

func TestRuntimeLatencySamplerPreservesDurationBounds(t *testing.T) {
	metrics := newRuntimeMetrics()
	for range exactLatencySampleSize {
		metrics.recordCallback(time.Duration(math.MaxInt64), time.Time{}, false)
	}

	latency := metrics.snapshot().Latency
	if latency.Count != exactLatencySampleSize {
		t.Fatalf("observation count = %d, want %d", latency.Count, exactLatencySampleSize)
	}
	if latency.P50 != time.Duration(math.MaxInt64) ||
		latency.P90 != time.Duration(math.MaxInt64) ||
		latency.P95 != time.Duration(math.MaxInt64) ||
		latency.P99 != time.Duration(math.MaxInt64) ||
		latency.Max != time.Duration(math.MaxInt64) ||
		latency.Mean != time.Duration(math.MaxInt64) {
		t.Fatalf("maximum duration was not preserved: %+v", latency)
	}
}

func TestRuntimeLatencySamplerReportsArithmeticMean(t *testing.T) {
	metrics := newRuntimeMetrics()
	metrics.recordCallback(time.Nanosecond, time.Time{}, false)
	metrics.recordCallback(2*time.Nanosecond, time.Time{}, false)
	metrics.recordCallback(3*time.Nanosecond, time.Time{}, false)

	latency := metrics.snapshot().Latency
	if latency.Mean != 2*time.Nanosecond {
		t.Fatalf("arithmetic mean = %v, want 2ns", latency.Mean)
	}
}

func TestRuntimeLatencySamplerReportsExactEvenMedian(t *testing.T) {
	tests := []struct {
		name    string
		samples []time.Duration
		want    time.Duration
	}{
		{name: "two", samples: []time.Duration{time.Nanosecond, 9 * time.Nanosecond}, want: 5 * time.Nanosecond},
		{name: "four", samples: []time.Duration{time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond, 4 * time.Millisecond}, want: 2500 * time.Microsecond},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metrics := newRuntimeMetrics()
			for _, sample := range test.samples {
				metrics.recordCallback(sample, time.Time{}, false)
			}

			latency := metrics.snapshot().Latency
			if latency.Count != uint64(len(test.samples)) {
				t.Fatalf("observation count = %d, want %d", latency.Count, len(test.samples))
			}
			if latency.P50 != test.want {
				t.Fatalf("P50 = %v, want exact median %v", latency.P50, test.want)
			}
		})
	}
}

func TestRuntimeLatencySamplerUsesExactFiveObservationPercentiles(t *testing.T) {
	metrics := newRuntimeMetrics()
	for i := 1; i <= exactLatencySampleSize; i++ {
		metrics.recordCallback(time.Duration(i)*time.Millisecond, time.Time{}, false)
	}

	latency := metrics.snapshot().Latency
	if latency.P50 != 3*time.Millisecond ||
		latency.P90 != 5*time.Millisecond ||
		latency.P95 != 5*time.Millisecond ||
		latency.P99 != 5*time.Millisecond ||
		latency.Max != 5*time.Millisecond {
		t.Fatalf("five-observation percentiles are not exact: %+v", latency)
	}
}

func TestRuntimeMetricsIncludeFastPathQueuesAndMicrotasks(t *testing.T) {
	loop := New(WithMetrics(true), WithAutoExit(true))
	for range 3 {
		if err := loop.Submit(func() {}); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := loop.ScheduleMicrotask(func() {}); err != nil {
			t.Fatal(err)
		}
	}
	if err := loop.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	snapshot := loop.Metrics()
	if loop.fastPathEntries.Load() == 0 {
		t.Fatal("test did not exercise task-only fast path")
	}
	if snapshot.Queue.IngressMax < 3 || snapshot.Queue.MicrotaskMax < 2 {
		t.Fatalf("fast-path queue depths were not sampled: %+v", snapshot.Queue)
	}
	if snapshot.Latency.Count != 5 {
		t.Fatalf("scheduled callback count = %d, want 5", snapshot.Latency.Count)
	}
}

func TestRuntimeMetricsIncludeMaterializedPhaseQueues(t *testing.T) {
	loop := New(WithMetrics(true), WithAutoExit(true))
	for range 2 {
		if err := loop.ScheduleImmediate(func() {}); err != nil {
			t.Fatalf("ScheduleImmediate: %v", err)
		}
	}
	for range 3 {
		if err := loop.ScheduleCloseCallback(func() {}); err != nil {
			t.Fatalf("ScheduleCloseCallback: %v", err)
		}
	}
	if err := loop.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	snapshot := loop.Metrics()
	if snapshot.Queue.IngressMax < 5 {
		t.Fatalf("materialized immediate and close depth was not sampled: %+v", snapshot.Queue)
	}
	if snapshot.Latency.Count != 5 {
		t.Fatalf("scheduled callback count = %d, want 5", snapshot.Latency.Count)
	}
}

func TestRuntimeMetricsExcludeAbnormalCallbacksFromTPS(t *testing.T) {
	loop := New(WithMetrics(true), WithAutoExit(true))
	for _, callback := range []func(){
		func() { panic("metric panic") },
		runtime.Goexit,
		func() {},
	} {
		if err := loop.ScheduleMicrotask(callback); err != nil {
			t.Fatal(err)
		}
	}
	if err := loop.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	snapshot := loop.Metrics()
	if snapshot.Latency.Count != 3 {
		t.Fatalf("scheduled callback count = %d, want 3", snapshot.Latency.Count)
	}
	if snapshot.TPS != 0.1 {
		t.Fatalf("successful-return TPS = %v, want 0.1", snapshot.TPS)
	}
}

func TestLoopMetricsReturnsDetachedSnapshots(t *testing.T) {
	loop := New(WithMetrics(true))
	registerLoopCleanupT(t, loop)
	loop.metrics.recordQueueDepths(3, 2, 1)
	loop.metrics.recordCallback(time.Millisecond, time.Now(), true)

	first := loop.Metrics()
	first.Latency.Count = 0
	first.Queue.IngressMax = 0
	first.TPS = 0
	second := loop.Metrics()
	if second.Latency.Count != 1 || second.Queue.IngressMax != 3 || second.TPS <= 0 {
		t.Fatalf("caller mutation affected later metrics snapshot: %+v", second)
	}
}

func TestRuntimeTPSRecordsCompletionTime(t *testing.T) {
	loop := New(WithMetrics(true))
	registerLoopCleanupT(t, loop)

	var (
		anchor  time.Time
		counter *tpsCounter
	)

	loop.safeExecute(func() {
		anchor = loop.tickNow.Add(time.Nanosecond)
		for !time.Now().After(anchor) {
		}
		counter = newTPSCounterAt(100*time.Millisecond, 10*time.Millisecond, anchor)
		counter.IncrementAt(anchor)
		loop.metrics.tps = counter
	})

	if got := counter.tpsAt(anchor); got != 20 {
		t.Fatalf("TPS after sentinel and completed callback = %v, want 20", got)
	}
}
