package eventloop

import (
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/sys/unix"
)

func TestFastPathMode_DefaultIsAuto(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l.Close()

	if FastPathMode(l.fastPathMode.Load()) != FastPathAuto {
		t.Fatalf("default fastPathMode != FastPathAuto, got %v", FastPathMode(l.fastPathMode.Load()))
	}
}

func TestSetFastPathMode_Incompatible(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l.Close()

	// Simulate registered I/O FD
	l.userIOFDCount.Add(1)
	if err := l.SetFastPathMode(FastPathForced); err == nil {
		t.Fatalf("expected ErrFastPathIncompatible when forcing fast path with I/O FDs, got nil")
	}
	// Clean up counter to keep state sane
	l.userIOFDCount.Add(-1)
}

// TestRegisterFD_RejectsWhenForced verifies RegisterFD returns ErrFastPathIncompatible
// when mode is explicitly set to FastPathForced.
func TestRegisterFD_RejectsWhenForced(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l.Close()

	// Establish forced mode (allowed when count=0)
	if err := l.SetFastPathMode(FastPathForced); err != nil {
		t.Fatalf("SetFastPathMode(Forced) failed: %v", err)
	}

	// Create test FD (pipe)
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	// Attempt registration - must fail
	err = l.RegisterFD(fds[0], EventRead, func(IOEvents) {})
	if err != ErrFastPathIncompatible {
		t.Fatalf("RegisterFD with Forced mode: expected ErrFastPathIncompatible, got %v", err)
	}

	// Verify count remains 0 (rollback successful)
	if count := l.userIOFDCount.Load(); count != 0 {
		t.Fatalf("userIOFDCount after failed RegisterFD: expected 0, got %d", count)
	}
}

