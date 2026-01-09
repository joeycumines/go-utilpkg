package eventloop

import (
	"testing"
	"time"
)

func TestPollTimeoutMath(t *testing.T) {
	l, _ := New()

	// Case 1: No timers -> Default 10s
	// (Note: Default is 10s = 10000ms)
	timeout := l.calculateTimeout()
	// Allow for small delays if time.Now() moves, but with no timers it should be maxBlockTime (10s)
	// Actually maxBlockTime is 10s.
	if timeout != 10000 {
		t.Errorf("Expected 10000ms default, got %d", timeout)
	}

	// Case 2: Sub-millisecond rounding (Task 3.1)
	// Add timer for 0.5ms from now
	// We construct it carefully to ensure delta is > 0 and < 1ms
	l.timers = make(timerHeap, 0)
	l.timers = append(l.timers, timer{when: time.Now().Add(500 * time.Microsecond)})

	timeout = l.calculateTimeout()
	if timeout != 1 {
		t.Errorf("Expected 1ms (rounded up from 0.5ms), got %d", timeout)
	}

	// Case 3: Tiny delta (0.5ms) - must be enough to not expire between setup and calculation
	l.timers = make(timerHeap, 0)
	l.timers = append(l.timers, timer{when: time.Now().Add(500 * time.Microsecond)})
	timeout = l.calculateTimeout()
	// Either rounds up to 1ms (correct) or expired to 0 (acceptable for fast machines)
	if timeout != 1 && timeout != 0 {
		t.Errorf("Expected 1ms (rounded up from 0.5ms) or 0 (expired), got %d", timeout)
	}

	// Case 4: Zero or Negative (Expired)
	l.timers = make(timerHeap, 0)
	l.timers = append(l.timers, timer{when: time.Now().Add(-1 * time.Second)})
	timeout = l.calculateTimeout()
	if timeout != 0 {
		t.Errorf("Expected 0ms for expired timer, got %d", timeout)
	}
}

func TestOversleepPrevention(t *testing.T) {
	// Task 3.2: Verify timeout is capped by next timer
	l, _ := New()

	targetDelay := 50 * time.Millisecond
	l.timers = make(timerHeap, 0)
	l.timers = append(l.timers, timer{when: time.Now().Add(targetDelay)})

	timeout := l.calculateTimeout()

	// Should be close to 50ms (e.g., 40-50ms depending on execution speed)
	// But definitively NOT 10000ms (default)
	if timeout > 60 {
		t.Errorf("Timeout %dms is too long (expected ~50ms). Oversleep risk!", timeout)
	}
	if timeout < 0 {
		t.Errorf("Timeout %dms cannot be negative", timeout)
	}
}
