package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// BENCHMARK SUITE: Alive() + RefTimer/UnrefTimer Design Alternatives
//
// Purpose: Produce EMPIRICAL data for design decisions. No hallucinated numbers.
//
// Run: go test -bench='BenchmarkAlive|BenchmarkRefUnref|BenchmarkIsLoopThread' \
//     -benchmem -count=5 -run=^$ ./eventloop/
// ============================================================================

// ============================================================================
// SECTION 1: isLoopThread() COST — The goroutine identity check
// ============================================================================

func BenchmarkIsLoopThread_False(b *testing.B) {
	// Not on loop thread — the common case for external goroutines
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.isLoopThread()
	}
}

func BenchmarkIsLoopThread_True(b *testing.B) {
	// ON the loop thread — simulates JS callback context
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Measure isLoopThread from WITHIN the loop goroutine
	resultCh := make(chan time.Duration, 1)
	_ = loop.SubmitInternal(func() {
		b.ResetTimer()
		start := time.Now()
		for i := 0; i < b.N; i++ {
			_ = loop.isLoopThread()
		}
		resultCh <- time.Since(start)
	})
	<-resultCh
	cancel()
}

// ============================================================================
// SECTION 2: Alive() DESIGN ALTERNATIVES — Full method benchmarks
// ============================================================================

// aliveWithMutexes implements Alive() using mutexes for queue length checks.
// This is the proposed design from the plan.
func (l *Loop) aliveWithMutexes() bool {
	if l.refedTimerCount.Load() > 0 {
		return true
	}
	l.internalQueueMu.Lock()
	hasInternal := l.internal.Length() > 0
	l.internalQueueMu.Unlock()
	if hasInternal {
		return true
	}
	l.externalMu.Lock()
	hasExternal := l.external.Length() > 0 || len(l.auxJobs) > 0
	l.externalMu.Unlock()
	if hasExternal {
		return true
	}
	if !l.microtasks.IsEmpty() || !l.nextTickQueue.IsEmpty() {
		return true
	}
	if l.promisifyCount.Load() > 0 {
		return true
	}
	if l.userIOFDCount.Load() > 0 {
		return true
	}
	return false
}

// aliveAllAtomic implements Alive() using only atomic counters.
// Requires additional atomic fields that track queue lengths.
func (l *Loop) aliveAllAtomic() bool {
	if l.refedTimerCount.Load() > 0 {
		return true
	}
	if internalCount.Load() > 0 {
		return true
	}
	if externalCount.Load() > 0 {
		return true
	}
	if !l.microtasks.IsEmpty() || !l.nextTickQueue.IsEmpty() {
		return true
	}
	if l.promisifyCount.Load() > 0 {
		return true
	}
	if l.userIOFDCount.Load() > 0 {
		return true
	}
	return false
}

// Placeholder atomic fields for the all-atomic alternative.
// These would need to be maintained alongside the existing queues.
var (
	internalCount atomic.Int32
	externalCount atomic.Int32
)

// BenchmarkAlive_WithMutexes measures the proposed Alive() design (3 mutexes).
func BenchmarkAlive_WithMutexes(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	// Pre-populate to simulate a running loop with no pending work
	// (the common case: Alive() returns false quickly)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.aliveWithMutexes()
	}
}

// BenchmarkAlive_WithMutexes_Contended measures Alive() while the loop is running.
func BenchmarkAlive_WithMutexes_Contended(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.aliveWithMutexes()
	}
}

// BenchmarkAlive_WithMutexes_HighContention measures Alive() while the loop is under load.
func BenchmarkAlive_WithMutexes_HighContention(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Submit work continuously to create contention on externalMu
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				_ = loop.Submit(func() {})
			}
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.aliveWithMutexes()
	}
	b.StopTimer()
	close(stop)
}

// BenchmarkAlive_AllAtomic measures the all-atomic alternative (no mutexes).
func BenchmarkAlive_AllAtomic(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.aliveAllAtomic()
	}
}

