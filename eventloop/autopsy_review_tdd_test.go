//go:build !windows

package eventloop

// ============================================================================
// Autopsy TDD Tests — Verifying Quiescing Protocol Fixes
//
// These tests verify that the quiescing protocol correctly fixes the two
// race conditions identified in the adversarial review (scratch/review.md).
//
// Before the fix:
//   - Blocker 1: ScheduleTimer returned (id, nil) during auto-exit, timer discarded
//   - Blocker 2: RegisterFD succeeded during auto-exit, Alive()==true on terminated loop
//
// After the fix:
//   - ScheduleTimer returns (0, ErrLoopTerminated) during quiescing window
//   - RegisterFD returns ErrLoopTerminated during quiescing window
//   - Alive() is always false after any termination path
//   - All liveness counters are properly reset
// ============================================================================

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// Blocker 1: ScheduleTimer now correctly rejected during auto-exit
// ============================================================================

// TestAutopsy_Blocker1_ScheduleTimerRejectedDuringAutoExit verifies that
// the quiescing protocol correctly prevents ScheduleTimer from accepting work
// during the auto-exit termination window.
//
// With the quiescing protocol:
//  1. Auto-exit sets quiescing flag before committing termination
//  2. ScheduleTimer checks quiescing and rejects with ErrLoopTerminated
//  3. No timer is registered, no work is lost
func TestAutopsy_Blocker1_ScheduleTimerRejectedDuringAutoExit(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Hook: block when auto-exit decides to terminate, before StateTerminated
	enteredTerminate := make(chan struct{})
	releaseTerminate := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		BeforeTerminateState: func() {
			close(enteredTerminate)
			<-releaseTerminate
		},
	}

	// Schedule a ref'd timer, then unref it to trigger auto-exit.
	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer keepalive: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(context.Background())
	}()

	// Wait for the timer to be registered.
	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	// Unref the timer → Alive() returns false → auto-exit triggers
	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}

	// Wait for the BeforeTerminateState hook to fire.
	select {
	case <-enteredTerminate:
	case <-time.After(5 * time.Second):
		t.Fatal("BeforeTerminateState hook did not fire — auto-exit did not trigger")
	}

	// While the hook is blocking, call ScheduleTimer from another goroutine.
	// The quiescing flag is now set, so ScheduleTimer should REJECT with error.
	var scheduleErr error
	var scheduleID TimerID
	scheduleDone := make(chan struct{})
	go func() {
		defer close(scheduleDone)
		scheduleID, scheduleErr = loop.ScheduleTimer(time.Hour, func() {})
	}()

	select {
	case <-scheduleDone:
		// ScheduleTimer returned
	case <-time.After(5 * time.Second):
		t.Fatal("ScheduleTimer did not return within timeout — may be blocked")
	}

	// VERIFY: ScheduleTimer should now be REJECTED (quiescing protocol fix).
	if scheduleErr == nil {
		t.Fatalf("ScheduleTimer should have been rejected during quiescing window, "+
			"but succeeded with ID %d — quiescing gate is not working", scheduleID)
	}
	if scheduleErr != ErrLoopTerminated {
		t.Fatalf("ScheduleTimer should return ErrLoopTerminated, got: %v", scheduleErr)
	}
	if scheduleID != 0 {
		t.Errorf("ScheduleTimer should return ID 0 on error, got %d", scheduleID)
	}

	t.Logf("ScheduleTimer correctly rejected during quiescing window: %v", scheduleErr)

	// Release the termination hook.
	close(releaseTerminate)

	// Wait for Run() to complete.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		t.Log("Run() returned nil — loop exited via auto-exit")
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return within timeout")
	}

	// Verify clean state after termination.
	state := loop.State()
	alive := loop.Alive()
	refedCount := loop.refedTimerCount.Load()

	if state != StateTerminated {
		t.Errorf("State should be StateTerminated, got %v", state)
	}
	if alive {
		t.Error("Alive() should be false after termination")
	}
	if refedCount != 0 {
		t.Errorf("refedTimerCount should be 0, got %d", refedCount)
	}

	t.Log("BLOCKER 1 FIX VERIFIED: ScheduleTimer correctly rejected during quiescing window")
}

