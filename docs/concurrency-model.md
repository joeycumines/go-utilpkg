# Concurrency Model

## Overview

The system uses a **single-threaded event loop** model for JavaScript execution,
combined with Go's goroutine-based concurrency for off-loop work.

## Thread Safety Guarantees

### Event Loop Goroutine

- All JavaScript (Goja) code runs on the event loop goroutine
- All `StreamHandlerFunc` callbacks run on the event loop goroutine
- All `StreamSender.Send()`, `StreamReceiver.Recv()` calls must be on the event loop
- `RPCStream.Finish()` must be called on the event loop

### Off-Loop (Any Goroutine)

- `channel.Invoke()` — blocks calling goroutine, submits work to loop
- `channel.NewStream()` — blocks calling goroutine, submits work to loop
- `loop.Submit()` — thread-safe, enqueues work for the loop

### Message Cloning

Messages are cloned when crossing the loop/off-loop boundary:

```
Go goroutine                Event Loop
     |                          |
     | -- Invoke(req) --------> |
     |    [clone(req)]          |
     |                    [handler(clone)]
     |                    [send(resp)]
     | <---- resp --------------|
     |    [copy(resp, clone)]   |
```

## Interaction Diagram

```
┌─────────────────┐     Submit()     ┌──────────────┐
│  Go Goroutine   │ ───────────────> │  Event Loop  │
│  (blocking I/O) │                  │  (non-block) │
│                 │ <─── resCh ───── │              │
│  channel.Invoke │                  │  JS Handler  │
│  channel.Stream │                  │  RPCStream   │
└─────────────────┘                  └──────────────┘
                                           │
                                     ┌─────┴─────┐
                                     │ Goja       │
                                     │ Runtime    │
                                     │ (JS eval)  │
                                     └───────────┘
```

## When Things Run Where

| Operation | Runs On | Blocking? |
|-----------|---------|-----------|
| `channel.Invoke()` | Calling goroutine | Yes (waits for response) |
| `channel.NewStream()` | Calling goroutine | Yes (waits for setup) |
| `loop.Submit(fn)` | Calling goroutine (enqueue) | No |
| Submitted `fn` | Event loop goroutine | Must not block |
| JS handler function | Event loop goroutine | Must not block |
| `RPCStream.Recv().Recv(cb)` | Event loop goroutine | No (callback-based) |
| `RPCStream.Send().Send(msg)` | Event loop goroutine | No |
| `RPCStream.Finish(err)` | Event loop goroutine | No |

## RunOnLoop Semantics

For tests using `runOnLoop(t, code, timeout)`:

1. JS code is submitted to the event loop
2. `loop.Run(ctx)` starts in a background goroutine
3. JS code executes on the loop and calls `__done()` when finished
4. `__done()` cancels the context, stopping the loop
5. **After `runOnLoop` returns, the event loop is NOT running**

For Go→JS concurrent tests, manage the loop lifecycle manually:

```go
env.loop.Submit(func() { /* setup */ })
go env.loop.Run(ctx)
// ... concurrent Go calls ...
cancel()
```
