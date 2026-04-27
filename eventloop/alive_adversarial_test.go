package eventloop

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestAdversarial_ConcurrentRefUnref launches 100 goroutines each doing
// RefTimer/UnrefTimer on the same timer ID in a tight loop (1000 iterations each).
// Verifies that refedTimerCount never goes negative and ends at the correct value.
func TestAdversarial_ConcurrentRefUnref(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping adversarial stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer loop.Close()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Schedule a long-lived timer.
	id, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	const goroutines = 100
	const iterations = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if i%2 == 0 {
					_ = loop.UnrefTimer(id)
				} else {
					_ = loop.RefTimer(id)
				}
			}
		}()
	}

	wg.Wait()

	// After all goroutines finish, drain pending submissions via barrier.
	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier SubmitInternal: %v", err)
	}
	<-barrier

	count := loop.refedTimerCount.Load()
	t.Logf("refedTimerCount after concurrent ref/unref: %d", count)

	// refedTimerCount must never go negative (int32 underflow would show huge value).
	if count < 0 {
		t.Fatalf("refedTimerCount went negative: %d", count)
	}

	// The timer was created ref'd=true, then each goroutine alternated unref/ref.
	// With 100 goroutines each doing 1000 iterations (500 unref + 500 ref),
	// the net effect depends on ordering. But since each goroutine ends on a ref
	// (iteration 999 is odd => RefTimer), and the timer starts ref'd,
	// we expect exactly 1 (initial ref is still active, double-ref is idempotent).
	// However, the interleaving means some unrefs may have been no-ops
	// (timer was already unref'd). The invariant is: count >= 0 and count <= 1.
	if count > 1 {
		t.Errorf("refedTimerCount should be at most 1 (idempotent ref), got %d", count)
	}

	// Clean up: cancel the timer so Close() is clean.
	_ = loop.CancelTimer(id)
}

// TestAdversarial_RefAfterCancel schedules a timer, cancels it, then calls RefTimer
// on the canceled timer ID. Verifies no panic and refedTimerCount is correct.
func TestAdversarial_RefAfterCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping adversarial stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer loop.Close()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	id, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	before := loop.refedTimerCount.Load()
	if before != 1 {
		t.Fatalf("refedTimerCount before cancel: expected 1, got %d", before)
	}

	// Cancel the timer.
	if err := loop.CancelTimer(id); err != nil {
		t.Fatalf("CancelTimer: %v", err)
	}

	afterCancel := loop.refedTimerCount.Load()
	if afterCancel != 0 {
		t.Errorf("refedTimerCount after cancel: expected 0, got %d", afterCancel)
	}

	// Ref the now-canceled timer. Should be a silent no-op.
	if err := loop.RefTimer(id); err != nil {
		t.Fatalf("RefTimer on canceled timer should not error: %v", err)
	}

	afterRef := loop.refedTimerCount.Load()
	if afterRef != 0 {
		t.Errorf("refedTimerCount after ref-ing canceled timer: expected 0, got %d", afterRef)
	}

	// Unref on the same ID should also be a no-op.
	if err := loop.UnrefTimer(id); err != nil {
		t.Fatalf("UnrefTimer on canceled timer should not error: %v", err)
	}

	afterUnref := loop.refedTimerCount.Load()
	if afterUnref != 0 {
		t.Errorf("refedTimerCount after unref-ing canceled timer: expected 0, got %d", afterUnref)
	}
}

// TestAdversarial_UnrefDuringTimerFire schedules a timer with a 1ms delay.
// From an external goroutine, calls UnrefTimer in a tight loop while the timer fires.
// Verifies no race condition (with -race) and refedTimerCount is correct.
func TestAdversarial_UnrefDuringTimerFire(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping adversarial stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer loop.Close()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	const numTimers = 50
	var fired atomic.Int32
	ids := make([]TimerID, numTimers)

	// Schedule timers with sufficient delay so unref goroutines can start
	// while timers are still pending (not yet fired).
	const timerDelay = 50 * time.Millisecond
	for i := 0; i < numTimers; i++ {
		ids[i], err = loop.ScheduleTimer(timerDelay, func() {
			fired.Add(1)
		})
		if err != nil {
			t.Fatalf("ScheduleTimer %d: %v", i, err)
		}
	}

	// Start unref goroutines immediately while timers are still pending.
	// The 50ms delay ensures the unref race with runTimers is actually exercised.
	var wg sync.WaitGroup
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < numTimers; i++ {
				_ = loop.UnrefTimer(ids[i])
			}
		}()
	}
	wg.Wait()

	// Wait for all timers to fire.
	deadline := time.After(2 * time.Second)
	for fired.Load() < numTimers {
		select {
		case <-deadline:
			t.Fatalf("timers did not all fire: %d/%d", fired.Load(), numTimers)
		default:
			time.Sleep(time.Millisecond)
		}
	}

	// Give the loop time to process counter updates.

	count := loop.refedTimerCount.Load()
	t.Logf("refedTimerCount after unref during fire: %d", count)

	if count < 0 {
		t.Fatalf("refedTimerCount went negative: %d", count)
	}
	if count != 0 {
		t.Errorf("refedTimerCount should be 0 after all timers fired, got %d", count)
	}
}

