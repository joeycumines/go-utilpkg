package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

func startCancelableLoopT(t *testing.T, loop *Loop) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			if err := waitContractValue(t, runDone, "canceled Run completion"); err != context.Canceled {
				t.Errorf("Run after cancellation = %v, want context.Canceled", err)
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

func observeTimerRefCountT(t *testing.T, loop *Loop) int64 {
	t.Helper()
	observed := make(chan int64, 1)
	if err := loop.SubmitInternal(func() { observed <- loop.refedTimerCount.Load() }); err != nil {
		t.Fatalf("timer ref-count observation admission: %v", err)
	}
	return waitContractValue(t, observed, "timer ref-count owner observation")
}

func observeAliveT(t *testing.T, loop *Loop) bool {
	t.Helper()
	observed := make(chan bool, 1)
	if err := loop.SubmitInternal(func() { observed <- loop.Alive() }); err != nil {
		t.Fatalf("liveness observation admission: %v", err)
	}
	return waitContractValue(t, observed, "owner liveness observation")
}

func TestIOMode_RefUnrefFromExternalGoroutine(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	fd, fdCleanup := testCreateIOFD(t)
	t.Cleanup(fdCleanup)
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}
	stop := startCancelableLoopT(t, loop)

	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	if got := observeTimerRefCountT(t, loop); got != 1 {
		t.Fatalf("refedTimerCount after schedule = %d, want 1", got)
	}
	if err := loop.RefTimer(timerID); err != nil {
		t.Fatalf("repeated external RefTimer: %v", err)
	}
	if got := observeTimerRefCountT(t, loop); got != 1 {
		t.Fatalf("refedTimerCount after repeated ref = %d, want 1", got)
	}
	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("external UnrefTimer: %v", err)
	}
	if got := observeTimerRefCountT(t, loop); got != 0 {
		t.Fatalf("refedTimerCount after unref = %d, want 0", got)
	}
	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("repeated external UnrefTimer: %v", err)
	}
	if got := observeTimerRefCountT(t, loop); got != 0 {
		t.Fatalf("refedTimerCount after repeated unref = %d, want 0", got)
	}
	if err := loop.RefTimer(timerID); err != nil {
		t.Fatalf("external RefTimer: %v", err)
	}
	if got := observeTimerRefCountT(t, loop); got != 1 {
		t.Fatalf("refedTimerCount after ref = %d, want 1", got)
	}
	stop()
}

func TestIOMode_AliveWithUnrefdTimer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	fd, fdCleanup := testCreateIOFD(t)
	t.Cleanup(fdCleanup)
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}
	stop := startCancelableLoopT(t, loop)

	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	if got := observeTimerRefCountT(t, loop); got != 1 || !loop.Alive() {
		t.Fatalf("refed timer plus FD = (refs=%d, alive=%v), want (1, true)", got, loop.Alive())
	}
	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}
	if got := observeTimerRefCountT(t, loop); got != 0 || !loop.Alive() {
		t.Fatalf("unrefed timer plus FD = (refs=%d, alive=%v), want (0, true)", got, loop.Alive())
	}
	if err := loop.RefTimer(timerID); err != nil {
		t.Fatalf("RefTimer: %v", err)
	}
	if got := observeTimerRefCountT(t, loop); got != 1 || !loop.Alive() {
		t.Fatalf("re-refed timer plus FD = (refs=%d, alive=%v), want (1, true)", got, loop.Alive())
	}
	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("final UnrefTimer: %v", err)
	}
	if got := observeTimerRefCountT(t, loop); got != 0 {
		t.Fatalf("refedTimerCount after final unref = %d, want 0", got)
	}
	if err := loop.UnregisterFD(fd); err != nil {
		t.Fatalf("UnregisterFD: %v", err)
	}
	if observeAliveT(t, loop) {
		t.Fatal("Alive returned true with only an unrefed timer")
	}
	stop()
}

func TestIOMode_ConcurrentRefUnrefUnderLoad(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	fd, fdCleanup := testCreateIOFD(t)
	t.Cleanup(fdCleanup)
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}
	stop := startCancelableLoopT(t, loop)

	const timerCount = 12
	const contenders = 6
	ids := make([]TimerID, timerCount)
	for index := range ids {
		ids[index], err = loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer %d: %v", index, err)
		}
	}
	if got := observeTimerRefCountT(t, loop); got != timerCount {
		t.Fatalf("refedTimerCount after schedule = %d, want %d", got, timerCount)
	}

	runContenders := func(t *testing.T, operation func(TimerID) error) {
		t.Helper()
		start := make(chan struct{})
		results := make(chan error, contenders)
		joined := make(chan struct{})
		var workers sync.WaitGroup
		workers.Add(contenders)
		for index := range contenders {
			go func() {
				defer workers.Done()
				<-start
				results <- operation(ids[index])
			}()
		}
		go func() {
			workers.Wait()
			close(joined)
		}()
		close(start)
		waitContractSignal(t, joined, "concurrent timer ref operation")
		close(results)
		for result := range results {
			if result != nil {
				t.Fatalf("concurrent timer ref operation: %v", result)
			}
		}
	}

	runContenders(t, loop.UnrefTimer)
	if got := observeTimerRefCountT(t, loop); got != timerCount-contenders {
		t.Fatalf("refedTimerCount after concurrent unref = %d, want %d", got, timerCount-contenders)
	}
	runContenders(t, loop.RefTimer)
	if got := observeTimerRefCountT(t, loop); got != timerCount {
		t.Fatalf("refedTimerCount after concurrent ref = %d, want %d", got, timerCount)
	}
	stop()
}

