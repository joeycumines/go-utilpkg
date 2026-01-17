package eventloop

import (
	"runtime"
	"sync"
	"sync/atomic"
	"weak"
)

// Task represents a unit of work submitted to the event loop.
type Task struct {
	Runnable func()
}

// ============================================================================
// CHUNKED INGRESS QUEUE (Unlocked - caller must hold mutex)
// ============================================================================
// This implementation uses a chunked linked-list for task submission.
// Unlike mutex-protected queues, this struct has NO internal mutex - the caller
// (Loop) is responsible for synchronization via an external mutex.
//
// This design matches AlternateThree's architecture which was proven faster
// in benchmarks because it allows the Loop to check state and push atomically
// under a single mutex, eliminating the need for inflight counters.
//
// Performance rationale:
// - External mutex: Loop can check shutdown state while holding the same mutex
//   used for queue operations, making state+push atomic without extra atomics.
// - Chunking: 128-task arrays provide cache locality and amortize allocations.
// - sync.Pool: Chunk recycling prevents GC thrashing under high throughput.
// ============================================================================

// chunkPool prevents GC thrashing under high load.
// Chunks are ~3KB each (128 Tasks * 24 bytes per Task).
var chunkPool = sync.Pool{
	New: func() any {
		return &chunk{}
	},
}

// chunk is a fixed-size node in the chunked linked-list.
// This optimizes cache locality compared to standard linked lists.
// Uses readPos/writePos cursors for O(1) push/pop without shifting.
type chunk struct {
	tasks   [128]Task
	next    *chunk
	readPos int // First unread slot (index into tasks)
	pos     int // First unused slot / writePos (index into tasks)
}

// newChunk creates and returns a new chunk from the pool.
func newChunk() *chunk {
	c := chunkPool.Get().(*chunk)
	// Reset fields for reuse (chunk may have been returned with stale data)
	c.pos = 0
	c.readPos = 0
	c.next = nil
	return c
}

// returnChunk returns an exhausted chunk to the pool.
// Clears all task references before returning to prevent memory leaks.
func returnChunk(c *chunk) {
	// Zero out all task slots to ensure no leaking references remain
	for i := 0; i < len(c.tasks); i++ {
		c.tasks[i] = Task{}
	}
	c.pos = 0
	c.readPos = 0
	c.next = nil
	chunkPool.Put(c)
}

// ChunkedIngress is a chunked linked-list queue for task submission.
//
// Thread Safety: This struct has NO internal mutex. The caller MUST hold an
// external mutex when calling any method. This design allows the Loop to
// perform atomic state-check-and-push operations under a single mutex.
//
// Usage pattern (from Loop.Submit):
//
//	l.externalMu.Lock()
//	if terminated { l.externalMu.Unlock(); return ErrLoopTerminated }
//	l.external.pushLocked(task)  // No mutex needed - caller holds externalMu
//	l.externalMu.Unlock()
type ChunkedIngress struct { // betteralign:ignore
	head, tail *chunk
	length     int64
}

// NewChunkedIngress creates a new chunked ingress queue.
func NewChunkedIngress() *ChunkedIngress {
	return &ChunkedIngress{}
}

// pushLocked adds a task to the queue. CALLER MUST HOLD EXTERNAL MUTEX.
func (q *ChunkedIngress) pushLocked(task Task) {
	if q.tail == nil {
		q.tail = newChunk()
		q.head = q.tail
	}

	if q.tail.pos == len(q.tail.tasks) {
		// Alloc new chunk
		newTail := newChunk()
		q.tail.next = newTail
		q.tail = newTail
	}

	q.tail.tasks[q.tail.pos] = task
	q.tail.pos++
	q.length++
}

// popLocked removes and returns a task. CALLER MUST HOLD EXTERNAL MUTEX.
// Returns false if the queue is empty.
func (q *ChunkedIngress) popLocked() (Task, bool) {
	if q.head == nil {
		return Task{}, false
	}

	// Check if current chunk is exhausted (readPos == pos means all written tasks read)
	if q.head.readPos >= q.head.pos {
		// If this is the only chunk, queue is empty - reset cursors for reuse
		if q.head == q.tail {
			// Reset cursors instead of keeping stale chunk.
			q.head.pos = 0
			q.head.readPos = 0
			return Task{}, false
		}
		// Move to next chunk
		oldHead := q.head
		q.head = q.head.next
		// Return exhausted chunk to pool
		returnChunk(oldHead)
	}

	// Double-check after potential chunk advancement
	if q.head.readPos >= q.head.pos {
		return Task{}, false
	}

	// O(1) read at readPos, then increment
	task := q.head.tasks[q.head.readPos]
	// Zero out popped slot for GC safety
	q.head.tasks[q.head.readPos] = Task{}
	q.head.readPos++
	q.length--

	// If chunk is now exhausted, free it or reset cursors
	if q.head.readPos >= q.head.pos {
		if q.head == q.tail {
			// Reset cursors instead of allocating new chunk.
			q.head.pos = 0
			q.head.readPos = 0
			return task, true
		}
		// Multiple chunks: Move to next chunk and return old to pool
		oldHead := q.head
		q.head = q.head.next
		returnChunk(oldHead)
	}

	return task, true
}

