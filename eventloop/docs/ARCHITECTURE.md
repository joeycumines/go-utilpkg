# Event Loop Architecture

This document describes the internal architecture of the `eventloop` package.

## Component Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Event Loop System                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────────────────────┐   │
│  │  External   │     │   Timer     │     │       Platform Poller       │   │
│  │   Queue     │     │   Heap      │     │  (kqueue/epoll/IOCP)        │   │
│  │ (ChunkedIn) │     │  (min-heap) │     └──────────────┬──────────────┘   │
│  └──────┬──────┘     └──────┬──────┘                    │                  │
│         │                   │                           │                  │
│         ▼                   ▼                           ▼                  │
│  ┌────────────────────────────────────────────────────────────────────┐   │
│  │                           LOOP CORE                                 │   │
│  │  ┌──────────────────────────────────────────────────────────────┐  │   │
│  │  │                      tick() cycle                             │  │   │
│  │  │  1. runTimers()     → Execute expired timers                  │  │   │
│  │  │  2. processInternal → Drain internal priority queue           │  │   │
│  │  │  3. processExternal → Drain external queue (budgeted)         │  │   │
│  │  │  4. drainMicrotasks → Execute microtask queue                 │  │   │
│  │  │  5. poll()          → Block for I/O or wakeup                 │  │   │
│  │  │  6. drainMicrotasks → Final microtask flush                   │  │   │
│  │  └──────────────────────────────────────────────────────────────┘  │   │
│  └────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────┐     ┌─────────────────┐     ┌─────────────────────────┐   │
│  │microtaskRing │     │ Internal Queue │     │    Promise Registry    │   │
│  │ (lock-free) │     │(chunkedIngress) │     │   (weak refs + GC)     │   │
│  └─────────────┘     └─────────────────┘     └─────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              JS Adapter Layer                                │
├─────────────────────────────────────────────────────────────────────────────┤
│  setTimeout / setInterval / clearTimeout / clearInterval                    │
│  queueMicrotask                                                             │
│  Promise (ChainedPromise wrapper)                                           │
│  AbortController / AbortSignal                                              │
│  performance.now() / performance.mark() / performance.measure()             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Data Flow

### Task Submission

```
External Goroutine          Event Loop Thread
       │                           │
       │  Submit(task)             │
       │──────────────────────────▶│
       │    [mutex lock]           │
       │    external.Push(task)    │
       │    [mutex unlock]         │
       │    doWakeup()             │
       │                           │
       │                    ┌──────┴──────┐
       │                    │ processExt  │
       │                    │ task()      │
       │                    └─────────────┘
```

### Fast Path Mode

When no user I/O file descriptors are registered, the loop uses a channel-based
fast path that achieves ~50ns wakeup latency instead of ~10µs with kqueue/epoll:

