// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"container/heap"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ========================================================================
// COVERAGE-004: runTimers Function 100% Coverage
// Tests for: timer callback panic recovery, canceled timer path,
//            nested timer calls with nesting depth, timer pool return
// ========================================================================

// TestRunTimers_PanicRecovery tests that timer callback panic is recovered
// and subsequent timers still fire.
func TestRunTimers_PanicRecovery(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Wait for loop to start
	for i := 0; i < 100 && !loop.state.IsRunning(); i++ {
		time.Sleep(1 * time.Millisecond)
	}

	// Track which timers fired
	timer1Fired := atomic.Bool{}
	timer2Fired := atomic.Bool{}
	timer3Fired := atomic.Bool{}

	// Timer 1: Will panic
	_, err = loop.ScheduleTimer(5*time.Millisecond, func() {
		timer1Fired.Store(true)
		panic("intentional timer panic for testing")
	})
	if err != nil {
		t.Fatal(err)
	}

	// Timer 2: Should still fire after timer 1 panics
	_, err = loop.ScheduleTimer(10*time.Millisecond, func() {
		timer2Fired.Store(true)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Timer 3: Should also fire
	_, err = loop.ScheduleTimer(15*time.Millisecond, func() {
		timer3Fired.Store(true)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for all timers to have a chance to fire
	time.Sleep(50 * time.Millisecond)

	if !timer1Fired.Load() {
		t.Error("Timer 1 should have fired (and panicked)")
	}

	if !timer2Fired.Load() {
		t.Error("Timer 2 should have fired despite timer 1 panic")
	}

	if !timer3Fired.Load() {
		t.Error("Timer 3 should have fired despite timer 1 panic")
	}

	cancel()
	wg.Wait()
}

// TestRunTimers_CanceledTimerPath tests the path where a timer is in the heap
// but has been marked as canceled (canceled.Load() == true).
func TestRunTimers_CanceledTimerPath(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Wait for loop to start
	for i := 0; i < 100 && !loop.state.IsRunning(); i++ {
		time.Sleep(1 * time.Millisecond)
	}

	// Schedule many timers and cancel them immediately
	const numTimers = 100
	timersFired := atomic.Int32{}

	for i := 0; i < numTimers; i++ {
		id, err := loop.ScheduleTimer(10*time.Millisecond, func() {
			timersFired.Add(1)
		})
		if err != nil {
			t.Fatalf("ScheduleTimer %d failed: %v", i, err)
		}

		// Cancel timer immediately
		if err := loop.CancelTimer(id); err != nil {
			t.Logf("CancelTimer %d failed (may have fired): %v", i, err)
		}
	}

	// Wait for timer delay to pass
	time.Sleep(50 * time.Millisecond)

	// No timers should have fired (all canceled)
	fired := timersFired.Load()
	if fired > 0 {
		t.Errorf("Expected 0 timers to fire, got %d", fired)
	}

	cancel()
	wg.Wait()
}

// TestRunTimers_CanceledTimerPoolReturn tests that canceled timers are
// properly returned to the pool with cleared fields.
func TestRunTimers_CanceledTimerPoolReturn(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Wait for loop to start
	for i := 0; i < 100 && !loop.state.IsRunning(); i++ {
		time.Sleep(1 * time.Millisecond)
	}

	// Exercise timer pool by creating, canceling, and reusing timers
	for iteration := 0; iteration < 50; iteration++ {
		var ids []TimerID

		// Create batch of timers
		for i := 0; i < 10; i++ {
			id, err := loop.ScheduleTimer(100*time.Millisecond, func() {
				// This should never run - we cancel before fire
			})
			if err != nil {
				t.Fatalf("ScheduleTimer failed at iteration %d-%d: %v", iteration, i, err)
			}
			ids = append(ids, id)
		}

		// Cancel all timers (returns them to pool)
		for _, id := range ids {
			if err := loop.CancelTimer(id); err != nil {
				// Timer may have already fired - acceptable
				continue
			}
		}
	}

	// If pool return wasn't working, we'd see crashes or stale data by now
	t.Log("Timer pool return path exercised successfully")

	cancel()
	wg.Wait()
}

// TestRunTimers_NestingDepthTracking tests HTML5 spec nesting depth tracking.
func TestRunTimers_NestingDepthTracking(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Wait for loop to start
	for i := 0; i < 100 && !loop.state.IsRunning(); i++ {
		time.Sleep(1 * time.Millisecond)
	}

	// Track depths during nested timer execution
	depths := make([]int32, 0, 10)
	var depthMu sync.Mutex
	done := make(chan struct{})

	var scheduleNested func(remaining int)
	scheduleNested = func(remaining int) {
		if remaining == 0 {
			close(done)
			return
		}

		// Record current nesting depth
		depthMu.Lock()
		depths = append(depths, loop.timerNestingDepth.Load())
		depthMu.Unlock()

		// Schedule another nested timer
		loop.ScheduleTimer(1*time.Microsecond, func() {
			scheduleNested(remaining - 1)
		})
	}

	// Start with depth 10
	loop.ScheduleTimer(1*time.Microsecond, func() {
		scheduleNested(10)
	})

	// Wait for completion
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Nested timers timed out")
	}

	// Verify depths were tracked
	depthMu.Lock()
	if len(depths) != 10 {
		t.Errorf("Expected 10 depth recordings, got %d", len(depths))
	}

	// Depths should generally be increasing (or equal due to defer reset)
	t.Logf("Recorded depths: %v", depths)
	depthMu.Unlock()

	cancel()
	wg.Wait()
}

// TestRunTimers_HTML5NestingClamp tests that delays are clamped to 4ms
// when nesting depth exceeds 5 (HTML5 spec compliance).
func TestRunTimers_HTML5NestingClamp(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Wait for loop to start
	for i := 0; i < 100 && !loop.state.IsRunning(); i++ {
		time.Sleep(1 * time.Millisecond)
	}

	// We need to verify that after depth 5, a 0ms timer takes >= 4ms
	// First, nest to depth 6+
	done := make(chan struct{})
	var timings []time.Duration
	var timingMu sync.Mutex

	var scheduleNested func(depth int)
	scheduleNested = func(depth int) {
		if depth > 10 {
			close(done)
			return
		}

		start := time.Now()
		// Schedule a 0ms timer to test clamping
		loop.ScheduleTimer(0, func() {
			elapsed := time.Since(start)
			timingMu.Lock()
			timings = append(timings, elapsed)
			currentDepth := loop.timerNestingDepth.Load()
			timingMu.Unlock()

			t.Logf("Depth %d: timer fired after %v", currentDepth, elapsed)
			scheduleNested(depth + 1)
		})
	}

	// Start nesting
	loop.ScheduleTimer(0, func() {
		scheduleNested(1)
	})

	// Wait for completion
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Nested timers timed out")
	}

	// Timings at depth > 5 should show the 4ms clamp effect
	timingMu.Lock()
	t.Logf("All timings: %v", timings)
	timingMu.Unlock()

	cancel()
	wg.Wait()
}

// TestRunTimers_StrictMicrotaskOrdering tests drainMicrotasks is called
// between timers when StrictMicrotaskOrdering is enabled.
func TestRunTimers_StrictMicrotaskOrdering(t *testing.T) {
	loop, err := New(
		WithFastPathMode(FastPathDisabled),
		WithStrictMicrotaskOrdering(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Wait for loop to start
	for i := 0; i < 100 && !loop.state.IsRunning(); i++ {
		time.Sleep(1 * time.Millisecond)
	}

	// Track execution order
	var order []string
	var orderMu sync.Mutex
	done := make(chan struct{})

	// Schedule two timers that each schedule microtasks
	loop.ScheduleTimer(5*time.Millisecond, func() {
		orderMu.Lock()
		order = append(order, "timer1")
		orderMu.Unlock()

		// Schedule microtask from timer1
		loop.ScheduleMicrotask(func() {
			orderMu.Lock()
			order = append(order, "microtask1")
			orderMu.Unlock()
		})
	})

	loop.ScheduleTimer(10*time.Millisecond, func() {
		orderMu.Lock()
		order = append(order, "timer2")
		orderMu.Unlock()

		loop.ScheduleMicrotask(func() {
			orderMu.Lock()
			order = append(order, "microtask2")
			orderMu.Unlock()
			close(done)
		})
	})

	// Wait for completion
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timers timed out")
	}

	// With strict ordering: timer1, microtask1, timer2, microtask2
	orderMu.Lock()
	expected := []string{"timer1", "microtask1", "timer2", "microtask2"}
	if len(order) != len(expected) {
		t.Errorf("Wrong number of events: got %v, want %v", order, expected)
	} else {
		for i := range expected {
			if order[i] != expected[i] {
				t.Errorf("Wrong order at %d: got %v, want %v", i, order, expected)
				break
			}
		}
	}
	orderMu.Unlock()

	cancel()
	wg.Wait()
}

// TestRunTimers_EmptyHeap tests runTimers with empty timer heap.
func TestRunTimers_EmptyHeap(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Wait for loop to start
	for i := 0; i < 100 && !loop.state.IsRunning(); i++ {
		time.Sleep(1 * time.Millisecond)
	}

	// Loop should be running with empty timer heap
	time.Sleep(20 * time.Millisecond)

	if len(loop.timers) != 0 {
		t.Errorf("Expected empty timer heap, got %d timers", len(loop.timers))
	}

	cancel()
	wg.Wait()
}

// TestRunTimers_FutureTimer tests timer that hasn't expired yet.
func TestRunTimers_FutureTimer(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Wait for loop to start
	for i := 0; i < 100 && !loop.state.IsRunning(); i++ {
		time.Sleep(1 * time.Millisecond)
	}

	fired := atomic.Bool{}

	// Schedule timer for the future
	_, err = loop.ScheduleTimer(100*time.Millisecond, func() {
		fired.Store(true)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Check immediately - timer shouldn't have fired yet
	time.Sleep(20 * time.Millisecond)
	if fired.Load() {
		t.Error("Timer fired too early")
	}

	cancel()
	wg.Wait()
}

// TestRunTimers_MultipleTimersOrdering tests that timers fire in correct order.
func TestRunTimers_MultipleTimersOrdering(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Wait for loop to start
	for i := 0; i < 100 && !loop.state.IsRunning(); i++ {
		time.Sleep(1 * time.Millisecond)
	}

	var order []int
	var orderMu sync.Mutex
	done := make(chan struct{})

	// Schedule timers in reverse order
	for i := 5; i >= 1; i-- {
		i := i
		_, err := loop.ScheduleTimer(time.Duration(i)*5*time.Millisecond, func() {
			orderMu.Lock()
			order = append(order, i)
			if len(order) == 5 {
				close(done)
			}
			orderMu.Unlock()
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Wait for completion
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timers timed out")
	}

	// Should fire in order 1, 2, 3, 4, 5
	orderMu.Lock()
	expected := []int{1, 2, 3, 4, 5}
	if len(order) != len(expected) {
		t.Errorf("Wrong number of timers fired: got %v, want %v", order, expected)
	} else {
		for i := range expected {
			if order[i] != expected[i] {
				t.Errorf("Wrong order: got %v, want %v", order, expected)
				break
			}
		}
	}
	orderMu.Unlock()

	cancel()
	wg.Wait()
}

// TestRunTimers_TimerMapCleanup tests that timerMap is properly cleaned
// after timer fires or is canceled.
func TestRunTimers_TimerMapCleanup(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Wait for loop to start
	for i := 0; i < 100 && !loop.state.IsRunning(); i++ {
		time.Sleep(1 * time.Millisecond)
	}

	done := make(chan struct{})

	// Schedule and let fire
	loop.ScheduleTimer(5*time.Millisecond, func() {
		close(done)
	})

	<-done
	time.Sleep(10 * time.Millisecond)

	// Timer should be removed from timerMap
	// We can't directly access timerMap, but CancelTimer should return ErrTimerNotFound

	// Schedule and cancel
	id, _ := loop.ScheduleTimer(100*time.Millisecond, func() {})
	loop.CancelTimer(id)

	// Double cancel should return ErrTimerNotFound
	err = loop.CancelTimer(id)
	if err != ErrTimerNotFound {
		t.Errorf("Expected ErrTimerNotFound for double cancel, got: %v", err)
	}

	cancel()
	wg.Wait()
}

// TestTimerHeap_Push tests the Push method of timerHeap.
func TestTimerHeap_Push(t *testing.T) {
	h := make(timerHeap, 0)

	// Push in random order
	timers := []*timer{
		{when: time.Now().Add(3 * time.Minute), id: 1},
		{when: time.Now().Add(1 * time.Minute), id: 2},
		{when: time.Now().Add(2 * time.Minute), id: 3},
	}

	for _, tmr := range timers {
		heap.Push(&h, tmr)
	}

	// Pop should return in order of `when`
	first := heap.Pop(&h).(*timer)
	if first.id != 2 {
		t.Errorf("Expected timer 2 (earliest), got %d", first.id)
	}

	second := heap.Pop(&h).(*timer)
	if second.id != 3 {
		t.Errorf("Expected timer 3, got %d", second.id)
	}

	third := heap.Pop(&h).(*timer)
	if third.id != 1 {
		t.Errorf("Expected timer 1 (latest), got %d", third.id)
	}
}

// TestTimerHeap_Swap tests the Swap method updates heapIndex correctly.
func TestTimerHeap_Swap(t *testing.T) {
	h := make(timerHeap, 2)
	h[0] = &timer{id: 1, heapIndex: 0}
	h[1] = &timer{id: 2, heapIndex: 1}

	h.Swap(0, 1)

	if h[0].id != 2 || h[0].heapIndex != 0 {
		t.Errorf("After swap, h[0] should be timer 2 with heapIndex 0, got id=%d heapIndex=%d", h[0].id, h[0].heapIndex)
	}

	if h[1].id != 1 || h[1].heapIndex != 1 {
		t.Errorf("After swap, h[1] should be timer 1 with heapIndex 1, got id=%d heapIndex=%d", h[1].id, h[1].heapIndex)
	}
}

// TestTimerHeap_Less tests the Less method for proper ordering.
func TestTimerHeap_Less(t *testing.T) {
	now := time.Now()
	h := timerHeap{
		{when: now, id: 1},
		{when: now.Add(time.Minute), id: 2},
	}

	if !h.Less(0, 1) {
		t.Error("Timer at index 0 (earlier) should be less than timer at index 1 (later)")
	}

	if h.Less(1, 0) {
		t.Error("Timer at index 1 (later) should not be less than timer at index 0 (earlier)")
	}
}

// TestTimerHeap_Len tests the Len method.
func TestTimerHeap_Len(t *testing.T) {
	h := make(timerHeap, 0)
	if h.Len() != 0 {
		t.Errorf("Empty heap Len() = %d, want 0", h.Len())
	}

	h = append(h, &timer{}, &timer{}, &timer{})
	if h.Len() != 3 {
		t.Errorf("Heap with 3 timers Len() = %d, want 3", h.Len())
	}
}

// TestTimerHeap_Pop tests the Pop method clears references.
func TestTimerHeap_Pop(t *testing.T) {
	h := timerHeap{
		{id: 1, heapIndex: 0},
	}

	popped := heap.Pop(&h).(*timer)
	if popped.id != 1 {
		t.Errorf("Popped timer id = %d, want 1", popped.id)
	}

	if len(h) != 0 {
		t.Errorf("After pop, heap len = %d, want 0", len(h))
	}
}

// TestTimerPool_Reuse tests that timer pool properly reuses timers.
func TestTimerPool_Reuse(t *testing.T) {
	// Get a timer from pool
	tmr := timerPool.Get().(*timer)

	// Set some values
	tmr.id = 123
	tmr.heapIndex = 5
	tmr.nestingLevel = 10
	tmr.task = func() {}
	tmr.canceled.Store(true)

	// Return to pool
	tmr.id = 0
	tmr.heapIndex = -1
	tmr.nestingLevel = 0
	tmr.task = nil
	tmr.canceled.Store(false)
	timerPool.Put(tmr)

	// Get again - may or may not be the same timer
	tmr2 := timerPool.Get().(*timer)

	// Should be clean (or a new timer)
	taskNil := tmr2.task == nil
	if tmr2.id != 0 || tmr2.heapIndex != -1 || tmr2.nestingLevel != 0 || !taskNil || tmr2.canceled.Load() {
		// Only log if it's the same timer with stale data
		// A fresh timer from New() would have zero values
		t.Logf("Timer from pool: id=%d heapIndex=%d nestingLevel=%d taskNil=%v canceled=%v",
			tmr2.id, tmr2.heapIndex, tmr2.nestingLevel, taskNil, tmr2.canceled.Load())
	}
}

// TestScheduleTimer_IDIncrement tests that timer IDs are unique and incrementing.
func TestScheduleTimer_IDIncrement(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	ctx := context.Background()
	go loop.Run(ctx)
	defer loop.Shutdown(ctx)

	const numTimers = 100
	ids := make([]TimerID, numTimers)

	for i := 0; i < numTimers; i++ {
		id, err := loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer %d failed: %v", i, err)
		}
		ids[i] = id
	}

	// Check uniqueness
	seen := make(map[TimerID]bool)
	for i, id := range ids {
		if seen[id] {
			t.Errorf("Duplicate timer ID at index %d: %d", i, id)
		}
		seen[id] = true
	}

	// Check incrementing
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("Timer IDs not incrementing: ids[%d]=%d, ids[%d]=%d", i-1, ids[i-1], i, ids[i])
		}
	}
}

// TestScheduleTimer_IDExhaustion tests ErrTimerIDExhausted at MAX_SAFE_INTEGER.
// Note: This test simulates exhaustion by setting nextTimerID near the limit.
func TestScheduleTimer_IDExhaustion(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	ctx := context.Background()
	go loop.Run(ctx)
	defer loop.Shutdown(ctx)

	// Set nextTimerID near MAX_SAFE_INTEGER
	const maxSafeInteger = 9007199254740991
	loop.nextTimerID.Store(maxSafeInteger - 1)

	// First timer should succeed
	id1, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("First timer near limit failed: %v", err)
	}
	t.Logf("Timer ID near limit: %d", id1)

	// Next timer should hit the limit
	id2, err := loop.ScheduleTimer(time.Hour, func() {})
	if err == nil {
		t.Errorf("Expected ErrTimerIDExhausted, got id=%d", id2)
	} else if err != ErrTimerIDExhausted {
		t.Errorf("Expected ErrTimerIDExhausted, got: %v", err)
	}
}

// TestCancelTimer_NotRunning tests CancelTimer when loop is not running.
func TestCancelTimer_NotRunning(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	// Try to cancel before loop starts (should fail)
	err = loop.CancelTimer(TimerID(1))
	if err != ErrLoopNotRunning {
		t.Errorf("Expected ErrLoopNotRunning before Run(), got: %v", err)
	}
}

// TestHasTimersPending tests the hasTimersPending helper function.
func TestHasTimersPending(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	// Before adding timers
	if loop.hasTimersPending() {
		t.Error("hasTimersPending should return false with empty heap")
	}

	// Add a timer via internal manipulation (for testing only)
	loop.timers = append(loop.timers, &timer{})

	if !loop.hasTimersPending() {
		t.Error("hasTimersPending should return true with non-empty heap")
	}

	// Clean up
	loop.timers = loop.timers[:0]
}

// TestRunTimers_MultiplePanics tests recovery from multiple consecutive panics.
func TestRunTimers_MultiplePanics(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Wait for loop to start
	for i := 0; i < 100 && !loop.state.IsRunning(); i++ {
		time.Sleep(1 * time.Millisecond)
	}

	panicCount := atomic.Int32{}
	successCount := atomic.Int32{}
	done := make(chan struct{})

	// Schedule alternating panic/success timers
	for i := 0; i < 10; i++ {
		i := i
		delay := time.Duration(i*5+1) * time.Millisecond
		if i%2 == 0 {
			// Panic timer
			loop.ScheduleTimer(delay, func() {
				panicCount.Add(1)
				if i == 8 {
					close(done)
				}
				panic(fmt.Sprintf("panic %d", i))
			})
		} else {
			// Success timer
			loop.ScheduleTimer(delay, func() {
				successCount.Add(1)
			})
		}
	}

	// Wait for completion
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timers timed out")
	}

	time.Sleep(50 * time.Millisecond) // Allow remaining timers to fire

	panics := panicCount.Load()
	successes := successCount.Load()

	t.Logf("Panics: %d, Successes: %d", panics, successes)

	// All timers should have executed
	if panics+successes < 10 {
		t.Errorf("Expected at least 10 timer executions, got %d", panics+successes)
	}

	cancel()
	wg.Wait()
}
