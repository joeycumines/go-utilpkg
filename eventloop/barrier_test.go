package eventloop

import (
	"testing"
)

func TestBarrierOrderingModesUnit(t *testing.T) {
	// Task 7.2: Verify Per-Task Barrier (Interleaved)
	// Strict microtask ordering is now the only behavior (unconditional).

	{
		l, _ := New()

		var order []string

		// Task A
		l.external.Push(func() {
			order = append(order, "A")
			l.microtasks.Push(func() {
				order = append(order, "M1")
			})
		})

		// Task B
		l.external.Push(func() {
			order = append(order, "B")
			l.microtasks.Push(func() {
				order = append(order, "M2")
			})
		})

		// Simulate Tick
		l.processExternal()
		l.drainMicrotasks() // Cleanup any remainder

		// Check Order
		// Per-task barrier: processExternal runs A, then drains (M1). Then runs B, then drains (M2).
		expected := []string{"A", "M1", "B", "M2"}

		if len(order) != 4 {
			t.Fatalf("Expected 4 items, got %d: %v", len(order), order)
		}
		for i, v := range expected {
			if order[i] != v {
				t.Errorf("Index %d expected %s, got %s", i, v, order[i])
			}
		}
	}
}