// BenchmarkAlive_AllAtomic_Contended measures all-atomic Alive() while the loop is running.
func BenchmarkAlive_AllAtomic_Contended(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.aliveAllAtomic()
	}
}

// ============================================================================
// SECTION 3: RefTimer/UnrefTimer DESIGN ALTERNATIVES
// ============================================================================

// refViaIsLoopThread: Design A — isLoopThread() + direct access on loop goroutine.
// NOTE: applyTimerRefChange is now defined in loop.go (the chosen implementation).
func (l *Loop) refViaIsLoopThread(id TimerID, ref bool) error {
	if l.isLoopThread() {
		l.applyTimerRefChange(id, ref)
		return nil
	}
	return l.SubmitInternal(func() {
		l.applyTimerRefChange(id, ref)
	})
}

// refViaSubmitInternal: Design B — always use SubmitInternal (original proposal).
func (l *Loop) refViaSubmitInternal(id TimerID, ref bool) error {
	return l.SubmitInternal(func() {
		l.applyTimerRefChange(id, ref)
	})
}

// refViaSyncMap: Design C — use sync.Map for timer lookup (no isLoopThread needed).
var timerSyncMap sync.Map // TimerID → *timer

func (l *Loop) refViaSyncMap(id TimerID, ref bool) {
	val, ok := timerSyncMap.Load(uint64(id))
	if !ok {
		return
	}
	t := val.(*timer)
	old := t.refed.Swap(ref)
	if old != ref {
		if ref {
			l.refedTimerCount.Add(1)
		} else {
			l.refedTimerCount.Add(-1)
		}
	}
}

// refViaRWMutex: Design D — use RWMutex-protected timerMap for lookup.
var timerRefMu sync.RWMutex

func (l *Loop) refViaRWMutex(id TimerID, ref bool) {
	timerRefMu.RLock()
	t, ok := l.timerMap[id]
	timerRefMu.RUnlock()
	if !ok {
		return
	}
	old := t.refed.Swap(ref)
	if old != ref {
		if ref {
			l.refedTimerCount.Add(1)
		} else {
			l.refedTimerCount.Add(-1)
		}
	}
}

// --- Benchmarks: ref/unref from LOOP GOROUTINE (primary use case: JS callbacks) ---

func BenchmarkRefUnref_IsLoopThread_OnLoop(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Schedule a timer to get a valid ID, then unref/ref it from the loop goroutine
	id, _ := loop.ScheduleTimer(time.Hour, func() {})

	resultCh := make(chan time.Duration, 1)
	_ = loop.SubmitInternal(func() {
		b.ResetTimer()
		start := time.Now()
		for i := 0; i < b.N; i++ {
			_ = loop.refViaIsLoopThread(id, false)
			_ = loop.refViaIsLoopThread(id, true)
		}
		resultCh <- time.Since(start)
	})
	<-resultCh
	cancel()
}

func BenchmarkRefUnref_SubmitInternal_OnLoop(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	id, _ := loop.ScheduleTimer(time.Hour, func() {})

	resultCh := make(chan time.Duration, 1)
	_ = loop.SubmitInternal(func() {
		b.ResetTimer()
		start := time.Now()
		for i := 0; i < b.N; i++ {
			_ = loop.refViaSubmitInternal(id, false)
			_ = loop.refViaSubmitInternal(id, true)
		}
		resultCh <- time.Since(start)
	})
	<-resultCh
	cancel()
}

func BenchmarkRefUnref_SyncMap_OnLoop(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Schedule a timer, register it in sync.Map
	id, _ := loop.ScheduleTimer(time.Hour, func() {})
	timerSyncMap.Store(uint64(id), &timer{})

	resultCh := make(chan time.Duration, 1)
	_ = loop.SubmitInternal(func() {
		b.ResetTimer()
		start := time.Now()
		for i := 0; i < b.N; i++ {
			loop.refViaSyncMap(id, false)
			loop.refViaSyncMap(id, true)
		}
		resultCh <- time.Since(start)
	})
	<-resultCh
	timerSyncMap.Delete(uint64(id))
	cancel()
}

