# Darwin vs Linux Benchmark Comparison

**Date:** 2026-02-10
**Platforms:** Darwin ARM64 (macOS, GOMAXPROCS=10) vs Linux ARM64 (container, GOMAXPROCS=10)
**Methodology:** `go test -bench=. -benchmem -count=5 -run=^$ -benchtime=1s -timeout=10m`
**Benchmarks Compared:** 108 common benchmarks

## Executive Summary

This report compares eventloop benchmark performance between **Darwin (macOS)** and **Linux**,
both running on **ARM64** architecture. Since the architecture is identical, performance
differences reflect OS-level differences: kernel scheduling, memory management, syscall
overhead, and Go runtime behavior on each OS.

### Key Metrics

| Metric | Value |
|--------|-------|
| Common benchmarks | 108 |
| Darwin-only benchmarks | 0 |
| Linux-only benchmarks | 0 |
| Darwin wins (faster) | **55** (50.9%) |
| Linux wins (faster) | **53** (49.1%) |
| Ties | 0 |
| Statistically significant differences | 70 |
| Darwin mean (common benchmarks) | 27,508.22 ns/op |
| Linux mean (common benchmarks) | 80,983.90 ns/op |
| Mean ratio (Darwin/Linux) | 0.980x |
| Median ratio (Darwin/Linux) | 0.999x |
| Allocation match rate | 97/108 (89.8%) |
| Zero-allocation benchmarks (both) | 46 |

## Full Statistical Comparison Table

