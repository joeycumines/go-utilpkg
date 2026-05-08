package eventloop

import (
	"container/heap"
	"context"
	"math/rand"
	"testing"
	"time"
)

func newTimerListForHeapTest(key int64, deadline time.Time) *timerList {
	return &timerList{key: key, deadline: deadline, heapIndex: -1}
}

// TestTimerListHeapEmptyOperations verifies the deadline-bucket heap starts and
// initializes empty. Popping an empty heap is intentionally left to heap.Interface
// and panics, matching container/heap's contract.
func TestTimerListHeapEmptyOperations(t *testing.T) {
	h := make(timerListHeap, 0)
	if h.Len() != 0 {
		t.Fatalf("empty deadline heap Len() = %d, want 0", h.Len())
	}
	heap.Init(&h)
	if h.Len() != 0 {
		t.Fatalf("initialized empty deadline heap Len() = %d, want 0", h.Len())
	}
	defer func() {
		if recover() == nil {
			t.Fatal("heap.Pop on an empty deadline heap should panic")
		}
	}()
	heap.Pop(&h)
}

// TestTimerListHeapOrdersDeadlineBuckets verifies distinct millisecond buckets
// pop by bucket key, with true deadline as the deterministic tie-breaker.
func TestTimerListHeapOrdersDeadlineBuckets(t *testing.T) {
	now := time.Now()
	h := make(timerListHeap, 0)
	heap.Init(&h)
	heap.Push(&h, newTimerListForHeapTest(3, now.Add(3*time.Millisecond)))
	heap.Push(&h, newTimerListForHeapTest(1, now.Add(time.Millisecond)))
	heap.Push(&h, newTimerListForHeapTest(2, now.Add(2*time.Millisecond)))

	for _, want := range []int64{1, 2, 3} {
		list := heap.Pop(&h).(*timerList)
		if list.key != want {
			t.Fatalf("popped key = %d, want %d", list.key, want)
		}
		if list.heapIndex != -1 {
			t.Fatalf("popped list heapIndex = %d, want -1", list.heapIndex)
		}
	}

	h = make(timerListHeap, 0)
	late := newTimerListForHeapTest(7, now.Add(7*time.Millisecond))
	early := newTimerListForHeapTest(7, now.Add(7*time.Millisecond-time.Microsecond))
	heap.Push(&h, late)
	heap.Push(&h, early)
	if got := heap.Pop(&h).(*timerList); got != early {
		t.Fatalf("same-key tie-break popped %p, want earlier list %p", got, early)
	}
}

// TestTimerListHeapSwapFixAndRemoveIndexes verifies heap operations maintain
// list heapIndex values so deadline updates and removals target the right list.
func TestTimerListHeapSwapFixAndRemoveIndexes(t *testing.T) {
	now := time.Now()
	h := make(timerListHeap, 0, 3)
	heap.Push(&h, newTimerListForHeapTest(10, now.Add(10*time.Millisecond)))
	heap.Push(&h, newTimerListForHeapTest(20, now.Add(20*time.Millisecond)))
	heap.Push(&h, newTimerListForHeapTest(30, now.Add(30*time.Millisecond)))
	for i, list := range h {
		if list.heapIndex != i {
			t.Fatalf("after init list %d heapIndex = %d, want %d", list.key, list.heapIndex, i)
		}
	}

	h[2].key = 1
	h[2].deadline = now.Add(time.Millisecond)
	heap.Fix(&h, h[2].heapIndex)
	if h[0].key != 1 || h[0].heapIndex != 0 {
		t.Fatalf("heap.Fix root key/index = %d/%d, want 1/0", h[0].key, h[0].heapIndex)
	}

	removed := heap.Remove(&h, h[1].heapIndex).(*timerList)
	if removed.heapIndex != -1 {
		t.Fatalf("removed list heapIndex = %d, want -1", removed.heapIndex)
	}
	for i, list := range h {
		if list.heapIndex != i {
			t.Fatalf("remaining list %d heapIndex = %d, want %d", list.key, list.heapIndex, i)
		}
	}
}

// TestTimerListHeapLargeRandomDeadlines exercises a large number of distinct
// deadline buckets without depending on the removed one-node-per-timer heap.
func TestTimerListHeapLargeRandomDeadlines(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large deadline-list heap test in short mode")
	}
	now := time.Now()
	random := rand.New(rand.NewSource(0x54494d4552))
	h := make(timerListHeap, 0, 5000)
	for i := range 5000 {
		offset := random.Int63n(10000)
		heap.Push(&h, newTimerListForHeapTest(offset, now.Add(time.Duration(offset)*time.Microsecond)))
		if h[i].heapIndex < 0 {
			t.Fatalf("inserted list %d has invalid heapIndex", i)
		}
	}

	var prevKey int64 = -1
	for h.Len() > 0 {
		list := heap.Pop(&h).(*timerList)
		if list.key < prevKey {
			t.Fatalf("deadline heap popped key %d after %d", list.key, prevKey)
		}
		if list.heapIndex != -1 {
			t.Fatalf("popped list heapIndex = %d, want -1", list.heapIndex)
		}
		prevKey = list.key
	}
}

// TestDeadlineListLoopIntegrationFiresTimerOrder verifies the public scheduler
// still fires timers in deadline order after switching to deadline buckets.
func TestDeadlineListLoopIntegrationFiresTimerOrder(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)
	fired := make(chan int, 3)
	for _, item := range []struct {
		delay time.Duration
		value int
	}{
		{30 * time.Millisecond, 3},
		{10 * time.Millisecond, 1},
		{20 * time.Millisecond, 2},
	} {
		if _, err := loop.ScheduleTimer(item.delay, func() { fired <- item.value }); err != nil {
			t.Fatalf("ScheduleTimer(%d): %v", item.value, err)
		}
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	for want := 1; want <= 3; want++ {
		if got := waitContractValue(t, fired, "ordered deadline timer"); got != want {
			t.Fatalf("fired value = %d, want %d", got, want)
		}
	}
	if err := waitContractValue(t, runDone, "deadline-order auto-exit completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestTimerListReuseClearsDetachedState(t *testing.T) {
	loop := New()
	now := time.Now()
	timer := &timer{id: 1, when: now, task: func() {}, heapIndex: -1}
	loop.pushTimerNode(timer)
	first := timer.list

	list := loop.popTimerList()
	if head := detachTimerList(list); head != timer {
		t.Fatalf("detached head = %p, want %p", head, timer)
	}
	loop.releaseTimerList(list)
	if loop.timerListSpare != first {
		t.Fatalf("timer list spare = %p, want %p", loop.timerListSpare, first)
	}
	if first.head != nil || first.tail != nil || first.len != 0 || first.key != 0 || !first.deadline.IsZero() || first.heapIndex != -1 {
		t.Fatalf("released timer list retained state: %+v", *first)
	}

	timer.when = now.Add(time.Second)
	loop.pushTimerNode(timer)
	if timer.list != first {
		t.Fatalf("reused timer list = %p, want %p", timer.list, first)
	}
	if loop.timerListSpare != nil {
		t.Fatalf("timer list spare remained published while active: %p", loop.timerListSpare)
	}
	loop.unlinkTimerNode(timer)
	if timer.list != nil || loop.timerListSpare != first {
		t.Fatalf("unlinked timer/list ownership = (%p, %p), want (nil, %p)", timer.list, loop.timerListSpare, first)
	}
}
