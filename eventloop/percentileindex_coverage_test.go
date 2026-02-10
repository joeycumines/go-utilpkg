package eventloop

import (
	"testing"
	"time"
)

// =============================================================================
// COVERAGE-007: percentileIndex Function 100% Coverage
// =============================================================================
// Target: metrics.go percentileIndex function
// Gaps covered:
// - Percentile calculation edge case where result index >= n (when p=100 or n=1)
// - Boundary conditions for small sample sizes
// - Various percentile values ensuring index clamping works correctly
// =============================================================================

// TestPercentileIndex_IndexGeN tests the branch where index >= n.
// This occurs when p=100 (any n) or when p*n/100 >= n.
func TestPercentileIndex_IndexGeN(t *testing.T) {
	testCases := []struct {
		name     string
		n        int
		p        int
		expected int
	}{
		// p=100 always produces index=n, which should clamp to n-1
		{"p=100, n=1", 1, 100, 0},
		{"p=100, n=2", 2, 100, 1},
		{"p=100, n=10", 10, 100, 9},
		{"p=100, n=100", 100, 100, 99},
		{"p=100, n=1000", 1000, 100, 999},

		// p>100 edge cases (should clamp to n-1)
		{"p=101, n=100", 100, 101, 99},
		{"p=200, n=100", 100, 200, 99},
		{"p=1000, n=100", 100, 1000, 99},

		// n=1 always produces index clamped to 0
		{"n=1, p=0", 1, 0, 0},
		{"n=1, p=50", 1, 50, 0},
		{"n=1, p=99", 1, 99, 0},
		{"n=1, p=100", 1, 100, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := percentileIndex(tc.n, tc.p)
			if result != tc.expected {
				t.Errorf("percentileIndex(%d, %d) = %d, expected %d",
					tc.n, tc.p, result, tc.expected)
			}
			// Verify result is always valid index
			if tc.n > 0 && (result < 0 || result >= tc.n) {
				t.Errorf("percentileIndex(%d, %d) = %d, out of valid range [0, %d)",
					tc.n, tc.p, result, tc.n)
			}
		})
	}
}

// TestPercentileIndex_SmallSampleSizes tests boundary conditions
// for very small sample sizes.
func TestPercentileIndex_SmallSampleSizes(t *testing.T) {
	// n=2: only indices 0 and 1 are valid
	for p := 0; p <= 100; p++ {
		result := percentileIndex(2, p)
		if result < 0 || result > 1 {
			t.Errorf("percentileIndex(2, %d) = %d, expected 0 or 1", p, result)
		}
	}

	// n=3: indices 0, 1, 2 are valid
	for p := 0; p <= 100; p++ {
		result := percentileIndex(3, p)
		if result < 0 || result > 2 {
			t.Errorf("percentileIndex(3, %d) = %d, expected 0, 1, or 2", p, result)
		}
	}

	// n=5: verify distribution
	results := make(map[int]bool)
	for p := 0; p <= 100; p++ {
		results[percentileIndex(5, p)] = true
	}
	if len(results) == 0 {
		t.Error("Expected at least one distinct result for n=5")
	}
	t.Logf("n=5 produced %d distinct indices: %v", len(results), results)
}

// TestPercentileIndex_BoundaryConditions tests at exact boundaries.
func TestPercentileIndex_BoundaryConditions(t *testing.T) {
	// Test at exact percentile boundaries for n=100
	// These test the == case in index >= n
	boundaries := []struct {
		p        int
		shouldBe int
	}{
		{0, 0},    // 0 * 100 / 100 = 0
		{1, 1},    // 1 * 100 / 100 = 1
		{50, 50},  // 50 * 100 / 100 = 50
		{99, 99},  // 99 * 100 / 100 = 99
		{100, 99}, // 100 * 100 / 100 = 100 → clamp to 99
	}

	for _, b := range boundaries {
		result := percentileIndex(100, b.p)
		if result != b.shouldBe {
			t.Errorf("percentileIndex(100, %d) = %d, expected %d",
				b.p, result, b.shouldBe)
		}
	}
}

