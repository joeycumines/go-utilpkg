package eventloop

import (
	"testing"
	"time"
)

// Test_tpsCounter_NegativeElapsed verifies that the TPS counter handles
// negative elapsed times gracefully (which can happen if the system clock
// jumps backwards due to NTP adjustments, VM snapshot restores, or other
// time synchronization issues).
//
// This test directly manipulates the lastRotation timestamp to simulate
// a clock backward jump by setting it backwards relative to the current time.
// Overflow protection should:
// 1. Detect elapsed < 0 (clock backward jump) in the rotate() function
// 2. Trigger full window reset to recover
// 3. Continue normal operation after reset
// 4. Ensure TPS() returns non-negative values even after clock anomalies
func Test_tpsCounter_NegativeElapsed(t *testing.T) {
	counter := newTPSCounter(10*time.Second, 100*time.Millisecond)

	// Record some initial samples
	for i := 0; i < 10; i++ {
		counter.Increment()
	}

	// Simulate a clock rollback by setting lastRotation to a time in the FUTURE
	// relative to now. This makes now.Sub(lastRotation) negative and simulates
	// a clock jump backwards (NTP, VM restore, etc).
	futureTime := time.Now().Add(5 * time.Second)

	counter.mu.Lock()
	counter.lastRotation.Store(futureTime)
	counter.mu.Unlock()

	// Calling TPS() should trigger rotate() which detects negative elapsed and performs
	// a full window reset (all buckets zeroed and lastRotation synchronized to now).
	tps := counter.TPS()
	if tps != 0 {
		t.Errorf("Expected TPS to be 0 after negative elapsed full reset, got %f", tps)
	}

	// Ensure buckets were reset to zero
	counter.mu.Lock()
	sum := int64(0)
	for _, v := range counter.buckets {
		sum += v
	}
	counter.mu.Unlock()
	if sum != 0 {
		t.Fatalf("Expected buckets sum to be 0 after full reset, got %d", sum)
	}

	// Ensure lastRotation synchronized to now (within 1 second tolerance)
	lastRotation := counter.lastRotation.Load().(time.Time)
	if delta := time.Since(lastRotation); delta < 0 || delta > time.Second {
		t.Fatalf("lastRotation not synchronized after reset: lastRotation=%v, now=%v, delta=%v", lastRotation, time.Now(), delta)
	}

	// System should recover - subsequent increments should work
	counter.Increment()
	tps2 := counter.TPS()
	if tps2 <= 0 {
		t.Errorf("TPS should be positive after recovery increment, got %f", tps2)
	}
}

