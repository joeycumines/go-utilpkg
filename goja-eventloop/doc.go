// Package gojaeventloop provides a bridge between the [eventloop] package and
// the [goja] JavaScript runtime, exposing standard JavaScript globals that delegate to the underlying Go event loop.
//
// # Overview
//
// The [Adapter] wraps an [eventloop.Loop] and a [goja.Runtime], binding
// JavaScript APIs to their Go implementations. After calling [Adapter.Bind],
// JavaScript code running in the goja runtime has access to standard Web
// Platform APIs.
//
// # Bound JavaScript APIs
//
// Timer functions:
//   - setTimeout / clearTimeout
//   - setInterval / clearInterval
//   - setImmediate / clearImmediate
//   - queueMicrotask
//
// Promises:
//   - Promise (constructor, resolve, reject, all, allSettled, any, race, try, withResolvers)
//
// Abort:
//   - AbortController / AbortSignal
//
// Events:
//   - EventTarget, Event, CustomEvent
//
// Performance:
//   - performance.now(), performance.timeOrigin
//
// Console:
//   - console.log, console.warn, console.error, console.info, console.debug
//
// Encoding:
//   - TextEncoder / TextDecoder
//   - atob / btoa
//
// Data structures:
//   - Blob, Headers, FormData, URL, URLSearchParams
//
// Storage:
//   - localStorage / sessionStorage (in-memory)
//
// Crypto:
//   - crypto.getRandomValues, crypto.randomUUID
//
// Structured cloning:
//   - structuredClone
//
// DOM compatibility:
//   - DOMException (with standard error code constants)
//
// # Usage
//
//	loop, _ := eventloop.New()
//	defer loop.Close()
//	rt := goja.New()
//
//	adapter, _ := gojaeventloop.New(loop, rt)
//	_ = adapter.Bind()
//
//	loop.Submit(func() {
//	    rt.RunString(`
//	        setTimeout(() => console.log("hello from JS"), 100);
//	    `)
//	})
//
//	loop.Run(context.Background())
//
// [eventloop]: github.com/joeycumines/go-eventloop
// [goja]: github.com/dop251/goja
package gojaeventloop
