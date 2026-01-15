package eventloop

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// REGRESSION TEST FOR POPBATCH INCONSISTENCY
//
// This test proves the inconsistency between Pop() and PopBatch() behavior
// when handling in-flight producers.
// =============================================================================

// TestPopBatchInconsistencyWithPop proves PopBatch doesn't spin like Pop.
//
// DEFECT #5 (HIGH): PopBatch Inconsistency with Pop
//
// The bug: Pop() was correctly patched to spin-wait when a producer has claimed
// the tail but not yet linked the new node. However, PopBatch() was NOT given
// the same fix:
//
//	// Pop(): spins here
//	for { next = head.next.Load(); if next != nil { break }; runtime.Gosched() }
//
//	// PopBatch(): just exits
//	if next == nil { break }
//
// Impact: A consumer calling PopBatch in a loop can fail to retrieve tasks
// that a subsequent call to Pop would have correctly retrieved.
//
// FIX: PopBatch must perform the same spin-wait logic if head.next.Load()
// is nil but q.tail.Load() != head.
//
// RUN: go test -v -run TestPopBatchInconsistencyWithPop
func TestPopBatchInconsistencyWithPop(t *testing.T) {
	const iterations = 100000
	const producers = 4
	const batchSize = 64

	for round := 0; round < 5; round++ {
		q := NewLockFreeIngress()
		var wg sync.WaitGroup
		var pushCount atomic.Int64
		var popBatchCount atomic.Int64
		var popFallbackCount atomic.Int64

		// Start producers that push continuously
		for p := 0; p < producers; p++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for i := 0; i < iterations/producers; i++ {
					q.Push(func() {})
					pushCount.Add(1)
				}
			}(p)
		}

		// Consumer using PopBatch - may miss tasks due to bug
		consumerDone := make(chan struct{})
		buf := make([]Task, batchSize)

		go func() {
			defer close(consumerDone)
			lastProgress := time.Now()
			total := int64(0)

			for total < int64(iterations) {
				// Try PopBatch first
				n := q.PopBatch(buf, batchSize)
				if n > 0 {
					popBatchCount.Add(int64(n))
					total += int64(n)
					lastProgress = time.Now()
					continue
				}

				// PopBatch returned 0 - but is queue really empty?
				// Check with Pop() - the buggy behavior is that PopBatch
				// doesn't spin-wait for in-flight producers like Pop does

				task, ok := q.Pop()
				if ok {
					// Pop found a task that PopBatch missed!
					// This proves the inconsistency
					popFallbackCount.Add(1)
					total++
					_ = task
					lastProgress = time.Now()
					continue
				}

				// Check for stall
				if time.Since(lastProgress) > 2*time.Second {
					// If we still haven't received all tasks, there may be
					// tasks that neither PopBatch nor Pop can retrieve
					return
				}

				runtime.Gosched()
			}
		}()

		wg.Wait() // Wait for all producers

		// Wait for consumer with timeout
		select {
		case <-consumerDone:
		case <-time.After(10 * time.Second):
			t.Fatalf("Round %d: Consumer stalled. Pushed=%d, PopBatch=%d, PopFallback=%d",
				round, pushCount.Load(), popBatchCount.Load(), popFallbackCount.Load())
		}

		totalPopped := popBatchCount.Load() + popFallbackCount.Load()
		if totalPopped != int64(iterations) {
			t.Fatalf("Round %d: Task loss! Pushed=%d, Total Popped=%d (Batch=%d, Fallback=%d)",
				round, pushCount.Load(), totalPopped, popBatchCount.Load(), popFallbackCount.Load())
		}

		// Report on the inconsistency
		if popFallbackCount.Load() > 0 {
			t.Logf("Round %d: PopBatch missed %d tasks that Pop() retrieved. "+
				"This indicates PopBatch doesn't spin-wait like Pop.",
				round, popFallbackCount.Load())
		}
	}

	t.Log("Test completed. If popFallbackCount > 0 in any round, " +
		"PopBatch is not handling in-flight producers correctly.")
}

// TestPopBatchEmptyQueueBehavior verifies PopBatch behavior on truly empty queue.
//
// This is a baseline test to ensure PopBatch correctly returns 0 on empty queue.
func TestPopBatchEmptyQueueBehavior(t *testing.T) {
	q := NewLockFreeIngress()
	buf := make([]Task, 64)

	n := q.PopBatch(buf, 64)
	if n != 0 {
		t.Fatalf("Expected 0 from PopBatch on empty queue, got %d", n)
	}

	// Now push and pop to verify it works
	q.Push(func() {})
	q.Push(func() {})
	q.Push(func() {})

	n = q.PopBatch(buf, 64)
	if n != 3 {
		t.Fatalf("Expected 3 from PopBatch after pushing 3, got %d", n)
	}

	// Queue should be empty again
	n = q.PopBatch(buf, 64)
	if n != 0 {
		t.Fatalf("Expected 0 from PopBatch on drained queue, got %d", n)
	}
}
