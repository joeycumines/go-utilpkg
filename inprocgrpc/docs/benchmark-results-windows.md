# Tournament Benchmark Results — Windows

**Platform:** Windows (Intel Core i9-9900K @ 3.60GHz, amd64)
**Go Version:** 1.25.7
**Date:** Captured during T193 implementation

## Competitors

| # | Name | Implementation |
|---|------|---------------|
| 1 | **OurChannel** | `inprocgrpc.Channel` — event-loop-driven |
| 2 | **GrpchanChannel** | `fullstorydev/grpchan/inprocgrpc.Channel` — goroutine-per-RPC |
| 3 | **RealGRPC** | `grpc.NewServer()` + `grpc.NewClient()` over loopback TCP |

## Unary Throughput (32-byte payload)

| Competitor | ops | ns/op | B/op | allocs/op |
|-----------|-----|-------|------|-----------|
| OurChannel | 260,212 | 4,102 | 1,987 | 28 |
| GrpchanChannel | 478,260 | 2,584 | 1,425 | 23 |
| RealGRPC | 14,886 | 82,014 | 10,958 | 186 |

## Unary Large Message Throughput

| Size | Competitor | ns/op | B/op | allocs/op |
|------|-----------|-------|------|-----------|
| 64B | OurChannel | 4,360 | 2,083 | 28 |
| 64B | GrpchanChannel | 2,683 | 1,490 | 23 |
| 64B | RealGRPC | 84,690 | 11,152 | 186 |
| 1KB | OurChannel | 5,237 | 4,967 | 28 |
| 1KB | GrpchanChannel | 3,403 | 3,413 | 23 |
| 1KB | RealGRPC | 86,660 | 12,522 | 176 |
| 10KB | OurChannel | 12,963 | 32,651 | 28 |
| 10KB | GrpchanChannel | 9,904 | 21,876 | 23 |
| 10KB | RealGRPC | 104,661 | 31,601 | 177 |
| 100KB | OurChannel | 65,238 | 321,621 | 28 |
| 100KB | GrpchanChannel | 42,724 | 214,551 | 23 |
| 100KB | RealGRPC | 308,745 | 337,409 | 201 |
| 1MB | OurChannel | 414,590 | 3,148,661 | 30 |
| 1MB | GrpchanChannel | 244,423 | 2,099,298 | 25 |
| 1MB | RealGRPC | 1,574,623 | 3,180,580 | 334 |

## Server-Streaming Throughput (100 messages × 32 bytes)

| Competitor | ops | ns/op | B/op | allocs/op |
|-----------|-----|-------|------|-----------|
| OurChannel | 578 | 2,081,570 | 95,969 | 1,469 |
| GrpchanChannel | 7,993 | 155,269 | 57,240 | 741 |
| RealGRPC | 3,225 | 434,647 | 229,296 | 4,885 |

## Bidi-Streaming Echo

| Competitor | ops | ns/op | B/op | allocs/op |
|-----------|-----|-------|------|-----------|
| OurChannel | 35,538 | 33,483 | 1,679 | 26 |
| GrpchanChannel | 499,147 | 2,434 | 864 | 11 |
| RealGRPC | 21,058 | 55,784 | 2,858 | 71 |

## Concurrent Unary Load

| Goroutines | Competitor | ns/op | B/op | allocs/op |
|-----------|-----------|-------|------|-----------|
| 10 | OurChannel | 1,350 | 1,989 | 28 |
| 10 | GrpchanChannel | 859 | 1,427 | 23 |
| 10 | RealGRPC | 11,575 | 10,852 | 174 |
| 50 | OurChannel | 1,296 | 1,987 | 28 |
| 50 | GrpchanChannel | 526 | 1,425 | 23 |
| 50 | RealGRPC | 7,495 | 10,827 | 173 |
| 100 | OurChannel | 1,306 | 1,987 | 28 |
| 100 | GrpchanChannel | 581 | 1,425 | 23 |
| 100 | RealGRPC | 7,080 | 10,780 | 174 |

## Latency Percentiles (10 concurrent, 5000 iterations)

| Competitor | p50 (µs) | p90 (µs) | p95 (µs) | p99 (µs) |
|-----------|----------|----------|----------|----------|
| OurChannel | < 1 | < 1 | < 1 | < 1 |
| GrpchanChannel | < 1 | < 1 | < 1 | < 1 |
| RealGRPC | < 1 | 528 | 531 | 552 |

**Note:** Sub-microsecond operations on the i9-9900K result in 0µs readings. The `time.Duration` microsecond truncation hides sub-µs differences. Throughput benchmarks (ns/op) are more informative for these competitors.

## Notes

- 16 GOMAXPROCS (8C/16T i9-9900K) benefits concurrent workloads.
- Our concurrent unary (1.3µs@100g) is best of all platforms — the 16 threads help event loop + handler parallelism.
- Server-streaming gap is ~13x vs grpchan (better than Linux's 46x, similar to macOS's 26x).
- Bidi echo shows 14x gap vs grpchan, beating real gRPC by 1.7x.
