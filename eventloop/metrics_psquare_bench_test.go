// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"math/rand"
	"sort"
	"testing"
	"time"
)

// =============================================================================
// PERF-001: TPS Metrics Percentile Computation Optimization
// =============================================================================
// Benchmarks comparing old O(n log n) sort-based approach vs new O(1) P-Square
// algorithm for streaming percentile computation.
// =============================================================================

// BenchmarkLatencySample_OldSortBased simulates the old sorting-based approach.
// This allocates a slice and sorts it every time.
func BenchmarkLatencySample_OldSortBased(b *testing.B) {
	benchmarkLatencySample(b, false)
}

// BenchmarkLatencySample_NewPSquare uses the new P-Square algorithm.
// Percentiles are computed incrementally during Record(), Sample() is O(1).
func BenchmarkLatencySample_NewPSquare(b *testing.B) {
	benchmarkLatencySample(b, true)
}

func benchmarkLatencySample(b *testing.B, usePSquare bool) {
	lm := &LatencyMetrics{}

	// Pre-fill with 1000 samples to simulate steady-state
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < sampleSize; i++ {
		// Realistic latency distribution: 1-100ms with occasional outliers
		duration := time.Duration(rng.ExpFloat64()*10+1) * time.Millisecond
		lm.Record(duration)
	}

	if usePSquare {
		// Ensure P-Square is initialized
		lm.Sample()
	}

	b.ResetTimer()
	b.ReportAllocs()

	if usePSquare {
		// New approach: Sample() is O(1) as percentiles are pre-computed
		for i := 0; i < b.N; i++ {
			lm.Sample()
		}
	} else {
		// Simulate old approach: sort every call
		for i := 0; i < b.N; i++ {
			_ = sampleWithSort(lm)
		}
	}
}

// sampleWithSort simulates the old sorting-based Sample() implementation.
func sampleWithSort(l *LatencyMetrics) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	count := l.sampleCount
	if count == 0 {
		return 0
	}

	// Clone and sort samples (O(n log n))
	sorted := make([]time.Duration, count)
	copy(sorted, l.samples[:count])
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	// Compute percentiles
	_ = sorted[percentileIndex(count, 50)]
	_ = sorted[percentileIndex(count, 90)]
	_ = sorted[percentileIndex(count, 95)]
	_ = sorted[percentileIndex(count, 99)]
	_ = sorted[count-1] // Max

	return count
}

// BenchmarkLatencyRecord_WithPSquare benchmarks the Record() overhead with P-Square.
func BenchmarkLatencyRecord_WithPSquare(b *testing.B) {
	lm := &LatencyMetrics{}

	// Pre-initialize P-Square
	lm.Record(time.Millisecond)
	lm.Sample()

	rng := rand.New(rand.NewSource(42))
	durations := make([]time.Duration, b.N)
	for i := range durations {
		durations[i] = time.Duration(rng.ExpFloat64()*10+1) * time.Millisecond
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		lm.Record(durations[i])
	}
}

// BenchmarkLatencyRecord_WithoutPSquare benchmarks Record() without P-Square updates.
func BenchmarkLatencyRecord_WithoutPSquare(b *testing.B) {
	lm := &LatencyMetrics{}

	rng := rand.New(rand.NewSource(42))
	durations := make([]time.Duration, b.N)
	for i := range durations {
		durations[i] = time.Duration(rng.ExpFloat64()*10+1) * time.Millisecond
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Simulate old Record() without P-Square
	for i := 0; i < b.N; i++ {
		recordWithoutPSquare(lm, durations[i])
	}
}

// recordWithoutPSquare simulates the old Record() without P-Square.
func recordWithoutPSquare(l *LatencyMetrics, duration time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.sampleCount >= sampleSize {
		old := l.samples[l.sampleIdx]
		l.Sum -= old
	}

	l.samples[l.sampleIdx] = duration
	l.Sum += duration
	l.sampleIdx++
	if l.sampleIdx >= sampleSize {
		l.sampleIdx = 0
	}
	if l.sampleCount < sampleSize {
		l.sampleCount++
	}
}

// BenchmarkCombinedWorkload_Old simulates realistic workload with old approach.
// Pattern: Many records, periodic Sample() calls.
func BenchmarkCombinedWorkload_Old(b *testing.B) {
	benchmarkCombinedWorkload(b, false)
}

// BenchmarkCombinedWorkload_New simulates realistic workload with P-Square.
func BenchmarkCombinedWorkload_New(b *testing.B) {
	benchmarkCombinedWorkload(b, true)
}

func benchmarkCombinedWorkload(b *testing.B, usePSquare bool) {
	lm := &LatencyMetrics{}

	rng := rand.New(rand.NewSource(42))
	sampleInterval := 100 // Sample every 100 records

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		duration := time.Duration(rng.ExpFloat64()*10+1) * time.Millisecond

		if usePSquare {
			lm.Record(duration)
			if i%sampleInterval == 0 {
				lm.Sample()
			}
		} else {
			recordWithoutPSquare(lm, duration)
			if i%sampleInterval == 0 {
				_ = sampleWithSort(lm)
			}
		}
	}
}

// BenchmarkHighFrequencyMonitoring_Old simulates high-frequency metrics polling (old).
func BenchmarkHighFrequencyMonitoring_Old(b *testing.B) {
	benchmarkHighFrequencyMonitoring(b, false)
}

