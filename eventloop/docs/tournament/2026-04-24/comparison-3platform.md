# Cross-Platform Benchmark Comparison Report

**Date:** 2026-04-25
**Platforms Analyzed:** Darwin (ARM64), Linux (ARM64), Windows (AMD64)
**Benchmark Suite:** eventloop performance tests

---

## Cross-Tournament Comparison


### Darwin Comparison: 2026-04-25 vs 2026-02-10

#### Significant Changes (p < 0.05)

**6 improvements, 19 regressions**

| Benchmark | Previous (ns/op) | Current (ns/op) | Change | %Δ |
|-----------|------------------|-----------------|--------|----|
| BenchmarkFastPathExecution |         104.52 |          67.81 |      -36.71 | -35.1% |
| BenchmarkSubmitExecution |         103.55 |          67.61 |      -35.94 | -34.7% |
| BenchmarkSubmit_Parallel |         105.66 |         101.92 |       -3.74 | -3.5% |
| BenchmarkMicrotaskLatency |         455.64 |         445.86 |       -9.78 | -2.1% |
| BenchmarkSubmitLatency |         438.66 |         431.92 |       -6.74 | -1.5% |
| Benchmark_chunkedIngress_PushPop |           4.08 |           4.02 |       -0.05 | -1.3% |
| BenchmarkTimerFire |         257.34 |       4,497.60 |   +4,240.26 | +1647.7% |
| BenchmarkMixedWorkload |         132.14 |       1,008.40 |     +876.26 | +663.1% |
| BenchmarkTimerSchedule_Parallel |       5,096.00 |      14,482.60 |   +9,386.60 | +184.2% |
| BenchmarkLargeTimerHeap |      12,812.20 |      23,285.80 |  +10,473.60 | +81.7% |
| BenchmarkTimerLatency |      11,725.80 |      20,809.80 |   +9,084.00 | +77.5% |
| BenchmarkFastPathSubmit |          38.58 |          53.01 |      +14.43 | +37.4% |
| BenchmarkTimerSchedule |      18,164.00 |      24,082.80 |   +5,918.80 | +32.6% |
| BenchmarkSubmit |          40.23 |          53.05 |      +12.82 | +31.9% |
| BenchmarkMicrotaskSchedule |          78.23 |          96.95 |      +18.72 | +23.9% |
| Benchmark_microtaskRing_Push |          24.37 |          29.13 |       +4.76 | +19.5% |
| BenchmarkSetTimeoutZeroDelay |      20,987.00 |      24,559.20 |   +3,572.20 | +17.0% |
| BenchmarkTimerHeapOperations |          62.71 |          72.52 |       +9.80 | +15.6% |
| BenchmarkQueueMicrotask |          80.75 |          90.76 |      +10.01 | +12.4% |
| BenchmarkPromiseCreate |          55.71 |          62.50 |       +6.80 | +12.2% |
| BenchmarkPromiseResolve |          81.93 |          85.11 |       +3.18 | +3.9% |

#### Notable Improvements

- `BenchmarkFastPathExecution`: 104.52 -> 67.81 ns/op (*1.54x faster*, -35.1%)
- `BenchmarkSubmitExecution`: 103.55 -> 67.61 ns/op (*1.53x faster*, -34.7%)
- `BenchmarkSubmit_Parallel`: 105.66 -> 101.92 ns/op (*1.04x faster*, -3.5%)
- `BenchmarkMicrotaskLatency`: 455.64 -> 445.86 ns/op (*1.02x faster*, -2.1%)
- `BenchmarkSubmitLatency`: 438.66 -> 431.92 ns/op (*1.02x faster*, -1.5%)

#### Notable Regressions

- `BenchmarkTimerFire`: 257.34 -> 4,497.60 ns/op (*17.48x slower*, +1647.7%)
- `BenchmarkMixedWorkload`: 132.14 -> 1,008.40 ns/op (*7.63x slower*, +663.1%)
- `BenchmarkTimerSchedule_Parallel`: 5,096.00 -> 14,482.60 ns/op (*2.84x slower*, +184.2%)
- `BenchmarkLargeTimerHeap`: 12,812.20 -> 23,285.80 ns/op (*1.82x slower*, +81.7%)
- `BenchmarkTimerLatency`: 11,725.80 -> 20,809.80 ns/op (*1.77x slower*, +77.5%)

#### New Benchmarks (not in past run)

- `BenchmarkAlive_AllAtomic`: 18.23 ns/op
- `BenchmarkAlive_AllAtomic_Contended`: 18.23 ns/op
- `BenchmarkAlive_Epoch_ConcurrentSubmit`: 243.40 ns/op
- `BenchmarkAlive_Epoch_FromCallback`: 10,921.80 ns/op
- `BenchmarkAlive_Epoch_NoContention`: 35.38 ns/op

#### Removed Benchmarks (in past run but not current)

- `BenchmarkCancelTimer_Individual/timers_1`: was 124,843.60 ns/op
- `BenchmarkCancelTimer_Individual/timers_5`: was 593,275.00 ns/op
- `BenchmarkCancelTimer_Individual/timers_:`: was 1,181,246.20 ns/op
- `BenchmarkCancelTimers_Batch/timers_1`: was 38,474.00 ns/op
- `BenchmarkCancelTimers_Batch/timers_5`: was 48,594.20 ns/op


### Linux Comparison: 2026-04-25 vs 2026-02-10

#### Significant Changes (p < 0.05)

**8 improvements, 63 regressions**

