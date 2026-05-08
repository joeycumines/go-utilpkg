# go-eventloop

Package eventloop provides a JavaScript-inspired event loop implementation for Go.

See the [API docs](https://pkg.go.dev/github.com/joeycumines/go-eventloop).

## Features

- **JavaScript-Style Timer APIs**: Implements `setTimeout`, `setInterval`, `clearTimeout`, `clearInterval`-style scheduling with shared timeout/interval cancellation handles, deadline-list timer buckets, and native repeating interval nodes
- **Owner-Local Microtasks**: `queueMicrotask`, `process.nextTick`-style queues, and checkpoint diagnostics use loop-owner queues with typed command ingress for external goroutines
- **Go-Loop Promise Profile**: `ChainedPromise` supports `Then`, `Catch`, `Finally`, combinators, microtask reactions, and adoption of other `ChainedPromise` values; it is not a full ECMAScript Promise or arbitrary-thenable implementation
- **Promise Combinators**: `All`, `Race`, `AllSettled`, `Any` for composing multiple promises
- **Go-Native Cancellation and Events**: concurrent `AbortSignal` composition and synchronous `EventTarget` dispatch with explicit Go callback, removal, and panic boundaries
- **Unhandled Rejection Tracking**: Configurable callbacks for unhandled promise rejections
- **Platform I/O**: epoll on Linux/Android, kqueue on Apple and BSD targets, and poll on AIX/Solaris/illumos, with lazy wake resources, owned descriptors, generation-safe reuse, and at most one eligible user callback per committed registration in each poll batch; Windows, Plan 9, and WebAssembly targets retain task/timer support without the public readiness FD API
- **Low-Allocation Hot Paths**: Owner-local queues, typed command ingress, bounded buffer reuse, pooled timer nodes, cached loop clock reads, and reusable fast-mode sleep timers
- **Performance Monitoring**: Detached snapshots of scheduled-callback execution duration (excluding queue residence), successful-return throughput, and owner-turn boundary samples of task, immediate, close, internal, and microtask queue depths
- **Instance-Scoped Diagnostics**: `WithLogger` configures `logiface`; `Loop.Log` gives adapters a synchronous role-preserving boundary that directly accepts nil logger receivers and contains backend panic or `runtime.Goexit`
- **Host-neutral timers**: Go durations are preserved; JavaScript adapters own delay coercion and host-specific policy
- **Adapter control boundaries**: internal control timers retain ordinary timer safety without inflating user metrics; owner-only `RunCallback` records each multiplexed user callback, `RunCallbackDeferredCheckpoint` permits required host bookkeeping before its explicit `RunMicrotaskCheckpoint`, `YieldMicrotasks` defers a handled-exception checkpoint, `AdvanceMicrotaskCheckpoint` consumes one in-phase deferral, and `ResumeMicrotaskCheckpoint` force-completes it only after an explicit host phase publishes its task-selection state

## Installation

```bash
go get github.com/joeycumines/go-eventloop
```

## Usage

### Basic Event Loop

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/joeycumines/go-eventloop"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    loop := eventloop.New(eventloop.WithAutoExit(true))

    // Schedule a timer
    if _, err := loop.ScheduleTimer(100*time.Millisecond, func() {
        fmt.Println("Timer fired!")
    }); err != nil {
        log.Fatal(err)
    }

    // Run the loop
    if err := loop.Run(ctx); err != nil {
        log.Fatal(err)
    }
}
```

### JavaScript-Style Timers

```go
loop := eventloop.New()
js := eventloop.NewJS(loop)

// setTimeout
id, err := js.SetTimeout(func() {
    fmt.Println("Hello after 100ms")
}, 100)
if err != nil {
    log.Fatal(err)
}

// clearTimeout
if err := js.ClearTimeout(id); err != nil {
    log.Fatal(err)
}

// ClearTimeout and ClearInterval share the JavaScript timer handle namespace:
// either clear function can cancel a timeout or interval ID.
// ClearImmediate uses a distinct handle namespace and reports
// ErrImmediateNotFound for an unknown or completed immediate.

// setInterval
intervalID, err := js.SetInterval(func() {
    fmt.Println("Tick!")
}, 1000)
if err != nil {
    log.Fatal(err)
}