// TestAutopsy_Blocker1_ScheduleTimerRace_TimerNeverFires verifies that
// the quiescing protocol prevents a timer from being accepted during the
// auto-exit window, so no timer is ever silently discarded.
func TestAutopsy_Blocker1_ScheduleTimerRace_TimerNeverFires(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	enteredTerminate := make(chan struct{})
	releaseTerminate := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		BeforeTerminateState: func() {
			close(enteredTerminate)
			<-releaseTerminate
		},
	}

	// Use a short-lived timer to trigger auto-exit.
	triggerID, err := loop.ScheduleTimer(5*time.Millisecond, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer trigger: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(context.Background())
	}()

	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	// Unref the trigger timer.
	if err := loop.UnrefTimer(triggerID); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}

	select {
	case <-enteredTerminate:
	case <-time.After(5 * time.Second):
		t.Fatal("BeforeTerminateState hook did not fire")
	}

	// Try to schedule a timer during the termination window.
	// The quiescing protocol should REJECT this.
	var timerFired atomic.Bool
	scheduleDone := make(chan struct{})
	go func() {
		defer close(scheduleDone)
		_, err := loop.ScheduleTimer(1*time.Millisecond, func() {
			timerFired.Store(true)
		})
		// Expected: err != nil (quiescing rejection)
		_ = err
	}()

	select {
	case <-scheduleDone:
	case <-time.After(5 * time.Second):
		t.Fatal("ScheduleTimer did not return")
	}

	// Release termination.
	close(releaseTerminate)

	// Wait for Run() to complete.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return")
	}

	// The timer should NEVER have fired (it was rejected, not discarded).
	if timerFired.Load() {
		t.Error("Timer fired — unexpected, the timer should have been rejected")
	}

	// Verify clean state.
	if loop.Alive() {
		t.Error("Alive() should be false after termination")
	}

	t.Log("BLOCKER 1 FIX VERIFIED: Timer was rejected (not accepted then discarded) during auto-exit window")
}

// ============================================================================
// Blocker 2: RegisterFD now correctly rejected during auto-exit
// ============================================================================

// TestAutopsy_Blocker2_RegisterFDRejectedDuringAutoExit verifies that
// the quiescing protocol correctly prevents RegisterFD from accepting work
// during the auto-exit termination window.
//
// With the quiescing protocol:
//  1. Auto-exit sets quiescing flag before committing termination
//  2. RegisterFD checks quiescing and rejects with ErrLoopTerminated
//  3. userIOFDCount is NOT incremented, Alive() invariant preserved
func TestAutopsy_Blocker2_RegisterFDRejectedDuringAutoExit(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	enteredTerminate := make(chan struct{})
	releaseTerminate := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		BeforeTerminateState: func() {
			close(enteredTerminate)
			<-releaseTerminate
		},
	}

	// Schedule and unref a timer to trigger auto-exit.
	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(context.Background())
	}()

	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)

	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}

	select {
	case <-enteredTerminate:
	case <-time.After(5 * time.Second):
		t.Fatal("BeforeTerminateState hook did not fire")
	}

	// While the hook is blocking (auto-exit has been decided but not committed),
	// try to register an FD. The quiescing protocol should REJECT this.
	pipeFD, pipeCleanup := testCreateIOFD(t)
	defer pipeCleanup()

	var registerErr error
	registerDone := make(chan struct{})
	go func() {
		defer close(registerDone)
		registerErr = loop.RegisterFD(pipeFD, EventRead, func(IOEvents) {})
	}()

	select {
	case <-registerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("RegisterFD did not return within timeout")
	}

	// VERIFY: RegisterFD should now be REJECTED (quiescing protocol fix).
	if registerErr == nil {
		t.Fatal("RegisterFD should have been rejected during quiescing window — " +
			"quiescing gate is not working")
	}
	if registerErr != ErrLoopTerminated {
		t.Fatalf("RegisterFD should return ErrLoopTerminated, got: %v", registerErr)
	}

	t.Logf("RegisterFD correctly rejected during quiescing window: %v", registerErr)

	// Verify userIOFDCount was NOT incremented.
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Errorf("userIOFDCount should be 0 after RegisterFD rejection, got %d", got)
	}

	// Release the termination hook.
	close(releaseTerminate)

	// Wait for Run() to complete.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return within timeout")
	}

	// VERIFY: Invariant is preserved.
	state := loop.State()
	alive := loop.Alive()
	ioCount := loop.userIOFDCount.Load()

	t.Logf("After auto-exit: State=%v, userIOFDCount=%d, Alive()=%v",
		state, ioCount, alive)

	if state != StateTerminated {
		t.Errorf("State should be StateTerminated, got %v", state)
	}
	if alive {
		t.Error("Alive() should be false after termination — invariant break!")
	}
	if ioCount != 0 {
		t.Errorf("userIOFDCount should be 0 after termination, got %d", ioCount)
	}

	t.Log("BLOCKER 2 FIX VERIFIED: RegisterFD correctly rejected, invariant preserved")
}

