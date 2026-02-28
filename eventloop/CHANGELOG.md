# Changelog

All notable changes to the `go-eventloop` package will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **JS.Timeout(delay time.Duration)**: New convenience method that returns a promise
  rejecting with `TimeoutError` after the specified delay. Companion to `JS.Sleep()`.
  Useful with `JS.Race()` for implementing operation timeouts.

- **fetch() stub with clear error message**: Added `fetch()` binding that returns a
  rejected promise with a helpful error message explaining that fetch is not implemented
  and suggesting alternatives.

- **localStorage/sessionStorage stubs**: In-memory storage API for compatibility. Not
  persisted to disk.

### Changed

- **BREAKING: ChainedPromise struct shrunk from 120B to 64B** — Field removal and
  architecture changes: `toChannels`, `creationStack`, and `id` fields moved to
  side tables on the `JS` struct, keyed by `weak.Pointer[ChainedPromise]` for
  GC-safe automatic cleanup via `runtime.AddCleanup`.

- **API cleanup: unexported internal-only types** — Tightened the public API surface by
  unexporting symbols that were only used internally: `FastState` → `fastState`,
  `ChunkedIngress` → `chunkedIngress`, `MicrotaskRing` → `microtaskRing`,
  `FastPoller` → `fastPoller`, `IOCallback` → `ioCallback`, `MaxFDLimit` → `maxFDLimit`,
  `TPSCounter` → `tpsCounter`, `ErrorWrapper` → `errorWrapper`, and others.

- **GojaWrapPromise re-exported** — `Adapter.GojaWrapPromise()` confirmed as part of the
  public API surface for wrapping Go `*ChainedPromise` values into Goja-compatible JS objects.

### Fixed

- **Run() returns ErrLoopTerminated for StateTerminating**: Fixed a race condition where
  `Run()` would return `ErrLoopAlreadyRunning` instead of `ErrLoopTerminated` when
  `Shutdown()` won the scheduling race on Windows.

- **`debugStacks` now uses `weak.Pointer` for GC-safe cleanup** — Creation stack traces
  (debug mode only) stored in a side table with automatic cleanup when promises are
  garbage collected.

- **`promiseHandlers` orphan leak fixed** — Fixed a race condition where a promise could be
  garbage collected between handler registration and rejection tracking.

- **WHATWG timer negative delay compliance**: Timer functions now clamp negative delays to 0
  per WHATWG HTML Spec Section 8.6, instead of throwing TypeError.

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