// TestFastPathAuto_DisablesWhenFDRegistered verifies FastPathAuto mode
// auto-detects and disables fast path when I/O FDs are registered.
func TestFastPathAuto_DisablesWhenFDRegistered(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l.Close()

	// Verify we can use fast path initially (no FDs, Auto mode)
	if !l.canUseFastPath() {
		t.Fatalf("expected canUseFastPath()=true with Auto mode and no FDs")
	}

	// Create test FD (pipe)
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	// Register the FD
	if err := l.RegisterFD(fds[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Verify count is now 1
	if count := l.userIOFDCount.Load(); count != 1 {
		t.Fatalf("userIOFDCount after RegisterFD: expected 1, got %d", count)
	}

	// Should NOT use fast path when FDs present (Auto mode)
	if l.canUseFastPath() {
		t.Fatalf("expected canUseFastPath()=false with Auto mode and FDs registered")
	}

	// Cleanup: Unregister FD
	if err := l.UnregisterFD(fds[0]); err != nil {
		t.Fatalf("UnregisterFD failed: %v", err)
	}

	// Verify count back to 0
	if count := l.userIOFDCount.Load(); count != 0 {
		t.Fatalf("userIOFDCount after UnregisterFD: expected 0, got %d", count)
	}

	// Verify fast path is available again after unregistering
	if !l.canUseFastPath() {
		t.Fatalf("expected canUseFastPath()=true after UnregisterFD in Auto mode")
	}
}

// TestFastPathForced_InvariantUnderConcurrency runs 1000 concurrent races between
// RegisterFD and SetFastPathMode(Forced), verifying the invariant is maintained:
//
//	If mode == FastPathForced, then userIOFDCount == 0
func TestFastPathForced_InvariantUnderConcurrency(t *testing.T) {
	// Run 1000 iterations to expose races
	for iteration := 0; iteration < 1000; iteration++ {
		l, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		// Create test FD for registration attempts
		var fds [2]int
		if err := unix.Pipe(fds[:]); err != nil {
			l.Close()
			t.Fatalf("Pipe failed: %v", err)
		}

		var wg sync.WaitGroup
		var registerErr error
		var modeErr error

		// Goroutine 1: Try to register FD
		wg.Add(1)
		go func() {
			defer wg.Done()
			registerErr = l.RegisterFD(fds[0], EventRead, func(IOEvents) {})
		}()

		// Goroutine 2: Try to force fast path mode
		wg.Add(1)
		go func() {
			defer wg.Done()
			modeErr = l.SetFastPathMode(FastPathForced)
		}()

		wg.Wait()

		// Verify invariant: One should succeed, one should fail,
		// but never both succeed in violation state.

		// Get final state
		finalMode := FastPathMode(l.fastPathMode.Load())
		finalCount := l.userIOFDCount.Load()

		// INVARIANT: mode == Forced ⟹ count == 0
		if finalMode == FastPathForced && finalCount != 0 {
			// Clean up before failing
			if registerErr == nil {
				_ = l.poller.UnregisterFD(fds[0])
			}
			l.Close()
			unix.Close(fds[0])
			unix.Close(fds[1])
			t.Fatalf("Iteration %d: INVARIANT VIOLATED - mode=Forced but count=%d", iteration, finalCount)
		}

		// At least one must have failed (either registration rejected
		// or mode rejection or rollback happened)
		// Note: Both CAN fail in race scenarios, that's OK
		if registerErr == nil && modeErr == nil {
			// This is allowed: RegisterFD succeeded (FD registered),
			// and SetFastPathMode(Forced) also succeeded (after RegisterFD)
			// BUT then we should have Mode=Forced and Count=0 via rollback
			unix.Close(fds[0])
			unix.Close(fds[1])
			if finalMode == FastPathForced && finalCount == 0 {
				// This is correctly handled - rollback occurred
			} else {
				t.Fatalf("Iteration %d: both succeeded but invariant not enforced (mode=%v, count=%d)", iteration, finalMode, finalCount)
			}
		}

		// Cleanup
		if registerErr == nil {
			_ = l.poller.UnregisterFD(fds[0])
		}
		l.Close()
		unix.Close(fds[0])
		unix.Close(fds[1])
	}
}

// TestSetFastPathMode_RollbackPreservesPreviousMode verifies that when SetFastPathMode
// fails due to incompatible FDs, the previous explicit mode is restored (no silent
// fallback to FastPathAuto). This test uses hooks to ensure the rollback path is
// ACTUALLY EXECUTED, not just the early-return path.
//
// CRITICAL: The test uses AfterOptimisticCheck hook to increment count AFTER
// Step 1's optimistic check but BEFORE Step 2's Swap. Without the hook, setting
// count first would cause immediate early return, giving false confidence that
// rollback works.
func TestSetFastPathMode_RollbackPreservesPreviousMode(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l.Close()

	// Explicitly set Disabled mode
	if err := l.SetFastPathMode(FastPathDisabled); err != nil {
		t.Fatalf("SetFastPathMode(Disabled) failed: %v", err)
	}
	if FastPathMode(l.fastPathMode.Load()) != FastPathDisabled {
		t.Fatalf("expected mode Disabled, got %v", FastPathMode(l.fastPathMode.Load()))
	}

	// PROOF POINT: Verify rollback code path is executed
	var rollbackReached atomic.Bool

	l.testHooks = &loopTestHooks{
		AfterOptimisticCheck: func() {
			// Simulate a RegisterFD racing between Step 1 and Step 2:
			// - Step 1: count check passes (count is still 0)
			// - We increment count here (now count is 1)
			// - Step 2: Swap mode to Forced
			// - Step 3: Verify count, detect incompatibility
			// - Rollback: CAS mode back to Disabled
			l.userIOFDCount.Add(1)
		},
		BeforeFastPathRollback: func() {
			rollbackReached.Store(true)
		},
	}

	// Attempt to force fast path - should fail, rollback, and preserve Disabled
	if err := l.SetFastPathMode(FastPathForced); err != ErrFastPathIncompatible {
		t.Fatalf("expected ErrFastPathIncompatible, got %v", err)
	}

	// PROOF POINT 1: Rollback path was actually executed
	if !rollbackReached.Load() {
		t.Fatal("REGRESSION: Test did not execute rollback path. Hook-based proof failed.")
	}

	// PROOF POINT 2: Previous mode preserved (not Auto fallback)
	if FastPathMode(l.fastPathMode.Load()) != FastPathDisabled {
		t.Fatalf("expected mode to remain Disabled after failed SetFastPathMode, got %v", FastPathMode(l.fastPathMode.Load()))
	}

	// Cleanup: restore count before next test
	l.userIOFDCount.Add(-1)
}
