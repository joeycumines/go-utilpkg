# go-inprocgrpc

[![Go Reference](https://pkg.go.dev/badge/github.com/joeycumines/go-inprocgrpc.svg)](https://pkg.go.dev/github.com/joeycumines/go-inprocgrpc)

An event-loop-driven in-process gRPC channel for Go. RPCs are direct
function calls - no network I/O, no serialization overhead.

## Features

- **Event-loop-driven**: All stream state managed by [go-eventloop](https://github.com/joeycumines/go-eventloop); no mutexes on the fast path
- **Zero I/O**: No sockets, no HTTP/2 transport, no syscalls
- **Zero encoding**: Messages cloned in-memory, not serialized to bytes
- **Full gRPC semantics**: Deadlines, cancellation, metadata, trailers, status codes
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
	loop, err := eventloop.New()
	if err != nil {
		log.Fatal(err)
	}
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
            // Process request, send response
            stream.Send().Send(&pb.Response{Result: "ok"})
            stream.Finish(nil)
        })
    },
)
```

## Architecture

All RPC communication is coordinated by an [eventloop.Loop](https://github.com/joeycumines/go-eventloop).
The channel uses a callback-based internal stream core where every state
transition (message send/receive, headers, trailers, close) is a task on
the event loop goroutine. Standard gRPC handler goroutines use blocking
adapters wrapping this callback core.

See [docs/design.md](docs/design.md) for the full design.

## License

[MIT](LICENSE)
