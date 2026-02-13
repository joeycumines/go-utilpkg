# Changelog

All notable changes to the `goja-eventloop` package will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **Phase 3 coverage tests**: Added `coverage_phase3_test.go`, `coverage_phase3b_test.go`,
  and `coverage_phase3c_test.go` with comprehensive tests for structuredClone (Date, RegExp,
  Map, Set, circular refs), Blob API edge cases, Headers initialization, console methods,
  Performance API, TextEncoder/TextDecoder, crypto.getRandomValues, EventTarget/CustomEvent,
  AbortSignal statics, DOMException constants, Symbol polyfill, FormData, localStorage,
  btoa/atob, direct Go method calls (formatCellValue, inspectValue, renderTable, etc.),
  timer functions on terminated loop, Promise method theft, AbortSignal.any with fake signals,
  AbortSignal.timeout with negative delay, type-detection on constructor-undefined objects,
  process.nextTick on terminated loop, and console.clear with nil output.
  Coverage: 96.3% → 97.8% (+1.5pp).

- **Adapter**: Core bridge between [goja](https://github.com/dop251/goja) JavaScript runtime
  and [go-eventloop](https://github.com/joeycumines/go-eventloop), providing 56+ Web Platform
  APIs as JavaScript globals via `Adapter.Bind()`.

- **Timer APIs**: `setTimeout`, `clearTimeout`, `setInterval`, `clearInterval`,
  `setImmediate`, `clearImmediate`, `queueMicrotask`, `delay(ms)`, `process.nextTick`.
  All timers delegate to the Go event loop scheduler with WHATWG-compliant negative delay
  clamping.

- **Promise/A+ implementation**: Event-loop-integrated `Promise` constructor replacing Goja's
  native implementation. Supports full specification including `.then()`, `.catch()`,
  `.finally()`, `Promise.resolve()`, `Promise.reject()`, `Promise.all()`, `Promise.race()`,
  `Promise.allSettled()`, `Promise.any()`.
  - **ES2024**: `Promise.withResolvers()` for deferred promise patterns.
  - **ES2025**: `Promise.try()` for wrapping synchronous/asynchronous calls.
  - `GojaWrapPromise()` public API for wrapping Go-side `*ChainedPromise` values into
    Goja-compatible JS objects with `.then()`, `.catch()`, `.finally()` methods.
  - Thenable adoption: objects with a `.then()` method are correctly resolved per Promise/A+
    §2.3.3.

- **AbortController / AbortSignal**: `new AbortController()` with `.abort(reason)` and
  `.signal` properties. Signal supports `.aborted`, `.reason`, `.onabort`,
  `.addEventListener("abort", fn)`, `.throwIfAborted()`.
  - `AbortSignal.any(signals)`: Composite signal that aborts when any input signal aborts.
  - `AbortSignal.timeout(ms)`: Signal that auto-aborts after the specified duration.

- **Performance API**: `performance.now()`, `performance.timeOrigin`,
  `performance.mark(name, options?)`, `performance.measure(name, start?, end?)`,
  `performance.getEntries()`, `performance.getEntriesByType(type)`,
  `performance.getEntriesByName(name, type?)`, `performance.clearMarks(name?)`,
  `performance.clearMeasures(name?)`, `performance.clearResourceTimings()`,
  `performance.toJSON()`. Supports `detail` option for marks and measures.

- **Console utilities**: `console.time`, `console.timeEnd`, `console.timeLog`,
  `console.count`, `console.countReset`, `console.assert`, `console.table`,
  `console.group`, `console.groupCollapsed`, `console.groupEnd`, `console.trace`,
  `console.clear`, `console.dir`. Output defaults to `os.Stderr`; configurable via
  `SetConsoleOutput(w)`.
  - Note: `console.log`, `console.warn`, `console.error`, `console.info`, and
    `console.debug` are intentionally not provided by this adapter.

- **Crypto**: `crypto.randomUUID()` (UUID v4), `crypto.getRandomValues(typedArray)` with
  65536-byte limit per WHATWG spec.

- **Encoding**: `new TextEncoder()` with `encode(string)` and `encodeInto(src, dest)`;
  `new TextDecoder(encoding?, options?)` with `fatal` and `ignoreBOM` options.
  `atob(encoded)` / `btoa(string)` for base64 encoding/decoding.

- **Events**: `new EventTarget()` with `addEventListener`, `removeEventListener`,
  `dispatchEvent`; `new Event(type, options?)` with `bubbles`, `cancelable`;
  `new CustomEvent(type, options?)` with `detail` property.

- **URL**: `new URL(url, base?)` WHATWG URL parser with all standard properties and
  `searchParams`; `new URLSearchParams(init?)` with `append`, `delete`, `get`, `getAll`,
  `has`, `set`, `sort`, `forEach`, `size`, iterator support.

- **Data structures**: `new Blob(parts?, options?)` with `size`, `type`, `text()`,
  `arrayBuffer()`, `slice()`; `new Headers(init?)` HTTP headers;
  `new FormData()` for form data.

- **Web Storage**: `localStorage` and `sessionStorage` (in-memory implementations) with
  `getItem`, `setItem`, `removeItem`, `clear`, `key`, `length`.

- **DOM compatibility**: `new DOMException(message?, name?)` with standard error code
  constants (INDEX_SIZE_ERR, HIERARCHY_REQUEST_ERR, INVALID_CHARACTER_ERR,
  NOT_SUPPORTED_ERR, INVALID_STATE_ERR, SYNTAX_ERR, TYPE_MISMATCH_ERR, SECURITY_ERR,
  NETWORK_ERR, ABORT_ERR, QUOTA_EXCEEDED_ERR, TIMEOUT_ERR, DATA_CLONE_ERR).

- **Structured cloning**: `structuredClone(value)` deep-clones objects, arrays, Date,
  RegExp, Map, Set. Throws on functions (matching browser behavior).

- **Symbol utilities**: `Symbol.for(key)` and `Symbol.keyFor(sym)` polyfilled if not
  natively available in the Goja runtime.

### Known Limitations

- `console.log`, `console.warn`, `console.error`, `console.info`, `console.debug` are
  **not provided** — supply your own or use Goja's built-in if available.
- `fetch()` is not implemented (Headers and FormData are provided for future use).
- `ReadableStream` / Streams API is intentionally omitted.
- `Worker` / `MessageChannel` — no threading model.
- `Intl` — partial support (limited by Goja).
- `File` / `FileReader` — not provided (use Blob).
- `WebSocket` — not provided.
- `Blob.stream()` returns `undefined` — use `text()` or `arrayBuffer()` instead.

### Thread Safety

The adapter coordinates thread safety between the non-thread-safe Goja runtime,
the event loop goroutine, and arbitrary Go goroutines. After calling `Bind()`,
JavaScript callbacks execute on the event loop goroutine. Access the Goja runtime
from within callbacks or via `loop.Submit()`, never concurrently from a separate
goroutine. Promise `resolve`/`reject` functions are thread-safe.

### Test Coverage

- **96.3% statement coverage** — Comprehensive test suite covering all 56+ Web APIs,
  promise combinators, structured cloning, crypto, encoding, URL parsing, storage,
  and event handling. Remaining ~3.7% is genuinely unreachable defensive code (nil checks
  after operations that never return nil, polyfill branches for features Goja supports
  natively, and type switch cases Goja's runtime never produces).
