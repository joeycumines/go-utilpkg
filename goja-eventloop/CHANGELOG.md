# Changelog

All notable changes to the `goja-eventloop` package will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Core bridge between [goja](https://github.com/dop251/goja) JavaScript runtime
  and [go-eventloop](https://github.com/joeycumines/go-eventloop), providing 56+
  Web Platform APIs as JavaScript globals via `Adapter.Bind()`
- Timer APIs: `setTimeout`, `clearTimeout`, `setInterval`, `clearInterval`,
  `setImmediate`, `clearImmediate`, `queueMicrotask`, `delay(ms)`, `process.nextTick`
- Event-loop-integrated Promise/A+ implementation with `.then()`, `.catch()`,
  `.finally()`, `Promise.resolve()`, `.reject()`, `.all()`, `.race()`,
  `.allSettled()`, `.any()`, `.withResolvers()`, `.try()`
- `GojaWrapPromise()` public API for wrapping Go-side `*ChainedPromise` values
- AbortController / AbortSignal with `.any(signals)` and `.timeout(ms)`
- Performance API (`performance.now()`, marks, measures, entries)
- Console utilities (`console.time`, `.count`, `.assert`, `.table`, `.group`, `.trace`, etc.)
- Crypto: `crypto.randomUUID()`, `crypto.getRandomValues(typedArray)`
- TextEncoder / TextDecoder, `atob()` / `btoa()`
- EventTarget, Event, CustomEvent
- URL / URLSearchParams (WHATWG)
- Blob, Headers, FormData
- localStorage / sessionStorage (in-memory)
- DOMException with standard error code constants
- `structuredClone(value)` deep-clone
- Symbol.for / Symbol.keyFor polyfill
