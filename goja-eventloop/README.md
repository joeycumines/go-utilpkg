# goja-eventloop

Package goja-eventloop provides bindings between [go-eventloop](https://pkg.go.dev/github.com/joeycumines/go-eventloop) and the [Goja](https://github.com/dop251/goja) JavaScript runtime.

See the [API docs](https://pkg.go.dev/github.com/joeycumines/goja-eventloop).

## Features

- **Full JavaScript Timer APIs**: `setTimeout`, `clearTimeout`, `setInterval`, `clearInterval`
- **Microtask Support**: `queueMicrotask` with correct priority semantics
- **Promise Integration**: Native `Promise` constructor with `then`/`catch`/`finally`
- **Promise Combinators**: Access to `All`, `Race`, `AllSettled`, `Any` from Go
- **Thread-Safe Coordination**: Proper synchronization between Goja and event loop threads

## Installation

```bash
go get github.com/joeycumines/goja-eventloop
```

## Usage

### Basic Setup

```go
package main

import (
    "context"
    "fmt"
    "time"
    
    "github.com/dop251/goja"
    eventloop "github.com/joeycumines/go-eventloop"
    gojaeventloop "github.com/joeycumines/goja-eventloop"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    // Create event loop and Goja runtime
    loop := eventloop.New()
    runtime := goja.New()
    
    // Create adapter and bind JavaScript globals
    adapter, err := gojaeventloop.New(loop, runtime)
    if err != nil {
        panic(err)
    }
    if err := adapter.Bind(); err != nil {
        panic(err)
    }
    
    // JavaScript code now has access to async APIs
    _, err = runtime.RunString(`
        console.log("Starting...");
        
        setTimeout(() => {
            console.log("Hello after 100ms!");
        }, 100);
        
        queueMicrotask(() => {
            console.log("Microtask runs before timer");
        });
        
        new Promise((resolve) => {
            resolve(42);
        }).then((value) => {
            console.log("Promise resolved with:", value);
        });
    `)
    if err != nil {
        panic(err)
    }
    
    // Run the event loop to process callbacks
    loop.Run(ctx)
}
```

### Using Timers from JavaScript

```javascript
// setTimeout returns a timer ID
const id = setTimeout(() => {
    console.log("Timer fired!");
}, 1000);

// clearTimeout cancels the timer
clearTimeout(id);

// setInterval fires repeatedly
const intervalId = setInterval(() => {
    console.log("Tick!");
}, 100);

// clearInterval stops the interval
setTimeout(() => {
    clearInterval(intervalId);
}, 550); // Stop after ~5 ticks
```

### Promises from JavaScript

#### Creating Promises

```javascript
// Create a promise
const promise = new Promise((resolve, reject) => {
    setTimeout(() => {
        resolve("Success!");
    }, 100);
});

// Chain handlers
promise
    .then((value) => {
        console.log("Got:", value);
        return value.toUpperCase();
    })
    .then((upper) => {
        console.log("Upper:", upper);
    })
    .catch((error) => {
        console.log("Error:", error);
    })
    .finally(() => {
        console.log("Cleanup");
    });
```

#### Promise Static Methods

```javascript
// Resolve with a value
Promise.resolve(42).then(value => console.log(value));

// Reject with a reason
Promise.reject(new Error("failed")).catch(err => console.log(err.message));

// These create promises that are already settled without waiting
```

### Promise Combinators from Go

```go
// Create promises
p1, resolve1, _ := adapter.JS().NewChainedPromise()
p2, resolve2, _ := adapter.JS().NewChainedPromise()

// Resolve asynchronously
go func() {
    resolve1("first")
    resolve2("second")
}()

// Use combinators
all := adapter.All([]*eventloop.ChainedPromise{p1, p2})
race := adapter.Race([]*eventloop.ChainedPromise{p1, p2})
settled := adapter.AllSettled([]*eventloop.ChainedPromise{p1, p2})
any := adapter.Any([]*eventloop.ChainedPromise{p1, p2})
```

### Promise Combinators from JavaScript

```javascript
// Promise.all - wait for all to resolve
Promise.all([p1, p2, p3]).then(values => console.log(values));

// Promise.race - first to settle wins
Promise.race([p1, p2]).then(first => console.log(first));

// Promise.allSettled - wait for all to settle
Promise.allSettled([p1, p2]).then(results => console.log(results));

// Promise.any - first to resolve wins
Promise.any([p1, p2, p3]).then(value => console.log(value));
```

#### AggregateError

`Promise.any` throws `AggregateError` when ALL input promises reject. The error contains all rejection reasons:

```javascript
Promise.any([
  Promise.reject(new Error("error 1")),
  Promise.reject(new Error("error 2"))
]).catch(err => {
  console.error("All failed:", err.message);  // "All promises were rejected"
  console.error("Reasons:", err.errors);   // [Error: error 1, Error: error 2]
});
```

### Accessing Components

```go
// Get the underlying JS adapter for Go-level timer/promise access
js := adapter.JS()
js.SetTimeout(func() {
    fmt.Println("Go callback!")
}, 100)

// Get the Goja runtime for script execution
runtime := adapter.Runtime()
runtime.RunString(`console.log("Hello from JS!")`)

// Get the event loop for advanced control
loop := adapter.Loop()
```

## Thread Safety

The adapter coordinates thread safety between:

1. **Goja Runtime**: Not thread-safe; should only be accessed from one goroutine
2. **Event Loop**: Processes callbacks on its own thread
3. **Go Code**: Can schedule timers/promises from any goroutine
4. **Promise APIs**: Fully thread-safe; `then`, `catch`, `finally`, `resolve`, `reject` can be called from any goroutine

After calling `Bind()`, JavaScript callbacks execute on the event loop thread. The Goja runtime should be accessed from the same thread (typically via `loop.Submit()` or from within callbacks).

### ClearInterval Safety

`clearInterval` is safe to call from any goroutine, including from within the interval's own callback. The current execution will complete, and no further executions will occur:

```javascript
const id = setInterval(() => {
    console.log("Tick");
    clearInterval(id); // Safe: current tick completes, no more ticks
}, 1000);
```

## Binding Reference

After calling `adapter.Bind()`, the following globals are available in JavaScript:

| Global | Signature | Description |
|--------|-----------|-------------|
| `setTimeout` | `(fn, delay?) → number` | Schedule one-time callback |
| `clearTimeout` | `(id)` | Cancel scheduled timeout |
| `setInterval` | `(fn, delay?) → number` | Schedule repeating callback |
| `clearInterval` | `(id)` | Cancel scheduled interval |
| `queueMicrotask` | `(fn)` | Schedule high-priority callback |
| `Promise` | `new Promise((resolve, reject) => ...)` | Create async promise || `Promise.resolve` | `(value) → promise` | Create already-fulfilled promise |
| `Promise.reject` | `(reason) → promise` | Create already-rejected promise |
| `Promise.all` | `(iterable) → promise` | Wait for all to resolve |
| `Promise.race` | `(iterable) → promise` | First to settle wins |
| `Promise.allSettled` | `(iterable) → promise` | Wait for all to settle |
| `Promise.any` | `(iterable) → promise` | First to resolve wins |
## Requirements

- Go 1.21+
- github.com/dop251/goja
- github.com/joeycumines/go-eventloop

## License

MIT License - see [LICENSE](../LICENSE) for details.
