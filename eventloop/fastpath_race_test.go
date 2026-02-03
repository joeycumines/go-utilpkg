//go:build linux || darwin

package eventloop

import (
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestSetFastPathMode_RollbackWhenNoConcurrentChange verifies that rollback
// works correctly when there is no concurrent modification.
func TestSetFastPathMode_RollbackWhenNoConcurrentChange(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l.Close()

	// Start in Auto mode
	if FastPathMode(l.fastPathMode.Load()) != FastPathAuto {
		t.Fatalf("initial mode should be Auto, got %v", FastPathMode(l.fastPathMode.Load()))
	}

	// Register an FD
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	err = l.RegisterFD(fds[0], EventRead, func(IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Try to set Forced - should fail and rollback to Auto
	err = l.SetFastPathMode(FastPathForced)
	if err != ErrFastPathIncompatible {
		t.Fatalf("expected ErrFastPathIncompatible, got %v", err)
	}

	// Mode should be back to Auto (CAS-based rollback succeeded)
	if FastPathMode(l.fastPathMode.Load()) != FastPathAuto {
		t.Fatalf("mode after rollback should be Auto, got %v", FastPathMode(l.fastPathMode.Load()))
	}
}

// TestSetFastPathMode_ConcurrentChanges tests multiple goroutines calling
// SetFastPathMode to ensure no lost updates occur.
func TestSetFastPathMode_ConcurrentChanges(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l.Close()

	var wg sync.WaitGroup
	done := make(chan struct{})
	modes := []FastPathMode{FastPathAuto, FastPathDisabled}

	// Launch multiple goroutines that all call SetFastPathMode concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					// Toggle between Auto and Disabled (avoid Forced which errors out)
					for _, m := range modes {
						l.SetFastPathMode(m)
					}
				}
			}
		}()
	}

	// Let them run for a bit
	time.Sleep(100 * time.Millisecond)
	close(done)
	wg.Wait()

	// Verify final state is consistent
	finalMode := FastPathMode(l.fastPathMode.Load())
	if finalMode != FastPathAuto && finalMode != FastPathDisabled {
		t.Fatalf("Invalid final mode: %v", finalMode)
	}
}

// TestSetFastPathMode_ForceThenDisable tests a specific sequence to ensure
// mode changes don't interfere with each other.
func TestSetFastPathMode_ForceThenDisable(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l.Close()

	// Set Forced (allowed since no FDs)
	if err := l.SetFastPathMode(FastPathForced); err != nil {
		t.Fatalf("SetFastPathMode(Forced) failed: %v", err)
	}

	if FastPathMode(l.fastPathMode.Load()) != FastPathForced {
		t.Fatalf("mode should be Forced after SetFastPathMode(Forced)")
	}

	// Now set Disabled (should succeed)
	if err := l.SetFastPathMode(FastPathDisabled); err != nil {
		t.Fatalf("SetFastPathMode(Disabled) failed: %v", err)
	}

	if FastPathMode(l.fastPathMode.Load()) != FastPathDisabled {
		t.Fatalf("mode should be Disabled after SetFastPathMode(Disabled)")
	}
}
