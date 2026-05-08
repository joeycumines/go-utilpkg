package eventloop

import (
	"context"
	"errors"
	"testing"
	"time"
)

func runAutoExitLoop(t *testing.T, loop *Loop) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), fuzzLoopRunTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()

	select {
	case err := <-done:
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			t.Fatalf("event loop failed to auto-exit before context timeout: %v", err)
		}
		return err
	case <-time.After(fuzzLoopRunTimeout + time.Second):
		t.Fatal("event loop did not terminate before the watchdog")
		return nil
	}
}

func TestSecondRunDoesNotCloseOwnerCompletion(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	release := releaseSignalT(t, releaseCallback)
	if err := loop.Submit(func() {
		close(callbackEntered)
		<-releaseCallback
	}); err != nil {
		t.Fatalf("Submit owner barrier: %v", err)
	}
	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitContractSignal(t, callbackEntered, "winning Run owner callback")

	if err := loop.Run(ctx); !errors.Is(err, ErrLoopAlreadyRunning) {
		t.Fatalf("second Run = %v, want ErrLoopAlreadyRunning", err)
	}
	select {
	case <-loop.loopDone:
		t.Fatal("second Run closed the winning owner's completion signal")
	default:
	}

	release()
	if err := loop.CancelTimer(timerID); err != nil {
		t.Fatalf("CancelTimer after second Run: %v", err)
	}
	cancel()
	if err := waitContractValue(t, runDone, "winning Run completion"); !errors.Is(err, context.Canceled) {
		t.Fatalf("winning Run = %v, want context.Canceled", err)
	}
}

func TestRunOnTerminatedLoop(t *testing.T) {
	loop := New()
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := loop.Run(context.Background()); !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("Run after Close = %v, want ErrLoopTerminated", err)
	}
}
