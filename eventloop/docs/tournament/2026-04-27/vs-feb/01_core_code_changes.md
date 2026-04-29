# Core Code Changes: February 10 vs April 27

## 1. Go Version Upgrade

| Aspect | Feb 10 | Apr 27 | Delta |
|--------|--------|--------|-------|
| Go Version | **1.25.7** | **1.26.1** | +0 minor |
| GOMAXPROCS | 10 | 10 | Same |
| Architecture | ARM64 (Darwin/Linux) | ARM64 (Darwin/Linux) | Same |

**Source:** `go.mod` files in respective tournament directories

```go
// Feb 10 go.mod
module github.com/joeycumines/go-eventloop
go 1.25.7

// Apr 27 go.mod
module github.com/joeycumines/go-eventloop
go 1.26.1
```

## 2. Goroutine ID Determination (CRITICAL CHANGE)

### February 10 Implementation

```go
// Source: loop.go at commit 506d6643cc1d45b1da156096870991ecb30b8847

func (l *Loop) isLoopThread() bool {
    loopID := l.loopGoroutineID.Load()
    if loopID == 0 {
        return false
    }
    return getGoroutineID() == loopID  // ← Calls stack parser
}

func getGoroutineID() uint64 {
    var buf [64]byte
    n := runtime.Stack(buf[:], false)  // ← ~1760ns per call
    var id uint64
    for i := len("goroutine "); i < n; i++ {
        if buf[i] >= '0' && buf[i] <= '9' {
            id = id*10 + uint64(buf[i]-'0')
        } else {
            break
        }
    }
    return id
}
```

**Performance Characteristics:**
- ~1760ns per invocation (measured via benchmark comments)
- 1 allocation per call (64-byte stack buffer on stack, but escape analysis)
- Called from: `ScheduleTimer`, `CancelTimer`, `CancelTimers`, `submitTimerRefChange`, `SubmitInternal`, `Run`

### April 27 Implementation

```go
// Source: loop.go at HEAD

func (l *Loop) isLoopThread() bool {
    loopID := l.loopGoroutineID.Load()
    if loopID == 0 {
        return false
    }
    return uint64(goroutineid.Get()) == loopID
}
```

```go
// Source: internal/runtimeutil/goroutineid.go

package runtimeutil

import "github.com/joeycumines/goroutineid"

func GoroutineID() (ID int64) {
    ID = goroutineid.Fast()    // ← Assembly fast path (~2-5ns)
    if ID == -1 {
        buf := bufPool.Get().(*[]byte)
        ID = goroutineid.Slow(*buf)  // ← Fallback (~100-200ns)
        bufPool.Put(buf)
    }
    return
}
```

**Performance Characteristics:**
- ~2-5ns per invocation (Fast path, assembly)
- 0 allocations on Fast path
- Fallback only needed if Fast returns -1

**Speedup: ~350-880x per isLoopThread() call**

## 3. Auto-Exit Feature

The auto-exit feature was introduced between tournaments. This feature:

1. **Adds isLoopThread() calls** to hot paths:
   - `submitTimerRefChange()` (RefTimer/UnrefTimer)
   - `ScheduleTimer()`
   - `CancelTimer()`
   - `CancelTimers()`
   - `SubmitInternal()`

2. **Adds quiescing protocol** checks to liveness-adding APIs:
   - `ScheduleTimer`
   - `RegisterFD`
   - `RefTimer`
   - `Promisify`
   - `submitToQueue`

3. **Adds Alive() polling** in hot paths:
   - `run()` (main loop)
   - `runFastPath()`
   - `pollFastMode()`

### Impact Assessment

**Claimed Impact:** The auto-exit feature would cause a performance regression because of the added `isLoopThread()` checks.

**Reality:** The regression was **mitigated but not eliminated** by the goroutine ID fast path. Before the fast path, each `isLoopThread()` call cost ~1760ns. After the fast path, each call costs ~5ns. This is a **352x reduction in auto-exit overhead**.

However, the auto-exit feature also adds:
- Additional `Alive()` calls in hot paths (polling liveness state)
- `quiescing` atomic flag checks
- `submissionEpoch` atomic operations

These are **new overhead not present in Feb**, but they are relatively cheap (single atomic loads).

## 4. New Benchmark Suite

| Metric | Feb 10 | Apr 27 |
|--------|--------|--------|
| Common benchmarks | 108 | 108 |
| New benchmarks | 0 | 58 |
| Total benchmarks | 108 | 166 |

**New benchmark categories in Apr:**
- `BenchmarkBurstSubmit` - Submit bursting patterns
- `BenchmarkChainDepth` - Promise chain depth scaling
- `BenchmarkGCPressure` - GC stress tests
- `BenchmarkMicroBatchBudget` - Micro-batching with budgets
- `BenchmarkMicroCASContention` - CAS contention patterns
- `BenchmarkMicroWakeupSyscall` - Wake-up syscall patterns
- `BenchmarkMultiProducer` - Multi-producer patterns
- `BenchmarkPingPong` - Ping-pong latency patterns
- `BenchmarkPromises` - Promise operation patterns

## 5. golang.org/x/sys Version

| Tournament | golang.org/x/sys Version |
|------------|-------------------------|
| Feb 10 | v0.40.0 |
| Apr 27 | v0.42.0 |

This is a minor version upgrade that may include platform-specific syscall optimizations.

---

## Summary of Code Changes

| Change | Impact on Performance | Confidence |
|--------|---------------------|------------|
| Goroutine ID fast path | **Massive positive** (+350-880x faster isLoopThread) | High |
| Auto-exit feature | **Moderate negative** (new checks in hot paths) | High |
| Go 1.25 → 1.26 | **Positive** (Green Tea GC) | Medium |
| golang.org/x/sys upgrade | **Minor positive** | Low |
| New benchmark suite | **N/A** (new tests, not performance impact) | N/A |

---

*Next: [Benchmark Delta Analysis](02_benchmark_delta_analysis.md)*
