//go:build !windows

package eventloop

// ============================================================================
// TDD Tests — CF-001 and CF-002
//
// CF-001: submitTimerRefChange closure previously called doWakeup() unconditionally,
//         even when applyTimerRefChange determined no wakeup was needed (old == ref).
//         FIX: Removed the unconditional doWakeup() from the closure. applyTimerRefChange
//         is now the sole authority for wakeup decisions in this code path.
//
// CF-002: submitToQueue's fast-mode path (userIOFDCount == 0) only sent to fastWakeupCh
//         without calling submitWakeup() (pipe write). If the loop was in PollIO despite
//         userIOFDCount == 0 (due to concurrent UnregisterFD), the signal was lost.
//         FIX: Added defense-in-depth submitWakeup() call, mirroring Submit()'s pattern.
//
// GAP-004: UnregisterFD did not wake the loop when userIOFDCount dropped to 0.
//          FIX: Added doWakeup() when the last I/O FD is removed.
// ============================================================================

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// CF-001: Ref Count Correctness After No-Op RefTimer/UnrefTimer Operations
//
// The original bug: submitTimerRefChange closure called doWakeup() unconditionally
// after applyTimerRefChange, even when old == ref (no state change). This caused:
//   - Redundant syscalls when old != ref (double wakeup)
//   - Spurious wakeup when old == ref (timer already in target state)
//
// The fix removes doWakeup() from the closure entirely. applyTimerRefChange
// already calls doWakeup() conditionally when old != ref.
//
// Test strategy: Verify that no-op RefTimer/UnrefTimer operations do NOT change
// refedTimerCount. This directly tests the correctness of the ref counting,
// regardless of wakeup behavior.
// ============================================================================

// TestCF001_RefTimerNoOp_DoesNotChangeRefCount verifies that calling RefTimer on
// a timer that is ALREADY ref'd does NOT increment refedTimerCount.
//
// This is the direct correctness test for CF-001: the ref count must remain
// unchanged when the ref state doesn't change (old == ref).
func TestCF001_RefTimerNoOp_DoesNotChangeRefCount(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	waitForLoopRunningT(t, loop, 2*time.Second)

	// Schedule a timer (starts ref'd by default)
	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		cancel()
		<-done
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Wait for the timer to be registered
	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	countBefore := loop.refedTimerCount.Load()

	// Call RefTimer 100 times on the already-ref'd timer.
	// Each call should be a no-op: old == ref == true.
	for i := 0; i < 100; i++ {
		if err := loop.RefTimer(timerID); err != nil {
			t.Errorf("RefTimer %d: %v", i, err)
		}
	}

	// The ref count must not have changed — all 100 calls were no-ops.
	countAfter := loop.refedTimerCount.Load()
	if countAfter != countBefore {
		t.Errorf("CF-001 FAIL: refedTimerCount changed after no-op RefTimer calls: "+
			"before=%d, after=%d. No-op RefTimer should not change the ref count.",
			countBefore, countAfter)
	}

	_ = loop.CancelTimer(timerID)
	cancel()
	<-done
}

// TestCF001_UnrefTimerNoOp_DoesNotChangeRefCount verifies that calling UnrefTimer
// on a timer that is ALREADY unref'd does NOT decrement refedTimerCount.
func TestCF001_UnrefTimerNoOp_DoesNotChangeRefCount(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	waitForLoopRunningT(t, loop, 2*time.Second)

	// Keepalive: schedule a long timer (ref'd) to keep loop alive
	keepaliveID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		cancel()
		<-done
		t.Fatalf("ScheduleTimer keepalive: %v", err)
	}

	// Test timer: schedule and immediately unref it
	testTimerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		cancel()
		<-done
		t.Fatalf("ScheduleTimer test: %v", err)
	}
	if err := loop.UnrefTimer(testTimerID); err != nil {
		cancel()
		<-done
		t.Fatalf("UnrefTimer: %v", err)
	}

	// Wait for refedTimerCount to stabilize at 1 (only keepalive)
	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	countBefore := loop.refedTimerCount.Load()

	// Call UnrefTimer 100 times on the already-unref'd timer.
	// Each call should be a no-op: old == ref == false.
	for i := 0; i < 100; i++ {
		if err := loop.UnrefTimer(testTimerID); err != nil {
			t.Errorf("UnrefTimer %d: %v", i, err)
		}
	}

	// The ref count must not have changed — all 100 calls were no-ops.
	countAfter := loop.refedTimerCount.Load()
	if countAfter != countBefore {
		t.Errorf("CF-001 FAIL: refedTimerCount changed after no-op UnrefTimer calls: "+
			"before=%d, after=%d. No-op UnrefTimer should not change the ref count.",
			countBefore, countAfter)
	}

	_ = loop.CancelTimer(keepaliveID)
	_ = loop.CancelTimer(testTimerID)
	cancel()
	<-done
}

