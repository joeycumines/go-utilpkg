package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/eventlooptest"
)

const timerScale100K = 100_000

const benchmarkMicrotaskDrainBatch = 64 * 1024

func startBenchmarkLoop(tb testing.TB, opts ...LoopOption) (*Loop, func()) {
	tb.Helper()

	loop, err := New(opts...)
	if err != nil {
		panic(err)
	}

	runDone := make(chan error, 1)
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			result := eventlooptest.Terminate(loop, runDone, 5*time.Second)
			if result.ShutdownErr != nil && !errors.Is(result.ShutdownErr, ErrLoopTerminated) {
				tb.Errorf("Shutdown: %v", result.ShutdownErr)
			}
			if result.CloseErr != nil && !errors.Is(result.CloseErr, ErrLoopTerminated) {
				tb.Errorf("fallback Close: %v", result.CloseErr)
			}
			if result.RunErr != nil {
				tb.Errorf("Run: %v", result.RunErr)
			}
		})
	}
	tb.Cleanup(cleanup)
	go func() { runDone <- loop.Run(context.Background()) }()

	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		tb.Fatalf("warmup Submit: %v", err)
	}
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		tb.Fatal("loop warmup timed out")
	}

	return loop, cleanup
}

func waitBenchmarkSignal(b *testing.B, ch <-chan struct{}, label string) {
	b.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		b.Fatalf("timed out waiting for %s", label)
	}
}

func waitBenchmarkLoopTurn(b *testing.B, loop *Loop, label string) {
	b.Helper()
	done := make(chan struct{})
	if err := loop.Submit(func() { close(done) }); err != nil {
		b.Fatalf("Submit %s barrier: %v", label, err)
	}
	waitBenchmarkSignal(b, done, label)
}

func BenchmarkCommandIngressEmptyDrain(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if loop.drainCommandIngress() {
			b.Fatal("empty ingress reported work")
		}
	}
}

func BenchmarkAliveLoopThreadNoIngress(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	result := make(chan bool, 1)
	if err := loop.Submit(func() {
		unexpected := false
		b.ResetTimer()
		for range b.N {
			unexpected = loop.Alive() || unexpected
		}
		b.StopTimer()
		result <- unexpected
	}); err != nil {
		b.Fatalf("Submit benchmark callback: %v", err)
	}
	select {
	case unexpected := <-result:
		if unexpected {
			b.Fatal("Alive reported work in the empty owner callback")
		}
	case <-time.After(10 * time.Second):
		b.Fatal("timed out waiting for Alive benchmark callback")
	}
}

func BenchmarkHasMacrotaskWorkLoopThreadNoIngress(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	result := make(chan bool, 1)
	if err := loop.Submit(func() {
		unexpected := false
		b.ResetTimer()
		for range b.N {
			unexpected = loop.HasMacrotaskWork() || unexpected
		}
		b.StopTimer()
		result <- unexpected
	}); err != nil {
		b.Fatalf("Submit benchmark callback: %v", err)
	}
	select {
	case unexpected := <-result:
		if unexpected {
			b.Fatal("HasMacrotaskWork reported work in the empty owner callback")
		}
	case <-time.After(10 * time.Second):
		b.Fatal("timed out waiting for HasMacrotaskWork benchmark callback")
	}
}

// BenchmarkMicrotaskScheduleExternal measures external admission cost; draining is untimed.
func BenchmarkMicrotaskScheduleExternal(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()

	var executed atomic.Int64

	b.ReportAllocs()
	watchdog := time.AfterFunc(30*time.Minute, func() { panic("BenchmarkMicrotaskScheduleExternal timed out") })
	defer watchdog.Stop()
	b.ResetTimer()
	b.StopTimer()
	for scheduledTotal := int64(0); scheduledTotal < int64(b.N); {
		batch := int64(benchmarkMicrotaskDrainBatch)
		if remaining := int64(b.N) - scheduledTotal; remaining < batch {
			batch = remaining
		}
		batchTarget := scheduledTotal + batch
		batchDrained := make(chan struct{})
		callback := func() {
			if executed.Add(1) == batchTarget {
				close(batchDrained)
			}
		}

		b.StartTimer()
		for i := int64(0); i < batch; i++ {
			if err := loop.ScheduleMicrotask(callback); err != nil {
				b.Fatalf("ScheduleMicrotask: %v", err)
			}
		}
		b.StopTimer()
		waitBenchmarkSignal(b, batchDrained, "external microtask drain")
		scheduledTotal = batchTarget
		if scheduledTotal < int64(b.N) {
			waitBenchmarkLoopTurn(b, loop, "external microtask batch boundary")
		}
	}
}

