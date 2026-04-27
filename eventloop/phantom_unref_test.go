package eventloop

import (
	"context"
	"testing"
	"time"
)

// TestPhantomUnref_ScheduleThenUnrefOnLoopThread_IOMode exercises the exact
// Phantom Unref scenario: I/O mode forces non-fast-path. ScheduleTimer registers
// synchronously on the loop thread, so UnrefTimer finds the timer in timerMap.
// No pending tracking needed — FIFO ordering handles external goroutines.
func TestPhantomUnref_ScheduleThenUnrefOnLoopThread_IOMode(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Register an I/O FD to force non-fast-path mode
	fd, fdCleanup := testCreateIOFD(t)
	defer fdCleanup()

	if err := loop.RegisterFD(fd, EventRead, func(events IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	// Wait for loop to be running
	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	// From a loop-thread callback: ScheduleTimer then immediately UnrefTimer
	var timerID TimerID
	resultCh := make(chan struct{})
	if err := loop.SubmitInternal(func() {
		var sErr error
		timerID, sErr = loop.ScheduleTimer(time.Hour, func() {})
		if sErr != nil {
			t.Errorf("ScheduleTimer: %v", sErr)
			close(resultCh)
			return
		}
		// Immediately unref — the timer registration closure is still queued
		if uErr := loop.UnrefTimer(timerID); uErr != nil {
			t.Errorf("UnrefTimer: %v", uErr)
		}
		close(resultCh)
	}); err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}
	<-resultCh

	// Submit a barrier to ensure all pending operations are processed
	barrier2 := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier2) }); err != nil {
		t.Fatalf("barrier2: %v", err)
	}
	<-barrier2

	// Verify: refedTimerCount should be 0 (timer was unref'd)
	if count := loop.refedTimerCount.Load(); count != 0 {
		t.Errorf("refedTimerCount should be 0 after phantom unref, got %d", count)
	}
}

// TestPhantomUnref_ScheduleThenUnrefThenRefOnLoopThread_IOMode tests the
// inverse: Schedule + Unref + Ref should leave the timer ref'd.
func TestPhantomUnref_ScheduleThenUnrefThenRefOnLoopThread_IOMode(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fd, fdCleanup := testCreateIOFD(t)
	defer fdCleanup()

	if err := loop.RegisterFD(fd, EventRead, func(events IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	var timerID TimerID
	resultCh := make(chan struct{})
	if err := loop.SubmitInternal(func() {
		var sErr error
		timerID, sErr = loop.ScheduleTimer(time.Hour, func() {})
		if sErr != nil {
			t.Errorf("ScheduleTimer: %v", sErr)
			close(resultCh)
			return
		}
		// Unref then immediately Ref
		if uErr := loop.UnrefTimer(timerID); uErr != nil {
			t.Errorf("UnrefTimer: %v", uErr)
		}
		if rErr := loop.RefTimer(timerID); rErr != nil {
			t.Errorf("RefTimer: %v", rErr)
		}
		close(resultCh)
	}); err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}
	<-resultCh

	barrier2 := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier2) }); err != nil {
		t.Fatalf("barrier2: %v", err)
	}
	<-barrier2

	// Net effect: ScheduleTimer adds 1, Unref subtracts 1, Ref adds 1 = 1
	if count := loop.refedTimerCount.Load(); count != 1 {
		t.Errorf("refedTimerCount should be 1 after unref+ref, got %d", count)
	}
}

// TestPhantomUnref_MultiplePendingRefChanges schedules multiple timers
// and unrefs them all from the loop thread.
func TestPhantomUnref_MultiplePendingRefChanges(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fd, fdCleanup := testCreateIOFD(t)
	defer fdCleanup()

	if err := loop.RegisterFD(fd, EventRead, func(events IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	const n = 3
	resultCh := make(chan struct{})
	if err := loop.SubmitInternal(func() {
		for i := 0; i < n; i++ {
			id, sErr := loop.ScheduleTimer(time.Hour, func() {})
			if sErr != nil {
				t.Errorf("ScheduleTimer %d: %v", i, sErr)
				close(resultCh)
				return
			}
			if uErr := loop.UnrefTimer(id); uErr != nil {
				t.Errorf("UnrefTimer %d: %v", i, uErr)
			}
		}
		close(resultCh)
	}); err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}
	<-resultCh

	barrier2 := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier2) }); err != nil {
		t.Fatalf("barrier2: %v", err)
	}
	<-barrier2

	if count := loop.refedTimerCount.Load(); count != 0 {
		t.Errorf("refedTimerCount should be 0 after %d phantom unrefs, got %d", n, count)
	}

}

