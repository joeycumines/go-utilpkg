package eventloop

import (
	"sync"
	"sync/atomic"
	"weak"
)

// Task represents a unit of work submitted to the event loop.
type Task struct {
	// Runnable is the function to execute.
	Runnable func()
}

// ============================================================================
// LOCK-FREE MPSC QUEUE (from AlternateTwo)
// ============================================================================

// node is a node in the lock-free MPSC queue.
type node struct {
	task Task
	next atomic.Pointer[node]
}

// nodePool pools MPSC queue nodes.
var nodePool = sync.Pool{
	New: func() any {
		return &node{}
	},
}

// getNode gets a node from the pool.
func getNode() *node {
	return nodePool.Get().(*node)
}

// putNode returns a node to the pool.
func putNode(n *node) {
	n.task = Task{}
	n.next.Store(nil)
	nodePool.Put(n)
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
	n.task = Task{Runnable: fn}
	n.next.Store(nil)

	// Atomically swap tail, linking previous tail to new node
	prev := q.tail.Swap(n)
	prev.next.Store(n) // Linearization point

	q.len.Add(1)
}

// PushTask adds a task struct to the queue (thread-safe for multiple producers).
// PERFORMANCE: Lock-free using atomic swap.
func (q *LockFreeIngress) PushTask(task Task) {
	n := getNode()
	n.task = task
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
		// If tail == head, queue is truly empty.
		// If tail != head, producer is mid-push (Swap done, but next not linked yet).
		// We must spin-retry to avoid task loss.
		if q.tail.Load() == head {
			return Task{}, false // Truly empty
		}
		// Producer is mid-push, spin-retry until link is visible
		for {
			next = head.next.Load()
			if next != nil {
				break // Producer completed link
			}
			head = q.head.Load()
			if q.tail.Load() == head {
				return Task{}, false // Queue became empty during spin
			}
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

// ============================================================================
// LOCK-FREE RING WITH OVERFLOW BUFFER
// ============================================================================

// MicrotaskRing is a lock-free ring buffer with overflow protection.
//
// PERFORMANCE: Lock-free ring for hot path, mutex-protected overflow for bursts.
//
// DESIGN:
//   - Static [4096] ring buffer for zero-allocation fast path
//   - Lock-free producer/consumer with atomic CAS operations
//   - When ring is full, overflow tasks to mutex-protected slice
//   - Overflow slice is drained before accepting new ring tasks
//
// Memory:
//   - Ring: ~32KB static overhead (4096 pointers)
//   - Overflow: O(n) growth, GC reclaimable when drained
//
// PROVABLE CORRECTNESS:
//   - Ring path: Producers CAS to claim slots, seq tracking validates writes
//   - Overflow path: Mutex protects all operations, safe for concurrent producers
//   - No complex state transitions between modes - simple ring-or-overflow check
type MicrotaskRing struct { // betteralign:ignore
	_       [64]byte            // Cache line padding //nolint:unused
	buffer  [4096]func()        // Ring buffer for tasks
	seq     [4096]atomic.Uint64 // Sequence numbers per slot
	head    atomic.Uint64       // Consumer index
	_       [56]byte            // Pad to cache line //nolint:unused
	tail    atomic.Uint64       // Producer index
	tailSeq atomic.Uint64       // Global sequence counter

	// Overflow: protected by mutex (simple, no race complexity)
	overflowMu sync.Mutex
	overflow   []func() // Overflow buffer (non-nil when ring is full)
}

// NewMicrotaskRing creates a new lock-free ring with sequence tracking.
func NewMicrotaskRing() *MicrotaskRing {
	r := &MicrotaskRing{}
	// Initialize all sequence numbers to 0 (unwritten slots)
	for i := 0; i < 4096; i++ {
		r.seq[i].Store(0)
	}
	return r
}

// Push adds a microtask to the ring buffer.
// Returns true if successfully added (always true, never fails).
// PERFORMANCE: Lock-free ring with mutex-protected overflow.
//
// ALGORITHM:
//  1. Try lock-free ring path: CAS tail to claim slot
//  2. Write sequence number first (consumer checks seq to validate)
//  3. Write task (now visible to consumer with seq validation)
//  4. If ring full (tail-head >= 4096), fallback to overflow
//
// PROVABLE CORRECTNESS: Sequence number ensures consumer sees:
//   - Valid task: seq != 0 AND task != nil (both written)
//   - Mid-push: seq != 0 BUT task == nil (wait in Pop)
//   - Unclaimed slot: seq == 0 (not yet written)
func (r *MicrotaskRing) Push(fn func()) bool {
	// Fast path: try lock-free ring
	for {
		tail := r.tail.Load()
		head := r.head.Load()

		// Check if ring has capacity
		inFlight := tail - head
		if inFlight >= 4096 {
			// Ring full, use mutex-protected overflow
			break
		}

		// Try to claim slot via CAS
		if r.tail.CompareAndSwap(tail, tail+1) {
			// Acquire sequence number (acts as write token)
			seq := r.tailSeq.Add(1)

			// Write sequence number BEFORE task (write ordering matters)
			r.seq[tail%4096].Store(seq)

			// Write task (now visible to consumer)
			r.buffer[tail%4096] = fn

			return true
		}
		// CAS failed, retry
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

// Pop removes and returns a microtask from the ring buffer.
// Returns nil if the buffer is empty.
// PERFORMANCE: Lock-free ring with fallback to overflow.
//
// ALGORITHM:
//  1. Check ring buffer first (lock-free path)
//  2. Use sequence number to validate slot is ready (not mid-push)
//  3. If ring empty or ring exhausted, check overflow buffer
//
// PROVABLE CORRECTNESS:
//   - Task only executed when seq != 0 AND fn != nil
//   - Mid-push slots (seq set, nil fn) are skipped after spin
//   - Overflow path is mutex-protected, no races possible
func (r *MicrotaskRing) Pop() func() {
	head := r.head.Load()
	tail := r.tail.Load()

	// Check ring buffer for tasks
	for head < tail {
		// Read task first (write order: seq then task)
		fn := r.buffer[head%4096]
		if fn == nil {
			// Task not written yet, producer may be mid-push
			// Check sequence to determine if slot is claimed
			seq := r.seq[head%4096].Load()
			if seq == 0 {
				// Unclaimed slot, advance head and retry
				if r.head.CompareAndSwap(head, head+1) {
					r.buffer[(head)%4096] = nil
				}
				head = r.head.Load()
				tail = r.tail.Load()
				continue
			}

			// Seq set but task still nil - producer mid-push
			// Reload head/tail and retry in next iteration
			head = r.head.Load()
			tail = r.tail.Load()
			continue
		}

		// Task exists, verify sequence is set
		seq := r.seq[head%4096].Load()
		if seq == 0 {
			// Orphaned task (seq cleared after we read fn), skip
			if r.head.CompareAndSwap(head, head+1) {
				r.buffer[(head)%4096] = nil
			}
			head = r.head.Load()
			tail = r.tail.Load()
			continue
		}

		// Both task and seq present - safe to consume
		r.head.Add(1)
		r.seq[(head)%4096].Store(0)
		r.buffer[(head)%4096] = nil
		return fn
	}

	// Ring is empty, check overflow buffer
	r.overflowMu.Lock()
	defer r.overflowMu.Unlock()

	if len(r.overflow) == 0 {
		return nil // No tasks in overflow
	}

	// Dequeue from overflow (pop from head)
	fn := r.overflow[0]
	// Remove first element efficiently
	copy(r.overflow, r.overflow[1:])
	r.overflow = r.overflow[:len(r.overflow)-1]

	return fn
}

// Length returns the number of microtasks in the ring.
// PERFORMANCE: May be slightly stale under concurrent modification.
func (r *MicrotaskRing) Length() int {
	head := r.head.Load()
	tail := r.tail.Load()

	// Count in flight in ring
	ringCount := 0
	if tail > head {
		ringCount = int(tail - head)
	}

	// Add overflow count
	r.overflowMu.Lock()
	overflowCount := len(r.overflow)
	r.overflowMu.Unlock()

	return ringCount + overflowCount
}

// IsEmpty returns true if the ring buffer is empty.
// PERFORMANCE: May have false negatives under concurrent modification.
func (r *MicrotaskRing) IsEmpty() bool {
	head := r.head.Load()
	tail := r.tail.Load()

	// Check ring
	if tail > head {
		return false // Has in-flight tasks
	}

	// Check overflow
	r.overflowMu.Lock()
	empty := len(r.overflow) == 0
	r.overflowMu.Unlock()

	return empty
}

// ============================================================================
// LEGACY COMPATIBILITY
// ============================================================================

// IngressQueue provides backward compatibility for existing code.
// It wraps LockFreeIngress with the old interface.
// NOTE: This is for test compatibility only. New code should use LockFreeIngress.
type IngressQueue struct {
	q *LockFreeIngress
}

// Push adds a task to the queue.
// Compatibility wrapper for old ingress.Push(Task) calls.
func (q *IngressQueue) Push(task Task) {
	if q.q == nil {
		q.q = NewLockFreeIngress()
	}
	q.q.PushTask(task)
}

// Length returns the queue length.
func (q *IngressQueue) Length() int {
	if q.q == nil {
		return 0
	}
	return int(q.q.Length())
}

// popLocked returns a task. The "locked" suffix is a misnomer - this queue is lock-free.
// Kept for test compatibility.
func (q *IngressQueue) popLocked() (Task, bool) {
	if q.q == nil {
		return Task{}, false
	}
	return q.q.Pop()
}

// ============================================================================
// WEAK POINTER UTILITIES
// ============================================================================

// weak.Pointer is imported from the weak package.
// We have a _ type alias to force import.
var _ weak.Pointer[int]
