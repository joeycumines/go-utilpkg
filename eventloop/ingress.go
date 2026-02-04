package eventloop

import (
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
)

const (
	// chunkSize is the number of tasks per node in the ChunkedIngress linked list.
	// 128 tasks * 8 bytes/task + overhead = ~1KB per chunk.
	chunkSize = 128

	// ringBufferSize is the fixed size of the MicrotaskRing buffer.
	// It must be a power of 2 to allow for efficient bitwise wrapping.
	ringBufferSize = 4096

	// ringSeqSkip is the sentinel value for "empty slot" in sequence tracking.
	// We use this value (1 << 63, half of uint64) to avoid ambiguity with
	// sequence number wrap-around, which could legitimately produce 0.
	ringSeqSkip = uint64(1) << 63

	// ringOverflowInitCap is the initial capacity for the overflow slice in MicrotaskRing.
	ringOverflowInitCap = 1024

	// ringOverflowCompactThreshold is the threshold for compacting the overflow slice.
	// When more than this many items have been read from the head of the overflow,
	// and it exceeds half the slice length, a copy/compaction occurs.
	ringOverflowCompactThreshold = 512

	// ringHeadPadSize is the padding size required after the 'head' field in MicrotaskRing
	// to ensure 'tail' starts on a new cache line.
	ringHeadPadSize = sizeOfCacheLine - sizeOfAtomicUint64
)

// ChunkedIngress is a chunked linked-list queue for task submission.
//
// Thread Safety: This struct is NOT thread-safe.
// The caller must provide external synchronization (e.g., the Event Loop's mutex).
//
// Performance rationale:
// - Fixed-size arrays (chunkSize) provide cache locality and amortize allocations.
// - sync.Pool chunk recycling prevents GC thrashing under high throughput.
type ChunkedIngress struct { // betteralign:ignore
	head   *chunk
	tail   *chunk
	length int
}

// chunkPool prevents GC thrashing under high load.
var chunkPool = sync.Pool{
	New: func() any {
		return &chunk{}
	},
}

// chunk is a fixed-size node in the chunked linked-list.
// It uses readPos/writePos cursors for O(1) push/pop without shifting.
type chunk struct {
	tasks   [chunkSize]func()
	next    *chunk
	readPos int // First unread slot (index into tasks)
	pos     int // First unused slot / writePos (index into tasks)
}

// newChunk creates and returns a new chunk from the pool.
func newChunk() *chunk {
	c := chunkPool.Get().(*chunk)
	// Reset fields for reuse as the chunk may have been returned with stale data
	c.pos = 0
	c.readPos = 0
	c.next = nil
	return c
}

// returnChunk returns an exhausted chunk to the pool.
// It assumes all tasks have been cleared by Pop before returning.
//
// IMP-002 Fix: Clear task slots before returning to prevent memory leaks
// from retained references to task closures.
func returnChunk(c *chunk) {
	// Clear all task slots to prevent memory leaks from retained closures
	// Matches pattern from alternatetwo/returnChunkFast()
	for i := 0; i < c.pos; i++ {
		c.tasks[i] = nil
	}
	c.pos = 0
	c.readPos = 0
	c.next = nil
	chunkPool.Put(c)
}

// NewChunkedIngress creates a new chunked ingress queue.
func NewChunkedIngress() *ChunkedIngress {
	return &ChunkedIngress{}
}

// Push adds a task to the queue.
//
// CALLER MUST HOLD EXTERNAL MUTEX.
func (q *ChunkedIngress) Push(task func()) {
	if q.tail == nil {
		q.tail = newChunk()
		q.head = q.tail
	}

	if q.tail.pos == len(q.tail.tasks) {
		newTail := newChunk()
		q.tail.next = newTail
		q.tail = newTail
	}

	q.tail.tasks[q.tail.pos] = task
	q.tail.pos++
	q.length++
}

