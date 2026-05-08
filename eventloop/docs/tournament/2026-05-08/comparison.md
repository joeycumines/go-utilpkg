# Darwin vs Linux Benchmark Comparison

**Date:** 2026-05-08
**Platforms:** Darwin (darwin/arm64) vs Linux (linux/arm64)
**Methodology:** focused `eventloop-tournament-bench` suite from `project.mk`: `go test -bench=<EVENTLOOP_REVIEW11_BENCH_RE> -benchmem -count=5 -run=^$ -benchtime=1s -timeout=10m` across `./eventloop`, `./eventloop/internal/tournament`, and `./goja-eventloop`
**Benchmarks Compared:** 38 common benchmarks

## Executive Summary

This report compares eventloop benchmark performance between **Darwin (darwin/arm64)** and
**Linux (linux/arm64)** using the platform metadata embedded in the parsed JSON files.
Both inputs report **arm64** architecture, so architecture is not an
observed differentiator for this run; remaining differences reflect OS, runtime,
container, kernel, and measurement-environment effects.

### Key Metrics

| Metric | Value |
|--------|-------|
| Common benchmarks | 38 |
| Darwin-only benchmarks | 0 |
| Linux-only benchmarks | 0 |
| Darwin wins (faster) | **26** (68.4%) |
| Linux wins (faster) | **12** (31.6%) |
| Ties | 0 |
| Statistically significant differences | 33 |
| Darwin mean (common benchmarks) | 353,881.40 ns/op |
| Linux mean (common benchmarks) | 365,463.73 ns/op |
| Mean ratio (Darwin/Linux) | 0.835x |
| Median ratio (Darwin/Linux) | 0.918x |
| Allocation match rate | 36/38 (94.7%) |
| Zero-allocation benchmarks (both) | 7 |

## Full Statistical Comparison Table

