package alternatethree

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournamenttest"
)

type observedContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func newObservedContext(ctx context.Context) *observedContext {
	return &observedContext{Context: ctx, observed: make(chan struct{})}
}

func (c *observedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

const alternateThreeTestTimeout = 5 * time.Second

type alternateThreeTestLoop struct {
	loop        *Loop
	runDone     chan error
	cleanupOnce sync.Once
}

func startAlternateThreeTestLoop(t testing.TB, loop *Loop) *alternateThreeTestLoop {
	t.Helper()
	harness := &alternateThreeTestLoop{loop: loop, runDone: make(chan error, 1)}
	go func() { harness.runDone <- loop.Run(context.Background()) }()
	t.Cleanup(func() { harness.cleanup(t) })
	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		t.Fatalf("Submit(start barrier) failed: %v", err)
	}
	waitAlternateThreeSignal(t, ready, "loop start")
	return harness
}

func (h *alternateThreeTestLoop) shutdown(t testing.TB) {
	t.Helper()
	h.cleanupOnce.Do(func() {
		result := tournamenttest.Terminate(h.loop, h.runDone, alternateThreeTestTimeout)
		if result.ShutdownErr != nil {
			t.Fatalf("Shutdown() failed: %v", result.ShutdownErr)
		}
		if result.CloseErr != nil && !errors.Is(result.CloseErr, ErrLoopTerminated) {
			t.Fatalf("fallback Close() failed: %v", result.CloseErr)
		}
		if result.RunErr != nil {
			t.Fatalf("Run() failed: %v", result.RunErr)
		}
	})
}

func (h *alternateThreeTestLoop) cleanup(t testing.TB) {
	t.Helper()
	h.cleanupOnce.Do(func() {
		result := tournamenttest.Terminate(h.loop, h.runDone, alternateThreeTestTimeout)
		if result.ShutdownErr != nil && !errors.Is(result.ShutdownErr, ErrLoopTerminated) {
			t.Errorf("cleanup Shutdown failed: %v", result.ShutdownErr)
		}
		if result.CloseErr != nil && !errors.Is(result.CloseErr, ErrLoopTerminated) {
			t.Errorf("cleanup Close failed: %v", result.CloseErr)
		}
		if result.RunErr != nil {
			t.Errorf("Run() failed: %v", result.RunErr)
		}
	})
}

func (h *alternateThreeTestLoop) wait(t testing.TB) {
	t.Helper()
	timer := time.NewTimer(alternateThreeTestTimeout)
	defer timer.Stop()
	select {
	case err := <-h.runDone:
		h.runDone <- err
		if err != nil {
			t.Errorf("Run() failed: %v", err)
		}
	case <-timer.C:
		t.Fatalf("Run() did not return within %v", alternateThreeTestTimeout)
	}
}

func waitAlternateThreeSignal(t testing.TB, signal <-chan struct{}, description string) {
	t.Helper()
	timer := time.NewTimer(alternateThreeTestTimeout)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", description)
	}
}
