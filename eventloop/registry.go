package eventloop

import (
	"sync"
	"weak"
)

// registry tracks active promises using weak pointers to allow garbage collection.
// It uses a Ring Buffer strategy for efficient scavenging.
type registry struct {
	// data stores weak pointers to promises as membership set.
	data map[weak.Pointer[promise]]struct{}

	// ring is a circular buffer of weak pointers used for scavenging.
	ring []weak.Pointer[promise]

	// head is the current cursor position in the ring for the scavenger.
	head int

	mu sync.RWMutex

	// scavengeMu serializes scavenge operations to prevent overlap
	// and to ensure compaction safety.
	scavengeMu sync.Mutex
}

// newRegistry creates a new initialized registry.
func newRegistry() *registry {
	return &registry{
		data: make(map[weak.Pointer[promise]]struct{}),
		ring: make([]weak.Pointer[promise], 0, 1024),
	}
}

// NewPromise creates a new promise, registers it, and returns the concrete promise.
func (r *registry) NewPromise() *promise {
	p := &promise{
		state: Pending,
	}
	wp := weak.Make(p)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.data == nil {
		r.data = make(map[weak.Pointer[promise]]struct{})
	}
	r.data[wp] = struct{}{}
	r.ring = append(r.ring, wp)
	return p
}

// Scavenge performs a partial cleanup of dead promises.
// It iterates through a batch of the ring buffer, checking for GC'd or Settled promises.
func (r *registry) Scavenge(batchSize int) {
	r.scavengeMu.Lock()
	defer r.scavengeMu.Unlock()

	if batchSize <= 0 {
		return
	}

	r.mu.RLock()
	ringLen := len(r.ring)
	if ringLen == 0 {
		r.mu.RUnlock()
		return
	}

	start := r.head
	end := min(start+batchSize, ringLen)

	type item struct {
		wp  weak.Pointer[promise]
		idx int
	}
	items := make([]item, 0, end-start)

	for i := start; i < end; i++ {
		wp := r.ring[i]
		if wp != (weak.Pointer[promise]{}) {
			items = append(items, item{wp, i})
		}
	}

	validItems := items[:0]
	for _, it := range items {
		if _, ok := r.data[it.wp]; ok {
			validItems = append(validItems, it)
		}
	}

	nextHead := end
	if nextHead >= ringLen {
		nextHead = 0
	}
	r.mu.RUnlock()

	cycleCompleted := nextHead == 0

	itemsToRemove := validItems[:0]

	for _, it := range validItems {
		val := it.wp.Value()
		if val == nil || val.State() != Pending {
			itemsToRemove = append(itemsToRemove, it)
		}
	}

	if len(itemsToRemove) > 0 || cycleCompleted {
		r.mu.Lock()
		for _, it := range itemsToRemove {
			delete(r.data, it.wp)
			if it.idx < len(r.ring) && r.ring[it.idx] == it.wp {
				r.ring[it.idx] = weak.Pointer[promise]{}
			}
		}
		r.head = nextHead
		if cycleCompleted {
			active := len(r.data)
			capacity := len(r.ring)
			if capacity > 256 && float64(active) < float64(capacity)*0.25 {
				r.compactAndRenew()
			}
		}
		r.mu.Unlock()
	} else {
		r.mu.Lock()
		r.head = nextHead
		r.mu.Unlock()
	}
}

// RejectAll rejects all pending promises with the given error.
// Called during shutdown to ensure no promises hang indefinitely.
func (r *registry) RejectAll(err error) {
	r.scavengeMu.Lock()
	defer r.scavengeMu.Unlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	for wp := range r.data {
		p := wp.Value()
		if p != nil && p.State() == Pending {
			p.reject(err)
		}
		delete(r.data, wp)
	}

	r.data = nil
	r.ring = nil
	r.head = 0
}

// compactAndRenew removes null markers from the ring buffer AND rebuilds the map.
// Go's delete() doesn't free hashmap bucket array; allocating a new map reclaims memory.
// Must be called with mu.Lock held.
func (r *registry) compactAndRenew() {
	newRing := make([]weak.Pointer[promise], 0, len(r.data))
	newData := make(map[weak.Pointer[promise]]struct{}, len(r.data))

	for _, wp := range r.ring {
		if wp != (weak.Pointer[promise]{}) {
			if _, ok := r.data[wp]; ok {
				newRing = append(newRing, wp)
				newData[wp] = struct{}{}
			}
		}
	}

	r.ring = newRing
	r.data = newData
	r.head = 0
}
