package alternatetwo

import (
	"sync"
	"sync/atomic"
)

// arenaSize is the size of the pre-allocated task arena.
const arenaSize = 65536

// TaskArena provides lock-free allocation of tasks from a pre-allocated buffer.
//
// PERFORMANCE: Avoids GC pressure by pre-allocating tasks.
// Uses atomic increment for thread-safe index management.
type TaskArena struct { // betteralign:ignore
	_      [64]byte        // Cache line padding //nolint:unused
	buffer [arenaSize]Task // Pre-allocated task buffer
	head   atomic.Uint32   // Next allocation index
	_      [60]byte        // Pad to complete cache line //nolint:unused
}

// Alloc returns a pointer to a task slot from the arena.
// PERFORMANCE: Single atomic increment, no locking.
// Note: Arena wraps around, so old tasks may be overwritten.
func (a *TaskArena) Alloc() *Task {
	idx := a.head.Add(1) - 1
	return &a.buffer[idx%arenaSize]
}

// Reset resets the arena head to 0.
// SAFETY: Only call when no concurrent allocations are happening.
func (a *TaskArena) Reset() {
	a.head.Store(0)
}

// Object pools for various types.
var (
	// nodePool pools MPSC queue nodes.
	nodePool = sync.Pool{
		New: func() any {
			return &node{}
		},
	}

	// resultPool pools result objects.
	resultPool = sync.Pool{
		New: func() any {
			return &Result{}
		},
	}
)

// Result represents an async operation result.
type Result struct {
	Value any
	Err   error
}

// GetNode gets a node from the pool.
func getNode() *node {
	return nodePool.Get().(*node)
}

// PutNode returns a node to the pool.
func putNode(n *node) {
	n.task = Task{}
	n.next.Store(nil)
	nodePool.Put(n)
}

// GetResult gets a result from the pool.
func GetResult() *Result {
	return resultPool.Get().(*Result)
}

// PutResult returns a result to the pool.
func PutResult(r *Result) {
	r.Value = nil
	r.Err = nil
	resultPool.Put(r)
}
