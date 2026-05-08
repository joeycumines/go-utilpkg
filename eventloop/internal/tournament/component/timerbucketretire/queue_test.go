package timerbucketretire

import (
	"testing"
	"time"
)

func TestQueueStableOrderEligibilityAndRetirement(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	var order []int
	retired := [3]int{}
	queue.Insert(InsertInput{When: epoch.Add(100 * time.Microsecond), EarliestTick: 2, Task: func() { order = append(order, 1) }, Retire: func() { retired[0]++ }})
	queue.Insert(InsertInput{When: epoch.Add(200 * time.Microsecond), Task: func() { order = append(order, 2) }, Retire: func() { retired[1]++ }})
	queue.Insert(InsertInput{When: epoch.Add(200 * time.Microsecond), Task: func() { order = append(order, 3) }, Retire: func() { retired[2]++ }})
	result := queue.BatchDrain(DrainInput{Now: epoch.Add(time.Millisecond), RepeatNow: epoch.Add(time.Millisecond), Tick: 1})
	if result != (DrainResult{Executed: 2, Deferred: 1}) {
		t.Fatalf("BatchDrain = %+v, want two executed and one deferred", result)
	}
	if len(order) != 2 || order[0] != 2 || order[1] != 3 || retired != [3]int{0, 1, 1} {
		t.Fatalf("order/retired = (%v, %v), want ([2 3], [0 1 1])", order, retired)
	}
	queue.BatchDrain(DrainInput{Now: epoch.Add(time.Millisecond), RepeatNow: epoch.Add(time.Millisecond), Tick: 2})
	if retired != [3]int{1, 1, 1} {
		t.Fatalf("retired = %v, want every hook exactly once", retired)
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

func TestQueueCancelAndResetRetireExactlyOnce(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	retired := 0
	first := mustInsert(t, queue, InsertInput{When: epoch.Add(time.Hour), Retire: func() { retired++ }})
	queue.Insert(InsertInput{When: epoch.Add(2 * time.Hour), Retire: func() { retired++ }})
	if err := queue.Cancel(first); err != nil {
		t.Fatal(err)
	}
	if retired != 1 {
		t.Fatalf("retired after Cancel = %d, want 1", retired)
	}
	queue.resetQuiescent()
	if retired != 2 {
		t.Fatalf("retired after Reset = %d, want 2", retired)
	}
	queue.resetQuiescent()
	if retired != 2 {
		t.Fatalf("retired after second Reset = %d, want still 2", retired)
	}
	if stats := queue.Stats(); stats.RetainedListAnchors == 0 {
		t.Fatalf("Stats = %+v, want historical list-anchor retention", stats)
	}
}

func TestQueueDetachedSiblingCancellationPanicAndRetirement(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	retired := [3]int{}
	var third Handle
	if _, err := queue.Insert(InsertInput{When: epoch, Refed: true, Retire: func() { retired[0]++ }, Task: func() {
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
	if _, err := queue.Insert(InsertInput{When: epoch, Refed: true, Retire: func() { retired[1]++ }, Task: func() { panic("sentinel") }}); err != nil {
		t.Fatal(err)
	}
	var err error
	third, err = queue.Insert(InsertInput{When: epoch, Refed: true, Retire: func() { retired[2]++ }, Task: func() { t.Error("canceled sibling executed") }})
	if err != nil {
		t.Fatal(err)
	}
	if result := queue.BatchDrain(DrainInput{Now: epoch, Tick: 1}); result != (DrainResult{Executed: 2, Canceled: 1, Panics: 1}) {
		t.Fatalf("BatchDrain = %+v, want two executions, one cancellation, and one panic", result)
	}
	if retired != [3]int{1, 1, 1} {
		t.Fatalf("retirements = %v, want each exactly once", retired)
	}
	if stats := queue.Stats(); stats.Active != 0 || stats.MapEntries != 0 || stats.ListEntries != 0 || stats.Refed != 0 || stats.RetainedCallbacks != 0 || stats.RetainedRetireHooks != 0 {
		t.Fatalf("Stats after drain = %+v, want released queue", stats)
	}
}

func TestQueueRepeatDefersRetirement(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	called, retired := 0, 0
	handle := mustInsert(t, queue, InsertInput{
		When: epoch, Task: func() { called++ }, Retire: func() { retired++ },
		Interval: time.Millisecond, Repeat: true,
	})
	result := queue.BatchDrain(DrainInput{Now: epoch, RepeatNow: epoch, Tick: 1})
	if result != (DrainResult{Executed: 1, Repeated: 1}) || called != 1 || retired != 0 {
		t.Fatalf("result/called/retired = (%+v, %d, %d)", result, called, retired)
	}
	if err := queue.Cancel(handle); err != nil {
		t.Fatal(err)
	}
	if retired != 1 {
		t.Fatalf("retired after repeat cancel = %d, want 1", retired)
	}
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
			retired := 0
			handle, err := queue.Insert(InsertInput{
				When: epoch, Task: func() {}, Retire: func() { retired++ }, Interval: test.interval,
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
			if retired != 0 {
				t.Fatalf("retired before cancellation = %d, want 0", retired)
			}
			if err := queue.Cancel(handle); err != nil {
				t.Fatal(err)
			}
			if retired != 1 {
				t.Fatalf("retired after cancellation = %d, want 1", retired)
			}
		})
	}
}

func TestQueueReentrantCancelRetiresOnce(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	retired := 0
	var handle Handle
	handle = mustInsert(t, queue, InsertInput{When: epoch, Retire: func() { retired++ }, Task: func() {
		if err := queue.Cancel(handle); err != nil {
			t.Errorf("Cancel: %v", err)
		}
	}})
	if result := queue.BatchDrain(DrainInput{Now: epoch, RepeatNow: epoch}); result != (DrainResult{Executed: 1}) {
		t.Fatalf("BatchDrain = %+v, want one", result)
	}
	if retired != 1 || queue.Len() != 0 {
		t.Fatalf("retired/Len = (%d, %d), want (1, 0)", retired, queue.Len())
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