// TestPhantomUnref_ExternalGoroutineThenLoopThread verifies that when a timer
// is scheduled from an external goroutine (goes through SubmitInternal queue),
// and then UnrefTimer is called from a loop callback (which runs after the
// registration), the unref is NOT phantom (timer was already registered).
func TestPhantomUnref_ExternalGoroutineThenLoopThread(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fd, fdCleanup := testCreateIOFD(t)
	defer fdCleanup()

	if err := loop.RegisterFD(fd, EventRead, func(events IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	// Schedule from external goroutine
	id, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	// Now from a loop callback, unref the timer — it should be registered already
	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() {
		if uErr := loop.UnrefTimer(id); uErr != nil {
			t.Errorf("UnrefTimer: %v", uErr)
		}
		close(barrier)
	}); err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}
	<-barrier

	// The unref is NOT phantom because the timer was already registered
	// when the external goroutine's ScheduleTimer returned (it blocks until
	// the registration closure runs via the channel round-trip).
	if count := loop.refedTimerCount.Load(); count != 0 {
		t.Errorf("refedTimerCount should be 0 after normal unref, got %d", count)
	}
}

// TestPhantomUnref_FastPathDirectExecution verifies that in fast-path mode
// (no I/O FDs), ScheduleTimer + UnrefTimer from a callback works because
// SubmitInternal executes the registration closure immediately.
func TestPhantomUnref_FastPathDirectExecution(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// No I/O FD registered — fast-path mode

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	var timerID TimerID
	resultCh := make(chan struct{})
	if err := loop.SubmitInternal(func() {
		var sErr error
		timerID, sErr = loop.ScheduleTimer(time.Hour, func() {})
		if sErr != nil {
			t.Errorf("ScheduleTimer: %v", sErr)
			close(resultCh)
			return
		}
		// In fast-path mode, SubmitInternal may execute directly or queue
		// depending on the state. Either way, the unref must not be lost.
		if uErr := loop.UnrefTimer(timerID); uErr != nil {
			t.Errorf("UnrefTimer: %v", uErr)
		}
		close(resultCh)
	}); err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}
	<-resultCh

	barrier2 := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier2) }); err != nil {
		t.Fatalf("barrier2: %v", err)
	}
	<-barrier2

	if count := loop.refedTimerCount.Load(); count != 0 {
		t.Errorf("refedTimerCount should be 0 after fast-path unref, got %d", count)
	}
}

// TestPhantomUnref_CancelDuringPendingRegistration verifies that CancelTimer
// can be used on a timer that was scheduled and then unref'd from a loop-thread
// callback. CancelTimer is called from an external goroutine to exercise the
// SubmitInternal round-trip path.
func TestPhantomUnref_CancelDuringPendingRegistration(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fd, fdCleanup := testCreateIOFD(t)
	defer fdCleanup()

	if err := loop.RegisterFD(fd, EventRead, func(events IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	// From loop thread: schedule + unref (creates pending ref change)
	var timerID TimerID
	scheduleDone := make(chan struct{})
	if err := loop.SubmitInternal(func() {
		var sErr error
		timerID, sErr = loop.ScheduleTimer(time.Hour, func() {})
		if sErr != nil {
			t.Errorf("ScheduleTimer: %v", sErr)
		}
		// Unref the not-yet-registered timer
		_ = loop.UnrefTimer(timerID)
		close(scheduleDone)
	}); err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}
	<-scheduleDone

	// From external goroutine: cancel the timer
	cancelErr := loop.CancelTimer(timerID)
	// May return nil (timer found after registration) or ErrTimerNotFound (timer not in map yet)
	if cancelErr != nil && cancelErr != ErrTimerNotFound {
		t.Errorf("CancelTimer should succeed or return ErrTimerNotFound, got: %v", cancelErr)
	}

	barrier2 := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier2) }); err != nil {
		t.Fatalf("barrier2: %v", err)
	}
	<-barrier2

	if count := loop.refedTimerCount.Load(); count != 0 {
		t.Errorf("refedTimerCount should be 0 after cancel, got %d", count)
	}
}
