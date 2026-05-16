package eventloop

import (
	"testing"
)

// TestMicrotaskBudget_ResetsPolling verifies that the forceNonBlockingPoll flag
// is properly reset after usage, preventing busy-spin.
func TestMicrotaskBudget_ResetsPolling(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// Set up state to Running so poll() doesn't early-return
	l.state.Store(StateRunning)

	l.forceNonBlockingPoll = true

	// tick() no longer takes context - it's handled by run()
	l.tick()

	if l.forceNonBlockingPoll {
		t.Fatalf("CRITICAL: forceNonBlockingPoll was not reset after usage. Loop will busy-spin.")
	}
}
