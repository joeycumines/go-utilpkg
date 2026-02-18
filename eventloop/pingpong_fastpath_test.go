package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestPingPongFastPathCounter verifies that the fast path is being used
// in a ping-pong benchmark scenario (submitting tasks one at a time, waiting for each).
func TestPingPongFastPathCounter(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Force fast path for deterministic test
	if err := loop.SetFastPathMode(FastPathForced); err != nil {
		t.Fatalf("SetFastPathMode failed: %v", err)
	}

	// Start the loop in background
	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.Run(ctx)
	}()

	// Wait for loop to be running
	time.Sleep(10 * time.Millisecond)

	// Initial fast path entries
	initialEntries := loop.fastPathEntries.Load()
	t.Logf("Initial FastPathEntries: %d", initialEntries)

	// Submit 100 tasks one at a time, waiting for each to complete
	const numTasks = 100
	for i := range numTasks {
		var wg sync.WaitGroup
		wg.Add(1)
		err := loop.Submit(func() {
			wg.Done()
		})
		if err != nil {
			t.Fatalf("Failed to submit task %d: %v", i, err)
		}
		wg.Wait()
	}

	// Read final fast path entries
	finalEntries := loop.fastPathEntries.Load()

	// Report
	t.Logf("Final FastPathEntries: %d", finalEntries)
	t.Logf("Delta FastPathEntries: %d", finalEntries-initialEntries)
	t.Logf("Expected: >= 1 if fast path is working")

	// Shutdown
	cancel()
	select {
	case <-loopDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Loop did not stop in time")
	}

	// Assert that we got some fast path usage
	if finalEntries < 1 {
		t.Errorf("Expected FastPathEntries >= 1, got %d", finalEntries)
	}
}
