// Package eventloop provides a high-performance, JavaScript-compatible event
// loop for Go, featuring timers, promises (Promise/A+), microtask scheduling,
// and cross-platform I/O polling.
//
// # Architecture
//
// The event loop is built around a [Loop] core that manages task scheduling,
// timer processing, and I/O readiness notification. A [JS] adapter provides
// JavaScript-compatible APIs such as [JS.SetTimeout], [JS.SetInterval],
// [JS.SetImmediate], [JS.QueueMicrotask], and promise combinators
// ([JS.All], [JS.Race], [JS.Any], [JS.AllSettled]).
//
// The promise implementation ([ChainedPromise]) is compliant with the
// Promise/A+ specification, supporting full thenable chaining with
// microtask-based resolution.
//
// # Platform Support
//
// I/O polling is implemented using platform-native mechanisms:
//   - macOS: kqueue
//   - Linux: epoll
//   - Windows: IOCP (I/O Completion Ports)
//
// File descriptor operations ([Loop.RegisterFD], [Loop.UnregisterFD],
// [Loop.ModifyFD]) provide cross-platform I/O readiness notification.
//
// # Thread Safety
//
// The loop is designed for concurrent access:
//   - [Loop.Submit] and [Loop.SubmitInternal] are safe to call from any goroutine
//   - [Loop.ScheduleMicrotask] is lock-free (MPSC ring buffer)
//   - Timer and FD registration methods are thread-safe
//   - Promise resolution must occur on the loop goroutine (enforced automatically)
//
// # Execution Model
//
// The loop supports a dual-path execution model:
//   - Fast path (~50ns/task): channel-based scheduling for low-latency scenarios
//   - I/O path (~8-15µs): poll-based scheduling when I/O FDs are registered
//
// Task priority ordering within each tick:
//  1. Timer callbacks (earliest deadline first)
//  2. Internal queue tasks ([Loop.SubmitInternal])
//  3. External queue tasks ([Loop.Submit])
//  4. Microtasks (drained after each macrotask when strict ordering is enabled)
//
// # Usage
//
//	loop, err := eventloop.New(
//	    eventloop.WithStrictMicrotaskOrdering(true),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer loop.Close()
//
//	js, err := eventloop.NewJS(loop)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	loop.Submit(func() {
//	    js.SetTimeout(func() {
//	        fmt.Println("Hello after 100ms")
//	        loop.Shutdown(context.Background())
//	    }, 100)
//	})
//
//	if err := loop.Run(context.Background()); err != nil {
//	    log.Fatal(err)
//	}
//
// # Error Types
//
// The package provides JavaScript-compatible error types:
//   - [AggregateError]: for [JS.Any] rejections (multi-error, Go 1.20+ compatible)
//   - [AbortError]: for abort operations via [AbortController]
//   - [TypeError], [RangeError]: for argument validation
//   - [TimeoutError]: for promise timeouts
//   - [PanicError]: wraps recovered panics from [Loop.Promisify]
//
// All error types implement the standard [error] interface, [errors.Unwrap],
// and type-based matching via Is().
package eventloop
