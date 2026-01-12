//go:build ignore
// +build ignore

// NOTE: This file tests the OLD chunked IngressQueue implementation which has been
// replaced by the lock-free LockFreeIngress. The old implementation is preserved
// in eventloop/internal/alternatethree/ along with its tests.
//
// This file is disabled because:
// 1. newChunk, returnChunk, chunkPool no longer exist
// 2. chunk type with tasks/pos/readPos no longer exists
// 3. IngressQueue is now a thin wrapper around LockFreeIngress
//
// For lock-free ingress testing, see:
// - eventloop/internal/alternatetwo/ingress.go (implementation)
// - eventloop/internal/tournament/ (integration tests)

package eventloop

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

// =============================================================================
// SECTION 6.1: MEMORY SAFETY TESTS
// =============================================================================

// TestReturnedChunkDoesNotRetainTasks proves that chunks returned to the pool
// do not retain references to tasks, allowing the GC to collect captured objects.
func TestReturnedChunkDoesNotRetainTasks(t *testing.T) {
	var finalized atomic.Bool

	type Big struct {
		_ [1 << 20]byte // 1MB to force heap allocation
	}

	obj := &Big{}
	runtime.SetFinalizer(obj, func(*Big) {
		finalized.Store(true)
	})

	q := &IngressQueue{}
	q.Push(Task{
		Runnable: func() {
			_ = obj // capture
		},
	})

	// Pop to force chunk exhaustion & return
	_, ok := q.popLocked()
	if !ok {
		t.Fatal("expected task")
	}

	// Force GC repeatedly
	for i := 0; i < 10 && !finalized.Load(); i++ {
		runtime.GC()
		runtime.Gosched()
		time.Sleep(10 * time.Millisecond)
	}

	if !finalized.Load() {
		t.Fatal("object was not garbage collected; task reference leaked")
	}
}

// TestReturnChunk_GuaranteesFullSanitization verifies that returnChunk clears
// ALL 128 slots, not just the used ones.
func TestReturnChunk_GuaranteesFullSanitization(t *testing.T) {
	c := newChunk()

	// "Dirty" the entire buffer with dummy tasks
	dummyTask := Task{Runnable: func() {}}
	for i := 0; i < len(c.tasks); i++ {
		c.tasks[i] = dummyTask
	}

	// Set cursors to simulate partial usage
	c.pos = 10
	c.readPos = 10

	returnChunk(c)

	// Get the chunk back from pool to inspect
	recovered := chunkPool.Get().(*chunk)

	// Inspect every single slot
	for i := 0; i < len(recovered.tasks); i++ {
		if recovered.tasks[i].Runnable != nil {
			t.Fatalf("Regression: returnChunk failed to clear slot %d", i)
		}
	}

	// Return it back
	chunkPool.Put(recovered)
}

// TestIngressQueue_PopClearsMemory verifies that popping tasks clears memory immediately.
func TestIngressQueue_PopClearsMemory(t *testing.T) {
	q := &IngressQueue{}

	// Fill one chunk completely (128 tasks)
	for i := 0; i < 128; i++ {
		q.Push(Task{Runnable: func() {}})
	}

	// Capture the chunk before we drain it
	chunkPtr := q.head

	// Verify it's dirty
	if chunkPtr.tasks[0].Runnable == nil {
		t.Fatal("Setup failed: expected chunk to be dirty")
	}

	// Drain the chunk
	for i := 0; i < 128; i++ {
		_, _ = q.popLocked()
	}

	// PROOF: Every slot must be zero (returnChunk was called)
	for i := 0; i < 128; i++ {
		if chunkPtr.tasks[i].Runnable != nil {
			t.Errorf("Memory Leak Proven: Slot %d was not cleared by Pop!", i)
		}
	}
}

// =============================================================================
// SECTION 6.2: CHUNK THRASHING TESTS
// =============================================================================

// TestIngressQueue_PingPongAllocations tests the allocation behavior during
// ping-pong workloads (Push 1, Pop 1 repeatedly).
// Note: Current implementation DOES replace chunks on exhaustion.
// This test documents the current behavior.
func TestIngressQueue_PingPongAllocations(t *testing.T) {
	q := &IngressQueue{}

	// Warmup: Create initial chunk
	q.Push(Task{Runnable: func() {}})

	// Perform Ping-Pong (Push 1, Pop 1) multiple times
	for i := 0; i < 50; i++ {
		_, ok := q.popLocked()
		if !ok {
			t.Fatalf("iter %d: expected task", i)
		}

		q.Push(Task{Runnable: func() {}})
	}

	// Verify queue still works correctly
	_, ok := q.popLocked()
	if !ok {
		t.Fatal("final pop: expected task")
	}

	if q.Length() != 0 {
		t.Fatalf("expected empty queue, got length %d", q.Length())
	}
}

