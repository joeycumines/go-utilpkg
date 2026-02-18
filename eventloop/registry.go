package eventloop

import (
	"sync"
	"weak"
)

// registry tracks active promises using weak pointers to allow garbage collection.
// It uses a Ring Buffer strategy for efficient scavenging.
type registry struct {
	// data stores weak pointers to promises.
	data map[uint64]weak.Pointer[promise]

	// ring is a circular buffer of IDs used for scavenging.
	// It allows deterministic checking of all promises over time.
	ring []uint64

	// head is the current cursor position in the ring for the scavenger.
	head int

	// nextID is the counter for generating unique promise IDs.
	nextID uint64
	mu     sync.RWMutex

	// scavengeMu serializes scavenge operations to prevent overlap
	// and to ensure compaction safety.
	scavengeMu sync.Mutex
}

// newRegistry creates a new initialized registry.
func newRegistry() *registry {
	return &registry{
		data:   make(map[uint64]weak.Pointer[promise]),
		ring:   make([]uint64, 0, 1024), // Initial capacity
		nextID: 1,                       // Start at 1 so 0 is null marker
	}
}

// NewPromise creates a new promise, registers it, and returns the ID and the concrete promise.
func (r *registry) NewPromise() (uint64, *promise) {
	p := &promise{
		state: Pending,
	}

	wp := weak.Make(p)

	r.mu.Lock()
	defer r.mu.Unlock()

	id := r.nextID
	r.nextID++

	r.data[id] = wp
	r.ring = append(r.ring, id)

	return id, p
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

	// Calculate batch range
	start := r.head
	end := min(start+batchSize, ringLen)

	type item struct {
		id  uint64
		idx int
	}
	items := make([]item, 0, end-start)

	for i := start; i < end; i++ {
		id := r.ring[i]
		if id != 0 {
			items = append(items, item{id, i})
		}
	}

	wps := make([]weak.Pointer[promise], len(items))
	validItems := items[:0]

	for _, it := range items {
		if wp, ok := r.data[it.id]; ok {
			wps[len(validItems)] = wp
			validItems = append(validItems, it)
		}
	}
	wps = wps[:len(validItems)]

	nextHead := end
	if nextHead >= ringLen {
		nextHead = 0
	}
	r.mu.RUnlock()

	cycleCompleted := (nextHead == 0)

	// Perform Checks (OUTSIDE LOCK)
	var itemsToRemove []item

	for i, it := range validItems {
		wp := wps[i]
		val := wp.Value()

		// Remove if GC'd (nil) OR Settled (State != Pending)
		shouldRemove := false
		if val == nil {
			shouldRemove = true
		} else {
			if val.State() != Pending {
				shouldRemove = true
			}
		}

		if shouldRemove {
			itemsToRemove = append(itemsToRemove, it)
		}
	}

	// Perform Deletions (INSIDE LOCK)
	if len(itemsToRemove) > 0 || cycleCompleted {
		r.mu.Lock()

		for _, it := range itemsToRemove {
			delete(r.data, it.id)

			// Mark as 0 (Null Marker) in ring
			if it.idx < len(r.ring) && r.ring[it.idx] == it.id {
				r.ring[it.idx] = 0
			}
		}

		r.head = nextHead

		// Compaction
		if cycleCompleted {
			active := len(r.data)
			capacity := len(r.ring)

			// Trigger compaction when load factor < 25%
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
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, wp := range r.data {
		p := wp.Value()
		if p != nil && p.State() == Pending {
			p.Reject(err)
		}
		delete(r.data, id)
	}

	r.ring = r.ring[:0]
	r.head = 0
}

// compactAndRenew removes null markers from the ring buffer AND rebuilds the map.
// Go's delete() doesn't free hashmap bucket array; allocating a new map reclaims memory.
// Must be called with mu.Lock held.
func (r *registry) compactAndRenew() {
	newRing := make([]uint64, 0, len(r.data))
	newData := make(map[uint64]weak.Pointer[promise], len(r.data))

	for _, id := range r.ring {
		if id != 0 {
			if wp, ok := r.data[id]; ok {
				newRing = append(newRing, id)
				newData[id] = wp
			}
		}
	}

	r.ring = newRing
	r.data = newData
	r.head = 0
}
