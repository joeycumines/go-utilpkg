package timerbucketretire

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestNativeIDBoundaryRetirementAndWrapDefect(t *testing.T) {
	epoch := time.Unix(1, 0)
	queue := NewNative(epoch)
	queue.nextID.Store(maxSafeInteger - 1)
	if handle, err := queue.Insert(InsertInput{When: epoch}); handle != maxSafeInteger || err != nil {
		t.Fatalf("maximum valid Insert = (%d, %v)", handle, err)
	}
	retired := 0
	for attempt := range 2 {
		if handle, err := queue.Insert(InsertInput{When: epoch, Retire: func() { retired++ }}); handle != 0 || !errors.Is(err, component.ErrTimerExhausted) {
			t.Fatalf("exhausted Insert %d = (%d, %v)", attempt, handle, err)
		}
	}
	if retired != 2 {
		t.Fatalf("rejected retire count = %d, want 2", retired)
	}
	wantCounter := uint64(maxSafeInteger + 2)
	queue.resetQuiescent()
	if got := queue.nextID.Load(); got != wantCounter {
		t.Fatalf("nextID after cleanup = %d, want %d", got, wantCounter)
	}
	queue.nextID.Store(math.MaxUint64)
	if handle, err := queue.Insert(InsertInput{When: epoch}); handle != 0 || err != nil {
		t.Fatalf("wrapped Insert = (%d, %v), want historical accepted zero", handle, err)
	}
	queue.resetQuiescent()
}

func TestNativeIDWrapCollisionOverwritesMapOwnership(t *testing.T) {
	epoch := time.Unix(1, 0)
	queue := NewNative(epoch)
	retired := [3]int{}
	first, err := queue.Insert(InsertInput{When: epoch, Retire: func() { retired[0]++ }})
	if err != nil || first != 1 {
		t.Fatalf("first Insert = (%d, %v), want (1, nil)", first, err)
	}
	original := queue.entries[first]
	queue.nextID.Store(math.MaxUint64)
	zero, err := queue.Insert(InsertInput{When: epoch, Retire: func() { retired[1]++ }})
	if err != nil || zero != 0 {
		t.Fatalf("wrapped Insert = (%d, %v), want (0, nil)", zero, err)
	}
	collision, err := queue.Insert(InsertInput{When: epoch, Retire: func() { retired[2]++ }})
	if err != nil || collision != first {
		t.Fatalf("collision Insert = (%d, %v), want (%d, nil)", collision, err, first)
	}
	if queue.entries[first] == original {
		t.Fatal("colliding handle did not overwrite original map ownership")
	}
	list := queue.lists[0]
	if stats := queue.Stats(); stats.MapEntries != 2 || stats.HeapLists != 1 || list == nil || list.len != 3 {
		t.Fatalf("collision Stats/list = (%+v, %+v), want two map entries and three list nodes", stats, list)
	}
	if result := queue.BatchDrain(DrainInput{Now: epoch, Tick: 1}); result.Executed != 3 || result.Panics != 0 || result.Deferred != 0 {
		t.Fatalf("collision drain = %+v, want three executions", result)
	}
	if retired != [3]int{1, 1, 1} {
		t.Fatalf("retirements = %v, want all colliding nodes retired once", retired)
	}
}
