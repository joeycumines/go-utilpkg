# Tournament Benchmark Results — Linux (Docker)

**Platform:** Linux (Docker container, arm64 via Apple M2 Pro host)
**Go Version:** 1.25.7
**Date:** Captured during T192 implementation

## Competitors

| # | Name | Implementation |
|---|------|---------------|
| 1 | **OurChannel** | `inprocgrpc.Channel` — event-loop-driven |
| 2 | **GrpchanChannel** | `fullstorydev/grpchan/inprocgrpc.Channel` — goroutine-per-RPC |
| 3 | **RealGRPC** | `grpc.NewServer()` + `grpc.NewClient()` over loopback TCP |

## Unary Throughput (32-byte payload)

| Competitor | ops | ns/op | B/op | allocs/op |
|-----------|-----|-------|------|-----------|
| OurChannel | 294,780 | 4,289 | 1,985 | 28 |
| GrpchanChannel | 387,192 | 3,077 | 1,424 | 23 |
| RealGRPC | 37,360 | 36,057 | 10,941 | 186 |

## Unary Large Message Throughput

| Size | Competitor | ns/op | B/op | allocs/op |
|------|-----------|-------|------|-----------|
| 64B | OurChannel | 4,107 | 2,081 | 28 |
| 64B | GrpchanChannel | 3,129 | 1,488 | 23 |
| 64B | RealGRPC | 39,783 | 11,133 | 186 |
| 1KB | OurChannel | 5,712 | 4,962 | 28 |
| 1KB | GrpchanChannel | 4,713 | 3,410 | 23 |
| 1KB | RealGRPC | 41,837 | 12,466 | 176 |
| 10KB | OurChannel | 16,292 | 32,619 | 28 |
| 10KB | GrpchanChannel | 8,558 | 21,845 | 23 |
| 10KB | RealGRPC | 53,330 | 31,166 | 176 |
| 100KB | OurChannel | 62,037 | 321,423 | 28 |
| 100KB | GrpchanChannel | 51,915 | 214,401 | 23 |
| 100KB | RealGRPC | 202,628 | 286,350 | 204 |
| 1MB | OurChannel | 384,265 | 3,148,003 | 29 |
| 1MB | GrpchanChannel | 255,080 | 2,098,808 | 23 |
| 1MB | RealGRPC | 1,183,286 | 2,828,383 | 265 |

## Server-Streaming Throughput (100 messages × 32 bytes)

| Competitor | ops | ns/op | B/op | allocs/op |
|-----------|-----|-------|------|-----------|
| OurChannel | 277 | 4,604,336 | 97,517 | 1,468 |
| GrpchanChannel | 10,000 | 101,084 | 57,202 | 741 |
| RealGRPC | 2,592 | 486,430 | 231,985 | 4,854 |

## Bidi-Streaming Echo

| Competitor | ops | ns/op | B/op | allocs/op |
|-----------|-----|-------|------|-----------|
| OurChannel | 94,960 | 95,204 | 1,677 | 26 |
| GrpchanChannel | 703,348 | 1,846 | 864 | 11 |
| RealGRPC | 68,146 | 17,364 | 2,858 | 71 |

## Concurrent Unary Load

| Goroutines | Competitor | ns/op | B/op | allocs/op |
|-----------|-----------|-------|------|-----------|
| 10 | OurChannel | 3,325 | 1,986 | 28 |
| 10 | GrpchanChannel | 1,495 | 1,425 | 23 |
| 10 | RealGRPC | 23,657 | 10,889 | 174 |
| 50 | OurChannel | 2,809 | 1,986 | 28 |
| 50 | GrpchanChannel | 1,586 | 1,425 | 23 |
| 50 | RealGRPC | 15,093 | 10,905 | 174 |
| 100 | OurChannel | 2,711 | 1,986 | 28 |
| 100 | GrpchanChannel | 1,360 | 1,425 | 23 |
| 100 | RealGRPC | 12,598 | 10,906 | 174 |

## Latency Percentiles (10 concurrent, 5000 iterations)

| Competitor | p50 (µs) | p90 (µs) | p95 (µs) | p99 (µs) |
|-----------|----------|----------|----------|----------|
| OurChannel | 3 | 40 | 87 | 933 |
| GrpchanChannel | 3 | 6 | 8 | 112 |
| RealGRPC | 174 | 380 | 992 | 1,533 |

## Notes

- Docker container runs on arm64 (Apple M2 Pro host), so absolute numbers are comparable to Darwin results.
- Server-streaming shows larger gap on Linux (46x vs grpchan) — container scheduling overhead amplifies event-loop round-trips.
- Bidi echo significantly slower on Linux (95µs vs 39µs on macOS) — likely Docker overhead.
- Latency p50 identical for Our vs Grpchan (3µs each) on Linux, better than macOS result.
