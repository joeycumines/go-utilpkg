package eventloop

import (
	"context"
	"testing"
	"time"
)

func TestAutoExitUnrefTimerClearsTimerStorage(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}

	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitRefedTimerCount(t, loop, 1)

	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}
	if err := waitContractValue(t, runDone, "auto-exit Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount after auto-exit = %d, want 0", got)
	}
	if got := len(loop.timerMap); got != 0 {
		t.Fatalf("timerMap after auto-exit contains %d entries, want 0", got)
	}
	if got := len(loop.timers); got != 0 {
		t.Fatalf("timer heap after auto-exit contains %d entries, want 0", got)
	}
}