// BenchmarkHighFrequencyMonitoring_New simulates high-frequency metrics polling (new).
func BenchmarkHighFrequencyMonitoring_New(b *testing.B) {
	benchmarkHighFrequencyMonitoring(b, true)
}

func benchmarkHighFrequencyMonitoring(b *testing.B, usePSquare bool) {
	lm := &LatencyMetrics{}

	// Pre-fill buffer
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < sampleSize; i++ {
		lm.Record(time.Duration(rng.Intn(100)+1) * time.Millisecond)
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Simulate high-frequency monitoring: Sample() called every iteration
	for i := 0; i < b.N; i++ {
		if usePSquare {
			lm.Sample()
		} else {
			_ = sampleWithSort(lm)
		}
	}
}

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

// TestPSquareQuantile_WithFewSamples tests P-Square with < 5 samples.
func TestPSquareQuantile_WithFewSamples(t *testing.T) {
	ps := newPSquareQuantile(0.5)

	// Edge case: no samples
	if ps.Quantile() != 0 {
		t.Errorf("Quantile with 0 samples should be 0, got %.2f", ps.Quantile())
	}

	// 1 sample
	ps.Update(100)
	if ps.Count() != 1 {
		t.Errorf("Count should be 1, got %d", ps.Count())
	}
	if ps.Quantile() != 100 {
		t.Errorf("Quantile with 1 sample should be 100, got %.2f", ps.Quantile())
	}

	// 3 samples
	ps.Update(200)
	ps.Update(300)
	q := ps.Quantile()
	if q < 100 || q > 300 {
		t.Errorf("P50 of {100, 200, 300} should be around 200, got %.2f", q)
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

	// Check statistics
	if mq.Count() != 100 {
		t.Errorf("Count should be 100, got %d", mq.Count())
	}

	// Sum of 1-100 = 5050
	if mq.Sum() != 5050 {
		t.Errorf("Sum should be 5050, got %.2f", mq.Sum())
	}

	// Mean = 50.5
	if mq.Mean() < 50 || mq.Mean() > 51 {
		t.Errorf("Mean should be 50.5, got %.2f", mq.Mean())
	}

	if mq.Max() != 100 {
		t.Errorf("Max should be 100, got %.2f", mq.Max())
	}
}

// TestPSquareMultiQuantile_Reset tests reset functionality.
func TestPSquareMultiQuantile_Reset(t *testing.T) {
	mq := newPSquareMultiQuantile(0.5, 0.99)

	for i := 1; i <= 100; i++ {
		mq.Update(float64(i))
	}

	if mq.Count() != 100 {
		t.Errorf("Before reset: count should be 100, got %d", mq.Count())
	}

	mq.Reset()

	if mq.Count() != 0 {
		t.Errorf("After reset: count should be 0, got %d", mq.Count())
	}

	if mq.Sum() != 0 {
		t.Errorf("After reset: sum should be 0, got %.2f", mq.Sum())
	}

	// Add new data
	mq.Update(1000)
	if mq.Count() != 1 {
		t.Errorf("After reset and update: count should be 1, got %d", mq.Count())
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

			for i := 0; i < 10000; i++ {
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

			t.Logf("%s: exact P99=%.2f, estimated P99=%.2f, error=%.2f%%",
				tc.name, exactP99, estimatedP99, relError*100)
		})
	}
}

// TestLatencyMetrics_PSquareIntegration tests LatencyMetrics with P-Square.
func TestLatencyMetrics_PSquareIntegration(t *testing.T) {
	lm := &LatencyMetrics{}

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

	t.Logf("P50=%v, P90=%v, P95=%v, P99=%v, Max=%v, Mean=%v",
		lm.P50, lm.P90, lm.P95, lm.P99, lm.Max, lm.Mean)
}

// TestLatencyMetrics_SmallSamplesFallback tests that small samples use exact sorting.
func TestLatencyMetrics_SmallSamplesFallback(t *testing.T) {
	lm := &LatencyMetrics{}

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

// TestLatencyMetrics_ThreadSafety tests concurrent access.
func TestLatencyMetrics_ThreadSafety(t *testing.T) {
	lm := &LatencyMetrics{}

	done := make(chan struct{})

	// Writer goroutine
	go func() {
		for i := 0; i < 10000; i++ {
			lm.Record(time.Duration(i%100+1) * time.Millisecond)
		}
		close(done)
	}()

	// Reader goroutine
	sampleCount := 0
	for {
		select {
		case <-done:
			// Final sample
			lm.Sample()
			t.Logf("Completed %d samples during concurrent writes", sampleCount)
			return
		default:
			lm.Sample()
			sampleCount++
		}
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

// TestPSquareQuantile_NewMinMax tests handling of new min/max values.
func TestPSquareQuantile_NewMinMax(t *testing.T) {
	ps := newPSquareQuantile(0.5)

	// Initialize with values 50-54
	for i := 50; i <= 54; i++ {
		ps.Update(float64(i))
	}

	// Add new minimum
	ps.Update(1)
	t.Log("Internal state updated for new minimum")

	// Add new maximum
	ps.Update(100)

	// P50 should still be reasonable
	q := ps.Quantile()
	if q < 1 || q > 100 {
		t.Errorf("Quantile should be within [1, 100], got %.2f", q)
	}

	// Max should be 100
	if ps.Max() != 100 {
		t.Errorf("Max should be 100, got %.2f", ps.Max())
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
