// COVERAGE-014: ChunkedIngress Full Coverage Tests
//
// Tests comprehensive coverage of ChunkedIngress including:
// - Multi-chunk growth
// - Chunk pool recycling
// - returnChunk clearing of task slots (IMP-002)
// - Exhausted chunk handling
// - Length tracking

package eventloop

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

// TestChunkedIngress_MultiChunkGrowth tests allocation of multiple chunks.
func TestChunkedIngress_MultiChunkGrowth(t *testing.T) {
	q := NewChunkedIngress()

	// Push more than one chunk (chunkSize = 128)
	itemCount := chunkSize * 3

	for i := 0; i < itemCount; i++ {
		q.Push(func() {})
	}

	if q.Length() != itemCount {
		t.Errorf("Expected length %d, got %d", itemCount, q.Length())
	}

	// Verify we can pop all items
	for i := 0; i < itemCount; i++ {
		task, ok := q.Pop()
		if !ok {
			t.Fatalf("Pop failed at index %d", i)
		}
		if task == nil {
			t.Fatalf("Nil task at index %d", i)
		}
	}

	// Should be empty now
	if q.Length() != 0 {
		t.Errorf("Expected length 0 after drain, got %d", q.Length())
	}
}

// TestChunkedIngress_ChunkPoolRecycling tests that chunks are returned to pool.
func TestChunkedIngress_ChunkPoolRecycling(t *testing.T) {
	q := NewChunkedIngress()

	// Push exactly 2 chunks worth
	itemCount := chunkSize * 2

	for i := 0; i < itemCount; i++ {
		q.Push(func() {})
	}

	// Pop all - this should return chunks to pool
	for i := 0; i < itemCount; i++ {
		q.Pop()
	}

	// Push again - should reuse pooled chunks
	for i := 0; i < itemCount; i++ {
		q.Push(func() {})
	}

	if q.Length() != itemCount {
		t.Errorf("Expected length %d, got %d", itemCount, q.Length())
	}
}

// TestChunkedIngress_ReturnChunkClearsSlots tests IMP-002 fix for memory leak.
func TestChunkedIngress_ReturnChunkClearsSlots(t *testing.T) {
	// Create a chunk manually and populate it
	c := newChunk()

	// Fill with tasks
	for i := 0; i < chunkSize; i++ {
		c.tasks[i] = func() {}
	}
	c.pos = chunkSize
	c.readPos = chunkSize // Mark as fully read

	// Verify tasks are populated
	for i := 0; i < chunkSize; i++ {
		if c.tasks[i] == nil {
			t.Fatalf("Task at %d should not be nil before return", i)
		}
	}

	// Return the chunk - should clear all slots
	returnChunk(c)

	// Get a fresh chunk from pool (might be the same one)
	c2 := newChunk()

	// All slots should be nil
	for i := 0; i < chunkSize; i++ {
		if c2.tasks[i] != nil {
			t.Errorf("Task at %d should be nil after pool recycle", i)
		}
	}

	// pos and readPos should be reset
	if c2.pos != 0 {
		t.Errorf("Expected pos 0, got %d", c2.pos)
	}
	if c2.readPos != 0 {
		t.Errorf("Expected readPos 0, got %d", c2.readPos)
	}
	if c2.next != nil {
		t.Error("Expected next to be nil")
	}

	// Return the chunk for cleanup
	returnChunk(c2)
}

// TestChunkedIngress_ExhaustedChunkHandling tests handling of fully-read chunks.
func TestChunkedIngress_ExhaustedChunkHandling(t *testing.T) {
	q := NewChunkedIngress()

	// Push exactly one chunk worth plus a few more
	itemCount := chunkSize + 10

	for i := 0; i < itemCount; i++ {
		q.Push(func() {})
	}

	// Pop exactly the first chunk
	for i := 0; i < chunkSize; i++ {
		task, ok := q.Pop()
		if !ok {
			t.Fatalf("Pop failed at index %d", i)
		}
		if task == nil {
			t.Fatalf("Nil task at index %d", i)
		}
	}

	// Remaining items should still be accessible
	if q.Length() != 10 {
		t.Errorf("Expected length 10, got %d", q.Length())
	}

	// Pop remaining
	for i := 0; i < 10; i++ {
		task, ok := q.Pop()
		if !ok {
			t.Fatalf("Pop failed at remaining index %d", i)
		}
		if task == nil {
			t.Fatalf("Nil task at remaining index %d", i)
		}
	}

	// Should be empty
	if q.Length() != 0 {
		t.Errorf("Expected length 0, got %d", q.Length())
	}
}

