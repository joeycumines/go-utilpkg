# go-inprocgrpc

[![Go Reference](https://pkg.go.dev/badge/github.com/joeycumines/go-inprocgrpc.svg)](https://pkg.go.dev/github.com/joeycumines/go-inprocgrpc)

An event-loop-driven in-process gRPC channel for Go. RPCs are direct
function calls - no network I/O, no serialization overhead.

## Features

- **Event-loop-driven**: Ordinary stream data and metadata publication is
  owned by [go-eventloop](https://github.com/joeycumines/go-eventloop), with a
  synchronized terminal arbiter for cancellation and scheduler loss
- **Zero I/O**: No sockets, no HTTP/2 transport, no syscalls
- **Zero encoding**: Messages cloned in-memory, not serialized to bytes
- **gRPC-compatible call surface**: Deadlines, cancellation, metadata,
  trailers, status codes, stats handlers, and interceptors for the supported
  in-process transport profile
- **Bounded streaming**: Finite per-direction queues apply backpressure instead of
  retaining an unrestricted burst
- **Deterministic completion**: One first-terminal-wins claim per RPC;
  terminal observation is stable before final resource release, and scheduler
  loss recovers exclusively owned accepted data without leaking waiters
- **Context isolation**: Server handlers cannot access client-side context values
- **Stats handlers**: Client and server stats handler support
- **Interceptors**: Server-side unary and stream interceptors
- **Concurrent RPCs**: Multiple in-flight RPCs on a single channel
- **Pluggable cloning**: Custom `Cloner` implementations for non-proto messages
- **Extensible**: Callback-based stream handlers for integration with JS runtimes (Goja)

## Install

```bash
go get github.com/joeycumines/go-inprocgrpc
```

## Quick Start

```go
package main

import (
	"context"
	"log"

	eventloop "github.com/joeycumines/go-eventloop"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	pb "your/protobuf/package"
)

func main() {
	// Create and start an event loop
	loop := eventloop.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)

	// Create the in-process channel
	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))

	// Register your gRPC service (same as grpc.Server)
	pb.RegisterYourServiceServer(ch, &yourServiceImpl{})

	// Use as a grpc.ClientConnInterface - no Dial needed
	client := pb.NewYourServiceClient(ch)

	resp, err := client.YourMethod(context.Background(), &pb.YourRequest{})
	if err != nil {
		log.Fatal(err)
	}
	log.Println(resp)
}
```

### Configuration

Use functional options to configure the channel:

```go
ch := inprocgrpc.NewChannel(
    inprocgrpc.WithLoop(loop),
    inprocgrpc.WithServerUnaryInterceptor(myUnaryInterceptor),
    inprocgrpc.WithServerStreamInterceptor(myStreamInterceptor),
    inprocgrpc.WithClientStatsHandler(myStatsHandler),
    inprocgrpc.WithCloner(myCloner),
    inprocgrpc.WithStreamBuffer(16),
)
```

### Accessing Client Context from Server

```go
func (s *server) MyMethod(ctx context.Context, req *pb.Request) (*pb.Response, error) {
	if clientCtx := inprocgrpc.ClientContext(ctx); clientCtx != nil {
		// Access the original client context (e.g., for tracing propagation)
		_ = clientCtx
	}
	return &pb.Response{}, nil
}
```

### Callback-Based Stream Handlers

For non-blocking integration (e.g., JS runtimes), register callback-based
stream handlers that run directly on the event loop goroutine:

```go
ch.RegisterStreamHandler("/mypackage.MyService/MyMethod",
    func(ctx context.Context, stream *inprocgrpc.RPCStream) {
        // Runs ON the event loop goroutine - no blocking allowed
        stream.Recv().Recv(func(msg any, err error) {
            if err != nil {
                if err == io.EOF {
                    stream.Finish(nil)
                } else {
                    stream.Abort(err)
                }
                return
            }
            if err := stream.Send().Send(&pb.Response{Result: "ok"}); err != nil {
                stream.Abort(err)
                return
            }
            stream.Finish(nil)
        })
    },
)
```

#### Unregistering handlers and services

Registrations can be retired so the same service or method can be registered
again (delete/recreate lifecycles). Removal is idempotent — a name that is not
registered is a silent no-op — and atomic with respect to dispatch:

```go
// Remove a generated service registration.
ch.UnregisterService("mypackage.MyService")

// Remove an event-loop-native handler.
ch.UnregisterStreamHandler("/mypackage.MyService/MyMethod")

// Or remove both atomically. For goja-grpc-style servers whose MethodDesc
// carries a nil Handler, removing both in one batch is required: leaving the
// service entry behind after its stream handler is gone would leave unary
// dispatch dereferencing that nil Handler — a recovered panic surfacing as an
// Internal error instead of a clean Unimplemented.
ch.UnregisterBatch(inprocgrpc.UnregistrationBatch{
    Services:       []string{"mypackage.MyService"},
    StreamHandlers: []string{"/mypackage.MyService/MyMethod"},
})
```

## Architecture

All RPC communication is coordinated by an [eventloop.Loop](https://github.com/joeycumines/go-eventloop).
The channel uses a callback-based internal stream core where ordinary message
and metadata transitions are tasks on the event loop goroutine. A synchronized
first-terminal-wins arbiter admits handler results, cancellation, and scheduler
loss from their originating goroutines. Standard gRPC handler goroutines use
blocking adapters around the callback core. Blocking sends wait for finite
queue credit; callback sends are non-blocking and return `ResourceExhausted`
when credit is unavailable. After the loop's terminal `Done` signal proves
owner callbacks cannot execute, recovery takes exclusive ownership of the
state. It preserves already accepted buffered responses and transferred
prepared unary material for client draining, and releases inaccessible
requests and waiters without invoking callbacks off-owner. The RPC's stable
terminal signal identifies the immutable winner; its full `Done` signal waits
for accepted deliveries, stats callbacks, and final resource release.

See [docs/design.md](docs/design.md) for the full design.

## License

[MIT](LICENSE)