// BenchmarkMicrotaskScheduleLoopThread measures owner-handoff admission cost.
func BenchmarkMicrotaskScheduleLoopThread(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	var executed atomic.Int64

	b.ReportAllocs()
	b.ResetTimer()
	b.StopTimer()
	for scheduledTotal := int64(0); scheduledTotal < int64(b.N); {
		batch := int64(benchmarkMicrotaskDrainBatch)
		if remaining := int64(b.N) - scheduledTotal; remaining < batch {
			batch = remaining
		}
		batchTarget := scheduledTotal + batch
		batchDrained := make(chan struct{})
		callback := func() {
			if executed.Add(1) == batchTarget {
				close(batchDrained)
			}
		}
		scheduled := make(chan error, 1)

		b.StartTimer()
		if err := loop.Submit(func() {
			for i := int64(0); i < batch; i++ {
				if err := loop.ScheduleMicrotask(callback); err != nil {
					scheduled <- err
					return
				}
			}
			scheduled <- nil
		}); err != nil {
			b.Fatalf("Submit scheduler: %v", err)
		}
		select {
		case err := <-scheduled:
			if err != nil {
				b.Fatalf("loop-thread ScheduleMicrotask: %v", err)
			}
		case <-deadline.C:
			b.Fatal("timed out waiting for loop-thread microtask admission")
		}
		b.StopTimer()
		waitBenchmarkSignalDeadline(b, batchDrained, deadline.C, "loop-thread microtask drain")
		scheduledTotal = batchTarget
	}
}

// BenchmarkNextTickScheduleExternal measures external admission cost; draining is untimed.
func BenchmarkNextTickScheduleExternal(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()

	var executed atomic.Int64

	b.ReportAllocs()
	b.ResetTimer()
	b.StopTimer()
	for scheduledTotal := int64(0); scheduledTotal < int64(b.N); {
		batch := int64(benchmarkMicrotaskDrainBatch)
		if remaining := int64(b.N) - scheduledTotal; remaining < batch {
			batch = remaining
		}
		batchTarget := scheduledTotal + batch
		batchDrained := make(chan struct{})
		callback := func() {
			if executed.Add(1) == batchTarget {
				close(batchDrained)
			}
		}

		b.StartTimer()
		for i := int64(0); i < batch; i++ {
			if err := loop.ScheduleNextTick(callback); err != nil {
				b.Fatalf("ScheduleNextTick: %v", err)
			}
		}
		b.StopTimer()
		waitBenchmarkSignal(b, batchDrained, "external nextTick drain")
		scheduledTotal = batchTarget
		if scheduledTotal < int64(b.N) {
			waitBenchmarkLoopTurn(b, loop, "external nextTick batch boundary")
		}
	}
}

// BenchmarkNextTickScheduleLoopThread measures owner-handoff admission cost.
func BenchmarkNextTickScheduleLoopThread(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	var executed atomic.Int64

	b.ReportAllocs()
	b.ResetTimer()
	b.StopTimer()
	for scheduledTotal := int64(0); scheduledTotal < int64(b.N); {
		batch := int64(benchmarkMicrotaskDrainBatch)
		if remaining := int64(b.N) - scheduledTotal; remaining < batch {
			batch = remaining
		}
		batchTarget := scheduledTotal + batch
		batchDrained := make(chan struct{})
		callback := func() {
			if executed.Add(1) == batchTarget {
				close(batchDrained)
			}
		}
		scheduled := make(chan error, 1)

		b.StartTimer()
		if err := loop.Submit(func() {
			for i := int64(0); i < batch; i++ {
				if err := loop.ScheduleNextTick(callback); err != nil {
					scheduled <- err
					return
				}
			}
			scheduled <- nil
		}); err != nil {
			b.Fatalf("Submit scheduler: %v", err)
		}
		select {
		case err := <-scheduled:
			if err != nil {
				b.Fatalf("loop-thread ScheduleNextTick: %v", err)
			}
		case <-deadline.C:
			b.Fatal("timed out waiting for loop-thread nextTick admission")
		}
		b.StopTimer()
		waitBenchmarkSignalDeadline(b, batchDrained, deadline.C, "loop-thread nextTick drain")
		scheduledTotal = batchTarget
	}
}

