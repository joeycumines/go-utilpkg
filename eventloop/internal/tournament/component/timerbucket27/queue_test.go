package timerbucket27

import (
	"errors"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestQueueStableBucketOrderAndEligibility(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	var order []int
	queue.Insert(InsertInput{When: epoch.Add(100 * time.Microsecond), EarliestTick: 2, Task: func() { order = append(order, 1) }})
	queue.Insert(InsertInput{When: epoch.Add(200 * time.Microsecond), EarliestTick: 0, Task: func() { order = append(order, 2) }})
	queue.Insert(InsertInput{When: epoch.Add(200 * time.Microsecond), EarliestTick: 0, Task: func() { order = append(order, 3) }})
	result := queue.BatchDrain(DrainInput{Now: epoch.Add(time.Millisecond), RepeatNow: epoch.Add(time.Millisecond), Tick: 1})
	if result != (DrainResult{Executed: 2, Deferred: 1}) {
		t.Fatalf("BatchDrain = %+v, want two executed and one deferred", result)
	}
	if len(order) != 2 || order[0] != 2 || order[1] != 3 {
		t.Fatalf("order = %v, want stable [2 3]", order)
	}
	if result := queue.BatchDrain(DrainInput{Now: epoch.Add(time.Millisecond), RepeatNow: epoch.Add(time.Millisecond), Tick: 2}); result != (DrainResult{Executed: 1}) {
		t.Fatalf("second BatchDrain = %+v, want deferred timer", result)
	}
}

func TestQueueStaticEqualPriorityFIFOAndSameMillisecondOrder(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	equal := NewNative(epoch)
	var equalOrder []int
	for ordinal := range 8 {
		if _, err := equal.Insert(InsertInput{When: epoch.Add(500 * time.Microsecond), EarliestTick: 1, Task: func() { equalOrder = append(equalOrder, ordinal) }}); err != nil {
			t.Fatal(err)
		}
	}
	if result := equal.BatchDrain(DrainInput{Now: epoch.Add(time.Millisecond), Tick: 1}); result != (DrainResult{Executed: 8}) {
		t.Fatalf("equal BatchDrain = %+v, want eight executions", result)
	}
	if len(equalOrder) != 8 {
		t.Fatalf("equal order length = %d, want 8", len(equalOrder))
	}
	for ordinal, got := range equalOrder {
		if got != ordinal {
			t.Fatalf("equal-priority order = %v, want insertion FIFO", equalOrder)
		}
	}

	distinct := NewNative(epoch)
	var distinctOrder []int
	for ordinal, micros := range []int{900, 100, 700, 300, 800, 200, 600, 400} {
		ordinal, offset := ordinal, time.Duration(micros)*time.Microsecond
		if _, err := distinct.Insert(InsertInput{When: epoch.Add(offset), Task: func() { distinctOrder = append(distinctOrder, ordinal) }}); err != nil {
			t.Fatal(err)
		}
	}
	if result := distinct.BatchDrain(DrainInput{Now: epoch.Add(time.Millisecond), Tick: 1}); result != (DrainResult{Executed: 8}) {
		t.Fatalf("distinct BatchDrain = %+v, want eight executions", result)
	}
	wantDistinct := []int{1, 5, 3, 7, 6, 2, 4, 0}
	if len(distinctOrder) != len(wantDistinct) {
		t.Fatalf("same-millisecond order length = %d, want %d", len(distinctOrder), len(wantDistinct))
	}
	for index, got := range distinctOrder {
		if got != wantDistinct[index] {
			t.Fatalf("same-millisecond order = %v, want %v", distinctOrder, wantDistinct)
		}
	}
}

func TestQueueDeferredReentrantInsertionReordersEqualPriority(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	var order []int
	deadline := epoch.Add(500 * time.Microsecond)
	if _, err := queue.Insert(InsertInput{When: deadline, EarliestTick: 2, Task: func() { order = append(order, 1) }}); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Insert(InsertInput{When: epoch.Add(100 * time.Microsecond), EarliestTick: 1, Task: func() {
		order = append(order, 2)
		if _, insertErr := queue.Insert(InsertInput{When: deadline, EarliestTick: 2, Task: func() { order = append(order, 3) }}); insertErr != nil {
			t.Errorf("reentrant Insert: %v", insertErr)
		}
	}}); err != nil {
		t.Fatal(err)
	}
	if result := queue.BatchDrain(DrainInput{Now: epoch.Add(100 * time.Microsecond), Tick: 1}); result != (DrainResult{Executed: 1, Deferred: 1}) {
		t.Fatalf("first BatchDrain = %+v, want one execution and one deferral", result)
	}
	if result := queue.BatchDrain(DrainInput{Now: deadline, Tick: 2}); result != (DrainResult{Executed: 2}) {
		t.Fatalf("second BatchDrain = %+v, want two executions", result)
	}
	want := []int{2, 3, 1}
	if len(order) != len(want) {
		t.Fatalf("order length = %d, want %d", len(order), len(want))
	}
	for index, got := range order {
		if got != want[index] {
			t.Fatalf("order = %v, want historical reinsert order %v", order, want)
		}
	}
}

func TestQueueCancellationAndReentrantOwnership(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	canceled := mustInsert(t, queue, InsertInput{When: epoch, Task: func() { t.Fatal("canceled callback ran") }})
	if err := queue.Cancel(canceled); err != nil {
		t.Fatal(err)
	}
	if err := queue.Cancel(canceled); !errors.Is(err, component.ErrTimerMissing) {
		t.Fatalf("second Cancel error = %v, want missing", err)
	}
	var active Handle
	active = mustInsert(t, queue, InsertInput{When: epoch, Task: func() {
		if err := queue.Cancel(active); err != nil {
			t.Errorf("reentrant Cancel: %v", err)
		}
	}})
	if result := queue.BatchDrain(DrainInput{Now: epoch, RepeatNow: epoch}); result != (DrainResult{Executed: 1}) {
		t.Fatalf("BatchDrain = %+v, want one execution", result)
	}
	if queue.Len() != 0 {
		t.Fatalf("Len = %d, want zero", queue.Len())
	}
}

func TestQueueDetachedSiblingCancellationPanicAndRelease(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	var third Handle
	if _, err := queue.Insert(InsertInput{When: epoch, Refed: true, Task: func() {
		if err := queue.Cancel(third); err != nil {
			t.Errorf("Cancel detached sibling: %v", err)
		}
		stats := queue.Stats()
		if stats.Active != 2 || stats.Refed != 2 {
			t.Errorf("Stats after detached cancellation = %+v, want two refed visible nodes", stats)
		}
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Insert(InsertInput{When: epoch, Refed: true, Task: func() { panic("sentinel") }}); err != nil {
		t.Fatal(err)
	}
	var err error
	third, err = queue.Insert(InsertInput{When: epoch, Refed: true, Task: func() { t.Error("canceled sibling executed") }})
	if err != nil {
		t.Fatal(err)
	}
	if result := queue.BatchDrain(DrainInput{Now: epoch, Tick: 1}); result != (DrainResult{Executed: 2, Canceled: 1, Panics: 1}) {
		t.Fatalf("BatchDrain = %+v, want two executions, one cancellation, and one panic", result)
	}
	if stats := queue.Stats(); stats.Active != 0 || stats.MapEntries != 0 || stats.ListEntries != 0 || stats.Refed != 0 || stats.RetainedCallbacks != 0 {
		t.Fatalf("Stats after drain = %+v, want released queue", stats)
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

func TestQueueRepeatingTimer(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	called := 0
	queue.Insert(InsertInput{
		When: epoch, Task: func() { called++ }, Interval: time.Millisecond,
		Repeat: true, Refed: true,
	})
	result := queue.BatchDrain(DrainInput{Now: epoch, RepeatNow: epoch, Tick: 1})
	if result != (DrainResult{Executed: 1, Repeated: 1}) || called != 1 || queue.Len() != 1 {
		t.Fatalf("first repeat result/called/Len = (%+v, %d, %d)", result, called, queue.Len())
	}
	result = queue.BatchDrain(DrainInput{Now: epoch.Add(time.Millisecond), RepeatNow: epoch.Add(time.Millisecond), Tick: 2})
	if result != (DrainResult{Executed: 1, Repeated: 1}) || called != 2 {
		t.Fatalf("second repeat result/called = (%+v, %d)", result, called)
	}
	queue.resetQuiescent()
}

func TestQueueRepeatingTimerNestedClampBoundaries(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	for _, test := range []struct {
		name       string
		interval   time.Duration
		nesting    int32
		applyClamp bool
		wantDelay  time.Duration
	}{
		{name: "nesting five", interval: time.Millisecond, nesting: 5, applyClamp: true, wantDelay: time.Millisecond},
		{name: "nesting six short", interval: time.Millisecond, nesting: 6, applyClamp: true, wantDelay: 4 * time.Millisecond},
		{name: "nesting six boundary", interval: 4 * time.Millisecond, nesting: 6, applyClamp: true, wantDelay: 4 * time.Millisecond},
		{name: "nesting six negative", interval: -time.Millisecond, nesting: 6, applyClamp: true, wantDelay: 4 * time.Millisecond},
		{name: "unclamped negative", interval: -time.Millisecond, nesting: 6, wantDelay: -time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			queue := NewNative(epoch)
			handle, err := queue.Insert(InsertInput{
				When: epoch, Task: func() {}, Interval: test.interval,
				NestedClamp: test.applyClamp, Repeat: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			wantResult := DrainResult{Executed: 1, Repeated: 1}
			if test.wantDelay <= 0 {
				wantResult.Deferred = 1
			}
			if result := queue.BatchDrain(DrainInput{Now: epoch, RepeatNow: epoch, Tick: 1, CurrentNesting: test.nesting}); result != wantResult {
				t.Fatalf("BatchDrain = %+v, want %+v", result, wantResult)
			}
			if when, ok := queue.Peek(); !ok || !when.Equal(epoch.Add(test.wantDelay)) {
				t.Fatalf("Peek = (%v, %v), want (%v, true)", when, ok, epoch.Add(test.wantDelay))
			}
			if entry := queue.entries[handle]; entry == nil || entry.nestingLevel != test.nesting {
				t.Fatalf("rescheduled nesting = %+v, want %d", entry, test.nesting)
			}
			if err := queue.Cancel(handle); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestQueueResetPreservesHistoricalListAnchors(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	queue.Insert(InsertInput{When: epoch.Add(time.Hour), Task: func() {}})
	if stats := queue.Stats(); stats.RetainedListAnchors != 1 {
		t.Fatalf("pre-reset Stats = %+v, want one live list anchor", stats)
	}
	queue.resetQuiescent()
	stats := queue.Stats()
	if stats.Active != 0 || stats.HeapLists != 0 || stats.MapEntries != 0 || stats.ListEntries != 0 {
		t.Fatalf("post-reset active Stats = %+v, want zero", stats)
	}
	if stats.RetainedListAnchors != 1 {
		t.Fatalf("post-reset retained anchors = %d, want historical retention", stats.RetainedListAnchors)
	}
}

func TestQueueFixedEpochConstructor(t *testing.T) {
	epoch := time.Unix(1, 0)
	queue := NewNative(epoch)
	queue.Insert(InsertInput{When: epoch})
	if queue.Len() != 1 {
		t.Fatalf("Len = %d, want 1", queue.Len())
	}
	queue.resetQuiescent()
}
