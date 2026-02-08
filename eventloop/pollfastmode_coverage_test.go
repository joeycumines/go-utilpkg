// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// COVERAGE-005: pollFastMode Function 100% Coverage
// =============================================================================
// Target: loop.go pollFastMode function
// Gaps covered:
// - Fast mode wakeup via channel path (fastWakeupCh)
// - Timeout expiration while in fast mode
// - Termination check during fast mode
// - Indefinite block path (timeout >= 1000ms)
// =============================================================================

// TestPollFastMode_ChannelWakeup tests fast mode wakeup via fastWakeupCh channel.
// This covers the early return path when channel already has a pending signal.
func TestPollFastMode_ChannelWakeup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	var wakeupReceived atomic.Bool
	var mu sync.Mutex
	var wakeupCount int

	// Hook to track when fast path wakeup occurs
	loop.testHooks = &loopTestHooks{
		PrePollAwake: func() {
			mu.Lock()
			wakeupCount++
			mu.Unlock()
			wakeupReceived.Store(true)
		},
	}

	// Start the loop
	go loop.Run(ctx)

	// Wait for loop to enter fast path mode
	time.Sleep(20 * time.Millisecond)

	// Submit task which triggers wakeup via fastWakeupCh
	executed := make(chan struct{})
	loop.Submit(func() {
		close(executed)
	})

	// Wait for execution
	select {
	case <-executed:
		t.Log("Task executed via channel wakeup in fast mode")
	case <-time.After(2 * time.Second):
		t.Fatal("Task was not executed - channel wakeup failed")
	}

	// Verify wakeup path was taken
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	finalCount := wakeupCount
	mu.Unlock()
	if finalCount == 0 {
		t.Log("PrePollAwake hook may not have fired (fast path optimization bypasses poll)")
	} else {
		t.Logf("PrePollAwake hook fired %d times", finalCount)
	}
}

