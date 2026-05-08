// Package timerrefalwayssubmit materializes the considered strategy that
// always submits timer reference changes as closures, including on the owner.
package timerrefalwayssubmit

import (
	"sync"
	"sync/atomic"
)

type ID uint64

type entry struct {
	refed atomic.Bool
}

// Core isolates unconditional closure submission from lifecycle and wake
// policy. Seed, Drain, and Snapshot are untimed owner operations.
type Core struct {
	entries  map[ID]*entry
	refed    atomic.Int32
	queueMu  sync.Mutex
	queue    []func()
	consumed []func()
}

type Snapshot struct {
	Present    bool
	Refed      bool
	RefedCount int64
	Queued     int
}

func New() *Core {
	return &Core{entries: make(map[ID]*entry)}
}

func (c *Core) Seed(id ID, refed bool) bool {
	if _, exists := c.entries[id]; exists {
		return false
	}
	value := new(entry)
	value.refed.Store(refed)
	c.entries[id] = value
	if refed {
		c.refed.Add(1)
	}
	return true
}

// Apply always enqueues one closure and returns before owner application.
func (c *Core) Apply(id ID, refed bool) {
	c.queueMu.Lock()
	c.queue = append(c.queue, func() {
		c.apply(id, refed)
	})
	c.queueMu.Unlock()
}

func (c *Core) apply(id ID, refed bool) {
	value, exists := c.entries[id]
	if !exists {
		return
	}
	old := value.refed.Swap(refed)
	if old != refed {
		if refed {
			c.refed.Add(1)
		} else {
			c.refed.Add(-1)
		}
	}
}

func (c *Core) Drain() int {
	c.queueMu.Lock()
	batch := c.queue
	c.queue = c.consumed[:0]
	c.consumed = batch[:0]
	c.queueMu.Unlock()
	for _, task := range batch {
		task()
	}
	return len(batch)
}

func (c *Core) Snapshot(id ID) Snapshot {
	value, exists := c.entries[id]
	c.queueMu.Lock()
	queued := len(c.queue)
	c.queueMu.Unlock()
	return Snapshot{Present: exists, Refed: exists && value.refed.Load(), RefedCount: int64(c.refed.Load()), Queued: queued}
}
