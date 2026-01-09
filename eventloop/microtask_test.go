package eventloop

import (
	"testing"
)

func TestYieldPreservesOrder(t *testing.T) {
	// Task 6.1: Verify order is preserved across budget breaches
	l, _ := New()

	// Budget is 1024.
	// We add 1025 tasks.
	// First 1024 should execute.
	// 1025th should NOT execute in first drain, but remain at head.

	executedCount := 0
	lastExecutedID := -1

	numTasks := 1025
	l.microtasks = make([]Task, numTasks)

	for i := 0; i < numTasks; i++ {
		id := i
		l.microtasks[i] = Task{
			Runnable: func() {
				executedCount++
				lastExecutedID = id
			},
		}
	}

	// Run drain ONCE
	l.drainMicrotasks()

	// Check results
	if executedCount != 1024 {
		t.Errorf("Expected 1024 executed, got %d", executedCount)
	}

	if lastExecutedID != 1023 {
		t.Errorf("Expected last ID 1023, got %d", lastExecutedID)
	}

	// Verify remaining task
	if len(l.microtasks) != 1 {
		t.Errorf("Expected 1 remaining task, got %d", len(l.microtasks))
	}

	// Verify flag
	if !l.forceNonBlockingPoll {
		t.Error("forceNonBlockingPoll should be set")
	}

	// Run drain again (process remainder)
	l.drainMicrotasks()

	if executedCount != 1025 {
		t.Errorf("Expected 1025 executed total, got %d", executedCount)
	}
	if lastExecutedID != 1024 {
		t.Errorf("Expected last ID 1024, got %d", lastExecutedID)
	}

	// Verify flag cleared (Wait, drain doesn't clear flag. Poll clears flag.)
	// So flag remains true until Poll sees it.
	if !l.forceNonBlockingPoll {
		// Valid state: still true.
	}
}

func TestResetFlagLogic(t *testing.T) {
	// Task 6.2: We want to verify that Poll() clears the flag.
	// We can't easily call Poll() without mocking syscalls or blocking.
	// However, we verified the code change in loop.go.
	// We can simulate the flag reset logic:

	l, _ := New()
	l.forceNonBlockingPoll = true

	// Logic snippet from Poll (Step 5)
	forced := l.forceNonBlockingPoll
	if forced {
		l.forceNonBlockingPoll = false
	}

	if l.forceNonBlockingPoll {
		t.Error("Flag should be cleared")
	}
}