| # | Benchmark | Darwin (ns/op) | Darwin CV% | Linux (ns/op) | Linux CV% | Faster | Ratio | Sig? |
|---|-----------|----------------|------------|---------------|-----------|--------|-------|------|
| 1 | BenchmarkCancelTimer_Individual/timers_1 |     124,843.60 |       9.4% |    342,611.20 |      1.1% | 🍎 Darwin |  0.36x | ✅ |
| 2 | BenchmarkCancelTimer_Individual/timers_5 |     593,275.00 |       1.6% |  1,775,338.20 |      4.2% | 🍎 Darwin |  0.33x | ✅ |
| 3 | BenchmarkCancelTimer_Individual/timers_: |   1,181,246.20 |       2.4% |  3,570,868.60 |      7.7% | 🍎 Darwin |  0.33x | ✅ |
| 4 | BenchmarkCancelTimers_Batch/timers_1 |      38,474.00 |       0.8% |     72,940.20 |      1.0% | 🍎 Darwin |  0.53x | ✅ |
| 5 | BenchmarkCancelTimers_Batch/timers_5 |      48,594.20 |       1.4% |    205,931.20 |      0.7% | 🍎 Darwin |  0.24x | ✅ |
| 6 | BenchmarkCancelTimers_Batch/timers_: |      57,085.80 |       1.1% |    344,207.60 |      0.3% | 🍎 Darwin |  0.17x | ✅ |
| 7 | BenchmarkCancelTimers_Comparison/Batch |      48,169.80 |       1.7% |    208,699.80 |      0.8% | 🍎 Darwin |  0.23x | ✅ |
| 8 | BenchmarkCancelTimers_Comparison/Individual |     602,813.80 |       1.7% |  1,732,770.80 |      2.6% | 🍎 Darwin |  0.35x | ✅ |
| 9 | BenchmarkChannelWithMutexQueue |         466.06 |       2.0% |        422.92 |      0.7% | 🐧 Linux |  1.10x | ✅ |
| 10 | BenchmarkCombinedWorkload_New |          83.94 |       0.2% |         84.03 |      0.0% | 🍎 Darwin |  1.00x |  |
| 11 | BenchmarkCombinedWorkload_Old |         345.04 |       1.8% |        344.10 |      0.6% | 🐧 Linux |  1.00x |  |
| 12 | BenchmarkFastPathExecution |         104.52 |       1.7% |         41.95 |      4.2% | 🐧 Linux |  2.49x | ✅ |
| 13 | BenchmarkFastPathSubmit |          38.58 |       3.2% |         33.50 |      2.7% | 🐧 Linux |  1.15x | ✅ |
| 14 | BenchmarkGojaStyleSwap |         472.76 |       7.9% |        400.38 |      1.0% | 🐧 Linux |  1.18x | ✅ |
| 15 | BenchmarkHighContention |         222.12 |       0.9% |        119.46 |      1.5% | 🐧 Linux |  1.86x | ✅ |
| 16 | BenchmarkHighFrequencyMonitoring_New |          24.60 |       0.3% |         24.61 |      0.4% | 🍎 Darwin |  1.00x |  |
| 17 | BenchmarkHighFrequencyMonitoring_Old |      23,700.20 |       2.4% |     23,668.20 |      3.5% | 🐧 Linux |  1.00x |  |
| 18 | BenchmarkLargeTimerHeap |      12,812.20 |       5.7% |     34,918.00 |      2.2% | 🍎 Darwin |  0.37x | ✅ |
| 19 | BenchmarkLatencyAnalysis_EndToEnd |         591.78 |       8.7% |        559.20 |      1.6% | 🐧 Linux |  1.06x |  |
| 20 | BenchmarkLatencyAnalysis_PingPong |         594.86 |       2.4% |        418.96 |      1.4% | 🐧 Linux |  1.42x | ✅ |
| 21 | BenchmarkLatencyAnalysis_SubmitWhileRunning |         434.86 |       3.7% |        325.24 |      2.8% | 🐧 Linux |  1.34x | ✅ |
| 22 | BenchmarkLatencyChannelBufferedRoundTrip |         330.44 |       1.0% |        241.68 |      1.1% | 🐧 Linux |  1.37x | ✅ |
| 23 | BenchmarkLatencyChannelRoundTrip |         348.64 |       0.6% |        242.80 |      2.6% | 🐧 Linux |  1.44x | ✅ |
| 24 | BenchmarkLatencyDeferRecover |           2.40 |       0.9% |          2.38 |      0.2% | 🐧 Linux |  1.01x |  |
| 25 | BenchmarkLatencyDirectCall |           0.30 |       0.7% |          0.30 |      0.1% | 🐧 Linux |  1.01x | ✅ |
| 26 | BenchmarkLatencyMutexLockUnlock |           8.52 |       9.8% |          7.53 |      0.1% | 🐧 Linux |  1.13x |  |
| 27 | BenchmarkLatencyRWMutexRLockRUnlock |           8.34 |       7.1% |          7.88 |      1.7% | 🐧 Linux |  1.06x |  |
| 28 | BenchmarkLatencyRecord_WithPSquare |          74.89 |       0.8% |         74.76 |      0.1% | 🐧 Linux |  1.00x |  |
| 29 | BenchmarkLatencyRecord_WithoutPSquare |          23.90 |       0.5% |         23.31 |      0.4% | 🐧 Linux |  1.03x | ✅ |
| 30 | BenchmarkLatencySafeExecute |           3.10 |       7.5% |          3.03 |      3.8% | 🐧 Linux |  1.02x |  |
| 31 | BenchmarkLatencySample_NewPSquare |          26.27 |       6.2% |         25.23 |      4.5% | 🐧 Linux |  1.04x |  |
| 32 | BenchmarkLatencySample_OldSortBased |      17,562.80 |       3.1% |     16,505.40 |      2.9% | 🐧 Linux |  1.06x | ✅ |
| 33 | BenchmarkLatencySimulatedPoll |          12.49 |       0.1% |         14.15 |      5.5% | 🍎 Darwin |  0.88x | ✅ |
| 34 | BenchmarkLatencySimulatedSubmit |          12.93 |       8.9% |         13.93 |      1.9% | 🍎 Darwin |  0.93x |  |
| 35 | BenchmarkLatencyStateLoad |           0.30 |       1.1% |          0.32 |      1.9% | 🍎 Darwin |  0.93x | ✅ |
| 36 | BenchmarkLatencyStateTryTransition |           4.03 |       3.1% |          4.08 |      2.5% | 🍎 Darwin |  0.99x |  |
| 37 | BenchmarkLatencyStateTryTransition_NoOp |          17.08 |       1.2% |         16.36 |      3.6% | 🐧 Linux |  1.04x |  |
| 38 | BenchmarkLatencychunkedIngressPop |           3.19 |       8.3% |          4.16 |      2.9% | 🍎 Darwin |  0.77x | ✅ |
| 39 | BenchmarkLatencychunkedIngressPush |           5.02 |       3.4% |          8.20 |     18.0% | 🍎 Darwin |  0.61x | ✅ |
| 40 | BenchmarkLatencychunkedIngressPushPop |           3.95 |       0.5% |          4.08 |      2.9% | 🍎 Darwin |  0.97x |  |
| 41 | BenchmarkLatencychunkedIngressPush_WithContention |          70.13 |       1.5% |         38.88 |      2.2% | 🐧 Linux |  1.80x | ✅ |
| 42 | BenchmarkLatencymicrotaskRingPop |          15.30 |       0.6% |         15.72 |      0.3% | 🍎 Darwin |  0.97x | ✅ |
| 43 | BenchmarkLatencymicrotaskRingPush |          24.70 |       3.0% |         26.08 |      4.1% | 🍎 Darwin |  0.95x |  |
| 44 | BenchmarkLatencymicrotaskRingPushPop |          22.43 |       1.8% |         22.11 |      1.9% | 🐧 Linux |  1.01x |  |
| 45 | BenchmarkLoopDirect |         471.70 |       0.8% |        482.62 |      2.3% | 🍎 Darwin |  0.98x |  |
| 46 | BenchmarkLoopDirectWithSubmit |      11,695.80 |       7.3% |     34,474.00 |      0.5% | 🍎 Darwin |  0.34x | ✅ |
| 47 | BenchmarkMetricsCollection |          32.08 |      11.0% |         34.63 |      8.5% | 🍎 Darwin |  0.93x |  |
| 48 | BenchmarkMicroPingPong |         430.88 |       2.8% |        404.72 |     13.6% | 🐧 Linux |  1.06x |  |
| 49 | BenchmarkMicroPingPongWithCount |         442.34 |       2.5% |        429.08 |      1.1% | 🐧 Linux |  1.03x |  |
| 50 | BenchmarkMicrotaskExecution |          84.54 |       6.9% |        103.36 |      2.1% | 🍎 Darwin |  0.82x | ✅ |
| 51 | BenchmarkMicrotaskLatency |         455.64 |       0.6% |        344.12 |      4.4% | 🐧 Linux |  1.32x | ✅ |
| 52 | BenchmarkMicrotaskOverflow |          23.97 |       0.3% |         24.52 |      1.6% | 🍎 Darwin |  0.98x | ✅ |
| 53 | BenchmarkMicrotaskSchedule |          78.23 |       5.7% |         60.69 |      8.5% | 🐧 Linux |  1.29x | ✅ |
| 54 | BenchmarkMicrotaskSchedule_Parallel |         109.64 |       0.4% |         60.13 |      2.1% | 🐧 Linux |  1.82x | ✅ |
| 55 | BenchmarkMinimalLoop |         460.50 |       3.0% |        405.72 |      1.7% | 🐧 Linux |  1.14x | ✅ |
| 56 | BenchmarkMixedWorkload |         132.14 |       1.0% |        244.74 |     25.5% | 🍎 Darwin |  0.54x | ✅ |
| 57 | BenchmarkNoMetrics |          38.96 |       5.6% |         38.71 |     19.1% | 🐧 Linux |  1.01x |  |
| 58 | BenchmarkPromiseAll |       1,522.80 |       4.2% |      1,758.20 |      3.7% | 🍎 Darwin |  0.87x | ✅ |
| 59 | BenchmarkPromiseAll_Memory |       1,490.80 |       0.8% |      1,486.40 |      4.2% | 🐧 Linux |  1.00x |  |
| 60 | BenchmarkPromiseChain |         450.68 |       5.5% |        544.18 |      3.0% | 🍎 Darwin |  0.83x | ✅ |
| 61 | BenchmarkPromiseCreate |          55.71 |       2.0% |         68.76 |     12.8% | 🍎 Darwin |  0.81x | ✅ |
| 62 | BenchmarkPromiseCreation |          66.04 |       2.5% |         64.54 |      0.6% | 🐧 Linux |  1.02x |  |
| 63 | BenchmarkPromiseGC |      59,481.40 |       0.6% |     92,369.80 |      7.7% | 🍎 Darwin |  0.64x | ✅ |
| 64 | BenchmarkPromiseHandlerTracking_Optimized |          80.64 |       5.1% |        108.74 |      8.2% | 🍎 Darwin |  0.74x | ✅ |
| 65 | BenchmarkPromiseHandlerTracking_Parallel_Optimized |         334.82 |       1.1% |        177.92 |      1.9% | 🐧 Linux |  1.88x | ✅ |
| 66 | BenchmarkPromiseRace |       1,289.40 |       0.3% |      1,329.40 |      4.4% | 🍎 Darwin |  0.97x |  |
| 67 | BenchmarkPromiseReject |         545.74 |       1.7% |        531.84 |      1.2% | 🐧 Linux |  1.03x |  |
| 68 | BenchmarkPromiseRejection |         530.96 |       9.1% |        526.32 |      1.7% | 🐧 Linux |  1.01x |  |
| 69 | BenchmarkPromiseResolution |         101.24 |       5.6% |         96.20 |      2.5% | 🐧 Linux |  1.05x |  |
| 70 | BenchmarkPromiseResolve |          81.93 |       1.9% |         97.61 |     14.0% | 🍎 Darwin |  0.84x |  |
| 71 | BenchmarkPromiseResolve_Memory |          99.54 |       5.8% |        105.25 |     10.8% | 🍎 Darwin |  0.95x |  |
| 72 | BenchmarkPromiseThen |         323.12 |       0.7% |        318.40 |      5.4% | 🐧 Linux |  1.01x |  |
| 73 | BenchmarkPromiseThenChain |         563.74 |       1.4% |        622.94 |      3.5% | 🍎 Darwin |  0.90x | ✅ |
| 74 | BenchmarkPromiseTry |          97.99 |       0.9% |        105.16 |      6.6% | 🍎 Darwin |  0.93x |  |
| 75 | BenchmarkPromiseWithResolvers |          94.61 |       1.7% |         99.88 |      0.8% | 🍎 Darwin |  0.95x | ✅ |
| 76 | BenchmarkPromisifyAllocation |       5,472.60 |       1.4% |      6,605.20 |      4.2% | 🍎 Darwin |  0.83x | ✅ |
| 77 | BenchmarkPureChannelPingPong |         351.62 |       0.9% |        340.54 |      1.4% | 🐧 Linux |  1.03x | ✅ |
| 78 | BenchmarkQueueMicrotask |          80.75 |       5.7% |         57.34 |      4.1% | 🐧 Linux |  1.41x | ✅ |
| 79 | BenchmarkScheduleTimerCancel |      19,385.80 |       3.2% |     33,705.20 |     19.2% | 🍎 Darwin |  0.58x | ✅ |
| 80 | BenchmarkScheduleTimerWithPool |         481.08 |       5.0% |        463.26 |      5.4% | 🐧 Linux |  1.04x |  |
| 81 | BenchmarkScheduleTimerWithPool_FireAndReuse |         276.84 |       3.6% |        459.26 |      5.1% | 🍎 Darwin |  0.60x | ✅ |
| 82 | BenchmarkScheduleTimerWithPool_Immediate |         222.14 |       2.8% |        322.86 |      4.6% | 🍎 Darwin |  0.69x | ✅ |
| 83 | BenchmarkSetImmediate_Optimized |         157.68 |       4.7% |        117.90 |     10.6% | 🐧 Linux |  1.34x | ✅ |
| 84 | BenchmarkSetInterval_Optimized |      21,956.60 |       1.6% |     39,217.40 |      3.5% | 🍎 Darwin |  0.56x | ✅ |
| 85 | BenchmarkSetInterval_Parallel_Optimized |       6,114.60 |       1.9% |     17,232.60 |      1.7% | 🍎 Darwin |  0.35x | ✅ |
| 86 | BenchmarkSetTimeoutZeroDelay |      20,987.00 |       5.3% |     43,203.40 |     11.6% | 🍎 Darwin |  0.49x | ✅ |
| 87 | BenchmarkSetTimeout_Optimized |      20,230.40 |       6.0% |     38,020.40 |      8.8% | 🍎 Darwin |  0.53x | ✅ |
| 88 | BenchmarkSubmit |          40.23 |       4.2% |         33.05 |      1.6% | 🐧 Linux |  1.22x | ✅ |
| 89 | BenchmarkSubmitExecution |         103.55 |       2.8% |         46.37 |     10.7% | 🐧 Linux |  2.23x | ✅ |
| 90 | BenchmarkSubmitInternal |       3,537.60 |       1.9% |      3,020.20 |      5.7% | 🐧 Linux |  1.17x | ✅ |
| 91 | BenchmarkSubmitLatency |         438.66 |       0.7% |        322.90 |      2.7% | 🐧 Linux |  1.36x | ✅ |
| 92 | BenchmarkSubmit_Parallel |         105.66 |       0.6% |         62.86 |      3.2% | 🐧 Linux |  1.68x | ✅ |
| 93 | BenchmarkTask1_2_ConcurrentSubmissions |         105.88 |       0.7% |         69.19 |      3.8% | 🐧 Linux |  1.53x | ✅ |
| 94 | BenchmarkTimerFire |         257.34 |       6.2% |        350.32 |     13.3% | 🍎 Darwin |  0.73x | ✅ |
| 95 | BenchmarkTimerHeapOperations |          62.71 |       3.0% |         78.86 |      6.6% | 🍎 Darwin |  0.80x | ✅ |
| 96 | BenchmarkTimerLatency |      11,725.80 |       0.6% |     40,101.80 |     13.7% | 🍎 Darwin |  0.29x | ✅ |
| 97 | BenchmarkTimerSchedule |      18,164.00 |       2.5% |     36,807.20 |      1.6% | 🍎 Darwin |  0.49x | ✅ |
| 98 | BenchmarkTimerSchedule_Parallel |       5,096.00 |       0.7% |     15,283.60 |      0.8% | 🍎 Darwin |  0.33x | ✅ |
| 99 | BenchmarkWakeUpDeduplicationIntegration |         102.82 |       3.8% |         71.02 |      2.1% | 🐧 Linux |  1.45x | ✅ |
| 100 | Benchmark_chunkedIngress_Batch |         507.00 |       3.9% |        517.62 |      0.6% | 🍎 Darwin |  0.98x |  |
| 101 | Benchmark_chunkedIngress_ParallelWithSync |          87.80 |       2.6% |         43.54 |      2.4% | 🐧 Linux |  2.02x | ✅ |
| 102 | Benchmark_chunkedIngress_Pop |           3.42 |       8.5% |          4.21 |      4.6% | 🍎 Darwin |  0.81x | ✅ |
| 103 | Benchmark_chunkedIngress_Push |           5.06 |       5.0% |          6.77 |     13.2% | 🍎 Darwin |  0.75x | ✅ |
| 104 | Benchmark_chunkedIngress_PushPop |           4.08 |       0.8% |          4.05 |      3.9% | 🐧 Linux |  1.01x |  |
| 105 | Benchmark_chunkedIngress_Sequential |           4.06 |       7.5% |          4.08 |      1.3% | 🍎 Darwin |  0.99x |  |
| 106 | Benchmark_microtaskRing_Parallel |         130.08 |       1.8% |         89.97 |      3.5% | 🐧 Linux |  1.45x | ✅ |
| 107 | Benchmark_microtaskRing_Push |          24.37 |      10.5% |         27.43 |     11.5% | 🍎 Darwin |  0.89x |  |
| 108 | Benchmark_microtaskRing_PushPop |          21.95 |       1.3% |         21.69 |      0.8% | 🐧 Linux |  1.01x |  |