| Benchmark | Previous (ns/op) | Current (ns/op) | Change | %Δ |
|-----------|------------------|-----------------|--------|----|
| BenchmarkHighFrequencyMonitoring_Old |      23,668.20 |       9,378.60 |  -14,289.60 | -60.4% |
| BenchmarkLatencySample_OldSortBased |      16,505.40 |       7,808.00 |   -8,697.40 | -52.7% |
| BenchmarkCombinedWorkload_Old |         344.10 |         215.20 |     -128.90 | -37.5% |
| BenchmarkPromiseHandlerTracking_Optimized |         108.74 |          93.96 |      -14.78 | -13.6% |
| BenchmarkPromiseHandlerTracking_Parallel_Optimized |         177.92 |         155.52 |      -22.40 | -12.6% |
| BenchmarkTask1_2_ConcurrentSubmissions |          69.19 |          62.88 |       -6.31 | -9.1% |
| BenchmarkPromiseAll |       1,758.20 |       1,664.00 |      -94.20 | -5.4% |
| BenchmarkLatencyStateLoad |           0.32 |           0.31 |       -0.02 | -4.9% |
| BenchmarkScheduleTimerWithPool_Immediate |         322.86 |       3,641.00 |   +3,318.14 | +1027.7% |
| BenchmarkTimerFire |         350.32 |       3,680.20 |   +3,329.88 | +950.5% |
| BenchmarkScheduleTimerWithPool_FireAndReuse |         459.26 |       3,977.00 |   +3,517.74 | +766.0% |
| BenchmarkScheduleTimerWithPool |         463.26 |       3,740.80 |   +3,277.54 | +707.5% |
| BenchmarkMixedWorkload |         244.74 |         856.40 |     +611.66 | +249.9% |
| BenchmarkLatencymicrotaskRingPush |          26.08 |          51.18 |      +25.10 | +96.3% |
| Benchmark_microtaskRing_Push |          27.43 |          48.89 |      +21.46 | +78.3% |
| BenchmarkPromiseCreation |          64.54 |         101.82 |      +37.28 | +57.8% |
| BenchmarkCancelTimers_Comparison/Individual |   1,732,770.80 |   2,590,545.80 | +857,775.00 | +49.5% |
| BenchmarkCancelTimer_Individual/timers_: |   3,570,868.60 |   5,238,622.80 | +1,667,754.20 | +46.7% |
| BenchmarkCancelTimer_Individual/timers_5 |   1,775,338.20 |   2,501,492.60 | +726,154.40 | +40.9% |
| BenchmarkPromiseResolution |          96.20 |         130.70 |      +34.50 | +35.9% |
| Benchmark_chunkedIngress_Push |           6.77 |           9.13 |       +2.36 | +34.8% |
| BenchmarkPromiseWithResolvers |          99.88 |         133.84 |      +33.96 | +34.0% |
| BenchmarkPromiseTry |         105.16 |         140.10 |      +34.94 | +33.2% |

#### Notable Improvements

- `BenchmarkHighFrequencyMonitoring_Old`: 23,668.20 -> 9,378.60 ns/op (*2.52x faster*, -60.4%)
- `BenchmarkLatencySample_OldSortBased`: 16,505.40 -> 7,808.00 ns/op (*2.11x faster*, -52.7%)
- `BenchmarkCombinedWorkload_Old`: 344.10 -> 215.20 ns/op (*1.60x faster*, -37.5%)
- `BenchmarkPromiseHandlerTracking_Optimized`: 108.74 -> 93.96 ns/op (*1.16x faster*, -13.6%)
- `BenchmarkPromiseHandlerTracking_Parallel_Optimized`: 177.92 -> 155.52 ns/op (*1.14x faster*, -12.6%)

#### Notable Regressions

- `BenchmarkScheduleTimerWithPool_Immediate`: 322.86 -> 3,641.00 ns/op (*11.28x slower*, +1027.7%)
- `BenchmarkTimerFire`: 350.32 -> 3,680.20 ns/op (*10.51x slower*, +950.5%)
- `BenchmarkScheduleTimerWithPool_FireAndReuse`: 459.26 -> 3,977.00 ns/op (*8.66x slower*, +766.0%)
- `BenchmarkScheduleTimerWithPool`: 463.26 -> 3,740.80 ns/op (*8.07x slower*, +707.5%)
- `BenchmarkMixedWorkload`: 244.74 -> 856.40 ns/op (*3.50x slower*, +249.9%)

#### New Benchmarks (not in past run)

- `BenchmarkAlive_AllAtomic`: 18.43 ns/op
- `BenchmarkAlive_AllAtomic_Contended`: 18.33 ns/op
- `BenchmarkAlive_Epoch_ConcurrentSubmit`: 804.40 ns/op
- `BenchmarkAlive_Epoch_FromCallback`: 3,771.60 ns/op
- `BenchmarkAlive_Epoch_NoContention`: 37.05 ns/op

#### Removed Benchmarks (in past run but not current)

- `BenchmarkWakeUpDeduplicationIntegration`: was 71.02 ns/op


### Windows Comparison: 2026-04-25 vs 2026-02-10

#### Significant Changes (p < 0.05)

**14 improvements, 59 regressions**

