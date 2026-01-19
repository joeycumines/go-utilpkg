package eventloop

import (
	"testing"
)

func TestBarrierOrderingModesUnit(t *testing.T) {
	// Task 7.1 & 7.2: Verify Default (Batch) vs Strict (Interleaved)

	// Case 1: Default Mode (Batch)
	{
		l, _ := New()
		l.StrictMicrotaskOrdering = false

		var order []string

		// Fill ingress queue directly (CALLER MUST HOLD MUTEX)
		// Note: This test runs single-threaded, so no race.
		// Task A: Queues M1
		l.external.Push(Task{Runnable: func() {
			order = append(order, "A")
			l.microtasks.Push(func() {
				order = append(order, "M1")
			})
		}})

		// Task B: Queues M2
		l.external.Push(Task{Runnable: func() {
			order = append(order, "B")
			l.microtasks.Push(func() {
				order = append(order, "M2")
			})
		}})

		// Simulate Tick
		// tick() calls processExternal() then drainMicrotasks()
		l.processExternal()
		l.drainMicrotasks()

		// Check Order
		// Default mode: processExternal runs A, B. Then drainMicrotasks runs M1, M2.
		expected := []string{"A", "B", "M1", "M2"}

		if len(order) != 4 {
			t.Fatalf("Default: Expected 4 items, got %d: %v", len(order), order)
		}
		for i, v := range expected {
			if order[i] != v {
				t.Errorf("Default: Index %d expected %s, got %s", i, v, order[i])
			}
		}
	}

	// Case 2: Strict Mode (Per-Task)
	{
		l, _ := New()
		l.StrictMicrotaskOrdering = true

		var order []string

		// Task A
		l.external.Push(Task{Runnable: func() {
			order = append(order, "A")
			l.microtasks.Push(func() {
				order = append(order, "M1")
			})
		}})

		// Task B
		l.external.Push(Task{Runnable: func() {
			order = append(order, "B")
			l.microtasks.Push(func() {
				order = append(order, "M2")
			})
		}})

		// Simulate Tick
		l.processExternal()
		l.drainMicrotasks() // Cleanup any remainder

		// Check Order
		// Strict mode: processExternal runs A, then drains (M1). Then runs B, then drains (M2).
		expected := []string{"A", "M1", "B", "M2"}

		if len(order) != 4 {
			t.Fatalf("Strict: Expected 4 items, got %d: %v", len(order), order)
		}
		for i, v := range expected {
			if order[i] != v {
				t.Errorf("Strict: Index %d expected %s, got %s", i, v, order[i])
			}
		}
	}
}
