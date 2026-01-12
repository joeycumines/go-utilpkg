package alternatethree

import (
	"sync"
)

// T11 FIX: Chunk pool to prevent GC thrashing under high load.
// Chunks are ~3KB each (128 Tasks * 24 bytes per Task).
// Without pooling, high-throughput producers cause allocation storms.
var chunkPool = sync.Pool{
	New: func() any {
		return &chunk{}
	},
}

// Task represents a unit of work submitted to the event loop.
type Task struct {
	// Runnable is the function to execute.
	Runnable func()
}

// IngressQueue is a chunked linked-list queue for task submission.
//
// Thread Safety: This struct is NOT thread-safe. Synchronization must be
// managed by the container (Loop) using a mutex (e.g., Loop.ingressMu).
//
// It supports the "Check-Then-Sleep" protocol by allowing the Loop to
// inspect Length while holding the specific mutex used for the barrier.
type IngressQueue struct {
	head, tail *chunk

	// Length tracks the number of pending tasks in the queue.
	// Must only be accessed while the container's mutex is held.
	length int
}

// chunk is a fixed-size node in the chunked linked-list.
// This optimizes cache locality compared to standard linked lists.
// D10: Uses readPos/writePos cursors for O(1) push/pop without shifting.
type chunk struct {
	tasks   [128]Task
	next    *chunk
	readPos int // First unread slot (index into tasks)
	pos     int // First unused slot / writePos (index into tasks)
}

// newChunk creates and returns a new chunk.
// T11 FIX: Uses sync.Pool for efficient allocation, preventing GC thrashing.
func newChunk() *chunk {
	c := chunkPool.Get().(*chunk)
	// Reset fields for reuse (chunk may have been returned with stale data)
	c.pos = 0
	c.readPos = 0
	c.next = nil
	return c
}

// returnChunk returns an exhausted chunk to the pool.
// T11 FIX: Clears all task references before returning to prevent memory leaks.
// We clear all 128 slots to ensure no leaking references, even if only a few were used.
func returnChunk(c *chunk) {
	// Zero out all task slots to ensure no leaking references remain
	// This prevents memory leaks even if pos was lower than 128
	for i := 0; i < len(c.tasks); i++ {
		c.tasks[i] = Task{}
	}
	c.pos = 0
	c.readPos = 0
	c.next = nil
	chunkPool.Put(c)
}

// Push appends a task to the tail of the queue.
// This is called by Producer goroutines (external to the loop) via Loop.Submit.
//
// Precondition: Caller must hold the container's mutex (Loop.ingressMu).
func (q *IngressQueue) Push(task Task) {
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

// Pop removes and returns a task from the head of the queue.
// Returns false if queue is empty.
//
// This is called by the Loop Routine only.
// Precondition: _mu must be held by caller (to support Length() check in Check-Then-Sleep).
//
// D10: O(1) pop using readPos cursor instead of O(N) element shifting.
//
//lint:ignore U1000 // Will be used in Phase 2 when implementing ingress processing
func (q *IngressQueue) popLocked() (Task, bool) {
	if q.head == nil {
		return Task{}, false
	}

	// Check if current chunk is exhausted (readPos == pos means all written tasks read)
	if q.head.readPos >= q.head.pos {
		// If this is the only chunk, queue is empty - reset cursors for reuse
		if q.head == q.tail {
			// INGRESS-FIX-UNIFIED (Early Path): Reset cursors instead of keeping stale chunk.
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
		// T11 FIX: Return exhausted chunk to pool
		returnChunk(oldHead)
	}

	// Double-check after potential chunk advancement
	if q.head.readPos >= q.head.pos {
		return Task{}, false
	}

	// D10: O(1) read at readPos, then increment
	task := q.head.tasks[q.head.readPos]
	// Zero out popped slot for GC safety
	q.head.tasks[q.head.readPos] = Task{}
	q.head.readPos++
	q.length--

	// If chunk is now exhausted, free it or reset cursors
	if q.head.readPos >= q.head.pos {
		if q.head == q.tail {
			// INGRESS-FIX-UNIFIED (Late Path): Reset cursors instead of allocating new chunk.
			// This eliminates sync.Pool churn in ping-pong workloads (Push 1, Pop 1).
			// Safe because all slots [0..pos) were zeroed during pop operations.
			q.head.pos = 0
			q.head.readPos = 0
			// Return the task we already popped - queue is now empty but reusable
			return task, true
		}
		// Multiple chunks: Move to next chunk and return old to pool
		oldHead := q.head
		q.head = q.head.next
		// T11 FIX: Return exhausted chunk to pool
		returnChunk(oldHead)
	}

	return task, true
}

// Length returns the number of pending tasks in the queue.
//
// CRITICAL: This MUST NOT be called except while the container's mutex is held.
//
// The correct usage is inside the Check-Then-Sleep protocol:
//
//	// Inside Loop Routine Poll phase:
//	atomic.StoreInt32(&loop.state, StateSleeping)
//	ingressMu.Lock()
//	len := ingress.length  // <- CRITICAL: read while holding lock
//	ingressMu.Unlock()
//	if len > 0 { ... }
func (q *IngressQueue) Length() int {
	return q.length
}
