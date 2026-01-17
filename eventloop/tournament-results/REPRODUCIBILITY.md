# Reproducibility Guide

This document provides step-by-step instructions for reproducing all benchmark results and analysis in this repository.

## Prerequisites

- **Go:** Version 1.23 or later (tested with Go 1.25.5)
- **Platform:** macOS ARM64 (Apple Silicon) or Linux x86_64
- **Tools:** `make`, `git`

## Quick Start

```bash
# Clone and navigate
git clone <repository-url>
cd go-utilpkg

# Verify all tests pass
make make-all-with-log

# Run quick performance check
make bench-eventloop-quick
```

## Benchmark Make Targets

### 1. Quick Performance Check (1s benchtime, ~5 min)

```bash
make bench-eventloop-quick
```

Runs:
- PingPong Throughput
- PingPong Latency
- Multi-Producer Throughput (10 producers)
- Burst Submit (1000 tasks per burst)

Output: `eventloop/tournament-results/quick-run-TIMESTAMP/`

### 2. Full Suite (10 iterations, ~10 min)

```bash
make bench-eventloop-full
```

Runs all benchmarks with:
- `-count=10` for multiple iterations
- `-benchmem` for allocation metrics

Output: `eventloop/tournament-results/full-run-TIMESTAMP/`

### 3. Expanded Suite (100 iterations, ~20-30 min)

```bash
make bench-eventloop-expanded
```

Runs all benchmarks with:
- `-count=100` for statistical significance
- `-benchtime=100ms` per iteration
- `-benchmem` for allocation metrics

Output: `eventloop/tournament-results/expanded-run-TIMESTAMP/`

### 4. Root Cause Microbenchmarks (~15 min)

```bash
make bench-eventloop-micro
```

Runs:
- CAS Contention Analysis (`micro_cas_test.go`)
- Wakeup Syscall Overhead (`micro_wakeup_test.go`)
- Batch Budget Variation (`micro_batch_test.go`)

Output: `eventloop/tournament-results/microbench-TIMESTAMP/`

## Critical Bug Verification

### Thread Affinity Fix (Critical Bug #1)

```bash
make test-eventloop-thread-affinity
```

Runs 10 iterations with `-race` flag to verify:
- Fast path only executes on loop goroutine
- No goroutine ID mismatches

### MicrotaskRing.IsEmpty() Fix (Critical Bug #2)

```bash
make test-microtaskring-isempty
```

Verifies:
- IsEmpty() returns true only when truly empty
- Consistent with Length() after partial consumption

### Loop.tick() Data Race Fix (Critical Bug #3)

```bash
make test-tickanchor-datarace
```

Verifies:
- No race warnings with `-race` flag
- Concurrent tickAnchor access is safe

## Analyzing Results

### Using benchstat

Install benchstat for statistical analysis:

```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

Compare two runs:

```bash
benchstat old.txt new.txt
```

### Manual Analysis

Raw benchmark files contain:
- `ns/op` - nanoseconds per operation
- `B/op` - bytes allocated per operation
- `allocs/op` - allocations per operation
- `p50_ns`, `p95_ns`, `p99_ns` - latency percentiles (where available)

## Data File Locations

### Tournament Results

```
eventloop/tournament-results/
├── quick-run-TIMESTAMP/
│   ├── bench_pingpong.raw
│   ├── bench_latency.raw
│   ├── bench_multiproducer.raw
│   └── bench_burst.raw
├── microbench-TIMESTAMP/
│   ├── bench_cas.raw
│   ├── bench_batch.raw
│   ├── bench_wakeup.raw
│   └── SUMMARY.md
├── FINAL_ANALYSIS.md
├── KEY_FINDINGS.md
└── FINAL_REPORT_2026-01-17.md
```

### Source Files

```
eventloop/
├── loop.go                 # Main event loop (bugs fixed here)
├── ingress.go              # MPSC queue implementations
├── internal/tournament/
│   ├── bench_test.go       # Tournament benchmarks
│   ├── micro_cas_test.go   # CAS contention microbenchmark
│   ├── micro_wakeup_test.go # Wakeup syscall microbenchmark
│   └── micro_batch_test.go  # Batch budget microbenchmark
```

## Cross-Platform Testing (Optional)

### Linux Container Setup

```bash
# Build container
docker build -f Dockerfile.eventloop-bench -t eventloop-bench .

# Run benchmarks in container
docker run -v $(pwd):/workspace eventloop-bench make bench-eventloop-quick
```

Note: Dockerfile.eventloop-bench needs to be created for cross-platform testing.

## Troubleshooting

### Tests Fail with Race Warnings

Ensure you're running with the bug fixes applied:
- `loop.go` line 732: `l.isLoopThread()` check
- `ingress.go` IsEmpty(): `len(r.overflow) - r.overflowHead == 0`
- `loop.go` tick(): RLock around tickAnchor read

### Benchmarks Timeout

Increase timeout:
```bash
go test -bench=... -timeout=30m ./internal/tournament/
```

### Wakeup Test Crashes (SIGABRT)

The wakeup microbenchmark had a goroutine coordination bug (fixed 2026-01-17). Ensure you have the latest version of `micro_wakeup_test.go`.

## Verification Checklist

- [ ] `make make-all-with-log` passes with zero failures
- [ ] `make test-eventloop-thread-affinity` passes 10 iterations
- [ ] `make test-microtaskring-isempty` passes with -race
- [ ] `make test-tickanchor-datarace` passes with -race
- [ ] `make bench-eventloop-quick` completes without errors
- [ ] `make bench-eventloop-micro` generates all three .raw files

---

*Last updated: 2026-01-17*
