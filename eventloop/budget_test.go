package eventloop

import (
	"context"
	"testing"
	"time"
)

// TestMicrotaskBudget_ResetsPolling verifies that the forceNonBlockingPoll flag
// is properly reset after usage, preventing busy-spin.
func TestMicrotaskBudget_ResetsPolling(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}

	l.forceNonBlockingPoll = true

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	l.tick(ctx)

	if l.forceNonBlockingPoll {
		t.Fatalf("CRITICAL: forceNonBlockingPoll was not reset after usage. Loop will busy-spin.")
	}
}
