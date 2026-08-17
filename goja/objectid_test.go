package goja

import (
	"runtime"
	"sync"
	"testing"
)

// Tests for Object.getId(). The required properties are:
//   - IDs are non-zero (0 is unassigned);
//   - An object's ID is stable;
//   - Distinct objects never share an ID, even across GC cycles;
//   - Assignment is safe from concurrent access.

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

// TestObjectIDNeverReusedAcrossGC verifies that IDs are not reused for new
// allocations after previous objects are garbage collected.
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
		// Force collection of the current batch.
		runtime.GC()
		for _, id := range ids {
			if _, exists := used[id]; exists {
				t.Fatalf("object ID %d reused after GC", id)
			}
			used[id] = struct{}{}
		}
	}
}

// TestObjectIDConcurrentAssignment verifies that concurrent lazy assignments
// on shared and distinct objects are safe and unique.
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

// TestWeakMapNoCrossObjectContamination verifies that a fresh WeakMap key
// cannot access an entry belonging to a collected key whose cleanup has not
// yet executed.
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
	// Force collection of keyA to ensure its pending cleanup does not
	// alias with a new allocation.
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
		t.Fatal("fresh WeakMap key observed a stale entry from a collected key")
	}
}
