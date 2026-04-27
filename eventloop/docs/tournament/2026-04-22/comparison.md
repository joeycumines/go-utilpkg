# Darwin vs Linux Benchmark Comparison

**Date:** 2026-04-22
**Platforms:** Darwin ARM64 (macOS, GOMAXPROCS=10) vs Linux ARM64 (container, GOMAXPROCS=10)
**Methodology:** `go test -bench=. -benchmem -count=5 -run=^$ -benchtime=1s -timeout=10m`
**Benchmarks Compared:** 158 common benchmarks

## Executive Summary

This report compares eventloop benchmark performance between **Darwin (macOS)** and **Linux**,
both running on **ARM64** architecture. Since the architecture is identical, performance
differences reflect OS-level differences: kernel scheduling, memory management, syscall
overhead, and Go runtime behavior on each OS.

### Key Metrics

| Metric | Value |
|--------|-------|
| Common benchmarks | 158 |
| Darwin-only benchmarks | 0 |
| Linux-only benchmarks | 0 |
| Darwin wins (faster) | **99** (62.7%) |
| Linux wins (faster) | **59** (37.3%) |
| Ties | 0 |
| Statistically significant differences | 110 |
| Darwin mean (common benchmarks) | 60,291.47 ns/op |
| Linux mean (common benchmarks) | 98,585.78 ns/op |
| Mean ratio (Darwin/Linux) | 0.962x |
| Median ratio (Darwin/Linux) | 0.966x |
| Allocation match rate | 151/158 (95.6%) |
| Zero-allocation benchmarks (both) | 77 |

## Full Statistical Comparison Table

