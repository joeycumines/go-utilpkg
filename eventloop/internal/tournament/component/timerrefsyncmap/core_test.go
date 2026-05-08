package timerrefsyncmap

import (
	"sync"
	"testing"
)

func TestCoreTransitions(t *testing.T) {
	core := New()
	core.Apply(1, true)
	if !core.Seed(0, true) || core.Seed(0, false) || !core.Seed(1, false) {
		t.Fatal("seed acceptance differs")
	}
	if got := core.Snapshot(0); got != (Snapshot{Present: true, Refed: true, RefedCount: 1}) {
		t.Fatalf("zero-ID snapshot = %+v", got)
	}
	core.Apply(1, true)
	core.Apply(1, true)
	if got := core.Snapshot(1); got != (Snapshot{Present: true, Refed: true, RefedCount: 2}) {
		t.Fatalf("referenced snapshot = %+v", got)
	}
	core.Apply(0, false)
	if got := core.Snapshot(0); got != (Snapshot{Present: true, RefedCount: 1}) {
		t.Fatalf("unreferenced snapshot = %+v", got)
	}
	if !core.Remove(1) || core.Remove(1) {
		t.Fatal("remove acceptance differs")
	}
	if got := core.Snapshot(0); got != (Snapshot{Present: true}) {
		t.Fatalf("final snapshot = %+v", got)
	}
}

func TestCoreConcurrentApply(t *testing.T) {
	core := New()
	const entries = 32
	for id := range entries {
		if !core.Seed(ID(id), false) {
			t.Fatalf("seed %d failed", id)
		}
	}
	var wait sync.WaitGroup
	wait.Add(entries)
	for id := range entries {
		go func() {
			defer wait.Done()
			for range 100 {
				core.Apply(ID(id), true)
				core.Apply(ID(id), false)
			}
		}()
	}
	wait.Wait()
	for id := range entries {
		if got := core.Snapshot(ID(id)); got != (Snapshot{Present: true}) {
			t.Fatalf("snapshot %d = %+v", id, got)
		}
	}
}

func FuzzCoreTrace(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5})
	f.Fuzz(func(t *testing.T, operations []byte) {
		core := New()
		model := make(map[ID]bool)
		for _, operation := range operations {
			id := ID(operation % 8)
			switch operation % 5 {
			case 0, 1:
				refed := operation%2 == 0
				accepted := core.Seed(id, refed)
				_, exists := model[id]
				if accepted != !exists {
					t.Fatalf("Seed(%d) = %v, existed %v", id, accepted, exists)
				}
				if accepted {
					model[id] = refed
				}
			case 2, 3:
				refed := operation%2 == 0
				core.Apply(id, refed)
				if _, exists := model[id]; exists {
					model[id] = refed
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
	wantCount := int64(0)
	for _, refed := range model {
		if refed {
			wantCount++
		}
	}
	for id, refed := range model {
		if got := core.Snapshot(id); got != (Snapshot{Present: true, Refed: refed, RefedCount: wantCount}) {
			t.Fatalf("Snapshot(%d) = %+v", id, got)
		}
	}
	if got := core.Snapshot(ID(^uint64(0))).RefedCount; got != wantCount {
		t.Fatalf("aggregate = %d, want %d", got, wantCount)
	}
}
