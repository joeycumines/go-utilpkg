package eventloop

import (
	"testing"
	"time"
)

func TestCancelTimerBeforeRun(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Schedule a timer - this should work (it uses SubmitInternal which allows StateAwake)
	id, err := l.ScheduleTimer(100*time.Millisecond, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer failed: %v", err)
	}

	// Now try to cancel BEFORE l.Run()
	// Before the fix, this would DEADLOCK
	err = l.CancelTimer(id)
	if err != ErrLoopNotRunning {
		t.Errorf("Expected ErrLoopNotRunning, got %v", err)
	}
}

func TestCancelTimerAfterRun(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	id, err := l.ScheduleTimer(100*time.Millisecond, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer failed: %v", err)
	}

	ctx := t.Context()

	// Start the loop
	go l.Run(ctx)

	// Give it a tiny bit of time to transition to StateRunning
	time.Sleep(10 * time.Millisecond)

	// Now cancel should work normally
	err = l.CancelTimer(id)
	if err != nil {
		t.Errorf("CancelTimer failed after Run(): %v", err)
	}
}
