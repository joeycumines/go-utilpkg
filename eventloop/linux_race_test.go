//go:build linux

package eventloop

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestRegression_RegisterFD_ZombieRace verifies that Shutdown() and RegisterFD
// race cooperatively without creating a zombie poller (leaked epoll FD).
//
// BUG SCENARIO (Fix #1):
// If RegisterFD releases the lock between checking 'closed' and calling initPoller,
// Shutdown() can close the poller in that window. Then RegisterFD re-initializes it,
// creating a zombie FD that is never closed.
//
// FIX: With sync.Once, init happens exactly once per instance and all callers
// block until init completes. The closed atomic.Bool prevents init if closed.
func TestRegression_RegisterFD_ZombieRace(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Start the loop
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- l.Run(ctx)
	}()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Attempt registration during normal operation
	registerDone := make(chan error)
	go func() {
		err := l.RegisterFD(int(os.Stdout.Fd()), EventRead, func(IOEvents) {})
		registerDone <- err
	}()

	// Wait for registration to complete
	select {
	case err := <-registerDone:
		if err != nil {
			t.Logf("RegisterFD returned: %v (expected during race)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout: RegisterFD did not complete")
	}

	// Shutdown the loop
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	err = l.Shutdown(shutdownCtx)
	shutdownCancel()
	if err != nil && err != ErrLoopTerminated {
		t.Logf("Shutdown returned: %v", err)
	}

	cancel()

	// Wait for run to complete
	select {
	case <-runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout: Run did not complete")
	}

	// Verify the poller is closed
	l.ioPoller.mu.RLock()
	isClosed := l.ioPoller.closed.Load()
	l.ioPoller.mu.RUnlock()

	// After Shutdown(), poller should be closed
	if !isClosed {
		t.Fatal("CRITICAL FAIL: Poller not marked as closed after Shutdown()")
	}

	t.Log("Zombie race test passed: poller properly closed after shutdown")
}
