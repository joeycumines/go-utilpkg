//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestIngressSleepBoundarySkipsNativeWait(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	sleepBoundary := make(chan struct{})
	releasePoll := make(chan struct{})
	release := contractRelease(t, releasePoll)
	var boundaryOnce sync.Once
	var taskRan atomic.Bool
	var polledBeforeTask atomic.Bool
	loop.testHooks = &loopTestHooks{
		PrePollSleep: func() {
			boundaryOnce.Do(func() {
				close(sleepBoundary)
				<-releasePoll
			})
		},
		PollIO: func(int) (int, error) {
			if !taskRan.Load() {
				polledBeforeTask.Store(true)
			}
			return 0, nil
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, sleepBoundary, "pre-poll sleep boundary")

	executed := make(chan struct{})
	if err := loop.Submit(func() {
		taskRan.Store(true)
		close(executed)
	}); err != nil {
		t.Fatalf("Submit at sleep boundary: %v", err)
	}
	release()
	waitContractSignal(t, executed, "sleep-boundary ingress callback")
	if polledBeforeTask.Load() {
		t.Fatal("native poll ran before admitted ingress callback")
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "sleep-boundary Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestIngressPostAdmissionWakePollWaiters(t *testing.T) {
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
			if err := loop.ensurePoller(); err != nil {
				t.Fatalf("ensurePoller: %v", err)
			}
			loop.state.Store(StateSleeping)
			loop.userIOFDCount.Store(1)
			var physicalWakeups atomic.Int32
			loop.testHooks = &loopTestHooks{
				OnSubmitWakeup: func() { physicalWakeups.Add(1) },
			}

			if err := tc.admit(loop); err != nil {
				t.Fatalf("admit: %v", err)
			}

			if physicalWakeups.Load() == 0 {
				t.Fatal("admitted work did not wake poll waiter")
			}
		})
	}
}

func TestIngressDuringLazyPollerInitializationSkipsNativeWait(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	initReached := make(chan struct{})
	releaseInit := make(chan struct{})
	releaseInitFn := releaseSignalT(t, releaseInit)
	pollEntered := make(chan struct{})
	releasePoll := make(chan struct{})
	releasePollFn := releaseSignalT(t, releasePoll)
	taskRan := make(chan struct{})
	releaseTask := make(chan struct{})
	releaseTaskFn := releaseSignalT(t, releaseTask)
	var initOnce sync.Once
	var pollOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeWakeFDRegister: func(_, _ int) {
			initOnce.Do(func() {
				close(initReached)
				<-releaseInit
			})
		},
		PollIO: func(int) (int, error) {
			pollOnce.Do(func() { close(pollEntered) })
			<-releasePoll
			return 0, nil
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, initReached, "lazy poller wake-descriptor registration")
	if loop.State() != StateSleeping || loop.pollerReady.Load() {
		t.Fatalf("lazy initialization boundary state=%s ready=%v, want Sleeping and not ready", loop.State(), loop.pollerReady.Load())
	}
	if err := loop.Submit(func() {
		close(taskRan)
		<-releaseTask
	}); err != nil {
		t.Fatalf("Submit during lazy poller initialization: %v", err)
	}
	releaseInitFn()

	select {
	case <-taskRan:
	case <-pollEntered:
		t.Fatal("native PollIO overtook ingress acknowledged during lazy poller initialization")
	}
	select {
	case <-pollEntered:
		t.Fatal("native PollIO began before the admitted callback completed its wake turn")
	default:
	}
	releaseTaskFn()
	releasePollFn()
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion after lazy-init wake"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
