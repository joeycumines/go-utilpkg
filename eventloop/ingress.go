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

// ============================================================================
// HYBRID MICROTASK RING (D2 FIX)
// ============================================================================

// MicrotaskRing is a hybrid ring buffer with dynamic slice fallback.
//
// D2 FIX: Implements dynamic growth to eliminate data loss on overflow.
//
// PERFORMANCE: Two-tier design:
//   - Fast path: Static [4096] ring buffer with lock-free atomic operations (no allocation)
//   - Slow path: Dynamic []func() slice protected by RWMutex, grows on demand
//
// DESIGN:
//   - Starts in ring mode using fixed-size array for zero-allocation fast path
//   - When ring is full and under contention, automatically transitions to slice mode
//   - Slice mode uses append() for natural growth, protected by mutex
//   - The slow path only activates under sustained burst conditions
//
// Memory:
//   - Ring mode: ~32KB static overhead (4096 pointers)
//   - Slice mode: O(n) growth, GC reclaimable when drained
type MicrotaskRing struct { // betteralign:ignore
	_      [64]byte      // Cache line padding //nolint:unused
	buffer [4096]func()  // Fast path: static ring buffer
	head   atomic.Uint64 // Consumer index (ring mode)
	_      [56]byte      // Pad to cache line //nolint:unused
	tail   atomic.Uint64 // Producer index (ring mode)
	_      [56]byte      // Pad to cache line //nolint:unused

	// Slow path: dynamic slice fallback
	onSlice   atomic.Bool  // Atomic flag: true when using slice mode
	sliceMu   sync.RWMutex // Protects slice access
	slice     []func()     // Dynamic fallback buffer (non-nil only in slice mode)
	sliceHead int          // Consumer index (slice mode)
}

// NewMicrotaskRing creates a new hybrid microtask ring buffer.
func NewMicrotaskRing() *MicrotaskRing {
	return &MicrotaskRing{}
}

// Push adds a microtask to the ring buffer.
// Returns true if successfully added (always true - never fails).
// PERFORMANCE: Two-tier design with lock-free fast path.
//
// ALGORITHM:
//  1. Check if already in slice mode (atomic read) - if so, use slow path
//  2. Try fast path: CAS ring head/tail to claim slot in static ring
//  3. If ring full, transition to slice mode and append
//
// D2 FIX: Never returns false - implements dynamic growth instead.
func (r *MicrotaskRing) Push(fn func()) bool {
	// Slow path: already in slice mode
	if r.onSlice.Load() {
		r.sliceMu.Lock()
		r.slice = append(r.slice, fn)
		r.sliceMu.Unlock()
		return true
	}

	// Fast path: try to use ring buffer
	for {
		tail := r.tail.Load()
		head := r.head.Load()

		// Check if ring is full
		if tail-head >= 4096 {
			// Ring full - transition to slice mode
			if r.onSlice.CompareAndSwap(false, true) {
				// We won the race to switch to slice mode
				// Drain ring into slice to preserve order
				r.sliceMu.Lock()
				r.slice = make([]func(), 0, 8192)
				currentHead := r.head.Load()
				currentTail := r.tail.Load()
				for i := currentHead; i < currentTail; i++ {
					r.slice = append(r.slice, r.buffer[i%4096])
					r.buffer[i%4096] = nil // Clear for GC
				}
				r.sliceHead = 0
				// Reset ring state (consumer will see onSlice=true)
				r.sliceMu.Unlock()
			}
			// Retry push in slice mode
			// Fall through to slow path (atomic read will see onSlice=true)
			if r.onSlice.Load() {
				r.sliceMu.Lock()
				r.slice = append(r.slice, fn)
				r.sliceMu.Unlock()
				return true
			}
			// Lost race to switch, retry CAS loop
			continue
		}

		// Try to claim slot via CAS
		if r.tail.CompareAndSwap(tail, tail+1) {
			// Write AFTER increment - producer has claimed slot
			// Consumer may see this write immediately
			// But before write, check if we raced to slice mode
			if r.onSlice.Load() {
				// Lost race - another goroutine switched to slice mode
				// Revert our tail increment and retry in slice mode
				r.tail.Add(^uint64(0)) // Tail - 1
				r.sliceMu.Lock()
				r.slice = append(r.slice, fn)
				r.sliceMu.Unlock()
				return true
			}
			r.buffer[tail%4096] = fn
			return true
		}
		// CAS failed, retry
	}
}

// Pop removes and returns a microtask from the ring buffer.
// Returns nil if the buffer is empty.
// PERFORMANCE: Two-tier design with lock-free fast path.
//
// ALGORITHM:
//  1. Check mode (atomic read onSlice flag)
//  2. Ring mode: read slot with barrier check, advance head
//  3. Slice mode: read from slice under read-lock, advance head
//
// CRITICAL FIX: Barrier check handles the race where consumer sees head < tail
// but producer hasn't completed the write yet. Spin until data is visible.
func (r *MicrotaskRing) Pop() func() {
	// Slow path: slice mode
	if r.onSlice.Load() {
		r.sliceMu.Lock()
		defer r.sliceMu.Unlock()

		if r.sliceHead >= len(r.slice) {
			return nil // Empty
		}

		fn := r.slice[r.sliceHead]
		r.slice[r.sliceHead] = nil // Clear for GC
		r.sliceHead++

		// Optimize: if slice drained, reset to ring mode
		if r.sliceHead >= len(r.slice) {
			r.slice = nil
			r.sliceHead = 0
			r.onSlice.Store(false)
		}

		return fn
	}

	// Fast path: ring mode
	head := r.head.Load()
	tail := r.tail.Load()

	// Double-check for race to slice mode
	if r.onSlice.Load() {
		// Race - switched to slice mode, retry from top
		return r.Pop()
	}

	if head >= tail {
		return nil // Empty
	}

	// Barrier: Wait for producer to complete write if it hasn't yet
	// This handles the case where tail advanced but write not visible
	slot := r.buffer[head%4096]

	// Check race again while spinning (another goroutine might switch mode)
	for slot == nil {
		if r.onSlice.Load() {
			// Race - switched to slice mode, abort and retry
			return r.Pop()
		}
		// Producer has claimed slot (head < tail) but hasn't written yet
		// Spin briefly to wait for write visibility
		slot = r.buffer[head%4096]
	}

	// Now increment head to consume the slot
	r.head.Add(1)

	// Clear buffer slot after moving head (safe for GC, after consumption point)
	r.buffer[(head)%4096] = nil

	return slot
}

// Length returns the number of microtasks in the ring.
// PERFORMANCE: May be slightly stale under concurrent modification.
func (r *MicrotaskRing) Length() int {
	// Slice mode
	if r.onSlice.Load() {
		r.sliceMu.RLock()
		defer r.sliceMu.RUnlock()
		return len(r.slice) - r.sliceHead
	}

	// Ring mode
	return int(r.tail.Load() - r.head.Load())
}

// IsEmpty returns true if the ring buffer is empty.
// PERFORMANCE: May have false negatives under concurrent modification.
func (r *MicrotaskRing) IsEmpty() bool {
	// Slice mode
	if r.onSlice.Load() {
		r.sliceMu.RLock()
		defer r.sliceMu.RUnlock()
		return r.sliceHead >= len(r.slice)
	}

	// Ring mode
	return r.head.Load() >= r.tail.Load()
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
