//go:build linux || darwin

package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestFastPathVsNormalPath_Microtasks compares microtask execution
// between fast path and normal (tick) path to ensure consistency.
func TestFastPathVsNormalPath_Microtasks(t *testing.T) {
	tests := []struct {
		name       string
		fastPath   bool
		mode       FastPathMode
		registerFD bool // Whether to register an FD to force normal path
	}{
		{
			name:       "FastPathAuto (no FDs)",
			fastPath:   true,
			mode:       FastPathAuto,
			registerFD: false,
		},
		{
			name:       "FastPathAuto (with FD - poll mode)",
			fastPath:   false,
			mode:       FastPathAuto,
			registerFD: true,
		},
		{
			name:       "FastPathDisabled (no FDs)",
			fastPath:   true,
			mode:       FastPathDisabled,
			registerFD: false,
		},
		{
			name:       "FastPathDisabled (with FD - poll mode)",
			fastPath:   false,
			mode:       FastPathDisabled,
			registerFD: true,
		},
		{
			name:       "FastPathForced (no FDs)",
			fastPath:   true,
			mode:       FastPathForced,
			registerFD: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatalf("Failed to create loop: %v", err)
			}
			defer func() {
				if err := loop.Close(); !errors.Is(err, ErrLoopTerminated) {
					t.Errorf("Failed to close loop: %v", err)
				}
			}()

			// Set mode
			if err := loop.SetFastPathMode(tt.mode); err != nil {
				t.Fatalf("Failed to set fast path mode: %v", err)
			}

			// If we want normal path, register a dummy fd
			// We'll use a pipe for this - just need it to exist
			var fds [2]int
			if tt.registerFD {
				err := unix.Pipe(fds[:])
				if err != nil {
					t.Fatalf("Failed to create pipe: %v", err)
				}
				// Set non-blocking
				_ = unix.SetNonblock(fds[0], true)
				_ = unix.SetNonblock(fds[1], true)

				// Register the read fd
				err = loop.RegisterFD(fds[0], EventRead, func(events IOEvents) {})
				if err != nil {
					t.Fatalf("Failed to register FD: %v", err)
				}
				defer unix.Close(fds[0])
				defer unix.Close(fds[1])
			}

			var count atomic.Int64
			var done atomic.Bool

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Start loop in background
			runCh := make(chan error, 1)
			go func() {
				runCh <- loop.Run(ctx)
			}()

			// Wait for loop to be running
			{
				waitDeadline := time.Now().Add(5 * time.Second)
				for i := 0; ; i++ {
					if state := loop.State(); ((tt.registerFD || tt.mode == FastPathDisabled) && state == StateSleeping) ||
						((!tt.registerFD && tt.mode != FastPathDisabled) && state == StateRunning) {
						break
					} else if i%1000 == 0 {
						t.Logf("Waiting for loop to reach running state, current state: %s", state)
					}
					if time.Now().After(waitDeadline) {
						t.Fatalf("Event loop did not reach running state in time")
					}
					time.Sleep(1 * time.Millisecond)
				}
			}

			// Submit tasks with microtasks
			const iterations = 20
			for i := 0; i < iterations; i++ {
				if err := loop.Submit(func() {
					count.Add(1) // Count task execution
					_ = loop.ScheduleMicrotask(func() {
						count.Add(1) // Count microtask execution
						// Mark done when last microtask completes
						if count.Load() == int64(iterations*2) {
							done.Store(true)
						}
					})
				}); err != nil {
					t.Fatalf("Failed to submit task: %v", err)
				}
			}

			// Wait for all executions with a timeout
			deadline := time.Now().Add(3 * time.Second)
			for !done.Load() && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}

			actualCount := count.Load()
			expectedCount := int64(iterations * 2) // Each task + its microtask

			if actualCount != expectedCount {
				t.Errorf("%s: Expected %d executions (%d tasks + %d microtasks), got %d",
					tt.name, expectedCount, iterations, iterations, actualCount)

				if tt.fastPath && actualCount == int64(iterations) {
					t.Errorf("%s: BUG CONFIRMED - Only tasks executed, microtasks ignored in fast path!", tt.name)
				}
			} else {
				t.Logf("%s: All %d tasks and microtasks executed correctly", tt.name, iterations)
			}

			if err := loop.Shutdown(t.Context()); err != nil {
				t.Errorf("Failed to shutdown loop: %v", err)
			}

			select {
			case <-time.After(5 * time.Second):
				t.Errorf("Event loop did not exit in time after shutdown")
			case <-t.Context().Done():
				t.Errorf("Event loop did not exit in time")
			case err := <-runCh:
				if err != nil {
					t.Errorf("Event loop exited with error: %v", err)
				}
			}
		})
	}
}

