//go:build linux || darwin

package eventloop

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestRollback_PathActuallyExecuted proves that rollback code path executes.
// CRITICAL: Fails if rollback is never reached, catching ineffective tests.
func TestRollback_PathActuallyExecuted(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l.Close()

	if err := l.SetFastPathMode(FastPathDisabled); err != nil {
		t.Fatalf("initial SetFastPathMode failed: %v", err)
	}

	var rollbackReached atomic.Bool

	l.testHooks = &loopTestHooks{
		AfterOptimisticCheck: func() {
			// Simulate RegisterFD racing between Step 1 and Step 2
			l.userIOFDCount.Add(1)
		},
		BeforeFastPathRollback: func() {
			rollbackReached.Store(true)
		},
	}

	err = l.SetFastPathMode(FastPathForced)

	// PROOF POINT 1: Rollback path was executed
	if !rollbackReached.Load() {
		t.Fatal("REGRESSION: rollback code path was NOT executed")
	}

	// PROOF POINT 2: Error returned
	if err != ErrFastPathIncompatible {
		t.Fatalf("expected ErrFastPathIncompatible, got %v", err)
	}

	// PROOF POINT 3: Previous mode restored (not Auto)
	if FastPathMode(l.fastPathMode.Load()) != FastPathDisabled {
		t.Fatalf("mode not restored to Disabled, got %v", FastPathMode(l.fastPathMode.Load()))
	}

	l.userIOFDCount.Add(-1)
}

// TestRollback_CASPreventsLostUpdate proves CAS protects concurrent writes.
func TestRollback_CASPreventsLostUpdate(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l.Close()

	var casReached atomic.Bool

	l.testHooks = &loopTestHooks{
		AfterOptimisticCheck: func() {
			l.userIOFDCount.Add(1)
		},
		BeforeFastPathRollback: func() {
			casReached.Store(true)
			// Concurrent writer intervenes BEFORE our CAS
			l.fastPathMode.Store(int32(FastPathDisabled))
		},
	}

	_ = l.SetFastPathMode(FastPathForced)

	if !casReached.Load() {
		t.Fatal("REGRESSION: CAS path not reached")
	}

	// PROOF: CAS must have failed; Disabled must be preserved
	// If CAS overwrote, we'd see Auto (the prev value before Forced)
	finalMode := FastPathMode(l.fastPathMode.Load())
	if finalMode != FastPathDisabled {
		t.Fatalf("CAS overwrote concurrent write: expected Disabled, got %v", finalMode)
	}

	l.userIOFDCount.Add(-1)
}

// TestRollback_CASSucceedsUncontended proves CAS restores prev when uncontended.
func TestRollback_CASSucceedsUncontended(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l.Close()

	if err := l.SetFastPathMode(FastPathDisabled); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	var casReached atomic.Bool

	l.testHooks = &loopTestHooks{
		AfterOptimisticCheck: func() {
			l.userIOFDCount.Add(1)
		},
		BeforeFastPathRollback: func() {
			casReached.Store(true)
			// No intervention - CAS should succeed
		},
	}

	_ = l.SetFastPathMode(FastPathForced)

	if !casReached.Load() {
		t.Fatal("CAS path not reached")
	}

	// PROOF: CAS succeeded, Disabled restored
	if FastPathMode(l.fastPathMode.Load()) != FastPathDisabled {
		t.Fatalf("CAS failed to restore: expected Disabled, got %v",
			FastPathMode(l.fastPathMode.Load()))
	}

	l.userIOFDCount.Add(-1)
}

// TestRollback_AllPreviousModes proves each starting mode is correctly restored.
func TestRollback_AllPreviousModes(t *testing.T) {
	testCases := []FastPathMode{FastPathAuto, FastPathDisabled}

	for _, prevMode := range testCases {
		t.Run(prevMode.String(), func(t *testing.T) {
			l, _ := New()
			defer l.Close()

			_ = l.SetFastPathMode(prevMode)

			var reached atomic.Bool
			l.testHooks = &loopTestHooks{
				AfterOptimisticCheck:   func() { l.userIOFDCount.Add(1) },
				BeforeFastPathRollback: func() { reached.Store(true) },
			}

			_ = l.SetFastPathMode(FastPathForced)

			if !reached.Load() {
				t.Fatal("rollback not reached")
			}
			if FastPathMode(l.fastPathMode.Load()) != prevMode {
				t.Fatalf("expected %v, got %v", prevMode, FastPathMode(l.fastPathMode.Load()))
			}

			l.userIOFDCount.Add(-1)
		})
	}
}

