package eventloop

import "testing"

func TestRegistryNewPromisePublication(t *testing.T) {
	r := newRegistry()

	id, promise := r.NewPromise()

	r.mu.RLock()
	registered := r.data[id].Value()
	ring := append([]uint64(nil), r.ring...)
	head := r.head
	nextID := r.nextID
	r.mu.RUnlock()

	if id != 1 {
		t.Fatalf("first promise ID = %d, want 1", id)
	}
	if promise == nil {
		t.Fatal("first promise is nil")
	}
	if promise.State() != Pending {
		t.Fatalf("first promise state = %v, want Pending", promise.State())
	}
	if registered != promise {
		t.Fatalf("registered promise = %p, want %p", registered, promise)
	}
	if len(ring) != 1 || ring[0] != id {
		t.Fatalf("registry ring = %v, want [%d]", ring, id)
	}
	if head != 0 || nextID != 2 {
		t.Fatalf("registry cursor = (head %d, next ID %d), want (0, 2)", head, nextID)
	}
}

func TestRegistryNewPromiseWrapSkipsSentinelAndLiveIDs(t *testing.T) {
	r := newRegistry()

	firstID, first := r.NewPromise()
	if firstID != 1 {
		t.Fatalf("first ID = %d, want 1", firstID)
	}

	r.mu.Lock()
	r.nextID = ^uint64(0)
	r.mu.Unlock()

	maxID, maximum := r.NewPromise()
	if maxID != ^uint64(0) {
		t.Fatalf("maximum ID = %d, want %d", maxID, ^uint64(0))
	}
	wrappedID, wrapped := r.NewPromise()
	if wrappedID != 2 {
		t.Fatalf("wrapped ID = %d, want first free ID 2", wrappedID)
	}

	r.mu.RLock()
	firstRegistered := r.data[firstID].Value() == first
	maximumRegistered := r.data[maxID].Value() == maximum
	wrappedRegistered := r.data[wrappedID].Value() == wrapped
	_, sentinelRegistered := r.data[0]
	nextID := r.nextID
	r.mu.RUnlock()

	if !firstRegistered || !maximumRegistered || !wrappedRegistered {
		t.Fatalf("live registrations after wrap = first %v maximum %v wrapped %v, want all true", firstRegistered, maximumRegistered, wrappedRegistered)
	}
	if sentinelRegistered {
		t.Fatal("promise registry stored a live promise under sentinel ID 0")
	}
	if nextID != 3 {
		t.Fatalf("next ID after wrap = %d, want 3", nextID)
	}
}

func TestRegistryNewPromiseConcurrentIDs(t *testing.T) {
	const (
		workerCount       = 16
		promisesPerWorker = 64
	)

	type result struct {
		id      uint64
		promise *promise
	}

	r := newRegistry()
	results := make(chan result, workerCount*promisesPerWorker)
	for range workerCount {
		go func() {
			for range promisesPerWorker {
				id, promise := r.NewPromise()
				results <- result{id: id, promise: promise}
			}
		}()
	}

	registered := make(map[uint64]*promise, workerCount*promisesPerWorker)
	for range workerCount * promisesPerWorker {
		got := waitContractValue(t, results, "concurrent registry allocation")
		if got.id == 0 {
			t.Fatal("concurrent allocation published sentinel ID 0")
		}
		if got.promise == nil {
			t.Fatalf("concurrent allocation for ID %d published a nil promise", got.id)
		}
		if got.promise.State() != Pending {
			t.Fatalf("concurrent allocation for ID %d published state %v, want Pending", got.id, got.promise.State())
		}
		if previous := registered[got.id]; previous != nil {
			t.Fatalf("duplicate registry ID %d published promises %p and %p", got.id, previous, got.promise)
		}
		registered[got.id] = got.promise
	}

	r.mu.RLock()
	if len(r.data) != len(registered) || len(r.ring) != len(registered) {
		t.Fatalf("registry sizes = (data %d, ring %d), want (%d, %d)", len(r.data), len(r.ring), len(registered), len(registered))
	}
	for id, promise := range registered {
		if got := r.data[id].Value(); got != promise {
			t.Errorf("registry ID %d points to %p, want %p", id, got, promise)
		}
	}
	r.mu.RUnlock()
}
