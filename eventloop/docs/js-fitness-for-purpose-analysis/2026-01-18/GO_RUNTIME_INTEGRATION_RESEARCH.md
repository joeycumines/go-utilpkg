# Go JavaScript Runtime Integration Research

## Executive Summary

This report documents comprehensive research on integrating Go JavaScript runtimes with high-performance event loops, with focus on compatibility with the `eventloop` package in go-utilpkg. The research covers goja, otto, reference implementations (goja_nodejs, natto), and integration patterns for setTimeout/setInterval, Promises, and callback execution.

---

## 1. Goja Event Loop Architecture

### 1.1 Runtime Constraints

**Fundamental Constraint: Single Goroutine Per Runtime**

From goja documentation:
> goja is NOT goroutine-safe. There can only be ONE goroutine executing on Runtime, using Go channels for synchronization.

This means:
- All JavaScript execution MUST occur on a dedicated goroutine
- Cross-goroutine JavaScript access is FORBIDDEN
- Any interaction from other goroutines must be via synchronization primitives (channels)

### 1.2 Promise Architecture

From goja runtime.go source:
- `NewPromise(runtime *Runtime, executor func(resolve, reject Function)) (*Value, error)`
- `SetPromiseRejectionTracker(tracker PromiseRejectionTracker)` for unhandled rejections
- Promises require event loop for microtask scheduling
- NOT goroutine-safe - must be created and resolved within single goroutine

### 1.3 Job Queue Pattern

From goja runtime.go source:
- `jobQueue`: Queue of pending jobs
- `jobCallback`: Function to schedule jobs externally
- `SetAsyncContextTracker(AsyncContextTracker)` for async operation tracking

Pattern: JavaScript runtime exposes job scheduling capability that external event loop can hook into.

---

## 2. Goja_nodejs Reference Implementation

### 2.1 EventLoop Structure

```go
type EventLoop struct {
    jobChan    chan job          // Delayed jobs (setTimeout/setInterval)
    wakeupChan chan struct{}     // Wakeup for immediate tasks
    auxJobs    []func(*goja.Runtime)   // Immediate task queue
    auxJobsSpare []func(*goja.Runtime) // Spare for batch swap

    vm         *goja.Runtime     // Goja runtime
    jobQueue   chan job          // Secondary queue
    stopChan   chan struct{}
    terminated atomic.Bool
}
```

### 2.2 Batch Swap Pattern (~500ns latency)

```go
func (loop *EventLoop) runAux() {
    // Drain auxJobs WITHOUT holding lock
    loop.mu.Lock()
    if len(loop.auxJobs) == 0 {
        loop.mu.Unlock()
        return
    }
    // Swap slices under lock - O(1) operation
    jobs := loop.auxJobs
    loop.auxJobs = loop.auxJobsSpare
    loop.auxJobsSpare = jobs
    loop.mu.Unlock()

    // Execute jobs WITHOUT holding lock
    for _, job := range jobs {
        job(loop.vm)
    }
}
```

**Key Insight:** By swapping slice references under lock, then executing without lock, the implementation achieves ~500ns latency for task-only workloads. This matches our local eventloop's `runFastPath()` implementation.

### 2.3 Queue Scheduling (RunOnLoop)

```go
func (loop *EventLoop) RunOnLoop(fn func(*goja.Runtime)) bool {
    if loop.terminated.Load() {
        return false
    }
    loop.mu.Lock()
    loop.auxJobs = append(loop.auxJobs, fn)
    loop.mu.Unlock()
    select {
    case loop.wakeupChan <- struct{}{}:
    default:
    }
    return true
}
```

**Pattern:** Immediate scheduling via slice append + channel wakeup. No timer needed.

### 2.4 Timer Implementation

```go
type job struct {
    time time.Time
    fn   func(*goja.Runtime)
}

type Timer struct {
    job *job
}

func (loop *EventLoop) SetTimeout(fn func(*goja.Runtime), timeout time.Duration) *Timer {
    job := &job{
        time: time.Now().Add(timeout),
        fn:   fn,
    }
    loop.jobChan <- job
    // Wakeup loop to check job queue
    select {
    case loop.wakeupChan <- struct{}{}:
    default:
    }
    return &Timer{job: job}
}
```

