# 03 — Cross-Platform Results

## Overview

The 2026-04-22 tournament measured 158 benchmarks on each of 3 platforms. This document presents the detailed cross-platform results with statistical significance testing.

## Performance Summary

| Platform | Mean ns/op | Architecture | GOMAXPROCS | Environment |
|----------|------------|--------------|------------|-------------|
| Darwin | 60,291.47 | ARM64 | 10 | Native macOS |
| Linux | 98,585.78 | ARM64 | 10 | Docker container |
| Windows | 61,387.59 | AMD64 | 16 | WSL/moo |

**Overall speedup factor (slowest vs fastest)**: 1.64x (Linux slowest, Darwin fastest)

## 2-Platform Analysis: Darwin vs Linux (ARM64 vs ARM64)

Since both Darwin and Linux run on ARM64, this comparison isolates OS-level effects:
- Kernel scheduling (macOS Mach scheduler vs Linux CFS)
- Memory management (macOS memory pressure vs Linux cgroups in container)
- Syscall overhead differences
- Go runtime behavior variations between `darwin/arm64` and `linux/arm64`

### Win Distribution

| Metric | Value |
|--------|-------|
| Common benchmarks | 158 |
| Darwin wins (faster) | 99 (62.7%) |
| Linux wins (faster) | 59 (37.3%) |
| Mean ratio (Darwin/Linux) | 0.962x |
| Median ratio (Darwin/Linux) | 0.966x |

### Statistically Significant Differences

110 out of 158 benchmarks (69.6%) show statistically significant differences (Welch t-test, p < 0.05).

- Darwin significantly faster: 67 benchmarks
- Linux significantly faster: 43 benchmarks

## Category Breakdown

### Timer Operations (26 benchmarks)

Darwin dominates timer operations:

| Metric | Value |
|--------|-------|
| Darwin wins | 18/26 (69.2%) |
| Linux wins | 8/26 (30.8%) |
| Darwin category mean | 282,760.57 ns/op |
| Linux category mean | 462,314.27 ns/op |

**Largest differences**:
- `BenchmarkCancelTimer_Individual/timers_:` — Darwin 2.7M ns/op, Linux 5.2M ns/op (1.93x faster)
- `BenchmarkSentinelIteration_WithTimers` — Darwin 15.8K ns/op, Linux 42.8K ns/op (2.71x faster)
- `BenchmarkTimerLatency` — Darwin 19.9K ns/op, Linux 49.5K ns/op (2.48x faster)

**Note**: Linux timer benchmarks show high variance (CV > 10% on several), while Darwin timer benchmarks are more consistent.

### Promise Operations (18 benchmarks)

Darwin dominates promise operations:

| Metric | Value |
|--------|-------|
| Darwin wins | 17/18 (94.4%) |
| Linux wins | 1/18 (5.6%) |
| Darwin category mean | 6,635.04 ns/op |
| Linux category mean | 7,068.04 ns/op |

**Linux exception**: `BenchmarkPromiseHandlerTracking_Parallel_Optimized` — Linux is 2.07x faster (167.36 vs 346.34 ns/op)

### Task Submission (32 benchmarks)

Roughly even split:

| Metric | Value |
|--------|-------|
| Darwin wins | 17/32 (53.1%) |
| Linux wins | 15/32 (46.9%) |
| Darwin category mean | 8,672.86 ns/op |
| Linux category mean | 14,547.20 ns/op |

### Microtask Operations (12 benchmarks)

Linux dominates microtask operations:

| Metric | Value |
|--------|-------|
| Darwin wins | 3/12 (25%) |
| Linux wins | 9/12 (75%) |

Notable Linux advantages:
- `BenchmarkMicrotaskLatency` — Linux 1.32x faster
- `BenchmarkMicrotaskSchedule` — Linux 1.31x faster
- `BenchmarkMicrotaskSchedule_Parallel` — Linux 1.48x faster

## Architecture Comparison (All 3 Platforms)

### ARM64 vs ARM64 (Darwin vs Linux)
- Mean ratio: 0.962x (Darwin faster)
- Median ratio: 0.966x (Darwin faster)
- Darwin faster: 99 benchmarks
- Linux faster: 59 benchmarks

### ARM64 vs AMD64 (Darwin/Linux vs Windows)

Since Windows uses AMD64 and different GOMAXPROCS, these comparisons conflate architecture and OS effects.

**Darwin vs Windows**:
- Mean ratio: 0.990x
- Darwin faster: 104 benchmarks
- Windows faster: 54 benchmarks

**Linux vs Windows**:
- Mean ratio: 1.157x (Linux faster overall)
- Linux faster: 92 benchmarks
- Windows faster: 66 benchmarks

## Top 10 Fastest Benchmarks by Platform

