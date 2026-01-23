# Group 2: Performance & Metrics Exhaustive Review

## Succinct Summary
The Metrics system contains **CRITICAL** concurrency defects that compromise data integrity: `TPSCounter.rotate()` suffers from a read-modify-write race condition causing double-shifting of buckets, and `Loop.Metrics()` reads nested structure fields unsynchronized, leading to torn reads and potential panics. Furthermore, `LatencyMetrics.Sample()` utilizes an O(N²) bubble sort that degrades event loop performance, and `TPSCounter.Increment()` introduces a high-contention mutex bottleneck in the hot path. The component is **NOT CORRECT** and requires immediate remediation.

## Detailed Analysis

### 1. TPSCounter Race Condition (CRITICAL)
**Location**: `eventloop/metrics.go:270-296` (`rotate`)
**Issue**: The `lastRotation` logic checks time and calculates `bucketsToAdvance` **outside** the mutex lock.
```go
lastRotation := t.lastRotation.Load().(time.Time)
if bucketsToAdvance > 0 {
    t.mu.Lock()
    // ... shift buckets ...
    t.mu.Unlock()
    t.lastRotation.Store(...)
}
```
If two goroutines call `Increment()` (which calls `rotate()`) concurrently:
1. Both load same `lastRotation`.
2. Both calculate `bucketsToAdvance` > 0.
3. G1 locks, shifts buckets, releases.
4. G2 locks, shifts buckets **AGAIN**, releases.
**Impact**: Bucket data is shifted twice for the same time interval, destroying TPS accuracy.
**Fix**: Move `lastRotation` check and update INSIDE the mutex critical section.

### 2. Metrics() Data Races (MEDIUM)
**Location**: `eventloop/loop.go:1542` (`Metrics`)
**Issue**: `Metrics()` creates a snapshot by copying fields from `l.metrics`.
```go
snapshot := &Metrics{
    Queue: QueueMetrics{
        IngressCurrent: l.metrics.Queue.IngressCurrent, // RACE
    },
    // ...
}
```
`l.metrics.mu` is held, but `QueueMetrics` is protected by `l.metrics.Queue.mu` (a different lock). `UpdateIngress` acquires `Queue.mu` but not `Metrics.mu`. Thus, `Metrics()` reads fields concurrently with writes.
Similarly for `LatencyMetrics`: `Record()` modifies `samples` and `Sum` under `Latency.mu`. `Metrics()` copies them without holding `Latency.mu` (it releases the lock acquired by `Sample()` before copying).
**Impact**: Torn reads, inconsistent snapshots (Sum doesn't match samples).
**Fix**: `Metrics()` must acquire `Queue.mu.RLock()` and `Latency.mu.RLock()` while copying respective fields.

### 3. Latency Sorting Performance (MEDIUM)
**Location**: `eventloop/metrics.go:107` (`Sample`)
**Issue**: Implementation uses an O(N²) bubble sort (manual swap loop). behavior.
```go
for i := 0; i < count; i++ {
    for j := i + 1; j < count; j++ { ... }
}
```
For N=1000, this performs ~500,000 comparisons inside a lock. While Go is fast, this burns CPU unnecessarily.
**Fix**: Use `sort.Slice` (O(N log N)) or `slices.Sort` if available.

### 4. TPSCounter Contention (LOW)
**Location**: `eventloop/metrics.go:262` (`Increment`)
**Issue**: `Increment()` acquires a Mutex for *every* event.
```go
t.mu.Lock()
t.buckets[len(t.buckets)-1]++
t.mu.Unlock()
```
This serializes all event loop tasks if many are processed quickly.
**Fix**: Consider minimizing lock scope or using atomic counters for the current bucket if possible. Given time constraints, fixing the Critical Race is higher priority, but this remains a perf bottlenecks.

### 5. Loop Optimizations (Verified)
- **Timer Pooling**: `sync.Pool` usage is correct. `heapIndex` and `canceled` are reset properly.
- **WakeBuf**: `[8]byte` buffer on stack/struct avoids allocation. Correct.
- **RunFastPath**: Zero-allocation select and swap. Correct.

## Action Plan
1.  **Fix TPSCounter Race**: Refactor `rotate` to handle state updates atomically under lock.
2.  **Fix Metrics() Races**: locking nested mutexes during snapshot.
3.  **Optimize Sort**: Replace bubble sort with `sort.Slice`.
4.  **Verify**: Run `go test -race`.
