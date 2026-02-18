package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestScheduleMicrotask_Basic tests microtask scheduling.
// Priority: CRITICAL - scheduleMicrotask currently at 0.0% coverage.
//
// This function is called for queueing tasks that run after I/O and
// timers, used heavily by promises and async operations.
func TestScheduleMicrotask_Basic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var executed atomic.Bool
	microtask := func() {
		executed.Store(true)
	}

	// Call ScheduleMicrotask directly
	err = loop.ScheduleMicrotask(microtask)
	if err != nil {
		t.Fatalf("ScheduleMicrotask failed: %v", err)
	}

	// Start loop
	go loop.Run(ctx)

	// Wait for microtask to execute
	time.Sleep(50 * time.Millisecond)

	if !executed.Load() {
		t.Error("Microtask did not execute")
	}

	t.Log("ScheduleMicrotask basic test passed")
}

// TestScheduleMicrotask_Ordering tests microtask execution order.
// Priority: HIGH - Microtask ordering is critical for promise chaining.
func TestScheduleMicrotask_Ordering(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var results []int
	var resultsMu sync.Mutex
	var done atomic.Bool
	done.Store(false)

	// Schedule multiple microtasks
	for i := range 5 {
		loop.ScheduleMicrotask(func() {
			resultsMu.Lock()
			results = append(results, i)
			resultsMu.Unlock()
		})
	}

	// Schedule a final microtask to signal completion
	loop.ScheduleMicrotask(func() {
		done.Store(true)
	})

	go loop.Run(ctx)

	// Wait for completion signal
	for !done.Load() {
		time.Sleep(10 * time.Millisecond)
	}

	// Now safe to read results - all microtasks have completed
	resultsMu.Lock()
	defer resultsMu.Unlock()

	if len(results) != 5 {
		t.Errorf("Expected 5 results, got: %d", len(results))
	}

	// Verify FIFO order
	for i, r := range results {
		if r != i {
			t.Errorf("Microtask order mismatch: expected %d, got %d", i, r)
		}
	}

	t.Log("ScheduleMicrotask ordering test passed")
}

// TestScheduleMicrotask_Concurrent tests concurrent microtask scheduling.
// Priority: HIGH - Thread-safety verification.
func TestScheduleMicrotask_Concurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var executed atomic.Int64
	const numGoroutines = 10
	const numTasksEach = 10

	// Schedule microtasks concurrently
	for range numGoroutines {
		go func() {
			for range numTasksEach {
				err := loop.ScheduleMicrotask(func() {
					executed.Add(1)
				})
				if err != nil {
					t.Error("ScheduleMicrotask failed:", err)
				}
			}
		}()
	}

	go loop.Run(ctx)

	// Wait for execution
	time.Sleep(300 * time.Millisecond)

	count := executed.Load()
	expected := numGoroutines * numTasksEach
	if count < int64(expected) {
		t.Errorf("Expected at least %d executions, got: %d", expected, count)
	}

	t.Logf("ScheduleMicrotask concurrent test passed: %d tasks scheduled", count)
}

// TestScheduleMicrotask_EmptyFunction tests scheduling empty functions.
// Priority: LOW - Edge case safety.
func TestScheduleMicrotask_EmptyFunction(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var executed atomic.Bool
	loop.ScheduleMicrotask(func() {
		executed.Store(true)
	})

	go loop.Run(ctx)

	time.Sleep(50 * time.Millisecond)

	if !executed.Load() {
		t.Error("Even empty microtask function should execute")
	}

	t.Log("ScheduleMicrotask empty function test passed")
}

// TestScheduleMicrotask_AfterShutdown tests error handling.
// Priority: HIGH - Shutdown behavior verification.
func TestScheduleMicrotask_AfterShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}

	// Shutdown immediately
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	go func() {
		loop.Run(ctx)
	}()
	time.Sleep(20 * time.Millisecond)

	// Try to schedule microtask after shutdown
	err = loop.ScheduleMicrotask(func() {})
	if err != ErrLoopTerminated {
		t.Errorf("Expected ErrLoopTerminated after shutdown, got: %v", err)
	}

	t.Log("ScheduleMicrotask after shutdown test passed")
}
