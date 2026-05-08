package eventloop

import (
	"context"
	"testing"
	"time"
)

func TestCancelTimersEmptyInput(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)
	if got := loop.CancelTimers(); got != nil {
		t.Fatalf("CancelTimers() = %#v, want nil", got)
	}
}

func TestCancelTimersExactSequentialResults(t *testing.T) {
	assertResults := func(t *testing.T, results []error) {
		t.Helper()
		want := []error{nil, ErrTimerNotFound, ErrTimerNotFound, nil}
		if len(results) != len(want) {
			t.Fatalf("CancelTimers returned %d results, want %d", len(results), len(want))
		}
		for index := range want {
			if results[index] != want[index] {
				t.Errorf("CancelTimers result %d = %v, want %v", index, results[index], want[index])
			}
		}
	}

	t.Run("before Run", func(t *testing.T) {
		loop := New(WithAutoExit(true))
		registerLoopCleanupT(t, loop)

		fired := make(chan TimerID, 2)
		var first TimerID
		first, err := loop.ScheduleTimer(0, func() { fired <- first })
		if err != nil {
			t.Fatal(err)
		}
		var second TimerID
		second, err = loop.ScheduleTimer(0, func() { fired <- second })
		if err != nil {
			t.Fatal(err)
		}
		missing := TimerID(^uint64(0))
		assertResults(t, loop.CancelTimers(first, missing, first, second))

		if err := runAutoExitLoop(t, loop); err != nil {
			t.Fatalf("Run: %v", err)
		}
		select {
		case id := <-fired:
			t.Fatalf("pre-Run canceled timer %d fired", id)
		default:
		}
	})

	t.Run("running owner", func(t *testing.T) {
		loop := New(WithAutoExit(true))
		registerLoopCleanupT(t, loop)

		fired := make(chan TimerID, 2)
		var first TimerID
		first, err := loop.ScheduleTimer(time.Hour, func() { fired <- first })
		if err != nil {
			t.Fatal(err)
		}
		var second TimerID
		second, err = loop.ScheduleTimer(time.Hour, func() { fired <- second })
		if err != nil {
			t.Fatal(err)
		}
		ownerTick := make(chan struct{}, 1)
		loop.testHooks = &loopTestHooks{
			BeforeRunTimers: func() {
				select {
				case ownerTick <- struct{}{}:
				default:
				}
			},
		}
		runDone := make(chan error, 1)
		go func() { runDone <- loop.Run(context.Background()) }()
		waitContractSignal(t, ownerTick, "running timer-owner tick")

		missing := TimerID(^uint64(0))
		assertResults(t, loop.CancelTimers(first, missing, first, second))
		if err := waitContractValue(t, runDone, "batch-cancellation auto-exit completion"); err != nil {
			t.Fatalf("Run: %v", err)
		}
		select {
		case id := <-fired:
			t.Fatalf("running-owner canceled timer %d fired", id)
		default:
		}
		if loop.Alive() {
			t.Fatal("canceled timers retained loop liveness")
		}
	})
}

func TestCancelTimersExecutingTimerDuplicateSequentialResult(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)

	results := make(chan []error, 1)
	var timerID TimerID
	timerID, err := loop.ScheduleTimer(0, func() {
		results <- loop.CancelTimers(timerID, timerID)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := waitContractValue(t, results, "executing timer batch cancellation")
	if len(got) != 2 {
		t.Fatalf("CancelTimers returned %d results, want 2", len(got))
	}
	if got[0] != nil || got[1] != ErrTimerNotFound {
		t.Fatalf("CancelTimers executing duplicate = %v, want [nil ErrTimerNotFound]", got)
	}
}