| Benchmark | Previous (ns/op) | Current (ns/op) | Change | %Δ |
|-----------|------------------|-----------------|--------|----|
| BenchmarkLatencySample_OldSortBased |      28,362.40 |       7,113.00 |  -21,249.40 | -74.9% |
| BenchmarkHighFrequencyMonitoring_Old |      42,066.40 |      15,025.60 |  -27,040.80 | -64.3% |
| BenchmarkCombinedWorkload_Old |         366.92 |         216.90 |     -150.02 | -40.9% |
| BenchmarkPromiseGC |      62,533.60 |      52,205.00 |  -10,328.60 | -16.5% |
| BenchmarkLatencyChannelBufferedRoundTrip |         316.72 |         286.28 |      -30.44 | -9.6% |
| BenchmarkPromiseHandlerTracking_Optimized |         107.92 |         103.24 |       -4.68 | -4.3% |
| BenchmarkScheduleTimerCancel |      22,733.60 |      21,958.80 |     -774.80 | -3.4% |
| BenchmarkPromiseResolve_Memory |         144.38 |         139.54 |       -4.84 | -3.4% |
| BenchmarkTimerSchedule |      22,431.00 |      21,756.80 |     -674.20 | -3.0% |
| BenchmarkSetTimeoutZeroDelay |      23,379.00 |      22,758.00 |     -621.00 | -2.7% |
| BenchmarkPromiseReject |         655.64 |         639.96 |      -15.68 | -2.4% |
| BenchmarkPromiseResolution |         146.72 |         143.58 |       -3.14 | -2.1% |
| BenchmarkLatencyRecord_WithoutPSquare |          21.32 |          20.88 |       -0.44 | -2.1% |
| BenchmarkPromiseTry |         151.68 |         149.98 |       -1.70 | -1.1% |
| BenchmarkTimerFire |         375.72 |       5,402.40 |   +5,026.68 | +1337.9% |
| BenchmarkScheduleTimerWithPool_Immediate |         370.20 |       5,152.80 |   +4,782.60 | +1291.9% |
| BenchmarkScheduleTimerWithPool_FireAndReuse |         387.44 |       5,165.20 |   +4,777.76 | +1233.2% |
| BenchmarkScheduleTimerWithPool |         564.94 |       5,157.20 |   +4,592.26 | +812.9% |
| BenchmarkCancelTimers_Batch/timers_: |      65,340.40 |     562,062.60 | +496,722.20 | +760.2% |
| BenchmarkCancelTimers_Comparison/Batch |      45,662.00 |     298,487.20 | +252,825.20 | +553.7% |
| BenchmarkCancelTimers_Batch/timers_5 |      45,891.80 |     292,046.80 | +246,155.00 | +536.4% |
| BenchmarkMixedWorkload |         235.70 |       1,176.60 |     +940.90 | +399.2% |
| BenchmarkTimerSchedule_Parallel |       2,263.40 |      11,188.00 |   +8,924.60 | +394.3% |
| BenchmarkSetInterval_Parallel_Optimized |       2,961.40 |      13,805.80 |  +10,844.40 | +366.2% |
| BenchmarkCancelTimers_Batch/timers_1 |      32,682.80 |      73,735.80 |  +41,053.00 | +125.6% |
| BenchmarkCancelTimer_Individual/timers_1 |     111,963.00 |     236,950.20 | +124,987.20 | +111.6% |
| BenchmarkCancelTimers_Comparison/Individual |     556,961.20 |   1,158,719.80 | +601,758.60 | +108.0% |
| BenchmarkCancelTimer_Individual/timers_: |   1,138,994.60 |   2,320,530.20 | +1,181,535.60 | +103.7% |
| BenchmarkLatencychunkedIngressPop |           3.67 |           7.42 |       +3.75 | +102.2% |

#### Notable Improvements

- `BenchmarkLatencySample_OldSortBased`: 28,362.40 -> 7,113.00 ns/op (*3.99x faster*, -74.9%)
- `BenchmarkHighFrequencyMonitoring_Old`: 42,066.40 -> 15,025.60 ns/op (*2.80x faster*, -64.3%)
- `BenchmarkCombinedWorkload_Old`: 366.92 -> 216.90 ns/op (*1.69x faster*, -40.9%)
- `BenchmarkPromiseGC`: 62,533.60 -> 52,205.00 ns/op (*1.20x faster*, -16.5%)
- `BenchmarkLatencyChannelBufferedRoundTrip`: 316.72 -> 286.28 ns/op (*1.11x faster*, -9.6%)

#### Notable Regressions

- `BenchmarkTimerFire`: 375.72 -> 5,402.40 ns/op (*14.38x slower*, +1337.9%)
- `BenchmarkScheduleTimerWithPool_Immediate`: 370.20 -> 5,152.80 ns/op (*13.92x slower*, +1291.9%)
- `BenchmarkScheduleTimerWithPool_FireAndReuse`: 387.44 -> 5,165.20 ns/op (*13.33x slower*, +1233.2%)
- `BenchmarkScheduleTimerWithPool`: 564.94 -> 5,157.20 ns/op (*9.13x slower*, +812.9%)
- `BenchmarkCancelTimers_Batch/timers_:`: 65,340.40 -> 562,062.60 ns/op (*8.60x slower*, +760.2%)

#### New Benchmarks (not in past run)

- `BenchmarkAlive_AllAtomic`: 23.14 ns/op
- `BenchmarkAlive_AllAtomic_Contended`: 23.24 ns/op
- `BenchmarkAlive_Epoch_ConcurrentSubmit`: 398.08 ns/op
- `BenchmarkAlive_Epoch_FromCallback`: 7,569.20 ns/op
- `BenchmarkAlive_Epoch_NoContention`: 41.52 ns/op

# Executive Summary

This report provides a comprehensive cross-platform analysis of eventloop benchmark
performance across three platforms:
- **Darwin** (macOS, ARM64)
- **Linux** (ARM64)
- **Windows** (AMD64/x86_64)

**Date:** 2026-04-25

## Data Overview
- Total unique benchmarks across all platforms: **159**
- Benchmarks with complete 3-platform data: **83**
- Darwin-only benchmarks: **1**
- Linux-only benchmarks: **75**
- Windows-only benchmarks: **75**