// lengthLocked returns the queue length. CALLER MUST HOLD EXTERNAL MUTEX.
func (q *ChunkedIngress) lengthLocked() int64 {
	return q.length
}
		// Alloc new chunk
		newTail := newChunk()
		q.tail.next = newTail
		q.tail = newTail
	}

	q.tail.tasks[q.tail.pos] = task
	q.tail.pos++
	q.length++

	q.mu.Unlock()
}

// Pop removes and returns a task. Safe for concurrent use.
// Returns false if the queue is empty.
func (q *ChunkedIngress) Pop() (Task, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.popLocked()
}

// popLocked removes and returns a task. Caller must hold q.mu.
func (q *ChunkedIngress) popLocked() (Task, bool) {
	if q.head == nil {
		return Task{}, false
	}

	// Check if current chunk is exhausted (readPos == pos means all written tasks read)
	if q.head.readPos >= q.head.pos {
		// If this is the only chunk, queue is empty - reset cursors for reuse
		if q.head == q.tail {
			// Reset cursors instead of keeping stale chunk.
			// This is safe because popLocked() zeros each slot before incrementing readPos,
			// so all slots [0..pos) are already zero. Resetting cursors allows the chunk
			// to be reused without sync.Pool churn.
			q.head.pos = 0
			q.head.readPos = 0
			return Task{}, false
		}
		// Move to next chunk
		oldHead := q.head
		q.head = q.head.next
		// Return exhausted chunk to pool
		returnChunk(oldHead)
	}

	// Double-check after potential chunk advancement
	if q.head.readPos >= q.head.pos {
		return Task{}, false
	}

	// O(1) read at readPos, then increment
	task := q.head.tasks[q.head.readPos]
	// Zero out popped slot for GC safety
	q.head.tasks[q.head.readPos] = Task{}
	q.head.readPos++
	q.length--

	// If chunk is now exhausted, free it or reset cursors
	if q.head.readPos >= q.head.pos {
		if q.head == q.tail {
			// Reset cursors instead of allocating new chunk.
			// This eliminates sync.Pool churn in ping-pong workloads (Push 1, Pop 1).
			q.head.pos = 0
			q.head.readPos = 0
			return task, true
		}
		// Multiple chunks: Move to next chunk and return old to pool
		oldHead := q.head
		q.head = q.head.next
		returnChunk(oldHead)
	}

	return task, true
}

// PopBatch removes up to max tasks from the queue into buf.
// Returns the number of tasks popped.
func (q *ChunkedIngress) PopBatch(buf []Task, max int) int {
	q.mu.Lock()
	defer q.mu.Unlock()

	if max > len(buf) {
		max = len(buf)
	}

	count := 0
	for count < max {
		task, ok := q.popLocked()
		if !ok {
			break
		}
		buf[count] = task
		count++
	}

	return count
}

// Length returns the queue length.
func (q *ChunkedIngress) Length() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.length
}

// IsEmpty returns true if the queue is empty.
func (q *ChunkedIngress) IsEmpty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.length == 0
}

// ============================================================================
// LEGACY LOCK-FREE MPSC QUEUE (Preserved for backward compatibility)
// ============================================================================
// This implementation is preserved for backward compatibility and testing.
// New code should use ChunkedIngress which performs better under contention.
// ============================================================================

type node struct {
	task Task
	next atomic.Pointer[node]
}

var nodePool = sync.Pool{
	New: func() any {
		return &node{}
	},
}

func getNode() *node {
	return nodePool.Get().(*node)
}

func putNode(n *node) {
	n.task = Task{}
	n.next.Store(nil)
	nodePool.Put(n)
}