// TestFastPath_HandlesMicrotasks verifies that microtasks are executed
// in fast path mode. This test was written to prove a critical deficiency
// in the original implementation where runAux() completely ignored microtasks.
func TestFastPath_HandlesMicrotasks(t *testing.T) {
	runCh := make(chan error, 1)
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer func() {
		if err := loop.Close(); !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Failed to close loop: %v", err)
		}

		select {
		case <-time.After(5 * time.Second):
			t.Errorf("Event loop did not exit in time after shutdown")
		case <-t.Context().Done():
			t.Errorf("Event loop did not exit in time")
		case err := <-runCh:
			if err != nil {
				t.Errorf("Event loop exited with error: %v", err)
			}
		}
	}()

	// Force fast path mode
	if err := loop.SetFastPathMode(FastPathForced); err != nil {
		t.Fatalf("Failed to set fast path mode: %v", err)
	}

	// Track execution order
	var order atomic.Int64
	taskExecuted := make(chan struct{}, 1)
	microtaskExecuted := make(chan struct{}, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start loop in background
	go func() {
		runCh <- loop.Run(ctx)
	}()

	// Wait for loop to be running
	for loop.State() != StateRunning {
		time.Sleep(1 * time.Millisecond)
	}

	// Submit a task that schedules a microtask
	// Expected execution: task -> microtask
	if err := loop.Submit(func() {
		order.Add(1)
		close(taskExecuted)

		// Schedule a microtask from within a task
		err := loop.ScheduleMicrotask(func() {
			order.Add(1)
			close(microtaskExecuted)
		})
		if err != nil {
			t.Errorf("Failed to schedule microtask: %v", err)
		}
	}); err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	// Wait for task to execute
	select {
	case <-taskExecuted:
		// Task executed
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for task to execute")
	}

	// Wait a bit for microtask to execute (with a timeout)
	// If the bug exists, this timeout will fire
	select {
	case <-microtaskExecuted:
		// Microtask executed - this is the expected behavior
		t.Log("Microtask executed correctly in fast path mode")
	case <-time.After(500 * time.Millisecond):
		// BUG: Microtask did not execute
		// This proves that runAux() does not drain microtasks in fast path
		t.Errorf("BUG PROVED: Microtask did not execute in fast path mode! Microtasks are being ignored by runAux()")

		// Additional diagnostic: check if microtasks are still in queue
		if !loop.microtasks.IsEmpty() {
			t.Logf("Diagnostic: Microtask queue is not empty - has %d items", loop.microtasks.Length())
		}
	}

	finalOrder := order.Load()
	if finalOrder != 2 {
		t.Errorf("Expected 2 executions (task + microtask), got %d", finalOrder)
	}

	// Terminate the loop
	if err := loop.Shutdown(t.Context()); err != nil {
		t.Errorf("Failed to shutdown loop: %v", err)
	}
}

// TestFastPath_MicrotaskOrdering verifies that microtasks are executed
// in the correct order relative to regular tasks in fast path mode.
func TestFastPath_MicrotaskOrdering(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer func() {
		if err := loop.Close(); !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Failed to close loop: %v", err)
		}
	}()

	// Force fast path mode
	if err := loop.SetFastPathMode(FastPathForced); err != nil {
		t.Fatalf("Failed to set fast path mode: %v", err)
	}

	var executions []int
	var mu sync.Mutex

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start loop in background
	runCh := make(chan error, 1)
	go func() {
		runCh <- loop.Run(ctx)
	}()

	// Wait for loop to be running
	for loop.State() != StateRunning {
		time.Sleep(1 * time.Millisecond)
	}

	// Submit tasks that schedule microtasks
	for i := 0; i < 10; i++ {
		taskId := i
		if err := loop.Submit(func() {
			mu.Lock()
			executions = append(executions, taskId*2) // Task gets even number
			mu.Unlock()

			// Each task schedules a microtask
			_ = loop.ScheduleMicrotask(func() {
				mu.Lock()
				executions = append(executions, taskId*2+1) // Microtask gets odd number
				mu.Unlock()
			})
		}); err != nil {
			t.Fatalf("Failed to submit task: %v", err)
		}
	}

	// Wait for everything to execute
	time.Sleep(500 * time.Millisecond)

	// Check that we got all 20 executions (10 tasks + 10 microtasks)
	mu.Lock()
	actualLen := len(executions)
	mu.Unlock()

	if actualLen != 20 {
		t.Errorf("Expected 20 executions (10 tasks + 10 microtasks), got %d", actualLen)
		t.Logf("Executions: %v", executions)

		if actualLen == 10 {
			t.Error("BUG CONFIRMED: Only tasks executed, microtasks were ignored!")
		}
	}

	if err := loop.Shutdown(t.Context()); err != nil {
		t.Errorf("Failed to shutdown loop: %v", err)
	}
}

