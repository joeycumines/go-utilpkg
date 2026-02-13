# goja-grpc Design Document

## Overview

`goja-grpc` provides a JavaScript gRPC module for the [goja](https://github.com/dop251/goja) runtime.
It bridges `goja-protobuf` message handling with `inprocgrpc` channels, enabling JavaScript code to act
as both gRPC clients and servers within a Go process via the event loop.

## Go API

### Module

```go
type Module struct {
    runtime  *goja.Runtime
    channel  *inprocgrpc.Channel
    protobuf *gojaprotobuf.Module
    adapter  *gojaeventloop.Adapter
}

func New(runtime *goja.Runtime, opts ...Option) (*Module, error)
func (m *Module) Runtime() *goja.Runtime
```

- `New` panics if `runtime` is nil (programming error).
- All three dependencies (channel, protobuf, adapter) are required; missing any returns an error.

### Options

```go
type Option interface { applyOption(*moduleOptions) error }

func WithChannel(ch *inprocgrpc.Channel) Option   // required, nil → error
func WithProtobuf(pb *gojaprotobuf.Module) Option  // required, nil → error
func WithAdapter(a *gojaeventloop.Adapter) Option   // required, nil → error
```

### Registration

```go
func Require(opts ...Option) require.ModuleLoader
```

Returns a module loader. The integrator registers it under their chosen name:
`registry.RegisterNativeModule("grpc", gojagrpc.Require(opts...))`.

## JS API

```javascript
const grpc = require('grpc');

// Status codes object
grpc.status.OK           // 0
grpc.status.CANCELLED    // 1
grpc.status.UNKNOWN      // 2
// ... all 17 standard gRPC codes
grpc.status.createError(code, message)  // → GrpcError

// Metadata factory
grpc.metadata.create()   // → Metadata wrapper
```

### Client API

```javascript
const client = grpc.createClient('my.package.ServiceName');

// Unary
const response = await client.myMethod(request);
const response = await client.myMethod(request, { metadata, signal });

// Server streaming
const stream = client.myServerStreamMethod(request);
while (true) {
    const { value, done } = await stream.recv();
    if (done) break;
    // process value
}

// Client streaming
const call = client.myClientStreamMethod();
call.send(msg1);
call.send(msg2);
call.closeSend();
const response = await call.response;

// Bidirectional streaming
const bidi = client.myBidiMethod();
bidi.send(msg);
const { value, done } = await bidi.recv();
bidi.closeSend();
```

### Server API

```javascript
const server = grpc.createServer();

server.addService('my.package.ServiceName', {
    myMethod: (request, call) => {
        // Unary: return response or Promise
        return { field: 'value' };
    },
    myServerStreamMethod: async (request, call) => {
        call.send(response1);
        call.send(response2);
        // streaming handler ends when function returns
    },
    myClientStreamMethod: async (call) => {
        while (true) {
            const { value, done } = await call.recv();
            if (done) break;
        }
        return response;
    },
    myBidiMethod: async (call) => {
        // interleave recv() and send()
    }
});

server.start();  // registers StreamHandlerFunc on the channel
```

### Status Codes

| Name                | Code |
|---------------------|------|
| OK                  | 0    |
| CANCELLED           | 1    |
| UNKNOWN             | 2    |
| INVALID_ARGUMENT    | 3    |
| DEADLINE_EXCEEDED   | 4    |
| NOT_FOUND           | 5    |
| ALREADY_EXISTS      | 6    |
| PERMISSION_DENIED   | 7    |
| RESOURCE_EXHAUSTED  | 8    |
| FAILED_PRECONDITION | 9    |
| ABORTED             | 10   |
| OUT_OF_RANGE        | 11   |
| UNIMPLEMENTED       | 12   |
| INTERNAL            | 13   |
| UNAVAILABLE         | 14   |
| DATA_LOSS           | 15   |
| UNAUTHENTICATED     | 16   |

### GrpcError

```javascript
const err = grpc.status.createError(grpc.status.NOT_FOUND, 'resource not found');
err.code     // 5
err.message  // 'resource not found'
err.name     // 'GrpcError'
```

### Metadata

```javascript
const md = grpc.metadata.create();
md.set('key', 'value');
md.get('key');           // 'value'
md.getAll('key');        // ['value']
md.delete('key');
md.forEach((value, key) => { ... });
md.toObject();           // { key: ['value'] }
```

## Message Flow

### Unary RPC (JS Client → JS Server)

```
JS Client                    Event Loop              JS Server Handler
    |                            |                         |
    |-- createClient(svc) ------>|                         |
    |   client.method(req)       |                         |
    |-- encode(req) ------------>|                         |
    |   submit to loop           |                         |
    |                            |-- StreamHandlerFunc --->|
    |                            |   RPCStream.Recv()      |
    |                            |   decode(req)           |
    |                            |   call handler(req)     |
    |                            |<-- handler returns -----|
    |                            |   encode(resp)          |
    |                            |   RPCStream.Send()      |
    |                            |   RPCStream.Finish()    |
    |<-- Promise resolves -------|                         |
    |   (decoded response)       |                         |
```

### Server Streaming (JS Client → JS Server)

```
JS Client                    Event Loop              JS Server Handler
    |                            |                         |
    |-- method(req) ------------>|                         |
    |                            |-- handler(req, call) -->|
    |                            |                         |-- call.send(r1)
    |<-- recv() resolves --------|<-- RPCStream.Send() ----|
    |                            |                         |-- call.send(r2)
    |<-- recv() resolves --------|<-- RPCStream.Send() ----|
    |                            |                         |-- return
    |                            |   RPCStream.Finish()    |
    |<-- recv() done:true -------|                         |
```

## Dependencies

- `github.com/dop251/goja` — JavaScript runtime
- `github.com/dop251/goja_nodejs/require` — CommonJS require() support
- `github.com/joeycumines/go-inprocgrpc` — In-process gRPC channels
- `github.com/joeycumines/goja-protobuf` — Protobuf message handling for goja
- `github.com/joeycumines/goja-eventloop` — Event loop adapter for goja
- `google.golang.org/grpc` — gRPC framework (status codes, metadata)
