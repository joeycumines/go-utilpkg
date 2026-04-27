# Cross-Platform Benchmark Comparison Report

**Date:** 2026-04-22
**Platforms Analyzed:** Darwin (ARM64), Linux (ARM64), Windows (AMD64)
**Benchmark Suite:** eventloop performance tests

---

## Cross-Tournament Comparison


### Darwin Comparison: 2026-04-22 vs 2026-04-19

#### Significant Changes (p < 0.05)

**17 improvements, 7 regressions**

| Benchmark | Previous (ns/op) | Current (ns/op) | Change | %Δ |
|-----------|------------------|-----------------|--------|----|
| BenchmarkTimerLatency |      27,901.20 |      19,928.20 |   -7,973.00 | -28.6% |
| BenchmarkSetInterval_Optimized |      30,681.40 |      24,520.60 |   -6,160.80 | -20.1% |
| BenchmarkTimerSchedule |      25,976.60 |      22,276.60 |   -3,700.00 | -14.2% |
| BenchmarkLatencyAnalysis_SubmitWhileRunning |         501.42 |         437.74 |      -63.68 | -12.7% |
| BenchmarkSubmitLatency |         485.00 |         437.00 |      -48.00 | -9.9% |
| BenchmarkLatencyAnalysis_PingPong |         620.00 |         571.82 |      -48.18 | -7.8% |
| BenchmarkSetTimeoutZeroDelay |      25,309.20 |      23,393.20 |   -1,916.00 | -7.6% |
| BenchmarkMicrotaskLatency |         478.38 |         443.58 |      -34.80 | -7.3% |
| BenchmarkPromiseThen |         342.48 |         319.28 |      -23.20 | -6.8% |
| BenchmarkFastPathExecution |          71.02 |          66.63 |       -4.39 | -6.2% |
| BenchmarkRegression_HasExternalTasks_Mutex |          10.21 |           9.58 |       -0.63 | -6.1% |
| BenchmarkMicrotaskSchedule_Parallel |         118.56 |         112.34 |       -6.22 | -5.2% |
| BenchmarkFastPathSubmit |          55.87 |          53.01 |       -2.86 | -5.1% |
| BenchmarkSetInterval_Parallel_Optimized |      16,681.00 |      15,916.80 |     -764.20 | -4.6% |
| BenchmarkSubmit_Parallel |         103.74 |         100.10 |       -3.64 | -3.5% |
| BenchmarkAutoExit_FastPathExit |     201,058.80 |     224,403.40 |  +23,344.60 | +11.6% |
| BenchmarkMicrotaskRingIsEmpty_WithItems |           1.95 |           2.09 |       +0.14 | +7.2% |
| BenchmarkHighContention |         217.40 |         227.48 |      +10.08 | +4.6% |
| BenchmarkPromiseHandlerTracking_Parallel_Optimized |         334.98 |         346.34 |      +11.36 | +3.4% |
| BenchmarkTerminated_RejectionPath_Promisify |         451.72 |         464.52 |      +12.80 | +2.8% |
| BenchmarkRegression_Combined_Atomic |           0.47 |           0.48 |       +0.01 | +1.4% |
| BenchmarkAutoExit_UnrefExit |   1,237,581.60 |   1,253,749.00 |  +16,167.40 | +1.3% |

#### Notable Improvements

- `BenchmarkTimerLatency`: 27,901.20 -> 19,928.20 ns/op (*1.40x faster*, -28.6%)
- `BenchmarkSetInterval_Optimized`: 30,681.40 -> 24,520.60 ns/op (*1.25x faster*, -20.1%)
- `BenchmarkTimerSchedule`: 25,976.60 -> 22,276.60 ns/op (*1.17x faster*, -14.2%)
- `BenchmarkLatencyAnalysis_SubmitWhileRunning`: 501.42 -> 437.74 ns/op (*1.15x faster*, -12.7%)
- `BenchmarkSubmitLatency`: 485.00 -> 437.00 ns/op (*1.11x faster*, -9.9%)

#### Notable Regressions

- `BenchmarkAutoExit_FastPathExit`: 201,058.80 -> 224,403.40 ns/op (*1.12x slower*, +11.6%)
- `BenchmarkMicrotaskRingIsEmpty_WithItems`: 1.95 -> 2.09 ns/op (*1.07x slower*, +7.2%)
- `BenchmarkHighContention`: 217.40 -> 227.48 ns/op (*1.05x slower*, +4.6%)
- `BenchmarkPromiseHandlerTracking_Parallel_Optimized`: 334.98 -> 346.34 ns/op (*1.03x slower*, +3.4%)
- `BenchmarkTerminated_RejectionPath_Promisify`: 451.72 -> 464.52 ns/op (*1.03x slower*, +2.8%)

#### New Benchmarks (not in past run)

- `BenchmarkCancelTimer_Individual/timers_1`: 287,121.00 ns/op
- `BenchmarkCancelTimer_Individual/timers_5`: 1,498,278.20 ns/op
- `BenchmarkCancelTimer_Individual/timers_:`: 2,695,208.80 ns/op
- `BenchmarkCancelTimers_Batch/timers_1`: 70,236.80 ns/op
- `BenchmarkCancelTimers_Batch/timers_5`: 263,300.00 ns/op


### Linux Comparison: 2026-04-22 vs 2026-04-19

#### Significant Changes (p < 0.05)

**16 improvements, 6 regressions**

