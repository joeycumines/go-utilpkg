package eventloop

import (
	"testing"
)

func TestIngressPostAdmissionWakeFastPathWaiters(t *testing.T) {
	cases := []struct {
		name  string
		admit func(*Loop) error
	}{
		{name: "Submit", admit: func(l *Loop) error { return l.Submit(func() {}) }},
		{name: "SubmitInternal", admit: func(l *Loop) error { return l.SubmitInternal(func() {}) }},
		{name: "ScheduleMicrotask", admit: func(l *Loop) error { return l.ScheduleMicrotask(func() {}) }},
		{name: "ScheduleNextTick", admit: func(l *Loop) error { return l.ScheduleNextTick(func() {}) }},
		{name: "scheduleMicrotaskCheckpoint", admit: func(l *Loop) error { return l.scheduleMicrotaskCheckpoint(func() {}) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerFDResourceCleanupT(t, loop)
			loop.state.Store(StateRunning)
			loop.userIOFDCount.Store(0)

			if err := tc.admit(loop); err != nil {
				t.Fatalf("admit: %v", err)
			}

			select {
			case <-loop.fastWakeupCh:
			default:
				t.Fatal("admitted work did not wake fast-path waiter")
			}
		})
	}
}

func TestCanUseFastPathFallsBackWhenForcedInvariantBroken(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	if err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)

	loop.userIOFDCount.Store(1)
	if loop.canUseFastPath() {
		t.Fatal("canUseFastPath returned true for FastPathForced with registered user FD")
	}
}