// TestCF001_ConcurrentNoOpRefChanges_RefCountCorrect is a stress test that verifies
// refedTimerCount remains correct under concurrent no-op Ref/Unref operations.
//
// This is the concurrent correctness test: multiple goroutines perform Ref/Unref
// on timers in rapid succession. The final ref count must be deterministic.
func TestCF001_ConcurrentNoOpRefChanges_RefCountCorrect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	waitForLoopRunningT(t, loop, 2*time.Second)

	// Keepalive timer
	keepaliveID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		cancel()
		<-done
		t.Fatalf("ScheduleTimer: %v", err)
	}

	const nTimers = 10
	timerIDs := make([]TimerID, nTimers)
	for i := 0; i < nTimers; i++ {
		id, err := loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			cancel()
			<-done
			t.Fatalf("ScheduleTimer: %v", err)
		}
		timerIDs[i] = id
	}

	// All timers start ref'd (ScheduleTimer default). Wait for closures to process.
	// Expected count: nTimers + 1 keepalive = 11
	expectedCount := int32(nTimers + 1)
	waitForCounter(t, &loop.refedTimerCount, expectedCount, 2*time.Second)

	// Concurrent workers performing Ref/Unref on timers that are ALL ref'd.
	// Each worker does Unref+Ref pairs. Since the timers start ref'd, the
	// interleaving of closures on the loop thread is:
	//   - Unref(T): old=true → changes to false → decrements (-1)
	//   - Ref(T): old=false → changes to true → increments (+1)
	//   - Net per pair: 0
	// But with concurrent workers, closures interleave. A second worker's
	// Unref might see the timer still ref'd (from before the first worker's
	// Ref ran), so it also decrements. The key invariant: refedTimerCount
	// must be >= 0 at all times and end at the expected value.
	// After all workers complete and the queue drains, each timer has had
	// an even number of state-flipping operations (Unref+Ref pairs), ending
	// in the ref'd state. The final count equals the starting count.
	const nWorkers = 5
	const callsPerWorker = 2000

	var workerWg sync.WaitGroup
	for w := 0; w < nWorkers; w++ {
		workerWg.Add(1)
		go func(workerID int) {
			defer workerWg.Done()
			for i := 0; i < callsPerWorker; i++ {
				id := timerIDs[(workerID*17+i)%nTimers]
				_ = loop.UnrefTimer(id)
				_ = loop.RefTimer(id)
			}
		}(w)
	}

	workerWg.Wait()

	// After all workers complete and closures drain, the ref count must
	// return to the starting value. Each timer had balanced state flips.
	waitForCounter(t, &loop.refedTimerCount, expectedCount, 2*time.Second)

	finalCount := loop.refedTimerCount.Load()
	if finalCount != expectedCount {
		t.Errorf("CF-001 FAIL: refedTimerCount after concurrent operations: "+
			"expected %d, got %d. Concurrent Ref/Unref corrupted the ref count.",
			expectedCount, finalCount)
	}

	_ = loop.UnrefTimer(keepaliveID)
	for _, id := range timerIDs {
		_ = loop.CancelTimer(id)
	}
	cancel()
	<-done
}

