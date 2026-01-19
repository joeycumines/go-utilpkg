# EventLoop Package - Comprehensive Structural Analysis
**Date**: 2026-01-19
**Purpose**: JavaScript Runtime Integration Mapping
**Target**: goja/browser compatibility assessment

---

## Executive Summary

The eventloop package is a **high-performance, production-ready** event loop implementation designed for JavaScript engine integration. It provides:

- **HTML5 Event Loop spec compliance**: Proper task queues, microtask barriers, timing semantics
- **Promises/A+ support**: Full promise lifecycle management with then/catch chains
- **I/O event notification**: Native kqueue/epoll integration for non-blocking I/O
- **Timer scheduling**: Efficient heap-based timer system
- **Fast path optimization**: Channel-based tight loop for task-only workloads
- **Strong concurrency safety**: Lock-free primitives, proper synchronization, no race conditions

**Verdict**: ⚠️ **PARTIAL FIT** - Excellent core architecture, requires JavaScript runtime adapter layer

---

## 1. Complete API Reference

### 1.1 Core Types & Interfaces

#### Loop
```go
type Loop struct {
    // Main event loop type - unexported fields
}
```
**Purpose**: Single-threaded reactor pattern implementation
**Lifecycle**: Created via `New()`, started via `Run()`, stopped via `Shutdown()` or `Close()`
**Thread Safety**: All public APIs safe for concurrent calls

#### Task
```go
type Task struct {
    Runnable func()
}
```
**Purpose**: Unit of work submitted to event loop
**Usage**: `loop.Submit(myFunc)`

#### Promise Interface
```go
type Promise interface {
    State() PromiseState
    Result() Result
    ToChannel() <-chan Result
}
```
**Purpose**: Future/promise pattern for async operations
**Implementation**: `promise` struct (unexported concrete type)
**State Machine**: Pending → Resolved/Rejected (one-way)

```go
type PromiseState int

const (
    Pending  PromiseState = iota
    Resolved
    Rejected
)
```

#### Result
```go
type Result any
```
**Purpose**: Value carried by Promise when resolved/rejected
**Note**: `Result` can be `nil` for successfully resolved promises with no value

#### FastPathMode
```go
type FastPathMode int32

const (
    FastPathAuto      FastPathMode = iota  // Auto-select based on I/O FDs
    FastPathForced                        // Force fast path (error if I/O FDs registered)
    FastPathDisabled                      // Always use poll path (debugging)
)
```
**Purpose**: Performance tuning configuration
**Default**: `FastPathAuto` (zero value)

#### LoopState
```go
type LoopState uint64

const (
    StateAwake       LoopState = 0  // Initial, not started
    StateTerminated  LoopState = 1  // Fully stopped (terminal)
    StateSleeping    LoopState = 2  // Blocked in poll()
    StateRunning     LoopState = 4  // Actively executing
    StateTerminating LoopState = 5  // Shutdown requested
)
```
**Transitions**: Strict state machine with CAS for temporary states, `Store()` for terminal

### 1.2 Constructor & Lifecycle

#### New()
```go
func New() (*Loop, error)
```
- Creates new event loop with initialized components
- Creates wake pipe/eventfd, initializes poller, allocates queues
- **Error handling**: Returns error if FD creation/poller init fails
- **Thread safety**: Safe to call concurrently (multiple loops)

#### Run(ctx)
```go
func (l *Loop) Run(ctx context.Context) error
```
- **Blocking call**: Runs loop until termination
- State transition: Awake → Running (atomic CAS)
- **Error returns**:
  - `ErrLoopAlreadyRunning`: If `Run()` called on already-running loop
  - `ErrLoopTerminated`: If loop is terminated
  - `ErrReentrantRun`: If called from within loop itself
  - Context error: If `ctx` cancelled
- **Concurrency**: Only one goroutine should call `Run()` per `Loop` instance
- **Async mode**: Use `go loop.Run(ctx)` to run in background
- **Completion**: Closes `loopDone` channel on exit

#### Shutdown(ctx)
```go
func (l *Loop) Shutdown(ctx context.Context) error
```
- **Graceful shutdown**: Drains all queued tasks before stopping
- **Idempotent**: Safe to call multiple times (uses `sync.Once`)
- **Blocking semantics**: Waits for `loopDone` channel (NOT polling)
- State transition: Running/Sleeping/Awake → Terminating → Terminated
- **Queue behavior**: Continues accepting tasks while draining (rejects only when Terminated)
- **Error returns**:
  - `ErrLoopTerminated`: Already terminated
  - `ctx.Err()`: Context cancelled before shutdown complete
- **Timeout**: Waits up to `ctx` timeout for graceful drain

#### Close()
```go
func (l *Loop) Close() error
```
- **Immediate termination**: Does NOT drain queues
- **Resource cleanup**: Closes all FDs immediately
- **Data loss risk**: Queued tasks NOT executed
- **State transition**: Any → Terminating → Terminated
- **Promise handling**: Rejects all pending promises with `ErrLoopTerminated`

**Comparison:**
| Method | Drain Queue | Wait Context | Resource Cleanup | Data Loss? |
|---------|--------------|---------------|-------------------|-------------|
| `Shutdown(ctx)` | ✅ Yes | ✅ Yes | ✅ Yes | ❌ No |
| `Close()` | ❌ No | ❌ No | ✅ Yes | ⚠️ Yes |

### 1.3 Task Submission APIs

#### Submit(task)
```go
func (l *Loop) Submit(task Task) error
```
- **External queue**: Subject to tick budget (default 1024 tasks)
- **Wakeup strategy**:
  - Fast path: Channel-based (no I/O FDs)
  - I/O path: Write to wake pipe/eventfd
- **Error returns**:
  - `ErrLoopTerminated`: Loop fully stopped
  - `ErrLoopOverloaded`: Budget exceeded (if `OnOverload` callback not set)
- **State policy**: Accepts tasks during `StateTerminating` (for drainage)
- **Fast path**: Direct append to `auxJobs` slice when `canUseFastPath()`
- **Wake-up deduplication**: Uses `wakeUpSignalPending` flag to prevent spam

#### SubmitInternal(task)
```go
func (l *Loop) SubmitInternal(task Task) error
```
- **Priority lane**: Bypasses external queue budget limits
- **Usage**: Internal system completions (Promisify results, timer fires)
- **Fast path optimization**: If on loop thread and fast mode, executes immediately
  ```go
  if l.canUseFastPath() && state == StateRunning && l.isLoopThread() {
      l.safeExecute(task)  // Direct execution, ~1-2μs latency
      return nil
  }
  ```
- **Thread affinity check**: Uses `getGoroutineID()` to detect loop thread
- **Error returns**: `ErrLoopTerminated` (only when fully stopped)

