# Event Loop Debugging Guide

This document provides practical guidance for debugging common issues in the `eventloop` package.

## Table of Contents

1. [Common Issues and Solutions](#common-issues-and-solutions)
2. [Debugging with Test Hooks](#debugging-with-test-hooks)
3. [Interpreting Metrics](#interpreting-metrics)
4. [Race Detector Usage](#race-detector-usage)
5. [Structured Logging Configuration](#structured-logging-configuration)
6. [Using ToChannel for Promise Debugging](#using-tochannel-for-promise-debugging)

---

## Common Issues and Solutions

### Deadlocks

#### Symptoms
- Program hangs indefinitely
- `go tool pprof` shows goroutines blocked on mutexes or channels
- No output from callbacks despite submitting tasks

#### Causes

1. **Calling blocking operations on the loop thread:**
   ```go
   // BAD: Blocks the loop thread waiting for itself
   loop.Submit(func() {
       result := <-someChannel  // This blocks the loop!
       process(result)
   })
   ```

2. **Circular promise dependencies:**
   ```go
   // BAD: Promise A waits for B, B waits for A
   var promiseA, promiseB *ChainedPromise
   promiseA = js.NewPromise(func(resolve, reject Settler) {
       promiseB.Then(func(r Result) Result {
           resolve(r)
           return nil
       }, nil)
   })
   // promiseB depends on promiseA... DEADLOCK
   ```

3. **Synchronous blocking in Submit callback:**
   ```go
   // BAD: Blocks loop waiting for external response
   loop.Submit(func() {
       resp, _ := http.Get("https://slow-server.com") // BLOCKS!
       process(resp)
   })
   ```

#### Detection

1. **Use pprof goroutine dump:**
   ```bash
   curl http://localhost:6060/debug/pprof/goroutine?debug=2 > goroutines.txt
   ```
   Look for goroutines blocked on `sync.Mutex` or channel operations.

2. **Add timeout to your Run context:**
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   err := loop.Run(ctx)
   if err == context.DeadlineExceeded {
       // Likely deadlock - capture debugging info
   }
   ```

3. **Check loop state:**
   ```go
   state := loop.State()
   fmt.Printf("Loop state: %v\n", state)
   // StateRunning = 1, StateSleeping = 2, StateTerminating = 3, StateTerminated = 4
   ```

#### Prevention

1. **Use Promisify for blocking operations:**
   ```go
   // GOOD: Run blocking operation outside loop thread
   promise := loop.Promisify(ctx, func(ctx context.Context) (any, error) {
       return http.Get("https://slow-server.com")
   })
   promise.Then(func(r Result) Result {
       process(r.(*http.Response))
       return nil
   }, nil)
   ```

2. **Use PromisifyWithTimeout for operations with deadlines:**
   ```go
   promise := loop.PromisifyWithTimeout(ctx, 5*time.Second, func(ctx context.Context) (any, error) {
       return fetchData(ctx)
   })
   ```

3. **Avoid nested blocking calls:**
   ```go
   // GOOD: Chain promises instead of blocking
   fetchUser(userID).
       Then(func(user Result) Result {
           return fetchOrders(user.(*User).ID) // Returns promise
       }, nil).
       Then(func(orders Result) Result {
           displayOrders(orders)
           return nil
       }, nil)
   ```

---

### Memory Leaks

#### Detection

1. **Runtime memory stats:**
   ```go
   func getMemStats() runtime.MemStats {
       runtime.GC()
       runtime.GC() // Run twice for thorough collection
       var m runtime.MemStats
       runtime.ReadMemStats(&m)
       return m
   }

   baseline := getMemStats()
   // ... run operations ...
   runtime.GC()
   current := getMemStats()

   growth := current.HeapAlloc - baseline.HeapAlloc
   if growth > threshold {
       log.Printf("Memory grew by %d bytes", growth)
   }
   ```

2. **pprof heap profiling:**
   ```bash
   go tool pprof http://localhost:6060/debug/pprof/heap
   (pprof) top 20
   (pprof) list eventloop
   ```

3. **Monitor promise registry:**
   ```go
   // Metrics show pending promises
   metrics := loop.Metrics()
   // Large Queue.IngressMax or Queue.MicrotaskMax may indicate backlogs
   ```

#### Common Sources

1. **Unresolved promises:**
   ```go
   // BAD: Promise never settles, leaks memory
   p, resolve, _ := js.NewChainedPromise()
   // ... never call resolve() or reject()
   ```
   **Fix:** Always settle promises. Use contexts with deadlines:
   ```go
   p := loop.PromisifyWithTimeout(ctx, 30*time.Second, func(ctx context.Context) (any, error) {
       return doWork(ctx) // Guaranteed to return or timeout
   })
   ```

2. **Uncanceled intervals:**
   ```go
   // BAD: Interval runs forever
   js.SetInterval(func() { log.Print("tick") }, 1000)

   // GOOD: Store ID and clear on shutdown
   id := js.SetInterval(func() { log.Print("tick") }, 1000)
   defer js.ClearInterval(id)
   ```

3. **Closure captures:**
   ```go
   // BAD: Large object captured in long-lived closure
   largeData := loadHugeDataset()
   js.SetInterval(func() {
       process(largeData) // largeData never released
   }, 60000)

   // GOOD: Process and release
   js.SetImmediate(func() {
       largeData := loadHugeDataset()
       process(largeData)
       // largeData released after function returns
   })
   ```

4. **Unhandled rejections accumulating:**
   ```go
   // Configure handler to prevent accumulation
   js, _ := NewJS(loop, WithUnhandledRejection(func(p *ChainedPromise, reason any) {
       log.Printf("Unhandled rejection: %v", reason)
       // Log and acknowledge - prevents memory buildup
   }))
   ```

#### Verification

```go
func TestNoMemoryLeak(t *testing.T) {
    loop, _ := New()
    ctx, cancel := context.WithCancel(context.Background())
    go loop.Run(ctx)

    baseline := getMemStats()

    // Run workload
    for i := 0; i < 10000; i++ {
        loop.Submit(func() { /* work */ })
    }

    // Allow processing and GC
    time.Sleep(100 * time.Millisecond)
    runtime.GC()
    runtime.GC()

    final := getMemStats()
    growth := int64(final.HeapAlloc) - int64(baseline.HeapAlloc)

    // Allow some tolerance (1MB)
    if growth > 1024*1024 {
        t.Errorf("Memory grew by %d bytes", growth)
    }

    cancel()
}
```

---

### Handler Not Firing

If your callback isn't being executed, check these common causes:

#### 1. Check registration succeeded

```go
err := loop.Submit(myHandler)
if err != nil {
    log.Printf("Submit failed: %v", err)
    // Common errors:
    // - ErrLoopTerminated: Loop has shut down
    // - ErrLoopOverloaded: Queue exceeded budget
}
```

#### 2. Verify loop is running

```go
state := loop.State()
if state != StateRunning && state != StateSleeping {
    log.Printf("Loop not running, state: %v", state)
}

// Ensure Run() was called
go func() {
    err := loop.Run(ctx)
    if err != nil {
        log.Printf("Loop exited with error: %v", err)
    }
}()
```

#### 3. Check for silently swallowed panics

The loop recovers from panics to prevent crashes. Check logs:

```go
// Panics are logged but don't stop the loop
loop.Submit(func() {
    panic("oops") // Logged: "eventloop: task panicked: oops"
})

// With structured logging:
loop, _ := New(WithLogger(logger))
// Panics logged via logger.Err()
```

#### 4. Timer not firing

```go
// Verify timer was scheduled
id, err := js.SetTimeout(handler, 1000)
if err != nil {
    log.Printf("SetTimeout failed: %v", err)
    // ErrTimerIDExhausted: Over 2^53 timers created
}

// Check if timer was cancelled
// Note: CancelTimer is synchronous
err = loop.CancelTimer(id)
if errors.Is(err, ErrTimerNotFound) {
    log.Print("Timer already fired or cancelled")
}
```

---

### Promise Never Settles

#### Check for uncancelled async operations

```go
// BAD: Context cancel doesn't reach the goroutine
p := loop.Promisify(ctx, func(ctx context.Context) (any, error) {
    result := <-slowChannel // Context not checked!
    return result, nil
})

// GOOD: Check context
p := loop.Promisify(ctx, func(ctx context.Context) (any, error) {
    select {
    case result := <-slowChannel:
        return result, nil
    case <-ctx.Done():
        return nil, ctx.Err()
    }
})
```

#### Use ToChannel with timeout to detect stuck promises

```go
ch := promise.ToChannel()
select {
case result := <-ch:
    process(result)
case <-time.After(10 * time.Second):
    log.Print("Promise stuck - may indicate:")
    log.Print("  - Promisify function never returns")
    log.Print("  - resolve/reject never called")
    log.Print("  - Blocking operation in callback")
}
```

---

## Debugging with Test Hooks

The `loopTestHooks` struct provides injection points for deterministic testing.

### Available Hooks

```go
type loopTestHooks struct {
    PrePollSleep           func()       // Before CAS to StateSleeping
    PrePollAwake           func()       // Before CAS back to StateRunning
    OnFastPathEntry        func()       // Entering fast path mode
    AfterOptimisticCheck   func()       // After check, before Swap
    BeforeFastPathRollback func()       // Before rollback attempt
    PollError              func() error // Inject poll errors
}
```

### Poll Error Injection

Test error handling by injecting simulated poll failures:

```go
func TestHandlePollError(t *testing.T) {
    loop, _ := New()

    var errorInjected atomic.Bool
    loop.testHooks = &loopTestHooks{
        PollError: func() error {
            if errorInjected.CompareAndSwap(false, true) {
                return errors.New("simulated EBADF")
            }
            return nil
        },
    }

    // Force I/O mode (PollError only triggers in I/O mode)
    pipeR, pipeW, _ := os.Pipe()
    defer pipeR.Close()
    defer pipeW.Close()
    loop.RegisterFD(int(pipeR.Fd()), EventRead, func(IOEvents) {})

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    err := loop.Run(ctx)
    // Error triggers handlePollError -> state transitions to Terminated

    if loop.State() != StateTerminated {
        t.Error("Expected terminated state after poll error")
    }
}
```

### Testing Race Conditions

Use hooks to control scheduling at critical points:

```go
func TestFastPathModeRace(t *testing.T) {
    loop, _ := New()

    proceed := make(chan struct{})
    loop.testHooks = &loopTestHooks{
        AfterOptimisticCheck: func() {
            // Pause between check and swap
            <-proceed
        },
    }

    // Start SetFastPathMode in background
    go func() {
        loop.SetFastPathMode(FastPathForced)
    }()

    // Race with RegisterFD
    time.Sleep(time.Millisecond)
    pipeR, pipeW, _ := os.Pipe()
    defer pipeR.Close()
    defer pipeW.Close()

    err := loop.RegisterFD(int(pipeR.Fd()), EventRead, func(IOEvents) {})
    close(proceed) // Allow SetFastPathMode to complete

    // Verify invariant: never Force + FDs registered
    if loop.fastPathMode.Load() == int32(FastPathForced) && loop.userIOFDCount.Load() > 0 {
        t.Error("Invariant violated: FastPathForced with registered FDs")
    }
}
```

### Metrics for Runtime Inspection

Enable metrics and inspect at runtime:

```go
loop, _ := New(WithMetrics(true))
go loop.Run(ctx)

// Periodically sample
go func() {
    for range time.Tick(time.Second) {
        m := loop.Metrics()
        log.Printf("TPS: %.2f, P99: %v, Queue: %d",
            m.TPS, m.Latency.P99, m.Queue.IngressCurrent)
    }
}()
```

---

## Interpreting Metrics

### Core Metrics

Enable metrics collection:

```go
loop, _ := New(WithMetrics(true))
```

Access metrics snapshot:

```go
m := loop.Metrics()  // Returns deep copy, safe to retain
```

### TasksProcessed / TPS

The TPS (Transactions Per Second) is calculated over a rolling 10-second window:

```go
tps := m.TPS

// Interpretation:
// - 0: No tasks processed in last 10 seconds
// - 1000+: High throughput, monitor latency
// - Sudden drops: May indicate blocking or deadlock
```

### Latency Percentiles

Latency metrics use the P-Square algorithm for O(1) streaming estimation:

```go
// Force percentile computation
m.Latency.Sample()

p50 := m.Latency.P50  // Median latency
p90 := m.Latency.P90  // 90th percentile
p95 := m.Latency.P95  // 95th percentile
p99 := m.Latency.P99  // 99th percentile (tail latency)
max := m.Latency.Max  // Maximum observed

// Interpretation:
// - P50 < 1ms: Excellent
// - P99 < 10ms: Good for most applications
// - P99 > 100ms: Consider profiling callbacks
// - P99 >> P50: Long-tail, some callbacks slow
```

### Queue Depths

Monitor for backpressure:

```go
// Current depths
ingress := m.Queue.IngressCurrent    // External queue (from Submit)
internal := m.Queue.InternalCurrent  // Internal priority queue
microtask := m.Queue.MicrotaskCurrent

// Maximum observed
ingressMax := m.Queue.IngressMax

// Exponential moving average
ingressAvg := m.Queue.IngressAvg

// Warning signs:
// - IngressMax > 1000: May hit overload
// - IngressAvg growing: Sustained load increase
// - MicrotaskMax high: Many promise resolutions
```

### P-Square Percentile Interpretation

The P-Square algorithm provides O(1) streaming percentile estimation with ~1-5% relative error:

```go
// For sample counts >= 5, P-Square is used
// For smaller counts, exact sorting is used

// Accuracy characteristics:
// - Error typically < 5% for P99
// - More accurate with more samples
// - Best for monitoring trends, not exact values

// If you need exact percentiles:
// 1. Use small sample windows (< 5 samples)
// 2. Collect raw data externally
```

### Example: Metrics Dashboard

```go
func monitorLoop(loop *Loop, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for range ticker.C {
        m := loop.Metrics()
        m.Latency.Sample()  // Compute percentiles

        state := loop.State()
        if state != StateRunning && state != StateSleeping {
            log.Printf("⚠️  Loop state: %v", state)
        }

        log.Printf("📊 TPS: %.1f | P50: %v | P99: %v | Queue: %d/%d",
            m.TPS,
            m.Latency.P50,
            m.Latency.P99,
            m.Queue.IngressCurrent,
            m.Queue.IngressMax)

        // Alert on high latency
        if m.Latency.P99 > 100*time.Millisecond {
            log.Printf("⚠️  High P99 latency: %v", m.Latency.P99)
        }

        // Alert on queue growth
        if m.Queue.IngressMax > 500 {
            log.Printf("⚠️  Queue peaked at %d", m.Queue.IngressMax)
        }
    }
}
```

---

## Race Detector Usage

### Running with -race

```bash
# Run all tests with race detector
go test -race ./eventloop/...

# Run specific test with race detector
go test -race -run TestMyFunction ./eventloop/

# Run with timeout (race detector is slower)
go test -race -timeout 5m ./eventloop/...
```

### Common Race Patterns

#### 1. Variable access from callback and caller

```go
// BAD: Race between main goroutine and callback
var result string
loop.Submit(func() {
    result = "done"  // Write
})
time.Sleep(time.Millisecond)
fmt.Println(result)  // Read - RACE!

// GOOD: Use channel or sync
resultCh := make(chan string, 1)
loop.Submit(func() {
    resultCh <- "done"
})
result := <-resultCh
fmt.Println(result)

// GOOD: Use atomic for simple values
var done atomic.Bool
loop.Submit(func() {
    done.Store(true)
})
// ... later
if done.Load() { ... }
```

#### 2. Interval callback races

```go
// BAD: intervalID read before assignment
var intervalID uint64
intervalID = js.SetInterval(func() {
    if shouldStop {
        js.ClearInterval(intervalID)  // May read stale value
    }
}, 100)

// GOOD: Use atomic
var intervalID atomic.Uint64
id := js.SetInterval(func() {
    if shouldStop {
        js.ClearInterval(intervalID.Load())
    }
}, 100)
intervalID.Store(id)
```

#### 3. Shared slice access

```go
// BAD: Append from callback races with read
var results []string
for i := 0; i < 10; i++ {
    loop.Submit(func() {
        results = append(results, "item")  // RACE!
    })
}
fmt.Println(len(results))  // RACE!

// GOOD: Use mutex
var (
    results []string
    mu      sync.Mutex
)
for i := 0; i < 10; i++ {
    loop.Submit(func() {
        mu.Lock()
        results = append(results, "item")
        mu.Unlock()
    })
}
// Wait for completion, then read
mu.Lock()
fmt.Println(len(results))
mu.Unlock()
```

### Fixing Data Races

1. **Use atomic for simple values:**
   ```go
   var counter atomic.Int64
   counter.Add(1)  // Thread-safe
   ```

2. **Use channels for coordination:**
   ```go
   done := make(chan struct{})
   loop.Submit(func() {
       // ... work ...
       close(done)
   })
   <-done  // Wait for completion
   ```

3. **Use ToChannel for promise results:**
   ```go
   ch := promise.ToChannel()
   result := <-ch  // Safe, no races
   ```

4. **Synchronize with mutex:**
   ```go
   var (
       data map[string]int
       mu   sync.RWMutex
   )

   // Write
   mu.Lock()
   data["key"] = value
   mu.Unlock()

   // Read
   mu.RLock()
   v := data["key"]
   mu.RUnlock()
   ```

---

## Structured Logging Configuration

### Setting Up logiface Logger

```go
import (
    "github.com/joeycumines/logiface"
    "github.com/joeycumines/logiface-zerolog"
)

// Create logger with zerolog backend
logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
logifaceLogger := logiface.NewLogger(logifacezerolog.NewWriter(&logger))

// Create loop with logger
loop, err := New(WithLogger(&logifaceLogger))
if err != nil {
    log.Fatal(err)
}
```

### Log Levels and Meanings

| Level | When Used | Meaning |
|-------|-----------|---------|
| ERROR | Task panic | A callback panicked; recovered but logged |
| ERROR | OnOverload panic | OnOverload callback panicked |
| CRITICAL | Poll error | kqueue/epoll/IOCP failed; loop terminating |

### What Gets Logged

```go
// Task panic logs:
// {
//   "level": "error",
//   "component": "eventloop",
//   "panic": "division by zero",
//   "msg": "task panicked"
// }

// Poll error logs:
// {
//   "level": "critical",
//   "component": "eventloop",
//   "err": "EBADF",
//   "msg": "poll error"
// }
```

### Fallback Behavior

When no logger is configured (or logger is nil), the event loop falls back to `log.Printf`:

```go
// Without logger:
// 2026/02/06 10:30:45 ERROR: eventloop: task panicked: division by zero

// With logger:
// {"level":"error","component":"eventloop","panic":"division by zero","msg":"task panicked"}
```

### Filtering by Component

Set up filtering to focus on eventloop logs:

```go
// With zerolog
logger := zerolog.New(os.Stderr).
    Level(zerolog.InfoLevel).                    // Minimum level
    With().
    Str("service", "myapp").
    Logger()

// Or filter by component in log aggregator:
// component == "eventloop" AND level >= "error"
```

---

## Using ToChannel for Promise Debugging

### Basic Promise Awaiting

`ToChannel()` returns a buffered channel that receives the result when the promise settles:

```go
promise := js.Resolve(42)

// Get channel before settlement
ch := promise.ToChannel()

// Block until settled
result := <-ch
fmt.Printf("Result: %v\n", result)  // Output: Result: 42
```

### Timeout Patterns

Avoid indefinite waits with timeout:

```go
func awaitWithTimeout(p *ChainedPromise, timeout time.Duration) (Result, error) {
    select {
    case result := <-p.ToChannel():
        if p.State() == Rejected {
            return nil, fmt.Errorf("rejected: %v", result)
        }
        return result, nil
    case <-time.After(timeout):
        return nil, fmt.Errorf("timeout after %v", timeout)
    }
}

// Usage:
result, err := awaitWithTimeout(promise, 5*time.Second)
if err != nil {
    log.Printf("Promise failed: %v", err)
}
```

### State Inspection

```go
// Check state without blocking
state := promise.State()
switch state {
case Pending:
    fmt.Println("Still waiting...")
case Fulfilled:
    fmt.Printf("Resolved with: %v\n", promise.Result())
case Rejected:
    fmt.Printf("Rejected with: %v\n", promise.Result())
}
```

### Debugging Stuck Promises

```go
func debugPromise(p *ChainedPromise, name string) {
    ch := p.ToChannel()

    select {
    case result := <-ch:
        state := p.State()
        switch state {
        case Fulfilled:
            log.Printf("✅ %s fulfilled: %v", name, result)
        case Rejected:
            log.Printf("❌ %s rejected: %v", name, result)
        }
    case <-time.After(5 * time.Second):
        log.Printf("⏳ %s still pending after 5s", name)
        log.Printf("   State: %v", p.State())
        log.Printf("   Possible causes:")
        log.Printf("   - Promisify function blocking")
        log.Printf("   - resolve/reject never called")
        log.Printf("   - Deadlock in promise chain")
    }
}

// Usage:
p := fetchUserData(userID)
go debugPromise(p, "fetchUserData")
```

### Concurrent Promise Debugging

```go
func awaitAll(promises []*ChainedPromise, timeout time.Duration) ([]Result, error) {
    results := make([]Result, len(promises))
    errors := make([]error, len(promises))
    var wg sync.WaitGroup

    for i, p := range promises {
        wg.Add(1)
        go func(idx int, promise *ChainedPromise) {
            defer wg.Done()

            select {
            case result := <-promise.ToChannel():
                if promise.State() == Rejected {
                    errors[idx] = fmt.Errorf("promise %d rejected: %v", idx, result)
                } else {
                    results[idx] = result
                }
            case <-time.After(timeout):
                errors[idx] = fmt.Errorf("promise %d timed out", idx)
            }
        }(i, p)
    }

    wg.Wait()

    // Collect errors
    var errs []error
    for _, err := range errors {
        if err != nil {
            errs = append(errs, err)
        }
    }

    if len(errs) > 0 {
        return results, fmt.Errorf("%d promises failed: %v", len(errs), errs)
    }

    return results, nil
}
```

### Promise Chain Tracing

```go
func tracePromiseChain(initial *ChainedPromise) *ChainedPromise {
    step := 0

    trace := func(name string) func(Result) Result {
        myStep := step
        step++
        return func(r Result) Result {
            state := "processing"
            if r == nil {
                state = "nil"
            }
            log.Printf("[Step %d] %s: %v (%s)", myStep, name, r, state)
            return r
        }
    }

    return initial.
        Then(trace("after-initial"), nil).
        Then(func(r Result) Result {
            // Your actual logic
            return transform(r)
        }, nil).
        Then(trace("after-transform"), nil).
        Catch(func(r Result) Result {
            log.Printf("[ERROR] Chain rejected: %v", r)
            return r  // Re-reject
        })
}
```

---

## Quick Reference

### Loop States

| State | Value | Meaning |
|-------|-------|---------|
| StateAwake | 0 | Created but not running |
| StateRunning | 1 | Actively processing tasks |
| StateSleeping | 2 | Waiting for I/O or wakeup |
| StateTerminating | 3 | Shutting down |
| StateTerminated | 4 | Fully stopped |

### Common Errors

| Error | Meaning |
|-------|---------|
| ErrLoopAlreadyRunning | Run() called on running loop |
| ErrLoopTerminated | Operation on stopped loop |
| ErrLoopOverloaded | Queue exceeded tick budget |
| ErrReentrantRun | Run() called from within loop |
| ErrTimerNotFound | Timer already fired/cancelled |
| ErrTimerIDExhausted | Over 2^53 timers created |

### Debugging Checklist

1. ☐ Is the loop running? Check `loop.State()`
2. ☐ Did Submit/Timer return an error?
3. ☐ Are callbacks panicking? Check logs
4. ☐ Is there a deadlock? Use `-race` and pprof
5. ☐ Are promises settling? Use `ToChannel` with timeout
6. ☐ Is memory growing? Profile with pprof heap
7. ☐ Are intervals cleared? Track IDs, clear on shutdown
8. ☐ Is context being checked? Use `select` with `ctx.Done()`