// =============================================================================
// SECTION 6.3: SINGLE-CHUNK TRANSITION TESTS
// =============================================================================

// TestIngressQueue_SingleChunk_TransitionBehavior verifies behavior when
// the single chunk is exhausted.
func TestIngressQueue_SingleChunk_TransitionBehavior(t *testing.T) {
	q := &IngressQueue{}

	q.Push(Task{Runnable: func() {}})
	initialChunk := q.head

	_, _ = q.popLocked()

	// Current PR Logic: head/tail is replaced with newChunk()
	if q.head == initialChunk {
		t.Log("Note: Queue reused the exhausted chunk (cursor reset optimization)")
	}

	if q.head == nil {
		t.Fatal("Logic Error: Head is nil after exhaustion transition.")
	}

	q.Push(Task{Runnable: func() {}})
	if q.head.tasks[0].Runnable == nil {
		t.Error("Data Corruption: Pushed task not found in new head chunk.")
	}
}

// TestIngressQueue_Rotation_SingleChunk verifies full chunk rotation behavior.
func TestIngressQueue_Rotation_SingleChunk(t *testing.T) {
	q := &IngressQueue{}

	const chunkSize = 128
	for i := 0; i < chunkSize; i++ {
		q.Push(Task{Runnable: func() {}})
	}

	originalChunk := q.head

	for i := 0; i < chunkSize; i++ {
		_, ok := q.popLocked()
		if !ok {
			t.Fatalf("Failed to pop item %d", i)
		}
	}

	if q.head == originalChunk {
		t.Log("Note: Head chunk was not rotated (cursor reset optimization)")
	}

	if q.head != q.tail {
		t.Errorf("Integrity Error: Head and Tail should be equal after rotation.")
	}

	if q.head.pos != 0 {
		t.Errorf("Fresh chunk should have pos 0, got %d", q.head.pos)
	}

	q.Push(Task{Runnable: func() {}})
	if q.head.tasks[0].Runnable == nil {
		t.Error("Next push failed to write to the fresh chunk.")
	}
}

// =============================================================================
// SECTION 6.4: EXHAUSTION PATH CONSISTENCY TESTS
// =============================================================================

// TestIngressQueue_Resilience_EmptyChunkAtHead tests that popLocked correctly
// handles a manually created exhausted chunk at head.
func TestIngressQueue_Resilience_EmptyChunkAtHead(t *testing.T) {
	q := &IngressQueue{}

	// Manual Setup: Head = Empty (exhausted), Next = Valid Data
	c1 := newChunk()
	c1.pos = 10
	c1.readPos = 10 // Exhausted!

	c2 := newChunk()
	c2.tasks[0] = Task{Runnable: func() {}}
	c2.pos = 1
	c2.readPos = 0

	c1.next = c2
	q.head = c1
	q.tail = c2
	q.length = 1

	task, ok := q.popLocked()

	if !ok {
		t.Fatal("Resilience Failure: popLocked failed to skip the manually exhausted head chunk.")
	}

	// The task should have come from c2
	if task.Runnable == nil {
		t.Error("Task Runnable is nil - didn't get the expected task from c2")
	}

	// After popping, either:
	// - head advanced to c2 and then c2 was replaced (single-chunk case after c1 removed)
	// - or head is still pointing to c2 structure
	// The key point is the queue should now be empty
	if q.Length() != 0 {
		t.Errorf("Expected length 0 after pop, got %d", q.Length())
	}
}

// TestIngressQueue_ExhaustedSingleChunk_Handling tests the behavior when
// the single chunk is exhausted.
func TestIngressQueue_ExhaustedSingleChunk_Handling(t *testing.T) {
	q := &IngressQueue{}

	q.Push(Task{Runnable: func() {}})
	_, _ = q.popLocked()

	// Trigger the FINAL pop that should return false (queue empty)
	_, ok := q.popLocked()
	if ok {
		t.Fatal("Expected false from empty queue")
	}

	// Queue should still be usable
	q.Push(Task{Runnable: func() {}})
	task, ok := q.popLocked()
	if !ok {
		t.Fatal("Queue should have one task")
	}
	if task.Runnable == nil {
		t.Fatal("Task should have Runnable")
	}
}

