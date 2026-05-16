package eventloop

import (
	"errors"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestMixedIngressOwnerPreservesPhasePriority(t *testing.T) {
	tests := []struct {
		name     string
		external func(*Loop, func()) error
		owner    func(*Loop, func()) error
		want     []string
	}{
		{
			name:     "next tick before older microtask",
			external: func(loop *Loop, fn func()) error { return loop.ScheduleMicrotask(fn) },
			owner:    func(loop *Loop, fn func()) error { return loop.ScheduleNextTick(fn) },
			want:     []string{"owner", "external"},
		},
		{
			name:     "microtask before older external task",
			external: func(loop *Loop, fn func()) error { return loop.Submit(fn) },
			owner:    func(loop *Loop, fn func()) error { return loop.ScheduleMicrotask(fn) },
			want:     []string{"owner", "external"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New(WithAutoExit(true))
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)

			var callbackErrs fuzzErrs
			var order []string
			if err := loop.Submit(func() {
				externalResult := make(chan error, 1)
				go func() {
					externalResult <- test.external(loop, func() { order = append(order, "external") })
				}()
				select {
				case err := <-externalResult:
					if err != nil {
						callbackErrs.add("external schedule: %v", err)
						return
					}
				case <-time.After(time.Second):
					callbackErrs.add("external schedule did not acknowledge ingress")
					return
				}
				if err := test.owner(loop, func() { order = append(order, "owner") }); err != nil {
					callbackErrs.add("owner schedule: %v", err)
				}
			}); err != nil {
				t.Fatalf("Submit owner callback: %v", err)
			}

			if err := runAutoExitLoop(t, loop); err != nil {
				t.Fatalf("Run: %v", err)
			}
			callbackErrs.failNow(t)
			if !reflect.DeepEqual(order, test.want) {
				t.Fatalf("callback order = %v, want %v", order, test.want)
			}
		})
	}
}

func admitForeignCallback(schedule func(func()) error, fn func()) error {
	admitted := make(chan error, 1)
	go func() { admitted <- schedule(fn) }()
	select {
	case err := <-admitted:
		return err
	case <-time.After(time.Second):
		return errors.New("foreign callback admission did not return")
	}
}

