package tournament

import (
	"context"
	"testing"
)

func TestTimerScheduleCapability(t *testing.T) {
	for _, implementation := range Implementations() {
		t.Run(implementation.Name, func(t *testing.T) {
			loop, err := implementation.Factory()
			if err != nil {
				t.Fatalf("create %s loop: %v", implementation.VariantID, err)
			}
			scheduler, supported := loop.(TimerScheduleEventLoop)
			if implementation.VariantID == "scheduler.alternate-two.max-performance" {
				if supported {
					t.Fatal("AlternateTwo claims a timer API absent from its source")
				}
				return
			}
			if !supported {
				t.Fatalf("%s erased its source-backed timer schedule API", implementation.VariantID)
			}

			fired := make(chan struct{}, 1)
			if err := scheduler.ScheduleTimer(0, func() { fired <- struct{}{} }); err != nil {
				t.Fatalf("pre-Run ScheduleTimer: %v", err)
			}
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			cleanup := benchmarkLoopCleanup(t, loop, runDone, implementation.VariantID)
			t.Cleanup(cleanup)
			waitTournamentSignal(t, fired, "pre-Run scheduled timer")
			checkpoint := make(chan struct{})
			if err := loop.Submit(func() { close(checkpoint) }); err != nil {
				t.Fatalf("post-timer checkpoint: %v", err)
			}
			waitTournamentSignal(t, checkpoint, "post-timer checkpoint")
			select {
			case <-fired:
				t.Fatal("scheduled timer fired more than once")
			default:
			}
			cleanup()
		})
	}
}