| # | Benchmark | Darwin (ns/op) | Darwin CV% | Linux (ns/op) | Linux CV% | Faster | Ratio | Sig? |
|---|-----------|----------------|------------|---------------|-----------|--------|-------|------|
| 1 | BenchmarkAlive_AllAtomic |          18.24 |       0.1% |         18.60 |      0.2% | Darwin |  0.98x | yes |
| 2 | BenchmarkAlive_AllAtomic_Contended |          18.49 |       2.0% |         22.88 |     32.4% | Darwin |  0.81x |  |
| 3 | BenchmarkAlive_Epoch_ConcurrentSubmit |         244.08 |       2.5% |        609.74 |     16.0% | Darwin |  0.40x | yes |
| 4 | BenchmarkAlive_Epoch_FromCallback |      12,819.80 |      23.9% |     26,345.20 |     59.9% | Darwin |  0.49x |  |
| 5 | BenchmarkAlive_Epoch_NoContention |          35.74 |       0.9% |         40.05 |      3.2% | Darwin |  0.89x | yes |
| 6 | BenchmarkAlive_Uncontended |          35.59 |       0.6% |         38.39 |      0.3% | Darwin |  0.93x | yes |
| 7 | BenchmarkAlive_WithMutexes |          34.81 |       0.5% |         37.67 |      0.4% | Darwin |  0.92x | yes |
| 8 | BenchmarkAlive_WithMutexes_Contended |          34.89 |       0.6% |         36.51 |      4.8% | Darwin |  0.96x |  |
| 9 | BenchmarkAlive_WithMutexes_HighContention |         190.10 |       5.0% |        606.22 |     14.9% | Darwin |  0.31x | yes |
| 10 | BenchmarkAlive_WithTimer |           2.18 |      13.5% |          2.22 |      0.5% | Darwin |  0.98x |  |
| 11 | BenchmarkAutoExit_AliveCheckCost_Disabled |          54.24 |       0.9% |         43.01 |      1.3% | Linux |  1.26x | yes |
| 12 | BenchmarkAutoExit_AliveCheckCost_Enabled |          49.85 |       0.8% |         42.09 |      0.9% | Linux |  1.18x | yes |
| 13 | BenchmarkAutoExit_FastPathExit |     224,403.40 |       5.7% |    396,258.00 |    106.3% | Darwin |  0.57x |  |
| 14 | BenchmarkAutoExit_ImmediateExit |     196,374.80 |       1.4% |    394,390.80 |    107.7% | Darwin |  0.50x |  |
| 15 | BenchmarkAutoExit_PollPathExit |     132,551.00 |      32.8% |     99,004.20 |     49.8% | Linux |  1.34x |  |
| 16 | BenchmarkAutoExit_TimerFire |     109,175.60 |       8.6% |     74,595.00 |      6.6% | Linux |  1.46x | yes |
| 17 | BenchmarkAutoExit_UnrefExit |   1,253,749.00 |       0.2% |  2,166,326.40 |      2.1% | Darwin |  0.58x | yes |
| 18 | BenchmarkCancelTimer_Individual/timers_1 |     287,121.00 |       0.8% |    440,906.00 |      1.4% | Darwin |  0.65x | yes |
| 19 | BenchmarkCancelTimer_Individual/timers_5 |   1,498,278.20 |       7.6% |  2,492,180.20 |      1.7% | Darwin |  0.60x | yes |
| 20 | BenchmarkCancelTimer_Individual/timers_: |   2,695,208.80 |       2.4% |  5,198,410.60 |      1.5% | Darwin |  0.52x | yes |
| 21 | BenchmarkCancelTimers_Batch/timers_1 |      70,236.80 |       4.6% |     76,672.20 |      0.8% | Darwin |  0.92x | yes |
| 22 | BenchmarkCancelTimers_Batch/timers_5 |     263,300.00 |       3.4% |    228,906.20 |      0.5% | Linux |  1.15x | yes |
| 23 | BenchmarkCancelTimers_Batch/timers_: |     474,486.60 |       1.2% |    415,786.80 |      0.9% | Linux |  1.14x | yes |
| 24 | BenchmarkCancelTimers_Comparison/Batch |     250,715.60 |       0.3% |    226,902.80 |      0.3% | Linux |  1.10x | yes |
| 25 | BenchmarkCancelTimers_Comparison/Individual |   1,517,221.80 |       7.4% |  2,486,208.60 |      2.5% | Darwin |  0.61x | yes |
| 26 | BenchmarkChannelWithMutexQueue |         479.56 |       3.9% |        456.22 |      3.6% | Linux |  1.05x |  |
| 27 | BenchmarkCombinedWorkload_New |          90.56 |       4.6% |         89.23 |      3.2% | Linux |  1.01x |  |
| 28 | BenchmarkCombinedWorkload_Old |         229.62 |       2.2% |        218.38 |      0.7% | Linux |  1.05x | yes |
| 29 | BenchmarkFastPathExecution |          66.63 |       2.6% |         57.36 |      1.3% | Linux |  1.16x | yes |
| 30 | BenchmarkFastPathSubmit |          53.01 |       0.7% |         40.67 |      1.3% | Linux |  1.30x | yes |
| 31 | BenchmarkGetGoroutineID |       1,889.80 |      18.7% |      1,866.40 |     16.0% | Linux |  1.01x |  |
| 32 | BenchmarkGojaStyleSwap |         459.16 |       4.6% |        465.76 |     19.7% | Darwin |  0.99x |  |
| 33 | BenchmarkHighContention |         227.48 |       1.1% |        154.66 |      1.9% | Linux |  1.47x | yes |
| 34 | BenchmarkHighFrequencyMonitoring_New |          26.72 |       2.8% |         25.17 |      0.5% | Linux |  1.06x | yes |
| 35 | BenchmarkHighFrequencyMonitoring_Old |       8,769.00 |       2.9% |      9,553.00 |      1.1% | Darwin |  0.92x | yes |
| 36 | BenchmarkIsLoopThread_False |           0.38 |      11.7% |          0.37 |      5.4% | Linux |  1.01x |  |
| 37 | BenchmarkIsLoopThread_True |       4,599.00 |       2.2% |      4,883.80 |      0.4% | Darwin |  0.94x | yes |
| 38 | BenchmarkLargeTimerHeap |      22,297.00 |       9.2% |     43,618.80 |      2.0% | Darwin |  0.51x | yes |
| 39 | BenchmarkLatencyAnalysis_EndToEnd |         583.64 |       5.2% |        597.80 |      4.7% | Darwin |  0.98x |  |
| 40 | BenchmarkLatencyAnalysis_PingPong |         571.82 |       2.3% |        476.64 |     15.2% | Linux |  1.20x | yes |
| 41 | BenchmarkLatencyAnalysis_SubmitWhileRunning |         437.74 |       2.2% |        338.94 |      2.9% | Linux |  1.29x | yes |
| 42 | BenchmarkLatencyChannelBufferedRoundTrip |         268.68 |       1.9% |        246.70 |      0.7% | Linux |  1.09x | yes |
| 43 | BenchmarkLatencyChannelRoundTrip |         360.58 |       0.8% |        234.76 |      3.4% | Linux |  1.54x | yes |
| 44 | BenchmarkLatencyDeferRecover |           2.53 |       4.3% |          2.48 |      0.5% | Linux |  1.02x |  |
| 45 | BenchmarkLatencyDirectCall |           0.31 |       1.5% |          0.31 |      1.1% | Darwin |  1.00x |  |
| 46 | BenchmarkLatencyMutexLockUnlock |           7.99 |       0.5% |          8.02 |      0.1% | Darwin |  1.00x |  |
| 47 | BenchmarkLatencyRWMutexRLockRUnlock |           9.62 |      24.6% |          8.14 |      0.5% | Linux |  1.18x |  |
| 48 | BenchmarkLatencyRecord_WithPSquare |          83.05 |       7.0% |         84.21 |      9.5% | Darwin |  0.99x |  |
| 49 | BenchmarkLatencyRecord_WithoutPSquare |          25.56 |       3.4% |         24.34 |      0.2% | Linux |  1.05x | yes |
| 50 | BenchmarkLatencySafeExecute |           3.08 |       0.3% |          3.11 |      1.3% | Darwin |  0.99x |  |
| 51 | BenchmarkLatencySample_NewPSquare |          25.32 |       0.3% |         25.59 |      3.2% | Darwin |  0.99x |  |
| 52 | BenchmarkLatencySample_OldSortBased |       6,246.20 |       1.2% |      7,801.20 |      1.4% | Darwin |  0.80x | yes |
| 53 | BenchmarkLatencySimulatedPoll |          15.48 |      16.8% |         14.34 |      4.3% | Linux |  1.08x |  |
| 54 | BenchmarkLatencySimulatedSubmit |          14.49 |       5.7% |         15.73 |      3.8% | Darwin |  0.92x |  |
| 55 | BenchmarkLatencyStateLoad |           0.32 |       5.4% |          0.31 |      1.1% | Linux |  1.03x |  |
| 56 | BenchmarkLatencyStateTryTransition |           4.10 |       0.5% |          4.24 |      3.4% | Darwin |  0.97x |  |
| 57 | BenchmarkLatencyStateTryTransition_NoOp |          18.60 |       4.1% |         17.73 |      1.0% | Linux |  1.05x |  |
| 58 | BenchmarkLatencychunkedIngressPop |           4.03 |      47.5% |          4.96 |     40.9% | Darwin |  0.81x |  |
| 59 | BenchmarkLatencychunkedIngressPush |           8.71 |       4.3% |         10.43 |     13.4% | Darwin |  0.84x |  |
| 60 | BenchmarkLatencychunkedIngressPushPop |           4.27 |       3.4% |          4.05 |      0.6% | Linux |  1.05x | yes |
| 61 | BenchmarkLatencychunkedIngressPush_WithContention |          79.95 |       3.7% |         38.89 |      1.0% | Linux |  2.06x | yes |
| 62 | BenchmarkLatencymicrotaskRingPop |          18.02 |      12.2% |         16.26 |      0.2% | Linux |  1.11x |  |
| 63 | BenchmarkLatencymicrotaskRingPush |          30.48 |      13.6% |         52.68 |     11.0% | Darwin |  0.58x | yes |
| 64 | BenchmarkLatencymicrotaskRingPushPop |          22.45 |       0.2% |         22.56 |      0.2% | Darwin |  1.00x | yes |
| 65 | BenchmarkLoopDirect |         486.94 |       0.9% |        503.68 |      1.2% | Darwin |  0.97x | yes |
| 66 | BenchmarkLoopDirectWithSubmit |      15,359.80 |       1.9% |     38,934.40 |      1.3% | Darwin |  0.39x | yes |
| 67 | BenchmarkMetricsCollection |          43.21 |      11.6% |         46.65 |      6.6% | Darwin |  0.93x |  |
| 68 | BenchmarkMicroPingPong |         483.70 |       3.8% |        459.26 |      1.4% | Linux |  1.05x | yes |
| 69 | BenchmarkMicroPingPongWithCount |         452.58 |       2.6% |        458.58 |      0.5% | Darwin |  0.99x |  |
| 70 | BenchmarkMicrotaskExecution |          76.80 |       2.5% |        121.74 |      4.0% | Darwin |  0.63x | yes |
| 71 | BenchmarkMicrotaskLatency |         443.58 |       0.8% |        336.36 |      0.5% | Linux |  1.32x | yes |
| 72 | BenchmarkMicrotaskOverflow |          24.41 |       0.4% |         24.61 |      0.5% | Darwin |  0.99x | yes |
| 73 | BenchmarkMicrotaskRingIsEmpty |           7.97 |       0.6% |          8.12 |      0.8% | Darwin |  0.98x | yes |
| 74 | BenchmarkMicrotaskRingIsEmpty_WithItems |           2.09 |       3.4% |          2.26 |     17.7% | Darwin |  0.92x |  |
| 75 | BenchmarkMicrotaskSchedule |          98.13 |       7.2% |         74.67 |      8.4% | Linux |  1.31x | yes |
| 76 | BenchmarkMicrotaskSchedule_Parallel |         112.34 |       1.9% |         76.02 |      9.4% | Linux |  1.48x | yes |
| 77 | BenchmarkMinimalLoop |         463.04 |       1.4% |        432.32 |      1.1% | Linux |  1.07x | yes |
| 78 | BenchmarkMixedWorkload |         985.76 |       0.8% |        863.00 |      1.3% | Linux |  1.14x | yes |
| 79 | BenchmarkNoMetrics |          55.12 |       6.5% |         42.63 |      3.9% | Linux |  1.29x | yes |
| 80 | BenchmarkPromiseAll |       1,498.60 |       1.3% |      1,954.80 |      5.9% | Darwin |  0.77x | yes |
| 81 | BenchmarkPromiseAll_Memory |       1,435.40 |       2.1% |      1,776.60 |      0.7% | Darwin |  0.81x | yes |
| 82 | BenchmarkPromiseChain |         534.34 |      30.3% |        535.34 |      3.7% | Darwin |  1.00x |  |
| 83 | BenchmarkPromiseCreate |          60.95 |       0.7% |        100.13 |      1.5% | Darwin |  0.61x | yes |
| 84 | BenchmarkPromiseCreation |          71.17 |       2.6% |        123.44 |      4.9% | Darwin |  0.58x | yes |
| 85 | BenchmarkPromiseGC |     111,599.60 |      93.9% |    117,750.40 |      2.2% | Darwin |  0.95x |  |
| 86 | BenchmarkPromiseHandlerTracking_Optimized |          81.94 |       0.5% |         99.29 |      3.8% | Darwin |  0.83x | yes |
| 87 | BenchmarkPromiseHandlerTracking_Parallel_Optimized |         346.34 |       1.3% |        167.36 |      1.0% | Linux |  2.07x | yes |
| 88 | BenchmarkPromiseRace |       1,226.20 |       1.6% |      1,645.60 |      4.5% | Darwin |  0.75x | yes |
| 89 | BenchmarkPromiseReject |         537.88 |       6.0% |        619.56 |      1.2% | Darwin |  0.87x | yes |
| 90 | BenchmarkPromiseRejection |         618.40 |      27.9% |        618.66 |      2.8% | Darwin |  1.00x |  |
| 91 | BenchmarkPromiseResolution |         103.02 |       3.7% |        144.78 |      1.7% | Darwin |  0.71x | yes |
| 92 | BenchmarkPromiseResolve |         101.58 |      23.3% |        115.90 |      1.3% | Darwin |  0.88x |  |
| 93 | BenchmarkPromiseResolve_Memory |         100.93 |       1.3% |        148.44 |      2.4% | Darwin |  0.68x | yes |
| 94 | BenchmarkPromiseThen |         319.28 |       1.7% |        456.12 |     16.6% | Darwin |  0.70x | yes |
| 95 | BenchmarkPromiseThenChain |         597.50 |       1.8% |        663.14 |      3.0% | Darwin |  0.90x | yes |
| 96 | BenchmarkPromiseTry |         100.34 |       1.2% |        151.58 |      2.3% | Darwin |  0.66x | yes |
| 97 | BenchmarkPromiseWithResolvers |          97.27 |       4.8% |        153.56 |      7.1% | Darwin |  0.63x | yes |
| 98 | BenchmarkPromisifyAllocation |       5,795.00 |       1.0% |      7,148.60 |      1.1% | Darwin |  0.81x | yes |
| 99 | BenchmarkPureChannelPingPong |         404.78 |      20.5% |        369.98 |      1.2% | Linux |  1.09x |  |
| 100 | BenchmarkQueueMicrotask |          89.50 |       2.6% |         76.78 |     12.8% | Linux |  1.17x | yes |
| 101 | BenchmarkQuiescing_ScheduleTimer_NoAutoExit |      26,789.80 |      46.5% |     57,575.60 |      5.3% | Darwin |  0.47x | yes |
| 102 | BenchmarkQuiescing_ScheduleTimer_WithAutoExit |      22,156.00 |       2.4% |     55,562.00 |      2.7% | Darwin |  0.40x | yes |
| 103 | BenchmarkRefUnref_IsLoopThread_External |       7,780.60 |      15.1% |      9,008.40 |      3.0% | Darwin |  0.86x |  |
| 104 | BenchmarkRefUnref_IsLoopThread_OnLoop |      10,714.20 |      15.5% |     10,263.20 |      1.5% | Linux |  1.04x |  |
| 105 | BenchmarkRefUnref_RWMutex_External |          33.88 |       0.3% |         34.76 |      2.4% | Darwin |  0.97x |  |
| 106 | BenchmarkRefUnref_RWMutex_OnLoop |          33.98 |       0.2% |         37.82 |      7.1% | Darwin |  0.90x | yes |
| 107 | BenchmarkRefUnref_SubmitInternal_External |         164.70 |       1.3% |        427.70 |      4.5% | Darwin |  0.39x | yes |
| 108 | BenchmarkRefUnref_SubmitInternal_OnLoop |      11,133.00 |       0.6% |     13,104.20 |     11.7% | Darwin |  0.85x | yes |
| 109 | BenchmarkRefUnref_SyncMap_External |          25.23 |       0.2% |         27.83 |      9.7% | Darwin |  0.91x |  |
| 110 | BenchmarkRefUnref_SyncMap_OnLoop |          25.08 |       0.2% |         26.55 |      1.7% | Darwin |  0.94x | yes |
| 111 | BenchmarkRegression_Combined_Atomic |           0.48 |       0.5% |          0.51 |      0.4% | Darwin |  0.94x | yes |
| 112 | BenchmarkRegression_Combined_Mutex |          17.96 |       1.9% |         19.39 |      2.6% | Darwin |  0.93x | yes |
| 113 | BenchmarkRegression_FastPathWakeup_NoWork |      12,278.60 |      28.4% |      3,744.20 |      0.6% | Linux |  3.28x | yes |
| 114 | BenchmarkRegression_HasExternalTasks_Mutex |           9.58 |       0.5% |         10.21 |      1.1% | Darwin |  0.94x | yes |
| 115 | BenchmarkRegression_HasInternalTasks_Mutex |           9.59 |       3.3% |         11.06 |      4.9% | Darwin |  0.87x | yes |
| 116 | BenchmarkRegression_HasInternalTasks_SimulatedAtomic |           0.30 |       0.6% |          0.34 |     11.5% | Darwin |  0.88x |  |
| 117 | BenchmarkScheduleTimerCancel |      25,139.40 |       3.5% |     40,510.80 |     16.5% | Darwin |  0.62x | yes |
| 118 | BenchmarkScheduleTimerWithPool |       4,349.20 |       0.7% |      3,739.20 |      0.9% | Linux |  1.16x | yes |
| 119 | BenchmarkScheduleTimerWithPool_FireAndReuse |       4,314.60 |       1.4% |      4,001.40 |      1.0% | Linux |  1.08x | yes |
| 120 | BenchmarkScheduleTimerWithPool_Immediate |       4,103.00 |       1.4% |      3,673.60 |      0.5% | Linux |  1.12x | yes |
| 121 | BenchmarkSentinelDrain_NoWork |      10,370.00 |      57.3% |     17,650.20 |    136.1% | Darwin |  0.59x |  |
| 122 | BenchmarkSentinelDrain_WithTimers |      20,636.60 |       1.1% |     54,457.00 |      4.5% | Darwin |  0.38x | yes |
| 123 | BenchmarkSentinelIteration |       6,569.40 |      47.9% |      5,312.40 |     65.4% | Linux |  1.24x |  |
| 124 | BenchmarkSentinelIteration_WithTimers |      15,798.60 |      12.2% |     42,817.60 |      2.7% | Darwin |  0.37x | yes |
| 125 | BenchmarkSetImmediate_Optimized |         198.68 |       3.0% |        143.38 |      1.5% | Linux |  1.39x | yes |
| 126 | BenchmarkSetInterval_Optimized |      24,520.60 |      17.1% |     41,440.00 |      8.9% | Darwin |  0.59x | yes |
| 127 | BenchmarkSetInterval_Parallel_Optimized |      15,916.80 |       1.8% |     20,102.60 |      1.2% | Darwin |  0.79x | yes |
| 128 | BenchmarkSetTimeoutZeroDelay |      23,393.20 |       1.8% |     43,391.20 |     15.7% | Darwin |  0.54x | yes |
| 129 | BenchmarkSetTimeout_Optimized |      25,479.60 |      22.1% |     40,257.00 |      6.7% | Darwin |  0.63x | yes |
| 130 | BenchmarkSubmit |          55.22 |       5.4% |         41.51 |      3.6% | Linux |  1.33x | yes |
| 131 | BenchmarkSubmitExecution |          67.85 |       1.2% |         57.33 |      2.9% | Linux |  1.18x | yes |
| 132 | BenchmarkSubmitInternal |       3,530.60 |       1.5% |      2,853.80 |      0.2% | Linux |  1.24x | yes |
| 133 | BenchmarkSubmitInternal_Cost |       3,511.60 |       1.0% |      2,855.00 |      0.5% | Linux |  1.23x | yes |
| 134 | BenchmarkSubmitInternal_FastPath_OnLoop |       5,308.80 |       3.1% |      5,262.60 |      0.5% | Linux |  1.01x |  |
| 135 | BenchmarkSubmitInternal_QueuePath_OnLoop |          34.27 |       3.5% |         44.77 |      7.1% | Darwin |  0.77x | yes |
| 136 | BenchmarkSubmitLatency |         437.00 |       1.7% |        337.10 |      3.0% | Linux |  1.30x | yes |
| 137 | BenchmarkSubmit_Parallel |         100.10 |       2.0% |         65.27 |     15.6% | Linux |  1.53x | yes |
| 138 | BenchmarkTask1_2_ConcurrentSubmissions |          95.56 |       3.0% |         60.69 |      1.8% | Linux |  1.57x | yes |
| 139 | BenchmarkTerminated_RejectionPath_Promisify |         464.52 |       1.3% |        561.20 |      1.2% | Darwin |  0.83x | yes |
| 140 | BenchmarkTerminated_RejectionPath_RefTimer |           2.02 |       1.4% |          2.30 |      3.4% | Darwin |  0.88x | yes |
| 141 | BenchmarkTerminated_RejectionPath_ScheduleTimer |          46.20 |       0.9% |         59.22 |      0.6% | Darwin |  0.78x | yes |
| 142 | BenchmarkTerminated_RejectionPath_submitToQueue |           9.94 |       3.7% |         10.98 |      0.3% | Darwin |  0.91x | yes |
| 143 | BenchmarkTerminated_UnrefTimer_NotGated |           2.05 |       2.7% |          2.27 |      2.0% | Darwin |  0.90x | yes |
| 144 | BenchmarkTimerFire |       4,587.60 |       5.1% |      4,041.40 |      0.3% | Linux |  1.14x | yes |
| 145 | BenchmarkTimerHeapOperations |          69.61 |       6.8% |        106.41 |      8.8% | Darwin |  0.65x | yes |
| 146 | BenchmarkTimerLatency |      19,928.20 |       1.3% |     49,512.20 |      0.9% | Darwin |  0.40x | yes |
| 147 | BenchmarkTimerSchedule |      22,276.60 |       1.1% |     49,300.20 |     13.4% | Darwin |  0.45x | yes |
| 148 | BenchmarkTimerSchedule_Parallel |      13,460.00 |       1.3% |     20,132.60 |      4.1% | Darwin |  0.67x | yes |
| 149 | BenchmarkWakeUpDeduplicationIntegration |          91.94 |       2.9% |         59.08 |      2.1% | Linux |  1.56x | yes |
| 150 | Benchmark_chunkedIngress_Batch |         519.80 |       1.1% |        523.38 |      0.6% | Darwin |  0.99x |  |
| 151 | Benchmark_chunkedIngress_ParallelWithSync |          85.17 |       4.7% |         45.08 |      2.7% | Linux |  1.89x | yes |
| 152 | Benchmark_chunkedIngress_Pop |           4.28 |      38.3% |          5.31 |     22.9% | Darwin |  0.81x |  |
| 153 | Benchmark_chunkedIngress_Push |           8.65 |       5.0% |         10.20 |      9.9% | Darwin |  0.85x | yes |
| 154 | Benchmark_chunkedIngress_PushPop |           4.09 |       1.3% |          4.04 |      0.2% | Linux |  1.01x |  |
| 155 | Benchmark_chunkedIngress_Sequential |           4.01 |       0.5% |          4.08 |      0.6% | Darwin |  0.98x | yes |
| 156 | Benchmark_microtaskRing_Parallel |         122.68 |       0.6% |         99.61 |      7.1% | Linux |  1.23x | yes |
| 157 | Benchmark_microtaskRing_Push |          27.60 |      13.6% |         44.35 |     20.3% | Darwin |  0.62x | yes |
| 158 | Benchmark_microtaskRing_PushPop |          22.49 |       0.3% |         22.52 |      0.3% | Darwin |  1.00x |  |

