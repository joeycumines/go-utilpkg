# Darwin vs Linux Benchmark Comparison

**Date:** 2026-04-24
**Platforms:** Darwin ARM64 (macOS, GOMAXPROCS=10) vs Linux ARM64 (container, GOMAXPROCS=10)
**Methodology:** `go test -bench=. -benchmem -count=5 -run=^$ -benchtime=1s -timeout=10m`
**Benchmarks Compared:** 158 common benchmarks


---

## Cross-Tournament Comparison

### Darwin Comparison


---

## Cross-Tournament Comparison: 2026-04-24 vs 2026-02-10

### Significant Changes (p < 0.05)

**19 improvements, 52 regressions**

| Benchmark | Previous (ns/op) | Current (ns/op) | Change | %Δ |
|-----------|------------------|-----------------|--------|----|
| BenchmarkLatencySample_OldSortBased |      17,562.80 |       6,250.40 |  -11,312.40 | -64.4% |
| BenchmarkHighFrequencyMonitoring_Old |      23,700.20 |       8,676.00 |  -15,024.20 | -63.4% |
| BenchmarkCombinedWorkload_Old |         345.04 |         210.84 |     -134.20 | -38.9% |
| BenchmarkSubmitExecution |         103.55 |          67.67 |      -35.88 | -34.6% |
| BenchmarkFastPathExecution |         104.52 |          70.57 |      -33.95 | -32.5% |
| BenchmarkLatencyChannelBufferedRoundTrip |         330.44 |         266.92 |      -63.52 | -19.2% |
| BenchmarkGojaStyleSwap |         472.76 |         423.32 |      -49.44 | -10.5% |
| BenchmarkPromiseAll_Memory |       1,490.80 |       1,362.20 |     -128.60 | -8.6% |
| BenchmarkPromiseReject |         545.74 |         501.10 |      -44.64 | -8.2% |
| BenchmarkTask1_2_ConcurrentSubmissions |         105.88 |          97.98 |       -7.90 | -7.5% |
| BenchmarkMinimalLoop |         460.50 |         426.92 |      -33.58 | -7.3% |
| Benchmark_microtaskRing_Parallel |         130.08 |         121.12 |       -8.96 | -6.9% |
| BenchmarkPromiseRace |       1,289.40 |       1,217.40 |      -72.00 | -5.6% |
| BenchmarkLatencyAnalysis_PingPong |         594.86 |         565.26 |      -29.60 | -5.0% |
| BenchmarkChannelWithMutexQueue |         466.06 |         444.48 |      -21.58 | -4.6% |
| BenchmarkMicroPingPongWithCount |         442.34 |         423.24 |      -19.10 | -4.3% |
| BenchmarkSubmit_Parallel |         105.66 |         101.50 |       -4.16 | -3.9% |
| BenchmarkLoopDirect |         471.70 |         461.22 |      -10.48 | -2.2% |
| Benchmark_chunkedIngress_PushPop |           4.08 |           4.00 |       -0.08 | -1.9% |
| BenchmarkScheduleTimerWithPool_Immediate |         222.14 |       4,341.20 |   +4,119.06 | +1854.3% |
| BenchmarkTimerFire |         257.34 |       4,513.60 |   +4,256.26 | +1653.9% |
| BenchmarkScheduleTimerWithPool_FireAndReuse |         276.84 |       4,333.80 |   +4,056.96 | +1465.5% |
| BenchmarkScheduleTimerWithPool |         481.08 |       4,380.40 |   +3,899.32 | +810.5% |
| BenchmarkCancelTimers_Batch/timers_: |      57,085.80 |     478,140.40 | +421,054.60 | +737.6% |
| BenchmarkMixedWorkload |         132.14 |       1,016.00 |     +883.86 | +668.9% |
| BenchmarkCancelTimers_Comparison/Batch |      48,169.80 |     248,791.60 | +200,621.80 | +416.5% |
| BenchmarkCancelTimers_Batch/timers_5 |      48,594.20 |     249,934.00 | +201,339.80 | +414.3% |
| BenchmarkTimerSchedule_Parallel |       5,096.00 |      13,643.80 |   +8,547.80 | +167.7% |
| BenchmarkSetInterval_Parallel_Optimized |       6,114.60 |      16,210.20 |  +10,095.60 | +165.1% |
| BenchmarkCancelTimer_Individual/timers_: |   1,181,246.20 |   2,541,663.60 | +1,360,417.40 | +115.2% |
| BenchmarkCancelTimer_Individual/timers_5 |     593,275.00 |   1,275,863.60 | +682,588.60 | +115.1% |
| BenchmarkCancelTimers_Comparison/Individual |     602,813.80 |   1,292,803.40 | +689,989.60 | +114.5% |
| BenchmarkCancelTimer_Individual/timers_1 |     124,843.60 |     251,132.80 | +126,289.20 | +101.2% |
| BenchmarkLargeTimerHeap |      12,812.20 |      23,146.60 |  +10,334.40 | +80.7% |
| BenchmarkCancelTimers_Batch/timers_1 |      38,474.00 |      65,037.20 |  +26,563.20 | +69.0% |
| BenchmarkTimerLatency |      11,725.80 |      19,582.60 |   +7,856.80 | +67.0% |
| Benchmark_chunkedIngress_Push |           5.06 |           8.38 |       +3.32 | +65.5% |
| BenchmarkLatencychunkedIngressPush |           5.02 |           8.21 |       +3.18 | +63.4% |
| BenchmarkNoMetrics |          38.96 |          59.06 |      +20.10 | +51.6% |

### Notable Improvements

- `BenchmarkLatencySample_OldSortBased`: 17,562.80 -> 6,250.40 ns/op (*2.81x faster*, -64.4%)
- `BenchmarkHighFrequencyMonitoring_Old`: 23,700.20 -> 8,676.00 ns/op (*2.73x faster*, -63.4%)
- `BenchmarkCombinedWorkload_Old`: 345.04 -> 210.84 ns/op (*1.64x faster*, -38.9%)
- `BenchmarkSubmitExecution`: 103.55 -> 67.67 ns/op (*1.53x faster*, -34.6%)
- `BenchmarkFastPathExecution`: 104.52 -> 70.57 ns/op (*1.48x faster*, -32.5%)
- `BenchmarkLatencyChannelBufferedRoundTrip`: 330.44 -> 266.92 ns/op (*1.24x faster*, -19.2%)
- `BenchmarkGojaStyleSwap`: 472.76 -> 423.32 ns/op (*1.12x faster*, -10.5%)
- `BenchmarkPromiseAll_Memory`: 1,490.80 -> 1,362.20 ns/op (*1.09x faster*, -8.6%)
- `BenchmarkPromiseReject`: 545.74 -> 501.10 ns/op (*1.09x faster*, -8.2%)
- `BenchmarkTask1_2_ConcurrentSubmissions`: 105.88 -> 97.98 ns/op (*1.08x faster*, -7.5%)

### Notable Regressions

- `BenchmarkScheduleTimerWithPool_Immediate`: 222.14 -> 4,341.20 ns/op (*19.54x slower*, +1854.3%)
- `BenchmarkTimerFire`: 257.34 -> 4,513.60 ns/op (*17.54x slower*, +1653.9%)
- `BenchmarkScheduleTimerWithPool_FireAndReuse`: 276.84 -> 4,333.80 ns/op (*15.65x slower*, +1465.5%)
- `BenchmarkScheduleTimerWithPool`: 481.08 -> 4,380.40 ns/op (*9.11x slower*, +810.5%)
- `BenchmarkCancelTimers_Batch/timers_:`: 57,085.80 -> 478,140.40 ns/op (*8.38x slower*, +737.6%)
- `BenchmarkMixedWorkload`: 132.14 -> 1,016.00 ns/op (*7.69x slower*, +668.9%)
- `BenchmarkCancelTimers_Comparison/Batch`: 48,169.80 -> 248,791.60 ns/op (*5.16x slower*, +416.5%)
- `BenchmarkCancelTimers_Batch/timers_5`: 48,594.20 -> 249,934.00 ns/op (*5.14x slower*, +414.3%)
- `BenchmarkTimerSchedule_Parallel`: 5,096.00 -> 13,643.80 ns/op (*2.68x slower*, +167.7%)
- `BenchmarkSetInterval_Parallel_Optimized`: 6,114.60 -> 16,210.20 ns/op (*2.65x slower*, +165.1%)

### New Benchmarks (not in past run)

- `BenchmarkAlive_AllAtomic`: 21.27 ns/op
- `BenchmarkAlive_AllAtomic_Contended`: 18.68 ns/op
- `BenchmarkAlive_Epoch_ConcurrentSubmit`: 245.22 ns/op
- `BenchmarkAlive_Epoch_FromCallback`: 7,644.00 ns/op
- `BenchmarkAlive_Epoch_NoContention`: 35.47 ns/op
- `BenchmarkAlive_Uncontended`: 35.44 ns/op
- `BenchmarkAlive_WithMutexes`: 35.06 ns/op
- `BenchmarkAlive_WithMutexes_Contended`: 37.50 ns/op
- `BenchmarkAlive_WithMutexes_HighContention`: 181.54 ns/op
- `BenchmarkAlive_WithTimer`: 2.04 ns/op


### Linux Comparison


---

## Cross-Tournament Comparison: 2026-04-24 vs 2026-02-10

### Significant Changes (p < 0.05)

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
| BenchmarkFastPathExecution |          41.95 |          55.72 |      +13.77 | +32.8% |
| BenchmarkTimerLatency |      40,101.80 |      52,290.80 |  +12,189.00 | +30.4% |
| BenchmarkCancelTimers_Batch/timers_: |     344,207.60 |     445,835.60 | +101,628.00 | +29.5% |
| BenchmarkCancelTimer_Individual/timers_1 |     342,611.20 |     441,482.00 |  +98,870.80 | +28.9% |
| BenchmarkSubmit |          33.05 |          42.57 |       +9.53 | +28.8% |

### Notable Improvements

- `BenchmarkHighFrequencyMonitoring_Old`: 23,668.20 -> 9,378.60 ns/op (*2.52x faster*, -60.4%)
- `BenchmarkLatencySample_OldSortBased`: 16,505.40 -> 7,808.00 ns/op (*2.11x faster*, -52.7%)
- `BenchmarkCombinedWorkload_Old`: 344.10 -> 215.20 ns/op (*1.60x faster*, -37.5%)
- `BenchmarkPromiseHandlerTracking_Optimized`: 108.74 -> 93.96 ns/op (*1.16x faster*, -13.6%)
- `BenchmarkPromiseHandlerTracking_Parallel_Optimized`: 177.92 -> 155.52 ns/op (*1.14x faster*, -12.6%)
- `BenchmarkTask1_2_ConcurrentSubmissions`: 69.19 -> 62.88 ns/op (*1.10x faster*, -9.1%)
- `BenchmarkPromiseAll`: 1,758.20 -> 1,664.00 ns/op (*1.06x faster*, -5.4%)
- `BenchmarkLatencyStateLoad`: 0.32 -> 0.31 ns/op (*1.05x faster*, -4.9%)