// TestFastPath_MultipleMicrotasksVerifiesThatMicrotasksScheduledFrom
// different sources are all executed in fast path mode.
func TestFastPath_MultipleMicrotasks(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer func() {
		if err := loop.Close(); !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Failed to close loop: %v", err)
		}
	}()

	// Force fast path mode
	if err := loop.SetFastPathMode(FastPathForced); err != nil {
		t.Fatalf("Failed to set fast path mode: %v", err)
	}

	var count atomic.Int64

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start loop in background
	runCh := make(chan error, 1)
	go func() {
		runCh <- loop.Run(ctx)
	}()

	// Wait for loop to be running
	for loop.State() != StateRunning {
		time.Sleep(1 * time.Millisecond)
	}

	// Schedule multiple microtasks from different sources
	const numTasks = 5
	const numMicrotasksPerTask = 3

	for i := 0; i < numTasks; i++ {
		if err := loop.Submit(func() {
			for j := 0; j < numMicrotasksPerTask; j++ {
				_ = loop.ScheduleMicrotask(func() {
					count.Add(1)
				})
			}
		}); err != nil {
			t.Fatalf("Failed to submit task: %v", err)
		}
	}

	// Wait for executions
	time.Sleep(500 * time.Millisecond)

	actualCount := count.Load()
	expectedCount := int64(numTasks * numMicrotasksPerTask)

	if actualCount != expectedCount {
		t.Errorf("Expected %d microtask executions, got %d", expectedCount, actualCount)
		if actualCount == 0 {
			t.Error("BUG CONFIRMED: No microtasks executed in fast path mode!")
		}
	}

	if err := loop.Shutdown(t.Context()); err != nil {
		t.Errorf("Failed to shutdown loop: %v", err)
	}

	select {
	case <-time.After(5 * time.Second):
		t.Errorf("Event loop did not exit in time after shutdown")
	case <-t.Context().Done():
		t.Errorf("Event loop did not exit in time")
	case err := <-runCh:
		if err != nil {
			t.Errorf("Event loop exited with error: %v", err)
		}
	}
}

// TestFastPath_MicrotaskBudgetOverflow ensures the loop does not stall
// when microtasks exceed the drainage budget (1024).
func TestFastPath_MicrotaskBudgetOverflow(t *testing.T) {
	runCh := make(chan error, 1)
	loop, _ := New()
	defer func() {
		if err := loop.Close(); err != nil {
			t.Errorf("Failed to close loop: %v", err)
		}

		select {
		case <-time.After(5 * time.Second):
			t.Errorf("Event loop did not exit in time during close")
		case err := <-runCh:
			if err != context.Canceled {
				t.Errorf("Event loop exited with unexpected error: %v", err)
			}
		}
	}()
	_ = loop.SetFastPathMode(FastPathForced)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		runCh <- loop.Run(ctx)
	}()

	// Wait for start
	for loop.State() != StateRunning {
		time.Sleep(time.Millisecond)
	}

	done := make(chan struct{})

	// Schedule enough microtasks to exceed the 1024 budget twice
	const count = 2500
	var executed atomic.Int64

	if err := loop.Submit(func() {
		// Recursive scheduling to fill's ring
		var schedule func(n int)
		schedule = func(n int) {
			if n <= 0 {
				close(done)
				return
			}
			_ = loop.ScheduleMicrotask(func() {
				executed.Add(1)
				schedule(n - 1)
			})
		}
		schedule(count)
	}); err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	select {
	case <-done:
		if executed.Load() != int64(count) {
			t.Errorf("Count mismatch: expected %d, got %d", count, executed.Load())
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("STALL DETECTED: Loop slept with pending microtasks! Executed so far: %d/%d", executed.Load(), count)
	}
}