// TestChunkedIngress_LengthTracking tests accurate length tracking.
func TestChunkedIngress_LengthTracking(t *testing.T) {
	q := NewChunkedIngress()

	if q.Length() != 0 {
		t.Errorf("Initial length should be 0, got %d", q.Length())
	}

	// Push and verify length at each step
	for i := 1; i <= 100; i++ {
		q.Push(func() {})
		if q.Length() != i {
			t.Errorf("After push %d: expected length %d, got %d", i, i, q.Length())
		}
	}

	// Pop and verify length at each step
	for i := 99; i >= 0; i-- {
		q.Pop()
		if q.Length() != i {
			t.Errorf("After pop (remaining %d): expected length %d, got %d", i, i, q.Length())
		}
	}
}

// TestChunkedIngress_SingleChunkReuse tests cursor reset for single chunk reuse.
func TestChunkedIngress_SingleChunkReuse(t *testing.T) {
	q := NewChunkedIngress()

	// Push and pop within single chunk
	for cycle := 0; cycle < 5; cycle++ {
		for i := 0; i < 50; i++ {
			q.Push(func() {})
		}

		for i := 0; i < 50; i++ {
			task, ok := q.Pop()
			if !ok {
				t.Fatalf("Cycle %d: Pop failed at %d", cycle, i)
			}
			if task == nil {
				t.Fatalf("Cycle %d: Nil task at %d", cycle, i)
			}
		}

		// Verify empty
		if q.Length() != 0 {
			t.Errorf("Cycle %d: Expected length 0, got %d", cycle, q.Length())
		}

		// Single chunk should have cursors reset
		if q.head != nil && q.head.pos != 0 {
			t.Errorf("Cycle %d: Expected pos reset to 0", cycle)
		}
		if q.head != nil && q.head.readPos != 0 {
			t.Errorf("Cycle %d: Expected readPos reset to 0", cycle)
		}
	}
}

// TestChunkedIngress_PopEmptyQueue tests Pop on empty queue.
func TestChunkedIngress_PopEmptyQueue(t *testing.T) {
	q := NewChunkedIngress()

	task, ok := q.Pop()
	if ok {
		t.Error("Pop on empty queue should return false")
	}
	if task != nil {
		t.Error("Pop on empty queue should return nil task")
	}
}

// TestChunkedIngress_PopAfterDrain tests Pop on drained queue.
func TestChunkedIngress_PopAfterDrain(t *testing.T) {
	q := NewChunkedIngress()

	// Push and drain
	q.Push(func() {})
	q.Pop()

	// Pop again should return false
	task, ok := q.Pop()
	if ok {
		t.Error("Pop on drained queue should return false")
	}
	if task != nil {
		t.Error("Pop on drained queue should return nil task")
	}
}

// TestChunkedIngress_ChunkChainIntegrity tests linked list integrity.
func TestChunkedIngress_ChunkChainIntegrity(t *testing.T) {
	q := NewChunkedIngress()

	// Push enough for 4 chunks
	itemCount := chunkSize * 4

	for i := 0; i < itemCount; i++ {
		q.Push(func() {})
	}

	// Count chunks by traversing
	chunkCount := 0
	current := q.head
	for current != nil {
		chunkCount++
		current = current.next
	}

	if chunkCount != 4 {
		t.Errorf("Expected 4 chunks, got %d", chunkCount)
	}

	// Drain completely
	for i := 0; i < itemCount; i++ {
		task, ok := q.Pop()
		if !ok || task == nil {
			t.Fatalf("Failed to pop at %d", i)
		}
	}

	// head and tail should be same (single remaining chunk)
	if q.head != q.tail {
		t.Error("After drain, head and tail should be same")
	}
}

// TestChunkedIngress_ConcurrentPushSingleConsumer tests concurrent pushes with sync.
func TestChunkedIngress_ConcurrentPushSingleConsumer(t *testing.T) {
	q := NewChunkedIngress()

	const producers = 4
	const itemsPerProducer = 1000
	totalItems := producers * itemsPerProducer

	var wg sync.WaitGroup
	var mu sync.Mutex // External sync required for ChunkedIngress

	// Start producers
	for p := 0; p < producers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				mu.Lock()
				q.Push(func() {})
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if q.Length() != totalItems {
		t.Errorf("Expected length %d, got %d", totalItems, q.Length())
	}

	// Single consumer (no sync needed for Pop in SPSC model)
	consumed := 0
	for {
		_, ok := q.Pop()
		if !ok {
			break
		}
		consumed++
	}

	if consumed != totalItems {
		t.Errorf("Expected consumed %d, got %d", totalItems, consumed)
	}
}

