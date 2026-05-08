//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFastPathDisabledIngressSubmitsPhysicalWake(t *testing.T) {
	loop := New(WithFastPathMode(FastPathDisabled))
	pollEntered := make(chan struct{})
	physicalWake := make(chan struct{}, 1)
	var pollOnce sync.Once
	var wakeWrites atomic.Int32
	loop.testHooks = &loopTestHooks{
		PollIO: func(int) (int, error) {
			pollOnce.Do(func() { close(pollEntered) })
			<-physicalWake
			return 0, nil
		},
		WriteWakeFD: func(_ int, payload []byte) (int, error) {
			wakeWrites.Add(1)
			select {
			case physicalWake <- struct{}{}:
			default:
			}
			return len(payload), nil
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	registerActiveLoopCleanupT(t, loop, runDone)
	waitContractSignal(t, pollEntered, "native poll entry")

	taskRan := make(chan struct{})
	if err := loop.Submit(func() { close(taskRan) }); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitContractSignal(t, taskRan, "FastPathDisabled submitted task")
	if got := wakeWrites.Load(); got != 1 {
		t.Fatalf("physical wake writes: got %d, want 1", got)
	}
}

func TestLastFDUnregisterReleasesPendingTimerCommands(t *testing.T) {
	tests := []struct {
		name      string
		operation func(*Loop, TimerID) error
	}{
		{name: "ref timer", operation: func(loop *Loop, id TimerID) error { return loop.RefTimer(id) }},
		{name: "cancel timer", operation: func(loop *Loop, id TimerID) error { return loop.CancelTimer(id) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := New()
			fd, cleanupFD := testCreateIOFD(t)
			t.Cleanup(cleanupFD)
			if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
				t.Fatal(err)
			}
			timerID, err := loop.ScheduleTimer(time.Hour, func() {})
			if err != nil {
				t.Fatal(err)
			}

			loop.drainWakeUpPipe()
			select {
			case <-loop.fastWakeupCh:
			default:
			}

			pollEntered := make(chan struct{})
			physicalWake := make(chan struct{}, 1)
			var pollOnce sync.Once
			var wakeWrites atomic.Int32
			loop.testHooks = &loopTestHooks{
				PollIO: func(int) (int, error) {
					pollOnce.Do(func() { close(pollEntered) })
					<-physicalWake
					return 0, nil
				},
				WriteWakeFD: func(_ int, payload []byte) (int, error) {
					wakeWrites.Add(1)
					select {
					case physicalWake <- struct{}{}:
					default:
					}
					return len(payload), nil
				},
			}

			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			registerActiveLoopCleanupT(t, loop, runDone)
			waitContractSignal(t, pollEntered, "native poll entry")

			if err := loop.UnregisterFD(fd); err != nil {
				t.Fatalf("UnregisterFD: %v", err)
			}
			if got := loop.userIOFDCount.Load(); got != 0 {
				t.Fatalf("userIOFDCount = %d, want 0", got)
			}
			operationDone := make(chan error, 1)
			go func() { operationDone <- test.operation(loop, timerID) }()
			if err := waitContractValue(t, operationDone, test.name+" completion"); err != nil {
				t.Fatalf("%s: %v", test.name, err)
			}
			if got := wakeWrites.Load(); got == 0 {
				t.Fatal("last FD unregistration did not submit a physical wake")
			}
		})
	}
}
