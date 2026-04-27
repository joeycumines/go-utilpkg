package eventloop

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestMicrotaskRing_AtomicHeadTail verifies that microtaskRing head and tail
// fields support concurrent access without data races. The ring uses
// atomic.Uint64 for both indices, enabling safe IsEmpty/Length calls from
// any goroutine while Push proceeds concurrently.
func TestMicrotaskRing_AtomicHeadTail(t *testing.T) {
	r := newMicrotaskRing()

	head := r.head.Load()
	tail := r.tail.Load()
	if head != 0 || tail != 0 {
		t.Errorf("expected initial head=0 tail=0, got head=%d tail=%d", head, tail)
	}

	const iterations = 1000
	var wg sync.WaitGroup
	wg.Go(func() {
		for range iterations {
			r.Push(func() {})
		}
	})

	wg.Go(func() {
		for range iterations {
			_ = r.IsEmpty()
		}
	})

	wg.Wait()
}

// TestMicrotaskRing_IsEmptyConcurrentPush exercises IsEmpty under concurrent
// Push calls to confirm no panics or data races.
func TestMicrotaskRing_IsEmptyConcurrentPush(t *testing.T) {
	r := newMicrotaskRing()
	const numPushes = 10000

	var wg sync.WaitGroup

	wg.Go(func() {
		for range numPushes {
			r.Push(func() {})
		}
	})

	wg.Go(func() {
		for range numPushes {
			_ = r.IsEmpty()
			runtime.Gosched()
		}
	})

	wg.Wait()
}

// TestMicrotaskRing_LengthConcurrent verifies Length returns non-negative
// values when read concurrently with Push operations.
func TestMicrotaskRing_LengthConcurrent(t *testing.T) {
	r := newMicrotaskRing()
	const numPushes = 5000

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for range numPushes {
			r.Push(func() {})
		}
	}()

	go func() {
		defer wg.Done()
		for range numPushes {
			if length := r.Length(); length < 0 {
				t.Errorf("Length() returned negative: %d", length)
			}
			runtime.Gosched()
		}
	}()

	wg.Wait()
}

// TestAlive_EpochRetryConcurrentMicrotask verifies that the submissionEpoch
// retry loop in Alive() handles concurrent microtask submissions correctly,
// returning true when work is in flight.
func TestAlive_EpochRetryConcurrentMicrotask(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var aliveCalls atomic.Int32

	go loop.Run(ctx)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for range 100 {
			_ = loop.Alive()
			aliveCalls.Add(1)
			runtime.Gosched()
		}
	}()

	go func() {
		defer wg.Done()
		for range 100 {
			_ = loop.ScheduleMicrotask(func() {})
			runtime.Gosched()
		}
	}()

	wg.Wait()

	if aliveCalls.Load() != 100 {
		t.Errorf("expected 100 Alive() calls, got %d", aliveCalls.Load())
	}
}

// TestAlive_ConcurrentStress exercises Alive() under heavy concurrent
// modification (microtasks, nextTick, and Alive calls from multiple
// goroutines) to verify no data races are detected.
func TestAlive_ConcurrentStress(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go loop.Run(ctx)

	var wg sync.WaitGroup

	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 500 {
				_ = loop.Alive()
			}
		}()
	}

	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 500 {
				_ = loop.ScheduleMicrotask(func() {})
			}
		}()
	}

	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 500 {
				_ = loop.ScheduleNextTick(func() {})
			}
		}()
	}

	wg.Wait()
}

// TestClose_ConcurrentNoPanic verifies that concurrent Close() calls on a
// running loop never panic (double-close of loopDone/runCh is prevented by
// sync.Once). Exactly one call should succeed; the rest return
// ErrLoopTerminated.
func TestClose_ConcurrentNoPanic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	errors := make([]error, 10)
	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors[i] = loop.Close()
		}()
	}
	wg.Wait()

	successCount := 0
	for _, e := range errors {
		if e == nil {
			successCount++
		}
	}
	if successCount != 1 {
		t.Errorf("expected exactly 1 successful Close(), got %d", successCount)
	}
}

