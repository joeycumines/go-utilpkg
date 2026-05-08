package promisealtone_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-eventloop/internal/tournamenttest"
)

const promiseAltOneTestTimeout = 5 * time.Second

func newPromiseAltOneAutoLoop(t testing.TB) (*eventloop.Loop, *eventloop.JS) {
	t.Helper()
	loop := eventloop.New(eventloop.WithAutoExit(true))
	return loop, eventloop.NewJS(loop)
}

func newPromiseAltOneUnstartedLoop(t testing.TB) (*eventloop.Loop, *eventloop.JS) {
	t.Helper()
	loop := eventloop.New()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), promiseAltOneTestTimeout)
		defer cancel()
		if err := loop.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown() failed: %v", err)
		}
	})
	return loop, eventloop.NewJS(loop)
}

func runPromiseAltOneAutoLoop(t testing.TB, loop *eventloop.Loop) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), promiseAltOneTestTimeout)
	defer cancel()
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run() failed: %v", err)
	}
}

type promiseAltOneRunningLoop struct {
	loop        *eventloop.Loop
	runDone     chan error
	deadline    <-chan time.Time
	cleanupOnce sync.Once
}

func startPromiseAltOneRunningLoop(t testing.TB) (*promiseAltOneRunningLoop, *eventloop.JS) {
	t.Helper()
	loop := eventloop.New()
	harness := &promiseAltOneRunningLoop{
		loop:     loop,
		runDone:  make(chan error, 1),
		deadline: time.After(30 * time.Minute),
	}
	go func() { harness.runDone <- loop.Run(context.Background()) }()
	t.Cleanup(func() { harness.cleanup(t) })
	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		t.Fatalf("Submit(start barrier) failed: %v", err)
	}
	waitPromiseAltOneSignal(t, ready, "loop start")
	return harness, eventloop.NewJS(loop)
}

func (h *promiseAltOneRunningLoop) wait(t testing.TB, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-h.deadline:
		t.Fatalf("timed out waiting for %s", description)
	}
}

func (h *promiseAltOneRunningLoop) cleanup(t testing.TB) {
	t.Helper()
	h.cleanupOnce.Do(func() {
		result := tournamenttest.Terminate(h.loop, h.runDone, promiseAltOneTestTimeout)
		if result.ShutdownErr != nil && !errors.Is(result.ShutdownErr, eventloop.ErrLoopTerminated) {
			t.Errorf("cleanup Shutdown failed: %v", result.ShutdownErr)
		}
		if result.CloseErr != nil && !errors.Is(result.CloseErr, eventloop.ErrLoopTerminated) {
			t.Errorf("cleanup Close failed: %v", result.CloseErr)
		}
		if result.RunErr != nil {
			t.Errorf("Run() failed: %v", result.RunErr)
		}
	})
}

func waitPromiseAltOneSignal(t testing.TB, signal <-chan struct{}, description string) {
	t.Helper()
	timer := time.NewTimer(promiseAltOneTestTimeout)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", description)
	}
}