### Darwin (ARM64)
| Rank | Benchmark | ns/op | Allocs/op |
|------|-----------|-------|----------|
| 1 | BenchmarkRegression_HasInternalTasks_SimulatedAtom | 0.30 | 0 |
| 2 | BenchmarkLatencyDirectCall | 0.31 | 0 |
| 3 | BenchmarkLatencyStateLoad | 0.32 | 0 |
| 4 | BenchmarkIsLoopThread_False | 0.38 | 0 |
| 5 | BenchmarkRegression_Combined_Atomic | 0.48 | 0 |
| 6 | BenchmarkTerminated_RejectionPath_RefTimer | 2.02 | 0 |
| 7 | BenchmarkTerminated_UnrefTimer_NotGated | 2.05 | 0 |
| 8 | BenchmarkMicrotaskRingIsEmpty_WithItems | 2.09 | 0 |
| 9 | BenchmarkAlive_WithTimer | 2.18 | 0 |
| 10 | BenchmarkLatencyDeferRecover | 2.53 | 0 |

### Linux (ARM64)
| Rank | Benchmark | ns/op | Allocs/op |
|------|-----------|-------|----------|
| 1 | BenchmarkLatencyStateLoad | 0.31 | 0 |
| 2 | BenchmarkLatencyDirectCall | 0.31 | 0 |
| 3 | BenchmarkRegression_HasInternalTasks_SimulatedAtom | 0.34 | 0 |
| 4 | BenchmarkIsLoopThread_False | 0.37 | 0 |
| 5 | BenchmarkRegression_Combined_Atomic | 0.51 | 0 |
| 6 | BenchmarkAlive_WithTimer | 2.22 | 0 |
| 7 | BenchmarkMicrotaskRingIsEmpty_WithItems | 2.26 | 0 |
| 8 | BenchmarkTerminated_UnrefTimer_NotGated | 2.27 | 0 |
| 9 | BenchmarkTerminated_RejectionPath_RefTimer | 2.30 | 0 |
| 10 | BenchmarkLatencyDeferRecover | 2.48 | 0 |

### Windows (AMD64)
| Rank | Benchmark | ns/op | Allocs/op |
|------|-----------|-------|----------|
| 1 | BenchmarkLatencyDirectCall | 0.21 | 0 |
| 2 | BenchmarkRegression_HasInternalTasks_SimulatedAtom | 0.21 | 0 |
| 3 | BenchmarkLatencyStateLoad | 0.26 | 0 |
| 4 | BenchmarkRegression_Combined_Atomic | 0.41 | 0 |
| 5 | BenchmarkIsLoopThread_False | 0.42 | 0 |
| 6 | BenchmarkAlive_WithTimer | 1.43 | 0 |
| 7 | BenchmarkMicrotaskRingIsEmpty_WithItems | 1.44 | 0 |
| 8 | BenchmarkTerminated_UnrefTimer_NotGated | 2.25 | 0 |
| 9 | BenchmarkTerminated_RejectionPath_RefTimer | 2.25 | 0 |
| 10 | BenchmarkLatencyDeferRecover | 2.89 | 0 |

## Top 10 Slowest Benchmarks by Platform

### Darwin (ARM64)
| Rank | Benchmark | ns/op | Allocs/op |
|------|-----------|-------|----------|
| 1 | BenchmarkCancelTimer_Individual/timers_: | 2,695,208.80 | 401 |
| 2 | BenchmarkCancelTimers_Comparison/Individual | 1,517,221.80 | 200 |
| 3 | BenchmarkCancelTimer_Individual/timers_5 | 1,498,278.20 | 201 |
| 4 | BenchmarkAutoExit_UnrefExit | 1,253,749.00 | 41 |
| 5 | BenchmarkCancelTimers_Batch/timers_: | 474,486.60 | 106 |
| 6 | BenchmarkCancelTimer_Individual/timers_1 | 287,121.00 | 41 |
| 7 | BenchmarkCancelTimers_Batch/timers_5 | 263,300.00 | 56 |
| 8 | BenchmarkCancelTimers_Comparison/Batch | 250,715.60 | 55 |
| 9 | BenchmarkAutoExit_FastPathExit | 224,403.40 | 34 |
| 10 | BenchmarkAutoExit_ImmediateExit | 196,374.80 | 25 |

### Linux (ARM64)
| Rank | Benchmark | ns/op | Allocs/op |
|------|-----------|-------|----------|
| 1 | BenchmarkCancelTimer_Individual/timers_: | 5,198,410.60 | 401 |
| 2 | BenchmarkCancelTimer_Individual/timers_5 | 2,492,180.20 | 201 |
| 3 | BenchmarkCancelTimers_Comparison/Individual | 2,486,208.60 | 200 |
| 4 | BenchmarkAutoExit_UnrefExit | 2,166,326.40 | 40 |
| 5 | BenchmarkCancelTimer_Individual/timers_1 | 440,906.00 | 41 |
| 6 | BenchmarkCancelTimers_Batch/timers_: | 415,786.80 | 106 |
| 7 | BenchmarkAutoExit_FastPathExit | 396,258.00 | 32 |
| 8 | BenchmarkAutoExit_ImmediateExit | 394,390.80 | 23 |
| 9 | BenchmarkCancelTimers_Batch/timers_5 | 228,906.20 | 56 |
| 10 | BenchmarkCancelTimers_Comparison/Batch | 226,902.80 | 55 |