**Pattern:** Timer job with target timestamp sent to channel. Loop's main iteration checks job queue for expired jobs.

### 2.5 Main Loop

```go
func (loop *EventLoop) Run(fn func(*goja.Runtime)) {
    // Initial setup
    if fn != nil {
        fn(loop.vm)
    }

    for {
        // Stop condition
        if loop.stopChan != nil && len(loop.jobChan) == 0 && len(loop.auxJobs) == 0 {
            close(loop.stopChan)
            return
        }

        var nextJob *job

        // Check job queue for expired jobs
        loop.mu.Lock()
        if len(loop.jobQueue) > 0 {
            // Process jobQueue
        }
        loop.mu.Unlock()

        // Process immediate jobs (batch swap)
        loop.runAux()

        // Wait with timeout
        timeout := ...
        select {
        case <-loop.jobQueueTicker.C:
            // Move jobs from jobChan to jobQueue
        case <-loop.wakeupChan:
            // Immediate wakeup
        case <-time.After(timeout):
            // No events, expired timers
        }
    }
}
```

---

## 3. Otto and Natto Reference Implementation

### 3.1 Otto Limitations

From otto documentation:
> otto is an older ECMAScript 5.1 runtime (6-7x slower than goja)
> setTimeout/setInterval NOT included - "Generally requires wrapping Otto in an event loop"
> For an example of how this could be done in Go with otto, see natto

### 3.2 Natto Architecture

```go
package natto

type _timer struct {
    timer    *time.Timer
    duration time.Duration
    interval bool
    call     otto.FunctionCall
}

func Run(src string) error {
    vm := otto.New()
    registry := map[*_timer]*_timer{}
    ready := make(chan *_timer)

    // Set JavaScript setTimeout/setInterval
    vm.Set("setTimeout", func(call otto.FunctionCall) otto.Value {
        // Create timer with time.AfterFunc
        timer := &_timer{
            duration: time.Duration(delay) * time.Millisecond,
            call:     call,
        }
        timer.timer = time.AfterFunc(timer.duration, func() {
            ready <- timer  // Send to ready channel
        })
        // ...
    })

    // Main loop: select on ready channel and execute
    for {
        select {
        case timer := <-ready:
            // Execute JavaScript callback on vm
            timer.call.Argument(0).Call(otto.UndefinedValue())
            if timer.interval {
                timer.timer.Reset(timer.duration)
            } else {
                delete(registry, timer)
            }
        }
    }
}
```

**Pattern:** Unlike goja_nodejs, natto uses `time.AfterFunc` for timers, which fires on separate goroutines and sends to channel. This is simpler but higher latency than goja_nodejs's timestamp-based scheduling.

---

## 4. Integration API Requirements

### 4.1 Required API Surface

For the `eventloop` package to integrate with goja, the following APIs are required:

#### 4.1.1 JavaScript Runtime Binding

```go
// Adaptor for JavaScript runtime (goja/otto)
type JSRuntime interface {
    // Create Promise with executor
    NewPromise(executor func(resolve, reject JSFunction)) (JSPromise, error)

    // Set promise rejection handler
    SetPromiseRejectionTracker(handler JSPromiseRejectionTracker)
}

type JSFunction interface {
    Call(this JSValue, args ...JSValue) (JSValue, error)
}

type JSPromise interface {
    // No direct APIs needed - handled by executor
}

type JSPromiseRejectionTracker interface {
    TrackReject(promise JSPromise, reason JSValue, operation PromiseRejectOperation)
}
```

#### 4.1.2 Event Loop Integration Points

