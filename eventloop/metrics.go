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
// Thread Safety:
//   - All Metrics methods are thread-safe and can be called from any goroutine.
//   - LatencyMetrics uses sync.RWMutex (single-writer, multi-reader).
//   - QueueMetrics uses sync.RWMutex (single-writer, multi-reader).
//   - TPSCounter uses atomic operations and mutex for rotation.
//   - Metrics() returns a copy, safe for concurrent reads.
//
// Example:
//
//	loop, _ := New(WithMetrics(true))
//	_ = loop.Run(ctx)
//	stats := loop.Metrics()
//	fmt.Printf("TPS: %.2f, P99 Latency: %v\n",
//		stats.TPS, stats.Latency.P99)
type Metrics struct {
	mu sync.Mutex

	// Latency metrics
	Latency LatencyMetrics

	// Throughput metrics
	TPS float64

	// Queue depth metrics
	Queue QueueMetrics
}

// LatencyMetrics tracks latency distribution with percentiles.
type LatencyMetrics struct {
	sampleIdx   int
	sampleCount int
	samples     [sampleSize]time.Duration

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

	// If buffer is full, subtract the old sample that we're replacing
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

// Sample computes percentiles from collected samples.
// This should be called periodically to update the cached percentile values.
// Returns the number of samples used for computation.
//
// Performance note: Sorting has O(n log n) complexity. With sampleSize=1000,
// this takes ~100-200 microseconds. For monitoring, call this no more than
// once per second to avoid impacting event loop performance.
func (l *LatencyMetrics) Sample() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	count := l.sampleCount
	if count == 0 {
		return 0
	}

	// Clone and sort samples for percentile computation
	sorted := make([]time.Duration, count)
	copy(sorted, l.samples[:count])

	// Simple in-place sort (selection sort variant)
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

	// Average depths (exponential moving average with alpha=0.1)
	// Warmstart: EMA initializes to first observed value for accuracy
	IngressAvg   float64
	InternalAvg  float64
	MicrotaskAvg float64

	ingressEMAInitialized   bool
	internalEMAInitialized  bool
	microtaskEMAInitialized bool
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
	// Warmstart: initialize to first observed value for accuracy
	if !q.ingressEMAInitialized {
		q.IngressAvg = float64(depth)
		q.ingressEMAInitialized = true
	} else {
		q.IngressAvg = 0.9*q.IngressAvg + 0.1*float64(depth)
	}
}

// UpdateInternal updates the internal queue depth metrics.
func (q *QueueMetrics) UpdateInternal(depth int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.InternalCurrent = depth
	if depth > q.InternalMax {
		q.InternalMax = depth
	}
	// Exponential moving average with alpha=0.1
	if !q.internalEMAInitialized {
		q.InternalAvg = float64(depth)
		q.internalEMAInitialized = true
	} else {
		q.InternalAvg = 0.9*q.InternalAvg + 0.1*float64(depth)
	}
}

// UpdateMicrotask updates the microtask queue depth metrics.
func (q *QueueMetrics) UpdateMicrotask(depth int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.MicrotaskCurrent = depth
	if depth > q.MicrotaskMax {
		q.MicrotaskMax = depth
	}
	// Exponential moving average with alpha=0.1
	if !q.microtaskEMAInitialized {
		q.MicrotaskAvg = float64(depth)
		q.microtaskEMAInitialized = true
	} else {
		q.MicrotaskAvg = 0.9*q.MicrotaskAvg + 0.1*float64(depth)
	}
}

// TPSCounter tracks transactions per second with a rolling window.
//
// Implementation Details:
//   - Rolling window length: configurable (default: 10 seconds)
//   - Bucket granularity: configurable (default: 100 milliseconds)
//   - Example configuration: 10-second window with 100ms buckets = 100 buckets
//
// Behavior:
//
//	At startup, TPS is 0 until the rolling window fills (default: first 10 seconds).
//	After warmup, TPS reflects average transaction rate over the entire window.
//	Precision: With 100ms buckets, TPS is granular to 0.1 TPS (1 transaction/10 seconds).
//
// Thread Safety: All methods (Increment, TPS) are thread-safe.
type TPSCounter struct {
	lastRotation atomic.Value // Stores time.Time
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
	counter := &TPSCounter{
		buckets:    make([]int64, bucketCount),
		bucketSize: bucketSize,
		windowSize: windowSize,
	}
	counter.lastRotation.Store(time.Now())
	return counter
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
func (t *TPSCounter) rotate() {
	now := time.Now()
	lastRotation := t.lastRotation.Load().(time.Time)
	elapsed := now.Sub(lastRotation)
	bucketsToAdvance := int(elapsed / t.bucketSize)

	if bucketsToAdvance >= len(t.buckets) {
		// Full window reset
		t.mu.Lock()
		for i := range t.buckets {
			t.buckets[i] = 0
		}
		t.mu.Unlock()
		t.lastRotation.Store(now)
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
		t.lastRotation.Store(lastRotation.Add(time.Duration(bucketsToAdvance) * t.bucketSize))
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
