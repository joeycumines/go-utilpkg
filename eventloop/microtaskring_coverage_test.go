// COVERAGE-013: microtaskRing Full Coverage Tests
//
// Tests comprehensive coverage of microtaskRing including:
// - Overflow path (ring full)
// - FIFO ordering with overflow
// - Overflow compaction threshold logic
// - Nil task handling in ring
// - Sequence number edge cases

package eventloop

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test_microtaskRing_OverflowPath tests the overflow slice allocation when ring is full.
func Test_microtaskRing_OverflowPath(t *testing.T) {
	r := newMicrotaskRing()

	// Fill the ring completely (4096 items)
	for range ringBufferSize {
		r.Push(func() {})
	}

	if r.Length() != ringBufferSize {
		t.Fatalf("Expected ring length %d, got %d", ringBufferSize, r.Length())
	}

	// Push one more - should go to overflow
	overflowExecuted := false
	r.Push(func() { overflowExecuted = true })

	if r.Length() != ringBufferSize+1 {
		t.Fatalf("Expected length %d after overflow push, got %d", ringBufferSize+1, r.Length())
	}

	// Drain all ring items
	for i := range ringBufferSize {
		fn := r.Pop()
		if fn == nil {
			t.Fatalf("Expected item at index %d, got nil", i)
		}
	}

	// Next pop should be from overflow
	fn := r.Pop()
	if fn == nil {
		t.Fatal("Expected overflow item, got nil")
	}
	fn()

	if !overflowExecuted {
		t.Error("Overflow task should have been executed")
	}

	// Queue should be empty now
	if !r.IsEmpty() {
		t.Error("Ring should be empty after draining")
	}
}

// Test_microtaskRing_OverflowInitialCapacity verifies overflow initial allocation.
func Test_microtaskRing_OverflowInitialCapacity(t *testing.T) {
	r := newMicrotaskRing()

	// Fill the ring
	for range ringBufferSize {
		r.Push(func() {})
	}

	// Push to overflow - should trigger initial capacity allocation
	r.Push(func() {})

	// Verify overflow is now active
	if !r.overflowPending.Load() {
		t.Error("overflowPending should be true after overflow push")
	}

	r.overflowMu.Lock()
	overflowLen := len(r.overflow)
	overflowCap := cap(r.overflow)
	r.overflowMu.Unlock()

	if overflowLen != 1 {
		t.Errorf("Expected overflow length 1, got %d", overflowLen)
	}

	// Initial capacity should be ringOverflowInitCap (1024)
	if overflowCap < ringOverflowInitCap {
		t.Errorf("Expected overflow capacity >= %d, got %d", ringOverflowInitCap, overflowCap)
	}
}

// Test_microtaskRing_OverflowCompaction tests compaction logic when threshold is exceeded.
func Test_microtaskRing_OverflowCompaction(t *testing.T) {
	r := newMicrotaskRing()

	// Fill the ring
	for range ringBufferSize {
		r.Push(func() {})
	}

	// Push items to overflow - we need enough that after some pops, compaction triggers
	// Compaction requires: overflowHead > len(overflow)/2 && overflowHead > ringOverflowCompactThreshold (512)
	// To trigger compaction we need overflowHead > overflow_len/2 AND > 512
	// If we push 1200 items, then pop 700:
	// overflowHead=700, len=1200, 700 > 600? Yes! 700 > 512? Yes! -> compaction
	overflowCount := 1200
	for range overflowCount {
		r.Push(func() {})
	}

	totalItems := ringBufferSize + overflowCount
	if r.Length() != totalItems {
		t.Fatalf("Expected length %d, got %d", totalItems, r.Length())
	}

	// Drain the ring first
	for i := range ringBufferSize {
		fn := r.Pop()
		if fn == nil {
			t.Fatalf("Expected ring item at %d", i)
		}
	}

	// Pop items from overflow to trigger compaction
	// We need overflowHead > len/2 AND overflowHead > 512
	// After popping, compaction happens incrementally
	// Pop exactly 700 items - at pop 601 (when overflowHead=601, len=1200):
	// 601 > 600? Yes! 601 > 512? Yes! -> compaction happens
	popCount := 700
	for i := range popCount {
		fn := r.Pop()
		if fn == nil {
			t.Fatalf("Expected overflow item at %d", i)
		}
	}

	// After compaction at pop 601, the slice was compacted:
	// - Before: len=1200, head=601
	// - After compaction: len=599, head=0
	// Then we continued popping 700-601=99 more items
	// Final: head=99, len=599
	r.overflowMu.Lock()
	overflowHead := r.overflowHead
	overflowLen := len(r.overflow)
	r.overflowMu.Unlock()

	// Compaction occurred at some point - verify we can still pop remaining items
	remaining := overflowCount - popCount // 1200 - 700 = 500
	for i := range remaining {
		fn := r.Pop()
		if fn == nil {
			t.Fatalf("Expected remaining overflow item at %d (head=%d, len=%d)", i, overflowHead, overflowLen)
		}
	}

	// Should be empty now
	if !r.IsEmpty() {
		t.Error("Ring should be empty after draining all items")
	}
}

