// Package eventloop provides a JavaScript-inspired event loop for Go, featuring
// timers, promise-style chaining, microtask scheduling, and platform I/O polling
// where the target OS supports it.
//
// # Architecture
//
// The event loop is built around a [Loop] core that manages task scheduling,
// timer processing, and I/O readiness notification. A [JS] adapter provides
// JavaScript-shaped APIs such as [JS.SetTimeout], [JS.SetInterval],
// [JS.SetImmediate], [JS.QueueMicrotask], and promise combinators
// ([JS.All], [JS.Race], [JS.Any], [JS.AllSettled]).
// [JS.ClearTimeout] and [JS.ClearInterval] share the JavaScript timer handle
// namespace: either clear function can cancel a timeout or interval returned by
// the adapter. Immediate handles occupy a separate namespace and are canceled
// only through [JS.ClearImmediate]. Core [TimerID] values span the full uint64
// namespace; JavaScript-shaped handles stop at JavaScript's maximum safe integer.
//
// The promise implementation ([ChainedPromise]) is a Go-loop promise profile
// modeled on Promise/A+ reaction ordering. It supports chaining, catch/finally,
// combinators, and adoption of other [ChainedPromise] values through the loop
// microtask queue. It is not a full ECMAScript Promise implementation and does
// not claim arbitrary JavaScript thenable assimilation or Test262 conformance.
//
// # Platform Support
//
// I/O polling is initialized lazily:
//   - Linux and Android: epoll
//   - Darwin, iOS, DragonFly, FreeBSD, NetBSD, and OpenBSD: kqueue
//   - AIX/ppc64, Solaris/amd64, and illumos/amd64: poll with a private control pipe
//   - Windows, Plan 9, js/wasm, and wasip1/wasm: task-only support. Timers,
//     microtasks, promises, and [Loop.Submit]/[Loop.SubmitInternal] use the
//     channel-based wait path; readiness operations return
//     [ErrReadinessUnsupported] and allocate no readiness descriptors.
//
// File descriptor operations ([Loop.RegisterFD], [Loop.UnregisterFD],
// [Loop.ModifyFD]) provide readiness notification on the readiness targets, backed by
// hybrid dense/sparse registration storage and loop-owned ready-event dispatch.
// Task-only targets return [ErrReadinessUnsupported] from the public readiness API.
//
// # Thread Safety
//
// The loop is designed for concurrent access:
//   - [Loop.Submit] and [Loop.SubmitInternal] are safe to call from any goroutine
//   - [Loop.ScheduleMicrotask], nextTick/checkpoint scheduling, immediate/close
//     phase scheduling, and timer lifecycle operations use loop-owner local
//     queues when called from the logical callback owner and typed command
//     ingress plus lifecycle/wakeup locking when called externally
//   - Timer and FD registration methods are thread-safe
//   - A successful [Loop.ScheduleTimer] publishes its [TimerID] before callback
//     entry. Timer cancellation returns exact sequential results before Run as
//     well as while the owner is active; pre-Run ref changes queue without
//     waiting for ownership.
//   - [ChainedPromise] resolve/reject functions are safe from any goroutine;
//     during normal [Loop.Run] execution, handlers execute on the event-loop
//     callback owner via microtasks
//   - Host adapters may use [Loop.ScheduleControlTimer] without inflating user
//     callback metrics, then enter each synchronous user callback and its full
//     microtask checkpoint through owner-only [Loop.RunCallback]. Hosts that
//     must update task-selection state first use [Loop.RunCallbackDeferredCheckpoint]
//     followed by [Loop.RunMicrotaskCheckpoint]. If the host handles a callback
//     exception by unwinding that checkpoint, owner-only [Loop.YieldMicrotasks]
//     defers its remainder. [Loop.AdvanceMicrotaskCheckpoint] consumes one such
//     in-phase deferral, while [Loop.ResumeMicrotaskCheckpoint] force-completes
//     it only at an explicit host phase boundary after task-selection state is
//     published.
//
// # Lifecycle Notes
//
// [Loop.Run] blocks until its loop-owner execution exits. A winning immediate
// [Loop.Close] completes terminal resource cleanup before Run returns. Graceful
// terminal completion may continue after Run while admitted [Loop.Promisify]
// functions finish. External Shutdown or Close calls whose initial completion
// probe observes that barrier still open join its result; Shutdown remains
// bounded by its context.
//
// [Loop.Shutdown] is safe to call from a loop callback: from the logical callback
// owner it initiates or acknowledges non-immediate termination and returns
// without waiting for [Loop.Run] to close its own done channel, including from a
// synchronous cleanup diagnostic after the graceful drain ends. [Loop.Close] is an immediate-stop API and is
// explicitly rejected from loop callbacks with [ErrReentrantClose]. Close rejects
// registered pending promises before waiting for the loop owner, permits only
// callbacks whose execution was already claimed, and does not wait for user
// functions that already claimed entry through [Loop.Promisify]: it returns
// after loop cleanup while those functions may continue under their caller-provided
// contexts. The claim is the lifecycle boundary, so a claimed worker may execute
// its first user-function instruction even after Close has already returned. A committed
// worker that has not claimed entry skips its user function after Close wins. JS
// adapter handle publication is serialized with terminal cleanup, so an admitted
// timeout, interval, or immediate cannot publish stale state afterward. An
// external non-winning Close joins an open terminal-completion barrier and
// receives the aggregate terminal result. A Promisify worker never joins a
// barrier that may depend on itself: a worker Close returns nil as a request
// acknowledgement when immediate mode is already active, whether or not that
// worker won the transition, and [ErrLoopTerminated] when graceful mode won. A
// callback executing in the terminal drain retains drain ownership, so Shutdown
// acknowledges graceful mode and Close returns [ErrReentrantClose]. A post-drain
// dependency-release or cleanup diagnostic callback holding terminal-completion
// ownership uses the mode-sensitive nonjoining result. A completion probe that
// observes terminal completion already published returns [ErrLoopTerminated].
//
// [Loop.Requests] returns a transferable capability for dependency goroutines
// that cannot safely join their parent callback or worker. Its lifecycle methods
// acknowledge the committed terminal mode without waiting for cleanup, and its
// timer methods acknowledge FIFO admission without waiting for owner application
// or exact per-timer results. Ordinary Loop lifecycle and timer methods retain
// their stronger completion and result contracts.
//
// If Shutdown is called before Run ever starts, there is no loop goroutine to
// preserve callback affinity. In that StateAwake case, a dedicated terminal
// finisher drains already accepted callbacks before waiting for dependent
// Promisify workers and completing cleanup. An external Shutdown caller that
// wins the transition waits only until cleanup completes or its context ends;
// a winning Promisify worker returns nil after the independent finisher owns the
// request so it cannot join completion that includes itself. Concurrent external
// callers join the same open completion barrier, each with its own context, and
// no public caller becomes the drain owner. Code with goroutine affinity,
// including Goja runtime access, must start Run before submitting callbacks
// rather than relying on a Shutdown caller's goroutine.
//
// # Execution Model
//
// The loop supports a dual-path execution model:
//   - task-only fast path: channel-based waiting plus owner-local task/phase
//     snapshots when no user FDs require the platform poller and fast-path mode
//     permits it
//   - I/O path: lazily initialized epoll, kqueue, or poll readiness when user
//     FDs are registered, or when FastPathDisabled deliberately selects native
//     polling without a user descriptor
//
// Task priority ordering within each tick:
//  1. Start-of-tick microtasks (nextTick and promise reactions, drained exhaustively)
//  2. Poll / wake processing
//  3. Check-phase callbacks ([Loop.ScheduleImmediate])
//  4. Inter-phase microtask drain
//  5. Close callbacks ([Loop.ScheduleCloseCallback])
//  6. Inter-phase microtask drain
//  7. Timer callbacks (deadline-list buckets keyed by monotonic milliseconds;
//     exact deadlines order within a bucket and exact ties preserve FIFO)
//  8. Inter-phase microtask drain
//  9. Internal queue tasks ([Loop.SubmitInternal])
//  10. Inter-phase microtask drain
//  11. External task phase ([Loop.Submit])
//  12. Final microtask drain
//
// The graceful terminal-drain pass processes callbacks still queued when it
// begins in check, close, internal, then external order, with a microtask
// checkpoint after each callback and phase. The drain itself does not invoke
// timers; cleanup discards timers that remain pending. A turn already in
// progress may finish eligible callbacks before the drain begins.
//
// Microtask draining uses a Node.js-v11+-shaped alternating-batch algorithm: the
// nextTick and microtask queues are drained in alternating BATCHES — all pending
// nextTick callbacks, then all pending promise/queueMicrotask callbacks,
// repeating until both queues are empty. A nextTick scheduled during a promise
// microtask is processed in the next nextTick batch rather than preempting the
// remaining microtasks. This is a compatibility target for the supported queues,
// not a claim that the package implements Node's complete process or event-loop
// model. Draining is exhaustive (no budget cap), so a self-rescheduling
// microtask or nextTick can starve timers and I/O. After 100000 callbacks in one
// drain, the loop calls [logiface.Logger.Log] directly through [Loop.Log] for
// one error-level diagnostic; a nil or disabled logger emits no event.
//
// During Run, context cancellation and committed graceful Shutdown do not stop
// owner execution until owner work returns to the outer run loop. Neither
// preempts a callback or an active checkpoint. During ordinary and terminal
// drains, the owner may recursively enqueue next ticks, microtasks, checkpoint
// callbacks, and Promise reactions until the queues empty. This preserves
// accepted continuations and Promise dependencies, but an unbounded chain can
// delay host control as it can in a JavaScript runtime.
//
// Draining occurs after each host callback, between every phase boundary in
// tick(), and at the start of each tick. Node-style per-callback checkpoints
// are an unconditional main-loop invariant.
//
// Task, check, and close queues use phase-snapshot draining: each phase captures
// its admitted work at phase entry and processes that snapshot. Work admitted
// while the phase is running is deferred to a later tick. This avoids arbitrary
// callback-count caps while preventing concurrent Go producers from extending
// one phase forever. [WithQueuePressureHandler] observes when external work
// remains after a task-phase snapshot that processed work; it is a backpressure
// hint, not a numeric budget failure.
//
// [JS.SetImmediate] uses the check-phase queue, which runs after poll and before
// close callbacks and timers in the current Node-v26-shaped topology. Callbacks
// scheduled from a running check callback roll over to a later iteration and can
// run before timers admitted by the earlier check callback.
//
// See docs/architecture.md in the module repository for the current internal
// architecture notes.
//
// # Go-Native Cancellation and Events
//
// [AbortController], [AbortSignal], [AbortAny], and [AbortTimeout] provide
// concurrent Go cancellation with stable reason identity. [EventTarget]
// dispatches [Event] and [CustomEvent] listeners synchronously with removal
// visible until callback-start claim and atomic once-listener claims. These APIs
// are browser-inspired Go extensions, not DOM implementations; goja-eventloop
// owns JavaScript-visible behavior.
//
// # Usage
//
//	loop := eventloop.New(eventloop.WithAutoExit(true))
//	defer func() {
//	    if err := loop.Close(); err != nil {
//	        log.Printf("close event loop: %v", err)
//	    }
//	}()
//
//	js := eventloop.NewJS(loop)
//
//	if err := loop.Submit(func() {
//	    if _, err := js.SetTimeout(func() {
//	        fmt.Println("Hello after 100ms")
//	    }, 100); err != nil {
//	        log.Printf("schedule timeout: %v", err)
//	    }
//	}); err != nil {
//	    log.Fatal(err)
//	}
//
//	if err := loop.Run(context.Background()); err != nil {
//	    log.Fatal(err)
//	}
//
// # Error Types
//
// The package provides error types and stable identities for the Go-loop profile:
//   - [AggregateError]: for [JS.Any] rejections (multi-error, Go 1.20+ compatible)
//   - [NilPromiseError]: for nil inputs to [JS.All], [JS.Race], [JS.AllSettled],
//     and [JS.Any] (its Index identifies the zero-based input position)
//   - [AbortError]: for abort operations via [AbortController]
//   - [TimeoutError]: for promise and abort timeouts
//   - [PanicError]: wraps recovered panics from Go promise callbacks
//   - [FDRegistrationRollbackError]: reports registration rollback failure and
//     exposes final ownership through [FDRegistrationRollbackError.Registered]
//   - [FDUnregisterError]: reports unregister cleanup failure and exposes final
//     ownership through [FDUnregisterError.Released]
//   - [ErrPromiseSelfResolution] and [ErrPromiseNilAdoption]: stable native
//     promise-resolution rejection identities
//   - [ErrFDRegistrationExhausted] and [ErrReadinessUnsupported]: stable
//     readiness identities
//   - [ErrJSBindState]: atomic JavaScript-adapter binding attempted outside
//     StateAwake
//   - [ErrJSBindConflict]: a loop already has an atomic JavaScript-adapter
//     binding
//
// Match sentinels and underlying causes with [errors.Is]. Recover concrete error
// types with [errors.As]. Readiness cleanup errors implement [errors.Unwrap] so
// both operations preserve their underlying native and lifecycle causes.
package eventloop
