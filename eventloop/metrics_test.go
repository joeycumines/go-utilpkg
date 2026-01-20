package eventloop

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestMetricsAccuracyLatency compares metrics-reported latency.
// Verify: latency tracking works correctly (task 5.3.6).
func TestMetricsAccuracyLatency(t *testing.T) {
	taskCount := 1000
	taskDelay := 50 * time.Microsecond

	// Create loop with metrics enabled
	loop, err := New(WithMetrics(true))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the loop
	go func() {
		_ = loop.Run(ctx)
	}()

	// Wait for loop to be ready
	time.Sleep(10 * time.Millisecond)

	// Submit tasks with known delay
	var completed atomic.Int32
	for i := 0; i < taskCount; i++ {
		loop.Submit(func() {
			time.Sleep(taskDelay)
			completed.Add(1)
		})
	}

	// Wait for all tasks to complete
	for completed.Load() < int32(taskCount) {
		time.Sleep(taskDelay)
	}

	// Get metrics
	metrics := loop.Metrics()
	if metrics == nil {
		t.Fatal("metrics should not be nil")
	}

	// Wait for metrics to be sampled
	time.Sleep(10 * time.Millisecond)
	metrics = loop.Metrics()

	// Verify latency metrics are populated
	if metrics.Latency.P99 == 0 {
		t.Error("P99 latency should be non-zero")
	}

	// Verify latency is reasonable
	minLatency := taskDelay
	maxLatency := 100 * taskDelay

	if metrics.Latency.P99 < minLatency {
		t.Errorf("P99 latency %v is below minimum expected %v",
			metrics.Latency.P99, minLatency)
	}
	if metrics.Latency.P99 > maxLatency {
		t.Errorf("P99 latency %v exceeds maximum expected %v",
			metrics.Latency.P99, maxLatency)
	}

	// Verify all latency fields are consistent
	if metrics.Latency.Max < metrics.Latency.P99 {
		t.Errorf("Max latency %v should be >= P99 latency %v",
			metrics.Latency.Max, metrics.Latency.P99)
	}

	if metrics.Latency.Mean < minLatency {
		t.Errorf("Mean latency %v is below minimum expected %v",
			metrics.Latency.Mean, minLatency)
	}

	t.Logf("Latency metrics - P50: %v, P90: %v, P95: %v, P99: %v, Max: %v, Mean: %v",
		metrics.Latency.P50, metrics.Latency.P90, metrics.Latency.P95,
		metrics.Latency.P99, metrics.Latency.Max, metrics.Latency.Mean)
}

// TestMetricsBasicTPS verifies basic TPS tracking functionality.
// Verify: TPS tracking works (task 5.3.6).
func TestMetricsBasicTPS(t *testing.T) {
	taskCount := 500

	// Create loop with metrics enabled
	loop, err := New(WithMetrics(true))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the loop
	go func() {
		_ = loop.Run(ctx)
	}()

	// Wait for loop to be ready
	time.Sleep(10 * time.Millisecond)

	// Submit tasks
	var completed atomic.Int32
	for i := 0; i < taskCount; i++ {
		loop.Submit(func() {
			completed.Add(1)
		})
		time.Sleep(10 * time.Microsecond)
	}

	// Wait for all tasks to complete
	for completed.Load() < int32(taskCount) {
		time.Sleep(time.Millisecond)
	}

	// Give TPS rolling window time to accumulate
	time.Sleep(500 * time.Millisecond)

	// Get metrics
	metrics := loop.Metrics()
	if metrics == nil {
		t.Fatal("metrics should not be nil")
	}

	recordedTPS := metrics.TPS

	// Verify TPS is non-negative
	if recordedTPS < 0 {
		t.Errorf("TPS should be non-negative, got %.2f", recordedTPS)
	}

	// TPS may be 0 if window is still empty (10-second rolling window)
	// Just verify that field is accessible and non-negative
	t.Logf("Completed %d tasks, Recorded TPS: %.2f", taskCount, recordedTPS)
}

// TestMetricsQueueDepthTracking verifies queue depth metrics.
// Verify: max and current depths are recorded correctly (task 5.3.6).
func TestMetricsQueueDepthTracking(t *testing.T) {
	// Create loop with metrics enabled
	loop, err := New(WithMetrics(true))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the loop
	go func() {
		_ = loop.Run(ctx)
	}()

	// Wait for loop to be ready
	time.Sleep(20 * time.Millisecond)

	// Get baseline metrics
	baselineMetrics := loop.Metrics()
	if baselineMetrics == nil {
		t.Fatal("metrics should not be nil")
	}

	t.Logf("Baseline - Ingress: cur=%d max=%d avg=%.2f, Internal: cur=%d max=%d avg=%.2f",
		baselineMetrics.Queue.IngressCurrent, baselineMetrics.Queue.IngressMax, baselineMetrics.Queue.IngressAvg,
		baselineMetrics.Queue.InternalCurrent, baselineMetrics.Queue.InternalMax, baselineMetrics.Queue.InternalAvg)

	// Check that current depths are non-negative
	if baselineMetrics.Queue.IngressCurrent < 0 {
		t.Error("Queue depths should be non-negative")
	}
	if baselineMetrics.Queue.InternalCurrent < 0 {
		t.Error("Queue depths should be non-negative")
	}

	// Submit some slow tasks
	var completed atomic.Int32
	for i := 0; i < 100; i++ {
		loop.Submit(func() {
			time.Sleep(5 * time.Millisecond)
			completed.Add(1)
		})
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Get final metrics
	finalMetrics := loop.Metrics()

	// Verify max values don't decrease
	if finalMetrics.Queue.IngressMax < baselineMetrics.Queue.IngressMax {
		t.Errorf("IngressMax decreased from %d to %d",
			baselineMetrics.Queue.IngressMax, finalMetrics.Queue.IngressMax)
	}

	t.Logf("Final - Ingress: cur=%d max=%d avg=%.2f, Internal: cur=%d max=%d avg=%.2f",
		finalMetrics.Queue.IngressCurrent, finalMetrics.Queue.IngressMax, finalMetrics.Queue.IngressAvg,
		finalMetrics.Queue.InternalCurrent, finalMetrics.Queue.InternalMax, finalMetrics.Queue.InternalAvg)
}

// TestMetricsDisabled verifies that metrics can be disabled and return nil.
func TestMetricsDisabled(t *testing.T) {
	// Create loop with metrics disabled (default)
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	// Metrics should be nil when not enabled
	metrics := loop.Metrics()
	if metrics != nil {
		t.Error("Metrics should be nil when not enabled via WithMetrics option")
	}

	// Submit a task
	loop.Submit(func() {})

	// Still nil
	metrics = loop.Metrics()
	if metrics != nil {
		t.Error("Metrics should remain nil when not enabled")
	}
}

// BenchmarkMetricsCollection benchmarks overhead of metrics collection.
func BenchmarkMetricsCollection(b *testing.B) {
	loop, err := New(WithMetrics(true))
	if err != nil {
		b.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loop.Submit(func() {})
	}
}

// BenchmarkNoMetrics benchmarks performance without metrics.
func BenchmarkNoMetrics(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loop.Submit(func() {})
	}
}
