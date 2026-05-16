package eventloop

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestYieldMicrotasksRequiresCallbackOwner(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.YieldMicrotasks(); err != ErrCallbackOwner {
		t.Fatalf("YieldMicrotasks error = %v, want %v", err, ErrCallbackOwner)
	}
}

func TestYieldMicrotasksNilReceiverPanics(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("YieldMicrotasks nil receiver did not panic")
		}
	}()
	var loop *Loop
	_ = loop.YieldMicrotasks()
}

func TestYieldMicrotasksRejectsTerminalLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := loop.YieldMicrotasks(); err != ErrLoopTerminated {
		t.Fatalf("YieldMicrotasks error = %v, want %v", err, ErrLoopTerminated)
	}
}

func TestYieldMicrotasksResumesAtTaskBoundary(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	if err := loop.ScheduleNextTick(func() {
		events = append(events, "tick1")
		if err := loop.ScheduleNextTick(func() { events = append(events, "tick2") }); err != nil {
			t.Errorf("ScheduleNextTick: %v", err)
		}
		if err := loop.ScheduleMicrotask(func() { events = append(events, "micro") }); err != nil {
			t.Errorf("ScheduleMicrotask: %v", err)
		}
		if err := loop.YieldMicrotasks(); err != nil {
			t.Errorf("YieldMicrotasks: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := loop.ScheduleImmediate(func() { events = append(events, "immediate") }); err != nil {
		t.Fatal(err)
	}

	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"tick1", "immediate", "tick2", "micro"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestYieldMicrotasksRepeatedRequestIsIdempotent(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.ScheduleNextTick(func() {
		before := loop.submissionEpoch.Load()
		if err := loop.YieldMicrotasks(); err != nil {
			t.Errorf("first YieldMicrotasks: %v", err)
		}
		afterFirst := loop.submissionEpoch.Load()
		if err := loop.YieldMicrotasks(); err != nil {
			t.Errorf("second YieldMicrotasks: %v", err)
		}
		afterSecond := loop.submissionEpoch.Load()
		if afterFirst != before+1 {
			t.Errorf("first yield epoch = %d, want %d", afterFirst, before+1)
		}
		if afterSecond != afterFirst {
			t.Errorf("second yield epoch = %d, want idempotent %d", afterSecond, afterFirst)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestYieldMicrotasksResumesAtEmptyCheckBoundary(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	if err := loop.ScheduleNextTick(func() {
		events = append(events, "tick1")
		if err := loop.ScheduleNextTick(func() { events = append(events, "tick2") }); err != nil {
			t.Errorf("ScheduleNextTick: %v", err)
		}
		if err := loop.YieldMicrotasks(); err != nil {
			t.Errorf("YieldMicrotasks: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"tick1", "tick2"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestYieldMicrotasksResumesBeforeFutureTimer(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	timerID, err := loop.ScheduleTimer(time.Minute, func() { events = append(events, "timer") })
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.ScheduleNextTick(func() {
		events = append(events, "tick1")
		if err := loop.ScheduleNextTick(func() {
			events = append(events, "tick2")
			if err := loop.CancelTimer(timerID); err != nil {
				t.Errorf("CancelTimer: %v", err)
			}
		}); err != nil {
			t.Errorf("ScheduleNextTick: %v", err)
		}
		if err := loop.YieldMicrotasks(); err != nil {
			t.Errorf("YieldMicrotasks: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		_ = loop.Close()
		t.Fatal("yielded microtasks waited for a future timer")
	}
	want := []string{"tick1", "tick2"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestYieldMicrotasksDoesNotRetainUnrefedTimer(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	timerID, err := loop.ScheduleTimer(time.Minute, func() { events = append(events, "timer") })
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatal(err)
	}
	if err := loop.ScheduleNextTick(func() {
		events = append(events, "tick1")
		if err := loop.ScheduleNextTick(func() { events = append(events, "tick2") }); err != nil {
			t.Errorf("ScheduleNextTick: %v", err)
		}
		if err := loop.YieldMicrotasks(); err != nil {
			t.Errorf("YieldMicrotasks: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		_ = loop.Close()
		t.Fatal("yield retained an unrefed future timer")
	}
	want := []string{"tick1", "tick2"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestRunMicrotaskCheckpointRequiresCallbackOwner(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.RunMicrotaskCheckpoint(); err != ErrCallbackOwner {
		t.Fatalf("RunMicrotaskCheckpoint error = %v, want %v", err, ErrCallbackOwner)
	}
}

func TestRunMicrotaskCheckpointNilReceiverPanics(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("RunMicrotaskCheckpoint nil receiver did not panic")
		}
	}()
	var loop *Loop
	_ = loop.RunMicrotaskCheckpoint()
}

func TestRunMicrotaskCheckpointDrainsWithoutSyntheticMetric(t *testing.T) {
	loop, err := New(WithAutoExit(true), WithMetrics(true))
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	var checkpointErr error
	if _, err := loop.ScheduleControlTimer(0, func() {
		if err := loop.ScheduleNextTick(func() { events = append(events, "nextTick") }); err != nil {
			t.Errorf("ScheduleNextTick: %v", err)
		}
		if err := loop.ScheduleMicrotask(func() { events = append(events, "microtask") }); err != nil {
			t.Errorf("ScheduleMicrotask: %v", err)
		}
		checkpointErr = loop.RunMicrotaskCheckpoint()
		events = append(events, "control-return")
	}); err != nil {
		t.Fatalf("ScheduleControlTimer: %v", err)
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if checkpointErr != nil {
		t.Fatalf("RunMicrotaskCheckpoint: %v", checkpointErr)
	}
	want := []string{"nextTick", "microtask", "control-return"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
	if got := loop.metrics.latency.count.Load(); got != 2 {
		t.Fatalf("callback latency samples = %d, want 2", got)
	}
}

func TestRunMicrotaskCheckpointPreservesYield(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	var checkpointErr error
	if _, err := loop.ScheduleControlTimer(0, func() {
		if err := loop.ScheduleNextTick(func() { events = append(events, "nextTick") }); err != nil {
			t.Errorf("ScheduleNextTick: %v", err)
		}
		if err := loop.YieldMicrotasks(); err != nil {
			t.Errorf("YieldMicrotasks: %v", err)
		}
		checkpointErr = loop.RunMicrotaskCheckpoint()
		events = append(events, "control-return")
	}); err != nil {
		t.Fatalf("ScheduleControlTimer: %v", err)
	}
	if _, err := loop.ScheduleTimer(0, func() { events = append(events, "task") }); err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if checkpointErr != nil {
		t.Fatalf("RunMicrotaskCheckpoint: %v", checkpointErr)
	}
	want := []string{"control-return", "task", "nextTick"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestAdvanceMicrotaskCheckpointRequiresCallbackOwner(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.AdvanceMicrotaskCheckpoint(); err != ErrCallbackOwner {
		t.Fatalf("AdvanceMicrotaskCheckpoint error = %v, want %v", err, ErrCallbackOwner)
	}
}

func TestAdvanceMicrotaskCheckpointNilReceiverPanics(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("AdvanceMicrotaskCheckpoint nil receiver did not panic")
		}
	}()
	var loop *Loop
	_ = loop.AdvanceMicrotaskCheckpoint()
}

func TestAdvanceMicrotaskCheckpointConsumesOneYieldWithoutSyntheticMetric(t *testing.T) {
	loop, err := New(WithAutoExit(true), WithMetrics(true))
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	var firstErr, secondErr error
	if _, err := loop.ScheduleControlTimer(0, func() {
		if err := loop.ScheduleNextTick(func() { events = append(events, "nextTick") }); err != nil {
			t.Errorf("ScheduleNextTick: %v", err)
		}
		if err := loop.YieldMicrotasks(); err != nil {
			t.Errorf("YieldMicrotasks: %v", err)
		}
		firstErr = loop.AdvanceMicrotaskCheckpoint()
		events = append(events, "first-return")
		if loop.microtaskYield.Load() {
			t.Error("first Advance left the consumed yield active")
		}
		secondErr = loop.AdvanceMicrotaskCheckpoint()
		events = append(events, "second-return")
	}); err != nil {
		t.Fatalf("ScheduleControlTimer: %v", err)
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if firstErr != nil || secondErr != nil {
		t.Fatalf("AdvanceMicrotaskCheckpoint errors = (%v, %v)", firstErr, secondErr)
	}
	want := []string{"first-return", "nextTick", "second-return"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
	if got := loop.metrics.latency.count.Load(); got != 1 {
		t.Fatalf("callback latency samples = %d, want 1", got)
	}
}

func TestAdvanceMicrotaskCheckpointClearsYieldRaisedWhileDraining(t *testing.T) {
	loop, err := New(WithAutoExit(true), WithMetrics(true))
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	var checkpointErrs [3]error
	if _, err := loop.ScheduleControlTimer(0, func() {
		if err := loop.ScheduleMicrotask(func() {
			events = append(events, "first")
			if err := loop.ScheduleMicrotask(func() { events = append(events, "later") }); err != nil {
				t.Errorf("ScheduleMicrotask later: %v", err)
			}
			if err := loop.YieldMicrotasks(); err != nil {
				t.Errorf("YieldMicrotasks: %v", err)
			}
		}); err != nil {
			t.Errorf("ScheduleMicrotask first: %v", err)
		}
		checkpointErrs[0] = loop.AdvanceMicrotaskCheckpoint()
		events = append(events, "first-return")
		if loop.microtaskYield.Load() {
			t.Error("first Advance left its newly raised yield active")
		}
		checkpointErrs[1] = loop.AdvanceMicrotaskCheckpoint()
		events = append(events, "second-return")
		checkpointErrs[2] = loop.AdvanceMicrotaskCheckpoint()
		events = append(events, "third-return")
	}); err != nil {
		t.Fatalf("ScheduleControlTimer: %v", err)
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for index, err := range checkpointErrs {
		if err != nil {
			t.Fatalf("AdvanceMicrotaskCheckpoint %d: %v", index, err)
		}
	}
	want := []string{"first", "first-return", "later", "second-return", "third-return"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
	if got := loop.metrics.latency.count.Load(); got != 2 {
		t.Fatalf("callback latency samples = %d, want 2", got)
	}
}

func TestResumeMicrotaskCheckpointRequiresCallbackOwner(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.ResumeMicrotaskCheckpoint(); err != ErrCallbackOwner {
		t.Fatalf("ResumeMicrotaskCheckpoint error = %v, want %v", err, ErrCallbackOwner)
	}
}

func TestResumeMicrotaskCheckpointNilReceiverPanics(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("ResumeMicrotaskCheckpoint nil receiver did not panic")
		}
	}()
	var loop *Loop
	_ = loop.ResumeMicrotaskCheckpoint()
}

func TestResumeMicrotaskCheckpointForcesYieldWithoutSyntheticMetric(t *testing.T) {
	loop, err := New(WithAutoExit(true), WithMetrics(true))
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	var checkpointErr error
	if _, err := loop.ScheduleControlTimer(0, func() {
		if err := loop.ScheduleNextTick(func() { events = append(events, "nextTick") }); err != nil {
			t.Errorf("ScheduleNextTick: %v", err)
		}
		if err := loop.ScheduleMicrotask(func() { events = append(events, "microtask") }); err != nil {
			t.Errorf("ScheduleMicrotask: %v", err)
		}
		if err := loop.YieldMicrotasks(); err != nil {
			t.Errorf("YieldMicrotasks: %v", err)
		}
		checkpointErr = loop.ResumeMicrotaskCheckpoint()
		events = append(events, "control-return")
	}); err != nil {
		t.Fatalf("ScheduleControlTimer: %v", err)
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if checkpointErr != nil {
		t.Fatalf("ResumeMicrotaskCheckpoint: %v", checkpointErr)
	}
	want := []string{"nextTick", "microtask", "control-return"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
	if loop.microtaskYield.Load() {
		t.Fatal("ResumeMicrotaskCheckpoint left the yield suspension active")
	}
	if got := loop.metrics.latency.count.Load(); got != 2 {
		t.Fatalf("callback latency samples = %d, want 2", got)
	}
}
