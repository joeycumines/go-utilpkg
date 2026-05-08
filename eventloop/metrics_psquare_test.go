package eventloop

import (
	"math"
	"math/rand"
	"sort"
	"testing"
	"time"
)

// =============================================================================
// P-Square Algorithm Unit Tests
// =============================================================================

// TestPSquareQuantile_Basic tests basic P-Square functionality.
func TestPSquareQuantile_Basic(t *testing.T) {
	ps50 := newPSquareQuantile(0.5)

	// Add enough observations to initialize
	observations := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, v := range observations {
		ps50.Update(v)
	}

	q := ps50.Quantile()
	// P50 of 1-10 should be around 5-6
	if q < 4 || q > 7 {
		t.Errorf("P50 of 1-10 should be around 5-6, got %.2f", q)
	}

	if ps50.Count() != 10 {
		t.Errorf("Count should be 10, got %d", ps50.Count())
	}

	if ps50.Max() != 10 {
		t.Errorf("Max should be 10, got %.2f", ps50.Max())
	}
}

func TestPSquareQuantileInitialization(t *testing.T) {
	ps := newPSquareQuantile(0.5)

	if ps.Count() != 0 || ps.Quantile() != 0 || ps.Max() != 0 {
		t.Fatalf("initial estimator = (count=%d, quantile=%v, max=%v), want all zero", ps.Count(), ps.Quantile(), ps.Max())
	}

	ps.Update(100)
	if ps.Count() != 1 || ps.Quantile() != 100 || ps.Max() != 100 {
		t.Fatalf("one-observation estimator = (count=%d, quantile=%v, max=%v), want (1, 100, 100)", ps.Count(), ps.Quantile(), ps.Max())
	}

	ps.Update(200)
	ps.Update(300)
	if ps.Count() != 3 || ps.Quantile() != 200 || ps.Max() != 300 {
		t.Fatalf("three-observation estimator = (count=%d, quantile=%v, max=%v), want (3, 200, 300)", ps.Count(), ps.Quantile(), ps.Max())
	}

	ps.Update(400)
	ps.Update(500)
	if ps.Count() != 5 || ps.Quantile() != 300 || ps.Max() != 500 {
		t.Fatalf("initialized estimator = (count=%d, quantile=%v, max=%v), want (5, 300, 500)", ps.Count(), ps.Quantile(), ps.Max())
	}
	ps.Update(50)
	if ps.q[0] != 50 || ps.Max() != 500 {
		t.Fatalf("late minimum markers = (min=%v, max=%v), want (50, 500)", ps.q[0], ps.Max())
	}
}

// TestPSquareMultiQuantile tests multi-quantile tracking.
func TestPSquareMultiQuantile(t *testing.T) {
	// Track P50, P90, P95, P99
	mq := newPSquareMultiQuantile(0.50, 0.90, 0.95, 0.99)

	// Add 100 observations (1-100)
	for i := 1; i <= 100; i++ {
		mq.Update(float64(i))
	}

	p50 := mq.Quantile(0)
	p90 := mq.Quantile(1)
	p95 := mq.Quantile(2)
	p99 := mq.Quantile(3)

	// Verify percentiles are in order
	if p50 > p90 || p90 > p95 || p95 > p99 {
		t.Errorf("Percentiles out of order: P50=%.2f, P90=%.2f, P95=%.2f, P99=%.2f",
			p50, p90, p95, p99)
	}

	// P50 should be around 50
	if p50 < 40 || p50 > 60 {
		t.Errorf("P50 should be around 50, got %.2f", p50)
	}

	// P99 should be around 99
	if p99 < 90 || p99 > 100 {
		t.Errorf("P99 should be around 99, got %.2f", p99)
	}

}

// TestPSquareQuantile_Accuracy tests P-Square accuracy against exact percentiles.
func TestPSquareQuantile_Accuracy(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	// Test with different distributions
	testCases := []struct {
		name     string
		generate func() float64
	}{
		{"uniform", func() float64 { return rng.Float64() * 1000 }},
		{"exponential", func() float64 { return rng.ExpFloat64() * 100 }},
		{"normal", func() float64 { return rng.NormFloat64()*50 + 200 }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ps99 := newPSquareQuantile(0.99)
			exact := make([]float64, 10000)

			for i := range 10000 {
				v := tc.generate()
				exact[i] = v
				ps99.Update(v)
			}

			// Compute exact P99
			sort.Float64s(exact)
			exactP99 := exact[9900]

			estimatedP99 := ps99.Quantile()

			// Allow 5% relative error for P99 estimation
			relError := (estimatedP99 - exactP99) / exactP99
			if relError < -0.05 || relError > 0.05 {
				t.Errorf("%s: P99 relative error %.2f%% exceeds 5%% (exact=%.2f, estimated=%.2f)",
					tc.name, relError*100, exactP99, estimatedP99)
			}

		})
	}
}