## Performance by Category

### Concurrency (5 benchmarks)

- Darwin wins: 3/5
- Linux wins: 2/5
- Darwin category mean: 3,293.14 ns/op
- Linux category mean: 4,192.84 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkSetInterval_Parallel_Optimized |      15,916.80 |     20,102.60 | Darwin | 1.26x |
| BenchmarkAlive_WithMutexes_HighContention |         190.10 |        606.22 | Darwin | 3.19x |
| BenchmarkHighContention |         227.48 |        154.66 | Linux | 1.47x |
| BenchmarkTask1_2_ConcurrentSubmissions |          95.56 |         60.69 | Linux | 1.57x |
| BenchmarkAlive_Epoch_NoContention |          35.74 |         40.05 | Darwin | 1.12x |

### Latency & Primitives (29 benchmarks)

- Darwin wins: 14/29
- Linux wins: 15/29
- Darwin category mean: 1,022.61 ns/op
- Linux category mean: 2,077.24 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkTimerLatency |      19,928.20 |     49,512.20 | Darwin | 2.48x |
| BenchmarkLatencySample_OldSortBased |       6,246.20 |      7,801.20 | Darwin | 1.25x |
| BenchmarkLatencyChannelRoundTrip |         360.58 |        234.76 | Linux | 1.54x |
| BenchmarkMicrotaskLatency |         443.58 |        336.36 | Linux | 1.32x |
| BenchmarkSubmitLatency |         437.00 |        337.10 | Linux | 1.30x |
| BenchmarkLatencyAnalysis_SubmitWhileRunning |         437.74 |        338.94 | Linux | 1.29x |
| BenchmarkLatencyAnalysis_PingPong |         571.82 |        476.64 | Linux | 1.20x |
| BenchmarkLatencychunkedIngressPush_WithContention |          79.95 |         38.89 | Linux | 2.06x |
| BenchmarkLatencymicrotaskRingPush |          30.48 |         52.68 | Darwin | 1.73x |
| BenchmarkLatencyChannelBufferedRoundTrip |         268.68 |        246.70 | Linux | 1.09x |
| BenchmarkLatencyAnalysis_EndToEnd |         583.64 |        597.80 | Darwin | 1.02x |
| BenchmarkLatencymicrotaskRingPop |          18.02 |         16.26 | Linux | 1.11x |
| BenchmarkLatencychunkedIngressPush |           8.71 |         10.43 | Darwin | 1.20x |
| BenchmarkLatencyRWMutexRLockRUnlock |           9.62 |          8.14 | Linux | 1.18x |
| BenchmarkLatencySimulatedSubmit |          14.49 |         15.73 | Darwin | 1.09x |
| BenchmarkLatencyRecord_WithoutPSquare |          25.56 |         24.34 | Linux | 1.05x |
| BenchmarkLatencyRecord_WithPSquare |          83.05 |         84.21 | Darwin | 1.01x |
| BenchmarkLatencySimulatedPoll |          15.48 |         14.34 | Linux | 1.08x |
| BenchmarkLatencychunkedIngressPop |           4.03 |          4.96 | Darwin | 1.23x |
| BenchmarkLatencyStateTryTransition_NoOp |          18.60 |         17.73 | Linux | 1.05x |
| BenchmarkLatencySample_NewPSquare |          25.32 |         25.59 | Darwin | 1.01x |
| BenchmarkLatencychunkedIngressPushPop |           4.27 |          4.05 | Linux | 1.05x |
| BenchmarkLatencyStateTryTransition |           4.10 |          4.24 | Darwin | 1.04x |
| BenchmarkLatencymicrotaskRingPushPop |          22.45 |         22.56 | Darwin | 1.00x |
| BenchmarkLatencyDeferRecover |           2.53 |          2.48 | Linux | 1.02x |
| BenchmarkLatencySafeExecute |           3.08 |          3.11 | Darwin | 1.01x |
| BenchmarkLatencyMutexLockUnlock |           7.99 |          8.02 | Darwin | 1.00x |
| BenchmarkLatencyStateLoad |           0.32 |          0.31 | Linux | 1.03x |
| BenchmarkLatencyDirectCall |           0.31 |          0.31 | Darwin | 1.00x |

