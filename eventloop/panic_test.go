package eventloop

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestPanicIsolation_IngressTaskPanic verifies that a panic in one task
// does not crash the loop and subsequent tasks continue to execute.
func TestPanicIsolation_IngressTaskPanic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runDone := make(chan struct{})
	go func() {
		if err := loop.Run(ctx); err != nil {
			t.Errorf("Run() unexpected error: %v", err)
		}
		close(runDone)
	}()

	var (
		task1Executed atomic.Bool
		task3Executed atomic.Bool
	)

	done := make(chan struct{})

	loop.Submit(func() {
		task1Executed.Store(true)
	})

	loop.Submit(func() {
		panic("intentional test panic")
	})

	loop.Submit(func() {
		task3Executed.Store(true)
		close(done)
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Loop appears to have crashed - task 3 never executed")
	}

	if !task1Executed.Load() {
		t.Error("Task 1 should have executed")
	}
	if !task3Executed.Load() {
		t.Fatal("PANIC ISOLATION FAILURE: Task 3 did not execute after Task 2 panicked")
	}

	loop.Shutdown(context.Background())
	<-runDone
}

// TestLoopSurvivesPanic_ContinuesProcessing verifies that the loop survives
// multiple panics and continues processing tasks. 100 tasks are submitted,
// every 10th panics, and 90 should complete successfully.
func TestLoopSurvivesPanic_ContinuesProcessing(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runDone := make(chan struct{})
	go func() {
		if err := loop.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Run() unexpected error: %v", err)
		}
		close(runDone)
	}()

	const total = 100
	var executed atomic.Int32

	done := make(chan struct{})

	for i := 0; i < total; i++ {
		idx := i
		loop.Submit(func() {
			if idx%10 == 5 {
				panic("periodic panic")
			}
			executed.Add(1)
			if executed.Load() == total-10 {
				close(done)
			}
		})
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("Only %d/%d tasks executed", executed.Load(), total-10)
	}

	if executed.Load() != 90 {
		t.Fatalf("Expected 90 successful executions, got %d", executed.Load())
	}
}

// TestLoop_SurvivesPanic verifies the loop continues processing after a task panics.
// This is a simplified version of TestLoopSurvivesPanic_ContinuesProcessing for clarity.
func TestLoop_SurvivesPanic(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runDone := make(chan struct{})
	go func() {
		if err := l.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Run() unexpected error: %v", err)
		}
		close(runDone)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	l.Submit(func() {
		panic("This should not crash the loop")
	})

	time.Sleep(10 * time.Millisecond)

	done := make(chan struct{})
	l.Submit(func() {
		close(done)
	})

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Loop dead: Panic crashed the worker")
	}

	// Shutdown and wait for loop to complete
	l.Shutdown(context.Background())
	<-runDone
}
