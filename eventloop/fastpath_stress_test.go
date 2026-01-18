package eventloop

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestFastPath_Stress performs concurrent operations on fast path mode and FD registration
// to verify the invariant holds under high contention.
func TestFastPath_Stress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l.Close()

	var wg sync.WaitGroup
	done := make(chan struct{})

	// Goroutine 1: Randomly toggles modes
	wg.Add(1)
	go func() {
		defer wg.Done()
		modes := []FastPathMode{FastPathAuto, FastPathForced, FastPathDisabled}
		rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
		for {
			select {
			case <-done:
				return
			default:
				// Pick random mode
				m := modes[rnd.Intn(len(modes))]
				l.SetFastPathMode(m)
				// Tiny sleep to induce context switches
				time.Sleep(time.Microsecond)
			}
		}
	}()

	// Goroutine 2: Randomly registers/unregisters FDs
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Use a pipe for valid FDs
		fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
		if err != nil {
			t.Errorf("Socketpair failed: %v", err)
			return
		}
		registered := false

		for {
			select {
			case <-done:
				if registered {
					l.UnregisterFD(fds[0])
				}
				unix.Close(fds[0])
				unix.Close(fds[1])
				return
			default:
				if registered {
					l.UnregisterFD(fds[0])
					registered = false
				} else {
					// Try to register. If mode is Forced, this should fail safely.
					err := l.RegisterFD(fds[0], EventRead, func(IOEvents) {})
					if err == nil {
						registered = true
					} else if err != ErrFastPathIncompatible {
						// Expected error is ErrFastPathIncompatible when forced
						// Other errors are unexpected
						t.Logf("RegisterFD failed with unexpected error: %v", err)
					}
				}
				time.Sleep(time.Microsecond)
			}
		}
	}()

	// Run for 1 second (shorter than plan's 2s for CI)
	time.Sleep(1 * time.Second)
	close(done)
	wg.Wait()

	// Final Invariant Check
	mode := FastPathMode(l.fastPathMode.Load())
	count := l.userIOFDCount.Load()

	if mode == FastPathForced && count > 0 {
		t.Fatalf("Stress test ended in invalid state: Mode=Forced but Count=%d", count)
	}

	// Verify count is reasonable (should be 0 or 1 depending on final op)
	if count < 0 || count > 1 {
		t.Fatalf("Stress test ended with invalid count: %d", count)
	}

	t.Logf("Stress test completed successfully. Final state: Mode=%v, Count=%d", mode, count)
}

// TestFastPath_ConcurrentModeChanges stresses the SetFastPathMode with multiple concurrent callers.
func TestFastPath_ConcurrentModeChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer l.Close()

	var wg sync.WaitGroup
	done := make(chan struct{})
	modes := []FastPathMode{FastPathAuto, FastPathForced, FastPathDisabled}
	numGoroutines := 10

	// Launch multiple goroutines that all call SetFastPathMode concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			rnd := rand.New(rand.NewSource(time.Now().UnixNano() + int64(gid)))
			for {
				select {
				case <-done:
					return
				default:
					m := modes[rnd.Intn(len(modes))]
					l.SetFastPathMode(m)
					time.Sleep(time.Microsecond)
				}
			}
		}(i)
	}

	time.Sleep(500 * time.Millisecond)
	close(done)
	wg.Wait()

	// Just verify we didn't crash or cause data races
	finalMode := FastPathMode(l.fastPathMode.Load())
	t.Logf("Concurrent mode changes test completed. Final mode: %v", finalMode)
}
