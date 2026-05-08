// Package timerrefsyncmap materializes the considered sync.Map timer-reference
// strategy retained by the 802436f7 and 986e2378 benchmark sources.
package timerrefsyncmap

import (
	"sync"
	"sync/atomic"
)

type ID uint64

type entry struct {
	refed atomic.Bool
}

type Core struct {
	entries sync.Map
	refed   atomic.Int32
}

type Snapshot struct {
	Present    bool
	Refed      bool
	RefedCount int64
}

func New() *Core { return new(Core) }

func (c *Core) Seed(id ID, refed bool) bool {
	value := new(entry)
	value.refed.Store(refed)
	if _, loaded := c.entries.LoadOrStore(uint64(id), value); loaded {
		return false
	}
	if refed {
		c.refed.Add(1)
	}
	return true
}

func (c *Core) Remove(id ID) bool {
	loaded, exists := c.entries.LoadAndDelete(uint64(id))
	if !exists {
		return false
	}
	if loaded.(*entry).refed.Load() {
		c.refed.Add(-1)
	}
	return true
}

// Apply preserves sync.Map lookup, concrete entry assertion, atomic reference
// swap, and conditional Int32 aggregate update as the complete timed core.
func (c *Core) Apply(id ID, refed bool) {
	loaded, exists := c.entries.Load(uint64(id))
	if !exists {
		return
	}
	value := loaded.(*entry)
	old := value.refed.Swap(refed)
	if old != refed {
		if refed {
			c.refed.Add(1)
		} else {
			c.refed.Add(-1)
		}
	}
}

func (c *Core) Snapshot(id ID) Snapshot {
	loaded, exists := c.entries.Load(uint64(id))
	return Snapshot{Present: exists, Refed: exists && loaded.(*entry).refed.Load(), RefedCount: int64(c.refed.Load())}
}