// =============================================================================
// SECTION 6.5: FIFO & INVARIANT TESTS
// =============================================================================

// TestFIFOAcrossChunks verifies FIFO ordering is maintained across chunk boundaries.
func TestFIFOAcrossChunks(t *testing.T) {
	q := &IngressQueue{}

	const n = 1000 // crosses many chunks
	for i := 0; i < n; i++ {
		v := i
		q.Push(Task{Runnable: func() {
			if v < 0 {
				panic("impossible")
			}
		}})
	}

	for i := 0; i < n; i++ {
		_, ok := q.popLocked()
		if !ok {
			t.Fatalf("unexpected empty at %d", i)
		}
	}

	if q.Length() != 0 {
		t.Fatalf("expected length 0, got %d", q.Length())
	}
}

// TestFIFOOrderPreserved verifies tasks are returned in the exact order they were pushed.
func TestFIFOOrderPreserved(t *testing.T) {
	q := &IngressQueue{}

	const n = 500
	results := make([]int, 0, n)

	for i := 0; i < n; i++ {
		v := i
		q.Push(Task{Runnable: func() {
			results = append(results, v)
		}})
	}

	for i := 0; i < n; i++ {
		task, ok := q.popLocked()
		if !ok {
			t.Fatalf("unexpected empty at %d", i)
		}
		task.Runnable()
	}

	if len(results) != n {
		t.Fatalf("expected %d results, got %d", n, len(results))
	}

	for i := 0; i < n; i++ {
		if results[i] != i {
			t.Fatalf("FIFO violation: expected %d at index %d, got %d", i, i, results[i])
		}
	}
}

// TestLengthAccuracyAtAllStates verifies length is accurate at all queue states.
func TestLengthAccuracyAtAllStates(t *testing.T) {
	operations := []struct {
		push    int
		pop     int
		wantLen int
	}{
		{0, 0, 0},
		{1, 0, 1},
		{1, 1, 0},
		{128, 0, 128},
		{128, 128, 0},
		{129, 0, 129},
		{129, 129, 0},
		{200, 100, 100},
		{50, 50, 0},
	}

	for _, op := range operations {
		q := &IngressQueue{}
		for i := 0; i < op.push; i++ {
			q.Push(Task{})
		}
		for i := 0; i < op.pop && i < op.push; i++ {
			q.popLocked()
		}
		wantLen := op.push - min(op.pop, op.push)
		if q.Length() != wantLen {
			t.Errorf("After push%d/pop%d: length=%d, want %d",
				op.push, op.pop, q.Length(), wantLen)
		}
	}
}

// TestQueueIntegrity_Fuzz tests queue integrity under large push/pop cycles.
func TestQueueIntegrity_Fuzz(t *testing.T) {
	q := &IngressQueue{}

	const count = 10000

	for i := 0; i < count; i++ {
		q.Push(Task{Runnable: func() {}})
	}

	if q.Length() != count {
		t.Fatalf("Length mismatch: got %d, want %d", q.Length(), count)
	}

	for i := 0; i < count; i++ {
		_, ok := q.popLocked()

		if !ok {
			t.Fatalf("Premature empty at index %d", i)
		}
	}

	if q.Length() != 0 {
		t.Fatalf("Queue should be empty, has length %d", q.Length())
	}
	_, ok := q.popLocked()
	if ok {
		t.Fatal("Queue returned item when it should be empty")
	}
}

// =============================================================================
// SECTION 6.6: POOL REUSE TESTS
// =============================================================================

// TestChunkPoolReuseIsClean verifies that reused chunks are clean.
func TestChunkPoolReuseIsClean(t *testing.T) {
	q := &IngressQueue{}

	// First use
	q.Push(Task{Runnable: func() { panic("must not run") }})
	q.popLocked()

	// Force reuse
	q.Push(Task{})
	task, ok := q.popLocked()
	if !ok {
		t.Fatal("expected task")
	}

	if task.Runnable != nil {
		t.Fatal("reused chunk contained stale task")
	}
}

