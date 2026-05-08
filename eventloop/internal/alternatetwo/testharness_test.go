package alternatetwo

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournamenttest"
)

const alternateTwoTestTimeout = 5 * time.Second

type alternateTwoTestLoop struct {
	loop        *Loop
	runDone     chan error
	cleanupOnce sync.Once
}

func startAlternateTwoTestLoop(t testing.TB, loop *Loop) *alternateTwoTestLoop {
	t.Helper()
	harness := &alternateTwoTestLoop{loop: loop, runDone: make(chan error, 1)}
	go func() { harness.runDone <- loop.Run(context.Background()) }()
	t.Cleanup(func() { harness.cleanup(t) })
	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		t.Fatalf("Submit(start barrier) failed: %v", err)
	}
	waitAlternateTwoSignal(t, ready, "loop start")
	return harness
}

func (h *alternateTwoTestLoop) shutdown(t testing.TB) {
	t.Helper()
	h.cleanupOnce.Do(func() {
		result := tournamenttest.Terminate(h.loop, h.runDone, alternateTwoTestTimeout)
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

func (h *alternateTwoTestLoop) cleanup(t testing.TB) {
	t.Helper()
	h.cleanupOnce.Do(func() {
		result := tournamenttest.Terminate(h.loop, h.runDone, alternateTwoTestTimeout)
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

func waitAlternateTwoSignal(t testing.TB, signal <-chan struct{}, description string) {
	t.Helper()
	timer := time.NewTimer(alternateTwoTestTimeout)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", description)
	}
}
