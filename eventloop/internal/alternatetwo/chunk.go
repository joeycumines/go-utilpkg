package alternatetwo

import "sync"

// chunkSize is the fixed size of each chunk in the chunked linked-list.
const chunkSize = 128

// Task represents a unit of work submitted to the event loop.
type Task struct {
	Fn func()
}

// chunk is a fixed-size node in the chunked linked-list.
// PERFORMANCE: Minimal clearing on return to pool.
type chunk struct {
	next    *chunk
	tasks   [chunkSize]Task
	readPos int
	pos     int
}

// chunkPool pools chunks for reuse.
var chunkPool = sync.Pool{
	New: func() any {
		return &chunk{}
	},
}

// newChunk gets a chunk from the pool, minimally initialized.
func newChunk() *chunk {
	c := chunkPool.Get().(*chunk)
	c.pos = 0
	c.readPos = 0
	c.next = nil
	return c
}

// returnChunkFast returns a chunk to the pool with minimal clearing.
// PERFORMANCE: Only clears used slots (pos), not all 128.
// This reduces cache line touches and improves throughput.
func returnChunkFast(c *chunk) {
	// PERFORMANCE: Only clear up to pos (used slots)
	for i := 0; i < c.pos; i++ {
		c.tasks[i] = Task{}
	}
	c.pos = 0
	c.readPos = 0
	c.next = nil
	chunkPool.Put(c)
}
