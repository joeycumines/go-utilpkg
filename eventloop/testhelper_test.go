package eventloop

import (
	"testing"
	"time"
)

// waitLoopState waits for a loop to reach a specific state within a timeout.
// This is used by multiple test files across platforms.
func waitLoopState(t *testing.T, loop *Loop, expected LoopState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for loop.State() != expected && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if loop.State() != expected {
		// Accept either Running or Sleeping as "running"
		state := loop.State()
		if expected == StateRunning && (state == StateRunning || state == StateSleeping) {
			return
		}
		t.Fatalf("Loop failed to reach %v state (got %v)", expected, state)
	}
}
