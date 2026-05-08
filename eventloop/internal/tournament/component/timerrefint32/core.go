// Package timerrefint32 materializes the historical owner-local timer
// reference transition introduced at cc005d72.
package timerrefint32

import "sync/atomic"

type ID uint64

type entry struct {
	refed atomic.Bool
}

// Core preserves the historical map lookup, atomic reference bit, and Int32
// aggregate. Its map is owner-local and is not safe for concurrent mutation.
type Core struct {
	entries map[ID]*entry
	refed   atomic.Int32
}

type Snapshot struct {
	Present    bool
	Refed      bool
	RefedCount int64
}

func New() *Core {
	return &Core{entries: make(map[ID]*entry)}
}

// Seed is an untimed qualification operation.
func (c *Core) Seed(id ID, refed bool) bool {
	if c.entries == nil {
		c.entries = make(map[ID]*entry)
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

// Remove is an untimed qualification operation modeling fire or cancellation.
func (c *Core) Remove(id ID) bool {
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

// Apply is the exact owner-local timed transition core. Missing IDs are
// ignored, and the aggregate changes only when the reference bit changes.
func (c *Core) Apply(id ID, refed bool) {
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

// Snapshot is an untimed qualification operation. It normalizes the historical
// aggregate width to Int64 without changing its Int32 wrap behavior.
func (c *Core) Snapshot(id ID) Snapshot {
	value, exists := c.entries[id]
	return Snapshot{
		Present:    exists,
		Refed:      exists && value.refed.Load(),
		RefedCount: int64(c.refed.Load()),
	}
}