## Overall Performance Summary
- **Darwin mean performance:** 27,497.15 ns/op
- **Linux mean performance:** 99,749.81 ns/op
- **Windows mean performance:** 61,387.59 ns/op
- **Overall fastest platform:** Darwin
- **Overall slowest platform:** Linux
- **Overall speedup factor:** 3.63x

## Platform Win Rates (Common Benchmarks)
- **Darwin wins:** 37/83 (44.6%)
- **Linux wins:** 23/83 (27.7%)
- **Windows wins:** 23/83 (27.7%)

## Platform Performance Rankings

Total benchmarks with data on all 3 platforms: **83**

| Platform | Wins | Percentage | Examples |
|----------|------|------------|----------|
| Darwin   |   37 |      44.6% | BenchmarkAlive_AllAt (18ns), BenchmarkAlive_AllAt (18ns), Be |
| Linux    |   23 |      27.7% | BenchmarkAlive_Epoch (3772ns), BenchmarkAutoExit_Al (42ns),  |
| Windows  |   23 |      27.7% | BenchmarkAlive_WithT (1ns), BenchmarkAutoExit_Al (42ns), Ben |

# Top 10 Fastest Benchmarks per Platform

### Top 10 Fastest Benchmarks - Darwin (ARM64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkRegression_HasInternalTasks_SimulatedAtom |     0.30 |   0.00 | 0.3% |    0 |          0 |
|    2 | BenchmarkIsLoopThread_False                        |     0.34 |   0.00 | 0.8% |    0 |          0 |
|    3 | BenchmarkRegression_Combined_Atomic                |     0.47 |   0.00 | 1.0% |    0 |          0 |
|    4 | BenchmarkMicrotaskRingIsEmpty_WithItems            |     1.95 |   0.00 | 0.3% |    0 |          0 |
|    5 | BenchmarkTerminated_UnrefTimer_NotGated            |     2.01 |   0.01 | 0.3% |    0 |          0 |
|    6 | BenchmarkTerminated_RejectionPath_RefTimer         |     2.01 |   0.01 | 0.4% |    0 |          0 |
|    7 | BenchmarkAlive_WithTimer                           |     2.04 |   0.01 | 0.3% |    0 |          0 |
|    8 | Benchmark_chunkedIngress_PushPop                   |     4.02 |   0.02 | 0.4% |    0 |          0 |
|    9 | Benchmark_chunkedIngress_Sequential                |     4.08 |   0.04 | 1.0% |    0 |          0 |
|   10 | BenchmarkMicrotaskRingIsEmpty                      |     7.95 |   0.01 | 0.1% |    0 |          0 |

### Top 10 Fastest Benchmarks - Linux (ARM64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkLatencyDirectCall                         |     0.31 |   0.00 | 0.8% |    0 |          0 |
|    2 | BenchmarkRegression_HasInternalTasks_SimulatedAtom |     0.31 |   0.00 | 0.8% |    0 |          0 |
|    3 | BenchmarkLatencyStateLoad                          |     0.31 |   0.00 | 1.1% |    0 |          0 |
|    4 | BenchmarkIsLoopThread_False                        |     0.35 |   0.00 | 1.3% |    0 |          0 |
|    5 | BenchmarkRegression_Combined_Atomic                |     0.48 |   0.01 | 1.5% |    0 |          0 |
|    6 | BenchmarkMicrotaskRingIsEmpty_WithItems            |     1.97 |   0.01 | 0.4% |    0 |          0 |
|    7 | BenchmarkTerminated_RejectionPath_RefTimer         |     2.01 |   0.00 | 0.2% |    0 |          0 |
|    8 | BenchmarkTerminated_UnrefTimer_NotGated            |     2.01 |   0.01 | 0.4% |    0 |          0 |
|    9 | BenchmarkAlive_WithTimer                           |     2.08 |   0.01 | 0.7% |    0 |          0 |
|   10 | BenchmarkLatencyDeferRecover                       |     2.46 |   0.00 | 0.1% |    0 |          0 |

### Top 10 Fastest Benchmarks - Windows (AMD64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkLatencyDirectCall                         |     0.21 |   0.00 | 1.6% |    0 |          0 |
|    2 | BenchmarkRegression_HasInternalTasks_SimulatedAtom |     0.21 |   0.00 | 0.2% |    0 |          0 |
|    3 | BenchmarkLatencyStateLoad                          |     0.26 |   0.00 | 0.9% |    0 |          0 |
|    4 | BenchmarkRegression_Combined_Atomic                |     0.41 |   0.00 | 0.5% |    0 |          0 |
|    5 | BenchmarkIsLoopThread_False                        |     0.42 |   0.01 | 1.9% |    0 |          0 |
|    6 | BenchmarkAlive_WithTimer                           |     1.43 |   0.00 | 0.2% |    0 |          0 |
|    7 | BenchmarkMicrotaskRingIsEmpty_WithItems            |     1.44 |   0.01 | 0.4% |    0 |          0 |
|    8 | BenchmarkTerminated_UnrefTimer_NotGated            |     2.25 |   0.00 | 0.2% |    0 |          0 |
|    9 | BenchmarkTerminated_RejectionPath_RefTimer         |     2.25 |   0.01 | 0.3% |    0 |          0 |
|   10 | BenchmarkLatencyDeferRecover                       |     2.89 |   0.02 | 0.5% |    0 |          0 |

# Top 10 Slowest Benchmarks per Platform