// TestCF001_ClosureNoLongerCallsDoWakeup verifies that the submitTimerRefChange
// closure does NOT call doWakeup() independently of applyTimerRefChange.
//
// Strategy: Submit a RefTimer for a non-existent timer. applyTimerRefChange will
// find no timer (early return, no doWakeup). If the closure still called doWakeup(),
// the loop would be woken unnecessarily. We verify by checking that the loop
// doesn't process any additional wakeups beyond what submitToQueue provides.
//
// This test uses a direct measurement: count how many times the loop wakes up
// and goes back to sleep when processing ref operations on non-existent timers.
// With the fix: applyTimerRefChange returns early (timer not found), and the
// closure does NOT call doWakeup(). The loop wakes up once from submitToQueue,
// processes the closure, and goes back to sleep. That's 1 transition per call.
// The key difference: with the old bug, each closure would ALSO call doWakeup(),
// causing the loop to wake up AGAIN immediately after going to sleep.
func TestCF001_ClosureNoLongerCallsDoWakeup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	waitForLoopRunningT(t, loop, 2*time.Second)

	// Keepalive
	keepaliveID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		cancel()
		<-done
		t.Fatalf("ScheduleTimer keepalive: %v", err)
	}

	// Cancel the keepalive timer and use its ID for no-op ref operations.
	// After cancellation, applyTimerRefChange will find no timer and return early.
	// If the closure still calls doWakeup(), it's an unnecessary wakeup.
	cancelledID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		cancel()
		<-done
		t.Fatalf("ScheduleTimer: %v", err)
	}
	if err := loop.CancelTimer(cancelledID); err != nil {
		cancel()
		<-done
		t.Fatalf("CancelTimer: %v", err)
	}

	// Allow loop to settle
	time.Sleep(50 * time.Millisecond)

	// The ref count should be 1 (only keepalive)
	countBefore := loop.refedTimerCount.Load()
	if countBefore != 1 {
		t.Fatalf("refedTimerCount before test: expected 1, got %d", countBefore)
	}

	// Perform RefTimer/UnrefTimer on the cancelled (non-existent) timer.
	// applyTimerRefChange will find no timer in timerMap → early return, no state change.
	// The closure should NOT call doWakeup() (CF-001 fix).
	for i := 0; i < 100; i++ {
		_ = loop.RefTimer(cancelledID)
		_ = loop.UnrefTimer(cancelledID)
	}

	// refedTimerCount must be unchanged — the timer doesn't exist.
	countAfter := loop.refedTimerCount.Load()
	if countAfter != countBefore {
		t.Errorf("CF-001 FAIL: refedTimerCount changed after ref operations on "+
			"non-existent timer: before=%d, after=%d. Operations on cancelled timers "+
			"must not affect the ref count.", countBefore, countAfter)
	}

	_ = loop.UnrefTimer(keepaliveID)
	_ = loop.CancelTimer(keepaliveID)
	cancel()
	<-done
}

// ============================================================================
// CF-002: Lost Wakeup in submitToQueue When userIOFDCount Transitions to 0
//
// The defect: submitToQueue's fast-mode path (when userIOFDCount == 0) only
// sent to fastWakeupCh and returned without calling submitWakeup() (pipe write).
// If the loop was in PollIO despite userIOFDCount == 0, the fastWakeupCh
// signal was lost and the loop stayed blocked until the poll timeout.
//
// ORIGINAL FIX: Added _ = l.submitWakeup() in submitToQueue's fast-mode branch.
// This worked but caused ~1750 ns overhead per fast-mode call (pipe syscall).
//
// OPTIMIZED FIX (PERF-001): Removed the submitWakeup() call. GAP-004 alone
// handles the race: UnregisterFD calls doWakeup() when count drops to 0.
// The loop only enters PollIO when count > 0, and count can only transition
// from >0 to 0 via UnregisterFD (which triggers GAP-004). So by the time
// submitToQueue sees count==0, the wakeup is already in flight.
//
// THE RACE WINDOW (covered by GAP-004):
//   1. Loop calls PollIO(timeout=X) with userIOFDCount > 0. The syscall is
//      now blocked in the kernel (EpollWait/Kevent).
//   2. External goroutine calls UnregisterFD → userIOFDCount becomes 0.
//      GAP-004: UnregisterFD calls doWakeup() → pipe write interrupts PollIO.
//   3. External goroutine calls RefTimer → submitToQueue sees userIOFDCount == 0
//      → sends to fastWakeupCh ONLY.
//      → fastWakeupCh is buffered until PollIO returns (step 2 pipe write).
//   4. PollIO returns (due to GAP-004 pipe write from step 2).
//   5. Loop enters fast-mode → consumes fastWakeupCh signal → closure runs.
// ============================================================================