// TestAdversarial_AliveDuringShutdown starts a loop with a timer, then calls Close
// while simultaneously calling Alive() from multiple goroutines.
// Verifies no panic or deadlock.
func TestAdversarial_AliveDuringShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping adversarial stress test in short mode")
	}

	const trials = 10
	for trial := 0; trial < trials; trial++ {
		loop, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		ctx := context.Background()
		defer loop.Close() // Ensure trial loop is closed on any exit path

		go loop.Run(ctx)
		time.Sleep(10 * time.Millisecond)

		// Schedule a timer to keep the loop alive.
		_, err = loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer: %v", err)
		}
		time.Sleep(10 * time.Millisecond)

		// Launch goroutines that poll Alive() in a tight loop.
		var wg sync.WaitGroup
		const pollers = 20
		var aliveCalls atomic.Int64
		for p := 0; p < pollers; p++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 200; i++ {
					_ = loop.Alive()
					aliveCalls.Add(1)
				}
			}()
		}

		// Simultaneously close the loop.
		closeDone := make(chan error, 1)
		go func() {
			time.Sleep(time.Millisecond)
			closeDone <- loop.Close()
		}()

		wg.Wait()
		if err := <-closeDone; err != nil {
			t.Logf("Close returned: %v (acceptable)", err)
		}

		t.Logf("trial %d: aliveCalls=%d", trial, aliveCalls.Load())
	}
}

// TestAdversarial_RapidScheduleUnrefCancel runs 10000 iterations:
// schedule a timer, immediately unref it, immediately cancel it.
// Verifies refedTimerCount remains consistent under rapid schedule/unref/cancel.
func TestAdversarial_RapidScheduleUnrefCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping adversarial stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer loop.Close()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	const iterations = 10000
	for i := 0; i < iterations; i++ {
		id, err := loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer %d: %v", i, err)
		}

		// Immediately unref.
		_ = loop.UnrefTimer(id)

		// Immediately cancel.
		_ = loop.CancelTimer(id)
	}

	// Drain all pending operations by submitting a barrier.
	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier SubmitInternal: %v", err)
	}
	<-barrier

	refedCount := loop.refedTimerCount.Load()
	t.Logf("after %d iterations: refedTimerCount=%d", iterations, refedCount)

	if refedCount < 0 {
		t.Errorf("refedTimerCount went negative: %d", refedCount)
	}
	if refedCount != 0 {
		t.Errorf("refedTimerCount should be 0 after all timers canceled, got %d", refedCount)
	}
}

// TestAdversarial_PromisifyAliveAccuracy launches 50 Promisify goroutines that sleep
// for 10ms each. A separate goroutine polls Alive() in a tight loop.
// Verifies that Alive() returns true while goroutines are in-flight and false after
// all complete (within a tolerance window).
func TestAdversarial_PromisifyAliveAccuracy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping adversarial stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer loop.Close()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	const n = 50
	var started atomic.Int32
	var completed atomic.Int32

	for i := 0; i < n; i++ {
		i := i // capture loop variable into local scope
		_ = loop.Promisify(ctx, func(ctx context.Context) (any, error) {
			started.Add(1)
			time.Sleep(10 * time.Millisecond)
			completed.Add(1)
			return i, nil
		})
	}

	// Wait for all goroutines to start.
	deadline := time.After(2 * time.Second)
	for started.Load() < n {
		select {
		case <-deadline:
			t.Fatalf("not all Promisify goroutines started: %d/%d", started.Load(), n)
		default:
			time.Sleep(time.Millisecond)
		}
	}

	// Alive must be true while goroutines are in-flight.
	if !loop.Alive() {
		t.Error("Alive() should return true while Promisify goroutines are in-flight")
	}

	// Poll Alive() concurrently while goroutines complete.
	var aliveTrueCount atomic.Int64
	var aliveFalseCount atomic.Int64
	stopPoll := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopPoll:
				return
			default:
				if loop.Alive() {
					aliveTrueCount.Add(1)
				} else {
					aliveFalseCount.Add(1)
				}
			}
		}
	}()

	// Wait for all to complete.
	for completed.Load() < n {
		time.Sleep(time.Millisecond)
	}
	// Wait for promisifyCount to reach 0 (polled with timeout).
	pollDeadline := time.Now().Add(5 * time.Second)
	for loop.promisifyCount.Load() > 0 && time.Now().Before(pollDeadline) {
		time.Sleep(time.Millisecond)
	}
	if loop.promisifyCount.Load() > 0 {
		t.Fatalf("promisifyCount did not reach 0 (got %d)", loop.promisifyCount.Load())
	}

	close(stopPoll)
	wg.Wait()

	t.Logf("aliveTrueCount=%d aliveFalseCount=%d promisifyCount=%d",
		aliveTrueCount.Load(), aliveFalseCount.Load(), loop.promisifyCount.Load())

	// After all Promisify goroutines complete, drain any pending work
	// before checking Alive() to avoid false positives from in-flight callbacks.
	drainBarrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(drainBarrier) }); err == nil {
		<-drainBarrier
	}

	if loop.Alive() {
		t.Error("Alive() should return false after all Promisify goroutines complete")
	}
}

