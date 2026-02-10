package alternateone

import "sync"

// chunkSize is the fixed size of each chunk in the chunked linked-list.
// This provides good cache locality.
const chunkSize = 128

// SafeTask represents a unit of work submitted to the event loop.
// Includes explicit ownership tracking for debugging.
type SafeTask struct {
	Fn   func() // The function to execute
	ID   uint64 // Unique task ID for tracking
	Lane Lane   // Which lane this task belongs to
}

// Lane identifies the priority lane for a task.
type Lane int

const (
	LaneExternal  Lane = iota // Normal external tasks
	LaneInternal              // Priority internal tasks
	LaneMicrotask             // Microtasks (highest priority)
)

// chunk is a fixed-size node in the chunked linked-list.
// SAFETY: Full-clear is always performed when returning to pool.
type chunk struct {
	next    *chunk
	tasks   [chunkSize]SafeTask
	readPos int // First unread slot
	pos     int // First unused slot (write position)
}

// chunkPool pools chunks for reuse.
// SAFETY: All chunks are fully cleared before returning to pool.
var chunkPool = sync.Pool{
	New: func() any {
		return &chunk{}
	},
}

// newChunk gets a chunk from the pool, fully initialized.
func newChunk() *chunk {
	c := chunkPool.Get().(*chunk)
	c.pos = 0
	c.readPos = 0
	c.next = nil
	return c
}

// returnChunk returns an exhausted chunk to the pool.
// SAFETY: Always clears ALL 128 slots regardless of usage.
// This is the conservative approach - we accept the overhead for correctness.
func returnChunk(c *chunk) {
	// SAFETY: Always full iteration - no optimization
	// Clear ALL slots to prevent memory leaks
	for i := range c.tasks {
		c.tasks[i] = SafeTask{}
	}
	c.pos = 0
	c.readPos = 0
	c.next = nil
	chunkPool.Put(c)
}

// taskList is a single linked list of chunks.
// Used internally by SafeIngress for each lane.
type taskList struct {
	head   *chunk
	tail   *chunk
	length int
}

// push adds a task to the tail of the list.
func (l *taskList) push(task SafeTask) {
	if l.tail == nil {
		l.tail = newChunk()
		l.head = l.tail
	}

	if l.tail.pos == chunkSize {
		// Current chunk is full, allocate new
		newTail := newChunk()
		l.tail.next = newTail
		l.tail = newTail
	}

	l.tail.tasks[l.tail.pos] = task
	l.tail.pos++
	l.length++
}

// pop removes and returns a task from the head of the list.
// Returns false if the list is empty.
func (l *taskList) pop() (SafeTask, bool) {
	if l.head == nil {
		return SafeTask{}, false
	}

	// Check if current chunk is exhausted
	if l.head.readPos >= l.head.pos {
		// If this is the only chunk, list is empty
		if l.head == l.tail {
			// Reset cursors for reuse
			l.head.pos = 0
			l.head.readPos = 0
			return SafeTask{}, false
		}
		// Move to next chunk
		oldHead := l.head
		l.head = l.head.next
		returnChunk(oldHead)
	}

	// Double-check after potential chunk advancement
	if l.head.readPos >= l.head.pos {
		return SafeTask{}, false
	}

	// Read task
	task := l.head.tasks[l.head.readPos]
	// SAFETY: Zero out slot immediately for GC
	l.head.tasks[l.head.readPos] = SafeTask{}
	l.head.readPos++
	l.length--

	// If chunk is now exhausted, handle it
	if l.head.readPos >= l.head.pos {
		if l.head == l.tail {
			// Single chunk, reset for reuse
			l.head.pos = 0
			l.head.readPos = 0
		} else {
			// Multiple chunks, return old to pool
			oldHead := l.head
			l.head = l.head.next
			returnChunk(oldHead)
		}
	}

	return task, true
}
