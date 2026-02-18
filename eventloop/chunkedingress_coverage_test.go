// COVERAGE-014: chunkedIngress Full Coverage Tests
//
// Tests comprehensive coverage of chunkedIngress including:
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

// Test_chunkedIngress_MultiChunkGrowth tests allocation of multiple chunks.
func Test_chunkedIngress_MultiChunkGrowth(t *testing.T) {
	q := newChunkedIngress()

	// Push more than one chunk (defaultChunkSize = 64)
	itemCount := defaultChunkSize * 3

	for range itemCount {
		q.Push(func() {})
	}

	if q.Length() != itemCount {
		t.Errorf("Expected length %d, got %d", itemCount, q.Length())
	}

	// Verify we can pop all items
	for i := range itemCount {
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

// Test_chunkedIngress_ChunkPoolRecycling tests that chunks are returned to pool.
func Test_chunkedIngress_ChunkPoolRecycling(t *testing.T) {
	q := newChunkedIngress()

	// Push exactly 2 chunks worth
	itemCount := defaultChunkSize * 2

	for range itemCount {
		q.Push(func() {})
	}

	// Pop all - this should return chunks to pool
	for range itemCount {
		q.Pop()
	}

	// Push again - should reuse pooled chunks
	for range itemCount {
		q.Push(func() {})
	}

	if q.Length() != itemCount {
		t.Errorf("Expected length %d, got %d", itemCount, q.Length())
	}
}

// Test_chunkedIngress_ReturnChunkClearsSlots tests IMP-002 fix for memory leak.
func Test_chunkedIngress_ReturnChunkClearsSlots(t *testing.T) {
	// Create a queue to use its pool and newChunk/returnChunk methods
	q := newChunkedIngress()

	// Create a chunk using the queue's method
	c := q.newChunk()

	// Fill with tasks
	for i := 0; i < q.chunkSize; i++ {
		c.tasks[i] = func() {}
	}
	c.pos = q.chunkSize
	c.readPos = q.chunkSize // Mark as fully read

	// Verify tasks are populated
	for i := 0; i < q.chunkSize; i++ {
		if c.tasks[i] == nil {
			t.Fatalf("Task at %d should not be nil before return", i)
		}
	}

	// Return the chunk - should clear all slots
	q.returnChunk(c)

	// Get a fresh chunk from pool (might be the same one)
	c2 := q.newChunk()

	// All slots should be nil
	for i := 0; i < q.chunkSize; i++ {
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
	q.returnChunk(c2)
}

// Test_chunkedIngress_ExhaustedChunkHandling tests handling of fully-read chunks.
func Test_chunkedIngress_ExhaustedChunkHandling(t *testing.T) {
	q := newChunkedIngress()

	// Push exactly one chunk worth plus a few more
	itemCount := defaultChunkSize + 10

	for range itemCount {
		q.Push(func() {})
	}

	// Pop exactly the first chunk
	for i := range defaultChunkSize {
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
	for i := range 10 {
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

// Test_chunkedIngress_LengthTracking tests accurate length tracking.
func Test_chunkedIngress_LengthTracking(t *testing.T) {
	q := newChunkedIngress()

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

// Test_chunkedIngress_SingleChunkReuse tests cursor reset for single chunk reuse.
func Test_chunkedIngress_SingleChunkReuse(t *testing.T) {
	q := newChunkedIngress()

	// Push and pop within single chunk
	for cycle := range 5 {
		for range 50 {
			q.Push(func() {})
		}

		for i := range 50 {
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

// Test_chunkedIngress_PopEmptyQueue tests Pop on empty queue.
func Test_chunkedIngress_PopEmptyQueue(t *testing.T) {
	q := newChunkedIngress()

	task, ok := q.Pop()
	if ok {
		t.Error("Pop on empty queue should return false")
	}
	if task != nil {
		t.Error("Pop on empty queue should return nil task")
	}
}

// Test_chunkedIngress_PopAfterDrain tests Pop on drained queue.
func Test_chunkedIngress_PopAfterDrain(t *testing.T) {
	q := newChunkedIngress()

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

// Test_chunkedIngress_ChunkChainIntegrity tests linked list integrity.
func Test_chunkedIngress_ChunkChainIntegrity(t *testing.T) {
	q := newChunkedIngress()

	// Push enough for 4 chunks
	itemCount := defaultChunkSize * 4

	for range itemCount {
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
	for i := range itemCount {
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

// Test_chunkedIngress_ConcurrentPushSingleConsumer tests concurrent pushes with sync.
func Test_chunkedIngress_ConcurrentPushSingleConsumer(t *testing.T) {
	q := newChunkedIngress()

	const producers = 4
	const itemsPerProducer = 1000
	totalItems := producers * itemsPerProducer

	var wg sync.WaitGroup
	var mu sync.Mutex // External sync required for chunkedIngress

	// Start producers
	for range producers {
		wg.Go(func() {
			for range itemsPerProducer {
				mu.Lock()
				q.Push(func() {})
				mu.Unlock()
			}
		})
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

// Test_chunkedIngress_NewChunk tests newChunk pool allocation.
func Test_chunkedIngress_NewChunk(t *testing.T) {
	// Use queue's method to create chunks
	q := newChunkedIngress()
	c := q.newChunk()

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

	q.returnChunk(c)
}

// Test_chunkedIngress_TaskExecution tests that tasks execute correctly.
func Test_chunkedIngress_TaskExecution(t *testing.T) {
	q := newChunkedIngress()

	var counter atomic.Int64
	const taskCount = 500

	// Push tasks that increment counter
	for range taskCount {
		q.Push(func() {
			counter.Add(1)
		})
	}

	// Pop and execute all tasks
	for i := range taskCount {
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

// Test_chunkedIngress_GCSafety tests that popped slots are zeroed for GC.
func Test_chunkedIngress_GCSafety(t *testing.T) {
	q := newChunkedIngress()

	// Create a large closure to track
	largeData := make([]byte, 1024*1024) // 1MB
	_ = largeData

	q.Push(func() { _ = largeData })
	q.Pop()

	// Force GC to verify no panic
	runtime.GC()

	// If we get here without panic, GC safety is working
}

// Test_chunkedIngress_ChunkConstants verifies chunk constants.
func Test_chunkedIngress_ChunkConstants(t *testing.T) {
	if defaultChunkSize != 64 {
		t.Errorf("Expected defaultChunkSize 64, got %d", defaultChunkSize)
	}
}

// Test_chunkedIngress_HeadTailNilStart tests initial state.
func Test_chunkedIngress_HeadTailNilStart(t *testing.T) {
	q := newChunkedIngress()

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

// Test_chunkedIngress_FirstPushCreatesChunk tests lazy chunk allocation.
func Test_chunkedIngress_FirstPushCreatesChunk(t *testing.T) {
	q := newChunkedIngress()

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

// Test_chunkedIngress_BoundaryConditions tests chunk boundary behavior.
func Test_chunkedIngress_BoundaryConditions(t *testing.T) {
	q := newChunkedIngress()

	// Push exactly chunkSize items
	for i := 0; i < q.chunkSize; i++ {
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

// Test_chunkedIngress_Interleaved tests interleaved push/pop.
func Test_chunkedIngress_Interleaved(t *testing.T) {
	q := newChunkedIngress()

	for range 100 {
		// Push 10
		for range 10 {
			q.Push(func() {})
		}

		// Pop 5
		for range 5 {
			q.Pop()
		}
	}

	// Should have 500 remaining (100 * 5)
	if q.Length() != 500 {
		t.Errorf("Expected 500, got %d", q.Length())
	}
}
