# goja-eventloop

Package goja-eventloop provides bindings between [go-eventloop](https://pkg.go.dev/github.com/joeycumines/go-eventloop) and the [Goja](https://github.com/dop251/goja) JavaScript runtime.

See the [API docs](https://pkg.go.dev/github.com/joeycumines/goja-eventloop).

## Features

- **56+ JavaScript APIs** bound as globals — timers, promises, abort, performance, console, crypto, URL, encoding, and more
- **Promise/A+** implementation integrated with the Go event loop's microtask queue
- **ES2024/ES2025** features: `Promise.withResolvers()`, `Promise.try()`
- **Cross-platform** — runs on Linux (epoll), macOS (kqueue), and Windows (IOCP)
- **Thread-safe** coordination between Goja and the event loop

## Installation

```bash
go get github.com/joeycumines/goja-eventloop
```

## Quick Start

```go
package main

import (
    "context"
    "time"

    "github.com/dop251/goja"
    eventloop "github.com/joeycumines/go-eventloop"
    gojaeventloop "github.com/joeycumines/goja-eventloop"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    loop, _ := eventloop.New()
    defer loop.Shutdown(context.Background())
    runtime := goja.New()

    adapter, _ := gojaeventloop.New(loop, runtime)
    adapter.Bind()

    runtime.RunString(`
        setTimeout(() => console.log("Hello after 100ms!"), 100);
        queueMicrotask(() => console.log("Microtask runs first"));
        new Promise(resolve => resolve(42))
            .then(v => console.log("Promise:", v));
    `)

    loop.Run(ctx)
}
```

## Complete API Reference

After calling `adapter.Bind()`, the following globals are available in JavaScript.

### Timers

| API | Signature | Description |
|-----|-----------|-------------|
| `setTimeout` | `(fn, delay?) → number` | Schedule one-time callback after `delay` ms |
| `clearTimeout` | `(id)` | Cancel a scheduled timeout |
| `setInterval` | `(fn, delay?) → number` | Schedule repeating callback every `delay` ms |
| `clearInterval` | `(id)` | Cancel a scheduled interval |
| `setImmediate` | `(fn) → number` | Schedule callback after I/O events (Node.js-style) |
| `clearImmediate` | `(id)` | Cancel a scheduled immediate |
| `delay` | `(ms) → Promise` | Returns a Promise that resolves after `ms` milliseconds |

### Microtasks

| API | Signature | Description |
|-----|-----------|-------------|
| `queueMicrotask` | `(fn)` | Queue a microtask (runs before next macrotask) |
| `process.nextTick` | `(fn)` | Node.js-style microtask scheduling |

### Promises

The adapter **overrides** Goja's native `Promise` with an event-loop-integrated implementation.

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
| `performance.now` | `() → number` | High-resolution elapsed time (ms) from loop origin |
| `performance.timeOrigin` | getter | UNIX timestamp (ms) when the loop was created |
| `performance.mark` | `(name, options?) → PerformanceMark` | Create a named performance mark |
| `performance.measure` | `(name, start?, end?) → PerformanceMeasure` | Measure duration between marks |
| `performance.getEntries` | `() → PerformanceEntry[]` | Get all recorded entries |
| `performance.getEntriesByType` | `(type) → PerformanceEntry[]` | Filter by `"mark"` or `"measure"` |
| `performance.getEntriesByName` | `(name, type?) → PerformanceEntry[]` | Filter by name |
| `performance.clearMarks` | `(name?)` | Clear marks (all or by name) |
| `performance.clearMeasures` | `(name?)` | Clear measures (all or by name) |
| `performance.toJSON` | `() → object` | Serialize performance data |

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

### Base64

| API | Signature | Description |
|-----|-----------|-------------|
| `atob` | `(encoded) → string` | Decode base64 to Latin-1 string |
| `btoa` | `(string) → string` | Encode Latin-1 string to base64 |

### Events

| API | Signature | Description |
|-----|-----------|-------------|
| `new EventTarget` | `()` | Create event target with `addEventListener`/`removeEventListener`/`dispatchEvent` |
| `new Event` | `(type, options?)` | Create event with `bubbles`, `cancelable` |
| `new CustomEvent` | `(type, options?)` | Create event with `detail` property |

### URL

| API | Signature | Description |
|-----|-----------|-------------|
| `new URL` | `(url, base?)` | WHATWG URL parser (all standard properties + `searchParams`) |
| `new URLSearchParams` | `(init?)` | Query string builder with `append`/`delete`/`get`/`getAll`/`has`/`set`/`sort`/`forEach`/`size` |

### Encoding

| API | Signature | Description |
|-----|-----------|-------------|
| `new TextEncoder` | `()` | UTF-8 encoder: `encode(string)` → `Uint8Array`, `encodeInto(src, dest)` |
| `new TextDecoder` | `(encoding?, options?)` | UTF-8 decoder: `decode(input?)` with `fatal`/`ignoreBOM` options |

### Blob

| API | Signature | Description |
|-----|-----------|-------------|
| `new Blob` | `(parts?, options?)` | Binary data container: `size`, `type`, `text()`, `arrayBuffer()`, `slice()` |

> **Note:** `blob.stream()` returns `undefined` — ReadableStream is intentionally not implemented. Use `text()` or `arrayBuffer()` instead.

### Web Storage

| API | Signature | Description |
|-----|-----------|-------------|
| `localStorage` | object | In-memory storage: `getItem`/`setItem`/`removeItem`/`clear`/`key`/`length` |
| `sessionStorage` | object | Separate in-memory storage instance (same API) |

### HTTP Primitives

| API | Signature | Description |
|-----|-----------|-------------|
| `new Headers` | `(init?)` | HTTP headers: `append`/`delete`/`get`/`getSetCookie`/`has`/`set`/`entries`/`keys`/`values`/`forEach` |
| `new FormData` | `()` | Form data (string-only): `append`/`delete`/`get`/`getAll`/`has`/`set`/`entries`/`keys`/`values`/`forEach` |

### DOMException

| API | Signature | Description |
|-----|-----------|-------------|
| `new DOMException` | `(message?, name?)` | DOM exception with `message`, `name`, `code`, `toString()` |

Static constants: `INDEX_SIZE_ERR` (1), `HIERARCHY_REQUEST_ERR` (3), `INVALID_CHARACTER_ERR` (5), `NOT_SUPPORTED_ERR` (9), `INVALID_STATE_ERR` (11), `SYNTAX_ERR` (12), `TYPE_MISMATCH_ERR` (17), `SECURITY_ERR` (18), `NETWORK_ERR` (19), `ABORT_ERR` (20), `QUOTA_EXCEEDED_ERR` (22), `TIMEOUT_ERR` (23), `DATA_CLONE_ERR` (25).

### Utility

| API | Signature | Description |
|-----|-----------|-------------|
| `structuredClone` | `(value) → value` | Deep-clone objects, arrays, Date, RegExp, Map, Set (throws on functions) |
| `Symbol.for` | `(key) → Symbol` | Global Symbol registry (Goja native, polyfilled if missing) |
| `Symbol.keyFor` | `(sym) → string` | Reverse lookup in the global Symbol registry |

## Goja-Native APIs

These are provided by the [Goja](https://github.com/dop251/goja) JavaScript engine itself (ECMAScript 2020+):

- **Primitives:** `Boolean`, `Number`, `String`, `BigInt`, `Symbol`
- **Core:** `Object`, `Array`, `Function`, `Date`, `RegExp`, `JSON`, `Math`
- **Errors:** `Error`, `TypeError`, `RangeError`, `ReferenceError`, `SyntaxError`, `URIError`, `AggregateError`
- **Collections:** `Map`, `Set`, `WeakMap`, `WeakSet`
- **Binary:** `ArrayBuffer`, `DataView`, `Int8Array`, `Uint8Array`, `Uint8ClampedArray`, `Int16Array`, `Uint16Array`, `Int32Array`, `Uint32Array`, `Float32Array`, `Float64Array`
- **Metaprogramming:** `Proxy`, `Reflect`
- **Memory:** `WeakRef`, `FinalizationRegistry`
- **Global functions:** `parseInt`, `parseFloat`, `isNaN`, `isFinite`, `eval`, `encodeURI`, `decodeURI`, `encodeURIComponent`, `decodeURIComponent`
- **Modern syntax:** Arrow functions, classes, destructuring, template literals, optional chaining (`?.`), nullish coalescing (`??`)

## Known Limitations

| API | Status |
|-----|--------|
| `console.log/warn/error/info/debug` | Not provided — supply your own |
| `fetch()` | Not provided (Headers/FormData available for future use) |
| `ReadableStream` / Streams API | Intentionally omitted |
| `Worker` / `MessageChannel` | No threading model |
| `Intl` | Partial (Goja has limited support) |
| `File` / `FileReader` | Not provided (use Blob) |
| `WebSocket` | Not provided |

## Thread Safety

The adapter coordinates thread safety between:

1. **Goja Runtime** — Not thread-safe; access from one goroutine only
2. **Event Loop** — Processes callbacks on its own goroutine
3. **Go Code** — Can schedule timers/promises from any goroutine via `loop.Submit()`
4. **Promise APIs** — `then`, `catch`, `finally`, `resolve`, `reject` are thread-safe

After calling `Bind()`, JavaScript callbacks execute on the event loop goroutine.
Access the Goja runtime from within callbacks or via `loop.Submit()`, **never**
concurrently from a separate goroutine.

## Requirements

- Go 1.25+
- [github.com/dop251/goja](https://github.com/dop251/goja)
- [github.com/joeycumines/go-eventloop](https://pkg.go.dev/github.com/joeycumines/go-eventloop)

## License

MIT License — see [LICENSE](../LICENSE) for details.