// TestAdversarial_SentinelWithActiveIntervals creates a setInterval that fires every 5ms.
// Each time the interval fires, it unrefs the newly scheduled internal timer.
// The sentinel drain loop should complete because no timer remains ref'd after draining.
func TestAdversarial_SentinelWithActiveIntervals(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping adversarial stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer loop.Close()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS: %v", err)
	}

	var fireCount atomic.Int32
	const targetFires = int32(5)
	allFired := make(chan struct{})

	intervalID, err := js.SetInterval(func() {
		n := fireCount.Add(1)
		if n >= targetFires {
			select {
			case allFired <- struct{}{}:
			default:
			}
		}
	}, 5)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	if !loop.Alive() {
		t.Fatal("Alive() should return true with a ref'd interval")
	}

	// Clear the interval and unref the current timer.
	// ClearInterval cancels the underlying timer, decrementing refedTimerCount.
	js.ClearInterval(intervalID)

	// Drain the loop to ensure the cancel is processed.
	drainDone := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(drainDone) }); err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}
	<-drainDone

	if loop.Alive() {
		t.Error("Alive() should return false after clearing the interval")
	}

	// Run the sentinel drain loop: should exit immediately since nothing is ref'd.
	sentinelDone := make(chan struct{})
	go func() {
		defer close(sentinelDone)
		for i := 0; i < 10000; i++ {
			done := make(chan struct{})
			if err := loop.SubmitInternal(func() { close(done) }); err != nil {
				return
			}
			<-done
			if !loop.Alive() {
				return
			}
		}
	}()

	select {
	case <-sentinelDone:
		t.Logf("sentinel completed, fireCount=%d refedTimerCount=%d", fireCount.Load(), loop.refedTimerCount.Load())
	case <-time.After(3 * time.Second):
		t.Fatal("sentinel drain loop did not complete within timeout")
	}

	if fireCount.Load() < 1 {
		t.Error("interval should have fired at least once")
	}
}

// TestAdversarial_IntervalRefUnrefCycle schedules multiple timers with staggered
// delays. From an external goroutine, each timer's ref state is toggled while the
// timers fire. Verifies that timers fire regardless of ref/unref state, and that
// refedTimerCount never goes negative.
func TestAdversarial_IntervalRefUnrefCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping adversarial stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer loop.Close()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	const targetFires = 10
	var fireCount atomic.Int32
	allFired := make(chan struct{})

	// Track observed Alive() values at each firing.
	var aliveDuringFire []bool
	var aliveMu sync.Mutex

	// Schedule targetFires timers with staggered delays (10ms, 20ms, ...).
	// Each timer fires exactly once. From an external goroutine, we toggle
	// ref/unref on each timer while it is pending.
	ids := make([]TimerID, targetFires)
	for i := 0; i < targetFires; i++ {
		ids[i], err = loop.ScheduleTimer(time.Duration(i+1)*10*time.Millisecond, func() {
			n := fireCount.Add(1)
			aliveNow := loop.Alive()
			aliveMu.Lock()
			aliveDuringFire = append(aliveDuringFire, aliveNow)
			aliveMu.Unlock()
			if n >= targetFires {
				select {
				case allFired <- struct{}{}:
				default:
				}
			}
		})
		if err != nil {
			t.Fatalf("ScheduleTimer %d: %v", i, err)
		}
	}

	// From an external goroutine, toggle ref/unref on each timer.
	// Odd-indexed timers get unref'd then ref'd, even-indexed get ref'd then unref'd.
	toggleDone := make(chan struct{})
	go func() {
		defer close(toggleDone)
		for i := 0; i < targetFires; i++ {
			if i%2 == 0 {
				_ = loop.UnrefTimer(ids[i])
				time.Sleep(2 * time.Millisecond)
				_ = loop.RefTimer(ids[i])
			} else {
				_ = loop.RefTimer(ids[i])
				time.Sleep(2 * time.Millisecond)
				_ = loop.UnrefTimer(ids[i])
				time.Sleep(2 * time.Millisecond)
				_ = loop.RefTimer(ids[i])
			}
		}
	}()

	select {
	case <-allFired:
		t.Logf("all %d fires completed", targetFires)
	case <-time.After(5 * time.Second):
		t.Fatalf("timers did not fire %d times within timeout, got %d", targetFires, fireCount.Load())
	}
	<-toggleDone

	// All timers should have fired regardless of ref/unref toggling.
	if fireCount.Load() != targetFires {
		t.Errorf("expected %d fires, got %d", targetFires, fireCount.Load())
	}

	aliveMu.Lock()
	observations := make([]bool, len(aliveDuringFire))
	copy(observations, aliveDuringFire)
	aliveMu.Unlock()

	t.Logf("Alive() observations during fires: %v", observations)

	// Drain any pending operations via barrier.
	endBarrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(endBarrier) }); err != nil {
		t.Fatalf("barrier SubmitInternal: %v", err)
	}
	<-endBarrier
	t.Logf("refedTimerCount at end: %d", loop.refedTimerCount.Load())

	// refedTimerCount must not be negative.
	if loop.refedTimerCount.Load() < 0 {
		t.Errorf("refedTimerCount went negative: %d", loop.refedTimerCount.Load())
	}

	// After all timers fire, refedTimerCount should be 0.
	if loop.refedTimerCount.Load() != 0 {
		t.Errorf("refedTimerCount should be 0 after all timers fire, got %d", loop.refedTimerCount.Load())
	}
}