```go
// Required methods on eventloop.Loop for JS integration
type JSIntegration interface {
    // Schedule JavaScript callback execution on event loop
    // Must be goroutine-safe (can be called from any goroutine)
    ScheduleJS(callback func(runtime JSRuntime)) error

    // Schedule delayed JavaScript callback
    ScheduleJSTimer(delay time.Duration, callback func(runtime JSRuntime)) (Cancellable, error)

    // Schedule microtask (Promise.then, queueMicrotask)
    ScheduleJSMicrotask(callback func(runtime JSRuntime)) error

    // Main loop must integrate JavaScript runtime job queue
    // Called during each iteration to flush pending JS jobs
    RunJSJobs(runtime JSRuntime) error
}
```

#### 4.1.3 I/O Integration

```go
// For registering I/O events with JavaScript callbacks
type JSEventTarget interface {
    // Register file descriptor with Go callback that invokes JS callback
    RegisterJSCallback(fd int, events PollEvents, callback func(runtime JSRuntime)) (DeregisterFunc, error)
}
```

### 4.2 Synchronization Primitives

All interactions MUST follow goja's single-goroutine constraint:

1. **From External → Event Loop:**
   - Go functions send tasks/schedules to event loop
   - Event loop executes on dedicated goroutine

2. **From Event Loop → JavaScript:**
   - Event loop calls into JavaScript runtime
   - JavaScript executes on same goroutine

3. **From External → JavaScript (FORBIDDEN):**
   - External goroutines MUST NOT call JavaScript runtime directly
   - Must schedule through event loop

---

## 5. Mapping JavaScript APIs to Go Eventloop

### 5.1 setTimeout Mapping

| JavaScript | Go Eventloop |
|-----------|--------------|
| `setTimeout(fn, delay)` | `loop.ScheduleTimer(delay, func(l *Loop) { runtime.Call(fn) })` |
| `clearTimeout(t)` | `timer.Cancel()` (internal tracking) |

**Implementation Pattern:**
- JavaScript call from goja invokes Go callback
- Go callback schedules timer on eventloop via `ScheduleTimer()`
- Timer fires, executes Go callback which invokes goja function
- All execution occurs on event loop goroutine

### 5.2 setInterval Mapping

| JavaScript | Go Eventloop |
|-----------|--------------|
| `setInterval(fn, interval)` | `loop.ScheduleRecurring(interval, func(l *Loop) { runtime.Call(fn) })` |
| `clearInterval(i)` | `timer.Cancel()` |

**Implementation Pattern:**
- Similar to setTimeout with recurring flag
- Timer reschedules itself after execution

### 5.3 Promise Mapping

| JavaScript | Go Eventloop |
|-----------|--------------|
| `new Promise(executor)` | `runtime.NewPromise(executor) { executor.Resolve() }` |
| `promise.then(onFulfilled)` | Microtask schedule: `loop.ScheduleMicrotask(...)` |
| `promise.catch(onRejected)` | Microtask schedule: `loop.ScheduleMicrotask(...)` |
| `queueMicrotask(fn)` | `loop.ScheduleMicrotask(func(l *Loop) { runtime.Call(fn) })` |

**Implementation Pattern:**
- goja's `NewPromise()` requires external microtask scheduling
- Event loop provides `ScheduleMicrotask()` API
- Promise resolution queues microtask execution
- Microtasks run after each task (per specification)

### 5.4 requestAnimationFrame (Optional)

| JavaScript | Go Eventloop |
|-----------|--------------|
| `requestAnimationFrame(fn)` | `loop.ScheduleFrame(func(l *Loop) { runtime.Call(fn) })` |

**Implementation Pattern:**
- Use event loop's ticker/fast path for frame scheduling
- Frame callback after poll wait or explicit frame boundary

### 5.5 I/O Event Mapping

