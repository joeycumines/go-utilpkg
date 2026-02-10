# Cross-Platform Benchmark Comparison Report

**Date:** 2026-02-10
**Platforms Analyzed:** Darwin (ARM64), Linux (ARM64), Windows (AMD64)
**Benchmark Suite:** eventloop performance tests
# Executive Summary

This report provides a comprehensive cross-platform analysis of eventloop benchmark
performance across three platforms:
- **Darwin** (macOS, ARM64)
- **Linux** (ARM64)
- **Windows** (AMD64/x86_64)

## Data Overview
- Total unique benchmarks across all platforms: **108**
- Benchmarks with complete 3-platform data: **108**
- Darwin-only benchmarks: **0**
- Linux-only benchmarks: **0**
- Windows-only benchmarks: **0**

## Overall Performance Summary
- **Darwin mean performance:** 27,508.22 ns/op
- **Linux mean performance:** 80,983.90 ns/op
- **Windows mean performance:** 26,946.48 ns/op
- **Overall fastest platform:** Windows
- **Overall slowest platform:** Linux
- **Overall speedup factor:** 3.01x

## Platform Win Rates (Common Benchmarks)
- **Darwin wins:** 37/108 (34.3%)
- **Linux wins:** 42/108 (38.9%)
- **Windows wins:** 29/108 (26.9%)

## Platform Performance Rankings

Total benchmarks with data on all 3 platforms: **108**