func TestSubmitTimerRefChange_TerminatedState(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	stop := startCancelableLoopT(t, loop)
	stop()
	if err := loop.RefTimer(timerID); err != ErrLoopTerminated {
		t.Fatalf("RefTimer after termination = %v, want ErrLoopTerminated", err)
	}
	if err := loop.UnrefTimer(timerID); err != ErrLoopTerminated {
		t.Fatalf("UnrefTimer after termination = %v, want ErrLoopTerminated", err)
	}
}

func TestSubmitTimerRefChange_OnLoopGoroutine(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	stop := startCancelableLoopT(t, loop)
	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	type result struct {
		unrefErr   error
		refErr     error
		afterUnref int64
		afterRef   int64
	}
	results := make(chan result, 1)
	if err := loop.SubmitInternal(func() {
		value := result{unrefErr: loop.UnrefTimer(timerID),
			afterUnref: loop.refedTimerCount.Load(),
			refErr:     loop.RefTimer(timerID),
			afterRef:   loop.refedTimerCount.Load()}
		results <- value
	}); err != nil {
		t.Fatalf("owner timer-ref admission: %v", err)
	}
	value := waitContractValue(t, results, "owner timer-ref operations")
	if value.unrefErr != nil || value.refErr != nil || value.afterUnref != 0 || value.afterRef != 1 {
		t.Fatalf("owner timer-ref result = (unrefErr=%v, afterUnref=%d, refErr=%v, afterRef=%d), want (nil, 0, nil, 1)", value.unrefErr, value.afterUnref, value.refErr, value.afterRef)
	}
	stop()
}

func TestAlive_MicrotaskPath(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	stop := startCancelableLoopT(t, loop)
	type result struct {
		alive       bool
		scheduleErr error
	}
	observed := make(chan result, 1)
	microtaskDone := make(chan struct{})
	if err := loop.SubmitInternal(func() {
		value := result{scheduleErr: loop.ScheduleMicrotask(func() { close(microtaskDone) }),
			alive: loop.Alive()}
		observed <- value
	}); err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}
	value := waitContractValue(t, observed, "pending microtask liveness")
	if value.scheduleErr != nil || !value.alive {
		t.Fatalf("pending microtask = (err=%v, alive=%v), want (nil, true)", value.scheduleErr, value.alive)
	}
	waitContractSignal(t, microtaskDone, "microtask completion")
	afterDrain := make(chan bool, 1)
	if err := loop.SubmitInternal(func() { afterDrain <- loop.Alive() }); err != nil {
		t.Fatalf("post-microtask observation: %v", err)
	}
	if waitContractValue(t, afterDrain, "post-microtask liveness") {
		t.Fatal("Alive returned true after the microtask drained")
	}
	stop()
}

func TestAlive_NextTickPath(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	stop := startCancelableLoopT(t, loop)
	type result struct {
		alive       bool
		scheduleErr error
	}
	observed := make(chan result, 1)
	nextTickDone := make(chan struct{})
	if err := loop.SubmitInternal(func() {
		value := result{scheduleErr: loop.ScheduleNextTick(func() { close(nextTickDone) }),
			alive: loop.Alive()}
		observed <- value
	}); err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}
	value := waitContractValue(t, observed, "pending nextTick liveness")
	if value.scheduleErr != nil || !value.alive {
		t.Fatalf("pending nextTick = (err=%v, alive=%v), want (nil, true)", value.scheduleErr, value.alive)
	}
	waitContractSignal(t, nextTickDone, "nextTick completion")
	afterDrain := make(chan bool, 1)
	if err := loop.SubmitInternal(func() { afterDrain <- loop.Alive() }); err != nil {
		t.Fatalf("post-nextTick observation: %v", err)
	}
	if waitContractValue(t, afterDrain, "post-nextTick liveness") {
		t.Fatal("Alive returned true after nextTick drained")
	}
	stop()
}

func TestAlive_UserIOFDOnly(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	fd, fdCleanup := testCreateIOFD(t)
	t.Cleanup(fdCleanup)
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}
	stop := startCancelableLoopT(t, loop)
	observed := make(chan bool, 1)
	if err := loop.SubmitInternal(func() { observed <- loop.Alive() }); err != nil {
		t.Fatalf("FD-only liveness observation: %v", err)
	}
	if !waitContractValue(t, observed, "FD-only liveness") {
		t.Fatal("Alive returned false with one registered user FD")
	}
	stop()
}
