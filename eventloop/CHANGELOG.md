# Changelog

All notable changes to the `go-eventloop` package will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased] - 2026-02-09

### Added

- **JS.Timeout(delay time.Duration)**: New convenience method that returns a promise
  rejecting with `TimeoutError` after the specified delay. Companion to `JS.Sleep()`.
  Useful with `JS.Race()` for implementing operation timeouts.

- **fetch() stub with clear error message**: Added `fetch()` binding that returns a
  rejected promise with a helpful error message explaining that fetch is not implemented
  and suggesting alternatives (expose custom Go functions via net/http).

- **localStorage/sessionStorage limitation documentation**: Added prominent godoc
  comments (⚠️ IMPORTANT LIMITATION) explaining that storage is in-memory only,
  not persisted to disk, and listing unsupported browser features (storage events,
  size limits, cross-origin isolation).

- **New coverage tests**: Added `loop_coverage_extra_test.go` with 11 tests covering
  ScheduleNextTick edge cases, SubmitInternal I/O mode, RunTimers strict ordering,
  OnOverload callbacks, ProcessExternal nextTick priority, and PSquare edge cases.

### Changed

- **BREAKING: ChainedPromise memory optimization** - Struct redesigned for performance:
  - `id` field now lazy-allocated (atomic.Uint64 with 0 sentinel value)
  - `channels` field renamed to `toChannels` and is slice (not pre-allocated)
  - ToChannel() optimized for immediate notification (no event loop needed)
  - Result: 8 bytes saved for promises that don't need tracking
  - External code accessing `p.id` directly must use new `p.getID()` method

### Performance Improvements

- **Promise tournament**: ChainedPromise achieves 2nd place (362.5 ns/op)
  - 36.7% faster than PromiseAltFive (362.5 vs 570.8 ns/op)
  - Competitive with PromiseAltOne (420.9 ns/op) and PromiseAltTwo (533.4 ns/op)
  - Full tournament results: `/Users/joeyc/dev/go-utilpkg/internal/tournament/RESULTS-2026-02-09.md`

- ChainedPromise memory footprint reduced through lazy ID allocation:
  - Promises without rejection tracking: 8 bytes saved (id field uses 0 sentinel)
  - Promises without ToChannel: toChannels slice not allocated (memory savings only when needed)
  - Thread-safe lazy ID allocation using atomic.CompareAndSwap (no mutex contention)
  - Actual struct size: 120 bytes (measured via unsafe.Sizeof)

### Fixed

- **WHATWG timer negative delay compliance (goja-eventloop)**: Timer functions
  (`setTimeout`, `setInterval`, `delay`) now clamp negative delays to 0 per WHATWG
  HTML Spec Section 8.6, instead of throwing TypeError. This matches browser behavior.

---

## [Previous] - 2026-02-07

### 🎉 Major New Features

#### Windows Platform Support
Removed `//go:build linux || darwin` constraints from all core source files and the
goja-eventloop adapter. The event loop now compiles and runs on all three major
platforms:

| Platform | I/O Poller | Wakeup Mechanism |
|----------|-----------|------------------|
| Linux | epoll | eventfd |
| macOS | kqueue | pipe |
| Windows | IOCP | event handle |

- Removed build tags from: `loop.go`, `errors.go`, `abort.go`, `eventtarget.go`,
  `performance.go`, `goja-eventloop/adapter.go`, and 53+ goja-eventloop test files
- Added `EFD_CLOEXEC` / `EFD_NONBLOCK` stubs in `wakeup_windows.go`
- Cross-compilation verified for all 6 target combinations (GOOS × package)

#### crypto.getRandomValues() (Web Crypto API)
Added `crypto.getRandomValues(typedArray)` to the goja-eventloop adapter:

- Fills any integer TypedArray with cryptographically random values via Go's `crypto/rand`
- Supports: `Uint8Array`, `Uint16Array`, `Uint32Array`, `Int8Array`, `Int16Array`,
  `Int32Array`, `Uint8ClampedArray`
- Throws `TypeError` for non-TypedArray input or `Float32Array`/`Float64Array`
- Throws `QuotaExceededError` `DOMException` when `byteLength > 65536`
- Returns the same TypedArray passed in (per spec)
- 22 tests in `crypto_getrandomvalues_test.go`

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
- **Main Package:** 93.3% statement coverage
- **goja-eventloop:** 88.6%+ statement coverage

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
3. `leak_test.go` - fixed callCount (atomic.Int32) and id (atomic.Uint64) races
4. `workload_test.go` - fixed flushTimerID race with atomic.Uint64
5. `promise_combinator_fuzz_test.go` - fixed 4 shared `math/rand.Rand` sites with pre-computed delays
6. `goja-eventloop` - restructured 28+ tests to prevent concurrent `goja.Runtime` access
   (runtime is NOT thread-safe; all access must occur on a single goroutine)

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

#### Cross-Platform Architecture
The event loop runs natively on all three major platforms with platform-optimized I/O polling:

| Platform | Architecture | I/O Poller | Wakeup Mechanism | Status |
|----------|-------------|-----------|------------------|--------|
| Linux | amd64, arm64 | epoll | eventfd | ✅ Full support |
| macOS | amd64, arm64 | kqueue | pipe | ✅ Full support |
| Windows | amd64 | IOCP | event handle | ✅ Full support |

Platform-specific source files:
- `poller_linux.go` / `poller_darwin.go` / `poller_windows.go` — I/O polling
- `wakeup_linux.go` / `wakeup_darwin.go` / `wakeup_windows.go` — Cross-goroutine wake
- `fd_unix.go` / `fd_windows.go` — File descriptor abstractions

#### Cross-Platform CI
Verified via GitHub Actions matrix on:
- `ubuntu-latest` (Linux/epoll)
- `macos-latest` (Darwin/kqueue)
- `windows-latest` (Windows/IOCP)

#### Race Detector Verification
- All tests pass with `-race` flag on macOS and Linux
- Zero data races in both `eventloop` and `goja-eventloop` packages

---

## Coverage Report Summary

| Package | Coverage | Notes |
|---------|----------|-------|
| eventloop (main) | 93.3% | Production code |
| goja-eventloop | 88.6%+ | JS adapter layer |
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
5. **Blob.stream():** Returns `undefined` — ReadableStream (WHATWG Streams API) is intentionally not implemented due to complexity; use `blob.text()` or `blob.arrayBuffer()` instead
6. **Intl:** Only basic number/date formatting via Goja; full ICU support not available

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