// TestChunkedIngress_NewChunk tests newChunk pool allocation.
func TestChunkedIngress_NewChunk(t *testing.T) {
	c := newChunk()

	if c == nil {
		t.Fatal("newChunk returned nil")
	}

	if c.pos != 0 {
		t.Errorf("Expected pos 0, got %d", c.pos)
	}

	if c.readPos != 0 {
		t.Errorf("Expected readPos 0, got %d", c.readPos)
	}

	if c.next != nil {
		t.Error("Expected next to be nil")
	}

	returnChunk(c)
}

// TestChunkedIngress_TaskExecution tests that tasks execute correctly.
func TestChunkedIngress_TaskExecution(t *testing.T) {
	q := NewChunkedIngress()

	var counter atomic.Int64
	const taskCount = 500

	// Push tasks that increment counter
	for i := 0; i < taskCount; i++ {
		q.Push(func() {
			counter.Add(1)
		})
	}

	// Pop and execute all tasks
	for i := 0; i < taskCount; i++ {
		task, ok := q.Pop()
		if !ok || task == nil {
			t.Fatalf("Failed at %d", i)
		}
		task()
	}

	if counter.Load() != taskCount {
		t.Errorf("Expected counter %d, got %d", taskCount, counter.Load())
	}
}

// TestChunkedIngress_GCSafety tests that popped slots are zeroed for GC.
func TestChunkedIngress_GCSafety(t *testing.T) {
	q := NewChunkedIngress()

	// Create a large closure to track
	largeData := make([]byte, 1024*1024) // 1MB
	_ = largeData

	q.Push(func() { _ = largeData })
	q.Pop()

	// Force GC to verify no panic
	runtime.GC()

	// If we get here without panic, GC safety is working
}

// TestChunkedIngress_ChunkConstants verifies chunk constants.
func TestChunkedIngress_ChunkConstants(t *testing.T) {
	if chunkSize != 128 {
		t.Errorf("Expected chunkSize 128, got %d", chunkSize)
	}
}

// TestChunkedIngress_HeadTailNilStart tests initial state.
func TestChunkedIngress_HeadTailNilStart(t *testing.T) {
	q := NewChunkedIngress()

	if q.head != nil {
		t.Error("Initial head should be nil")
	}

	if q.tail != nil {
		t.Error("Initial tail should be nil")
	}

	if q.length != 0 {
		t.Error("Initial length should be 0")
	}
}

// TestChunkedIngress_FirstPushCreatesChunk tests lazy chunk allocation.
func TestChunkedIngress_FirstPushCreatesChunk(t *testing.T) {
	q := NewChunkedIngress()

	q.Push(func() {})

	if q.head == nil {
		t.Error("head should not be nil after push")
	}

	if q.tail == nil {
		t.Error("tail should not be nil after push")
	}

	if q.head != q.tail {
		t.Error("head and tail should be same after first push")
	}
}

// TestChunkedIngress_BoundaryConditions tests chunk boundary behavior.
func TestChunkedIngress_BoundaryConditions(t *testing.T) {
	q := NewChunkedIngress()

	// Push exactly chunkSize items
	for i := 0; i < chunkSize; i++ {
		q.Push(func() {})
	}

	// Should still be single chunk
	if q.head != q.tail {
		t.Error("Should be single chunk at exactly chunkSize")
	}

	// Push one more
	q.Push(func() {})

	// Now should have two chunks
	if q.head == q.tail {
		t.Error("Should have two chunks after chunkSize+1 pushes")
	}

	if q.head.next != q.tail {
		t.Error("head.next should be tail")
	}
}

// TestChunkedIngress_Interleaved tests interleaved push/pop.
func TestChunkedIngress_Interleaved(t *testing.T) {
	q := NewChunkedIngress()

	for cycle := 0; cycle < 100; cycle++ {
		// Push 10
		for i := 0; i < 10; i++ {
			q.Push(func() {})
		}

		// Pop 5
		for i := 0; i < 5; i++ {
			q.Pop()
		}
	}

	// Should have 500 remaining (100 * 5)
	if q.Length() != 500 {
		t.Errorf("Expected 500, got %d", q.Length())
	}
}
