// Package gojaeventloop provides a bridge between the [eventloop] package and
// the [goja] JavaScript runtime, exposing selected JavaScript/Web/Node-inspired
// globals that delegate to the underlying Go event loop.
//
// # Overview
//
// The [Adapter] wraps an [eventloop.Loop] and a [goja.Runtime], binding
// JavaScript APIs to their Go implementations. After calling [Adapter.Bind],
// JavaScript code running in the goja runtime has access to selected Web
// Platform and Node-inspired APIs. The package does not claim full browser,
// Node.js, WPT, or Test262 conformance; documented behavior is the supported
// compatibility boundary. Timer, immediate, process.nextTick,
// Promise-job/rejection, and process lifecycle behavior follows exact Node.js
// v26.5.0 for the declared Node profile. Retained Web APIs, including
// AbortSignal and EventTarget, follow revision-pinned standards. APIs that are
// not implemented, including fetch, are not installed by [Adapter.Bind], and
// pre-existing host implementations outside the profile are preserved. The
// adapter inherits the linked [eventloop] module's declared targets. Public FD
// readiness uses epoll on Linux/Android, kqueue on
// Darwin/iOS/DragonFly/FreeBSD/NetBSD/OpenBSD, and poll on AIX/Solaris/illumos.
// Windows, Plan 9, js/wasm, and wasip1/wasm retain task/timer behavior but
// return [eventloop.ErrReadinessUnsupported] for public FD registration.
// Construction and binding require exclusive runtime ownership and ordinary
// Goja installation targets for the global object, native Promise, native
// Symbol, and any existing console.
//
// # Bound JavaScript APIs
//
// Timer functions:
//   - setTimeout / clearTimeout
//   - setInterval / clearInterval
//   - setImmediate / clearImmediate
//   - queueMicrotask
//   - delay (bounded, ref'ed native-Promise adapter extension)
//
// Timer handles expose Node-v26-shaped ref/unref/hasRef/refresh/close/dispose
// behavior for the supported surface, including visible `_idleTimeout` lifecycle
// state.
// The delay extension coerces its millisecond input once, rejects values beyond
// the time.Duration boundary synchronously, fulfills accepted terminally
// disposed work with undefined, and rejects calls made after terminal cleanup.
//
// Promises:
//   - Promise (constructor, resolve, reject, all, allSettled, any, race, try, withResolvers)
//
// Adapter.Bind keeps Goja's native global Promise constructor and prototype as
// the JavaScript intrinsics. Native Goja promise jobs such as async/await
// continuations are routed into the event loop's microtask queue. Bind replaces
// Promise.all, Promise.race, Promise.allSettled, and Promise.any with wrappers
// for the declared Node semantics. Promise.withResolvers and Promise.try are
// installed only when absent.
//
// Process APIs:
//   - process.on / once / off / emit / listenerCount / emitWarning
//   - process.exit / exitCode
//   - process.nextTick
//   - process.beforeExit, exit, warning, uncaughtExceptionMonitor,
//     uncaughtException, unhandledRejection, rejectionHandled event delivery
//
// Abort:
//   - AbortController / AbortSignal
//
// Events:
//   - EventTarget, Event, CustomEvent
//
// Performance:
//   - performance.now(), performance.timeOrigin, performance.toJSON()
//
// Console helpers:
//   - console.time, console.timeEnd, console.timeLog, console.count,
//     console.countReset, console.assert, console.table, console.group,
//     console.groupCollapsed, console.groupEnd, console.trace, console.clear,
//     console.dir
//
// Encoding:
//   - atob / btoa
//
// Crypto:
//   - crypto.getRandomValues, crypto.randomUUID (SubtleCrypto is not retained)
//
// Structured cloning:
//   - structuredClone (objects, arrays, Date, RegExp with the linked engine's
//     exact gimsuy flags, Map, Set, Error, ArrayBuffer, DataView, TypedArray,
//     circular references, and ArrayBuffer transfer; unsupported
//     clone/transfer inputs throw DOMException DataCloneError)
//
// DOM compatibility:
//   - DOMException (with standard error code constants)
//
// # Usage
//
//	loop, err := eventloop.New(eventloop.WithAutoExit(true))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer func() {
//	    if err := loop.Close(); err != nil && !errors.Is(err, eventloop.ErrLoopTerminated) {
//	        log.Printf("close event loop: %v", err)
//	    }
//	}()
//	rt := goja.New()
//
//	adapter, err := gojaeventloop.New(loop, rt)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if err := adapter.Bind(); err != nil {
//	    log.Fatal(err)
//	}
//
//	if _, err := rt.RunString(`
//	    setTimeout(() => { globalThis.done = true; }, 100);
//	`); err != nil {
//	    log.Fatal(err)
//	}
//	if err := loop.Run(context.Background()); err != nil {
//	    log.Fatal(err)
//	}
//
// # Callback errors
//
// Timer and microtask callbacks installed by `setTimeout`, `setInterval`,
// `setImmediate`, `queueMicrotask`, and `process.nextTick` run after the
// scheduling JavaScript call has returned; `setTimeout`, `setInterval`,
// `setImmediate`, and `process.nextTick` forward the extra arguments supplied at
// scheduling time. `queueMicrotask` intentionally accepts only the callback.
// Thrown callback errors emit `process.uncaughtExceptionMonitor` and
// `process.uncaughtException`; without an `uncaughtException` handler the adapter
// enters the fatal path, emits `exit`, and suppresses later JavaScript callbacks,
// including `exit` microtasks and Promise jobs. Synchronous `AbortSignal` and
// `EventTarget` callback invocation or `handleEvent` lookup failures use the
// same process exception path. Listener return values are ignored; returned
// thenables are not inspected, and returned native Promise rejections retain
// ordinary `unhandledRejection` and `rejectionHandled` tracking.
// Promise `then`, `catch`, and `finally` callback exceptions reject the returned
// promise according to native Promise semantics.
// During natural auto-exit, explicit `process.exit`, and fatal exception exits,
// asynchronous work scheduled by `exit` listeners is ignored: `process.nextTick`,
// `queueMicrotask`, native Promise jobs, timers, and immediates cannot resurrect
// the loop. Microtask-only `beforeExit` work does not cause a repeated
// `beforeExit`; timer or immediate work does.
//
// # Thread Safety
//
// Goja runtimes are not goroutine-safe. The adapter serializes asynchronous
// JavaScript work under one logical callback-owner role; the event loop may
// temporarily transfer that role to an isolated callback worker, so physical
// goroutine identity is not part of the contract. Direct setup is valid before
// the loop starts only while the caller has exclusive runtime ownership. After
// callbacks may execute, callers must route external Goja work through
// [Adapter.Submit] and must never access the runtime or its values concurrently.
// [Adapter.OwnsRuntime] and [Adapter.OwnsLoop] are post-bind identity
// predicates; they return true only while the exact runtime/loop pair remains
// claimed by that adapter.
// JavaScript exceptions raised by direct Goja operations inside a submitted
// callback enter the adapter's uncaught-exception path; native Go panics retain
// the event loop callback-panic contract.
// [Adapter.NewPromise] is owner-only. Its returned [PromiseSettler] may be used
// from another goroutine; result callbacks and native settlement execute under
// the logical owner.
// Native Goja Promise rejections are routed to JavaScript process events;
// install `process.on("unhandledRejection", ...)` or
// `process.on("rejectionHandled", ...)` handlers when Goja-aware rejection
// diagnostics are needed. The [eventloop.JSOption] values accepted by [New]
// configure lower-level Go `eventloop.JS` primitives and do not replace the
// native Goja Promise rejection tracker. New panics for nil inputs or invalid
// options before it claims the runtime or event loop, matching the repository's
// static-contract policy.
//
// [eventloop]: https://pkg.go.dev/github.com/joeycumines/go-eventloop
// [goja]: https://pkg.go.dev/github.com/joeycumines/goja
package gojaeventloop
