package eventloop

import (
	"container/heap"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

var timerPool = sync.Pool{New: func() any { return new(timer) }}

// TimerID uniquely identifies a scheduled timer and can be used to cancel it.
type TimerID uint64

// timer represents a scheduled task
type timer struct {
	when          time.Time
	task          func()
	retire        func()
	prev          *timer
	next          *timer
	list          *timerList
	publication   <-chan struct{}
	id            TimerID
	scheduledTick uint64
	interval      time.Duration
	heapIndex     int
	canceled      atomic.Bool
	deferTick     bool        // true until the turn that scheduled this timer ends
	executing     bool        // true while runTimers owns this callback invocation
	repeat        bool        // true for native repeating interval nodes
	control       bool        // true for host-adapter plumbing excluded from user callback metrics
	refed         atomic.Bool // default true; when false, timer doesn't keep loop alive
}

type timerList struct {
	deadline  time.Time
	head      *timer
	tail      *timer
	key       int64
	heapIndex int
	len       int
}

type timerListHeap []*timerList

func (h timerListHeap) Len() int { return len(h) }

func (h timerListHeap) Less(i, j int) bool {
	if h[i].key == h[j].key {
		return h[i].deadline.Before(h[j].deadline)
	}
	return h[i].key < h[j].key
}

func (h timerListHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIndex = i
	h[j].heapIndex = j
}

func (h *timerListHeap) Push(x any) {
	n := len(*h)
	list := x.(*timerList)
	list.heapIndex = n
	*h = append(*h, list)
}

func (h *timerListHeap) Pop() any {
	old := *h
	n := len(old)
	list := old[n-1]
	old[n-1] = nil
	list.heapIndex = -1
	next := old[:n-1]
	if len(next) == 0 {
		next = resetRetainedSlice(next, retainedTimerHeapCapacity)
	}
	*h = next
	return list
}

func (x *Loop) prepareTimerCommit(t *timer) {
	if t == nil {
		return
	}
	// Timers admitted while a tick is already executing must not fire in that
	// same tick's later timer phase. This preserves the Node/libuv phase boundary:
	// a timeout scheduled from poll/check/close/timer callback code is eligible no
	// earlier than the next event-loop iteration.
	t.deferTick = x.tickActive
	t.scheduledTick = x.tickCount
}

func (x *Loop) timerBucketKey(when time.Time) int64 {
	if x.timerEpoch.IsZero() {
		x.timerEpoch = time.Now()
	}
	delta := when.Sub(x.timerEpoch)
	if delta <= 0 {
		return 0
	}
	return delta.Milliseconds()
}

func (x *Loop) commitTimer(t *timer) {
	if t == nil {
		return
	}
	x.prepareTimerCommit(t)
	x.timerMap = retainedMapStore(x.timerMap, &x.timerMapRetention, t.id, t)
	x.pushTimerNode(t)
	if t.refed.Load() {
		x.refedTimerCount.Add(1)
	}
}

func (x *Loop) pushTimerNode(t *timer) {
	if t == nil {
		return
	}
	key := x.timerBucketKey(t.when)
	list := x.timerLists[key]
	if list == nil {
		list = x.acquireTimerList(key, t.when)
		x.timerLists = retainedMapStore(x.timerLists, &x.timerListsRetention, key, list)
		heap.Push(&x.timers, list)
	} else if t.when.Before(list.deadline) {
		list.deadline = t.when
		if list.heapIndex >= 0 && list.heapIndex < len(x.timers) {
			heap.Fix(&x.timers, list.heapIndex)
		}
	}
	x.insertTimerNode(list, t)
}

func (x *Loop) acquireTimerList(key int64, deadline time.Time) *timerList {
	list := x.timerListSpare
	if list == nil {
		list = new(timerList)
	} else {
		x.timerListSpare = nil
	}
	list.key = key
	list.deadline = deadline
	list.heapIndex = -1
	return list
}

func (x *Loop) releaseTimerList(list *timerList) {
	if list == nil {
		return
	}
	*list = timerList{heapIndex: -1}
	if x.timerListSpare == nil {
		x.timerListSpare = list
	}
}

func timerNodeBefore(a, b *timer) bool {
	return a.when.Before(b.when)
}

func (x *Loop) insertTimerNode(list *timerList, t *timer) {
	if list.head == nil || timerNodeBefore(t, list.head) {
		t.prev = nil
		t.next = list.head
		t.list = list
		t.heapIndex = -1
		if list.head != nil {
			list.head.prev = t
		} else {
			list.tail = t
		}
		list.head = t
		list.len++
		list.deadline = t.when
		return
	}
	if list.tail != nil && !timerNodeBefore(t, list.tail) {
		t.prev = list.tail
		t.next = nil
		t.list = list
		t.heapIndex = -1
		list.tail.next = t
		list.tail = t
		list.len++
		return
	}
	for cur := list.head.next; cur != nil; cur = cur.next {
		if timerNodeBefore(t, cur) {
			t.prev = cur.prev
			t.next = cur
			t.list = list
			t.heapIndex = -1
			cur.prev.next = t
			cur.prev = t
			list.len++
			return
		}
	}
	t.prev = list.tail
	t.next = nil
	t.list = list
	t.heapIndex = -1
	if list.tail != nil {
		list.tail.next = t
	} else {
		list.head = t
	}
	list.tail = t
	list.len++
}

func (x *Loop) unlinkTimerNode(t *timer) {
	if t == nil || t.list == nil {
		return
	}
	list := t.list
	if t.prev != nil {
		t.prev.next = t.next
	} else {
		list.head = t.next
	}
	if t.next != nil {
		t.next.prev = t.prev
	} else {
		list.tail = t.prev
	}
	t.prev = nil
	t.next = nil
	t.list = nil
	list.len--
	if list.len == 0 {
		if list.heapIndex >= 0 && list.heapIndex < len(x.timers) {
			heap.Remove(&x.timers, list.heapIndex)
		}
		x.deleteTimerList(list.key)
		x.releaseTimerList(list)
		return
	}
	x.refreshTimerListDeadline(list)
}

func (x *Loop) refreshTimerListDeadline(list *timerList) {
	if list == nil || list.head == nil {
		return
	}
	// Timer lists are kept sorted by exact deadline with stable insertion order. The head
	// is therefore always the bucket's next true deadline; avoid rescanning large
	// same-deadline buckets during cleanup-heavy benchmark and cancellation paths.
	list.deadline = list.head.when
	if list.heapIndex >= 0 && list.heapIndex < len(x.timers) {
		heap.Fix(&x.timers, list.heapIndex)
	}
}

func (x *Loop) popTimerList() *timerList {
	if len(x.timers) == 0 {
		return nil
	}
	list := heap.Pop(&x.timers).(*timerList)
	x.deleteTimerList(list.key)
	return list
}

func (x *Loop) deleteTimerList(key int64) {
	var rebuilt bool
	x.timerLists, rebuilt = retainedMapDelete(x.timerLists, &x.timerListsRetention, key)
	if rebuilt {
		x.rebuildTimerHeap()
	}
}

func (x *Loop) rebuildTimerHeap() {
	if len(x.timers) == 0 {
		x.timers = nil
		return
	}
	replacement := make(timerListHeap, len(x.timers))
	copy(replacement, x.timers)
	for index, list := range replacement {
		list.heapIndex = index
	}
	x.timers = replacement
}

func (x *Loop) deleteTimer(id TimerID) {
	x.timerMap, _ = retainedMapDelete(x.timerMap, &x.timerMapRetention, id)
}

func detachTimerList(list *timerList) *timer {
	if list == nil {
		return nil
	}
	head := list.head
	for t := head; t != nil; t = t.next {
		t.list = nil
		t.prev = nil
	}
	list.head = nil
	list.tail = nil
	list.len = 0
	return head
}

func (x *Loop) nextTimerDeadline() (time.Time, bool) {
	if len(x.timers) == 0 {
		return time.Time{}, false
	}
	return x.timers[0].deadline, true
}

func (x *Loop) rescheduleRepeatingTimer(t *timer, callbackStart time.Time) {
	if t == nil || !t.repeat || t.canceled.Load() {
		return
	}
	t.when = callbackStart.Add(t.interval)
	x.prepareTimerCommit(t)
	x.pushTimerNode(t)
}

func resetTimerForPool(t *timer) {
	if t == nil {
		return
	}
	retire := t.retire
	t.retire = nil
	t.when = time.Time{}
	t.prev = nil
	t.next = nil
	t.list = nil
	t.id = 0
	t.heapIndex = -1
	t.interval = 0
	t.scheduledTick = 0
	t.publication = nil
	t.deferTick = false
	t.executing = false
	t.repeat = false
	t.control = false
	t.canceled.Store(false)
	t.refed.Store(false)
	t.task = nil
	if retire != nil {
		retire()
	}
}

func (x *Loop) cleanupTimers() {
	x.timers = discardSlice(x.timers)
	for _, list := range x.timerLists {
		x.releaseTimerList(list)
	}
	x.timerLists = discardRetainedMap(x.timerLists, &x.timerListsRetention)
	for _, t := range x.timerMap {
		t.task = nil
		t.refed.Store(false)
		t.canceled.Store(true)
		resetTimerForPool(t)
		timerPool.Put(t)
	}
	x.timerMap = discardRetainedMap(x.timerMap, &x.timerMapRetention)
	x.timerListSpare = nil
	x.refedTimerCount.Store(0)
}

// runTimers executes all expired timers.
func (x *Loop) runTimers() {
	if x.hardAbortRequested() {
		return
	}
	x.drainCommandIngress()
	if x.autoExitReady() {
		return
	}
	// Startup materialization intentionally occurs before an active turn so
	// timers admitted by bootstrap microtasks remain eligible for the startup
	// timer pass. Once the timer phase itself begins, every later admission must
	// observe an active turn and defer until tickCount advances. Normal tick and
	// runAux callers already set this marker; preserve their outer ownership.
	previousTickActive := x.tickActive
	x.tickActive = true
	defer func() { x.tickActive = previousTickActive }()
	now := x.refreshTickTime()
	var deferred []*timer
	defer func() {
		for _, t := range deferred {
			if t == nil {
				continue
			}
			if t.canceled.Load() {
				x.retireTimer(t)
				continue
			}
			x.pushTimerNode(t)
		}
	}()
	for len(x.timers) > 0 {
		if x.timers[0].deadline.After(now) {
			break
		}
		list := x.popTimerList()
		t := detachTimerList(list)
		x.releaseTimerList(list)
		for t != nil {
			next := t.next
			t.next = nil
			if t.canceled.Load() {
				x.retireTimer(t)
				t = next
				continue
			}
			if t.when.After(now) || (t.deferTick && t.scheduledTick == x.tickCount) {
				deferred = append(deferred, t)
				t = next
				continue
			}

			if x.testHooks != nil && x.testHooks.BeforeTimerExecutionClaim != nil {
				x.testHooks.BeforeTimerExecutionClaim(t.id)
			}
			// A false pending observation is the uncontended callback-entry claim:
			// every earlier command would have published true before its payload.
			// When work is pending, externalMu instead joins all earlier publishers
			// into one drain, cancellation check, and executing-state commitment.
			if !x.commandIngressPending.Load() {
				if t.canceled.Load() {
					x.retireTimer(t)
					t = next
					continue
				}
				t.executing = true
			} else {
				x.externalMu.Lock()
				x.drainCommandIngressLocked()
				if x.testHooks != nil && x.testHooks.AfterTimerExecutionIngressDrain != nil {
					x.testHooks.AfterTimerExecutionIngressDrain(t.id)
				}
				if t.canceled.Load() {
					x.externalMu.Unlock()
					x.retireTimer(t)
					t = next
					continue
				}
				t.executing = true
				x.externalMu.Unlock()
			}
			if x.testHooks != nil && x.testHooks.BeforeTimerPublicationWait != nil {
				x.testHooks.BeforeTimerPublicationWait(t.id)
			}
			if t.publication != nil {
				<-t.publication
			}
			var repeatStart time.Time
			if t.repeat {
				// Node repeating timers anchor the next deadline immediately before
				// callback entry. Callback duration therefore consumes interval time
				// instead of accumulating as permanent drift.
				repeatStart = x.refreshTickTime()
			}
			if t.control {
				x.safeExecuteControl(t.task)
			} else {
				x.safeExecute(t.task)
			}
			t.executing = false
			if t.repeat && !t.canceled.Load() {
				x.rescheduleRepeatingTimer(t, repeatStart)
			} else {
				x.retireTimer(t)
			}

			x.drainMicrotasks()
			if x.hardAbortRequested() {
				for next != nil {
					pending := next
					next = next.next
					pending.next = nil
					pending.prev = nil
					deferred = append(deferred, pending)
				}
				return
			}
			t = next
		}
	}
}

func (x *Loop) retireTimer(t *timer) {
	if t == nil {
		return
	}
	x.unlinkTimerNode(t)
	x.deleteTimer(t.id)
	if t.refed.Load() {
		// Decrement refedTimerCount without incrementing submissionEpoch.
		// This is correct: epoch tracks liveness-*adding* mutations. A timer
		// firing or cancellation reduces liveness, so Alive() returning false
		// after this decrement is the correct outcome.
		x.refedTimerCount.Add(-1)
	}
	resetTimerForPool(t)
	timerPool.Put(t)
}

// ScheduleTimer schedules a task to be executed after the specified delay.
//
// Returns a TimerID that can be used to cancel the timer before it fires.
// A successful call fully publishes that ID before the callback can enter,
// including for an already-due timer scheduled from another goroutine.
//
// The delay is a Go duration and is not modified by JavaScript or HTML timer
// policies. Host adapters own any delay normalization or nesting rules.
// A timer admitted from an executing loop turn, including a startup timer
// callback, is eligible no earlier than a later iteration. Timers admitted by
// bootstrap work before the startup timer phase remain eligible for that phase.
// ScheduleTimer panics if fn is nil.
func (x *Loop) ScheduleTimer(delay time.Duration, fn func()) (TimerID, error) {
	if fn == nil {
		panic("eventloop: nil ScheduleTimer callback")
	}
	return x.scheduleTimerNodeRef(delay, fn, false, true)
}

// ScheduleTimerUnrefed schedules a task without adding loop liveness. The
// timer remains eligible while other referenced work keeps the loop alive.
// Its unreferenced state is part of the initial publication, so concurrent
// liveness observers never see a transient referenced timer.
// ScheduleTimerUnrefed panics if fn is nil.
func (x *Loop) ScheduleTimerUnrefed(delay time.Duration, fn func()) (TimerID, error) {
	if fn == nil {
		panic("eventloop: nil ScheduleTimerUnrefed callback")
	}
	return x.scheduleTimerNodeRef(delay, fn, false, false)
}

// ScheduleControlTimer schedules referenced host-adapter plumbing while
// excluding its execution from user callback latency and throughput metrics.
// Cancellation, liveness, panic containment, microtask checkpoints, and timer
// phase ordering are otherwise identical to [Loop.ScheduleTimer].
//
// ScheduleControlTimer panics if fn is nil.
func (x *Loop) ScheduleControlTimer(delay time.Duration, fn func()) (TimerID, error) {
	if fn == nil {
		panic("eventloop: nil ScheduleControlTimer callback")
	}
	return x.scheduleTimerControlRef(delay, fn, true)
}

// ScheduleControlTimerUnrefed is [Loop.ScheduleControlTimer] without initial
// loop liveness. The timer remains eligible while referenced work exists.
//
// ScheduleControlTimerUnrefed panics if fn is nil.
func (x *Loop) ScheduleControlTimerUnrefed(delay time.Duration, fn func()) (TimerID, error) {
	if fn == nil {
		panic("eventloop: nil ScheduleControlTimerUnrefed callback")
	}
	return x.scheduleTimerControlRef(delay, fn, false)
}

func (x *Loop) scheduleRepeatingTimer(delay time.Duration, fn func()) (TimerID, error) {
	return x.scheduleTimerNodeRef(delay, fn, true, true)
}

func (x *Loop) scheduleTimerNodeRef(delay time.Duration, fn func(), repeat, refed bool) (TimerID, error) {
	return x.scheduleTimerNodeRetireMode(delay, fn, repeat, nil, refed, false)
}

func (x *Loop) scheduleTimerControlRef(delay time.Duration, fn func(), refed bool) (TimerID, error) {
	return x.scheduleTimerNodeRetireMode(delay, fn, false, nil, refed, true)
}

func (x *Loop) scheduleTimerRetire(delay time.Duration, fn, retire func()) (TimerID, error) {
	return x.scheduleTimerNodeRetireMode(delay, fn, false, retire, true, false)
}

func (x *Loop) scheduleTimerNodeRetireMode(
	delay time.Duration,
	fn func(),
	repeat bool,
	retire func(),
	refed bool,
	control bool,
) (TimerID, error) {
	// Timers add liveness and are rejected during active terminal drain,
	// StateTerminating/StateTerminated, and current-epoch quiescing.
	if err := x.rejectLivenessAdd(); err != nil {
		return 0, err
	}

	scheduledAt := time.Now()
	if x.isLoopThread() {
		scheduledAt = x.refreshTickTime()
	}

	publication := make(chan struct{})
	t, err := x.acquireTimerRef(scheduledAt, delay, fn, repeat, retire, publication, refed, control)
	if err != nil {
		close(publication)
		return 0, err
	}
	id := t.id

	// On the logical owner: register synchronously. This bypasses
	// SubmitInternal which may queue the task in I/O mode (when
	// canUseFastPath is false), causing Schedule-then-Unref from a
	// loop callback to race if the unref arrives before the queued
	// registration is processed.
	if x.isLoopThread() {
		x.livenessMu.Lock()
		if err := x.rejectLivenessAddLocked(); err != nil {
			x.livenessMu.Unlock()
			close(publication)
			resetTimerForPool(t)
			timerPool.Put(t)
			return 0, err
		}
		x.materializeCommandIngress()
		x.commitTimer(t)
		x.submissionEpoch.Add(1)
		if x.testHooks != nil && x.testHooks.BeforeScheduleTimerReturn != nil {
			x.testHooks.BeforeScheduleTimerReturn(id)
		}
		close(publication)
		x.livenessMu.Unlock()
		return id, nil
	}

	// External goroutine: use submitToQueue (not SubmitInternal) to
	// skip the redundant isLoopThread() check. We already proved
	// we're not on the logical owner above.
	if x.testHooks != nil && x.testHooks.BeforeScheduleTimerCommit != nil {
		x.testHooks.BeforeScheduleTimerCommit()
	}
	err = x.submitLivenessCommand(loopCommand{kind: loopCommandTimerAdd, timer: t}, nil)
	if err != nil {
		// Put back to pool on error.
		// Reset all pooled timer fields before reuse.
		close(publication)
		resetTimerForPool(t)
		timerPool.Put(t)
		return 0, err
	}

	if x.testHooks != nil && x.testHooks.BeforeScheduleTimerReturn != nil {
		x.testHooks.BeforeScheduleTimerReturn(id)
	}
	close(publication)
	return id, nil
}

func (x *Loop) acquireTimer(
	scheduledAt time.Time,
	delay time.Duration,
	fn func(),
	repeat bool,
	retire func(),
	publication chan struct{},
) (*timer, error) {
	return x.acquireTimerRef(scheduledAt, delay, fn, repeat, retire, publication, true, false)
}

func (x *Loop) acquireTimerRef(
	scheduledAt time.Time,
	delay time.Duration,
	fn func(),
	repeat bool,
	retire func(),
	publication chan struct{},
	refed bool,
	control bool,
) (*timer, error) {
	t := timerPool.Get().(*timer)
	id, ok := allocateID(&x.nextTimerID, math.MaxUint64)
	if !ok {
		t.retire = retire
		resetTimerForPool(t)
		timerPool.Put(t)
		return nil, ErrTimerIDExhausted
	}
	t.id = TimerID(id)
	t.when = scheduledAt.Add(delay)
	t.task = fn
	t.retire = retire
	t.publication = publication
	t.interval = delay
	t.scheduledTick = 0
	t.deferTick = false
	t.repeat = repeat
	t.control = control
	t.canceled.Store(false)
	t.refed.Store(refed)
	t.heapIndex = -1
	return t, nil
}
