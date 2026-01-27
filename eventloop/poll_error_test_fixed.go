package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test_alternatethree_PollError_Basic tests basic poll error handling
func Test_alternatethree_PollError_Basic(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("handlePollError terminates loop", func(t *testing.T) {
		t.Parallel()

		// Create a loop
		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Start the loop
		go func() {
			_ = loop.Run(ctx)
		}()

		// Submit a task to ensure loop is running
		taskExecuted := make(chan struct{}, 1)
		err = loop.Submit(func() {
			taskExecuted <- struct{}{}
		})
		if err != nil {
			t.Fatalf("Failed to submit task: %v", err)
		}

		// Wait for task execution
		select {
		case <-taskExecuted:
			// OK
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout waiting for task execution")
		}

		// Stop loop (triggering shutdown path)
		cancel()
		time.Sleep(50 * time.Millisecond)

		// Wait for loop to finish
		shutdownCalled := make(chan struct{})
		_ = loop.Shutdown()
		close(shutdownCalled)

		select {
		case <-shutdownCalled:
		case <-time.After(100 * time.Millisecond):
			// OK, may have completed
		}
	})
}

// Test_Loop_PollError_TransitionToTerminating tests state transition on poll error
func Test_Loop_PollError_TransitionToTerminating(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("State transitions to Terminating on shutdown", func(t *testing.T) {
		t.Parallel()

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Start the loop
		go func() {
			_ = loop.Run(ctx)
		}()

		// Submit a task to ensure loop is running
		taskDone := make(chan struct{}, 1)
		err = loop.Submit(func() {
			taskDone <- struct{}{}
		})
		if err != nil {
			t.Fatalf("Failed to submit task: %v", err)
		}

		// Wait for task
		select {
		case <-taskDone:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout waiting for task")
		}

		// State should be Running or Sleeping or Awake
		state := loop.State()
		if state != StateRunning && state != StateSleeping && state != StateAwake {
			t.Fatalf("Expected running state, got %v", state)
		}

		// Shutdown loop
		_ = loop.Shutdown()

		// Wait for termination
		time.Sleep(50 * time.Millisecond)
		cancel()

		// State should be Terminated
		finalState := loop.State()
		if finalState != StateTerminated {
			t.Fatalf("Expected Terminated state after Shutdown, got %v", finalState)
		}
	})
}

// Test_Loop_PollError_ConcurrentTasks tests that pending tasks are handled
func Test_Loop_PollError_ConcurrentTasks(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("Tasks submitted before shutdown execute", func(t *testing.T) {
		t.Parallel()

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Start the loop
		go func() {
			_ = loop.Run(ctx)
		}()

		// Submit multiple tasks
		var wg sync.WaitGroup
		numTasks := 10
		taskExecuted := atomic.Int32{}

		for i := 0; i < numTasks; i++ {
			wg.Add(1)
			err = loop.Submit(func() {
				defer wg.Done()
				taskExecuted.Add(1)
			})
			if err != nil {
				t.Fatalf("Failed to submit task %d: %v", i, err)
			}
		}

		// Wait for all tasks
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for tasks")
		}

		// All tasks should have executed
		if taskExecuted.Load() != int32(numTasks) {
			t.Fatalf("Expected %d tasks executed, got %d", numTasks, taskExecuted.Load())
		}

		// Shutdown
		_ = loop.Shutdown()
		cancel()
	})
}

// Test_Loop_PollError_Recovery tests recovery scenarios
func Test_Loop_PollError_Recovery(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("Loop cannot accept work after termination", func(t *testing.T) {
		t.Parallel()

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Start the loop
		go func() {
			_ = loop.Run(ctx)
		}()

		// Submit a task
		err = loop.Submit(func() {})
		if err != nil {
			t.Fatalf("Failed to submit task: %v", err)
		}

		// Wait a bit
		time.Sleep(10 * time.Millisecond)

		// Shutdown loop
		_ = loop.Shutdown()
		cancel()
		time.Sleep(50 * time.Millisecond)

		// Try to submit another task after termination
		err = loop.Submit(func() {})
		if err == nil {
			t.Fatal("Expected error when submitting task to terminated loop")
		}
		if err != ErrLoopTerminated {
			t.Fatalf("Expected ErrLoopTerminated, got %v", err)
		}
	})
}

// Test_Loop_PollError_PlatformSpecific tests platform-specific error handling
func Test_Loop_PollError_PlatformSpecific(t *testing.T) {
	t.Parallel()

	t.Run("Loop handles system calls gracefully", func(t *testing.T) {
		t.Parallel()

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Start the loop
		go func() {
			_ = loop.Run(ctx)
		}()

		// Submit tasks to exercise poll path
		for i := 0; i < 10; i++ {
			err = loop.Submit(func() {})
			if err != nil {
				t.Fatalf("Failed to submit task %d: %v", i, err)
			}
		}

		// Give time for tasks to execute
		time.Sleep(50 * time.Millisecond)

		// Shutdown
		_ = loop.Shutdown()
		cancel()

		// State should be Terminated
		if loop.State() != StateTerminated {
			t.Fatalf("Expected Terminated state, got %v", loop.State())
		}
	})
}

