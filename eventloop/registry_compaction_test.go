package eventloop

import (
	"runtime"
	"testing"
)

func TestRegistryCompactionBelowLoadThreshold(t *testing.T) {
	r := newRegistry()
	kept := make(map[uint64]*promise, 30)
	for i := range 300 {
		id, promise := r.NewPromise()
		if i < 30 {
			kept[id] = promise
		} else {
			promise.resolve(nil)
		}
	}

	r.Scavenge(300)

	r.mu.RLock()
	if len(r.data) != len(kept) || len(r.ring) != len(kept) || r.head != 0 {
		t.Fatalf("compacted registry = (data %d, ring %d, head %d), want (%d, %d, 0)", len(r.data), len(r.ring), r.head, len(kept), len(kept))
	}
	for i, id := range r.ring {
		promise := kept[id]
		if promise == nil || r.data[id].Value() != promise {
			t.Errorf("compacted ring[%d] ID %d is not a retained promise", i, id)
		}
	}
	r.mu.RUnlock()
	runtime.KeepAlive(kept)
}

func TestRegistryCompactionRetainsHighLoadRing(t *testing.T) {
	r := newRegistry()
	kept := make(map[uint64]*promise, 50)
	settled := make([]uint64, 0, 50)
	for i := range 100 {
		id, promise := r.NewPromise()
		if i < 50 {
			kept[id] = promise
		} else {
			promise.resolve(nil)
			settled = append(settled, id)
		}
	}

	r.Scavenge(100)

	r.mu.RLock()
	if len(r.data) != len(kept) || len(r.ring) != 100 || r.head != 0 {
		t.Fatalf("high-load registry = (data %d, ring %d, head %d), want (%d, 100, 0)", len(r.data), len(r.ring), r.head, len(kept))
	}
	for id, promise := range kept {
		if got := r.data[id].Value(); got != promise {
			t.Errorf("retained registry ID %d points to %p, want %p", id, got, promise)
		}
	}
	for _, id := range settled {
		if _, exists := r.data[id]; exists {
			t.Errorf("settled registry ID %d survived high-load scavenge", id)
		}
	}
	r.mu.RUnlock()
	runtime.KeepAlive(kept)
}
