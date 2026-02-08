//go:build linux || darwin

package alternatethree

import (
	"context"
	"sync"
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- loop.Run(ctx)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Shutdown() failed: %v", err)
	}

	// Wait for Run to return
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after Shutdown()")
	}
}

// TestLoop_Submit tests basic task submission.
func TestLoop_Submit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var executed atomic.Bool

	// Run in background
	go func() {
		_ = loop.Run(ctx)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	// Submit task
	err = loop.Submit(func() {
		executed.Store(true)
	})
	if err != nil {
		t.Errorf("Submit() failed: %v", err)
	}

	// Wait for execution
	deadline := time.Now().Add(time.Second)
	for !executed.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if !executed.Load() {
		t.Error("Task was not executed")
	}

	// Cleanup
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	_ = loop.Shutdown(shutdownCtx)
}

// TestLoop_SubmitInternal tests internal task submission.
func TestLoop_SubmitInternal(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var executed atomic.Bool

	// Run in background
	go func() {
		_ = loop.Run(ctx)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	// Submit internal task
	err = loop.SubmitInternal(Task{Runnable: func() {
		executed.Store(true)
	}})
	if err != nil {
		t.Errorf("SubmitInternal() failed: %v", err)
	}

	// Wait for execution
	deadline := time.Now().Add(time.Second)
	for !executed.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if !executed.Load() {
		t.Error("Internal task was not executed")
	}

	// Cleanup
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	_ = loop.Shutdown(shutdownCtx)
}

// TestLoop_Close tests immediate close.
func TestLoop_Close(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- loop.Run(ctx)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	// Close immediately
	if err := loop.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// Wait for Run to return
	select {
	case <-errCh:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return after Close()")
	}
}

// TestLoop_ConcurrentSubmit tests concurrent task submission.
func TestLoop_ConcurrentSubmit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const numTasks = 1000
	var counter atomic.Int64

	// Run in background
	go func() {
		_ = loop.Run(ctx)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	// Submit tasks concurrently
	var wg sync.WaitGroup
	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = loop.Submit(func() {
				counter.Add(1)
			})
		}()
	}
	wg.Wait()

	// Wait for all tasks to execute
	deadline := time.Now().Add(5 * time.Second)
	for counter.Load() < numTasks && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if counter.Load() != numTasks {
		t.Errorf("Expected %d tasks executed, got %d", numTasks, counter.Load())
	}

	// Cleanup
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	_ = loop.Shutdown(shutdownCtx)
}

// TestLoop_ShutdownIdempotent tests that Shutdown is idempotent.
func TestLoop_ShutdownIdempotent(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run in background
	go func() {
		_ = loop.Run(ctx)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	// Multiple concurrent shutdown calls
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
			defer shutdownCancel()
			_ = loop.Shutdown(shutdownCtx)
		}()
	}
	wg.Wait()
}

// TestLoop_ErrLoopAlreadyRunning tests that Run returns error if already running.
func TestLoop_ErrLoopAlreadyRunning(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run in background
	go func() {
		_ = loop.Run(ctx)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	// Try to run again
	err = loop.Run(ctx)
	if err != ErrLoopAlreadyRunning {
		t.Errorf("Expected ErrLoopAlreadyRunning, got: %v", err)
	}

	// Cleanup
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	_ = loop.Shutdown(shutdownCtx)
}

// TestLoop_ScheduleTimer tests timer scheduling.
func TestLoop_ScheduleTimer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var executed atomic.Bool

	// Run in background
	go func() {
		_ = loop.Run(ctx)
	}()

	// Give loop time to start
	time.Sleep(10 * time.Millisecond)

	// Schedule timer
	err = loop.ScheduleTimer(50*time.Millisecond, func() {
		executed.Store(true)
	})
	if err != nil {
		t.Errorf("ScheduleTimer() failed: %v", err)
	}

	// Wait for execution (with margin)
	time.Sleep(200 * time.Millisecond)

	if !executed.Load() {
		t.Error("Timer task was not executed")
	}

	// Cleanup
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	_ = loop.Shutdown(shutdownCtx)
}
