// Package timerrefownersubmit materializes the considered owner-test strategy
// that applies timer reference changes directly on the owner and submits a
// closure from external goroutines.
package timerrefownersubmit

import (
	"sync"
	"sync/atomic"

	"github.com/joeycumines/goroutineid"
)

type ID uint64

type entry struct {
	refed atomic.Bool
}

// Core isolates the considered owner-test plus submitted-closure topology.
// Queue draining and state inspection are untimed qualification operations.
type Core struct {
	entries  map[ID]*entry
	refed    atomic.Int32
	ownerID  atomic.Int64
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

// BindOwner binds the calling goroutine as the sole map owner.
func (c *Core) BindOwner() bool {
	id := goroutineid.Get()
	if id == 0 {
		return false
	}
	return c.ownerID.CompareAndSwap(0, id) || c.ownerID.Load() == id
}

func (c *Core) isOwner() bool {
	owner := c.ownerID.Load()
	return owner != 0 && goroutineid.Get() == owner
}

// Seed is an untimed owner-only qualification operation.
func (c *Core) Seed(id ID, refed bool) bool {
	if !c.isOwner() {
		return false
	}
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

// Apply preserves the considered branch: direct owner application, otherwise
// enqueue one closure. It excludes terminal admission, epoch, and wake policy.
func (c *Core) Apply(id ID, refed bool) {
	if c.isOwner() {
		c.apply(id, refed)
		return
	}
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

// Drain applies one detached queue batch on the owner.
func (c *Core) Drain() int {
	if !c.isOwner() {
		return 0
	}
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

// Snapshot is an untimed owner-only qualification operation.
func (c *Core) Snapshot(id ID) Snapshot {
	if !c.isOwner() {
		return Snapshot{}
	}
	value, exists := c.entries[id]
	c.queueMu.Lock()
	queued := len(c.queue)
	c.queueMu.Unlock()
	return Snapshot{Present: exists, Refed: exists && value.refed.Load(), RefedCount: int64(c.refed.Load()), Queued: queued}
}