```
┌──────────────────────────────────────────────────────────────────┐
│  Fast Path Mode (userIOFDCount == 0)                             │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Submit() ──▶ auxJobs slice ──▶ fastWakeupCh (buffered chan)    │
│                     │                     │                      │
│                     ▼                     ▼                      │
│              ┌─────────────────────────────────┐                │
│              │   runFastPath() select loop    │                │
│              │   - No kqueue/epoll            │                │
│              │   - No OS thread locking       │                │
│              │   - ~50ns latency             │                │
│              └─────────────────────────────────┘                │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────┐
│  I/O Mode (userIOFDCount > 0)                                    │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Submit() ──▶ external queue ──▶ wakePipe write                 │
│                     │                     │                      │
│                     ▼                     ▼                      │
│              ┌─────────────────────────────────┐                │
│              │   poll() with kqueue/epoll     │                │
│              │   - OS thread locked           │                │
│              │   - ~10µs latency              │                │
│              │   - Full I/O support           │                │
│              └─────────────────────────────────┘                │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

## Thread Model

### Goroutine Architecture

```
┌───────────────────────────────────────────────────────────────────────┐
│                        Thread/Goroutine Model                          │
├───────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  Main Goroutine (caller)                                              │
│  ├── Creates Loop                                                     │
│  ├── Starts loop.Run(ctx) in separate goroutine                      │
│  └── Schedules work via Submit()/SubmitInternal()                    │
│                                                                       │
│  Loop Goroutine (locked to OS thread when using I/O)                 │
│  ├── Runs tick() cycle continuously                                  │
│  ├── Executes all callbacks (timers, tasks, microtasks)              │
│  ├── Polls for I/O events (kqueue/epoll/IOCP)                        │
│  └── Terminates on Shutdown() or context cancellation                │
│                                                                       │
│  Context Watcher Goroutine                                            │
│  ├── Monitors ctx.Done()                                              │
│  └── Wakes loop on cancellation                                       │
│                                                                       │
│  Promisify Goroutines (per Promisify call)                           │
│  ├── Execute blocking Go code                                        │
│  ├── Track via WaitGroup                                              │
│  └── Resolve/reject via SubmitInternal                               │
│                                                                       │
└───────────────────────────────────────────────────────────────────────┘
```

### Thread Safety Guarantees

| Component | Thread Safety | Access Pattern |
|-----------|---------------|----------------|
| Loop.Submit | ✅ Safe | Any goroutine |
| Loop.SubmitInternal | ✅ Safe | Any goroutine |
| Loop.ScheduleTimer | ✅ Safe | Any goroutine |
| Loop.CancelTimer | ✅ Safe | Any goroutine |
| Loop.ScheduleMicrotask | ✅ Safe | Any goroutine |
| ChainedPromise.Then/Catch/Finally | ✅ Safe | Any goroutine |
| resolve/reject functions | ✅ Safe | Any goroutine |
| Callback execution | ⚠️ Loop thread only | Single consumer |
| microtaskRing | ✅ Lock-free | MPSC pattern |
| chunkedIngress | ❌ External mutex | Mutex-protected |

## State Machine

```
                    ┌───────────────────────────────────────────┐
                    │              STATE MACHINE                 │
                    └───────────────────────────────────────────┘

                              ┌─────────┐
                              │  Awake  │ (initial state)
                              │  (0)    │
                              └────┬────┘
                                   │ Run()
                                   ▼
                    ┌─────────────────────────────┐
              ┌─────│          Running            │◀────┐
              │     │            (4)              │     │
              │     └─────────────┬───────────────┘     │
              │                   │                     │
              │  Shutdown()       │ poll()              │ wake
              │                   ▼                     │
              │     ┌─────────────────────────────┐     │
              │     │          Sleeping           │─────┘
              │     │            (2)              │
              │     └─────────────┬───────────────┘
              │                   │
              │  Shutdown()       │ Shutdown()
              ▼                   ▼
        ┌─────────────────────────────────────┐
        │            Terminating              │
        │               (5)                   │
        └─────────────────┬───────────────────┘
                          │ shutdown complete
                          ▼
                    ┌─────────┐
                    │Terminated│ (terminal)
                    │   (1)   │
                    └─────────┘