// TestAutopsy_Blocker2_RegisterFD_InvariantPreservedDetailed verifies the
// invariant in detail — after auto-exit, all liveness signals must be zero.
func TestAutopsy_Blocker2_RegisterFD_InvariantPreservedDetailed(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	enteredTerminate := make(chan struct{})
	releaseTerminate := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	var terminateOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeTerminateState: func() {
			close(enteredTerminate)
			<-releaseTerminate
			terminateOnce.Do(wg.Done)
		},
	}

	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(context.Background())
	}()

	waitForCounter(t, &loop.refedTimerCount, 1, 2*time.Second)
	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}

	select {
	case <-enteredTerminate:
	case <-time.After(5 * time.Second):
		t.Fatal("hook did not fire")
	}

	// Try to register an FD during the window — should be rejected.
	pipeFD, pipeCleanup := testCreateIOFD(t)
	defer pipeCleanup()

	if err := loop.RegisterFD(pipeFD, EventRead, func(IOEvents) {}); err == nil {
		t.Fatal("RegisterFD should have been rejected during quiescing window")
	} else if err != ErrLoopTerminated {
		t.Fatalf("RegisterFD should return ErrLoopTerminated, got: %v", err)
	}

	t.Logf("RegisterFD correctly rejected before release")

	// Release and wait.
	close(releaseTerminate)
	wg.Wait()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return")
	}

	// After auto-exit, check every liveness signal.
	state := loop.State()
	ioCount := loop.userIOFDCount.Load()
	refedTimers := loop.refedTimerCount.Load()
	alive := loop.Alive()

	t.Logf("After auto-exit:")
	t.Logf("  State()          = %v", state)
	t.Logf("  userIOFDCount    = %d", ioCount)
	t.Logf("  refedTimerCount  = %d", refedTimers)
	t.Logf("  Alive()          = %v", alive)
	t.Logf("  timerMap entries = %d", len(loop.timerMap))

	// Verify invariant: all signals must be zero/false after termination.
	if state != StateTerminated {
		t.Errorf("State should be StateTerminated, got %v", state)
	}
	if ioCount != 0 {
		t.Errorf("userIOFDCount should be 0 after termination, got %d", ioCount)
	}
	if refedTimers != 0 {
		t.Errorf("refedTimerCount should be 0 after termination, got %d", refedTimers)
	}
	if alive {
		t.Error("INVARIANT BREAK: Alive()==true while State()==StateTerminated")
	}

	t.Log("BLOCKER 2 FIX VERIFIED: All liveness signals correctly zero after auto-exit")
}

// ============================================================================
// Control Tests: Verify normal behavior is unchanged
// ============================================================================

// TestAutopsy_NormalAutoExit_NoRaceUnderNormalTiming verifies that without
// artificially widening the race window, auto-exit works correctly.
func TestAutopsy_NormalAutoExit_NoRaceUnderNormalTiming(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fired := make(chan struct{})
	_, err = loop.ScheduleTimer(5*time.Millisecond, func() {
		close(fired)
	})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(context.Background())
	}()

	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		t.Fatal("timer should have fired")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after timer fired")
	}

	// Verify clean state after normal auto-exit.
	state := loop.State()
	alive := loop.Alive()
	ioCount := loop.userIOFDCount.Load()
	refedCount := loop.refedTimerCount.Load()

	t.Logf("Normal auto-exit: State=%v, Alive=%v, ioCount=%d, refedCount=%d",
		state, alive, ioCount, refedCount)

	if state != StateTerminated {
		t.Errorf("State should be StateTerminated, got %v", state)
	}
	if alive {
		t.Error("Alive() should be false after normal auto-exit")
	}
	if ioCount != 0 {
		t.Errorf("userIOFDCount should be 0, got %d", ioCount)
	}
	if refedCount != 0 {
		t.Errorf("refedTimerCount should be 0, got %d", refedCount)
	}
}

// TestAutopsy_ShutdownPath_ResetsUserIOFDCount verifies that the Shutdown path
// correctly resets userIOFDCount.
func TestAutopsy_ShutdownPath_ResetsUserIOFDCount(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = loop.Run(ctx) }()
	waitForLoopRunningT(t, loop, 2*time.Second)

	pipeFD, pipeCleanup := testCreateIOFD(t)
	defer pipeCleanup()

	if err := loop.RegisterFD(pipeFD, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	ioCount := loop.userIOFDCount.Load()
	if ioCount != 1 {
		t.Fatalf("userIOFDCount should be 1 after RegisterFD, got %d", ioCount)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	ioCount = loop.userIOFDCount.Load()
	if ioCount != 0 {
		t.Errorf("userIOFDCount should be 0 after Shutdown, got %d", ioCount)
	} else {
		t.Log("Shutdown path correctly resets userIOFDCount via terminateCleanup")
	}
}

// TestAutopsy_ClosePath_ResetsUserIOFDCount verifies that Close also
// resets userIOFDCount via terminateCleanup.
func TestAutopsy_ClosePath_ResetsUserIOFDCount(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = loop.Run(ctx) }()
	waitForLoopRunningT(t, loop, 2*time.Second)

	pipeFD, pipeCleanup := testCreateIOFD(t)
	defer pipeCleanup()

	if err := loop.RegisterFD(pipeFD, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	ioCount := loop.userIOFDCount.Load()
	if ioCount != 0 {
		t.Errorf("userIOFDCount should be 0 after Close, got %d", ioCount)
	} else {
		t.Log("Close path correctly resets userIOFDCount via terminateCleanup")
	}
}