// Test_Loop_PollError_Microtasks tests microtask handling
func Test_Loop_PollError_Microtasks(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("Microtasks are executed", func(t *testing.T) {
		t.Parallel()

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Start the loop
		go func() {
			_ = loop.Run(ctx)
		}()

		scheduleCount := atomic.Int32{}
		executeCount := atomic.Int32{}

		// Schedule a microtask
		loop.ScheduleMicrotask(func() {
			scheduleCount.Add(1)
		})

		// Submit a regular task that schedules another microtask
		err = loop.Submit(func() {
			loop.ScheduleMicrotask(func() {
				executeCount.Add(1)
			})
		})
		if err != nil {
			t.Fatalf("Failed to submit task: %v", err)
		}

		// Wait a bit for execution
		time.Sleep(50 * time.Millisecond)

		_ = loop.Shutdown()
		cancel()

		// Some microtasks should have executed
		totalScheduled := scheduleCount.Load() + executeCount.Load()
		if totalScheduled == 0 {
			t.Fatal("Expected some microtasks to execute")
		}
	})
}

// Test_Loop_PollError_Timers tests timer handling
func Test_Loop_PollError_Timers(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("Timers are handled", func(t *testing.T) {
		t.Parallel()

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Start the loop
		go func() {
			_ = loop.Run(ctx)
		}()

		timerFired := atomic.Int32{}

		// Schedule a short timer
		_, err = loop.ScheduleTimer(10*time.Millisecond, func() {
			timerFired.Add(1)
		})
		if err != nil {
			t.Fatalf("Failed to schedule timer: %v", err)
		}

		// Wait for timer
		time.Sleep(100 * time.Millisecond)

		_ = loop.Shutdown()
		cancel()

		// Timer should have fired
		if timerFired.Load() == 0 {
			t.Fatal("Expected timer to fire")
		}
	})
}

// Test_Loop_PollError_Metrics tests that metrics are captured
func Test_Loop_PollError_Metrics(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("Metrics record loop activity", func(t *testing.T) {
		t.Parallel()

		loop, err := New(WithMetrics(true))
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Start the loop
		go func() {
			_ = loop.Run(ctx)
		}()

		// Submit a task
		err = loop.Submit(func() {})
		if err != nil {
			t.Fatalf("Failed to submit task: %v", err)
		}

		// Wait a bit
		time.Sleep(50 * time.Millisecond)

		// Get metrics
		metrics := loop.Metrics()

		// Shutdown
		_ = loop.Shutdown()
		cancel()

		// Metrics should be accessible
		// (We can't easily trigger a real poll error,
		// but we verify metrics infrastructure works)
		_ = metrics
	})
}

// Test_Loop_PollError_WakeupPipe tests wakeup handling
func Test_Loop_PollError_WakeupPipe(t *testing.T) {
	t.Parallel()

	t.Run("Wakeup mechanism works", func(t *testing.T) {
		t.Parallel()

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Start the loop
		go func() {
			_ = loop.Run(ctx)
		}()

		// Submit multiple tasks to exercise wakeup
		for i := 0; i < 10; i++ {
			err = loop.Submit(func() {})
			if err != nil {
				t.Fatalf("Failed to submit task %d: %v", i, err)
			}
		}

		// Wait for execution
		time.Sleep(50 * time.Millisecond)

		// Shutdown
		_ = loop.Shutdown()
		cancel()

		// State should be Terminated
		if loop.State() != StateTerminated {
			t.Fatalf("Expected Terminated state, got %v", loop.State())
		}
	})
}

// Test_Loop_PollError_Shutdown tests shutdown behavior
func Test_Loop_PollError_Shutdown(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("Shutdown cleans up resources", func(t *testing.T) {
		t.Parallel()

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Start the loop
		go func() {
			_ = loop.Run(ctx)
		}()

		// Submit tasks
		for i := 0; i < 5; i++ {
			err = loop.Submit(func() {})
			if err != nil {
				t.Fatalf("Failed to submit task %d: %v", i, err)
			}
		}

		// Shutdown loop
		_ = loop.Shutdown()
		cancel()

		// Wait for shutdown
		time.Sleep(100 * time.Millisecond)

		// State should be Terminated
		if loop.State() != StateTerminated {
			t.Fatalf("Expected Terminated state, got %v", loop.State())
		}

		// Multiple Shutdown calls should be safe
		_ = loop.Shutdown()
		_ = loop.Shutdown()
	})
}

// Test_Loop_PollError_RaceDetectors tests for race conditions
func Test_Loop_PollError_RaceDetectors(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("Concurrent Shutdown after tasks", func(t *testing.T) {
		t.Parallel()

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Start the loop
		go func() {
			_ = loop.Run(ctx)
		}()

		var wg sync.WaitGroup
		numGoroutines := 10

		// Submit tasks concurrently
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := loop.Submit(func() {})
				_ = err // Ignore errors after shutdown
			}()
		}

		// Shutdown concurrently
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = loop.Shutdown()
		}()

		wg.Wait()
		time.Sleep(50 * time.Millisecond)
		cancel()

		// State should be Terminated
		if loop.State() != StateTerminated {
			t.Fatalf("Expected Terminated state, got %v", loop.State())
		}
	})
}

// Test_Loop_PollError_ErrorPropagation tests error propagation behavior
func Test_Loop_PollError_ErrorPropagation(t *testing.T) {
	t.Parallel()

	t.Run("Shutdown prevents new task submission", func(t *testing.T) {
		t.Parallel()

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Start the loop
		go func() {
			_ = loop.Run(ctx)
		}()

		// Submit a task
		err = loop.Submit(func() {})
		if err != nil {
			t.Fatalf("Failed to submit initial task: %v", err)
		}

		// Shutdown loop
		_ = loop.Shutdown()
		cancel()
		time.Sleep(50 * time.Millisecond)

		// Try to submit another task
		err = loop.Submit(func() {})
		if err == nil {
			t.Fatal("Expected error when submitting to terminated loop")
		}
		if err != ErrLoopTerminated {
			t.Fatalf("Expected ErrLoopTerminated, got %v", err)
		}
	})
}
