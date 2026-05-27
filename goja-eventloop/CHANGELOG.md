# Changelog

All notable changes to the `goja-eventloop` package will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Core bridge between [goja](https://github.com/joeycumines/goja) JavaScript runtime
  and [go-eventloop](https://github.com/joeycumines/go-eventloop), providing a
  version-pinned selected host profile via `Adapter.Bind()`
- Timer APIs: `setTimeout`, `clearTimeout`, `setInterval`, `clearInterval`,
  `setImmediate`, `clearImmediate`, `queueMicrotask`, `delay(ms)`, `process.nextTick`
- Goja native Promise integration with event-loop-routed native promise jobs,
  Node-profile wrappers for `Promise.all()`, `Promise.race()`,
  `Promise.allSettled()`, and `Promise.any()`, and conditional host additions
  for `Promise.withResolvers()` and `Promise.try()`
- Owner-safe `Adapter.Submit()` and `Adapter.NewPromise()` bridges for
  asynchronous Go integrations
- `Adapter.Done()` exposes the exact underlying loop terminal-cleanup signal
  without exposing the loop itself
- Owner-only `Adapter.TrackAbortSignal()` for host cancellation integration,
  with idempotent cleanup safe from any goroutine
- AbortController / AbortSignal with `.any(signals)` and `.timeout(ms)`
- High-resolution time (`performance.now()` and `performance.timeOrigin`)
- Console utilities (`console.time`, `.count`, `.assert`, `.table`, `.group`, `.trace`, etc.)
- Crypto: `crypto.randomUUID()`, `crypto.getRandomValues(typedArray)`
- `atob()` / `btoa()`
- EventTarget, Event, CustomEvent
- DOMException with standard error code constants
- `structuredClone(value, options?)` deep-clone for objects, arrays, Date,
  RegExp, Map, Set, Error, ArrayBuffer, DataView, TypedArray, circular
  references, and ArrayBuffer transfer
- Machine-readable exposed-surface manifest with authenticated fixture bytes
  and canonical Node.js v26.5.0 / Goja observation comparison

### Changed

- `New` returns errors for nil loop, nil runtime, and invalid JS options before
  claiming the runtime or event loop; construction validation failures are
  never panics
- `Adapter.Bind()` documents and enforces the fetch boundary: it does not install
  a `fetch` stub and preserves any host-provided `fetch` binding.
- Construction now claims one exact loop/runtime/adapter identity. Binding
  prepares a reversible installation, final-preflights ordinary target
  identities, descriptors, and extensibility, then performs one callback-free
  lifecycle commit. Defensive commit failures restore attempted definitions;
  native panic or `runtime.Goexit` from restoration itself may interrupt the
  remaining rollback.
- `structuredClone` now throws `DOMException` `DataCloneError` for functions,
  symbols, duplicate or unsupported transfer entries, and non-cloneable values
  instead of silently dropping function-valued properties.
- RegExp cloning preserves every flag implemented by the exact linked Goja
  engine (`gimsuy`); unsupported `d` and `v` engine features are not
  approximated.
- Timer, immediate, `process.nextTick`, native Promise job, `beforeExit`, and
  `exit` behavior is now documented and tested against exact Node.js v26.5.0
  for the declared Node profile; AbortSignal, EventTarget, and other retained
  Web behavior is pinned to declared standards revisions.
- Timeout handle lifecycle state now follows Node-shaped `_idleTimeout`,
  `refresh()`, `close()`, and `[Symbol.dispose]()` behavior for active,
  inactive, executing, and already-fired one-shot handles.
- The adapter inherits the `go-eventloop` owner-topology scheduler: native Goja
  Promise jobs, microtasks, timers, and immediates run through owner-local queues
  and typed command ingress instead of legacy ring/heap hot paths.

### Fixed

- The `delay(ms)` extension now uses architecture-independent bounded
  millisecond coercion and an owner-confined exactly-once bridge, so oversized
  durations cannot wrap and terminal cleanup cannot leave its native Promise
  pending.
- Adapter diagnostics now use the role-preserving `Loop.Log` boundary, including
  direct nil-safe logger invocation and panic / `runtime.Goexit` containment.
  Disabled or failing instance loggers drop the diagnostic without violating
  instance scope through a process-global standard-library fallback.
- Direct Goja exceptions raised inside `Adapter.Submit()` now enter the
  Node-style uncaught-exception path without exposing runtime-owned values to
  core logging; native Go panics retain the event loop callback-panic contract.
- Node-coded errors and `performance.toJSON()` now create their own data
  properties without invoking inherited setters. A throwing replacement for
  the configurable `process._exiting` property aborts explicit `process.exit()`
  before terminal state is committed while preserving the exact thrown value.

- AbortSignal and EventTarget listener cleanup now uses adapter-owned internal
  algorithms where needed, so hostile abort listeners cannot suppress host cleanup.
- Native Promise job and Goja promise-chain benchmarks now surface production
  enqueuer errors instead of hanging behind package timeouts.

### Removed

- Nonconformant standard-branded stand-ins for URL/URLSearchParams,
  TextEncoder/TextDecoder, Blob, Headers, FormData, Web Storage, and User
  Timing/Performance Timeline. `Adapter.Bind()` preserves a host implementation
  of these names instead of replacing it.
- The leaked `consumeIterable` global and ownership-sensitive `Adapter.Loop()`,
  `Adapter.Runtime()`, `Adapter.JS()`, and `Adapter.GojaWrapPromise()` accessors.
  Their owner-safe replacements are mapped in the
  [migration guide](docs/migration.md#migrating-removed-adapter-accessors).
