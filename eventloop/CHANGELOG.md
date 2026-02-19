# Changelog

All notable changes to the `go-eventloop` package will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased] - 2026-02-10

### Fixed

- **Run() returns ErrLoopTerminated for StateTerminating**: Fixed a race condition where
  `Run()` would return `ErrLoopAlreadyRunning` instead of `ErrLoopTerminated` when
  `Shutdown()` won the scheduling race on Windows. The `Run()` function now checks for
  both `StateTerminating` and `StateTerminated` before returning `ErrLoopTerminated`.

### Added

- **Phase 3 coverage tests**: Added `coverage_phase3_test.go` (62 tests) and
  `coverage_phase3b_test.go` (21 tests) targeting deep coverage of pollFastMode,
  SubmitInternal, promise handlers, Promisify fallback paths, SetInterval/SetImmediate
  edge cases, transitionToTerminated draining, and p-square quantile bounds.
  Coverage: 97.0% → 97.8% (+0.8pp).

### Changed

- **BREAKING: ChainedPromise struct shrunk from 120B to 64B** — Major memory optimization
  through field removal and architectural changes:
  - Removed `toChannels []chan Result` (24B) — moved to `JS.toChannels` side table; direct
    synchronous notification during resolve/reject (no microtask queue dependency)
  - Removed `creationStack []uintptr` (24B) — moved to `JS.debugStacks` side table using
    `weak.Pointer[ChainedPromise]` as key for GC-safe automatic cleanup via `runtime.AddCleanup`
  - Removed `id atomic.Uint64` (8B) — replaced with pointer identity (`*ChainedPromise` as map
    key); eliminates global atomic counter and per-promise ID allocation
  - Go allocator benefit: 120B → 128B size class vs 64B → 64B size class = 64 bytes saved per
    promise from class rounding alone

- **API cleanup: unexported internal-only types** — Tightened the public API surface by
  unexporting symbols that were only used internally:
  - `FastState` → `fastState`
  - `ChunkedIngress` → `chunkedIngress`
  - `MicrotaskRing` → `microtaskRing`
  - `FastPoller` → `fastPoller`
  - `IOCallback` → `ioCallback`
  - `MaxFDLimit` → `maxFDLimit`
  - `EFD_CLOEXEC` → `efdCloexec`, `EFD_NONBLOCK` → `efdNonblock`
  - `TPSCounter` → `tpsCounter`
  - `ErrorWrapper` → `errorWrapper`
  - `ErrNoPromiseResolved` → `errNoPromiseResolved`
  - And several other internal-only types

- **GojaWrapPromise re-exported** — `Adapter.GojaWrapPromise()` confirmed as part of the
  public API surface for wrapping Go `*ChainedPromise` values into Goja-compatible JS objects
  with `.then()`, `.catch()`, `.finally()` methods.

### Fixed

- **`debugStacks` now uses `weak.Pointer` for GC-safe cleanup** — Creation stack traces
  (debug mode only) are stored in a `JS.debugStacks` side table keyed by
  `weak.Pointer[ChainedPromise]`. When a promise is garbage collected, `runtime.AddCleanup`
  automatically removes its stack trace entry, preventing memory leaks in long-running loops.

- **`promiseHandlers` orphan leak fixed** — Fixed a race condition where a promise could be
  garbage collected between handler registration and rejection tracking, leaving an orphaned
  entry in the `promiseHandlers` map. The fix ensures cleanup occurs in both the resolve/reject
  path and the handler attachment path.

---

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

### Added

- Windows platform support (IOCP poller, event handle wakeup)
- `crypto.getRandomValues()` (Web Crypto API subset)
- AbortController/AbortSignal (W3C DOM)
- Performance API (`performance.now()`, marks, measures)
- `Promise.withResolvers()` (ES2024)
- Console timer API (`console.time`, `console.timeEnd`, `console.timeLog`)
- O(1) streaming percentile estimation (P-Square algorithm)

### Changed

- Unexported internal-only types (`FastState` → `fastState`, etc.)

### Fixed

- Race conditions in timer callbacks, leak tests, workload tests, fuzz tests
- Restructured goja-eventloop tests to prevent concurrent `goja.Runtime` access