// Pop removes and returns a task.
//
// Returns false if the queue is empty.
//
// CALLER MUST HOLD EXTERNAL MUTEX.
func (q *ChunkedIngress) Pop() (func(), bool) {
	if q.head == nil {
		return nil, false
	}

	// Check if current chunk is exhausted (readPos == pos means all written tasks read)
	if q.head.readPos >= q.head.pos {
		// If this is the only chunk, queue is empty - reset cursors for reuse
		if q.head == q.tail {
			q.head.pos = 0
			q.head.readPos = 0
			return nil, false
		}
		// Move to next chunk and return exhausted one to pool
		oldHead := q.head
		q.head = q.head.next
		returnChunk(oldHead)
	}

	// Double-check after potential chunk advancement
	if q.head.readPos >= q.head.pos {
		return nil, false
	}

	// O(1) read at readPos, then increment
	task := q.head.tasks[q.head.readPos]
	// Zero out popped slot for GC safety
	q.head.tasks[q.head.readPos] = nil
	q.head.readPos++
	q.length--

	// If chunk is now exhausted, free it or reset cursors
	if q.head.readPos >= q.head.pos {
		if q.head == q.tail {
			q.head.pos = 0
			q.head.readPos = 0
			return task, true
		}
		oldHead := q.head
		q.head = q.head.next
		returnChunk(oldHead)
	}

	return task, true
}

// Length returns the queue length.
//
// CALLER MUST HOLD EXTERNAL MUTEX.
func (q *ChunkedIngress) Length() int {
	return q.length
}

// MicrotaskRing is a lock-free ring buffer with overflow protection.
//
// Memory Ordering & Correctness:
// This implementation adheres to strict Release/Acquire semantics to prevent bugs
// where a consumer sees a valid sequence but reads uninitialized data.
//
// R101 Fix: Sequence Zero Edge Case:
// The ring buffer uses explicit validity flags (atomic.Bool) to distinguish between
// 'empty slot' and 'slot with valid data', avoiding ambiguity from sequence number
// wrap-around. Previously, seq==0 was used as a sentinel, but under extreme producer
// load, sequence numbers can legitimately wrap to 0, causing infinite spin loops.
// Now validity is tracked separately and sequence numbers use ringSeqSkip (1<<63)
// as the empty sentinel, providing ~2^63 valid sequence values before wrap-around.
//
// Concurrency Model: MPSC (Multiple Producers, Single Consumer)
// - Push: Called from any goroutine (producers)
// - Pop: Called ONLY from the event loop goroutine (single consumer)
//
// Algorithm:
// - Push: Write Data -> Write Validity -> Store Seq (Release)
// - Pop:  Load Seq (Acquire) -> Check Validity -> Read Data
// - Overflow: When ring is full, tasks spill to a mutex-protected slice.
// - FIFO: If overflow has items, Push appends to overflow to maintain ordering.
type MicrotaskRing struct { // betteralign:ignore
	_       [sizeOfCacheLine]byte         // Cache line padding
	buffer  [ringBufferSize]func()        // Ring buffer for tasks
	valid   [ringBufferSize]atomic.Bool   // Slot validity flags (R101 fix)
	seq     [ringBufferSize]atomic.Uint64 // Sequence numbers per slot
	head    atomic.Uint64                 // Consumer index
	_       [ringHeadPadSize]byte         // Pad to cache line
	tail    atomic.Uint64                 // Producer index
	tailSeq atomic.Uint64                 // Global sequence counter

	overflowMu      sync.Mutex
	overflow        []func()
	overflowHead    int         // FIFO: Index of first valid item in overflow
	overflowPending atomic.Bool // FIFO: Flag indicating overflow has items
}

// NewMicrotaskRing creates a new lock-free ring with sequence tracking.
func NewMicrotaskRing() *MicrotaskRing {
	r := &MicrotaskRing{}
	for i := 0; i < ringBufferSize; i++ {
		r.seq[i].Store(ringSeqSkip) // Use skip sentinel (R101 fix)
		r.valid[i].Store(false)     // Start with all slots invalid
	}
	return r
}