// BenchmarkNextTickRecursiveDrain measures recursive admission through full drain.
func BenchmarkNextTickRecursiveDrain(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	if b.N == 0 {
		return
	}

	var completed int64
	errs := make(chan error, 1)
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	for completed < int64(b.N) {
		batch := int64(benchmarkMicrotaskDrainBatch)
		if remaining := int64(b.N) - completed; remaining < batch {
			batch = remaining
		}
		batchDone := make(chan struct{})
		var batchCount int64
		var step func()
		step = func() {
			batchCount++
			if batchCount >= batch {
				close(batchDone)
				return
			}
			if err := loop.ScheduleNextTick(step); err != nil {
				select {
				case errs <- err:
				default:
				}
			}
		}
		if err := loop.ScheduleNextTick(step); err != nil {
			b.Fatalf("ScheduleNextTick: %v", err)
		}
		select {
		case <-batchDone:
		case err := <-errs:
			b.Fatalf("recursive ScheduleNextTick: %v", err)
		case <-deadline.C:
			b.Fatal("timed out waiting for recursive nextTick drain")
		}
		completed += batch
		if completed < int64(b.N) {
			b.StopTimer()
			waitBenchmarkLoopTurn(b, loop, "recursive nextTick batch boundary")
			b.StartTimer()
		}
	}
	b.StopTimer()
}

// BenchmarkTimerScheduleSameDeadline100K measures the production timer topology component.
func BenchmarkTimerScheduleSameDeadline100K(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	ids := make([]TimerID, timerScale100K)
	callback := func() {}
	scheduled := make(chan error, 1)
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		when := time.Now().Add(time.Hour)
		if err := loop.Submit(func() {
			for index := range ids {
				id, err := commitBenchmarkTimer(loop, when, callback)
				if err != nil {
					scheduled <- err
					return
				}
				ids[index] = id
			}
			scheduled <- nil
		}); err != nil {
			b.Fatalf("Submit timer batch: %v", err)
		}
		select {
		case err := <-scheduled:
			if err != nil {
				b.Fatalf("schedule same-deadline batch: %v", err)
			}
		case <-deadline.C:
			b.Fatal("timed out waiting for same-deadline timer batch")
		}
		b.StopTimer()
		for index, err := range loop.CancelTimers(ids...) {
			if err != nil {
				b.Fatalf("CancelTimers cleanup result[%d]: %v", index, err)
			}
		}
		if iteration+1 < b.N {
			b.StartTimer()
		}
	}
	b.StopTimer()
	b.ReportMetric(timerScale100K, "timers/op")
}

// BenchmarkTimerScheduleRandomDeadlines measures the production timer topology component.
func BenchmarkTimerScheduleRandomDeadlines(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	ids := make([]TimerID, b.N)
	var seed uint64 = 0x9e3779b97f4a7c15
	nextDelay := func() time.Duration {
		seed ^= seed << 7
		seed ^= seed >> 9
		seed ^= seed << 8
		return time.Duration(seed%uint64(time.Hour)) + time.Millisecond
	}
	scheduled := make(chan error, 1)
	releaseCleanup := make(chan struct{})
	cleanupResults := make(chan []error, 1)
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseCleanup) })
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	if err := loop.Submit(func() {
		base := time.Now()
		retainBenchmarkTimers(
			loop,
			ids,
			func() time.Time { return base.Add(nextDelay()) },
			func() {},
			scheduled,
			releaseCleanup,
			cleanupResults,
		)
	}); err != nil {
		b.Fatalf("Submit random-deadline batch: %v", err)
	}
	select {
	case err := <-scheduled:
		if err != nil {
			b.Fatalf("schedule random-deadline batch: %v", err)
		}
	case <-deadline.C:
		b.Fatal("timed out waiting for random-deadline timer batch")
	}
	b.StopTimer()
	releaseOnce.Do(func() { close(releaseCleanup) })
	var results []error
	select {
	case results = <-cleanupResults:
	case <-deadline.C:
		b.Fatal("timed out waiting for random-deadline timer cleanup")
	}
	for index, err := range results {
		if err != nil {
			b.Fatalf("CancelTimers cleanup result[%d]: %v", index, err)
		}
	}
}