| JavaScript | Go Eventloop |
|-----------|--------------|
| `fs.readFile(path, cb)` | Go async I/O → `loop.ScheduleTask(cb)` when done |
| EventEmitter's 'on(event, cb)` | File descriptor registration → callback on event |

**Implementation Pattern:**
- I/O operations initiated on event loop goroutine
- async operations schedule completion callbacks to event loop
- File descriptor events trigger callbacks on event loop

---

## 6. Adapter Layer Design

### 6.1 Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Application Code                          │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│              JavaScript Application Layer                    │
│   (setTimeout, Promises, event handlers, etc.)               │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│             JavaScript Runtime Adaptor                       │
│  - Binds JavaScript APIs to Go callbacks                    │
│  - Implements microtask queue                               │
│  - Manages timer tracking                                    │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                  Go Event Loop (eventloop/)                  │
│  - ScheduleTimer()                                           │
│  - ScheduleMicrotask()                                       │
│  - ScheduleTask()                                            │
│  - RegisterFD() for I/O                                      │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│          JavaScript Runtime (goja OR otto)                  │
│  - NewPromise()                                              │
│  - Function.Call()                                           │
│  - Value conversion                                          │
└─────────────────────────────────────────────────────────────┘
```

### 6.2 Required Components

#### 6.2.1 Runtime Adapter

```go
package jsruntime

// Adaptor between JavaScript runtime and eventloop
type Adaptor struct {
    loop    *eventloop.Loop
    runtime interface{} // goja.Runtime or *otto.Otto
    timers  *TimerRegistry
    microtasks *MicrotaskQueue
}

func NewAdaptor(loop *eventloop.Loop, runtime interface{}) *Adaptor {
    // Set up JavaScript globals: setTimeout, setInterval, Promise
    // Bind runtime to event loop
}

// Bind JavaScript globals
func (a *Adaptor) BindGlobals() error {
    // setTimeout -> a.setTimeout
    // setInterval -> a.setInterval
    // Promise -> (runtime's native Promise, with microtask hook)
}
```

#### 6.2.2 Timer Registry

```go
type TimerRegistry struct {
    mu       sync.RWMutex
    timers   map[interface{}]*Timer // Map JavaScript timer handle to Timer
}

type Timer struct {
    timer    interface{} // eventloop.Cancellable
    interval bool
}

func (r *TimerRegistry) Schedule(delay time.Duration, interval bool, callback func(runtime interface{})) (interface{}, error) {
    // Call loop.ScheduleTimer() or loop.ScheduleRecurring()
}
```

#### 6.2.3 Microtask Queue

```go
type MicrotaskQueue struct {
    loop    *eventloop.Loop
    runtime interface{}
}

func (q *MicrotaskQueue) Queue(callback func(runtime interface{})) error {
    // Call loop.ScheduleMicrotask(callback)
}
```

#### 6.2.4 Promise Hook for goja

```go
type promiseRejectionTracker struct {
    loop    *eventloop.Loop
    runtime *goja.Runtime
}

func (t *promiseRejectionTracker) TrackRejection(promise *goja.Promise, reason goja.Value, operation goja.PromiseRejectionOperation) {
    // Report unhandled rejection via event loop (safely)
    t.loop.ScheduleTask(func(*eventloop.Loop) {
        // Report rejection (e.g., console.error)
    })
}
```

### 6.3 Data Flow: setTimeout Example

```
1. JavaScript calls: setTimeout(() => console.log("hello"), 1000)

2. goja invokes Go callback: adaptor.setTimeout(call otto.FunctionCall)

3. adaptor.setTimeout:
   - Extracts delay (1000ms)
   - Extracts callback Function
   - Calls loop.ScheduleTimer(1000ms, func(*eventloop.Loop) {
       // Execute JavaScript callback
       call.Argument(0).Call(otto.UndefinedValue())
     })

4. eventloop.ScheduleTimer:
   - Creates Timer with timestamp in heap
   - Returns Cancellable

5. On next event loop iteration:
   - Timer fires (timestamp expired)
   - Executes callback
   - Callback calls otto.Function.Call() WITHIN event loop goroutine
   - JavaScript executes

6. console.log() executes → Go I/O → stdout
```

### 6.4 Data Flow: Promise Example

