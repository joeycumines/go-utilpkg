package eventloop

import (
	"context"
	"reflect"
	"testing"
)

func TestScheduleNextTickTerminatedState(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := loop.ScheduleNextTick(func() { t.Error("terminated nextTick callback ran") }); err != ErrLoopTerminated {
		t.Fatalf("ScheduleNextTick after Shutdown: got %v, want %v", err, ErrLoopTerminated)
	}
}

func TestSubmitInternalTerminatedState(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := loop.SubmitInternal(func() { t.Error("terminated internal callback ran") }); err != ErrLoopTerminated {
		t.Fatalf("SubmitInternal after Shutdown: got %v, want %v", err, ErrLoopTerminated)
	}
}

func TestScheduleNextTickIOMode(t *testing.T) {
	testTaskOnlyNativePollCallback(t, "ScheduleNextTick", func(loop *Loop, callback func()) error {
		return loop.ScheduleNextTick(callback)
	})
}

func TestSubmitInternalIOMode(t *testing.T) {
	testTaskOnlyNativePollCallback(t, "SubmitInternal", func(loop *Loop, callback func()) error {
		return loop.SubmitInternal(callback)
	})
}

func testTaskOnlyNativePollCallback(
	t *testing.T,
	name string,
	schedule func(*Loop, func()) error,
) {
	t.Helper()
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	callbackRan := make(chan struct{})
	if err := schedule(loop, func() { close(callbackRan) }); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	waitContractSignal(t, callbackRan, name+" callback")
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestProcessExternalProcessesNextTickPriority(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	completed := make(chan struct{})
	if err := loop.Submit(func() {
		order = append(order, "task")
		if err := loop.ScheduleMicrotask(func() {
			order = append(order, "microtask")
			close(completed)
		}); err != nil {
			t.Errorf("ScheduleMicrotask: %v", err)
		}
		if err := loop.ScheduleNextTick(func() {
			order = append(order, "nextTick")
		}); err != nil {
			t.Errorf("ScheduleNextTick: %v", err)
		}
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, completed, "nextTick and microtask checkpoint")
	want := []string{"task", "nextTick", "microtask"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("callback order: got %v, want %v", order, want)
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
