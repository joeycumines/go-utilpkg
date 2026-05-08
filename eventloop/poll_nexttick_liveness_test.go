package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPollSkipsNativeWaitForPendingNextTick(t *testing.T) {
	loop := New(WithFastPathMode(FastPathDisabled))
	registerLoopCleanupT(t, loop)

	nextTickAdmission := make(chan error, 1)
	nextTickDone := make(chan struct{})
	var nextTickComplete atomic.Bool
	var polledBeforeNextTick atomic.Bool
	var scheduleOnce sync.Once
	loop.testHooks = &loopTestHooks{
		PrePollSleep: func() {
			scheduleOnce.Do(func() {
				nextTickAdmission <- loop.ScheduleNextTick(func() {
					nextTickComplete.Store(true)
					close(nextTickDone)
				})
			})
		},
		PollIO: func(int) (int, error) {
			if !nextTickComplete.Load() {
				polledBeforeNextTick.Store(true)
			}
			return 0, nil
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	if err := waitContractValue(t, nextTickAdmission, "nextTick admission from pre-poll boundary"); err != nil {
		t.Fatalf("ScheduleNextTick: %v", err)
	}
	waitContractSignal(t, nextTickDone, "nextTick callback")
	if polledBeforeNextTick.Load() {
		t.Fatal("poll reached native wait while nextTick work was pending")
	}

	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "nextTick liveness Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
