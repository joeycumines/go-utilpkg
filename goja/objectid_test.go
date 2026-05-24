package goja

import (
	"runtime"
	"sync"
	"testing"
)

// Tests for Object.getId() — the lazily-assigned monotonic identity counter
// that replaced the object's heap address as WeakMap/Map/Set identity
// (see Object.objID in object.go). The contract pinned here:
//
//   - every assigned ID is non-zero (0 is the unassigned sentinel);
//   - an object's ID is stable across calls;
//   - distinct objects never share an ID, even across GC cycles (an
//     address-based scheme can reuse a collected object's identity before
//     its weak-map cleanup runs — the ABA hazard);
//   - assignment is safe from any goroutine (runtime.AddCleanup may run on
//     non-runtime goroutines), verified under -race.

func TestObjectIDAssignment(t *testing.T) {
	vm := New()
	obj := vm.NewObject()
	id := obj.getId()
	if id == 0 {
		t.Fatal("fresh object ID = 0, want non-zero")
	}
	if again := obj.getId(); again != id {
		t.Fatalf("object ID changed across calls: first %d, second %d", id, again)
	}
	other := vm.NewObject()
	if otherID := other.getId(); otherID == id {
		t.Fatalf("distinct objects share ID %d", id)
	}
}

// Objects created and dropped across GC cycles must never receive an ID that
// was previously assigned to a collected object. This is the property the
// heap-address scheme violated: a new allocation could reuse a dead object's
// address while its weak-map cleanup was still pending.
func TestObjectIDNeverReusedAcrossGC(t *testing.T) {
	vm := New()
	used := make(map[uint64]struct{})
	for batch := 0; batch < 64; batch++ {
		ids := make([]uint64, 0, 8)
		for range 8 {
			obj := vm.NewObject()
			id := obj.getId()
			if id == 0 {
				t.Fatal("assigned object ID = 0")
			}
			ids = append(ids, id)
		}
		// All objects of this batch are now unreachable; force collection so
		// the next batch cannot be confused with them.
		runtime.GC()
		for _, id := range ids {
			if _, exists := used[id]; exists {
				t.Fatalf("object ID %d was previously assigned and reused after GC (address-identity ABA)", id)
			}
			used[id] = struct{}{}
		}
	}
}

// getId is a mutating lazy assignment and must be safe when called from any
// goroutine. This exercises the atomic load/compare-and-swap path with
// concurrent first-assignment on shared objects, plus concurrent assignment
// on distinct objects (uniqueness). Run under -race.
func TestObjectIDConcurrentAssignment(t *testing.T) {
	vm := New()
	objects := make([]*Object, 8)
	for i := range objects {
		objects[i] = vm.NewObject()
	}
	const workers = 8
	const rounds = 1000
	results := make([][]uint64, workers)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			results[w] = make([]uint64, len(objects)*rounds)
			for r := 0; r < rounds; r++ {
				for i, obj := range objects {
					results[w][i*rounds+r] = obj.getId()
				}
			}
		}(w)
	}
	wg.Wait()

	distinct := make(map[uint64]struct{}, len(objects))
	for i, obj := range objects {
		first := results[0][i*rounds]
		if first == 0 {
			t.Fatalf("object %d assigned ID 0", i)
		}
		if _, exists := distinct[first]; exists {
			t.Fatalf("distinct objects concurrently assigned the same ID %d", first)
		}
		distinct[first] = struct{}{}
		for w := range workers {
			for r := 0; r < rounds; r++ {
				if got := results[w][i*rounds+r]; got != first {
					t.Fatalf("object %d (%p) ID unstable under concurrent access: got %d, first %d", i, obj, got, first)
				}
			}
		}
	}
}

// Behavioral regression for the weak-map ABA hazard: a fresh WeakMap key must
// never observe an entry left behind by a collected key whose cleanup has not
// yet run. WeakMap entries live in a strong Go map keyed by getId()
// (builtin_weakmap.go) and are deleted by a per-key runtime.AddCleanup, so
// identity reuse — exactly what the counter prevents — would make a new key
// read stale data and corrupt its own new entry.
func TestWeakMapNoCrossObjectContamination(t *testing.T) {
	vm := New()
	if _, err := vm.RunString(`
		var wm = new WeakMap();
		var keyA = {};
		wm.set(keyA, 'stale');
		keyA = null;
		void 0;
	`); err != nil {
		t.Fatal(err)
	}
	// Force collection of keyA. Its cleanup may still be pending — precisely
	// the window in which an address-based scheme would alias a new
	// allocation onto the stale entry.
	for range 5 {
		runtime.GC()
	}
	value, err := vm.RunString(`
		var keyB = {};
		var stale = wm.get(keyB);
		wm.set(keyB, 'fresh');
		var fresh = wm.get(keyB);
		var gone = stale === undefined && fresh === 'fresh';
		keyB = null;
		gone;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !value.ToBoolean() {
		t.Fatal("fresh WeakMap key observed a stale entry from a collected key (weak-map ABA)")
	}
}