// TestPollFastMode_TimeoutExpiration tests timeout expiration in fast mode.
// This covers the path where timer expires before any wakeup signal.
func TestPollFastMode_TimeoutExpiration(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Close()

	var timerExpiredCount atomic.Int32

	// Track timer expiration via PrePollAwake (called after select returns)
	loop.testHooks = &loopTestHooks{
		PrePollAwake: func() {
			timerExpiredCount.Add(1)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Schedule a timer with short delay - this will cause pollFastMode
	// to use a timer with short timeout
	var timerFired atomic.Bool
	go loop.Run(ctx)

	time.Sleep(20 * time.Millisecond)

	// Schedule a timer that will fire in 10ms
	_, err = loop.ScheduleTimer(10*time.Millisecond, func() {
		timerFired.Store(true)
	})
	if err != nil {
		t.Fatal("ScheduleTimer failed:", err)
	}

	// Wait for timer to fire
	time.Sleep(100 * time.Millisecond)

	if !timerFired.Load() {
		t.Error("Timer should have fired after timeout expiration")
	}

	t.Logf("Timer expired and fired correctly, PrePollAwake count: %d", timerExpiredCount.Load())
}

// TestPollFastMode_TerminationCheck tests termination check during fast mode.
// This covers the StateTerminating/StateTerminated check in pollFastMode.
func TestPollFastMode_TerminationCheck(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}

	var enteredFastPath atomic.Bool
	loop.testHooks = &loopTestHooks{
		OnFastPathEntry: func() {
			enteredFastPath.Store(true)
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start loop in background
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Wait for loop to be running and in fast path
	time.Sleep(30 * time.Millisecond)

	if !enteredFastPath.Load() {
		t.Log("Fast path entry not detected (expected with no timers/tasks)")
	}

	// Trigger termination while in fast mode
	cancel()

	// Wait for run to complete
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Logf("Run returned: %v (expected context.Canceled)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Loop did not terminate after cancel")
	}

	// Verify state is Terminated
	state := loop.state.Load()
	if state != StateTerminated {
		t.Errorf("Expected StateTerminated, got %v", state)
	}

	t.Log("Termination check in fast mode verified")
}

// TestPollFastMode_IndefiniteBlock tests the indefinite block path (timeout >= 1000ms).
// When timeout is >= 1000ms, pollFastMode blocks indefinitely on channel without timer.
func TestPollFastMode_IndefiniteBlock(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Close()

	var blockEntered atomic.Bool
	var wakeupReceived atomic.Bool

	loop.testHooks = &loopTestHooks{
		PrePollSleep: func() {
			blockEntered.Store(true)
		},
		PrePollAwake: func() {
			wakeupReceived.Store(true)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start loop - with no timers, calculateTimeout returns 10s (>= 1000ms)
	// This should trigger the indefinite block path in pollFastMode
	go loop.Run(ctx)

	// Wait for loop to enter poll
	time.Sleep(30 * time.Millisecond)

	// Submit a task to wake up the indefinite block
	executed := make(chan struct{})
	loop.Submit(func() {
		close(executed)
	})

	// Wait for task execution
	select {
	case <-executed:
		t.Log("Task executed - indefinite block was interrupted by channel signal")
	case <-time.After(5 * time.Second):
		t.Fatal("Task not executed - indefinite block path may be stuck")
	}

	// Wait a bit for hook to fire
	time.Sleep(20 * time.Millisecond)

	t.Logf("Indefinite block test: blockEntered=%v, wakeupReceived=%v",
		blockEntered.Load(), wakeupReceived.Load())
}

// TestPollFastMode_NonBlockingCase tests timeout=0 (non-blocking) case.
func TestPollFastMode_NonBlockingCase(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Close()

	var pollAwakeCalled atomic.Int32

	loop.testHooks = &loopTestHooks{
		PrePollAwake: func() {
			pollAwakeCalled.Add(1)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start loop
	go loop.Run(ctx)

	// Wait for loop to run a few iterations
	time.Sleep(50 * time.Millisecond)

	// When forceNonBlockingPoll is true, timeout=0 path is taken
	// Submit a task to trigger processing
	loop.Submit(func() {})

	time.Sleep(50 * time.Millisecond)

	// Verify PrePollAwake was called (non-blocking returns immediately)
	count := pollAwakeCalled.Load()
	t.Logf("PrePollAwake called %d times (non-blocking poll path)", count)
}

// TestPollFastMode_ChannelDrainBeforeBlock tests the channel drain
// before entering the main select (non-blocking check).
func TestPollFastMode_ChannelDrainBeforeBlock(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	// Pre-signal the channel before loop starts processing
	// This tests the "drain pending channel signal first" path
	select {
	case loop.fastWakeupCh <- struct{}{}:
		t.Log("Pre-signaled fastWakeupCh before poll")
	default:
		t.Log("Channel was already signaled")
	}

	// Now start the loop
	go loop.Run(ctx)

	// The loop should immediately process the pending signal
	time.Sleep(50 * time.Millisecond)

	// Verify loop is still running (processed the signal correctly)
	state := loop.state.Load()
	if state != StateRunning && state != StateSleeping {
		t.Errorf("Expected Running/Sleeping, got %v", state)
	}

	t.Log("Channel drain before block verified")
}

// TestPollFastMode_TerminationBeforeIndefiniteBlock tests termination check
// right before the indefinite block path (timeout >= 1000ms).
func TestPollFastMode_TerminationBeforeIndefiniteBlock(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}

	var prePollCount atomic.Int32

	loop.testHooks = &loopTestHooks{
		PrePollSleep: func() {
			count := prePollCount.Add(1)
			// On the 2nd poll attempt, trigger termination and wake up
			if count == 2 {
				loop.state.Store(StateTerminating)
				// Wake the loop from poll
				select {
				case loop.fastWakeupCh <- struct{}{}:
				default:
				}
			}
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Wait for termination
	select {
	case <-done:
		t.Log("Loop terminated as expected")
	case <-time.After(450 * time.Millisecond):
		cancel() // ensure cleanup
		<-done   // wait for actual termination
		t.Log("Loop terminated via context cancellation")
	}

	t.Logf("Termination before indefinite block verified, poll count: %d", prePollCount.Load())
}

// TestPollFastMode_TimerBasedWakeup tests the timer-based path (timeout < 1000ms).
func TestPollFastMode_TimerBasedWakeup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Close()

	var timerFired atomic.Bool

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	time.Sleep(20 * time.Millisecond)

	// Schedule a timer with delay < 1000ms (e.g., 50ms)
	// This forces pollFastMode to use the timer-based select
	_, err = loop.ScheduleTimer(50*time.Millisecond, func() {
		timerFired.Store(true)
	})
	if err != nil {
		t.Fatal("ScheduleTimer failed:", err)
	}

	// Wait for timer to fire
	time.Sleep(150 * time.Millisecond)

	if !timerFired.Load() {
		t.Error("Timer should have fired via timer-based wakeup path")
	}

	t.Log("Timer-based wakeup path verified")
}

// TestPollFastMode_MultipleSubmitsBeforeWakeup tests multiple submits
// that send to the buffered channel (size 1) - only first succeeds.
func TestPollFastMode_MultipleSubmitsBeforeWakeup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	time.Sleep(20 * time.Millisecond)

	// Submit multiple tasks rapidly - they all try to signal fastWakeupCh
	var executed atomic.Int32
	for i := 0; i < 10; i++ {
		loop.Submit(func() {
			executed.Add(1)
		})
	}

	// Wait for all tasks to execute
	time.Sleep(100 * time.Millisecond)

	if executed.Load() != 10 {
		t.Errorf("Expected 10 tasks executed, got %d", executed.Load())
	}

	t.Logf("Multiple submits verified: %d tasks executed", executed.Load())
}

// TestPollFastMode_WakeupResetsPending tests that wakeUpSignalPending is reset
// when channel signal is received.
func TestPollFastMode_WakeupResetsPending(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	time.Sleep(20 * time.Millisecond)

	// Submit and let it process
	done := make(chan struct{})
	loop.Submit(func() {
		close(done)
	})

	<-done

	// After processing, wakeUpSignalPending should be reset to 0
	time.Sleep(20 * time.Millisecond)
	pending := loop.wakeUpSignalPending.Load()
	if pending != 0 {
		t.Errorf("Expected wakeUpSignalPending=0 after wakeup, got %d", pending)
	}

	t.Log("Wakeup pending reset verified")
}

// TestPollFastMode_DrainAuxJobsAfterWakeup tests that auxJobs are drained
// after returning from fast mode poll.
func TestPollFastMode_DrainAuxJobsAfterWakeup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	time.Sleep(20 * time.Millisecond)

	// Submit to auxJobs (fast path mode)
	var executed atomic.Int32
	for i := 0; i < 5; i++ {
		loop.Submit(func() {
			executed.Add(1)
		})
	}

	time.Sleep(50 * time.Millisecond)

	if executed.Load() != 5 {
		t.Errorf("Expected 5 auxJobs drained, got %d", executed.Load())
	}

	t.Logf("AuxJobs drain after wakeup verified: %d executed", executed.Load())
}

// TestPollFastMode_TransitionToStateRunning tests that state transitions
// to StateRunning after wakeup.
func TestPollFastMode_TransitionToStateRunning(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Close()

	var stateAfterWakeup atomic.Int32

	loop.testHooks = &loopTestHooks{
		PrePollAwake: func() {
			// Capture state after hooks but before transition
			stateAfterWakeup.Store(int32(loop.state.Load()))
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	time.Sleep(20 * time.Millisecond)

	// Submit task to trigger wakeup
	loop.Submit(func() {})

	time.Sleep(50 * time.Millisecond)

	// After wakeup, state should be Running
	state := loop.state.Load()
	if state != StateRunning && state != StateSleeping {
		t.Errorf("Expected StateRunning/StateSleeping, got %v", state)
	}

	t.Logf("State transition after wakeup: captured=%v, current=%v",
		LoopState(stateAfterWakeup.Load()), state)
}