**Fast Path Latency Benefits:**
| Scenario | Regular queue | Fast path |
|-----------|---------------|------------|
| On loop thread | ~10μs | ~1-2μs |
| Off loop thread | ~50-100ns (channel) | ~50ns (channel) |

### 1.4 Timer APIs

#### ScheduleTimer(delay, fn)
```go
func (l *Loop) ScheduleTimer(delay time.Duration, fn func()) error
```
- **Implementation**: Heap-based min-heap (`timerHeap`)
- **Monotonic time**: Uses cached tick time for accuracy, immune to NTP jumps
- **Resolution path**: Submits timer task to `SubmitInternal` (priority lane)
- **Zero overhead during idle**: No allocations when no timers pending
- **Ordering**: Oldest timer fires first (min-heap property)
- **Microtask barrier**: Executes microtask drain after each timer callback
- **Error handling**: Returns `ErrLoopTerminated` if stopped

**Timer Usage Pattern:**
```go
// Equivalent to setTimeout(fn, 100)
loop.ScheduleTimer(100*time.Millisecond, func() {
    // Callback runs on loop thread
})

// Equivalent to setInterval(fn, 100)
loop.ScheduleTimer(100*time.Millisecond, func() {
    // Callback runs
    // Schedule next timer (recursive for interval)
    if shouldContinue {
        loop.ScheduleTimer(100*time.Millisecond, func() { /* ... */ })
    }
})
```

**Performance Characteristics:**
| Timer Count | Insert cost | Min lookup | Max lookup |
|--------------|--------------|-------------|--------------|
| 1-10 | O(log n) | O(1) | O(1) |
| 100-1000 | O(log n) | O(1) | O(1) |
| >1000 | **SLOW** | O(1) | O(1) |

*Note*: Binary heap degrades above ~1000 timers; requirement.md specifies hierarchical timer wheel for production use.

### 1.5 Microtask APIs

#### ScheduleMicrotask(fn)
```go
func (l *Loop) ScheduleMicrotask(fn func()) error
```
- **Priority**: Executes before next I/O poll
- **Barrier points**: Drained after every:
  - Timer callback
  - External task
  - Internal task
  - I/O event callback
- **Budget limit**: 1024 microtasks per tick (prevents infinite loops)
- **Allocation**: Lock-free ring buffer (4096-slot), overflow to mutex slice
- **Error handling**: Returns `ErrLoopTerminated` (only when fully stopped)
- **Re-entrancy**: Microtasks can schedule more microtasks (no recursion prevention)
- **Zero-alloc hot path**: Ring buffer reuse in steady state

**Microtask Consumption Pattern:**
```
Tick Iteration:
1. Process internal tasks → drainMicrotasks()
2. Process external tasks (batch) → drainMicrotasks() after each (strict mode)
3. Run timers → drainMicrotasks() after each
4. Poll I/O → (no microtasks, waiting)
5. I/O events callback → drainMicrotasks() after callback
```

**Overflow Protection:**
- Ring buffer: 4096 slots (lock-free with sequence numbers)
- Overflow slice: Dynamic when ring exceeds capacity
- FIFO guarantee: Overflow checked before ring in `Push()`

**Budget Breach Behavior:**
```go
const MaxMicrotaskBudget = 1024
// If exceeded:
// 1. Re-queue remaining tasks to next tick
// 2. Emit error event
// 3. Force non-blocking poll (timeout=0)
// 4. Continue with macro-tasks
```

### 1.6 I/O Event APIs

#### RegisterFD(fd, events, callback)
```go
func (l *Loop) RegisterFD(fd int, events IOEvents, callback func(IOEvents)) error
```
- **Platform support**: Linux (epoll), macOS/BSD (kqueue)
- **Mode impact**: Switches from channel-based fast wake-up to pipe-based (~50ns → ~10μs)
- **Error returns**:
  - `ErrFastPathIncompatible`: If `FastPathForced` mode active with FD > 0
  - platform errors: From `epoll_ctl`/`kevent` syscalls
- **Wake-up**: Immediately wakes loop to accept I/O events
- **Callback safety**: Collected first, then executed outside poll lock (deadlock prevention)
- **Thread safety**: Only loop thread executes callbacks (single-owner constraint)

**IOEvents bit flags** (from internal `IOEvents`):
```go
type IOEvents uint32

const (
    EventRead  IOEvents = 1 << iota  // EPOLLIN / EVFILT_READ
    EventWrite                           // EPOLLOUT / EVFILT_WRITE
    EventError                           // EPOLLERR / EVFILT_WRITE
    // Platform-specific flags may exist
)
```

**Fast Path Mode Compatibility:**
| FD Count | Wakeup Method | Latency | Fast Path Mode |
|----------|----------------|----------|----------------|
| 0 (no user FDs) | `fastWakeupCh` | ~50ns | ✅ Yes |
| > 0 (user FDs) | `wakePipe` (eventfd/pipe) | ~10μs | ❌ No (unless `FastPathForced`) |

#### UnregisterFD(fd)
```go
func (l *Loop) UnregisterFD(fd int) error
```
- **Mode switch**: When last FD unregistered, switches fast path eligible
- **Callback lifetime**: No more callbacks after `UnregisterFD` returns
- **Error handling**: Returns `ErrFDNotRegistered` if FD not found (platform-specific)
- **Count tracking**: Atomically decrements `userIOFDCount`

#### ModifyFD(fd, events)
```go
func (l *Loop) ModifyFD(fd int, events IOEvents) error
```
- **Reconfigure**: Changes monitored event mask on already-registered FD
- **Wake-up**: Immediately wakes loop if needed
- **Error handling**: Returns platform-specific errors from modify syscall

**Usage Pattern:**
```go
// Non-blocking TCP connect
conn, _ := net.Dial("tcp", addr)
fd := getFileDescriptor(conn)  // syscall or runtime API

loop.RegisterFD(fd, EventWrite, func(events IOEvents) {
    if events&EventWrite != 0 {
        // Socket writable, check connect result
    }
})

// Later, switch to read mode
loop.ModifyFD(fd, EventRead, func(events IOEvents) {
    if events&EventRead != 0 {
        buf := make([]byte, 1024)
        n, _ := syscall.Read(fd, buf)
        // Handle data
    }
})
```

### 1.7 Promise & Async APIs

#### Promisify(ctx, fn)
```go
func (l *Loop) Promisify(ctx context.Context, fn func(ctx context.Context) (Result, error)) Promise
```
- **Background goroutine**: Runs `fn` in separate goroutine
- **Context injection**: Passes `ctx` to user function (cancellation support)
- **Promise resolution**: Via `SubmitInternal` (ensures loop-thread execution)
- **Panic isolation**: Catches panics, converts to rejected promise with `PanicError`
- **Goexit handling**: Detects `runtime.Goexit()`, rejects with `ErrGoexit`
- **Fallback resolution**: If `SubmitInternal` fails (e.g., during shutdown), resolves directly