// TestLatencyMetrics_PSquareIntegration tests latencyTestSampler with P-Square.
func TestLatencyMetrics_PSquareIntegration(t *testing.T) {
	lm := &latencyTestSampler{}

	// Record enough samples to trigger P-Square path
	for i := 1; i <= 100; i++ {
		lm.Record(time.Duration(i) * time.Millisecond)
	}

	count := lm.Sample()
	if count != 100 {
		t.Errorf("Sample count should be 100, got %d", count)
	}

	// Verify percentiles are populated and in order
	if lm.P50 <= 0 {
		t.Error("P50 should be > 0")
	}
	if lm.P90 <= lm.P50 {
		t.Errorf("P90 (%v) should be > P50 (%v)", lm.P90, lm.P50)
	}
	if lm.P95 < lm.P90 {
		t.Errorf("P95 (%v) should be >= P90 (%v)", lm.P95, lm.P90)
	}
	if lm.P99 < lm.P95 {
		t.Errorf("P99 (%v) should be >= P95 (%v)", lm.P99, lm.P95)
	}
	if lm.Max < lm.P99 {
		t.Errorf("Max (%v) should be >= P99 (%v)", lm.Max, lm.P99)
	}

	// Max should be 100ms
	if lm.Max != 100*time.Millisecond {
		t.Errorf("Max should be 100ms, got %v", lm.Max)
	}

}

// TestLatencyMetrics_SmallSamplesFallback tests that small samples use exact sorting.
func TestLatencyMetrics_SmallSamplesFallback(t *testing.T) {
	lm := &latencyTestSampler{}

	// Record < 5 samples (should use sorting path)
	lm.Record(10 * time.Millisecond)
	lm.Record(20 * time.Millisecond)
	lm.Record(30 * time.Millisecond)

	count := lm.Sample()
	if count != 3 {
		t.Errorf("Sample count should be 3, got %d", count)
	}

	// With 3 samples, percentiles should be exact
	// P50 of [10, 20, 30] at index (3*50/100)=1 is 20ms
	if lm.P50 != 20*time.Millisecond {
		t.Errorf("P50 should be 20ms (exact), got %v", lm.P50)
	}

	// Max should be 30ms
	if lm.Max != 30*time.Millisecond {
		t.Errorf("Max should be 30ms, got %v", lm.Max)
	}
}

// TestPSquareQuantile_EdgeCases tests edge cases.
func TestPSquareQuantile_EdgeCases(t *testing.T) {
	// Test extreme percentiles
	ps0 := newPSquareQuantile(0)
	ps100 := newPSquareQuantile(1)

	for i := 1; i <= 100; i++ {
		ps0.Update(float64(i))
		ps100.Update(float64(i))
	}

	// P0 should be minimum (1)
	if ps0.Quantile() < 1 || ps0.Quantile() > 10 {
		t.Errorf("P0 should be near minimum (1), got %.2f", ps0.Quantile())
	}

	// P100 should be maximum (100)
	if ps100.Quantile() < 90 || ps100.Quantile() > 100 {
		t.Errorf("P100 should be near maximum (100), got %.2f", ps100.Quantile())
	}
}

// TestPSquareMultiQuantile_InvalidIndex tests out-of-bounds access.
func TestPSquareMultiQuantile_InvalidIndex(t *testing.T) {
	mq := newPSquareMultiQuantile(0.5, 0.99)

	mq.Update(100)

	// Invalid indices should return 0
	if mq.Quantile(-1) != 0 {
		t.Error("Quantile(-1) should return 0")
	}
	if mq.Quantile(5) != 0 {
		t.Error("Quantile(5) should return 0")
	}
}

// TestPSquareQuantile_NegativePercentile tests handling of invalid percentiles.
func TestPSquareQuantile_NegativePercentile(t *testing.T) {
	// Should clamp to 0
	ps := newPSquareQuantile(-0.5)
	if ps.p != 0 {
		t.Errorf("Negative percentile should clamp to 0, got %.2f", ps.p)
	}

	// Should clamp to 1
	ps = newPSquareQuantile(1.5)
	if ps.p != 1 {
		t.Errorf("Percentile > 1 should clamp to 1, got %.2f", ps.p)
	}
}

func TestPSquarePositionAdvancesBeyondFloatIntegralPrecision(t *testing.T) {
	position := newPSquarePosition(float64(uint64(1) << 53))
	increment := newPSquarePosition(0.5)
	position.add(increment)
	position.add(increment)
	if position.whole != uint64(1)<<53+1 || position.fraction != 0 {
		t.Fatalf("fixed marker position did not advance: %+v", position)
	}
}

func TestPSquareQuantileCountExceedsLegacyInt32(t *testing.T) {
	quantile := newPSquareQuantile(0.5)
	for i := range 5 {
		quantile.Update(float64(i))
	}
	quantile.count = uint64(math.MaxInt32)
	quantile.Update(5)
	if quantile.Count() != uint64(math.MaxInt32)+1 {
		t.Fatalf("P-Square count = %d, want %d", quantile.Count(), uint64(math.MaxInt32)+1)
	}
}