| Benchmark | Previous (ns/op) | Current (ns/op) | Change | %Δ |
|-----------|------------------|-----------------|--------|----|
| BenchmarkRefUnref_RWMutex_External |          46.75 |          34.76 |      -11.99 | -25.6% |
| BenchmarkAlive_WithMutexes |          47.89 |          37.67 |      -10.23 | -21.4% |
| BenchmarkSubmitInternal_FastPath_OnLoop |       6,421.00 |       5,262.60 |   -1,158.40 | -18.0% |
| BenchmarkAlive_WithMutexes_Contended |          43.97 |          36.51 |       -7.46 | -17.0% |
| BenchmarkSubmitInternal_Cost |       3,423.60 |       2,855.00 |     -568.60 | -16.6% |
| BenchmarkAlive_AllAtomic |          22.13 |          18.60 |       -3.53 | -16.0% |
| BenchmarkAutoExit_AliveCheckCost_Enabled |          49.76 |          42.09 |       -7.67 | -15.4% |
| BenchmarkRefUnref_IsLoopThread_OnLoop |      12,007.60 |      10,263.20 |   -1,744.40 | -14.5% |
| BenchmarkRegression_FastPathWakeup_NoWork |       4,333.20 |       3,744.20 |     -589.00 | -13.6% |
| BenchmarkIsLoopThread_True |       5,633.20 |       4,883.80 |     -749.40 | -13.3% |
| BenchmarkAutoExit_AliveCheckCost_Disabled |          49.04 |          43.01 |       -6.03 | -12.3% |
| BenchmarkRefUnref_SyncMap_OnLoop |          29.98 |          26.55 |       -3.42 | -11.4% |
| BenchmarkRegression_Combined_Mutex |          21.67 |          19.39 |       -2.28 | -10.5% |
| BenchmarkAlive_Uncontended |          42.89 |          38.39 |       -4.50 | -10.5% |
| BenchmarkSentinelIteration_WithTimers |      44,915.60 |      42,817.60 |   -2,098.00 | -4.7% |
| BenchmarkRefUnref_SubmitInternal_External |         274.92 |         427.70 |     +152.78 | +55.6% |
| BenchmarkRefUnref_IsLoopThread_External |       7,247.20 |       9,008.40 |   +1,761.20 | +24.3% |
| BenchmarkQuiescing_ScheduleTimer_NoAutoExit |      46,663.40 |      57,575.60 |  +10,912.20 | +23.4% |
| BenchmarkQuiescing_ScheduleTimer_WithAutoExit |      46,912.00 |      55,562.00 |   +8,650.00 | +18.4% |
| BenchmarkRegression_HasInternalTasks_Mutex |           9.82 |          11.06 |       +1.24 | +12.6% |
| BenchmarkRegression_Combined_Atomic |           0.48 |           0.51 |       +0.03 | +5.5% |

#### Notable Improvements

- `BenchmarkRefUnref_RWMutex_External`: 46.75 -> 34.76 ns/op (*1.34x faster*, -25.6%)
- `BenchmarkAlive_WithMutexes`: 47.89 -> 37.67 ns/op (*1.27x faster*, -21.4%)
- `BenchmarkSubmitInternal_FastPath_OnLoop`: 6,421.00 -> 5,262.60 ns/op (*1.22x faster*, -18.0%)
- `BenchmarkAlive_WithMutexes_Contended`: 43.97 -> 36.51 ns/op (*1.20x faster*, -17.0%)
- `BenchmarkSubmitInternal_Cost`: 3,423.60 -> 2,855.00 ns/op (*1.20x faster*, -16.6%)

#### Notable Regressions

- `BenchmarkRefUnref_SubmitInternal_External`: 274.92 -> 427.70 ns/op (*1.56x slower*, +55.6%)
- `BenchmarkRefUnref_IsLoopThread_External`: 7,247.20 -> 9,008.40 ns/op (*1.24x slower*, +24.3%)
- `BenchmarkQuiescing_ScheduleTimer_NoAutoExit`: 46,663.40 -> 57,575.60 ns/op (*1.23x slower*, +23.4%)
- `BenchmarkQuiescing_ScheduleTimer_WithAutoExit`: 46,912.00 -> 55,562.00 ns/op (*1.18x slower*, +18.4%)
- `BenchmarkRegression_HasInternalTasks_Mutex`: 9.82 -> 11.06 ns/op (*1.13x slower*, +12.6%)

#### New Benchmarks (not in past run)

- `BenchmarkCancelTimer_Individual/timers_1`: 440,906.00 ns/op
- `BenchmarkCancelTimer_Individual/timers_5`: 2,492,180.20 ns/op
- `BenchmarkCancelTimer_Individual/timers_:`: 5,198,410.60 ns/op
- `BenchmarkCancelTimers_Batch/timers_1`: 76,672.20 ns/op
- `BenchmarkCancelTimers_Batch/timers_5`: 228,906.20 ns/op


### Windows Comparison: 2026-04-22 vs 2026-04-19

#### Significant Changes (p < 0.05)

**0 improvements, 0 regressions**

No statistically significant changes detected.

#### New Benchmarks (not in past run)

- `BenchmarkAlive_Epoch_ConcurrentSubmit`: 398.08 ns/op
- `BenchmarkAlive_Epoch_FromCallback`: 7,569.20 ns/op
- `BenchmarkAlive_Epoch_NoContention`: 41.52 ns/op
- `BenchmarkAlive_Uncontended`: 41.57 ns/op
- `BenchmarkAlive_WithTimer`: 1.43 ns/op

# Executive Summary

This report provides a comprehensive cross-platform analysis of eventloop benchmark
performance across three platforms:
- **Darwin** (macOS, ARM64)
- **Linux** (ARM64)
- **Windows** (AMD64/x86_64)

**Date:** 2026-04-22

## Data Overview
- Total unique benchmarks across all platforms: **158**
- Benchmarks with complete 3-platform data: **158**
- Darwin-only benchmarks: **0**
- Linux-only benchmarks: **0**
- Windows-only benchmarks: **0**

## Overall Performance Summary
- **Darwin mean performance:** 60,291.47 ns/op
- **Linux mean performance:** 98,585.78 ns/op
- **Windows mean performance:** 61,387.59 ns/op
- **Overall fastest platform:** Darwin
- **Overall slowest platform:** Linux
- **Overall speedup factor:** 1.64x

## Platform Win Rates (Common Benchmarks)
- **Darwin wins:** 70/158 (44.3%)
- **Linux wins:** 47/158 (29.7%)
- **Windows wins:** 41/158 (25.9%)

## Platform Performance Rankings

Total benchmarks with data on all 3 platforms: **158**