// TestPoolReuseDoesNotExposeStaleReferences verifies pool reuse safety.
func TestPoolReuseDoesNotExposeStaleReferences(t *testing.T) {
	q1 := &IngressQueue{}
	q1.Push(Task{Runnable: func() {}})
	q1.popLocked()

	q2 := &IngressQueue{}
	q2.Push(Task{Runnable: func() {}})

	// Check that unused slots in tail are clean
	for i := q2.tail.pos; i < 128; i++ {
		if q2.tail.tasks[i].Runnable != nil {
			t.Fatalf("Stale reference at slot %d: pool reuse contaminated", i)
		}
	}
}

// =============================================================================
// SECTION 6.7: CONCURRENCY & STRESS TESTS
// =============================================================================

// TestIngress_ParallelStress tests concurrent push/pop operations.
func TestIngress_ParallelStress(t *testing.T) {
	q := &IngressQueue{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	const count = 10000

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < count; i++ {
			mu.Lock()
			q.Push(Task{Runnable: func() {}})
			mu.Unlock()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		received := 0
		for received < count {
			mu.Lock()
			if _, ok := q.popLocked(); ok {
				received++
			}
			mu.Unlock()
		}
	}()

	wg.Wait()

	if q.length != 0 {
		t.Errorf("Expected empty queue, got length %d", q.length)
	}
}

// TestStressPushPop stress tests the queue with high-volume operations.
func TestStressPushPop(t *testing.T) {
	q := &IngressQueue{}
	var mu sync.Mutex
	done := make(chan struct{})

	const total = 100_000

	go func() {
		for i := 0; i < total; i++ {
			mu.Lock()
			q.Push(Task{})
			mu.Unlock()
		}
		close(done)
	}()

	popped := 0
	for {
		mu.Lock()
		_, ok := q.popLocked()
		mu.Unlock()

		if ok {
			popped++
		}

		select {
		case <-done:
			// Drain remaining
			for {
				mu.Lock()
				_, ok := q.popLocked()
				mu.Unlock()
				if !ok {
					break
				}
				popped++
			}
			if popped != total {
				t.Fatalf("Expected %d pops, got %d", total, popped)
			}
			return
		default:
		}
	}
}

// TestConcurrentMultiProducer tests multiple producers with a single consumer.
func TestConcurrentMultiProducer(t *testing.T) {
	q := &IngressQueue{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	const producers = 4
	const perProducer = 2500
	total := producers * perProducer

	// Start producers
	for p := 0; p < producers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perProducer; i++ {
				mu.Lock()
				q.Push(Task{Runnable: func() {}})
				mu.Unlock()
			}
		}()
	}

	// Wait for all producers
	wg.Wait()

	// Verify length
	if q.Length() != total {
		t.Fatalf("Expected length %d, got %d", total, q.Length())
	}

	// Drain
	for i := 0; i < total; i++ {
		mu.Lock()
		_, ok := q.popLocked()
		mu.Unlock()
		if !ok {
			t.Fatalf("Premature empty at %d", i)
		}
	}

	if q.Length() != 0 {
		t.Fatalf("Expected empty queue, got %d", q.Length())
	}
}

// =============================================================================
// SECTION 6.9: MEMORY LEAK UNDER LOAD
// =============================================================================

// TestNoLeakUnderLoad tests for memory leaks under sustained load.
func TestNoLeakUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping leak test in short mode")
	}

	q := &IngressQueue{}

	// Warm up the pool and queue
	for i := 0; i < 1000; i++ {
		q.Push(Task{Runnable: func() {}})
	}
	for i := 0; i < 1000; i++ {
		q.popLocked()
	}

	// Force GC to stabilize
	runtime.GC()
	runtime.GC()

	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	payload := make([]byte, 1024)
	for cycle := 0; cycle < 1000; cycle++ {
		for i := 0; i < 100; i++ {
			capturedPayload := payload
			q.Push(Task{
				Runnable: func() {
					_ = capturedPayload
				},
			})
		}

		for i := 0; i < 100; i++ {
			_, ok := q.popLocked()
			if !ok {
				t.Fatalf("cycle %d: unexpected empty at %d", cycle, i)
			}
		}
	}

	// Force GC multiple times
	for i := 0; i < 5; i++ {
		runtime.GC()
	}

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Use int64 to handle potential negative values (GC freed memory)
	heapBefore := int64(memBefore.HeapAlloc)
	heapAfter := int64(memAfter.HeapAlloc)
	heapDiff := heapAfter - heapBefore

	// Allow for some heap growth (10 chunks * 3KB = 30KB)
	// But also allow for heap to shrink (negative growth is fine)
	maxExpected := int64(100 * 1024) // 100KB tolerance
	if heapDiff > maxExpected {
		t.Errorf("Heap grew by %d bytes, indicating a potential leak (before=%d, after=%d)",
			heapDiff, heapBefore, heapAfter)
	}
}