### Other (48 benchmarks)

- Darwin wins: 30/48
- Linux wins: 18/48
- Darwin category mean: 36,066.53 ns/op
- Linux category mean: 60,050.90 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkAutoExit_UnrefExit |   1,253,749.00 |  2,166,326.40 | Darwin | 1.73x |
| BenchmarkAutoExit_ImmediateExit |     196,374.80 |    394,390.80 | Darwin | 2.01x |
| BenchmarkAutoExit_PollPathExit |     132,551.00 |     99,004.20 | Linux | 1.34x |
| BenchmarkSetTimeoutZeroDelay |      23,393.20 |     43,391.20 | Darwin | 1.85x |
| BenchmarkSetInterval_Optimized |      24,520.60 |     41,440.00 | Darwin | 1.69x |
| BenchmarkSetTimeout_Optimized |      25,479.60 |     40,257.00 | Darwin | 1.58x |
| BenchmarkAlive_Epoch_FromCallback |      12,819.80 |     26,345.20 | Darwin | 2.06x |
| BenchmarkSentinelDrain_NoWork |      10,370.00 |     17,650.20 | Darwin | 1.70x |
| BenchmarkPromisifyAllocation |       5,795.00 |      7,148.60 | Darwin | 1.23x |
| BenchmarkSentinelIteration |       6,569.40 |      5,312.40 | Linux | 1.24x |
| BenchmarkRefUnref_IsLoopThread_External |       7,780.60 |      9,008.40 | Darwin | 1.16x |
| BenchmarkHighFrequencyMonitoring_Old |       8,769.00 |      9,553.00 | Darwin | 1.09x |
| BenchmarkRefUnref_IsLoopThread_OnLoop |      10,714.20 |     10,263.20 | Linux | 1.04x |
| BenchmarkIsLoopThread_True |       4,599.00 |      4,883.80 | Darwin | 1.06x |
| BenchmarkMixedWorkload |         985.76 |        863.00 | Linux | 1.14x |
| BenchmarkTerminated_RejectionPath_Promisify |         464.52 |        561.20 | Darwin | 1.21x |
| BenchmarkSetImmediate_Optimized |         198.68 |        143.38 | Linux | 1.39x |
| BenchmarkPureChannelPingPong |         404.78 |        369.98 | Linux | 1.09x |
| BenchmarkWakeUpDeduplicationIntegration |          91.94 |         59.08 | Linux | 1.56x |
| BenchmarkMinimalLoop |         463.04 |        432.32 | Linux | 1.07x |
| BenchmarkMicroPingPong |         483.70 |        459.26 | Linux | 1.05x |
| BenchmarkGetGoroutineID |       1,889.80 |      1,866.40 | Linux | 1.01x |
| BenchmarkChannelWithMutexQueue |         479.56 |        456.22 | Linux | 1.05x |
| BenchmarkLoopDirect |         486.94 |        503.68 | Darwin | 1.03x |
| BenchmarkNoMetrics |          55.12 |         42.63 | Linux | 1.29x |
| BenchmarkCombinedWorkload_Old |         229.62 |        218.38 | Linux | 1.05x |
| BenchmarkAutoExit_AliveCheckCost_Disabled |          54.24 |         43.01 | Linux | 1.26x |
| BenchmarkAutoExit_AliveCheckCost_Enabled |          49.85 |         42.09 | Linux | 1.18x |
| BenchmarkGojaStyleSwap |         459.16 |        465.76 | Darwin | 1.01x |
| BenchmarkMicroPingPongWithCount |         452.58 |        458.58 | Darwin | 1.01x |
| BenchmarkAlive_AllAtomic_Contended |          18.49 |         22.88 | Darwin | 1.24x |
| BenchmarkRefUnref_RWMutex_OnLoop |          33.98 |         37.82 | Darwin | 1.11x |
| BenchmarkMetricsCollection |          43.21 |         46.65 | Darwin | 1.08x |
| BenchmarkAlive_WithMutexes |          34.81 |         37.67 | Darwin | 1.08x |
| BenchmarkAlive_Uncontended |          35.59 |         38.39 | Darwin | 1.08x |
| BenchmarkRefUnref_SyncMap_External |          25.23 |         27.83 | Darwin | 1.10x |
| BenchmarkAlive_WithMutexes_Contended |          34.89 |         36.51 | Darwin | 1.05x |
| BenchmarkHighFrequencyMonitoring_New |          26.72 |         25.17 | Linux | 1.06x |
| BenchmarkRegression_HasInternalTasks_Mutex |           9.59 |         11.06 | Darwin | 1.15x |
| BenchmarkRefUnref_SyncMap_OnLoop |          25.08 |         26.55 | Darwin | 1.06x |
| BenchmarkRegression_Combined_Mutex |          17.96 |         19.39 | Darwin | 1.08x |
| BenchmarkCombinedWorkload_New |          90.56 |         89.23 | Linux | 1.01x |
| BenchmarkRefUnref_RWMutex_External |          33.88 |         34.76 | Darwin | 1.03x |
| BenchmarkRegression_HasExternalTasks_Mutex |           9.58 |         10.21 | Darwin | 1.07x |
| BenchmarkAlive_AllAtomic |          18.24 |         18.60 | Darwin | 1.02x |
| BenchmarkRegression_HasInternalTasks_SimulatedAtomic |           0.30 |          0.34 | Darwin | 1.14x |
| BenchmarkRegression_Combined_Atomic |           0.48 |          0.51 | Darwin | 1.07x |
| BenchmarkIsLoopThread_False |           0.38 |          0.37 | Linux | 1.01x |

### Promise Operations (18 benchmarks)

- Darwin wins: 17/18
- Linux wins: 1/18
- Darwin category mean: 6,635.04 ns/op
- Linux category mean: 7,068.04 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkPromiseGC |     111,599.60 |    117,750.40 | Darwin | 1.06x |
| BenchmarkPromiseAll |       1,498.60 |      1,954.80 | Darwin | 1.30x |
| BenchmarkPromiseRace |       1,226.20 |      1,645.60 | Darwin | 1.34x |
| BenchmarkPromiseAll_Memory |       1,435.40 |      1,776.60 | Darwin | 1.24x |
| BenchmarkPromiseHandlerTracking_Parallel_Optimized |         346.34 |        167.36 | Linux | 2.07x |
| BenchmarkPromiseThen |         319.28 |        456.12 | Darwin | 1.43x |
| BenchmarkPromiseReject |         537.88 |        619.56 | Darwin | 1.15x |
| BenchmarkPromiseThenChain |         597.50 |        663.14 | Darwin | 1.11x |
| BenchmarkPromiseWithResolvers |          97.27 |        153.56 | Darwin | 1.58x |
| BenchmarkPromiseCreation |          71.17 |        123.44 | Darwin | 1.73x |
| BenchmarkPromiseTry |         100.34 |        151.58 | Darwin | 1.51x |
| BenchmarkPromiseResolve_Memory |         100.93 |        148.44 | Darwin | 1.47x |
| BenchmarkPromiseResolution |         103.02 |        144.78 | Darwin | 1.41x |
| BenchmarkPromiseCreate |          60.95 |        100.13 | Darwin | 1.64x |
| BenchmarkPromiseHandlerTracking_Optimized |          81.94 |         99.29 | Darwin | 1.21x |
| BenchmarkPromiseResolve |         101.58 |        115.90 | Darwin | 1.14x |
| BenchmarkPromiseChain |         534.34 |        535.34 | Darwin | 1.00x |
| BenchmarkPromiseRejection |         618.40 |        618.66 | Darwin | 1.00x |

### Task Submission (32 benchmarks)