func BenchmarkRefUnref_RWMutex_OnLoop(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	id, _ := loop.ScheduleTimer(time.Hour, func() {})

	resultCh := make(chan time.Duration, 1)
	_ = loop.SubmitInternal(func() {
		b.ResetTimer()
		start := time.Now()
		for i := 0; i < b.N; i++ {
			loop.refViaRWMutex(id, false)
			loop.refViaRWMutex(id, true)
		}
		resultCh <- time.Since(start)
	})
	<-resultCh
	cancel()
}

// --- Benchmarks: ref/unref from EXTERNAL GOROUTINE (secondary use case) ---

func BenchmarkRefUnref_IsLoopThread_External(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	id, _ := loop.ScheduleTimer(time.Hour, func() {})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.refViaIsLoopThread(id, false)
		_ = loop.refViaIsLoopThread(id, true)
	}
	cancel()
}

func BenchmarkRefUnref_SubmitInternal_External(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	id, _ := loop.ScheduleTimer(time.Hour, func() {})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.refViaSubmitInternal(id, false)
		_ = loop.refViaSubmitInternal(id, true)
	}
	cancel()
}

func BenchmarkRefUnref_SyncMap_External(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	id, _ := loop.ScheduleTimer(time.Hour, func() {})
	timerSyncMap.Store(uint64(id), &timer{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loop.refViaSyncMap(id, false)
		loop.refViaSyncMap(id, true)
	}
	timerSyncMap.Delete(uint64(id))
	cancel()
}

func BenchmarkRefUnref_RWMutex_External(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Schedule timer and wait for registration to complete.
	// Without the barrier, refViaRWMutex races on timerMap (async
	// SubmitInternal registration vs. direct external-goroutine read).
	id, _ := loop.ScheduleTimer(time.Hour, func() {})
	regBarrier := make(chan struct{})
	_ = loop.SubmitInternal(func() { close(regBarrier) })
	<-regBarrier

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loop.refViaRWMutex(id, false)
		loop.refViaRWMutex(id, true)
	}
	cancel()
}

// ============================================================================
// SECTION 4: Pure atomic ops vs mutex — for the Alive() lock-free question
// ============================================================================

func BenchmarkMicrotaskRingIsEmpty(b *testing.B) {
	ring := newMicrotaskRing()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ring.IsEmpty()
	}
}

func BenchmarkMicrotaskRingIsEmpty_WithItems(b *testing.B) {
	ring := newMicrotaskRing()
	// Fill the ring partially
	for i := 0; i < 100; i++ {
		ring.Push(func() {})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ring.IsEmpty()
	}
}

// BenchmarkSubmitInternal_FastPath_OnLoop measures SubmitInternal's fast-path
// (direct execution) when called from the loop goroutine in fast-path mode.
func BenchmarkSubmitInternal_FastPath_OnLoop(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	resultCh := make(chan time.Duration, 1)
	n := int64(0)
	_ = loop.SubmitInternal(func() {
		b.ResetTimer()
		start := time.Now()
		for i := 0; i < b.N; i++ {
			_ = loop.SubmitInternal(func() { atomic.AddInt64(&n, 1) })
		}
		resultCh <- time.Since(start)
	})
	<-resultCh
	cancel()
}

