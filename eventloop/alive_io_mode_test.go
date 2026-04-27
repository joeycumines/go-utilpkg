package eventloop

import (
	"container/heap"
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestIOMode_RefUnrefFromExternalGoroutine verifies that RefTimer/UnrefTimer
// work correctly when called from an external goroutine while the loop is in
// I/O mode (userIOFDCount > 0, canUseFastPath() == false).
//
// In I/O mode, SubmitInternal has no fast path -- it always queues to the
// internal queue. This means submitTimerRefChange called from an external
// goroutine goes through:
//
//	SubmitInternal -> internal queue -> processInternalQueue -> applyTimerRefChange
//
// This test closes the coverage gap in submitTimerRefChange for the
// I/O mode + external goroutine code path.
func TestIOMode_RefUnrefFromExternalGoroutine(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Register an I/O FD to force I/O mode (canUseFastPath returns false).
	fd, fdCleanup := testCreateIOFD(t)
	defer fdCleanup()

	err = loop.RegisterFD(fd, EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()
	time.Sleep(20 * time.Millisecond)

	// Schedule a timer from the external goroutine. ScheduleTimer uses
	// SubmitInternal internally, which in I/O mode queues to the internal queue.
	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Wait for the timer to be registered (internal queue processes it).
	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	// Unref from an EXTERNAL goroutine. This exercises the I/O mode slow path
	// in submitTimerRefChange: SubmitInternal -> internal queue -> processInternalQueue.
	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("UnrefTimer from external goroutine: %v", err)
	}

	if loop.refedTimerCount.Load() != 0 {
		t.Errorf("refedTimerCount should be 0 after unref, got %d", loop.refedTimerCount.Load())
	}

	// Ref from an EXTERNAL goroutine. Same slow path.
	if err := loop.RefTimer(timerID); err != nil {
		t.Fatalf("RefTimer from external goroutine: %v", err)
	}

	if loop.refedTimerCount.Load() != 1 {
		t.Errorf("refedTimerCount should be 1 after ref, got %d", loop.refedTimerCount.Load())
	}

	cancel()
	<-done
}

// TestIOMode_AliveWithUnrefdTimer verifies that Alive() correctly accounts
// for I/O FDs and unref'd timers in I/O mode.
//
// When a timer is unref'd but an I/O FD is still registered, Alive() should
// return true because the FD itself keeps the loop alive. When the FD is
// unregistered and the timer is unref'd, Alive() should return false.
func TestIOMode_AliveWithUnrefdTimer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fd, fdCleanup := testCreateIOFD(t)
	defer fdCleanup()

	err = loop.RegisterFD(fd, EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()
	time.Sleep(20 * time.Millisecond)

	// Schedule a long timer and wait for it to be registered.
	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	// Alive should be true: timer is ref'd + FD is registered.
	if !loop.Alive() {
		t.Fatal("Alive() should be true with ref'd timer and registered FD")
	}

	// Unref the timer. Alive should STILL be true because the FD keeps the loop alive.
	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}

	if !loop.Alive() {
		t.Error("Alive() should still be true: FD keeps loop alive even with unref'd timer")
	}

	// Ref the timer again to verify Alive() still tracks it.
	if err := loop.RefTimer(timerID); err != nil {
		t.Fatalf("RefTimer: %v", err)
	}

	if !loop.Alive() {
		t.Error("Alive() should be true after re-refing the timer")
	}

	// Unref again and unregister FD together.
	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}
	if err := loop.UnregisterFD(fd); err != nil {
		t.Fatalf("UnregisterFD: %v", err)
	}

	// After unregistering the FD with an unref'd timer, we cannot reliably
	// flush via SubmitInternal (the loop may be stuck in PollIO). Instead,
	// cancel the context to wind down and verify the counter state.
	cancel()
	<-done

	// Verify the counter reflects the unref'd state.
	if loop.refedTimerCount.Load() != 0 {
		t.Errorf("refedTimerCount should be 0 after unref, got %d", loop.refedTimerCount.Load())
	}
}

