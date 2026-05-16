package eventloop

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestCancelTimersRejectDuringStateTerminating(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var callbacks atomic.Int32
	ids := make([]TimerID, 2)
	for index := range ids {
		ids[index], err = loop.ScheduleTimer(time.Hour, func() { callbacks.Add(1) })
		if err != nil {
			t.Fatalf("ScheduleTimer[%d]: %v", index, err)
		}
	}

	terminating := make(chan struct{})
	releaseShutdown := make(chan struct{})
	releaseShutdownNow := contractRelease(t, releaseShutdown)
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			close(terminating)
			<-releaseShutdown
		},
	}
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
	waitContractSignal(t, terminating, "StateTerminating publication")
	if got := loop.State(); got != StateTerminating {
		t.Fatalf("State during terminal hook = %v, want StateTerminating", got)
	}

	singleDone := make(chan error, 1)
	go func() { singleDone <- loop.CancelTimer(ids[0]) }()
	if got := waitContractValue(t, singleDone, "single cancellation rejection"); got != ErrLoopTerminated {
		t.Fatalf("CancelTimer during StateTerminating = %v, want ErrLoopTerminated", got)
	}

	batchDone := make(chan []error, 1)
	go func() { batchDone <- loop.CancelTimers(ids[0], TimerID(999), ids[0], ids[1]) }()
	batch := waitContractValue(t, batchDone, "batch cancellation rejection")
	if len(batch) != 4 {
		t.Fatalf("CancelTimers result count = %d, want 4", len(batch))
	}
	for index, got := range batch {
		if got != ErrLoopTerminated {
			t.Fatalf("CancelTimers[%d] during StateTerminating = %v, want ErrLoopTerminated", index, got)
		}
	}
	if got := loop.CancelTimers(); got != nil {
		t.Fatalf("CancelTimers() = %#v, want nil", got)
	}

	releaseShutdownNow()
	if err := waitContractValue(t, shutdownDone, "Shutdown completion"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if got := loop.State(); got != StateTerminated {
		t.Fatalf("final State = %v, want StateTerminated", got)
	}
	if got := callbacks.Load(); got != 0 {
		t.Fatalf("timer callbacks = %d, want 0", got)
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refed timer count = %d, want 0", got)
	}
	if got := len(loop.timerMap); got != 0 {
		t.Fatalf("timer map length = %d, want 0", got)
	}
}