### Notable Regressions

- `BenchmarkScheduleTimerWithPool_Immediate`: 322.86 -> 3,641.00 ns/op (*11.28x slower*, +1027.7%)
- `BenchmarkTimerFire`: 350.32 -> 3,680.20 ns/op (*10.51x slower*, +950.5%)
- `BenchmarkScheduleTimerWithPool_FireAndReuse`: 459.26 -> 3,977.00 ns/op (*8.66x slower*, +766.0%)
- `BenchmarkScheduleTimerWithPool`: 463.26 -> 3,740.80 ns/op (*8.07x slower*, +707.5%)
- `BenchmarkMixedWorkload`: 244.74 -> 856.40 ns/op (*3.50x slower*, +249.9%)
- `BenchmarkLatencymicrotaskRingPush`: 26.08 -> 51.18 ns/op (*1.96x slower*, +96.3%)
- `Benchmark_microtaskRing_Push`: 27.43 -> 48.89 ns/op (*1.78x slower*, +78.3%)
- `BenchmarkPromiseCreation`: 64.54 -> 101.82 ns/op (*1.58x slower*, +57.8%)
- `BenchmarkCancelTimers_Comparison/Individual`: 1,732,770.80 -> 2,590,545.80 ns/op (*1.50x slower*, +49.5%)
- `BenchmarkCancelTimer_Individual/timers_:`: 3,570,868.60 -> 5,238,622.80 ns/op (*1.47x slower*, +46.7%)

### New Benchmarks (not in past run)

- `BenchmarkAlive_AllAtomic`: 18.43 ns/op
- `BenchmarkAlive_AllAtomic_Contended`: 18.33 ns/op
- `BenchmarkAlive_Epoch_ConcurrentSubmit`: 804.40 ns/op
- `BenchmarkAlive_Epoch_FromCallback`: 3,771.60 ns/op
- `BenchmarkAlive_Epoch_NoContention`: 37.05 ns/op
- `BenchmarkAlive_Uncontended`: 36.70 ns/op
- `BenchmarkAlive_WithMutexes`: 34.82 ns/op
- `BenchmarkAlive_WithMutexes_Contended`: 34.78 ns/op
- `BenchmarkAlive_WithMutexes_HighContention`: 726.56 ns/op
- `BenchmarkAlive_WithTimer`: 2.08 ns/op

### Removed Benchmarks (in past run but not current)

- `BenchmarkWakeUpDeduplicationIntegration`: was 71.02 ns/op

## Executive Summary

This report compares eventloop benchmark performance between **Darwin (macOS)** and **Linux**,
both running on **ARM64** architecture. Since the architecture is identical, performance
differences reflect OS-level differences: kernel scheduling, memory management, syscall
overhead, and Go runtime behavior on each OS.

### Key Metrics

| Metric | Value |
|--------|-------|
| Common benchmarks | 158 |
| Darwin-only benchmarks | 1 |
| Linux-only benchmarks | 0 |
| Darwin wins (faster) | **98** (62.0%) |
| Linux wins (faster) | **60** (38.0%) |
| Ties | 0 |
| Statistically significant differences | 107 |
| Darwin mean (common benchmarks) | 56,025.95 ns/op |
| Linux mean (common benchmarks) | 99,749.81 ns/op |
| Mean ratio (Darwin/Linux) | 0.970x |
| Median ratio (Darwin/Linux) | 0.989x |
| Allocation match rate | 151/158 (95.6%) |
| Zero-allocation benchmarks (both) | 76 |

## Full Statistical Comparison Table