```

### State Transition Rules

- **Awake → Running**: Only via `Run()` call
- **Running ↔ Sleeping**: Via CAS in `poll()`
- **Running/Sleeping → Terminating**: Via `Shutdown()` or context cancellation
- **Terminating → Terminated**: After all queues drained
- **Terminated**: Terminal state, no further transitions

## Platform Pollers

| Platform | Poller | Wake Mechanism | Thread Lock Required |
|----------|--------|----------------|---------------------|
| Darwin | kqueue(2) | pipe | Yes |
| Linux | epoll(7) | eventfd(2) | Yes |
| Windows | IOCP | PostQueuedCompletionStatus | No |

Fast path (no user I/O FDs): channel-based, ~50ns wakeup latency.
I/O path: platform poller, ~8-15µs wakeup latency.

See `poller_darwin.go`, `poller_linux.go`, `poller_windows.go` for details.

## Allocation Profile

| Operation | Allocations | Notes |
|-----------|-------------|-------|
| Timer schedule | ~7 allocs | Result channel, closures |
| Microtask queue | 0 allocs | Ring buffer, steady state |
| Chunked ingress push | 0 allocs | Chunk pooling |
| Submit (fast path) | <2 allocs | Slice growth amortized |
| Promise create | ~3 allocs | Promise struct, closures |

## Queue System

### Chunked Ingress Queue (External/Internal Queues)

```
┌────────────────────────────────────────────────────────────────┐
│  chunkedIngress - Chunked Linked-List Queue                    │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  head ──▶ [chunk 0: 64 tasks] ──▶ [chunk 1] ──▶ [chunk 2]     │
│                                                      ▲         │
│                                                      │         │
│                                                    tail        │
│                                                                │
│  Features:                                                     │
│  - 64 tasks per chunk (~512B)                                 │
│  - sync.Pool for chunk recycling                              │
│  - O(1) push/pop with cursor tracking                         │
│  - Requires external mutex                                     │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

### Microtask Ring Buffer (Lock-Free)

```
┌────────────────────────────────────────────────────────────────┐
│  microtaskRing - Lock-Free MPSC Ring Buffer                    │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  [4096 slots] with sequence numbers for ABA prevention         │
│                                                                │
│       tail                    head                             │
│        │                       │                               │
│        ▼                       ▼                               │
│  ┌───┬───┬───┬───┬───┬───┬───┬───┬───┬───┐                   │
│  │   │ X │ X │ X │ X │   │   │   │   │   │                   │
│  └───┴───┴───┴───┴───┴───┴───┴───┴───┴───┘                   │
│        ├───────────────┤                                       │
│        └── valid items ─┘                                      │
│                                                                │
│  Overflow: When ring full, falls back to mutex slice           │
│  Algorithm: CAS-based with Release/Acquire ordering            │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

## Timer System

### Timer Heap

```
┌────────────────────────────────────────────────────────────────┐
│  Timer Heap - Min-Heap by Expiration Time                      │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│                    ┌────────────┐                              │
│                    │  t=100ms   │ ◀── root (earliest)          │
│                    └─────┬──────┘                              │
│               ┌──────────┴──────────┐                          │
│          ┌────┴────┐           ┌────┴────┐                     │
│          │ t=200ms │           │ t=150ms │                     │
│          └─────────┘           └─────────┘                     │
│                                                                │
│  Operations:                                                   │
│  - Schedule: O(log n) heap.Push                               │
│  - Cancel: O(log n) heap.Remove via heapIndex                 │
│  - Fire: O(log n) heap.Pop                                    │
│                                                                │
│  Timer Pool: sync.Pool for zero-allocation steady state       │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

### HTML5 Timer Clamping

Per the HTML Living Standard, nested timeouts (depth > 5) are clamped to 4ms minimum:

```go
// HTML5 nested timeout clamping
if nestingDepth > 5 && delay < 4 * time.Millisecond {
    delay = 4 * time.Millisecond
}
```

### Thread Identity Check: Performance Trade-off

`ScheduleTimer` must determine whether the caller is the loop goroutine to decide
between **direct registration** (synchronous, on the loop thread) and **queued
submission** (asynchronous, via `submitToQueue`). This check is performed by
`isLoopThread()`, which uses `runtime.Stack()` to capture a partial stack trace:

```go
func (l *Loop) isLoopThread() bool {
    buf := getBuf()
    n := runtime.Stack(buf, false)
    // parse goroutine ID from stack trace...
}

func getBuf() []byte {
    buf := bufPool.Get().([]byte)
    if len(buf) == 0 {
        buf = make([]byte, 2048)
    }
    return buf[:2048]
}
```

**Performance cost:** `runtime.Stack()` costs approximately **1,760 ns** on
Darwin ARM64 and **2,413 ns** on Windows AMD64 per call. This overhead is paid
on **every** `ScheduleTimer` call from an external goroutine.

