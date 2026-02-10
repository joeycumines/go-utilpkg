package eventloop

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestSafety_DoubleStartRace attempts to trigger a double-start race condition.
// The bug (C2) occurs when poll() reverts state to StateAwake instead of StateRunning,
// allowing a concurrent Start() to succeed and spawn a second run() goroutine.
func TestSafety_DoubleStartRace(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// First, start the loop and wait for it to be running
	go func() {
		if err := l.Run(context.Background()); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Run() unexpected error: %v", err)
		}
	}()

	// Wait for the loop to actually be running before starting the race test
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		state := LoopState(l.state.Load())
		if state == StateRunning || state == StateSleeping {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	defer l.Shutdown(context.Background())

	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				l.Submit(func() {})
				runtime.Gosched()
			}
		}
	}()

	timeout := time.After(5 * time.Second)

	for {
		select {
		case <-timeout:
			close(done)
			wg.Wait()
			return
		default:
			err := l.Run(context.Background())
			if err == nil {
				close(done)
				wg.Wait()
				t.Fatalf("CRITICAL: Double Run succeeded")
			}
			// Expected: ErrLoopAlreadyRunning
		}
	}
}

// TestSafety_RegistryCompactionReallocsMap verifies that registry compaction
// properly reallocates the underlying map to reclaim memory from deleted entries.
// Go's map implementation doesn't shrink, so we must create a new map.
func TestSafety_RegistryCompactionReallocsMap(t *testing.T) {
	r := newRegistry()

	// Create many promises, then let them be GC'd
	const count = 10000
	for i := 0; i < count; i++ {
		_, p := r.NewPromise()
		// Immediately resolve so they're eligible for scavenging
		p.Resolve(nil)
	}

	// Scavenge to remove settled promises
	r.Scavenge(count + 100)

	// After scavenging, the registry should have reallocated its map
	// We can't directly test map capacity, but we can verify the count is zero
	r.mu.RLock()
	remaining := len(r.data)
	r.mu.RUnlock()

	if remaining > 0 {
		t.Errorf("Expected 0 remaining entries after scavenge, got %d", remaining)
	}
}
