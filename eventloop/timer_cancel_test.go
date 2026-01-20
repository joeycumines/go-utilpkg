// Copyright 2025 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test 1.1.9: Cancel before expiration
func TestScheduleTimerCancelBeforeExpiration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		if err := l.Run(ctx); err != nil {
			errChan <- err
		}
	}()

	// Give loop time to start
	time.Sleep(50 * time.Millisecond)

	var callbackRan atomic.Bool
	scheduleDelay := 100 * time.Millisecond

	id, err := l.ScheduleTimer(scheduleDelay, func() {
		callbackRan.Store(true)
	})
	if err != nil {
		t.Fatalf("ScheduleTimer failed: %v", err)
	}

	// Cancel the timer immediately
	if err := l.CancelTimer(id); err != nil {
		t.Fatalf("CancelTimer failed: %v", err)
	}

	// Wait long enough for timer to have fired if not canceled
	time.Sleep(2 * scheduleDelay)

	if callbackRan.Load() {
		t.Error("Timer callback ran after cancellation (expected: not run)")
	}

	l.Shutdown(context.Background())

	select {
	case err := <-errChan:
		t.Fatalf("Run() error: %v", err)
	default:
	}
}

// Test 1.1.10: Cancel after expiration
func TestScheduleTimerCancelAfterExpiration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		if err := l.Run(ctx); err != nil {
			errChan <- err
		}
	}()

	// Give loop time to start
	time.Sleep(50 * time.Millisecond)

	var callbackRan atomic.Bool
	scheduleDelay := 10 * time.Millisecond

	id, err := l.ScheduleTimer(scheduleDelay, func() {
		callbackRan.Store(true)
	})
	if err != nil {
		t.Fatalf("ScheduleTimer failed: %v", err)
	}

	// Wait for timer to fire
	time.Sleep(3 * scheduleDelay)

	if !callbackRan.Load() {
		t.Error("Timer callback did not fire (expected: run)")
	}

	// Try to cancel after timer has fired
	err = l.CancelTimer(id)
	if err != ErrTimerNotFound {
		t.Errorf("CancelTimer after expiration should return ErrTimerNotFound, got: %v", err)
	}

	l.Shutdown(context.Background())

	select {
	case err := <-errChan:
		t.Fatalf("Run() error: %v", err)
	default:
	}
}

// Test 1.1.11: Rapid successive cancellations
func TestScheduleTimerRapidCancellations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		if err := l.Run(ctx); err != nil {
			errChan <- err
		}
	}()

	// Give loop time to start
	time.Sleep(50 * time.Millisecond)

	const numTimers = 100
	var callbackCount atomic.Int32
	var ids [numTimers]TimerID
	var cancelErrors atomic.Int32

	// Schedule all timers
	for i := 0; i < numTimers; i++ {
		id, err := l.ScheduleTimer(100*time.Millisecond, func() {
			callbackCount.Add(1)
		})
		if err != nil {
			t.Fatalf("ScheduleTimer %d failed: %v", i, err)
		}
		ids[i] = id
	}

	// Cancel in random order
	rand.Shuffle(len(ids), func(i, j int) {
		ids[i], ids[j] = ids[j], ids[i]
	})

	for i, id := range ids {
		if err := l.CancelTimer(id); err != nil {
			t.Errorf("CancelTimer %d failed: %v", i, err)
			cancelErrors.Add(1)
		}
	}

	// Wait until all timers would have fired
	time.Sleep(2 * 100 * time.Millisecond)

	if count := callbackCount.Load(); count != 0 {
		t.Errorf("Expected 0 callbacks after cancellation, got %d", count)
	}

	if errCount := cancelErrors.Load(); errCount > 0 {
		t.Errorf("Had %d cancellation errors", errCount)
	}

	l.Shutdown(context.Background())

	select {
	case err := <-errChan:
		t.Fatalf("Run() error: %v", err)
	default:
	}
}