// LockFreeIngress is a lock-free multi-producer single-consumer (MPSC) queue.
//
// Theoretical Classification: Linearizing Queue (Not a Multiplexer).
// This implementation enforces a total global order via tail swapping.
// A single stalled producer can block the consumer, meaning acausality within
// streams is not preserved. However, this is required to satisfy "Zero-allocation"
// and "Unbounded" requirements.
//
// Performance:
// - Uses atomic CAS for producers with no mutex on hot paths.
// - Single-threaded consumer pops in batches to amortize cache misses.
//
// NOTE: ChunkedIngress outperforms this under high contention. This is preserved
// for backward compatibility and edge cases where lock-free is preferred.
type LockFreeIngress struct { // betteralign:ignore
	_    [64]byte             // Cache line padding
	head atomic.Pointer[node] // Consumer reads from head
	_    [56]byte             // Pad to cache line
	tail atomic.Pointer[node] // Producers swap tail
	_    [56]byte             // Pad to cache line
	stub node                 // Sentinel node
	len  atomic.Int64         // Approximate queue length
	_    [56]byte             // Pad to cache line
}

// NewLockFreeIngress creates a new lock-free MPSC queue.
func NewLockFreeIngress() *LockFreeIngress {
	q := &LockFreeIngress{}
	q.head.Store(&q.stub)
	q.tail.Store(&q.stub)
	return q
}

// Push adds a task to the queue. Safe for concurrent use by multiple producers.
// It establishes a causal link via atomic tail swapping (Linearization point).
func (q *LockFreeIngress) Push(fn func()) {
	n := getNode()
	n.task = Task{Runnable: fn}
	n.next.Store(nil)

	// Atomically swap tail, linking previous tail to new node.
	prev := q.tail.Swap(n)
	prev.next.Store(n)

	q.len.Add(1)
}

// PushTask adds a task struct to the queue. Safe for concurrent use by multiple producers.
func (q *LockFreeIngress) PushTask(task Task) {
	n := getNode()
	n.task = task
	n.next.Store(nil)

	prev := q.tail.Swap(n)
	prev.next.Store(n)

	q.len.Add(1)
}

// Pop removes and returns a task. Safe for use by a SINGLE consumer only.
// Returns false if the queue is empty.
func (q *LockFreeIngress) Pop() (Task, bool) {
	head := q.head.Load()
	next := head.next.Load()
	if next == nil {
		if q.tail.Load() == head {
			return Task{}, false // Truly empty
		}

		// Acausality Risk: Tail moved, but 'next' pointer isn't linked yet.
		// A producer has Swapped but not yet Stored next.
		// We structurally block/spin until the specific producer finishes.
		for {
			next = head.next.Load()
			if next != nil {
				break
			}
			runtime.Gosched()
		}
	}

	task := next.task
	next.task = Task{} // Clear for GC
	q.head.Store(next)

	// Recycle old head (unless it's the stub)
	if head != &q.stub {
		putNode(head)
	}

	q.len.Add(-1)
	return task, true
}

// PopBatch removes up to max tasks from the queue into buf.
// Batched popping amortizes cache misses.
// DEFECT #5 FIX: Implements the same spin-wait logic as Pop() to handle
// the acausality window where tail has moved but next pointer isn't linked yet.
func (q *LockFreeIngress) PopBatch(buf []Task, max int) int {
	count := 0
	head := q.head.Load()

	if max > len(buf) {
		max = len(buf)
	}

	for count < max {
		next := head.next.Load()
		if next == nil {
			// DEFECT #5 FIX: Check if tail != head, indicating a producer is in-flight.
			// A producer has Swapped tail but not yet Stored next pointer.
			// We must spin-wait just like Pop() does, or we'll miss tasks.
			if q.tail.Load() == head {
				break // Truly empty - both tail and next confirm no pending tasks
			}
			// Producer in-flight: spin until next pointer is linked
			for {
				next = head.next.Load()
				if next != nil {
					break
				}
				runtime.Gosched()
			}
		}

		buf[count] = next.task
		next.task = Task{} // Clear for GC
		q.head.Store(next)

		if head != &q.stub {
			putNode(head)
		}

		head = next
		count++
	}

	if count > 0 {
		q.len.Add(int64(-count))
	}
	return count
}

// Length returns the approximate queue length.
// Note: Result may be stale due to concurrent operations.
func (q *LockFreeIngress) Length() int64 {
	return q.len.Load()
}

// IsEmpty returns true if the queue appears empty.
// Note: May have false negatives under concurrent modification.
func (q *LockFreeIngress) IsEmpty() bool {
	head := q.head.Load()
	return head.next.Load() == nil
}

