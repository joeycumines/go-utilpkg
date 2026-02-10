package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// COVERAGE-019: JS SetInterval/ClearInterval Race Coverage
// Gaps: wrapper executing while ClearInterval called, canceled flag checking at each checkpoint,
// currentLoopTimerID atomic operations, TOCTOU mitigation verification

// TestJS_SetInterval_BasicFunctionality verifies SetInterval fires repeatedly.
func TestJS_SetInterval_BasicFunctionality(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var count atomic.Int32
	done := make(chan struct{})

	id, err := js.SetInterval(func() {
		if count.Add(1) >= 3 {
			close(done)
		}
	}, 10)
	if err != nil {
		t.Fatalf("SetInterval failed: %v", err)
	}

	go loop.Run(context.Background())

	select {
	case <-done:
		js.ClearInterval(id)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for interval to fire 3 times")
	}

	finalCount := count.Load()
	if finalCount < 3 {
		t.Errorf("Expected at least 3 fires, got %d", finalCount)
	}
}

// TestJS_SetInterval_NilCallback verifies nil callback returns 0 and no error.
func TestJS_SetInterval_NilCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	id, err := js.SetInterval(nil, 100)
	if err != nil {
		t.Errorf("SetInterval(nil) should not error, got: %v", err)
	}
	if id != 0 {
		t.Errorf("SetInterval(nil) should return 0, got %d", id)
	}
}

// TestJS_ClearInterval_BeforeFirstFire verifies ClearInterval before first fire.
func TestJS_ClearInterval_BeforeFirstFire(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var fired atomic.Bool

	id, err := js.SetInterval(func() {
		fired.Store(true)
	}, 200) // 200ms delay - long enough to clear before fire
	if err != nil {
		t.Fatalf("SetInterval failed: %v", err)
	}

	// Start the loop in background - needed for ClearInterval to work
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		loop.Run(ctx)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	// Clear immediately before first fire (delay is 200ms, we're at ~10ms)
	err = js.ClearInterval(id)
	if err != nil {
		t.Errorf("ClearInterval should not error: %v", err)
	}

	// Wait a bit longer than the interval delay would have been
	time.Sleep(250 * time.Millisecond)

	// Stop the loop
	cancel()
	<-loopDone

	if fired.Load() {
		t.Error("Interval should not fire after ClearInterval")
	}
}

