package eventloop

import (
	"errors"
	"testing"
	"time"
)

// TestPercentileIndex_EdgeCases tests percentileIndex edge cases
func TestPercentileIndex_EdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		n        int
		p        int
		expected int
	}{
		// Normal cases
		{"50th of 100", 100, 50, 50},
		{"25th of 100", 100, 25, 25},
		{"75th of 100", 100, 75, 75},

		// Edge case: p = 100
		{"100th of 100", 100, 100, 99}, // index = 100, n-1 = 99
		{"100th of 50", 50, 100, 49},   // index = 50, n-1 = 49
		{"100th of 10", 10, 100, 9},    // index = 10, n-1 = 9
		{"100th of 1", 1, 100, 0},      // index = 1, n-1 = 0

		// Edge case: p > 100
		{"150th of 100", 100, 150, 99}, // index = 150, n-1 = 99

		// Edge case: p = 0
		{"0th of 100", 100, 0, 0},

		// Edge case: n = 1
		{"50th of 1", 1, 50, 0},

		// Edge case: n = 0 (though likely won't happen in practice)
		{"50th of 0", 0, 50, -1}, // Division by zero protection needed
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := percentileIndex(tc.n, tc.p)
			if result != tc.expected {
				t.Errorf("percentileIndex(%d, %d) = %d, expected %d", tc.n, tc.p, result, tc.expected)
			}
		})
	}
}

// TestPercentileIndex_VariousPercentiles tests various percentile values
func TestPercentileIndex_VariousPercentiles(t *testing.T) {
	n := 100
	for p := 0; p <= 200; p += 10 {
		result := percentileIndex(n, p)
		if result < 0 || result >= n {
			t.Errorf("percentileIndex(%d, %d) = %d, out of bounds [0, %d)", n, p, result, n)
		}
	}
}

// TestPercentileIndex_LargeN tests percentileIndex with large n
func TestPercentileIndex_LargeN(t *testing.T) {
	n := 10000
	for _, p := range []int{1, 10, 25, 50, 75, 90, 99, 100, 101} {
		result := percentileIndex(n, p)
		expected := (p * n) / 100
		if expected >= n {
			expected = n - 1
		}
		if result != expected {
			t.Errorf("percentileIndex(%d, %d) = %d, expected %d", n, p, result, expected)
		}
	}
}

// TestQueueMetrics_Basic tests basic QueueMetrics operations
func TestQueueMetrics_Basic(t *testing.T) {
	q := &QueueMetrics{}

	// Test initial state
	if q.IngressCurrent != 0 {
		t.Errorf("Expected IngressCurrent=0, got: %d", q.IngressCurrent)
	}
	if q.InternalCurrent != 0 {
		t.Errorf("Expected InternalCurrent=0, got: %d", q.InternalCurrent)
	}
	if q.MicrotaskCurrent != 0 {
		t.Errorf("Expected MicrotaskCurrent=0, got: %d", q.MicrotaskCurrent)
	}
}

// TestQueueMetrics_UpdateIngress tests updating ingress metrics
func TestQueueMetrics_UpdateIngress(t *testing.T) {
	q := &QueueMetrics{}

	q.UpdateIngress(5)
	if q.IngressCurrent != 5 {
		t.Errorf("Expected IngressCurrent=5, got: %d", q.IngressCurrent)
	}
	if q.IngressMax != 5 {
		t.Errorf("Expected IngressMax=5, got: %d", q.IngressMax)
	}

	q.UpdateIngress(10)
	if q.IngressCurrent != 10 {
		t.Errorf("Expected IngressCurrent=10, got: %d", q.IngressCurrent)
	}
	if q.IngressMax != 10 {
		t.Errorf("Expected IngressMax=10, got: %d", q.IngressMax)
	}

	q.UpdateIngress(3)
	if q.IngressCurrent != 3 {
		t.Errorf("Expected IngressCurrent=3, got: %d", q.IngressCurrent)
	}
	if q.IngressMax != 10 {
		t.Errorf("Expected IngressMax=10 (unchanged), got: %d", q.IngressMax)
	}
}

// TestQueueMetrics_UpdateInternal tests updating internal metrics
func TestQueueMetrics_UpdateInternal(t *testing.T) {
	q := &QueueMetrics{}

	q.UpdateInternal(7)
	if q.InternalCurrent != 7 {
		t.Errorf("Expected InternalCurrent=7, got: %d", q.InternalCurrent)
	}
	if q.InternalMax != 7 {
		t.Errorf("Expected InternalMax=7, got: %d", q.InternalMax)
	}
}

