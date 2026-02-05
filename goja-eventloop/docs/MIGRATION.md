# Migration Guide for Goja Users

This guide helps you integrate the `goja-eventloop` package with your existing
[Goja](https://github.com/dop251/goja) JavaScript runtime.

## Table of Contents

- [Quick Start](#quick-start)
- [Basic Setup](#basic-setup)
- [Differences from Node.js](#differences-from-nodejs)
- [Common Patterns](#common-patterns)
- [Gotchas and Best Practices](#gotchas-and-best-practices)
- [Thread Safety](#thread-safety)
- [Error Handling](#error-handling)

## Quick Start

```go
package main

import (
    "context"
    "time"
    
    "github.com/dop251/goja"
    eventloop "github.com/joeycumines/go-eventloop"
    gojaeventloop "github.com/joeycumines/goja-eventloop"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    // 1. Create the event loop
    loop, _ := eventloop.New()
    
    // 2. Create Goja runtime
    runtime := goja.New()
    
    // 3. Create adapter and bind globals
    adapter, _ := gojaeventloop.New(loop, runtime)
    adapter.Bind()
    
    // 4. Run JavaScript code (this sets up async operations)
    runtime.RunString(`
        setTimeout(() => console.log("Hello!"), 100);
    `)
    
    // 5. Run the event loop to process async callbacks
    loop.Run(ctx)
}
```

## Basic Setup

### Step-by-Step Integration

```go
// Step 1: Create the core event loop
loop, err := eventloop.New(
    eventloop.WithMetrics(true),              // Optional: enable metrics
    eventloop.WithStrictMicrotaskOrdering(true), // Optional: strict mode
)
if err != nil {
    log.Fatal(err)
}

// Step 2: Create your Goja runtime as usual
runtime := goja.New()

// Step 3: Create the adapter that bridges them
adapter, err := gojaeventloop.New(loop, runtime)
if err != nil {
    log.Fatal(err)
}

// Step 4: Bind JavaScript globals (setTimeout, Promise, etc.)
if err := adapter.Bind(); err != nil {
    log.Fatal(err)
}

// Now your JavaScript has access to:
// - setTimeout, clearTimeout
// - setInterval, clearInterval
// - setImmediate, clearImmediate
// - queueMicrotask
// - Promise (with .then, .catch, .finally, Promise.all, etc.)
// - AbortController, AbortSignal
// - performance.now(), performance.mark(), performance.measure()
// - console.time(), console.timeEnd(), console.timeLog()
```

### Execution Model

```
Your Code                    Event Loop Thread
    │                              │
    ├── runtime.RunString()        │
    │   (sets up async ops)        │
    │                              │
    ├── loop.Run(ctx) ────────────▶│
    │   (blocks here)              │
    │                        ┌─────┴─────┐
    │                        │ tick()    │
    │                        │ - timers  │
    │                        │ - tasks   │
    │                        │ - I/O     │
    │                        └─────┬─────┘
    │                              │
    ◀────────────────── (ctx done) │
    │                              │
```

## Differences from Node.js

### Available APIs

| API | Status | Notes |
|-----|--------|-------|
| `setTimeout` | ✅ Full | Returns number ID |
| `setInterval` | ✅ Full | Returns number ID |
| `setImmediate` | ✅ Full | Available (not standard browser) |
| `clearTimeout` | ✅ Full | |
| `clearInterval` | ✅ Full | |
| `clearImmediate` | ✅ Full | |
| `queueMicrotask` | ✅ Full | |
| `Promise` | ✅ Full | Promise/A+ compliant |
| `Promise.resolve` | ✅ Full | |
| `Promise.reject` | ✅ Full | |
| `Promise.all` | ✅ Full | Supports iterables |
| `Promise.race` | ✅ Full | |
| `Promise.allSettled` | ✅ Full | |
| `Promise.any` | ✅ Full | With AggregateError |
| `Promise.withResolvers` | ✅ Full | ES2024 API |
| `AbortController` | ✅ Full | W3C spec |
| `AbortSignal` | ✅ Full | |
| `performance.now()` | ✅ Full | Sub-millisecond precision |
| `performance.mark()` | ✅ Full | User Timing API |
| `performance.measure()` | ✅ Full | |
| `console.time()` | ✅ Full | Timer API |
| `console.timeEnd()` | ✅ Full | |
| `console.timeLog()` | ✅ Full | |
| `process.nextTick` | ❌ N/A | Use `queueMicrotask` |
| `fetch` | ❌ N/A | Implement via Go HTTP |
| `fs` module | ❌ N/A | Implement via Promisify |
| `EventEmitter` | ❌ N/A | Implement if needed |

### Timer Behavior Differences

```javascript
// Node.js: process.nextTick runs before Promise microtasks
// goja-eventloop: Use queueMicrotask instead (runs at same priority as Promise)

// Node.js
process.nextTick(() => console.log('1'));
Promise.resolve().then(() => console.log('2'));
// Output: 1, 2

// goja-eventloop
queueMicrotask(() => console.log('1'));
Promise.resolve().then(() => console.log('2'));
// Output: 1, 2 (FIFO order within microtask queue)
```

### HTML5 Timer Clamping

Like browsers, nested `setTimeout` calls (depth > 5) are clamped to 4ms minimum:

```javascript
function nested(depth) {
    if (depth > 10) return;
    console.log(`Depth ${depth}`);
    setTimeout(() => nested(depth + 1), 0);
}
nested(1);
// Depths 1-5: immediate (0ms delay)
// Depths 6+: 4ms minimum delay
```

### Promise Chaining Across Go/JS Boundary

```go
// Create promise in Go, consume in JavaScript
promise, resolve, _ := adapter.JS().NewChainedPromise()

go func() {
    result := doAsyncWork()
    resolve(result)
}()

// Make available to JS
runtime.Set("myPromise", adapter.GojaWrapPromise(promise))

// JS can chain normally
_, _ = runtime.RunString(`
    myPromise.then(r => console.log(r));
`)
```

## Common Patterns

### Pattern 1: Async Operation with Timeout

```javascript
// JavaScript
function withTimeout(promise, ms) {
    return Promise.race([
        promise,
        new Promise((_, reject) => 
            setTimeout(() => reject(new Error('Timeout')), ms)
        )
    ]);
}

withTimeout(
    fetch('/api/data'),
    5000
).catch(err => {
    console.log('Failed:', err.message);
});
```

### Pattern 2: Implementing fetch() with Go

```go
// Go side - implement fetch
adapter.Runtime().Set("fetch", func(call goja.FunctionCall) goja.Value {
    url := call.Argument(0).String()
    
    promise, resolve, reject := adapter.JS().NewChainedPromise()
    
    go func() {
        resp, err := http.Get(url)
        if err != nil {
            reject(err.Error())
            return
        }
        defer resp.Body.Close()
        
        body, _ := io.ReadAll(resp.Body)
        resolve(string(body))
    }()
    
    return adapter.GojaWrapPromise(promise)
})

// JavaScript side - use it naturally
_, _ = runtime.RunString(`
    fetch('https://api.example.com/data')
        .then(data => JSON.parse(data))
        .then(obj => console.log(obj))
        .catch(err => console.error(err));
`)
```

### Pattern 3: Event-Driven Architecture

```go
// Create an event emitter pattern
runtime.RunString(`
    const listeners = new Map();
    
    globalThis.on = function(event, handler) {
        if (!listeners.has(event)) {
            listeners.set(event, []);
        }
        listeners.get(event).push(handler);
    };
    
    globalThis.emit = function(event, data) {
        const handlers = listeners.get(event) || [];
        handlers.forEach(h => queueMicrotask(() => h(data)));
    };
`)

// Emit from Go
runtime.RunString(`emit('data', { value: 42 })`)
```

### Pattern 4: Worker Pool with Promises

```javascript
async function workerPool(tasks, concurrency) {
    const results = [];
    const executing = [];
    
    for (const task of tasks) {
        const p = task().then(result => {
            executing.splice(executing.indexOf(p), 1);
            return result;
        });
        results.push(p);
        executing.push(p);
        
        if (executing.length >= concurrency) {
            await Promise.race(executing);
        }
    }
    
    return Promise.all(results);
}
```

### Pattern 5: Cancellable Operations

```javascript
function cancellableFetch(url, signal) {
    return new Promise((resolve, reject) => {
        if (signal.aborted) {
            reject(signal.reason);
            return;
        }
        
        signal.addEventListener('abort', () => {
            reject(signal.reason);
        });
        
        // Simulated fetch
        setTimeout(() => resolve({ data: 'response' }), 100);
    });
}

const controller = new AbortController();
const signal = controller.signal;

cancellableFetch('/api/data', signal)
    .then(console.log)
    .catch(err => console.log('Cancelled:', err));

// Cancel after 50ms
setTimeout(() => controller.abort('User cancelled'), 50);
```

## Gotchas and Best Practices

### ⚠️ Gotcha 1: Goja Runtime is NOT Thread-Safe

```go
// ❌ WRONG: Accessing runtime from multiple goroutines
go func() {
    runtime.RunString(`...`) // RACE CONDITION!
}()

// ✅ CORRECT: Use loop.Submit() to execute on loop thread
loop.Submit(func() {
    runtime.RunString(`...`) // Safe - runs on loop thread
})
```

### ⚠️ Gotcha 2: Complete RunString Before Run

```go
// ❌ WRONG: Starting loop before setup
go loop.Run(ctx)  // Loop running!
runtime.RunString(`...`) // Race with callbacks!

// ✅ CORRECT: Setup first, then start loop
runtime.RunString(`
    setTimeout(() => console.log('Hello'), 100);
`)
loop.Run(ctx) // Now process callbacks
```

### ⚠️ Gotcha 3: Shutdown Properly

```go
// ❌ WRONG: Just canceling context
cancel() // Callbacks may not complete!

// ✅ CORRECT: Graceful shutdown
loop.Shutdown(ctx) // Drains queues, then terminates
```

### ⚠️ Gotcha 4: Promise Error Handling

```javascript
// ❌ Unhandled rejection
Promise.reject(new Error('oops'));
// Warning logged but execution continues

// ✅ Handle rejections
Promise.reject(new Error('oops'))
    .catch(err => console.log('Handled:', err.message));

// ✅ Or use global handler (Go side)
js, _ := eventloop.NewJS(loop,
    eventloop.WithUnhandledRejection(func(reason eventloop.Result) {
        log.Printf("Unhandled: %v", reason)
    }),
)
```

### ⚠️ Gotcha 5: Timer ID Type

```javascript
// Timer IDs are JavaScript numbers (float64-safe integers)
// They are safe up to 2^53 - 1 (MAX_SAFE_INTEGER)

const id = setTimeout(() => {}, 1000);
console.log(typeof id); // "number"

// This works fine
clearTimeout(id);
```

### Best Practices

1. **Always run setup before loop.Run()**
   ```go
   // Setup
   adapter.Bind()
   runtime.RunString(`/* your code */`)
   
   // Then run
   loop.Run(ctx)
   ```

2. **Use context for timeouts**
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   loop.Run(ctx)
   ```

3. **Handle panics in callbacks**
   ```javascript
   setTimeout(() => {
       try {
           riskyOperation();
       } catch (e) {
           console.error('Caught:', e);
       }
   }, 100);
   ```

4. **Clean up intervals**
   ```javascript
   const id = setInterval(() => {
       if (done) {
           clearInterval(id);
       }
   }, 100);
   ```

5. **Use Promise.finally for cleanup**
   ```javascript
   doAsync()
       .then(handle)
       .catch(handleError)
       .finally(cleanup);
   ```

## Thread Safety

### Safe Operations (From Any Goroutine)

```go
// These are all thread-safe:
loop.Submit(task)
loop.SubmitInternal(task)
loop.ScheduleTimer(delay, fn)
loop.CancelTimer(id)
loop.ScheduleMicrotask(fn)
js.SetTimeout(fn, delay)
js.SetInterval(fn, delay)
js.ClearTimeout(id)
js.ClearInterval(id)
js.QueueMicrotask(fn)
promise.Then(onFulfilled, onRejected)
promise.Catch(onRejected)
promise.Finally(onFinally)
resolve(value)
reject(reason)
```

### Unsafe Operations (Loop Thread Only)

```go
// These should only be called from the loop thread:
runtime.RunString(...)    // Goja is not thread-safe
runtime.Set(...)          // Direct runtime access
runtime.Get(...)          // Direct runtime access

// Use Submit to run safely:
loop.Submit(func() {
    runtime.RunString(`console.log("safe!")`)
})
```

## Error Handling

### Go Errors in Promises

```go
go func() {
    err := doWork()
    if err != nil {
        reject(err) // Error type preserved
    }
}()
```

```javascript
promise.catch(err => {
    console.log(err.message); // Go error message available
});
```

### JavaScript Errors in Go

```go
_, err := runtime.RunString(`throw new Error('oops')`)
if err != nil {
    if ex, ok := err.(*goja.Exception); ok {
        fmt.Println(ex.Value().ToObject(runtime).Get("message"))
    }
}
```

### AggregateError from Promise.any

```javascript
Promise.any([
    Promise.reject(1),
    Promise.reject(2),
]).catch(err => {
    console.log(err.name);    // "AggregateError"
    console.log(err.message); // "All promises were rejected"
    console.log(err.errors);  // [1, 2]
});
```
