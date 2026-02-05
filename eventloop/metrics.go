package eventloop

import (
	"sort"
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
	// Latency metrics (has pointer field - put first for alignment)
	Latency LatencyMetrics

	// Queue depth metrics
	Queue QueueMetrics

	mu sync.Mutex

	// Throughput metrics
	TPS float64
}

// LatencyMetrics tracks latency distribution with percentiles.
// Uses the P-Square algorithm for O(1) streaming percentile estimation,
// which is more efficient than the previous O(n log n) sorting approach.
type LatencyMetrics struct {
	// Pointer fields first for optimal alignment (betteralign)
	psquare *pSquareMultiQuantile

	// Lock for thread-safe access
	mu sync.RWMutex

	// Legacy sample buffer (kept for backward compatibility with tests
	// that check exact percentile values with small sample counts)
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
}

// sampleSize is the maximum number of latency samples to retain.
// We keep a rolling buffer of 1000 samples to compute percentiles.
const sampleSize = 1000

// Record records a latency sample.
// This is called internally by the loop after each task execution.
// Uses O(1) P-Square algorithm for streaming percentile updates.
func (l *LatencyMetrics) Record(duration time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Initialize P-Square estimator on first use (lazy initialization)
	if l.psquare == nil {
		// Track P50 (0.5), P90 (0.9), P95 (0.95), P99 (0.99)
		l.psquare = newPSquareMultiQuantile(0.50, 0.90, 0.95, 0.99)
	}

	// Update P-Square estimator with the new sample (O(1))
	l.psquare.Update(float64(duration))

	// Also update legacy sample buffer for backward compatibility
	// (used when sample count < sampleSize for exact percentiles)
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
// Performance note: For sample counts >= 5, this uses the P-Square algorithm
// which is O(1). For smaller counts, falls back to O(n log n) sorting for
// exact percentile values. The previous implementation was always O(n log n).
func (l *LatencyMetrics) Sample() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	count := l.sampleCount
	if count == 0 {
		return 0
	}

	// For small sample counts (< 5), use exact sorting method
	// This ensures backward compatibility with tests that expect exact values
	if count < 5 || l.psquare == nil {
		// Clone and sort samples for percentile computation
		sorted := make([]time.Duration, count)
		copy(sorted, l.samples[:count])

		// Use standard library sort (O(n log n))
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i] < sorted[j]
		})

		// Compute percentiles
		l.P50 = sorted[percentileIndex(count, 50)]
		l.P90 = sorted[percentileIndex(count, 90)]
		l.P95 = sorted[percentileIndex(count, 95)]
		l.P99 = sorted[percentileIndex(count, 99)]
		l.Max = sorted[count-1]
		l.Mean = l.Sum / time.Duration(count)

		return count
	}

	// Use P-Square algorithm for O(1) percentile retrieval
	// Index 0 = P50, Index 1 = P90, Index 2 = P95, Index 3 = P99
	l.P50 = time.Duration(l.psquare.Quantile(0))
	l.P90 = time.Duration(l.psquare.Quantile(1))
	l.P95 = time.Duration(l.psquare.Quantile(2))
	l.P99 = time.Duration(l.psquare.Quantile(3))
	l.Max = time.Duration(l.psquare.Max())

	// Use the ring buffer's Sum for Mean calculation to maintain semantic
	// compatibility with the circular buffer (tracks last sampleSize samples)
	l.Mean = l.Sum / time.Duration(count)

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
//   - Rolling window length: configurable via windowSize parameter
//   - Bucket granularity: configurable via bucketSize parameter
//   - Rolling window algorithm: ring buffer with time-based rotation
//
// Configuration Trade-offs:
//
//	Window Size (windowSize):
//	  - Larger windows (e.g., 30 seconds): Smoother TPS, slower to detect changes
//	  - Smaller windows (e.g., 5 seconds): Faster response, more volatile
//	  - Recommended: 10-30 seconds for production monitoring
//
//	Bucket Size (bucketSize):
//	  - Smaller buckets (e.g., 50ms): Higher precision (0.02 TPS), more CPU overhead
//	  - Larger buckets (e.g., 500ms): Lower precision (0.5 TPS), less CPU overhead
//	  - Recommended: 100ms for good balance (0.1 TPS precision) in production
//
// Behavior:
//
//	At startup, TPS is 0 until the rolling window fills (depends on windowSize).
//	After warmup, TPS reflects average transaction rate over the entire window.
//	Precision granularity: (1 / bucketSize in seconds), e.g., 100ms = 0.1 TPS precision.
//
// Thread Safety: All methods (Increment, TPS) are thread-safe.
// Concurrent calls are safe from multiple goroutines.
type TPSCounter struct {
	lastRotation atomic.Value // Stores time.Time
	buckets      []int64
	bucketSize   time.Duration
	windowSize   time.Duration
	mu           sync.Mutex
}

