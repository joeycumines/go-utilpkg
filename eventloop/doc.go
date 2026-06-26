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
//   - wasm/js: task-only support. Timers, microtasks, promises, and
//     [Loop.Submit]/[Loop.SubmitInternal] run via the channel-based fast path
//     (no I/O file descriptors are registered, so the loop never enters the
//     kqueue/epoll/IOCP poll path). I/O readiness operations
//     ([Loop.RegisterFD], [Loop.UnregisterFD], [Loop.ModifyFD]) are unsupported
//     and return errors; there is no wake file descriptor. The
//     internal/alternate* experimental packages do not build on wasm; use the
//     main package for wasm workloads.
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
//  4. Microtasks (nextTick and promise reactions, drained exhaustively)
//
// Microtask draining follows Node.js v11+ semantics. The nextTick and microtask
// queues are drained in alternating BATCHES: all pending nextTick callbacks,
// then all pending promise/queueMicrotask callbacks, repeating until both queues
// are empty. A nextTick scheduled during a promise microtask is therefore
// processed in the next nextTick batch rather than preempting the remaining
// microtasks — matching Node's processTicksAndRejections plus V8
// MicrotaskQueue::PerformCheckpoint. Draining is exhaustive (no budget cap), so
// a self-rescheduling microtask or nextTick can starve timers and I/O just as in
// JavaScript (a one-shot warning is logged after 100000 callbacks).
//
// Draining occurs after each internal task execution (unconditionally), between
// every phase boundary in tick(), and at the start of each tick. Per-callback
// draining after each timer callback, external task, and aux job is opt-in via
// [WithStrictMicrotaskOrdering]: the default batches those phases (as Node did
// before v11); enable it for exact Node v11+ per-callback checkpoints.
//
// Both task queues apply a per-tick callback budget as overload protection: the
// internal queue processes at most 4096 tasks per tick and the external queue at
// most 1024. Work beyond the budget is deferred (not dropped) to the next tick
// and the loop advances to the next phase; when the external budget is exceeded,
// [Loop.OnOverload] is signaled so callers can apply backpressure. These limits
// are a deliberate design choice (see docs/process_external_budget.md), not a
// Node.js equivalence.
//
// [JS.SetImmediate] uses the external queue and therefore runs in the external
// phase, before poll; in Node.js, setImmediate runs in a separate check phase
// after poll. This is a deliberate deviation.
//
// # Usage
//
//	loop, err := eventloop.New()
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
