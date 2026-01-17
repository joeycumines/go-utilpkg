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
// Thread Safety: This struct has an OPTIONAL internal mutex. For highest
// performance, use the *Locked methods with an external mutex held by the caller.
// For convenience (e.g., in tests), use the non-Locked methods which acquire
// the internal mutex.
//
// The Loop uses the Locked methods for optimal performance, while tests can
// use the convenience methods directly.
type ChunkedIngress struct { // betteralign:ignore
	mu         sync.Mutex // Internal mutex for convenience methods
	head, tail *chunk
	length     int64
}

// NewChunkedIngress creates a new chunked ingress queue.
func NewChunkedIngress() *ChunkedIngress {
	return &ChunkedIngress{}
}

// Push adds a function to the queue. Thread-safe via internal mutex.
// For higher performance, use pushLocked with an external mutex.
func (q *ChunkedIngress) Push(fn func()) {
	q.mu.Lock()
	q.pushLocked(Task{Runnable: fn})
	q.mu.Unlock()
}

// PushTask adds a task to the queue. Thread-safe via internal mutex.
// For higher performance, use pushLocked with an external mutex.
func (q *ChunkedIngress) PushTask(task Task) {
	q.mu.Lock()
	q.pushLocked(task)
	q.mu.Unlock()
}

// Pop removes and returns a task. Thread-safe via internal mutex.
// For higher performance, use popLocked with an external mutex.
func (q *ChunkedIngress) Pop() (Task, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.popLocked()
}

// Length returns the queue length. Thread-safe via internal mutex.
func (q *ChunkedIngress) Length() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.length
}

// IsEmpty returns true if the queue is empty. Thread-safe via internal mutex.
func (q *ChunkedIngress) IsEmpty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.length == 0
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
// WEAK POINTER UTILITIES
// ============================================================================

// Force import of weak package
var _ weak.Pointer[int]