| # | Benchmark | Darwin (ns/op) | Darwin CV% | Linux (ns/op) | Linux CV% | Faster | Ratio | Sig? |
|---|-----------|----------------|------------|---------------|-----------|--------|-------|------|
| 1 | BenchmarkAlive_AllAtomic |          21.27 |       9.5% |         18.43 |      0.9% | Linux |  1.15x | yes |
| 2 | BenchmarkAlive_AllAtomic_Contended |          18.68 |       0.5% |         18.33 |      0.4% | Linux |  1.02x | yes |
| 3 | BenchmarkAlive_Epoch_ConcurrentSubmit |         245.22 |       4.7% |        804.40 |     20.6% | Darwin |  0.30x | yes |
| 4 | BenchmarkAlive_Epoch_FromCallback |       7,644.00 |      57.7% |      3,771.60 |      0.4% | Linux |  2.03x |  |
| 5 | BenchmarkAlive_Epoch_NoContention |          35.47 |       0.7% |         37.05 |      3.3% | Darwin |  0.96x | yes |
| 6 | BenchmarkAlive_Uncontended |          35.44 |       0.5% |         36.70 |      0.3% | Darwin |  0.97x | yes |
| 7 | BenchmarkAlive_WithMutexes |          35.06 |       0.5% |         34.82 |      0.4% | Linux |  1.01x |  |
| 8 | BenchmarkAlive_WithMutexes_Contended |          37.50 |      11.9% |         34.78 |      0.1% | Linux |  1.08x |  |
| 9 | BenchmarkAlive_WithMutexes_HighContention |         181.54 |       5.6% |        726.56 |     31.1% | Darwin |  0.25x | yes |
| 10 | BenchmarkAlive_WithTimer |           2.04 |       0.2% |          2.08 |      0.7% | Darwin |  0.98x | yes |
| 11 | BenchmarkAutoExit_AliveCheckCost_Disabled |          54.29 |       1.0% |         41.71 |      2.2% | Linux |  1.30x | yes |
| 12 | BenchmarkAutoExit_AliveCheckCost_Enabled |          49.86 |       3.8% |         44.49 |      1.6% | Linux |  1.12x | yes |
| 13 | BenchmarkAutoExit_FastPathExit |     211,352.00 |       0.5% |    388,627.40 |    102.3% | Darwin |  0.54x |  |
| 14 | BenchmarkAutoExit_ImmediateExit |     203,965.60 |       7.0% |    401,694.20 |     97.2% | Darwin |  0.51x |  |
| 15 | BenchmarkAutoExit_PollPathExit |     135,692.60 |      33.5% |     70,527.80 |      7.2% | Linux |  1.92x | yes |
| 16 | BenchmarkAutoExit_TimerFire |     114,878.80 |       8.8% |     72,323.00 |      3.9% | Linux |  1.59x | yes |
| 17 | BenchmarkAutoExit_UnrefExit |   1,259,395.00 |       0.1% |  2,213,390.80 |      1.0% | Darwin |  0.57x | yes |
| 18 | BenchmarkCancelTimer_Individual/timers_1 |     251,132.80 |       2.4% |    441,482.00 |      0.8% | Darwin |  0.57x | yes |
| 19 | BenchmarkCancelTimer_Individual/timers_5 |   1,275,863.60 |       1.5% |  2,501,492.60 |      0.6% | Darwin |  0.51x | yes |
| 20 | BenchmarkCancelTimer_Individual/timers_: |   2,541,663.60 |       0.6% |  5,238,622.80 |      1.0% | Darwin |  0.49x | yes |
| 21 | BenchmarkCancelTimers_Batch/timers_1 |      65,037.20 |       0.3% |     76,685.60 |      0.7% | Darwin |  0.85x | yes |
| 22 | BenchmarkCancelTimers_Batch/timers_5 |     249,934.00 |       0.5% |    227,781.20 |      0.5% | Linux |  1.10x | yes |
| 23 | BenchmarkCancelTimers_Batch/timers_: |     478,140.40 |       0.4% |    445,835.60 |     14.7% | Linux |  1.07x |  |
| 24 | BenchmarkCancelTimers_Comparison/Batch |     248,791.60 |       0.5% |    227,991.00 |      1.0% | Linux |  1.09x | yes |
| 25 | BenchmarkCancelTimers_Comparison/Individual |   1,292,803.40 |       2.0% |  2,590,545.80 |      4.2% | Darwin |  0.50x | yes |
| 26 | BenchmarkChannelWithMutexQueue |         444.48 |       0.4% |        457.16 |      0.6% | Darwin |  0.97x | yes |
| 27 | BenchmarkCombinedWorkload_New |          86.95 |       0.4% |         88.36 |      0.2% | Darwin |  0.98x | yes |
| 28 | BenchmarkCombinedWorkload_Old |         210.84 |       0.5% |        215.20 |      0.3% | Darwin |  0.98x | yes |
| 29 | BenchmarkFastPathExecution |          70.57 |       2.8% |         55.72 |      1.7% | Linux |  1.27x | yes |
| 30 | BenchmarkFastPathSubmit |          53.61 |       0.8% |         42.65 |      1.4% | Linux |  1.26x | yes |
| 31 | BenchmarkGetGoroutineID |       1,760.60 |       1.4% |      1,740.60 |      0.7% | Linux |  1.01x |  |
| 32 | BenchmarkGetGoroutineIDOld |       1,763.60 |       0.4% |      1,771.40 |      0.3% | Darwin |  1.00x |  |
| 33 | BenchmarkGojaStyleSwap |         423.32 |       0.8% |        441.36 |      1.2% | Darwin |  0.96x | yes |
| 34 | BenchmarkHighContention |         223.98 |       1.5% |        150.76 |      3.9% | Linux |  1.49x | yes |
| 35 | BenchmarkHighFrequencyMonitoring_New |          24.80 |       0.3% |         24.98 |      0.3% | Darwin |  0.99x | yes |
| 36 | BenchmarkHighFrequencyMonitoring_Old |       8,676.00 |       0.4% |      9,378.60 |      0.5% | Darwin |  0.93x | yes |
| 37 | BenchmarkIsLoopThread_False |           0.35 |       2.3% |          0.35 |      1.3% | Linux |  1.01x |  |
| 38 | BenchmarkIsLoopThread_True |       4,636.00 |       1.5% |      4,493.40 |      0.3% | Linux |  1.03x | yes |
| 39 | BenchmarkLargeTimerHeap |      23,146.60 |       6.0% |     42,925.20 |      1.5% | Darwin |  0.54x | yes |
| 40 | BenchmarkLatencyAnalysis_EndToEnd |         551.54 |       0.6% |        570.62 |      1.0% | Darwin |  0.97x | yes |
| 41 | BenchmarkLatencyAnalysis_PingPong |         565.26 |       0.7% |        434.32 |      0.6% | Linux |  1.30x | yes |
| 42 | BenchmarkLatencyAnalysis_SubmitWhileRunning |         432.34 |       0.5% |        331.24 |      0.4% | Linux |  1.31x | yes |
| 43 | BenchmarkLatencyChannelBufferedRoundTrip |         266.92 |       5.2% |        247.32 |      0.9% | Linux |  1.08x | yes |
| 44 | BenchmarkLatencyChannelRoundTrip |         373.50 |       1.7% |        237.84 |      2.3% | Linux |  1.57x | yes |
| 45 | BenchmarkLatencyDeferRecover |           2.45 |       0.5% |          2.46 |      0.1% | Darwin |  1.00x |  |
| 46 | BenchmarkLatencyDirectCall |           0.31 |       0.5% |          0.31 |      0.8% | Darwin |  0.99x |  |
| 47 | BenchmarkLatencyMutexLockUnlock |           7.91 |       0.8% |          8.00 |      0.8% | Darwin |  0.99x |  |
| 48 | BenchmarkLatencyRWMutexRLockRUnlock |           8.16 |       0.8% |          8.15 |      0.3% | Linux |  1.00x |  |
| 49 | BenchmarkLatencyRecord_WithPSquare |          77.36 |       0.4% |         78.31 |      0.5% | Darwin |  0.99x | yes |
| 50 | BenchmarkLatencyRecord_WithoutPSquare |          24.36 |       2.1% |         24.20 |      0.2% | Linux |  1.01x |  |
| 51 | BenchmarkLatencySafeExecute |           3.07 |       0.1% |          3.08 |      0.3% | Darwin |  1.00x |  |
| 52 | BenchmarkLatencySample_NewPSquare |          24.82 |       0.3% |         24.99 |      0.5% | Darwin |  0.99x |  |
| 53 | BenchmarkLatencySample_OldSortBased |       6,250.40 |       0.3% |      7,808.00 |      0.8% | Darwin |  0.80x | yes |
| 54 | BenchmarkLatencySimulatedPoll |          13.63 |       5.2% |         13.91 |      4.2% | Darwin |  0.98x |  |
| 55 | BenchmarkLatencySimulatedSubmit |          14.03 |       2.5% |         16.45 |     12.4% | Darwin |  0.85x |  |
| 56 | BenchmarkLatencyStateLoad |           0.31 |       0.6% |          0.31 |      1.1% | Darwin |  0.99x |  |
| 57 | BenchmarkLatencyStateTryTransition |           4.09 |       0.3% |          4.12 |      0.8% | Darwin |  0.99x |  |
| 58 | BenchmarkLatencyStateTryTransition_NoOp |          17.59 |       0.7% |         17.61 |      0.6% | Darwin |  1.00x |  |
| 59 | BenchmarkLatencychunkedIngressPop |           4.50 |      44.9% |          5.66 |     37.0% | Darwin |  0.80x |  |
| 60 | BenchmarkLatencychunkedIngressPush |           8.21 |       4.9% |          9.88 |      9.6% | Darwin |  0.83x | yes |
| 61 | BenchmarkLatencychunkedIngressPushPop |           4.06 |       0.8% |          4.06 |      0.5% | Darwin |  1.00x |  |
| 62 | BenchmarkLatencychunkedIngressPush_WithContention |          76.86 |       0.7% |         38.77 |      2.7% | Linux |  1.98x | yes |
| 63 | BenchmarkLatencymicrotaskRingPop |          15.58 |       1.1% |         16.37 |      0.6% | Darwin |  0.95x | yes |
| 64 | BenchmarkLatencymicrotaskRingPush |          29.75 |       2.0% |         51.18 |     15.7% | Darwin |  0.58x | yes |
| 65 | BenchmarkLatencymicrotaskRingPushPop |          22.61 |       1.2% |         22.55 |      0.2% | Linux |  1.00x |  |
| 66 | BenchmarkLoopDirect |         461.22 |       0.7% |        506.88 |      1.0% | Darwin |  0.91x | yes |
| 67 | BenchmarkLoopDirectWithSubmit |      15,131.20 |       0.9% |     41,866.80 |      5.2% | Darwin |  0.36x | yes |
| 68 | BenchmarkMetricsCollection |          38.16 |       9.8% |         43.07 |      3.8% | Darwin |  0.89x |  |
| 69 | BenchmarkMicroPingPong |         422.38 |       0.4% |        472.40 |      3.7% | Darwin |  0.89x | yes |
| 70 | BenchmarkMicroPingPongWithCount |         423.24 |       0.8% |        478.78 |      4.1% | Darwin |  0.88x | yes |
| 71 | BenchmarkMicrotaskExecution |          90.54 |      18.1% |        129.16 |      7.6% | Darwin |  0.70x | yes |
| 72 | BenchmarkMicrotaskLatency |         456.12 |       1.1% |        339.84 |      2.0% | Linux |  1.34x | yes |
| 73 | BenchmarkMicrotaskOverflow |          25.04 |       3.5% |         24.45 |      0.3% | Linux |  1.02x |  |
| 74 | BenchmarkMicrotaskRingIsEmpty |           8.04 |       0.8% |          8.01 |      0.1% | Linux |  1.00x |  |
| 75 | BenchmarkMicrotaskRingIsEmpty_WithItems |           2.03 |       8.4% |          1.97 |      0.4% | Linux |  1.03x |  |
| 76 | BenchmarkMicrotaskSchedule |          86.24 |       2.7% |         78.02 |     16.8% | Linux |  1.11x |  |
| 77 | BenchmarkMicrotaskSchedule_Parallel |         111.58 |       0.3% |         68.89 |      6.5% | Linux |  1.62x | yes |
| 78 | BenchmarkMinimalLoop |         426.92 |       0.5% |        444.34 |      1.4% | Darwin |  0.96x | yes |
| 79 | BenchmarkMixedWorkload |       1,016.00 |       0.8% |        856.40 |      1.1% | Linux |  1.19x | yes |
| 80 | BenchmarkNoMetrics |          59.06 |      13.8% |         49.23 |     16.8% | Linux |  1.20x |  |
| 81 | BenchmarkPromiseAll |       1,540.40 |       6.9% |      1,664.00 |      1.7% | Darwin |  0.93x |  |
| 82 | BenchmarkPromiseAll_Memory |       1,362.20 |       0.4% |      1,567.20 |      3.7% | Darwin |  0.87x | yes |
| 83 | BenchmarkPromiseChain |         460.44 |       3.7% |        526.88 |      4.7% | Darwin |  0.87x | yes |
| 84 | BenchmarkPromiseCreate |          63.01 |       4.2% |         86.77 |      1.6% | Darwin |  0.73x | yes |
| 85 | BenchmarkPromiseCreation |          64.93 |       0.3% |        101.82 |      3.1% | Darwin |  0.64x | yes |
| 86 | BenchmarkPromiseGC |      63,005.00 |       0.8% |    113,294.60 |      2.6% | Darwin |  0.56x | yes |
| 87 | BenchmarkPromiseHandlerTracking_Optimized |          89.03 |       6.1% |         93.96 |      1.7% | Darwin |  0.95x |  |
| 88 | BenchmarkPromiseHandlerTracking_Parallel_Optimized |         345.02 |       1.3% |        155.52 |      1.5% | Linux |  2.22x | yes |
| 89 | BenchmarkPromiseRace |       1,217.40 |       0.5% |      1,337.60 |      2.1% | Darwin |  0.91x | yes |
| 90 | BenchmarkPromiseReject |         501.10 |       1.8% |        627.30 |      2.6% | Darwin |  0.80x | yes |
| 91 | BenchmarkPromiseRejection |         495.52 |       4.2% |        607.74 |      3.7% | Darwin |  0.82x | yes |
| 92 | BenchmarkPromiseResolution |          94.91 |       0.5% |        130.70 |      1.6% | Darwin |  0.73x | yes |
| 93 | BenchmarkPromiseResolve |          86.25 |       6.4% |        110.36 |      1.8% | Darwin |  0.78x | yes |
| 94 | BenchmarkPromiseResolve_Memory |          95.71 |       0.6% |        132.48 |      0.9% | Darwin |  0.72x | yes |
| 95 | BenchmarkPromiseThen |         327.28 |       0.6% |        329.54 |      3.4% | Darwin |  0.99x |  |
| 96 | BenchmarkPromiseThenChain |         556.96 |       1.2% |        667.58 |      2.2% | Darwin |  0.83x | yes |
| 97 | BenchmarkPromiseTry |          97.76 |       0.4% |        140.10 |      6.0% | Darwin |  0.70x | yes |
| 98 | BenchmarkPromiseWithResolvers |          94.68 |       0.5% |        133.84 |      2.1% | Darwin |  0.71x | yes |
| 99 | BenchmarkPromisifyAllocation |       5,979.00 |       1.1% |      7,945.00 |      4.1% | Darwin |  0.75x | yes |
| 100 | BenchmarkPureChannelPingPong |         345.18 |       1.7% |        376.70 |      2.1% | Darwin |  0.92x | yes |
| 101 | BenchmarkQueueMicrotask |          90.32 |       5.9% |         71.26 |     11.5% | Linux |  1.27x | yes |
| 102 | BenchmarkQuiescing_ScheduleTimer_NoAutoExit |      21,190.40 |       1.1% |     39,773.00 |      3.5% | Darwin |  0.53x | yes |
| 103 | BenchmarkQuiescing_ScheduleTimer_WithAutoExit |      23,117.60 |      10.3% |     43,583.00 |      1.1% | Darwin |  0.53x | yes |
| 104 | BenchmarkRefUnref_IsLoopThread_External |       7,214.00 |       0.3% |      6,391.00 |     22.3% | Linux |  1.13x |  |
| 105 | BenchmarkRefUnref_IsLoopThread_OnLoop |      10,083.80 |       0.6% |      9,997.80 |      0.5% | Linux |  1.01x |  |
| 106 | BenchmarkRefUnref_RWMutex_External |          34.25 |       0.5% |         34.06 |      0.3% | Linux |  1.01x |  |
| 107 | BenchmarkRefUnref_RWMutex_OnLoop |          34.25 |       0.2% |         34.60 |      0.2% | Darwin |  0.99x | yes |
| 108 | BenchmarkRefUnref_SubmitInternal_External |         168.46 |       2.3% |        204.80 |      4.4% | Darwin |  0.82x | yes |
| 109 | BenchmarkRefUnref_SubmitInternal_OnLoop |      11,297.40 |       0.7% |     11,153.00 |      0.3% | Linux |  1.01x | yes |
| 110 | BenchmarkRefUnref_SyncMap_External |          25.83 |       1.0% |         25.36 |      0.2% | Linux |  1.02x | yes |
| 111 | BenchmarkRefUnref_SyncMap_OnLoop |          25.48 |       0.4% |         25.08 |      0.2% | Linux |  1.02x | yes |
| 112 | BenchmarkRegression_Combined_Atomic |           0.47 |       0.4% |          0.48 |      1.5% | Darwin |  0.98x | yes |
| 113 | BenchmarkRegression_Combined_Mutex |          17.85 |       2.7% |         17.96 |      0.6% | Darwin |  0.99x |  |
| 114 | BenchmarkRegression_FastPathWakeup_NoWork |       9,783.00 |      74.3% |     10,680.80 |    146.0% | Darwin |  0.92x |  |
| 115 | BenchmarkRegression_HasExternalTasks_Mutex |           9.70 |       2.1% |          9.64 |      3.0% | Linux |  1.01x |  |
| 116 | BenchmarkRegression_HasInternalTasks_Mutex |           9.53 |       2.0% |          9.44 |      0.1% | Linux |  1.01x |  |
| 117 | BenchmarkRegression_HasInternalTasks_SimulatedAtomic |           0.31 |       2.5% |          0.31 |      0.8% | Darwin |  1.00x |  |
| 118 | BenchmarkScheduleTimerCancel |      23,930.20 |       3.2% |     39,530.40 |      5.9% | Darwin |  0.61x | yes |
| 119 | BenchmarkScheduleTimerWithPool |       4,380.40 |       0.3% |      3,740.80 |      0.5% | Linux |  1.17x | yes |
| 120 | BenchmarkScheduleTimerWithPool_FireAndReuse |       4,333.80 |       3.3% |      3,977.00 |      0.2% | Linux |  1.09x | yes |
| 121 | BenchmarkScheduleTimerWithPool_Immediate |       4,341.20 |       0.5% |      3,641.00 |      0.8% | Linux |  1.19x | yes |
| 122 | BenchmarkSentinelDrain_NoWork |      12,743.40 |      51.0% |     12,224.00 |    153.3% | Linux |  1.04x |  |
| 123 | BenchmarkSentinelDrain_WithTimers |      42,258.40 |      12.4% |    101,303.60 |      0.4% | Darwin |  0.42x | yes |
| 124 | BenchmarkSentinelIteration |       7,411.00 |      59.4% |     11,815.20 |    151.4% | Darwin |  0.63x |  |
| 125 | BenchmarkSentinelIteration_WithTimers |      14,870.40 |       1.2% |     41,797.60 |      4.7% | Darwin |  0.36x | yes |
| 126 | BenchmarkSetImmediate_Optimized |         195.50 |       3.2% |        127.82 |      1.2% | Linux |  1.53x | yes |
| 127 | BenchmarkSetInterval_Optimized |      33,110.00 |      10.7% |     40,992.20 |      4.6% | Darwin |  0.81x | yes |
| 128 | BenchmarkSetInterval_Parallel_Optimized |      16,210.20 |       0.9% |     21,467.40 |      1.4% | Darwin |  0.76x | yes |
| 129 | BenchmarkSetTimeoutZeroDelay |      22,807.80 |       1.2% |     39,482.20 |      5.1% | Darwin |  0.58x | yes |
| 130 | BenchmarkSetTimeout_Optimized |      25,641.80 |      17.4% |     42,295.20 |      4.8% | Darwin |  0.61x | yes |
| 131 | BenchmarkSubmit |          53.32 |       2.1% |         42.57 |      2.2% | Linux |  1.25x | yes |
| 132 | BenchmarkSubmitExecution |          67.67 |       2.8% |         55.56 |      0.7% | Linux |  1.22x | yes |
| 133 | BenchmarkSubmitInternal |       3,574.20 |       1.3% |      2,918.40 |      4.2% | Linux |  1.22x | yes |
| 134 | BenchmarkSubmitInternal_Cost |       3,635.40 |       0.3% |      2,983.80 |      1.7% | Linux |  1.22x | yes |
| 135 | BenchmarkSubmitInternal_FastPath_OnLoop |       5,180.60 |       1.3% |      5,221.60 |      1.7% | Darwin |  0.99x |  |
| 136 | BenchmarkSubmitInternal_QueuePath_OnLoop |          36.18 |       6.2% |         39.15 |      5.3% | Darwin |  0.92x |  |
| 137 | BenchmarkSubmitLatency |         462.52 |       3.9% |        334.94 |      3.7% | Linux |  1.38x | yes |
| 138 | BenchmarkSubmit_Parallel |         101.50 |       0.5% |         62.22 |      3.5% | Linux |  1.63x | yes |
| 139 | BenchmarkTask1_2_ConcurrentSubmissions |          97.98 |       3.4% |         62.88 |      2.1% | Linux |  1.56x | yes |
| 140 | BenchmarkTerminated_RejectionPath_Promisify |         453.54 |       1.6% |        509.80 |      2.0% | Darwin |  0.89x | yes |
| 141 | BenchmarkTerminated_RejectionPath_RefTimer |           2.00 |       1.2% |          2.01 |      0.2% | Darwin |  0.99x |  |
| 142 | BenchmarkTerminated_RejectionPath_ScheduleTimer |          46.29 |       3.7% |         53.93 |      1.9% | Darwin |  0.86x | yes |
| 143 | BenchmarkTerminated_RejectionPath_submitToQueue |           9.68 |       0.5% |          9.79 |      0.2% | Darwin |  0.99x | yes |
| 144 | BenchmarkTerminated_UnrefTimer_NotGated |           2.01 |       1.9% |          2.01 |      0.4% | Darwin |  1.00x |  |
| 145 | BenchmarkTimerFire |       4,513.60 |       0.7% |      3,680.20 |      0.4% | Linux |  1.23x | yes |
| 146 | BenchmarkTimerHeapOperations |          69.64 |       3.8% |         87.29 |      2.6% | Darwin |  0.80x | yes |
| 147 | BenchmarkTimerLatency |      19,582.60 |       1.2% |     52,290.80 |      2.3% | Darwin |  0.37x | yes |
| 148 | BenchmarkTimerSchedule |      21,516.80 |       0.9% |     38,359.60 |     11.7% | Darwin |  0.56x | yes |
| 149 | BenchmarkTimerSchedule_Parallel |      13,643.80 |       0.2% |     18,839.60 |      2.2% | Darwin |  0.72x | yes |
| 150 | Benchmark_chunkedIngress_Batch |         526.28 |       2.3% |        525.38 |      0.8% | Linux |  1.00x |  |
| 151 | Benchmark_chunkedIngress_ParallelWithSync |          84.27 |       2.5% |         44.26 |      3.0% | Linux |  1.90x | yes |
| 152 | Benchmark_chunkedIngress_Pop |           3.18 |       4.3% |          4.95 |     35.6% | Darwin |  0.64x |  |
| 153 | Benchmark_chunkedIngress_Push |           8.38 |       5.2% |          9.13 |      4.1% | Darwin |  0.92x | yes |
| 154 | Benchmark_chunkedIngress_PushPop |           4.00 |       0.1% |          4.02 |      0.4% | Darwin |  0.99x | yes |
| 155 | Benchmark_chunkedIngress_Sequential |           4.10 |       3.1% |          4.11 |      0.4% | Darwin |  1.00x |  |
| 156 | Benchmark_microtaskRing_Parallel |         121.12 |       4.3% |        110.73 |     10.5% | Linux |  1.09x |  |
| 157 | Benchmark_microtaskRing_Push |          29.02 |       5.6% |         48.89 |      8.9% | Darwin |  0.59x | yes |
| 158 | Benchmark_microtaskRing_PushPop |          22.27 |       0.3% |         22.64 |      0.4% | Darwin |  0.98x | yes |