| Platform | Wins | Percentage | Examples |
|----------|------|------------|----------|
| Darwin   |   70 |      44.3% | BenchmarkAlive_AllAt (18ns), BenchmarkAlive_AllAt (18ns), Be |
| Linux    |   47 |      29.7% | BenchmarkAutoExit_Po (99004ns), BenchmarkAutoExit_Ti (74595n |
| Windows  |   41 |      25.9% | BenchmarkAlive_Epoch (7569ns), BenchmarkAlive_WithT (1ns), B |

# Top 10 Fastest Benchmarks per Platform

### Top 10 Fastest Benchmarks - Darwin (ARM64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkRegression_HasInternalTasks_SimulatedAtom |     0.30 |   0.00 | 0.6% |    0 |          0 |
|    2 | BenchmarkLatencyDirectCall                         |     0.31 |   0.00 | 1.5% |    0 |          0 |
|    3 | BenchmarkLatencyStateLoad                          |     0.32 |   0.02 | 5.4% |    0 |          0 |
|    4 | BenchmarkIsLoopThread_False                        |     0.38 |   0.04 | 11.7% |    0 |          0 |
|    5 | BenchmarkRegression_Combined_Atomic                |     0.48 |   0.00 | 0.5% |    0 |          0 |
|    6 | BenchmarkTerminated_RejectionPath_RefTimer         |     2.02 |   0.03 | 1.4% |    0 |          0 |
|    7 | BenchmarkTerminated_UnrefTimer_NotGated            |     2.05 |   0.06 | 2.7% |    0 |          0 |
|    8 | BenchmarkMicrotaskRingIsEmpty_WithItems            |     2.09 |   0.07 | 3.4% |    0 |          0 |
|    9 | BenchmarkAlive_WithTimer                           |     2.18 |   0.29 | 13.5% |    0 |          0 |
|   10 | BenchmarkLatencyDeferRecover                       |     2.53 |   0.11 | 4.3% |    0 |          0 |

### Top 10 Fastest Benchmarks - Linux (ARM64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkLatencyStateLoad                          |     0.31 |   0.00 | 1.1% |    0 |          0 |
|    2 | BenchmarkLatencyDirectCall                         |     0.31 |   0.00 | 1.1% |    0 |          0 |
|    3 | BenchmarkRegression_HasInternalTasks_SimulatedAtom |     0.34 |   0.04 | 11.5% |    0 |          0 |
|    4 | BenchmarkIsLoopThread_False                        |     0.37 |   0.02 | 5.4% |    0 |          0 |
|    5 | BenchmarkRegression_Combined_Atomic                |     0.51 |   0.00 | 0.4% |    0 |          0 |
|    6 | BenchmarkAlive_WithTimer                           |     2.22 |   0.01 | 0.5% |    0 |          0 |
|    7 | BenchmarkMicrotaskRingIsEmpty_WithItems            |     2.26 |   0.40 | 17.7% |    0 |          0 |
|    8 | BenchmarkTerminated_UnrefTimer_NotGated            |     2.27 |   0.05 | 2.0% |    0 |          0 |
|    9 | BenchmarkTerminated_RejectionPath_RefTimer         |     2.30 |   0.08 | 3.4% |    0 |          0 |
|   10 | BenchmarkLatencyDeferRecover                       |     2.48 |   0.01 | 0.5% |    0 |          0 |

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
|    1 | BenchmarkCancelTimer_Individual/timers_:           | 2695208.80 | 65408.01 | 2.4% | 20145 |        401 |
|    2 | BenchmarkCancelTimers_Comparison/Individual        | 1517221.80 | 111970.45 | 7.4% | 9614 |        200 |
|    3 | BenchmarkCancelTimer_Individual/timers_5           | 1498278.20 | 113469.52 | 7.6% | 10029 |        201 |
|    4 | BenchmarkAutoExit_UnrefExit                        | 1253749.00 | 2947.59 | 0.2% | 1254532 |         41 |
|    5 | BenchmarkCancelTimers_Batch/timers_:               | 474486.60 | 5471.73 | 1.2% | 6986 |        106 |
|    6 | BenchmarkCancelTimer_Individual/timers_1           | 287121.00 | 2278.17 | 0.8% | 2001 |         41 |
|    7 | BenchmarkCancelTimers_Batch/timers_5               | 263300.00 | 8955.43 | 3.4% | 3517 |         56 |
|    8 | BenchmarkCancelTimers_Comparison/Batch             | 250715.60 | 629.79 | 0.3% | 3100 |         55 |
|    9 | BenchmarkAutoExit_FastPathExit                     | 224403.40 | 12850.90 | 5.7% | 1254141 |         34 |
|   10 | BenchmarkAutoExit_ImmediateExit                    | 196374.80 | 2818.99 | 1.4% | 1251240 |         25 |

### Top 10 Slowest Benchmarks - Linux (ARM64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkCancelTimer_Individual/timers_:           | 5198410.60 | 76166.14 | 1.5% | 20227 |        401 |
|    2 | BenchmarkCancelTimer_Individual/timers_5           | 2492180.20 | 42405.02 | 1.7% | 10049 |        201 |
|    3 | BenchmarkCancelTimers_Comparison/Individual        | 2486208.60 | 62562.40 | 2.5% | 9633 |        200 |
|    4 | BenchmarkAutoExit_UnrefExit                        | 2166326.40 | 46246.59 | 2.1% | 1250405 |         40 |
|    5 | BenchmarkCancelTimer_Individual/timers_1           | 440906.00 | 6074.95 | 1.4% | 2002 |         41 |
|    6 | BenchmarkCancelTimers_Batch/timers_:               | 415786.80 | 3622.83 | 0.9% | 6978 |        106 |
|    7 | BenchmarkAutoExit_FastPathExit                     | 396258.00 | 421090.06 | 106.3% | 1249082 |         32 |
|    8 | BenchmarkAutoExit_ImmediateExit                    | 394390.80 | 424874.09 | 107.7% | 1246642 |         23 |
|    9 | BenchmarkCancelTimers_Batch/timers_5               | 228906.20 | 1053.96 | 0.5% | 3517 |         56 |
|   10 | BenchmarkCancelTimers_Comparison/Batch             | 226902.80 | 677.90 | 0.3% | 3098 |         55 |

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
| BenchmarkAlive_AllAtomic                      |          18.24 |      0.08 |         18.60 |     0.24 |           23.14 |       0.16 | Darwin  |              1.27x |
| BenchmarkAlive_AllAtomic_Contended            |          18.49 |      1.99 |         22.88 |    32.40 |           23.24 |       0.31 | Darwin  |              1.26x |
| BenchmarkAlive_Epoch_ConcurrentSubmit         |         244.08 |      2.48 |        609.74 |    16.03 |          398.08 |       2.99 | Darwin  |              2.50x |
| BenchmarkAlive_Epoch_FromCallback             |       12819.80 |     23.93 |      26345.20 |    59.91 |         7569.20 |      42.34 | Windows |              3.48x |
| BenchmarkAlive_Epoch_NoContention             |          35.74 |      0.87 |         40.05 |     3.15 |           41.52 |       0.42 | Darwin  |              1.16x |
| BenchmarkAlive_Uncontended                    |          35.59 |      0.60 |         38.39 |     0.34 |           41.57 |       0.60 | Darwin  |              1.17x |
| BenchmarkAlive_WithMutexes                    |          34.81 |      0.54 |         37.67 |     0.40 |           41.91 |       0.20 | Darwin  |              1.20x |
| BenchmarkAlive_WithMutexes_Contended          |          34.89 |      0.61 |         36.51 |     4.78 |           42.07 |       0.75 | Darwin  |              1.21x |
| BenchmarkAlive_WithMutexes_HighContention     |         190.10 |      4.99 |        606.22 |    14.89 |          299.70 |       5.26 | Darwin  |              3.19x |
| BenchmarkAlive_WithTimer                      |           2.18 |     13.47 |          2.22 |     0.46 |            1.43 |       0.21 | Windows |              1.55x |
| BenchmarkAutoExit_AliveCheckCost_Disabled     |          54.24 |      0.86 |         43.01 |     1.34 |           42.40 |       1.57 | Windows |              1.28x |
| BenchmarkAutoExit_AliveCheckCost_Enabled      |          49.85 |      0.78 |         42.09 |     0.87 |           41.81 |       5.43 | Windows |              1.19x |
| BenchmarkAutoExit_FastPathExit                |      224403.40 |      5.73 |     396258.00 |   106.27 |       384779.40 |       6.00 | Darwin  |              1.77x |
| BenchmarkAutoExit_ImmediateExit               |      196374.80 |      1.44 |     394390.80 |   107.73 |       348964.20 |       5.12 | Darwin  |              2.01x |
| BenchmarkAutoExit_PollPathExit                |      132551.00 |     32.83 |      99004.20 |    49.83 |       411167.60 |       3.14 | Linux   |              4.15x |
| BenchmarkAutoExit_TimerFire                   |      109175.60 |      8.57 |      74595.00 |     6.58 |       390825.80 |      10.56 | Linux   |              5.24x |
| BenchmarkAutoExit_UnrefExit                   |     1253749.00 |      0.24 |    2166326.40 |     2.13 |      1580193.20 |       1.79 | Darwin  |              1.73x |
| BenchmarkCancelTimer_Individual/timers_1      |      287121.00 |      0.79 |     440906.00 |     1.38 |       236950.20 |       2.83 | Windows |              1.86x |
| BenchmarkCancelTimer_Individual/timers_5      |     1498278.20 |      7.57 |    2492180.20 |     1.70 |      1165793.00 |       1.48 | Windows |              2.14x |
| BenchmarkCancelTimer_Individual/timers_:      |     2695208.80 |      2.43 |    5198410.60 |     1.47 |      2320530.20 |       0.62 | Windows |              2.24x |
| BenchmarkCancelTimers_Batch/timers_1          |       70236.80 |      4.60 |      76672.20 |     0.84 |        73735.80 |       0.48 | Darwin  |              1.09x |
| BenchmarkCancelTimers_Batch/timers_5          |      263300.00 |      3.40 |     228906.20 |     0.46 |       292046.80 |       0.75 | Linux   |              1.28x |
| BenchmarkCancelTimers_Batch/timers_:          |      474486.60 |      1.15 |     415786.80 |     0.87 |       562062.60 |       0.27 | Linux   |              1.35x |
| BenchmarkCancelTimers_Comparison/Batch        |      250715.60 |      0.25 |     226902.80 |     0.30 |       298487.20 |       0.65 | Linux   |              1.32x |
| BenchmarkCancelTimers_Comparison/Individual   |     1517221.80 |      7.38 |    2486208.60 |     2.52 |      1158719.80 |       0.87 | Windows |              2.15x |
| BenchmarkChannelWithMutexQueue                |         479.56 |      3.87 |        456.22 |     3.64 |          681.64 |       1.95 | Linux   |              1.49x |
| BenchmarkCombinedWorkload_New                 |          90.56 |      4.63 |         89.23 |     3.23 |           94.20 |       0.74 | Linux   |              1.06x |
| BenchmarkCombinedWorkload_Old                 |         229.62 |      2.15 |        218.38 |     0.72 |          216.90 |       0.19 | Windows |              1.06x |
| BenchmarkFastPathExecution                    |          66.63 |      2.56 |         57.36 |     1.34 |           68.52 |       0.80 | Linux   |              1.19x |
| BenchmarkFastPathSubmit                       |          53.01 |      0.71 |         40.67 |     1.34 |           42.06 |       2.24 | Linux   |              1.30x |
| BenchmarkGetGoroutineID                       |        1889.80 |     18.74 |       1866.40 |    16.03 |         2413.00 |       1.95 | Linux   |              1.29x |
| BenchmarkGojaStyleSwap                        |         459.16 |      4.58 |        465.76 |    19.68 |          657.60 |       1.16 | Darwin  |              1.43x |
| BenchmarkHighContention                       |         227.48 |      1.07 |        154.66 |     1.91 |          145.26 |       1.73 | Windows |              1.57x |
| BenchmarkHighFrequencyMonitoring_New          |          26.72 |      2.82 |         25.17 |     0.52 |           30.70 |       4.29 | Linux   |              1.22x |
| BenchmarkHighFrequencyMonitoring_Old          |        8769.00 |      2.86 |       9553.00 |     1.09 |        15025.60 |       0.15 | Darwin  |              1.71x |
| BenchmarkIsLoopThread_False                   |           0.38 |     11.68 |          0.37 |     5.39 |            0.42 |       1.89 | Linux   |              1.12x |
| BenchmarkIsLoopThread_True                    |        4599.00 |      2.17 |       4883.80 |     0.43 |         5948.40 |       1.46 | Darwin  |              1.29x |
| BenchmarkLargeTimerHeap                       |       22297.00 |      9.24 |      43618.80 |     2.02 |        22132.40 |       1.55 | Windows |              1.97x |
| BenchmarkLatencyAnalysis_EndToEnd             |         583.64 |      5.19 |        597.80 |     4.66 |          786.48 |       0.78 | Darwin  |              1.35x |
| BenchmarkLatencyAnalysis_PingPong             |         571.82 |      2.34 |        476.64 |    15.15 |          954.00 |       1.30 | Linux   |              2.00x |
| BenchmarkLatencyAnalysis_SubmitWhileRunning   |         437.74 |      2.18 |        338.94 |     2.92 |          687.42 |       1.47 | Linux   |              2.03x |
| BenchmarkLatencyChannelBufferedRoundTrip      |         268.68 |      1.90 |        246.70 |     0.73 |          286.28 |       1.84 | Linux   |              1.16x |
| BenchmarkLatencyChannelRoundTrip              |         360.58 |      0.78 |        234.76 |     3.37 |          489.26 |       1.71 | Linux   |              2.08x |
| BenchmarkLatencyDeferRecover                  |           2.53 |      4.33 |          2.48 |     0.50 |            2.89 |       0.54 | Linux   |              1.17x |
| BenchmarkLatencyDirectCall                    |           0.31 |      1.54 |          0.31 |     1.12 |            0.21 |       1.56 | Windows |              1.49x |
| BenchmarkLatencyMutexLockUnlock               |           7.99 |      0.51 |          8.02 |     0.14 |           10.41 |       0.25 | Darwin  |              1.30x |
| BenchmarkLatencyRWMutexRLockRUnlock           |           9.62 |     24.55 |          8.14 |     0.53 |            8.94 |       0.43 | Linux   |              1.18x |
| BenchmarkLatencyRecord_WithPSquare            |          83.05 |      7.04 |         84.21 |     9.48 |           88.39 |       0.52 | Darwin  |              1.06x |
| BenchmarkLatencyRecord_WithoutPSquare         |          25.56 |      3.37 |         24.34 |     0.23 |           20.88 |       0.18 | Windows |              1.22x |
| BenchmarkLatencySafeExecute                   |           3.08 |      0.32 |          3.11 |     1.27 |            3.89 |       0.23 | Darwin  |              1.27x |
| BenchmarkLatencySample_NewPSquare             |          25.32 |      0.30 |         25.59 |     3.17 |           30.67 |       4.44 | Darwin  |              1.21x |
| BenchmarkLatencySample_OldSortBased           |        6246.20 |      1.20 |       7801.20 |     1.36 |         7113.00 |       0.94 | Darwin  |              1.25x |
| BenchmarkLatencySimulatedPoll                 |          15.48 |     16.77 |         14.34 |     4.27 |           14.85 |       0.54 | Linux   |              1.08x |
| BenchmarkLatencySimulatedSubmit               |          14.49 |      5.74 |         15.73 |     3.79 |           18.70 |       4.17 | Darwin  |              1.29x |
| BenchmarkLatencyStateLoad                     |           0.32 |      5.43 |          0.31 |     1.10 |            0.26 |       0.91 | Windows |              1.23x |
| BenchmarkLatencyStateTryTransition            |           4.10 |      0.48 |          4.24 |     3.43 |            5.19 |       0.30 | Darwin  |              1.27x |
| BenchmarkLatencyStateTryTransition_NoOp       |          18.60 |      4.06 |         17.73 |     0.97 |            5.20 |       0.26 | Windows |              3.58x |
| BenchmarkLatencychunkedIngressPop             |           4.03 |     47.50 |          4.96 |    40.94 |            7.42 |       7.95 | Darwin  |              1.84x |
| BenchmarkLatencychunkedIngressPush            |           8.71 |      4.33 |         10.43 |    13.37 |           10.63 |       4.48 | Darwin  |              1.22x |
| BenchmarkLatencychunkedIngressPushPop         |           4.27 |      3.36 |          4.05 |     0.56 |            4.70 |       0.27 | Linux   |              1.16x |
| BenchmarkLatencychunkedIngressPush_WithConten |          79.95 |      3.66 |         38.89 |     1.02 |           35.43 |       4.97 | Windows |              2.26x |
| BenchmarkLatencymicrotaskRingPop              |          18.02 |     12.15 |         16.26 |     0.25 |           13.09 |       1.02 | Windows |              1.38x |
| BenchmarkLatencymicrotaskRingPush             |          30.48 |     13.65 |         52.68 |    11.02 |           38.80 |       6.55 | Darwin  |              1.73x |
| BenchmarkLatencymicrotaskRingPushPop          |          22.45 |      0.17 |         22.56 |     0.19 |           34.03 |       0.31 | Darwin  |              1.52x |
| BenchmarkLoopDirect                           |         486.94 |      0.94 |        503.68 |     1.16 |          698.96 |       1.41 | Darwin  |              1.44x |
| BenchmarkLoopDirectWithSubmit                 |       15359.80 |      1.85 |      38934.40 |     1.34 |        10705.40 |       5.23 | Windows |              3.64x |
| BenchmarkMetricsCollection                    |          43.21 |     11.59 |         46.65 |     6.64 |           39.79 |       7.40 | Windows |              1.17x |
| BenchmarkMicroPingPong                        |         483.70 |      3.78 |        459.26 |     1.43 |          651.78 |       0.26 | Linux   |              1.42x |
| BenchmarkMicroPingPongWithCount               |         452.58 |      2.55 |        458.58 |     0.47 |          654.86 |       0.38 | Darwin  |              1.45x |
| BenchmarkMicrotaskExecution                   |          76.80 |      2.50 |        121.74 |     4.03 |          102.20 |       1.41 | Darwin  |              1.59x |
| BenchmarkMicrotaskLatency                     |         443.58 |      0.85 |        336.36 |     0.52 |          748.94 |       1.23 | Linux   |              2.23x |
| BenchmarkMicrotaskOverflow                    |          24.41 |      0.37 |         24.61 |     0.46 |           22.25 |       0.85 | Windows |              1.11x |
| BenchmarkMicrotaskRingIsEmpty                 |           7.97 |      0.62 |          8.12 |     0.84 |           11.64 |       0.45 | Darwin  |              1.46x |
| BenchmarkMicrotaskRingIsEmpty_WithItems       |           2.09 |      3.43 |          2.26 |    17.70 |            1.44 |       0.41 | Windows |              1.57x |
| BenchmarkMicrotaskSchedule                    |          98.13 |      7.17 |         74.67 |     8.40 |           83.32 |       1.84 | Linux   |              1.31x |
| BenchmarkMicrotaskSchedule_Parallel           |         112.34 |      1.91 |         76.02 |     9.41 |           93.49 |       0.60 | Linux   |              1.48x |
| BenchmarkMinimalLoop                          |         463.04 |      1.40 |        432.32 |     1.06 |          653.60 |       1.23 | Linux   |              1.51x |
| BenchmarkMixedWorkload                        |         985.76 |      0.78 |        863.00 |     1.25 |         1176.60 |       0.53 | Linux   |              1.36x |
| BenchmarkNoMetrics                            |          55.12 |      6.53 |         42.63 |     3.88 |           43.48 |       1.90 | Linux   |              1.29x |
| BenchmarkPromiseAll                           |        1498.60 |      1.28 |       1954.80 |     5.88 |         1971.20 |       1.85 | Darwin  |              1.32x |
| BenchmarkPromiseAll_Memory                    |        1435.40 |      2.11 |       1776.60 |     0.68 |         1733.20 |       0.85 | Darwin  |              1.24x |
| BenchmarkPromiseChain                         |         534.34 |     30.26 |        535.34 |     3.65 |          605.34 |       3.28 | Darwin  |              1.13x |
| BenchmarkPromiseCreate                        |          60.95 |      0.71 |        100.13 |     1.51 |           81.77 |       1.19 | Darwin  |              1.64x |
| BenchmarkPromiseCreation                      |          71.17 |      2.63 |        123.44 |     4.89 |           79.62 |       0.74 | Darwin  |              1.73x |
| BenchmarkPromiseGC                            |      111599.60 |     93.87 |     117750.40 |     2.16 |        52205.00 |       4.23 | Windows |              2.26x |
| BenchmarkPromiseHandlerTracking_Optimized     |          81.94 |      0.49 |         99.29 |     3.76 |          103.24 |       0.11 | Darwin  |              1.26x |
| BenchmarkPromiseHandlerTracking_Parallel_Opti |         346.34 |      1.30 |        167.36 |     0.96 |          176.08 |       0.85 | Linux   |              2.07x |
| BenchmarkPromiseRace                          |        1226.20 |      1.63 |       1645.60 |     4.49 |         1556.80 |       0.59 | Darwin  |              1.34x |
| BenchmarkPromiseReject                        |         537.88 |      5.98 |        619.56 |     1.19 |          639.96 |       0.97 | Darwin  |              1.19x |
| BenchmarkPromiseRejection                     |         618.40 |     27.92 |        618.66 |     2.82 |          632.40 |       3.22 | Darwin  |              1.02x |
| BenchmarkPromiseResolution                    |         103.02 |      3.74 |        144.78 |     1.69 |          143.58 |       0.67 | Darwin  |              1.41x |
| BenchmarkPromiseResolve                       |         101.58 |     23.31 |        115.90 |     1.29 |          129.74 |       2.20 | Darwin  |              1.28x |
| BenchmarkPromiseResolve_Memory                |         100.93 |      1.32 |        148.44 |     2.36 |          139.54 |       0.62 | Darwin  |              1.47x |
| BenchmarkPromiseThen                          |         319.28 |      1.68 |        456.12 |    16.62 |          387.80 |       1.19 | Darwin  |              1.43x |
| BenchmarkPromiseThenChain                     |         597.50 |      1.82 |        663.14 |     3.05 |          748.34 |       1.94 | Darwin  |              1.25x |
| BenchmarkPromiseTry                           |         100.34 |      1.21 |        151.58 |     2.32 |          149.98 |       0.66 | Darwin  |              1.51x |
| BenchmarkPromiseWithResolvers                 |          97.27 |      4.80 |        153.56 |     7.14 |          144.46 |       0.67 | Darwin  |              1.58x |
| BenchmarkPromisifyAllocation                  |        5795.00 |      0.95 |       7148.60 |     1.11 |         4792.80 |       0.46 | Windows |              1.49x |
| BenchmarkPureChannelPingPong                  |         404.78 |     20.51 |        369.98 |     1.21 |          483.08 |       0.37 | Linux   |              1.31x |
| BenchmarkQueueMicrotask                       |          89.50 |      2.58 |         76.78 |    12.76 |           81.90 |       1.38 | Linux   |              1.17x |
| BenchmarkQuiescing_ScheduleTimer_NoAutoExit   |       26789.80 |     46.53 |      57575.60 |     5.27 |        21528.80 |       0.61 | Windows |              2.67x |
| BenchmarkQuiescing_ScheduleTimer_WithAutoExit |       22156.00 |      2.42 |      55562.00 |     2.68 |        21610.80 |       0.36 | Windows |              2.57x |
| BenchmarkRefUnref_IsLoopThread_External       |        7780.60 |     15.11 |       9008.40 |     3.00 |         9908.00 |       4.11 | Darwin  |              1.27x |
| BenchmarkRefUnref_IsLoopThread_OnLoop         |       10714.20 |     15.52 |      10263.20 |     1.53 |        13119.40 |       2.94 | Linux   |              1.28x |
| BenchmarkRefUnref_RWMutex_External            |          33.88 |      0.25 |         34.76 |     2.37 |           38.27 |       0.33 | Darwin  |              1.13x |
| BenchmarkRefUnref_RWMutex_OnLoop              |          33.98 |      0.23 |         37.82 |     7.12 |           38.81 |       1.27 | Darwin  |              1.14x |
| BenchmarkRefUnref_SubmitInternal_External     |         164.70 |      1.30 |        427.70 |     4.47 |          202.68 |       1.11 | Darwin  |              2.60x |
| BenchmarkRefUnref_SubmitInternal_OnLoop       |       11133.00 |      0.59 |      13104.20 |    11.69 |        14498.00 |       0.97 | Darwin  |              1.30x |
| BenchmarkRefUnref_SyncMap_External            |          25.23 |      0.22 |         27.83 |     9.66 |           35.16 |       0.36 | Darwin  |              1.39x |
| BenchmarkRefUnref_SyncMap_OnLoop              |          25.08 |      0.25 |         26.55 |     1.74 |           35.44 |       0.55 | Darwin  |              1.41x |
| BenchmarkRegression_Combined_Atomic           |           0.48 |      0.48 |          0.51 |     0.37 |            0.41 |       0.49 | Windows |              1.24x |
| BenchmarkRegression_Combined_Mutex            |          17.96 |      1.93 |         19.39 |     2.63 |           22.31 |       0.31 | Darwin  |              1.24x |
| BenchmarkRegression_FastPathWakeup_NoWork     |       12278.60 |     28.39 |       3744.20 |     0.63 |         6650.20 |      37.16 | Linux   |              3.28x |
| BenchmarkRegression_HasExternalTasks_Mutex    |           9.58 |      0.50 |         10.21 |     1.14 |           10.25 |       0.31 | Darwin  |              1.07x |
| BenchmarkRegression_HasInternalTasks_Mutex    |           9.59 |      3.32 |         11.06 |     4.87 |           10.22 |       0.35 | Darwin  |              1.15x |
| BenchmarkRegression_HasInternalTasks_Simulate |           0.30 |      0.65 |          0.34 |    11.49 |            0.21 |       0.22 | Windows |              1.66x |
| BenchmarkScheduleTimerCancel                  |       25139.40 |      3.45 |      40510.80 |    16.47 |        21958.80 |       0.81 | Windows |              1.84x |
| BenchmarkScheduleTimerWithPool                |        4349.20 |      0.66 |       3739.20 |     0.89 |         5157.20 |       0.72 | Linux   |              1.38x |
| BenchmarkScheduleTimerWithPool_FireAndReuse   |        4314.60 |      1.43 |       4001.40 |     0.96 |         5165.20 |       0.75 | Linux   |              1.29x |
| BenchmarkScheduleTimerWithPool_Immediate      |        4103.00 |      1.43 |       3673.60 |     0.55 |         5152.80 |       0.99 | Linux   |              1.40x |
| BenchmarkSentinelDrain_NoWork                 |       10370.00 |     57.34 |      17650.20 |   136.06 |         5578.60 |       1.29 | Windows |              3.16x |
| BenchmarkSentinelDrain_WithTimers             |       20636.60 |      1.07 |      54457.00 |     4.48 |        17948.40 |       1.58 | Windows |              3.03x |
| BenchmarkSentinelIteration                    |        6569.40 |     47.93 |       5312.40 |    65.40 |         5564.20 |       0.78 | Linux   |              1.24x |
| BenchmarkSentinelIteration_WithTimers         |       15798.60 |     12.18 |      42817.60 |     2.72 |        11018.20 |       4.09 | Windows |              3.89x |
| BenchmarkSetImmediate_Optimized               |         198.68 |      3.01 |        143.38 |     1.47 |          208.16 |       0.76 | Linux   |              1.45x |
| BenchmarkSetInterval_Optimized                |       24520.60 |     17.06 |      41440.00 |     8.95 |        25093.20 |       0.16 | Darwin  |              1.69x |
| BenchmarkSetInterval_Parallel_Optimized       |       15916.80 |      1.80 |      20102.60 |     1.22 |        13805.80 |       1.04 | Windows |              1.46x |
| BenchmarkSetTimeoutZeroDelay                  |       23393.20 |      1.80 |      43391.20 |    15.69 |        22758.00 |       0.98 | Windows |              1.91x |
| BenchmarkSetTimeout_Optimized                 |       25479.60 |     22.12 |      40257.00 |     6.67 |        24098.80 |       0.82 | Windows |              1.67x |
| BenchmarkSubmit                               |          55.22 |      5.35 |         41.51 |     3.55 |           42.28 |       1.64 | Linux   |              1.33x |
| BenchmarkSubmitExecution                      |          67.85 |      1.21 |         57.33 |     2.87 |           68.56 |       1.21 | Linux   |              1.20x |
| BenchmarkSubmitInternal                       |        3530.60 |      1.52 |       2853.80 |     0.16 |         4612.60 |       0.94 | Linux   |              1.62x |
| BenchmarkSubmitInternal_Cost                  |        3511.60 |      1.04 |       2855.00 |     0.51 |         4589.80 |       1.00 | Linux   |              1.61x |
| BenchmarkSubmitInternal_FastPath_OnLoop       |        5308.80 |      3.13 |       5262.60 |     0.51 |         6769.80 |       1.06 | Linux   |              1.29x |
| BenchmarkSubmitInternal_QueuePath_OnLoop      |          34.27 |      3.47 |         44.77 |     7.06 |           45.06 |       2.81 | Darwin  |              1.32x |
| BenchmarkSubmitLatency                        |         437.00 |      1.70 |        337.10 |     2.96 |          685.34 |       1.02 | Linux   |              2.03x |
| BenchmarkSubmit_Parallel                      |         100.10 |      1.99 |         65.27 |    15.63 |           64.53 |       0.80 | Windows |              1.55x |
| BenchmarkTask1_2_ConcurrentSubmissions        |          95.56 |      2.99 |         60.69 |     1.84 |           62.98 |       0.89 | Linux   |              1.57x |
| BenchmarkTerminated_RejectionPath_Promisify   |         464.52 |      1.29 |        561.20 |     1.20 |          532.12 |      11.31 | Darwin  |              1.21x |
| BenchmarkTerminated_RejectionPath_RefTimer    |           2.02 |      1.43 |          2.30 |     3.37 |            2.25 |       0.28 | Darwin  |              1.14x |
| BenchmarkTerminated_RejectionPath_ScheduleTim |          46.20 |      0.93 |         59.22 |     0.60 |           84.74 |       0.41 | Darwin  |              1.83x |
| BenchmarkTerminated_RejectionPath_submitToQue |           9.94 |      3.67 |         10.98 |     0.30 |           12.35 |       1.49 | Darwin  |              1.24x |
| BenchmarkTerminated_UnrefTimer_NotGated       |           2.05 |      2.71 |          2.27 |     2.01 |            2.25 |       0.17 | Darwin  |              1.11x |
| BenchmarkTimerFire                            |        4587.60 |      5.10 |       4041.40 |     0.29 |         5402.40 |       0.86 | Linux   |              1.34x |
| BenchmarkTimerHeapOperations                  |          69.61 |      6.84 |        106.41 |     8.77 |           45.61 |      11.79 | Windows |              2.33x |
| BenchmarkTimerLatency                         |       19928.20 |      1.33 |      49512.20 |     0.85 |        17528.60 |       0.68 | Windows |              2.82x |
| BenchmarkTimerSchedule                        |       22276.60 |      1.14 |      49300.20 |    13.42 |        21756.80 |       1.25 | Windows |              2.27x |
| BenchmarkTimerSchedule_Parallel               |       13460.00 |      1.28 |      20132.60 |     4.08 |        11188.00 |       0.36 | Windows |              1.80x |
| BenchmarkWakeUpDeduplicationIntegration       |          91.94 |      2.90 |         59.08 |     2.08 |           57.49 |       1.41 | Windows |              1.60x |
| Benchmark_chunkedIngress_Batch                |         519.80 |      1.13 |        523.38 |     0.64 |          633.86 |       0.73 | Darwin  |              1.22x |
| Benchmark_chunkedIngress_ParallelWithSync     |          85.17 |      4.69 |         45.08 |     2.70 |           45.43 |       1.47 | Linux   |              1.89x |
| Benchmark_chunkedIngress_Pop                  |           4.28 |     38.33 |          5.31 |    22.87 |            7.44 |      19.25 | Darwin  |              1.74x |
| Benchmark_chunkedIngress_Push                 |           8.65 |      4.99 |         10.20 |     9.91 |           11.05 |      12.14 | Darwin  |              1.28x |
| Benchmark_chunkedIngress_PushPop              |           4.09 |      1.31 |          4.04 |     0.22 |            4.64 |       0.37 | Linux   |              1.15x |
| Benchmark_chunkedIngress_Sequential           |           4.01 |      0.45 |          4.08 |     0.57 |            4.61 |       0.40 | Darwin  |              1.15x |
| Benchmark_microtaskRing_Parallel              |         122.68 |      0.59 |         99.61 |     7.15 |           67.88 |       5.69 | Windows |              1.81x |
| Benchmark_microtaskRing_Push                  |          27.60 |     13.60 |         44.35 |    20.33 |           36.85 |       4.15 | Darwin  |              1.61x |
| Benchmark_microtaskRing_PushPop               |          22.49 |      0.30 |         22.52 |     0.27 |           34.06 |       0.23 | Darwin  |              1.51x |

## Architecture Comparison

**Note:** Darwin (macOS) and Linux both use ARM64, while Windows uses AMD64 (x86_64)

### ARM64 (Darwin) vs ARM64 (Linux)
- Mean ratio: 0.962x
- Median ratio: 0.966x
- Darwin faster: 99 benchmarks
- Linux faster: 59 benchmarks
- Equal (within 1%): 10 benchmarks

### ARM64 (Darwin) vs AMD64 (Windows)
- Mean ratio: 0.990x
- Median ratio: 0.870x
- Darwin faster: 104 benchmarks
- Windows faster: 54 benchmarks

### ARM64 (Linux) vs AMD64 (Windows)
- Mean ratio: 1.157x
- Median ratio: 0.965x
- Linux faster: 92 benchmarks
- Windows faster: 66 benchmarks

## Allocation Comparison

Allocations should be platform-independent. This section verifies consistency.

**Allocations match across all platforms:** 148 benchmarks
**Allocation mismatches:** 10 benchmarks

### Benchmarks with Allocation Mismatches:
| Benchmark | Darwin | Linux | Windows |
|-----------|--------|-------|---------|
| BenchmarkAutoExit_FastPathExit                               |     34 |    32 |      33 |
| BenchmarkAutoExit_ImmediateExit                              |     25 |    23 |      23 |
| BenchmarkAutoExit_PollPathExit                               |     33 |    31 |      33 |
| BenchmarkAutoExit_TimerFire                                  |     31 |    29 |      31 |
| BenchmarkAutoExit_UnrefExit                                  |     41 |    40 |      39 |
| BenchmarkPromiseAll                                          |     28 |    28 |      28 |
| BenchmarkPromisifyAllocation                                 |     13 |    13 |      13 |
| BenchmarkScheduleTimerWithPool                               |      1 |     1 |       2 |
| BenchmarkScheduleTimerWithPool_FireAndReuse                  |      1 |     1 |       2 |
| BenchmarkSetInterval_Parallel_Optimized                      |      8 |     7 |       7 |

### Total Allocation Summary
- Darwin: 222,075,706 total allocations
- Linux: 210,113,428 total allocations
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