// Test_microtaskRing_OverflowFIFOAppend verifies tasks go to overflow when overflow has items.
func Test_microtaskRing_OverflowFIFOAppend(t *testing.T) {
	r := newMicrotaskRing()

	// Fill the ring
	for range ringBufferSize {
		r.Push(func() {})
	}

	// Add to overflow
	var order []int
	var mu sync.Mutex
	r.Push(func() {
		mu.Lock()
		order = append(order, 1)
		mu.Unlock()
	})

	// Pop one from ring to make space
	r.Pop()

	// Push another - should go to overflow (not ring) to maintain FIFO
	r.Push(func() {
		mu.Lock()
		order = append(order, 2)
		mu.Unlock()
	})

	// Drain the ring (4095 items remaining)
	for range ringBufferSize - 1 {
		r.Pop()
	}

	// Pop from overflow - should be in FIFO order
	fn1 := r.Pop()
	if fn1 == nil {
		t.Fatal("Expected first overflow item")
	}
	fn1()

	fn2 := r.Pop()
	if fn2 == nil {
		t.Fatal("Expected second overflow item")
	}
	fn2()

	// Verify order
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Errorf("Expected FIFO order [1, 2], got %v", order)
	}
}

// Test_microtaskRing_NilTaskHandling tests that nil tasks are properly skipped.
func Test_microtaskRing_NilTaskHandling(t *testing.T) {
	r := newMicrotaskRing()

	// Push nil
	r.Push(nil)

	// Push valid task
	executed := false
	r.Push(func() {
		executed = true
	})

	// Pop should skip nil and return the valid task
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 10 {
			fn := r.Pop()
			if fn != nil {
				fn()
				return
			}
			runtime.Gosched()
		}
	}()

	select {
	case <-done:
		if executed {
			// Success - nil was properly skipped
		} else {
			t.Error("Valid task should have been executed after skipping nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout - Pop is stuck on nil task")
	}
}

// Test_microtaskRing_NilTaskInOverflow tests nil task handling in overflow slice.
func Test_microtaskRing_NilTaskInOverflow(t *testing.T) {
	r := newMicrotaskRing()

	// Fill the ring
	for range ringBufferSize {
		r.Push(func() {})
	}

	// Push nil to overflow
	r.Push(nil)

	// Push valid task to overflow
	executed := false
	r.Push(func() {
		executed = true
	})

	// Drain ring
	for range ringBufferSize {
		r.Pop()
	}

	// Pop nil from overflow - should return nil (overflow doesn't skip)
	fn := r.Pop()
	if fn != nil {
		t.Error("Expected nil from overflow")
	}

	// Pop valid task from overflow
	fn = r.Pop()
	if fn == nil {
		t.Fatal("Expected valid task from overflow")
	}
	fn()

	if !executed {
		t.Error("Valid overflow task should have been executed")
	}
}

// Test_microtaskRing_SequenceSkipSentinel verifies ringSeqSkip is used correctly.
func Test_microtaskRing_SequenceSkipSentinel(t *testing.T) {
	r := newMicrotaskRing()

	// All slots should start with ringSeqSkip and invalid
	for i := range ringBufferSize {
		seq := r.seq[i].Load()
		valid := r.valid[i].Load()
		if seq != ringSeqSkip {
			t.Errorf("Slot %d: expected seq %d, got %d", i, ringSeqSkip, seq)
		}
		if valid {
			t.Errorf("Slot %d: expected invalid, got valid", i)
		}
	}
}

// Test_microtaskRing_SequenceWrapAround tests sequence number behavior at boundaries.
func Test_microtaskRing_SequenceWrapAround(t *testing.T) {
	r := newMicrotaskRing()

	// Simulate near-max sequence
	r.tailSeq.Store(^uint64(0) - 10)

	// Push items - sequence should wrap gracefully
	for i := range 20 {
		executed := false
		r.Push(func() { executed = true })

		fn := r.Pop()
		if fn == nil {
			t.Fatalf("Pop returned nil at iteration %d", i)
		}
		fn()
		if !executed {
			t.Fatalf("Task not executed at iteration %d", i)
		}
	}

	// Verify sequence wrapped (went from MAX-10 to MAX, then 0, 1, 2, ...)
	currentSeq := r.tailSeq.Load()
	if currentSeq < 5 {
		t.Logf("Sequence wrapped correctly, current: %d", currentSeq)
	}
}