// BenchmarkSubmitInternal_QueuePath_OnLoop measures SubmitInternal's queue path
// when called from the loop goroutine in I/O mode (simulated by registering a dummy FD).
// This shows the cost when the fast-path doesn't trigger.
func BenchmarkSubmitInternal_QueuePath_OnLoop(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	// Force I/O mode by registering a user FD
	loop.userIOFDCount.Add(1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// We need to measure the cost of SubmitInternal on the loop goroutine when it goes to queue path
	// But actually in I/O mode, the loop goroutine calls tick() not runFastPath()
	// SubmitInternal on the loop goroutine in I/O mode falls through to the queue
	resultCh := make(chan time.Duration, 1)
	n := int64(0)
	_ = loop.SubmitInternal(func() {
		b.ResetTimer()
		start := time.Now()
		for i := 0; i < b.N; i++ {
			_ = loop.SubmitInternal(func() { atomic.AddInt64(&n, 1) })
		}
		resultCh <- time.Since(start)
	})
	<-resultCh
	loop.userIOFDCount.Add(-1)
	cancel()
}

// ============================================================================
// SECTION 5: End-to-end sentinel drain benchmark
// ============================================================================

// BenchmarkSentinelIteration measures one iteration of the sentinel drain loop:
// SubmitInternal sentinel → wait → check Alive()
// Uses SubmitInternal (not Submit) for FIFO ordering with timer registrations.
func BenchmarkSentinelIteration(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		_ = loop.SubmitInternal(func() { close(done) })
		<-done
		_ = loop.Alive()
	}

	cancel()
}

// BenchmarkSentinelIteration_WithTimers measures sentinel when timers are pending.
func BenchmarkSentinelIteration_WithTimers(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Schedule a long-lived timer
	_, _ = loop.ScheduleTimer(time.Hour, func() {})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		_ = loop.SubmitInternal(func() { close(done) })
		<-done
		_ = loop.Alive()
	}

	cancel()
}

func BenchmarkSubmitInternal_Cost(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = loop.SubmitInternal(func() {})
	}

	cancel()
}

// BenchmarkSentinelDrain measures the cost of the sentinel drain loop pattern
// used by waitForAsyncWork in one-shot-man. Each iteration submits a barrier
// via SubmitInternal, waits for it to execute, then checks Alive().
//
// This is the exact pattern used in production to wait for async work completion.
func BenchmarkSentinelDrain_NoWork(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()
	defer func() { cancel(); <-done }()

	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := make(chan struct{})
		if err := loop.SubmitInternal(func() { close(d) }); err != nil {
			b.Fatal(err)
		}
		<-d
		if loop.Alive() {
			// would continue looping
		}
	}
	b.StopTimer()
}

// BenchmarkSentinelDrain_WithTimers measures the sentinel drain pattern when
// timers are being registered and firing concurrently.
func BenchmarkSentinelDrain_WithTimers(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()
	defer func() { cancel(); <-done }()

	time.Sleep(10 * time.Millisecond)

	// Pre-register a short-lived timer that fires during the benchmark.
	_, _ = loop.ScheduleTimer(time.Millisecond, func() {})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Schedule a timer to keep the loop alive.
		timerID, err := loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			b.Fatal(err)
		}

		d := make(chan struct{})
		if err := loop.SubmitInternal(func() { close(d) }); err != nil {
			b.Fatal(err)
		}
		<-d
		// Alive() returns true because the timer is still pending.
		_ = loop.Alive()

		// Cancel the timer to allow the sentinel to eventually complete.
		// In production, the timer would fire naturally.
		if err := loop.CancelTimer(timerID); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}

// BenchmarkAlive_Uncontended measures Alive() cost with a running loop and no contention.
func BenchmarkAlive_Uncontended(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()
	defer func() { cancel(); <-done }()

	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.Alive()
	}
	b.StopTimer()
}

// ============================================================================
// SECTION 7: PingPong Regression Isolation
//
// The 37% PingPong regression (Jan→Apr) was root-caused to three sources:
//   1. hasInternalTasks() mutex check (~8 ns) after every runAux() wakeup
//   2. hasExternalTasks() mutex check (~8 ns) after every runAux() wakeup
//   3. safeExecute metrics overhead (~3-5 ns)
//
// This section isolates each source with micro-benchmarks, proving the
// exact cost difference between mutex-protected and atomic-based checks.
// ============================================================================