## Performance by Category

### Concurrency (5 benchmarks)

- Darwin wins: 3/5
- Linux wins: 2/5
- Darwin category mean: 3,349.83 ns/op
- Linux category mean: 4,488.93 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkSetInterval_Parallel_Optimized |      16,210.20 |     21,467.40 | Darwin | 1.32x |
| BenchmarkAlive_WithMutexes_HighContention |         181.54 |        726.56 | Darwin | 4.00x |
| BenchmarkHighContention |         223.98 |        150.76 | Linux | 1.49x |
| BenchmarkTask1_2_ConcurrentSubmissions |          97.98 |         62.88 | Linux | 1.56x |
| BenchmarkAlive_Epoch_NoContention |          35.47 |         37.05 | Darwin | 1.04x |

### Latency & Primitives (29 benchmarks)

- Darwin wins: 19/29
- Linux wins: 10/29
- Darwin category mean: 1,010.37 ns/op
- Linux category mean: 2,170.53 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkTimerLatency |      19,582.60 |     52,290.80 | Darwin | 2.67x |
| BenchmarkLatencySample_OldSortBased |       6,250.40 |      7,808.00 | Darwin | 1.25x |
| BenchmarkLatencyChannelRoundTrip |         373.50 |        237.84 | Linux | 1.57x |
| BenchmarkLatencyAnalysis_PingPong |         565.26 |        434.32 | Linux | 1.30x |
| BenchmarkSubmitLatency |         462.52 |        334.94 | Linux | 1.38x |
| BenchmarkMicrotaskLatency |         456.12 |        339.84 | Linux | 1.34x |
| BenchmarkLatencyAnalysis_SubmitWhileRunning |         432.34 |        331.24 | Linux | 1.31x |
| BenchmarkLatencychunkedIngressPush_WithContention |          76.86 |         38.77 | Linux | 1.98x |
| BenchmarkLatencymicrotaskRingPush |          29.75 |         51.18 | Darwin | 1.72x |
| BenchmarkLatencyChannelBufferedRoundTrip |         266.92 |        247.32 | Linux | 1.08x |
| BenchmarkLatencyAnalysis_EndToEnd |         551.54 |        570.62 | Darwin | 1.03x |
| BenchmarkLatencySimulatedSubmit |          14.03 |         16.45 | Darwin | 1.17x |
| BenchmarkLatencychunkedIngressPush |           8.21 |          9.88 | Darwin | 1.20x |
| BenchmarkLatencychunkedIngressPop |           4.50 |          5.66 | Darwin | 1.26x |
| BenchmarkLatencyRecord_WithPSquare |          77.36 |         78.31 | Darwin | 1.01x |
| BenchmarkLatencymicrotaskRingPop |          15.58 |         16.37 | Darwin | 1.05x |
| BenchmarkLatencySimulatedPoll |          13.63 |         13.91 | Darwin | 1.02x |
| BenchmarkLatencySample_NewPSquare |          24.82 |         24.99 | Darwin | 1.01x |
| BenchmarkLatencyRecord_WithoutPSquare |          24.36 |         24.20 | Linux | 1.01x |
| BenchmarkLatencyMutexLockUnlock |           7.91 |          8.00 | Darwin | 1.01x |
| BenchmarkLatencymicrotaskRingPushPop |          22.61 |         22.55 | Linux | 1.00x |
| BenchmarkLatencyStateTryTransition |           4.09 |          4.12 | Darwin | 1.01x |
| BenchmarkLatencyStateTryTransition_NoOp |          17.59 |         17.61 | Darwin | 1.00x |
| BenchmarkLatencySafeExecute |           3.07 |          3.08 | Darwin | 1.00x |
| BenchmarkLatencychunkedIngressPushPop |           4.06 |          4.06 | Darwin | 1.00x |
| BenchmarkLatencyRWMutexRLockRUnlock |           8.16 |          8.15 | Linux | 1.00x |
| BenchmarkLatencyDeferRecover |           2.45 |          2.46 | Darwin | 1.00x |
| BenchmarkLatencyStateLoad |           0.31 |          0.31 | Darwin | 1.01x |
| BenchmarkLatencyDirectCall |           0.31 |          0.31 | Darwin | 1.01x |

### Other (48 benchmarks)