// clearInterval after 5 ticks
go func() {
    time.Sleep(5500 * time.Millisecond)
    if err := js.ClearInterval(intervalID); err != nil {
        log.Printf("clear interval: %v", err)
    }
}()
```

The Go-native `JS` adapter normalizes negative millisecond delays to zero and
panics before admission if a non-negative value would overflow `time.Duration`.
Its timeout/interval handles are monotonic through JavaScript's maximum safe
integer and then return `ErrTimerIDExhausted`; immediate handles use their own
namespace and `ErrImmediateIDExhausted`. Core `Loop.ScheduleTimer` uses the full
`uint64` `TimerID` namespace. All allocators saturate instead of wrapping or
reusing an exhausted value.

### Microtask Queue

```go
if err := js.QueueMicrotask(func() {
    fmt.Println("Microtask 1")
}); err != nil {
    log.Fatal(err)
}

if _, err := js.SetTimeout(func() {
    fmt.Println("Timer")
}, 0); err != nil {
    log.Fatal(err)
}

if err := js.QueueMicrotask(func() {
    fmt.Println("Microtask 2")
}); err != nil {
    log.Fatal(err)
}

// Output:
// Microtask 1
// Microtask 2
// Timer
```

### Promises

#### Creating Promises

```go
// Create a pending promise with resolver and rejector functions
promise, resolve, reject := js.NewChainedPromise()

// Resolve asynchronously
go func() {
    result, err := doAsyncWork()
    if err != nil {
        reject(err)
    } else {
        resolve(result)
    }
}()

// Chain handlers
promise.
    Then(func(v any) any {
        fmt.Printf("Got: %v\n", v)
        return transform(v)
    }, nil).
    Catch(func(r any) any {
        fmt.Printf("Error: %v\n", r)
        return nil
    }).
    Finally(func() {
        cleanup()
    })
```

#### Promise Static Methods

```go
// Promise.resolve - create already-settled promise
resolvedPromise := js.Resolve(42)

// Promise.reject - create already-rejected promise
rejectedPromise := js.Reject(errors.New("failed"))

// These create promises that are already settled without waiting
```

### Promise Combinators

```go
// Promise.all - wait for all to resolve
allPromise := js.All([]*eventloop.ChainedPromise{p1, p2, p3})

// Promise.race - first to settle wins
racePromise := js.Race([]*eventloop.ChainedPromise{p1, p2, p3})

// Promise.allSettled - wait for all to settle
settledPromise := js.AllSettled([]*eventloop.ChainedPromise{p1, p2, p3})

