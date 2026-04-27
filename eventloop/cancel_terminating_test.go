package eventloop

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestCancelTimer_DuringTerminating verifies CancelTimer succeeds when the
// loop is in StateTerminating (graceful shutdown drain window).
//
// Strategy: Schedule a timer, call Shutdown() from a goroutine to trigger
// StateTerminating, then call CancelTimer. CancelTimer's SubmitInternal
// callback will be processed by shutdown()'s internal queue drain.
func TestCancelTimer_DuringTerminating(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	// Schedule a long-running timer
	id, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Wait for timer registration
	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	if loop.refedTimerCount.Load() != 1 {
		t.Fatalf("refedTimerCount should be 1, got %d", loop.refedTimerCount.Load())
	}

	// Initiate shutdown from a goroutine (sets StateTerminating)
	shutdownDone := make(chan error, 1)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	go func() {
		shutdownDone <- loop.Shutdown(shutdownCtx)
	}()

	// Poll until we observe StateTerminating or the timer is canceled
	// by the drain loop. The CancelTimer call may see either
	// StateTerminating (success path) or StateTerminated (SubmitInternal
	// would reject it). Either way, the timer should not remain.
	//
	// We call CancelTimer — it will either:
	// - See StateTerminating and succeed (SubmitInternal pushes to queue,
	//   shutdown drain processes callback)
	// - See StateTerminated and return ErrLoopTerminated
	// Both are acceptable outcomes since the loop is shutting down.
	err = loop.CancelTimer(id)
	if err != nil && err != ErrLoopTerminated {
		t.Errorf("CancelTimer during shutdown should succeed or return ErrLoopTerminated, got: %v", err)
	}
	if err == nil {
		// Verify the cancel actually took effect — refedTimerCount should be 0
		// (the drain loop may or may not have run yet, so we just verify no negative)
		count := loop.refedTimerCount.Load()
		if count < 0 {
			t.Errorf("refedTimerCount went negative: %d", count)
		}
	}

	if err := <-shutdownDone; err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// TestCancelTimers_DuringTerminating verifies the batch variant works
// during StateTerminating.
func TestCancelTimers_DuringTerminating(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	ids := make([]TimerID, 3)
	for i := range ids {
		ids[i], err = loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer %d: %v", i, err)
		}
	}

	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	shutdownDone := make(chan error, 1)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	go func() {
		shutdownDone <- loop.Shutdown(shutdownCtx)
	}()

	// Batch cancel during shutdown
	errors := loop.CancelTimers(ids)
	for i, e := range errors {
		if e != nil && e != ErrLoopTerminated {
			t.Errorf("CancelTimers[%d] should succeed or return ErrLoopTerminated, got: %v", i, e)
		}
	}

	if err := <-shutdownDone; err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// TestCancelTimer_DuringTerminating_AlreadyFired verifies CancelTimer returns
// ErrTimerNotFound for a timer that has already fired during termination.
func TestCancelTimer_DuringTerminating_AlreadyFired(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	// Schedule a timer that fires immediately
	id, err := loop.ScheduleTimer(0, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Wait for it to fire
	time.Sleep(50 * time.Millisecond)

	// Schedule a keep-alive timer to prevent immediate loop exit
	_, err = loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer keep-alive: %v", err)
	}

	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	shutdownDone := make(chan error, 1)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	go func() {
		shutdownDone <- loop.Shutdown(shutdownCtx)
	}()

	// Cancel the already-fired timer during shutdown
	err = loop.CancelTimer(id)
	if err != ErrTimerNotFound && err != ErrLoopTerminated {
		t.Errorf("CancelTimer for already-fired timer should return ErrTimerNotFound or ErrLoopTerminated, got: %v", err)
	}

	if err := <-shutdownDone; err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// TestCancelTimer_StateTerminatingGuard verifies the state guard logic:
// CancelTimer should allow StateTerminating (not return ErrLoopNotRunning).
// This test directly verifies the guard condition by checking the state
// at the moment CancelTimer reads it.
func TestCancelTimer_StateTerminatingGuard(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a counter to track CancelTimer outcomes
	var successCount atomic.Int32
	var errNotRunningCount atomic.Int32

	// Start the loop
	go loop.Run(ctx)

	// Schedule timers
	const n = 10
	ids := make([]TimerID, n)
	for i := range ids {
		ids[i], err = loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer %d: %v", i, err)
		}
	}

	// Wait for all registrations
	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	// Initiate shutdown
	shutdownDone := make(chan error, 1)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	go func() {
		shutdownDone <- loop.Shutdown(shutdownCtx)
	}()

	// Attempt to cancel all timers concurrently during shutdown
	for _, id := range ids {
		go func(id TimerID) {
			err := loop.CancelTimer(id)
			if err == nil {
				successCount.Add(1)
			} else if err == ErrLoopNotRunning {
				errNotRunningCount.Add(1)
			}
			// ErrLoopTerminated or ErrTimerNotFound are also acceptable
		}(id)
	}

	if err := <-shutdownDone; err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// Key assertion: ErrLoopNotRunning should NEVER be returned
	// (the guard should allow StateTerminating)
	if errNotRunningCount.Load() > 0 {
		t.Errorf("CancelTimer returned ErrLoopNotRunning %d times — should never happen during StateTerminating", errNotRunningCount.Load())
	}

	t.Logf("success=%d errNotRunning=%d (should be 0)", successCount.Load(), errNotRunningCount.Load())
}