| # | Benchmark | Darwin (ns/op) | Darwin CV% | Linux (ns/op) | Linux CV% | Faster | Ratio | Sig? |
|---|-----------|----------------|------------|---------------|-----------|--------|-------|------|
| 1 | BenchmarkAdapterAsyncAwaitResolve |       4,402.00 |       1.2% |      4,230.60 |      6.0% | Linux |  1.04x |  |
| 2 | BenchmarkAutoExit_ImmediateExit |       3,634.60 |       2.6% |      4,741.80 |      4.1% | Darwin |  0.77x | yes |
| 3 | BenchmarkFDReadinessDispatchHighCount |     974,711.20 |       4.2% |    693,600.20 |     15.4% | Linux |  1.41x | yes |
| 4 | BenchmarkFDReadinessDispatchSingle |       8,125.40 |       4.6% |     41,870.60 |      2.4% | Darwin |  0.19x | yes |
| 5 | BenchmarkGojaPromiseChainHandoff |      18,612.40 |       1.9% |     22,585.40 |      5.1% | Darwin |  0.82x | yes |
| 6 | BenchmarkMetricsHotPath |          89.43 |       1.6% |         94.93 |      4.3% | Darwin |  0.94x | yes |
| 7 | BenchmarkMetricsHotPathSnapshot |       2,927.80 |       1.1% |      4,810.20 |      3.4% | Darwin |  0.61x | yes |
| 8 | BenchmarkMicrotaskScheduleExternal |          56.73 |       3.7% |         50.29 |      6.3% | Linux |  1.13x | yes |
| 9 | BenchmarkMicrotaskScheduleLoopThread |           5.59 |       0.3% |          7.14 |      0.9% | Darwin |  0.78x | yes |
| 10 | BenchmarkNativeAsyncAwaitResolve |       4,341.20 |       1.4% |      3,954.80 |      0.8% | Linux |  1.10x | yes |
| 11 | BenchmarkNativePromiseThenChain |      18,650.80 |       2.5% |     23,466.40 |      6.2% | Darwin |  0.79x | yes |
| 12 | BenchmarkNextTickRecursiveDrain |          11.83 |       0.5% |         12.50 |      3.9% | Darwin |  0.95x | yes |
| 13 | BenchmarkNextTickScheduleExternal |          55.31 |       1.3% |         48.65 |      2.1% | Linux |  1.14x | yes |
| 14 | BenchmarkNextTickScheduleLoopThread |           5.56 |       0.4% |          8.18 |     16.9% | Darwin |  0.68x | yes |
| 15 | BenchmarkProcessBeforeExitSchedulesTimer |   1,169,298.60 |       0.3% |  1,529,592.00 |      1.6% | Darwin |  0.76x | yes |
| 16 | BenchmarkPromiseJobEnqueuerOverhead |         722.74 |       1.0% |        670.92 |      7.5% | Linux |  1.08x |  |
| 17 | BenchmarkScheduleMicrotaskBaseline |         615.32 |       2.0% |        592.14 |      9.7% | Linux |  1.04x |  |
| 18 | BenchmarkSchedulerInternalExternalBurst/AlternateOne |         261.96 |       2.9% |        245.48 |      3.5% | Linux |  1.07x | yes |
| 19 | BenchmarkSchedulerInternalExternalBurst/AlternateThree |         129.74 |       2.9% |        196.48 |      3.2% | Darwin |  0.66x | yes |
| 20 | BenchmarkSchedulerInternalExternalBurst/AlternateTwo |         288.02 |       3.2% |        321.88 |      4.2% | Darwin |  0.89x | yes |
| 21 | BenchmarkSchedulerInternalExternalBurst/Baseline |         129.30 |       2.4% |        135.10 |      1.2% | Darwin |  0.96x | yes |
| 22 | BenchmarkSchedulerInternalExternalBurst/Main |         233.70 |       0.5% |        193.54 |      1.1% | Linux |  1.21x | yes |
| 23 | BenchmarkSchedulerPriorityLatency/AlternateOne |      24,254.20 |       1.3% |     90,419.40 |      8.2% | Darwin |  0.27x | yes |
| 24 | BenchmarkSchedulerPriorityLatency/AlternateThree |      23,815.60 |       2.2% |     95,025.00 |      3.5% | Darwin |  0.25x | yes |
| 25 | BenchmarkSchedulerPriorityLatency/AlternateTwo |      27,405.00 |       2.8% |    100,953.80 |      7.8% | Darwin |  0.27x | yes |
| 26 | BenchmarkSchedulerPriorityLatency/Baseline |       7,515.80 |       2.4% |      7,873.20 |      2.4% | Darwin |  0.95x | yes |
| 27 | BenchmarkSchedulerPriorityLatency/Main |       8,259.20 |       1.7% |      7,675.40 |      3.7% | Linux |  1.08x | yes |
| 28 | BenchmarkSetImmediateBurst |         264.20 |       0.8% |        286.14 |      8.0% | Darwin |  0.92x |  |
| 29 | BenchmarkSetIntervalSteadyTicks |   4,420,216.00 |       0.3% |  4,609,422.40 |      2.0% | Darwin |  0.96x | yes |
| 30 | BenchmarkSparseFDRegistration |       1,222.40 |       8.0% |        879.54 |      1.1% | Linux |  1.39x | yes |
| 31 | BenchmarkSubmitInternalChainHandoff/AlternateOne |      11,413.80 |      12.5% |     29,343.00 |      1.1% | Darwin |  0.39x | yes |
| 32 | BenchmarkSubmitInternalChainHandoff/AlternateThree |      11,551.00 |       6.5% |     34,753.00 |     11.6% | Darwin |  0.33x | yes |
| 33 | BenchmarkSubmitInternalChainHandoff/AlternateTwo |      11,100.80 |       9.7% |     32,876.80 |      4.4% | Darwin |  0.34x | yes |
| 34 | BenchmarkSubmitInternalChainHandoff/Baseline |       4,920.20 |       0.7% |      5,234.80 |      0.2% | Darwin |  0.94x | yes |
| 35 | BenchmarkSubmitInternalChainHandoff/Main |       8,697.60 |       1.1% |      9,532.80 |      0.8% | Darwin |  0.91x | yes |
| 36 | BenchmarkTimerCancelScale |         199.08 |       2.3% |        250.00 |      6.7% | Darwin |  0.80x | yes |
| 37 | BenchmarkTimerScheduleRandomDeadlines |         433.84 |       2.1% |        493.74 |      3.3% | Darwin |  0.88x | yes |
| 38 | BenchmarkTimerScheduleSameDeadline100K |   6,678,915.20 |      27.3% |  6,531,173.40 |      3.6% | Linux |  1.02x |  |

