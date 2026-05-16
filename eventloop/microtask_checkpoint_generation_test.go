package eventloop

import (
	"slices"
	"testing"
)

func TestMicrotaskCheckpointScheduledFromCheckpointYieldsToPrimaryMicrotasks(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	var order []string
	if err := loop.scheduleMicrotaskCheckpoint(func() {
		order = append(order, "first-checkpoint")
		loop.scheduleMicrotask(func() {
			order = append(order, "primary-microtask")
		})
		if err := loop.scheduleMicrotaskCheckpoint(func() {
			order = append(order, "second-checkpoint")
		}); err != nil {
			t.Fatalf("schedule nested checkpoint: %v", err)
		}
	}); err != nil {
		t.Fatalf("schedule checkpoint: %v", err)
	}

	loop.drainMicrotasks()

	want := []string{"first-checkpoint", "primary-microtask", "second-checkpoint"}
	if !slices.Equal(order, want) {
		t.Fatalf("checkpoint/primary order = %v, want %v", order, want)
	}
}
