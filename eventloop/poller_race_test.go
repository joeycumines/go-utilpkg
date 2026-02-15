package eventloop

import (
	"testing"
)

// Test_fastPoller_InitClose tests basic fastPoller initialization and close.
func Test_fastPoller_InitClose(t *testing.T) {
	var p fastPoller

	if err := p.Init(); err != nil {
		t.Fatalf("Failed to init poller: %v", err)
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Failed to close poller: %v", err)
	}
}

// Test_fastPoller_ConcurrentInit tests that multiple sequential Init/Close cycles are safe.
// Note: fastPoller.Init() is NOT designed for concurrent initialization from multiple goroutines.
// This test verifies that sequential initialization works correctly.
func Test_fastPoller_ConcurrentInit(t *testing.T) {
	for range 10 {
		var p fastPoller
		if err := p.Init(); err != nil {
			t.Fatalf("Failed to init poller: %v", err)
		}
		if err := p.Close(); err != nil {
			t.Fatalf("Failed to close poller: %v", err)
		}
	}
}

// Test_fastPoller_ConcurrentClose tests that multiple sequential Close calls are safe (idempotent).
// Note: fastPoller.Close() should be safe to call multiple times from the same goroutine.
func Test_fastPoller_ConcurrentClose(t *testing.T) {
	for range 10 {
		var p fastPoller
		if err := p.Init(); err != nil {
			t.Fatalf("Failed to init poller: %v", err)
		}

		// Multiple sequential Close calls should be safe
		_ = p.Close()
		_ = p.Close()
		_ = p.Close()
	}
}
