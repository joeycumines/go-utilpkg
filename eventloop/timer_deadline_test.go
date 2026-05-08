package eventloop

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goroutineid"
)

func resetTestTimerLists(l *Loop) {
	l.timers = make(timerListHeap, 0)
	l.timerLists = make(map[int64]*timerList)
}

func pushTestTimer(l *Loop, t *timer) {
	if l.timerLists == nil {
		l.timerLists = make(map[int64]*timerList)
	}
	l.pushTimerNode(t)
}

func TestDeadlineListSameBucketCancelAndFIFO(t *testing.T) {
	loop := New()
	loop.state.Store(StateRunning)
	loop.tickCount = 1

	when := time.Now().Add(-time.Millisecond)
	var order []int
	for i := 1; i <= 3; i++ {
		i := i
		tm := &timer{
			id:        TimerID(i),
			when:      when,
			heapIndex: -1,
			task:      func() { order = append(order, i) },
		}
		tm.refed.Store(true)
		loop.timerMap[tm.id] = tm
		loop.refedTimerCount.Add(1)
		pushTestTimer(loop, tm)
	}

	if got := len(loop.timers); got != 1 {
		t.Fatalf("deadline bucket count = %d, want 1", got)
	}
	if err := loop.applyCancelTimer(2); err != nil {
		t.Fatalf("applyCancelTimer(2): %v", err)
	}

	loop.runTimers()
	if !reflect.DeepEqual(order, []int{1, 3}) {
		t.Fatalf("timer order = %v, want [1 3]", order)
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount = %d, want 0", got)
	}
	if got := len(loop.timerMap); got != 0 {
		t.Fatalf("timerMap entries = %d, want 0", got)
	}
}

func TestDeadlineListSameDeadlineDrainsMicrotaskBetweenCallbacks(t *testing.T) {
	loop := New()
	loop.state.Store(StateRunning)
	loop.tickCount = 1
	loop.loopGoroutineID.Store(goroutineid.Get())
	defer loop.loopGoroutineID.Store(0)

	when := time.Now().Add(-time.Millisecond)
	var order []string
	var scheduleErr error
	first := &timer{
		id:        1,
		when:      when,
		heapIndex: -1,
		task: func() {
			order = append(order, "timer1")
			scheduleErr = loop.ScheduleMicrotask(func() { order = append(order, "microtask") })
		},
	}
	second := &timer{
		id:        2,
		when:      when,
		heapIndex: -1,
		task:      func() { order = append(order, "timer2") },
	}
	for _, timer := range []*timer{first, second} {
		timer.refed.Store(true)
		loop.timerMap[timer.id] = timer
		loop.refedTimerCount.Add(1)
		pushTestTimer(loop, timer)
	}

	loop.runTimers()
	if scheduleErr != nil {
		t.Fatalf("ScheduleMicrotask: %v", scheduleErr)
	}
	if !reflect.DeepEqual(order, []string{"timer1", "microtask", "timer2"}) {
		t.Fatalf("same-deadline callback order = %v, want [timer1 microtask timer2]", order)
	}
}

func TestDeadlineListSameBucketOrdersExactDeadlines(t *testing.T) {
	loop := New()
	resetTestTimerLists(loop)
	loop.state.Store(StateRunning)
	loop.tickCount = 1
	base := time.Now().Add(-time.Second)
	loop.timerEpoch = base

	var order []int
	for _, item := range []struct {
		id     TimerID
		offset time.Duration
	}{
		{id: 1, offset: 900 * time.Microsecond},
		{id: 2, offset: 100 * time.Microsecond},
	} {
		tm := &timer{
			id:        item.id,
			when:      base.Add(item.offset),
			heapIndex: -1,
			task:      func() { order = append(order, int(item.id)) },
		}
		tm.refed.Store(true)
		loop.timerMap[tm.id] = tm
		loop.refedTimerCount.Add(1)
		pushTestTimer(loop, tm)
	}
	if got := len(loop.timers); got != 1 {
		t.Fatalf("deadline bucket count = %d, want 1", got)
	}

	loop.runTimers()
	if !reflect.DeepEqual(order, []int{2, 1}) {
		t.Fatalf("timer order = %v, want exact-deadline order [2 1]", order)
	}
}

