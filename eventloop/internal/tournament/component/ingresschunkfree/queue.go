// Package ingresschunkfree materializes the later Main mutex-owned chunked
// ingress with a bounded queue-local free list.
package ingresschunkfree

const (
	defaultChunkSize = 64
	defaultFreeLimit = 32
)

// Queue is not thread-safe. Its source owner provided external synchronization.
type Queue struct {
	head      *chunk
	tail      *chunk
	free      *chunk
	length    int
	freeCount int
	freeLimit int
	chunkSize int
}

type chunk struct {
	tasks   []func()
	next    *chunk
	readPos int
	pos     int
}

// NewNative constructs the source-native queue. Nonpositive sizes select the
// historical default of 64 callbacks per chunk.
func NewNative(chunkSize int) *Queue {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	return &Queue{chunkSize: chunkSize, freeLimit: defaultFreeLimit}
}

func (q *Queue) newChunk() *chunk {
	value := q.free
	if value == nil {
		return &chunk{tasks: make([]func(), q.chunkSize)}
	}
	q.free = value.next
	q.freeCount--
	value.pos = 0
	value.readPos = 0
	value.next = nil
	if cap(value.tasks) < q.chunkSize {
		value.tasks = make([]func(), q.chunkSize)
	} else {
		value.tasks = value.tasks[:q.chunkSize]
	}
	return value
}

func (q *Queue) returnChunk(value *chunk) {
	if value == nil {
		return
	}
	if cap(value.tasks) < q.chunkSize {
		value.tasks = make([]func(), q.chunkSize)
	} else {
		value.tasks = value.tasks[:q.chunkSize]
	}
	clear(value.tasks)
	value.pos = 0
	value.readPos = 0
	if q.freeLimit <= 0 {
		q.freeLimit = defaultFreeLimit
	}
	if q.freeCount >= q.freeLimit {
		value.next = nil
		return
	}
	value.next = q.free
	q.free = value
	q.freeCount++
}

// Push adds one callback. Nil is a source-valid queued value.
func (q *Queue) Push(fn func()) {
	if q.tail == nil {
		q.tail = q.newChunk()
		q.head = q.tail
	}
	if q.tail.pos == q.chunkSize {
		tail := q.newChunk()
		q.tail.next = tail
		q.tail = tail
	}
	q.tail.tasks[q.tail.pos] = fn
	q.tail.pos++
	q.length++
}

// Pop removes one callback. The boolean distinguishes a queued nil callback
// from an empty queue.
func (q *Queue) Pop() (func(), bool) {
	if q.head == nil {
		return nil, false
	}
	if q.head.readPos >= q.head.pos {
		if q.head == q.tail {
			q.head.pos = 0
			q.head.readPos = 0
			return nil, false
		}
		head := q.head
		q.head = q.head.next
		q.returnChunk(head)
	}
	if q.head.readPos >= q.head.pos {
		return nil, false
	}
	fn := q.head.tasks[q.head.readPos]
	q.head.tasks[q.head.readPos] = nil
	q.head.readPos++
	q.length--
	if q.head.readPos >= q.head.pos {
		if q.head == q.tail {
			q.head.pos = 0
			q.head.readPos = 0
			return fn, true
		}
		head := q.head
		q.head = q.head.next
		q.returnChunk(head)
	}
	return fn, true
}

// Length returns the number of queued callbacks.
func (q *Queue) Length() int {
	return q.length
}
