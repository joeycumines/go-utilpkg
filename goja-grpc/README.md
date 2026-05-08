# goja-grpc

[![Go Reference](https://pkg.go.dev/badge/github.com/joeycumines/goja-grpc.svg)](https://pkg.go.dev/github.com/joeycumines/goja-grpc)

JavaScript gRPC module for the [goja](https://github.com/joeycumines/goja) runtime. JavaScript servers and the default client channel run in-process through [inprocgrpc](https://github.com/joeycumines/go-inprocgrpc); clients may instead use `grpc.dial` for an external gRPC endpoint.

## Features

- **All RPC types**: Unary, server-streaming, client-streaming, bidirectional streaming
- **Promise-based**: Client calls return promises; server handlers can return promises
- **Owner-affine**: Handlers, Goja values, and Promise settlement run through
  the exact bound adapter's serialized logical owner
- **AbortSignal**: Cancel in-flight RPCs via `AbortController.signal`
- **Metadata**: Send and receive gRPC metadata
- **Status codes**: Full gRPC status code support with typed error objects
- **Go↔JS interop**: Go clients can call JS servers, JS clients can call Go servers
- **Optional external clients**: `grpc.dial` supplies a network-backed client channel
- **require() integration**: Standard goja module loading via `require('grpc')`

## Installation

```sh
go get github.com/joeycumines/goja-grpc
```

## Dependencies

This module requires four companion packages:

- [go-eventloop](https://github.com/joeycumines/go-eventloop) — Event loop runtime
- [goja-eventloop](https://github.com/joeycumines/goja-eventloop) — Goja ownership and scheduling adapter
- [goja-protobuf](https://github.com/joeycumines/goja-protobuf) — Protobuf message support
- [go-inprocgrpc](https://github.com/joeycumines/go-inprocgrpc) — In-process gRPC channel

## Quick Start

The example expects `api.protoset` to contain `mypackage.MyService` and its
request/response messages. Generate it from the service's proto sources with
`protoc --descriptor_set_out=api.protoset --include_imports api.proto`.

```go
package main

import (
    "context"
    "errors"
    "log"
    "os"
    "time"

    "github.com/joeycumines/goja"
    "github.com/joeycumines/goja_nodejs/require"
    eventloop "github.com/joeycumines/go-eventloop"
    gojaeventloop "github.com/joeycumines/goja-eventloop"
    gojaprotobuf "github.com/joeycumines/goja-protobuf"
    inprocgrpc "github.com/joeycumines/go-inprocgrpc"
    gojagrpc "github.com/joeycumines/goja-grpc"
)

func main() {
    // Create event loop and runtime
    loop := eventloop.New()
    defer loop.Close()
    rt := goja.New()

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := rt.Set("__done", func(message string) {
        log.Print(message)
        cancel()
    }); err != nil {
        log.Fatal(err)
    }

    // Set up require() registry
    registry := require.NewRegistry()
    adapter, err := gojaeventloop.New(loop, rt)
    if err != nil {
        log.Fatal(err)
    }
    if err := adapter.Bind(); err != nil {
        log.Fatal(err)
    }

    // Create in-process gRPC channel
    channel := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))
    pbMod, err := gojaprotobuf.New(rt)
    if err != nil {
        log.Fatal(err)
    }
    descriptorBytes, err := os.ReadFile("api.protoset")
    if err != nil {
        log.Fatal(err)
    }
    if _, err := pbMod.LoadDescriptorSetBytes(descriptorBytes); err != nil {
        log.Fatal(err)
    }
    pbExports := rt.NewObject()
    if err := pbMod.SetupExports(pbExports); err != nil {
        log.Fatal(err)
    }
    if err := rt.Set("pb", pbExports); err != nil {
        log.Fatal(err)
    }

    registry.RegisterNativeModule("grpc", gojagrpc.Require(
        gojagrpc.WithChannel(channel),
        gojagrpc.WithProtobuf(pbMod),
        gojagrpc.WithAdapter(adapter),
    ))
    registry.Enable(rt)

    // Run JavaScript through the adapter's logical owner.
    if err := adapter.Submit(func(owner *goja.Runtime) {
        if _, err := owner.RunString(`
            const grpc = require('grpc');

            // Register server handlers
            const server = grpc.createServer();
            server.addService('mypackage.MyService', {
                echo(request) {
                    const resp = new (pb.messageType('mypackage.EchoResponse'))();
                    resp.set('message', request.get('message'));
                    return resp;
                }
            });
            server.start();

            // Create client and make RPC
            const client = grpc.createClient('mypackage.MyService');
            const req = new (pb.messageType('mypackage.EchoRequest'))();
            req.set('message', 'hello');
            client.echo(req).then(resp => {
                __done(resp.get('message')); // "hello"
            });
        `); err != nil {
            panic(err)
        }
    }); err != nil {
        log.Fatal(err)
    }

    if err := loop.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
        log.Fatal(err)
    }
}
```

## Runtime Ownership

Goja is not goroutine-safe. `goja-grpc` performs all Goja work under the exact
bound adapter's serialized logical callback-owner role. The underlying event
loop may temporarily transfer that role to an isolated callback worker, so a
fixed physical goroutine is not part of the contract. Transport workers keep
Go-only data and use `Adapter.Submit` or an adapter Promise settler to hand work
back to the owner.

Initial runtime and module setup may access Goja directly only while the caller
has exclusive ownership before the loop starts. After callbacks may execute,
external callers must use `Adapter.Submit` and must never access the runtime or
Goja values concurrently. `Adapter.NewPromise` is owner-only; the settler it
returns may be called from another goroutine, but its result callback and native
settlement execute under the logical owner.

## JavaScript API

### Client

```javascript
const grpc = require('grpc');
const client = grpc.createClient('mypackage.MyService');
```

#### Unary RPC

```javascript
const response = await client.myMethod(request);
```

#### Server-Streaming RPC

```javascript
const stream = await client.listItems(request);
while (true) {
    const { value, done } = await stream.recv();
    if (done) break;
    console.log(value.get('name'));
}
```

#### Client-Streaming RPC

```javascript
const call = await client.upload();
await call.send(item1);
await call.send(item2);
await call.closeSend();
const response = await call.response;
```

#### Bidi-Streaming RPC

```javascript
const stream = await client.chat();
await stream.send(msg1);
await stream.send(msg2);
await stream.closeSend();
while (true) {
    const { value, done } = await stream.recv();
    if (done) break;
}
```

#### Call Options

```javascript
// AbortSignal cancellation
const ac = new AbortController();
const response = await client.myMethod(request, { signal: ac.signal });
ac.abort(); // cancels with CANCELLED status

// Metadata
const md = grpc.metadata.create();
md.set('authorization', 'Bearer token');
const response = await client.myMethod(request, { metadata: md });
```

Every call shape accepts `timeoutMs` and an eventloop `AbortSignal`. Reflection
operations accept the same options:

```javascript
const reflection = grpc.createReflectionClient();
const services = await reflection.listServices({ signal, timeoutMs: 5000 });
const service = await reflection.describeService('mypackage.MyService', {
    signal,
    timeoutMs: 5000,
});
```

`send()` admission is bounded and never blocks the JavaScript owner. Await each
send to apply backpressure. A second concurrent `recv()` is rejected; call it
again only after the prior receive settles.

### Server

```javascript
const server = grpc.createServer();

server.addService('mypackage.MyService', {
    // Unary: return response or Promise
    echo(request, call) {
        const resp = new EchoResponse();
        resp.set('message', request.get('message'));
        return resp;
    },

    // Server-streaming: use call.send()
    listItems(request, call) {
        call.send(item1);
        call.send(item2);
        // completion on return
    },

    // Client-streaming: use call.recv()
    upload(call) {
        return new Promise((resolve) => {
            function read() {
                call.recv().then(({ value, done }) => {
                    if (done) { resolve(response); return; }
                    // process value
                    read();
                });
            }
            read();
        });
    },

    // Bidi: use call.send() + call.recv()
    chat(call) {
        return new Promise((resolve) => {
            function read() {
                call.recv().then(({ value, done }) => {
                    if (done) { resolve(); return; }
                    call.send(echoBack(value));
                    read();
                });
            }
            read();
        });
    }
});

server.start();
```

### Status Codes

```javascript
const { status } = require('grpc');

status.OK               // 0
status.CANCELLED         // 1
status.UNKNOWN           // 2
status.INVALID_ARGUMENT  // 3
status.DEADLINE_EXCEEDED // 4
status.NOT_FOUND         // 5
status.ALREADY_EXISTS    // 6
status.PERMISSION_DENIED // 7
// ... all 17 standard codes

// Create and throw gRPC errors
const err = status.createError(status.NOT_FOUND, 'item not found');
throw err; // in a server handler
```

Every supplied detail must be a canonical protobuf wrapper from the configured
runtime. Invalid details are rejected instead of being omitted. Private detail
identity is retained without exposing a mutable branding property.

### Metadata

```javascript
const { metadata } = require('grpc');

const md = metadata.create();
md.set('key', 'value');
md.get('key');              // 'value'
md.getAll('key');           // ['value']
md.delete('key');
md.forEach((value, key) => { ... });
md.toObject();              // { key: ['value'] }
```

Incoming `call.requestHeader` metadata is an isolated read-only view. Module
shutdown is explicit and idempotent:

```javascript
grpc.close();
```

Closing the module cancels active client calls, active JavaScript server calls,
and reflection requests, and closes connections created by `grpc.dial`.
Independently supplied Go connections remain caller-owned. A dial result may
also be closed earlier with its own idempotent `close()` method. Event-loop
terminal cleanup closes the module as well. After closure, new client, server,
reflection, dial, and export installation admissions fail; repeated `close()`
calls remain harmless.

## Architecture

```
┌────────────────────────────────────────────────┐
│                  Event Loop                     │
│  ┌──────────┐  ┌────────────┐  ┌────────────┐ │
│  │ JS Client│  │  JS Server │  │ Go Handlers│ │
│  └────┬─────┘  └──────┬─────┘  └──────┬─────┘ │
│       │               │               │        │
│  ┌────▼───────────────▼───────────────▼─────┐  │
│  │ default: inprocgrpc.Channel               │  │
│  │ client option: grpc.dial channel          │  │
│  └───────────────────────────────────────────┘  │
│                                                  │
│  ┌───────────────────────────────────────────┐  │
│  │         goja-protobuf (encode/decode)      │  │
│  └───────────────────────────────────────────┘  │
└────────────────────────────────────────────────┘
```

- **In-process by default** — JavaScript servers and the default client channel
  use `inprocgrpc`; `grpc.dial` opts a client into network I/O
- **Owner-safe** — server handlers and all Goja work run under the bound
  adapter's logical owner
- **Promise-based** — client operations return promises settled under the bound
  adapter's logical owner
- **Bounded lifecycle** — streaming admission is bounded; cancellation,
  deadlines, module shutdown, and transport completion converge on one
  exactly-once terminal outcome
- **Interoperable** — Go and JS can freely mix as clients and servers
- **Canonical protobuf identity** — generated and dynamic messages retain the
  exact runtime registry identity, including unknown fields and status details

## License

[MIT](LICENSE)