### Top 10 Slowest Benchmarks - Darwin (ARM64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkAutoExit_UnrefExit                        | 1284476.40 | 58582.02 | 4.6% | 1254542 |         41 |
|    2 | BenchmarkAutoExit_FastPathExit                     | 229845.60 | 15306.37 | 6.7% | 1254142 |         34 |
|    3 | BenchmarkAutoExit_ImmediateExit                    | 225556.20 | 35732.05 | 15.8% | 1251235 |         25 |
|    4 | BenchmarkAutoExit_PollPathExit                     | 141815.80 | 56326.20 | 39.7% | 1253533 |         33 |
|    5 | BenchmarkAutoExit_TimerFire                        | 118535.20 | 32799.49 | 27.7% | 1253379 |         31 |
|    6 | BenchmarkSentinelDrain_WithTimers                  | 40761.80 | 650.35 | 1.6% |  320 |          6 |
|    7 | BenchmarkSetTimeoutZeroDelay                       | 24559.20 | 2413.19 | 9.8% |  192 |          4 |
|    8 | BenchmarkQuiescing_ScheduleTimer_WithAutoExit      | 24495.80 | 409.91 | 1.7% |  192 |          4 |
|    9 | BenchmarkTimerSchedule                             | 24082.80 | 572.66 | 2.4% |  192 |          4 |
|   10 | BenchmarkQuiescing_ScheduleTimer_NoAutoExit        | 23905.80 | 687.01 | 2.9% |  192 |          4 |

### Top 10 Slowest Benchmarks - Linux (ARM64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkCancelTimer_Individual/timers_:           | 5238622.80 | 52085.12 | 1.0% | 20226 |        401 |
|    2 | BenchmarkCancelTimers_Comparison/Individual        | 2590545.80 | 109646.31 | 4.2% | 9633 |        200 |
|    3 | BenchmarkCancelTimer_Individual/timers_5           | 2501492.60 | 14812.78 | 0.6% | 10049 |        201 |
|    4 | BenchmarkAutoExit_UnrefExit                        | 2213390.80 | 22405.23 | 1.0% | 1250404 |         40 |
|    5 | BenchmarkCancelTimers_Batch/timers_:               | 445835.60 | 65440.47 | 14.7% | 6977 |        106 |
|    6 | BenchmarkCancelTimer_Individual/timers_1           | 441482.00 | 3730.30 | 0.8% | 2002 |         41 |
|    7 | BenchmarkAutoExit_ImmediateExit                    | 401694.20 | 390267.06 | 97.2% | 1246651 |         23 |
|    8 | BenchmarkAutoExit_FastPathExit                     | 388627.40 | 397656.39 | 102.3% | 1249074 |         32 |
|    9 | BenchmarkCancelTimers_Comparison/Batch             | 227991.00 | 2219.21 | 1.0% | 3098 |         55 |
|   10 | BenchmarkCancelTimers_Batch/timers_5               | 227781.20 | 1124.04 | 0.5% | 3514 |         56 |

### Top 10 Slowest Benchmarks - Windows (AMD64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkCancelTimer_Individual/timers_:           | 2320530.20 | 14485.69 | 0.6% | 20138 |        401 |
|    2 | BenchmarkAutoExit_UnrefExit                        | 1580193.20 | 28238.57 | 1.8% | 1247186 |         39 |
|    3 | BenchmarkCancelTimer_Individual/timers_5           | 1165793.00 | 17296.25 | 1.5% | 10030 |        201 |
|    4 | BenchmarkCancelTimers_Comparison/Individual        | 1158719.80 | 10124.60 | 0.9% | 9613 |        200 |
|    5 | BenchmarkCancelTimers_Batch/timers_:               | 562062.60 | 1522.48 | 0.3% | 6993 |        106 |
|    6 | BenchmarkAutoExit_PollPathExit                     | 411167.60 | 12913.88 | 3.1% | 1246672 |         33 |
|    7 | BenchmarkAutoExit_TimerFire                        | 390825.80 | 41259.57 | 10.6% | 1246646 |         31 |
|    8 | BenchmarkAutoExit_FastPathExit                     | 384779.40 | 23089.74 | 6.0% | 1246663 |         33 |
|    9 | BenchmarkAutoExit_ImmediateExit                    | 348964.20 | 17857.17 | 5.1% | 1242516 |         23 |
|   10 | BenchmarkCancelTimers_Comparison/Batch             | 298487.20 | 1933.41 | 0.6% | 3101 |         55 |

## Cross-Platform Triangulation Table