**Why it cannot be guarded cheaply:** The old code (pre-auto-exit) guarded
`isLoopThread()` behind `canUseFastPath() && state == StateRunning`:

```go
// Old SubmitInternal pattern:
if l.canUseFastPath() && state == StateRunning && l.isLoopThread() {
    // direct path
}
```

This saved the `runtime.Stack()` cost when the loop was sleeping (`StateSleeping`).
However, the auto-exit feature requires **direct registration on the loop thread**
to prevent the following race:

```
1. Loop callback calls ScheduleTimer() → registration queued (external path)
2. Same callback calls UnrefTimer() → timer not yet in timerMap → no-op
3. Registration processed → timer is ref'd but UnrefTimer already returned
4. Timer never unregistered → refedTimerCount stays > 0 → loop never exits
```

By calling `isLoopThread()` unconditionally, the implementation ensures that loop
callbacks always use direct registration, making `UnrefTimer` synchronous and
correct. The cost is ~1,760 ns per external `ScheduleTimer` call.

**Current workaround:** Schedule timers from within loop callbacks (e.g., via
`Submit()`) to avoid the `isLoopThread()` overhead, since the check returns
immediately after `runtime.Stack()` when called from the loop goroutine.

**Future improvement:** A goroutine-local flag (set during `Run()`, cleared on exit)
could provide a <10 ns thread identity check without `runtime.Stack()`. This
would require architecting `drainAuxJobs()` to handle the loop-thread-direct-path
invariant differently, as the current design relies on the `isLoopThread()` race
prevention during `StateSleeping`.

## Promise System

### ChainedPromise Lifecycle

```
┌────────────────────────────────────────────────────────────────┐
│  ChainedPromise State Transitions                              │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│                    ┌─────────────┐                             │
│                    │   Pending   │                             │
│                    │    (0)      │                             │
│                    └──────┬──────┘                             │
│                           │                                    │
│          ┌────────────────┼────────────────┐                   │
│          │ resolve(val)   │   reject(err)  │                   │
│          ▼                │                ▼                   │
│    ┌───────────┐          │          ┌───────────┐             │
│    │ Fulfilled │          │          │ Rejected  │             │
│    │    (1)    │          │          │    (2)    │             │
│    └───────────┘          │          └───────────┘             │
│                           │                                    │
│  Handler execution via microtask queue                         │
│  Promise/A+ thenable resolution procedure                      │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

### Unhandled Rejection Detection

```
1. Promise rejected ──▶ trackRejection(p, reason)    // p is *ChainedPromise
2. Store in unhandledRejections map
3. Schedule checkUnhandledRejections microtask
4. Meanwhile, if .catch() attached:
   - Set promiseHandlers[p] = true
   - Signal via handlerReadyChans
5. checkUnhandledRejections runs:
   - If handler exists: remove from tracking (handled)
   - If no handler: invoke onUnhandled callback
```

## Shutdown Sequence

```
┌────────────────────────────────────────────────────────────────┐
│  Graceful Shutdown Sequence                                    │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  1. Shutdown() called                                          │
│     ├── State → Terminating                                   │
│     └── Wake loop if sleeping                                 │
│                                                                │
│  2. Loop detects Terminating state                            │
│     ├── Finish current tick                                   │
│     └── Exit run() loop                                       │
│                                                                │
│  3. transitionToTerminated()                                  │
│     ├── State → Terminated (under promisifyMu)                │
│     ├── Drain internal queue                                  │
│     ├── Drain external queue                                  │
│     ├── Drain auxJobs                                         │
│     ├── Drain microtasks                                      │
│     └── RejectAll pending promises                            │
│                                                                │
│  4. shutdown() from Shutdown caller thread                    │
│     ├── Wait for promisify goroutines (WaitGroup)             │
│     ├── Final queue draining (3 consecutive empty checks)     │
│     └── Close file descriptors                                │
│                                                                │
│  5. loopDone channel closed                                   │
│     └── Shutdown() returns to caller                          │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