### Windows (AMD64)
| Rank | Benchmark | ns/op | Allocs/op |
|------|-----------|-------|----------|
| 1 | BenchmarkCancelTimer_Individual/timers_: | 2,320,530.20 | 401 |
| 2 | BenchmarkAutoExit_UnrefExit | 1,580,193.20 | 39 |
| 3 | BenchmarkCancelTimer_Individual/timers_5 | 1,165,793.00 | 201 |
| 4 | BenchmarkCancelTimers_Comparison/Individual | 1,158,719.80 | 200 |
| 5 | BenchmarkCancelTimers_Batch/timers_: | 562,062.60 | 106 |
| 6 | BenchmarkAutoExit_PollPathExit | 411,167.60 | 33 |
| 7 | BenchmarkAutoExit_TimerFire | 390,825.80 | 31 |
| 8 | BenchmarkAutoExit_FastPathExit | 384,779.40 | 33 |
| 9 | BenchmarkAutoExit_ImmediateExit | 348,964.20 | 23 |
| 10 | BenchmarkCancelTimers_Comparison/Batch | 298,487.20 | 55 |

## High Variance Benchmarks (CV > 5%)

High variance indicates measurement instability and may affect the reliability of rankings.

### Darwin High-Variance (42 benchmarks with CV > 5%)
Notable extreme cases:
- `BenchmarkPromiseGC`: CV=93.9% (extreme instability)
- `BenchmarkSentinelDrain_NoWork`: CV=57.3%
- `BenchmarkLatencychunkedIngressPop`: CV=47.5%
- `BenchmarkQuiescing_ScheduleTimer_NoAutoExit`: CV=46.5%

### Linux High-Variance (43 benchmarks with CV > 5%)
Notable extreme cases:
- `BenchmarkAutoExit_ImmediateExit`: CV=107.7% (extreme instability)
- `BenchmarkAutoExit_FastPathExit`: CV=106.3% (extreme instability)
- `BenchmarkSentinelDrain_NoWork`: CV=136.1% (highest variance observed)
- `BenchmarkSentinelIteration`: CV=65.4%

### Windows High-Variance (fewer high-variance benchmarks overall)
Notable cases:
- `BenchmarkAlive_Epoch_FromCallback`: CV=42.3%
- `BenchmarkSetTimeout_Optimized`: CV=32.7%

## Cross-Tournament Changes (2026-04-22 vs 2026-04-19)

### Darwin
**17 improvements, 7 regressions** (statistically significant, p<0.05)

Top improvements:
- `BenchmarkTimerLatency`: 27,901 -> 19,928 ns/op (-28.6%)
- `BenchmarkSetInterval_Optimized`: 30,681 -> 24,521 ns/op (-20.1%)
- `BenchmarkTimerSchedule`: 25,977 -> 22,277 ns/op (-14.2%)

Top regressions:
- `BenchmarkAutoExit_FastPathExit`: 201,059 -> 224,403 ns/op (+11.6%)
- `BenchmarkMicrotaskRingIsEmpty_WithItems`: 1.95 -> 2.09 ns/op (+7.2%)

### Linux
**16 improvements, 6 regressions** (statistically significant, p<0.05)

Top improvements:
- `BenchmarkRefUnref_RWMutex_External`: 46.75 -> 34.76 ns/op (-25.6%)
- `BenchmarkAlive_WithMutexes`: 47.89 -> 37.67 ns/op (-21.4%)
- `BenchmarkSubmitInternal_FastPath_OnLoop`: 6,421 -> 5,263 ns/op (-18.0%)

Top regressions:
- `BenchmarkRefUnref_SubmitInternal_External`: 275 -> 428 ns/op (+55.6%)
- `BenchmarkQuiescing_ScheduleTimer_NoAutoExit`: 46,663 -> 57,576 ns/op (+23.4%)

### Windows
**0 improvements, 0 regressions**

Windows shows no statistically significant changes because all Windows benchmarks from 2026-04-19 are absent from the 2026-04-22 comparison set (Windows had only 25 benchmarks in 2026-04-19 vs 158 in 2026-04-22). The new Windows benchmarks have no prior baseline for comparison.

## Key Finding: Darwin ARM64 Performance Advantages

**Status**: **Confirmed** — Darwin shows consistent performance advantages in timer and promise operations when compared against Linux ARM64.

**Evidence**:
- Timer operations: Darwin wins 69.2% (18/26)
- Promise operations: Darwin wins 94.4% (17/18)
- Mean ratio 0.962x across all benchmarks

**But**:
- Microtask operations favor Linux (75% win rate)
- High-variance on Linux AutoExit benchmarks raises reliability questions
- GOMAXPROCS differs (Windows uses 16 vs 10 for others)

**Production impact**: For timer-heavy and promise-heavy workloads, Darwin ARM64 shows measurably better performance. For microtask-heavy workloads, Linux may be preferred.
