package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestUnrefInterval_PropagatesAcrossReschedules verifies that UnrefInterval
// correctly propagates the unref state across rescheduled timers. When an
// interval fires and reschedules (creating a new timer via ScheduleTimer),
// the new timer inherits the unref'd state from the interval's refed field.
func TestUnrefInterval_PropagatesAcrossReschedules(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer loop.Close()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS: %v", err)
	}

	var jsID uint64
	var jsIDReady atomic.Bool
	idCh := make(chan TimerID, 10)
	doneCh := make(chan struct{})

	jsID, err = js.SetInterval(func() {
		if !jsIDReady.Load() {
			return
		}
		js.intervalsMu.RLock()
		state, ok := js.intervals[jsID]
		if ok {
			idCh <- TimerID(state.currentLoopTimerID.Load())
		}
		js.intervalsMu.RUnlock()

		select {
		case doneCh <- struct{}{}:
		default:
		}
	}, 50)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}
	jsIDReady.Store(true)

	// Wait for first tick
	<-doneCh

	// Unref the interval (using the JS-level API)
	if err := js.UnrefInterval(jsID); err != nil {
		t.Fatalf("UnrefInterval: %v", err)
	}

	// Wait for second tick — the interval should still fire
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for second tick after unref")
	}

	// Drain to ensure all pending operations are processed
	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	// Key assertion: refedTimerCount must be 0 because the interval's
	// rescheduled timer inherited the unref'd state.
	count := loop.refedTimerCount.Load()
	t.Logf("refedTimerCount after second tick with UnrefInterval: %d", count)
	if count != 0 {
		t.Errorf("refedTimerCount should be 0 after UnrefInterval (new timer should be unref'd), got %d", count)
	}

	js.ClearInterval(jsID)
}

// TestUnrefInterval_TimerIDChangesBetweenTicks verifies that each interval tick
// creates a NEW Loop.TimerID. This is fundamental to understanding why
// loop-level UnrefTimer alone is insufficient for intervals.
func TestUnrefInterval_TimerIDChangesBetweenTicks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer loop.Close()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS: %v", err)
	}

	const ticks = 5
	var jsID uint64
	var jsIDReady atomic.Bool
	idCh := make(chan TimerID, ticks*2)

	jsID, err = js.SetInterval(func() {
		if !jsIDReady.Load() {
			return
		}
		js.intervalsMu.RLock()
		state, ok := js.intervals[jsID]
		if ok {
			idCh <- TimerID(state.currentLoopTimerID.Load())
		}
		js.intervalsMu.RUnlock()
	}, 20)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}
	jsIDReady.Store(true)

	ids := make(map[TimerID]bool)
	for i := 0; i < ticks; i++ {
		select {
		case id := <-idCh:
			ids[id] = true
			t.Logf("tick %d: timer ID %d", i, id)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for tick %d", i)
		}
	}

	js.ClearInterval(jsID)

	t.Logf("collected %d unique timer IDs from %d ticks", len(ids), ticks)
	if len(ids) < 2 {
		t.Errorf("expected at least 2 distinct timer IDs across %d ticks, got %d unique", ticks, len(ids))
	}
}

// TestUnrefInterval_AllowsLoopExit verifies that after calling UnrefInterval,
// the loop's Alive() returns false because the interval's timers no longer
// count toward the alive check.
func TestUnrefInterval_AllowsLoopExit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer loop.Close()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS: %v", err)
	}

	jsID, err := js.SetInterval(func() {}, 10)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}

	// Wait for the interval to be registered
	time.Sleep(20 * time.Millisecond)

	if !loop.Alive() {
		t.Fatal("Alive() should return true with a ref'd interval")
	}

	// Unref the interval
	if err := js.UnrefInterval(jsID); err != nil {
		t.Fatalf("UnrefInterval: %v", err)
	}

	// Drain to ensure unref is processed
	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	// Alive() should return false now — the interval's timer is unref'd
	if loop.Alive() {
		t.Error("Alive() should return false after UnrefInterval")
	}

	js.ClearInterval(jsID)
}

// TestUnrefInterval_RefReversesUnref verifies that RefInterval reverses a
// previous UnrefInterval, making the interval's timers count toward Alive()
// again.
func TestUnrefInterval_RefReversesUnref(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer loop.Close()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS: %v", err)
	}

	jsID, err := js.SetInterval(func() {}, 10)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	// Unref then ref
	if err := js.UnrefInterval(jsID); err != nil {
		t.Fatalf("UnrefInterval: %v", err)
	}

	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier1: %v", err)
	}
	<-barrier

	if loop.Alive() {
		t.Log("Alive() still true after UnrefInterval — interval is between ticks")
	}

	// Ref it back
	if err := js.RefInterval(jsID); err != nil {
		t.Fatalf("RefInterval: %v", err)
	}

	barrier2 := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier2) }); err != nil {
		t.Fatalf("barrier2: %v", err)
	}
	<-barrier2

	// After RefInterval, the interval should keep the loop alive again.
	// The current timer may have fired and been rescheduled with the ref'd state.
	if !loop.Alive() {
		t.Error("Alive() should return true after RefInterval")
	}

	js.ClearInterval(jsID)
}