## Auto-Exit Mode

### Motivation

When using the event loop in script-style workloads (e.g., running a JavaScript
file), the loop should terminate once all work completes -- just like Node.js
exits when the event loop drains. Without auto-exit, `Run()` blocks until the
context is cancelled or `Shutdown()`/`Close()` is called, which is appropriate
for long-lived servers but wrong for fire-and-forget scripts.

Auto-exit mode (`WithAutoExit(true)`) makes `Run()` return when `Alive()` is
false, analogous to libuv's `UV_RUN_DEFAULT` behavior.

### Implementation

When `autoExit` is enabled, the main loop (`run()`) and fast-path loop
(`runFastPath()`) check `Alive()` at the top of each iteration. If `Alive()`
returns false, the quiescing protocol is engaged before committing to
termination (see Quiescing Protocol below).

```
┌────────────────────────────────────────────────────────────────┐
│  Auto-Exit Decision Flow (run() main loop)                     │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  ┌──────────────┐                                              │
│  │ autoExit on? │─── No ──▶ normal loop iteration              │
│  └──────┬───────┘                                              │
│         │ Yes                                                  │
│         ▼                                                      │
│  ┌──────────────┐                                              │
│  │  Alive()?    │─── Yes ──▶ normal loop iteration              │
│  └──────┬───────┘                                              │
│         │ No (no liveness remains)                              │
│         ▼                                                      │
│  ┌──────────────────┐                                          │
│  │ Set quiescing=true│  ◀── gate liveness-adding APIs          │
│  └──────┬───────────┘                                          │
│         │                                                      │
│         ▼                                                      │
│  ┌──────────────┐                                              │
│  │ Alive()?     │─── Yes ──▶ clear quiescing, continue loop    │
│  │ (re-check)   │       (work arrived between checks)          │
│  └──────┬───────┘                                              │
│         │ No (confirmed dead)                                   │
│         ▼                                                      │
│  transitionToTerminated() ──▶ terminateCleanup() ──▶ return    │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

### Alive() Checks

`Alive()` reports whether the loop has ref'd pending work. It is false when
**all** of the following are zero/empty:

| Signal | Type | Meaning |
|--------|------|---------|
| `refedTimerCount` | `atomic.Int32` | Ref'd timers still pending |
| `promisifyCount` | `atomic.Int64` | In-flight Promisify goroutines |
| `userIOFDCount` | `atomic.Int32` | Registered user I/O file descriptors |
| `internal` queue | mutex-protected | Internal task queue length |
| `external` queue | mutex-protected | External task queue + auxJobs length |
| `microtasks` ring | lock-free | Microtask ring buffer |
| `nextTickQueue` ring | lock-free | NextTick ring buffer |

## Quiescing Protocol

### Motivation

A race exists between the auto-exit decision (`Alive()` returns false) and
concurrent API calls from external goroutines that add liveness (e.g.,
`ScheduleTimer`, `RegisterFD`, `Promisify`). Without protection, the loop could
decide to exit while a timer is being scheduled, causing the timer to be lost.

The quiescing protocol closes this race by gating all liveness-adding APIs
during the brief termination window.

### Implementation

An atomic `quiescing` flag (`atomic.Bool`) is set by the loop goroutine **before**
committing to termination. All liveness-adding APIs check this flag and reject
work with `ErrLoopTerminated` when set.

```
┌────────────────────────────────────────────────────────────────┐
│  Quiescing Protocol - Gated APIs                               │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  Loop Goroutine                     External Goroutine         │
│  ─────────────                      ──────────────────         │
│                                                                │
│  1. Alive() → false                                           │
│  2. quiescing.Store(true)            3. ScheduleTimer()        │
│                                      4. quiescing.Load()→true │
│                                      5. return ErrLoopTerminated│
│  6. Alive() → false (re-check)                                │
│  7. transitionToTerminated()                                  │
│  8. terminateCleanup()                                        │
│  9. quiescing.Store(false)                                    │
│                                                                │
│  ─── OR (termination abort) ───                               │
│                                                                │
│  1. Alive() → false                                           │
│  2. quiescing.Store(true)            3. ScheduleTimer()        │
│     [race: timer added before                             │
│      quiescing was visible]                                    │
│                                      4. quiescing.Load()→false │
│                                      5. timer accepted         │
│  6. Alive() → TRUE (epoch changed!)                           │
│  7. quiescing.Store(false)            ◀── abort termination   │
│  8. continue normal loop                                       │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

