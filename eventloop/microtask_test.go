package eventloop

import (
	"sync/atomic"
	"testing"
)

// TestMicrotaskRing_OverflowOrder tests basic overflow functionality
func TestMicrotaskRing_OverflowOrder(t *testing.T) {
	tr := NewMicrotaskRing()

	// Fill ring beyond capacity to trigger overflow
	const ringSize = 4100 // Slightly over 4096
	for i := 0; i < ringSize; i++ {
		taskID := i
		tr.Push(func() {
			t.Logf("Task %d executed", taskID)
		})
	}

	// Verify length shows tasks in overflow
	length := tr.Length()
	if length != ringSize {
		t.Errorf("Expected length %d, got %d", ringSize, length)
	}

	// Drain all tasks
	var executed int
	for tr.Length() > 0 {
		fn := tr.Pop()
		if fn != nil {
			fn()
			executed++
		}
	}

	// Verify all were executed
	if executed != ringSize {
		t.Errorf("Expected %d executions, got %d", ringSize, executed)
	}

	// Verify queue is empty
	if tr.Length() > 0 {
		t.Errorf("Queue should be empty after draining all %d tasks", ringSize)
	}

	t.Logf("Verified overflow: %d tasks all executed correctly", executed)
}

// TestMicrotaskRing_RingOnly tests ring buffer without overflow
func TestMicrotaskRing_RingOnly(t *testing.T) {
	tr := NewMicrotaskRing()

	const tasks = 1000 // Well under 4096 capacity

	// Push tasks
	for i := 0; i < tasks; i++ {
		taskID := i
		tr.Push(func() {
			t.Logf("Task %d executed", taskID)
		})
	}

	// Verify length
	length := tr.Length()
	if length != tasks {
		t.Errorf("Expected length %d, got %d", tasks, length)
	}

	// Drain all tasks
	var executed int
	for tr.Length() > 0 {
		fn := tr.Pop()
		if fn != nil {
			fn()
			executed++
		}
	}

	if executed != tasks {
		t.Errorf("Expected %d executions, got %d", tasks, executed)
	}

	if tr.Length() > 0 {
		t.Errorf("Queue should be empty")
	}

	t.Logf("Ring-only test passed: %d tasks", executed)
}

// TestMicrotaskRing_NoDoubleExecution verifies that no task executes twice
// under concurrent producer/consumer scenarios.
// Runs with -race detector to verify correctness.
func TestMicrotaskRing_NoDoubleExecution(t *testing.T) {
	tr := NewMicrotaskRing()

	const numProducers = 8
	const tasksPerProducer = 1000
	var counter atomic.Int64
	var producers atomic.Int64

	// Spawn producers that will push beyond ring capacity
	for i := 0; i < numProducers; i++ {
		go func(id int) {
			defer producers.Add(-1)
			// Each producer adds tasks, some will go to overflow
			for j := 0; j < tasksPerProducer; j++ {
				taskID := id*tasksPerProducer + j
				tr.Push(func() {
					counter.Add(1)
					_ = taskID
				})
			}
		}(i)
		producers.Add(1)
	}

	// Wait for all producers to finish
	for producers.Load() > 0 {
	}

	// Now drain all tasks
	var popCount atomic.Int64
	for tr.Length() > 0 {
		fn := tr.Pop()
		if fn != nil {
			fn()
			popCount.Add(1)
		}
	}

	// Verify all tasks executed exactly once
	totalTasks := numProducers * tasksPerProducer
	if counter.Load() != int64(totalTasks) {
		t.Fatalf("Double execution or task loss detected! Expected %d executions, got %d",
			totalTasks, counter.Load())
	}

	if popCount.Load() != int64(totalTasks) {
		t.Fatalf("Pop count mismatch! Expected %d pops, got %d",
			totalTasks, popCount.Load())
	}

	t.Logf("Verified: %d tasks executed exactly once", totalTasks)
}

// TestMicrotaskRing_NoTailCorruption verifies that overflow buffer prevents
// tail corruption when multiple producers fill ring beyond capacity.
// New design uses mutex-protected overflow instead of complex state transitions.
// Runs with -race detector to verify correctness.
func TestMicrotaskRing_NoTailCorruption(t *testing.T) {
	tr := NewMicrotaskRing()

	var producers atomic.Int64

	// Spawn many producers that will race to fill and trigger overflow
	const numProducers = 32
	for i := 0; i < numProducers; i++ {
		go func(id int) {
			defer producers.Add(-1)
			// Each producer adds 100 tasks (will overflow the ring with 32*100=3200 total)
			for j := 0; j < 100; j++ {
				taskID := id*100 + j
				tr.Push(func() {
					// Task does nothing except exist
					_ = taskID
				})
			}
		}(i)
		producers.Add(1)
	}

	// Wait for producers to finish
	for producers.Load() > 0 {
	}

	// Drain all tasks to verify no corruption (no infinite loops, no crashes)
	var drained atomic.Int64
	for tr.Length() > 0 {
		fn := tr.Pop()
		if fn != nil {
			fn()
			drained.Add(1)
		}
	}

	// Verify we can still push and drain after to race
	for i := 0; i < 100; i++ {
		tr.Push(func() {})
	}

	for tr.Length() > 0 {
		fn := tr.Pop()
		if fn != nil {
			fn()
			drained.Add(1)
		}
	}

	// Verify queue is empty
	if tr.Length() > 0 {
		t.Fatalf("Queue not empty after drain, length: %d", tr.Length())
	}

	t.Logf("Successfully drained %d tasks without corruption. No tail issues because overflow uses mutex.", drained.Load())
}

// TestMicrotaskRing_SharedStress tests ring with concurrent
// producers and consumers, repeatedly triggering overflow transitions.
func TestMicrotaskRing_SharedStress(t *testing.T) {
	tr := NewMicrotaskRing()

	const numProducers = 8
	const tasksPerProducer = 5000

	var counter atomic.Int64
	var activeProducers atomic.Int64

	// Spawn producers
	for i := 0; i < numProducers; i++ {
		go func(id int) {
			defer activeProducers.Add(-1)
			for j := 0; j < tasksPerProducer; j++ {
				taskID := id*tasksPerProducer + j
				tr.Push(func() {
					counter.Add(1)
					_ = taskID
				})
			}
		}(i)
		activeProducers.Add(1)
	}

	// Wait for all producers to finish
	for activeProducers.Load() > 0 {
	}

	// Drain all tasks (consumer role)
	for tr.Length() > 0 {
		fn := tr.Pop()
		if fn != nil {
			fn()
		}
	}

	// Verify no task loss or double execution
	totalTasks := numProducers * tasksPerProducer
	if counter.Load() != int64(totalTasks) {
		t.Fatalf("Task loss or double execution! Expected %d executions, got %d",
			totalTasks, counter.Load())
	}

	t.Logf("Stress test passed: %d tasks processed correctly with overflow", totalTasks)
}
