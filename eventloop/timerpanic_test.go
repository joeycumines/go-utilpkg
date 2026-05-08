package eventloop

import (
	"context"
	"testing"
)

func TestRunTimersRecoversPanickingCallbacks(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	const timerCount = 10
	executed := make(chan int, timerCount)
	for index := range timerCount {
		if _, err := loop.ScheduleTimer(0, func() {
			executed <- index
			if index%2 == 0 {
				panic(index)
			}
		}); err != nil {
			t.Fatalf("ScheduleTimer %d: %v", index, err)
		}
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	seen := make([]bool, timerCount)
	for range timerCount {
		index := waitContractValue(t, executed, "timer callback after panic recovery")
		if index < 0 || index >= timerCount {
			t.Fatalf("timer callback index = %d, want [0,%d)", index, timerCount)
		}
		if seen[index] {
			t.Fatalf("timer callback %d executed more than once", index)
		}
		seen[index] = true
	}
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := waitContractValue(t, runDone, "timer recovery Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
