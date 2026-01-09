//go:build linux

package eventloop

import (
	"context"
	"os"
	"testing"
	"time"
)

// testHookRegisterFDPreInit is a global hook for deterministic race testing.
// It allows us to pause execution at a critical point in RegisterFD.
var testHookRegisterFDPreInit func()

// TestRegression_RegisterFD_ZombieRace verifies that Stop() and RegisterFD
// race cooperatively without creating a zombie poller (leaked epoll FD).
//
// BUG SCENARIO (Fix #1):
// If RegisterFD releases the lock between checking 'closed' and calling initPoller,
// Stop() can close the poller in that window. Then RegisterFD re-initializes it,
// creating a zombie FD that is never closed.
//
// FIX: RegisterFD now holds the lock through the entire initPollerImpl call.
func TestRegression_RegisterFD_ZombieRace(t *testing.T) {
	resetHook := func() {
		testHookRegisterFDPreInit = nil
	}
	defer resetHook()

	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	pause := make(chan struct{})
	proceed := make(chan struct{})

	// Set Trap: Pause RegisterFD at critical point
	testHookRegisterFDPreInit = func() {
		close(pause) // Signal: In critical section
		<-proceed    // Wait for Stop() to complete
	}

	// Trigger Victim: RegisterFD will pause at the hook
	registerDone := make(chan error)
	go func() {
		// Use os.Stdout FD as a valid FD for registration
		_ = l.RegisterFD(int(os.Stdout.Fd()), EventRead, func(IOEvents) {})
		close(registerDone)
	}()

	// Wait until RegisterFD is paused in critical section
	select {
	case <-pause:
		// Good, we caught it
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout: RegisterFD did not reach critical section")
	}

	// Execute Attack: Call Stop() while RegisterFD holds lock
	stopDone := make(chan struct{})
	go func() {
		l.Stop(context.Background())
		close(stopDone)
	}()

	// Release the victim: Let RegisterFD continue
	close(proceed)

	// Wait for both operations to complete
	select {
	case <-registerDone:
		// RegisterFD completed
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout: RegisterFD did not complete")
	}

	select {
	case <-stopDone:
		// Stop completed
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout: Stop did not complete")
	}

	// Verify Body Count: Check for zombie poller
	l.ioPoller.mu.RLock()
	leakedFD := l.ioPoller.epfd
	isInit := l.ioPoller.initialized
	isClosed := l.ioPoller.closed
	l.ioPoller.mu.RUnlock()

	// After Stop(), poller should be closed and NOT initialized
	if isInit {
		t.Fatalf("CRITICAL FAIL: Poller is initialized after Stop(). State: initialized=%v, closed=%v, epfd=%d",
			isInit, isClosed, leakedFD)
	}

	if leakedFD > 0 && isClosed {
		t.Fatalf("CRITICAL FAIL: Zombie Poller Detected! FD %d exists after Stop() with closed=true", leakedFD)
	}

	if leakedFD > 0 {
		t.Logf("Warning: epfd=%d after Stop() (may be closed but not zeroed)", leakedFD)
		// This is acceptable if the FD is closed - we just need to verify
		// the poller is not in 'initialized' state (which would cause leaks)
	}
}
