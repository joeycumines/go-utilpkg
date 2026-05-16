package eventloop

import (
	"context"
	"math"
	"slices"
	"strconv"
	"testing"
	"time"
)

func TestCleanupTimersClearsHeapAndListReferences(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	when := time.Now().Add(time.Hour)
	first := &timer{id: 1, when: when, task: func() {}, heapIndex: -1}
	second := &timer{id: 2, when: when.Add(time.Millisecond), task: func() {}, heapIndex: -1}
	first.refed.Store(true)
	second.refed.Store(true)
	loop.commitTimer(first)
	loop.commitTimer(second)

	heapBacking := loop.timers[:cap(loop.timers)]
	lists := make([]*timerList, 0, len(loop.timerLists))
	for _, list := range loop.timerLists {
		lists = append(lists, list)
	}

	loop.cleanupTimers()

	if len(loop.timers) != 0 || len(loop.timerLists) != 0 || len(loop.timerMap) != 0 {
		t.Fatalf("cleanup retained timer state: heap=%d lists=%d map=%d", len(loop.timers), len(loop.timerLists), len(loop.timerMap))
	}
	for index, list := range heapBacking {
		if list != nil {
			t.Errorf("heap backing slot %d retained timer list %p", index, list)
		}
	}
	for index, list := range lists {
		if list.head != nil || list.tail != nil || list.len != 0 || list.heapIndex != -1 {
			t.Errorf("timer list %d retained state: head=%p tail=%p len=%d heapIndex=%d", index, list.head, list.tail, list.len, list.heapIndex)
		}
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refed timer count after cleanup = %d, want 0", got)
	}
}

func TestTimerTurnDeferralSurvivesTickCounterWrap(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	loop.tickCount = math.MaxUint64
	loop.tickActive = true
	fired := false
	timer := &timer{id: 1, when: time.Now().Add(-time.Millisecond), task: func() { fired = true }, heapIndex: -1}
	timer.refed.Store(true)
	loop.commitTimer(timer)
	loop.tickActive = false

	loop.runTimers()
	if fired {
		t.Fatal("timer scheduled during active turn fired in that turn")
	}
	if _, ok := loop.timerMap[timer.id]; !ok {
		t.Fatal("deferred timer disappeared at tick counter boundary")
	}

	loop.tickCount = 0
	loop.runTimers()
	if !fired {
		t.Fatal("deferred timer did not fire in the next wrapped turn")
	}
}

func TestTimerQueuedBeforeTurnRunsInThatTurn(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	fired := false
	if _, err := loop.ScheduleTimer(0, func() { fired = true }); err != nil {
		t.Fatal(err)
	}
	loop.tick()
	if !fired {
		t.Fatal("timer accepted before the turn was deferred as in-turn work")
	}
}

func TestStartupTimerPhaseDefersNestedTimer(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	var order []string
	if err := loop.ScheduleImmediate(func() { order = append(order, "check") }); err != nil {
		t.Fatalf("ScheduleImmediate: %v", err)
	}
	var nestedErr error
	if _, err := loop.ScheduleTimer(-time.Hour, func() {
		order = append(order, "outer")
		_, nestedErr = loop.ScheduleTimer(-time.Hour, func() { order = append(order, "nested") })
	}); err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if nestedErr != nil {
		t.Fatalf("nested ScheduleTimer: %v", nestedErr)
	}
	if want := []string{"outer", "check", "nested"}; !slices.Equal(order, want) {
		t.Fatalf("startup phase order = %v, want %v", order, want)
	}
}

func TestStartupMicrotaskTimerRemainsStartupEligible(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	var order []string
	if err := loop.ScheduleImmediate(func() { order = append(order, "check") }); err != nil {
		t.Fatalf("ScheduleImmediate: %v", err)
	}
	var timerErr error
	if err := loop.ScheduleMicrotask(func() {
		order = append(order, "microtask")
		_, timerErr = loop.ScheduleTimer(-time.Hour, func() { order = append(order, "timer") })
	}); err != nil {
		t.Fatalf("ScheduleMicrotask: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if timerErr != nil {
		t.Fatalf("microtask ScheduleTimer: %v", timerErr)
	}
	if want := []string{"microtask", "timer", "check"}; !slices.Equal(order, want) {
		t.Fatalf("startup bootstrap order = %v, want %v", order, want)
	}
}

func TestRefedTimerCountExceedsInt32(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	timer := &timer{id: 1, heapIndex: -1}
	loop.timerMap[timer.id] = timer
	loop.refedTimerCount.Store(math.MaxInt32)
	loop.applyTimerRefChange(timer.id, true)
	if got, want := loop.refedTimerCount.Load(), int64(math.MaxInt32)+1; got != want {
		t.Fatalf("refed timer count = %d, want %d", got, want)
	}
	loop.applyTimerRefChange(timer.id, false)
	if got, want := loop.refedTimerCount.Load(), int64(math.MaxInt32); got != want {
		t.Fatalf("refed timer count after unref = %d, want %d", got, want)
	}
}

func TestJSTimerDelayOverflowPanicsBeforeAdmission(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("int cannot represent an overflowing millisecond delay")
	}
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	overflowMilliseconds := int64(math.MaxInt64/int64(time.Millisecond) + 1)
	overflow := int(overflowMilliseconds)
	for _, test := range []struct {
		name string
		call func()
	}{
		{name: "timeout", call: func() { _, _ = js.SetTimeout(func() {}, overflow) }},
		{name: "interval", call: func() { _, _ = js.SetInterval(func() {}, overflow) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := js.nextTimerID.Load()
			if got := captureErrorContractPanic(test.call); got == nil {
				t.Fatal("overflowing delay did not panic")
			}
			if got := js.nextTimerID.Load(); got != before {
				t.Fatalf("timer ID advanced after rejected delay: got %d, want %d", got, before)
			}
		})
	}
	js.timeoutsMu.Lock()
	timeoutCount := len(js.timeouts)
	js.timeoutsMu.Unlock()
	js.intervalsMu.Lock()
	intervalCount := len(js.intervals)
	js.intervalsMu.Unlock()
	if timeoutCount != 0 || intervalCount != 0 {
		t.Fatalf("rejected delays published handles: timeouts=%d intervals=%d", timeoutCount, intervalCount)
	}
}
