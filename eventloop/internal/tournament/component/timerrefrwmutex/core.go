// Package timerrefrwmutex materializes the considered RWMutex-protected map
// lookup strategy retained by the 802436f7 and 986e2378 benchmark sources.
package timerrefrwmutex

import (
	"sync"
	"sync/atomic"
)

type ID uint64

type entry struct {
	refed atomic.Bool
}

// Core requires Seal before timed Apply calls. The seal is the explicit
// registration/quiescence barrier missing from the historical benchmark body;
// it prevents concurrent map mutation from invalidating its RLock-only lookup.
type Core struct {
	entries map[ID]*entry
	mu      sync.RWMutex
	refed   atomic.Int32
	sealed  atomic.Bool
}

type Snapshot struct {
	Present    bool
	Refed      bool
	RefedCount int64
	Sealed     bool
}

func New() *Core {
	return &Core{entries: make(map[ID]*entry)}
}

func (c *Core) Seed(id ID, refed bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealed.Load() {
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

func (c *Core) Remove(id ID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealed.Load() {
		return false
	}
	value, exists := c.entries[id]
	if !exists {
		return false
	}
	delete(c.entries, id)
	if value.refed.Load() {
		c.refed.Add(-1)
	}
	return true
}

// Seal establishes the registration/quiescence barrier required before Apply.
func (c *Core) Seal() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sealed.CompareAndSwap(false, true)
}

// Apply preserves RLock lookup, atomic reference swap, and conditional Int32
// aggregate update. Callers must establish Seal outside the timed region.
func (c *Core) Apply(id ID, refed bool) {
	c.mu.RLock()
	value, exists := c.entries[id]
	c.mu.RUnlock()
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

func (c *Core) Snapshot(id ID) Snapshot {
	c.mu.RLock()
	value, exists := c.entries[id]
	c.mu.RUnlock()
	return Snapshot{Present: exists, Refed: exists && value.refed.Load(), RefedCount: int64(c.refed.Load()), Sealed: c.sealed.Load()}
}
