// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestTimerNestingDepthPanicRestore verifies that nesting depth is properly
// restored even when a timer callback panics. This tests the fix for bug
// where defer needed to be added before safeExecute().
func TestTimerNestingDepthPanicRestore(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Schedule 10 nested timers, the 5th one (index 4) will panic
	panicTriggered := atomic.Bool{}
	var depthBeforePanic atomic.Int32
	var depthAfterPanic atomic.Int32

	// Use a channel to synchronize the panic test
	panicChan := make(chan struct{})

	var nestedSchedule func(depth int)
	nestedSchedule = func(depth int) {
		if depth == 5 {
			// Record depth before panic
			depthBeforePanic.Store(loop.timerNestingDepth.Load())
			panicTriggered.Store(true)
			close(panicChan)
			panic("intentional test panic")
		}

		if depth < 10 {
			loop.ScheduleTimer(1*time.Microsecond, func() {
				nestedSchedule(depth + 1)
			})
		}
	}

	// Start the loop in goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Start nested recursion
	loop.ScheduleTimer(1*time.Microsecond, func() {
		nestedSchedule(1)
	})

	// Run until panic occurs or timeout
	done := make(chan struct{})
	go func() {
		<-panicChan
		// After panic, check if nesting depth was restored
		// Give some time for the defer to execute
		time.Sleep(100 * time.Millisecond)
		depthAfterPanic.Store(loop.timerNestingDepth.Load())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("test timed out waiting for panic")
	}

	// Stop the loop
	cancel()
	wg.Wait()

	// Verify that nesting depth was restored properly after panic
	// The depth before panic should be > 0 (at depth 5)
	before := depthBeforePanic.Load()
	after := depthAfterPanic.Load()

	if before <= 0 {
		t.Errorf("Expected nesting depth > 0 before panic, got %d", before)
	}

	// After panic and defer, depth should be back to 0
	// (or a different timer might be running, so we just verify it's not stuck at panic depth)
	if after == before {
		t.Errorf("Nesting depth not restored after panic: before=%d, after=%d", before, after)
	}

	t.Logf("Panic test completed: depth before panic=%d, depth after panic=%d", before, after)
}