- Darwin wins: 17/32
- Linux wins: 15/32
- Darwin category mean: 8,672.86 ns/op
- Linux category mean: 14,547.20 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkAutoExit_FastPathExit |     224,403.40 |    396,258.00 | Darwin | 1.77x |
| BenchmarkLoopDirectWithSubmit |      15,359.80 |     38,934.40 | Darwin | 2.53x |
| BenchmarkRegression_FastPathWakeup_NoWork |      12,278.60 |      3,744.20 | Linux | 3.28x |
| BenchmarkRefUnref_SubmitInternal_OnLoop |      11,133.00 |     13,104.20 | Darwin | 1.18x |
| BenchmarkSubmitInternal |       3,530.60 |      2,853.80 | Linux | 1.24x |
| BenchmarkSubmitInternal_Cost |       3,511.60 |      2,855.00 | Linux | 1.23x |
| BenchmarkAlive_Epoch_ConcurrentSubmit |         244.08 |        609.74 | Darwin | 2.50x |
| BenchmarkRefUnref_SubmitInternal_External |         164.70 |        427.70 | Darwin | 2.60x |
| BenchmarkSubmitInternal_FastPath_OnLoop |       5,308.80 |      5,262.60 | Linux | 1.01x |
| BenchmarkMicrotaskExecution |          76.80 |        121.74 | Darwin | 1.59x |
| Benchmark_chunkedIngress_ParallelWithSync |          85.17 |         45.08 | Linux | 1.89x |
| BenchmarkMicrotaskSchedule_Parallel |         112.34 |         76.02 | Linux | 1.48x |
| BenchmarkSubmit_Parallel |         100.10 |         65.27 | Linux | 1.53x |
| BenchmarkMicrotaskSchedule |          98.13 |         74.67 | Linux | 1.31x |
| Benchmark_microtaskRing_Parallel |         122.68 |         99.61 | Linux | 1.23x |
| Benchmark_microtaskRing_Push |          27.60 |         44.35 | Darwin | 1.61x |
| BenchmarkSubmit |          55.22 |         41.51 | Linux | 1.33x |
| BenchmarkQueueMicrotask |          89.50 |         76.78 | Linux | 1.17x |
| BenchmarkFastPathSubmit |          53.01 |         40.67 | Linux | 1.30x |
| BenchmarkSubmitExecution |          67.85 |         57.33 | Linux | 1.18x |
| BenchmarkSubmitInternal_QueuePath_OnLoop |          34.27 |         44.77 | Darwin | 1.31x |
| BenchmarkFastPathExecution |          66.63 |         57.36 | Linux | 1.16x |
| Benchmark_chunkedIngress_Batch |         519.80 |        523.38 | Darwin | 1.01x |
| Benchmark_chunkedIngress_Push |           8.65 |         10.20 | Darwin | 1.18x |
| BenchmarkTerminated_RejectionPath_submitToQueue |           9.94 |         10.98 | Darwin | 1.10x |
| Benchmark_chunkedIngress_Pop |           4.28 |          5.31 | Darwin | 1.24x |
| BenchmarkMicrotaskOverflow |          24.41 |         24.61 | Darwin | 1.01x |
| BenchmarkMicrotaskRingIsEmpty_WithItems |           2.09 |          2.26 | Darwin | 1.08x |
| BenchmarkMicrotaskRingIsEmpty |           7.97 |          8.12 | Darwin | 1.02x |
| Benchmark_chunkedIngress_Sequential |           4.01 |          4.08 | Darwin | 1.02x |
| Benchmark_chunkedIngress_PushPop |           4.09 |          4.04 | Linux | 1.01x |
| Benchmark_microtaskRing_PushPop |          22.49 |         22.52 | Darwin | 1.00x |

### Timer Operations (26 benchmarks)

- Darwin wins: 18/26
- Linux wins: 8/26
- Darwin category mean: 282,760.57 ns/op
- Linux category mean: 462,314.27 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkCancelTimer_Individual/timers_: |   2,695,208.80 |  5,198,410.60 | Darwin | 1.93x |
| BenchmarkCancelTimer_Individual/timers_5 |   1,498,278.20 |  2,492,180.20 | Darwin | 1.66x |
| BenchmarkCancelTimers_Comparison/Individual |   1,517,221.80 |  2,486,208.60 | Darwin | 1.64x |
| BenchmarkCancelTimer_Individual/timers_1 |     287,121.00 |    440,906.00 | Darwin | 1.54x |
| BenchmarkCancelTimers_Batch/timers_: |     474,486.60 |    415,786.80 | Linux | 1.14x |
| BenchmarkAutoExit_TimerFire |     109,175.60 |     74,595.00 | Linux | 1.46x |
| BenchmarkCancelTimers_Batch/timers_5 |     263,300.00 |    228,906.20 | Linux | 1.15x |
| BenchmarkSentinelDrain_WithTimers |      20,636.60 |     54,457.00 | Darwin | 2.64x |
| BenchmarkQuiescing_ScheduleTimer_WithAutoExit |      22,156.00 |     55,562.00 | Darwin | 2.51x |
| BenchmarkQuiescing_ScheduleTimer_NoAutoExit |      26,789.80 |     57,575.60 | Darwin | 2.15x |
| BenchmarkTimerSchedule |      22,276.60 |     49,300.20 | Darwin | 2.21x |
| BenchmarkSentinelIteration_WithTimers |      15,798.60 |     42,817.60 | Darwin | 2.71x |
| BenchmarkCancelTimers_Comparison/Batch |     250,715.60 |    226,902.80 | Linux | 1.10x |
| BenchmarkLargeTimerHeap |      22,297.00 |     43,618.80 | Darwin | 1.96x |
| BenchmarkScheduleTimerCancel |      25,139.40 |     40,510.80 | Darwin | 1.61x |
| BenchmarkTimerSchedule_Parallel |      13,460.00 |     20,132.60 | Darwin | 1.50x |
| BenchmarkCancelTimers_Batch/timers_1 |      70,236.80 |     76,672.20 | Darwin | 1.09x |
| BenchmarkScheduleTimerWithPool |       4,349.20 |      3,739.20 | Linux | 1.16x |
| BenchmarkTimerFire |       4,587.60 |      4,041.40 | Linux | 1.14x |
| BenchmarkScheduleTimerWithPool_Immediate |       4,103.00 |      3,673.60 | Linux | 1.12x |
| BenchmarkScheduleTimerWithPool_FireAndReuse |       4,314.60 |      4,001.40 | Linux | 1.08x |
| BenchmarkTimerHeapOperations |          69.61 |        106.41 | Darwin | 1.53x |
| BenchmarkTerminated_RejectionPath_ScheduleTimer |          46.20 |         59.22 | Darwin | 1.28x |
| BenchmarkTerminated_RejectionPath_RefTimer |           2.02 |          2.30 | Darwin | 1.14x |
| BenchmarkTerminated_UnrefTimer_NotGated |           2.05 |          2.27 | Darwin | 1.11x |
| BenchmarkAlive_WithTimer |           2.18 |          2.22 | Darwin | 1.02x |

## Statistically Significant Differences