// NewTPSCounter creates a new TPS counter with configurable rolling window.
//
// Parameters:
//
//	windowSize - Time window for TPS calculation. Larger windows provide smoother
//	            TPS but slower change detection. Recommended: 10-30 seconds for
//	            production monitoring. Must be > 0.
//	bucketSize - Granularity of rolling window. Smaller buckets provide higher
//	            precision but more CPU overhead. Recommended: 100ms for 0.1 TPS
//	            precision in production. Must be > 0 and <= windowSize.
//
// Configuration Examples:
//
//	// Production: Balanced precision and smoothness
//	NewTPSCounter(10*time.Second, 100*time.Millisecond) // 100 buckets, 0.1 TPS precision
//
//	// High-frequency trading: Fast response, more volatile
//	NewTPSCounter(5*time.Second, 50*time.Millisecond) // 100 buckets, 0.2 TPS precision
//
//	// Long-term analysis: Very smooth, slow response
//	NewTPSCounter(60*time.Second, 500*time.Millisecond) // 120 buckets, 0.5 TPS precision
//
// Returns:
//
//	Ready-to-use TPS counter. TPS is 0 until window fills.
//
// Note: At startup, TPS is 0 until the first 'windowSize' period elapses,
//
//	providing time for the rolling window to fill with actual metrics.
func NewTPSCounter(windowSize, bucketSize time.Duration) *TPSCounter {
	// Input validation: Prevent zero or negative durations
	if windowSize <= 0 {
		panic("eventloop: windowSize must be positive (use > 0 duration)")
	}
	if bucketSize <= 0 {
		panic("eventloop: bucketSize must be positive (use > 0 duration)")
	}
	if bucketSize > windowSize {
		panic("eventloop: bucketSize cannot exceed windowSize (use <= windowSize)")
	}

	// bucketCount is guaranteed to be >= 1 after the above validation
	bucketCount := int(windowSize / bucketSize)
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
	t.rotate()
	t.mu.Lock()
	t.buckets[len(t.buckets)-1]++
	t.mu.Unlock()
}

// rotate advances the bucket counter if time has passed.
func (t *TPSCounter) rotate() {
	t.mu.Lock() // critical fix: lock first to prevent race
	defer t.mu.Unlock()

	now := time.Now()
	lastRotation := t.lastRotation.Load().(time.Time)
	elapsed := now.Sub(lastRotation)

	// Overflow protection: calculate as int64, clamp to safe range, then cast to int
	// This prevents 32-bit overflow on extreme time jumps (system suspend, NTP changes)
	bucketsToAdvanceInt64 := int64(elapsed) / int64(t.bucketSize)

	// Clamp to window size to handle extreme negative/positive elapsed values
	if bucketsToAdvanceInt64 < 0 {
		// Clock jumped backwards - trigger full reset to recover
		bucketsToAdvanceInt64 = int64(len(t.buckets))
	} else if bucketsToAdvanceInt64 > int64(len(t.buckets)) {
		// Elapsed time exceeded window - clamp to full window reset
		bucketsToAdvanceInt64 = int64(len(t.buckets))
	}

	// NOW safe to cast to int (value guaranteed to be within [0, len(buckets)])
	bucketsToAdvance := int(bucketsToAdvanceInt64)

	// Full window reset: if we've exceeded window duration, reset all buckets
	// and sync lastRotation to current time to prevent permanent lag
	if bucketsToAdvance >= len(t.buckets) {
		for i := range t.buckets {
			t.buckets[i] = 0
		}
		t.lastRotation.Store(now)
		return
	}

	if bucketsToAdvance <= 0 {
		return
	}

	// Shift buckets left
	// Use copy for efficiency: bucket[0] gets bucket[advance], etc.
	copy(t.buckets, t.buckets[bucketsToAdvance:])

	// Zero out the new buckets at the end
	for i := len(t.buckets) - bucketsToAdvance; i < len(t.buckets); i++ {
		t.buckets[i] = 0
	}

	// Update last rotation aligned to bucket size
	t.lastRotation.Store(lastRotation.Add(time.Duration(bucketsToAdvance) * t.bucketSize))
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

	// TPS = total count / monitored duration (len(buckets) * bucketSize)
	// This uses the actual monitored duration, not the configured windowSize.
	monitoredDuration := float64(len(t.buckets)) * t.bucketSize.Seconds()
	return float64(sum) / monitoredDuration
}