func TestRetainBenchmarkTimersPreventsDueDispatch(t *testing.T) {
	loop, cleanup := startBenchmarkLoop(t)
	defer cleanup()
	ids := make([]TimerID, 32)
	scheduled := make(chan error, 1)
	releaseCleanup := make(chan struct{})
	cleanupResults := make(chan []error, 1)
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseCleanup) })
	var fired atomic.Int64

	if err := loop.Submit(func() {
		retainBenchmarkTimers(
			loop,
			ids,
			func() time.Time { return time.Now().Add(-time.Hour) },
			func() { fired.Add(1) },
			scheduled,
			releaseCleanup,
			cleanupResults,
		)
	}); err != nil {
		t.Fatalf("Submit due timer batch: %v", err)
	}
	select {
	case err := <-scheduled:
		if err != nil {
			t.Fatalf("schedule due timer batch: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for due timer batch")
	}

	releaseOnce.Do(func() { close(releaseCleanup) })
	select {
	case results := <-cleanupResults:
		if len(results) != len(ids) {
			t.Fatalf("CancelTimers result length = %d, want %d", len(results), len(ids))
		}
		for index, err := range results {
			if err != nil {
				t.Fatalf("CancelTimers result[%d]: %v", index, err)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for due timer cleanup")
	}

	barrier := make(chan struct{})
	if err := loop.Submit(func() { close(barrier) }); err != nil {
		t.Fatalf("Submit post-cleanup barrier: %v", err)
	}
	select {
	case <-barrier:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for post-cleanup barrier")
	}
	if got := fired.Load(); got != 0 {
		t.Fatalf("fired callback count = %d, want 0", got)
	}
}

func retainBenchmarkTimers(
	loop *Loop,
	ids []TimerID,
	deadline func() time.Time,
	callback func(),
	scheduled chan<- error,
	releaseCleanup <-chan struct{},
	cleanupResults chan<- []error,
) {
	for index := range ids {
		id, err := commitBenchmarkTimer(loop, deadline(), callback)
		if err != nil {
			scheduled <- err
			return
		}
		ids[index] = id
	}
	scheduled <- nil
	<-releaseCleanup
	cleanupResults <- loop.CancelTimers(ids...)
}

// BenchmarkTimerCancelScale measures the public batch-cancellation component.
func BenchmarkTimerCancelScale(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	ids := make([]TimerID, b.N)
	scheduled := make(chan error, 1)
	if err := loop.Submit(func() {
		when := time.Now().Add(time.Hour)
		for index := range ids {
			id, err := commitBenchmarkTimer(loop, when, func() {})
			if err != nil {
				scheduled <- err
				return
			}
			ids[index] = id
		}
		scheduled <- nil
	}); err != nil {
		b.Fatalf("Submit cancellation prefill: %v", err)
	}
	select {
	case err := <-scheduled:
		if err != nil {
			b.Fatalf("schedule cancellation prefill: %v", err)
		}
	case <-time.After(30 * time.Second):
		b.Fatal("timed out waiting for cancellation prefill")
	}

	b.ReportAllocs()
	b.ResetTimer()
	errs := loop.CancelTimers(ids...)
	b.StopTimer()
	if len(errs) != len(ids) {
		b.Fatalf("CancelTimers result length = %d, want %d", len(errs), len(ids))
	}
	for index, err := range errs {
		if err != nil {
			b.Fatalf("CancelTimers result[%d]: %v", index, err)
		}
	}
}

// commitBenchmarkTimer invokes the production timer constructor and owner commit path.
func commitBenchmarkTimer(loop *Loop, when time.Time, fn func()) (TimerID, error) {
	publication := make(chan struct{})
	timer, err := loop.acquireTimer(when, 0, fn, false, nil, publication)
	if err != nil {
		close(publication)
		return 0, err
	}
	loop.livenessMu.Lock()
	if err := loop.rejectLivenessAddLocked(); err != nil {
		loop.livenessMu.Unlock()
		close(publication)
		resetTimerForPool(timer)
		timerPool.Put(timer)
		return 0, err
	}
	loop.commitTimer(timer)
	loop.submissionEpoch.Add(1)
	close(publication)
	loop.livenessMu.Unlock()
	return timer.id, nil
}

// BenchmarkSetIntervalSteadyTicks measures complete interval firing and self-cancellation.
func BenchmarkSetIntervalSteadyTicks(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	if b.N == 0 {
		return
	}
	js, err := NewJS(loop)
	if err != nil {
		b.Fatal(err)
	}

	var executed atomic.Int64
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	done, err := startPublishedSteadyInterval(js, int64(b.N), &executed)
	if err != nil {
		b.Fatalf("SetInterval: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			b.Fatalf("ClearInterval: %v", err)
		}
	case <-deadline.C:
		b.Fatal("timed out waiting for interval completion")
	}
	b.StopTimer()
	waitBenchmarkLoopTurn(b, loop, "interval retirement")
	if got := executed.Load(); got != int64(b.N) {
		b.Fatalf("interval callbacks = %d, want %d", got, b.N)
	}
}

func startPublishedSteadyInterval(js *JS, count int64, executed *atomic.Int64) (<-chan error, error) {
	done := make(chan error, 1)
	published := make(chan struct{})
	var intervalID uint64
	var err error
	intervalID, err = js.SetInterval(func() {
		<-published
		if executed.Add(1) == count {
			done <- js.ClearInterval(intervalID)
		}
	}, 0)
	close(published)
	return done, err
}

func TestPublishedSteadyIntervalFixtureLifecycle(t *testing.T) {
	loop, _ := startBenchmarkLoop(t)
	var executed atomic.Int64
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	done, err := startPublishedSteadyInterval(js, 16, &executed)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ClearInterval: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for interval completion")
	}
	retired := make(chan struct{})
	if err := loop.Submit(func() { close(retired) }); err != nil {
		t.Fatalf("Submit retirement barrier: %v", err)
	}
	waitContractSignal(t, retired, "interval retirement")
	if got := executed.Load(); got != 16 {
		t.Fatalf("interval callbacks = %d, want 16", got)
	}
}

// BenchmarkSetImmediateBurst measures public admission through complete burst retirement.
func BenchmarkSetImmediateBurst(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	js, err := NewJS(loop)
	if err != nil {
		b.Fatal(err)
	}

	var executed atomic.Int64
	done := make(chan struct{})
	var once sync.Once
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	callback := func() {
		if executed.Add(1) == int64(b.N) {
			once.Do(func() { close(done) })
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := js.SetImmediate(callback); err != nil {
			b.Fatalf("SetImmediate: %v", err)
		}
	}
	if b.N > 0 {
		waitBenchmarkSignalDeadline(b, done, deadline.C, "immediate burst completion")
	}
	b.StopTimer()
	waitBenchmarkLoopTurn(b, loop, "immediate burst retirement")
	if got := executed.Load(); got != int64(b.N) {
		b.Fatalf("immediate callbacks = %d, want %d", got, b.N)
	}
}

// BenchmarkMetricsHotPath measures the live metrics-update component.
func BenchmarkMetricsHotPath(b *testing.B) {
	metrics := newRuntimeMetrics()
	now := time.Now()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.recordQueueDepths(i&255, (i>>1)&255, (i>>2)&255)
		metrics.recordCallback(time.Duration(i&1023)*time.Nanosecond, now, true)
	}
}

// BenchmarkMetricsHotPathSnapshot measures the public detached-snapshot component.
func BenchmarkMetricsHotPathSnapshot(b *testing.B) {
	loop, err := New(WithMetrics(true))
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := loop.Close(); err != nil {
			b.Errorf("Close: %v", err)
		}
	}()
	for i := range 2000 {
		loop.metrics.recordQueueDepths(i&127, i&63, i&31)
		loop.metrics.recordCallback(time.Duration(i+1)*time.Nanosecond, time.Time{}, false)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if snapshot := loop.Metrics(); snapshot == nil {
			b.Fatal("Metrics returned nil")
		}
	}
}