// TestRollback_InvariantUnderStress proves invariant holds under heavy concurrency.
// Runs 10000 iterations with 8 concurrent goroutines.
func TestRollback_InvariantUnderStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	const (
		iterations = 10000
		goroutines = 8
	)

	for i := 0; i < iterations; i++ {
		l, _ := New()

		var wg sync.WaitGroup

		// Half try to force mode
		for j := 0; j < goroutines/2; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = l.SetFastPathMode(FastPathForced)
			}()
		}

		// Half manipulate count
		for j := 0; j < goroutines/2; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				l.userIOFDCount.Add(1)
				runtime.Gosched()
				l.userIOFDCount.Add(-1)
			}()
		}

		wg.Wait()

		mode := FastPathMode(l.fastPathMode.Load())
		count := l.userIOFDCount.Load()

		if mode == FastPathForced && count != 0 {
			l.Close()
			t.Fatalf("iter %d: INVARIANT VIOLATED mode=Forced count=%d", i, count)
		}

		l.Close()
	}
}

// TestRollback_DoubleRollbackRace tests concurrent rollback scenarios.
//
// ABA RACE MITIGATION - ACCEPTED TRADE-OFF:
// When two goroutines call SetFastPathMode(Forced) concurrently while count changes,
// the CAS-based rollback mechanism provides a safe final state but may result in
// "de-synchronization" from the user's perspective:
//
//  1. At least one goroutine returns ErrFastPathIncompatible (guaranteed)
//  2. Final mode is NOT FastPathForced when count > 0 (guaranteed safe)
//  3. However, final mode may be either FastPathAuto OR FastPathDisabled (both safe)
//  4. The specific goroutine that failed may not be the one that "should" have failed
//     relative to the final state
//
// This mitigation only occurs when SetFastPathMode is MISUSED (called concurrently
// with count changes during FD registration/unregistration in different goroutines).
// Proper usage synchronizes mode changes with FD registration.
//
// The trade-off is acceptable because:
//   - The system remains SAFE (IO events are still handled - mode is never FastPathForced
//     when count > 0)
//   - The complex alternative (full write serialization) would harm performance for
//     proper (synchronized) usage
//   - Misused concurrent mode changes are inherently ambiguous about "which" operation
//     should win - there is no correct ordering
//
// This test documents the edge case and verifies the safety guarantees are maintained.
func TestRollback_DoubleRollbackRace(t *testing.T) {
	// Run iterations to stress the race condition
	for i := 0; i < 100; i++ {
		l, _ := New()
		_ = l.SetFastPathMode(FastPathDisabled)

		var barrier atomic.Int32
		var rollbackWait sync.WaitGroup
		rollbackWait.Add(2)
		var wg sync.WaitGroup

		// Shared hook for both goroutines - set once, not raced
		l.testHooks = &loopTestHooks{
			AfterOptimisticCheck: func() {
				// Wait for BOTH goroutines to reach this point (STEP 1 passed, before Swap happens)
				if barrier.Add(1) == 1 {
					// First goroutine: wait for second
					for barrier.Load() < 2 {
						runtime.Gosched()
					}
					// Both goroutines passed optimistic check. NOW increment count
					// so BOTH will see it in Step 3's check, triggering rollback for both
					l.userIOFDCount.Add(1)
					barrier.Add(1) // Signal second goroutine to proceed
				} else {
					// Second goroutine: wait for first to increment count
					for barrier.Load() < 3 {
						runtime.Gosched()
					}
				}
			},
			BeforeFastPathRollback: func() {
				// Both goroutines wait here, creating double-rollback race
				rollbackWait.Done()
				rollbackWait.Wait()
			},
		}

		// Track errors from both goroutines to verify invariant #1
		var errors [2]error

		// Two goroutines both try Forced, both should rollback
		for j := 0; j < 2; j++ {
			wg.Add(1)
			goroutineIdx := j
			go func(idx int) {
				defer wg.Done()
				errors[idx] = l.SetFastPathMode(FastPathForced)
			}(goroutineIdx)
		}
		wg.Wait()

		// VERIFICATION #1: At least one goroutine must return ErrFastPathIncompatible
		// This is guaranteed by the algorithm - if count > 0, SetFastPathMode(Forced) must fail
		if errors[0] != ErrFastPathIncompatible && errors[1] != ErrFastPathIncompatible {
			t.Fatalf("iter %d: INVARIANT VIOLATION - neither goroutine returned ErrFastPathIncompatible (got %v, %v)",
				i, errors[0], errors[1])
		}

		// Collect final state for verification
		mode := FastPathMode(l.fastPathMode.Load())
		count := l.userIOFDCount.Load()

		// VERIFICATION #2: Final mode is NOT FastPathForced when count > 0 (guaranteed safe)
		// This is the critical safety invariant - IO events MUST be handled when FDs are registered
		if mode == FastPathForced && count > 0 {
			t.Fatalf("iter %d: SAFETY VIOLATION - mode=Forced with count=%d (IO events would be dropped!)",
				i, count)
		}

		// VERIFICATION #3: Final mode may be either FastPathAuto OR FastPathDisabled (both safe)
		// This documents the accepted ABA race mitigation behavior
		if mode != FastPathDisabled && mode != FastPathAuto {
			t.Fatalf("iter %d: UNEXPECTED final state - mode=%v count=%d (expected Auto or Disabled)",
				i, mode, count)
		}

		// VERIFICATION #4: The specific goroutine that failed may not be the one that
		// "should" have failed relative to final state. This is documented behavior.
		// Example: final mode=Auto forgoroutine 0 failed but goroutine 1 succeeded,
		// even though final mode matches goroutine 1's previous mode.
		t.Logf("iter %d: ABA mitigation - errors=(%v, %v), final mode=%v, count=%d",
			i, errors[0], errors[1], mode, count)

		// Cleanup: restore initial state
		l.userIOFDCount.Add(-1)
		l.Close()
	}
}

