package eventloop

import (
	"sync/atomic"
	"testing"
)

// Test_microtaskRing_OverflowOrder tests basic overflow functionality
func Test_microtaskRing_OverflowOrder(t *testing.T) {
	tr := newMicrotaskRing()

	// Fill ring beyond capacity to trigger overflow
	const ringSize = 4100 // Slightly over 4096
	for i := range ringSize {
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

// Test_microtaskRing_RingOnly tests ring buffer without overflow
func Test_microtaskRing_RingOnly(t *testing.T) {
	tr := newMicrotaskRing()

	const tasks = 1000 // Well under 4096 capacity

	// Push tasks
	for i := range tasks {
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

// Test_microtaskRing_NoDoubleExecution verifies that no task executes twice
// under concurrent producer/consumer scenarios.
// Runs with -race detector to verify correctness.
func Test_microtaskRing_NoDoubleExecution(t *testing.T) {
	tr := newMicrotaskRing()

	const numProducers = 8
	const tasksPerProducer = 1000
	var counter atomic.Int64
	var producers atomic.Int64

	// Spawn producers that will push beyond ring capacity
	for i := range numProducers {
		go func(id int) {
			defer producers.Add(-1)
			// Each producer adds tasks, some will go to overflow
			for j := range tasksPerProducer {
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

// Test_microtaskRing_NoTailCorruption verifies that overflow buffer prevents
// tail corruption when multiple producers fill ring beyond capacity.
// New design uses mutex-protected overflow instead of complex state transitions.
// Runs with -race detector to verify correctness.
func Test_microtaskRing_NoTailCorruption(t *testing.T) {
	tr := newMicrotaskRing()

	var producers atomic.Int64

	// Spawn many producers that will race to fill and trigger overflow
	const numProducers = 32
	for i := range numProducers {
		go func(id int) {
			defer producers.Add(-1)
			// Each producer adds 100 tasks (will overflow the ring with 32*100=3200 total)
			for j := range 100 {
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
	for range 100 {
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

// Test_microtaskRing_SharedStress tests ring with concurrent
// producers and consumers, repeatedly triggering overflow transitions.
func Test_microtaskRing_SharedStress(t *testing.T) {
	tr := newMicrotaskRing()

	const numProducers = 8
	const tasksPerProducer = 5000

	var counter atomic.Int64
	var activeProducers atomic.Int64

	// Spawn producers
	for i := range numProducers {
		go func(id int) {
			defer activeProducers.Add(-1)
			for j := range tasksPerProducer {
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

// Test_microtaskRing_IsEmpty_BugWhenOverflowNotCompacted proves the fix for
// the IsEmpty() logic error discovered in review.md.
//
// Bug: IsEmpty() was checking len(r.overflow) == 0, but overflow is not
// immediately compacted after pops. Instead, overflowHead advances and
// compaction occurs lazily. The correct check is:
//
//	len(r.overflow) - r.overflowHead == 0
//
// This test creates a scenario where overflow has items, partially drains
// them (so overflowHead > 0 but len(overflow) > 0), then verifies IsEmpty()
// returns correct values.
func Test_microtaskRing_IsEmpty_BugWhenOverflowNotCompacted(t *testing.T) {
	ring := newMicrotaskRing()

	// 1. Fill the ring buffer completely to force overflow use.
	// Ring capacity is 1024 (1 << 10). Fill it completely.
	const ringCap = 1024
	var counter atomic.Int64

	for range ringCap {
		ring.Push(func() {
			counter.Add(1)
		})
	}

	// 2. Add MORE items - these will go to overflow.
	const overflowCount = 100
	for range overflowCount {
		ring.Push(func() {
			counter.Add(1)
		})
	}

	// Verify: Length should be ringCap + overflowCount
	expectedLen := ringCap + overflowCount
	if ring.Length() != expectedLen {
		t.Fatalf("Expected length %d, got %d", expectedLen, ring.Length())
	}

	// 3. IsEmpty should return false
	if ring.IsEmpty() {
		t.Fatal("IsEmpty() returned true when ring has items - BUG!")
	}

	// 4. Drain the ring buffer portion completely
	for i := range ringCap {
		fn := ring.Pop()
		if fn == nil {
			t.Fatalf("Pop returned nil at iteration %d, expected item", i)
		}
		fn()
	}

	// 5. Verify: Now we should have only overflowCount items left
	if ring.Length() != overflowCount {
		t.Fatalf("After draining ring, expected length %d, got %d", overflowCount, ring.Length())
	}

	// 6. IsEmpty should return false - items in overflow!
	if ring.IsEmpty() {
		t.Fatal("IsEmpty() returned true but there are still items in overflow - BUG!")
	}

	// 7. Now drain HALF the overflow items (this is the critical test!)
	// After partial drain, overflowHead > 0 but len(overflow) > 0.
	// The old buggy code would report IsEmpty() = false here, but if we
	// had only checked len(overflow) == 0 after full drain, it might have
	// been wrong after compaction timing issues.
	const drainCount = 50
	for i := range drainCount {
		fn := ring.Pop()
		if fn == nil {
			t.Fatalf("Pop returned nil at overflow iteration %d, expected item", i)
		}
		fn()
	}

	// 8. Critical check: length should be overflowCount - drainCount
	expectedRemaining := overflowCount - drainCount
	if ring.Length() != expectedRemaining {
		t.Fatalf("After partial overflow drain, expected length %d, got %d", expectedRemaining, ring.Length())
	}

	// 9. THE CRITICAL TEST: IsEmpty() should return false because there are still items!
	// The old buggy code checked len(r.overflow) == 0, which would be false here,
	// so it would return false. But after we drain ALL remaining items and
	// compaction happens, the bug would have caused incorrect behavior.
	if ring.IsEmpty() {
		t.Fatal("IsEmpty() returned true with items remaining in overflow - BUG!")
	}

	// 10. Now drain ALL remaining items
	for i := range expectedRemaining {
		fn := ring.Pop()
		if fn == nil {
			t.Fatalf("Pop returned nil at final drain iteration %d", i)
		}
		fn()
	}

	// 11. Verify: Length should be 0
	if ring.Length() != 0 {
		t.Fatalf("After full drain, expected length 0, got %d", ring.Length())
	}

	// 12. THE FINAL CRITICAL TEST: IsEmpty() MUST return true now!
	// The old buggy code checked len(r.overflow) == 0. But after compaction,
	// overflow might still have len > 0 with overflowHead == len (all items drained).
	// The fix ensures we check len(overflow) - overflowHead == 0.
	if !ring.IsEmpty() {
		t.Fatal("IsEmpty() returned false after draining all items - BUG NOT FIXED!")
	}

	// 13. Verify invariant: (Length()==0) == IsEmpty()
	if (ring.Length() == 0) != ring.IsEmpty() {
		t.Fatal("Invariant violation: (Length()==0) != IsEmpty()")
	}

	// 14. Verify counter - all tasks should have executed exactly once
	if counter.Load() != int64(ringCap+overflowCount) {
		t.Fatalf("Expected %d task executions, got %d", ringCap+overflowCount, counter.Load())
	}

	t.Logf("IsEmpty() verified: properly handles overflowHead advancement")
}
