# goja-grpc Benchmark Results

**Platform:** macOS (Apple M2 Pro, arm64)
**Go Version:** 1.25.7

## JS Bridge Overhead: Unary RPC

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| BenchmarkUnaryRPC (JS→JS) | 50,181 | 49,455 | 668 |
| BenchmarkGoDirectUnaryRPC (Go→Go) | 5,306 | 3,356 | 45 |
| **JS overhead multiplier** | **9.5x** | **14.7x** | **14.8x** |

### What the JS bridge adds

The JS path includes:
1. **goja runtime overhead**: JS parsing, object creation, property access
2. **JS↔Go proto bridge**: JS message wrappers around dynamicpb, field access through goja
3. **Promise machinery**: JS Promise construction, microtask scheduling, resolution callbacks
4. **Event loop submission**: Each RPC requires Submit() to the event loop from the benchmark goroutine

The Go path uses the same `inprocgrpc.Channel` and `dynamicpb` messages but skips all JS/goja
layers, calling `channel.Invoke()` directly.

### Where the 668 allocations come from

| Component | Estimated allocs | Description |
|-----------|-----------------|-------------|
| goja runtime | ~400 | Object creation, property access, string interning |
| Proto bridge | ~120 | JS message wrappers, field descriptors, value conversions |
| Promise/callback | ~80 | Promise objects, microtask queue, closure allocations |
| Event loop sync | ~23 | Submit channel, response channel, task envelope |
| Base inprocgrpc | ~45 | Clone, handler dispatch, stream state |

## Other Benchmarks

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| BenchmarkStatusCreate | 4,222 | 5,200 | 79 |
| BenchmarkMetadataCreate | 11,465 | 13,352 | 207 |

### Status creation

Creating a gRPC error object (`grpc.status.createError(NOT_FOUND, 'not found')`) takes ~4.2µs.
This is dominated by goja string operations and gRPC status proto construction.

### Metadata creation

Creating and populating metadata with 2 key-value pairs plus one lookup takes ~11.5µs.
The Map-based metadata implementation adds overhead over a simple Go `metadata.MD` map.

## Comparison to Tournament Results

From the [inprocgrpc tournament](../../inprocgrpc/docs/benchmark-tournament.md):

| Path | ns/op (unary) | Description |
|------|--------------|-------------|
| JS → JS (goja-grpc) | 50,181 | Full JS bridge |
| Go → Go (inprocgrpc, dynamicpb) | 5,306 | Same channel, no JS |
| Go → Go (inprocgrpc, compiled proto) | 4,138 | Tournament baseline |
| Go → Go (grpchan, compiled proto) | 3,352 | Competitor baseline |
| Go → Go (real gRPC, TCP loopback) | 71,102 | Network baseline |

**Key takeaway:** The JS bridge adds ~9.5x overhead vs pure Go, but the total
JS→JS path (50µs) is still **29% faster than real gRPC over TCP** (71µs).
This means goja-grpc achieves useful performance — JS code can make gRPC calls
faster than a production Go client hitting a loopback server.