// TestRollback_RaceWithActualRegisterFD tests rollback with real RegisterFD calls.
func TestRollback_RaceWithActualRegisterFD(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	const (
		iterations = 500
		goroutines = 10
	)

	for i := 0; i < iterations; i++ {
		l, _ := New()

		var wg sync.WaitGroup

		// Goroutines changing mode
		for j := 0; j < goroutines; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Try each mode
				_ = l.SetFastPathMode(FastPathAuto)
				_ = l.SetFastPathMode(FastPathDisabled)
				_ = l.SetFastPathMode(FastPathForced) // May fail
			}()
		}

		// Goroutines registering/unregistering FDs
		fds, err := createPipePair()
		if err != nil {
			l.Close()
			t.Fatalf("iter %d: createPipePair failed: %v", i, err)
		}

		for j := 0; j < goroutines; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Quick register/unregister cycle
				// Use no-op callback instead of nil to avoid potential panic if events arrive
				callback := func(IOEvents) {}
				if err := l.RegisterFD(fds[0], EventRead, callback); err == nil {
					time.Sleep(time.Microsecond)
					_ = l.UnregisterFD(fds[0])
				}
			}()
		}

		wg.Wait()

		// Verify invariant
		mode := FastPathMode(l.fastPathMode.Load())
		count := l.userIOFDCount.Load()

		if mode == FastPathForced && count != 0 {
			_ = closePipePair(fds)
			l.Close()
			t.Fatalf("iter %d: INVARIANT VIOLATED mode=Forced count=%d", i, count)
		}

		_ = closePipePair(fds)
		l.Close()
	}
}

// Helper String method for test output
func (m FastPathMode) String() string {
	switch m {
	case FastPathAuto:
		return "Auto"
	case FastPathForced:
		return "Forced"
	case FastPathDisabled:
		return "Disabled"
	default:
		return "Unknown"
	}
}

// Helper functions for pipe pair creation/cleanup
type pipePair [2]int

func createPipePair() (pipePair, error) {
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		return pipePair{}, err
	}
	return pipePair(fds), nil
}

func closePipePair(p pipePair) error {
	if p[0] >= 0 {
		unix.Close(p[0])
	}
	if p[1] >= 0 && p[1] != p[0] {
		unix.Close(p[1])
	}
	return nil
}

// Pipe and Close are minimal wrappers around unix syscalls for test portability
var (
// Mock the FD operations for test portability
// In real usage, these would go through the poller/Pipe syscalls
)