// Promise.any - first to resolve wins
// The returned promise rejects with AggregateError if all inputs reject
anyPromise := js.Any([]*eventloop.ChainedPromise{p1, p2, p3})
```

If terminal state prevents an input reaction from executing, the combinator
rejects its non-empty aggregate with `ErrLoopTerminated` when that failure wins
the returned promise's settlement claim instead of leaving it pending. Across
different event loops, concurrently executing reactions arbitrate at that
atomic claim; wall-clock source-settlement order establishes no `Race` or `Any`
precedence because those loops have no shared microtask queue.

#### AggregateError

When all inputs reject, the promise returned by `Promise.any` rejects with an
`AggregateError` containing every rejection reason:

```go
// Handling AggregateError from Go
promise := js.Any([]*eventloop.ChainedPromise{
    js.Reject(errors.New("error 1")),
    js.Reject(errors.New("error 2")),
})
promise.Catch(func(r any) any {
    if agg, ok := r.(*eventloop.AggregateError); ok {
        log.Printf("All promises failed. Reasons:")
        for i, err := range agg.Errors {
            log.Printf("  [%d] %v", i, err)
        }
    }
    return nil
})
```

### Go-Native Cancellation and Events

`AbortController` publishes one stable reason. `ThrowIfAborted` returns a
non-nil error reason by identity and wraps other reasons, including typed-nil
errors, once. `AbortAny` removes its internal source propagation links when one
source wins; runtime cleanup removes them after an unsettled composite becomes
unreachable. `AbortTimeout` uses a referenced loop timer;
a manual winning abort cancels that timer. The timer callback or one `Abort`
invocation atomically claims settlement; every losing `Abort` waits only until
the winner publishes the signal's stable reason. A nil loop, negative
millisecond delay, or duration overflow is a
programming error and panics.
Terminal loop cleanup releases a pending timeout's loop reference even though
discarding the timer does not itself abort the signal.
Abort handler delivery continues after a panic and re-raises the first panic
after cleanup. A later `runtime.Goexit` ends the remaining delivery but does not
suppress an earlier captured panic.
Timeout-triggered handlers execute on a delegated-owner goroutine while the loop
waits, so callback-local scheduling and lifecycle rules remain owner-local while
`runtime.Goexit` cannot abandon the loop or its timer.

Required callbacks are static API contracts. `Submit`, phase schedulers, core
and JS timers, immediates, microtasks, next ticks, `Promisify`, and `Try` panic
synchronously when passed nil. Optional Promise reaction callbacks remain nil
to express pass-through behavior. Promise self-resolution and typed-nil native
Promise adoption reject with `ErrPromiseSelfResolution` and
`ErrPromiseNilAdoption`, which callers can match with `errors.Is`.

`EventTarget` invokes listeners synchronously on the dispatching goroutine. A
successful removal suppresses a listener that has not crossed its callback-start
claim. Once listeners are claimed before invocation, so concurrent or recursive
dispatch cannot invoke one twice. Listener panics propagate. The same `Event`
may be reused after dispatch returns, but recursively dispatching the same
pointer panics even if a listener overwrites the pointed-to value. Dispatching a
copied `Event` establishes an independent address identity. Internal dispatch
bookkeeping retains an active `Event` only for that dispatch and does not retain
it after dispatch; ordinary copied fields such as `Target` and custom-event
detail retain their referenced values normally. Dispatch-owned target,
cancellation, and propagation outcome also live outside the replaceable value
while active, so whole-value overwrite cannot erase an earlier
`PreventDefault` or `StopImmediatePropagation`. `EventTarget` contains
synchronization state and must not be copied; use a distinct zero value or call
`NewEventTarget` for an independent target. Listener IDs are unique among live
registrations on one target; successful removal or once claim invalidates an ID.
After a full counter wrap, a released value may be reused, so a stale same-type
ID can theoretically exhibit ABA behavior.

These are Go APIs inspired by familiar browser names, not DOM implementations.
The separate `goja-eventloop` module owns JavaScript-visible behavior.

### Unhandled Rejection Tracking

```go
js := eventloop.NewJS(loop,
    eventloop.WithUnhandledRejection(func(reason any) {
        log.Printf("Unhandled rejection: %v", reason)
    }),
)
```

Unhandled rejection callbacks normally run on the logical callback owner at a
microtask checkpoint. A diagnostic already accepted by a graceful terminal
drain retains that owner and completes before terminal completion. Once no
logical owner remains, the default fallback drains bookkeeping without invoking
user code and calls `Logger.Log` directly through the instance-scoped
`Loop.Log` path; a nil or disabled logger emits nothing. Select
`WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated)` only when
the handler is safe to run on an isolated goroutine and cannot touch a Goja
runtime or another loop-affine resource.

### Performance Metrics

```go
loop := eventloop.New(
    eventloop.WithMetrics(true),
)