- Darwin wins: 26/48
- Linux wins: 22/48
- Darwin category mean: 36,540.96 ns/op
- Linux category mean: 60,070.63 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkAutoExit_UnrefExit |   1,259,395.00 |  2,213,390.80 | Darwin | 1.76x |
| BenchmarkAutoExit_ImmediateExit |     203,965.60 |    401,694.20 | Darwin | 1.97x |
| BenchmarkAutoExit_PollPathExit |     135,692.60 |     70,527.80 | Linux | 1.92x |
| BenchmarkSetTimeoutZeroDelay |      22,807.80 |     39,482.20 | Darwin | 1.73x |
| BenchmarkSetTimeout_Optimized |      25,641.80 |     42,295.20 | Darwin | 1.65x |
| BenchmarkSetInterval_Optimized |      33,110.00 |     40,992.20 | Darwin | 1.24x |
| BenchmarkSentinelIteration |       7,411.00 |     11,815.20 | Darwin | 1.59x |
| BenchmarkAlive_Epoch_FromCallback |       7,644.00 |      3,771.60 | Linux | 2.03x |
| BenchmarkPromisifyAllocation |       5,979.00 |      7,945.00 | Darwin | 1.33x |
| BenchmarkRefUnref_IsLoopThread_External |       7,214.00 |      6,391.00 | Linux | 1.13x |
| BenchmarkHighFrequencyMonitoring_Old |       8,676.00 |      9,378.60 | Darwin | 1.08x |
| BenchmarkSentinelDrain_NoWork |      12,743.40 |     12,224.00 | Linux | 1.04x |
| BenchmarkMixedWorkload |       1,016.00 |        856.40 | Linux | 1.19x |
| BenchmarkIsLoopThread_True |       4,636.00 |      4,493.40 | Linux | 1.03x |
| BenchmarkRefUnref_IsLoopThread_OnLoop |      10,083.80 |      9,997.80 | Linux | 1.01x |
| BenchmarkSetImmediate_Optimized |         195.50 |        127.82 | Linux | 1.53x |
| BenchmarkTerminated_RejectionPath_Promisify |         453.54 |        509.80 | Darwin | 1.12x |
| BenchmarkMicroPingPongWithCount |         423.24 |        478.78 | Darwin | 1.13x |
| BenchmarkMicroPingPong |         422.38 |        472.40 | Darwin | 1.12x |
| BenchmarkLoopDirect |         461.22 |        506.88 | Darwin | 1.10x |
| BenchmarkPureChannelPingPong |         345.18 |        376.70 | Darwin | 1.09x |
| BenchmarkGetGoroutineID |       1,760.60 |      1,740.60 | Linux | 1.01x |
| BenchmarkGojaStyleSwap |         423.32 |        441.36 | Darwin | 1.04x |
| BenchmarkMinimalLoop |         426.92 |        444.34 | Darwin | 1.04x |
| BenchmarkChannelWithMutexQueue |         444.48 |        457.16 | Darwin | 1.03x |
| BenchmarkAutoExit_AliveCheckCost_Disabled |          54.29 |         41.71 | Linux | 1.30x |
| BenchmarkNoMetrics |          59.06 |         49.23 | Linux | 1.20x |
| BenchmarkGetGoroutineIDOld |       1,763.60 |      1,771.40 | Darwin | 1.00x |
| BenchmarkAutoExit_AliveCheckCost_Enabled |          49.86 |         44.49 | Linux | 1.12x |
| BenchmarkMetricsCollection |          38.16 |         43.07 | Darwin | 1.13x |
| BenchmarkCombinedWorkload_Old |         210.84 |        215.20 | Darwin | 1.02x |
| BenchmarkAlive_AllAtomic |          21.27 |         18.43 | Linux | 1.15x |
| BenchmarkAlive_WithMutexes_Contended |          37.50 |         34.78 | Linux | 1.08x |
| BenchmarkCombinedWorkload_New |          86.95 |         88.36 | Darwin | 1.02x |
| BenchmarkAlive_Uncontended |          35.44 |         36.70 | Darwin | 1.04x |
| BenchmarkRefUnref_SyncMap_External |          25.83 |         25.36 | Linux | 1.02x |
| BenchmarkRefUnref_SyncMap_OnLoop |          25.48 |         25.08 | Linux | 1.02x |
| BenchmarkRefUnref_RWMutex_OnLoop |          34.25 |         34.60 | Darwin | 1.01x |
| BenchmarkAlive_AllAtomic_Contended |          18.68 |         18.33 | Linux | 1.02x |
| BenchmarkAlive_WithMutexes |          35.06 |         34.82 | Linux | 1.01x |
| BenchmarkRefUnref_RWMutex_External |          34.25 |         34.06 | Linux | 1.01x |
| BenchmarkHighFrequencyMonitoring_New |          24.80 |         24.98 | Darwin | 1.01x |
| BenchmarkRegression_Combined_Mutex |          17.85 |         17.96 | Darwin | 1.01x |
| BenchmarkRegression_HasInternalTasks_Mutex |           9.53 |          9.44 | Linux | 1.01x |
| BenchmarkRegression_HasExternalTasks_Mutex |           9.70 |          9.64 | Linux | 1.01x |
| BenchmarkRegression_Combined_Atomic |           0.47 |          0.48 | Darwin | 1.02x |
| BenchmarkIsLoopThread_False |           0.35 |          0.35 | Linux | 1.01x |
| BenchmarkRegression_HasInternalTasks_SimulatedAtomic |           0.31 |          0.31 | Darwin | 1.00x |

### Promise Operations (18 benchmarks)

- Darwin wins: 17/18
- Linux wins: 1/18
- Darwin category mean: 3,916.53 ns/op
- Linux category mean: 6,761.56 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkPromiseGC |      63,005.00 |    113,294.60 | Darwin | 1.80x |
| BenchmarkPromiseAll_Memory |       1,362.20 |      1,567.20 | Darwin | 1.15x |
| BenchmarkPromiseHandlerTracking_Parallel_Optimized |         345.02 |        155.52 | Linux | 2.22x |
| BenchmarkPromiseReject |         501.10 |        627.30 | Darwin | 1.25x |
| BenchmarkPromiseAll |       1,540.40 |      1,664.00 | Darwin | 1.08x |
| BenchmarkPromiseRace |       1,217.40 |      1,337.60 | Darwin | 1.10x |
| BenchmarkPromiseRejection |         495.52 |        607.74 | Darwin | 1.23x |
| BenchmarkPromiseThenChain |         556.96 |        667.58 | Darwin | 1.20x |
| BenchmarkPromiseChain |         460.44 |        526.88 | Darwin | 1.14x |
| BenchmarkPromiseTry |          97.76 |        140.10 | Darwin | 1.43x |
| BenchmarkPromiseWithResolvers |          94.68 |        133.84 | Darwin | 1.41x |
| BenchmarkPromiseCreation |          64.93 |        101.82 | Darwin | 1.57x |
| BenchmarkPromiseResolve_Memory |          95.71 |        132.48 | Darwin | 1.38x |
| BenchmarkPromiseResolution |          94.91 |        130.70 | Darwin | 1.38x |
| BenchmarkPromiseResolve |          86.25 |        110.36 | Darwin | 1.28x |
| BenchmarkPromiseCreate |          63.01 |         86.77 | Darwin | 1.38x |
| BenchmarkPromiseHandlerTracking_Optimized |          89.03 |         93.96 | Darwin | 1.06x |
| BenchmarkPromiseThen |         327.28 |        329.54 | Darwin | 1.01x |

### Task Submission (32 benchmarks)

- Darwin wins: 15/32
- Linux wins: 17/32
- Darwin category mean: 8,186.76 ns/op
- Linux category mean: 14,560.14 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkAutoExit_FastPathExit |     211,352.00 |    388,627.40 | Darwin | 1.84x |
| BenchmarkLoopDirectWithSubmit |      15,131.20 |     41,866.80 | Darwin | 2.77x |
| BenchmarkRegression_FastPathWakeup_NoWork |       9,783.00 |     10,680.80 | Darwin | 1.09x |
| BenchmarkSubmitInternal |       3,574.20 |      2,918.40 | Linux | 1.22x |
| BenchmarkSubmitInternal_Cost |       3,635.40 |      2,983.80 | Linux | 1.22x |
| BenchmarkAlive_Epoch_ConcurrentSubmit |         245.22 |        804.40 | Darwin | 3.28x |
| BenchmarkRefUnref_SubmitInternal_OnLoop |      11,297.40 |     11,153.00 | Linux | 1.01x |
| BenchmarkMicrotaskSchedule_Parallel |         111.58 |         68.89 | Linux | 1.62x |
| BenchmarkSubmitInternal_FastPath_OnLoop |       5,180.60 |      5,221.60 | Darwin | 1.01x |
| Benchmark_chunkedIngress_ParallelWithSync |          84.27 |         44.26 | Linux | 1.90x |
| BenchmarkSubmit_Parallel |         101.50 |         62.22 | Linux | 1.63x |
| BenchmarkMicrotaskExecution |          90.54 |        129.16 | Darwin | 1.43x |
| BenchmarkRefUnref_SubmitInternal_External |         168.46 |        204.80 | Darwin | 1.22x |
| Benchmark_microtaskRing_Push |          29.02 |         48.89 | Darwin | 1.68x |
| BenchmarkQueueMicrotask |          90.32 |         71.26 | Linux | 1.27x |
| BenchmarkFastPathExecution |          70.57 |         55.72 | Linux | 1.27x |
| BenchmarkSubmitExecution |          67.67 |         55.56 | Linux | 1.22x |
| BenchmarkFastPathSubmit |          53.61 |         42.65 | Linux | 1.26x |
| BenchmarkSubmit |          53.32 |         42.57 | Linux | 1.25x |
| Benchmark_microtaskRing_Parallel |         121.12 |        110.73 | Linux | 1.09x |
| BenchmarkMicrotaskSchedule |          86.24 |         78.02 | Linux | 1.11x |
| BenchmarkSubmitInternal_QueuePath_OnLoop |          36.18 |         39.15 | Darwin | 1.08x |
| Benchmark_chunkedIngress_Pop |           3.18 |          4.95 | Darwin | 1.55x |
| Benchmark_chunkedIngress_Batch |         526.28 |        525.38 | Linux | 1.00x |
| Benchmark_chunkedIngress_Push |           8.38 |          9.13 | Darwin | 1.09x |
| BenchmarkMicrotaskOverflow |          25.04 |         24.45 | Linux | 1.02x |
| Benchmark_microtaskRing_PushPop |          22.27 |         22.64 | Darwin | 1.02x |
| BenchmarkTerminated_RejectionPath_submitToQueue |           9.68 |          9.79 | Darwin | 1.01x |
| BenchmarkMicrotaskRingIsEmpty_WithItems |           2.03 |          1.97 | Linux | 1.03x |
| BenchmarkMicrotaskRingIsEmpty |           8.04 |          8.01 | Linux | 1.00x |
| Benchmark_chunkedIngress_PushPop |           4.00 |          4.02 | Darwin | 1.01x |
| Benchmark_chunkedIngress_Sequential |           4.10 |          4.11 | Darwin | 1.00x |

### Timer Operations (26 benchmarks)