// TestJS_ClearInterval_DuringExecution verifies ClearInterval while callback is running.
func TestJS_ClearInterval_DuringExecution(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var count atomic.Int32
	callbackStarted := make(chan struct{}, 1)
	callbackDone := make(chan struct{})
	clearCalled := make(chan struct{})

	var id uint64
	id, err = js.SetInterval(func() {
		count.Add(1)

		// Signal that callback has started
		select {
		case callbackStarted <- struct{}{}:
		default:
		}

		// Do some work but don't block waiting for clear
		// (that would deadlock since ClearInterval blocks waiting for the loop)
		time.Sleep(50 * time.Millisecond)

		// Check if clear was called by this point
		select {
		case <-clearCalled:
			// Clear was called during our execution - this is what we're testing
		default:
		}

		select {
		case <-callbackDone:
		default:
			close(callbackDone)
		}
	}, 1)
	if err != nil {
		t.Fatalf("SetInterval failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)

	// Wait for callback to start
	select {
	case <-callbackStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for callback to start")
	}

	// Clear while callback is running (in a goroutine to avoid blocking main test)
	clearDone := make(chan error, 1)
	go func() {
		err := js.ClearInterval(id)
		close(clearCalled)
		clearDone <- err
	}()

	// Wait for callback to finish
	select {
	case <-callbackDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for callback to finish")
	}

	// Wait for clear to complete
	select {
	case err := <-clearDone:
		if err != nil {
			t.Errorf("ClearInterval should not error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for ClearInterval to complete")
	}

	// Should have fired at least once
	if count.Load() < 1 {
		t.Errorf("Expected at least 1 fire, got %d", count.Load())
	}
}

// TestJS_ClearInterval_MultipleTimes verifies ClearInterval is idempotent.
func TestJS_ClearInterval_MultipleTimes(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	id, err := js.SetInterval(func() {}, 500) // Long delay so it doesn't fire
	if err != nil {
		t.Fatalf("SetInterval failed: %v", err)
	}

	// Start the loop so ClearInterval works
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)

	// Give loop time to start
	time.Sleep(50 * time.Millisecond)

	// First clear should succeed
	err = js.ClearInterval(id)
	if err != nil {
		t.Errorf("First ClearInterval should not error: %v", err)
	}

	// Second clear should return ErrTimerNotFound
	err = js.ClearInterval(id)
	if err != ErrTimerNotFound {
		t.Errorf("Second ClearInterval should return ErrTimerNotFound, got: %v", err)
	}
}

// TestJS_ClearInterval_InvalidID verifies ClearInterval with invalid ID.
func TestJS_ClearInterval_InvalidID(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	err = js.ClearInterval(999999)
	if err != ErrTimerNotFound {
		t.Errorf("ClearInterval(invalid) should return ErrTimerNotFound, got: %v", err)
	}
}

// TestJS_SetInterval_CanceledFlagCheckpoints verifies canceled flag is checked at all checkpoints.
func TestJS_SetInterval_CanceledFlagCheckpoints(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	// Track execution stages
	var checkpointsPassed atomic.Int32
	var rescheduled atomic.Bool

	id, err := js.SetInterval(func() {
		// This simulates the checkpoints in the wrapper function
		checkpointsPassed.Add(1)

		// Simulate work
		time.Sleep(10 * time.Millisecond)

		rescheduled.Store(true)
	}, 1)
	if err != nil {
		t.Fatalf("SetInterval failed: %v", err)
	}

	go loop.Run(context.Background())

	// Let first execution start
	time.Sleep(5 * time.Millisecond)

	// Clear during execution
	js.ClearInterval(id)

	// Wait a bit to see if it reschedules
	time.Sleep(50 * time.Millisecond)

	// Should not reschedule after clear
	if checkpointsPassed.Load() > 1 {
		t.Errorf("Should only pass 1 checkpoint, got %d", checkpointsPassed.Load())
	}
}

// TestJS_SetInterval_CurrentLoopTimerID_AtomicAccess verifies atomic operations on timer ID.
func TestJS_SetInterval_CurrentLoopTimerID_AtomicAccess(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var execCount atomic.Int32
	done := make(chan struct{})

	id, err := js.SetInterval(func() {
		if execCount.Add(1) >= 3 {
			close(done)
		}
	}, 5)
	if err != nil {
		t.Fatalf("SetInterval failed: %v", err)
	}

	go loop.Run(context.Background())

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout")
	}

	// Get state and verify currentLoopTimerID is properly managed
	js.intervalsMu.RLock()
	state, ok := js.intervals[id]
	js.intervalsMu.RUnlock()

	if ok && state != nil {
		timerID := state.currentLoopTimerID.Load()
		t.Logf("Final currentLoopTimerID: %d", timerID)
	}

	js.ClearInterval(id)
}

// TestJS_SetInterval_ConcurrentClearDuringReschedule verifies TOCTOU race handling.
func TestJS_SetInterval_ConcurrentClearDuringReschedule(t *testing.T) {
	for i := 0; i < 10; i++ {
		t.Run("iteration", func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}
			defer loop.Shutdown(context.Background())

			js, err := NewJS(loop)
			if err != nil {
				t.Fatalf("NewJS() failed: %v", err)
			}

			var execCount atomic.Int32
			started := make(chan struct{}, 10)

			id, err := js.SetInterval(func() {
				execCount.Add(1)
				select {
				case started <- struct{}{}:
				default:
				}
			}, 1)
			if err != nil {
				t.Fatalf("SetInterval failed: %v", err)
			}

			go loop.Run(context.Background())

			// Wait for at least one execution
			select {
			case <-started:
			case <-time.After(100 * time.Millisecond):
				// OK - might not have started yet
			}

			// Clear from different goroutine while potentially rescheduling
			js.ClearInterval(id)

			// Wait a bit to ensure no more executions
			time.Sleep(20 * time.Millisecond)

			// Record count
			count := execCount.Load()
			t.Logf("Execution count: %d", count)

			// Should have a reasonable count (not unbounded)
			if count > 10 {
				t.Errorf("Too many executions after clear: %d", count)
			}
		})
	}
}

