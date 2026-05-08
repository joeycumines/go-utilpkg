# goja-eventloop

Package goja-eventloop provides bindings between [go-eventloop](https://pkg.go.dev/github.com/joeycumines/go-eventloop) and the [Goja](https://github.com/joeycumines/goja) JavaScript runtime.

See the [API docs](https://pkg.go.dev/github.com/joeycumines/goja-eventloop).

## Features

- **Version-pinned host profile** — exact Node.js v26.5.0 behavior for the
  declared event-loop surface and revision-pinned contracts for retained Web APIs
- **Goja native Promise integration** with native promise jobs routed through the event loop's microtask queue
- **ES2024/ES2025** features: `Promise.withResolvers()`, `Promise.try()`
- **Atomic ownership and installation** — one runtime/loop adapter identity and
  reversible global binding with documented native-panic rollback limits
- **Platform honest** — inherits go-eventloop's declared targets and reports
  unsupported public FD readiness instead of approximating it
- **Owner-safe handoff** — Goja values stay under the logical adapter owner
  while Go workers submit Go-only outcomes

## Installation

```bash
go get github.com/joeycumines/goja-eventloop
```

## Quick Start

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "log"
    "time"

    "github.com/joeycumines/goja"
    eventloop "github.com/joeycumines/go-eventloop"
    gojaeventloop "github.com/joeycumines/goja-eventloop"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    loop := eventloop.New(eventloop.WithAutoExit(true))
    defer func() {
        if err := loop.Close(); err != nil && !errors.Is(err, eventloop.ErrLoopTerminated) {
            log.Printf("close event loop: %v", err)
        }
    }()
    runtime := goja.New()
    if err := runtime.Set("print", fmt.Println); err != nil {
        log.Fatal(err)
    }

    adapter, err := gojaeventloop.New(loop, runtime)
    if err != nil {
        log.Fatal(err)
    }
    if err := adapter.Bind(); err != nil {
        log.Fatal(err)
    }

    if _, err := runtime.RunString(`
        setTimeout(() => print("Hello after 100ms!"), 100);
        queueMicrotask(() => print("Microtask runs first"));
        new Promise(resolve => resolve(42))
            .then(v => print("Promise:", v));
    `); err != nil {
        log.Fatal(err)
    }

    if err := loop.Run(ctx); err != nil && ctx.Err() == nil {
        log.Fatal(err)
    }
}
```

See [docs/migration.md](docs/migration.md) for migration guidance and diagrams
showing the adapter setup, callback routing, and Goja runtime ownership
rules.

## Complete API Reference

After calling `adapter.Bind()`, the following globals are available in JavaScript.
These APIs are selected JavaScript host bindings for Goja. Timers, immediates,
microtask scheduling, Promise rejection, and process lifecycle follow exact
Node.js v26.5.0 for the declared Node profile. Retained AbortSignal,
EventTarget, and other Web APIs follow the revision-pinned standards declared
by the manifest; this package is still not a complete Node.js runtime and does
not provide Node modules such as `fs` or `http`.

The machine-readable surface and executable authority are in
`testdata/oracle/surface.json`. Standard-branded APIs that cannot be implemented
faithfully inside this package are left absent. `Bind` preserves host globals
outside the declared profile rather than replacing them with partial stand-ins.
For `performance`/`Performance` and `crypto`/`Crypto`, a structurally complete
branded host pair retains its identity and own descriptors; incomplete pairs
are rejected atomically. A preserved `Performance.prototype` is reversibly
integrated under the adapter's `EventTarget.prototype`, and the singleton gains
the matching event-target state and timestamp clock relationship. The
privileged host is responsible for ensuring that a preserved implementation
meets the pinned semantic contract. The exact executable claims cover the
adapter-owned implementations.

`New` and `Bind` require exclusive runtime ownership. Atomic installation uses
ordinary Goja objects for the global object, native `Promise`, native `Symbol`,
and any existing `console`; callback-backed `Proxy` or dynamic-object property
definition traps are not installation targets.

The adapter inherits the rewritten `go-eventloop` owner-topology scheduler:
native Goja Promise jobs, `process.nextTick`, `queueMicrotask`, timers,
immediates, and the auto-exit `beforeExit` / natural `exit` lifecycle re-enter
the Goja runtime under the adapter's serialized logical callback-owner role.
The event loop may temporarily transfer that role to an isolated callback
worker; physical goroutine identity is not part of the adapter contract. An
explicit JavaScript `process.exit()` call emits `exit` synchronously within the
current Goja execution while it holds that role, so hosts must never access the
runtime concurrently.
Public FD readiness uses epoll on Linux/Android, kqueue on
Darwin/iOS/DragonFly/FreeBSD/NetBSD/OpenBSD, and poll on AIX/Solaris/illumos.
Windows, Plan 9, js/wasm, and wasip1/wasm retain task/timer behavior but return
`eventloop.ErrReadinessUnsupported` for public FD registration.

### Timers

| API | Signature | Description |
|-----|-----------|-------------|
| `setTimeout` | `(fn, delay?, ...args) → Timeout` | Schedule one-time callback after Node-coerced `delay` ms; forwards `...args`; callback `this` is the returned handle; overflow/negative/NaN delay warnings are emitted through `process` |
| `clearTimeout` | `(handle)` | Cancel a scheduled timeout or interval handle; accepts handle objects and primitive coercions |
| `setInterval` | `(fn, delay?, ...args) → Timeout` | Schedule repeating callback every Node-coerced `delay` ms; forwards `...args`; callback `this` is the returned handle; overflow/negative/NaN delay warnings are emitted through `process` |
| `clearInterval` | `(handle)` | Cancel a scheduled interval or timeout handle; accepts handle objects and primitive coercions |
| `setImmediate` | `(fn, ...args) → Immediate` | Schedule callback in the check phase; callbacks queued by an immediate roll over to the next iteration; forwards `...args`; callback `this` is the returned handle |
| `clearImmediate` | `(handle)` | Cancel a scheduled immediate handle |
| `delay` | `(ms) → Promise` | Adapter extension returning a ref'ed native Promise after a bounded millisecond delay |

`delay(ms)` is an adapter extension, not a Node global. It applies numeric
coercion once and truncates finite fractions toward zero. Omitted,
`undefined`, `NaN`, `-Infinity`, and non-positive values select `0ms`;
`+Infinity` and values above `9,223,372,036,854ms` throw `RangeError`
synchronously. Symbol, BigInt, and abrupt object coercions retain their normal
exceptions. The returned native Promise remains asynchronous and keeps the
loop alive. Terminal cleanup fulfills an already-accepted delay with
`undefined`; a call after terminal cleanup returns an immediately rejected
Promise, and no Promise reaction runs after terminal admission closes.

Timer handles follow the Node v26 lifecycle shape for the exposed surface:
`ref()`, `unref()`, `hasRef()`, `refresh()`, `close()`, and
`[Symbol.dispose]()` update liveness and visible handle state. The writable
`_idleTimeout` property preserves Node-coerced fractional delay state, drives
valid `refresh()` rescheduling, and becomes `-1` when an active, inactive, or
currently executing handle is cleared/closed/disposed. Clearing an already-fired
one-shot handle after its callback returns preserves the fired value, matching
the Node oracle used by this repository.

### Microtasks

| API | Signature | Description |
|-----|-----------|-------------|
| `queueMicrotask` | `(fn)` | Queue a microtask (runs before next macrotask) |
| `process.nextTick` | `(fn, ...args)` | Node-style next-tick scheduling; forwards `...args`; ignored after `process._exiting` is set |

### Process events

| API | Signature | Description |
|-----|-----------|-------------|
| `process.on` / `process.once` | `(event, listener) → process` | Register event listeners for process lifecycle, warning, exception, and rejection events |
| `process.off` / `process.removeListener` | `(event, listener) → process` | Remove a registered listener |
| `process.emit` | `(event, ...args) → boolean` | Synchronously emit an event to process listeners |
| `process.listenerCount` | `(event) → number` | Return listener count for an event |
| `process.emitWarning` | `(warning, typeOrOptions?, code?)` | Emit a warning object via the `warning` event, defaulting to `Warning` |
| `process.exit` / `process.exitCode` | `(code?)` / property | Emit `exit` and terminate the loop profile; exit listeners cannot schedule nextTick, timers, immediates, queueMicrotask, or Promise jobs |

`beforeExit` is emitted when an auto-exit loop would otherwise become empty. If
the listener schedules only `process.nextTick`, `queueMicrotask`, or native
Promise work, those queues drain and `beforeExit` is not repeated; scheduling a
timer or immediate extends the loop and permits a later `beforeExit`, matching
Node v26 behavior. During natural auto-exit, explicit `process.exit()`, and fatal
exception exits, `exit` listeners are synchronous terminal notifications:
`process.nextTick`, `queueMicrotask`, native Promise jobs, timers, and immediates
queued by an `exit` listener are suppressed and cannot resurrect the loop.

### Promises

The adapter keeps Goja's native global `Promise` constructor and prototype as
the JavaScript intrinsics and routes native promise jobs, including async/await
continuations, into the event loop's microtask queue with
`SetPromiseJobEnqueuer`. `Bind` replaces the constructor's `all`, `race`,
`allSettled`, and `any` methods with wrappers for the declared Node semantics.
`Promise.withResolvers` and `Promise.try` are installed only when absent.

| API | Signature | Description |
|-----|-----------|-------------|
| `new Promise` | `(executor) → Promise` | Create a promise with `resolve`/`reject` callbacks |
| `.then` | `(onFulfilled?, onRejected?) → Promise` | Attach fulfillment/rejection handlers |
| `.catch` | `(onRejected?) → Promise` | Attach a rejection handler |
| `.finally` | `(onFinally?) → Promise` | Attach a handler invoked on settlement |
| `Promise.resolve` | `(value) → Promise` | Create an already-fulfilled promise |
| `Promise.reject` | `(reason) → Promise` | Create an already-rejected promise |
| `Promise.all` | `(iterable) → Promise` | Resolve when all resolve; reject on first rejection |
| `Promise.race` | `(iterable) → Promise` | Settle with the first to settle |
| `Promise.allSettled` | `(iterable) → Promise` | Resolve when all settle with `{status, value/reason}` |
| `Promise.any` | `(iterable) → Promise` | First to resolve wins; `AggregateError` if all reject |
| `Promise.withResolvers` | `() → {promise, resolve, reject}` | ES2024 — deferred promise pattern |
| `Promise.try` | `(fn) → Promise` | ES2025 — wraps sync/async call in a Promise |

Native Goja promise jobs, including async/await continuations, are routed to the event loop's microtask queue. If the adapter cannot enqueue one of those jobs, or if Goja reports an uncatchable error while running it, the adapter attempts the diagnostic through `Loop.Log`. That path calls the configured `logiface.Logger.Log` directly, contains backend panic and `runtime.Goexit`, and emits nothing for a nil or disabled logger; it never falls back to a process-global logger. `Adapter.SetConsoleOutput` controls JavaScript console output only and does not suppress promise-job diagnostics.

Callback exception policy:

| Callback source | If callback throws |
|-----------------|--------------------|
| `setTimeout`, `setInterval`, `setImmediate`, `queueMicrotask`, `process.nextTick` | Emit `process.uncaughtExceptionMonitor` then `process.uncaughtException` with origin `uncaughtException`; without a handler, the adapter enters the fatal path, emits `exit`, and suppresses later JavaScript callbacks, including exit microtasks and Promise jobs |
| `AbortSignal` / `EventTarget` listeners | Synchronous callback invocation or `handleEvent` lookup failures use the same process exception path |
| Promise `then` / `catch` handlers | Reject the returned promise |
| Promise `finally` handlers | Native Promise semantics: thrown or rejected `finally` results reject the derived promise |

EventTarget ignores listener return values and never reads a returned thenable's
`then` property. A native Promise returned by a listener remains under Goja's
ordinary `unhandledRejection` / `rejectionHandled` tracking.

### AbortController / AbortSignal

| API | Signature | Description |
|-----|-----------|-------------|
| `new AbortController` | `()` | Create a controller with `.signal` and `.abort(reason?)` |
| `AbortSignal.any` | `(signals) → AbortSignal` | Composite signal — aborts when any input aborts |
| `AbortSignal.timeout` | `(ms) → AbortSignal` | Signal that auto-aborts after `ms` milliseconds |
| `signal.aborted` | getter | Whether the signal has been aborted |
| `signal.reason` | getter | The abort reason |
| `signal.onabort` | setter/getter | Handler for `"abort"` events |
| `signal.addEventListener` | `(type, fn)` | Listen for `"abort"` events |
| `signal.throwIfAborted` | `()` | Throws if already aborted |

### Performance API

| API | Signature | Description |
|-----|-----------|-------------|
| `performance.now` | `() → number` | High-resolution elapsed time (ms) from the shared performance origin |
| `performance.timeOrigin` | getter | UNIX timestamp (ms) of the selected shared performance origin |
| `performance.toJSON` | `() → object` | Serialize the retained High Resolution Time attributes |

`performance` is a branded `Performance` singleton inheriting `EventTarget`;
`new Performance()` is illegal. User Timing and Performance Timeline methods
are outside the retained profile and are not installed.

### Console

| API | Signature | Description |
|-----|-----------|-------------|
| `console.time` | `(label?)` | Start a named timer |
| `console.timeEnd` | `(label?)` | Stop timer and log elapsed time |
| `console.timeLog` | `(label?, ...data)` | Log elapsed time without stopping |
| `console.count` | `(label?)` | Increment and log a call counter |
| `console.countReset` | `(label?)` | Reset the counter |
| `console.assert` | `(condition, ...data)` | Log `"Assertion failed"` if falsy |
| `console.table` | `(data, columns?)` | Render data as an ASCII table |
| `console.group` | `(label?)` | Start an indented log group |
| `console.groupCollapsed` | `(label?)` | Start a collapsed log group |
| `console.groupEnd` | `()` | End current group |
| `console.trace` | `(msg?)` | Print a stack trace |
| `console.clear` | `()` | Simulate clearing the console |
| `console.dir` | `(obj, options?)` | Formatted object inspection |

> **Note:** `console.log`, `console.warn`, `console.error`, `console.info`, and `console.debug` are **not** provided by the adapter. Supply them yourself or use Goja's built-in if available.

### Crypto

| API | Signature | Description |
|-----|-----------|-------------|
| `crypto.randomUUID` | `() → string` | Cryptographically random UUID v4 |
| `crypto.getRandomValues` | `(typedArray) → typedArray` | Fill integer TypedArray with random bytes (max 65536 bytes) |

`crypto` is a branded `Crypto` singleton and `new Crypto()` is illegal. The
`SubtleCrypto` family is outside the retained profile, so `crypto.subtle` is not
installed.

### Base64

| API | Signature | Description |
|-----|-----------|-------------|
| `atob` | `(encoded) → string` | Decode base64 to Latin-1 string |
| `btoa` | `(string) → string` | Encode Latin-1 string to base64 |

### Events

| API | Signature | Description |
|-----|-----------|-------------|
| `new EventTarget` | `()` | Create event target with `addEventListener`/`removeEventListener`/`dispatchEvent` |
| `new Event` | `(type, options?)` | Create a pinned DOM Event with dispatch state, cancellation, propagation, composed path, legacy initialization, phase constants, and unforgeable `isTrusted` |
| `new CustomEvent` | `(type, options?)` | Create an Event with `detail` and `initCustomEvent` |

### DOMException

| API | Signature | Description |
|-----|-----------|-------------|
| `new DOMException` | `(message?, name?)` | Error-backed DOM exception with `message`, `name`, legacy `code`, stack behavior, and the `DOMException` brand |

All legacy DOMException numeric constants from the pinned DOM IDL are exposed
read-only on both `DOMException` and `DOMException.prototype`.

### Utility

| API | Signature | Description |
|-----|-----------|-------------|
| `structuredClone` | `(value, options?) → value` | Deep-clone objects, arrays, Date, RegExp, Map, Set, Error, DOMException, ArrayBuffer, DataView, TypedArray, and circular references; supports ArrayBuffer transfer; throws `DOMException` `DataCloneError` for functions, symbols, unsupported transfer entries, and non-cloneable values |
| `Symbol.for` | `(key) → Symbol` | Global Symbol registry supplied by Goja |
| `Symbol.keyFor` | `(sym) → string` | Reverse lookup in the global Symbol registry |

RegExp cloning preserves every flag represented by the exact linked Goja
engine (`gimsuy`) and resets `lastIndex`. The engine does not implement the
`d`/`hasIndices` or `v`/`unicodeSets` RegExp features; this package does not
approximate those unsupported language features.

## Goja-Native APIs

These are provided by the [Goja](https://github.com/joeycumines/goja) JavaScript engine itself (ECMAScript 2020+):

- **Primitives:** `Boolean`, `Number`, `String`, `BigInt`, `Symbol`
- **Core:** `Object`, `Array`, `Function`, `Date`, `RegExp`, `JSON`, `Math`
- **Errors:** `Error`, `TypeError`, `RangeError`, `ReferenceError`, `SyntaxError`, `URIError`, `AggregateError`
- **Collections:** `Map`, `Set`, `WeakMap`, `WeakSet`
- **Binary:** `ArrayBuffer`, `DataView`, `Int8Array`, `Uint8Array`, `Uint8ClampedArray`, `Int16Array`, `Uint16Array`, `Int32Array`, `Uint32Array`, `Float32Array`, `Float64Array`
- **Metaprogramming:** `Proxy`, `Reflect`
- **Global functions:** `parseInt`, `parseFloat`, `isNaN`, `isFinite`, `eval`, `encodeURI`, `decodeURI`, `encodeURIComponent`, `decodeURIComponent`
- **Modern syntax:** Arrow functions, classes, destructuring, template literals, optional chaining (`?.`), nullish coalescing (`??`)

## Known Limitations

| API | Status |
|-----|--------|
| `console.log/warn/error/info/debug` | Not provided — supply your own |
| `fetch()` | Not provided — supply or preserve a host binding |
| `URL`, text codecs, Blob, Headers, FormData, Web Storage | Not installed; a pre-existing host implementation is preserved |
| `ReadableStream` / Streams API | Intentionally omitted |
| `Worker` / `MessageChannel` | No threading model |
| `WeakRef` / `FinalizationRegistry` | Not provided by the pinned Goja revision |
| `Intl` | Partial (Goja has limited support) |
| `File` / `FileReader` | Not provided |
| `WebSocket` | Not provided |

## Thread Safety

The adapter coordinates thread safety between:

1. **Goja Runtime** — Not goroutine-safe; only the current logical owner may
   access the runtime or its values
2. **Adapter callbacks** — Serialized under one logical callback-owner role,
   which the event loop may transfer to an isolated callback worker
3. **Go Code** — Can schedule owner-only runtime work from any goroutine via `adapter.Submit()`
4. **Promise APIs** — `adapter.NewPromise()` is owner-only; its settler is safe
   to call from another goroutine and runs its result callback and native
   settlement under the owner
5. **Abort integration** — `adapter.TrackAbortSignal()` is owner-only and
   requires a non-nil callback; its returned idempotent cleanup may be called
   from any goroutine
6. **Terminal integration** — `adapter.Done()` is goroutine-safe and returns
   the exact stable loop signal that closes after accepted callbacks can no
   longer execute

After calling `Bind()`, scheduled timer, immediate, microtask, nextTick, and
adapter Promise callbacks execute under the adapter's logical owner.
`adapter.OwnsRuntime(runtime)` and `adapter.OwnsLoop(loop)` are post-`Bind`
identity predicates: they return true only while that exact bound pair remains
claimed by the adapter, and return false for nil, foreign, pre-bind, failed, or
released state. A copied adapter always reports false. Loop termination alone
does not release the identity claim or imply that the loop still accepts work.
Synchronous APIs such as `EventTarget.dispatchEvent()`,
`AbortController.abort()`, and
already-aborted AbortSignal listener registration execute inline as part of the
current owner-held JavaScript operation. Hosts may perform initial direct setup
before starting the loop only while they have exclusive runtime ownership.
After callbacks may execute, callers must route external Goja work through
`adapter.Submit()` and must never access the runtime or Goja values
concurrently. A JavaScript exception raised by a direct Goja operation inside a
submitted callback follows the adapter's Node-style `uncaughtException` path;
a native Go panic retains the event loop's callback-panic contract. An
`Adapter` is non-copyable: retain and use only the pointer returned by `New`. A
Promise settler result may return Go data or a Goja value from the runtime
supplied to the result callback; a value from another Goja runtime rejects the
Promise with a `TypeError`. A nil or zero `PromiseSettler` returns
`ErrAdapterInvalid` for a non-nil result callback; a nil result callback
panics, and any attempt after the first admitted settlement returns
`ErrPromiseSettled`.

`TrackAbortSignal` returns `(cleanup, aborted, ok)`. `ok` is false for a value
outside this adapter's AbortSignal implementation. An already-aborted signal
returns `aborted=true` without registering a callback. Otherwise `cleanup`
removes the host callback exactly once and is safe to invoke from any
goroutine.

Native Goja `Promise` rejections are surfaced through Node-style process events:
install `process.on("unhandledRejection", ...)` and
`process.on("rejectionHandled", ...)` handlers from JavaScript when application
code wants to observe or override default rejection escalation. The
`eventloop.JSOption` values accepted by `New` apply to lower-level Go
`eventloop.JS` primitives, not to native Goja Promise rejection events. Invalid
or nil option values panic before the adapter claims either the runtime or the
event loop, matching the static-construction contract of direct
`eventloop.NewJS` callers.

## Requirements

- Go 1.26.2+
- [github.com/joeycumines/goja](https://github.com/joeycumines/goja)
- [github.com/joeycumines/go-eventloop](https://pkg.go.dev/github.com/joeycumines/go-eventloop)

## License

MIT License — see [LICENSE](LICENSE) for details.