### Gated vs. Non-Gated APIs

**Gated** (liveness-adding -- rejected during quiescing):

| API | Reason |
|-----|--------|
| `ScheduleTimer` | Adds a ref'd timer (increases `refedTimerCount`) |
| `RefTimer` | Marks timer as keeping loop alive |
| `Promisify` | Spawns a goroutine (increases `promisifyCount`) |
| `RegisterFD` | Registers an I/O FD (increases `userIOFDCount`) |
| `submitToQueue` | Internal: adds tasks to the internal queue |

**NOT gated** (liveness-reducing or ephemeral -- allowed during quiescing):

| API | Reason |
|-----|--------|
| `CancelTimer` / `CancelTimers` | Reduces liveness (removes timers) |
| `UnrefTimer` | Reduces liveness (unrefs a timer) |
| `Submit` | Ephemeral: detected by `submissionEpoch` in `Alive()` |
| `ScheduleMicrotask` | Ephemeral: detected by `submissionEpoch` in `Alive()` |
| `ScheduleNextTick` | Ephemeral: detected by `submissionEpoch` in `Alive()` |

The ephemeral APIs (`Submit`, `ScheduleMicrotask`, `ScheduleNextTick`) are
intentionally not gated because they represent self-draining work. Their arrival
is detected by the epoch-based consistency mechanism inside `Alive()`, which
causes the termination to be aborted. Rejecting them would be actively harmful:
it would discard work that correctly prevents an otherwise-invalid termination.

### TOCTOU Defense in submitToQueue

`submitToQueue` performs a second quiescing check **under the
`internalQueueMu` lock**, before the task is pushed and before the epoch
is incremented. This closes a time-of-check-to-time-of-use gap: an external
goroutine may have passed the API-level quiescing check before the flag was set,
but this inner check (under the same lock that guards epoch increment) ensures
correctness.

```
submitToQueue(task):
    internalQueueMu.Lock()
    if StateTerminated → reject
    if quiescing → reject          ◀── TOCTOU defense
    internal.Push(task)
    submissionEpoch.Add(1)
    internalQueueMu.Unlock()
    ... wakeup logic ...
```

### Flag Lifecycle

The `quiescing` flag is:

1. **Set** by the loop goroutine in `run()` or `runFastPath()` when `Alive()`
   returns false and auto-exit is enabled.
2. **Cleared** if the `Alive()` re-check detects new work (termination abort).
3. **Cleared** in `terminateCleanup()` after `transitionToTerminated()` completes,
   maintaining the invariant that `quiescing` is only true during the brief
   window between `!Alive()` and `transitionToTerminated()`.

The flag is never set when `autoExit` is false.

## Epoch-Based Alive() Consistency

### Motivation

`Alive()` reads multiple counters and queues that are mutated concurrently by
external goroutines. A naive implementation could observe a snapshot where all
counters are zero but work was added halfway through the checks, producing a
false negative (reporting dead when work is in flight). This would cause
premature termination and lost work.

### Implementation

A `submissionEpoch` (`atomic.Uint64`) is incremented after every work-adding
mutation: `Submit`, `ScheduleTimer`, `Promisify`, `ScheduleMicrotask`,
`ScheduleNextTick`, `RegisterFD`, `UnregisterFD`, `submitToQueue`, and
`applyTimerRefChange`.

`Alive()` reads the epoch **before** checking counters/queues and validates it
**after**. If the epoch changed during the checks (concurrent work was added),
the entire check is retried.