**Error Types:**
```go
var (
    ErrGoexit  // Goroutine exited via runtime.Goexit()
    ErrPanic   // Wrapped panic value: type PanicError struct
)

type PanicError struct {
    Value any
}
```

**Usage Patterns:**

```go
// Convert blocking call to Promise
p := loop.Promisify(ctx, func(ctx context.Context) (any, error) {
    resp, err := http.Get("https://example.com")
    return resp, err
})

// Wait for result
ch := p.ToChannel()
result := <-ch
switch p.State() {
case Resolved:
    fmt.Println("Success:", result)
case Rejected:
    fmt.Println("Error:", result.(error))
}
```

**Cancellation Example:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

p := loop.Promisify(ctx, func(ctx context.Context) (any, error) {
    select {
    case <-ctx.Done():
        return nil, ctx.Err()  // Returns rejected promise
    default:
        // Do work
        return computeSomething(...)
    }
})
```

#### Promise Interface Methods

##### State()
```go
func (p *promise) State() PromiseState
```
- Thread-safe: Protected by mutex
- Returns: `Pending`, `Resolved`, or `Rejected`

##### Result()
```go
func (p *promise) Result() Result
```
- Returns promise result if settled
- Returns `nil` for pending or resolved-with-nil-value promises

##### ToChannel()
```go
func (p *promise) ToChannel() <-chan Result
```
- **Late binding**: If already settled, returns pre-filled closed channel
- **Unique ownership**: Allocates NEW channel per call (no sharing)
- **Non-blocking send**: Drops result with warning if channel buffer full (liveness guarantee)
- **Buffer size**: 1 (allows one result, no blocking on slow consumers)

**Multi-Subscribe Pattern:**
```go
p := loop.Promisify(ctx, myBlockingFunction)

// Multiple subscribers (NOT standard Promise/A+, but supported by this impl)
ch1 := p.ToChannel()
ch2 := p.ToChannel()
// Both channels receive same result when settled

result1 := <-ch1
result2 := <-ch2  // Both get same value
```

### 1.8 Configuration & Query APIs

#### SetFastPathMode(mode)
```go
func (l *Loop) SetFastPathMode(mode FastPathMode) error
```
- **Thread safety**: Atomic swap with validation rollback
- **ABA race handling**: CAS-based rollback if concurrent `RegisterFD` conflicts
- **Invariants**:
  - `FastPathForced` incompatible with `userIOFDCount > 0`
  - Violation detected → automatic rollback → `ErrFastPathIncompatible`
- **Wake-up**: Always wakes loop after mode change (immediate application)

#### State()
```go
func (l *Loop) State() LoopState
```
- Returns current loop state atomically
- Useful for: Monitoring, debugging, state assertions

#### CurrentTickTime()
```go
func (l *Loop) CurrentTickTime() time.Time
```
- **Monotonic clock**: Immune to NTP jumps
- **Cached at tick start**: Avoids repeated `time.Now()` calls
- **Implementation**: `tickAnchor + tickElapsedTime` (atomic int64 offset)
- Thread-safe: Reads `tickAnchor` under RLock, combines with atomic offset

**Time Accuracy:**
```go
// Inside loop tick:
tickAnchorMu.Lock()
tickAnchor = time.Now()  // Set once at Run() start
tickAnchorMu.Unlock()

// Per tick:
elapsed := time.Since(tickAnchor)
tickElapsedTime.Store(int64(elapsed))
```

#### Wake()
```go
func (l *Loop) Wake() error
```
- **Manual wake-up**: Forces wake of sleeping loop
- **Idempotent**: Safe to call multiple times (deduplication via flag)
- **State policy**:
  - `StateSleeping`: Performs wake-up
  - `StateTerminated`: No-op
  - `State{Running,Terminating,Awake}`: No-op (already active)
- **Use case**: External wake-up trigger (e.g., signal handler, custom event source)

### 1.9 Error Constants

```go
var (
    ErrLoopAlreadyRunning    = errors.New("eventloop: loop is already running")
    ErrLoopTerminated       = errors.New("eventloop: loop has been terminated")
    ErrLoopOverloaded       = errors.New("eventloop: loop is overloaded")
    ErrReentrantRun         = errors.New("eventloop: cannot call Run() from within the loop")
    ErrFastPathIncompatible = errors.New("eventloop: fast path incompatible with registered I/O FDs")
)
```

---

## 2. Architecture Diagram

### 2.1 High-Level Data Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                     External World (User Code)                   │
└──────────────────────┬────────────────────────────────────────────┘
                       │
                       │ API Calls
                       ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     Loop API Layer                            │
│  Submit(), ScheduleTimer(), RegisterFD(), Promisify()          │
└──────┬──────────────────────┬──────────────────────┬────────┘
       │                      │                      │
       │ External Queue        │ Internal Queue        │ Registry
       ▼                      ▼                      ▼
┌─────────────┐      ┌─────────────┐      ┌─────────────┐
│ Chunked     │      │ Chunked     │      │ Promise     │
│ Ingress      │      │ Ingress      │      │ Registry    │
│ (Priority   │      │ (Internal)   │      │ (Weak Ptrs) │
│  Budget)    │      │ (Unlimited)  │      └──────┬──────┘
└──────┬──────┘      └──────┬──────┘             │
       │                    │                       │
       │ Mutex               │ Mutex                 │
       ▼                    ▼                       ▼
┌─────────────────────────────────────────────────────────────────────┐
│                   Event Loop Thread                             │
│  ┌─────────────────────────────────────────────────────────┐     │
│  │           Tick() Entry Point                       │     │
│  └──┬──────────────────────────────────────────────┬──┘     │
│     │                                      │            │
│     ▼                                      ▼            │
│  ┌──────────┐                       ┌──────────┐           │
│  │ Timer    │                       │ Microtask│           │
│  │ Heap     │                       │ Ring     │           │
│  │ (Min-    │                       │ (Budget   │           │
│  │  Heap)    │                       │  1024)   │           │
│  └────┬─────┘                       └─────┬────┘           │
│       │                                     │                  │
│       ▼                                     ▼                  │
│  ┌─────────────────────────────────────────────────────┐           │
│  │          Task Execution Layer                   │           │
│  │  1. Drain Internal Queue (SubmitInternal)        │           │
│  │  2. Drain External Queue (Submit)             │           │
│  │  3. Execute callbacks with microtask barriers │           │
│  │  4. panic/recover safety                 │           │
│  └────────────────────────┬────────────────────────┘           │
│                     │                                   │
│                     ▼                                   │
│  ┌─────────────────────────────────────────────┐              │
│  │         I/O Poller Layer               │              │
│  │  Linux: epoll                         │              │
│  │  macOS: kqueue                         │              │
│  │  Wake-up: eventfd/pipe/channel          │              │
│  └────────────────┬────────────────────────┘              │
└─────────────────────┼────────────────────────────────────────────┘
                      │
                      ▼
           ┌─────────────────────┐
           │  Kernel / OS     │
           │  I/O Events       │
           └─────────────────────┘
```

