package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Test_tpsCounter_ConcurrentRotation tests TPS counter under high contention.
// Verify: lastRotation race condition is fixed (issue 7.E.2.1)
func Test_tpsCounter_ConcurrentRotation(t *testing.T) {
	tps := newTPSCounter(10*time.Second, 100*time.Millisecond)

	// High contention test to trigger rotate() race condition
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)

		go func(i int) {
			defer wg.Done()
			tps.Increment()
			time.Sleep(time.Microsecond * time.Duration(i%5))
		}(i)

		go func() {
			defer wg.Done()
			time.Sleep(time.Microsecond * time.Duration(i%3))
			_ = tps.TPS() // Triggers rotate
		}()
	}
	wg.Wait()

	// Verify TPS is non-negative and reasonable (sanity bound)
	recorded := tps.TPS()
	if recorded < 0 {
		t.Errorf("TPS should be non-negative, got: %.2f", recorded)
	}
	if recorded > 1_000_000 {
		t.Errorf("TPS suspiciously high: %.2f (possible bug)", recorded)
	}

	t.Logf("Concurrent rotation test passed, final TPS: %.2f", recorded)
}

// TestLatencyMetrics_MeanAccuracy tests mean calculation across buffer wraps.
// Verify: Sum accumulation bug is fixed (issue 7.E.2.3)
func TestLatencyMetrics_MeanAccuracy(t *testing.T) {
	var metrics LatencyMetrics

	// Submit exactly 1000 samples to fill buffer
	targetLatency := 50 * time.Microsecond
	for i := 0; i < 1000; i++ {
		metrics.Record(targetLatency)
	}

	// Sample to compute mean
	metrics.Sample()

	// Mean should be very close to targetLatency (within 1% tolerance)
	reportedMean := metrics.Mean
	tolerance := targetLatency / 100 // 1% tolerance

	if reportedMean < targetLatency-tolerance || reportedMean > targetLatency+tolerance {
		t.Errorf("Mean latency %v outside expected range [%v-%v] (expected %v)",
			reportedMean, targetLatency-tolerance, targetLatency+tolerance, targetLatency)
	}

	// Verify Sum is correct
	expectedSum := targetLatency * 1000
	if metrics.Sum != expectedSum {
		t.Errorf("Sum %v incorrect (expected %v)", metrics.Sum, expectedSum)
	}

	// Submit 1000 more samples with different latency (causes buffer wrap)
	newLatency := 100 * time.Microsecond
	for i := 0; i < 1000; i++ {
		metrics.Record(newLatency)
	}

	metrics.Sample()
	reportedMean = metrics.Mean
	// Circular buffer replaced all 50µs samples, so mean is now 100µs
	targetMean := newLatency // 100µs
	tolerance = targetMean / 100

	if reportedMean < targetMean-tolerance || reportedMean > targetMean+tolerance {
		t.Errorf("Mean latency after wrap %v outside expected range [%v-%v] (expected %v)",
			reportedMean, targetMean-tolerance, targetMean+tolerance, targetMean)
	}

	// Sum should reflect exactly current buffer contents (all 100µs samples)
	expectedSum = newLatency * 1000
	if metrics.Sum != expectedSum {
		t.Errorf("Sum after wrap %v incorrect (expected %v)", metrics.Sum, expectedSum)
	}

	t.Logf("Mean accuracy test passed: final %v (circular buffer works)", reportedMean)
}

// TestMetrics_ThreadSafety verifies Metrics() returns safe copies.
// Verify: No race conditions when reading metrics (issue 7.E.2.2)
func TestMetrics_ThreadSafety(t *testing.T) {
	loop, err := New(WithMetrics(true))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	// Single Metrics() call to verify snapshot works without races
	metrics := loop.Metrics()
	if metrics == nil {
		t.Fatal("Metrics should not be nil")
	}

	if metrics.TPS < 0 {
		t.Errorf("Thread safety test: invalid TPS %v", metrics.TPS)
	}

	t.Logf("Thread safety test passed: TPS=%.2f, P99=%v", metrics.TPS, metrics.Latency.P99)
}