// TestPercentileIndex_WithLatencyMetrics tests percentileIndex via
// LatencyMetrics.Sample() to ensure real-world usage is covered.
func TestPercentileIndex_WithLatencyMetrics(t *testing.T) {
	lm := &LatencyMetrics{}

	// Test with exactly 1 sample (exercises n=1 paths)
	lm.Record(10 * time.Millisecond)
	count := lm.Sample()
	if count != 1 {
		t.Errorf("Expected 1 sample, got %d", count)
	}

	// All percentiles should return the same value (only 1 sample)
	if lm.P50 != lm.P90 || lm.P90 != lm.P95 || lm.P95 != lm.P99 {
		t.Errorf("With 1 sample, all percentiles should be equal: P50=%v, P90=%v, P95=%v, P99=%v",
			lm.P50, lm.P90, lm.P95, lm.P99)
	}

	// Test with exactly 2 samples
	lm2 := &LatencyMetrics{}
	lm2.Record(5 * time.Millisecond)
	lm2.Record(15 * time.Millisecond)
	lm2.Sample()

	// P50 should be first value, P99 should be second value
	// (indices: 50*2/100=1 → clamp not needed, but 100*2/100=2 → clamp to 1)
	t.Logf("2 samples: P50=%v, P99=%v", lm2.P50, lm2.P99)

	// Test with exactly 100 samples for clean percentile boundaries
	lm100 := &LatencyMetrics{}
	for i := 1; i <= 100; i++ {
		lm100.Record(time.Duration(i) * time.Millisecond)
	}
	lm100.Sample()

	// Verify percentiles are in order
	if lm100.P50 > lm100.P90 || lm100.P90 > lm100.P95 || lm100.P95 > lm100.P99 || lm100.P99 > lm100.Max {
		t.Errorf("Percentiles not in order: P50=%v, P90=%v, P95=%v, P99=%v, Max=%v",
			lm100.P50, lm100.P90, lm100.P95, lm100.P99, lm100.Max)
	}

	t.Logf("100 samples: P50=%v, P90=%v, P95=%v, P99=%v, Max=%v",
		lm100.P50, lm100.P90, lm100.P95, lm100.P99, lm100.Max)
}

// TestPercentileIndex_ZeroCases tests edge cases with zero values.
func TestPercentileIndex_ZeroCases(t *testing.T) {
	// p=0 should always return 0
	for n := 1; n <= 100; n++ {
		result := percentileIndex(n, 0)
		if result != 0 {
			t.Errorf("percentileIndex(%d, 0) = %d, expected 0", n, result)
		}
	}
}

// TestPercentileIndex_LargeNAndP tests with large values to ensure
// no overflow issues.
func TestPercentileIndex_LargeNAndP(t *testing.T) {
	testCases := []struct {
		n        int
		p        int
		expected int
	}{
		{10000, 100, 9999},
		{10000, 99, 9900},
		{10000, 50, 5000},
		{10000, 1, 100},
		// Edge case: large p values
		{1000, 1000, 999}, // p*n/100 = 10000 >= n=1000
		{100, 10000, 99},  // p*n/100 = 10000 >= n=100
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			result := percentileIndex(tc.n, tc.p)
			if result != tc.expected {
				t.Errorf("percentileIndex(%d, %d) = %d, expected %d",
					tc.n, tc.p, result, tc.expected)
			}
		})
	}
}

// TestPercentileIndex_ConsecutivePercentiles tests that consecutive
// percentiles produce monotonically non-decreasing indices.
func TestPercentileIndex_ConsecutivePercentiles(t *testing.T) {
	for n := 1; n <= 100; n += 10 {
		prevIndex := -1
		for p := 0; p <= 100; p++ {
			index := percentileIndex(n, p)
			if index < prevIndex {
				t.Errorf("n=%d: percentileIndex decreased from p=%d (index=%d) to p=%d (index=%d)",
					n, p-1, prevIndex, p, index)
			}
			prevIndex = index
		}
	}
	t.Log("Consecutive percentiles produce monotonically non-decreasing indices")
}

// TestPercentileIndex_ExactClampBoundary tests the exact boundary
// where clamping kicks in.
func TestPercentileIndex_ExactClampBoundary(t *testing.T) {
	// For n=100, p=100: index = 100*100/100 = 100, which == n, so clamp to 99
	// For n=100, p=99:  index = 99*100/100 = 99, which < n, so no clamp
	p99 := percentileIndex(100, 99)
	p100 := percentileIndex(100, 100)

	if p99 != 99 {
		t.Errorf("percentileIndex(100, 99) = %d, expected 99", p99)
	}
	if p100 != 99 {
		t.Errorf("percentileIndex(100, 100) = %d, expected 99 (clamped)", p100)
	}

	// For n=50:
	// p=100: index = 100*50/100 = 50, which == n, clamp to 49
	// p=98:  index = 98*50/100 = 49, no clamp
	p98_50 := percentileIndex(50, 98)
	p100_50 := percentileIndex(50, 100)

	if p98_50 != 49 {
		t.Errorf("percentileIndex(50, 98) = %d, expected 49", p98_50)
	}
	if p100_50 != 49 {
		t.Errorf("percentileIndex(50, 100) = %d, expected 49 (clamped)", p100_50)
	}

	t.Log("Exact clamp boundary verified")
}