## Performance by Category

### Latency & Primitives (5 benchmarks)

- Darwin wins: 4/5
- Linux wins: 1/5
- Darwin category mean: 18,249.96 ns/op
- Linux category mean: 60,389.36 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkSchedulerPriorityLatency/AlternateTwo |      27,405.00 |    100,953.80 | Darwin | 3.68x |
| BenchmarkSchedulerPriorityLatency/AlternateThree |      23,815.60 |     95,025.00 | Darwin | 3.99x |
| BenchmarkSchedulerPriorityLatency/AlternateOne |      24,254.20 |     90,419.40 | Darwin | 3.73x |
| BenchmarkSchedulerPriorityLatency/Main |       8,259.20 |      7,675.40 | Linux | 1.08x |
| BenchmarkSchedulerPriorityLatency/Baseline |       7,515.80 |      7,873.20 | Darwin | 1.05x |

### Other (18 benchmarks)

- Darwin wins: 11/18
- Linux wins: 7/18
- Darwin category mean: 301,169.42 ns/op
- Linux category mean: 298,058.50 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkFDReadinessDispatchHighCount |     974,711.20 |    693,600.20 | Linux | 1.41x |
| BenchmarkSetIntervalSteadyTicks |   4,420,216.00 |  4,609,422.40 | Darwin | 1.04x |
| BenchmarkFDReadinessDispatchSingle |       8,125.40 |     41,870.60 | Darwin | 5.15x |
| BenchmarkMetricsHotPathSnapshot |       2,927.80 |      4,810.20 | Darwin | 1.64x |
| BenchmarkAutoExit_ImmediateExit |       3,634.60 |      4,741.80 | Darwin | 1.30x |
| BenchmarkNativeAsyncAwaitResolve |       4,341.20 |      3,954.80 | Linux | 1.10x |
| BenchmarkSparseFDRegistration |       1,222.40 |        879.54 | Linux | 1.39x |
| BenchmarkAdapterAsyncAwaitResolve |       4,402.00 |      4,230.60 | Linux | 1.04x |
| BenchmarkSchedulerInternalExternalBurst/AlternateThree |         129.74 |        196.48 | Darwin | 1.51x |
| BenchmarkSchedulerInternalExternalBurst/Main |         233.70 |        193.54 | Linux | 1.21x |
| BenchmarkSchedulerInternalExternalBurst/AlternateTwo |         288.02 |        321.88 | Darwin | 1.12x |
| BenchmarkSetImmediateBurst |         264.20 |        286.14 | Darwin | 1.08x |
| BenchmarkSchedulerInternalExternalBurst/AlternateOne |         261.96 |        245.48 | Linux | 1.07x |
| BenchmarkNextTickScheduleExternal |          55.31 |         48.65 | Linux | 1.14x |
| BenchmarkSchedulerInternalExternalBurst/Baseline |         129.30 |        135.10 | Darwin | 1.04x |
| BenchmarkMetricsHotPath |          89.43 |         94.93 | Darwin | 1.06x |
| BenchmarkNextTickScheduleLoopThread |           5.56 |          8.18 | Darwin | 1.47x |
| BenchmarkNextTickRecursiveDrain |          11.83 |         12.50 | Darwin | 1.06x |

### Promise Operations (8 benchmarks)

