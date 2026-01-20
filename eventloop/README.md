# go-eventloop

Package eventloop provides a high-performance, JavaScript-compatible event loop implementation for Go.

See the [API docs](https://pkg.go.dev/github.com/joeycumines/go-eventloop).

## Features

- **JavaScript-Compatible Timer APIs**: Implements `setTimeout`, `setInterval`, `clearTimeout`, `clearInterval` semantics with proper ID-based cancellation
- **Microtask Queue**: `queueMicrotask` implementation with correct priority over timer callbacks
- **Promise/A+ Compliance**: Full `ChainedPromise` implementation with `Then`, `Catch`, `Finally` and proper async semantics
- **Promise Combinators**: `All`, `Race`, `AllSettled`, `Any` for composing multiple promises
- **Unhandled Rejection Tracking**: Configurable callbacks for unhandled promise rejections
- **Cross-Platform**: Native I/O polling on macOS (kqueue), Linux (epoll), and Windows (IOCP)
- **Zero-Allocation Hot Paths**: Timer pooling and optimized memory management
- **Performance Monitoring**: Built-in latency, TPS, and queue depth metrics
- **HTML5 Spec Compliance**: Nested timeout clamping (4ms after 5 levels of nesting)

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
    "time"
    
    "github.com/joeycumines/go-eventloop"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    
    loop := eventloop.New()
    
    // Schedule a timer
    loop.ScheduleTimer(100*time.Millisecond, func() {
        fmt.Println("Timer fired!")
    })
    
    // Run the loop
    loop.Run(ctx)
}
```

### JavaScript-Compatible Timers

```go
loop := eventloop.New()
js, err := eventloop.NewJS(loop)
if err != nil {
    log.Fatal(err)
}

// setTimeout
id, _ := js.SetTimeout(func() {
    fmt.Println("Hello after 100ms")
}, 100)

// clearTimeout
js.ClearTimeout(id)

// setInterval
intervalID, _ := js.SetInterval(func() {
    fmt.Println("Tick!")
}, 1000)

// clearInterval after 5 ticks
go func() {
    time.Sleep(5500 * time.Millisecond)
    js.ClearInterval(intervalID)
}()
```

### Microtask Queue

```go
js.QueueMicrotask(func() {
    fmt.Println("Microtask 1")
})

js.SetTimeout(func() {
    fmt.Println("Timer")
}, 0)

js.QueueMicrotask(func() {
    fmt.Println("Microtask 2")
})

// Output:
// Microtask 1
// Microtask 2
// Timer
```

### Promises

```go
// Create a promise
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
    Then(func(v eventloop.Result) eventloop.Result {
        fmt.Printf("Got: %v\n", v)
        return transform(v)
    }, nil).
    Catch(func(r eventloop.Result) eventloop.Result {
        fmt.Printf("Error: %v\n", r)
        return nil
    }).
    Finally(func() {
        cleanup()
    })
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
anyPromise := js.Any([]*eventloop.ChainedPromise{p1, p2, p3})
```

### Unhandled Rejection Tracking

```go
js, err := eventloop.NewJS(loop,
    eventloop.WithUnhandledRejection(func(reason eventloop.Result) {
        log.Printf("Unhandled rejection: %v", reason)
    }),
)
```

### Performance Metrics

```go
loop := eventloop.New(
    eventloop.WithMetrics(true),
)

// Later, sample metrics
metrics := loop.Metrics()
fmt.Printf("P99 Latency: %v\n", metrics.Latency.P99)
fmt.Printf("Current TPS: %.2f\n", metrics.TPS)
fmt.Printf("Queue Depth: %d\n", metrics.Queue.Current)
```

## Architecture

The event loop consists of several key components:

1. **Loop**: The core scheduler that manages timers, tasks, and I/O polling
2. **JS Adapter**: Provides JavaScript-compatible timer and promise APIs
3. **ChainedPromise**: Promise/A+ implementation with proper microtask scheduling
4. **Platform Pollers**: Native I/O implementations (kqueue, epoll, IOCP)

## Thread Safety

- `Loop`, `JS`, and `ChainedPromise` are safe for concurrent use
- Timer and microtask callbacks always execute on the event loop thread
- Promise resolve/reject functions can be called from any goroutine

## Performance

- Timer scheduling: ~200-500 ns/op with pooling
- Microtask queue: lockless fast-path for single-producer scenarios
- Memory: Near-zero allocations in steady state

## License

ISC License - see [LICENSE](LICENSE) for details.