## Performance by Category

### Concurrency (3 benchmarks)

- Darwin wins: 1/3
- Linux wins: 2/3
- Darwin category mean: 2,147.53 ns/op
- Linux category mean: 5,807.08 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkSetInterval_Parallel_Optimized |       6,114.60 |     17,232.60 | 🍎 | 2.82x |
| BenchmarkHighContention |         222.12 |        119.46 | 🐧 | 1.86x |
| BenchmarkTask1_2_ConcurrentSubmissions |         105.88 |         69.19 | 🐧 | 1.53x |

### Latency & Primitives (29 benchmarks)

- Darwin wins: 10/29
- Linux wins: 19/29
- Darwin category mean: 1,131.82 ns/op
- Linux category mean: 2,047.40 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkTimerLatency |      11,725.80 |     40,101.80 | 🍎 | 3.42x |
| BenchmarkLatencySample_OldSortBased |      17,562.80 |     16,505.40 | 🐧 | 1.06x |
| BenchmarkLatencyAnalysis_PingPong |         594.86 |        418.96 | 🐧 | 1.42x |
| BenchmarkSubmitLatency |         438.66 |        322.90 | 🐧 | 1.36x |
| BenchmarkMicrotaskLatency |         455.64 |        344.12 | 🐧 | 1.32x |
| BenchmarkLatencyAnalysis_SubmitWhileRunning |         434.86 |        325.24 | 🐧 | 1.34x |
| BenchmarkLatencyChannelRoundTrip |         348.64 |        242.80 | 🐧 | 1.44x |
| BenchmarkLatencyChannelBufferedRoundTrip |         330.44 |        241.68 | 🐧 | 1.37x |
| BenchmarkLatencyAnalysis_EndToEnd |         591.78 |        559.20 | 🐧 | 1.06x |
| BenchmarkLatencychunkedIngressPush_WithContention |          70.13 |         38.88 | 🐧 | 1.80x |
| BenchmarkLatencychunkedIngressPush |           5.02 |          8.20 | 🍎 | 1.63x |
| BenchmarkLatencySimulatedPoll |          12.49 |         14.15 | 🍎 | 1.13x |
| BenchmarkLatencymicrotaskRingPush |          24.70 |         26.08 | 🍎 | 1.06x |
| BenchmarkLatencySample_NewPSquare |          26.27 |         25.23 | 🐧 | 1.04x |
| BenchmarkLatencySimulatedSubmit |          12.93 |         13.93 | 🍎 | 1.08x |
| BenchmarkLatencyMutexLockUnlock |           8.52 |          7.53 | 🐧 | 1.13x |
| BenchmarkLatencychunkedIngressPop |           3.19 |          4.16 | 🍎 | 1.30x |
| BenchmarkLatencyStateTryTransition_NoOp |          17.08 |         16.36 | 🐧 | 1.04x |
| BenchmarkLatencyRecord_WithoutPSquare |          23.90 |         23.31 | 🐧 | 1.03x |
| BenchmarkLatencyRWMutexRLockRUnlock |           8.34 |          7.88 | 🐧 | 1.06x |
| BenchmarkLatencymicrotaskRingPop |          15.30 |         15.72 | 🍎 | 1.03x |
| BenchmarkLatencymicrotaskRingPushPop |          22.43 |         22.11 | 🐧 | 1.01x |
| BenchmarkLatencyRecord_WithPSquare |          74.89 |         74.76 | 🐧 | 1.00x |
| BenchmarkLatencychunkedIngressPushPop |           3.95 |          4.08 | 🍎 | 1.03x |
| BenchmarkLatencySafeExecute |           3.10 |          3.03 | 🐧 | 1.02x |
| BenchmarkLatencyStateTryTransition |           4.03 |          4.08 | 🍎 | 1.01x |
| BenchmarkLatencyStateLoad |           0.30 |          0.32 | 🍎 | 1.08x |
| BenchmarkLatencyDeferRecover |           2.40 |          2.38 | 🐧 | 1.01x |
| BenchmarkLatencyDirectCall |           0.30 |          0.30 | 🐧 | 1.01x |

