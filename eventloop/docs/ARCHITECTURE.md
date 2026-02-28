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
if nestingDepth > 5 && delay < 4*time.Millisecond {
    delay = 4 * time.Millisecond
}
```

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
