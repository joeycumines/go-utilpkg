# Performance Guide

## Message Creation

### Pre-load Descriptors

Always load descriptors once during initialization, not per-RPC:

```javascript
// Good: Load once
var EchoRequest = pb.messageType('testgrpc.EchoRequest');
var EchoResponse = pb.messageType('testgrpc.EchoResponse');

// Bad: Load per RPC
client.echo(request).then(function(resp) {
    var EchoResponse = pb.messageType('testgrpc.EchoResponse'); // Wasteful
});
```

### Avoid Unnecessary JSON Round-trips

Use `pb.encode()`/`pb.decode()` for wire format — it's faster than JSON:

```javascript
// Fast: Binary encoding
var bytes = pb.encode(msg);
var decoded = pb.decode(EchoRequest, bytes);

// Slower: JSON round-trip
var json = pb.toJSON(msg);
var decoded = pb.fromJSON(EchoRequest, json);
```

## Concurrent RPC Patterns

### JS: Use Promise.all for Concurrent RPCs

```javascript
var promises = [];
for (var i = 0; i < 100; i++) {
    var req = new EchoRequest();
    req.set('message', 'request-' + i);
    promises.push(client.echo(req));
}
Promise.all(promises).then(function(results) {
    // All 100 RPCs completed
});
```

### Go: Use goroutines with shared channel

```go
var wg sync.WaitGroup
for i := range 100 {
    wg.Add(1)
    go func(idx int) {
        defer wg.Done()
        req := dynamicpb.NewMessage(reqDesc)
        resp := dynamicpb.NewMessage(respDesc)
        err := channel.Invoke(ctx, method, req, resp)
        // ...
    }(i)
}
wg.Wait()
```

## Descriptor Compatibility

When Go code interacts with JS services via `inprocgrpc.Channel`, both sides
MUST use the same descriptor instances. Use `pbMod.FileResolver().FindDescriptorByName()`
to resolve descriptors from the same source as the JS server.

## Benchmarking Methodology

Run benchmarks with:

```bash
gmake bench-goja-grpc      # gRPC benchmarks
gmake bench-goja-protobuf  # Protobuf benchmarks
```

Key metrics:
- **Unary RPC latency**: ~6µs (inprocgrpc, loopback)
- **Sustained throughput**: ~300K ops/sec (10 workers)
- **Concurrent streams**: 100 bidi streams × 100 messages in ~191ms