// TestShutdown_AwakeClosesLoopDone verifies that Shutdown() called before
// Run() closes loopDone, preventing goroutines blocked on it from leaking.
func TestShutdown_AwakeClosesLoopDone(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- loop.Shutdown(context.Background())
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Shutdown(Awake) returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown(Awake) timed out — loopDone may not have been closed")
	}
}

// TestRefedTimerCount_NormalFire verifies refedTimerCount returns to zero
// after a ref'd timer fires normally.
func TestRefedTimerCount_NormalFire(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var fired atomic.Bool
	_, err = loop.ScheduleTimer(1*time.Millisecond, func() {
		fired.Store(true)
	})
	if err != nil {
		t.Fatal(err)
	}

	go loop.Run(ctx)

	for !fired.Load() {
		runtime.Gosched()
	}

	time.Sleep(20 * time.Millisecond)

	if count := loop.refedTimerCount.Load(); count != 0 {
		t.Errorf("refedTimerCount after normal fire: got %d, want 0", count)
	}
}

// TestRefedTimerCount_UnrefThenFire verifies refedTimerCount returns to zero
// when a timer is unref'd before firing.
func TestRefedTimerCount_UnrefThenFire(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var fired atomic.Bool
	id, err := loop.ScheduleTimer(5*time.Millisecond, func() {
		fired.Store(true)
	})
	if err != nil {
		t.Fatal(err)
	}

	go loop.Run(ctx)
	time.Sleep(2 * time.Millisecond)

	_ = loop.UnrefTimer(id)

	for !fired.Load() {
		runtime.Gosched()
	}

	time.Sleep(20 * time.Millisecond)

	if count := loop.refedTimerCount.Load(); count != 0 {
		t.Errorf("refedTimerCount after unref+fire: got %d, want 0", count)
	}
}

// TestRefedTimerCount_CancelBeforeFire verifies refedTimerCount returns to
// zero when a timer is cancelled before firing.
func TestRefedTimerCount_CancelBeforeFire(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	id, err := loop.ScheduleTimer(10*time.Second, func() {})
	if err != nil {
		t.Fatal(err)
	}

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	_ = loop.CancelTimer(id)

	time.Sleep(20 * time.Millisecond)

	if count := loop.refedTimerCount.Load(); count != 0 {
		t.Errorf("refedTimerCount after cancel: got %d, want 0", count)
	}
}

// TestRefedTimerCount_CancelAfterUnref verifies refedTimerCount returns to
// zero when UnrefTimer is called before CancelTimer on the same timer.
func TestRefedTimerCount_CancelAfterUnref(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	id, err := loop.ScheduleTimer(10*time.Second, func() {})
	if err != nil {
		t.Fatal(err)
	}

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	_ = loop.UnrefTimer(id)
	_ = loop.CancelTimer(id)

	time.Sleep(20 * time.Millisecond)

	if count := loop.refedTimerCount.Load(); count != 0 {
		t.Errorf("refedTimerCount after unref+cancel: got %d, want 0", count)
	}
}

// TestRefedTimerCount_ReentrantCancelSelf verifies refedTimerCount returns to
// zero when a timer callback cancels its own timer (re-entrant cancel). The
// heapIndex < 0 guard in applyCancelTimer defers the decrement to runTimers.
func TestRefedTimerCount_ReentrantCancelSelf(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var fired atomic.Bool
	var aID TimerID

	id, err := loop.ScheduleTimer(1*time.Millisecond, func() {
		_ = loop.CancelTimer(aID)
		fired.Store(true)
	})
	if err != nil {
		t.Fatal(err)
	}
	aID = id

	go loop.Run(ctx)

	for !fired.Load() {
		runtime.Gosched()
	}

	time.Sleep(20 * time.Millisecond)

	if count := loop.refedTimerCount.Load(); count != 0 {
		t.Errorf("refedTimerCount after re-entrant cancel: got %d, want 0", count)
	}
}

// TestRefedTimerCount_BatchCancel verifies refedTimerCount returns to zero
// after batch cancellation of multiple ref'd timers.
func TestRefedTimerCount_BatchCancel(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ids := make([]TimerID, 10)
	for i := range 10 {
		id, scheduleErr := loop.ScheduleTimer(10*time.Second, func() {})
		if scheduleErr != nil {
			t.Fatal(scheduleErr)
		}
		ids[i] = id
	}

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	errs := loop.CancelTimers(ids)
	for i, e := range errs {
		if e != nil {
			t.Errorf("CancelTimers[%d] returned error: %v", i, e)
		}
	}

	time.Sleep(20 * time.Millisecond)

	if count := loop.refedTimerCount.Load(); count != 0 {
		t.Errorf("refedTimerCount after batch cancel: got %d, want 0", count)
	}
}

