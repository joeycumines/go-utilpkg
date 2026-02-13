# Migration from grpchan/inprocgrpc

This guide helps users of `github.com/fullstorydev/grpchan/inprocgrpc` migrate to
`github.com/joeycumines/go-inprocgrpc`.

## Key Differences

| Feature | grpchan/inprocgrpc | go-inprocgrpc |
|---------|-------------------|---------------|
| Event loop | None (goroutine per RPC) | Required (`eventloop.Loop`) |
| Handler model | Blocking (standard gRPC) | Non-blocking (`StreamHandlerFunc`) |
| Channel creation | `inprocgrpc.NewChannel()` | `inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))` |
| Message cloning | Proto clone | `ProtoCloner` (configurable) |
| Stats handlers | Not supported | Client + server stats |

## Migration Steps

### 1. Create an Event Loop

```go
import eventloop "github.com/joeycumines/go-eventloop"

loop, err := eventloop.New()
if err != nil { ... }

// Run loop in background
ctx, cancel := context.WithCancel(context.Background())
go loop.Run(ctx)
defer cancel()
```

### 2. Create Channel

```go
// Before (grpchan)
ch := inprocgrpc.NewChannel(svc)

// After (go-inprocgrpc)
ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))
```

### 3. Register Services

Both use `RegisterService` with the same `grpc.ServiceDesc`:

```go
ch.RegisterService(&myServiceDesc, myServer)
```

For event-loop-native handlers, use `RegisterStreamHandler`:

```go
ch.RegisterStreamHandler("/pkg.Service/Method", func(ctx context.Context, s *inprocgrpc.RPCStream) {
    s.Recv().Recv(func(msg any, err error) {
        if err != nil {
            s.Finish(err)
            return
        }
        s.Send().Send(response)
        s.Finish(nil)
    })
})
```

### 4. Invoke/NewStream

The `Invoke` and `NewStream` signatures are identical:

```go
err := ch.Invoke(ctx, method, req, resp, opts...)
stream, err := ch.NewStream(ctx, desc, method, opts...)
```

### 5. Cloner Configuration

go-inprocgrpc uses `ProtoCloner` by default. For custom cloning:

```go
ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop), inprocgrpc.WithCloner(myCloner))
```
