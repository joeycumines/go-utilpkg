package timerbucketcurrent

import (
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestQueueStableOrderKeepsEligibilityOutsideComparator(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	publication := closedPublication()
	var order []int
	if _, err := queue.Insert(InsertInput{When: epoch, ScheduledTick: 1, DeferTick: true, Publication: publication, Task: func() { order = append(order, 1) }}); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Insert(InsertInput{When: epoch, ScheduledTick: 0, Publication: publication, Task: func() { order = append(order, 2) }}); err != nil {
		t.Fatal(err)
	}
	result := queue.BatchDrain(DrainInput{Now: epoch, RepeatNow: epoch, Tick: 1})
	if result != (DrainResult{Executed: 1, Deferred: 1}) || len(order) != 1 || order[0] != 2 {
		t.Fatalf("result/order = (%+v, %v), want one deferred then [2]", result, order)
	}
	result = queue.BatchDrain(DrainInput{Now: epoch, RepeatNow: epoch, Tick: 2})
	if result != (DrainResult{Executed: 1}) || len(order) != 2 || order[1] != 1 {
		t.Fatalf("second result/order = (%+v, %v), want deferred execution", result, order)
	}
}

func TestQueueStaticEqualDeadlineFIFOAndSameMillisecondOrder(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	publication := closedPublication()

	equal := NewNative(epoch)
	var equalOrder []int
	for ordinal := range 8 {
		if _, err := equal.Insert(InsertInput{
			When: epoch.Add(500 * time.Microsecond), Publication: publication,
			Task: func() { equalOrder = append(equalOrder, ordinal) },
		}); err != nil {
			t.Fatal(err)
		}
	}
	if result := equal.BatchDrain(DrainInput{Now: epoch.Add(time.Millisecond)}); result != (DrainResult{Executed: 8}) {
		t.Fatalf("equal BatchDrain = %+v, want eight executions", result)
	}
	if len(equalOrder) != 8 {
		t.Fatalf("equal order length = %d, want 8", len(equalOrder))
	}
	for ordinal, got := range equalOrder {
		if got != ordinal {
			t.Fatalf("equal order = %v, want insertion FIFO", equalOrder)
		}
	}

	distinct := NewNative(epoch)
	var distinctOrder []int
	for ordinal, offset := range []time.Duration{900, 100, 700, 300, 800, 200, 600, 400} {
		ordinal, offset := ordinal, offset*time.Microsecond
		if _, err := distinct.Insert(InsertInput{
			When: epoch.Add(offset), Publication: publication,
			Task: func() { distinctOrder = append(distinctOrder, ordinal) },
		}); err != nil {
			t.Fatal(err)
		}
	}
	if result := distinct.BatchDrain(DrainInput{Now: epoch.Add(time.Millisecond)}); result != (DrainResult{Executed: 8}) {
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

func TestQueueDeferredReentrantInsertionReordersEqualDeadline(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	publication := closedPublication()
	queue := NewNative(epoch)
	var order []int
	deadline := epoch.Add(500 * time.Microsecond)
	if _, err := queue.Insert(InsertInput{
		When: deadline, ScheduledTick: 1, DeferTick: true, Publication: publication,
		Task: func() { order = append(order, 1) },
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Insert(InsertInput{
		When: epoch.Add(100 * time.Microsecond), Publication: publication,
		Task: func() {
			order = append(order, 2)
			if _, insertErr := queue.Insert(InsertInput{
				When: deadline, ScheduledTick: 1, DeferTick: true, Publication: publication,
				Task: func() { order = append(order, 3) },
			}); insertErr != nil {
				t.Errorf("reentrant Insert: %v", insertErr)
			}
		},
	}); err != nil {
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

func TestQueueWaitsForPublication(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	publication := make(chan struct{})
	releasePublication := sync.OnceFunc(func() { close(publication) })
	defer releasePublication()
	before := make(chan struct{})
	executed := make(chan struct{})
	done := make(chan DrainResult, 1)
	if _, err := queue.Insert(InsertInput{When: epoch, Publication: publication, Task: func() { close(executed) }}); err != nil {
		t.Fatal(err)
	}
	go func() {
		done <- queue.BatchDrain(DrainInput{
			Now: epoch, RepeatNow: epoch,
			BeforePublication: func(Handle) { close(before) },
		})
	}()
	select {
	case <-before:
	case <-time.After(time.Second):
		t.Fatal("BatchDrain did not reach the publication boundary")
	}
	select {
	case <-executed:
		t.Fatal("callback ran before publication")
	default:
	}
	releasePublication()
	select {
	case result := <-done:
		if result != (DrainResult{Executed: 1}) {
			t.Fatalf("BatchDrain = %+v, want one", result)
		}
	case <-time.After(time.Second):
		t.Fatal("BatchDrain did not finish after publication")
	}
	select {
	case <-executed:
	default:
		t.Fatal("callback did not run after publication")
	}
}

func TestQueueResetClearsAllRetention(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	publication := make(chan struct{})
	retired := 0
	if _, err := queue.Insert(InsertInput{
		When: epoch.Add(time.Hour), Task: func() {}, Retire: func() { retired++ },
		Publication: publication, Refed: true,
	}); err != nil {
		t.Fatal(err)
	}
	queue.resetQuiescent()
	if retired != 1 {
		t.Fatalf("retired = %d, want 1", retired)
	}
	stats := queue.Stats()
	if stats.Active != 0 || stats.HeapLists != 0 || stats.MapEntries != 0 || stats.ListEntries != 0 || stats.RetainedCallbacks != 0 || stats.RetainedRetireHooks != 0 || stats.RetainedPublications != 0 || stats.RetainedListAnchors != 0 {
		t.Fatalf("Stats after Reset = %+v, want complete release", stats)
	}
}

func TestQueueDetachedSiblingCancellationPanicAndRetirement(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	publication := closedPublication()
	queue := NewNative(epoch)
	retired := [3]int{}
	var third Handle
	if _, err := queue.Insert(InsertInput{When: epoch, Publication: publication, Refed: true, Retire: func() { retired[0]++ }, Task: func() {
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
	if _, err := queue.Insert(InsertInput{When: epoch, Publication: publication, Refed: true, Retire: func() { retired[1]++ }, Task: func() { panic("sentinel") }}); err != nil {
		t.Fatal(err)
	}
	var err error
	third, err = queue.Insert(InsertInput{When: epoch, Publication: publication, Refed: true, Retire: func() { retired[2]++ }, Task: func() { t.Error("canceled sibling executed") }})
	if err != nil {
		t.Fatal(err)
	}
	if result := queue.BatchDrain(DrainInput{Now: epoch, Tick: 1}); result != (DrainResult{Executed: 2, Canceled: 1, Panics: 1}) {
		t.Fatalf("BatchDrain = %+v, want two executions, one cancellation, and one panic", result)
	}
	if retired != [3]int{1, 1, 1} {
		t.Fatalf("retirements = %v, want each exactly once", retired)
	}
	if stats := queue.Stats(); stats.Active != 0 || stats.MapEntries != 0 || stats.ListEntries != 0 || stats.Refed != 0 || stats.RetainedCallbacks != 0 || stats.RetainedRetireHooks != 0 || stats.RetainedPublications != 0 {
		t.Fatalf("Stats after drain = %+v, want released queue", stats)
	}
}

func TestQueueIdentityExhaustionRetiresRejectedNode(t *testing.T) {
	queue := NewNative(time.Unix(1, 0))
	queue.nextID.Store(math.MaxUint64 - 1)
	retired := 0
	handle, err := queue.Insert(InsertInput{When: time.Unix(1, 0).Add(time.Hour), Retire: func() { retired++ }})
	if handle != Handle(math.MaxUint64) || err != nil {
		t.Fatalf("maximum Insert = (%d, %v), want (%d, nil)", handle, err, uint64(math.MaxUint64))
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if rejected, insertErr := queue.Insert(InsertInput{Retire: func() { retired++ }}); rejected != 0 || !errors.Is(insertErr, component.ErrTimerExhausted) {
			t.Fatalf("Insert exhaustion %d = (%d, %v), want (0, %v)", attempt, rejected, insertErr, component.ErrTimerExhausted)
		}
		if got := queue.nextID.Load(); got != math.MaxUint64 {
			t.Fatalf("nextID after rejection %d = %d, want sticky maximum", attempt, got)
		}
	}
	if retired != 2 {
		t.Fatalf("rejected retirements = %d, want 2", retired)
	}
	if err := queue.Cancel(handle); err != nil {
		t.Fatal(err)
	}
	if retired != 3 {
		t.Fatalf("total retirements = %d, want successful-node cancellation plus two rejections", retired)
	}
	queue.resetQuiescent()
	if got := queue.nextID.Load(); got != math.MaxUint64 {
		t.Fatalf("nextID after cleanup = %d, want sticky maximum", got)
	}
}

func TestQueueRepeatDefersSameTick(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	called := 0
	handle, err := queue.Insert(InsertInput{
		When: epoch, Task: func() { called++ }, Publication: closedPublication(), Repeat: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := queue.BatchDrain(DrainInput{Now: epoch, RepeatNow: epoch, Tick: 1})
	if result != (DrainResult{Executed: 1, Deferred: 1, Repeated: 1}) || called != 1 || queue.Len() != 1 {
		t.Fatalf("first repeat result/called/Len = (%+v, %d, %d)", result, called, queue.Len())
	}
	if err := queue.Cancel(handle); err != nil {
		t.Fatal(err)
	}
}

func TestQueueReentrantCancelRetiresOnce(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	retired := 0
	var handle Handle
	var err error
	handle, err = queue.Insert(InsertInput{When: epoch, Publication: closedPublication(), Retire: func() { retired++ }, Task: func() {
		if cancelErr := queue.Cancel(handle); cancelErr != nil {
			t.Errorf("Cancel: %v", cancelErr)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result := queue.BatchDrain(DrainInput{Now: epoch, RepeatNow: epoch}); result != (DrainResult{Executed: 1}) {
		t.Fatalf("BatchDrain = %+v, want one", result)
	}
	if retired != 1 || queue.Len() != 0 {
		t.Fatalf("retired/Len = (%d, %d), want (1, 0)", retired, queue.Len())
	}
}

func closedPublication() <-chan struct{} {
	publication := make(chan struct{})
	close(publication)
	return publication
}