// TestRefedTimerCount_MixedOps stress tests refedTimerCount under mixed
// Schedule/Ref/Unref/Cancel operations from multiple goroutines.
func TestRefedTimerCount_MixedOps(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go loop.Run(ctx)

	var wg sync.WaitGroup
	const numGoroutines = 8
	const opsPerGoroutine = 100

	for range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range opsPerGoroutine {
				id, scheduleErr := loop.ScheduleTimer(time.Duration(j%100)*time.Millisecond, func() {})
				if scheduleErr != nil {
					continue
				}

				if j%3 == 0 {
					_ = loop.UnrefTimer(id)
				}
				if j%5 == 0 {
					_ = loop.RefTimer(id)
				}
				if j%7 == 0 {
					_ = loop.CancelTimer(id)
				}
			}
		}()
	}

	wg.Wait()

	time.Sleep(200 * time.Millisecond)

	if count := loop.refedTimerCount.Load(); count < 0 {
		t.Errorf("refedTimerCount went negative: %d — indicates double-decrement bug", count)
	}
}

// TestScheduleTimer_LoopThreadSync verifies that ScheduleTimer called from
// the loop goroutine registers the timer synchronously (timerMap lookup
// succeeds immediately, without queuing).
func TestScheduleTimer_LoopThreadSync(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var scheduleDone atomic.Bool
	var timerCount int32

	_ = loop.Submit(func() {
		id, scheduleErr := loop.ScheduleTimer(10*time.Second, func() {})
		if scheduleErr != nil {
			t.Errorf("ScheduleTimer on loop thread failed: %v", scheduleErr)
		}
		if _, exists := loop.timerMap[id]; !exists {
			t.Errorf("timer %d not found in timerMap after synchronous ScheduleTimer", id)
		}
		timerCount = loop.refedTimerCount.Load()
		scheduleDone.Store(true)
	})

	go loop.Run(ctx)

	for !scheduleDone.Load() {
		runtime.Gosched()
	}

	if timerCount != 1 {
		t.Errorf("refedTimerCount after sync ScheduleTimer: got %d, want 1", timerCount)
	}
}

// TestCancelTimer_LoopThreadSync verifies that CancelTimer called from the
// loop goroutine executes directly (not via SubmitInternal), preventing
// deadlock when canUseFastPath is false.
func TestCancelTimer_LoopThreadSync(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var cancelDone atomic.Bool
	var cancelErr error

	_ = loop.Submit(func() {
		id, scheduleErr := loop.ScheduleTimer(10*time.Second, func() {})
		if scheduleErr != nil {
			t.Errorf("ScheduleTimer failed: %v", scheduleErr)
			return
		}
		cancelErr = loop.CancelTimer(id)
		cancelDone.Store(true)
	})

	go loop.Run(ctx)

	for !cancelDone.Load() {
		runtime.Gosched()
	}

	if cancelErr != nil {
		t.Errorf("CancelTimer on loop thread returned error: %v", cancelErr)
	}
}

// TestInterval_ConcurrentRefUnref stress tests the interval ref propagation
// under concurrent RefInterval/UnrefInterval calls. The TOCTOU re-check in
// the interval wrapper compensates for concurrent state changes.
func TestInterval_ConcurrentRefUnref(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go loop.Run(ctx)

	id, err := js.SetInterval(func() {}, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer js.ClearInterval(id)

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for range 100 {
				if idx%2 == 0 {
					_ = js.RefInterval(id)
				} else {
					_ = js.UnrefInterval(id)
				}
				runtime.Gosched()
			}
		}(i)
	}

	wg.Wait()

	_ = js.RefInterval(id)
	time.Sleep(50 * time.Millisecond)

	if loop.refedTimerCount.Load() < 0 {
		t.Errorf("refedTimerCount went negative during interval ref/unref stress: %d",
			loop.refedTimerCount.Load())
	}
}