// TestQueueMetrics_UpdateMicrotask tests updating microtask metrics
func TestQueueMetrics_UpdateMicrotask(t *testing.T) {
	q := &QueueMetrics{}

	q.UpdateMicrotask(12)
	if q.MicrotaskCurrent != 12 {
		t.Errorf("Expected MicrotaskCurrent=12, got: %d", q.MicrotaskCurrent)
	}
	if q.MicrotaskMax != 12 {
		t.Errorf("Expected MicrotaskMax=12, got: %d", q.MicrotaskMax)
	}
}

// Test_tpsCounter_Basic tests basic tpsCounter operations
func Test_tpsCounter_Basic(t *testing.T) {
	m := newTPSCounter(100*time.Millisecond, 10*time.Millisecond)

	// Initial state
	if m.TPS() != 0 {
		t.Errorf("Expected TPS()=0, got: %f", m.TPS())
	}
}

// Test_tpsCounter_AddEvents tests adding events to TPS counter
func Test_tpsCounter_AddEvents(t *testing.T) {
	m := newTPSCounter(100*time.Millisecond, 10*time.Millisecond)

	m.Increment()
	m.Increment()
	m.Increment()

	// TPS should reflect the events
	tps := m.TPS()
	if tps <= 0 {
		t.Errorf("Expected positive TPS, got: %f", tps)
	}
}

// Test_fastState_Basic tests fastState operations
func Test_fastState_Basic(t *testing.T) {
	s := newFastState()

	// newFastState initializes to StateAwake
	if s.Load() != StateAwake {
		t.Errorf("Expected StateAwake, got: %d", s.Load())
	}

	s.Store(StateRunning)
	if s.Load() != StateRunning {
		t.Errorf("Expected StateRunning, got: %d", s.Load())
	}
}

// Test_fastState_TryTransition tests state transitions
func Test_fastState_TryTransition(t *testing.T) {
	s := newFastState()

	// Valid transition from initial StateAwake
	if !s.TryTransition(StateAwake, StateRunning) {
		t.Error("TryTransition(StateAwake, StateRunning) should succeed")
	}
	if s.Load() != StateRunning {
		t.Errorf("Expected StateRunning, got: %d", s.Load())
	}

	// Invalid transition (already StateRunning)
	if s.TryTransition(StateAwake, StateSleeping) {
		t.Error("TryTransition(StateAwake, StateSleeping) should fail (wrong current state)")
	}
	if s.Load() != StateRunning {
		t.Errorf("State should still be StateRunning, got: %d", s.Load())
	}

	// Valid transition
	if !s.TryTransition(StateRunning, StateSleeping) {
		t.Error("TryTransition(StateRunning, StateSleeping) should succeed")
	}
}

// TestState_Constants tests state constants
func TestState_Constants(t *testing.T) {
	// Verify state constants are defined correctly
	if StateAwake != 0 {
		t.Errorf("Expected StateAwake=0, got: %d", StateAwake)
	}
	if StateTerminated != 1 {
		t.Errorf("Expected StateTerminated=1, got: %d", StateTerminated)
	}
	if StateSleeping != 2 {
		t.Errorf("Expected StateSleeping=2, got: %d", StateSleeping)
	}
	if StateRunning != 4 {
		t.Errorf("Expected StateRunning=4, got: %d", StateRunning)
	}
	if StateTerminating != 5 {
		t.Errorf("Expected StateTerminating=5, got: %d", StateTerminating)
	}
}

// TestPanicError tests PanicError type
func TestPanicError(t *testing.T) {
	pe := PanicError{Value: "test panic"}

	if pe.Error() != "promise: goroutine panicked: test panic" {
		t.Errorf("Unexpected error message: %s", pe.Error())
	}

	// Test with different value types
	for _, v := range []any{42, errors.New("error"), nil, []int{1, 2, 3}} {
		pe := PanicError{Value: v}
		if len(pe.Error()) == 0 {
			t.Errorf("PanicError.Error() should not be empty for value: %v", v)
		}
	}
}

// TestErrGoexit tests ErrGoerror variable
func TestErrGoexit(t *testing.T) {
	if ErrGoexit.Error() != "promise: goroutine exited via runtime.Goexit" {
		t.Errorf("Unexpected error message: %s", ErrGoexit.Error())
	}
}

// TestErrPanic tests ErrPanic error variable
func TestErrPanic(t *testing.T) {
	if ErrPanic.Error() != "promise: goroutine panicked" {
		t.Errorf("Unexpected error message: %s", ErrPanic.Error())
	}
}