// ============================================================================
// LOCK-FREE RING WITH OVERFLOW BUFFER
// ============================================================================

// MicrotaskRing is a lock-free ring buffer with overflow protection.
//
// Memory Ordering & Correctness:
// This implementation adheres to strict Release/Acquire semantics to prevent "Time Travel" bugs
// where a consumer sees a valid sequence but reads uninitialized data.
//
// Algorithm:
// - Push: Write Data -> Store Seq (Release)
// - Pop:  Load Seq (Acquire) -> Read Data
// - Overflow: When ring is full (4096 items), tasks spill to a mutex-protected slice.
// - FIFO: If overflow has items, Push appends to overflow to maintain ordering.
type MicrotaskRing struct { // betteralign:ignore
	_       [64]byte            // Cache line padding
	buffer  [4096]func()        // Ring buffer for tasks
	seq     [4096]atomic.Uint64 // Sequence numbers per slot
	head    atomic.Uint64       // Consumer index
	_       [56]byte            // Pad to cache line
	tail    atomic.Uint64       // Producer index
	tailSeq atomic.Uint64       // Global sequence counter

	// Overflow protected by mutex (simple, no race complexity)
	overflowMu      sync.Mutex
	overflow        []func()
	overflowHead    int         // FIFO: Index of first valid item in overflow (avoids O(n) copy)
	overflowPending atomic.Bool // FIFO: Flag indicating overflow has items
}

// NewMicrotaskRing creates a new lock-free ring with sequence tracking.
func NewMicrotaskRing() *MicrotaskRing {
	r := &MicrotaskRing{}
	for i := 0; i < 4096; i++ {
		r.seq[i].Store(0)
	}
	return r
}

// Push adds a microtask to the ring buffer. Always returns true.
func (r *MicrotaskRing) Push(fn func()) bool {
	// DEFECT #4 FIX: If overflow has items, append to overflow to maintain FIFO.
	// This fast-path check uses atomic bool to avoid mutex in common case.
	if r.overflowPending.Load() {
		r.overflowMu.Lock()
		// Double-check under lock
		if len(r.overflow)-r.overflowHead > 0 {
			r.overflow = append(r.overflow, fn)
			r.overflowMu.Unlock()
			return true
		}
		r.overflowMu.Unlock()
	}

	// Fast path: try lock-free ring
	for {
		tail := r.tail.Load()
		head := r.head.Load()

		if tail-head >= 4096 {
			break // Ring full, must use overflow
		}

		if r.tail.CompareAndSwap(tail, tail+1) {
			seq := r.tailSeq.Add(1)

			// DEFECT #10 FIX: Handle sequence wrap-around.
			// 0 is the sentinel value for "empty slot", so if we wrap to 0,
			// we must skip it to avoid confusion with uninitialized slots.
			if seq == 0 {
				seq = r.tailSeq.Add(1)
			}

			// CRITICAL ORDERING:
			// 1. Write Task (Data) FIRST.
			// 2. Write Sequence (Guard) SECOND.
			// atomic.Store acts as a Release barrier, ensuring the buffer write
			// is visible to any reader who acquires this seq value.
			r.buffer[tail%4096] = fn
			r.seq[tail%4096].Store(seq)

			return true
		}
	}

	// Slow path: ring full, use mutex-protected overflow
	r.overflowMu.Lock()
	if r.overflow == nil {
		r.overflow = make([]func(), 0, 1024)
	}
	r.overflow = append(r.overflow, fn)
	r.overflowPending.Store(true)
	r.overflowMu.Unlock()
	return true
}