**110** out of 158 benchmarks show statistically significant
differences (Welch's t-test, p < 0.05).

- Darwin significantly faster: **67** benchmarks
- Linux significantly faster: **43** benchmarks

### Largest Significant Differences

| Benchmark | Faster | Speedup | Darwin (ns/op) | Linux (ns/op) | t-stat |
|-----------|--------|---------|----------------|---------------|--------|
| BenchmarkRegression_FastPathWakeup_NoWork | Linux | 3.28x |      12,278.60 |      3,744.20 | 5.47 |
| BenchmarkAlive_WithMutexes_HighContention | Darwin | 3.19x |         190.10 |        606.22 | 10.25 |
| BenchmarkSentinelIteration_WithTimers | Darwin | 2.71x |      15,798.60 |     42,817.60 | 26.85 |
| BenchmarkSentinelDrain_WithTimers | Darwin | 2.64x |      20,636.60 |     54,457.00 | 30.85 |
| BenchmarkRefUnref_SubmitInternal_External | Darwin | 2.60x |         164.70 |        427.70 | 30.59 |
| BenchmarkLoopDirectWithSubmit | Darwin | 2.53x |      15,359.80 |     38,934.40 | 88.44 |
| BenchmarkQuiescing_ScheduleTimer_WithAutoExit | Darwin | 2.51x |      22,156.00 |     55,562.00 | 47.18 |
| BenchmarkAlive_Epoch_ConcurrentSubmit | Darwin | 2.50x |         244.08 |        609.74 | 8.35 |
| BenchmarkTimerLatency | Darwin | 2.48x |      19,928.20 |     49,512.20 | 132.71 |
| BenchmarkTimerSchedule | Darwin | 2.21x |      22,276.60 |     49,300.20 | 9.13 |
| BenchmarkQuiescing_ScheduleTimer_NoAutoExit | Darwin | 2.15x |      26,789.80 |     57,575.60 | 5.37 |
| BenchmarkPromiseHandlerTracking_Parallel_Optimized | Linux | 2.07x |         346.34 |        167.36 | 83.83 |
| BenchmarkLatencychunkedIngressPush_WithContention | Linux | 2.06x |          79.95 |         38.89 | 31.05 |
| BenchmarkLargeTimerHeap | Darwin | 1.96x |      22,297.00 |     43,618.80 | 21.27 |
| BenchmarkCancelTimer_Individual/timers_: | Darwin | 1.93x |   2,695,208.80 |  5,198,410.60 | 55.75 |
| Benchmark_chunkedIngress_ParallelWithSync | Linux | 1.89x |          85.17 |         45.08 | 21.46 |
| BenchmarkSetTimeoutZeroDelay | Darwin | 1.85x |      23,393.20 |     43,391.20 | 6.56 |
| BenchmarkPromiseCreation | Darwin | 1.73x |          71.17 |        123.44 | 18.50 |
| BenchmarkLatencymicrotaskRingPush | Darwin | 1.73x |          30.48 |         52.68 | 6.95 |
| BenchmarkAutoExit_UnrefExit | Darwin | 1.73x |   1,253,749.00 |  2,166,326.40 | 44.03 |
| BenchmarkSetInterval_Optimized | Darwin | 1.69x |      24,520.60 |     41,440.00 | 6.77 |
| BenchmarkCancelTimer_Individual/timers_5 | Darwin | 1.66x |   1,498,278.20 |  2,492,180.20 | 18.35 |
| BenchmarkPromiseCreate | Darwin | 1.64x |          60.95 |        100.13 | 55.83 |
| BenchmarkCancelTimers_Comparison/Individual | Darwin | 1.64x |   1,517,221.80 |  2,486,208.60 | 16.89 |
| BenchmarkScheduleTimerCancel | Darwin | 1.61x |      25,139.40 |     40,510.80 | 5.11 |
| Benchmark_microtaskRing_Push | Darwin | 1.61x |          27.60 |         44.35 | 3.83 |
| BenchmarkMicrotaskExecution | Darwin | 1.59x |          76.80 |        121.74 | 19.09 |
| BenchmarkSetTimeout_Optimized | Darwin | 1.58x |      25,479.60 |     40,257.00 | 5.29 |
| BenchmarkPromiseWithResolvers | Darwin | 1.58x |          97.27 |        153.56 | 10.57 |
| BenchmarkTask1_2_ConcurrentSubmissions | Linux | 1.57x |          95.56 |         60.69 | 25.43 |

## Top 10 Fastest Benchmarks

### Darwin

| Rank | Benchmark | ns/op | B/op | Allocs/op | CV% |
|------|-----------|-------|------|-----------|-----|
| 1 | BenchmarkRegression_HasInternalTasks_SimulatedAtom |       0.30 |    0 |         0 | 0.6% |
| 2 | BenchmarkLatencyDirectCall |       0.31 |    0 |         0 | 1.5% |
| 3 | BenchmarkLatencyStateLoad |       0.32 |    0 |         0 | 5.4% |
| 4 | BenchmarkIsLoopThread_False |       0.38 |    0 |         0 | 11.7% |
| 5 | BenchmarkRegression_Combined_Atomic |       0.48 |    0 |         0 | 0.5% |
| 6 | BenchmarkTerminated_RejectionPath_RefTimer |       2.02 |    0 |         0 | 1.4% |
| 7 | BenchmarkTerminated_UnrefTimer_NotGated |       2.05 |    0 |         0 | 2.7% |
| 8 | BenchmarkMicrotaskRingIsEmpty_WithItems |       2.09 |    0 |         0 | 3.4% |
| 9 | BenchmarkAlive_WithTimer |       2.18 |    0 |         0 | 13.5% |
| 10 | BenchmarkLatencyDeferRecover |       2.53 |    0 |         0 | 4.3% |

### Linux

| Rank | Benchmark | ns/op | B/op | Allocs/op | CV% |
|------|-----------|-------|------|-----------|-----|
| 1 | BenchmarkLatencyStateLoad |       0.31 |    0 |         0 | 1.1% |
| 2 | BenchmarkLatencyDirectCall |       0.31 |    0 |         0 | 1.1% |
| 3 | BenchmarkRegression_HasInternalTasks_SimulatedAtom |       0.34 |    0 |         0 | 11.5% |
| 4 | BenchmarkIsLoopThread_False |       0.37 |    0 |         0 | 5.4% |
| 5 | BenchmarkRegression_Combined_Atomic |       0.51 |    0 |         0 | 0.4% |
| 6 | BenchmarkAlive_WithTimer |       2.22 |    0 |         0 | 0.5% |
| 7 | BenchmarkMicrotaskRingIsEmpty_WithItems |       2.26 |    0 |         0 | 17.7% |
| 8 | BenchmarkTerminated_UnrefTimer_NotGated |       2.27 |    0 |         0 | 2.0% |
| 9 | BenchmarkTerminated_RejectionPath_RefTimer |       2.30 |    0 |         0 | 3.4% |
| 10 | BenchmarkLatencyDeferRecover |       2.48 |    0 |         0 | 0.5% |

## Allocation Comparison

Since both platforms run the same Go code, allocations (allocs/op) and bytes (B/op)
should be identical. Differences indicate platform-specific runtime behavior.

- **Allocs/op match:** 151/158 (95.6%)
- **B/op match:** 119/158 (75.3%)
- **Zero-allocation benchmarks (both platforms):** 77

### Zero-Allocation Benchmarks

These benchmarks achieve zero allocations on both platforms — the gold standard
for hot-path performance:

- `BenchmarkAlive_AllAtomic` — Darwin: 18.24 ns/op, Linux: 18.60 ns/op (Darwin faster)
- `BenchmarkAlive_AllAtomic_Contended` — Darwin: 18.49 ns/op, Linux: 22.88 ns/op (Darwin faster)
- `BenchmarkAlive_Epoch_ConcurrentSubmit` — Darwin: 244.08 ns/op, Linux: 609.74 ns/op (Darwin faster)
- `BenchmarkAlive_Epoch_NoContention` — Darwin: 35.74 ns/op, Linux: 40.05 ns/op (Darwin faster)
- `BenchmarkAlive_Uncontended` — Darwin: 35.59 ns/op, Linux: 38.39 ns/op (Darwin faster)
- `BenchmarkAlive_WithMutexes` — Darwin: 34.81 ns/op, Linux: 37.67 ns/op (Darwin faster)
- `BenchmarkAlive_WithMutexes_Contended` — Darwin: 34.89 ns/op, Linux: 36.51 ns/op (Darwin faster)
- `BenchmarkAlive_WithMutexes_HighContention` — Darwin: 190.10 ns/op, Linux: 606.22 ns/op (Darwin faster)
- `BenchmarkAlive_WithTimer` — Darwin: 2.18 ns/op, Linux: 2.22 ns/op (Darwin faster)
- `BenchmarkAutoExit_AliveCheckCost_Disabled` — Darwin: 54.24 ns/op, Linux: 43.01 ns/op (Linux faster)
- `BenchmarkAutoExit_AliveCheckCost_Enabled` — Darwin: 49.85 ns/op, Linux: 42.09 ns/op (Linux faster)
- `BenchmarkCombinedWorkload_New` — Darwin: 90.56 ns/op, Linux: 89.23 ns/op (Linux faster)
- `BenchmarkCombinedWorkload_Old` — Darwin: 229.62 ns/op, Linux: 218.38 ns/op (Linux faster)
- `BenchmarkFastPathSubmit` — Darwin: 53.01 ns/op, Linux: 40.67 ns/op (Linux faster)
- `BenchmarkGetGoroutineID` — Darwin: 1889.80 ns/op, Linux: 1866.40 ns/op (Linux faster)
- `BenchmarkHighContention` — Darwin: 227.48 ns/op, Linux: 154.66 ns/op (Linux faster)
- `BenchmarkHighFrequencyMonitoring_New` — Darwin: 26.72 ns/op, Linux: 25.17 ns/op (Linux faster)
- `BenchmarkIsLoopThread_False` — Darwin: 0.38 ns/op, Linux: 0.37 ns/op (Linux faster)
- `BenchmarkIsLoopThread_True` — Darwin: 4599.00 ns/op, Linux: 4883.80 ns/op (Darwin faster)
- `BenchmarkLatencyChannelBufferedRoundTrip` — Darwin: 268.68 ns/op, Linux: 246.70 ns/op (Linux faster)
- `BenchmarkLatencyChannelRoundTrip` — Darwin: 360.58 ns/op, Linux: 234.76 ns/op (Linux faster)
- `BenchmarkLatencyDeferRecover` — Darwin: 2.53 ns/op, Linux: 2.48 ns/op (Linux faster)
- `BenchmarkLatencyDirectCall` — Darwin: 0.31 ns/op, Linux: 0.31 ns/op (Darwin faster)
- `BenchmarkLatencyMutexLockUnlock` — Darwin: 7.99 ns/op, Linux: 8.02 ns/op (Darwin faster)
- `BenchmarkLatencyRWMutexRLockRUnlock` — Darwin: 9.62 ns/op, Linux: 8.14 ns/op (Linux faster)
- `BenchmarkLatencyRecord_WithPSquare` — Darwin: 83.05 ns/op, Linux: 84.21 ns/op (Darwin faster)
- `BenchmarkLatencyRecord_WithoutPSquare` — Darwin: 25.56 ns/op, Linux: 24.34 ns/op (Linux faster)
- `BenchmarkLatencySafeExecute` — Darwin: 3.08 ns/op, Linux: 3.11 ns/op (Darwin faster)
- `BenchmarkLatencySample_NewPSquare` — Darwin: 25.32 ns/op, Linux: 25.59 ns/op (Darwin faster)
- `BenchmarkLatencySimulatedPoll` — Darwin: 15.48 ns/op, Linux: 14.34 ns/op (Linux faster)
- `BenchmarkLatencySimulatedSubmit` — Darwin: 14.49 ns/op, Linux: 15.73 ns/op (Darwin faster)
- `BenchmarkLatencyStateLoad` — Darwin: 0.32 ns/op, Linux: 0.31 ns/op (Linux faster)
- `BenchmarkLatencyStateTryTransition` — Darwin: 4.10 ns/op, Linux: 4.24 ns/op (Darwin faster)
- `BenchmarkLatencyStateTryTransition_NoOp` — Darwin: 18.60 ns/op, Linux: 17.73 ns/op (Linux faster)
- `BenchmarkLatencychunkedIngressPop` — Darwin: 4.03 ns/op, Linux: 4.96 ns/op (Darwin faster)
- `BenchmarkLatencychunkedIngressPush` — Darwin: 8.71 ns/op, Linux: 10.43 ns/op (Darwin faster)
- `BenchmarkLatencychunkedIngressPushPop` — Darwin: 4.27 ns/op, Linux: 4.05 ns/op (Linux faster)
- `BenchmarkLatencychunkedIngressPush_WithContention` — Darwin: 79.95 ns/op, Linux: 38.89 ns/op (Linux faster)
- `BenchmarkLatencymicrotaskRingPop` — Darwin: 18.02 ns/op, Linux: 16.26 ns/op (Linux faster)
- `BenchmarkLatencymicrotaskRingPush` — Darwin: 30.48 ns/op, Linux: 52.68 ns/op (Darwin faster)
- `BenchmarkLatencymicrotaskRingPushPop` — Darwin: 22.45 ns/op, Linux: 22.56 ns/op (Darwin faster)
- `BenchmarkMetricsCollection` — Darwin: 43.21 ns/op, Linux: 46.65 ns/op (Darwin faster)
- `BenchmarkMicrotaskOverflow` — Darwin: 24.41 ns/op, Linux: 24.61 ns/op (Darwin faster)
- `BenchmarkMicrotaskRingIsEmpty` — Darwin: 7.97 ns/op, Linux: 8.12 ns/op (Darwin faster)
- `BenchmarkMicrotaskRingIsEmpty_WithItems` — Darwin: 2.09 ns/op, Linux: 2.26 ns/op (Darwin faster)
- `BenchmarkMicrotaskSchedule` — Darwin: 98.13 ns/op, Linux: 74.67 ns/op (Linux faster)
- `BenchmarkMicrotaskSchedule_Parallel` — Darwin: 112.34 ns/op, Linux: 76.02 ns/op (Linux faster)
- `BenchmarkNoMetrics` — Darwin: 55.12 ns/op, Linux: 42.63 ns/op (Linux faster)
- `BenchmarkQueueMicrotask` — Darwin: 89.50 ns/op, Linux: 76.78 ns/op (Linux faster)
- `BenchmarkRefUnref_IsLoopThread_OnLoop` — Darwin: 10714.20 ns/op, Linux: 10263.20 ns/op (Linux faster)
- `BenchmarkRefUnref_RWMutex_External` — Darwin: 33.88 ns/op, Linux: 34.76 ns/op (Darwin faster)
- `BenchmarkRefUnref_RWMutex_OnLoop` — Darwin: 33.98 ns/op, Linux: 37.82 ns/op (Darwin faster)
- `BenchmarkRefUnref_SyncMap_External` — Darwin: 25.23 ns/op, Linux: 27.83 ns/op (Darwin faster)
- `BenchmarkRefUnref_SyncMap_OnLoop` — Darwin: 25.08 ns/op, Linux: 26.55 ns/op (Darwin faster)
- `BenchmarkRegression_Combined_Atomic` — Darwin: 0.48 ns/op, Linux: 0.51 ns/op (Darwin faster)
- `BenchmarkRegression_Combined_Mutex` — Darwin: 17.96 ns/op, Linux: 19.39 ns/op (Darwin faster)
- `BenchmarkRegression_HasExternalTasks_Mutex` — Darwin: 9.58 ns/op, Linux: 10.21 ns/op (Darwin faster)
- `BenchmarkRegression_HasInternalTasks_Mutex` — Darwin: 9.59 ns/op, Linux: 11.06 ns/op (Darwin faster)
- `BenchmarkRegression_HasInternalTasks_SimulatedAtomic` — Darwin: 0.30 ns/op, Linux: 0.34 ns/op (Darwin faster)
- `BenchmarkSubmit` — Darwin: 55.22 ns/op, Linux: 41.51 ns/op (Linux faster)
- `BenchmarkSubmitInternal` — Darwin: 3530.60 ns/op, Linux: 2853.80 ns/op (Linux faster)
- `BenchmarkSubmitInternal_Cost` — Darwin: 3511.60 ns/op, Linux: 2855.00 ns/op (Linux faster)
- `BenchmarkSubmit_Parallel` — Darwin: 100.10 ns/op, Linux: 65.27 ns/op (Linux faster)
- `BenchmarkTask1_2_ConcurrentSubmissions` — Darwin: 95.56 ns/op, Linux: 60.69 ns/op (Linux faster)
- `BenchmarkTerminated_RejectionPath_RefTimer` — Darwin: 2.02 ns/op, Linux: 2.30 ns/op (Darwin faster)
- `BenchmarkTerminated_RejectionPath_submitToQueue` — Darwin: 9.94 ns/op, Linux: 10.98 ns/op (Darwin faster)
- `BenchmarkTerminated_UnrefTimer_NotGated` — Darwin: 2.05 ns/op, Linux: 2.27 ns/op (Darwin faster)
- `BenchmarkWakeUpDeduplicationIntegration` — Darwin: 91.94 ns/op, Linux: 59.08 ns/op (Linux faster)
- `Benchmark_chunkedIngress_Batch` — Darwin: 519.80 ns/op, Linux: 523.38 ns/op (Darwin faster)
- `Benchmark_chunkedIngress_ParallelWithSync` — Darwin: 85.17 ns/op, Linux: 45.08 ns/op (Linux faster)
- `Benchmark_chunkedIngress_Pop` — Darwin: 4.28 ns/op, Linux: 5.31 ns/op (Darwin faster)
- `Benchmark_chunkedIngress_Push` — Darwin: 8.65 ns/op, Linux: 10.20 ns/op (Darwin faster)
- `Benchmark_chunkedIngress_PushPop` — Darwin: 4.09 ns/op, Linux: 4.04 ns/op (Linux faster)
- `Benchmark_chunkedIngress_Sequential` — Darwin: 4.01 ns/op, Linux: 4.08 ns/op (Darwin faster)
- `Benchmark_microtaskRing_Parallel` — Darwin: 122.68 ns/op, Linux: 99.61 ns/op (Linux faster)
- `Benchmark_microtaskRing_Push` — Darwin: 27.60 ns/op, Linux: 44.35 ns/op (Darwin faster)
- `Benchmark_microtaskRing_PushPop` — Darwin: 22.49 ns/op, Linux: 22.52 ns/op (Darwin faster)

### Allocation Mismatches

| Benchmark | Darwin allocs | Linux allocs | Delta |
|-----------|---------------|--------------|-------|
| BenchmarkAutoExit_FastPathExit | 34 | 32 | 2 |
| BenchmarkAutoExit_ImmediateExit | 25 | 23 | 2 |
| BenchmarkAutoExit_PollPathExit | 33 | 31 | 2 |
| BenchmarkAutoExit_TimerFire | 31 | 29 | 2 |
| BenchmarkAutoExit_UnrefExit | 41 | 40 | 1 |
| BenchmarkPromiseAll | 28 | 28 | 0 |
| BenchmarkSetInterval_Parallel_Optimized | 8 | 7 | 1 |

### B/op Mismatches

| Benchmark | Darwin B/op | Linux B/op | Delta |
|-----------|-------------|------------|-------|
| BenchmarkAlive_Epoch_ConcurrentSubmit | 0 | 2 | 2 |
| BenchmarkAlive_WithMutexes_HighContention | 0 | 3 | 3 |
| BenchmarkAutoExit_FastPathExit | 1,254,141 | 1,249,082 | 5,059 |
| BenchmarkAutoExit_ImmediateExit | 1,251,240 | 1,246,642 | 4,598 |
| BenchmarkAutoExit_PollPathExit | 1,253,505 | 1,248,626 | 4,879 |
| BenchmarkAutoExit_TimerFire | 1,253,289 | 1,248,596 | 4,693 |
| BenchmarkAutoExit_UnrefExit | 1,254,532 | 1,250,405 | 4,127 |
| BenchmarkCancelTimer_Individual/timers_1 | 2,001 | 2,002 | 1 |
| BenchmarkCancelTimer_Individual/timers_5 | 10,029 | 10,049 | 20 |
| BenchmarkCancelTimer_Individual/timers_: | 20,145 | 20,227 | 82 |
| BenchmarkCancelTimers_Batch/timers_: | 6,986 | 6,978 | 7 |
| BenchmarkCancelTimers_Comparison/Batch | 3,100 | 3,098 | 1 |
| BenchmarkCancelTimers_Comparison/Individual | 9,614 | 9,633 | 19 |
| BenchmarkHighContention | 1 | 46 | 46 |
| BenchmarkLatencymicrotaskRingPush | 44 | 44 | 0 |
| BenchmarkMetricsCollection | 42 | 44 | 1 |
| BenchmarkMicrotaskExecution | 16 | 58 | 42 |
| BenchmarkMicrotaskSchedule | 9 | 45 | 36 |
| BenchmarkMicrotaskSchedule_Parallel | 0 | 43 | 43 |
| BenchmarkMixedWorkload | 45 | 48 | 3 |
| BenchmarkPromiseAll | 1,240 | 1,240 | 0 |
| BenchmarkPromiseChain | 481 | 480 | 1 |
| BenchmarkPromiseGC | 9,601 | 9,600 | 1 |
| BenchmarkPromiseThenChain | 519 | 512 | 6 |
| BenchmarkPromisifyAllocation | 731 | 751 | 20 |
| BenchmarkQueueMicrotask | 3 | 45 | 41 |
| BenchmarkRefUnref_SubmitInternal_External | 64 | 66 | 2 |
| BenchmarkScheduleTimerWithPool | 61 | 34 | 26 |
| BenchmarkScheduleTimerWithPool_Immediate | 32 | 33 | 1 |
| BenchmarkSentinelDrain_WithTimers | 322 | 298 | 24 |
| BenchmarkSetInterval_Optimized | 296 | 296 | 0 |
| BenchmarkSetInterval_Parallel_Optimized | 412 | 344 | 68 |
| BenchmarkTask1_2_ConcurrentSubmissions | 1 | 0 | 1 |
| BenchmarkTerminated_RejectionPath_Promisify | 181 | 191 | 10 |
| BenchmarkTimerFire | 50 | 54 | 4 |
| BenchmarkTimerSchedule_Parallel | 192 | 193 | 1 |
| BenchmarkWakeUpDeduplicationIntegration | 1 | 0 | 1 |
| Benchmark_microtaskRing_Parallel | 43 | 43 | 0 |
| Benchmark_microtaskRing_Push | 42 | 43 | 1 |

## Measurement Stability

Coefficient of variation (CV%) indicates measurement consistency. Lower is better.

- Benchmarks with CV < 2% on both platforms: **42**
- Darwin benchmarks with CV > 5%: **42**
- Linux benchmarks with CV > 5%: **43**

### High-Variance Benchmarks (CV > 5%)

| Benchmark | Darwin CV% | Linux CV% |
|-----------|------------|-----------|
| BenchmarkAlive_AllAtomic_Contended | 2.0% | 32.4% (high) |
| BenchmarkAlive_Epoch_ConcurrentSubmit | 2.5% | 16.0% (high) |
| BenchmarkAlive_Epoch_FromCallback | 23.9% (high) | 59.9% (high) |
| BenchmarkAlive_WithMutexes_HighContention | 5.0% | 14.9% (high) |
| BenchmarkAlive_WithTimer | 13.5% (high) | 0.5% |
| BenchmarkAutoExit_FastPathExit | 5.7% (high) | 106.3% (high) |
| BenchmarkAutoExit_ImmediateExit | 1.4% | 107.7% (high) |
| BenchmarkAutoExit_PollPathExit | 32.8% (high) | 49.8% (high) |
| BenchmarkAutoExit_TimerFire | 8.6% (high) | 6.6% (high) |
| BenchmarkCancelTimer_Individual/timers_5 | 7.6% (high) | 1.7% |
| BenchmarkCancelTimers_Comparison/Individual | 7.4% (high) | 2.5% |
| BenchmarkGetGoroutineID | 18.7% (high) | 16.0% (high) |
| BenchmarkGojaStyleSwap | 4.6% | 19.7% (high) |
| BenchmarkIsLoopThread_False | 11.7% (high) | 5.4% (high) |
| BenchmarkLargeTimerHeap | 9.2% (high) | 2.0% |
| BenchmarkLatencyAnalysis_EndToEnd | 5.2% (high) | 4.7% |
| BenchmarkLatencyAnalysis_PingPong | 2.3% | 15.2% (high) |
| BenchmarkLatencyRWMutexRLockRUnlock | 24.6% (high) | 0.5% |
| BenchmarkLatencyRecord_WithPSquare | 7.0% (high) | 9.5% (high) |
| BenchmarkLatencySimulatedPoll | 16.8% (high) | 4.3% |
| BenchmarkLatencySimulatedSubmit | 5.7% (high) | 3.8% |
| BenchmarkLatencyStateLoad | 5.4% (high) | 1.1% |
| BenchmarkLatencychunkedIngressPop | 47.5% (high) | 40.9% (high) |
| BenchmarkLatencychunkedIngressPush | 4.3% | 13.4% (high) |
| BenchmarkLatencymicrotaskRingPop | 12.2% (high) | 0.2% |
| BenchmarkLatencymicrotaskRingPush | 13.6% (high) | 11.0% (high) |
| BenchmarkMetricsCollection | 11.6% (high) | 6.6% (high) |
| BenchmarkMicrotaskRingIsEmpty_WithItems | 3.4% | 17.7% (high) |
| BenchmarkMicrotaskSchedule | 7.2% (high) | 8.4% (high) |
| BenchmarkMicrotaskSchedule_Parallel | 1.9% | 9.4% (high) |
| BenchmarkNoMetrics | 6.5% (high) | 3.9% |
| BenchmarkPromiseAll | 1.3% | 5.9% (high) |
| BenchmarkPromiseChain | 30.3% (high) | 3.7% |
| BenchmarkPromiseGC | 93.9% (high) | 2.2% |
| BenchmarkPromiseReject | 6.0% (high) | 1.2% |
| BenchmarkPromiseRejection | 27.9% (high) | 2.8% |
| BenchmarkPromiseResolve | 23.3% (high) | 1.3% |
| BenchmarkPromiseThen | 1.7% | 16.6% (high) |
| BenchmarkPromiseWithResolvers | 4.8% | 7.1% (high) |
| BenchmarkPureChannelPingPong | 20.5% (high) | 1.2% |
| BenchmarkQueueMicrotask | 2.6% | 12.8% (high) |
| BenchmarkQuiescing_ScheduleTimer_NoAutoExit | 46.5% (high) | 5.3% (high) |
| BenchmarkRefUnref_IsLoopThread_External | 15.1% (high) | 3.0% |
| BenchmarkRefUnref_IsLoopThread_OnLoop | 15.5% (high) | 1.5% |
| BenchmarkRefUnref_RWMutex_OnLoop | 0.2% | 7.1% (high) |
| BenchmarkRefUnref_SubmitInternal_OnLoop | 0.6% | 11.7% (high) |
| BenchmarkRefUnref_SyncMap_External | 0.2% | 9.7% (high) |
| BenchmarkRegression_FastPathWakeup_NoWork | 28.4% (high) | 0.6% |
| BenchmarkRegression_HasInternalTasks_SimulatedAtomic | 0.6% | 11.5% (high) |
| BenchmarkScheduleTimerCancel | 3.5% | 16.5% (high) |
| BenchmarkSentinelDrain_NoWork | 57.3% (high) | 136.1% (high) |
| BenchmarkSentinelIteration | 47.9% (high) | 65.4% (high) |
| BenchmarkSentinelIteration_WithTimers | 12.2% (high) | 2.7% |
| BenchmarkSetInterval_Optimized | 17.1% (high) | 8.9% (high) |
| BenchmarkSetTimeoutZeroDelay | 1.8% | 15.7% (high) |
| BenchmarkSetTimeout_Optimized | 22.1% (high) | 6.7% (high) |
| BenchmarkSubmit | 5.4% (high) | 3.6% |
| BenchmarkSubmitInternal_QueuePath_OnLoop | 3.5% | 7.1% (high) |
| BenchmarkSubmit_Parallel | 2.0% | 15.6% (high) |
| BenchmarkTimerFire | 5.1% (high) | 0.3% |
| BenchmarkTimerHeapOperations | 6.8% (high) | 8.8% (high) |
| BenchmarkTimerSchedule | 1.1% | 13.4% (high) |
| Benchmark_chunkedIngress_Pop | 38.3% (high) | 22.9% (high) |
| Benchmark_chunkedIngress_Push | 5.0% | 9.9% (high) |
| Benchmark_microtaskRing_Parallel | 0.6% | 7.1% (high) |
| Benchmark_microtaskRing_Push | 13.6% (high) | 20.3% (high) |

## Key Findings

### 1. Architecture Parity

Both platforms run ARM64, eliminating architectural differences. Performance gaps
are attributable to:
- **OS kernel scheduling** (macOS Mach scheduler vs Linux CFS)
- **Memory management** (macOS memory pressure vs Linux cgroups in container)
- **Syscall overhead** differences
- **Go runtime behavior** variations between `darwin/arm64` and `linux/arm64`

### 2. Performance Distribution

- Darwin significantly faster (ratio < 0.9): **60** benchmarks
- Roughly equal (0.9–1.1x): **59** benchmarks
- Linux significantly faster (ratio > 1.1): **39** benchmarks

### 3. Timer Operations

- Total timer benchmarks: 27
- Darwin faster: 19
- Linux faster: 8
- Biggest difference: `BenchmarkCancelTimer_Individual/timers_:` — Linux is 1.93x slower

### 4. Concurrency & Contention

- `BenchmarkAlive_Epoch_ConcurrentSubmit`: Darwin (2.50x)
- `BenchmarkAlive_Epoch_NoContention`: Darwin (1.12x)
- `BenchmarkAlive_WithMutexes_HighContention`: Darwin (3.19x)
- `BenchmarkFastPathSubmit`: Linux (1.30x)
- `BenchmarkHighContention`: Linux (1.47x)
- `BenchmarkLatencyAnalysis_SubmitWhileRunning`: Linux (1.29x)
- `BenchmarkLatencySimulatedSubmit`: Darwin (1.09x)
- `BenchmarkLatencychunkedIngressPush_WithContention`: Linux (2.06x)
- `BenchmarkLoopDirectWithSubmit`: Darwin (2.53x)
- `BenchmarkMicrotaskSchedule_Parallel`: Linux (1.48x)
- `BenchmarkPromiseHandlerTracking_Parallel_Optimized`: Linux (2.07x)
- `BenchmarkRefUnref_SubmitInternal_External`: Darwin (2.60x)
- `BenchmarkRefUnref_SubmitInternal_OnLoop`: Darwin (1.18x)
- `BenchmarkSetInterval_Parallel_Optimized`: Darwin (1.26x)
- `BenchmarkSubmit`: Linux (1.33x)
- `BenchmarkSubmitExecution`: Linux (1.18x)
- `BenchmarkSubmitInternal`: Linux (1.24x)
- `BenchmarkSubmitInternal_Cost`: Linux (1.23x)
- `BenchmarkSubmitInternal_FastPath_OnLoop`: Linux (1.01x)
- `BenchmarkSubmitInternal_QueuePath_OnLoop`: Darwin (1.31x)
- `BenchmarkSubmitLatency`: Linux (1.30x)
- `BenchmarkSubmit_Parallel`: Linux (1.53x)
- `BenchmarkTask1_2_ConcurrentSubmissions`: Linux (1.57x)
- `BenchmarkTerminated_RejectionPath_submitToQueue`: Darwin (1.10x)
- `BenchmarkTimerSchedule_Parallel`: Darwin (1.50x)
- `Benchmark_chunkedIngress_ParallelWithSync`: Linux (1.89x)
- `Benchmark_microtaskRing_Parallel`: Linux (1.23x)

### 5. Summary

**Darwin wins overall** with 99/158 benchmarks faster.

The mean performance ratio of 0.962x (Darwin/Linux) indicates
the platforms are remarkably close in overall performance, with each
excelling in different workload categories.