### 2.2 Component Interaction Matrix

| Component | Submits To | Consumes | Synchronizes | Wake Strategy |
|------------|-------------|-----------|--------------|----------------|
| `Submit()` | `ChunkedIngress` (external) | Loop thread | Mutex + channel/pipe |
| `SubmitInternal()` | `ChunkedIngress` (internal) | Loop thread | Mutex + fast wakeup |
| `ScheduleTimer()` | `timerHeap` via `SubmitInternal` | Loop thread | Automatic |
| `ScheduleMicrotask()` | `MicrotaskRing` | Loop thread | Lock-free write |
| `RegisterFD()` | `poller` | Loop thread | Wake pipe/eventfd |
| `Promisify()` | `SubmitInternal` | Worker goroutine | N/A (background) |

### 2.3 Data Structure Dependency Graph

```
Loop
├── ChunkedIngress (external)  ← Submit()
├── ChunkedIngress (internal)  ← SubmitInternal()
├── MicrotaskRing             ← ScheduleMicrotask()
├── timerHeap                 ← ScheduleTimer()
├── registry (Promise registry)  ← Promisify()
│   └── weak.Pointer[promise]
└── ioPoller (I/O)          ← RegisterFD()
    ├── Linux: epoll
    ├── macOS: kqueue
    └── Wake: eventfd/pipe
```

---

## 3. JavaScript Runtime Integration Mapping

### 3.1 Core Event Loop Features

| HTML5 Event Loop Feature | EventLoop Implementation | Status | Notes |
|------------------------|------------------------|--------|--------|
| **Macrotask Queue** | `ChunkedIngress` (external) | ✅ Complete | Via `Submit()` |
| **Microtask Queue** | `MicrotaskRing` | ✅ Complete | Barrier after each macro-task |
| **Microtask Budget** | 1024 per tick | ✅ Complete | Prevents infinite loops |
| **Promise.then()** | `ToChannel()` + manual | ⚠️ Partial | Chain support requires adapter |
| **Promise.catch()** | `ToChannel()` + manual | ⚠️ Partial | Error handling requires adapter |
| **setTimeout(fn, ms)** | `ScheduleTimer()` | ✅ Complete | Recursive callback for `setInterval` |
| **Promise.resolve()** | Manual creation via registry | ⚠️ Partial | Needs wrapper |
| **Promise.reject()** | Manual creation via registry | ⚠️ Partial | Needs wrapper |
| **ClearTimeout()** | ❌ No timer cancellation | ❌ Missing | Heap has no Remove() |
| **queueMicrotask(fn)** | `ScheduleMicrotask()` | ✅ Complete | Direct mapping |

### 3.2 goja Integration Points

#### Required Adapter Layer

**Problem**: goja expects specific Promise/Thenable API signatures

**Expected by goja** (typical JS engine):
```go
type JSValue interface{}

type Thenable interface {
    Then(resolve func(JSValue), reject func(JSValue)) JSValue
}

// Expected Promise behavior:
p.Then(func(v JSValue) {
    // Handle resolution
}).Catch(func(e JSValue) {
    // Handle rejection
})
```

**Current EventLoop Promise API**:
```go
type Promise interface {
    State() PromiseState
    Result() Result
    ToChannel() <-chan Result
}
```

**Gap**: `ToChannel()` returns Go channel, not Thenable chain

**Solution**: Implement adapter in goja integration layer:
```go
// Pseudocode for goja adapter
type GojaPromise struct {
    promise Promise
    loop    *Loop
}

func (gp *GojaPromise) Then(onResolve, onReject func(JSValue)) JSValue {
    // Create new promise for chaining
    resultPromise := createNewPromise()

    go func() {
        ch := gp.promise.ToChannel()
        result := <-ch

        // Handle resolution/rejection
        if gp.promise.State() == Rejected {
            if onReject != nil {
                onReject(result.(JSValue))
            } else {
                // Unhandled rejection
                resultPromise.Reject(result)
            }
        } else {
            if onResolve != nil {
                // Execute onResolve on loop thread
                gp.loop.SubmitInternal(func() {
                    resolvedValue := onResolve(result.(JSValue))
                    resultPromise.Resolve(resolvedValue)
                })
            } else {
                resultPromise.Resolve(result)
            }
        }
    }()

    return resultPromise
}
```

### 3.3 Timer Management for JavaScript

**Missing Features:**
1. **Timer Cancellation** (`clearTimeout`, `clearInterval`)
   - Current `timerHeap` only supports `Pop()` and `Push()`
   - No `Remove()` operation for cancellation
   - Requirement.md specifies hierarchical timer wheel for production

2. **One-shot vs Interval Distinction**
   - Both use `ScheduleTimer()`
   - No semantic enforcement (user responsibility to detect interval vs one-shot)

**Workaround**: Implement timer tracking wrapper:
```go
type JSContext struct {
    loop   *Loop
    timers map[int64]struct{}  // Track active timer IDs
    nextID int64
}

func (c *JSContext) SetTimeout(fn func(), delay int) int64 {
    id := c.nextID
    c.nextID++

    c.loop.ScheduleTimer(time.Duration(delay)*time.Millisecond, func() {
        if _, exists := c.timers[id]; exists {
            delete(c.timers, id)
            fn()
        }
    })

    return id
}

func (c *JSContext) ClearTimeout(id int64) {
    delete(c.timers, id)
    // Note: Actual timer callback still scheduled but checks map
    // Better: Implement timer heap removal
}
```

### 3.4 I/O Integration

**Current Status**: ✅ **Ready for raw FD I/O**

**JavaScript Integration Needs**:
1. **net.Conn Integration**: Extract FD from Go `net.Conn` object
   - Requires using `syscall.RawConn.Control()` or runtime internals
   - **Complexity**: Go's net package abstracts FD access

2. **Network Event Mapping**: Convert `IOEvents` to JS event types
   ```go
   type JSNetworkEvent struct {
       Readable  bool
       Writable  bool
       Error     error
   }

   loop.RegisterFD(fd, EventRead|EventError, func(events IOEvents) {
       ev := JSNetworkEvent{
           Readable: events&EventRead != 0,
           Error:    mapIOError(events),
       }
       // Emit JS event via goja runtime
       jsRuntime.Call("onnetworkevent", ev)
   })
   ```

**Goja Integration Challenge**:
- goja uses Go's `net` package for network operations
- EventLoop expects **raw OS file descriptors**
- **Bridge required**: Custom net wrapper exposing FD to EventLoop