// BenchmarkRegression_HasInternalTasks_Mutex measures the cost of the current
// mutex-based hasInternalTasks() check that runs after every fastWakeupCh event.
func BenchmarkRegression_HasInternalTasks_Mutex(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.hasInternalTasks()
	}
}

// BenchmarkRegression_HasExternalTasks_Mutex measures the cost of the current
// mutex-based hasExternalTasks() check that runs after every fastWakeupCh event.
func BenchmarkRegression_HasExternalTasks_Mutex(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.hasExternalTasks()
	}
}

// BenchmarkRegression_HasInternalTasks_SimulatedAtomic measures the cost of
// a hypothetical atomic-based check. This is the optimization target.
// Simulates: atomic.Int32.Load() > 0
func BenchmarkRegression_HasInternalTasks_SimulatedAtomic(b *testing.B) {
	var hasInternalTasks atomic.Int32
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hasInternalTasks.Load() > 0
	}
}

// BenchmarkRegression_Combined_MutexVsAtomic directly compares the combined
// cost of both checks (hasInternalTasks + hasExternalTasks) using mutex vs
// atomic. This is the exact code path in runFastPath lines 557-563.
func BenchmarkRegression_Combined_Mutex(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// This is exactly what runFastPath does after every runAux()
		_ = loop.hasInternalTasks()
		_ = loop.hasExternalTasks()
	}
}

func BenchmarkRegression_Combined_Atomic(b *testing.B) {
	var hasInternal atomic.Int32
	var hasExternal atomic.Int32
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulated atomic-based checks
		_ = hasInternal.Load() > 0
		_ = hasExternal.Load() > 0
	}
}

// BenchmarkRegression_FastPathWakeup_NoWork measures the full cost of a single
// fast-path wakeup cycle with no work, including the mutex checks.
// This is the end-to-end cost that regressed.
func BenchmarkRegression_FastPathWakeup_NoWork(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()
	defer func() { cancel(); <-done }()

	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		d := make(chan struct{})
		_ = loop.SubmitInternal(func() { close(d) })
		<-d
	}
	b.StopTimer()
}

// BenchmarkAlive_WithTimer measures Alive() cost when a timer is pending.
func BenchmarkAlive_WithTimer(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()
	defer func() { cancel(); <-done }()

	_, _ = loop.ScheduleTimer(time.Hour, func() {})
	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.Alive()
	}
	b.StopTimer()
}

// ============================================================================
// SECTION 8: EPOCH MECHANISM OVERHEAD — Alive() post-GAP-004 benchmarks
//
// Measures the cost of the submissionEpoch guard in Alive().
// The epoch adds two atomic.Uint64.Load() calls in the no-contention path.
// ============================================================================

// BenchmarkAlive_Epoch_NoContention measures Alive() on a running but idle loop.
// This is the common path: epoch unchanged, single pass through checks,
// two extra atomic loads (pre-check + post-check epoch).
func BenchmarkAlive_Epoch_NoContention(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.Alive()
	}
	b.StopTimer()
}

// BenchmarkAlive_Epoch_ConcurrentSubmit measures Alive() while another goroutine
// continuously submits work. This exercises the epoch retry path.
func BenchmarkAlive_Epoch_ConcurrentSubmit(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Submitter goroutine
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				_ = loop.Submit(func() {})
			}
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.Alive()
	}
	b.StopTimer()
	close(stop)
}

// BenchmarkAlive_Epoch_FromCallback measures Alive() called from within a
// SubmitInternal callback (loop goroutine). No concurrent mutation possible,
// but epoch mechanism still incurs two atomic loads.
func BenchmarkAlive_Epoch_FromCallback(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		_ = loop.SubmitInternal(func() {
			_ = loop.Alive()
			close(done)
		})
		<-done
	}
	b.StopTimer()
}