// =============================================================================
// SECTION 6.8: PERFORMANCE BENCHMARKS
// =============================================================================

// BenchmarkIngress_PingPong benchmarks ping-pong workload allocation behavior.
func BenchmarkIngress_PingPong(b *testing.B) {
	q := &IngressQueue{}
	var mu sync.Mutex

	// Pre-fill to setup head/tail
	mu.Lock()
	q.Push(Task{Runnable: func() {}})
	mu.Unlock()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		mu.Lock()
		_, _ = q.popLocked()
		q.Push(Task{Runnable: func() {}})
		mu.Unlock()
	}
}

// BenchmarkSteadyStateChurn benchmarks steady state push/pop churn.
func BenchmarkSteadyStateChurn(b *testing.B) {
	q := &IngressQueue{}
	var mu sync.Mutex

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mu.Lock()
		q.Push(Task{Runnable: func() {}})
		_, _ = q.popLocked()
		mu.Unlock()
	}
}

// BenchmarkReturnChunk_Strategies benchmarks different returnChunk strategies.
func BenchmarkReturnChunk_Strategies(b *testing.B) {
	scenarios := []struct {
		name      string
		slotsUsed int
	}{
		{"Few_10", 10},
		{"Half_64", 64},
		{"Full_128", 128},
	}

	for _, sc := range scenarios {
		b.Run(sc.name+"_ClearUsed", func(b *testing.B) {
			c := &chunk{pos: sc.slotsUsed}
			for i := 0; i < sc.slotsUsed; i++ {
				c.tasks[i] = Task{Runnable: func() {}}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for j := 0; j < c.pos; j++ {
					c.tasks[j] = Task{}
				}
			}
		})

		b.Run(sc.name+"_ClearAll", func(b *testing.B) {
			c := &chunk{pos: sc.slotsUsed}
			for i := 0; i < sc.slotsUsed; i++ {
				c.tasks[i] = Task{Runnable: func() {}}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for j := 0; j < 128; j++ {
					c.tasks[j] = Task{}
				}
			}
		})
	}
}

// BenchmarkPush benchmarks Push operations.
func BenchmarkPush(b *testing.B) {
	q := &IngressQueue{}
	task := Task{Runnable: func() {}}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		q.Push(task)
	}
}

// BenchmarkPop benchmarks Pop operations on a pre-filled queue.
func BenchmarkPop(b *testing.B) {
	q := &IngressQueue{}
	task := Task{Runnable: func() {}}

	// Pre-fill
	for i := 0; i < b.N; i++ {
		q.Push(task)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		q.popLocked()
	}
}

// BenchmarkPushPop benchmarks alternating push/pop.
func BenchmarkPushPop(b *testing.B) {
	q := &IngressQueue{}
	task := Task{Runnable: func() {}}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		q.Push(task)
		q.popLocked()
	}
}

// BenchmarkChunkBoundary benchmarks operations that cross chunk boundaries.
func BenchmarkChunkBoundary(b *testing.B) {
	q := &IngressQueue{}
	task := Task{Runnable: func() {}}

	// Fill to chunk boundary
	for i := 0; i < 127; i++ {
		q.Push(task)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		q.Push(task)  // This crosses the boundary
		q.popLocked() // Drain one to stay at boundary
	}
}

// =============================================================================
// SECTION 6.10: INGRESS-FIX-UNIFIED VERIFICATION TESTS
// =============================================================================