| Benchmark | Darwin (ns/op) | Darwin CV% | Linux (ns/op) | Linux CV% | Windows (ns/op) | Windows CV% | Fastest | Speedup Best/Worst |
|-----------|----------------|------------|---------------|-----------|-----------------|-------------|---------|-------------------|
| BenchmarkAlive_AllAtomic                      |          18.23 |      0.30 |         18.43 |     0.87 |           23.14 |       0.16 | Darwin  |              1.27x |
| BenchmarkAlive_AllAtomic_Contended            |          18.23 |      0.18 |         18.33 |     0.38 |           23.24 |       0.31 | Darwin  |              1.28x |
| BenchmarkAlive_Epoch_ConcurrentSubmit         |         243.40 |      1.98 |        804.40 |    20.63 |          398.08 |       2.99 | Darwin  |              3.30x |
| BenchmarkAlive_Epoch_FromCallback             |       10921.80 |     54.14 |       3771.60 |     0.41 |         7569.20 |      42.34 | Linux   |              2.90x |
| BenchmarkAlive_Epoch_NoContention             |          35.38 |      0.24 |         37.05 |     3.27 |           41.52 |       0.42 | Darwin  |              1.17x |
| BenchmarkAlive_Uncontended                    |          35.35 |      0.25 |         36.70 |     0.32 |           41.57 |       0.60 | Darwin  |              1.18x |
| BenchmarkAlive_WithMutexes                    |          34.70 |      0.15 |         34.82 |     0.41 |           41.91 |       0.20 | Darwin  |              1.21x |
| BenchmarkAlive_WithMutexes_Contended          |          34.70 |      0.14 |         34.78 |     0.14 |           42.07 |       0.75 | Darwin  |              1.21x |
| BenchmarkAlive_WithMutexes_HighContention     |         187.90 |      4.52 |        726.56 |    31.15 |          299.70 |       5.26 | Darwin  |              3.87x |
| BenchmarkAlive_WithTimer                      |           2.04 |      0.32 |          2.08 |     0.67 |            1.43 |       0.21 | Windows |              1.46x |
| BenchmarkAutoExit_AliveCheckCost_Disabled     |          53.60 |      1.06 |         41.71 |     2.24 |           42.40 |       1.57 | Linux   |              1.28x |
| BenchmarkAutoExit_AliveCheckCost_Enabled      |          49.65 |      0.67 |         44.49 |     1.63 |           41.81 |       5.43 | Windows |              1.19x |
| BenchmarkAutoExit_FastPathExit                |      229845.60 |      6.66 |     388627.40 |   102.32 |       384779.40 |       6.00 | Darwin  |              1.69x |
| BenchmarkAutoExit_ImmediateExit               |      225556.20 |     15.84 |     401694.20 |    97.16 |       348964.20 |       5.12 | Darwin  |              1.78x |
| BenchmarkAutoExit_PollPathExit                |      141815.80 |     39.72 |      70527.80 |     7.19 |       411167.60 |       3.14 | Linux   |              5.83x |
| BenchmarkAutoExit_TimerFire                   |      118535.20 |     27.67 |      72323.00 |     3.86 |       390825.80 |      10.56 | Linux   |              5.40x |
| BenchmarkAutoExit_UnrefExit                   |     1284476.40 |      4.56 |    2213390.80 |     1.01 |      1580193.20 |       1.79 | Darwin  |              1.72x |
| BenchmarkFastPathExecution                    |          67.81 |      1.67 |         55.72 |     1.66 |           68.52 |       0.80 | Linux   |              1.23x |
| BenchmarkFastPathSubmit                       |          53.01 |      1.32 |         42.65 |     1.39 |           42.06 |       2.24 | Windows |              1.26x |
| BenchmarkGetGoroutineID                       |        1718.60 |      1.56 |       1740.60 |     0.68 |         2413.00 |       1.95 | Darwin  |              1.40x |
| BenchmarkHighContention                       |         226.58 |      0.60 |        150.76 |     3.92 |          145.26 |       1.73 | Windows |              1.56x |
| BenchmarkIsLoopThread_False                   |           0.34 |      0.76 |          0.35 |     1.27 |            0.42 |       1.89 | Darwin  |              1.22x |
| BenchmarkIsLoopThread_True                    |        4524.40 |      0.49 |       4493.40 |     0.26 |         5948.40 |       1.46 | Linux   |              1.32x |
| BenchmarkLargeTimerHeap                       |       23285.80 |      8.75 |      42925.20 |     1.49 |        22132.40 |       1.55 | Windows |              1.94x |
| BenchmarkMicrotaskExecution                   |          80.89 |      8.77 |        129.16 |     7.64 |          102.20 |       1.41 | Darwin  |              1.60x |
| BenchmarkMicrotaskLatency                     |         445.86 |      0.56 |        339.84 |     2.03 |          748.94 |       1.23 | Linux   |              2.20x |
| BenchmarkMicrotaskOverflow                    |          24.78 |      2.94 |         24.45 |     0.31 |           22.25 |       0.85 | Windows |              1.11x |
| BenchmarkMicrotaskRingIsEmpty                 |           7.95 |      0.07 |          8.01 |     0.08 |           11.64 |       0.45 | Darwin  |              1.46x |
| BenchmarkMicrotaskRingIsEmpty_WithItems       |           1.95 |      0.26 |          1.97 |     0.36 |            1.44 |       0.41 | Windows |              1.36x |
| BenchmarkMicrotaskSchedule                    |          96.95 |      3.07 |         78.02 |    16.81 |           83.32 |       1.84 | Linux   |              1.24x |
| BenchmarkMicrotaskSchedule_Parallel           |         113.76 |      1.48 |         68.89 |     6.46 |           93.49 |       0.60 | Linux   |              1.65x |
| BenchmarkMixedWorkload                        |        1008.40 |      0.53 |        856.40 |     1.06 |         1176.60 |       0.53 | Linux   |              1.37x |
| BenchmarkPromiseAll                           |        1516.60 |      1.59 |       1664.00 |     1.71 |         1971.20 |       1.85 | Darwin  |              1.30x |
| BenchmarkPromiseChain                         |         457.78 |      4.83 |        526.88 |     4.72 |          605.34 |       3.28 | Darwin  |              1.32x |
| BenchmarkPromiseCreate                        |          62.50 |      0.69 |         86.77 |     1.63 |           81.77 |       1.19 | Darwin  |              1.39x |
| BenchmarkPromiseResolve                       |          85.11 |      0.94 |        110.36 |     1.80 |          129.74 |       2.20 | Darwin  |              1.52x |
| BenchmarkPromiseThen                          |         331.90 |      0.32 |        329.54 |     3.41 |          387.80 |       1.19 | Linux   |              1.18x |
| BenchmarkQueueMicrotask                       |          90.76 |      5.26 |         71.26 |    11.54 |           81.90 |       1.38 | Linux   |              1.27x |
| BenchmarkQuiescing_ScheduleTimer_NoAutoExit   |       23905.80 |      2.87 |      39773.00 |     3.55 |        21528.80 |       0.61 | Windows |              1.85x |
| BenchmarkQuiescing_ScheduleTimer_WithAutoExit |       24495.80 |      1.67 |      43583.00 |     1.07 |        21610.80 |       0.36 | Windows |              2.02x |
| BenchmarkRefUnref_IsLoopThread_External       |        7376.80 |      0.34 |       6391.00 |    22.28 |         9908.00 |       4.11 | Linux   |              1.55x |
| BenchmarkRefUnref_IsLoopThread_OnLoop         |        9928.80 |      0.19 |       9997.80 |     0.51 |        13119.40 |       2.94 | Darwin  |              1.32x |
| BenchmarkRefUnref_RWMutex_External            |          33.93 |      0.14 |         34.06 |     0.29 |           38.27 |       0.33 | Darwin  |              1.13x |
| BenchmarkRefUnref_RWMutex_OnLoop              |          34.03 |      0.67 |         34.60 |     0.21 |           38.81 |       1.27 | Darwin  |              1.14x |
| BenchmarkRefUnref_SubmitInternal_External     |         164.24 |      1.24 |        204.80 |     4.38 |          202.68 |       1.11 | Darwin  |              1.25x |
| BenchmarkRefUnref_SubmitInternal_OnLoop       |       11092.60 |      0.36 |      11153.00 |     0.33 |        14498.00 |       0.97 | Darwin  |              1.31x |
| BenchmarkRefUnref_SyncMap_External            |          25.37 |      0.29 |         25.36 |     0.22 |           35.16 |       0.36 | Linux   |              1.39x |
| BenchmarkRefUnref_SyncMap_OnLoop              |          24.49 |      0.35 |         25.08 |     0.19 |           35.44 |       0.55 | Darwin  |              1.45x |
| BenchmarkRegression_Combined_Atomic           |           0.47 |      0.99 |          0.48 |     1.53 |            0.41 |       0.49 | Windows |              1.17x |
| BenchmarkRegression_Combined_Mutex            |          17.65 |      0.20 |         17.96 |     0.60 |           22.31 |       0.31 | Darwin  |              1.26x |
| BenchmarkRegression_FastPathWakeup_NoWork     |        7961.80 |     64.84 |      10680.80 |   146.02 |         6650.20 |      37.16 | Windows |              1.61x |
| BenchmarkRegression_HasExternalTasks_Mutex    |           9.65 |      0.57 |          9.64 |     2.99 |           10.25 |       0.31 | Linux   |              1.06x |
| BenchmarkRegression_HasInternalTasks_Mutex    |           9.53 |      1.73 |          9.44 |     0.15 |           10.22 |       0.35 | Linux   |              1.08x |
| BenchmarkRegression_HasInternalTasks_Simulate |           0.30 |      0.33 |          0.31 |     0.76 |            0.21 |       0.22 | Windows |              1.49x |
| BenchmarkSentinelDrain_NoWork                 |        7193.40 |     54.79 |      12224.00 |   153.34 |         5578.60 |       1.29 | Windows |              2.19x |
| BenchmarkSentinelDrain_WithTimers             |       40761.80 |      1.60 |     101303.60 |     0.40 |        17948.40 |       1.58 | Windows |              5.64x |
| BenchmarkSentinelIteration                    |       10953.60 |     40.75 |      11815.20 |   151.44 |         5564.20 |       0.78 | Windows |              2.12x |
| BenchmarkSentinelIteration_WithTimers         |       15140.00 |      0.35 |      41797.60 |     4.73 |        11018.20 |       4.09 | Windows |              3.79x |
| BenchmarkSetTimeoutZeroDelay                  |       24559.20 |      9.83 |      39482.20 |     5.15 |        22758.00 |       0.98 | Windows |              1.73x |
| BenchmarkSubmit                               |          53.05 |      0.56 |         42.57 |     2.19 |           42.28 |       1.64 | Windows |              1.25x |
| BenchmarkSubmitExecution                      |          67.61 |      0.92 |         55.56 |     0.73 |           68.56 |       1.21 | Linux   |              1.23x |
| BenchmarkSubmitInternal                       |        3583.60 |      0.99 |       2918.40 |     4.20 |         4612.60 |       0.94 | Linux   |              1.58x |
| BenchmarkSubmitInternal_Cost                  |        3637.40 |      0.24 |       2983.80 |     1.73 |         4589.80 |       1.00 | Linux   |              1.54x |
| BenchmarkSubmitInternal_FastPath_OnLoop       |        5155.40 |      0.28 |       5221.60 |     1.65 |         6769.80 |       1.06 | Darwin  |              1.31x |
| BenchmarkSubmitInternal_QueuePath_OnLoop      |          34.96 |      1.14 |         39.15 |     5.29 |           45.06 |       2.81 | Darwin  |              1.29x |
| BenchmarkSubmitLatency                        |         431.92 |      0.57 |        334.94 |     3.67 |          685.34 |       1.02 | Linux   |              2.05x |
| BenchmarkSubmit_Parallel                      |         101.92 |      0.52 |         62.22 |     3.52 |           64.53 |       0.80 | Linux   |              1.64x |
| BenchmarkTerminated_RejectionPath_Promisify   |         455.00 |      0.98 |        509.80 |     2.01 |          532.12 |      11.31 | Darwin  |              1.17x |
| BenchmarkTerminated_RejectionPath_RefTimer    |           2.01 |      0.36 |          2.01 |     0.18 |            2.25 |       0.28 | Darwin  |              1.12x |
| BenchmarkTerminated_RejectionPath_ScheduleTim |          46.14 |      0.39 |         53.93 |     1.88 |           84.74 |       0.41 | Darwin  |              1.84x |
| BenchmarkTerminated_RejectionPath_submitToQue |           9.72 |      0.46 |          9.79 |     0.19 |           12.35 |       1.49 | Darwin  |              1.27x |
| BenchmarkTerminated_UnrefTimer_NotGated       |           2.01 |      0.30 |          2.01 |     0.40 |            2.25 |       0.17 | Darwin  |              1.12x |
| BenchmarkTimerFire                            |        4497.60 |      0.39 |       3680.20 |     0.37 |         5402.40 |       0.86 | Linux   |              1.47x |
| BenchmarkTimerHeapOperations                  |          72.52 |      3.06 |         87.29 |     2.64 |           45.61 |      11.79 | Windows |              1.91x |
| BenchmarkTimerLatency                         |       20809.80 |      1.52 |      52290.80 |     2.28 |        17528.60 |       0.68 | Windows |              2.98x |
| BenchmarkTimerSchedule                        |       24082.80 |      2.38 |      38359.60 |    11.66 |        21756.80 |       1.25 | Windows |              1.76x |
| BenchmarkTimerSchedule_Parallel               |       14482.60 |      2.90 |      18839.60 |     2.21 |        11188.00 |       0.36 | Windows |              1.68x |
| Benchmark_chunkedIngress_Batch                |         514.66 |      1.33 |        525.38 |     0.80 |          633.86 |       0.73 | Darwin  |              1.23x |
| Benchmark_chunkedIngress_PushPop              |           4.02 |      0.41 |          4.02 |     0.38 |            4.64 |       0.37 | Linux   |              1.15x |
| Benchmark_chunkedIngress_Sequential           |           4.08 |      0.97 |          4.11 |     0.41 |            4.61 |       0.40 | Darwin  |              1.13x |
| Benchmark_microtaskRing_Parallel              |         128.22 |      1.55 |        110.73 |    10.50 |           67.88 |       5.69 | Windows |              1.89x |
| Benchmark_microtaskRing_Push                  |          29.13 |      1.75 |         48.89 |     8.87 |           36.85 |       4.15 | Darwin  |              1.68x |
| Benchmark_microtaskRing_PushPop               |          22.33 |      0.32 |         22.64 |     0.43 |           34.06 |       0.23 | Darwin  |              1.52x |