- Darwin wins: 7/8
- Linux wins: 1/8
- Darwin category mean: 10,708.67 ns/op
- Linux category mean: 19,807.89 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkSubmitInternalChainHandoff/AlternateThree |      11,551.00 |     34,753.00 | Darwin | 3.01x |
| BenchmarkSubmitInternalChainHandoff/AlternateTwo |      11,100.80 |     32,876.80 | Darwin | 2.96x |
| BenchmarkSubmitInternalChainHandoff/AlternateOne |      11,413.80 |     29,343.00 | Darwin | 2.57x |
| BenchmarkNativePromiseThenChain |      18,650.80 |     23,466.40 | Darwin | 1.26x |
| BenchmarkGojaPromiseChainHandoff |      18,612.40 |     22,585.40 | Darwin | 1.21x |
| BenchmarkSubmitInternalChainHandoff/Main |       8,697.60 |      9,532.80 | Darwin | 1.10x |
| BenchmarkSubmitInternalChainHandoff/Baseline |       4,920.20 |      5,234.80 | Darwin | 1.06x |
| BenchmarkPromiseJobEnqueuerOverhead |         722.74 |        670.92 | Linux | 1.08x |

### Task Submission (3 benchmarks)

- Darwin wins: 1/3
- Linux wins: 2/3
- Darwin category mean: 225.88 ns/op
- Linux category mean: 216.53 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkScheduleMicrotaskBaseline |         615.32 |        592.14 | Linux | 1.04x |
| BenchmarkMicrotaskScheduleExternal |          56.73 |         50.29 | Linux | 1.13x |
| BenchmarkMicrotaskScheduleLoopThread |           5.59 |          7.14 | Darwin | 1.28x |

### Timer Operations (4 benchmarks)

- Darwin wins: 3/4
- Linux wins: 1/4
- Darwin category mean: 1,962,211.68 ns/op
- Linux category mean: 2,015,377.29 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkProcessBeforeExitSchedulesTimer |   1,169,298.60 |  1,529,592.00 | Darwin | 1.31x |
| BenchmarkTimerScheduleSameDeadline100K |   6,678,915.20 |  6,531,173.40 | Linux | 1.02x |
| BenchmarkTimerScheduleRandomDeadlines |         433.84 |        493.74 | Darwin | 1.14x |
| BenchmarkTimerCancelScale |         199.08 |        250.00 | Darwin | 1.26x |

## Statistically Significant Differences

**33** out of 38 benchmarks show statistically significant
differences (Welch-style t-statistic threshold).

- Darwin significantly faster: **25** benchmarks
- Linux significantly faster: **8** benchmarks

### Largest Significant Differences