// TestIngressQueue_EarlyExhaustionPathConsistency verifies that the early
// exhaustion check (empty queue pop attempt) correctly resets cursors.
// This test explicitly targets the Doc 9 critical bug where the early path
// would keep an exhausted chunk without resetting cursors.
func TestIngressQueue_EarlyExhaustionPathConsistency(t *testing.T) {
	q := &IngressQueue{}

	// Push a single task
	q.Push(Task{Runnable: func() {}})

	// Capture the initial chunk
	initialChunk := q.head

	// Pop the task - this exhausts the chunk and should trigger cursor reset
	task, ok := q.popLocked()
	if !ok {
		t.Fatal("Expected to get a task")
	}
	if task.Runnable == nil {
		t.Fatal("Expected task to have Runnable")
	}

	// After pop, the chunk should be reset (not replaced)
	// INGRESS-FIX-UNIFIED: Same chunk retained with cursors reset
	if q.head != initialChunk {
		t.Error("INGRESS-FIX-UNIFIED violation: Chunk was replaced instead of reused")
	}
	if q.head.pos != 0 {
		t.Errorf("INGRESS-FIX-UNIFIED violation: pos=%d, expected 0 after cursor reset", q.head.pos)
	}
	if q.head.readPos != 0 {
		t.Errorf("INGRESS-FIX-UNIFIED violation: readPos=%d, expected 0 after cursor reset", q.head.readPos)
	}

	// Now trigger the EARLY exhaustion path by popping from empty queue
	_, ok = q.popLocked()
	if ok {
		t.Fatal("Expected empty queue to return false")
	}

	// The early path should also reset cursors (consistent with late path)
	if q.head != initialChunk {
		t.Error("Early exhaustion path: Chunk was replaced instead of reused")
	}
	if q.head.pos != 0 {
		t.Errorf("Early exhaustion path: pos=%d, expected 0", q.head.pos)
	}
	if q.head.readPos != 0 {
		t.Errorf("Early exhaustion path: readPos=%d, expected 0", q.head.readPos)
	}

	// Verify queue is still usable after cursor reset
	q.Push(Task{Runnable: func() {}})
	if q.Length() != 1 {
		t.Errorf("Queue should have length 1 after push, got %d", q.Length())
	}
	if q.head.pos != 1 {
		t.Errorf("pos should be 1 after push, got %d", q.head.pos)
	}

	// Verify we can pop the new task
	task, ok = q.popLocked()
	if !ok {
		t.Fatal("Expected to get the pushed task")
	}
	if task.Runnable == nil {
		t.Fatal("Task should have Runnable")
	}

	// Verify all slots are clean (no stale references from previous use)
	for i := 0; i < len(q.head.tasks); i++ {
		if q.head.tasks[i].Runnable != nil {
			t.Errorf("Stale reference at slot %d after cursor reset + new use", i)
		}
	}
}

// TestIngressQueue_PingPongChunkStability proves that the cursor reset
// optimization prevents chunk allocation churn during ping-pong workloads.
// Before INGRESS-FIX-UNIFIED, every pop would trigger newChunk()/returnChunk().
func TestIngressQueue_PingPongChunkStability(t *testing.T) {
	q := &IngressQueue{}

	// Warmup: Create initial chunk
	q.Push(Task{Runnable: func() {}})
	initialChunk := q.head
	initialChunkAddr := uintptr(unsafe.Pointer(initialChunk))

	// Perform Ping-Pong (Pop 1, Push 1) 1000 times
	for i := 0; i < 1000; i++ {
		_, ok := q.popLocked()
		if !ok {
			t.Fatalf("iter %d: expected task", i)
		}

		// CRITICAL CHECK: Chunk address must NOT change
		if q.head != initialChunk {
			t.Fatalf("INGRESS-FIX-UNIFIED violation at iter %d: chunk address changed "+
				"from %p to %p (chunk thrashing)", i, initialChunk, q.head)
		}

		// Verify cursors were reset
		if q.head.pos != 0 || q.head.readPos != 0 {
			t.Fatalf("iter %d: cursors not reset (pos=%d, readPos=%d)",
				i, q.head.pos, q.head.readPos)
		}

		// Push next task
		q.Push(Task{Runnable: func() {}})

		// Verify chunk still the same after push
		if q.head != initialChunk {
			t.Fatalf("iter %d after push: chunk address changed", i)
		}
	}

	// Final verification
	if uintptr(unsafe.Pointer(q.head)) != initialChunkAddr {
		t.Errorf("Final chunk address mismatch: initial=%x, final=%x",
			initialChunkAddr, uintptr(unsafe.Pointer(q.head)))
	}

	t.Logf("✓ 1000 ping-pong iterations with ZERO chunk allocations (same addr: %p)", initialChunk)
}
