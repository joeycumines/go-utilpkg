package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestBarrierOrderingModes verifies Phase 7: Ingress Barrier Ambiguity Resolution.
// It proves the difference between Default Mode (Batch Barrier) and Strict Mode (Per-Task Barrier).
func TestBarrierOrderingModes(t *testing.T) {
	// Part 1: Verify Default Mode (Batch Barrier)
	// Expectation: Task A schedules Microtask. Task B runs BEFORE Microtask.
	t.Run("DefaultMode_BatchBarrier", func(t *testing.T) {
		l, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}
		// Ensure StrictMode is false (default)
		l.StrictMicrotaskOrdering = false

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan struct{})
		errChan := make(chan error, 1)
		go func() {
			if err := l.Run(ctx); err != nil {
				errChan <- err
				return
			}
			close(runDone)
		}()
		defer func() {
			l.Shutdown(context.Background())
			<-runDone
			select {
			case err := <-errChan:
				t.Fatalf("Failed to start loop: %v", err)
			default:
			}
		}()

		var executionOrder []string
		var mu sync.Mutex
		var wg sync.WaitGroup
		wg.Add(3) // Task A, Task B, Microtask

		// Task A: Schedules Microtask
		taskA := func() {
			mu.Lock()
			executionOrder = append(executionOrder, "TaskA")
			mu.Unlock()

			// Schedule Microtask
			l.microtasks = append(l.microtasks, Task{
				Runnable: func() {
					mu.Lock()
					executionOrder = append(executionOrder, "Microtask")
					mu.Unlock()
					wg.Done()
				},
			})
			wg.Done()
		}

		// Task B: Runs after Task A.
		// In Default Mode, this should run BEFORE the Microtask scheduled by A.
		taskB := func() {
			mu.Lock()
			executionOrder = append(executionOrder, "TaskB")
			mu.Unlock()
			wg.Done()
		}

		// Submit both tasks
		l.Submit(Task{Runnable: taskA})
		l.Submit(Task{Runnable: taskB})

		// Wait for completion
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

		// Verify Order
		mu.Lock()
		defer mu.Unlock()

		// Expected Order: TaskA -> TaskB -> Microtask
		// Because ingress is processed in a batch, and microtasks run after the batch.
		if len(executionOrder) != 3 {
			t.Fatalf("Expected 3 steps, got %d: %v", len(executionOrder), executionOrder)
		}
		if executionOrder[0] != "TaskA" {
			t.Errorf("Step 1 should be TaskA, got %s", executionOrder[0])
		}
		if executionOrder[1] != "TaskB" {
			t.Errorf("Step 2 should be TaskB, got %s (Default Mode should batch ingress)", executionOrder[1])
		}
		if executionOrder[2] != "Microtask" {
			t.Errorf("Step 3 should be Microtask, got %s", executionOrder[2])
		}
	})

	// Part 2: Verify Strict Mode (Per-Task Barrier)
	// Expectation: Task A schedules Microtask. Microtask runs IMMEDIATELY. Task B runs LAST.
	t.Run("StrictMode_PerTaskBarrier", func(t *testing.T) {
		l, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}
		// Enable Strict Mode
		l.StrictMicrotaskOrdering = true

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan struct{})
		errChan := make(chan error, 1)
		go func() {
			if err := l.Run(ctx); err != nil {
				errChan <- err
				return
			}
			close(runDone)
		}()
		defer func() {
			l.Shutdown(context.Background())
			<-runDone
			select {
			case err := <-errChan:
				t.Fatalf("Failed to start loop: %v", err)
			default:
			}
		}()

		var executionOrder []string
		var mu sync.Mutex
		var wg sync.WaitGroup
		wg.Add(3)

		// Task A: Schedules Microtask
		taskA := func() {
			mu.Lock()
			executionOrder = append(executionOrder, "TaskA")
			mu.Unlock()

			// Schedule Microtask directly to internal queue for this test
			// (Simulating a Promise resolution or similar microtask generation)
			// Note: We access l.microtasks unsafely here because we know we are inside the loop,
			// but wait... Submit runs from OUTSIDE.
			// Ah, we can't access l.microtasks from here safely if TaskA is running inside the loop?
			// Yes we can, TaskA runs on the loop goroutine.
			// But modifying l.microtasks directly is cheating/unsafe?
			// Let's assume we have a way to schedule microtask.
			// Currently `l.microtasks` is public.
			// Ideally we should use a method `l.ScheduleMicrotask(task)`.
			// Since we don't have that exposed yet, we'll append to the slice.
			// However, this test runs the closure ON the loop, so it has access to `l`.
			l.microtasks = append(l.microtasks, Task{
				Runnable: func() {
					mu.Lock()
					executionOrder = append(executionOrder, "Microtask")
					mu.Unlock()
					wg.Done()
				},
			})
			wg.Done()
		}

		// Task B: Runs after Task A
		taskB := func() {
			mu.Lock()
			executionOrder = append(executionOrder, "TaskB")
			mu.Unlock()
			wg.Done()
		}

		// Submit both tasks
		l.Submit(Task{Runnable: taskA})
		l.Submit(Task{Runnable: taskB})

		// Wait for completion
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

		// Verify Order
		mu.Lock()
		defer mu.Unlock()

		// Expected Order: TaskA -> Microtask -> TaskB
		// Because StrictMode forces a barrier (drain) after EACH ingress task.
		if len(executionOrder) != 3 {
			t.Fatalf("Expected 3 steps, got %d: %v", len(executionOrder), executionOrder)
		}
		if executionOrder[0] != "TaskA" {
			t.Errorf("Step 1 should be TaskA, got %s", executionOrder[0])
		}
		if executionOrder[1] != "Microtask" {
			t.Errorf("Step 2 should be Microtask, got %s (Strict Mode should drain immediately)", executionOrder[1])
		}
		if executionOrder[2] != "TaskB" {
			t.Errorf("Step 3 should be TaskB, got %s", executionOrder[2])
		}
	})
}

// TestMicrotaskBudgetBypass verifies that StrictMode respects the budget logic too,
// although the budget is per-drain call. If a single ingress task spawns 1000 microtasks,
// strict mode will try to drain them all before the next ingress task.
func TestStrictModeRespectsBudget(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}
	l.StrictMicrotaskOrdering = true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan struct{})
	errChan := make(chan error, 1)
	go func() {
		if err := l.Run(ctx); err != nil {
			errChan <- err
			return
		}
		close(runDone)
	}()
	defer func() {
		l.Shutdown(context.Background())
		<-runDone
		select {
		case err := <-errChan:
			t.Errorf("Run() unexpected error: %v", err)
		default:
		}
	}()

	var ops atomic.Int32
	done := make(chan struct{})

	// Task spawns many microtasks
	task := func() {
		// Spawn 2000 microtasks (Budget is 1024)
		for i := 0; i < 2000; i++ {
			l.microtasks = append(l.microtasks, Task{Runnable: func() {
				ops.Add(1)
				if ops.Load() == 2000 {
					close(done)
				}
			}})
		}
	}

	l.Submit(Task{Runnable: task})

	select {
	case <-done:
		if ops.Load() != 2000 {
			t.Errorf("Expected 2000 ops, got %d", ops.Load())
		}
	case <-time.After(time.Second * 2):
		t.Fatal("Timeout waiting for microtasks (budget logic might have stalled loop?)")
	}
}
