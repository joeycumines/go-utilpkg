package timervaluethree

import (
	"testing"
	"time"
)

func TestQueueLifecycleAndRetention(t *testing.T) {
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
		queue.Insert(InsertInput{
			When: epoch.Add(value.offset),
			Task: Task{Runnable: func() { order = append(order, id) }},
		})
	}
	if got, ok := queue.Peek(); !ok || !got.Equal(epoch.Add(time.Millisecond)) {
		t.Fatalf("Peek = (%v, %t), want first deadline", got, ok)
	}
	result := queue.BatchDrain(DrainInput{Now: epoch.Add(2 * time.Millisecond)})
	if result != (DrainResult{Executed: 2}) {
		t.Fatalf("BatchDrain = %+v, want two executions", result)
	}
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("execution order = %v, want [1 2]", order)
	}
	if stats := queue.Stats(); stats.Active != 1 || stats.RetainedCallbacks != 3 {
		t.Fatalf("Stats = %+v, want one active and three retained callbacks", stats)
	}
	queue.resetQuiescent()
	if stats := queue.Stats(); stats != (Stats{}) {
		t.Fatalf("Stats after Reset = %+v, want zero", stats)
	}
}

func TestQueueRecoversPanic(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative()
	queue.Insert(InsertInput{When: epoch, Task: Task{Runnable: func() { panic("expected") }}})
	if result := queue.BatchDrain(DrainInput{Now: epoch}); result != (DrainResult{Executed: 1, Panics: 1}) {
		t.Fatalf("BatchDrain = %+v, want recovered panic", result)
	}
}

func TestQueueZeroValue(t *testing.T) {
	var queue Queue
	queue.Insert(InsertInput{When: time.Unix(1, 0)})
	if queue.Len() != 1 {
		t.Fatalf("Len = %d, want 1", queue.Len())
	}
}
