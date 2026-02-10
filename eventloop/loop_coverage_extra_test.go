package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestScheduleNextTick_TerminatedState tests that ScheduleNextTick returns
// ErrLoopTerminated when called on a terminated loop.
func TestScheduleNextTick_TerminatedState(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = loop.Run(context.Background())
		close(done)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Shutdown the loop
	loop.Shutdown(context.Background())
	<-done

	// Now try to schedule a nextTick - should return ErrLoopTerminated
	err = loop.ScheduleNextTick(func() {
		t.Fatal("should not be called")
	})

	if err != ErrLoopTerminated {
		t.Errorf("expected ErrLoopTerminated, got: %v", err)
	}
}

// TestScheduleNextTick_NilFunc tests that ScheduleNextTick accepts nil function.
func TestScheduleNextTick_NilFunc(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	// Should not panic or error on nil function
	err = loop.ScheduleNextTick(nil)
	if err != nil {
		t.Errorf("expected nil error for nil func, got: %v", err)
	}
}

// TestScheduleNextTick_IOMode tests ScheduleNextTick wakeup in IO mode
// (when there are registered FDs).
func TestScheduleNextTick_IOMode(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = loop.Run(context.Background())
		close(done)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	var called atomic.Bool
	err = loop.ScheduleNextTick(func() {
		called.Store(true)
	})
	if err != nil {
		t.Fatalf("ScheduleNextTick failed: %v", err)
	}

	// Wait for callback
	time.Sleep(50 * time.Millisecond)

	if !called.Load() {
		t.Error("nextTick callback was not executed")
	}

	loop.Shutdown(context.Background())
	<-done
}

// TestSubmitInternal_IOMode tests SubmitInternal wakeup in IO mode.
func TestSubmitInternal_IOMode(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = loop.Run(context.Background())
		close(done)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	var called atomic.Bool
	err = loop.SubmitInternal(func() {
		called.Store(true)
	})
	if err != nil {
		t.Fatalf("SubmitInternal failed: %v", err)
	}

	// Wait for callback
	time.Sleep(50 * time.Millisecond)

	if !called.Load() {
		t.Error("internal task was not executed")
	}

	loop.Shutdown(context.Background())
	<-done
}

// TestSubmitInternal_TerminatedState tests that SubmitInternal returns
// ErrLoopTerminated when called on a terminated loop.
func TestSubmitInternal_TerminatedState(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = loop.Run(context.Background())
		close(done)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Shutdown the loop
	loop.Shutdown(context.Background())
	<-done

	// Now try to submit internal - should return ErrLoopTerminated
	err = loop.SubmitInternal(func() {
		t.Fatal("should not be called")
	})

	if err != ErrLoopTerminated {
		t.Errorf("expected ErrLoopTerminated, got: %v", err)
	}
}

// TestRunTimers_CanceledWithStrictOrdering tests the canceled timer path
// with StrictMicrotaskOrdering enabled.
func TestRunTimers_CanceledWithStrictOrdering(t *testing.T) {
	loop, err := New(WithStrictMicrotaskOrdering(true))
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = loop.Run(context.Background())
		close(done)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Schedule a timer and immediately cancel it
	timerID, err := loop.ScheduleTimer(100*time.Millisecond, func() {
		t.Fatal("canceled timer should not fire")
	})
	if err != nil {
		t.Fatalf("ScheduleTimer failed: %v", err)
	}

	// Cancel the timer
	if err := loop.CancelTimer(timerID); err != nil {
		t.Errorf("CancelTimer failed: %v", err)
	}

	// Wait past the timer deadline
	time.Sleep(150 * time.Millisecond)

	loop.Shutdown(context.Background())
	<-done
}

// TestOnOverload_NormalCallback tests the OnOverload path without panic.
func TestOnOverload_NormalCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	var overloadCalled atomic.Int32
	loop.OnOverload = func(err error) {
		overloadCalled.Add(1)
		if err != ErrLoopOverloaded {
			t.Errorf("expected ErrLoopOverloaded, got: %v", err)
		}
	}

	done := make(chan struct{})
	go func() {
		_ = loop.Run(context.Background())
		close(done)
	}()

	// Flood the loop to trigger overload (requires exceeding budget of 1024)
	for i := 0; i < 2000; i++ {
		_ = loop.Submit(func() {
			// Very slow task to ensure budget is exceeded
			time.Sleep(10 * time.Millisecond)
		})
	}

	// Wait for overload to be triggered
	time.Sleep(200 * time.Millisecond)

	// Note: Overload may or may not be triggered depending on timing
	// The test passes if no panic occurs - the callback may be called 0+ times
	t.Logf("OnOverload called %d times", overloadCalled.Load())

	loop.Shutdown(context.Background())
	<-done
}

// TestProcessExternal_ProcessesNextTickPriority tests that nextTick
// callbacks are processed before regular microtasks.
func TestProcessExternal_ProcessesNextTickPriority(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = loop.Run(context.Background())
		close(done)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	var order []string
	var mu sync.Mutex

	// Submit initial task
	var wg sync.WaitGroup
	wg.Add(3)

	err = loop.Submit(func() {
		// Schedule a regular microtask
		loop.ScheduleMicrotask(func() {
			mu.Lock()
			order = append(order, "microtask")
			mu.Unlock()
			wg.Done()
		})

		// Schedule a nextTick (should run first)
		loop.ScheduleNextTick(func() {
			mu.Lock()
			order = append(order, "nextTick")
			mu.Unlock()
			wg.Done()
		})

		mu.Lock()
		order = append(order, "task")
		mu.Unlock()
		wg.Done()
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for callbacks")
	}

	mu.Lock()
	defer mu.Unlock()

	// Order should be: task (synchronous), nextTick (priority), microtask
	if len(order) != 3 {
		t.Errorf("expected 3 items in order, got %d: %v", len(order), order)
	} else {
		if order[0] != "task" {
			t.Errorf("first item should be 'task', got %q", order[0])
		}
		if order[1] != "nextTick" {
			t.Errorf("second item should be 'nextTick', got %q", order[1])
		}
		if order[2] != "microtask" {
			t.Errorf("third item should be 'microtask', got %q", order[2])
		}
	}

	loop.Shutdown(context.Background())
	<-done
}

// TestPSquare_EdgeCases tests pSquareMultiQuantile edge cases.
func TestPSquare_EdgeCases(t *testing.T) {
	t.Run("empty_max", func(t *testing.T) {
		mq := newPSquareMultiQuantile(0.5, 0.9, 0.99)
		// Max on empty should return 0
		if mq.Max() != 0 {
			t.Errorf("expected 0, got %f", mq.Max())
		}
	})

	t.Run("empty_mean", func(t *testing.T) {
		mq := newPSquareMultiQuantile(0.5, 0.9, 0.99)
		// Mean on empty should return 0
		if mq.Mean() != 0 {
			t.Errorf("expected 0, got %f", mq.Mean())
		}
	})

	t.Run("single_value", func(t *testing.T) {
		mq := newPSquareMultiQuantile(0.5, 0.9, 0.99)
		mq.Update(42.0)
		if mq.Mean() != 42.0 {
			t.Errorf("expected 42.0, got %f", mq.Mean())
		}
		if mq.Max() != 42.0 {
			t.Errorf("expected 42.0, got %f", mq.Max())
		}
	})
}