// Later, take a detached, coherent snapshot.
metrics := loop.Metrics()
fmt.Printf("P99 Callback Duration: %v\n", metrics.Latency.P99)
fmt.Printf("Observed Callbacks: %d\n", metrics.Latency.Count)
fmt.Printf("Successful TPS: %.2f\n", metrics.TPS)
fmt.Printf("Latest Ingress Sample: %d\n", metrics.Queue.IngressCurrent)
```

## Architecture

See [docs/architecture.md](docs/architecture.md) for the current architecture notes, Mermaid state-machine diagrams, compatibility labels, and platform caveats. The short version:

1. **Loop**: owner-topology scheduler for timers, typed command ingress, task queues, microtasks, terminal drain, and platform polling.
2. **JS Adapter**: JavaScript-style timer, immediate, microtask, and promise APIs.
3. **ChainedPromise**: Promise-style chaining with event-loop microtask scheduling; not a full ECMAScript conformance target.
4. **Deadline-list timers**: timer buckets are keyed by monotonic millisecond deadlines; same-deadline timers preserve FIFO, exact deadlines order within a bucket, repeating intervals keep stable timer IDs, and successful scheduling publishes an ID before callback entry.
5. **Platform Pollers**: epoll serves Linux/Android; kqueue serves Darwin, iOS, DragonFly, FreeBSD, NetBSD, and OpenBSD; poll serves AIX/ppc64 and Solaris/illumos on amd64. All readiness backends use owned descriptors, non-reused generations, dense storage plus sparse fallback, valid descriptor zero, per-registration coalescing, and terminal ownership cleanup. Windows, Plan 9, js/wasm, and wasip1/wasm use the task-only channel path and return `ErrReadinessUnsupported` from FD readiness operations.
6. **Queue snapshots**: check, close, internal, and external phases process
   phase-entry snapshots with owner-local fast paths rather than arbitrary
   callback-count caps. Detached check and close batches remain visible to
   `Alive` and `HasMacrotaskWork` until every accepted callback in the batch is
   finished or discarded by immediate Close. Within a queue or timer-state
   domain, a foreign-goroutine operation that returns before a later owner-local
   mutation or liveness observation begins is observed first; priority between
   distinct event-loop phases is unchanged.

## Lifecycle

- `Done()` returns the loop's stable terminal-cleanup signal. Integrations can
  use it to release admitted work only after no accepted callback can still
  execute; `StateTerminated` alone is not that completion guarantee.
- `Run(ctx)` blocks until loop-owner execution exits. When an immediate `Close`
  wins, `Run` also waits for terminal resource cleanup and includes cleanup
  failures in its result. Graceful terminal completion may continue after
  `Run` while already-admitted `Promisify` functions finish. External
  `Shutdown` or `Close` calls whose initial completion probe observes that
  barrier still open join its result; `Shutdown` remains bounded by its context.
- With auto-exit enabled, context cancellation already visible at the final
  terminal-admission boundary wins and is included in `Run`'s result. Once clean
  auto-exit commits that admission, later cancellation does not replace its nil
  result. Any aborted auto-exit decision lowers its provisional quiescing gate
  before a quiescence callback or ordinary loop admission resumes.
- `Shutdown(ctx)` is graceful. Every external caller whose call overlaps an open
  terminal-completion barrier waits for complete terminal cleanup or for its own
  `ctx` to end; cleanup continues independently after a context error. A call
  begun after terminal completion returns `ErrLoopTerminated`. From a
  loop callback, Shutdown initiates or acknowledges non-immediate termination
  idempotently and returns without waiting on the same goroutine; this remains
  true for a synchronous cleanup diagnostic after the drain ends. A `Promisify` worker that wins
  or observes an active graceful Shutdown likewise returns nil once an
  independent finisher owns the request; nil acknowledges graceful termination,
  not completed cleanup, because cleanup waits for that worker to return. A
  worker that observes immediate termination receives `ErrLoopTerminated`.
- `Close()` is immediate-stop and must be called from a non-loop goroutine; a
  loop callback receives `ErrReentrantClose` instead of deadlocking. A winning
  Close rejects registered pending promises before waiting for the loop owner to exit,
  lets a callback already claimed by that owner finish, discards callbacks not
  yet claimed, and releases loop resources, but does not wait for `Promisify`
  user functions that already claimed entry. Those functions retain their
  caller-provided contexts and may continue after Close returns. The claim is
  the lifecycle boundary: a claimed worker may execute its first user-function
  instruction even after Close has already returned. A committed worker that has
  not claimed entry when Close wins skips its user function. Subsequent loop-work
  submissions are rejected, and JS adapter handle publication racing terminal
  cleanup returns `ErrLoopTerminated` without leaving stale timeout, interval,
  or immediate entries. If another terminal transition already chose the mode,
  an external Close joins its open completion barrier and returns the same
  aggregate terminal error. A `Promisify` worker never joins a barrier that may
  depend on itself. Its Close returns nil as a request acknowledgement when
  immediate mode is already active, whether or not that worker won the
  transition, and `ErrLoopTerminated` when graceful mode won. A callback
  executing in the terminal drain retains drain ownership, so Shutdown
  acknowledges graceful mode and Close returns `ErrReentrantClose`. A post-drain
  dependency-release or cleanup diagnostic callback holding terminal-completion
  ownership uses the mode-sensitive nonjoining result. A completion probe that
  observes already-published completion receives `ErrLoopTerminated`.
- `Requests()` returns a transferable, nonjoining `LoopRequests` capability for
  dependency goroutines. Its `Shutdown` and `Close` methods acknowledge the
  committed terminal mode without waiting for cleanup; nil is not a cleanup
  result. Its timer cancellation and ref methods acknowledge FIFO admission
  without waiting for owner application or reporting timer existence. Graceful
  shutdown applies admitted timer requests; immediate Close may discard them.
  Ordinary external lifecycle calls retain their role-permitted join behavior,
  while post-Run ordinary timer mutations normally wait for owner application;
  terminal dependency release may instead complete an admitted ref/unref as an
  unobservable successful no-op or return `ErrLoopTerminated` when an exact
  cancellation result is unavailable.
- `terminalDone` follows loop-owner exit through `loopDone`, including auto-exit,
  and publishes the aggregate result after the terminal cleanup attempt.
  Recoverable platform cleanup state may remain retained for a later lifecycle
  retry when that result reports its failure. `StateTerminated` closes admission
  earlier and does not by itself prove the cleanup attempt completed.
- During `Run`, context cancellation and committed graceful `Shutdown` do not
  stop owner execution until owner work returns to the outer run loop. Neither
  preempts a callback or its exhaustive checkpoint; the owner may recursively
  enqueue microtask, nextTick, checkpoint, and Promise continuations during
  ordinary or terminal drain. Accepted continuations and Promise dependencies
  finish, while an unbounded chain can delay host control like a JavaScript
  microtask chain. New `Submit` / `SubmitInternal` work is rejected so shutdown
  cannot become an unbounded task pump. The terminal-drain pass processes check,
  close, internal, and external callbacks still queued at its entry in that
  order, with an initial checkpoint and another after every callback and phase.
  It does not invoke timers; cleanup discards timers that remain pending. A
  normal turn already in progress may finish eligible callbacks before the
  terminal drain begins.
- If `Shutdown` is called before `Run`, there is no loop goroutine yet; a
  dedicated terminal finisher drains already accepted callbacks before waiting
  for dependent Promisify workers. The Shutdown caller does not own that drain.
  Start `Run` before queuing Goja or other goroutine-affine callbacks.

## Thread Safety

- **Loop**: Safe for concurrent use; use `Submit()` to schedule from any goroutine
- **JS**: Thread-safe; `SetTimeout/SetInterval/QueueMicrotask` from any goroutine. Once `Run` owns the loop, callbacks execute serially on its logical callback-owner goroutine.
- **ChainedPromise**: Thread-safe. Promise methods (`Then/Catch/Finally`) and resolve/reject functions can be called from any goroutine

### Thread Safety Guarantees

- `Loop`: Safe for concurrent use from multiple goroutines
- `JS`: Thread-safe; timer/microtask scheduling from any goroutine. Once `Run`
  owns the loop, callbacks execute serially on the event-loop's logical callback
  owner while the physical `Run` goroutine waits. Panics and `runtime.Goexit` are
  contained at that boundary; `Goexit` replaces the logical owner before later
  callbacks. Before `Run`, a
  graceful Shutdown uses a dedicated terminal-finisher goroutine, so callers
  must not queue runtime-affine callbacks and assume caller-goroutine execution.
- `ChainedPromise`: Thread-safe. Then/Catch/Finally can be chained concurrently. Resolve/Reject functions: Can be called from any goroutine without synchronization

Exception: post-termination unhandled-rejection fallback diagnostics are not
normal callbacks. By default bookkeeping is drained without invoking user code
and `Logger.Log` is called directly through the instance-scoped `Loop.Log` path;
a nil or disabled logger emits nothing;
callers may explicitly select an isolated fallback goroutine with
`WithUnhandledRejectionFallback`.

## Performance

- Timer scheduling: monotonic millisecond deadline buckets with O(1) same-deadline append, stable native interval handles, saturating ID allocation, and a callback publication barrier
- Microtask queue: owner-local nextTick / Promise / checkpoint queues for logical-owner scheduling, with typed command ingress for external goroutines
- I/O dispatch: loop-owned ready-event dispatch with generation checks, panic recovery, microtask checkpoints, and hybrid dense/sparse FD storage
- Memory: bounded queue, phase, timer-index, and adapter-registry reuse retains ordinary warmed storage while discarding oversized retired backings; callback slots are cleared, timer nodes are pooled, and fast-mode sleep timers are reused
- Metrics: one private sampler publishes a detached value only after a complete metric epoch. The historical `Latency` field measures admitted callback execution duration rather than queue residence, TPS counts successful returns, and queue depths include external tasks plus immediate and close phases sampled at startup, fast-path, and polling owner turns

### Performance Evidence

Run the live product benchmark lane from the monorepo root:

```sh
gmake eventloop-product-bench
```

This target measures the current eventloop package only. Preserve the exact
command, source identity, platform, and raw output with any result used as
release evidence.

Historical tournament instructions are archived in
[`docs/tournament/README.md`](docs/tournament/README.md). That corpus is
incomplete and unqualified and may omit major historical variants, so it does
not establish a correctness winner, longitudinal baseline, or current product
performance claim. Dated snapshots distinguish absent or non-equivalent rows
from measured deltas.

Darwin runtime verification runs on the native development host and Linux runs
through Docker. Other targets currently have compile or compile/link evidence as
reported by the cross-platform lane; those results are not runtime evidence.

## License

MIT License - see [LICENSE](LICENSE) for details.