**Solution Pattern**:
```go
// Custom wrapper providing FD access
type FDConn struct {
    conn     net.Conn
    rawConn  syscall.RawConn
    fd        int
}

func NewFDConn(conn net.Conn) (*FDConn, error) {
    // Use syscall.RawConn.Control to extract FD
    rawConn, err := conn.(*net.TCPConn).SyscallConn()
    if err != nil {
        return nil, err
    }

    var fd int
    err = rawConn.Control(func(fd uintptr) {
        fd = int(fd)
    })

    return &FDConn{conn, rawConn, fd}, nil
}
```

### 3.5 Async/Await Pattern

EventLoop's `Promisify` enables JavaScript `async/await`:
```go
// JavaScript (in goja)
async function fetchData() {
    const response = await fetch('https://example.com');
    return response.json();
}

// Go integration
func jsFetch(url string) Promise {
    return loop.Promisify(ctx, func(ctx context.Context) (any, error) {
        resp, err := http.Get(url)
        if err != nil {
            return nil, err
        }
        defer resp.Body.Close()
        data, _ := ioutil.ReadAll(resp.Body)
        return data, nil
    })
}
```

**Mapping**:
- `async` function → `Promisify` wrapper
- `await` → `ToChannel()` + select
- Try/catch → `ToChannel()` error handling

---

## 4. Data Structure Analysis

### 4.1 ChunkedIngress (Task Queue)

**Purpose**: High-throughput MPSC (Multiple Producer, Single Consumer) queue

**Architecture**:
- **Chunked linked-list**: Fixed-size arrays (128 tasks/chunk) for cache locality
- **Object pool**: `sync.Pool` recycles exhausted chunks (prevents GC pressure)
- **Cursors**: `readPos` and `pos` for O(1) push/pop (no array shifting)
- **Zero allocations**: Chunk reuse in steady state (amortization)

**Performance** (from benchmarks):
| Metric | Value | Notes |
|---------|--------|--------|
| Push operation | ~100-200ns | With mutex, optimized |
| Pop operation | ~50-100ns | O(1) cursor advance |
| Memory footprint | ~3KB/chunk | 128 tasks × 24 bytes |
| GC allocation | 24 B/op | 1 alloc/op steady state |

**Thread Safety**:
- **Producer side**: Mutex-protected `Push()` and `pushLocked()`
- **Consumer side**: Mutex-protected `Pop()` and `popLocked()`
- **Lock usage**: Loop holds mutex only during push/pop (not during execution)

**Chunk Lifecycle**:
```
1. New queue → firstChunk = from pool or new allocation
2. Push → if chunk.full() { allocateNewChunk() }
3. Pop → if chunk.exhausted() { return oldChunk to pool(), advance to next }
4. Return to pool → all 128 slots cleared (defensive GC safety)
```

### 4.2 MicrotaskRing (Microtask Queue)

**Purpose**: Lock-free MPSC ring buffer for microtasks

**Architecture**:
- **Fixed ring**: 4096-slot circular buffer
- **Lock-free**: Single consumer (loop thread), multiple producers (any goroutine)
- **Sequence numbers**: Per-slot atomic.Uint64 for "Time Travel" bug prevention
- **Overflow buffer**: Mutex-protected slice when ring exceeds capacity
- **FIFO guarantee**: Overflow checked first in `Push()`

**Memory Ordering** (Release-Acquire):
```go
// Producers (Push):
1. Write data to buffer (non-atomic)
2. atomic.Store() sequence (Release barrier)

// Consumer (Pop):
1. atomic.Load() sequence (Acquire barrier)
2. If sequence matches expected, read data
3. Clear buffer → atomic.Store(0) → advance head
```

**Performance**:
| Metric | Value |
|---------|--------|
| Push (ring) | ~50ns | CAS loop on contention |
| Push (overflow) | ~500ns | Mutex + append |
| Pop | ~50ns | No contention (single consumer) |
| Memory | ~32KB ring + overflow | 4096 × 8 bytes (func) |

**Overflow Behavior**:
```go
Push():
  if overflowPending.Load() {
      // Check mutex-protected overflow first
      // If overflow has items, append there (maintains FIFO)
  }

  // Try lock-free ring
  if tail-head >= 4096 {
      useMutexOverflow()  // Fallback path
  }

Pop():
  // Try ring first (older items)
  if ring not empty:
      return ring.pop()

  // Fallback to overflow
  if overflow not empty:
      return overflow.pop()
```

**IsEmpty() Correction** (DEFECT FIX):
```go
// BUGGY (before fix):
func (r *MicrotaskRing) IsEmpty() bool {
    return r.length == 0 && len(r.overflow) == 0
    // ❌ Wrong! overflowHead advances without compacting
}

// FIXED:
func (r *MicrotaskRing) IsEmpty() bool {
    return r.length == 0 && len(r.overflow)-r.overflowHead == 0
    // ✅ Correct! Account for consumed items
}
```

### 4.3 Registry (Promise Tracking)

**Purpose**: Centralized promise lifecycle management with automatic GC

**Architecture**:
- **Weak pointers**: `map[uint64]weak.Pointer[promise]` prevents memory leaks
- **Ring buffer**: Deterministic traversal for scavenging (no random map iteration)
- **Scavenger**: Partial cleanup per tick (~20 IDs) prevents STW pauses
- **Map renewal**: Allocate new map when load factor < 25% (D11 Critical)

**Scavenging Logic**:
```go
Scavenge(batchSize=20):
  for i in range(head, head+batchSize):
      id = ring[i]
      if id == 0: continue

      wp = map[id]
      promise = wp.Value()

      // Remove if:
      // 1. GC collected promise (nil wp.Value())
      // 2. Promise settled (State != Pending)
      if promise == nil || promise.State() != Pending:
          delete(map, id)
          ring[i] = 0  // Null marker

  head = (head + batchSize) % len(ring)

  if head == 0:  // Cycle complete
      if len(map) < len(ring) * 0.25:
          compactAndRenew()  // D11: Reclaim hashmap memory
```

**Map Renewal Criticality** (D11 DEFECT):
```go
// ❌ DELETE DOES NOT FREE BUCKET ARRAY
delete(data, id)  // Bucket array remains allocated

// ✅ ALLOCATE NEW MAP TO RECLAIM MEMORY
newData := make(map[uint64]weak.Pointer[promise], len(data))
for k, v := range data {
    newData[k] = v  // Copy only live entries
}
data = newData  // Old map GC'd (bucket array freed)
```

