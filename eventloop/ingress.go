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
// LOCK-FREE MPSC QUEUE
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
func (r *MicrotaskRing) IsEmpty() bool {
	head := r.head.Load()
	tail := r.tail.Load()

	if tail > head {
		return false
	}

	r.overflowMu.Lock()
	empty := len(r.overflow) == 0
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
