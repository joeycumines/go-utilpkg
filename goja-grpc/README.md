# goja-grpc

[![Go Reference](https://pkg.go.dev/badge/github.com/joeycumines/goja-grpc.svg)](https://pkg.go.dev/github.com/joeycumines/goja-grpc)

JavaScript gRPC module for the [goja](https://github.com/dop251/goja) runtime. Build gRPC clients and servers in JavaScript, running entirely in-process within Go via [inprocgrpc](https://github.com/joeycumines/go-inprocgrpc).

## Features

- **All RPC types**: Unary, server-streaming, client-streaming, bidirectional streaming
- **Promise-based**: Client calls return promises; server handlers can return promises
- **Event-loop native**: Handlers run on the event loop — thread-safe with goja runtime
- **AbortSignal**: Cancel in-flight RPCs via `AbortController.signal`
- **Metadata**: Send and receive gRPC metadata
- **Status codes**: Full gRPC status code support with typed error objects
- **Go↔JS interop**: Go clients can call JS servers, JS clients can call Go servers
- **require() integration**: Standard goja module loading via `require('grpc')`

## Installation

```sh
go get github.com/joeycumines/goja-grpc
```

## Dependencies

This module requires three companion packages:

- [go-eventloop](https://github.com/joeycumines/go-eventloop) — Event loop runtime
- [goja-protobuf](https://github.com/joeycumines/goja-protobuf) — Protobuf message support
- [go-inprocgrpc](https://github.com/joeycumines/go-inprocgrpc) — In-process gRPC channel

## Quick Start

```go
package main

import (
    "github.com/dop251/goja"
    "github.com/dop251/goja_nodejs/require"
    eventloop "github.com/joeycumines/go-eventloop"
    gojaeventloop "github.com/joeycumines/goja-eventloop"
    gojaprotobuf "github.com/joeycumines/goja-protobuf"
    inprocgrpc "github.com/joeycumines/go-inprocgrpc"
    gojagrpc "github.com/joeycumines/goja-grpc"
)

func main() {
    // Create event loop and runtime
    loop, _ := eventloop.New()
    defer loop.Close()
    rt := goja.New()

    // Set up require() registry
    registry := require.NewRegistry()
    registry.RegisterNativeModule("protobuf", gojaprotobuf.Require())
    adapter := gojaeventloop.NewAdapter(loop, rt)
    registry.Enable(rt)

    // Create in-process gRPC channel
    channel := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))
    pbMod, _ := gojaprotobuf.New(rt)

    registry.RegisterNativeModule("grpc", gojagrpc.Require(
        gojagrpc.WithChannel(channel),
        gojagrpc.WithProtobuf(pbMod),
        gojagrpc.WithAdapter(adapter),
    ))

    // Run JavaScript on the event loop
    loop.Submit(func() {
        rt.RunString(`
            const pb = require('protobuf');
            const grpc = require('grpc');

            // Load proto descriptors (pre-compiled)
            pb.loadDescriptorSet(descriptorBytes);

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
            const resp = await client.echo(req);
            console.log(resp.get('message')); // "hello"
        `)
    })
}
```

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
call.send(item1);
call.send(item2);
call.closeSend();
const response = await call.response;
```

#### Bidi-Streaming RPC

```javascript
const stream = await client.chat();
stream.send(msg1);
stream.send(msg2);
stream.closeSend();
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

### Metadata

```javascript
const { metadata } = require('grpc');

const md = metadata.create();
md.set('key', 'value');
md.get('key');              // 'value'
md.getAll('key');           // ['value']
md.delete('key');
md.forEach((key, values) => { ... });
md.toObject();              // { key: ['value'] }
```

## Architecture

```
┌────────────────────────────────────────────────┐
│                  Event Loop                     │
│  ┌──────────┐  ┌────────────┐  ┌────────────┐ │
│  │ JS Client│  │  JS Server │  │ Go Handlers│ │
│  └────┬─────┘  └──────┬─────┘  └──────┬─────┘ │
│       │               │               │        │
│  ┌────▼───────────────▼───────────────▼─────┐  │
│  │          inprocgrpc.Channel               │  │
│  │   (in-process gRPC, no network I/O)       │  │
│  └───────────────────────────────────────────┘  │
│                                                  │
│  ┌───────────────────────────────────────────┐  │
│  │         goja-protobuf (encode/decode)      │  │
│  └───────────────────────────────────────────┘  │
└────────────────────────────────────────────────┘
```

- **No network I/O** — all RPCs are in-process function calls
- **Thread-safe** — server handlers run on the event loop goroutine
- **Promise-based** — client operations return promises resolved on the event loop
- **Interoperable** — Go and JS can freely mix as clients and servers

## License

[MIT](LICENSE)