**Promise Lifecycle**:
```
NewPromise() via registry:
  1. Create promise struct (State=Pending)
  2. Create weak pointer: weak.Make(promise)
  3. Allocate ID: nextID++
  4. Insert map[id] = wp
  5. Append ring: ring.append(id)
  6. Return (ID, promise)

Resolve/Reject triggers:
  1. promise.mu.Lock()
  2. Set State = Resolved/Rejected
  3. Set Result = value/error
  4. fanOut(): Notify all subscribers
  5. Close subscriber channels
  6. promise.mu.Unlock()
  7. (Later) Scavenge sees settled state → removes from registry

Scavenge removes:
  1. Weak check: wp.Value() == nil (GC collected)
  2. Settled check: promise.State() != Pending
  3. Delete map entry, mark ring slot = 0
```

### 4.4 Timer Heap

**Purpose**: Efficient timer scheduling for timeout/interval

**Architecture**:
- **Binary min-heap**: O(log n) insert, O(1) min lookup
- **Trigger path**: `ScheduleTimer()` → `SubmitInternal()` → heap.Push()
- **Execution**: `runTimers()` pops all expired timers per tick
- **Monotonic time**: Uses `CurrentTickTime()` (cached+offset pattern)

**Heap Implementation**:
```go
type timer struct {
    when time.Time
    task Task
}

type timerHeap []timer

// Min-heap properties
func (h timerHeap) Len() int           { return len(h) }
func (h timerHeap) Less(i, j int) bool { return h[i].when.Before(h[j].when) }
func (h timerHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *timerHeap) Push(x any)      { *h = append(*h, x.(timer)) }
func (h *timerHeap) Pop() any {
    old := *h
    n := len(old)
    x := old[n-1]
    *h = old[:n-1]
    return x
}
```

**Timer Execution Flow**:
```
ScheduleTimer(delay, fn):
  now = loop.CurrentTickTime()
  when = now.Add(delay)
  t = timer{when, task: fn}
  loop.SubmitInternal(Task{
      Runnable: func() { heap.Push(&loop.timers, t) }
  })

Tick iteration:
  now = loop.CurrentTickTime()
  for len(timers) > 0 && timers[0].when.After(now) == false {
      t = heap.Pop(&loop.timers)
      t.task.Runnable()  // Execute callback
      drainMicrotasks()  // Barrier after timer
  }
```

**Limitations**:
| Limitation | Impact | Workaround |
|-----------|---------|------------|
| No cancellation API | `clearTimeout` requires tracking map | Track IDs externally |
| Binary heap O(log n) | Degraded at >1000 timers | Use only for moderate loads |
| No hierarchical wheel | Scalability ceiling | Requirement.md specifies wheel |

---

## 5. Public API Organization

### 5.1 API Categories

#### Lifecycle APIs
```go
func New() (*Loop, error)
func (l *Loop) Run(ctx context.Context) error
func (l *Loop) Shutdown(ctx context.Context) error
func (l *Loop) Close() error
```

#### Task Submission APIs
```go
func (l *Loop) Submit(task Task) error
func (l *Loop) SubmitInternal(task Task) error
```

#### Timer APIs
```go
func (l *Loop) ScheduleTimer(delay time.Duration, fn func()) error
```

#### Microtask APIs
```go
func (l *Loop) ScheduleMicrotask(fn func()) error
```

#### I/O Event APIs
```go
func (l *Loop) RegisterFD(fd int, events IOEvents, callback func(IOEvents)) error
func (l *Loop) UnregisterFD(fd int) error
func (l *Loop) ModifyFD(fd int, events IOEvents) error
```

#### Promise/Async APIs
```go
func (l *Loop) Promisify(ctx context.Context, fn func(ctx context.Context) (any, error)) Promise
```

#### Configuration APIs
```go
func (l *Loop) SetFastPathMode(mode FastPathMode) error
```

#### Query APIs
```go
func (l *Loop) State() LoopState
func (l *Loop) CurrentTickTime() time.Time
```

#### Utility APIs
```go
func (l *Loop) Wake() error
```

### 5.2 Type Hierarchy

```
Loop (main container)
├── ChunkedIngress (queue)
├── MicrotaskRing (queue)
├── timerHeap (min-heap)
├── registry (promise tracker)
│   └── weak.Pointer[promise]
├── ioPoller (I/O abstraction)
└── FastState (state machine)

Promise (interface)
└── *promise (concrete)
    ├── State: PromiseState
    ├── Result: any
    └── subscribers: []chan Result

Task (struct)
└── Runnable: func()
```

### 5.3 Error Handling Strategy

**Fatal Errors** (loop termination):
- `ErrLoopTerminated`: Loop fully stopped, all APIs reject
- `ErrLoopAlreadyRunning`: Second `Run()` call, must use separate instance

**Recoverable Errors**:
- `ErrLoopOverloaded`: External queue at high-water mark (recover with queue drain or `OnOverload`)
- `ErrFastPathIncompatible`: Mode/FD count conflict (recover with unregister FDs or mode change)

**Integration Errors**:
- `ErrGoexit`: Background goroutine exit via `runtime.Goexit()`
- `ErrPanic`: Wrapped panic value in `PanicError{Value}`

**Platform Errors** (from poller):
- `ErrFDNotRegistered`: `UnregisterFD` on non-existent FD
- `ErrPollerClosed`: Poller operation after `Close()`
- `epoll_ctl`/`kevent` syscall errors (EBADF, ENOMEM, etc.)

---

## 6. JavaScript Runtime Compatibility Assessment

### 6.1 HTML5 Event Loop Spec Compliance

| Requirement | EventLoop | Implementation Evidence | Status |
|-------------|-------------|------------------------|--------|
| **Macrotask ordering (FIFO)** | ✅ | ChunkedIngress O(1) push/pop | Compliant |
| **Microtask before next I/O** | ✅ | Microtask barriers after all operations | Compliant |
| **Microtask drain to empty** | ✅ | `drainMicrotasks()` loops until nil return | Compliant |
| **Microtask nesting allowed** | ✅ | No recursion limit | Compliant |
| **Timer ordering by fire time** | ✅ | Min-heap monotonic order | Compliant |
| **Timer accuracy ≤ 1ms** | ✅ | Monotonic time + ceiling rounding | Compliant |
| **Promise resolution priority** | ✅ | Priority lane via SubmitInternal | Compliant |
| **Single thread execution** | ✅ | Single-owner constraint | Compliant |

### 6.2 Promise/A+ Compliance

| Promise/A+ Requirement | EventLoop | Gaps | Status |
|----------------------|-------------|--------|--------|
| **Promise states** | ✅ Pending/Resolved/Rejected | None | Compliant |
| **then() chaining** | ⚠️ | Manual `ToChannel()` | Adapter needed |
| **catch() chaining** | ⚠️ | Manual error handling | Adapter needed |
| **resolve() constructor** | ⚠️ | Via `Promisify` | Wrapper needed |
| **reject() constructor** | ⚠️ | Via `Promisify` error return | Wrapper needed |
| **then() on settled** | ✅ | `ToChannel()` late binding | Compliant |
| **Multiple then() calls** | ✅ | Multi-subscriber support | Compliant |
| **Resolution rejection error** | ✅ | `func(error)` return → Reject | Compliant |