### Other (20 benchmarks)

- Darwin wins: 9/20
- Linux wins: 11/20
- Darwin category mean: 4,818.00 ns/op
- Linux category mean: 7,728.02 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkSetTimeoutZeroDelay |      20,987.00 |     43,203.40 | 🍎 | 2.06x |
| BenchmarkSetTimeout_Optimized |      20,230.40 |     38,020.40 | 🍎 | 1.88x |
| BenchmarkSetInterval_Optimized |      21,956.60 |     39,217.40 | 🍎 | 1.79x |
| BenchmarkPromisifyAllocation |       5,472.60 |      6,605.20 | 🍎 | 1.21x |
| BenchmarkMixedWorkload |         132.14 |        244.74 | 🍎 | 1.85x |
| BenchmarkGojaStyleSwap |         472.76 |        400.38 | 🐧 | 1.18x |
| BenchmarkMinimalLoop |         460.50 |        405.72 | 🐧 | 1.14x |
| BenchmarkChannelWithMutexQueue |         466.06 |        422.92 | 🐧 | 1.10x |
| BenchmarkSetImmediate_Optimized |         157.68 |        117.90 | 🐧 | 1.34x |
| BenchmarkHighFrequencyMonitoring_Old |      23,700.20 |     23,668.20 | 🐧 | 1.00x |
| BenchmarkWakeUpDeduplicationIntegration |         102.82 |         71.02 | 🐧 | 1.45x |
| BenchmarkMicroPingPong |         430.88 |        404.72 | 🐧 | 1.06x |
| BenchmarkMicroPingPongWithCount |         442.34 |        429.08 | 🐧 | 1.03x |
| BenchmarkPureChannelPingPong |         351.62 |        340.54 | 🐧 | 1.03x |
| BenchmarkLoopDirect |         471.70 |        482.62 | 🍎 | 1.02x |
| BenchmarkMetricsCollection |          32.08 |         34.63 | 🍎 | 1.08x |
| BenchmarkCombinedWorkload_Old |         345.04 |        344.10 | 🐧 | 1.00x |
| BenchmarkNoMetrics |          38.96 |         38.71 | 🐧 | 1.01x |
| BenchmarkCombinedWorkload_New |          83.94 |         84.03 | 🍎 | 1.00x |
| BenchmarkHighFrequencyMonitoring_New |          24.60 |         24.61 | 🍎 | 1.00x |

### Promise Operations (18 benchmarks)

- Darwin wins: 11/18
- Linux wins: 7/18
- Darwin category mean: 3,733.95 ns/op
- Linux category mean: 5,578.42 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkPromiseGC |      59,481.40 |     92,369.80 | 🍎 | 1.55x |
| BenchmarkPromiseAll |       1,522.80 |      1,758.20 | 🍎 | 1.15x |
| BenchmarkPromiseHandlerTracking_Parallel_Optimized |         334.82 |        177.92 | 🐧 | 1.88x |
| BenchmarkPromiseChain |         450.68 |        544.18 | 🍎 | 1.21x |
| BenchmarkPromiseThenChain |         563.74 |        622.94 | 🍎 | 1.11x |
| BenchmarkPromiseRace |       1,289.40 |      1,329.40 | 🍎 | 1.03x |
| BenchmarkPromiseHandlerTracking_Optimized |          80.64 |        108.74 | 🍎 | 1.35x |
| BenchmarkPromiseResolve |          81.93 |         97.61 | 🍎 | 1.19x |
| BenchmarkPromiseReject |         545.74 |        531.84 | 🐧 | 1.03x |
| BenchmarkPromiseCreate |          55.71 |         68.76 | 🍎 | 1.23x |
| BenchmarkPromiseTry |          97.99 |        105.16 | 🍎 | 1.07x |
| BenchmarkPromiseResolve_Memory |          99.54 |        105.25 | 🍎 | 1.06x |
| BenchmarkPromiseWithResolvers |          94.61 |         99.88 | 🍎 | 1.06x |
| BenchmarkPromiseResolution |         101.24 |         96.20 | 🐧 | 1.05x |
| BenchmarkPromiseThen |         323.12 |        318.40 | 🐧 | 1.01x |
| BenchmarkPromiseRejection |         530.96 |        526.32 | 🐧 | 1.01x |
| BenchmarkPromiseAll_Memory |       1,490.80 |      1,486.40 | 🐧 | 1.00x |
| BenchmarkPromiseCreation |          66.04 |         64.54 | 🐧 | 1.02x |

### Task Submission (21 benchmarks)