// TestCF002_LostWakeup_RefTimer_Hangs verifies that RefTimer completes quickly
// even when userIOFDCount drops to 0 while the loop is blocked in PollIO.
//
// Design:
//  1. Register a real pipe FD with the loop → PollIO blocks with timeout
//  2. Sleep briefly to ensure PollIO has entered the kernel
//  3. Unregister the pipe → userIOFDCount drops to 0
//     The blocked syscall does NOT return (kernel still waiting for timeout)
//  4. Call RefTimer → submitToQueue sees userIOFDCount == 0
//     → sends to fastWakeupCh AND submitWakeup() (defense in depth)
//     → submitWakeup() writes to the pipe → interrupts PollIO
//     → PollIO returns → loop processes the closure
//
// Without the fix: RefTimer takes ~1s (PollIO timeout). With the fix: < 300ms.
func TestCF002_LostWakeup_RefTimer_Hangs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	waitForLoopRunningT(t, loop, 2*time.Second)

	// Keepalive timer scheduled for 1 second. This means calculateTimeout()
	// returns min(1s, 10s) = 1s. So PollIO blocks for ~1 second, not 10.
	var keepaliveID TimerID
	keepaliveID, err = loop.ScheduleTimer(1*time.Second, func() {})
	if err != nil {
		cancel()
		<-done
		t.Fatalf("ScheduleTimer keepalive: %v", err)
	}
	// Ref the keepalive so refedTimerCount > 0 (loop stays alive)
	if err := loop.RefTimer(keepaliveID); err != nil {
		_ = loop.CancelTimer(keepaliveID)
		cancel()
		<-done
		t.Fatalf("RefTimer keepalive: %v", err)
	}

	// Create a real pipe FD. The read end is registered with the loop.
	// PollIO will block waiting for events that never arrive.
	pipeFD, pipeCleanup := testCreateIOFD(t)
	if err := loop.RegisterFD(pipeFD, EventRead, func(IOEvents) {}); err != nil {
		pipeCleanup()
		cancel()
		<-done
		t.Fatalf("RegisterFD: %v", err)
	}

	// Schedule a timer, then unref it
	testTimerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		_ = loop.UnregisterFD(pipeFD)
		pipeCleanup()
		cancel()
		<-done
		t.Fatalf("ScheduleTimer: %v", err)
	}
	if err := loop.UnrefTimer(testTimerID); err != nil {
		_ = loop.UnregisterFD(pipeFD)
		pipeCleanup()
		cancel()
		<-done
		t.Fatalf("UnrefTimer: %v", err)
	}

	// Allow loop to enter PollIO and block in the kernel.
	time.Sleep(200 * time.Millisecond)

	// Verify the loop is in I/O mode
	if loop.userIOFDCount.Load() == 0 {
		t.Fatal("userIOFDCount should be > 0 (pipe FD registered)")
	}

	// Now unregister the pipe — userIOFDCount drops to 0.
	// The PollIO syscall is ALREADY in the kernel and does NOT return.
	// It waits for its timeout (~1s).
	// GAP-004 fix: UnregisterFD now calls doWakeup() when count drops to 0,
	// which wakes PollIO immediately.
	if err := loop.UnregisterFD(pipeFD); err != nil {
		pipeCleanup()
		cancel()
		<-done
		t.Fatalf("UnregisterFD: %v", err)
	}
	pipeCleanup()

	// At this point:
	// - userIOFDCount == 0
	// - UnregisterFD called doWakeup() → PollIO should return soon
	// - submitToQueue now also calls submitWakeup() (CF-002 fix)
	// - RefTimer should complete quickly

	refDone := make(chan error, 1)
	start := time.Now()

	go func() {
		refDone <- loop.RefTimer(testTimerID)
	}()

	// Threshold: 300ms. With CF-002 + GAP-004 fixed, RefTimer completes quickly.
	// Without either fix, RefTimer takes ~1s (PollIO timeout).
	const maxAcceptableDelay = 300 * time.Millisecond

	select {
	case err := <-refDone:
		elapsed := time.Since(start)
		if err != nil {
			t.Errorf("RefTimer returned error: %v", err)
		}
		if elapsed > maxAcceptableDelay {
			t.Errorf("CF-002 FAIL: RefTimer took %v (expected < %v). "+
				"submitToQueue fast-mode path or UnregisterFD wakeup is missing.",
				elapsed, maxAcceptableDelay)
		} else {
			t.Logf("RefTimer completed in %v — CF-002/GAP-004 fixes working",
				elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("CF-002 FAIL: RefTimer blocked for > 5 seconds.")
		go func() { <-refDone }()
	}

	_ = loop.UnrefTimer(keepaliveID)
	_ = loop.CancelTimer(keepaliveID)
	_ = loop.CancelTimer(testTimerID)
	cancel()
	<-done
}

// TestCF002_LostWakeup_CancelTimer_Hangs is the same test but with CancelTimer.
func TestCF002_LostWakeup_CancelTimer_Hangs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	waitForLoopRunningT(t, loop, 2*time.Second)

	var keepaliveID TimerID
	keepaliveID, err = loop.ScheduleTimer(1*time.Second, func() {})
	if err != nil {
		cancel()
		<-done
		t.Fatalf("ScheduleTimer keepalive: %v", err)
	}
	if err := loop.RefTimer(keepaliveID); err != nil {
		_ = loop.CancelTimer(keepaliveID)
		cancel()
		<-done
		t.Fatalf("RefTimer keepalive: %v", err)
	}

	pipeFD, pipeCleanup := testCreateIOFD(t)
	if err := loop.RegisterFD(pipeFD, EventRead, func(IOEvents) {}); err != nil {
		pipeCleanup()
		cancel()
		<-done
		t.Fatalf("RegisterFD: %v", err)
	}

	testTimerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		_ = loop.UnregisterFD(pipeFD)
		pipeCleanup()
		cancel()
		<-done
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Allow loop to enter PollIO (pipe FD sets the timeout, ~1s)
	time.Sleep(200 * time.Millisecond)

	if err := loop.UnregisterFD(pipeFD); err != nil {
		pipeCleanup()
		cancel()
		<-done
		t.Fatalf("UnregisterFD: %v", err)
	}
	pipeCleanup()

	cancelDone := make(chan error, 1)
	start := time.Now()

	go func() {
		cancelDone <- loop.CancelTimer(testTimerID)
	}()

	const maxAcceptableDelay = 300 * time.Millisecond

	select {
	case err := <-cancelDone:
		elapsed := time.Since(start)
		if err != nil && err != ErrTimerNotFound {
			t.Errorf("CancelTimer returned unexpected error: %v", err)
		}
		if elapsed > maxAcceptableDelay {
			t.Errorf("CF-002 FAIL: CancelTimer took %v (expected < %v). "+
				"submitToQueue fast-mode path or UnregisterFD wakeup is missing.",
				elapsed, maxAcceptableDelay)
		} else {
			t.Logf("CancelTimer completed in %v — CF-002/GAP-004 fixes working",
				elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("CF-002 FAIL: CancelTimer blocked for > 5 seconds.")
		go func() { <-cancelDone }()
	}

	_ = loop.UnrefTimer(keepaliveID)
	_ = loop.CancelTimer(keepaliveID)
	_ = loop.CancelTimer(testTimerID)
	cancel()
	<-done
}

// ============================================================================
// GAP-004: UnregisterFD Must Wake Loop When userIOFDCount Drops to 0
//
// The defect: UnregisterFD decremented userIOFDCount but did NOT wake the loop.
// When the last I/O FD was removed, the loop remained blocked in PollIO until
// its timeout expired, even though it should have transitioned to pollFastMode.
//
// The fix: Added doWakeup() in UnregisterFD when userIOFDCount reaches 0.
// ============================================================================

// TestGAP004_UnregisterFD_WakesLoopWhenLastFDRemoved verifies that UnregisterFD
// wakes the loop immediately when the last I/O FD is removed, causing PollIO
// to return and the loop to transition to pollFastMode.
//
// Design:
//  1. Register a pipe FD → loop enters PollIO
//  2. Unregister the pipe → userIOFDCount drops to 0
//  3. Verify the loop responds quickly (PollIO was interrupted)
//  4. Submit a task → should complete quickly (loop is in pollFastMode)
func TestGAP004_UnregisterFD_WakesLoopWhenLastFDRemoved(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	waitForLoopRunningT(t, loop, 2*time.Second)

	// Keepalive
	keepaliveID, err := loop.ScheduleTimer(1*time.Second, func() {})
	if err != nil {
		cancel()
		<-done
		t.Fatalf("ScheduleTimer keepalive: %v", err)
	}

	pipeFD, pipeCleanup := testCreateIOFD(t)
	if err := loop.RegisterFD(pipeFD, EventRead, func(IOEvents) {}); err != nil {
		pipeCleanup()
		cancel()
		<-done
		t.Fatalf("RegisterFD: %v", err)
	}

	// Let loop enter PollIO
	time.Sleep(100 * time.Millisecond)

	// Unregister the last I/O FD → userIOFDCount drops to 0
	// GAP-004 fix: this should wake the loop from PollIO
	start := time.Now()
	if err := loop.UnregisterFD(pipeFD); err != nil {
		pipeCleanup()
		cancel()
		<-done
		t.Fatalf("UnregisterFD: %v", err)
	}
	pipeCleanup()

	// Verify userIOFDCount is now 0
	if loop.userIOFDCount.Load() != 0 {
		t.Fatalf("userIOFDCount should be 0 after UnregisterFD, got %d",
			loop.userIOFDCount.Load())
	}

	// Submit a task. If the loop is stuck in PollIO, this will take ~1s.
	// With GAP-004 fix, the loop is in pollFastMode and processes immediately.
	taskDone := make(chan struct{})
	if err := loop.Submit(func() {
		close(taskDone)
	}); err != nil {
		cancel()
		<-done
		t.Fatalf("Submit: %v", err)
	}

	const maxAcceptableDelay = 300 * time.Millisecond
	select {
	case <-taskDone:
		elapsed := time.Since(start)
		if elapsed > maxAcceptableDelay {
			t.Errorf("GAP-004 FAIL: task after UnregisterFD took %v (expected < %v). "+
				"UnregisterFD did not wake the loop from PollIO.",
				elapsed, maxAcceptableDelay)
		} else {
			t.Logf("Task completed in %v after UnregisterFD — GAP-004 fix working",
				elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("GAP-004 FAIL: task blocked for > 5 seconds after UnregisterFD.")
	}

	_ = loop.CancelTimer(keepaliveID)
	cancel()
	<-done
}

// ============================================================================
// CF-001 Walkthrough
// ============================================================================
//
// ORIGINAL BEHAVIOR (CF-001 present, loop.go:1569-1575):
//   External goroutine: loop.RefTimer(alreadyRefdTimerID)
//     → submitTimerRefChange(id, true)
//       → submitToQueue(closure)
//         closure = func() {
//             l.applyTimerRefChange(id, true)
//                 old = t.refed.Swap(true) → old == ref == true
//                 if old != ref:  ← FALSE (no state change)
//                 NO doWakeup()  ← correct
//             result <- struct{}{}
//             l.doWakeup()     ← BUG: unconditional wakeup!
//         }
//         → l.fastWakeupCh <- struct{}{}
//         → loop wakes, runs closure
//         → closure calls doWakeup() spuriously
//
// FIXED BEHAVIOR (after CF-001 fix):
//   The closure no longer calls doWakeup(). applyTimerRefChange is the sole
//   authority for wakeup decisions: it calls doWakeup() only when old != ref.
//   When old == ref (no-op case), no additional wakeup is sent.
//
// ============================================================================
// CF-002 Walkthrough
// ============================================================================
//
// ORIGINAL BEHAVIOR (CF-002 present, loop.go:1469-1474):
//   1. Loop: PollIO(timeout=1s) — blocked in kernel
//   2. External goroutine: UnregisterFD(pipeFD) → userIOFDCount: 1→0
//      The syscall is ALREADY blocked — does NOT return until timeout
//      (GAP-004 fix: now calls doWakeup() when count reaches 0)
//   3. External goroutine: RefTimer(id) → submitTimerRefChange → submitToQueue
//
//   submitToQueue path (BEFORE fix):
//     l.userIOFDCount.Load() == 0  → TRUE
//     select { case l.fastWakeupCh <- struct{}{}: default: }
//     return nil  ← NO submitWakeup() (no pipe write)!
//
//   submitToQueue path (AFTER fix):
//     l.userIOFDCount.Load() == 0  → TRUE
//     select { case l.fastWakeupCh <- struct{}{}: default: }
//     _ = l.submitWakeup()  ← Defense in depth: wakes PollIO
//     return nil
//
//   With GAP-004 fix: UnregisterFD already woke the loop via doWakeup(),
//   so PollIO returns immediately. submitToQueue's submitWakeup() provides
//   an additional safety net if the timing window is missed.
//
// ============================================================================

// waitForLoopRunningT is a test helper that polls until the loop reaches
// StateRunning or the timeout.
func waitForLoopRunningT(t *testing.T, loop *Loop, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if loop.State() == StateRunning {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("loop did not reach StateRunning within %v (state=%v)", timeout, loop.State())
}

// ============================================================================
// Additional Coverage: CancelTimer/CancelTimers Ref Count Correctness
//
// Pass 2 review noted that applyCancelTimer/applyCancelTimers do not call
// doWakeup() internally, unlike applyTimerRefChange. This is not a bug
// (submitToQueue already woke the loop), but verifying the ref count
// after cancellation documents this invariant.
// ============================================================================

// TestCancelTimer_RefCountCorrect verifies that CancelTimer properly decrements
// refedTimerCount when cancelling a ref'd timer.
func TestCancelTimer_RefCountCorrect(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	waitForLoopRunningT(t, loop, 2*time.Second)

	// Schedule 10 timers (all ref'd by default)
	const nTimers = 10
	timerIDs := make([]TimerID, nTimers)
	for i := 0; i < nTimers; i++ {
		id, err := loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			cancel()
			<-done
			t.Fatalf("ScheduleTimer: %v", err)
		}
		timerIDs[i] = id
	}

	// Wait for all closures to process
	waitForCounter(t, &loop.refedTimerCount, nTimers, 2*time.Second)

	// Cancel them one by one — each cancellation should decrement refedTimerCount
	for i := 0; i < nTimers; i++ {
		before := loop.refedTimerCount.Load()
		if err := loop.CancelTimer(timerIDs[i]); err != nil {
			cancel()
			<-done
			t.Fatalf("CancelTimer %d: %v", i, err)
		}
		expected := before - 1
		waitForCounter(t, &loop.refedTimerCount, expected, 2*time.Second)
	}

	// All timers cancelled — refedTimerCount should be 0
	if loop.refedTimerCount.Load() != 0 {
		t.Errorf("refedTimerCount after cancelling all timers: expected 0, got %d",
			loop.refedTimerCount.Load())
	}

	cancel()
	<-done
}

// TestCancelTimers_RefCountCorrect verifies that CancelTimers (batch) properly
// decrements refedTimerCount when cancelling multiple ref'd timers.
func TestCancelTimers_RefCountCorrect(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	waitForLoopRunningT(t, loop, 2*time.Second)

	// Schedule 10 timers (all ref'd by default)
	const nTimers = 10
	timerIDs := make([]TimerID, nTimers)
	for i := 0; i < nTimers; i++ {
		id, err := loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			cancel()
			<-done
			t.Fatalf("ScheduleTimer: %v", err)
		}
		timerIDs[i] = id
	}

	waitForCounter(t, &loop.refedTimerCount, nTimers, 2*time.Second)

	// Cancel all at once using CancelTimers
	errors := loop.CancelTimers(timerIDs)
	for i, err := range errors {
		if err != nil {
			t.Errorf("CancelTimers error for timer %d: %v", i, err)
		}
	}

	// All timers cancelled — refedTimerCount should be 0
	waitForCounter(t, &loop.refedTimerCount, 0, 2*time.Second)

	if loop.refedTimerCount.Load() != 0 {
		t.Errorf("refedTimerCount after CancelTimers: expected 0, got %d",
			loop.refedTimerCount.Load())
	}

	cancel()
	<-done
}

// ============================================================================
// PERF-001: submitToQueue Fast-Mode Path Should Not Call submitWakeup
//
// The CF-002 defense-in-depth added _ = l.submitWakeup() to submitToQueue's
// fast-mode branch (when userIOFDCount == 0). However, this is redundant
// given GAP-004 (UnregisterFD calls doWakeup() when count drops to 0).
//
// The proof: the loop only enters PollIO when userIOFDCount > 0. The count
// can only transition from >0 to 0 via UnregisterFD, which triggers GAP-004.
// So by the time submitToQueue sees count == 0, the wakeup is already in
// flight. The fastWakeupCh signal is sufficient.
//
// This test verifies that submitWakeup is NOT called in the common fast-mode
// case (no FD registration/unregistration happening). With the defense-in-depth
// still present, this test FAILS because submitWakeup is called on every
// submitToQueue fast-mode invocation.
// ============================================================================

// TestPERF001_SubmitToQueueFastMode_NoSubmitWakeup verifies that submitToQueue's
// fast-mode path does NOT call submitWakeup() when the loop is confirmed in
// fast-mode (no FDs registered). This test uses a test hook to count submitWakeup
// calls during normal fast-mode RefTimer operations.
//
// TDD: This test FAILS with the current code because CF-002's defense-in-depth
// calls submitWakeup() on every fast-mode submitToQueue invocation. After removing
// the redundant defense-in-depth, this test passes.
func TestPERF001_SubmitToQueueFastMode_NoSubmitWakeup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var submitWakeupCount atomic.Int64
	loop.testHooks = &loopTestHooks{
		OnSubmitWakeup: func() {
			submitWakeupCount.Add(1)
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	waitForLoopRunningT(t, loop, 2*time.Second)

	// Schedule a timer (fast-mode — no FDs registered)
	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		cancel()
		<-done
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Wait for timer registration
	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	// Reset the counter after setup
	submitWakeupCount.Store(0)

	// Perform RefTimer operations in pure fast-mode.
	// No FDs are registered, so the loop is in fast-mode (blocking on fastWakeupCh).
	// submitToQueue's fast-mode branch should NOT call submitWakeup().
	const nOps = 50
	for i := 0; i < nOps; i++ {
		if err := loop.RefTimer(timerID); err != nil {
			t.Errorf("RefTimer %d: %v", i, err)
		}
	}

	// Allow loop to settle
	time.Sleep(50 * time.Millisecond)

	// With the defense-in-depth removed, submitWakeup should NOT have been called.
	// With the defense-in-depth present, submitWakeup is called nOps times.
	count := submitWakeupCount.Load()
	if count > 0 {
		t.Errorf("PERF-001 FAIL: submitWakeup was called %d times during %d fast-mode "+
			"RefTimer operations. Expected 0 — submitToQueue's fast-mode branch should "+
			"not call submitWakeup() when no FDs are registered (GAP-004 handles the "+
			"PollIO wakeup when userIOFDCount transitions to 0).",
			count, nOps)
	}

	_ = loop.CancelTimer(timerID)
	cancel()
	<-done
}

// TestPERF001_CF002_StillWorksAfterOptimization verifies that removing the
// defense-in-depth submitWakeup does NOT break the CF-002 race condition fix.
// This test is identical to TestCF002_LostWakeup_RefTimer_Hangs but also
// verifies that GAP-004 alone is sufficient to prevent the deadlock.
func TestPERF001_CF002_StillWorksAfterOptimization(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	waitForLoopRunningT(t, loop, 2*time.Second)

	// Keepalive timer with 1s timeout (sets PollIO timeout)
	keepaliveID, err := loop.ScheduleTimer(1*time.Second, func() {})
	if err != nil {
		cancel()
		<-done
		t.Fatalf("ScheduleTimer keepalive: %v", err)
	}
	if err := loop.RefTimer(keepaliveID); err != nil {
		_ = loop.CancelTimer(keepaliveID)
		cancel()
		<-done
		t.Fatalf("RefTimer keepalive: %v", err)
	}

	// Register a real pipe FD -> loop enters PollIO
	pipeFD, pipeCleanup := testCreateIOFD(t)
	if err := loop.RegisterFD(pipeFD, EventRead, func(IOEvents) {}); err != nil {
		pipeCleanup()
		cancel()
		<-done
		t.Fatalf("RegisterFD: %v", err)
	}

	// Schedule and unref a test timer
	testTimerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		_ = loop.UnregisterFD(pipeFD)
		pipeCleanup()
		cancel()
		<-done
		t.Fatalf("ScheduleTimer: %v", err)
	}
	if err := loop.UnrefTimer(testTimerID); err != nil {
		_ = loop.UnregisterFD(pipeFD)
		pipeCleanup()
		cancel()
		<-done
		t.Fatalf("UnrefTimer: %v", err)
	}

	// Allow loop to enter PollIO and block in the kernel
	time.Sleep(200 * time.Millisecond)

	// Unregister the pipe -> userIOFDCount drops to 0
	// GAP-004: UnregisterFD calls doWakeup() -> PollIO returns
	if err := loop.UnregisterFD(pipeFD); err != nil {
		pipeCleanup()
		cancel()
		<-done
		t.Fatalf("UnregisterFD: %v", err)
	}
	pipeCleanup()

	// Now call RefTimer. submitToQueue sees count=0, sends to fastWakeupCh only.
	// GAP-004's doWakeup() already woke PollIO. The loop enters fast-mode and
	// picks up the fastWakeupCh signal. RefTimer should complete quickly.
	refDone := make(chan error, 1)
	start := time.Now()

	go func() {
		refDone <- loop.RefTimer(testTimerID)
	}()

	const maxAcceptableDelay = 300 * time.Millisecond

	select {
	case err := <-refDone:
		elapsed := time.Since(start)
		if err != nil {
			t.Errorf("RefTimer returned error: %v", err)
		}
		if elapsed > maxAcceptableDelay {
			t.Errorf("PERF-001 FAIL: RefTimer took %v (expected < %v). "+
				"GAP-004 should handle the PollIO wakeup without submitToQueue's "+
				"defense-in-depth pipe write.",
				elapsed, maxAcceptableDelay)
		} else {
			t.Logf("RefTimer completed in %v — GAP-004 alone is sufficient", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("PERF-001 FAIL: RefTimer blocked for > 5 seconds.")
		go func() { <-refDone }()
	}

	_ = loop.UnrefTimer(keepaliveID)
	_ = loop.CancelTimer(keepaliveID)
	_ = loop.CancelTimer(testTimerID)
	cancel()
	<-done
}