```
┌────────────────────────────────────────────────────────────────┐
│  Alive() - Epoch-Based Retry                                   │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  for attempt := 0; attempt < 3; attempt++ {                   │
│      epoch_before = submissionEpoch.Load()                     │
│                                                                │
│      ┌─── Fast path (no locks) ──────────────────────────┐    │
│      │ refedTimerCount    > 0?  → return true            │    │
│      │ promisifyCount     > 0?  → return true            │    │
│      │ userIOFDCount      > 0?  → return true            │    │
│      │ microtasks empty?       → return true if not      │    │
│      │ nextTickQueue empty?    → return true if not      │    │
│      └───────────────────────────────────────────────────┘    │
│                                                                │
│      ┌─── Slow path (mutexes) ───────────────────────────┐    │
│      │ internalQueueMu.Lock()                             │    │
│      │ internal.Length() > 0? → return true if so        │    │
│      │ internalQueueMu.Unlock()                           │    │
│      │                                                   │    │
│      │ externalMu.Lock()                                  │    │
│      │ external.Length() > 0 || auxJobs > 0?             │    │
│      │   → return true if so                              │    │
│      │ externalMu.Unlock()                                │    │
│      └───────────────────────────────────────────────────┘    │
│                                                                │
│      epoch_after = submissionEpoch.Load()                       │
│      if epoch_after == epoch_before → return false (dead)      │
│      // epoch changed → concurrent work added → RETRY          │
│  }                                                             │
│  return true  // max retries: conservatively say alive         │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

### Check Ordering

Atomic counters (`refedTimerCount`, `promisifyCount`, `userIOFDCount`) and
lock-free ring buffers (`microtasks`, `nextTickQueue`) are checked first because
they require no lock acquisition. Queue length checks (requiring
`internalQueueMu` and `externalMu`) are performed only when all fast-path checks
return zero. This reduces mutex contention under high load.

### Conservative Fallback

After 3 retries with a changed epoch each time, `Alive()` returns `true`. This
is the conservative choice: a false positive (saying alive when the loop might
actually be dead) is far safer than a false negative (saying dead when work was
just added), which would cause premature termination and lost work.

```
                    ┌─────────────────────────────┐
                    │  Retry Decision              │
                    └─────────────────────────────┘
                                 │
                    epoch unchanged?
                    ┌──── Yes ────┐  ┌──── No ────────────┐
                    │             │  │                     │
                    │ false       │  │ attempt < 3?        │
                    │ (dead)      │  │  Yes → retry         │
                    │             │  │  No  → true (alive)  │
                    └─────────────┘  │  (conservative)      │
                                     └─────────────────────┘
