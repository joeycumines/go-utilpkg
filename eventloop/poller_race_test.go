package eventloop

import (
	"testing"
)

// TestFastPoller_InitClose tests basic FastPoller initialization and close.
func TestFastPoller_InitClose(t *testing.T) {
	var p FastPoller

	if err := p.Init(); err != nil {
		t.Fatalf("Failed to init poller: %v", err)
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Failed to close poller: %v", err)
	}
}

// TestFastPoller_ConcurrentInit tests that multiple sequential Init/Close cycles are safe.
// Note: FastPoller.Init() is NOT designed for concurrent initialization from multiple goroutines.
// This test verifies that sequential initialization works correctly.
func TestFastPoller_ConcurrentInit(t *testing.T) {
	for i := 0; i < 10; i++ {
		var p FastPoller
		if err := p.Init(); err != nil {
			t.Fatalf("Failed to init poller: %v", err)
		}
		if err := p.Close(); err != nil {
			t.Fatalf("Failed to close poller: %v", err)
		}
	}
}

// TestFastPoller_ConcurrentClose tests that multiple sequential Close calls are safe (idempotent).
// Note: FastPoller.Close() should be safe to call multiple times from the same goroutine.
func TestFastPoller_ConcurrentClose(t *testing.T) {
	for i := 0; i < 10; i++ {
		var p FastPoller
		if err := p.Init(); err != nil {
			t.Fatalf("Failed to init poller: %v", err)
		}

		// Multiple sequential Close calls should be safe
		_ = p.Close()
		_ = p.Close()
		_ = p.Close()
	}
}
