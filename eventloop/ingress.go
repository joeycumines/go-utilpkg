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
func (q *LockFreeIngress) PopBatch(buf []Task, max int) int {
	count := 0
	head := q.head.Load()

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
type MicrotaskRing struct { // betteralign:ignore
	_       [64]byte            // Cache line padding
	buffer  [4096]func()        // Ring buffer for tasks
	seq     [4096]atomic.Uint64 // Sequence numbers per slot
	head    atomic.Uint64       // Consumer index
	_       [56]byte            // Pad to cache line
	tail    atomic.Uint64       // Producer index
	tailSeq atomic.Uint64       // Global sequence counter

	// Overflow protected by mutex (simple, no race complexity)
	overflowMu sync.Mutex
	overflow   []func()
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
	// Fast path: try lock-free ring
	for {
		tail := r.tail.Load()
		head := r.head.Load()

		if tail-head >= 4096 {
			break // Ring full
		}

		if r.tail.CompareAndSwap(tail, tail+1) {
			seq := r.tailSeq.Add(1)

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
	r.overflowMu.Unlock()
	return true
}

// Pop removes and returns a microtask. Returns nil if empty.
func (r *MicrotaskRing) Pop() func() {
	head := r.head.Load()
	tail := r.tail.Load()

	for head < tail {
		// CRITICAL ORDERING:
		// Read Sequence (Guard) via atomic Load (Acquire).
		// If we see the expected sequence, we are guaranteed to see the
		// corresponding data write from Push due to the Release-Acquire pair.
		seq := r.seq[head%4096].Load()

		if seq == 0 {
			// Producer claimed 'tail' but hasn't stored 'seq' yet.
			// Spin and retry. We cannot advance head (skipping) because the slot IS claimed.
			head = r.head.Load()
			tail = r.tail.Load()
			runtime.Gosched()
			continue
		}

		fn := r.buffer[head%4096]

		// Defensive check: fn should rarely be nil if seq != 0.
		if fn == nil {
			head = r.head.Load()
			tail = r.tail.Load()
			continue
		}

		r.head.Add(1)
		r.seq[(head)%4096].Store(0)
		r.buffer[(head)%4096] = nil
		return fn
	}

	// Check overflow buffer
	r.overflowMu.Lock()
	defer r.overflowMu.Unlock()

	if len(r.overflow) == 0 {
		return nil
	}

	fn := r.overflow[0]

	// Efficiently remove first element avoiding memory leaks
	copy(r.overflow, r.overflow[1:])
	r.overflow[len(r.overflow)-1] = nil // Zero out for GC
	r.overflow = r.overflow[:len(r.overflow)-1]

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
	overflowCount := len(r.overflow)
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
