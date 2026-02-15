package alternatethree

import (
	"sync"
	"weak"
)

// registry tracks active promises using weak pointers to allow garbage collection.
// It uses a Ring Buffer strategies for efficient scavenging.
type registry struct {

	// data stores the weak pointers to promises.
	// We use the concrete type *promise for type precision (Task 2.4).
	data map[uint64]weak.Pointer[promise]

	// ring is a circular buffer of IDs used for scavenging.
	// It allows deterministic checking of all promises over time.
	ring []uint64

	// head is the current cursor position in the ring for the scavenger.
	head int

	// nextID is the counter for generating unique promise IDs.
	nextID uint64
	mu     sync.RWMutex

	// scavengeMu serializes scavenge operations to prevent overlap (Task 2.3)
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
// It implements Task 2.2 (Proper Locking) and Task 2.4 (Weak Pointer Type Constraint).
func (r *registry) NewPromise() (uint64, *promise) {
	// Create the concrete promise
	p := &promise{
		state: Pending,
	}

	// Task 2.2: Ensure weak.Make is called OUTSIDE the lock if possible (though it's fast).
	// But critically, map insertion MUST be locked.
	// Task 2.4: weak.Make is called with *promise (concrete type).
	wp := weak.Make(p)

	r.mu.Lock()
	defer r.mu.Unlock()

	id := r.nextID
	r.nextID++

	// Register in map
	r.data[id] = wp

	// Add to ring buffer for scavenging
	r.ring = append(r.ring, id)

	return id, p
}

// Scavenge performs a partial cleanup of dead promises.
// It iterates through a batch of the ring buffer, checking for GC'd or Settled promises.
// Implements Task 2.3 (Proper Locking in Scavenge) and Phase 5 (Ring Buffer Scavenger Fix).
func (r *registry) Scavenge(batchSize int) {
	// Task 2.3: Multiple scavenge operations never overlap.
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

	// Collect IDs to check.
	// Phase 5.1: Null-Marker Strategy (Skip 0s)
	// We also verify map existence to handle consistency.
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

	// Optimize: Resolve weak pointers while holding RLock?
	// Or resolve them now?
	// Blueprint 2.3 says: "No Lock() is held during weak.Value() check".
	// But we need to get the WeakPointer from map first.
	// We do that under RLock properly.
	wps := make([]weak.Pointer[promise], len(items))
	// Valid items filter
	validItems := items[:0]

	for _, it := range items {
		if wp, ok := r.data[it.id]; ok {
			wps[len(validItems)] = wp
			validItems = append(validItems, it)
		} else {
			// Orphan usage? Or deleted?
			// If not in map, it should be 0 in ring.
			// Next compaction will clean it if we mark it 0.
			// But we need Write Lock to mark 0.
			// Ignore for now.
		}
	}
	// Truncate wps match validItems
	wps = wps[:len(validItems)]

	// Determine next head
	nextHead := end
	if nextHead >= ringLen {
		nextHead = 0
	}
	r.mu.RUnlock()

	// Check for Cycle Completion (for compaction trigger)
	cycleCompleted := (nextHead == 0)

	// Perform Checks (OUTSIDE LOCK)
	var itemsToRemove []item

	for i, it := range validItems {
		wp := wps[i]
		val := wp.Value()

		// Phase 5.3: Scavenger Pruning Logic
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

		// 1. Process Removals
		for _, it := range itemsToRemove {
			// Double check? No, Value() is authoritative for GC.
			// And State definition assumes monotonic transition to Settled?
			// Yes, once Settled, stays Settled.

			delete(r.data, it.id)

			// Phase 5.1: Mark as 0 (Null Marker)
			// Verify index is still valid (safe because scavengeMu and we hold mu.Lock)
			if it.idx < len(r.ring) && r.ring[it.idx] == it.id {
				r.ring[it.idx] = 0
			}
		}

		// 2. Update Head
		r.head = nextHead

		// 3. Compaction (Phase 5.2)
		if cycleCompleted {
			// Load Factor Calculation
			active := len(r.data)
			capacity := len(r.ring)

			// D20: Trigger compaction when load factor < 25% (not 12.5%)
			if capacity > 256 && float64(active) < float64(capacity)*0.25 {
				// D11: Use compactAndRenew to rebuild both ring AND map
				r.compactAndRenew()
				// head is already 0
			}
		}

		r.mu.Unlock()
	} else {
		// Just update head
		r.mu.Lock()
		r.head = nextHead
		r.mu.Unlock()
	}
}

// RejectAll rejects all pending promises with the given error (D16).
// Called during shutdown to ensure no promises hang indefinitely.
func (r *registry) RejectAll(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, wp := range r.data {
		p := wp.Value()
		if p != nil && p.State() == Pending {
			p.Reject(err)
		}
		// Mark as removed
		delete(r.data, id)
	}

	// Clear ring
	r.ring = r.ring[:0]
	r.head = 0
}

// compactAndRenew removes null markers from the ring buffer AND rebuilds the map.
// D11: Go's delete() doesn't free hashmap bucket array, causing unbounded memory growth.
// This function allocates a NEW map to reclaim that memory.
// Must be called with mu.Lock held.
func (r *registry) compactAndRenew() {
	// Build new ring with only active entries
	newRing := make([]uint64, 0, len(r.data))
	// D11: Allocate NEW map to free old bucket array
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