| Benchmark | Faster | Speedup | Darwin (ns/op) | Linux (ns/op) | t-stat |
|-----------|--------|---------|----------------|---------------|--------|
| BenchmarkFDReadinessDispatchSingle | Darwin | 5.15x |       8,125.40 |     41,870.60 | 70.46 |
| BenchmarkSchedulerPriorityLatency/AlternateThree | Darwin | 3.99x |      23,815.60 |     95,025.00 | 46.95 |
| BenchmarkSchedulerPriorityLatency/AlternateOne | Darwin | 3.73x |      24,254.20 |     90,419.40 | 20.00 |
| BenchmarkSchedulerPriorityLatency/AlternateTwo | Darwin | 3.68x |      27,405.00 |    100,953.80 | 20.79 |
| BenchmarkSubmitInternalChainHandoff/AlternateThree | Darwin | 3.01x |      11,551.00 |     34,753.00 | 12.64 |
| BenchmarkSubmitInternalChainHandoff/AlternateTwo | Darwin | 2.96x |      11,100.80 |     32,876.80 | 26.96 |
| BenchmarkSubmitInternalChainHandoff/AlternateOne | Darwin | 2.57x |      11,413.80 |     29,343.00 | 27.47 |
| BenchmarkMetricsHotPathSnapshot | Darwin | 1.64x |       2,927.80 |      4,810.20 | 25.32 |
| BenchmarkSchedulerInternalExternalBurst/AlternateT | Darwin | 1.51x |         129.74 |        196.48 | 20.47 |
| BenchmarkNextTickScheduleLoopThread | Darwin | 1.47x |           5.56 |          8.18 | 4.23 |
| BenchmarkFDReadinessDispatchHighCount | Linux | 1.41x |     974,711.20 |    693,600.20 | 5.51 |
| BenchmarkSparseFDRegistration | Linux | 1.39x |       1,222.40 |        879.54 | 7.78 |
| BenchmarkProcessBeforeExitSchedulesTimer | Darwin | 1.31x |   1,169,298.60 |  1,529,592.00 | 32.01 |
| BenchmarkAutoExit_ImmediateExit | Darwin | 1.30x |       3,634.60 |      4,741.80 | 11.31 |
| BenchmarkMicrotaskScheduleLoopThread | Darwin | 1.28x |           5.59 |          7.14 | 51.78 |
| BenchmarkNativePromiseThenChain | Darwin | 1.26x |      18,650.80 |     23,466.40 | 7.03 |
| BenchmarkTimerCancelScale | Darwin | 1.26x |         199.08 |        250.00 | 6.58 |
| BenchmarkGojaPromiseChainHandoff | Darwin | 1.21x |      18,612.40 |     22,585.40 | 7.32 |
| BenchmarkSchedulerInternalExternalBurst/Main | Linux | 1.21x |         233.70 |        193.54 | 37.85 |
| BenchmarkTimerScheduleRandomDeadlines | Darwin | 1.14x |         433.84 |        493.74 | 7.24 |
| BenchmarkNextTickScheduleExternal | Linux | 1.14x |          55.31 |         48.65 | 11.82 |
| BenchmarkMicrotaskScheduleExternal | Linux | 1.13x |          56.73 |         50.29 | 3.77 |
| BenchmarkSchedulerInternalExternalBurst/AlternateT | Darwin | 1.12x |         288.02 |        321.88 | 4.62 |
| BenchmarkNativeAsyncAwaitResolve | Linux | 1.10x |       4,341.20 |      3,954.80 | 12.48 |
| BenchmarkSubmitInternalChainHandoff/Main | Darwin | 1.10x |       8,697.60 |      9,532.80 | 15.03 |
| BenchmarkSchedulerPriorityLatency/Main | Linux | 1.08x |       8,259.20 |      7,675.40 | 4.09 |
| BenchmarkSchedulerInternalExternalBurst/AlternateO | Linux | 1.07x |         261.96 |        245.48 | 3.21 |
| BenchmarkSubmitInternalChainHandoff/Baseline | Darwin | 1.06x |       4,920.20 |      5,234.80 | 20.37 |
| BenchmarkMetricsHotPath | Darwin | 1.06x |          89.43 |         94.93 | 2.83 |
| BenchmarkNextTickRecursiveDrain | Darwin | 1.06x |          11.83 |         12.50 | 3.04 |

## Top 10 Fastest Benchmarks

### Darwin (darwin/arm64)

| Rank | Benchmark | ns/op | B/op | Allocs/op | CV% |
|------|-----------|-------|------|-----------|-----|
| 1 | BenchmarkNextTickScheduleLoopThread |       5.56 |    0 |         0 | 0.4% |
| 2 | BenchmarkMicrotaskScheduleLoopThread |       5.59 |    0 |         0 | 0.3% |
| 3 | BenchmarkNextTickRecursiveDrain |      11.83 |    0 |         0 | 0.5% |
| 4 | BenchmarkNextTickScheduleExternal |      55.31 |    0 |         0 | 1.3% |
| 5 | BenchmarkMicrotaskScheduleExternal |      56.73 |    0 |         0 | 3.7% |
| 6 | BenchmarkMetricsHotPath |      89.43 |    0 |         0 | 1.6% |
| 7 | BenchmarkSchedulerInternalExternalBurst/Baseline |     129.30 |  112 |         6 | 2.4% |
| 8 | BenchmarkSchedulerInternalExternalBurst/AlternateT |     129.74 |   32 |         2 | 2.9% |
| 9 | BenchmarkTimerCancelScale |     199.08 |   23 |         0 | 2.3% |
| 10 | BenchmarkSchedulerInternalExternalBurst/Main |     233.70 |   33 |         2 | 0.5% |

### Linux (linux/arm64)