// Pop removes and returns a microtask. Returns nil if empty.
func (r *MicrotaskRing) Pop() func() {
	// Try ring buffer first (maintains FIFO - ring items are older)
	head := r.head.Load()
	tail := r.tail.Load()

	for head < tail {
		// CRITICAL ORDERING:
		// Read Sequence (Guard) via atomic Load (Acquire).
		// If we see the expected sequence, we are guaranteed to see the
		// corresponding data write from Push due to the Release-Acquire pair.
		idx := head % 4096
		seq := r.seq[idx].Load()

		if seq == 0 {
			// Producer claimed 'tail' but hasn't stored 'seq' yet.
			// Spin and retry. We cannot advance head (skipping) because the slot IS claimed.
			head = r.head.Load()
			tail = r.tail.Load()
			runtime.Gosched()
			continue
		}

		fn := r.buffer[idx]

		// DEFECT #6 FIX: Handle nil tasks to avoid infinite loop.
		// If fn is nil but seq != 0, the slot was claimed but contains nil.
		// We must still consume the slot and continue to the next one.
		if fn == nil {
			// DEFECT #3 FIX: Clear buffer FIRST, then seq (release barrier),
			// then advance head. This ordering ensures:
			// 1. buffer=nil happens before seq.Store (seq.Store is release barrier)
			// 2. seq=0 happens before head.Add
			// 3. When producer reads new head value, it sees buffer=nil and seq=0
			r.buffer[idx] = nil
			r.seq[idx].Store(0)
			r.head.Add(1)
			head = r.head.Load()
			tail = r.tail.Load()
			continue
		}

		// DEFECT #3 FIX: Clear buffer and seq BEFORE advancing head.
		// CRITICAL ORDERING for memory visibility:
		// 1. Clear buffer (non-atomic) - must happen first
		// 2. Clear seq (atomic release) - provides memory barrier, ensures buffer write visible
		// 3. Advance head (atomic) - signals slot is free to producers
		// The seq.Store's release semantics ensure that when a producer sees the
		// new head value (via head.Load's acquire semantics), it also sees buffer=nil.
		r.buffer[idx] = nil
		r.seq[idx].Store(0)
		r.head.Add(1)
		return fn
	}

	// Ring empty, check overflow buffer
	if !r.overflowPending.Load() {
		return nil // Fast path: no overflow items
	}

	r.overflowMu.Lock()
	defer r.overflowMu.Unlock()

	// Calculate actual overflow count
	overflowCount := len(r.overflow) - r.overflowHead
	if overflowCount == 0 {
		r.overflowPending.Store(false)
		return nil
	}

	fn := r.overflow[r.overflowHead]
	r.overflow[r.overflowHead] = nil // Zero out for GC
	r.overflowHead++

	// Compact overflow slice if more than half is consumed
	if r.overflowHead > len(r.overflow)/2 && r.overflowHead > 512 {
		remaining := len(r.overflow) - r.overflowHead
		copy(r.overflow, r.overflow[r.overflowHead:])
		r.overflow = r.overflow[:remaining]
		r.overflowHead = 0
	}

	// Clear pending flag if overflow is now empty
	if r.overflowHead >= len(r.overflow) {
		r.overflowPending.Store(false)
	}

	return fn
}

// Length returns the total number of microtasks (ring + overflow).
func (r *MicrotaskRing) Length() int {
	head := r.head.Load()
	tail := r.tail.Load()

	ringCount := 0
	if tail > head {
		ringCount = int(tail - head)
	}

	r.overflowMu.Lock()
	overflowCount := len(r.overflow) - r.overflowHead
	r.overflowMu.Unlock()

	return ringCount + overflowCount
}

// IsEmpty returns true if the ring buffer and overflow are empty.
// Note: May have false negatives under concurrent modification.
//
// IMPORTANT: This uses len(r.overflow) - r.overflowHead to properly account for
// the FIFO head pointer, consistent with Length() method. The overflow slice is
// not immediately compacted after pops; instead, overflowHead advances, and
// compaction occurs lazily (see Pop()).
func (r *MicrotaskRing) IsEmpty() bool {
	head := r.head.Load()
	tail := r.tail.Load()

	if tail > head {
		return false
	}

	r.overflowMu.Lock()
	// BUG FIX (2026-01-17): Previously checked len(r.overflow) == 0, which was
	// incorrect when overflow had been partially drained (overflowHead > 0).
	// Must check effective count: len(overflow) - overflowHead == 0.
	empty := len(r.overflow)-r.overflowHead == 0
	r.overflowMu.Unlock()

	return empty
}

// ============================================================================
// LEGACY COMPATIBILITY
// ============================================================================

// IngressQueue wraps LockFreeIngress to provide backward compatibility for tests.
// New code should use LockFreeIngress directly.
type IngressQueue struct {
	q *LockFreeIngress
}

func (q *IngressQueue) Push(task Task) {
	if q.q == nil {
		q.q = NewLockFreeIngress()
	}
	q.q.PushTask(task)
}

func (q *IngressQueue) Length() int {
	if q.q == nil {
		return 0
	}
	return int(q.q.Length())
}

// popLocked returns a task.
// The "locked" suffix is a misnomer; the underlying queue is lock-free.
func (q *IngressQueue) popLocked() (Task, bool) {
	if q.q == nil {
		return Task{}, false
	}
	return q.q.Pop()
}

// ============================================================================
// WEAK POINTER UTILITIES
// ============================================================================

// Force import of weak package
var _ weak.Pointer[int]
