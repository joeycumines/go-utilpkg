package timerheapstall

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestNativeIDBoundaryAndWrapDefect(t *testing.T) {
	queue := NewNative()
	queue.nextID.Store(maxSafeInteger - 1)
	if handle, err := queue.Insert(InsertInput{}); handle != maxSafeInteger || err != nil {
		t.Fatalf("maximum valid Insert = (%d, %v)", handle, err)
	}
	for attempt := range 2 {
		if handle, err := queue.Insert(InsertInput{Task: func() {}}); handle != 0 || !errors.Is(err, component.ErrTimerExhausted) {
			t.Fatalf("exhausted Insert %d = (%d, %v)", attempt, handle, err)
		}
	}
	wantCounter := uint64(maxSafeInteger + 2)
	queue.resetQuiescent()
	if got := queue.nextID.Load(); got != wantCounter {
		t.Fatalf("nextID after cleanup = %d, want %d", got, wantCounter)
	}
	queue.nextID.Store(math.MaxUint64)
	if handle, err := queue.Insert(InsertInput{}); handle != 0 || err != nil {
		t.Fatalf("wrapped Insert = (%d, %v), want historical accepted zero", handle, err)
	}
	queue.resetQuiescent()
}

func TestNativeIDWrapCollisionOverwritesMapOwnership(t *testing.T) {
	epoch := time.Unix(1, 0)
	queue := NewNative()
	first, err := queue.Insert(InsertInput{When: epoch})
	if err != nil || first != 1 {
		t.Fatalf("first Insert = (%d, %v), want (1, nil)", first, err)
	}
	original := queue.entries[first]
	queue.nextID.Store(math.MaxUint64)
	zero, err := queue.Insert(InsertInput{When: epoch.Add(time.Microsecond)})
	if err != nil || zero != 0 {
		t.Fatalf("wrapped Insert = (%d, %v), want (0, nil)", zero, err)
	}
	collision, err := queue.Insert(InsertInput{When: epoch.Add(2 * time.Microsecond)})
	if err != nil || collision != first {
		t.Fatalf("collision Insert = (%d, %v), want (%d, nil)", collision, err, first)
	}
	if queue.entries[first] == original {
		t.Fatal("colliding handle did not overwrite original map ownership")
	}
	if stats := queue.Stats(); stats.HeapActive != 3 || stats.MapEntries != 2 {
		t.Fatalf("collision Stats = %+v, want three heap nodes and two map entries", stats)
	}
	if result := queue.BatchDrain(DrainInput{Now: epoch.Add(2 * time.Microsecond), Tick: 1}); result.Executed != 3 || result.Panics != 0 {
		t.Fatalf("collision drain = %+v, want three executions", result)
	}
}

func TestResetRetainsHeapBackingPointer(t *testing.T) {
	queue := NewNative()
	mustInsert(t, queue, InsertInput{When: time.Unix(1, 0)})
	queue.resetQuiescent()
	if got := queue.Stats().RetainedHeapPointers; got != 1 {
		t.Fatalf("RetainedHeapPointers = %d, want 1", got)
	}
}