| Rank | Benchmark | ns/op | B/op | Allocs/op | CV% |
|------|-----------|-------|------|-----------|-----|
| 1 | BenchmarkMicrotaskScheduleLoopThread |       7.14 |    0 |         0 | 0.9% |
| 2 | BenchmarkNextTickScheduleLoopThread |       8.18 |    0 |         0 | 16.9% |
| 3 | BenchmarkNextTickRecursiveDrain |      12.50 |    0 |         0 | 3.9% |
| 4 | BenchmarkNextTickScheduleExternal |      48.65 |    0 |         0 | 2.1% |
| 5 | BenchmarkMicrotaskScheduleExternal |      50.29 |    0 |         0 | 6.3% |
| 6 | BenchmarkMetricsHotPath |      94.93 |    0 |         0 | 4.3% |
| 7 | BenchmarkSchedulerInternalExternalBurst/Baseline |     135.10 |  112 |         6 | 1.2% |
| 8 | BenchmarkSchedulerInternalExternalBurst/Main |     193.54 |   32 |         2 | 1.1% |
| 9 | BenchmarkSchedulerInternalExternalBurst/AlternateT |     196.48 |   32 |         2 | 3.2% |
| 10 | BenchmarkSchedulerInternalExternalBurst/AlternateO |     245.48 |   33 |         2 | 3.5% |

## Allocation Comparison

This section compares measured allocation counts (allocs/op) and bytes (B/op)
between the loaded platform runs. Differences may reflect runtime, architecture,
compiler, benchmark calibration, or measurement-environment behavior; they are
reported as data, not assumed away.

- **Allocs/op match:** 36/38 (94.7%)
- **B/op match:** 23/38 (60.5%)
- **Zero-allocation benchmarks (both platforms):** 7

### Zero-Allocation Benchmarks

These benchmarks achieve zero allocations on both platforms — the gold standard
for hot-path performance:

- `BenchmarkMetricsHotPath` — Darwin: 89.43 ns/op, Linux: 94.93 ns/op (Darwin faster)
- `BenchmarkMicrotaskScheduleExternal` — Darwin: 56.73 ns/op, Linux: 50.29 ns/op (Linux faster)
- `BenchmarkMicrotaskScheduleLoopThread` — Darwin: 5.59 ns/op, Linux: 7.14 ns/op (Darwin faster)
- `BenchmarkNextTickRecursiveDrain` — Darwin: 11.83 ns/op, Linux: 12.50 ns/op (Darwin faster)
- `BenchmarkNextTickScheduleExternal` — Darwin: 55.31 ns/op, Linux: 48.65 ns/op (Linux faster)
- `BenchmarkNextTickScheduleLoopThread` — Darwin: 5.56 ns/op, Linux: 8.18 ns/op (Darwin faster)
- `BenchmarkTimerCancelScale` — Darwin: 199.08 ns/op, Linux: 250.00 ns/op (Darwin faster)

### Allocation Mismatches

| Benchmark | Darwin allocs | Linux allocs | Delta |
|-----------|---------------|--------------|-------|
| BenchmarkSparseFDRegistration | 3 | 1 | 2 |
| BenchmarkTimerScheduleSameDeadline100K | 3 | 3 | 0 |

### B/op Mismatches

| Benchmark | Darwin B/op | Linux B/op | Delta |
|-----------|-------------|------------|-------|
| BenchmarkAutoExit_ImmediateExit | 19,576 | 15,992 | 3,584 |
| BenchmarkFDReadinessDispatchHighCount | 16,016 | 16,011 | 5 |
| BenchmarkGojaPromiseChainHandoff | 31,429 | 31,429 | 0 |
| BenchmarkMicrotaskScheduleExternal | 0 | 0 | 0 |
| BenchmarkNativePromiseThenChain | 31,428 | 31,429 | 1 |
| BenchmarkProcessBeforeExitSchedulesTimer | 7,687 | 7,676 | 11 |
| BenchmarkSchedulerInternalExternalBurst/AlternateOne | 32 | 33 | 1 |
| BenchmarkSchedulerInternalExternalBurst/AlternateTwo | 42 | 45 | 3 |
| BenchmarkSchedulerInternalExternalBurst/Main | 33 | 32 | 1 |
| BenchmarkSchedulerPriorityLatency/AlternateTwo | 2,119 | 2,119 | 0 |
| BenchmarkSetImmediateBurst | 48 | 48 | 0 |
| BenchmarkSetIntervalSteadyTicks | 74 | 74 | 0 |
| BenchmarkSparseFDRegistration | 80 | 16 | 64 |
| BenchmarkTimerScheduleRandomDeadlines | 258 | 271 | 12 |
| BenchmarkTimerScheduleSameDeadline100K | 23,722 | 26,074 | 2,352 |

