package timerheapref

import (
	"errors"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestQueueLifecycleAndCancel(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative()
	called := 0
	first := mustInsert(t, queue, InsertInput{When: epoch, Task: func() { called++ }, Refed: true})
	queue.Insert(InsertInput{When: epoch.Add(time.Millisecond), Task: func() { called++ }})
	if stats := queue.Stats(); stats.Refed != 1 || stats.MapEntries != 2 {
		t.Fatalf("Stats = %+v, want one refed and two mapped", stats)
	}
	if err := queue.Cancel(first); err != nil {
		t.Fatal(err)
	}
	if err := queue.Cancel(first); !errors.Is(err, component.ErrTimerMissing) {
		t.Fatalf("second Cancel error = %v, want %v", err, component.ErrTimerMissing)
	}
	if result := queue.BatchDrain(DrainInput{Now: epoch.Add(time.Millisecond)}); result != (DrainResult{Executed: 1}) {
		t.Fatalf("BatchDrain = %+v, want one execution", result)
	}
	if called != 1 || queue.Len() != 0 {
		t.Fatalf("called/Len = (%d, %d), want (1, 0)", called, queue.Len())
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
	if result := queue.BatchDrain(DrainInput{Now: epoch.Add(time.Microsecond)}); result != (DrainResult{Executed: 2, Panics: 1}) {
		t.Fatalf("BatchDrain = %+v, want two executions and one panic", result)
	}
	if called != 1 {
		t.Fatalf("subsequent callback count = %d, want 1", called)
	}
	if stats := queue.Stats(); stats.HeapActive != 0 || stats.MapEntries != 0 || stats.Refed != 0 || stats.RetainedCallbacks != 0 || stats.RetainedHeapPointers != 0 {
		t.Fatalf("Stats after panic drain = %+v, want released queue", stats)
	}
}

func TestQueueReentrantCancelPreservesReplacement(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative()
	var first Handle
	first = mustInsert(t, queue, InsertInput{When: epoch, Task: func() {
		queue.Insert(InsertInput{When: epoch.Add(time.Hour), Task: func() {}})
		if err := queue.Cancel(first); err != nil {
			t.Errorf("reentrant Cancel: %v", err)
		}
	}})
	if result := queue.BatchDrain(DrainInput{Now: epoch}); result != (DrainResult{Executed: 1}) {
		t.Fatalf("BatchDrain = %+v, want one execution", result)
	}
	stats := queue.Stats()
	if stats.HeapActive != 1 || stats.MapEntries != 1 || stats.RetainedCallbacks != 1 {
		t.Fatalf("Stats = %+v, want preserved replacement", stats)
	}
	queue.resetQuiescent()
}

func TestQueuePopInvalidatesIndexAndClearsTail(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative()
	handle := mustInsert(t, queue, InsertInput{When: epoch})
	entry := queue.entries[handle]
	queue.BatchDrain(DrainInput{Now: epoch})
	if entry.heapIndex != -1 {
		t.Fatalf("popped heapIndex = %d, want -1", entry.heapIndex)
	}
	if cap(queue.timers) == 0 || queue.timers[:cap(queue.timers)][0] != nil {
		t.Fatal("Pop retained pointer in heap backing tail")
	}
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
	queue.Insert(InsertInput{When: time.Unix(1, 0), Refed: true})
	if stats := queue.Stats(); stats.HeapActive != 1 || stats.Refed != 1 {
		t.Fatalf("Stats = %+v, want one refed timer", stats)
	}
	queue.resetQuiescent()
}