// TestIOMode_SentinelDrainWithUnrefdTimer verifies that the sentinel drain
// pattern (SubmitInternal in a loop checking Alive()) works correctly in
// I/O mode when the only work is an unref'd timer.
//
// This test keeps the FD registered throughout, so the loop stays in I/O mode
// where SubmitInternal reliably wakes via pipe. The sentinel drain should
// complete because Alive() reflects the unref'd timer state.
func TestIOMode_SentinelDrainWithUnrefdTimer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fd, fdCleanup := testCreateIOFD(t)
	defer fdCleanup()

	err = loop.RegisterFD(fd, EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()
	time.Sleep(20 * time.Millisecond)

	// Schedule a short timer that fires quickly. Use a longer delay to ensure
	// both timers are registered before it fires.
	var executed atomic.Bool
	_, err = loop.ScheduleTimer(200*time.Millisecond, func() {
		executed.Store(true)
	})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Also schedule a long timer and unref it.
	longTimerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer long: %v", err)
	}
	waitForCounter(t, &loop.refedTimerCount, 2, 2*time.Second)

	if err := loop.UnrefTimer(longTimerID); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}

	// The short timer is still ref'd (refedTimerCount == 1).
	// Run the sentinel drain until the short timer fires and the loop becomes idle.
	// Note: FD is still registered, so Alive() remains true due to userIOFDCount > 0.
	// The sentinel drain completes because it detects the timer fired via the
	// executed flag, proving the unref'd long timer did not interfere with processing.
	//
	// Use a timeout-based approach: each sentinel iteration costs ~5-10µs, so
	// 10,000 iterations would only cover ~50-100ms — not enough for a 200ms timer.
	// Instead, run until the timer fires or we exceed a generous deadline.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for short timer to fire")
		default:
		}
		stepDone := make(chan struct{})
		if err := loop.SubmitInternal(func() { close(stepDone) }); err != nil {
			break
		}
		<-stepDone
		if executed.Load() {
			break
		}
	}

	if !executed.Load() {
		t.Error("short timer callback should have executed")
	}

	cancel()
	<-done
}

// TestIOMode_ConcurrentRefUnrefUnderLoad verifies that concurrent RefTimer
// and UnrefTimer calls from multiple external goroutines maintain consistent
// refedTimerCount in I/O mode.
//
// This exercises the SubmitInternal -> internal queue -> applyTimerRefChange
// path under concurrent load, ensuring the atomic counter updates are correct.
func TestIOMode_ConcurrentRefUnrefUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fd, fdCleanup := testCreateIOFD(t)
	defer fdCleanup()

	err = loop.RegisterFD(fd, EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()
	time.Sleep(20 * time.Millisecond)

	// Schedule 50 timers. All are ref'd by default.
	const numTimers = 50
	ids := make([]TimerID, numTimers)
	for i := range ids {
		ids[i], err = loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer %d: %v", i, err)
		}
	}
	waitForCounter(t, &loop.refedTimerCount, numTimers, 2*time.Second)

	// Launch 10 goroutines that each unref a different timer.
	const numUnrefGoroutines = 10
	var unrefWg sync.WaitGroup
	unrefWg.Add(numUnrefGoroutines)
	var unrefErrors atomic.Int64
	for i := 0; i < numUnrefGoroutines; i++ {
		go func(idx int) {
			defer unrefWg.Done()
			if err := loop.UnrefTimer(ids[idx]); err != nil {
				t.Errorf("UnrefTimer %d: %v", idx, err)
				unrefErrors.Add(1)
			}
		}(i)
	}
	unrefWg.Wait()

	if unrefErrors.Load() > 0 {
		t.Fatalf("%d unref errors", unrefErrors.Load())
	}

	// After 10 unreffed, count should be numTimers - 10.
	expected := int32(numTimers - numUnrefGoroutines)
	if loop.refedTimerCount.Load() != expected {
		t.Errorf("refedTimerCount after unref: expected %d, got %d", expected, loop.refedTimerCount.Load())
	}

	// Launch 10 goroutines that each ref a different timer (the same 10 we unref'd).
	var refWg sync.WaitGroup
	refWg.Add(numUnrefGoroutines)
	var refErrors atomic.Int64
	for i := 0; i < numUnrefGoroutines; i++ {
		go func(idx int) {
			defer refWg.Done()
			if err := loop.RefTimer(ids[idx]); err != nil {
				t.Errorf("RefTimer %d: %v", idx, err)
				refErrors.Add(1)
			}
		}(i)
	}
	refWg.Wait()

	if refErrors.Load() > 0 {
		t.Fatalf("%d ref errors", refErrors.Load())
	}

	// After re-reffing all 10, count should be back to numTimers.
	if loop.refedTimerCount.Load() != numTimers {
		t.Errorf("refedTimerCount after re-ref: expected %d, got %d", numTimers, loop.refedTimerCount.Load())
	}

	// Clean up.
	cancel()
	<-done
}