- Darwin wins: 18/26
- Linux wins: 8/26
- Darwin category mean: 258,446.56 ns/op
- Linux category mean: 469,386.84 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkCancelTimer_Individual/timers_: |   2,541,663.60 |  5,238,622.80 | Darwin | 2.06x |
| BenchmarkCancelTimers_Comparison/Individual |   1,292,803.40 |  2,590,545.80 | Darwin | 2.00x |
| BenchmarkCancelTimer_Individual/timers_5 |   1,275,863.60 |  2,501,492.60 | Darwin | 1.96x |
| BenchmarkCancelTimer_Individual/timers_1 |     251,132.80 |    441,482.00 | Darwin | 1.76x |
| BenchmarkSentinelDrain_WithTimers |      42,258.40 |    101,303.60 | Darwin | 2.40x |
| BenchmarkAutoExit_TimerFire |     114,878.80 |     72,323.00 | Linux | 1.59x |
| BenchmarkCancelTimers_Batch/timers_: |     478,140.40 |    445,835.60 | Linux | 1.07x |
| BenchmarkSentinelIteration_WithTimers |      14,870.40 |     41,797.60 | Darwin | 2.81x |
| BenchmarkCancelTimers_Batch/timers_5 |     249,934.00 |    227,781.20 | Linux | 1.10x |
| BenchmarkCancelTimers_Comparison/Batch |     248,791.60 |    227,991.00 | Linux | 1.09x |
| BenchmarkQuiescing_ScheduleTimer_WithAutoExit |      23,117.60 |     43,583.00 | Darwin | 1.89x |
| BenchmarkLargeTimerHeap |      23,146.60 |     42,925.20 | Darwin | 1.85x |
| BenchmarkQuiescing_ScheduleTimer_NoAutoExit |      21,190.40 |     39,773.00 | Darwin | 1.88x |
| BenchmarkTimerSchedule |      21,516.80 |     38,359.60 | Darwin | 1.78x |
| BenchmarkScheduleTimerCancel |      23,930.20 |     39,530.40 | Darwin | 1.65x |
| BenchmarkCancelTimers_Batch/timers_1 |      65,037.20 |     76,685.60 | Darwin | 1.18x |
| BenchmarkTimerSchedule_Parallel |      13,643.80 |     18,839.60 | Darwin | 1.38x |
| BenchmarkTimerFire |       4,513.60 |      3,680.20 | Linux | 1.23x |
| BenchmarkScheduleTimerWithPool_Immediate |       4,341.20 |      3,641.00 | Linux | 1.19x |
| BenchmarkScheduleTimerWithPool |       4,380.40 |      3,740.80 | Linux | 1.17x |
| BenchmarkScheduleTimerWithPool_FireAndReuse |       4,333.80 |      3,977.00 | Linux | 1.09x |
| BenchmarkTimerHeapOperations |          69.64 |         87.29 | Darwin | 1.25x |
| BenchmarkTerminated_RejectionPath_ScheduleTimer |          46.29 |         53.93 | Darwin | 1.16x |
| BenchmarkAlive_WithTimer |           2.04 |          2.08 | Darwin | 1.02x |
| BenchmarkTerminated_RejectionPath_RefTimer |           2.00 |          2.01 | Darwin | 1.01x |
| BenchmarkTerminated_UnrefTimer_NotGated |           2.01 |          2.01 | Darwin | 1.00x |

## Statistically Significant Differences

