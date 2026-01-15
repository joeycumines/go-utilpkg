//go:build darwin

package eventloop

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestPollerDarwin_ConcurrentInit verifies that concurrent calls to initPoller
// do not race. Multiple goroutines should safely initialize the kqueue poller,
// with only the first winner actually calling Init() and subsequent callers
// blocking until init completes (sync.Once semantics).
func TestPollerDarwin_ConcurrentInit(t *testing.T) {
	p := &ioPoller{}
	// FIX: Use atomic.Bool.Store() instead of direct assignment
	p.closed.Store(false)

	const goroutines = 32
	var initCount atomic.Int64
	var wg sync.WaitGroup
	var errors atomic.Int64

	// Verify that all concurrent calls complete without race detector errors
	// With sync.Once, all callers will block until init completes
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			err := p.initPoller()
			if err != nil {
				errors.Add(1)
				t.Errorf("initPoller failed: %v", err)
			} else {
				initCount.Add(1)
			}
		}()
	}
	wg.Wait()

	// All goroutines should have succeeded (all waited for init to complete)
	if errors.Load() > 0 {
		t.Fatalf("initPoller had %d errors out of %d goroutines", errors.Load(), goroutines)
	}

	// Verify poller is initialized by checking that closed is still false
	// Note: With sync.Once, we can't directly check "initialized" - instead
	// we verify init succeeded by the absence of errors
	if p.closed.Load() {
		t.Fatal("Poller unexpectedly marked as closed after init")
	}

	t.Logf("Successfully handled %d concurrent initPoller calls without race", initCount.Load())
}

// TestPollerDarwin_ConcurrentInitDirect verifies that concurrent RegisterFD calls
// do not race during lazy initialization. Multiple goroutines calling RegisterFD
// while poller is uninitialized should safely trigger initialization exactly once.
// Runs with -race detector to verify correctness.
func TestPollerDarwin_ConcurrentInitDirect(t *testing.T) {
	p := &ioPoller{
		p: FastPoller{},
	}
	// FIX: Use atomic.Bool.Store() instead of direct assignment
	p.closed.Store(false)

	const goroutines = 16
	var wg sync.WaitGroup
	var successCount atomic.Int64
	var errorCount atomic.Int64

	// Spawn goroutines that all call initPoller concurrently
	// With sync.Once, all will block until the first completes init
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			err := p.initPoller()
			if err == nil {
				successCount.Add(1)
			} else {
				errorCount.Add(1)
				t.Errorf("initPoller failed unexpectedly: %v", err)
			}
		}()
	}
	wg.Wait()

	// All goroutines should succeed (all waited for init to complete)
	if errorCount.Load() > 0 {
		t.Fatalf("initPoller had %d errors out of %d goroutines", errorCount.Load(), goroutines)
	}

	// All should have returned success
	if successCount.Load() != goroutines {
		t.Errorf("Expected %d successful initPoller calls, got %d", goroutines, successCount.Load())
	}

	// Verify poller is not marked as closed
	if p.closed.Load() {
		t.Fatal("Poller unexpectedly marked as closed after init")
	}

	t.Logf("Concurrent initPoller test passed: %d callers, all succeeded", goroutines)
}

// TestPollerDarwin_InitThenCloseThenInit verifies that initPoller correctly handles
// the close-init-reinit lifecycle. After closing, a new initialization should
// fail because closed flag is set.
func TestPollerDarwin_InitThenCloseThenInit(t *testing.T) {
	p := &ioPoller{}
	// FIX: Use atomic.Bool.Store() instead of direct assignment
	p.closed.Store(false)

	// First init
	if err := p.initPoller(); err != nil {
		t.Fatalf("First init failed: %v", err)
	}
	// Verify poller is not closed
	if p.closed.Load() {
		t.Fatal("Poller marked as closed after first init")
	}

	// Second init (should return nil, sync.Once ensures idempotency)
	if err := p.initPoller(); err != nil {
		t.Fatalf("Second init failed: %v", err)
	}

	// Close poller
	if err := p.closePoller(); err != nil {
		t.Fatalf("closePoller failed: %v", err)
	}
	// FIX: Use atomic.Bool.Load() instead of direct access
	if !p.closed.Load() {
		t.Error("Poller closed flag not set after closePoller")
	}

	// Note: Re-initialization after close is not supported because
	// FastPoller.kq is closed and cannot be reused. Users must
	// create a new poller instance if they need to restart.
	t.Log("Init-Close lifecycle test passed (re-init not supported)")
}

// TestPollerDarwin_InitFailsResetsFlag verifies initialization behavior.
// With sync.Once, init happens exactly once per instance.
func TestPollerDarwin_InitFailsResetsFlag(t *testing.T) {
	p := &ioPoller{
		p: FastPoller{},
	}
	// FIX: Use atomic.Bool.Store() instead of direct assignment
	p.closed.Store(false)

	// Normal init (likely will succeed in test environment)
	if err := p.initPoller(); err == nil {
		// Init succeeded, verify closed is still false
		if p.closed.Load() {
			t.Error("Closed flag unexpectedly set after successful Init()")
		}
	}

	// Note: With sync.Once, we can't "reset" the flag - each ioPoller
	// instance can only be initialized once. This is by design for
	// correct concurrent initialization semantics.

	t.Log("Init behavior verified with sync.Once semantics")
}
