package eventloop

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestFastModeTimerRefDoesNotSubmitPhysicalWake(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var physicalWakes atomic.Int64
	loop.testHooks = &loopTestHooks{OnSubmitWakeup: func() { physicalWakes.Add(1) }}

	runDone := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { runDone <- loop.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if runErr := waitContractValue(t, runDone, "Run cleanup"); !errors.Is(runErr, context.Canceled) {
			t.Errorf("Run: %v", runErr)
		}
	})
	waitLoopOwnerTurnT(t, loop)

	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatal(err)
	}
	waitRefedTimerCount(t, loop, 1)
	physicalWakes.Store(0)

	for range 50 {
		if err := loop.RefTimer(timerID); err != nil {
			t.Fatal(err)
		}
	}
	if got := physicalWakes.Load(); got != 0 {
		t.Fatalf("physical wakes for fast-mode RefTimer calls = %d, want 0", got)
	}
}