// Test_tpsCounter_LargeElapsed verifies that extremely large elapsed times
// (exceeding the window size) are handled correctly by clamping.
//
// Overflow protection should:
// 1. Detect elapsed > window size
// 2. Clamp bucketsToAdvance to len(t.buckets)
// 3. Perform full window reset
// 4. Reset lastRotation appropriately
func Test_tpsCounter_LargeElapsed(t *testing.T) {
	counter := newTPSCounter(10*time.Second, 100*time.Millisecond)

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

// Test_tpsCounter_ExtremeElapsed verifies handling of extremly large
// elapsed times that could theoretically cause integer overflow in the
// bucketsToAdvance calculation if not properly clamped.
//
// This test simulates:
// 1. Extremely large time jumps (years into the past)
// 2. Very large elapsed times due to system suspend/resume
// 3. Edge cases near int64 limits for time.Duration
func Test_tpsCounter_ExtremeElapsed(t *testing.T) {
	counter := newTPSCounter(10*time.Second, 100*time.Millisecond)

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

	if tps3 < 0 {
		t.Errorf("TPS should not be negative after extreme forward jump, got %f", tps3)
	}
}

// Test_tpsCounter_ClockJumps verifies that the TPS counter
// handles rapid clock jumps (both forward and backward) gracefully.
func Test_tpsCounter_ClockJumps(t *testing.T) {
	counter := newTPSCounter(10*time.Second, 100*time.Millisecond)

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

// Test_tpsCounter_TimeSynchronizationAfterLongPause verifies that after
// a long pause exceeding the rolling window duration, the internal
// time tracking (lastRotation) is synchronized to the current actual time
// rather than permanently lagging behind.
//
// This tests RV09 fix: Before the fix, if bucketsToAdvance > len(buckets),
// the code would only advance lastRotation by len(buckets)*bucketSize (one
// window), even if the actual elapsed time was much longer. This caused
// lastRotation to permanently lag behind real time, leading to incorrect TPS
// calculations and data loss.
//
// After the fix: If bucketsToAdvance >= len(buckets), we reset all
// buckets to 0 AND set lastRotation = time.Now(), ensuring the counter
// is fully synchronized with wall clock time.
func Test_tpsCounter_TimeSynchronizationAfterLongPause(t *testing.T) {
	// Create a 10-second window with 100ms buckets (100 buckets)
	windowSize := 10 * time.Second
	bucketSize := 100 * time.Millisecond
	counter := newTPSCounter(windowSize, bucketSize)

	// Add some initial events to prove counter is working
	for i := 0; i < 50; i++ {
		counter.Increment()
	}

	// Verify initial TPS is non-zero
	initialTPS := counter.TPS()
	if initialTPS <= 0 {
		t.Errorf("Expected non-zero TPS after initial events, got %f", initialTPS)
	}

	// Simulate a very long pause: 5 minutes (30x the window size)
	// This is the critical scenario for RV09 time synchronization defect
	t.Logf("Simulating long pause of 5 minutes...")
	longPause := 5 * time.Minute

	// Manually set lastRotation far in the past to simulate pause
	oldTime := time.Now().Add(-longPause)
	counter.mu.Lock()
	counter.lastRotation.Store(oldTime)
	// Also corrupt buckets to prove they get reset
	for i := range counter.buckets {
		counter.buckets[i] = 999 + int64(i)
	}
	counter.mu.Unlock()

	// Calling TPS() will trigger rotate() which should detect
	// that bucketsToAdvance >= len(buckets) and perform full reset
	tpsAfterPause := counter.TPS()

	// Capture time after sync
	timeAfterSync := time.Now()

	// Verify that after a very long pause exceeding window size:
	// 1. TPS is not negative
	if tpsAfterPause < 0 {
		t.Errorf("TPS should not be negative after long pause, got %f", tpsAfterPause)
	}

	// 2. All buckets were reset to 0 (not the corrupted values from before)
	// We need to lock to inspect bucket state
	counter.mu.Lock()
	bucketSum := int64(0)
	allZero := true
	for i, val := range counter.buckets {
		bucketSum += val
		if val != 0 {
			allZero = false
			t.Logf("Bucket %d has value %d (should be 0 after reset)", i, val)
		}
	}
	counter.mu.Unlock()

	if !allZero {
		t.Errorf("Expected all buckets to be reset to 0 after long pause, sum=%d", bucketSum)
	}

	// 3. Last rotation time is synchronized to current time (not lagging)
	// The key RV09 fix: lastRotation should be set to time.Now()
	// during the reset, not advanced by only one window
	lastRotationTime := counter.lastRotation.Load().(time.Time)
	timeSinceSync := timeAfterSync.Sub(lastRotationTime)

	// lastRotation should be close to timeAfterSync (within 1 second tolerance)
	// If the old bug existed, lastRotation would still be ~5 minutes in the past
	if timeSinceSync < 0 || timeSinceSync > time.Second {
		t.Errorf("lastRotation time not synchronized to current time after long pause: "+
			"lastRotation=%v, timeAfterSync=%v, timeSinceSync=%v (should be <1s)",
			lastRotationTime, timeAfterSync, timeSinceSync)
	}

	// 4. Counter continues to work correctly after sync
	// Add events after the long pause
	for i := 0; i < 20; i++ {
		counter.Increment()
	}

	tpsAfterResume := counter.TPS()

	// TPS should be reasonable now (not zero, not negative)
	if tpsAfterResume < 0 {
		t.Errorf("TPS should be valid after resume, got %f", tpsAfterResume)
	}

	t.Logf("Before sync: lastRotation would have lagged by ~5 minutes (bug)")
	t.Logf("After sync: lastRotation synchronized to Now(), timeSinceSync=%v", timeSinceSync)
	t.Logf("TPS initial=%f, after pause=%f, after resume=%f",
		initialTPS, tpsAfterPause, tpsAfterResume)
}
