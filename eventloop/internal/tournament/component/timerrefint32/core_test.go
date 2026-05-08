package timerrefint32

import (
	"math"
	"testing"
)

func TestCoreTransitions(t *testing.T) {
	core := New()
	core.Apply(1, true)
	if got := core.Snapshot(1); got != (Snapshot{}) {
		t.Fatalf("missing snapshot = %+v", got)
	}
	if !core.Seed(0, true) || core.Seed(0, false) {
		t.Fatal("historical zero ID seed acceptance differs")
	}
	if got := core.Snapshot(0); got != (Snapshot{Present: true, Refed: true, RefedCount: 1}) {
		t.Fatalf("zero ID snapshot = %+v", got)
	}
	if !core.Remove(0) {
		t.Fatal("historical zero ID removal failed")
	}
	if !core.Seed(1, true) || core.Seed(1, false) || !core.Seed(2, false) {
		t.Fatal("seed acceptance differs")
	}
	if got := core.Snapshot(1); got != (Snapshot{Present: true, Refed: true, RefedCount: 1}) {
		t.Fatalf("initial snapshot = %+v", got)
	}
	core.Apply(1, true)
	if got := core.Snapshot(1); got.RefedCount != 1 {
		t.Fatalf("idempotent ref count = %d", got.RefedCount)
	}
	core.Apply(1, false)
	if got := core.Snapshot(1); got != (Snapshot{Present: true, Refed: false}) {
		t.Fatalf("unref snapshot = %+v", got)
	}
	core.Apply(1, false)
	core.Apply(2, true)
	if got := core.Snapshot(2); got != (Snapshot{Present: true, Refed: true, RefedCount: 1}) {
		t.Fatalf("ref snapshot = %+v", got)
	}
	if !core.Remove(2) || core.Remove(2) {
		t.Fatal("remove acceptance differs")
	}
	if got := core.Snapshot(1); got != (Snapshot{Present: true, Refed: false}) {
		t.Fatalf("final snapshot = %+v", got)
	}
}

func TestCoreInt32BoundaryIsHistorical(t *testing.T) {
	core := New()
	if !core.Seed(1, false) {
		t.Fatal("seed failed")
	}
	core.refed.Store(math.MaxInt32)
	core.Apply(1, true)
	if got := core.Snapshot(1); got != (Snapshot{Present: true, Refed: true, RefedCount: math.MinInt32}) {
		t.Fatalf("wrapped snapshot = %+v", got)
	}
}

func FuzzCoreTrace(f *testing.F) {
	f.Add([]byte{1, 1, 2, 3, 4, 5, 6})
	f.Add([]byte{2, 4, 4, 3, 6, 1, 5})
	f.Fuzz(func(t *testing.T, operations []byte) {
		core := New()
		model := make(map[ID]bool)
		for _, operation := range operations {
			id := ID(operation % 8)
			switch operation % 6 {
			case 0:
				accepted := core.Seed(id, false)
				_, exists := model[id]
				if accepted != !exists {
					t.Fatalf("Seed(%d) = %v, existed %v", id, accepted, exists)
				}
				if accepted {
					model[id] = false
				}
			case 1:
				accepted := core.Seed(id, true)
				_, exists := model[id]
				if accepted != !exists {
					t.Fatalf("Seed(%d) = %v, existed %v", id, accepted, exists)
				}
				if accepted {
					model[id] = true
				}
			case 2, 3:
				target := operation%2 == 0
				core.Apply(id, target)
				if _, exists := model[id]; exists {
					model[id] = target
				}
			default:
				removed := core.Remove(id)
				_, exists := model[id]
				if removed != exists {
					t.Fatalf("Remove(%d) = %v, existed %v", id, removed, exists)
				}
				delete(model, id)
			}
			assertModel(t, core, model)
		}
	})
}

func assertModel(t *testing.T, core *Core, model map[ID]bool) {
	t.Helper()
	count := int64(0)
	for id, refed := range model {
		if refed {
			count++
		}
		if got := core.Snapshot(id); got != (Snapshot{Present: true, Refed: refed, RefedCount: countSnapshot(core)}) {
			t.Fatalf("Snapshot(%d) = %+v", id, got)
		}
	}
	if got := countSnapshot(core); got != count {
		t.Fatalf("aggregate = %d, want %d", got, count)
	}
}

func countSnapshot(core *Core) int64 { return core.Snapshot(0).RefedCount }
