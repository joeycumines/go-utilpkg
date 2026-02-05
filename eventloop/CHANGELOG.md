# Changelog

All notable changes to the `go-eventloop` package will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased] - 2026-02-07

### 🎉 Major New Features

#### AbortController/AbortSignal Support
Full W3C DOM-compliant implementation of the AbortController API for cancellation patterns:

- **AbortController**
  - `NewAbortController()` - Create new controller instance
  - `controller.Abort()` - Trigger abort signal
  - `controller.AbortWithReason(reason)` - Abort with custom reason
  - `controller.Signal()` - Access the associated AbortSignal

- **AbortSignal**
  - `signal.Aborted()` - Check if aborted
  - `signal.Reason()` - Get abort reason
  - `signal.OnAbort(callback)` - Register abort handler
  - `AbortSignalAbort()` - Create pre-aborted signal
  - `AbortSignalTimeout(duration)` - Create timeout-based signal

#### Performance API (W3C High Resolution Time Level 3)
Complete performance measurement API implementation:

- **performance.now()** - High-resolution monotonic timestamp (sub-millisecond precision)
- **performance.timeOrigin** - Time origin for the loop instance
- **performance.mark(name)** - Create named performance marks
- **performance.measure(name, startMark, endMark)** - Measure between marks
- **performance.getEntries()** - Get all performance entries
- **performance.getEntriesByType(type)** - Filter by "mark" or "measure"
- **performance.getEntriesByName(name)** - Filter by entry name
- **performance.clearMarks()** / **performance.clearMeasures()** - Clear entries

#### Promise.withResolvers() (ES2024)
ES2024-compliant Promise.withResolvers() implementation:
```go
resolvers := js.WithResolvers()
// resolvers.Promise, resolvers.Resolve, resolvers.Reject
```

#### Console Timer API
Node.js/Chrome-compatible console timer methods:
- `console.time(label)` - Start a named timer
- `console.timeEnd(label)` - Stop timer and log duration
- `console.timeLog(label, ...data)` - Log elapsed time without stopping

### 🚀 Performance Improvements

#### O(1) Streaming Percentile Estimation
Replaced O(n log n) sort-based percentile calculation with P-Square algorithm:
- **Before:** `LatencyMetrics.Sample()` sorted 1000 samples every call
- **After:** O(1) incremental update with <5% relative error for P99
- Files: `psquare.go`, `metrics.go` (modified)

#### Zero-Allocation Hot Paths (Verified)
Allocation profiling confirms:
| Component | Allocations | Notes |
|-----------|-------------|-------|
| MicrotaskRing | 0 allocs/op | Ring buffer with sequence numbers |
| ChunkedIngress | 0 allocs/op | Chunk pooling eliminates allocations |
| SubmitFastPath | <2 allocs/op | Amortized slice growth |
| Timer Scheduling | ~7 allocs/op | Due to CancelTimer result channel |

### ✅ Standards Compliance

#### Promise/A+ Specification
24 tests covering Promise/A+ spec sections 2.1-2.3:
- State transitions (pending→fulfilled/rejected)
- Then method requirements (nil callbacks, async execution, chaining)
- Resolution procedure (self-resolution TypeError, promise adoption)

**Intentional Deviation:** Section 2.3.3 (thenable handling) only supports `ChainedPromise`, not arbitrary objects with `Then` methods. This is due to Go's type system constraints.

#### HTML5 Timers Specification
21 tests covering HTML Living Standard timer behavior:
- Zero/negative delay handling
- Nested timeout clamping (4ms minimum after depth 5)
- Timer ID uniqueness and namespacing
- Self-clearing intervals

**Documented Deviations:**
1. `setTimeout`/`setInterval` use separate ID namespaces
2. Same-time timers not strictly FIFO (heap implementation)

#### WHATWG Microtask Queue
12 tests verifying microtask ordering:
- Microtasks run before next macro-task
- Nested microtasks processed in same checkpoint
- Promise reactions are microtasks

**Documented Deviation:** Microtasks queued during timer callbacks may execute after subsequent timers (implementation-specific optimization for performance).

#### Promise Combinator Edge Cases
25 tests covering combinator semantics:
- `Promise.all` - Empty array resolves to [], order preserved, first rejection wins
- `Promise.race` - Empty array pending forever, short-circuit behavior
- `Promise.allSettled` - Never rejects, returns all outcomes
- `Promise.any` - AggregateError for all rejections, first fulfillment wins

### 🔧 Coverage Improvements

#### eventloop Package
- **Main Package:** 77.9% statement coverage
- **goja-eventloop:** 88.6% statement coverage

Major coverage additions:
- 33 tests for Darwin/Linux poller (poller_darwin_full_coverage_test.go)
- 21 tests for timer operations (timer_coverage_test.go)
- 16 tests for promise Finally handling (promise_finally_coverage_test.go)
- 16 tests for promise rejection tracking (promise_trackrejection_coverage_test.go)
- 15 tests for ChunkedIngress (chunkedingress_coverage_test.go)
- 15 tests for MicrotaskRing (microtaskring_coverage_test.go)
- 14 tests for Promisify edge cases (promisify_coverage_test.go)
- 12 tests for pollFastMode (pollfastmode_coverage_test.go)
- 11 tests for ScheduleMicrotask (schedulemicrotask_coverage_test.go)
- 10 tests for handlePollError (handlepollerror_coverage_test.go)
- 20+ tests for Promise thenStandalone (promise_thenstandalone_coverage_test.go)
- 20+ tests for Registry (registry_coverage_test.go)
- 20+ tests for FastState (state_coverage_test.go)
- 20+ tests for JS timer race conditions (js_interval_race_coverage_test.go)
- 20+ tests for SetImmediate/ClearImmediate (js_immediate_coverage_test.go)