// TestTimerPoolFieldClearing verifies that timer fields are properly
// cleared before returning to the pool. This tests the fix for stale
// data leaks where heapIndex and nestingLevel were not reset.
func TestTimerPoolFieldClearing(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the loop in goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Wait for loop to initialize
	for i := 0; i < 100; i++ {
		if loop.state.IsRunning() {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	// Exercise the timer pool by creating many timers
	// If fields aren't cleared properly, timers from the pool
	// will have stale data that could cause crashes or bugs
	scheduleCount := 200
	cancelCount := 0
	fireCount := atomic.Int32{}
	var timerWG sync.WaitGroup

	for i := 0; i < scheduleCount; i++ {
		// Half the timers we let fire to exercise the normal path
		// Half we cancel to exercise the early return to pool path
		fire := i%2 == 0

		if fire {
			timerWG.Add(1)
			_, err := loop.ScheduleTimer(1*time.Millisecond, func() {
				defer timerWG.Done()
				fireCount.Add(1)
			})
			if err != nil {
				t.Fatalf("ScheduleTimer failed at iteration %d: %v", i, err)
			}
		} else {
			timerID, err := loop.ScheduleTimer(100*time.Millisecond, func() {
				t.Error("Timer should have been canceled")
			})
			if err != nil {
				t.Fatalf("ScheduleTimer failed at iteration %d: %v", i, err)
			}

			// Cancel immediately to return timer to pool
			if err := loop.CancelTimer(timerID); err != nil {
				t.Fatalf("CancelTimer failed at iteration %d: %v", i, err)
			}
			cancelCount++
		}
	}

	// Wait for all timers to either fire or be canceled
	timerWG.Wait()

	// Verify all the fired timers actually executed
	expectedFires := scheduleCount / 2
	actualFires := fireCount.Load()
	if actualFires != int32(expectedFires) {
		t.Errorf("Expected %d timers to fire, got %d", expectedFires, actualFires)
	}

	t.Logf("Successfully exercised timer pool: %d fired, %d canceled",
		expectedFires, cancelCount)

	// The test passes if we got here without crashes or panics
	// which would indicate stale heapIndex/nestingLevel issues

	// Stop the loop
	cancel()
	wg.Wait()
}

// TestCancelTimerInvalidHeapIndex verifies that CancelTimer properly
// handles timers with invalid heapIndex values (-1 or out of bounds).
// This tests the fix for bounds check bug where heapIndex < len(timers)
// didn't check for negative indices.
func TestCancelTimerInvalidHeapIndex(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the loop in goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Test 1: Cancel timer that has already fired (heapIndex might be invalid)
	TimerID1, err := loop.ScheduleTimer(1*time.Microsecond, func() {
		// This timer will fire very quickly
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait a bit for timer to fire
	time.Sleep(50 * time.Millisecond)

	// Now try to cancel it - it should return ErrTimerNotFound
	// and not panic on invalid heapIndex
	err = loop.CancelTimer(TimerID1)
	if err != ErrTimerNotFound {
		t.Errorf("Expected ErrTimerNotFound when canceling non-existent timer, got: %v", err)
	}

	// Test 2: Cancel timer with heapIndex = -1 after immediate scheduling
	// Schedule timer but don't wait for it to fire, then cancel
	var timer2Fired atomic.Bool
	TimerID2, err := loop.ScheduleTimer(1*time.Second, func() {
		timer2Fired.Store(true)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Cancel immediately - heapIndex might be valid now
	err = loop.CancelTimer(TimerID2)
	if err != nil {
		t.Errorf("CancelTimer failed: %v", err)
	}

	// Test 3: Multiple rapid cancellations
	for i := 0; i < 50; i++ {
		TimerID, err := loop.ScheduleTimer(1*time.Millisecond, func() {})
		if err != nil {
			t.Fatal(err)
		}

		// Immediately cancel
		if err := loop.CancelTimer(TimerID); err != nil {
			t.Errorf("CancelTimer failed at iteration %d: %v", i, err)
		}
	}

	// Test 4: Cancel from different goroutine concurrently
	TimerID3, err := loop.ScheduleTimer(100*time.Millisecond, func() {
		t.Error("Timer should not fire, it was canceled concurrently")
	})
	if err != nil {
		t.Fatal(err)
	}

	var cancelWG sync.WaitGroup
	for i := 0; i < 10; i++ {
		cancelWG.Add(1)
		go func() {
			defer cancelWG.Done()
			// Try to cancel from multiple goroutines
			loop.CancelTimer(TimerID3)
		}()
	}
	cancelWG.Wait()

	// Final test: verify no crashes or panics in any scenario
	t.Log("All CancelTimer edge cases passed without panic")

	// Stop the loop
	cancel()
	wg.Wait()
}

// TestTimerReuseSafety verifies that reused timers from pool don't
// carry over state from previous lifecycles.
func TestTimerReuseSafety(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the loop in goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Wait for loop to initialize
	for i := 0; i < 100; i++ {
		if loop.state.IsRunning() {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	// Create many timers sequentially, checking that each behaves correctly
	for iteration := 0; iteration < 50; iteration++ {
		executed := atomic.Bool{}

		// Schedule timer
		TimerID, err := loop.ScheduleTimer(1*time.Millisecond, func() {
			executed.Store(true)
		})
		if err != nil {
			t.Fatalf("ScheduleTimer failed at iteration %d: %v", iteration, err)
		}

		// Cancel half the timers to test pool return on cancel
		if iteration%2 == 0 {
			err = loop.CancelTimer(TimerID)
			// Timer may have already fired - that's OK
			if err != nil && !errors.Is(err, ErrTimerNotFound) {
				t.Fatalf("CancelTimer failed at iteration %d: %v", iteration, err)
			}

			// Verify timer didn't fire (unless it fired before cancel)
			time.Sleep(10 * time.Millisecond)
			if executed.Load() && err == nil {
				// Only error if we canceled successfully and it still fired
				t.Errorf("Timer executed despite cancellation at iteration %d", iteration)
			}
		} else {
			// Wait for timer to fire
			time.Sleep(15 * time.Millisecond)
			if !executed.Load() {
				t.Errorf("Timer failed to execute at iteration %d", iteration)
			}
		}
	}

	// Final verification: the pool should have reused timers
	// by now, with no state corruption
	t.Log("Timer reuse safety verified across 50 iterations")

	// Stop the loop
	cancel()
	wg.Wait()
}

// TestMultipleNestingLevelsWithPanic verifies that nesting depth
// recovers properly after multiple independent panic scenarios
// at different nesting depths.
func TestMultipleNestingLevelsWithPanic(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the loop in goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Test multiple scenarios with panic at different depths
	// Each scenario should properly restore nesting depth after panic
	scenarios := []struct {
		panicDepth int
		chainDepth int
	}{
		{panicDepth: 3, chainDepth: 5},
		{panicDepth: 4, chainDepth: 6},
		{panicDepth: 2, chainDepth: 4},
	}

	for i, scenario := range scenarios {
		var depthBeforePanic atomic.Int32
		var depthAfterPanic atomic.Int32
		panicTriggered := make(chan struct{})

		// Create a nested timer chain
		var createChain func(depth int)
		createChain = func(depth int) {
			if depth == scenario.panicDepth {
				// Record depth before panic
				depthBeforePanic.Store(loop.timerNestingDepth.Load())
				close(panicTriggered)
				panic("nesting panic test")
			}

			if depth < scenario.chainDepth {
				loop.ScheduleTimer(1*time.Microsecond, func() {
					createChain(depth + 1)
				})
			}
		}

		// Start the chain
		loop.ScheduleTimer(1*time.Microsecond, func() {
			createChain(1)
		})

		// Wait for panic to occur
		<-panicTriggered

		// Give time for defer to execute and restore nesting depth
		time.Sleep(50 * time.Millisecond)

		// Check nesting depth was restored
		depthAfterPanic.Store(loop.timerNestingDepth.Load())

		before := depthBeforePanic.Load()
		after := depthAfterPanic.Load()

		if before <= 0 {
			t.Errorf("Scenario %d: Expected nesting depth > 0 before panic, got %d", i, before)
		}

		// Verify nesting depth decreased after panic recovery
		if after >= before {
			t.Errorf("Scenario %d: Nesting depth not properly restored: before=%d, after=%d", i, before, after)
		}

		t.Logf("Scenario %d: panic at depth %d, before=%d, after=%d", i, scenario.panicDepth, before, after)
	}

	// Final check: nesting depth should be low after all scenarios
	finalDepth := loop.timerNestingDepth.Load()
	if finalDepth < 0 {
		t.Errorf("Nesting depth negative after all scenarios: %d", finalDepth)
	}

	t.Logf("Multiple nesting panic tests completed successfully, final depth=%d", finalDepth)

	// Stop the loop
	cancel()
	wg.Wait()
}
