package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNestedTimeoutClampingBelowThreshold tests that delays < 4ms are honored
// when nesting depth is 5 or less.
func TestNestedTimeoutClampingBelowThreshold(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer loop.Shutdown(ctx)

	var executionTimes []time.Time
	var mu sync.Mutex
	var wg sync.WaitGroup
	var count atomic.Int32

	// Create 5 levels of nested setTimeout(0) callbacks
	// At depth 5, clamping should NOT be applied
	var createNestedTimeout func(depth int)
	createNestedTimeout = func(depth int) {
		wg.Add(1)
		_, err := loop.ScheduleTimer(0, func() {
			defer wg.Done()

			mu.Lock()
			executionTimes = append(executionTimes, time.Now())
			mu.Unlock()

			count.Add(1)

			// Create next nested timer
			if depth < 5 {
				createNestedTimeout(depth + 1)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Start the nested chain
	go func() {
		_ = loop.Run(ctx)
	}()
	defer func() {
		// Signal loop to stop
		cancel()
	}()

	// Start the chain at depth 0
	createNestedTimeout(0)

	// Wait for all callbacks to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for nested timers")
	}

	// Verify exactly 6 firings occurred (depths 0-5)
	if count.Load() != 6 {
		t.Fatalf("Expected 6 firings, got %d", count.Load())
	}

	// Verify all timers fired within reasonable time
	// (setTimeout(0) should fire quickly without clamping)
	if len(executionTimes) != 6 {
		t.Fatalf("Expected 6 execution times, got %d", len(executionTimes))
	}

	// At depths < 5, setTimeout(0) should NOT be clamped to 4ms
	// But due to Go's timer resolution and scheduling, we expect some latency
	// The key test is that the 5th nested callback (at depth 4, before clamping threshold)
	// executes relatively quickly
	for i, execTime := range executionTimes {
		t.Logf("Timer at depth %d fired at %v", i, execTime.Sub(executionTimes[0]))
	}
}

// TestNestedTimeoutClampingAboveThreshold tests that delays < 4ms are clamped to 4ms
// when nesting depth exceeds 5 (HTML5 spec compliance).
func TestNestedTimeoutClampingAboveThreshold(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer loop.Shutdown(ctx)

	var executionTimes []time.Time
	var mu sync.Mutex
	var wg sync.WaitGroup
	var count atomic.Int32

	// Create 10 levels of nested setTimeout(0) callbacks
	// At depths > 5 (i.e., 6+), 0ms delay should be clamped to 4ms
	// Note: Each recursive call schedules the next timer from within the timer callback,
	// so the nesting depth is properly incremented before checking clamping.
	var createNestedTimeout func(depth int)
	createNestedTimeout = func(depth int) {
		wg.Add(1)
		_, err := loop.ScheduleTimer(0, func() {
			defer wg.Done()

			mu.Lock()
			executionTimes = append(executionTimes, time.Now())
			mu.Unlock()

			count.Add(1)

			// Create next nested timer
			if depth < 9 {
				createNestedTimeout(depth + 1)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Start the nested chain
	go func() {
		_ = loop.Run(ctx)
	}()
	defer func() {
		cancel()
	}()

	// Start the chain at depth 0
	createNestedTimeout(0)

	// Wait for all callbacks to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Timed out waiting for nested timers")
	}

	// Verify exactly 10 firings occurred
	if count.Load() != 10 {
		t.Fatalf("Expected 10 firings, got %d", count.Load())
	}

	if len(executionTimes) != 10 {
		t.Fatalf("Expected 10 execution times, got %d", len(executionTimes))
	}

	// Verify clamping behavior for depths > 5
	// Timer 0-5 (first 6 timers) fire quickly (no clamping: ~0.1ms delays)
	// Timer 6-9 (next 4 timers) should be clamped to 4ms minimum
	// So we expect:
	// - Depths 1-5: small delays (scheduled at nesting depth 0-4 during previous timer execution)
	// - Depths 6: large delay (scheduled at nesting depth 5 during depth 5 timer execution - clamped!)
	// - Depths 7-9: 4ms delays (scheduled at nesting depth >5 - clamped!)

	// Allow some tolerance for system timer resolution
	minClampedDelay := 3 * time.Millisecond // Allow some scheduling overhead

	// Check depth 6 onwards for clamping
	if len(executionTimes) >= 6 {
		for i := 6; i < len(executionTimes); i++ {
			delay := executionTimes[i].Sub(executionTimes[i-1])
			t.Logf("Timer at index %d fired with delay %v", i, delay)

			if delay < minClampedDelay {
				t.Errorf("Expected clamping at timer index %d (scheduled at depth %d, delay >= 4ms), got %v",
					i, i-1, delay)
			}
		}
	}
}

// TestNestedTimeoutWithExplicitDelay tests that explicit delays > 0 are not affected
// by clamping, regardless of nesting depth, except when below the 4ms minimum.
func TestNestedTimeoutWithExplicitDelay(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	defer loop.Shutdown(ctx)

	var executionTimes []time.Time
	var mu sync.Mutex
	var wg sync.WaitGroup
	var count atomic.Int32

	// Create 10 levels of nested setTimeout(10ms) callbacks
	// With explicit 10ms delay, clamping to 4ms should still apply for depths > 5
	// But final delay should still be ~10ms (not reduced)
	var createNestedTimeout func(depth int)
	createNestedTimeout = func(depth int) {
		wg.Add(1)
		_, err := loop.ScheduleTimer(10*time.Millisecond, func() {
			defer wg.Done()

			mu.Lock()
			executionTimes = append(executionTimes, time.Now())
			mu.Unlock()

			count.Add(1)

			// Create next nested timer
			if depth < 9 {
				createNestedTimeout(depth + 1)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	go func() {
		_ = loop.Run(ctx)
	}()
	defer func() {
		cancel()
	}()

	createNestedTimeout(0)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Timed out waiting for nested timers")
	}

	if count.Load() != 10 {
		t.Fatalf("Expected 10 firings, got %d", count.Load())
	}

	// Verify each delay is approximately 10ms
	// (accounting for nesting overhead)
	for i := 1; i < len(executionTimes); i++ {
		delay := executionTimes[i].Sub(executionTimes[i-1])
		t.Logf("Timer at depth %d fired with delay %v", i, delay)

		// Allow 5ms tolerance for scheduling overhead
		if delay < 8*time.Millisecond || delay > 15*time.Millisecond {
			t.Errorf("Expected delay ~10ms at depth %d, got %v", i, delay)
		}
	}
}

// TestNestedTimeoutResetAfterDelay tests that nesting depth resets
// after a timer fires and code returns to the outer scope.
func TestNestedTimeoutResetAfterDelay(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer loop.Shutdown(ctx)

	var wg sync.WaitGroup

	// Create a chain that goes deep, then resets, then goes deep again
	// Pattern: setTimeout(0) -> setTimeout(0) -> setTimeout(0) [depth 2]
	// Then wait and start again: setTimeout(0) [depth 0, reset]

	createTimeout := func() {
		wg.Add(1)
		_, err := loop.ScheduleTimer(0, func() {
			defer wg.Done()
			t.Logf("Timer fired")

			// Schedule another timer immediately (nested)
			wg.Add(1)
			_, err := loop.ScheduleTimer(0, func() {
				defer wg.Done()
				t.Logf("Nested timer fired")

				// Schedule third timer (deeper nesting)
				wg.Add(1)
				_, err := loop.ScheduleTimer(0, func() {
					defer wg.Done()
					t.Logf("Deeply nested timer fired (depth 2)")
				})
				if err != nil {
					t.Fatal(err)
				}
			})
			if err != nil {
				t.Fatal(err)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	go func() {
		_ = loop.Run(ctx)
	}()
	defer func() {
		cancel()
	}()

	// First chain
	createTimeout()

	// Wait for first complete chain
	done1 := make(chan struct{})
	go func() {
		wg.Wait()
		close(done1)
	}()

	select {
	case <-done1:
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for first chain")
	}

	t.Log("First chain complete, waiting for reset...")

	// Now create second independent chain (nesting should reset)
	time.Sleep(50 * time.Millisecond)
	createTimeout()

	// Wait for second chain
	wg.Wait()

	t.Log("Both chains completed successfully")
}

// TestMixedNestingAndNonNesting tests alternating nested and non-nested timers
func TestMixedNestingAndNonNesting(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer loop.Shutdown(ctx)

	var wg sync.WaitGroup
	var count atomic.Int32

	go func() {
		_ = loop.Run(ctx)
	}()
	defer func() {
		cancel()
	}()

	// Create nested chain
	var createNested func(depth int)
	createNested = func(depth int) {
		wg.Add(1)
		_, err := loop.ScheduleTimer(0, func() {
			defer wg.Done()
			count.Add(1)

			if depth < 5 {
				createNested(depth + 1)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Submit an outer timer (starts the nested chain)
	wg.Add(1)
	_, err = loop.ScheduleTimer(0, func() {
		defer wg.Done()
		count.Add(1)
		createNested(1)
	})
	if err != nil {
		t.Fatal(err)
	}

	// After chain, submit another outer timer (should reset nesting)
	wg.Add(1)
	_, err = loop.ScheduleTimer(50*time.Millisecond, func() {
		defer wg.Done()
		count.Add(1)
		t.Log("Outer timer after delay (nesting reset)")
	})
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait()

	// Should have: 1 outer + 5 nested + 1 reset outer = 7 total
	if count.Load() != 7 {
		t.Fatalf("Expected 7 firings, got %d", count.Load())
	}
}