// TestSubmitTimerRefChange_TerminatedState verifies that submitTimerRefChange
// returns ErrLoopTerminated when called on a terminated loop.
func TestSubmitTimerRefChange_TerminatedState(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()

	// Schedule a timer so we have an ID to ref/unref.
	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Terminate the loop.
	cancel()
	<-done

	// Now call RefTimer on the terminated loop.
	err = loop.RefTimer(timerID)
	if err != ErrLoopTerminated {
		t.Errorf("RefTimer on terminated loop: expected ErrLoopTerminated, got %v", err)
	}

	err = loop.UnrefTimer(timerID)
	if err != ErrLoopTerminated {
		t.Errorf("UnrefTimer on terminated loop: expected ErrLoopTerminated, got %v", err)
	}
}

// TestSubmitTimerRefChange_OnLoopGoroutine verifies the isLoopThread() fast path
// where RefTimer/UnrefTimer is called from within a callback (on the loop goroutine).
func TestSubmitTimerRefChange_OnLoopGoroutine(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()

	result := make(chan int32, 2)

	// Schedule a long-lived timer, then call UnrefTimer from within a SubmitInternal
	// callback (on the loop goroutine). This exercises the isLoopThread() == true path.
	id1, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	err = loop.SubmitInternal(func() {
		// This runs on the loop goroutine, so isLoopThread() should return true.
		if unrefErr := loop.UnrefTimer(id1); unrefErr != nil {
			t.Logf("UnrefTimer from callback: %v", unrefErr)
		}
		result <- loop.refedTimerCount.Load()
	})
	if err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}

	select {
	case count := <-result:
		t.Logf("refedTimerCount after on-loop unref: %d", count)
		if count != 0 {
			t.Errorf("expected refedTimerCount 0 after unref, got %d", count)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for SubmitInternal callback")
	}

	// Now ref it back from on-loop.
	err = loop.SubmitInternal(func() {
		if refErr := loop.RefTimer(id1); refErr != nil {
			t.Logf("RefTimer from callback: %v", refErr)
		}
		result <- loop.refedTimerCount.Load()
	})
	if err != nil {
		t.Fatalf("SubmitInternal ref: %v", err)
	}

	select {
	case count := <-result:
		t.Logf("refedTimerCount after on-loop ref: %d", count)
		if count != 1 {
			t.Errorf("expected refedTimerCount 1 after ref, got %d", count)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for SubmitInternal ref callback")
	}

	cancel()
	<-done
}

// TestAlive_MicrotaskPath verifies that Alive() returns true when microtasks
// are pending in the ring buffer, and false after they are drained.
//
// This closes the coverage gap on loop.go:1447 — the
//
//	!l.microtasks.IsEmpty() || !l.nextTickQueue.IsEmpty()
//
// check was previously untested. The approach uses SubmitInternal to run on
// the loop goroutine during processInternalQueue (tick step 2), which runs
// BEFORE drainMicrotasks (tick step 5). This ensures the microtask ring is
// populated when we check Alive().
func TestAlive_MicrotaskPath(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()

	// Wait for loop to start.
	time.Sleep(20 * time.Millisecond)

	// Strategy: SubmitInternal callback runs during processInternalQueue.
	// Within that callback, push a microtask directly to the ring, then
	// check Alive() — it should see the microtask as non-empty.
	type result struct {
		aliveWithMicrotask   bool
		aliveAfterDrain      bool
		microtaskWasExecuted bool
	}
	var res result
	var microtaskRan atomic.Bool

	resCh := make(chan result, 1)
	err = loop.SubmitInternal(func() {
		// No timers, no external/internal queue items at this point.
		// Push a microtask directly to the ring (bypasses externalMu since
		// we're on the loop goroutine — safe because only drainMicrotasks
		// pops from the loop goroutine).
		loop.microtasks.Push(func() {
			microtaskRan.Store(true)
		})

		// Alive() should return true — microtask ring is non-empty.
		res.aliveWithMicrotask = loop.Alive()

		// Now drain the microtasks manually to verify Alive() returns false.
		loop.drainMicrotasks()
		res.microtaskWasExecuted = microtaskRan.Load()

		// After draining, with no other signals, Alive() should return false.
		res.aliveAfterDrain = loop.Alive()

		resCh <- res
	})
	if err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}

	select {
	case res := <-resCh:
		if !res.aliveWithMicrotask {
			t.Error("Alive() should return true when microtasks are pending")
		}
		if !res.microtaskWasExecuted {
			t.Error("microtask should have been executed by drainMicrotasks")
		}
		if res.aliveAfterDrain {
			t.Error("Alive() should return false after microtasks drained (no other signals)")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for SubmitInternal callback")
	}

	cancel()
	<-done
}

// TestAlive_NextTickPath verifies that Alive() returns true when nextTick
// callbacks are pending in the ring buffer.
//
// This covers the !l.nextTickQueue.IsEmpty() branch in Alive().
func TestAlive_NextTickPath(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()

	time.Sleep(20 * time.Millisecond)

	type result struct {
		aliveWithNextTick bool
		aliveAfterDrain   bool
		nextTickRan       bool
	}
	var nextTickRan atomic.Bool
	resCh := make(chan result, 1)

	err = loop.SubmitInternal(func() {
		// Push directly to nextTickQueue (safe on loop goroutine).
		loop.nextTickQueue.Push(func() {
			nextTickRan.Store(true)
		})

		var res result
		res.aliveWithNextTick = loop.Alive()

		// Drain to execute the nextTick callback.
		loop.drainMicrotasks() // drainMicrotasks processes nextTick first
		res.nextTickRan = nextTickRan.Load()
		res.aliveAfterDrain = loop.Alive()

		resCh <- res
	})
	if err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}

	select {
	case res := <-resCh:
		if !res.aliveWithNextTick {
			t.Error("Alive() should return true when nextTick queue is pending")
		}
		if !res.nextTickRan {
			t.Error("nextTick callback should have been executed")
		}
		if res.aliveAfterDrain {
			t.Error("Alive() should return false after nextTick drained (no other signals)")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for SubmitInternal callback")
	}

	cancel()
	<-done
}

// TestAlive_UserIOFDOnly verifies that Alive() returns true when only
// userIOFDCount > 0 (no timers, no queues, no promisify goroutines).
//
// This covers the l.userIOFDCount.Load() > 0 branch in Alive().
func TestAlive_UserIOFDOnly(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fd, fdCleanup := testCreateIOFD(t)
	defer fdCleanup()

	err = loop.RegisterFD(fd, EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()

	time.Sleep(20 * time.Millisecond)

	// On the loop goroutine, verify that Alive() returns true with only FD registered.
	resCh := make(chan bool, 1)
	err = loop.SubmitInternal(func() {
		// At this point: no timers, no queue items, no promisify.
		// Only userIOFDCount > 0 keeps the loop alive.
		resCh <- loop.Alive()
	})
	if err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}

	select {
	case alive := <-resCh:
		if !alive {
			t.Error("Alive() should return true when userIOFDCount > 0 (I/O mode)")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
	<-done
}

// TestAlive_AllSignalsExercised exercises 4 of the 6 Alive() signals
// independently (refedTimerCount, internalQueue, microtasks, nextTickQueue),
// verifying that each one can make Alive() return true in isolation, and that
// Alive() returns false when all are drained.
//
// The remaining 2 signals (promisifyCount, userIOFDCount) are covered by:
//   - promisifyCount: TestAdversarial_PromisifyAliveAccuracy
//   - userIOFDCount: TestAlive_UserIOFDOnly
func TestAlive_AllSignalsExercised(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()

	time.Sleep(20 * time.Millisecond)

	type signalTest struct {
		name    string
		setup   func() // called on loop goroutine via SubmitInternal
		cleanup func() // called on loop goroutine to remove the signal
	}
	tests := []signalTest{
		{
			name: "refedTimerCount",
			setup: func() {
				_, _ = loop.ScheduleTimer(time.Hour, func() {})
			},
			cleanup: func() {
				// Cancel all timers via internal access
				loop.internalQueueMu.Lock()
				for loop.timers.Len() > 0 {
					t := heap.Pop(&loop.timers).(*timer)
					t.canceled.Store(true)
					t.refed.Store(false)
					loop.refedTimerCount.Add(-1)
					delete(loop.timerMap, t.id)
				}
				loop.internalQueueMu.Unlock()
			},
		},
		{
			name: "internalQueue",
			setup: func() {
				// Push directly to internal queue
				_ = loop.SubmitInternal(func() {
					// Re-push to keep queue non-empty for Alive() check
					loop.internal.Push(func() {})
				})
			},
			cleanup: func() {
				// Drain internal queue
				for {
					loop.internalQueueMu.Lock()
					_, ok := loop.internal.Pop()
					loop.internalQueueMu.Unlock()
					if !ok {
						break
					}
				}
			},
		},
		{
			name: "microtasks",
			setup: func() {
				loop.microtasks.Push(func() {
					// Re-push to keep ring non-empty
					loop.microtasks.Push(func() {})
				})
			},
			cleanup: func() {
				loop.drainMicrotasks()
			},
		},
		{
			name: "nextTickQueue",
			setup: func() {
				loop.nextTickQueue.Push(func() {
					// Re-push to keep ring non-empty
					loop.nextTickQueue.Push(func() {})
				})
			},
			cleanup: func() {
				loop.drainMicrotasks()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type result struct {
				aliveAfterSetup bool
				aliveAfterClean bool
			}
			resCh := make(chan result, 1)

			err := loop.SubmitInternal(func() {
				var res result
				// First verify Alive() is false before setup
				if loop.Alive() {
					t.Errorf("Alive() should be false before setup of %s", tt.name)
					resCh <- res
					return
				}

				tt.setup()

				// Give setup time to take effect for external signals
				// (e.g., ScheduleTimer submits to internal queue first)
				// For signals that are already active on-loop, check immediately.
				res.aliveAfterSetup = loop.Alive()

				// For ScheduleTimer, the refedTimerCount is incremented via SubmitInternal
				// so we need to wait one more tick for it to be applied.
				if tt.name == "refedTimerCount" {
					// ScheduleTimer uses SubmitInternal internally, so the counter
					// update happens when that internal task processes. Push a barrier.
					barrier := make(chan struct{})
					_ = loop.SubmitInternal(func() { close(barrier) })
					<-barrier
					res.aliveAfterSetup = loop.Alive()
				}

				tt.cleanup()

				// Drain any remaining internal tasks from cleanup
				for {
					loop.internalQueueMu.Lock()
					_, ok := loop.internal.Pop()
					loop.internalQueueMu.Unlock()
					if !ok {
						break
					}
				}

				res.aliveAfterClean = loop.Alive()
				resCh <- res
			})
			if err != nil {
				t.Fatalf("SubmitInternal: %v", err)
			}

			select {
			case res := <-resCh:
				if !res.aliveAfterSetup {
					t.Errorf("Alive() should return true after %s setup", tt.name)
				}
				if res.aliveAfterClean {
					t.Errorf("Alive() should return false after %s cleanup", tt.name)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("timeout")
			}
		})
	}

	cancel()
	<-done
}

// TestCancelTimers_RaceWithShutdown exercises the TOCTOU race window in CancelTimers
// between the state check (line 1888) and the SubmitInternal call (line 1901).
// When the loop terminates between these two points, SubmitInternal returns
// ErrLoopTerminated, which CancelTimers propagates as error for all IDs.
func TestCancelTimers_RaceWithShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping race-condition stress test in short mode")
	}

	const iterations = 500
	const goroutines = 8
	var hitError atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < iterations; i++ {
		loop, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		// Schedule timers for each goroutine.
		batchIDs := make([][]TimerID, goroutines)
		for j := 0; j < goroutines; j++ {
			ids := make([]TimerID, 5)
			for k := range ids {
				ids[k], err = loop.ScheduleTimer(time.Hour, func() {})
				if err != nil {
					t.Fatalf("ScheduleTimer: %v", err)
				}
			}
			batchIDs[j] = ids
		}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			loop.Run(ctx)
		}()

		wg.Add(goroutines)
		for j := 0; j < goroutines; j++ {
			go func(ids []TimerID) {
				defer wg.Done()
				for k := 0; k < 100; k++ {
					runtime.Gosched()
				}
				errs := loop.CancelTimers(ids)
				for _, e := range errs {
					if e != nil {
						hitError.Add(1)
					}
				}
			}(batchIDs[j])
		}

		runtime.Gosched()
		cancel()

		wg.Wait()
		<-done
	}

	t.Logf("Hit CancelTimers error path %d times across %d iterations", hitError.Load(), iterations)
}

// TestSubmitTimerRefChange_RaceWithShutdown exercises the TOCTOU race window
// in submitTimerRefChange between the state check (line 1384) and the
// SubmitInternal call (line 1394). When the loop terminates between these
// two points, SubmitInternal returns ErrLoopTerminated, which submitTimerRefChange
// propagates as its return value.
//
// This test runs many iterations to hit the narrow race window and covers the
// error path at loop.go:1397-1398.
func TestSubmitTimerRefChange_RaceWithShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping race-condition stress test in short mode")
	}

	const iterations = 500
	const goroutines = 8
	var hitError atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < iterations; i++ {
		loop, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		// Schedule timers so we have valid IDs.
		timerIDs := make([]TimerID, goroutines)
		for j := range timerIDs {
			timerIDs[j], err = loop.ScheduleTimer(time.Hour, func() {})
			if err != nil {
				t.Fatalf("ScheduleTimer: %v", err)
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			loop.Run(ctx)
		}()

		// Ramp up: multiple goroutines trying to ref/unref concurrently.
		wg.Add(goroutines)
		for j := 0; j < goroutines; j++ {
			go func(idx int) {
				defer wg.Done()
				// Busy-wait briefly to increase contention.
				for k := 0; k < 100; k++ {
					runtime.Gosched()
				}
				if err := loop.UnrefTimer(timerIDs[idx]); err != nil {
					hitError.Add(1)
				}
			}(j)
		}

		// Cancel concurrently with the goroutines.
		runtime.Gosched()
		cancel()

		wg.Wait()
		<-done
	}

	t.Logf("Hit SubmitInternal error path in %d/%d iterations (%.1f%%)",
		hitError.Load(), iterations, float64(hitError.Load())/float64(iterations)*100)
}

// waitForCounter polls an atomic int32 until it reaches the expected value
// or the timeout expires.
func waitForCounter(t *testing.T, counter *atomic.Int32, expected int32, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if counter.Load() == expected {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("counter did not reach %d within %v (got %d)", expected, timeout, counter.Load())
}