- Darwin wins: 8/21
- Linux wins: 13/21
- Darwin category mean: 799.57 ns/op
- Linux category mean: 1,844.64 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkLoopDirectWithSubmit |      11,695.80 |     34,474.00 | 🍎 | 2.95x |
| BenchmarkSubmitInternal |       3,537.60 |      3,020.20 | 🐧 | 1.17x |
| BenchmarkFastPathExecution |         104.52 |         41.95 | 🐧 | 2.49x |
| BenchmarkSubmitExecution |         103.55 |         46.37 | 🐧 | 2.23x |
| BenchmarkMicrotaskSchedule_Parallel |         109.64 |         60.13 | 🐧 | 1.82x |
| Benchmark_chunkedIngress_ParallelWithSync |          87.80 |         43.54 | 🐧 | 2.02x |
| BenchmarkSubmit_Parallel |         105.66 |         62.86 | 🐧 | 1.68x |
| Benchmark_microtaskRing_Parallel |         130.08 |         89.97 | 🐧 | 1.45x |
| BenchmarkQueueMicrotask |          80.75 |         57.34 | 🐧 | 1.41x |
| BenchmarkMicrotaskExecution |          84.54 |        103.36 | 🍎 | 1.22x |
| BenchmarkMicrotaskSchedule |          78.23 |         60.69 | 🐧 | 1.29x |
| Benchmark_chunkedIngress_Batch |         507.00 |        517.62 | 🍎 | 1.02x |
| BenchmarkSubmit |          40.23 |         33.05 | 🐧 | 1.22x |
| BenchmarkFastPathSubmit |          38.58 |         33.50 | 🐧 | 1.15x |
| Benchmark_microtaskRing_Push |          24.37 |         27.43 | 🍎 | 1.13x |
| Benchmark_chunkedIngress_Push |           5.06 |          6.77 | 🍎 | 1.34x |
| Benchmark_chunkedIngress_Pop |           3.42 |          4.21 | 🍎 | 1.23x |
| BenchmarkMicrotaskOverflow |          23.97 |         24.52 | 🍎 | 1.02x |
| Benchmark_microtaskRing_PushPop |          21.95 |         21.69 | 🐧 | 1.01x |
| Benchmark_chunkedIngress_PushPop |           4.08 |          4.05 | 🐧 | 1.01x |
| Benchmark_chunkedIngress_Sequential |           4.06 |          4.08 | 🍎 | 1.01x |

### Timer Operations (17 benchmarks)

- Darwin wins: 16/17
- Linux wins: 1/17
- Darwin category mean: 161,838.85 ns/op
- Linux category mean: 492,691.54 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkCancelTimer_Individual/timers_: |   1,181,246.20 |  3,570,868.60 | 🍎 | 3.02x |
| BenchmarkCancelTimer_Individual/timers_5 |     593,275.00 |  1,775,338.20 | 🍎 | 2.99x |
| BenchmarkCancelTimers_Comparison/Individual |     602,813.80 |  1,732,770.80 | 🍎 | 2.87x |
| BenchmarkCancelTimers_Batch/timers_: |      57,085.80 |    344,207.60 | 🍎 | 6.03x |
| BenchmarkCancelTimer_Individual/timers_1 |     124,843.60 |    342,611.20 | 🍎 | 2.74x |
| BenchmarkCancelTimers_Comparison/Batch |      48,169.80 |    208,699.80 | 🍎 | 4.33x |
| BenchmarkCancelTimers_Batch/timers_5 |      48,594.20 |    205,931.20 | 🍎 | 4.24x |
| BenchmarkCancelTimers_Batch/timers_1 |      38,474.00 |     72,940.20 | 🍎 | 1.90x |
| BenchmarkLargeTimerHeap |      12,812.20 |     34,918.00 | 🍎 | 2.73x |
| BenchmarkTimerSchedule |      18,164.00 |     36,807.20 | 🍎 | 2.03x |
| BenchmarkScheduleTimerCancel |      19,385.80 |     33,705.20 | 🍎 | 1.74x |
| BenchmarkTimerSchedule_Parallel |       5,096.00 |     15,283.60 | 🍎 | 3.00x |
| BenchmarkScheduleTimerWithPool_FireAndReuse |         276.84 |        459.26 | 🍎 | 1.66x |
| BenchmarkScheduleTimerWithPool_Immediate |         222.14 |        322.86 | 🍎 | 1.45x |
| BenchmarkTimerFire |         257.34 |        350.32 | 🍎 | 1.36x |
| BenchmarkScheduleTimerWithPool |         481.08 |        463.26 | 🐧 | 1.04x |
| BenchmarkTimerHeapOperations |          62.71 |         78.86 | 🍎 | 1.26x |

## Statistically Significant Differences

