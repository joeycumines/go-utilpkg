package timerrefrwmutex

import (
	"sync"
	"testing"
)

func TestCoreQualificationBarrierAndTransitions(t *testing.T) {
	missing := New()
	if !missing.Seal() {
		t.Fatal("missing-core seal failed")
	}
	missing.Apply(1, true)
	if got := missing.Snapshot(1); got != (Snapshot{Sealed: true}) {
		t.Fatalf("missing snapshot = %+v", got)
	}

	core := New()
	if !core.Seed(0, true) || core.Seed(0, false) || !core.Seed(1, false) {
		t.Fatal("seed acceptance differs")
	}
	if !core.Remove(0) || core.Remove(0) {
		t.Fatal("pre-seal remove acceptance differs")
	}
	if !core.Seal() || core.Seal() || core.Seed(2, true) || core.Remove(1) {
		t.Fatal("registration barrier acceptance differs")
	}
	core.Apply(1, true)
	core.Apply(1, true)
	if got := core.Snapshot(1); got != (Snapshot{Present: true, Refed: true, RefedCount: 1, Sealed: true}) {
		t.Fatalf("referenced snapshot = %+v", got)
	}
	core.Apply(1, false)
	if got := core.Snapshot(1); got != (Snapshot{Present: true, Sealed: true}) {
		t.Fatalf("unreferenced snapshot = %+v", got)
	}
}

func TestCoreConcurrentApplyAfterBarrier(t *testing.T) {
	core := New()
	const entries = 32
	for id := range entries {
		if !core.Seed(ID(id), false) {
			t.Fatalf("seed %d failed", id)
		}
	}
	if !core.Seal() {
		t.Fatal("seal failed")
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
		if got := core.Snapshot(ID(id)); got != (Snapshot{Present: true, Sealed: true}) {
			t.Fatalf("snapshot %d = %+v", id, got)
		}
	}
}

func FuzzCoreTraceBeforeBarrier(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5})
	f.Fuzz(func(t *testing.T, operations []byte) {
		core := New()
		model := make(map[ID]bool)
		for _, operation := range operations {
			id := ID(operation % 8)
			switch operation % 3 {
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
			default:
				removed := core.Remove(id)
				_, exists := model[id]
				if removed != exists {
					t.Fatalf("Remove(%d) = %v, existed %v", id, removed, exists)
				}
				delete(model, id)
			}
			assertModel(t, core, model, false)
		}
		if !core.Seal() {
			t.Fatal("seal failed")
		}
		for id, refed := range model {
			core.Apply(id, !refed)
			model[id] = !refed
		}
		assertModel(t, core, model, true)
	})
}

func assertModel(t *testing.T, core *Core, model map[ID]bool, sealed bool) {
	t.Helper()
	wantCount := int64(0)
	for _, refed := range model {
		if refed {
			wantCount++
		}
	}
	for id, refed := range model {
		if got := core.Snapshot(id); got != (Snapshot{Present: true, Refed: refed, RefedCount: wantCount, Sealed: sealed}) {
			t.Fatalf("Snapshot(%d) = %+v", id, got)
		}
	}
	if got := core.Snapshot(ID(^uint64(0))); got.RefedCount != wantCount || got.Sealed != sealed {
		t.Fatalf("aggregate snapshot = %+v, want count %d sealed %v", got, wantCount, sealed)
	}
}