#### Intentionally Uncovered Paths

1. **Windows IOCP Code** (`poller_windows.go`)
   - Cannot test on darwin/linux platforms
   - Requires Windows CI environment
   - Tests exist in `poller_windows_test.go` but can only run on Windows

2. **Internal Tournament Packages** (`internal/promisealt*`)
   - Experimental promise implementation variants for performance comparison
   - Coverage ranges 34-74% (not production code)
   - Serve as baselines for optimization testing

3. **Example Programs** (`examples/`)
   - Executable examples, no test files
   - Demonstrate API usage patterns

### 🧪 Testing & Quality

#### Comprehensive Test Suite
- 300+ tests across eventloop package
- 50+ tests in goja-eventloop package
- Stress tests with 1000+ concurrent operations
- Race detector verification (all races fixed)

#### Integration Tests
- 20 comprehensive integration tests for Go/JS boundary
- Memory leak detection suite (8 tests)
- Real-world workload simulations (8 tests)

#### Race Conditions Fixed
1. `TestHTML5_ClearIntervalFromCallback` - fixed with atomic.Uint64
2. `TestMicrotaskOrdering_IntervalInteraction` - fixed with atomic.Uint64 + sync.Mutex

#### Benchmark Suite
35+ benchmarks covering:
- Timer scheduling/firing (4 benchmarks)
- Microtask throughput (6 benchmarks)
- Promise creation/resolution (5 benchmarks)
- Submit operations (4 benchmarks)
- Combined workloads (3 benchmarks)
- Memory pressure (2 benchmarks)

### 📚 Documentation

#### Architecture Documentation
- `eventloop/docs/ARCHITECTURE.md` - 491 lines covering:
  - ASCII component diagram
  - Thread model (single-threaded + external queue)
  - State machine diagram
  - Platform-specific poller documentation
  - Performance characteristics

#### Migration Guide
- `goja-eventloop/docs/MIGRATION.md` - 532 lines covering:
  - Quick start guide
  - Differences from Node.js
  - Common patterns (fetch-like, debounce, task queue)
  - Thread safety guidelines

#### Examples
- `eventloop/examples/01_basic_usage/` - Loop creation, Submit, timers
- `eventloop/examples/02_promises/` - Promise patterns, chaining, combinators
- `eventloop/examples/03_timers/` - setTimeout, setInterval, debouncing
- `eventloop/examples/04_shutdown/` - Graceful shutdown patterns

### 🔒 Thread Safety

All major public types now have explicit thread safety documentation:
- `Loop` - Detailed list of concurrent-safe vs single-use methods
- `JS` - Safe for concurrent use from multiple goroutines
- `ChainedPromise` - Safe for concurrent use
- `Metrics`, `TPSCounter` - All public methods thread-safe
- `AbortSignal`, `AbortController` - Safe for concurrent access
- `PromiseWithResolvers` - All fields safe for concurrent use

### 🛡️ Panic Safety

All 10 callback execution sites verified to have panic recovery:
- `runTimers()`, `processInternalQueue()`, `processExternal()`
- `drainMicrotasks()`, `drainAuxJobs()`, `runAux()`
- `transitionToTerminated()`, `shutdown()`
- Promise handlers (`tryCall()`)
- Goja-eventloop callbacks (Goja panic convention)

### 🖥️ Platform Support

#### Cross-Platform CI
Verified via GitHub Actions matrix on:
- `ubuntu-latest` (Linux/epoll)
- `macos-latest` (Darwin/kqueue)
- `windows-latest` (Windows/IOCP)

#### Platform-Specific Pollers
- **Darwin:** kqueue-based FastPoller
- **Linux:** epoll-based FastPoller
- **Windows:** IOCP-based FastPoller

---

## Coverage Report Summary

| Package | Coverage | Notes |
|---------|----------|-------|
| eventloop (main) | 77.9% | Production code |
| goja-eventloop | 88.6% | JS adapter layer |
| internal/alternateone | 69.5% | Experimental |
| internal/alternatethree | 72.9% | Experimental |
| internal/alternatetwo | 72.7% | Experimental |
| internal/promisealtfour | 34.0% | Baseline snapshot |
| internal/promisealtone | 54.7% | Experimental |
| internal/promisealtthree | 74.4% | Experimental |
| internal/promisealttwo | 73.3% | Experimental |

---

## Known Limitations

1. **Thenable Resolution:** Only `ChainedPromise` is recognized as thenable, not arbitrary Go types with Then methods
2. **Timer Ordering:** Same-delay timers not guaranteed FIFO (heap-based scheduling)
3. **Windows Testing:** Windows poller tests require Windows platform
4. **ArrayBuffer:** Promise results cannot be ArrayBuffer (Goja limitation)

---

## Migration Notes

### From Previous Versions
- No breaking changes in this release
- All existing APIs remain compatible
- New APIs are purely additive

### Upgrading
```bash
go get -u github.com/joeycumines/go-eventloop@latest
go get -u github.com/joeycumines/goja-eventloop@latest
```