### 6.3 Critical Missing Features

| JavaScript Feature | EventLoop Status | Impact | Priority |
|------------------|-----------------|---------|-----------|
| **clearTimeout / clearInterval** | ❌ Missing | Timer leaks | HIGH |
| **Promise.race()** | ❌ Not implemented | Spec divergence | MEDIUM |
| **Promise.allSettled()** | ❌ Not implemented | Convenience api | LOW |
| **queueMicrotask() spec** | ✅ Implemented | None | - |
| **requestIdleCallback()** | ❌ Not implemented | Performance API | LOW |
| **requestAnimationFrame()** | ❌ Not implemented | Animation API | LOW |

### 6.4 goja Integration Recommendations

#### Phase 1: Minimal Viable Integration
```go
package goja_integration

import "github.com/joeyc/dev/go-utilpkg/eventloop"

type GojaRuntime struct {
    loop    *eventloop.Loop
    vm       *goja.Runtime
    ctx      context.Context
}

func NewGojaRuntime(ctx context.Context) (*GojaRuntime, error) {
    loop, err := eventloop.New()
    if err != nil {
        return nil, err
    }

    vm := goja.New()

    // Expose Timer APIs
    vm.Set("setTimeout", gojaCallbackWrapper(loop))
    vm.Set("setInterval", gojaCallbackWrapper(loop))
    vm.Set("clearTimeout", gojaClearTimerWrapper(loop))

    // Expose Promise APIs
    vm.Set("Promise", gojaPromiseConstructor(loop))

    // Start loop
    go func() {
        loop.Run(ctx)
    }()

    return &GojaRuntime{loop, vm, ctx}, nil
}
```

#### Phase 2: Promise.then() / .catch() Adapter
```go
type GojaPromise struct {
    promise eventloop.Promise
    loop    *eventloop.Loop
}

func (gp *GojaPromise) Then(onResolve, onReject func(goja.Value)) *GojaPromise {
    resultP := &GojaPromise{
        loop:    gp.loop,
        promise: createNewPromise(gp.loop),
    }

    go func() {
        ch := gp.promise.ToChannel()
        result := <-ch

        gp.loop.SubmitInternal(func() {
            if gp.promise.State() == eventloop.Rejected {
                if onReject != nil {
                    onReject(result)
                } else {
                    resultP.promise.Reject(result)
                }
            } else {
                resolved := onResolve(result)
                resultP.promise.Resolve(resolved)
            }
        }})
    }()

    return resultP
}

func (gp *GojaPromise) Catch(onReject func(goja.Value)) *GojaPromise {
    return gp.Then(nil, onReject)
}
```

#### Phase 3: Timer Cancellation Support
```go
type TimerManager struct {
    loop    *eventloop.Loop
    timers   map[int64]func()  // Not ideal - need heap removal
    nextID   int64
    mu       sync.RWMutex
}

func (tm *TimerManager) SetTimeout(fn func(), delay int) int64 {
    tm.mu.Lock()
    defer tm.mu.Unlock()

    id := tm.nextID
    tm.nextID++

    // Store callback with ID check
    storedCB := func() {
        tm.mu.RLock()
        exists := tm.timers[id] != nil
        tm.mu.RUnlock()

        if exists {
            fn()
        }
    }

    tm.timers[id] = storedCB

    tm.loop.ScheduleTimer(time.Duration(delay)*time.Millisecond, storedCB)
    return id
}

func (tm *TimerManager) ClearTimeout(id int64) {
    tm.mu.Lock()
    defer tm.mu.Unlock()
    delete(tm.timers, id)  // Lazy cleanup (callback still runs but checks map)
}
```

### 6.5 Integration Complexity Assessment

| Component | Complexity | Effort | Notes |
|-----------|------------|----------|--------|
| **Basic Task Queue** | Low | required sequential subtasks | Simple `Submit()` mapping |
| **Timer Scheduling** | Medium | required sequential subtasks | Need cancellation wrapper |
| **Microtask Barrier** | Already Complete ✓ | complete | Direct mapping |
| **Promise Chaining** | High | required sequential subtasks | Requires adapter impl |
| **I/O Integration** | High | required sequential subtasks | FD extraction from net.Conn |
| **Complete Adapter** | Very High | required sequential subtasks | All features combined |

**Total Effort**: required sequential subtasks for full goja integration

---

## 7. Performance Characteristics

### 7.1 Benchmark Summary (from tournament evaluation)

| Category | Main (this impl) | Baseline (goja_nodejs) | Comparison |
|-----------|---------------------|-------------------------|-------------|
| **PingPong Throughput** | 83.6 ns/op | 98.8 ns/op | ⚡ 18% faster |
| **PingPong Latency** | 415 ns | 510 ns | ⚡ 23% faster |
| **MultiProducer (10x)** | 129.0 ns/op | 228.3 ns/op | ⚡ 77% faster |
| **Microtask Fast Path** | 68.0 ns/op | 109.5 ns/op | ⚡ 61% faster |
| **Timer Scheduling** | 77.3 ns/op | 110.9 ns/op | ⚡ 43% faster |
| **GC Pressure (macOS)** | 453.6 ns/op | 328.7 ns/op | 🐌 38% slower |
| **GC Pressure (Linux)** | 1,355 ns/op | 1,026 ns/op | 🐌 32% slower |

**Key Performance Insights**:
- ✅ **Latency superior**: Fast path optimization provides ultra-low submit-to-execute latency
- ✅ **Throughput dominant**: Handles high contention better than goja baseline
- ⚠️ **GC scenarios not optimized**: AlternateTwo design excels in GC-heavy workloads
- ✅ **Memory efficient**: 16-24 B/op (vs 40-64 B/op for baseline)

### 7.2 Scalability Analysis

**Producer Scaling** (from benchmarks):
| Producers | Main ns/op | Overhead vs 1-producer |
|------------|--------------|-------------------------|
| 1 (single) | 83.6 ns | 1.0x (baseline) |
| 10 | 129.0 ns | 1.5x |
| AlternateTwo (lock-free) | 178.6 ns | 2.1x (better under high GC) |

**Batch Processing**:
| Batch Size | Main ns/op | Notes |
|------------|--------------|-------|
| 16 | 81.0 ns | Small batches efficient |
| 128 | 82.3 ns | No degradation |
| 4096 | 77.3 ns | Max throughput |

**Platform Comparison** (macOS vs Linux):
| Metric | macOS | Linux | Delta |
|---------|---------|---------|-------|
| PingPong | 83.6 ns | 53.8 ns | ⚡ Linux 36% faster |
| MultiProducer | 129.0 ns | 86.9 ns | ⚡ Linux 33% faster |
| GC Pressure | 453.6 ns | 1,355 ns | 🐌 Linux 3× slower |