func TestDeadlineListCancelDetachedPendingTimerRemovesLiveness(t *testing.T) {
	loop := New()
	resetTestTimerLists(loop)
	loop.state.Store(StateRunning)
	loop.tickCount = 1
	when := time.Now().Add(-time.Millisecond)

	var countDuringCancel int64
	timerA := &timer{
		id:        1,
		when:      when,
		heapIndex: -1,
		task: func() {
			if err := loop.applyCancelTimer(2); err != nil {
				t.Errorf("applyCancelTimer detached pending: %v", err)
			}
			if err := loop.applyCancelTimer(2); err != ErrTimerNotFound {
				t.Errorf("second applyCancelTimer detached pending = %v, want %v", err, ErrTimerNotFound)
			}
			countDuringCancel = loop.refedTimerCount.Load()
		},
	}
	timerB := &timer{
		id:            2,
		when:          when,
		scheduledTick: 1,
		deferTick:     true,
		heapIndex:     -1,
		task:          func() { t.Fatal("canceled ineligible timer fired") },
	}
	for _, tm := range []*timer{timerA, timerB} {
		tm.refed.Store(true)
		loop.timerMap[tm.id] = tm
		loop.refedTimerCount.Add(1)
		pushTestTimer(loop, tm)
	}

	loop.runTimers()
	if countDuringCancel != 1 {
		t.Fatalf("refedTimerCount during detached cancel = %d, want only executing timer count 1", countDuringCancel)
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount after runTimers = %d, want 0", got)
	}
	if got := len(loop.timerMap); got != 0 {
		t.Fatalf("timerMap entries after runTimers = %d, want 0", got)
	}
}

func TestDeadlineListExecutingTimerSelfCancellation(t *testing.T) {
	loop := New()
	t.Cleanup(loop.stopCallbackWorker)
	resetTestTimerLists(loop)
	loop.state.Store(StateRunning)
	loop.tickCount = 1
	loop.loopGoroutineID.Store(goroutineid.Get())
	defer loop.loopGoroutineID.Store(0)

	when := time.Now().Add(-time.Millisecond)
	const timerAID TimerID = 1
	var cancelErr error
	var repeatCancelErr error
	var siblingCalls int
	timerA := &timer{
		id:        timerAID,
		when:      when,
		heapIndex: -1,
		task: func() {
			cancelErr = loop.CancelTimer(timerAID)
			repeatCancelErr = loop.CancelTimer(timerAID)
		},
	}
	timerB := &timer{
		id:        2,
		when:      when,
		heapIndex: -1,
		task:      func() { siblingCalls++ },
	}
	for _, tm := range []*timer{timerA, timerB} {
		tm.refed.Store(true)
		loop.timerMap[tm.id] = tm
		loop.refedTimerCount.Add(1)
		pushTestTimer(loop, tm)
	}

	loop.runTimers()
	if cancelErr != nil {
		t.Fatalf("CancelTimer executing self: %v", cancelErr)
	}
	if repeatCancelErr != ErrTimerNotFound {
		t.Fatalf("second CancelTimer executing self = %v, want ErrTimerNotFound", repeatCancelErr)
	}
	if siblingCalls != 1 {
		t.Fatalf("unaffected sibling calls = %d, want 1", siblingCalls)
	}
	if err := loop.CancelTimer(timerAID); err != ErrTimerNotFound {
		t.Fatalf("CancelTimer retired self = %v, want ErrTimerNotFound", err)
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount after self-cancellation = %d, want 0", got)
	}
	if got := len(loop.timerMap); got != 0 {
		t.Fatalf("timerMap after self-cancellation = %d entries, want 0", got)
	}
}

