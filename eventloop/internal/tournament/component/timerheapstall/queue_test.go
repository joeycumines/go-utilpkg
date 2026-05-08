package timerheapstall

import (
	"errors"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestQueuePreservesIneligibleHeadStall(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative()
	var order []int
	queue.Insert(InsertInput{When: epoch, EarliestTick: 2, Task: func() { order = append(order, 1) }})
	queue.Insert(InsertInput{When: epoch.Add(time.Nanosecond), EarliestTick: 0, Task: func() { order = append(order, 2) }})
	if result := queue.BatchDrain(DrainInput{Now: epoch.Add(time.Millisecond), Tick: 1}); result != (DrainResult{}) {
		t.Fatalf("blocked BatchDrain = %+v, want zero", result)
	}
	if len(order) != 0 || queue.Len() != 2 {
		t.Fatalf("order/Len = (%v, %d), want blocked two-timer heap", order, queue.Len())
	}
	if result := queue.BatchDrain(DrainInput{Now: epoch.Add(time.Millisecond), Tick: 2}); result != (DrainResult{Executed: 2}) {
		t.Fatalf("eligible BatchDrain = %+v, want two", result)
	}
}

func TestQueuePanicRecoveryContinuesAndReleases(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative()
	if _, err := queue.Insert(InsertInput{When: epoch, Task: func() { panic("sentinel") }, Refed: true}); err != nil {
		t.Fatal(err)
	}
	called := 0
	if _, err := queue.Insert(InsertInput{When: epoch.Add(time.Microsecond), Task: func() { called++ }, Refed: true}); err != nil {
		t.Fatal(err)
	}
	if result := queue.BatchDrain(DrainInput{Now: epoch.Add(time.Microsecond), Tick: 1}); result != (DrainResult{Executed: 2, Panics: 1}) {
		t.Fatalf("BatchDrain = %+v, want two executions and one panic", result)
	}
	if called != 1 {
		t.Fatalf("subsequent callback count = %d, want 1", called)
	}
	if stats := queue.Stats(); stats.HeapActive != 0 || stats.MapEntries != 0 || stats.Refed != 0 || stats.RetainedCallbacks != 0 || stats.RetainedHeapPointers != 0 {
		t.Fatalf("Stats after panic drain = %+v, want released queue", stats)
	}
}

func TestQueueEqualDeadlineUsesEarliestTick(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative()
	var order []int
	queue.Insert(InsertInput{When: epoch, EarliestTick: 2, Task: func() { order = append(order, 2) }})
	queue.Insert(InsertInput{When: epoch, EarliestTick: 1, Task: func() { order = append(order, 1) }})
	queue.BatchDrain(DrainInput{Now: epoch, Tick: 2})
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("order = %v, want [1 2]", order)
	}
}

func TestQueueCancelAndReentrantOwnership(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative()
	canceled := mustInsert(t, queue, InsertInput{When: epoch, Task: func() { t.Fatal("canceled callback ran") }})
	if err := queue.Cancel(canceled); err != nil {
		t.Fatal(err)
	}
	if err := queue.Cancel(canceled); !errors.Is(err, component.ErrTimerMissing) {
		t.Fatalf("second Cancel error = %v, want missing", err)
	}
	var active Handle
	active = mustInsert(t, queue, InsertInput{When: epoch, Task: func() {
		queue.Insert(InsertInput{When: epoch.Add(time.Hour), Task: func() {}})
		if err := queue.Cancel(active); err != nil {
			t.Errorf("reentrant Cancel: %v", err)
		}
	}})
	if result := queue.BatchDrain(DrainInput{Now: epoch}); result != (DrainResult{Executed: 1}) {
		t.Fatalf("BatchDrain = %+v, want one", result)
	}
	if stats := queue.Stats(); stats.HeapActive != 1 || stats.MapEntries != 1 {
		t.Fatalf("Stats = %+v, want preserved replacement", stats)
	}
	queue.resetQuiescent()
}

func mustInsert(t *testing.T, queue *Queue, input InsertInput) Handle {
	t.Helper()
	handle, err := queue.Insert(input)
	if err != nil {
		t.Fatal(err)
	}
	return handle
}

func TestQueueZeroValue(t *testing.T) {
	var queue Queue
	queue.Insert(InsertInput{When: time.Unix(1, 0)})
	if queue.Len() != 1 {
		t.Fatalf("Len = %d, want 1", queue.Len())
	}
	queue.resetQuiescent()
}
