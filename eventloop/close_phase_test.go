package eventloop

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCloseCallbacksRunAfterCheckPhase(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	var (
		mu    sync.Mutex
		order []string
	)
	appendOrder := func(value string) {
		mu.Lock()
		order = append(order, value)
		mu.Unlock()
	}

	if _, err := js.SetImmediate(func() {
		appendOrder("check")
		if err := loop.ScheduleCloseCallback(func() { appendOrder("close") }); err != nil {
			appendOrder("close-error:" + err.Error())
		}
		if _, err := js.SetImmediate(func() { appendOrder("next-check") }); err != nil {
			appendOrder("next-check-error:" + err.Error())
		}
		if _, err := js.SetTimeout(func() { appendOrder("timer") }, 0); err != nil {
			appendOrder("timer-error:" + err.Error())
		}
	}); err != nil {
		t.Fatalf("SetImmediate: %v", err)
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	got := strings.Join(order, ",")
	mu.Unlock()
	if got != "check,close,next-check,timer" {
		t.Fatalf("phase order = %q, want check,close,next-check,timer", got)
	}
}

func TestCloseCallbacksQueuedDuringCloseDoNotSleepBehindIOPoll(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	fd, cleanup := testCreateIOFD(t)
	t.Cleanup(cleanup)
	registerLoopCleanupT(t, loop)
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	var (
		mu    sync.Mutex
		order []string
	)
	done := make(chan struct{})
	var secondEntered atomic.Bool
	var pollBeforeSecond atomic.Int32
	loop.testHooks = &loopTestHooks{
		PollIO: func(int) (int, error) {
			if !secondEntered.Load() {
				pollBeforeSecond.Add(1)
			}
			return 0, nil
		},
	}
	appendOrder := func(value string) {
		mu.Lock()
		order = append(order, value)
		mu.Unlock()
	}

	if err := loop.ScheduleCloseCallback(func() {
		appendOrder("first")
		if err := loop.ScheduleCloseCallback(func() {
			secondEntered.Store(true)
			appendOrder("second")
			close(done)
		}); err != nil {
			appendOrder("second-error:" + err.Error())
			close(done)
		}
	}); err != nil {
		t.Fatalf("ScheduleCloseCallback: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(t.Context()) }()
	waitContractSignal(t, done, "second close callback")

	if err := loop.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion after Shutdown"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := pollBeforeSecond.Load(); got != 0 {
		t.Fatalf("native PollIO calls before second close callback = %d, want 0", got)
	}

	mu.Lock()
	got := strings.Join(order, ",")
	mu.Unlock()
	if got != "first,second" {
		t.Fatalf("close callback order = %q, want first,second", got)
	}
}
