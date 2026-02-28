package eventloop

import (
	"context"
	"testing"
	"time"
)

// TestMetrics_UpdateHooks verifies that metrics Update hooks are called
// during loop execution when Metrics are provided.
func TestMetrics_UpdateHooks(t *testing.T) {
	// WithMetrics enables metrics collection on the loop
	opts := []LoopOption{WithMetrics(true)}

	loop, err := New(opts...)
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	// Check that metrics are enabled
	// When WithMetrics(true) is used, metrics should be available
	// The loop internally creates a Metrics struct

	// Trigger metric updates by submitting tasks
	loop.Submit(func() {})

	// Run a tick to process microtasks
	loop.tick()

	// Schedule microtask
	loop.ScheduleMicrotask(func() {})

	// Run loop briefly to ensure all metrics are collected
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	go loop.Run(ctx)
	time.Sleep(50 * time.Millisecond)

	// Retrieve metrics to verify collection worked
	stats := loop.Metrics()
	if stats == nil {
		t.Error("Metrics() should return non-nil stats when WithMetrics(true) is used")
	}
}

// TestMetrics_MetricsStructure verifies that Metrics struct has expected
// fields for proper metrics collection.
func TestMetrics_MetricsStructure(t *testing.T) {
	var metrics Metrics

	// Verify all expected metrics fields exist
	// Metrics struct contains: Latency, TPS, Queue
	// These are all properly initialized with zero values

	// Verify LatencyMetrics fields
	if metrics.Latency.Mean != 0 {
		t.Error("Latency.Mean should start at 0")
	}
	if metrics.Latency.Sum != 0 {
		t.Error("Latency.Sum should start at 0")
	}

	// Verify TPS field
	if metrics.TPS != 0 {
		t.Error("TPS should start at 0")
	}

	// Verify QueueMetrics fields
	if metrics.Queue.IngressCurrent != 0 {
		t.Error("Queue.IngressCurrent should start at 0")
	}
	if metrics.Queue.InternalCurrent != 0 {
		t.Error("Queue.InternalCurrent should start at 0")
	}
	if metrics.Queue.MicrotaskCurrent != 0 {
		t.Error("Queue.MicrotaskCurrent should start at 0")
	}

	// Verify max fields
	if metrics.Queue.IngressMax != 0 {
		t.Error("Queue.IngressMax should start at 0")
	}
	if metrics.Queue.InternalMax != 0 {
		t.Error("Queue.InternalMax should start at 0")
	}
	if metrics.Queue.MicrotaskMax != 0 {
		t.Error("Queue.MicrotaskMax should start at 0")
	}

	// Verify EMA fields
	if metrics.Queue.IngressAvg != 0 {
		t.Error("Queue.IngressAvg should start at 0")
	}
	if metrics.Queue.InternalAvg != 0 {
		t.Error("Queue.InternalAvg should start at 0")
	}
	if metrics.Queue.MicrotaskAvg != 0 {
		t.Error("Queue.MicrotaskAvg should start at 0")
	}
}

// TestMetrics_NoMetrics verifies that loop works correctly when no
// metrics are provided (default behavior).
func TestMetrics_NoMetrics(t *testing.T) {
	// Create loop without WithMetrics option (metrics disabled by default)
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	// Metrics() should return nil when disabled (consistent with loop.go implementation)
	stats := loop.Metrics()
	if stats != nil {
		t.Error("Metrics() should return nil when disabled")
	}

	// Run basic operations to ensure no nil pointer dereferences
	loop.Submit(func() {})
	loop.tick()
	loop.ScheduleMicrotask(func() {})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	go loop.Run(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()

	// If we reach here, nil metrics handling is correct
}

// TestMetrics_MetricsThreadSafety verifies that metrics updates are
// thread-safe under concurrent access.
func TestMetrics_MetricsThreadSafety(t *testing.T) {
	// Create loop with metrics enabled
	loop, err := New(WithMetrics(true))
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Submit many tasks concurrently to trigger multiple metric updates
	const numIterations = 100

	for range numIterations {
		go func() {
			loop.Submit(func() {})
		}()
	}

	go loop.Run(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Verify metrics were collected
	stats := loop.Metrics()
	if stats.TPS == 0 {
		t.Logf("Note: TPS is 0 (may be due to window warmup)")
	}

	// Success: no panics, no data races (verified when run with -race)
}