// Test 1.1.12: Cancel from different goroutine
func TestScheduleTimerCancelFromGoroutine(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		if err := l.Run(ctx); err != nil {
			errChan <- err
		}
	}()

	// Give loop time to start
	time.Sleep(50 * time.Millisecond)

	var callbackRan atomic.Bool
	scheduleDelay := 100 * time.Millisecond

	id, err := l.ScheduleTimer(scheduleDelay, func() {
		callbackRan.Store(true)
	})
	if err != nil {
		t.Fatalf("ScheduleTimer failed: %v", err)
	}

	// Cancel from different goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := l.CancelTimer(id); err != nil {
			t.Errorf("CancelTimer from different goroutine failed: %v", err)
		}
	}()

	wg.Wait()

	// Wait long enough for timer to have fired if not canceled
	time.Sleep(2 * scheduleDelay)

	if callbackRan.Load() {
		t.Error("Timer callback ran after cancellation from different goroutine")
	}

	l.Shutdown(context.Background())

	select {
	case err := <-errChan:
		t.Fatalf("Run() error: %v", err)
	default:
	}
}

// Test 1.1.13: Stress 1000 timers, cancel 50%
func TestScheduleTimerStressWithCancellations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		if err := l.Run(ctx); err != nil {
			errChan <- err
		}
	}()

	// Give loop time to start
	time.Sleep(50 * time.Millisecond)

	const numTimers = 1000
	const cancelCount = numTimers / 2 // Cancel 50%
	const expectedCallbacks = numTimers - cancelCount

	var callbackCount atomic.Int32
	var cancelErrors atomic.Int32
	ids := make([]TimerID, numTimers)

	// Schedule all timers with longer delays to avoid race between scheduling and firing
	// Using 100-200ms range gives enough time to schedule all 1000 timers before any fire
	for i := 0; i < numTimers; i++ {
		delay := time.Duration(100+(i%100)) * time.Millisecond // 100-200ms staggered delays
		id, err := l.ScheduleTimer(delay, func() {
			callbackCount.Add(1)
		})
		if err != nil {
			t.Fatalf("ScheduleTimer %d failed: %v", i, err)
		}
		ids[i] = id
	}

	// Cancel first 'cancelCount' timers
	for i := 0; i < cancelCount; i++ {
		if err := l.CancelTimer(ids[i]); err != nil {
			t.Errorf("CancelTimer %d failed: %v", i, err)
			cancelErrors.Add(1)
		}
	}

	// Wait for all non-canceled timers to fire (max delay ~200ms + buffer)
	time.Sleep(300 * time.Millisecond)

	count := callbackCount.Load()
	if count != expectedCallbacks {
		t.Errorf("Expected %d callbacks, got %d (cancelled %d)", expectedCallbacks, count, cancelCount)
	}

	if errCount := cancelErrors.Load(); errCount > 0 {
		t.Errorf("Had %d cancellation errors", errCount)
	}

	l.Shutdown(context.Background())

	select {
	case err := <-errChan:
		t.Fatalf("Run() error: %v", err)
	default:
	}
}

// Test: CancelTimer returns ErrTimerNotFound for invalid ID
func TestCancelTimerTimerNotFound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		if err := l.Run(ctx); err != nil {
			errChan <- err
		}
	}()

	// Give loop time to start
	time.Sleep(50 * time.Millisecond)

	// Try to cancel with invalid ID
	invalidID := TimerID(999999)
	err = l.CancelTimer(invalidID)
	if err != ErrTimerNotFound {
		t.Errorf("CancelTimer with invalid ID should return ErrTimerNotFound, got: %v", err)
	}

	l.Shutdown(context.Background())

	select {
	case err := <-errChan:
		t.Fatalf("Run() error: %v", err)
	default:
	}
}

// Test: Verify unique timer IDs
func TestScheduleTimerUniqueIdGeneration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		if err := l.Run(ctx); err != nil {
			errChan <- err
		}
	}()

	// Give loop time to start
	time.Sleep(50 * time.Millisecond)

	const numTimers = 1000
	ids := make(map[TimerID]struct{})

	// Schedule many timers and collect IDs
	for i := 0; i < numTimers; i++ {
		id, err := l.ScheduleTimer(100*time.Millisecond*time.Duration(i), func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer %d failed: %v", i, err)
		}

		// Check ID uniqueness
		if _, exists := ids[id]; exists {
			t.Errorf("Duplicate timer ID generated: %d", id)
		}
		ids[id] = struct{}{}
	}

	if len(ids) != numTimers {
		t.Errorf("Expected %d unique IDs, got %d", numTimers, len(ids))
	}

	l.Shutdown(context.Background())

	select {
	case err := <-errChan:
		t.Fatalf("Run() error: %v", err)
	default:
	}
}
