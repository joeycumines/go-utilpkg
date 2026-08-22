package eventloop

import (
	"runtime"
	"testing"
	"weak"
)

func TestRegistryCompactionBelowLoadThreshold(t *testing.T) {
	r := newRegistry()
	kept := make(map[weak.Pointer[promise]]*promise, 30)
	var allWPs []weak.Pointer[promise]
	for i := range 300 {
		p := r.NewPromise()
		wp := weak.Make(p)
		allWPs = append(allWPs, wp)
		if i < 30 {
			kept[wp] = p
		} else {
			p.resolve(nil)
		}
	}

	r.Scavenge(300)

	r.mu.RLock()
	if len(r.data) != len(kept) || len(r.ring) != len(kept) || r.head != 0 {
		t.Fatalf("compacted registry = (data %d, ring %d, head %d), want (%d, %d, 0)", len(r.data), len(r.ring), r.head, len(kept), len(kept))
	}
	for i, wp := range r.ring {
		promise := kept[wp]
		if promise == nil || wp.Value() != promise {
			t.Errorf("compacted ring[%d] wp %v is not a retained promise", i, wp)
		}
		if _, ok := r.data[wp]; !ok {
			t.Errorf("compacted ring wp %v not in data", wp)
		}
	}
	r.mu.RUnlock()
	runtime.KeepAlive(kept)
	runtime.KeepAlive(allWPs)
}

func TestRegistryCompactionRetainsHighLoadRing(t *testing.T) {
	r := newRegistry()
	kept := make(map[weak.Pointer[promise]]*promise, 50)
	settled := make([]weak.Pointer[promise], 0, 50)
	for i := range 100 {
		p := r.NewPromise()
		wp := weak.Make(p)
		if i < 50 {
			kept[wp] = p
		} else {
			p.resolve(nil)
			settled = append(settled, wp)
		}
	}

	r.Scavenge(100)

	r.mu.RLock()
	if len(r.data) != len(kept) || len(r.ring) != 100 || r.head != 0 {
		t.Fatalf("high-load registry = (data %d, ring %d, head %d), want (%d, 100, 0)", len(r.data), len(r.ring), r.head, len(kept))
	}
	for wp, promise := range kept {
		if _, ok := r.data[wp]; !ok {
			t.Errorf("retained registry wp %v missing", wp)
		}
		if wp.Value() != promise {
			t.Errorf("retained wp %v points to %p, want %p", wp, wp.Value(), promise)
		}
	}
	for _, wp := range settled {
		if _, exists := r.data[wp]; exists {
			t.Errorf("settled registry wp %v survived high-load scavenge", wp)
		}
	}
	r.mu.RUnlock()
	runtime.KeepAlive(kept)
}
