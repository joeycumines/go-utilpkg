package timerheapdeadline

import (
	"errors"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestQueueLifecycle(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative()
	var order []int
	for _, value := range []struct {
		offset time.Duration
		id     int
	}{
		{offset: 3 * time.Millisecond, id: 3},
		{offset: time.Millisecond, id: 1},
		{offset: 2 * time.Millisecond, id: 2},
	} {
		id := value.id
		queue.Insert(InsertInput{When: epoch.Add(value.offset), Task: func() { order = append(order, id) }})
	}
	if got, ok := queue.Peek(); !ok || !got.Equal(epoch.Add(time.Millisecond)) {
		t.Fatalf("Peek = (%v, %t), want first deadline", got, ok)
	}
	if result := queue.BatchDrain(DrainInput{Now: epoch.Add(2 * time.Millisecond)}); result != (DrainResult{Executed: 2}) {
		t.Fatalf("BatchDrain = %+v, want two executions", result)
	}
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("execution order = %v, want [1 2]", order)
	}
	if queue.Len() != 1 {
		t.Fatalf("Len = %d, want 1", queue.Len())
	}
	queue.resetQuiescent()
	if stats := queue.Stats(); stats.HeapActive != 0 || stats.MapEntries != 0 || stats.RetainedCallbacks != 0 {
		t.Fatalf("Stats after Reset = %+v, want no active state", stats)
	}
}

func TestQueuePanicRecoveryContinuesAndReleases(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative()
	if _, err := queue.Insert(InsertInput{When: epoch, Task: func() { panic("sentinel") }}); err != nil {
		t.Fatal(err)
	}
	called := 0
	if _, err := queue.Insert(InsertInput{When: epoch.Add(time.Microsecond), Task: func() { called++ }}); err != nil {
		t.Fatal(err)
	}
	if result := queue.BatchDrain(DrainInput{Now: epoch.Add(time.Microsecond)}); result != (DrainResult{Executed: 2, Panics: 1}) {
		t.Fatalf("BatchDrain = %+v, want two executions and one panic", result)
	}
	if called != 1 {
		t.Fatalf("subsequent callback count = %d, want 1", called)
	}
	if stats := queue.Stats(); stats.HeapActive != 0 || stats.MapEntries != 0 || stats.RetainedCallbacks != 0 || stats.RetainedHeapPointers != 0 {
		t.Fatalf("Stats after panic drain = %+v, want released queue", stats)
	}
}

func TestQueueCancel(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative()
	handle := mustInsert(t, queue, InsertInput{When: epoch, Task: func() { t.Fatal("canceled callback ran") }})
	if err := queue.Cancel(handle); err != nil {
		t.Fatal(err)
	}
	if err := queue.Cancel(handle); !errors.Is(err, component.ErrTimerMissing) {
		t.Fatalf("second Cancel error = %v, want %v", err, component.ErrTimerMissing)
	}
	if result := queue.BatchDrain(DrainInput{Now: epoch}); result != (DrainResult{}) {
		t.Fatalf("BatchDrain = %+v, want zero", result)
	}
}

func TestQueuePreservesReentrantStaleIndexDefect(t *testing.T) {
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
	if stats.HeapActive != 0 || stats.MapEntries != 1 || stats.RetainedCallbacks != 1 {
		t.Fatalf("Stats = %+v, want orphaned replacement proving stale-index defect", stats)
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

func TestQueuePopClearsHeapTail(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative()
	queue.Insert(InsertInput{When: epoch, Task: func() {}})
	queue.BatchDrain(DrainInput{Now: epoch})
	if cap(queue.timers) == 0 || queue.timers[:cap(queue.timers)][0] != nil {
		t.Fatal("Pop retained pointer in heap backing tail")
	}
}

func TestQueueZeroValue(t *testing.T) {
	var queue Queue
	queue.Insert(InsertInput{When: time.Unix(1, 0)})
	if queue.Len() != 1 {
		t.Fatalf("Len = %d, want 1", queue.Len())
	}
	queue.resetQuiescent()
}
