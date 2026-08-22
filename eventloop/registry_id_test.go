package eventloop

import (
	"testing"
	"weak"
)

func TestRegistryNewPromisePublication(t *testing.T) {
	r := newRegistry()

	p := r.NewPromise()
	wp := weak.Make(p)

	r.mu.RLock()
	_, exists := r.data[wp]
	ring := append([]weak.Pointer[promise](nil), r.ring...)
	head := r.head
	r.mu.RUnlock()

	if p == nil {
		t.Fatal("first promise is nil")
	}
	if p.State() != Pending {
		t.Fatalf("first promise state = %v, want Pending", p.State())
	}
	if !exists {
		t.Fatalf("registered promise not found in data via weak pointer")
	}
	if len(ring) != 1 || ring[0] != wp {
		t.Fatalf("registry ring = %v, want [%v]", ring, wp)
	}
	if head != 0 {
		t.Fatalf("registry head = %d, want 0", head)
	}
	if wp.Value() != p {
		t.Fatalf("weak pointer value = %p, want %p", wp.Value(), p)
	}
}

func TestRegistryNewPromiseUniquePointers(t *testing.T) {
	r := newRegistry()

	first := r.NewPromise()
	second := r.NewPromise()

	if first == second {
		t.Fatalf("first and second promise same pointer %p", first)
	}
	wp1 := weak.Make(first)
	wp2 := weak.Make(second)
	if wp1 == wp2 {
		t.Fatalf("weak pointers equal for distinct promises: %v", wp1)
	}

	r.mu.RLock()
	_, ok1 := r.data[wp1]
	_, ok2 := r.data[wp2]
	r.mu.RUnlock()

	if !ok1 || !ok2 {
		t.Fatalf("live registrations after second alloc missing: first %v second %v", ok1, ok2)
	}
	// Ensure zero sentinel not used
	r.mu.RLock()
	_, sentinelRegistered := r.data[weak.Pointer[promise]{}]
	r.mu.RUnlock()
	if sentinelRegistered {
		t.Fatal("promise registry stored entry under zero weak pointer sentinel")
	}
}

func TestRegistryNewPromiseConcurrentIDs(t *testing.T) {
	const (
		workerCount       = 16
		promisesPerWorker = 64
	)

	type result struct {
		wp      weak.Pointer[promise]
		promise *promise
	}

	r := newRegistry()
	results := make(chan result, workerCount*promisesPerWorker)
	for range workerCount {
		go func() {
			for range promisesPerWorker {
				promise := r.NewPromise()
				wp := weak.Make(promise)
				results <- result{wp: wp, promise: promise}
			}
		}()
	}

	registered := make(map[weak.Pointer[promise]]*promise, workerCount*promisesPerWorker)
	for range workerCount * promisesPerWorker {
		got := waitContractValue(t, results, "concurrent registry allocation")
		if got.wp == (weak.Pointer[promise]{}) {
			t.Fatal("concurrent allocation published zero weak pointer sentinel")
		}
		if got.promise == nil {
			t.Fatalf("concurrent allocation for wp %v published a nil promise", got.wp)
		}
		if got.promise.State() != Pending {
			t.Fatalf("concurrent allocation for wp %v published state %v, want Pending", got.wp, got.promise.State())
		}
		if got.wp.Value() != got.promise {
			t.Fatalf("weak pointer value mismatch: wp.Value()=%p want %p", got.wp.Value(), got.promise)
		}
		if previous := registered[got.wp]; previous != nil {
			t.Fatalf("duplicate registry weak pointer %v published promises %p and %p", got.wp, previous, got.promise)
		}
		registered[got.wp] = got.promise
	}

	r.mu.RLock()
	if len(r.data) != len(registered) || len(r.ring) != len(registered) {
		t.Fatalf("registry sizes = (data %d, ring %d), want (%d, %d)", len(r.data), len(r.ring), len(registered), len(registered))
	}
	for wp, promise := range registered {
		if got := wp.Value(); got != promise {
			t.Errorf("registry wp %v points to %p, want %p", wp, got, promise)
		}
		if _, ok := r.data[wp]; !ok {
			t.Errorf("registry missing wp %v", wp)
		}
	}
	r.mu.RUnlock()
}