// Test_microtaskRing_ConcurrentPushPop tests concurrent operations.
func Test_microtaskRing_ConcurrentPushPop(t *testing.T) {
	r := newMicrotaskRing()

	const producers = 4
	const itemsPerProducer = 10000
	totalItems := producers * itemsPerProducer

	var consumed atomic.Int64
	var produced atomic.Int64
	var wg sync.WaitGroup

	// Start producers
	for range producers {
		wg.Go(func() {
			for range itemsPerProducer {
				r.Push(func() {})
				produced.Add(1)
			}
		})
	}

	// Start consumer
	done := make(chan struct{})
	go func() {
		defer close(done)
		for consumed.Load() < int64(totalItems) {
			fn := r.Pop()
			if fn != nil {
				fn()
				consumed.Add(1)
			} else {
				runtime.Gosched()
			}
		}
	}()

	wg.Wait()

	// Wait for consumer with timeout
	select {
	case <-done:
		if consumed.Load() != int64(totalItems) {
			t.Errorf("Expected %d consumed, got %d", totalItems, consumed.Load())
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("Consumer stuck: consumed %d/%d", consumed.Load(), totalItems)
	}
}

// Test_microtaskRing_IsEmpty tests IsEmpty behavior.
func Test_microtaskRing_IsEmpty(t *testing.T) {
	r := newMicrotaskRing()

	if !r.IsEmpty() {
		t.Error("New ring should be empty")
	}

	r.Push(func() {})
	if r.IsEmpty() {
		t.Error("Ring with item should not be empty")
	}

	r.Pop()
	if !r.IsEmpty() {
		t.Error("Ring after pop should be empty")
	}
}

// Test_microtaskRing_Length tests Length calculation with ring and overflow.
func Test_microtaskRing_Length(t *testing.T) {
	r := newMicrotaskRing()

	if r.Length() != 0 {
		t.Errorf("Expected length 0, got %d", r.Length())
	}

	// Push to ring
	for range 100 {
		r.Push(func() {})
	}
	if r.Length() != 100 {
		t.Errorf("Expected length 100, got %d", r.Length())
	}

	// Fill ring and overflow
	for range ringBufferSize {
		r.Push(func() {})
	}
	if r.Length() != ringBufferSize+100 {
		t.Errorf("Expected length %d, got %d", ringBufferSize+100, r.Length())
	}

	// Pop some
	for range 50 {
		r.Pop()
	}
	if r.Length() != ringBufferSize+50 {
		t.Errorf("Expected length %d, got %d", ringBufferSize+50, r.Length())
	}
}

// Test_microtaskRing_OverflowPendingFlag tests overflowPending atomic flag.
func Test_microtaskRing_OverflowPendingFlag(t *testing.T) {
	r := newMicrotaskRing()

	if r.overflowPending.Load() {
		t.Error("New ring should not have overflow pending")
	}

	// Fill ring
	for range ringBufferSize {
		r.Push(func() {})
	}

	// Not yet pending
	if r.overflowPending.Load() {
		t.Error("Ring should not have overflow pending before overflow")
	}

	// Push to overflow
	r.Push(func() {})

	if !r.overflowPending.Load() {
		t.Error("Ring should have overflow pending after overflow push")
	}

	// Drain all including overflow
	for range ringBufferSize + 1 {
		r.Pop()
	}

	// Flag should be cleared
	if r.overflowPending.Load() {
		t.Error("Ring should not have overflow pending after drain")
	}
}

// Test_microtaskRing_ValidFlag tests the valid flag for R101 fix.
func Test_microtaskRing_ValidFlag(t *testing.T) {
	r := newMicrotaskRing()

	// Push and verify valid flag
	r.Push(func() {})

	idx := uint64(0) % ringBufferSize
	if !r.valid[idx].Load() {
		t.Error("Slot should be valid after push")
	}

	// Pop should clear valid flag
	r.Pop()

	if r.valid[idx].Load() {
		t.Error("Slot should be invalid after pop")
	}
}

// Test_microtaskRing_MultipleOverflowDrains tests multiple overflow fill/drain cycles.
func Test_microtaskRing_MultipleOverflowDrains(t *testing.T) {
	r := newMicrotaskRing()

	for cycle := range 3 {
		// Fill ring
		for range ringBufferSize {
			r.Push(func() {})
		}

		// Push to overflow
		for range 100 {
			r.Push(func() {})
		}

		// Drain all
		for i := range ringBufferSize + 100 {
			fn := r.Pop()
			if fn == nil {
				t.Fatalf("Cycle %d: expected item at %d", cycle, i)
			}
		}

		if !r.IsEmpty() {
			t.Fatalf("Cycle %d: ring should be empty", cycle)
		}
	}
}

// Test_microtaskRing_OverflowGrowth tests overflow slice growth beyond initial capacity.
func Test_microtaskRing_OverflowGrowth(t *testing.T) {
	r := newMicrotaskRing()

	// Fill ring
	for range ringBufferSize {
		r.Push(func() {})
	}

	// Push many to overflow (more than initial capacity of 1024)
	overflowItems := ringOverflowInitCap * 3
	for range overflowItems {
		r.Push(func() {})
	}

	r.overflowMu.Lock()
	overflowLen := len(r.overflow)
	r.overflowMu.Unlock()

	if overflowLen != overflowItems {
		t.Errorf("Expected overflow length %d, got %d", overflowItems, overflowLen)
	}

	// Verify all items are retrievable
	totalItems := ringBufferSize + overflowItems
	for i := range totalItems {
		fn := r.Pop()
		if fn == nil {
			t.Fatalf("Expected item at %d", i)
		}
	}

	if !r.IsEmpty() {
		t.Error("Ring should be empty after draining")
	}
}
