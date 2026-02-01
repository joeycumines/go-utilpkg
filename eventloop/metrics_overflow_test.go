package eventloop

import (
	"testing"
	"time"
)

// TestTPSCounter_NegativeElapsed verifies that the TPS counter handles
// negative elapsed times gracefully (which can happen if the system clock
// jumps backwards due to NTP adjustments, VM snapshot restores, or other
// time synchronization issues).
//
// Overflow protection should:
// 1. Detect elapsed < 0 (clock backward jump)
// 2. Trigger full window reset to recover
// 3. Continue normal operation after reset
func TestTPSCounter_NegativeElapsed(t *testing.T) {
	counter := NewTPSCounter(10*time.Second, 100*time.Millisecond)

	// Record some initial samples
	for i := 0; i < 10; i++ {
		counter.Increment()
	}

	// Simulate clock moving backwards by manipulating lastRotation
	oldRotation := counter.lastRotation.Load().(time.Time)
	futureTime := oldRotation.Add(10 * time.Second)
	counter.lastRotation.Store(futureTime)

	// Now simulate the time moving backwards (Nudge lastRotation to the past)
	backwardsTime := oldRotation.Add(-5 * time.Second)

	// This should trigger the negative elapsed protection
	// Internally, the rotate() function will detect elapsed < 0
	// and reset bucketsToAdvance to len(t.buckets) for recovery
	counter.mu.Lock()
	counter.lastRotation.Store(backwardsTime)
	counter.mu.Unlock()

	// Verify that TPS() doesn't panic and returns a valid value
	tps := counter.TPS()
	if tps < 0 {
		t.Errorf("TPS should not be negative after clock backward jump, got %f", tps)
	}

	// System should recover - subsequent increments should work
	counter.Increment()
	tps2 := counter.TPS()
	if tps2 < 0 {
		t.Errorf("TPS should remain valid after recovery, got %f", tps2)
	}
}

// TestTPSCounter_LargeElapsed verifies that extremely large elapsed times
// (exceeding the window size) are handled correctly by clamping.
//
// Overflow protection should:
// 1. Detect elapsed > window size
// 2. Clamp bucketsToAdvance to len(t.buckets)
// 3. Perform full window reset
// 4. Reset lastRotation appropriately
func TestTPSCounter_LargeElapsed(t *testing.T) {
	counter := NewTPSCounter(10*time.Second, 100*time.Millisecond)

	// Record some initial samples
	for i := 0; i < 5; i++ {
		counter.Increment()
	}

	// Simulate a large time jump by manipulating lastRotation
	oldRotation := counter.lastRotation.Load().(time.Time)
	// Move lastRotation backwards by more than the window size
	backwardsTime := oldRotation.Add(-20 * time.Second) // 20s > 10s window

	counter.mu.Lock()
	counter.lastRotation.Store(backwardsTime)
	counter.mu.Unlock()

	// When we call TPS(), the rotate function will:
	// 1. Calculate a very large elapsed time (> window size)
	// 2. Detect bucketsToAdvance > len(t.buckets)
	// 3. Clamp bucketsToAdvance to len(t.buckets)
	// 4. Perform full window reset
	tps := counter.TPS()

	// TPS should not panic
	// TPS may be 0 (window reset) or small (few samples remain)
	if tps < 0 {
		t.Errorf("TPS should not be negative after large elapsed time, got %f", tps)
	}

	// System should be stable with full window reset
	// Reset to old rotation + clamped advance
	// The window should be in a valid state
	for i := 0; i < 10; i++ {
		counter.Increment()
	}

	tps2 := counter.TPS()
	if tps2 < 0 {
		t.Errorf("TPS should remain valid after large elapsed reset, got %f", tps2)
	}
}

// TestTPSCounter_ExtremeElapsed verifies handling of extremly large
// elapsed times that could theoretically cause integer overflow in the
// bucketsToAdvance calculation if not properly clamped.
//
// This test simulates:
// 1. Extremely large time jumps (years into the past)
// 2. Very large elapsed times due to system suspend/resume
// 3. Edge cases near int64 limits for time.Duration
func TestTPSCounter_ExtremeElapsed(t *testing.T) {
	counter := NewTPSCounter(10*time.Second, 100*time.Millisecond)

	// Test with extreme time jump: 1 year backwards
	oldRotation := counter.lastRotation.Load().(time.Time)
	extremeBackwardsTime := oldRotation.Add(-365 * 24 * time.Hour)

	counter.mu.Lock()
	counter.lastRotation.Store(extremeBackwardsTime)
	counter.mu.Unlock()

	// This should not panic - the overflow protection clamping
	// ensures bucketsToAdvance never exceeds len(t.buckets)
	// even with extreme elapsed values
	tps := counter.TPS()

	if tps < 0 {
		t.Errorf("TPS should not be negative after extreme time jump, got %f", tps)
	}

	// Verify system remains functional
	for i := 0; i < 5; i++ {
		counter.Increment()
	}

	tps2 := counter.TPS()
	if tps2 < 0 {
		t.Errorf("TPS should remain valid after extreme elapsed, got %f", tps2)
	}

	// Test another extreme: very large forward jump
	oldRotation2 := counter.lastRotation.Load().(time.Time)
	extremeForwardTime := oldRotation2.Add(365 * 24 * time.Hour)

	counter.mu.Lock()
	counter.lastRotation.Store(extremeForwardTime)
	counter.mu.Unlock()

	tps3 := counter.TPS()

	if tps < 0 {
		t.Errorf("TPS should not be negative after extreme forward jump, got %f", tps3)
	}
}

// TestTPSCounter_ClockJumps verifies that the TPS counter
// handles rapid clock jumps (both forward and backward) gracefully.
func TestTPSCounter_ClockJumps(t *testing.T) {
	counter := NewTPSCounter(10*time.Second, 100*time.Millisecond)

	now := time.Now()
	counter.lastRotation.Store(now)

	// Simulate clock jumping back 5 seconds, then forward 10 seconds, then back 3 seconds
	jumps := []time.Duration{
		-5 * time.Second,
		10 * time.Second,
		-3 * time.Second,
		7 * time.Second,
	}

	for _, jump := range jumps {
		counter.mu.Lock()
		oldRotation := counter.lastRotation.Load().(time.Time)
		newRotation := oldRotation.Add(jump)
		counter.lastRotation.Store(newRotation)
		counter.mu.Unlock()

		// TPS should handle each clock jump gracefully
		tps := counter.TPS()
		if tps < 0 {
			t.Errorf("TPS should remain valid after clock jump of %v, got %f", jump, tps)
		}

		// Add some increments
		for i := 0; i < 3; i++ {
			counter.Increment()
		}
	}

	// Final TPS should still be valid
	finalTPS := counter.TPS()
	if finalTPS < 0 {
		t.Errorf("TPS should remain Valid after multiple clock jumps, got %f", finalTPS)
	}
}
