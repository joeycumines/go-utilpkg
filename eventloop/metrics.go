package eventloop

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics tracks runtime statistics for the event loop.
// Metrics are designed to be low-overhead and thread-safe.
// All metrics are optional and can be attached to a Loop via options (e.g., WithMetrics).
//
// # Thread Safety
//
// All Metrics methods are thread-safe and can be called from any goroutine.
//
// # Example
//
//	loop, _ := New(WithMetrics(true))
//	_ = loop.Run(ctx)
//	stats := loop.Metrics()
//	fmt.Printf("TPS: %.2f, P99 Latency: %v\n",
//		stats.TPS, stats.Latency.P99)
type Metrics struct {
	// Latency metrics
	Latency LatencyMetrics

	// Throughput metrics
	TPS float64

	// Queue depth metrics
	Queue QueueMetrics
}

// LatencyMetrics tracks latency distribution with percentiles.
type LatencyMetrics struct {
	samples []time.Duration

	// Computed percentiles (cached after Sample() call)
	P50 time.Duration
	P90 time.Duration
	P95 time.Duration
	P99 time.Duration
	Max time.Duration

	// Statistics
	Mean time.Duration
	Sum  time.Duration
	mu   sync.RWMutex
}

// sampleSize is the maximum number of latency samples to retain.
// We keep a rolling buffer of 1000 samples to compute percentiles.
const sampleSize = 1000

// Record records a latency sample.
// This is called internally by the loop after each task execution.
func (l *LatencyMetrics) Record(duration time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.samples) < sampleSize {
		l.samples = append(l.samples, duration)
	} else {
		// Rolling window: replace oldest sample (circular buffer simulation)
		// For simplicity, we just replace at a rotating index
		l.samples[0] = duration
		l.samples = l.samples[1:]
	}
	l.Sum += duration
}

// Sample computes percentiles from collected samples.
// This should be called periodically to update the cached percentile values.
// Returns the number of samples used for computation.
func (l *LatencyMetrics) Sample() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	count := len(l.samples)
	if count == 0 {
		return 0
	}

	// Clone and sort samples for percentile computation
	sorted := make([]time.Duration, count)
	copy(sorted, l.samples)

	// Simple in-place sort (insertion sort for small N, or quicksort for larger)
	for i := 0; i < count; i++ {
		for j := i + 1; j < count; j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Compute percentiles
	if count > 0 {
		l.P50 = sorted[percentileIndex(count, 50)]
		l.P90 = sorted[percentileIndex(count, 90)]
		l.P95 = sorted[percentileIndex(count, 95)]
		l.P99 = sorted[percentileIndex(count, 99)]
		l.Max = sorted[count-1]
		l.Mean = l.Sum / time.Duration(count)
	}

	return count
}

// percentileIndex computes the index for a given percentile (0-100).
func percentileIndex(n, p int) int {
	index := (p * n) / 100
	if index >= n {
		return n - 1
	}
	return index
}

// QueueMetrics tracks queue depth statistics.
type QueueMetrics struct {
	mu sync.RWMutex

	// Current queue depths
	IngressCurrent   int
	InternalCurrent  int
	MicrotaskCurrent int

	// Maximum observed depths
	IngressMax   int
	InternalMax  int
	MicrotaskMax int

	// Average depths (rolling average)
	IngressAvg   float64
	InternalAvg  float64
	MicrotaskAvg float64
}

// UpdateIngress updates the ingress queue depth metrics.
func (q *QueueMetrics) UpdateIngress(depth int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.IngressCurrent = depth
	if depth > q.IngressMax {
		q.IngressMax = depth
	}
	// Exponential moving average with alpha=0.1
	q.IngressAvg = 0.9*q.IngressAvg + 0.1*float64(depth)
}

// UpdateInternal updates the internal queue depth metrics.
func (q *QueueMetrics) UpdateInternal(depth int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.InternalCurrent = depth
	if depth > q.InternalMax {
		q.InternalMax = depth
	}
	q.InternalAvg = 0.9*q.InternalAvg + 0.1*float64(depth)
}

// UpdateMicrotask updates the microtask queue depth metrics.
func (q *QueueMetrics) UpdateMicrotask(depth int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.MicrotaskCurrent = depth
	if depth > q.MicrotaskMax {
		q.MicrotaskMax = depth
	}
	q.MicrotaskAvg = 0.9*q.MicrotaskAvg + 0.1*float64(depth)
}

// TPSCounter tracks transactions per second with a rolling window.
type TPSCounter struct {
	lastRotation time.Time
	buckets      []int64
	bucketSize   time.Duration
	windowSize   time.Duration
	totalCount   atomic.Int64
	mu           sync.Mutex
}

// NewTPSCounter creates a new TPS counter.
// windowSize is the time window for TPS calculation (e.g., 10*time.Second).
// bucketSize is the granularity of the rolling window (e.g., 100*time.Millisecond).
func NewTPSCounter(windowSize, bucketSize time.Duration) *TPSCounter {
	bucketCount := int(windowSize / bucketSize)
	if bucketCount < 1 {
		bucketCount = 1
	}
	return &TPSCounter{
		buckets:      make([]int64, bucketCount),
		bucketSize:   bucketSize,
		windowSize:   windowSize,
		lastRotation: time.Now(),
	}
}

// Increment records a task execution.
// Thread-safe and O(1).
func (t *TPSCounter) Increment() {
	t.totalCount.Add(1)
	t.rotate()
	t.mu.Lock()
	t.buckets[len(t.buckets)-1]++
	t.mu.Unlock()
}

// rotate advances the bucket counter if time has passed.
// Must hold mu.Lock() before calling.
func (t *TPSCounter) rotate() {
	now := time.Now()
	elapsed := now.Sub(t.lastRotation)
	bucketsToAdvance := int(elapsed / t.bucketSize)

	if bucketsToAdvance >= len(t.buckets) {
		// Full window reset
		t.mu.Lock()
		for i := range t.buckets {
			t.buckets[i] = 0
		}
		t.mu.Unlock()
		t.lastRotation = now
		return
	}

	if bucketsToAdvance > 0 {
		t.mu.Lock()
		// Shift buckets left, filling with zeros
		for i := 0; i < len(t.buckets)-bucketsToAdvance; i++ {
			t.buckets[i] = t.buckets[i+bucketsToAdvance]
		}
		for i := len(t.buckets) - bucketsToAdvance; i < len(t.buckets); i++ {
			t.buckets[i] = 0
		}
		t.mu.Unlock()
		t.lastRotation = t.lastRotation.Add(time.Duration(bucketsToAdvance) * t.bucketSize)
	}
}

// TPS returns the current transactions per second.
func (t *TPSCounter) TPS() float64 {
	t.rotate()

	t.mu.Lock()
	defer t.mu.Unlock()

	var sum int64
	for _, count := range t.buckets {
		sum += count
	}

	if sum == 0 {
		return 0
	}

	// TPS = total count / window size in seconds
	seconds := t.windowSize.Seconds()
	return float64(sum) / seconds
}