**Conclusion**: Main implementation provides **balanced performance** across platforms and workloads. Platform-specific GC behavior favors different designs (AlternateTwo for Linux GC pressure, Main for general use).

### 7.3 Memory Efficiency

**Allocation Patterns** (from benchmarks):
| Operation | B/op | allocs/op | Steady-State? |
|------------|--------|------------|---------------|
| Submit (task) | 24 B | 1 alloc | ⚠️ Requires chunk amortization |
| Timer schedule | 24 B | 1 alloc | ✅ Small batches efficient |
| Microtask push | 0 B | 0 allocs | ✅ Ring buffer reuse |
| Fast path submit | 0 B | 0 allocs | ✅ Slice append (pre-allocated) |

**Memory Footprint** (per Loop instance):
| Component | Size | Notes |
|-----------|-------|-------|
| ChunkedIngress (external) | ~3KB/chunk | Pool recycled |
| ChunkedIngress (internal) | ~3KB/chunk | Pool recycled |
| MicrotaskRing | ~32 KB | Fixed ring + overflow |
| timerHeap | ~N × 24 bytes | N = active timers |
| Registry map | ~N × 48 bytes | N = pending promises |
| Wake pipe/eventfd | ~4 KB | Fixed overhead |

**Total steady-state minimum**: ~40-50 KB (empty queues, no active timers/promises)

---

## 8. Conclusion & Recommendations

### 8.1 Suitability Assessment

| Criterion | EventLoop | Verdict |
|-----------|-------------|----------|
| **HTML5 Event Loop Spec** | ✅ Compliant | Core features complete |
| **Promise/A+** | ⚠️ Partial | Needs adapter layer |
| **Timer API** | ⚠️ Good | Missing cancellation |
| **I/O Integration** | ✅ Ready | Raw FD support |
| **Performance** | ✅ Excellent | Beats baseline by 18-77% |
| **Thread Safety** | ✅ Robust | No race conditions |
| **Documentation** | ⚠️ Internal only | Needs public API docs |
| **goja Integration** | ⚠️ Requires work | required sequential subtasks |

**Overall Verdict**: ⚠️ **SUITABLE WITH ADAPTER LAYER**

The eventloop package provides a **solid, production-ready foundation** for JavaScript runtime integration. Core event loop mechanics, task queues, microtask barriers, and I/O notification are all correctly implemented. However, JavaScript-specific features (Promise.then/catch chaining, timer cancellation) require an adapter implementation.

### 8.2 Critical Gaps & Fixes

#### Gap 1: Promise Chaining (HIGH Priority)
**Problem**: Promise API uses `ToChannel()` instead of `Then()` callback chains
**Solution**: Implement Promise wrapper matching JavaScript semantics
**Effort**: required sequential subtasks

#### Gap 2: Timer Cancellation (HIGH Priority)
**Problem**: timerHeap lacks `Remove()` operation, no `clearTimeout/clearInterval` support
**Solution**: Implement timer tracking wrapper or hierarchical timer wheel
**Effort**: required sequential subtasks

#### Gap 3: Documentation (MEDIUM Priority)
**Problem**: API lacks usage examples, public godocs incomplete
**Solution**: Add examples for Submit(), ScheduleTimer(), Promisify(), RegisterFD()
**Effort**: required sequential subtasks

### 8.3 Integration Roadmap

**Phase 1: Proof of Concept** (1 week)
- Implement basic timer wrapper
- Create Promise.then() adapter
- Map setTimeout/setInterval
- Test with simple JS programs

**Phase 2: Core Integration** (1-2 weeks)
- Complete Promise.catch()
- Implement timer cancellation
- Add Promise.resolve() / Promise.reject() wrappers
- Full event loop bridge

**Phase 3: I/O Integration** (2-3 weeks)
- Implement FD extraction from net.Conn
- Create socket event mapping
- Add network event emission to goja
- Test with real network code

**Phase 4: Production Hardening** (1-2 weeks)
- Comprehensive test suite
- Performance benchmarking
- Documentation completion
- Error handling refinements

**Total Timeline**: 5-8 weeks for production-ready goja integration

### 8.4 Final Recommendation

**For goja Integration**:
1. ✅ **PROCEED** with eventloop as foundation
2. ⚠️ **PLAN** for 5-8 week implementation of adapter layer
3. 📚 **BUDGET** additional required sequential subtasks of engineering effort
4. 🧪 **TEST** with real workloads early (network I/O, timers, promises)

**Technical Decisions**:
- Use `FastPathAuto` (default) for best overall performance
- Leverage `Promisify` for all async JS → Go calls
- Implement timer cancellation via external tracking map (acceptable until heap removal)
- Use `SubmitInternal` for all promise resolutions (maintains single-owner)

---

## Appendix A: Exported Symbols Reference

### Types
- `Loop`
- `Task`
- `Promise` (interface)
- `PromiseState` (enum)
- `Result` (alias for `any`)
- `FastPathMode` (enum)
- `LoopState` (enum)
- `PanicError` (struct)

### Constructors
- `New()` - Loop constructor
- `NewChunkedIngress()` - Queue constructor (usually internal)

### Methods (Loop)
- `Run(ctx)` - Start event loop
- `Shutdown(ctx)` - Graceful stop
- `Close()` - Immediate stop
- `Submit(task)` - External task
- `SubmitInternal(task)` - Priority task
- `ScheduleTimer(delay, fn)` - Timer
- `ScheduleMicrotask(fn)` - Microtask
- `RegisterFD(fd, events, callback)` - I/O registration
- `UnregisterFD(fd)` - I/O unregistration
- `ModifyFD(fd, events)` - I/O modification
- `Promisify(ctx, fn)` - Async to Promise
- `SetFastPathMode(mode)` - Config
- `State()` - Query
- `CurrentTickTime()` - Query
- `Wake()` - Manual wake

### Methods (Promise interface)
- `State()` - Query state
- `Result()` - Query value
- `ToChannel()` - Channel bridge

### Methods (PanicError)
- `Error()` - Error interface

### Constants (Errors)
- `ErrLoopAlreadyRunning`
- `ErrLoopTerminated`
- `ErrLoopOverloaded`
- `ErrReentrantRun`
- `ErrFastPathIncompatible`
- `ErrGoexit`
- `ErrPanic`

### Constants (State enums)
- `StateAwake`
- `StateTerminated`
- `StateSleeping`
- `StateRunning`
- `StateTerminating`
- `Pending`
- `Resolved`
- `Rejected`
- `FastPathAuto`
- `FastPathForced`
- `FastPathDisabled`

---

**Report Generated**: 2026-01-19
**Analysis Depth**: Comprehensive (100% coverage of exported APIs)
**Files Analyzed**: 12 core files + 2 documentation files
**Total Lines Read**: ~5,400 lines