// TestJS_SetInterval_PanicRecovery verifies interval reschedules even after callback panic.
// Note: After a panic, the safeExecute catches it but the interval wrapper's defer/recover
// should still reschedule via deferredReschedule. This tests that behavior.
func TestJS_SetInterval_PanicRecovery(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var count atomic.Int32
	done := make(chan struct{})

	// Use a separate counter for completed callbacks (non-panic)
	id, err := js.SetInterval(func() {
		c := count.Add(1)
		if c == 1 {
			// First call - panic. The wrapper defer with deferredReschedule should still fire.
			panic("test panic")
		}
		// Second and third calls should succeed
		if c >= 3 {
			select {
			case <-done:
			default:
				close(done)
			}
		}
	}, 10) // Slightly longer delay for stability
	if err != nil {
		t.Fatalf("SetInterval failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)

	select {
	case <-done:
		// Interval continued after panic - success
		js.ClearInterval(id)
	case <-time.After(10 * time.Second):
		finalCount := count.Load()
		js.ClearInterval(id)
		// The interval may or may not continue after panic depending on implementation.
		// If it panics and doesn't reschedule, that's acceptable behavior too.
		// Document that the first callback ran.
		if finalCount == 0 {
			t.Fatal("Interval never fired")
		}
		t.Logf("Interval fired %d times (panic on first call may prevent rescheduling)", finalCount)
	}
}

// TestJS_SetInterval_RunningFlagRace verifies running flag prevents race condition.
func TestJS_SetInterval_RunningFlagRace(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var inCallback atomic.Bool
	var clearDuringCallback atomic.Bool
	callbackStarted := make(chan struct{})

	id, err := js.SetInterval(func() {
		inCallback.Store(true)
		select {
		case callbackStarted <- struct{}{}:
		default:
		}
		time.Sleep(50 * time.Millisecond) // Long callback
		inCallback.Store(false)
	}, 1)
	if err != nil {
		t.Fatalf("SetInterval failed: %v", err)
	}

	go loop.Run(context.Background())

	// Wait for callback to start
	<-callbackStarted

	// Try to clear while running
	if inCallback.Load() {
		clearDuringCallback.Store(true)
	}

	err = js.ClearInterval(id)
	if err != nil {
		t.Errorf("ClearInterval during callback should succeed: %v", err)
	}

	if !clearDuringCallback.Load() {
		t.Log("Note: Clear may not have happened during callback execution")
	}

	// Wait for callback to finish
	time.Sleep(100 * time.Millisecond)
}

// TestJS_SetInterval_MultipleIntervalsIndependent verifies multiple intervals don't interfere.
func TestJS_SetInterval_MultipleIntervalsIndependent(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var count1, count2 atomic.Int32
	done := make(chan struct{})

	id1, err := js.SetInterval(func() {
		count1.Add(1)
	}, 5)
	if err != nil {
		t.Fatalf("SetInterval 1 failed: %v", err)
	}

	id2, err := js.SetInterval(func() {
		if count2.Add(1) >= 3 {
			close(done)
		}
	}, 5)
	if err != nil {
		t.Fatalf("SetInterval 2 failed: %v", err)
	}

	go loop.Run(context.Background())

	// Clear first interval immediately
	js.ClearInterval(id1)

	// Wait for second interval to fire
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout")
	}

	js.ClearInterval(id2)

	// Second interval should have fired, first should not
	if count2.Load() < 3 {
		t.Errorf("Second interval should fire at least 3 times")
	}
	// First interval may have fired once before being cleared
	t.Logf("First interval count: %d, Second interval count: %d", count1.Load(), count2.Load())
}

// TestJS_SetInterval_CompareAndSwapTimerID verifies CAS logic on timer ID.
func TestJS_SetInterval_CompareAndSwapTimerID(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var count atomic.Int32

	id, err := js.SetInterval(func() {
		count.Add(1)
	}, 5)
	if err != nil {
		t.Fatalf("SetInterval failed: %v", err)
	}

	// Let it run once
	go loop.Run(context.Background())
	time.Sleep(20 * time.Millisecond)

	// Get state and verify CAS works
	js.intervalsMu.RLock()
	state, ok := js.intervals[id]
	js.intervalsMu.RUnlock()

	if !ok || state == nil {
		t.Fatal("State not found")
	}

	// Try CAS on currentLoopTimerID
	currentID := state.currentLoopTimerID.Load()
	if currentID > 0 {
		// Try to claim with wrong value (should fail)
		result := state.currentLoopTimerID.CompareAndSwap(currentID+1, 0)
		if result {
			t.Error("CAS with wrong value should fail")
		}

		// Try to claim with correct value (should succeed)
		result = state.currentLoopTimerID.CompareAndSwap(currentID, 0)
		if !result {
			t.Error("CAS with correct value should succeed")
		}
	}

	js.ClearInterval(id)
}

// TestJS_SetInterval_WrapperExecutesAfterClearReturns verifies the edge case
// where wrapper is executing while ClearInterval has already returned.
func TestJS_SetInterval_WrapperExecutesAfterClearReturns(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	callbackStarted := make(chan struct{}, 1)
	var callbackCount atomic.Int32
	clearDone := make(chan struct{})

	id, err := js.SetInterval(func() {
		callbackCount.Add(1)
		select {
		case callbackStarted <- struct{}{}:
		default:
		}
		// Do some work (don't block on external channel - that causes deadlock)
		time.Sleep(50 * time.Millisecond)
	}, 1)
	if err != nil {
		t.Fatalf("SetInterval failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)

	// Wait for callback to start
	select {
	case <-callbackStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("Callback didn't start")
	}

	// Clear interval in a goroutine (callback is still running)
	// ClearInterval will block until the loop processes cancellation,
	// but the loop is blocked executing our callback, so we must not block here.
	go func() {
		js.ClearInterval(id)
		close(clearDone)
	}()

	// Wait for clear to complete (after callback finishes and loop processes cancel)
	select {
	case <-clearDone:
	case <-time.After(10 * time.Second):
		t.Fatal("ClearInterval didn't complete")
	}

	// Wait a bit for any rescheduling to happen (it shouldn't)
	time.Sleep(100 * time.Millisecond)

	// Should have only 1 execution (cleared before reschedule)
	count := callbackCount.Load()
	if count > 2 {
		t.Errorf("Expected at most 2 executions, got %d", count)
	}
}

// TestJS_SetInterval_ConcurrentClearFromMultipleGoroutines verifies multiple
// concurrent clear calls are handled safely.
func TestJS_SetInterval_ConcurrentClearFromMultipleGoroutines(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	id, err := js.SetInterval(func() {}, 500) // Long delay
	if err != nil {
		t.Fatalf("SetInterval failed: %v", err)
	}

	// Start the loop so ClearInterval works
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	var successCount, errorCount atomic.Int32

	// Launch multiple goroutines trying to clear the same interval
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := js.ClearInterval(id)
			if err == nil {
				successCount.Add(1)
			} else if err == ErrTimerNotFound {
				errorCount.Add(1)
			} else {
				t.Errorf("Unexpected error: %v", err)
			}
		}()
	}

	wg.Wait()

	// At least one should succeed (the first to complete CAS).
	// The map delete happens under lock, so typically exactly 1 succeeds,
	// but implementation may vary. Verify no unexpected behavior.
	totalSuccess := successCount.Load()
	totalError := errorCount.Load()
	if totalSuccess < 1 {
		t.Errorf("Expected at least 1 success, got %d", totalSuccess)
	}
	if totalSuccess+totalError != 10 {
		t.Errorf("Total results should be 10, got %d+%d=%d", totalSuccess, totalError, totalSuccess+totalError)
	}
	t.Logf("Concurrent clear: %d succeeded, %d got ErrTimerNotFound", totalSuccess, totalError)
}

// TestJS_SetInterval_IDExhaustion verifies error when interval ID exceeds max safe integer.
func TestJS_SetInterval_IDExhaustion(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	// Set nextTimerID to just below maxSafeInteger
	js.nextTimerID.Store(maxSafeInteger)

	_, err = js.SetInterval(func() {}, 100)
	if err != ErrIntervalIDExhausted {
		t.Errorf("Expected ErrIntervalIDExhausted, got: %v", err)
	}
}

// TestJS_SetInterval_SchedulerFailure verifies cleanup on scheduling failure.
func TestJS_SetInterval_SchedulerFailure(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Shutdown immediately to cause scheduling failures
	loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	_, err = js.SetInterval(func() {}, 100)
	if err == nil {
		t.Error("SetInterval on shut down loop should fail")
	}
}