**70** out of 108 benchmarks show statistically significant
differences (Welch's t-test, p < 0.05).

- Darwin significantly faster: **40** benchmarks
- Linux significantly faster: **30** benchmarks

### Largest Significant Differences

| Benchmark | Faster | Speedup | Darwin (ns/op) | Linux (ns/op) | t-stat |
|-----------|--------|---------|----------------|---------------|--------|
| BenchmarkCancelTimers_Batch/timers_: | 🍎 Darwin | 6.03x |      57,085.80 |    344,207.60 | 534.06 |
| BenchmarkCancelTimers_Comparison/Batch | 🍎 Darwin | 4.33x |      48,169.80 |    208,699.80 | 191.20 |
| BenchmarkCancelTimers_Batch/timers_5 | 🍎 Darwin | 4.24x |      48,594.20 |    205,931.20 | 231.85 |
| BenchmarkTimerLatency | 🍎 Darwin | 3.42x |      11,725.80 |     40,101.80 | 11.54 |
| BenchmarkCancelTimer_Individual/timers_: | 🍎 Darwin | 3.02x |   1,181,246.20 |  3,570,868.60 | 19.33 |
| BenchmarkTimerSchedule_Parallel | 🍎 Darwin | 3.00x |       5,096.00 |     15,283.60 | 185.98 |
| BenchmarkCancelTimer_Individual/timers_5 | 🍎 Darwin | 2.99x |     593,275.00 |  1,775,338.20 | 35.40 |
| BenchmarkLoopDirectWithSubmit | 🍎 Darwin | 2.95x |      11,695.80 |     34,474.00 | 58.91 |
| BenchmarkCancelTimers_Comparison/Individual | 🍎 Darwin | 2.87x |     602,813.80 |  1,732,770.80 | 54.47 |
| BenchmarkSetInterval_Parallel_Optimized | 🍎 Darwin | 2.82x |       6,114.60 |     17,232.60 | 78.42 |
| BenchmarkCancelTimer_Individual/timers_1 | 🍎 Darwin | 2.74x |     124,843.60 |    342,611.20 | 39.47 |
| BenchmarkLargeTimerHeap | 🍎 Darwin | 2.73x |      12,812.20 |     34,918.00 | 46.17 |
| BenchmarkFastPathExecution | 🐧 Linux | 2.49x |         104.52 |         41.95 | 56.78 |
| BenchmarkSubmitExecution | 🐧 Linux | 2.23x |         103.55 |         46.37 | 22.29 |
| BenchmarkSetTimeoutZeroDelay | 🍎 Darwin | 2.06x |      20,987.00 |     43,203.40 | 9.70 |
| BenchmarkTimerSchedule | 🍎 Darwin | 2.03x |      18,164.00 |     36,807.20 | 57.07 |
| Benchmark_chunkedIngress_ParallelWithSync | 🐧 Linux | 2.02x |          87.80 |         43.54 | 39.81 |
| BenchmarkCancelTimers_Batch/timers_1 | 🍎 Darwin | 1.90x |      38,474.00 |     72,940.20 | 96.84 |
| BenchmarkPromiseHandlerTracking_Parallel_Optimized | 🐧 Linux | 1.88x |         334.82 |        177.92 | 69.40 |
| BenchmarkSetTimeout_Optimized | 🍎 Darwin | 1.88x |      20,230.40 |     38,020.40 | 11.23 |
| BenchmarkHighContention | 🐧 Linux | 1.86x |         222.12 |        119.46 | 87.08 |
| BenchmarkMixedWorkload | 🍎 Darwin | 1.85x |         132.14 |        244.74 | 4.04 |
| BenchmarkMicrotaskSchedule_Parallel | 🐧 Linux | 1.82x |         109.64 |         60.13 | 83.30 |
| BenchmarkLatencychunkedIngressPush_WithContention | 🐧 Linux | 1.80x |          70.13 |         38.88 | 50.74 |
| BenchmarkSetInterval_Optimized | 🍎 Darwin | 1.79x |      21,956.60 |     39,217.40 | 27.26 |
| BenchmarkScheduleTimerCancel | 🍎 Darwin | 1.74x |      19,385.80 |     33,705.20 | 4.92 |
| BenchmarkSubmit_Parallel | 🐧 Linux | 1.68x |         105.66 |         62.86 | 45.34 |
| BenchmarkScheduleTimerWithPool_FireAndReuse | 🍎 Darwin | 1.66x |         276.84 |        459.26 | 16.03 |
| BenchmarkLatencychunkedIngressPush | 🍎 Darwin | 1.63x |           5.02 |          8.20 | 4.79 |
| BenchmarkPromiseGC | 🍎 Darwin | 1.55x |      59,481.40 |     92,369.80 | 10.31 |

## Top 10 Fastest Benchmarks

### Darwin

| Rank | Benchmark | ns/op | B/op | Allocs/op | CV% |
|------|-----------|-------|------|-----------|-----|
| 1 | BenchmarkLatencyDirectCall |       0.30 |    0 |         0 | 0.7% |
| 2 | BenchmarkLatencyStateLoad |       0.30 |    0 |         0 | 1.1% |
| 3 | BenchmarkLatencyDeferRecover |       2.40 |    0 |         0 | 0.9% |
| 4 | BenchmarkLatencySafeExecute |       3.10 |    0 |         0 | 7.5% |
| 5 | BenchmarkLatencychunkedIngressPop |       3.19 |    0 |         0 | 8.3% |
| 6 | Benchmark_chunkedIngress_Pop |       3.42 |    0 |         0 | 8.5% |
| 7 | BenchmarkLatencychunkedIngressPushPop |       3.95 |    0 |         0 | 0.5% |
| 8 | BenchmarkLatencyStateTryTransition |       4.03 |    0 |         0 | 3.1% |
| 9 | Benchmark_chunkedIngress_Sequential |       4.06 |    0 |         0 | 7.5% |
| 10 | Benchmark_chunkedIngress_PushPop |       4.08 |    0 |         0 | 0.8% |

### Linux

| Rank | Benchmark | ns/op | B/op | Allocs/op | CV% |
|------|-----------|-------|------|-----------|-----|
| 1 | BenchmarkLatencyDirectCall |       0.30 |    0 |         0 | 0.1% |
| 2 | BenchmarkLatencyStateLoad |       0.32 |    0 |         0 | 1.9% |
| 3 | BenchmarkLatencyDeferRecover |       2.38 |    0 |         0 | 0.2% |
| 4 | BenchmarkLatencySafeExecute |       3.03 |    0 |         0 | 3.8% |
| 5 | Benchmark_chunkedIngress_PushPop |       4.05 |    0 |         0 | 3.9% |
| 6 | BenchmarkLatencyStateTryTransition |       4.08 |    0 |         0 | 2.5% |
| 7 | BenchmarkLatencychunkedIngressPushPop |       4.08 |    0 |         0 | 2.9% |
| 8 | Benchmark_chunkedIngress_Sequential |       4.08 |    0 |         0 | 1.3% |
| 9 | BenchmarkLatencychunkedIngressPop |       4.16 |    0 |         0 | 2.9% |
| 10 | Benchmark_chunkedIngress_Pop |       4.21 |    0 |         0 | 4.6% |

## Allocation Comparison

Since both platforms run the same Go code, allocations (allocs/op) and bytes (B/op)
should be identical. Differences indicate platform-specific runtime behavior.

- **Allocs/op match:** 97/108 (89.8%)
- **B/op match:** 78/108 (72.2%)
- **Zero-allocation benchmarks (both platforms):** 46

### Zero-Allocation Benchmarks

These benchmarks achieve zero allocations on both platforms — the gold standard
for hot-path performance:

- `BenchmarkCombinedWorkload_New` — Darwin: 83.94 ns/op, Linux: 84.03 ns/op 🍎
- `BenchmarkCombinedWorkload_Old` — Darwin: 345.04 ns/op, Linux: 344.10 ns/op 🐧
- `BenchmarkFastPathSubmit` — Darwin: 38.58 ns/op, Linux: 33.50 ns/op 🐧
- `BenchmarkHighContention` — Darwin: 222.12 ns/op, Linux: 119.46 ns/op 🐧
- `BenchmarkHighFrequencyMonitoring_New` — Darwin: 24.60 ns/op, Linux: 24.61 ns/op 🍎
- `BenchmarkLatencyChannelBufferedRoundTrip` — Darwin: 330.44 ns/op, Linux: 241.68 ns/op 🐧
- `BenchmarkLatencyChannelRoundTrip` — Darwin: 348.64 ns/op, Linux: 242.80 ns/op 🐧
- `BenchmarkLatencyDeferRecover` — Darwin: 2.40 ns/op, Linux: 2.38 ns/op 🐧
- `BenchmarkLatencyDirectCall` — Darwin: 0.30 ns/op, Linux: 0.30 ns/op 🐧
- `BenchmarkLatencyMutexLockUnlock` — Darwin: 8.52 ns/op, Linux: 7.53 ns/op 🐧
- `BenchmarkLatencyRWMutexRLockRUnlock` — Darwin: 8.34 ns/op, Linux: 7.88 ns/op 🐧
- `BenchmarkLatencyRecord_WithPSquare` — Darwin: 74.89 ns/op, Linux: 74.76 ns/op 🐧
- `BenchmarkLatencyRecord_WithoutPSquare` — Darwin: 23.90 ns/op, Linux: 23.31 ns/op 🐧
- `BenchmarkLatencySafeExecute` — Darwin: 3.10 ns/op, Linux: 3.03 ns/op 🐧
- `BenchmarkLatencySample_NewPSquare` — Darwin: 26.27 ns/op, Linux: 25.23 ns/op 🐧
- `BenchmarkLatencySimulatedPoll` — Darwin: 12.49 ns/op, Linux: 14.15 ns/op 🍎
- `BenchmarkLatencySimulatedSubmit` — Darwin: 12.93 ns/op, Linux: 13.93 ns/op 🍎
- `BenchmarkLatencyStateLoad` — Darwin: 0.30 ns/op, Linux: 0.32 ns/op 🍎
- `BenchmarkLatencyStateTryTransition` — Darwin: 4.03 ns/op, Linux: 4.08 ns/op 🍎
- `BenchmarkLatencyStateTryTransition_NoOp` — Darwin: 17.08 ns/op, Linux: 16.36 ns/op 🐧
- `BenchmarkLatencychunkedIngressPop` — Darwin: 3.19 ns/op, Linux: 4.16 ns/op 🍎
- `BenchmarkLatencychunkedIngressPush` — Darwin: 5.02 ns/op, Linux: 8.20 ns/op 🍎
- `BenchmarkLatencychunkedIngressPushPop` — Darwin: 3.95 ns/op, Linux: 4.08 ns/op 🍎
- `BenchmarkLatencychunkedIngressPush_WithContention` — Darwin: 70.13 ns/op, Linux: 38.88 ns/op 🐧
- `BenchmarkLatencymicrotaskRingPop` — Darwin: 15.30 ns/op, Linux: 15.72 ns/op 🍎
- `BenchmarkLatencymicrotaskRingPush` — Darwin: 24.70 ns/op, Linux: 26.08 ns/op 🍎
- `BenchmarkLatencymicrotaskRingPushPop` — Darwin: 22.43 ns/op, Linux: 22.11 ns/op 🐧
- `BenchmarkMetricsCollection` — Darwin: 32.08 ns/op, Linux: 34.63 ns/op 🍎
- `BenchmarkMicrotaskOverflow` — Darwin: 23.97 ns/op, Linux: 24.52 ns/op 🍎
- `BenchmarkMicrotaskSchedule` — Darwin: 78.23 ns/op, Linux: 60.69 ns/op 🐧
- `BenchmarkMicrotaskSchedule_Parallel` — Darwin: 109.64 ns/op, Linux: 60.13 ns/op 🐧
- `BenchmarkNoMetrics` — Darwin: 38.96 ns/op, Linux: 38.71 ns/op 🐧
- `BenchmarkQueueMicrotask` — Darwin: 80.75 ns/op, Linux: 57.34 ns/op 🐧
- `BenchmarkSubmit` — Darwin: 40.23 ns/op, Linux: 33.05 ns/op 🐧
- `BenchmarkSubmit_Parallel` — Darwin: 105.66 ns/op, Linux: 62.86 ns/op 🐧
- `BenchmarkTask1_2_ConcurrentSubmissions` — Darwin: 105.88 ns/op, Linux: 69.19 ns/op 🐧
- `BenchmarkWakeUpDeduplicationIntegration` — Darwin: 102.82 ns/op, Linux: 71.02 ns/op 🐧
- `Benchmark_chunkedIngress_Batch` — Darwin: 507.00 ns/op, Linux: 517.62 ns/op 🍎
- `Benchmark_chunkedIngress_ParallelWithSync` — Darwin: 87.80 ns/op, Linux: 43.54 ns/op 🐧
- `Benchmark_chunkedIngress_Pop` — Darwin: 3.42 ns/op, Linux: 4.21 ns/op 🍎
- `Benchmark_chunkedIngress_Push` — Darwin: 5.06 ns/op, Linux: 6.77 ns/op 🍎
- `Benchmark_chunkedIngress_PushPop` — Darwin: 4.08 ns/op, Linux: 4.05 ns/op 🐧
- `Benchmark_chunkedIngress_Sequential` — Darwin: 4.06 ns/op, Linux: 4.08 ns/op 🍎
- `Benchmark_microtaskRing_Parallel` — Darwin: 130.08 ns/op, Linux: 89.97 ns/op 🐧
- `Benchmark_microtaskRing_Push` — Darwin: 24.37 ns/op, Linux: 27.43 ns/op 🍎
- `Benchmark_microtaskRing_PushPop` — Darwin: 21.95 ns/op, Linux: 21.69 ns/op 🐧

### Allocation Mismatches

| Benchmark | Darwin allocs | Linux allocs | Δ |
|-----------|---------------|--------------|---|
| BenchmarkCancelTimers_Batch/timers_1 | 21 | 26 | 5 |
| BenchmarkCancelTimers_Batch/timers_5 | 62 | 103 | 41 |
| BenchmarkCancelTimers_Batch/timers_: | 112 | 191 | 79 |
| BenchmarkCancelTimers_Comparison/Batch | 61 | 101 | 40 |
| BenchmarkPromiseAll | 28 | 28 | 0 |
| BenchmarkScheduleTimerCancel | 6 | 7 | 1 |
| BenchmarkSetTimeoutZeroDelay | 6 | 7 | 1 |
| BenchmarkSetTimeout_Optimized | 6 | 7 | 1 |
| BenchmarkSubmitInternal | 0 | 1 | 1 |
| BenchmarkTimerSchedule | 6 | 7 | 1 |
| BenchmarkTimerSchedule_Parallel | 5 | 6 | 1 |

### B/op Mismatches

| Benchmark | Darwin B/op | Linux B/op | Δ |
|-----------|-------------|------------|---|
| BenchmarkCancelTimer_Individual/timers_1 | 2,640 | 2,641 | 1 |
| BenchmarkCancelTimer_Individual/timers_5 | 13,221 | 13,230 | 9 |
| BenchmarkCancelTimer_Individual/timers_: | 26,530 | 26,551 | 20 |
| BenchmarkCancelTimers_Batch/timers_1 | 1,201 | 1,525 | 324 |
| BenchmarkCancelTimers_Batch/timers_5 | 3,915 | 6,535 | 2,619 |
| BenchmarkCancelTimers_Batch/timers_: | 7,377 | 12,426 | 5,049 |
| BenchmarkCancelTimers_Comparison/Batch | 3,487 | 6,106 | 2,619 |
| BenchmarkCancelTimers_Comparison/Individual | 12,805 | 12,814 | 9 |
| BenchmarkHighContention | 0 | 45 | 44 |
| BenchmarkLatencymicrotaskRingPush | 46 | 45 | 1 |
| BenchmarkMetricsCollection | 44 | 42 | 2 |
| BenchmarkMicrotaskExecution | 16 | 61 | 45 |
| BenchmarkMicrotaskSchedule | 1 | 44 | 43 |
| BenchmarkMicrotaskSchedule_Parallel | 0 | 43 | 43 |
| BenchmarkMixedWorkload | 46 | 52 | 6 |
| BenchmarkNoMetrics | 0 | 0 | 0 |
| BenchmarkPromiseAll | 1,240 | 1,241 | 1 |
| BenchmarkPromiseAll_Memory | 1,240 | 1,240 | 0 |
| BenchmarkPromiseChain | 488 | 489 | 0 |
| BenchmarkPromiseThenChain | 519 | 519 | 0 |
| BenchmarkPromisifyAllocation | 793 | 796 | 3 |
| BenchmarkQueueMicrotask | 0 | 44 | 43 |
| BenchmarkScheduleTimerWithPool | 50 | 56 | 6 |
| BenchmarkScheduleTimerWithPool_FireAndReuse | 34 | 33 | 1 |
| BenchmarkScheduleTimerWithPool_Immediate | 36 | 44 | 8 |
| BenchmarkSetInterval_Parallel_Optimized | 449 | 461 | 12 |
| BenchmarkSubmitInternal | 63 | 64 | 1 |
| BenchmarkTimerFire | 51 | 70 | 19 |
| BenchmarkTimerSchedule_Parallel | 296 | 332 | 36 |
| Benchmark_microtaskRing_Push | 46 | 43 | 3 |

## Measurement Stability

Coefficient of variation (CV%) indicates measurement consistency. Lower is better.

- Benchmarks with CV < 2% on both platforms: **24**
- Darwin benchmarks with CV > 5%: **29**
- Linux benchmarks with CV > 5%: **28**

### High-Variance Benchmarks (CV > 5%)

| Benchmark | Darwin CV% | Linux CV% |
|-----------|------------|-----------|
| BenchmarkCancelTimer_Individual/timers_1 | 9.4% ⚠️ | 1.1% |
| BenchmarkCancelTimer_Individual/timers_: | 2.4% | 7.7% ⚠️ |
| BenchmarkGojaStyleSwap | 7.9% ⚠️ | 1.0% |
| BenchmarkLargeTimerHeap | 5.7% ⚠️ | 2.2% |
| BenchmarkLatencyAnalysis_EndToEnd | 8.7% ⚠️ | 1.6% |
| BenchmarkLatencyMutexLockUnlock | 9.8% ⚠️ | 0.1% |
| BenchmarkLatencyRWMutexRLockRUnlock | 7.1% ⚠️ | 1.7% |
| BenchmarkLatencySafeExecute | 7.5% ⚠️ | 3.8% |
| BenchmarkLatencySample_NewPSquare | 6.2% ⚠️ | 4.5% |
| BenchmarkLatencySimulatedPoll | 0.1% | 5.5% ⚠️ |
| BenchmarkLatencySimulatedSubmit | 8.9% ⚠️ | 1.9% |
| BenchmarkLatencychunkedIngressPop | 8.3% ⚠️ | 2.9% |
| BenchmarkLatencychunkedIngressPush | 3.4% | 18.0% ⚠️ |
| BenchmarkLoopDirectWithSubmit | 7.3% ⚠️ | 0.5% |
| BenchmarkMetricsCollection | 11.0% ⚠️ | 8.5% ⚠️ |
| BenchmarkMicroPingPong | 2.8% | 13.6% ⚠️ |
| BenchmarkMicrotaskExecution | 6.9% ⚠️ | 2.1% |
| BenchmarkMicrotaskSchedule | 5.7% ⚠️ | 8.5% ⚠️ |
| BenchmarkMixedWorkload | 1.0% | 25.5% ⚠️ |
| BenchmarkNoMetrics | 5.6% ⚠️ | 19.1% ⚠️ |
| BenchmarkPromiseChain | 5.5% ⚠️ | 3.0% |
| BenchmarkPromiseCreate | 2.0% | 12.8% ⚠️ |
| BenchmarkPromiseGC | 0.6% | 7.7% ⚠️ |
| BenchmarkPromiseHandlerTracking_Optimized | 5.1% ⚠️ | 8.2% ⚠️ |
| BenchmarkPromiseRejection | 9.1% ⚠️ | 1.7% |
| BenchmarkPromiseResolution | 5.6% ⚠️ | 2.5% |
| BenchmarkPromiseResolve | 1.9% | 14.0% ⚠️ |
| BenchmarkPromiseResolve_Memory | 5.8% ⚠️ | 10.8% ⚠️ |
| BenchmarkPromiseThen | 0.7% | 5.4% ⚠️ |
| BenchmarkPromiseTry | 0.9% | 6.6% ⚠️ |
| BenchmarkQueueMicrotask | 5.7% ⚠️ | 4.1% |
| BenchmarkScheduleTimerCancel | 3.2% | 19.2% ⚠️ |
| BenchmarkScheduleTimerWithPool | 5.0% ⚠️ | 5.4% ⚠️ |
| BenchmarkScheduleTimerWithPool_FireAndReuse | 3.6% | 5.1% ⚠️ |
| BenchmarkSetImmediate_Optimized | 4.7% | 10.6% ⚠️ |
| BenchmarkSetTimeoutZeroDelay | 5.3% ⚠️ | 11.6% ⚠️ |
| BenchmarkSetTimeout_Optimized | 6.0% ⚠️ | 8.8% ⚠️ |
| BenchmarkSubmitExecution | 2.8% | 10.7% ⚠️ |
| BenchmarkSubmitInternal | 1.9% | 5.7% ⚠️ |
| BenchmarkTimerFire | 6.2% ⚠️ | 13.3% ⚠️ |
| BenchmarkTimerHeapOperations | 3.0% | 6.6% ⚠️ |
| BenchmarkTimerLatency | 0.6% | 13.7% ⚠️ |
| Benchmark_chunkedIngress_Pop | 8.5% ⚠️ | 4.6% |
| Benchmark_chunkedIngress_Push | 5.0% ⚠️ | 13.2% ⚠️ |
| Benchmark_chunkedIngress_Sequential | 7.5% ⚠️ | 1.3% |
| Benchmark_microtaskRing_Push | 10.5% ⚠️ | 11.5% ⚠️ |

## Key Findings

### 1. Architecture Parity

Both platforms run ARM64, eliminating architectural differences. Performance gaps
are attributable to:
- **OS kernel scheduling** (macOS Mach scheduler vs Linux CFS)
- **Memory management** (macOS memory pressure vs Linux cgroups in container)
- **Syscall overhead** differences
- **Go runtime behavior** variations between `darwin/arm64` and `linux/arm64`

### 2. Performance Distribution

- Darwin significantly faster (ratio < 0.9): **37** benchmarks
- Roughly equal (0.9–1.1x): **44** benchmarks
- Linux significantly faster (ratio > 1.1): **27** benchmarks

### 3. Timer Operations

- Total timer benchmarks: 18
- Darwin faster: 17
- Linux faster: 1
- Biggest difference: `BenchmarkCancelTimer_Individual/timers_:` — Linux is 3.02x slower

### 4. Concurrency & Contention

- `BenchmarkFastPathSubmit`: 🐧 Linux (1.15x)
- `BenchmarkHighContention`: 🐧 Linux (1.86x)
- `BenchmarkLatencyAnalysis_SubmitWhileRunning`: 🐧 Linux (1.34x)
- `BenchmarkLatencySimulatedSubmit`: 🍎 Darwin (1.08x)
- `BenchmarkLatencychunkedIngressPush_WithContention`: 🐧 Linux (1.80x)
- `BenchmarkLoopDirectWithSubmit`: 🍎 Darwin (2.95x)
- `BenchmarkMicrotaskSchedule_Parallel`: 🐧 Linux (1.82x)
- `BenchmarkPromiseHandlerTracking_Parallel_Optimized`: 🐧 Linux (1.88x)
- `BenchmarkSetInterval_Parallel_Optimized`: 🍎 Darwin (2.82x)
- `BenchmarkSubmit`: 🐧 Linux (1.22x)
- `BenchmarkSubmitExecution`: 🐧 Linux (2.23x)
- `BenchmarkSubmitInternal`: 🐧 Linux (1.17x)
- `BenchmarkSubmitLatency`: 🐧 Linux (1.36x)
- `BenchmarkSubmit_Parallel`: 🐧 Linux (1.68x)
- `BenchmarkTask1_2_ConcurrentSubmissions`: 🐧 Linux (1.53x)
- `BenchmarkTimerSchedule_Parallel`: 🍎 Darwin (3.00x)
- `Benchmark_chunkedIngress_ParallelWithSync`: 🐧 Linux (2.02x)
- `Benchmark_microtaskRing_Parallel`: 🐧 Linux (1.45x)

### 5. Summary

**Darwin wins overall** with 55/108 benchmarks faster.

The mean performance ratio of 0.980x (Darwin/Linux) indicates
the platforms are remarkably close in overall performance, with each
excelling in different workload categories.

