package eventloop

import (
	"math"
	"math/bits"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics is a detached runtime-statistics snapshot for an event loop.
// Collection is optional and can be enabled with [WithMetrics].
//
// Thread Safety:
//   - [Loop.Metrics] may be called from any goroutine.
//   - Loop hot-path metrics use private atomic and owner-local samplers.
//   - tpsCounter serializes bucket rotation and increment as one timestamped operation.
//   - Each returned value represents one fully committed sampler epoch and may
//     be retained or modified without affecting the Loop.
//
// Example:
//
//	loop := New(WithMetrics(true))
//	_ = loop.Run(ctx)
//	stats := loop.Metrics()
//	fmt.Printf("TPS: %.2f, P99 Callback Duration: %v\n",
//		stats.TPS, stats.Latency.P99)
type Metrics struct {
	// Latency is the observed scheduled-callback execution-duration distribution.
	// The retained field name is historical; queue residence before callback
	// admission is excluded.
	Latency LatencyMetrics

	// Queue is the latest owner-turn queue-depth sample and historical maxima and averages.
	Queue QueueMetrics

	// TPS is the rolling throughput of scheduled callbacks that returned successfully.
	TPS float64
}

func newRuntimeMetrics() *runtimeMetrics {
	return &runtimeMetrics{tps: newTPSCounter(10*time.Second, 100*time.Millisecond)}
}

type runtimeMetrics struct {
	tps         *tpsCounter
	latency     runtimeLatencyMetrics
	queue       runtimeQueueMetrics
	snapshotSeq atomic.Uint64
}

func (m *runtimeMetrics) write(update func()) {
	if m == nil || update == nil {
		return
	}
	for {
		seq := m.snapshotSeq.Load()
		if seq&1 != 0 {
			runtime.Gosched()
			continue
		}
		if !m.snapshotSeq.CompareAndSwap(seq, seq+1) {
			continue
		}
		func() {
			defer m.snapshotSeq.Store(seq + 2)
			update()
		}()
		return
	}
}

func (m *runtimeMetrics) recordQueueDepths(ingress, internal, microtask int) {
	if m == nil {
		return
	}
	m.write(func() { m.queue.record(ingress, internal, microtask) })
}

func (m *runtimeMetrics) recordCallback(duration time.Duration, completedAt time.Time, successful bool) {
	if m == nil {
		return
	}
	m.write(func() {
		m.latency.record(duration)
		if successful && m.tps != nil {
			m.tps.IncrementAt(completedAt)
		}
	})
}

func (m *runtimeMetrics) snapshot() *Metrics {
	return m.snapshotWith(copyRuntimeMetrics)
}

func (m *runtimeMetrics) snapshotWith(read func(*runtimeMetrics) *Metrics) *Metrics {
	if m == nil {
		return nil
	}
	if read == nil {
		return nil
	}
	for {
		seq := m.snapshotSeq.Load()
		if seq&1 != 0 {
			runtime.Gosched()
			continue
		}
		snapshot := read(m)
		if m.snapshotSeq.Load() == seq {
			return snapshot
		}
	}
}

func copyRuntimeMetrics(m *runtimeMetrics) *Metrics {
	snapshot := &Metrics{}
	if m.tps != nil {
		snapshot.TPS = m.tps.TPS()
	}
	m.latency.snapshot(&snapshot.Latency)
	m.queue.snapshot(&snapshot.Queue)
	return snapshot
}

type runtimeQueueMetrics struct {
	ingressAvgBits   atomic.Uint64
	internalAvgBits  atomic.Uint64
	microtaskAvgBits atomic.Uint64
	ingressCurrent   atomic.Int64
	internalCurrent  atomic.Int64
	microtaskCurrent atomic.Int64
	ingressMax       atomic.Int64
	internalMax      atomic.Int64
	microtaskMax     atomic.Int64
	ingressInit      atomic.Bool
	internalInit     atomic.Bool
	microtaskInit    atomic.Bool
}

func (q *runtimeQueueMetrics) record(ingress, internal, microtask int) {
	q.recordOne(q.ingressCurrent.Store, &q.ingressMax, &q.ingressAvgBits, &q.ingressInit, ingress)
	q.recordOne(q.internalCurrent.Store, &q.internalMax, &q.internalAvgBits, &q.internalInit, internal)
	q.recordOne(q.microtaskCurrent.Store, &q.microtaskMax, &q.microtaskAvgBits, &q.microtaskInit, microtask)
}

func (q *runtimeQueueMetrics) recordOne(storeCurrent func(int64), max *atomic.Int64, avg *atomic.Uint64, initialized *atomic.Bool, depth int) {
	if depth < 0 {
		depth = 0
	}
	value := int64(depth)
	storeCurrent(value)
	atomicMaxInt64(max, value)
	if !initialized.Load() {
		avg.Store(math.Float64bits(float64(depth)))
		initialized.Store(true)
		return
	}
	previous := math.Float64frombits(avg.Load())
	avg.Store(math.Float64bits(0.9*previous + 0.1*float64(depth)))
}

func (q *runtimeQueueMetrics) snapshot(dst *QueueMetrics) {
	dst.IngressCurrent = int(q.ingressCurrent.Load())
	dst.InternalCurrent = int(q.internalCurrent.Load())
	dst.MicrotaskCurrent = int(q.microtaskCurrent.Load())
	dst.IngressMax = int(q.ingressMax.Load())
	dst.InternalMax = int(q.internalMax.Load())
	dst.MicrotaskMax = int(q.microtaskMax.Load())
	dst.IngressAvg = math.Float64frombits(q.ingressAvgBits.Load())
	dst.InternalAvg = math.Float64frombits(q.internalAvgBits.Load())
	dst.MicrotaskAvg = math.Float64frombits(q.microtaskAvgBits.Load())
}

type runtimeLatencyMetrics struct {
	psquare *pSquareMultiQuantile
	sumHi   uint64
	sumLo   uint64
	count   atomic.Uint64
	mean    atomic.Int64
	p50     atomic.Int64
	p90     atomic.Int64
	p95     atomic.Int64
	p99     atomic.Int64
	max     atomic.Int64
	samples [exactLatencySampleSize]atomic.Int64
}

// record is called only by the loop owner / terminal-drain owner from
// safeExecute. It intentionally keeps the P-Square estimator owner-local while
// publishing snapshot fields through atomics for Metrics(), which may run from
// any goroutine.
func (l *runtimeLatencyMetrics) record(duration time.Duration) {
	if duration < 0 {
		duration = 0
	}
	count := l.count.Load()
	if count == ^uint64(0) {
		return
	}
	count++
	l.count.Store(count)
	if count <= exactLatencySampleSize {
		l.samples[count-1].Store(int64(duration))
	}
	var carry uint64
	l.sumLo, carry = bits.Add64(l.sumLo, uint64(duration), 0)
	l.sumHi += carry
	if l.psquare == nil {
		l.psquare = newPSquareMultiQuantile(0.50, 0.90, 0.95, 0.99)
	}
	l.psquare.Update(float64(duration))
	mean, _ := bits.Div64(l.sumHi, l.sumLo, count)
	l.mean.Store(int64(mean))
	l.p50.Store(int64(latencyDuration(l.psquare.Quantile(0))))
	l.p90.Store(int64(latencyDuration(l.psquare.Quantile(1))))
	l.p95.Store(int64(latencyDuration(l.psquare.Quantile(2))))
	l.p99.Store(int64(latencyDuration(l.psquare.Quantile(3))))
	atomicMaxInt64(&l.max, int64(duration))
}

func (l *runtimeLatencyMetrics) snapshot(dst *LatencyMetrics) {
	count := l.count.Load()
	if count == 0 {
		return
	}
	dst.Count = count
	dst.Mean = time.Duration(l.mean.Load())
	if count > exactLatencySampleSize {
		dst.P50 = time.Duration(l.p50.Load())
		dst.P90 = time.Duration(l.p90.Load())
		dst.P95 = time.Duration(l.p95.Load())
		dst.P99 = time.Duration(l.p99.Load())
		dst.Max = time.Duration(l.max.Load())
		return
	}

	var durations [exactLatencySampleSize]time.Duration
	for i := range int(count) {
		durations[i] = time.Duration(l.samples[i].Load())
	}
	exact := durations[:count]
	slices.Sort(exact)
	dst.P50 = exactMedian(exact)
	dst.P90 = exact[percentileIndex(int(count), 90)]
	dst.P95 = exact[percentileIndex(int(count), 95)]
	dst.P99 = exact[percentileIndex(int(count), 99)]
	dst.Max = exact[len(exact)-1]
}

func atomicMaxInt64(value *atomic.Int64, next int64) {
	for {
		old := value.Load()
		if next <= old || value.CompareAndSwap(old, next) {
			return
		}
	}
}

func latencyDuration(value float64) time.Duration {
	if math.IsNaN(value) || value <= 0 {
		return 0
	}
	if value >= float64(math.MaxInt64) {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(value)
}

func exactMedian(sorted []time.Duration) time.Duration {
	middle := len(sorted) / 2
	if len(sorted)%2 != 0 {
		return sorted[middle]
	}
	lower := sorted[middle-1]
	return lower + (sorted[middle]-lower)/2
}

// LatencyMetrics is a detached scheduled-callback execution-duration
// distribution returned by [Loop.Metrics]. The historical Latency name does not
// include time spent waiting in a queue. Measurement begins immediately before
// callback dispatch and includes synchronous abnormal-exit diagnostics.
// Percentiles, Max, and Mean cover all Count observations; percentiles use exact
// samples through the fifth observation and a streaming estimate thereafter.
type LatencyMetrics struct {
	// Count is the number of scheduled callbacks admitted to execution by the Loop,
	// including tasks, timers, I/O, phase callbacks, and microtasks. Internal
	// liveness and queue-pressure control callbacks are excluded.
	Count uint64

	// P50 is the 50th percentile (median) callback execution duration. For an
	// exact even-sized sample, it is the arithmetic midpoint of the two middle
	// observations, truncated to an integral nanosecond.
	P50 time.Duration
	// P90 is the 90th percentile callback execution duration.
	P90 time.Duration
	// P95 is the 95th percentile callback execution duration.
	P95 time.Duration
	// P99 is the 99th percentile callback execution duration.
	P99 time.Duration
	// Max is the maximum observed callback execution duration.
	Max time.Duration

	// Mean is the arithmetic mean of all observed callback execution durations,
	// truncated to an integral nanosecond.
	Mean time.Duration
}

const exactLatencySampleSize = 5

// percentileIndex computes the index for a given percentile (0-100).
func percentileIndex(n, p int) int {
	index := (p * n) / 100
	if index >= n {
		return n - 1
	}
	return index
}

// QueueMetrics is a detached queue-depth sample returned by [Loop.Metrics].
// Current fields come from the latest startup, fast-path, or polling owner turn;
// Max and Avg cover all such samples.
type QueueMetrics struct {
	// IngressCurrent is the pending external task, immediate, and close-callback
	// depth at the latest owner-turn sample.
	IngressCurrent int
	// InternalCurrent is the internal priority depth at the latest owner-turn sample.
	InternalCurrent int
	// MicrotaskCurrent is the microtask occupancy at the latest owner-turn sample.
	MicrotaskCurrent int

	// IngressMax is the maximum observed external task, immediate, and
	// close-callback depth.
	IngressMax int
	// InternalMax is the maximum observed internal queue depth.
	InternalMax int
	// MicrotaskMax is the maximum observed microtask queue depth.
	MicrotaskMax int

	// IngressAvg is the exponential moving average of external task, immediate,
	// and close-callback depth (alpha=0.1).
	IngressAvg float64
	// InternalAvg is the exponential moving average of internal queue depth (alpha=0.1).
	InternalAvg float64
	// MicrotaskAvg is the exponential moving average of microtask queue depth (alpha=0.1).
	MicrotaskAvg float64
}

// tpsCounter tracks transactions per second with a rolling window.
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
//	  - Smaller buckets expire old observations more precisely, with more CPU and memory overhead
//	  - Larger buckets expire observations less precisely, with less CPU and memory overhead
//	  - Recommended: 100ms for a practical expiration-granularity tradeoff
//
// Behavior:
//
//	TPS is available immediately after the first increment; there is no warmup period.
//	It reflects the observed count divided by the monitored window duration.
//	Numeric resolution is one event divided by that duration. Bucket size controls
//	when observations expire, not the value contributed by each observation.
//
// Thread Safety: All methods (Increment, TPS) are thread-safe.
// Concurrent calls are safe from multiple goroutines.
type tpsCounter struct {
	lastRotation atomic.Value // Stores time.Time
	buckets      []atomic.Int64
	bucketSize   time.Duration
	current      atomic.Uint64
	mu           sync.Mutex
}

// newTPSCounter creates a new TPS counter with configurable rolling window.
//
// Parameters:
//
//	windowSize - Time window for TPS calculation. Larger windows provide smoother
//	            TPS but slower change detection. Recommended: 10-30 seconds for
//	            production monitoring. Must be > 0.
//	bucketSize - Expiration granularity of the rolling window. Smaller buckets
//	            expire observations more precisely but use more CPU and memory.
//	            Must be > 0 and <= windowSize.
//
// Configuration Examples:
//
//	// Production: Balanced expiration granularity and smoothness
//	newTPSCounter(10*time.Second, 100*time.Millisecond) // 100 buckets, 0.1 TPS numeric resolution
//
//	// Faster expiration response, more volatile
//	newTPSCounter(5*time.Second, 50*time.Millisecond) // 100 buckets, 0.2 TPS numeric resolution
//
//	// Long-term analysis: Very smooth, slow response
//	newTPSCounter(60*time.Second, 500*time.Millisecond) // 120 buckets, 1/60 TPS numeric resolution
//
// Returns:
//
//	Ready-to-use TPS counter. TPS is zero until the first increment and is
//	available immediately afterward.
func newTPSCounter(windowSize, bucketSize time.Duration) *tpsCounter {
	return newTPSCounterAt(windowSize, bucketSize, time.Now())
}

func newTPSCounterAt(windowSize, bucketSize time.Duration, anchor time.Time) *tpsCounter {
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
	counter := &tpsCounter{
		buckets:    make([]atomic.Int64, bucketCount),
		bucketSize: bucketSize,
	}
	counter.lastRotation.Store(anchor)
	return counter
}

// Increment records a task execution.
// Thread-safe and O(1).
func (t *tpsCounter) Increment() {
	t.IncrementAt(time.Now())
}

func (t *tpsCounter) IncrementAt(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.rotateAtLocked(now)
	idx := int(t.current.Load() % uint64(len(t.buckets)))
	atomicIncrementInt64(&t.buckets[idx])
}

func (t *tpsCounter) rotateAtLocked(now time.Time) {
	lastRotation := t.lastRotation.Load().(time.Time)
	bucketsToAdvanceInt64 := elapsedBuckets(now, lastRotation, t.bucketSize)
	if bucketsToAdvanceInt64 == 0 {
		return
	}

	// Overflow protection: calculate as int64, clamp to safe range, then cast to int
	// This prevents 32-bit overflow on extreme timestamp discontinuities.

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
			t.buckets[i].Store(0)
		}
		t.current.Store(0)
		t.lastRotation.Store(now)
		return
	}

	if bucketsToAdvance <= 0 {
		return
	}

	// Advance a ring index instead of shifting buckets on every rotation. The
	// current bucket is the only one mutated by the hot Increment path; TPS()
	// aggregates all buckets when a caller asks for a snapshot.
	current := t.current.Load()
	for range bucketsToAdvance {
		current = (current + 1) % uint64(len(t.buckets))
		t.buckets[current].Store(0)
	}
	t.current.Store(current)

	// Update last rotation aligned to bucket size
	t.lastRotation.Store(lastRotation.Add(time.Duration(bucketsToAdvance) * t.bucketSize))
}

func elapsedBuckets(now, lastRotation time.Time, bucketSize time.Duration) int64 {
	return int64(now.Sub(lastRotation)) / int64(bucketSize)
}

// TPS returns the current transactions per second.
func (t *tpsCounter) TPS() float64 {
	return t.tpsAt(time.Now())
}

func (t *tpsCounter) tpsAt(now time.Time) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rotateAtLocked(now)

	var sum float64
	for i := range t.buckets {
		sum += float64(t.buckets[i].Load())
	}

	if sum == 0 {
		return 0
	}

	// TPS = total count / monitored duration (len(buckets) * bucketSize)
	// This uses the actual monitored duration, not the configured windowSize.
	monitoredDuration := float64(len(t.buckets)) * t.bucketSize.Seconds()
	return sum / monitoredDuration
}

func atomicIncrementInt64(value *atomic.Int64) {
	for {
		old := value.Load()
		if old == math.MaxInt64 || value.CompareAndSwap(old, old+1) {
			return
		}
	}
}