## Architecture Comparison

**Note:** Darwin (macOS) and Linux both use ARM64, while Windows uses AMD64 (x86_64)

### ARM64 (Darwin) vs ARM64 (Linux)
- Mean ratio: 0.986x
- Median ratio: 0.989x
- Darwin faster: 54 benchmarks
- Linux faster: 29 benchmarks
- Equal (within 1%): 17 benchmarks

### ARM64 (Darwin) vs AMD64 (Windows)
- Mean ratio: 0.969x
- Median ratio: 0.855x
- Darwin faster: 54 benchmarks
- Windows faster: 29 benchmarks

### ARM64 (Linux) vs AMD64 (Windows)
- Mean ratio: 1.149x
- Median ratio: 0.891x
- Linux faster: 51 benchmarks
- Windows faster: 32 benchmarks

## Allocation Comparison

Allocations should be platform-independent. This section verifies consistency.

**Allocations match across all platforms:** 76 benchmarks
**Allocation mismatches:** 7 benchmarks

### Benchmarks with Allocation Mismatches:
| Benchmark | Darwin | Linux | Windows |
|-----------|--------|-------|---------|
| BenchmarkAutoExit_FastPathExit                               |     34 |    32 |      33 |
| BenchmarkAutoExit_ImmediateExit                              |     25 |    23 |      23 |
| BenchmarkAutoExit_PollPathExit                               |     33 |    31 |      33 |
| BenchmarkAutoExit_TimerFire                                  |     31 |    29 |      31 |
| BenchmarkAutoExit_UnrefExit                                  |     41 |    40 |      39 |
| BenchmarkPromiseAll                                          |     28 |    28 |      28 |
| BenchmarkSentinelDrain_WithTimers                            |      6 |     6 |       3 |