func TestRepeatingTimerReschedulesFromCallbackStart(t *testing.T) {
	loop := New()
	resetTestTimerLists(loop)
	loop.tickActive = true
	loop.tickCount = 17

	callbackStart := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	tm := &timer{
		id:        1,
		interval:  25 * time.Millisecond,
		heapIndex: -1,
		repeat:    true,
	}
	loop.rescheduleRepeatingTimer(tm, callbackStart)

	want := callbackStart.Add(tm.interval)
	if !tm.when.Equal(want) {
		t.Fatalf("repeating deadline = %v, want callback start + interval %v", tm.when, want)
	}
	if !tm.deferTick || tm.scheduledTick != loop.tickCount {
		t.Fatalf("repeating timer turn guard = (defer=%v tick=%d), want (true, %d)", tm.deferTick, tm.scheduledTick, loop.tickCount)
	}
}

func TestNativeIntervalStableTimerIDAndUnrefAcrossTicks(t *testing.T) {
	loop := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitLoopOwnerTurnT(t, loop)
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-runDone:
			if err != context.Canceled {
				t.Fatalf("Run after cancel = %v, want %v", err, context.Canceled)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Run did not exit")
		}
	})

	js := NewJS(loop)

	var intervalID atomic.Uint64
	timerIDs := make(chan TimerID, 4)
	id, err := js.SetInterval(func() {
		id := intervalID.Load()
		if id == 0 {
			return
		}
		js.intervalsMu.RLock()
		state := js.intervals[id]
		js.intervalsMu.RUnlock()
		if state != nil {
			timerIDs <- TimerID(state.currentLoopTimerID.Load())
		}
	}, 10)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}
	intervalID.Store(id)

	first := receiveTimerID(t, timerIDs)
	if first == 0 {
		t.Fatal("first native interval timer ID is zero")
	}
	if err := js.UnrefInterval(id); err != nil {
		t.Fatalf("UnrefInterval: %v", err)
	}
	second := receiveTimerID(t, timerIDs)
	if second != first {
		t.Fatalf("native interval timer ID changed from %d to %d", first, second)
	}

	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("SubmitInternal barrier: %v", err)
	}
	select {
	case <-barrier:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for barrier")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount after unrefed native interval tick = %d, want 0", got)
	}
	if err := js.ClearInterval(id); err != nil {
		t.Fatalf("ClearInterval: %v", err)
	}
}

func TestNativeIntervalZeroDelayDoesNotRepeatSameTick(t *testing.T) {
	loop := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitLoopOwnerTurnT(t, loop)
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-runDone:
			if err != context.Canceled {
				t.Fatalf("Run after cancel = %v, want %v", err, context.Canceled)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Run did not exit")
		}
	})

	js := NewJS(loop)

	ticks := make(chan uint64, 3)
	clearResult := make(chan error, 1)
	created := make(chan error, 1)
	var intervalID uint64
	if err := loop.Submit(func() {
		var scheduleErr error
		intervalID, scheduleErr = js.SetInterval(func() {
			ticks <- loop.tickCount
			if len(ticks) == cap(ticks) {
				clearResult <- js.ClearInterval(intervalID)
			}
		}, 0)
		created <- scheduleErr
	}); err != nil {
		t.Fatalf("Submit SetInterval: %v", err)
	}
	select {
	case err := <-created:
		if err != nil {
			t.Fatalf("SetInterval: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SetInterval did not complete")
	}

	var got []uint64
	for len(got) < 3 {
		select {
		case tick := <-ticks:
			got = append(got, tick)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for interval ticks; got %v", got)
		}
	}
	if err := waitContractValue(t, clearResult, "zero-delay interval cancellation"); err != nil {
		t.Fatalf("ClearInterval: %v", err)
	}
	for i := 1; i < len(got); i++ {
		if got[i] <= got[i-1] {
			t.Fatalf("native interval repeated in same tick sequence: %v", got)
		}
	}
}

func receiveTimerID(t *testing.T, ch <-chan TimerID) TimerID {
	t.Helper()
	select {
	case id := <-ch:
		return id
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for timer ID")
	}
	return 0
}