## Measurement Stability

Coefficient of variation (CV%) indicates measurement consistency. Lower is better.

- Benchmarks with CV < 2% on both platforms: **7**
- Darwin benchmarks with CV > 5%: **5**
- Linux benchmarks with CV > 5%: **13**

### High-Variance Benchmarks (CV > 5%)

| Benchmark | Darwin CV% | Linux CV% |
|-----------|------------|-----------|
| BenchmarkAdapterAsyncAwaitResolve | 1.2% | 6.0% (high) |
| BenchmarkFDReadinessDispatchHighCount | 4.2% | 15.4% (high) |
| BenchmarkGojaPromiseChainHandoff | 1.9% | 5.1% (high) |
| BenchmarkMicrotaskScheduleExternal | 3.7% | 6.3% (high) |
| BenchmarkNativePromiseThenChain | 2.5% | 6.2% (high) |
| BenchmarkNextTickScheduleLoopThread | 0.4% | 16.9% (high) |
| BenchmarkPromiseJobEnqueuerOverhead | 1.0% | 7.5% (high) |
| BenchmarkScheduleMicrotaskBaseline | 2.0% | 9.7% (high) |
| BenchmarkSchedulerPriorityLatency/AlternateOne | 1.3% | 8.2% (high) |
| BenchmarkSchedulerPriorityLatency/AlternateTwo | 2.8% | 7.8% (high) |
| BenchmarkSetImmediateBurst | 0.8% | 8.0% (high) |
| BenchmarkSparseFDRegistration | 8.0% (high) | 1.1% |
| BenchmarkSubmitInternalChainHandoff/AlternateOne | 12.5% (high) | 1.1% |
| BenchmarkSubmitInternalChainHandoff/AlternateThree | 6.5% (high) | 11.6% (high) |
| BenchmarkSubmitInternalChainHandoff/AlternateTwo | 9.7% (high) | 4.4% |
| BenchmarkTimerCancelScale | 2.3% | 6.7% (high) |
| BenchmarkTimerScheduleSameDeadline100K | 27.3% (high) | 3.6% |

## Key Findings

### 1. Platform and Architecture Metadata

- Darwin input metadata: `Darwin (darwin/arm64)`
- Linux input metadata: `Linux (linux/arm64)`
- Both inputs report `arm64`; architecture is held constant for this run.

### 2. Performance Distribution

- Darwin significantly faster (ratio < 0.9): **18** benchmarks
- Roughly equal (0.9–1.1x): **15** benchmarks
- Linux significantly faster (ratio > 1.1): **5** benchmarks

### 3. Timer Operations

- Total timer benchmarks: 4
- Darwin faster: 3
- Linux faster: 1
- Biggest difference: `BenchmarkProcessBeforeExitSchedulesTimer` — Linux is 1.31x slower

### 4. Concurrency & Contention

- `BenchmarkSubmitInternalChainHandoff/AlternateOne`: Darwin (2.57x)
- `BenchmarkSubmitInternalChainHandoff/AlternateThree`: Darwin (3.01x)
- `BenchmarkSubmitInternalChainHandoff/AlternateTwo`: Darwin (2.96x)
- `BenchmarkSubmitInternalChainHandoff/Baseline`: Darwin (1.06x)
- `BenchmarkSubmitInternalChainHandoff/Main`: Darwin (1.10x)

### 5. Summary

**Darwin wins overall** with 26/38 benchmarks faster.

The mean performance ratio of 0.835x (Darwin/Linux) indicates
the aggregate mean leans toward Darwin for this run; use the benchmark,
category, and significance tables above to identify which workloads drove it.

