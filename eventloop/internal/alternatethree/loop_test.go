package alternatethree

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestLoop_BasicRunShutdown tests basic loop lifecycle.
func TestLoop_BasicRunShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateThreeTestLoop(t, loop)
	harness.shutdown(t)
}

// TestLoop_Submit tests basic task submission.
func TestLoop_Submit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	var executed atomic.Bool
	harness := startAlternateThreeTestLoop(t, loop)
	done := make(chan struct{})

	// Submit task
	err = loop.Submit(func() {
		executed.Store(true)
		close(done)
	})
	if err != nil {
		t.Errorf("Submit() failed: %v", err)
	}

	waitAlternateThreeSignal(t, done, "submitted task")

	if !executed.Load() {
		t.Error("Task was not executed")
	}

	harness.shutdown(t)
}

// TestLoop_SubmitInternal tests internal task submission.
func TestLoop_SubmitInternal(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	var executed atomic.Bool
	harness := startAlternateThreeTestLoop(t, loop)
	done := make(chan struct{})

	// Submit internal task
	err = loop.SubmitInternal(Task{Runnable: func() {
		executed.Store(true)
		close(done)
	}})
	if err != nil {
		t.Errorf("SubmitInternal() failed: %v", err)
	}

	waitAlternateThreeSignal(t, done, "internal task")

	if !executed.Load() {
		t.Error("Internal task was not executed")
	}

	harness.shutdown(t)
}

// TestLoop_Close tests immediate close.
func TestLoop_Close(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateThreeTestLoop(t, loop)

	// Close immediately
	if err := loop.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	harness.wait(t)
	if state := LoopState(loop.state.Load()); state != StateTerminated {
		t.Errorf("state = %v, want StateTerminated", state)
	}
}

// TestLoop_ConcurrentSubmit tests concurrent task submission.
func TestLoop_ConcurrentSubmit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	const numTasks = 1000
	var counter atomic.Int64
	harness := startAlternateThreeTestLoop(t, loop)
	results := make(chan error, numTasks)
	callbacks := make(chan struct{}, numTasks)

	// Submit tasks concurrently
	for range numTasks {
		go func() {
			results <- loop.Submit(func() {
				counter.Add(1)
				callbacks <- struct{}{}
			})
		}()
	}
	deadline := time.NewTimer(alternateThreeTestTimeout)
	defer deadline.Stop()
	for range numTasks {
		select {
		case submitErr := <-results:
			if submitErr != nil {
				t.Fatalf("concurrent Submit() failed: %v", submitErr)
			}
		case <-deadline.C:
			t.Fatalf("only %d/%d submissions returned", counter.Load(), numTasks)
		}
	}
	for range numTasks {
		waitAlternateThreeSignal(t, callbacks, "concurrent callback")
	}

	if counter.Load() != numTasks {
		t.Errorf("Expected %d tasks executed, got %d", numTasks, counter.Load())
	}

	harness.shutdown(t)
}

// TestLoop_ShutdownIdempotent tests that Shutdown is idempotent.
func TestLoop_ShutdownIdempotent(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateThreeTestLoop(t, loop)

	// Multiple concurrent shutdown calls
	results := make(chan error, 10)
	start := make(chan struct{})
	for range 10 {
		go func() {
			<-start
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), alternateThreeTestTimeout)
			defer shutdownCancel()
			results <- loop.Shutdown(shutdownCtx)
		}()
	}
	close(start)
	for range 10 {
		select {
		case shutdownErr := <-results:
			if shutdownErr != nil {
				t.Errorf("concurrent Shutdown() returned %v, want nil", shutdownErr)
			}
		case <-time.After(alternateThreeTestTimeout):
			t.Fatal("concurrent Shutdown() calls did not return")
		}
	}
	harness.wait(t)
}

// TestLoop_ErrLoopAlreadyRunning tests that Run returns error if already running.
func TestLoop_ErrLoopAlreadyRunning(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	harness := startAlternateThreeTestLoop(t, loop)

	// Try to run again
	err = loop.Run(context.Background())
	if err != ErrLoopAlreadyRunning {
		t.Errorf("Expected ErrLoopAlreadyRunning, got: %v", err)
	}

	harness.shutdown(t)
}

// TestLoop_ScheduleTimer tests timer scheduling.
func TestLoop_ScheduleTimer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	var executed atomic.Bool
	harness := startAlternateThreeTestLoop(t, loop)
	done := make(chan struct{})

	// Schedule timer
	err = loop.ScheduleTimer(50*time.Millisecond, func() {
		executed.Store(true)
		close(done)
	})
	if err != nil {
		t.Errorf("ScheduleTimer() failed: %v", err)
	}

	waitAlternateThreeSignal(t, done, "scheduled timer")

	if !executed.Load() {
		t.Error("Timer task was not executed")
	}

	harness.shutdown(t)
}