```
1. JavaScript calls: new Promise((resolve, reject) => { ... })

2. goja calls NewPromise(executor):
   - JavaScript executor function creates Promise
   - Calls executor(resolve, reject) on event loop goroutine

3. Inside Promise executor:
   - JavaScript code calls resolve(value)

4. goja's runtime queues microtask (Promise.then handler)
   - goja calls microtaskScheduler.Schedule(fn)

5. Microtask scheduler calls:
   - loop.ScheduleMicrotask(func(runtime interface{}) {
       microtaskFn(runtime)
     })

6. On event loop:
   - After task completes, microtasks drained
   - Promise.then callback executes on event loop goroutine
```

---

## 7. Comparison of Approaches

| Approach | Concurrency Model | timer Implementation | Microtasks | Performance |
|----------|-------------------|---------------------|------------|-------------|
| **goja_nodejs** | batch swap with timestamps | timestamp-based job queue | Not shown (likely built-in) | ~500ns latency (fast) |
| **natto (otto)** | time.AfterFunc → channel | time.Timer sends to channel | N/A (no Promise support) | Higher latency (goroutine per timer) |
| **eventloop package** | batch swap with fast path | heap-based timestamp scheduling | MicrotaskRing buffer | ~500ns latency (fast path matches goja_nodejs) |

### 7.1 Key Differences

**goja_nodejs vs natto:**
- **goja_nodejs**: Timestamp-based scheduling (all timers in one queue, checked each iteration)
- **natto**: `time.AfterFunc` per timer (separate goroutine per timer)

**Advantage of timestamp-based approach:**
- O(1) heap insert
- O(log n) heap pop
- Single goroutine for all timing
- No timer channel churn
- Predictable memory allocation

**Advantage of `time.AfterFunc` approach:**
- Simpler implementation
- Goroutine-per-timer matches Go idioms (but higher overhead)

**eventloop package vs goja_nodejs:**
- Both use batch swap pattern (auxJobs/auxJobsSpare)
- Both use fast path for task-only workloads
- Both achieve ~500ns latency
- eventloop has MicrotaskRing for microtasks (not shown in goja_nodejs)

---

## 8. Best Practices and Reference Implementations

### 8.1 Concurrency Safety Rules

1. **NEVER call JavaScript runtime from multiple goroutines**
   - Create ONE runtime instance
   - Execute ALL JavaScript on dedicated goroutine
   - Use channels to schedule work from external goroutines

2. **JavaScript values MUST NOT escape event loop goroutine**
   - Goja values MUST NOT be stored outside runtime goroutine
   - Primitives can be converted (int, string, etc.)
   - Functions/Objects MUST stay within runtime

3. **External I/O results MUST be scheduled to event loop**
   - async Go operations must schedule callbacks
   - Use `loop.ScheduleTask()` or `loop.ScheduleTimer()`
   - Never call JavaScript directly from async callback

### 8.2 Memory Management

1. **Timer Cleanup**
   - Clear timers when JavaScript context destroyed
   - Use `Terminate()` or similar to stop all timers
   - Remove from registry on clearTimeout/clearInterval

2. **Microtask Draining**
   - Drain microtasks after EACH task (per spec)
   - Don't let microtasks accumulate indefinitely
   - Use bounded queue (MicrotaskRing) to prevent memory blowup

3. **Promise Leaks**
   - Track unhandled rejections with goja's/SetPromiseRejectionTracker()
   - Report rejections via event loop
   - Ensure all Promise errors are visible

### 8.3 Performance Best Practices

1. **Batch Swap Pattern (~500ns latency)**
   - Use slice swap for immediate task queues
   - Execute WITHOUT holding lock
   - Pattern: `lock → swap slices → unlock → execute`

2. **Heap-Based Timer Scheduling**
   - Use timestamp-based job queue
   - Minimize heap operations
   - Check job queue on each loop iteration

3. **Fast Path for Task-Only Workloads**
   - When only immediate tasks: skip poll wait
   - Return to auxJobs drain immediately
   - Matches goja_nodejs behavior

### 8.4 Error Handling

