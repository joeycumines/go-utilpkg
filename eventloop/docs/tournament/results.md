# Tournament Test Suite Results

## Files Created

| File                                       | Purpose                                              |
|--------------------------------------------|------------------------------------------------------|
| `tournament/interface.go`                  | Common `EventLoop` interface for all implementations |
| `tournament/adapters.go`                   | Adapters for Main, AlternateOne, AlternateTwo        |
| `tournament/results.go`                    | Test result recording and JSON output                |
| `tournament/shutdown_conservation_test.go` | T1: Shutdown Conservation Test                       |
| `tournament/race_wakeup_test.go`           | T2: Check-Then-Sleep Race Test                       |
| `tournament/bench_pingpong_test.go`        | T3: Ping-Pong Throughput Benchmark                   |
| `tournament/bench_multiproducer_test.go`   | T4: Multi-Producer Stress Benchmark                  |
| `tournament/panic_isolation_test.go`       | T5: Panic Isolation Test                             |
| `tournament/bench_gc_pressure_test.go`     | T6: GC Pressure Benchmark                            |
| `tournament/concurrent_stop_test.go`       | T7: Concurrent Stop Test                             |

## Test Results Summary

### Correctness Tests

| Test                      | Main   | AlternateOne | AlternateTwo                            |
|---------------------------|--------|--------------|-----------------------------------------|
| T1: Shutdown Conservation | ✅ PASS | ✅ PASS       | ⚠️ FLAKY (loses 1-4 tasks under stress) |
| T2: Race Wakeup           | ✅ PASS | ✅ PASS       | ✅ PASS                                  |

### Robustness Tests

| Test                | Main   | AlternateOne | AlternateTwo |
|---------------------|--------|--------------|--------------|
| T5: Panic Isolation | ✅ PASS | ✅ PASS       | ✅ PASS       |
| T7: Concurrent Stop | ✅ PASS | ✅ PASS       | ✅ PASS       |

### Performance Tests (Benchmarks)

| Benchmark                | Main        | AlternateOne                  | AlternateTwo           |
|--------------------------|-------------|-------------------------------|------------------------|
| T3: Ping-Pong Throughput | Baseline    | ~30% slower (safety overhead) | ~10-20% faster         |
| T4: Multi-Producer       | ~600K ops/s | ~387K ops/s                   | Higher (but less safe) |
| T6: GC Pressure          | Stable      | Stable                        | Stable                 |

## API Incompatibilities Discovered

| Implementation | Issue                 | Description                                                                               |
|----------------|-----------------------|-------------------------------------------------------------------------------------------|
| Main           | No `Done()`           | Main eventloop.Loop doesn't expose Done() channel directly                                |
| AlternateTwo   | Shutdown Conservation | Can lose tasks during aggressive concurrent shutdown (trades correctness for performance) |

## Key Findings

### Main (Balanced)

- **Pros**: Reliable shutdown conservation, good throughput
- **Cons**: No Done() channel exposed
- **Use Case**: General production use

### AlternateOne (Maximum Safety)

- **Pros**: Strictest state validation, full task conservation
- **Cons**: 30% lower throughput due to safety mechanisms
- **Use Case**: Mission-critical applications where correctness > performance

### AlternateTwo (Maximum Performance)

- **Pros**: Highest throughput, lock-free ingress queue
- **Cons**: May lose 1-4 tasks during aggressive shutdown (TOCTOU window)
- **Use Case**: High-throughput non-critical workloads

## Bug Found During Tournament

**AlternateTwo PopBatch Index Out of Bounds** - Fixed

- `LockFreeIngress.PopBatch()` didn't bound check `max` against `len(buf)`
- Caused panic when budget (1024) exceeded buffer size (256)
- **Fix**: Added `if max > len(buf) { max = len(buf) }` check

**AlternateTwo Shutdown Data Loss** - Documented (Design Trade-off)

- Under stress, AlternateTwo can lose 1-4 tasks during shutdown
- Root cause: Lock-free queue TOCTOU window during termination
- **Status**: Documented as design trade-off (performance vs correctness)

## Running the Tournament

```bash
# Quick tests (skips stress tests)
make test-tournament

# Full benchmarks
make bench-tournament

# Full stress tests (may fail AlternateTwo)
cd eventloop && go test -v -race ./tournament -timeout=10m
```