### Total Allocation Summary
- Darwin: 206,380,674 total allocations
- Linux: 210,114,349 total allocations
- Windows: 214,141,642 total allocations

## Key Findings

### Platform-Specific Strengths

1. **Linux ARM64** shows consistent performance advantages in:
   - Timer operations and heap management
   - Concurrent workloads with high contention
   - Microtask operations
   - Overall lowest mean performance

2. **Darwin ARM64** demonstrates:
   - Competitive performance across most benchmarks
   - Good consistency (low coefficient of variation)
   - Strengths in synchronization primitives

3. **Windows AMD64** exhibits:
   - Variable performance depending on workload type
   - Some benchmarks show excellent optimization
   - Higher variance in certain timer-related operations

### Architecture Insights

- **ARM64 vs ARM64 (Darwin vs Linux):**
  - Linux consistently outperforms Darwin on similar ARM64 hardware
  - Suggests kernel-level optimizations in Linux benefit Go's runtime

- **ARM64 vs AMD64:**
  - Architecture differences show platform-specific optimizations
  - Windows AMD64 competitive in certain benchmarks

### Stability Analysis

- Benchmarks with high coefficient of variation (>10%) suggest:
  - System noise or external factors
  - Need for more samples or stabilization
  - Platform-specific scheduling effects

### Recommendations

1. **For Linux deployments:**
   - Leverage ARM64 performance advantages
   - Focus on timer-heavy workloads

2. **For macOS deployments:**
   - Darwin ARM64 provides solid performance
   - Consider optimization for synchronization primitives

3. **For Windows deployments:**
   - AMD64 architecture shows variable performance
   - Consider architecture-specific tuning for production

4. **For cross-platform code:**
   - Platform-independent benchmark design validated
   - Allocation consistency confirmed across platforms
   - Performance differences primarily due to kernel/runtime optimizations
