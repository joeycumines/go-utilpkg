package alternatetwo

import (
	"sync/atomic"
)

// node is a node in the lock-free MPSC queue.
type node struct {
	task Task
	next atomic.Pointer[node]
}

// LockFreeIngress is a lock-free multi-producer single-consumer queue.
//
// PERFORMANCE: Uses atomic CAS for producers with no mutex on hot paths.
// Single-threaded consumer pops in batches to amortize cache misses.
//
// Design: Intrusive linked list with stub node.
// Producers: Atomic swap of tail pointer, then link previous.
// Consumer: Walk from head, reclaiming nodes to pool.
type LockFreeIngress struct { // betteralign:ignore
	_    [64]byte             // Cache line padding //nolint:unused
	head atomic.Pointer[node] // Consumer reads from head
	_    [56]byte             // Pad to cache line //nolint:unused
	tail atomic.Pointer[node] // Producers swap tail
	_    [56]byte             // Pad to cache line //nolint:unused
	stub node                 // Sentinel node
	len  atomic.Int64         // Queue length (approximate)
	_    [56]byte             // Pad to cache line //nolint:unused
}

// NewLockFreeIngress creates a new lock-free MPSC queue.
func NewLockFreeIngress() *LockFreeIngress {
	q := &LockFreeIngress{}
	q.head.Store(&q.stub)
	q.tail.Store(&q.stub)
	return q
}

// Push adds a task to the queue (thread-safe for multiple producers).
// PERFORMANCE: Lock-free using atomic swap.
func (q *LockFreeIngress) Push(fn func()) {
	n := getNode()
	n.task = Task{Fn: fn}
	n.next.Store(nil)

	// Atomically swap tail, linking previous tail to new node
	prev := q.tail.Swap(n)
	prev.next.Store(n) // Linearization point

	q.len.Add(1)
}

// Pop removes and returns a task from the queue (single consumer only).
// Returns false if the queue is empty.
// PERFORMANCE: No locking, single-threaded consumer.
func (q *LockFreeIngress) Pop() (Task, bool) {
	head := q.head.Load()
	next := head.next.Load()
	if next == nil {
		return Task{}, false
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
// Returns the number of tasks popped.
// PERFORMANCE: Batched pop amortizes cache misses.
func (q *LockFreeIngress) PopBatch(buf []Task, max int) int {
	count := 0
	head := q.head.Load()

	// Limit to buffer size
	if max > len(buf) {
		max = len(buf)
	}

	for count < max {
		next := head.next.Load()
		if next == nil {
			break
		}

		buf[count] = next.task
		next.task = Task{} // Clear for GC
		q.head.Store(next)

		// Recycle old head (unless it's the stub)
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
// PERFORMANCE: May be slightly stale due to concurrent operations.
func (q *LockFreeIngress) Length() int64 {
	return q.len.Load()
}

// IsEmpty returns true if the queue appears empty.
// PERFORMANCE: May have false negatives under concurrent modification.
func (q *LockFreeIngress) IsEmpty() bool {
	head := q.head.Load()
	return head.next.Load() == nil
}

// MicrotaskRing is a lock-free ring buffer for microtasks.
//
// PERFORMANCE: Fixed-size ring buffer with atomic head/tail.
// Suitable for bounded microtask queues.
type MicrotaskRing struct { // betteralign:ignore
	_      [64]byte      // Cache line padding //nolint:unused
	buffer [4096]func()  // Ring buffer
	head   atomic.Uint64 // Consumer index
	_      [56]byte      // Pad to cache line //nolint:unused
	tail   atomic.Uint64 // Producer index
	_      [56]byte      // Pad to cache line //nolint:unused
}

// NewMicrotaskRing creates a new microtask ring buffer.
func NewMicrotaskRing() *MicrotaskRing {
	return &MicrotaskRing{}
}

// Push adds a microtask to the ring buffer.
// Returns false if the buffer is full.
// PERFORMANCE: Lock-free CAS loop.
func (r *MicrotaskRing) Push(fn func()) bool {
	for {
		tail := r.tail.Load()
		head := r.head.Load()
		if tail-head >= 4096 {
			return false // Full
		}
		if r.tail.CompareAndSwap(tail, tail+1) {
			r.buffer[tail%4096] = fn
			return true
		}
	}
}

// Pop removes and returns a microtask from the ring buffer.
// Returns nil if the buffer is empty.
// PERFORMANCE: Single consumer, no CAS needed.
func (r *MicrotaskRing) Pop() func() {
	head := r.head.Load()
	tail := r.tail.Load()
	if head >= tail {
		return nil // Empty
	}

	fn := r.buffer[head%4096]
	r.buffer[head%4096] = nil // Clear for GC
	r.head.Add(1)
	return fn
}

// Length returns the number of microtasks in the ring.
func (r *MicrotaskRing) Length() uint64 {
	return r.tail.Load() - r.head.Load()
}

// IsEmpty returns true if the ring buffer is empty.
func (r *MicrotaskRing) IsEmpty() bool {
	return r.head.Load() >= r.tail.Load()
}