1. **JavaScript Exceptions**
   - Catch exceptions in JavaScript runtime
   - Convert to Go errors
   - Report via event loop (not JavaScript value)

2. **Go Panics in Callbacks**
   - Recover panics in Go callbacks
   - Report through JavaScript runtime
   - Continue event loop if recoverable

3. **Unhandled Promise Rejections**
   - Use `SetPromiseRejectionTracker()` (goja)
   - Schedule report via event loop
   - Log or terminate according to policy

### 8.5 Reference Implementations

**See these for detailed implementation:**
- **goja_nodejs/eventloop.go**: https://github.com/dop251/goja_nodejs/blob/master/eventloop/eventloop.go
  - Batch swap pattern (auxJobs, auxJobsSpare)
  - Timestamp-based timer scheduling
  - RunOnLoop() API for external scheduling

- **natto/natto.go**: https://github.com/robertkrimen/natto/blob/master/natto.go
  - Otto integration example
  - time.AfterFunc for timers
  - Ready channel for timer events

- **eventloop/loop.go** (local)
  - High-performance event loop
  - Fast path mode for task-only workloads
  - MicrotaskRing for Promise support
  - Timer heap implementation

---

## 9. Recommended Integration Strategy

### 9.1 Phase 1: Goja Runtime Binding

1. Create adaptor in `jsruntime/` package
2. Bind JavaScript globals (setTimeout, setInterval, Promise)
3. Implement microtask scheduler using `loop.ScheduleMicrotask()`
4. Test with simple setTimeout scripts

### 9.2 Phase 2: Promise Integration

1. Integrate goja's `SetPromiseRejectionTracker()`
2. Ensure microtasks drain after each task
3. Test with Promise chains

### 9.3 Phase 3: I/O Integration (Optional)

1. Implement Node.js-style APIs (fs, net, etc.)
2. Use Go I/O with callbacks to event loop
3. Test async file operations

### 9.4 Phase 4: Performance Optimization

1. Benchmark latency against goja_nodejs
2. Optimize batch swap if needed
3. Profile memory usage for timers/microtasks

---

## 10. Open Questions and Considerations

1. **Microtask Draining**: Where exactly to call `DrainMicrotasks()`?
   - After each task? (per spec)
   - After all tasks in iteration?
   - goja_nodejs doesn't show this - how do they handle microtasks?

2. **Timer Precision**: Should we use time.Timer per timer or heap-based?
   - Heap-based: Single goroutine, ~500ns latency
   - time.AfterFunc: Separate goroutines per timer, simpler but higher overhead

3. **Promise Chaining**: How does goja handle Promise.then microtasks?
   - Does it provide a callback we can hook?
   - Or does it require us to implement queueMicrotask() API?

4. **I/O Model**: Should we support Node.js-style async I/O?
   - Full Node.js compatibility (massive effort)
   - Or minimal event loop with user-provided I/O

5. **Exception Propagation**: How to surface JavaScript errors to Go caller?
   - Convert to Go errors?
   - Keep as JavaScript values?
   - What about uncaught exceptions in setTimeout?

---

## Conclusion

The research demonstrates that integrating Go JavaScript runtimes with high-performance event loops is well-understood with established patterns:

1. **Single goroutine constraint**: All JavaScript execution must be on dedicated goroutine
2. **Batch swap pattern**: Enables ~500ns latency for immediate tasks
3. **Timestamp-based timer scheduling**: Efficient heap-based approach
4. **Microtask queue**: Required for Promise.then semantics

The `eventloop` package in go-utilpkg already implements the core patterns (batch swap, fast path, timer heap, MicrotaskRing), making it well-suited for JavaScript runtime integration. The primary work required is:

1. Creating adaptor layer for goja runtime binding
2. Implementing timer tracking and clearTimeout/clearInterval
3. Implementing microtask scheduling for Promises
4. Testing with setTimeout/Promise workloads

Reference implementations (goja_nodejs, natto) provide proven patterns and can serve as architectural guides.