```

### Epoch Increment Points

Every API that adds liveness increments `submissionEpoch`:

| API | When Incremented |
|-----|-----------------|
| `Submit` | After pushing to external queue or auxJobs |
| `ScheduleTimer` | After heap push on loop thread, or inside `submitToQueue` closure |
| `Promisify` | After `promisifyWg.Add(1)` and `promisifyCount.Add(1)` |
| `ScheduleMicrotask` | After `microtasks.Push(fn)` |
| `ScheduleNextTick` | After `nextTickQueue.Push(fn)` |
| `RegisterFD` | After `userIOFDCount.Add(1)` |
| `UnregisterFD` | After `userIOFDCount.Add(-1)` (liveness changed) |
| `submitToQueue` | After `internal.Push(task)` |
| `applyTimerRefChange` | After `refedTimerCount` changes (only when `old != ref`) |

Note: `CancelTimer` and timer firing do **not** increment the epoch. These
operations reduce liveness, and the epoch-based retry only needs to detect
concurrent additions, not removals.

## Timer Ref/Unref

### Motivation

In libuv (Node.js's event loop), timers have a ref/unref mechanism: an unref'd
timer does not keep the event loop alive. This enables patterns like "start a
heartbeat timer but don't prevent the process from exiting if all other work is
done." Without unref, every active timer would keep the loop running
indefinitely.

The `RefTimer`/`UnrefTimer` APIs provide the same semantics.

### Implementation

Each timer has an `atomic.Bool` field (`refed`) that defaults to `true` when
created by `ScheduleTimer`. A global `refedTimerCount` (`atomic.Int32`) tracks
the number of ref'd timers currently in the heap.

```
┌────────────────────────────────────────────────────────────────┐
│  Timer Ref/Unref State Machine                                 │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  ScheduleTimer()                                               │
│  ┌─────────────────────┐                                       │
│  │ t.refed = true      │ (default: ref'd)                     │
│  │ refedTimerCount +1  │                                      │
│  │ submissionEpoch +1  │                                      │
│  └──────────┬──────────┘                                       │
│             │                                                  │
│      ┌──────┴───────┐                                          │
│      │              │                                          │
│      ▼              ▼                                          │
│  ┌────────┐   ┌─────────┐                                     │
│  │ Ref'd  │   │ Unref'd │                                     │
│  │ (alive)│   │ (idle)  │                                     │
│  └───┬────┘   └────┬────┘                                     │
│      │              │                                          │
│   UnrefTimer()   RefTimer()                                    │
│      │              │                                          │
│      ▼              ▼                                          │
│  ┌─────────┐   ┌────────┐                                     │
│  │ Unref'd │   │ Ref'd  │                                     │
│  │ (idle)  │   │ (alive)│                                     │
│  └─────────┘   └────────┘                                     │
│                                                                │
│  Timer fires or is cancelled:                                  │
│  ┌──────────────────────┐                                      │
│  │ if t.refed:          │                                      │
│  │   refedTimerCount -1 │                                      │
│  │   (no epoch change)  │  ◀── removals don't bump epoch      │
│  │ return to pool       │                                      │
│  └──────────────────────┘                                      │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

### Thread Safety

`RefTimer` and `UnrefTimer` are safe to call from any goroutine. The per-timer
`refed` state is stored as an `atomic.Bool`, and `refedTimerCount` is an
`atomic.Int32`.

When called from the **loop goroutine**, `applyTimerRefChange` applies the
change directly: looks up the timer in `timerMap`, swaps the `refed` flag, and
updates `refedTimerCount`.

When called from an **external goroutine**, the change is submitted
synchronously to the internal queue (via `submitToQueue`) and the caller blocks
until the loop goroutine processes it. This matches libuv semantics where
`uv_ref()`/`uv_unref()` take immediate effect.

```
  External Goroutine              Loop Goroutine
       │                               │
       │ submitTimerRefChange(id,ref)  │
       │──────────────────────────────▶│
       │  [blocked on result chan]     │
       │                               │ applyTimerRefChange(id, ref)
       │                               │   timerMap[id] lookup
       │                               │   t.refed.Swap(ref)
       │                               │   refedTimerCount.Add(±1)
       │                               │   submissionEpoch.Add(1) *
       │                               │   result <- struct{}{}
       │◀──────────────────────────────│
       │  [unblocked]                  │
       │  return nil                   │

  * epoch only incremented when old != ref (state actually changed)
  ** auto-exit only: doWakeup() called when old != ref
```

### Quiescing Interaction

`RefTimer` (ref=true) is gated by the quiescing flag because it adds liveness.
`UnrefTimer` (ref=false) is not gated because it reduces liveness. This
asymmetric gating is intentional: during the quiescing window, callers can
unref timers but not re-ref them.

### Alive() Impact

`refedTimerCount` is the first check in `Alive()` (fast path, no lock
required). When zero, the timer subsystem does not keep the loop alive. Unref'd
timers still execute normally when they fire -- they simply do not prevent the
loop from exiting.

### Pool Hygiene

When a timer is returned to the pool (after firing, cancellation, or cleanup),
its `refed` flag is reset to `false`. This prevents stale state from leaking
into reused timer objects. Similarly, `ScheduleTimer` always sets `refed` to
`true` on the pooled timer before use.