**107** out of 158 benchmarks show statistically significant
differences (Welch's t-test, p < 0.05).

- Darwin significantly faster: **69** benchmarks
- Linux significantly faster: **38** benchmarks

### Largest Significant Differences

| Benchmark | Faster | Speedup | Darwin (ns/op) | Linux (ns/op) | t-stat |
|-----------|--------|---------|----------------|---------------|--------|
| BenchmarkAlive_WithMutexes_HighContention | Darwin | 4.00x |         181.54 |        726.56 | 5.38 |
| BenchmarkAlive_Epoch_ConcurrentSubmit | Darwin | 3.28x |         245.22 |        804.40 | 7.52 |
| BenchmarkSentinelIteration_WithTimers | Darwin | 2.81x |      14,870.40 |     41,797.60 | 30.31 |
| BenchmarkLoopDirectWithSubmit | Darwin | 2.77x |      15,131.20 |     41,866.80 | 27.27 |
| BenchmarkTimerLatency | Darwin | 2.67x |      19,582.60 |     52,290.80 | 60.01 |
| BenchmarkSentinelDrain_WithTimers | Darwin | 2.40x |      42,258.40 |    101,303.60 | 25.07 |
| BenchmarkPromiseHandlerTracking_Parallel_Optimized | Linux | 2.22x |         345.02 |        155.52 | 85.65 |
| BenchmarkCancelTimer_Individual/timers_: | Darwin | 2.06x |   2,541,663.60 |  5,238,622.80 | 110.75 |
| BenchmarkCancelTimers_Comparison/Individual | Darwin | 2.00x |   1,292,803.40 |  2,590,545.80 | 25.75 |
| BenchmarkLatencychunkedIngressPush_WithContention | Linux | 1.98x |          76.86 |         38.77 | 71.99 |
| BenchmarkCancelTimer_Individual/timers_5 | Darwin | 1.96x |   1,275,863.60 |  2,501,492.60 | 111.93 |
| BenchmarkAutoExit_PollPathExit | Linux | 1.92x |     135,692.60 |     70,527.80 | 3.19 |
| Benchmark_chunkedIngress_ParallelWithSync | Linux | 1.90x |          84.27 |         44.26 | 35.67 |
| BenchmarkQuiescing_ScheduleTimer_WithAutoExit | Darwin | 1.89x |      23,117.60 |     43,583.00 | 18.87 |
| BenchmarkQuiescing_ScheduleTimer_NoAutoExit | Darwin | 1.88x |      21,190.40 |     39,773.00 | 29.06 |
| BenchmarkLargeTimerHeap | Darwin | 1.85x |      23,146.60 |     42,925.20 | 28.74 |
| BenchmarkPromiseGC | Darwin | 1.80x |      63,005.00 |    113,294.60 | 37.25 |
| BenchmarkTimerSchedule | Darwin | 1.78x |      21,516.80 |     38,359.60 | 8.41 |
| BenchmarkCancelTimer_Individual/timers_1 | Darwin | 1.76x |     251,132.80 |    441,482.00 | 60.17 |
| BenchmarkAutoExit_UnrefExit | Darwin | 1.76x |   1,259,395.00 |  2,213,390.80 | 95.07 |
| BenchmarkSetTimeoutZeroDelay | Darwin | 1.73x |      22,807.80 |     39,482.20 | 18.17 |
| BenchmarkLatencymicrotaskRingPush | Darwin | 1.72x |          29.75 |         51.18 | 5.93 |
| Benchmark_microtaskRing_Push | Darwin | 1.68x |          29.02 |         48.89 | 9.59 |
| BenchmarkScheduleTimerCancel | Darwin | 1.65x |      23,930.20 |     39,530.40 | 14.18 |
| BenchmarkSetTimeout_Optimized | Darwin | 1.65x |      25,641.80 |     42,295.20 | 7.59 |
| BenchmarkSubmit_Parallel | Linux | 1.63x |         101.50 |         62.22 | 38.91 |
| BenchmarkMicrotaskSchedule_Parallel | Linux | 1.62x |         111.58 |         68.89 | 21.39 |
| BenchmarkAutoExit_TimerFire | Linux | 1.59x |     114,878.80 |     72,323.00 | 9.11 |
| BenchmarkLatencyChannelRoundTrip | Linux | 1.57x |         373.50 |        237.84 | 36.44 |
| BenchmarkPromiseCreation | Darwin | 1.57x |          64.93 |        101.82 | 26.49 |

## Top 10 Fastest Benchmarks

### Darwin

| Rank | Benchmark | ns/op | B/op | Allocs/op | CV% |
|------|-----------|-------|------|-----------|-----|
| 1 | BenchmarkLatencyDirectCall |       0.31 |    0 |         0 | 0.5% |
| 2 | BenchmarkLatencyStateLoad |       0.31 |    0 |         0 | 0.6% |
| 3 | BenchmarkRegression_HasInternalTasks_SimulatedAtom |       0.31 |    0 |         0 | 2.5% |
| 4 | BenchmarkIsLoopThread_False |       0.35 |    0 |         0 | 2.3% |
| 5 | BenchmarkRegression_Combined_Atomic |       0.47 |    0 |         0 | 0.4% |
| 6 | BenchmarkTerminated_RejectionPath_RefTimer |       2.00 |    0 |         0 | 1.2% |
| 7 | BenchmarkTerminated_UnrefTimer_NotGated |       2.01 |    0 |         0 | 1.9% |
| 8 | BenchmarkMicrotaskRingIsEmpty_WithItems |       2.03 |    0 |         0 | 8.4% |
| 9 | BenchmarkAlive_WithTimer |       2.04 |    0 |         0 | 0.2% |
| 10 | BenchmarkLatencyDeferRecover |       2.45 |    0 |         0 | 0.5% |

### Linux

| Rank | Benchmark | ns/op | B/op | Allocs/op | CV% |
|------|-----------|-------|------|-----------|-----|
| 1 | BenchmarkLatencyDirectCall |       0.31 |    0 |         0 | 0.8% |
| 2 | BenchmarkRegression_HasInternalTasks_SimulatedAtom |       0.31 |    0 |         0 | 0.8% |
| 3 | BenchmarkLatencyStateLoad |       0.31 |    0 |         0 | 1.1% |
| 4 | BenchmarkIsLoopThread_False |       0.35 |    0 |         0 | 1.3% |
| 5 | BenchmarkRegression_Combined_Atomic |       0.48 |    0 |         0 | 1.5% |
| 6 | BenchmarkMicrotaskRingIsEmpty_WithItems |       1.97 |    0 |         0 | 0.4% |
| 7 | BenchmarkTerminated_RejectionPath_RefTimer |       2.01 |    0 |         0 | 0.2% |
| 8 | BenchmarkTerminated_UnrefTimer_NotGated |       2.01 |    0 |         0 | 0.4% |
| 9 | BenchmarkAlive_WithTimer |       2.08 |    0 |         0 | 0.7% |
| 10 | BenchmarkLatencyDeferRecover |       2.46 |    0 |         0 | 0.1% |

## Allocation Comparison

Since both platforms run the same Go code, allocations (allocs/op) and bytes (B/op)
should be identical. Differences indicate platform-specific runtime behavior.

- **Allocs/op match:** 151/158 (95.6%)
- **B/op match:** 117/158 (74.1%)
- **Zero-allocation benchmarks (both platforms):** 76

### Zero-Allocation Benchmarks

These benchmarks achieve zero allocations on both platforms — the gold standard
for hot-path performance:

- `BenchmarkAlive_AllAtomic` — Darwin: 21.27 ns/op, Linux: 18.43 ns/op (Linux faster)
- `BenchmarkAlive_AllAtomic_Contended` — Darwin: 18.68 ns/op, Linux: 18.33 ns/op (Linux faster)
- `BenchmarkAlive_Epoch_ConcurrentSubmit` — Darwin: 245.22 ns/op, Linux: 804.40 ns/op (Darwin faster)
- `BenchmarkAlive_Epoch_NoContention` — Darwin: 35.47 ns/op, Linux: 37.05 ns/op (Darwin faster)
- `BenchmarkAlive_Uncontended` — Darwin: 35.44 ns/op, Linux: 36.70 ns/op (Darwin faster)
- `BenchmarkAlive_WithMutexes` — Darwin: 35.06 ns/op, Linux: 34.82 ns/op (Linux faster)
- `BenchmarkAlive_WithMutexes_Contended` — Darwin: 37.50 ns/op, Linux: 34.78 ns/op (Linux faster)
- `BenchmarkAlive_WithMutexes_HighContention` — Darwin: 181.54 ns/op, Linux: 726.56 ns/op (Darwin faster)
- `BenchmarkAlive_WithTimer` — Darwin: 2.04 ns/op, Linux: 2.08 ns/op (Darwin faster)
- `BenchmarkAutoExit_AliveCheckCost_Disabled` — Darwin: 54.29 ns/op, Linux: 41.71 ns/op (Linux faster)
- `BenchmarkAutoExit_AliveCheckCost_Enabled` — Darwin: 49.86 ns/op, Linux: 44.49 ns/op (Linux faster)
- `BenchmarkCombinedWorkload_New` — Darwin: 86.95 ns/op, Linux: 88.36 ns/op (Darwin faster)
- `BenchmarkCombinedWorkload_Old` — Darwin: 210.84 ns/op, Linux: 215.20 ns/op (Darwin faster)
- `BenchmarkFastPathSubmit` — Darwin: 53.61 ns/op, Linux: 42.65 ns/op (Linux faster)
- `BenchmarkGetGoroutineID` — Darwin: 1760.60 ns/op, Linux: 1740.60 ns/op (Linux faster)
- `BenchmarkHighContention` — Darwin: 223.98 ns/op, Linux: 150.76 ns/op (Linux faster)
- `BenchmarkHighFrequencyMonitoring_New` — Darwin: 24.80 ns/op, Linux: 24.98 ns/op (Darwin faster)
- `BenchmarkIsLoopThread_False` — Darwin: 0.35 ns/op, Linux: 0.35 ns/op (Linux faster)
- `BenchmarkIsLoopThread_True` — Darwin: 4636.00 ns/op, Linux: 4493.40 ns/op (Linux faster)
- `BenchmarkLatencyChannelBufferedRoundTrip` — Darwin: 266.92 ns/op, Linux: 247.32 ns/op (Linux faster)
- `BenchmarkLatencyChannelRoundTrip` — Darwin: 373.50 ns/op, Linux: 237.84 ns/op (Linux faster)
- `BenchmarkLatencyDeferRecover` — Darwin: 2.45 ns/op, Linux: 2.46 ns/op (Darwin faster)
- `BenchmarkLatencyDirectCall` — Darwin: 0.31 ns/op, Linux: 0.31 ns/op (Darwin faster)
- `BenchmarkLatencyMutexLockUnlock` — Darwin: 7.91 ns/op, Linux: 8.00 ns/op (Darwin faster)
- `BenchmarkLatencyRWMutexRLockRUnlock` — Darwin: 8.16 ns/op, Linux: 8.15 ns/op (Linux faster)
- `BenchmarkLatencyRecord_WithPSquare` — Darwin: 77.36 ns/op, Linux: 78.31 ns/op (Darwin faster)
- `BenchmarkLatencyRecord_WithoutPSquare` — Darwin: 24.36 ns/op, Linux: 24.20 ns/op (Linux faster)
- `BenchmarkLatencySafeExecute` — Darwin: 3.07 ns/op, Linux: 3.08 ns/op (Darwin faster)
- `BenchmarkLatencySample_NewPSquare` — Darwin: 24.82 ns/op, Linux: 24.99 ns/op (Darwin faster)
- `BenchmarkLatencySimulatedPoll` — Darwin: 13.63 ns/op, Linux: 13.91 ns/op (Darwin faster)
- `BenchmarkLatencySimulatedSubmit` — Darwin: 14.03 ns/op, Linux: 16.45 ns/op (Darwin faster)
- `BenchmarkLatencyStateLoad` — Darwin: 0.31 ns/op, Linux: 0.31 ns/op (Darwin faster)
- `BenchmarkLatencyStateTryTransition` — Darwin: 4.09 ns/op, Linux: 4.12 ns/op (Darwin faster)
- `BenchmarkLatencyStateTryTransition_NoOp` — Darwin: 17.59 ns/op, Linux: 17.61 ns/op (Darwin faster)
- `BenchmarkLatencychunkedIngressPop` — Darwin: 4.50 ns/op, Linux: 5.66 ns/op (Darwin faster)
- `BenchmarkLatencychunkedIngressPush` — Darwin: 8.21 ns/op, Linux: 9.88 ns/op (Darwin faster)
- `BenchmarkLatencychunkedIngressPushPop` — Darwin: 4.06 ns/op, Linux: 4.06 ns/op (Darwin faster)
- `BenchmarkLatencychunkedIngressPush_WithContention` — Darwin: 76.86 ns/op, Linux: 38.77 ns/op (Linux faster)
- `BenchmarkLatencymicrotaskRingPop` — Darwin: 15.58 ns/op, Linux: 16.37 ns/op (Darwin faster)
- `BenchmarkLatencymicrotaskRingPush` — Darwin: 29.75 ns/op, Linux: 51.18 ns/op (Darwin faster)
- `BenchmarkLatencymicrotaskRingPushPop` — Darwin: 22.61 ns/op, Linux: 22.55 ns/op (Linux faster)
- `BenchmarkMetricsCollection` — Darwin: 38.16 ns/op, Linux: 43.07 ns/op (Darwin faster)
- `BenchmarkMicrotaskOverflow` — Darwin: 25.04 ns/op, Linux: 24.45 ns/op (Linux faster)
- `BenchmarkMicrotaskRingIsEmpty` — Darwin: 8.04 ns/op, Linux: 8.01 ns/op (Linux faster)
- `BenchmarkMicrotaskRingIsEmpty_WithItems` — Darwin: 2.03 ns/op, Linux: 1.97 ns/op (Linux faster)
- `BenchmarkMicrotaskSchedule` — Darwin: 86.24 ns/op, Linux: 78.02 ns/op (Linux faster)
- `BenchmarkMicrotaskSchedule_Parallel` — Darwin: 111.58 ns/op, Linux: 68.89 ns/op (Linux faster)
- `BenchmarkNoMetrics` — Darwin: 59.06 ns/op, Linux: 49.23 ns/op (Linux faster)
- `BenchmarkQueueMicrotask` — Darwin: 90.32 ns/op, Linux: 71.26 ns/op (Linux faster)
- `BenchmarkRefUnref_IsLoopThread_OnLoop` — Darwin: 10083.80 ns/op, Linux: 9997.80 ns/op (Linux faster)
- `BenchmarkRefUnref_RWMutex_External` — Darwin: 34.25 ns/op, Linux: 34.06 ns/op (Linux faster)
- `BenchmarkRefUnref_RWMutex_OnLoop` — Darwin: 34.25 ns/op, Linux: 34.60 ns/op (Darwin faster)
- `BenchmarkRefUnref_SyncMap_External` — Darwin: 25.83 ns/op, Linux: 25.36 ns/op (Linux faster)
- `BenchmarkRefUnref_SyncMap_OnLoop` — Darwin: 25.48 ns/op, Linux: 25.08 ns/op (Linux faster)
- `BenchmarkRegression_Combined_Atomic` — Darwin: 0.47 ns/op, Linux: 0.48 ns/op (Darwin faster)
- `BenchmarkRegression_Combined_Mutex` — Darwin: 17.85 ns/op, Linux: 17.96 ns/op (Darwin faster)
- `BenchmarkRegression_HasExternalTasks_Mutex` — Darwin: 9.70 ns/op, Linux: 9.64 ns/op (Linux faster)
- `BenchmarkRegression_HasInternalTasks_Mutex` — Darwin: 9.53 ns/op, Linux: 9.44 ns/op (Linux faster)
- `BenchmarkRegression_HasInternalTasks_SimulatedAtomic` — Darwin: 0.31 ns/op, Linux: 0.31 ns/op (Darwin faster)
- `BenchmarkSubmit` — Darwin: 53.32 ns/op, Linux: 42.57 ns/op (Linux faster)
- `BenchmarkSubmitInternal` — Darwin: 3574.20 ns/op, Linux: 2918.40 ns/op (Linux faster)
- `BenchmarkSubmitInternal_Cost` — Darwin: 3635.40 ns/op, Linux: 2983.80 ns/op (Linux faster)
- `BenchmarkSubmit_Parallel` — Darwin: 101.50 ns/op, Linux: 62.22 ns/op (Linux faster)
- `BenchmarkTask1_2_ConcurrentSubmissions` — Darwin: 97.98 ns/op, Linux: 62.88 ns/op (Linux faster)
- `BenchmarkTerminated_RejectionPath_RefTimer` — Darwin: 2.00 ns/op, Linux: 2.01 ns/op (Darwin faster)
- `BenchmarkTerminated_RejectionPath_submitToQueue` — Darwin: 9.68 ns/op, Linux: 9.79 ns/op (Darwin faster)
- `BenchmarkTerminated_UnrefTimer_NotGated` — Darwin: 2.01 ns/op, Linux: 2.01 ns/op (Darwin faster)
- `Benchmark_chunkedIngress_Batch` — Darwin: 526.28 ns/op, Linux: 525.38 ns/op (Linux faster)
- `Benchmark_chunkedIngress_ParallelWithSync` — Darwin: 84.27 ns/op, Linux: 44.26 ns/op (Linux faster)
- `Benchmark_chunkedIngress_Pop` — Darwin: 3.18 ns/op, Linux: 4.95 ns/op (Darwin faster)
- `Benchmark_chunkedIngress_Push` — Darwin: 8.38 ns/op, Linux: 9.13 ns/op (Darwin faster)
- `Benchmark_chunkedIngress_PushPop` — Darwin: 4.00 ns/op, Linux: 4.02 ns/op (Darwin faster)
- `Benchmark_chunkedIngress_Sequential` — Darwin: 4.10 ns/op, Linux: 4.11 ns/op (Darwin faster)
- `Benchmark_microtaskRing_Parallel` — Darwin: 121.12 ns/op, Linux: 110.73 ns/op (Linux faster)
- `Benchmark_microtaskRing_Push` — Darwin: 29.02 ns/op, Linux: 48.89 ns/op (Darwin faster)
- `Benchmark_microtaskRing_PushPop` — Darwin: 22.27 ns/op, Linux: 22.64 ns/op (Darwin faster)

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
| BenchmarkAlive_Epoch_ConcurrentSubmit | 0 | 3 | 3 |
| BenchmarkAlive_WithMutexes_HighContention | 0 | 3 | 3 |
| BenchmarkAutoExit_AliveCheckCost_Enabled | 0 | 0 | 0 |
| BenchmarkAutoExit_FastPathExit | 1,254,143 | 1,249,074 | 5,069 |
| BenchmarkAutoExit_ImmediateExit | 1,251,233 | 1,246,651 | 4,583 |
| BenchmarkAutoExit_PollPathExit | 1,253,536 | 1,248,620 | 4,916 |
| BenchmarkAutoExit_TimerFire | 1,253,361 | 1,248,598 | 4,764 |
| BenchmarkAutoExit_UnrefExit | 1,254,538 | 1,250,404 | 4,134 |
| BenchmarkCancelTimer_Individual/timers_1 | 2,001 | 2,002 | 1 |
| BenchmarkCancelTimer_Individual/timers_5 | 10,028 | 10,049 | 22 |
| BenchmarkCancelTimer_Individual/timers_: | 20,139 | 20,226 | 87 |
| BenchmarkCancelTimers_Batch/timers_1 | 824 | 824 | 0 |
| BenchmarkCancelTimers_Batch/timers_5 | 3,518 | 3,514 | 4 |
| BenchmarkCancelTimers_Batch/timers_: | 6,991 | 6,977 | 14 |
| BenchmarkCancelTimers_Comparison/Batch | 3,099 | 3,098 | 1 |
| BenchmarkCancelTimers_Comparison/Individual | 9,612 | 9,633 | 22 |
| BenchmarkHighContention | 3 | 44 | 41 |
| BenchmarkLatencymicrotaskRingPush | 43 | 44 | 1 |
| BenchmarkMetricsCollection | 42 | 42 | 0 |
| BenchmarkMicrotaskExecution | 18 | 59 | 41 |
| BenchmarkMicrotaskSchedule | 2 | 44 | 42 |
| BenchmarkMicrotaskSchedule_Parallel | 0 | 45 | 45 |
| BenchmarkMixedWorkload | 45 | 48 | 3 |
| BenchmarkPromiseAll | 1,240 | 1,241 | 1 |
| BenchmarkPromiseAll_Memory | 1,240 | 1,241 | 1 |
| BenchmarkPromiseChain | 477 | 484 | 6 |
| BenchmarkPromiseThenChain | 516 | 518 | 2 |
| BenchmarkPromisifyAllocation | 739 | 787 | 48 |
| BenchmarkQueueMicrotask | 4 | 43 | 39 |
| BenchmarkRefUnref_SubmitInternal_External | 64 | 66 | 2 |
| BenchmarkScheduleTimerWithPool | 72 | 34 | 38 |
| BenchmarkScheduleTimerWithPool_FireAndReuse | 33 | 32 | 1 |
| BenchmarkScheduleTimerWithPool_Immediate | 32 | 33 | 1 |
| BenchmarkSetInterval_Optimized | 297 | 296 | 1 |
| BenchmarkSetInterval_Parallel_Optimized | 415 | 341 | 73 |
| BenchmarkTask1_2_ConcurrentSubmissions | 0 | 0 | 0 |
| BenchmarkTerminated_RejectionPath_Promisify | 177 | 186 | 9 |
| BenchmarkTimerFire | 48 | 53 | 5 |
| BenchmarkTimerSchedule_Parallel | 192 | 193 | 1 |
| Benchmark_microtaskRing_Parallel | 44 | 43 | 1 |
| Benchmark_microtaskRing_Push | 44 | 45 | 2 |

## Measurement Stability

Coefficient of variation (CV%) indicates measurement consistency. Lower is better.

- Benchmarks with CV < 2% on both platforms: **63**
- Darwin benchmarks with CV > 5%: **29**
- Linux benchmarks with CV > 5%: **28**

### High-Variance Benchmarks (CV > 5%)

| Benchmark | Darwin CV% | Linux CV% |
|-----------|------------|-----------|
| BenchmarkAlive_AllAtomic | 9.5% (high) | 0.9% |
| BenchmarkAlive_Epoch_ConcurrentSubmit | 4.7% | 20.6% (high) |
| BenchmarkAlive_Epoch_FromCallback | 57.7% (high) | 0.4% |
| BenchmarkAlive_WithMutexes_Contended | 11.9% (high) | 0.1% |
| BenchmarkAlive_WithMutexes_HighContention | 5.6% (high) | 31.1% (high) |
| BenchmarkAutoExit_FastPathExit | 0.5% | 102.3% (high) |
| BenchmarkAutoExit_ImmediateExit | 7.0% (high) | 97.2% (high) |
| BenchmarkAutoExit_PollPathExit | 33.5% (high) | 7.2% (high) |
| BenchmarkAutoExit_TimerFire | 8.8% (high) | 3.9% |
| BenchmarkCancelTimers_Batch/timers_: | 0.4% | 14.7% (high) |
| BenchmarkLargeTimerHeap | 6.0% (high) | 1.5% |
| BenchmarkLatencyChannelBufferedRoundTrip | 5.2% (high) | 0.9% |
| BenchmarkLatencySimulatedPoll | 5.2% (high) | 4.2% |
| BenchmarkLatencySimulatedSubmit | 2.5% | 12.4% (high) |
| BenchmarkLatencychunkedIngressPop | 44.9% (high) | 37.0% (high) |
| BenchmarkLatencychunkedIngressPush | 4.9% | 9.6% (high) |
| BenchmarkLatencymicrotaskRingPush | 2.0% | 15.7% (high) |
| BenchmarkLoopDirectWithSubmit | 0.9% | 5.2% (high) |
| BenchmarkMetricsCollection | 9.8% (high) | 3.8% |
| BenchmarkMicrotaskExecution | 18.1% (high) | 7.6% (high) |
| BenchmarkMicrotaskRingIsEmpty_WithItems | 8.4% (high) | 0.4% |
| BenchmarkMicrotaskSchedule | 2.7% | 16.8% (high) |
| BenchmarkMicrotaskSchedule_Parallel | 0.3% | 6.5% (high) |
| BenchmarkNoMetrics | 13.8% (high) | 16.8% (high) |
| BenchmarkPromiseAll | 6.9% (high) | 1.7% |
| BenchmarkPromiseHandlerTracking_Optimized | 6.1% (high) | 1.7% |
| BenchmarkPromiseResolve | 6.4% (high) | 1.8% |
| BenchmarkPromiseTry | 0.4% | 6.0% (high) |
| BenchmarkQueueMicrotask | 5.9% (high) | 11.5% (high) |
| BenchmarkQuiescing_ScheduleTimer_WithAutoExit | 10.3% (high) | 1.1% |
| BenchmarkRefUnref_IsLoopThread_External | 0.3% | 22.3% (high) |
| BenchmarkRegression_FastPathWakeup_NoWork | 74.3% (high) | 146.0% (high) |
| BenchmarkScheduleTimerCancel | 3.2% | 5.9% (high) |
| BenchmarkSentinelDrain_NoWork | 51.0% (high) | 153.3% (high) |
| BenchmarkSentinelDrain_WithTimers | 12.4% (high) | 0.4% |
| BenchmarkSentinelIteration | 59.4% (high) | 151.4% (high) |
| BenchmarkSetInterval_Optimized | 10.7% (high) | 4.6% |
| BenchmarkSetTimeoutZeroDelay | 1.2% | 5.1% (high) |
| BenchmarkSetTimeout_Optimized | 17.4% (high) | 4.8% |
| BenchmarkSubmitInternal_QueuePath_OnLoop | 6.2% (high) | 5.3% (high) |
| BenchmarkTimerSchedule | 0.9% | 11.7% (high) |
| Benchmark_chunkedIngress_Pop | 4.3% | 35.6% (high) |
| Benchmark_chunkedIngress_Push | 5.2% (high) | 4.1% |
| Benchmark_microtaskRing_Parallel | 4.3% | 10.5% (high) |
| Benchmark_microtaskRing_Push | 5.6% (high) | 8.9% (high) |

## Key Findings

### 1. Architecture Parity

Both platforms run ARM64, eliminating architectural differences. Performance gaps
are attributable to:
- **OS kernel scheduling** (macOS Mach scheduler vs Linux CFS)
- **Memory management** (macOS memory pressure vs Linux cgroups in container)
- **Syscall overhead** differences
- **Go runtime behavior** variations between `darwin/arm64` and `linux/arm64`

### 2. Performance Distribution

- Darwin significantly faster (ratio < 0.9): **54** benchmarks
- Roughly equal (0.9–1.1x): **71** benchmarks
- Linux significantly faster (ratio > 1.1): **33** benchmarks

### 3. Timer Operations

- Total timer benchmarks: 27
- Darwin faster: 19
- Linux faster: 8
- Biggest difference: `BenchmarkCancelTimer_Individual/timers_:` — Linux is 2.06x slower

### 4. Concurrency & Contention

- `BenchmarkAlive_Epoch_ConcurrentSubmit`: Darwin (3.28x)
- `BenchmarkAlive_Epoch_NoContention`: Darwin (1.04x)
- `BenchmarkAlive_WithMutexes_HighContention`: Darwin (4.00x)
- `BenchmarkFastPathSubmit`: Linux (1.26x)
- `BenchmarkHighContention`: Linux (1.49x)
- `BenchmarkLatencyAnalysis_SubmitWhileRunning`: Linux (1.31x)
- `BenchmarkLatencySimulatedSubmit`: Darwin (1.17x)
- `BenchmarkLatencychunkedIngressPush_WithContention`: Linux (1.98x)
- `BenchmarkLoopDirectWithSubmit`: Darwin (2.77x)
- `BenchmarkMicrotaskSchedule_Parallel`: Linux (1.62x)
- `BenchmarkPromiseHandlerTracking_Parallel_Optimized`: Linux (2.22x)
- `BenchmarkRefUnref_SubmitInternal_External`: Darwin (1.22x)
- `BenchmarkRefUnref_SubmitInternal_OnLoop`: Linux (1.01x)
- `BenchmarkSetInterval_Parallel_Optimized`: Darwin (1.32x)
- `BenchmarkSubmit`: Linux (1.25x)
- `BenchmarkSubmitExecution`: Linux (1.22x)
- `BenchmarkSubmitInternal`: Linux (1.22x)
- `BenchmarkSubmitInternal_Cost`: Linux (1.22x)
- `BenchmarkSubmitInternal_FastPath_OnLoop`: Darwin (1.01x)
- `BenchmarkSubmitInternal_QueuePath_OnLoop`: Darwin (1.08x)
- `BenchmarkSubmitLatency`: Linux (1.38x)
- `BenchmarkSubmit_Parallel`: Linux (1.63x)
- `BenchmarkTask1_2_ConcurrentSubmissions`: Linux (1.56x)
- `BenchmarkTerminated_RejectionPath_submitToQueue`: Darwin (1.01x)
- `BenchmarkTimerSchedule_Parallel`: Darwin (1.38x)
- `Benchmark_chunkedIngress_ParallelWithSync`: Linux (1.90x)
- `Benchmark_microtaskRing_Parallel`: Linux (1.09x)

### 5. Summary

**Darwin wins overall** with 98/158 benchmarks faster.

The mean performance ratio of 0.970x (Darwin/Linux) indicates
the platforms are remarkably close in overall performance, with each
excelling in different workload categories.

