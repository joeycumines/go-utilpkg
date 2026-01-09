package alternateone

import (
	"sync"
)

// SafeIngress is the single-lock ingress queue for the safety-first implementation.
//
// SAFETY: Uses a single sync.Mutex for the entire ingress subsystem.
// This eliminates lock ordering bugs and simplifies reasoning at the cost
// of higher contention.
//
// Thread Safety: This struct IS thread-safe. The single mutex protects
// all operations on all lanes.
type SafeIngress struct {
	external   taskList
	internal   taskList
	microtasks taskList
	mu         sync.Mutex
	taskIDGen  uint64
	closed     bool
}

// NewSafeIngress creates a new SafeIngress queue.
func NewSafeIngress() *SafeIngress {
	return &SafeIngress{}
}

// Push adds a task to the specified lane.
// Returns an error if the queue is closed.
//
// SAFETY: Validates invariants before and after push.
func (q *SafeIngress) Push(fn func(), lane Lane) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return ErrLoopTerminated
	}

	q.validateInvariants()
	defer q.validateInvariants()

	q.taskIDGen++
	task := SafeTask{
		Fn:   fn,
		ID:   q.taskIDGen,
		Lane: lane,
	}

	switch lane {
	case LaneExternal:
		q.external.push(task)
	case LaneInternal:
		q.internal.push(task)
	case LaneMicrotask:
		q.microtasks.push(task)
	}

	return nil
}

// Pop removes and returns a task from the queue.
// Priority order: Microtasks > Internal > External
// Returns false if all lanes are empty.
//
// SAFETY: Must be called with lock held (internal use).
func (q *SafeIngress) Pop() (SafeTask, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.popLocked()
}

// popLocked removes a task without acquiring the lock.
// Caller must hold the lock.
func (q *SafeIngress) popLocked() (SafeTask, bool) {
	q.validateInvariants()
	defer q.validateInvariants()

	// Priority: Microtasks first
	if task, ok := q.microtasks.pop(); ok {
		return task, true
	}

	// Then internal
	if task, ok := q.internal.pop(); ok {
		return task, true
	}

	// Finally external
	if task, ok := q.external.pop(); ok {
		return task, true
	}

	return SafeTask{}, false
}

// PopExternal removes and returns a task from the external lane only.
// Returns false if the external lane is empty.
func (q *SafeIngress) PopExternal() (SafeTask, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.validateInvariants()
	defer q.validateInvariants()

	return q.external.pop()
}

// PopInternal removes and returns a task from the internal lane only.
// Returns false if the internal lane is empty.
func (q *SafeIngress) PopInternal() (SafeTask, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.validateInvariants()
	defer q.validateInvariants()

	return q.internal.pop()
}

// PopMicrotask removes and returns a task from the microtask lane only.
// Returns false if the microtask lane is empty.
func (q *SafeIngress) PopMicrotask() (SafeTask, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.validateInvariants()
	defer q.validateInvariants()

	return q.microtasks.pop()
}

// Length returns the total number of pending tasks across all lanes.
// SAFETY: Must be called while holding the lock for Check-Then-Sleep protocol.
func (q *SafeIngress) Length() int {
	return q.external.length + q.internal.length + q.microtasks.length
}

// LengthLocked returns the total length while holding the lock.
// This is used by the Check-Then-Sleep protocol.
func (q *SafeIngress) LengthLocked() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.Length()
}

// ExternalLength returns the number of tasks in the external lane.
func (q *SafeIngress) ExternalLength() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.external.length
}

// InternalLength returns the number of tasks in the internal lane.
func (q *SafeIngress) InternalLength() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.internal.length
}

// MicrotaskLength returns the number of tasks in the microtask lane.
func (q *SafeIngress) MicrotaskLength() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.microtasks.length
}

// Close marks the queue as closed. Subsequent Push calls will return an error.
func (q *SafeIngress) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
}

// IsClosed returns true if the queue has been closed.
func (q *SafeIngress) IsClosed() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.closed
}

// Lock acquires the mutex.
// Used by the Check-Then-Sleep protocol to hold lock through sleep decision.
func (q *SafeIngress) Lock() {
	q.mu.Lock()
}

// Unlock releases the mutex.
func (q *SafeIngress) Unlock() {
	q.mu.Unlock()
}

// validateInvariants checks queue invariants.
// SAFETY: Panics if invariants are violated.
func (q *SafeIngress) validateInvariants() {
	// Check external lane
	validateListInvariants(&q.external, "external")
	// Check internal lane
	validateListInvariants(&q.internal, "internal")
	// Check microtask lane
	validateListInvariants(&q.microtasks, "microtasks")
}

// validateListInvariants checks invariants for a single task list.
func validateListInvariants(l *taskList, name string) {
	if l.length < 0 {
		panic("alternateone: " + name + " queue length is negative")
	}

	// head/tail asymmetry check
	if (l.head == nil) != (l.tail == nil) {
		panic("alternateone: " + name + " queue head/tail asymmetry")
	}

	// If both nil, length must be 0
	if l.head == nil && l.length != 0 {
		panic("alternateone: " + name + " queue nil but length non-zero")
	}
}