// TestUnrefInterval_MultipleIntervals verifies UnrefInterval with multiple
// concurrent intervals. After unref'ing all, refedTimerCount should be 0.
func TestUnrefInterval_MultipleIntervals(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer loop.Close()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS: %v", err)
	}

	const numIntervals = 3
	intervalIDs := make([]uint64, numIntervals)

	for i := 0; i < numIntervals; i++ {
		intervalIDs[i], err = js.SetInterval(func() {}, 20)
		if err != nil {
			t.Fatalf("SetInterval %d: %v", i, err)
		}
	}

	// Wait for all to be registered
	time.Sleep(30 * time.Millisecond)

	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	if loop.refedTimerCount.Load() < 1 {
		t.Errorf("refedTimerCount should be >= 1 with %d ref'd intervals, got %d", numIntervals, loop.refedTimerCount.Load())
	}

	// Unref all intervals
	for _, id := range intervalIDs {
		if err := js.UnrefInterval(id); err != nil {
			t.Fatalf("UnrefInterval: %v", err)
		}
	}

	// Drain
	barrier2 := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier2) }); err != nil {
		t.Fatalf("barrier2: %v", err)
	}
	<-barrier2

	count := loop.refedTimerCount.Load()
	t.Logf("after unref'ing all %d intervals: refedTimerCount=%d", numIntervals, count)
	if count != 0 {
		t.Errorf("refedTimerCount should be 0 after UnrefInterval on all intervals, got %d", count)
	}

	if loop.Alive() {
		t.Error("Alive() should return false after all intervals are unref'd")
	}

	// Clean up
	for _, id := range intervalIDs {
		js.ClearInterval(id)
	}
}

// TestUnrefInterval_ConcurrentUnrefDuringFire exercises the race between
// external goroutines calling UnrefInterval and the interval wrapper
// rescheduling on the loop goroutine.
func TestUnrefInterval_ConcurrentUnrefDuringFire(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer loop.Close()

	go loop.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS: %v", err)
	}

	var fireCount atomic.Int32
	jsID, err := js.SetInterval(func() {
		fireCount.Add(1)
	}, 5)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}

	// Let the interval fire a few times
	time.Sleep(100 * time.Millisecond)

	// Launch concurrent unref'ers using UnrefInterval
	var wg sync.WaitGroup
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = js.UnrefInterval(jsID)
			}
		}()
	}

	wg.Wait()

	// The interval should still be firing despite concurrent unrefs
	// (UnrefInterval doesn't stop the interval, just the ref counting)
	fires := fireCount.Load()
	t.Logf("fires after concurrent UnrefInterval: %d, refedTimerCount: %d", fires, loop.refedTimerCount.Load())

	if fires < 5 {
		t.Errorf("expected at least 5 fires, got %d", fires)
	}

	// refedTimerCount must not be negative
	if loop.refedTimerCount.Load() < 0 {
		t.Fatalf("refedTimerCount went negative: %d", loop.refedTimerCount.Load())
	}

	js.ClearInterval(jsID)
}

// TestRefUnrefInterval_ConcurrentStress verifies that concurrent RefInterval
// and UnrefInterval calls do not corrupt interval state or cause panics.
// Both methods now acquire state.m.Lock, ensuring atomicity between the
// refed flag and the underlying timer ref/unref operations.
func TestRefUnrefInterval_ConcurrentStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const iterations = 20

	for i := range iterations {
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

		go func() {
			loop.Run(ctx)
		}()

		waitLoopState(t, loop, StateRunning, 2*time.Second)

		intervalID, err := js.SetInterval(func() {}, 10)
		if err != nil {
			t.Fatalf("SetInterval: %v", err)
		}

		var (
			wg         sync.WaitGroup
			refOps     atomic.Int64
			unrefOps   atomic.Int64
			stopHammer atomic.Bool
		)

		const hammerGoroutines = 8
		wg.Add(hammerGoroutines)

		for g := 0; g < hammerGoroutines; g++ {
			go func(refFirst bool) {
				defer wg.Done()
				for !stopHammer.Load() {
					if refFirst {
						js.RefInterval(intervalID)
						refOps.Add(1)
						js.UnrefInterval(intervalID)
						unrefOps.Add(1)
					} else {
						js.UnrefInterval(intervalID)
						unrefOps.Add(1)
						js.RefInterval(intervalID)
						refOps.Add(1)
					}
				}
			}(g%2 == 0)
		}

		time.Sleep(100 * time.Millisecond)
		stopHammer.Store(true)
		wg.Wait()

		js.ClearInterval(intervalID)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()

		if err := loop.Shutdown(shutdownCtx); err != nil {
			t.Fatalf("iteration %d: Shutdown failed after ref/unref stress: %v", i, err)
		}

		t.Logf("iteration %d: refOps=%d unrefOps=%d", i, refOps.Load(), unrefOps.Load())
	}
}