func TestForeignNextTickAcknowledgedDuringNextTickBatchPrecedesMicrotasks(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	var callbackErrs fuzzErrs
	var order []string
	if err := loop.ScheduleNextTick(func() {
		order = append(order, "nextTick-1")
		if err := admitForeignCallback(loop.ScheduleNextTick, func() {
			order = append(order, "nextTick-2")
			if err := admitForeignCallback(loop.ScheduleNextTick, func() { order = append(order, "nextTick-3") }); err != nil {
				callbackErrs.add("second foreign ScheduleNextTick: %v", err)
			}
		}); err != nil {
			callbackErrs.add("first foreign ScheduleNextTick: %v", err)
		}
	}); err != nil {
		t.Fatalf("ScheduleNextTick: %v", err)
	}
	if err := loop.ScheduleMicrotask(func() { order = append(order, "microtask") }); err != nil {
		t.Fatalf("ScheduleMicrotask: %v", err)
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	callbackErrs.failNow(t)
	if want := []string{"nextTick-1", "nextTick-2", "nextTick-3", "microtask"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("callback order = %v, want %v", order, want)
	}
}

func TestForeignNextTickAcknowledgedDuringNextTickBatchPrecedesPromiseReaction(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	var callbackErrs fuzzErrs
	var order []string
	reaction := js.Resolve("promise").Then(func(value any) any {
		order = append(order, value.(string))
		return "done"
	}, nil)
	if err := loop.ScheduleMicrotaskCheckpoint(func() { order = append(order, "checkpoint") }); err != nil {
		t.Fatalf("ScheduleMicrotaskCheckpoint: %v", err)
	}
	if err := loop.ScheduleNextTick(func() {
		order = append(order, "nextTick-1")
		if err := admitForeignCallback(loop.ScheduleNextTick, func() { order = append(order, "nextTick-2") }); err != nil {
			callbackErrs.add("foreign ScheduleNextTick: %v", err)
		}
	}); err != nil {
		t.Fatalf("ScheduleNextTick: %v", err)
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	callbackErrs.failNow(t)
	if want := []string{"nextTick-1", "nextTick-2", "promise", "checkpoint"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("callback order = %v, want %v", order, want)
	}
	if reaction.State() != Fulfilled || reaction.Value() != "done" {
		t.Fatalf("Promise reaction = (%v, %v), want (Fulfilled, done)", reaction.State(), reaction.Value())
	}
}

func TestForeignNextTickAcknowledgedBeforeNextTickGoexitPrecedesMicrotasks(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	var callbackErrs fuzzErrs
	var order []string
	if err := loop.ScheduleNextTick(func() {
		order = append(order, "nextTick-1")
		if err := admitForeignCallback(loop.ScheduleNextTick, func() { order = append(order, "nextTick-2") }); err != nil {
			callbackErrs.add("foreign ScheduleNextTick: %v", err)
		}
		runtime.Goexit()
	}); err != nil {
		t.Fatalf("ScheduleNextTick: %v", err)
	}
	if err := loop.ScheduleMicrotask(func() { order = append(order, "microtask") }); err != nil {
		t.Fatalf("ScheduleMicrotask: %v", err)
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	callbackErrs.failNow(t)
	if want := []string{"nextTick-1", "nextTick-2", "microtask"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("callback order = %v, want %v", order, want)
	}
}

func TestForeignNextTickAcknowledgedDuringPromiseBatchDoesNotPreemptMicrotasks(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	var callbackErrs fuzzErrs
	var order []string
	if err := loop.ScheduleMicrotask(func() {
		order = append(order, "microtask-1")
		if err := admitForeignCallback(loop.ScheduleMicrotask, func() { order = append(order, "foreign-microtask") }); err != nil {
			callbackErrs.add("foreign ScheduleMicrotask: %v", err)
		}
		if err := admitForeignCallback(loop.ScheduleNextTick, func() { order = append(order, "nextTick") }); err != nil {
			callbackErrs.add("foreign ScheduleNextTick: %v", err)
		}
	}); err != nil {
		t.Fatalf("first ScheduleMicrotask: %v", err)
	}
	if err := loop.ScheduleMicrotask(func() { order = append(order, "microtask-2") }); err != nil {
		t.Fatalf("second ScheduleMicrotask: %v", err)
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	callbackErrs.failNow(t)
	if want := []string{"microtask-1", "microtask-2", "nextTick", "foreign-microtask"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("callback order = %v, want %v", order, want)
	}
}

func TestOwnerScheduleAfterForeignNextTickPreservesActivePromiseBatch(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	var callbackErrs fuzzErrs
	var order []string
	if err := loop.ScheduleMicrotask(func() {
		order = append(order, "microtask-1")
		if err := admitForeignCallback(loop.ScheduleNextTick, func() { order = append(order, "nextTick") }); err != nil {
			callbackErrs.add("foreign ScheduleNextTick: %v", err)
		}
		if err := loop.ScheduleMicrotask(func() { order = append(order, "owner-microtask") }); err != nil {
			callbackErrs.add("owner ScheduleMicrotask: %v", err)
		}
	}); err != nil {
		t.Fatalf("first ScheduleMicrotask: %v", err)
	}
	if err := loop.ScheduleMicrotask(func() { order = append(order, "microtask-2") }); err != nil {
		t.Fatalf("second ScheduleMicrotask: %v", err)
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	callbackErrs.failNow(t)
	if want := []string{"microtask-1", "microtask-2", "owner-microtask", "nextTick"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("callback order = %v, want %v", order, want)
	}
}