// Push adds a microtask to the ring buffer. Always returns true.
func (r *MicrotaskRing) Push(fn func()) bool {
	// If overflow has items, append to overflow to maintain FIFO.
	// This fast-path check uses atomic bool to avoid mutex in common case.
	if r.overflowPending.Load() {
		r.overflowMu.Lock()
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

		if tail-head >= ringBufferSize {
			break // Ring full, must use overflow
		}

		if r.tail.CompareAndSwap(tail, tail+1) {
			seq := r.tailSeq.Add(1)

			// R101: No need to skip 0 - we use ringSeqSkip as sentinel.
			// Sequence numbers can legitimately wrap from MAX_UINT64 to 0.
			// Validity is tracked separately via valid[] flags.

			// Critical Ordering:
			// - Write Task (Data) FIRST
			// - Write Validity SECOND (R101 fix)
			// - Write Sequence (Guard) THIRD
			// atomic.Store acts as a Release barrier, ensuring all writes
			// are visible to any reader who acquires this seq value.
			idx := tail % ringBufferSize
			r.buffer[idx] = fn
			r.valid[idx].Store(true) // Mark slot valid (R101)
			r.seq[idx].Store(seq)

			return true
		}
	}

	// Slow path: ring full, use mutex-protected overflow
	r.overflowMu.Lock()
	if r.overflow == nil {
		r.overflow = make([]func(), 0, ringOverflowInitCap)
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
		// Critical Ordering:
		// Read Sequence (Guard) via atomic Load (Acquire).
		// If we see the expected sequence, we are guaranteed to see the
		// corresponding data write from Push due to the Release-Acquire pair.
		idx := head % ringBufferSize
		seq := r.seq[idx].Load()

		// R101: Check validity instead of seq==0 to avoid ambiguity.
		// Use ringSeqSkip as empty sentinel instead of 0.
		if seq == ringSeqSkip || !r.valid[idx].Load() {
			// Producer claimed 'tail' but hasn't stored a valid sequence yet.
			// Spin and retry. We cannot advance head (skipping) because the slot IS claimed.
			head = r.head.Load()
			tail = r.tail.Load()
			runtime.Gosched()
			continue
		}

		fn := r.buffer[idx]

		// Handle nil tasks to avoid infinite loop.
		// If fn is nil but seq != ringSeqSkip and valid==true, the slot was
		// claimed but contains nil. We must still consume the slot and continue.
		if fn == nil {
			// Clear buffer FIRST, then validity, then seq (release barrier), then advance head.
			// This ordering ensures (R101 fix):
			// - buffer=nil happens before valid.Store
			// - seq=ringSeqSkip happens before head.Add
			// - When producer reads new head value, it sees buffer=nil and valid=false
			r.buffer[idx] = nil
			r.valid[idx].Store(false)
			r.seq[idx].Store(ringSeqSkip)
			r.head.Add(1)
			head = r.head.Load()
			tail = r.tail.Load()
			continue
		}

		// Clear buffer, validity, and seq BEFORE advancing head.
		// Critical ordering for memory visibility (R101 fix):
		// - Clear buffer (non-atomic) - must happen first
		// - Clear validity (atomic) - marks slot invalid
		// - Clear seq (atomic release) - provides memory barrier, ensures write visible
		// - Advance head (atomic) - signals slot is free to producers
		r.buffer[idx] = nil
		r.valid[idx].Store(false)     // R101: Mark invalid
		r.seq[idx].Store(ringSeqSkip) // R101: Use skip sentinel
		r.head.Add(1)
		return fn
	}

	// Ring empty, check overflow buffer
	if !r.overflowPending.Load() {
		return nil
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
	if r.overflowHead > len(r.overflow)/2 && r.overflowHead > ringOverflowCompactThreshold {
		copy(r.overflow, r.overflow[r.overflowHead:])
		r.overflow = slices.Delete(r.overflow, len(r.overflow)-r.overflowHead, len(r.overflow))
		r.overflowHead = 0
	}

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
func (r *MicrotaskRing) IsEmpty() bool {
	head := r.head.Load()
	tail := r.tail.Load()

	if tail > head {
		return false
	}

	r.overflowMu.Lock()
	// Must check effective count to properly account for the FIFO head pointer.
	// len(r.overflow) might not be 0 even if drained, until compaction.
	empty := len(r.overflow)-r.overflowHead == 0
	r.overflowMu.Unlock()

	return empty
}
