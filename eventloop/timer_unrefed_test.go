package eventloop

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduleTimerUnrefedPublishesWithoutLiveness(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	var ran atomic.Bool
	if _, err := loop.ScheduleTimerUnrefed(time.Hour, func() { ran.Store(true) }); err != nil {
		t.Fatalf("ScheduleTimerUnrefed: %v", err)
	}
	if loop.Alive() {
		t.Fatal("Alive reported an initially unrefed timer before Run")
	}
	if loop.HasMacrotaskWork() {
		t.Fatal("HasMacrotaskWork reported an initially unrefed timer before Run")
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ran.Load() {
		t.Fatal("initially unrefed timer ran without referenced liveness")
	}
}

func TestJSSetTimeoutUnrefedPublishesWithoutLiveness(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	var ran atomic.Bool
	if _, err := js.SetTimeoutUnrefed(func() { ran.Store(true) }, int(time.Hour/time.Millisecond)); err != nil {
		t.Fatalf("SetTimeoutUnrefed: %v", err)
	}
	if loop.Alive() {
		t.Fatal("Alive reported an initially unrefed JS timeout before Run")
	}
	if loop.HasMacrotaskWork() {
		t.Fatal("HasMacrotaskWork reported an initially unrefed JS timeout before Run")
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ran.Load() {
		t.Fatal("initially unrefed JS timeout ran without referenced liveness")
	}
}

func TestInitiallyUnrefedTimersRunWithReferencedWork(t *testing.T) {
	for _, test := range []struct {
		name     string
		schedule func(*Loop, func()) error
	}{
		{
			name: "Loop",
			schedule: func(loop *Loop, callback func()) error {
				_, err := loop.ScheduleTimerUnrefed(0, callback)
				return err
			},
		},
		{
			name: "JS",
			schedule: func(loop *Loop, callback func()) error {
				js, err := NewJS(loop)
				if err != nil {
					return err
				}
				_, err = js.SetTimeoutUnrefed(callback, 0)
				return err
			},
		},
		{
			name: "Loop control",
			schedule: func(loop *Loop, callback func()) error {
				_, err := loop.ScheduleControlTimerUnrefed(0, callback)
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New(WithAutoExit(true))
			if err != nil {
				t.Fatal(err)
			}
			var unrefedRan atomic.Bool
			if err := test.schedule(loop, func() { unrefedRan.Store(true) }); err != nil {
				t.Fatalf("schedule unrefed timer: %v", err)
			}
			if _, err := loop.ScheduleTimer(time.Millisecond, func() {}); err != nil {
				t.Fatalf("ScheduleTimer keepalive: %v", err)
			}
			if err := loop.Run(context.Background()); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if !unrefedRan.Load() {
				t.Fatal("initially unrefed timer did not run while referenced work was live")
			}
		})
	}
}

func TestControlTimerExcludesUserCallbackMetrics(t *testing.T) {
	loop, err := New(WithAutoExit(true), WithMetrics(true))
	if err != nil {
		t.Fatal(err)
	}
	var controlRan atomic.Bool
	var userRan atomic.Bool
	if _, err := loop.ScheduleControlTimer(0, func() {
		controlRan.Store(true)
		panic("control panic")
	}); err != nil {
		t.Fatalf("ScheduleControlTimer: %v", err)
	}
	if _, err := loop.ScheduleTimer(0, func() { userRan.Store(true) }); err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !controlRan.Load() || !userRan.Load() {
		t.Fatalf("callbacks ran = control:%t user:%t, want true/true", controlRan.Load(), userRan.Load())
	}
	if got := loop.metrics.latency.count.Load(); got != 1 {
		t.Fatalf("user callback latency samples = %d, want 1", got)
	}
	var throughput int64
	for index := range loop.metrics.tps.buckets {
		throughput += loop.metrics.tps.buckets[index].Load()
	}
	if throughput != 1 {
		t.Fatalf("user callback throughput samples = %d, want 1", throughput)
	}
}

func TestPendingTimerRefTransitionsFoldIntoLiveness(t *testing.T) {
	t.Run("Loop", func(t *testing.T) {
		loop, err := New(WithAutoExit(true))
		if err != nil {
			t.Fatal(err)
		}
		id, err := loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer: %v", err)
		}
		if err := loop.UnrefTimer(id); err != nil {
			t.Fatalf("UnrefTimer: %v", err)
		}
		if loop.Alive() || loop.HasMacrotaskWork() {
			t.Fatal("pending TimerAdd then UnrefTimer retained liveness")
		}
		if err := loop.RefTimer(id); err != nil {
			t.Fatalf("RefTimer: %v", err)
		}
		if !loop.Alive() || !loop.HasMacrotaskWork() {
			t.Fatal("pending TimerAdd then UnrefTimer then RefTimer lost liveness")
		}
		if err := loop.UnrefTimer(id); err != nil {
			t.Fatalf("second UnrefTimer: %v", err)
		}
		if loop.Alive() || loop.HasMacrotaskWork() {
			t.Fatal("final pending UnrefTimer did not remove liveness")
		}
		if err := loop.Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	t.Run("JS", func(t *testing.T) {
		loop, err := New(WithAutoExit(true))
		if err != nil {
			t.Fatal(err)
		}
		js, err := NewJS(loop)
		if err != nil {
			t.Fatal(err)
		}
		id, err := js.SetTimeout(func() {}, int(time.Hour/time.Millisecond))
		if err != nil {
			t.Fatalf("SetTimeout: %v", err)
		}
		if err := js.UnrefTimeout(id); err != nil {
			t.Fatalf("UnrefTimeout: %v", err)
		}
		if loop.Alive() || loop.HasMacrotaskWork() {
			t.Fatal("pending JS SetTimeout then UnrefTimeout retained liveness")
		}
		if err := loop.Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
}