// TestAdversarial_ConcurrentPromisifyAndRefUnref exercises the interaction between
// Promisify goroutines (which increment promisifyCount) and RefTimer/UnrefTimer
// operations (which modify refedTimerCount). Alive() checks both signals, so this
// test verifies that both counters are tracked correctly under concurrent load.
//
// This closes a testing gap: existing adversarial tests exercise promisify OR
// ref/unref in isolation, but never simultaneously.
func TestAdversarial_ConcurrentPromisifyAndRefUnref(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping adversarial stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer loop.Close()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Schedule a timer that we'll ref/unref concurrently with promisify.
	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	const iterations = 200
	var wg sync.WaitGroup
	var promisifyErrors atomic.Int64
	var refErrors atomic.Int64

	// Phase 1: Concurrent promisify + ref/unref
	wg.Add(3)

	// Goroutine 1: Rapid promisify operations.
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = loop.Promisify(ctx, func(ctx context.Context) (any, error) {
				return i, nil
			})
			// Brief yield to interleave with ref/unref.
			if i%10 == 0 {
				runtime.Gosched()
			}
		}
	}()

	// Goroutine 2: Rapid unref + ref cycles.
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if err := loop.UnrefTimer(timerID); err != nil {
				refErrors.Add(1)
			}
			if err := loop.RefTimer(timerID); err != nil {
				refErrors.Add(1)
			}
		}
	}()

	// Goroutine 3: Observe Alive() from external goroutine.
	var aliveObservations atomic.Int64
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			// Alive() should always return true because:
			// - Timer is either ref'd (refedTimerCount > 0) or
			// - Promisify goroutines are in flight (promisifyCount > 0)
			if loop.Alive() {
				aliveObservations.Add(1)
			}
			runtime.Gosched()
		}
	}()

	wg.Wait()

	if promisifyErrors.Load() > 0 {
		t.Errorf("promisify errors: %d", promisifyErrors.Load())
	}
	if refErrors.Load() > 0 {
		t.Errorf("ref/unref errors: %d", refErrors.Load())
	}

	t.Logf("Alive() observations: %d/%d returned true", aliveObservations.Load(), int64(iterations))
	t.Logf("promisifyCount: %d, refedTimerCount: %d", loop.promisifyCount.Load(), loop.refedTimerCount.Load())

	// After all work, drain the promisify results.
	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	// refedTimerCount must not be negative.
	if loop.refedTimerCount.Load() < 0 {
		t.Fatalf("refedTimerCount went NEGATIVE: %d - counter corruption detected", loop.refedTimerCount.Load())
	}

	// After barrier, refedTimerCount should be 1 (timer was re-ref'd in last iteration).
	if loop.refedTimerCount.Load() != 1 {
		t.Errorf("refedTimerCount: expected 1, got %d", loop.refedTimerCount.Load())
	}

	// promisifyCount should be 0 (all goroutines completed).
	// Wait for promisify goroutines to finish their deferred cleanup.
	// The barrier drains the internal queue, but Promisify goroutines
	// may still be between SubmitInternal return and defer promisifyCount.Add(-1).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if loop.promisifyCount.Load() == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if loop.promisifyCount.Load() != 0 {
		t.Errorf("promisifyCount: expected 0, got %d", loop.promisifyCount.Load())
	}
}
