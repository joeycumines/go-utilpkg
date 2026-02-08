//go:build linux || darwin

package alternatethree

import (
	"sync"
	"testing"
)

// TestIngressPop_MultiChunkEdgeCases tests the multi-chunk scenarios where
// chunks are returned to pool and head advances.
func TestIngressPop_MultiChunkEdgeCases(t *testing.T) {
	// t.Parallel() // Cannot parallel: modifies shared loop fields

	// Create a loop to access ingress queue
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	// Test 1: Push enough tasks to force multiple chunks
	// Chunk size is 128 tasks
	numTasks := 256 // Force at least 2 chunks
	for i := 0; i < numTasks; i++ {
		val := i
		loop.Submit(func() {
			// Task will verify we received correct value
			t.Logf("Task %d executed from multi-chunk queue", val)
		})
	}

	loop.internalMu.Lock()
	initialLength := loop.ingress.Length()
	loop.internalMu.Unlock()

	if initialLength != numTasks {
		t.Errorf("Expected queue length %d, got %d", numTasks, initialLength)
	}

	// Drain the queue and verify all tasks are executed
	executed := 0
	for i := 0; i < numTasks; i++ {
		loop.internalMu.Lock()
		task, ok := loop.ingress.popLocked()
		loop.internalMu.Unlock()

		if !ok {
			t.Errorf("popLocked() failed at index %d", i)
			break
		}

		task.Runnable()
		executed++
	}

	if executed != numTasks {
		t.Errorf("Expected %d executed tasks, got %d", numTasks, executed)
	}

	// After draining, queue should be empty
	loop.internalMu.Lock()
	finalLength := loop.ingress.Length()
	loop.internalMu.Unlock()

	if finalLength != 0 {
		t.Errorf("Expected empty queue after draining, got length %d", finalLength)
	}
}

// TestIngressPop_ResetCursorsOnEmpty tests that cursors are reset when queue becomes empty.
func TestIngressPop_ResetCursorsOnEmpty(t *testing.T) {
	// t.Parallel() // Cannot parallel: modifies shared loop fields

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	// Push and pop exactly one task - should reset cursors
	executed := false

	loop.Submit(func() {
		executed = true
	})

	loop.internalMu.Lock()
	task, ok := loop.ingress.popLocked()
	loop.internalMu.Unlock()

	if !ok {
		t.Fatal("popLocked() failed for single task")
	}

	task.Runnable()

	if !executed {
		t.Error("Task was not executed")
	}

	// Verify queue is empty and has reusable chunk (not nil)
	loop.internalMu.Lock()
	isEmpty := loop.ingress.Length() == 0
	hasChunk := loop.ingress.head != nil && loop.ingress.tail != nil
	loop.internalMu.Unlock()

	if !isEmpty {
		t.Error("Queue should be empty after popping all tasks")
	}

	if !hasChunk {
		t.Error("Queue should retain a reusable chunk when empty")
	}
}

// TestIngressPop_DoubleCheckEdgeCase tests the double-check scenario after chunk advancement.
// This covers the path at ingress.go:129-131 where we check readPos >= pos again.
func TestIngressPop_DoubleCheckEdgeCase(t *testing.T) {
	// t.Parallel() // Cannot parallel: modifies shared loop fields

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	// This test simulates the edge case where:
	// 1. We have multiple chunks
	// 2. Second chunk becomes exhausted
	// 3. We advance to third chunk but need to double-check

	// Push enough tasks to create 3 chunks
	numTasks := 300 // 128 + 128 + 44 tasks
	for i := 0; i < numTasks; i++ {
		val := i
		loop.Submit(func() {
			t.Logf("Executing task %d", val)
		})
	}

	// Now drain all tasks, exercising the double-check path
	executedCount := 0
	for executedCount < numTasks {
		loop.internalMu.Lock()
		task, ok := loop.ingress.popLocked()
		loop.internalMu.Unlock()

		if !ok {
			t.Errorf("popLocked() failed prematurely at count %d", executedCount)
			break
		}

		task.Runnable()
		executedCount++
	}

	if executedCount != numTasks {
		t.Errorf("Expected %d executed tasks, got %d", numTasks, executedCount)
	}
}

// TestIngressPop_ChunkReturnToPool tests that exhausted chunks are properly returned to pool.
func TestIngressPop_ChunkReturnToPool(t *testing.T) {
	// t.Parallel() // Cannot parallel: accesses shared chunkPool

	// Get initial pool stats - we can't directly get them, but we can observe behavior
	// by forcing chunk churn

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	// Create scenarios that will cause chunks to be returned to pool
	batchSize := 256 // 2 chunks
	numBatches := 2

	for batch := 0; batch < numBatches; batch++ {
		// Push a batch of tasks
		for i := 0; i < batchSize; i++ {
			val := i
			loop.Submit(func() {
				// Process task
				_ = val
			})
		}

		// Drain the batch - should return chunks to pool
		for i := 0; i < batchSize; i++ {
			loop.internalMu.Lock()
			task, ok := loop.ingress.popLocked()
			loop.internalMu.Unlock()

			if !ok {
				t.Fatalf("popLocked() failed at batch %d, index %d", batch, i)
			}
			task.Runnable()
		}

		// Queue should be empty but have a reusable chunk
		loop.internalMu.Lock()
		length := loop.ingress.Length()
		hasChunk := loop.ingress.head != nil
		loop.internalMu.Unlock()

		if length != 0 {
			t.Errorf("Expected empty queue after batch %d, got length %d", batch, length)
		}

		if !hasChunk {
			t.Errorf("Expected queue to have reusable chunk after batch %d", batch)
		}
	}
}

// TestIngressPop_ConcurrentSafeWithExternalLock tests that popLocked is safe
// when used with external mutex (as the loop does).
func TestIngressPop_ConcurrentSafeWithExternalLock(t *testing.T) {
	// t.Parallel() // Cannot parallel: modifies shared loop fields

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	numProducers := 4
	numTasks := 64
	var wg sync.WaitGroup

	// Producer goroutines pushing tasks
	for i := 0; i < numProducers; i++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			for j := 0; j < numTasks; j++ {
				val := producerID*numTasks + j
				loop.Submit(func() {
					// Task
					_ = val
				})
			}
		}(i)
	}

	wg.Wait()

	// Consumer goroutine popping tasks
	receivedCount := 0
	for receivedCount < numProducers*numTasks {
		loop.internalMu.Lock()
		task, ok := loop.ingress.popLocked()
		loop.internalMu.Unlock()

		if !ok {
			t.Errorf("popLocked() failed prematurely at count %d", receivedCount)
			break
		}

		task.Runnable()
		receivedCount++
	}

	expectedTotal := numProducers * numTasks
	if receivedCount != expectedTotal {
		t.Errorf("Expected %d received tasks, got %d", expectedTotal, receivedCount)
	}

	// Queue should be empty
	loop.internalMu.Lock()
	finalLength := loop.ingress.Length()
	loop.internalMu.Unlock()

	if finalLength != 0 {
		t.Errorf("Expected empty queue after complete drain, got length %d", finalLength)
	}
}