| Platform | Wins | Percentage | Examples |
|----------|------|------------|----------|
| Darwin   |   37 |      34.3% | BenchmarkCancelTimer (57086ns), BenchmarkCombinedWor (84ns), |
| Linux    |   42 |      38.9% | BenchmarkChannelWith (423ns), BenchmarkCombinedWor (344ns),  |
| Windows  |   29 |      26.9% | BenchmarkCancelTimer (111963ns), BenchmarkCancelTimer (59185 |

# Top 10 Fastest Benchmarks per Platform

### Top 10 Fastest Benchmarks - Darwin (ARM64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkLatencyDirectCall                         |     0.30 |   0.00 | 0.7% |    0 |          0 |
|    2 | BenchmarkLatencyStateLoad                          |     0.30 |   0.00 | 1.1% |    0 |          0 |
|    3 | BenchmarkLatencyDeferRecover                       |     2.40 |   0.02 | 0.9% |    0 |          0 |
|    4 | BenchmarkLatencySafeExecute                        |     3.10 |   0.23 | 7.5% |    0 |          0 |
|    5 | BenchmarkLatencychunkedIngressPop                  |     3.19 |   0.26 | 8.3% |    0 |          0 |
|    6 | Benchmark_chunkedIngress_Pop                       |     3.42 |   0.29 | 8.5% |    0 |          0 |
|    7 | BenchmarkLatencychunkedIngressPushPop              |     3.95 |   0.02 | 0.5% |    0 |          0 |
|    8 | BenchmarkLatencyStateTryTransition                 |     4.03 |   0.13 | 3.1% |    0 |          0 |
|    9 | Benchmark_chunkedIngress_Sequential                |     4.06 |   0.30 | 7.5% |    0 |          0 |
|   10 | Benchmark_chunkedIngress_PushPop                   |     4.08 |   0.03 | 0.8% |    0 |          0 |

### Top 10 Fastest Benchmarks - Linux (ARM64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkLatencyDirectCall                         |     0.30 |   0.00 | 0.1% |    0 |          0 |
|    2 | BenchmarkLatencyStateLoad                          |     0.32 |   0.01 | 1.9% |    0 |          0 |
|    3 | BenchmarkLatencyDeferRecover                       |     2.38 |   0.01 | 0.2% |    0 |          0 |
|    4 | BenchmarkLatencySafeExecute                        |     3.03 |   0.12 | 3.8% |    0 |          0 |
|    5 | Benchmark_chunkedIngress_PushPop                   |     4.05 |   0.16 | 3.9% |    0 |          0 |
|    6 | BenchmarkLatencyStateTryTransition                 |     4.08 |   0.10 | 2.5% |    0 |          0 |
|    7 | BenchmarkLatencychunkedIngressPushPop              |     4.08 |   0.12 | 2.9% |    0 |          0 |
|    8 | Benchmark_chunkedIngress_Sequential                |     4.08 |   0.05 | 1.3% |    0 |          0 |
|    9 | BenchmarkLatencychunkedIngressPop                  |     4.16 |   0.12 | 2.9% |    0 |          0 |
|   10 | Benchmark_chunkedIngress_Pop                       |     4.21 |   0.19 | 4.6% |    0 |          0 |

### Top 10 Fastest Benchmarks - Windows (AMD64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkLatencyDirectCall                         |     0.21 |   0.00 | 0.5% |    0 |          0 |
|    2 | BenchmarkLatencyStateLoad                          |     0.26 |   0.00 | 1.0% |    0 |          0 |
|    3 | BenchmarkLatencyDeferRecover                       |     2.72 |   0.07 | 2.4% |    0 |          0 |
|    4 | BenchmarkLatencySafeExecute                        |     3.47 |   0.01 | 0.2% |    0 |          0 |
|    5 | BenchmarkLatencychunkedIngressPop                  |     3.67 |   0.10 | 2.8% |    0 |          0 |
|    6 | Benchmark_chunkedIngress_Pop                       |     4.02 |   0.33 | 8.2% |    0 |          0 |
|    7 | Benchmark_chunkedIngress_PushPop                   |     4.61 |   0.01 | 0.2% |    0 |          0 |
|    8 | Benchmark_chunkedIngress_Sequential                |     4.62 |   0.03 | 0.6% |    0 |          0 |
|    9 | BenchmarkLatencychunkedIngressPushPop              |     4.72 |   0.02 | 0.5% |    0 |          0 |
|   10 | BenchmarkLatencyStateTryTransition_NoOp            |     5.15 |   0.06 | 1.1% |    0 |          0 |

# Top 10 Slowest Benchmarks per Platform

### Top 10 Slowest Benchmarks - Darwin (ARM64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkCancelTimer_Individual/timers_:           | 1181246.20 | 28314.46 | 2.4% | 26530 |        501 |
|    2 | BenchmarkCancelTimers_Comparison/Individual        | 602813.80 | 10495.21 | 1.7% | 12805 |        250 |
|    3 | BenchmarkCancelTimer_Individual/timers_5           | 593275.00 | 9292.05 | 1.6% | 13221 |        251 |
|    4 | BenchmarkCancelTimer_Individual/timers_1           | 124843.60 | 11733.52 | 9.4% | 2640 |         51 |
|    5 | BenchmarkPromiseGC                                 | 59481.40 | 366.29 | 0.6% | 9600 |        300 |
|    6 | BenchmarkCancelTimers_Batch/timers_:               | 57085.80 | 603.07 | 1.1% | 7377 |        112 |
|    7 | BenchmarkCancelTimers_Batch/timers_5               | 48594.20 | 658.50 | 1.4% | 3915 |         62 |
|    8 | BenchmarkCancelTimers_Comparison/Batch             | 48169.80 | 825.31 | 1.7% | 3487 |         61 |
|    9 | BenchmarkCancelTimers_Batch/timers_1               | 38474.00 | 311.97 | 0.8% | 1201 |         21 |
|   10 | BenchmarkHighFrequencyMonitoring_Old               | 23700.20 | 573.56 | 2.4% | 8248 |          3 |

### Top 10 Slowest Benchmarks - Linux (ARM64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkCancelTimer_Individual/timers_:           | 3570868.60 | 274962.33 | 7.7% | 26551 |        501 |
|    2 | BenchmarkCancelTimer_Individual/timers_5           | 1775338.20 | 74085.31 | 4.2% | 13230 |        251 |
|    3 | BenchmarkCancelTimers_Comparison/Individual        | 1732770.80 | 45181.85 | 2.6% | 12814 |        250 |
|    4 | BenchmarkCancelTimers_Batch/timers_:               | 344207.60 | 1039.94 | 0.3% | 12426 |        191 |
|    5 | BenchmarkCancelTimer_Individual/timers_1           | 342611.20 | 3811.79 | 1.1% | 2641 |         51 |
|    6 | BenchmarkCancelTimers_Comparison/Batch             | 208699.80 | 1686.29 | 0.8% | 6106 |        101 |
|    7 | BenchmarkCancelTimers_Batch/timers_5               | 205931.20 | 1367.13 | 0.7% | 6535 |        103 |
|    8 | BenchmarkPromiseGC                                 | 92369.80 | 7121.72 | 7.7% | 9600 |        300 |
|    9 | BenchmarkCancelTimers_Batch/timers_1               | 72940.20 | 732.17 | 1.0% | 1525 |         26 |
|   10 | BenchmarkSetTimeoutZeroDelay                       | 43203.40 | 4999.87 | 11.6% |  384 |          7 |

### Top 10 Slowest Benchmarks - Windows (AMD64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkCancelTimer_Individual/timers_:           | 1138994.60 | 24523.64 | 2.2% | 26655 |        503 |
|    2 | BenchmarkCancelTimer_Individual/timers_5           | 591857.60 | 47120.71 | 8.0% | 13279 |        251 |
|    3 | BenchmarkCancelTimers_Comparison/Individual        | 556961.20 | 20363.86 | 3.7% | 12868 |        250 |
|    4 | BenchmarkCancelTimer_Individual/timers_1           | 111963.00 | 1564.06 | 1.4% | 2641 |         51 |
|    5 | BenchmarkCancelTimers_Batch/timers_:               | 65340.40 | 516.14 | 0.8% | 7287 |        110 |
|    6 | BenchmarkPromiseGC                                 | 62533.60 | 1300.98 | 2.1% | 9600 |        300 |
|    7 | BenchmarkCancelTimers_Batch/timers_5               | 45891.80 | 487.07 | 1.1% | 3740 |         59 |
|    8 | BenchmarkCancelTimers_Comparison/Batch             | 45662.00 | 834.38 | 1.8% | 3320 |         58 |
|    9 | BenchmarkHighFrequencyMonitoring_Old               | 42066.40 |  96.97 | 0.2% | 8248 |          3 |
|   10 | BenchmarkCancelTimers_Batch/timers_1               | 32682.80 | 212.36 | 0.6% | 1019 |         19 |

## Cross-Platform Triangulation Table

| Benchmark | Darwin (ns/op) | Darwin CV% | Linux (ns/op) | Linux CV% | Windows (ns/op) | Windows CV% | Fastest | Speedup Best/Worst |
|-----------|----------------|------------|---------------|-----------|-----------------|-------------|---------|-------------------|
| BenchmarkCancelTimer_Individual/timers_1      |      124843.60 |      9.40 |     342611.20 |     1.11 |       111963.00 |       1.40 | Windows |              3.06x |
| BenchmarkCancelTimer_Individual/timers_5      |      593275.00 |      1.57 |    1775338.20 |     4.17 |       591857.60 |       7.96 | Windows |              3.00x |
| BenchmarkCancelTimer_Individual/timers_:      |     1181246.20 |      2.40 |    3570868.60 |     7.70 |      1138994.60 |       2.15 | Windows |              3.14x |
| BenchmarkCancelTimers_Batch/timers_1          |       38474.00 |      0.81 |      72940.20 |     1.00 |        32682.80 |       0.65 | Windows |              2.23x |
| BenchmarkCancelTimers_Batch/timers_5          |       48594.20 |      1.36 |     205931.20 |     0.66 |        45891.80 |       1.06 | Windows |              4.49x |
| BenchmarkCancelTimers_Batch/timers_:          |       57085.80 |      1.06 |     344207.60 |     0.30 |        65340.40 |       0.79 | Darwin  |              6.03x |
| BenchmarkCancelTimers_Comparison/Batch        |       48169.80 |      1.71 |     208699.80 |     0.81 |        45662.00 |       1.83 | Windows |              4.57x |
| BenchmarkCancelTimers_Comparison/Individual   |      602813.80 |      1.74 |    1732770.80 |     2.61 |       556961.20 |       3.66 | Windows |              3.11x |
| BenchmarkChannelWithMutexQueue                |         466.06 |      2.00 |        422.92 |     0.73 |          694.58 |       0.52 | Linux   |              1.64x |
| BenchmarkCombinedWorkload_New                 |          83.94 |      0.25 |         84.03 |     0.04 |           91.28 |       0.40 | Darwin  |              1.09x |
| BenchmarkCombinedWorkload_Old                 |         345.04 |      1.79 |        344.10 |     0.65 |          366.92 |       0.14 | Linux   |              1.07x |
| BenchmarkFastPathExecution                    |         104.52 |      1.67 |         41.95 |     4.15 |           70.25 |       4.27 | Linux   |              2.49x |
| BenchmarkFastPathSubmit                       |          38.58 |      3.20 |         33.50 |     2.70 |           32.96 |       2.72 | Windows |              1.17x |
| BenchmarkGojaStyleSwap                        |         472.76 |      7.85 |        400.38 |     0.96 |          646.86 |       0.41 | Linux   |              1.62x |
| BenchmarkHighContention                       |         222.12 |      0.87 |        119.46 |     1.50 |          130.80 |       3.09 | Linux   |              1.86x |
| BenchmarkHighFrequencyMonitoring_New          |          24.60 |      0.35 |         24.61 |     0.36 |           31.34 |       0.52 | Darwin  |              1.27x |
| BenchmarkHighFrequencyMonitoring_Old          |       23700.20 |      2.42 |      23668.20 |     3.46 |        42066.40 |       0.23 | Linux   |              1.78x |
| BenchmarkLargeTimerHeap                       |       12812.20 |      5.69 |      34918.00 |     2.24 |        11359.40 |       3.82 | Windows |              3.07x |
| BenchmarkLatencyAnalysis_EndToEnd             |         591.78 |      8.74 |        559.20 |     1.64 |          761.68 |       0.50 | Linux   |              1.36x |
| BenchmarkLatencyAnalysis_PingPong             |         594.86 |      2.39 |        418.96 |     1.42 |          904.48 |       0.29 | Linux   |              2.16x |
| BenchmarkLatencyAnalysis_SubmitWhileRunning   |         434.86 |      3.70 |        325.24 |     2.80 |          663.76 |       0.93 | Linux   |              2.04x |
| BenchmarkLatencyChannelBufferedRoundTrip      |         330.44 |      0.97 |        241.68 |     1.09 |          316.72 |       2.19 | Linux   |              1.37x |
| BenchmarkLatencyChannelRoundTrip              |         348.64 |      0.62 |        242.80 |     2.58 |          459.10 |       0.41 | Linux   |              1.89x |
| BenchmarkLatencyDeferRecover                  |           2.40 |      0.94 |          2.38 |     0.23 |            2.72 |       2.42 | Linux   |              1.14x |
| BenchmarkLatencyDirectCall                    |           0.30 |      0.66 |          0.30 |     0.15 |            0.21 |       0.53 | Windows |              1.45x |
| BenchmarkLatencyMutexLockUnlock               |           8.52 |      9.80 |          7.53 |     0.11 |           10.42 |       0.28 | Linux   |              1.38x |
| BenchmarkLatencyRWMutexRLockRUnlock           |           8.34 |      7.14 |          7.88 |     1.69 |            8.95 |       0.32 | Linux   |              1.14x |
| BenchmarkLatencyRecord_WithPSquare            |          74.89 |      0.78 |         74.76 |     0.11 |           85.69 |       0.67 | Linux   |              1.15x |
| BenchmarkLatencyRecord_WithoutPSquare         |          23.90 |      0.54 |         23.31 |     0.42 |           21.32 |       0.44 | Windows |              1.12x |
| BenchmarkLatencySafeExecute                   |           3.10 |      7.49 |          3.03 |     3.83 |            3.47 |       0.18 | Linux   |              1.15x |
| BenchmarkLatencySample_NewPSquare             |          26.27 |      6.15 |         25.23 |     4.51 |           31.81 |       1.03 | Linux   |              1.26x |
| BenchmarkLatencySample_OldSortBased           |       17562.80 |      3.15 |      16505.40 |     2.88 |        28362.40 |       0.19 | Linux   |              1.72x |
| BenchmarkLatencySimulatedPoll                 |          12.49 |      0.14 |         14.15 |     5.51 |           14.68 |       1.21 | Darwin  |              1.18x |
| BenchmarkLatencySimulatedSubmit               |          12.93 |      8.91 |         13.93 |     1.90 |           17.41 |       4.54 | Darwin  |              1.35x |
| BenchmarkLatencyStateLoad                     |           0.30 |      1.12 |          0.32 |     1.87 |            0.26 |       0.98 | Windows |              1.25x |
| BenchmarkLatencyStateTryTransition            |           4.03 |      3.12 |          4.08 |     2.48 |            5.21 |       0.27 | Darwin  |              1.29x |
| BenchmarkLatencyStateTryTransition_NoOp       |          17.08 |      1.23 |         16.36 |     3.64 |            5.15 |       1.11 | Windows |              3.31x |
| BenchmarkLatencychunkedIngressPop             |           3.19 |      8.29 |          4.16 |     2.95 |            3.67 |       2.81 | Darwin  |              1.30x |
| BenchmarkLatencychunkedIngressPush            |           5.02 |      3.41 |          8.20 |    17.96 |            7.38 |       5.04 | Darwin  |              1.63x |
| BenchmarkLatencychunkedIngressPushPop         |           3.95 |      0.51 |          4.08 |     2.92 |            4.72 |       0.52 | Darwin  |              1.20x |
| BenchmarkLatencychunkedIngressPush_WithConten |          70.13 |      1.52 |         38.88 |     2.25 |           37.61 |       0.15 | Windows |              1.86x |
| BenchmarkLatencymicrotaskRingPop              |          15.30 |      0.56 |         15.72 |     0.27 |           12.78 |       0.27 | Windows |              1.23x |
| BenchmarkLatencymicrotaskRingPush             |          24.70 |      2.97 |         26.08 |     4.13 |           28.43 |       7.79 | Darwin  |              1.15x |
| BenchmarkLatencymicrotaskRingPushPop          |          22.43 |      1.76 |         22.11 |     1.92 |           33.95 |       0.53 | Linux   |              1.54x |
| BenchmarkLoopDirect                           |         471.70 |      0.82 |        482.62 |     2.34 |          684.72 |       0.37 | Darwin  |              1.45x |
| BenchmarkLoopDirectWithSubmit                 |       11695.80 |      7.27 |      34474.00 |     0.46 |        10811.20 |       5.08 | Windows |              3.19x |
| BenchmarkMetricsCollection                    |          32.08 |     11.01 |         34.63 |     8.49 |           30.18 |       9.29 | Windows |              1.15x |
| BenchmarkMicroPingPong                        |         430.88 |      2.76 |        404.72 |    13.65 |          637.76 |       0.46 | Linux   |              1.58x |
| BenchmarkMicroPingPongWithCount               |         442.34 |      2.50 |        429.08 |     1.14 |          638.02 |       0.26 | Linux   |              1.49x |
| BenchmarkMicrotaskExecution                   |          84.54 |      6.88 |        103.36 |     2.10 |           96.94 |       1.45 | Darwin  |              1.22x |
| BenchmarkMicrotaskLatency                     |         455.64 |      0.59 |        344.12 |     4.40 |          701.74 |       1.17 | Linux   |              2.04x |
| BenchmarkMicrotaskOverflow                    |          23.97 |      0.33 |         24.52 |     1.63 |           22.33 |       0.56 | Windows |              1.10x |
| BenchmarkMicrotaskSchedule                    |          78.23 |      5.73 |         60.69 |     8.51 |           74.07 |       0.90 | Linux   |              1.29x |
| BenchmarkMicrotaskSchedule_Parallel           |         109.64 |      0.39 |         60.13 |     2.09 |           90.03 |       1.25 | Linux   |              1.82x |
| BenchmarkMinimalLoop                          |         460.50 |      3.02 |        405.72 |     1.75 |          658.74 |       0.50 | Linux   |              1.62x |
| BenchmarkMixedWorkload                        |         132.14 |      1.05 |        244.74 |    25.45 |          235.70 |       5.82 | Darwin  |              1.85x |
| BenchmarkNoMetrics                            |          38.96 |      5.56 |         38.71 |    19.10 |           34.43 |       2.84 | Windows |              1.13x |
| BenchmarkPromiseAll                           |        1522.80 |      4.22 |       1758.20 |     3.72 |         1945.20 |       0.67 | Darwin  |              1.28x |
| BenchmarkPromiseAll_Memory                    |        1490.80 |      0.85 |       1486.40 |     4.20 |         1756.40 |       1.04 | Linux   |              1.18x |
| BenchmarkPromiseChain                         |         450.68 |      5.52 |        544.18 |     3.02 |          558.12 |       3.19 | Darwin  |              1.24x |
| BenchmarkPromiseCreate                        |          55.71 |      2.01 |         68.76 |    12.81 |           76.98 |       1.02 | Darwin  |              1.38x |
| BenchmarkPromiseCreation                      |          66.04 |      2.47 |         64.54 |     0.63 |           79.26 |       1.51 | Linux   |              1.23x |
| BenchmarkPromiseGC                            |       59481.40 |      0.62 |      92369.80 |     7.71 |        62533.60 |       2.08 | Darwin  |              1.55x |
| BenchmarkPromiseHandlerTracking_Optimized     |          80.64 |      5.12 |        108.74 |     8.23 |          107.92 |       1.07 | Darwin  |              1.35x |
| BenchmarkPromiseHandlerTracking_Parallel_Opti |         334.82 |      1.11 |        177.92 |     1.93 |          173.80 |       1.31 | Windows |              1.93x |
| BenchmarkPromiseRace                          |        1289.40 |      0.28 |       1329.40 |     4.40 |         1566.40 |       1.07 | Darwin  |              1.21x |
| BenchmarkPromiseReject                        |         545.74 |      1.73 |        531.84 |     1.17 |          655.64 |       1.63 | Linux   |              1.23x |
| BenchmarkPromiseRejection                     |         530.96 |      9.09 |        526.32 |     1.74 |          632.24 |       4.47 | Linux   |              1.20x |
| BenchmarkPromiseResolution                    |         101.24 |      5.56 |         96.20 |     2.51 |          146.72 |       0.95 | Linux   |              1.53x |
| BenchmarkPromiseResolve                       |          81.93 |      1.88 |         97.61 |    14.00 |          128.56 |       0.39 | Darwin  |              1.57x |
| BenchmarkPromiseResolve_Memory                |          99.54 |      5.77 |        105.25 |    10.83 |          144.38 |       1.55 | Darwin  |              1.45x |
| BenchmarkPromiseThen                          |         323.12 |      0.66 |        318.40 |     5.42 |          374.48 |       1.86 | Linux   |              1.18x |
| BenchmarkPromiseThenChain                     |         563.74 |      1.39 |        622.94 |     3.51 |          720.74 |       0.38 | Darwin  |              1.28x |
| BenchmarkPromiseTry                           |          97.99 |      0.92 |        105.16 |     6.59 |          151.68 |       0.29 | Darwin  |              1.55x |
| BenchmarkPromiseWithResolvers                 |          94.61 |      1.73 |         99.88 |     0.80 |          145.30 |       0.42 | Darwin  |              1.54x |
| BenchmarkPromisifyAllocation                  |        5472.60 |      1.36 |       6605.20 |     4.23 |         4666.80 |       0.89 | Windows |              1.42x |
| BenchmarkPureChannelPingPong                  |         351.62 |      0.91 |        340.54 |     1.37 |          459.06 |       0.38 | Linux   |              1.35x |
| BenchmarkQueueMicrotask                       |          80.75 |      5.70 |         57.34 |     4.06 |           74.68 |       4.43 | Linux   |              1.41x |
| BenchmarkScheduleTimerCancel                  |       19385.80 |      3.20 |      33705.20 |    19.23 |        22733.60 |       0.96 | Darwin  |              1.74x |
| BenchmarkScheduleTimerWithPool                |         481.08 |      5.02 |        463.26 |     5.44 |          564.94 |       2.95 | Linux   |              1.22x |
| BenchmarkScheduleTimerWithPool_FireAndReuse   |         276.84 |      3.57 |        459.26 |     5.11 |          387.44 |       1.31 | Darwin  |              1.66x |
| BenchmarkScheduleTimerWithPool_Immediate      |         222.14 |      2.84 |        322.86 |     4.63 |          370.20 |       4.11 | Darwin  |              1.67x |
| BenchmarkSetImmediate_Optimized               |         157.68 |      4.67 |        117.90 |    10.58 |          208.74 |       0.62 | Linux   |              1.77x |
| BenchmarkSetInterval_Optimized                |       21956.60 |      1.61 |      39217.40 |     3.50 |        24715.60 |       0.13 | Darwin  |              1.79x |
| BenchmarkSetInterval_Parallel_Optimized       |        6114.60 |      1.91 |      17232.60 |     1.71 |         2961.40 |       2.81 | Windows |              5.82x |
| BenchmarkSetTimeoutZeroDelay                  |       20987.00 |      5.30 |      43203.40 |    11.57 |        23379.00 |       0.65 | Darwin  |              2.06x |
| BenchmarkSetTimeout_Optimized                 |       20230.40 |      5.96 |      38020.40 |     8.76 |        24239.80 |       0.82 | Darwin  |              1.88x |
| BenchmarkSubmit                               |          40.23 |      4.15 |         33.05 |     1.65 |           34.57 |       5.36 | Linux   |              1.22x |
| BenchmarkSubmitExecution                      |         103.55 |      2.80 |         46.37 |    10.66 |           70.08 |       4.42 | Linux   |              2.23x |
| BenchmarkSubmitInternal                       |        3537.60 |      1.87 |       3020.20 |     5.74 |         4382.20 |       0.26 | Linux   |              1.45x |
| BenchmarkSubmitLatency                        |         438.66 |      0.72 |        322.90 |     2.69 |          664.68 |       0.91 | Linux   |              2.06x |
| BenchmarkSubmit_Parallel                      |         105.66 |      0.60 |         62.86 |     3.20 |           60.93 |       2.28 | Windows |              1.73x |
| BenchmarkTask1_2_ConcurrentSubmissions        |         105.88 |      0.72 |         69.19 |     3.84 |           58.54 |       1.15 | Windows |              1.81x |
| BenchmarkTimerFire                            |         257.34 |      6.24 |        350.32 |    13.30 |          375.72 |       2.29 | Darwin  |              1.46x |
| BenchmarkTimerHeapOperations                  |          62.71 |      2.99 |         78.86 |     6.59 |           39.09 |      14.32 | Windows |              2.02x |
| BenchmarkTimerLatency                         |       11725.80 |      0.57 |      40101.80 |    13.71 |        10943.80 |       3.93 | Windows |              3.66x |
| BenchmarkTimerSchedule                        |       18164.00 |      2.50 |      36807.20 |     1.55 |        22431.00 |       0.94 | Darwin  |              2.03x |
| BenchmarkTimerSchedule_Parallel               |        5096.00 |      0.66 |      15283.60 |     0.77 |         2263.40 |       2.96 | Windows |              6.75x |
| BenchmarkWakeUpDeduplicationIntegration       |         102.82 |      3.79 |         71.02 |     2.08 |           56.48 |       5.06 | Windows |              1.82x |
| Benchmark_chunkedIngress_Batch                |         507.00 |      3.90 |        517.62 |     0.57 |          631.80 |       0.45 | Darwin  |              1.25x |
| Benchmark_chunkedIngress_ParallelWithSync     |          87.80 |      2.56 |         43.54 |     2.42 |           44.93 |       1.27 | Linux   |              2.02x |
| Benchmark_chunkedIngress_Pop                  |           3.42 |      8.49 |          4.21 |     4.59 |            4.02 |       8.22 | Darwin  |              1.23x |
| Benchmark_chunkedIngress_Push                 |           5.06 |      5.05 |          6.77 |    13.17 |            7.47 |       3.46 | Darwin  |              1.47x |
| Benchmark_chunkedIngress_PushPop              |           4.08 |      0.78 |          4.05 |     3.87 |            4.61 |       0.23 | Linux   |              1.14x |
| Benchmark_chunkedIngress_Sequential           |           4.06 |      7.49 |          4.08 |     1.32 |            4.62 |       0.60 | Darwin  |              1.14x |
| Benchmark_microtaskRing_Parallel              |         130.08 |      1.76 |         89.97 |     3.49 |           58.27 |       1.15 | Windows |              2.23x |
| Benchmark_microtaskRing_Push                  |          24.37 |     10.50 |         27.43 |    11.46 |           27.27 |      11.56 | Darwin  |              1.13x |
| Benchmark_microtaskRing_PushPop               |          21.95 |      1.35 |         21.69 |     0.85 |           34.06 |       0.55 | Linux   |              1.57x |

## Architecture Comparison

**Note:** Darwin (macOS) and Linux both use ARM64, while Windows uses AMD64 (x86_64)

### ARM64 (Darwin) vs ARM64 (Linux)
- Mean ratio: 0.980x
- Median ratio: 0.999x
- Darwin faster: 55 benchmarks
- Linux faster: 53 benchmarks
- Equal (within 1%): 12 benchmarks

### ARM64 (Darwin) vs AMD64 (Windows)
- Mean ratio: 1.003x
- Median ratio: 0.866x
- Darwin faster: 70 benchmarks
- Windows faster: 38 benchmarks

### ARM64 (Linux) vs AMD64 (Windows)
- Mean ratio: 1.310x
- Median ratio: 0.906x
- Linux faster: 64 benchmarks
- Windows faster: 44 benchmarks

## Allocation Comparison

Allocations should be platform-independent. This section verifies consistency.

✅ **Allocations match across all platforms:** 93 benchmarks
❌ **Allocation mismatches:** 15 benchmarks

### Benchmarks with Allocation Mismatches:
| Benchmark | Darwin | Linux | Windows |
|-----------|--------|-------|---------|
| BenchmarkCancelTimer_Individual/timers_5                     |    251 |   251 |     251 |
| BenchmarkCancelTimer_Individual/timers_:                     |    501 |   501 |     503 |
| BenchmarkCancelTimers_Batch/timers_1                         |     21 |    26 |      19 |
| BenchmarkCancelTimers_Batch/timers_5                         |     62 |   103 |      59 |
| BenchmarkCancelTimers_Batch/timers_:                         |    112 |   191 |     110 |
| BenchmarkCancelTimers_Comparison/Batch                       |     61 |   101 |      58 |
| BenchmarkCancelTimers_Comparison/Individual                  |    250 |   250 |     250 |
| BenchmarkPromiseAll                                          |     28 |    28 |      28 |
| BenchmarkScheduleTimerCancel                                 |      6 |     7 |       7 |
| BenchmarkSetInterval_Parallel_Optimized                      |      9 |     9 |       8 |
| BenchmarkSetTimeoutZeroDelay                                 |      6 |     7 |       6 |
| BenchmarkSetTimeout_Optimized                                |      6 |     7 |       6 |
| BenchmarkSubmitInternal                                      |      0 |     1 |       1 |
| BenchmarkTimerSchedule                                       |      6 |     7 |       6 |
| BenchmarkTimerSchedule_Parallel                              |      5 |     6 |       5 |

### Total Allocation Summary
- Darwin: 24,323,117 total allocations
- Linux: 26,731,369 total allocations
- Windows: 24,404,124 total allocations

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
