# Stress Test Results

## inprocgrpc Module

### Test: 1000 Concurrent Unary RPCs (T240)

- **Configuration**: 1000 goroutines, each making one unary RPC
- **Result**: All 1000 RPCs complete successfully
- **Latency**: avg ~6µs/rpc (total ~6ms)
- **Failures**: 0
- **Races**: 0 (verified with `-race`)

### Test: 100 Concurrent Bidi Streams (T241)

- **Configuration**: 100 concurrent bidirectional streams, each exchanging 100 messages
- **Result**: 10,000 total messages exchanged
- **Latency**: ~191ms total
- **Failures**: 0
- **Races**: 0

### Test: Sustained Throughput (T242)

- **Configuration**: 10 worker goroutines, continuous RPCs for 5 seconds
- **Result**: ~1.5M operations completed
- **Throughput**: ~300K ops/sec
- **Failures**: 0
- **Goroutine delta**: +1 (within tolerance)

### Test: Goroutine Leak Check (T245)

- **Configuration**: 10 cycles × 100 RPCs per cycle, measure goroutine residue
- **Result**: Goroutine delta within tolerance (≤30)
- **Assessment**: No goroutine leaks detected

### Test: Heap Allocation Check (T246)

- **Configuration**: 10,000 unary RPCs after warmup, measure heap growth
- **Result**: HeapInuse stays within 10MB tolerance
- **Assessment**: No heap leaks detected

## goja-grpc Module

### Test: JS Client 100 Concurrent RPCs (T243)

- **Configuration**: Pure JS: 100 concurrent RPCs via `Promise.all`, JS echo server
- **Result**: All 100 promises resolve correctly
- **Server handler**: `function(request, call)` signature (first arg is request)
- **Failures**: 0

### Test: Go Client → JS Server 100 Concurrent RPCs (T244)

- **Configuration**: 100 Go goroutines calling JS echo server via `channel.Invoke()`
- **Key**: Must use `dynamicpb.Message` with descriptors from `pbMod.FileResolver()` (same instances as JS server)
- **Result**: All 100 RPCs succeed with correct responses
- **Event loop management**: Manual lifecycle (Submit → Run → cancel) required for concurrent Go→JS

### Test: Goroutine Leak Check (T245)

- **Configuration**: 5 batches × 20 RPCs, create/destroy environments
- **Result**: Goroutine delta = 0
- **Assessment**: No goroutine leaks

### Test: Heap Allocation Check (T246)

- **Configuration**: 50 warmup + 500 RPCs, check heap after
- **Result**: Heap metrics logged, no significant growth

## Key Findings

### Descriptor Compatibility (Critical)

When Go code sends `dynamicpb.Message` to a JS server via `inprocgrpc.Channel`, the message descriptors **must** be the same instances used by the JS server. Different `protodesc.NewFile()` calls create structurally identical but referentially distinct descriptors that `proto.Merge` rejects with `descriptor mismatch` panic.

**Solution**: Resolve descriptors from `pbMod.FileResolver().FindDescriptorByName()`.

### Event Loop Lifecycle

The `runOnLoop()` test helper stops the event loop after `__done()`. For Go→JS concurrent tests, manage the loop lifecycle manually:

```go
env.loop.Submit(func() { /* setup JS server */ })
go env.loop.Run(ctx)
// ... Go RPCs ...
cancel() // stop loop
```

### Platform Results

All stress tests pass on all three platforms:
- **macOS**: ✅ (local `gmake`)
- **Linux**: ✅ (Docker container)
- **Windows**: ✅ (SSH to `moo`)
