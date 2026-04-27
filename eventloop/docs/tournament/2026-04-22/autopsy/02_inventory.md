# 02 — Inventory

## Dataset Inventory by Platform

| Platform JSON | Benchmark Count | Architecture | GOMAXPROCS | Environment |
|---|---:|---|--:|---|
| `darwin.json` | 158 | ARM64 | 10 | Native macOS (Apple Silicon) |
| `linux.json` | 158 | ARM64 | 10 | Docker container (golang:1.26.1) |
| `windows.json` | 158 | AMD64 | 16 | WSL/moo (Intel i9-9900K) |

**Total unique benchmarks across all platforms**: 158

**Complete 3-platform overlap**: 158 (100% of each platform's benchmarks are shared)

This is a dramatic improvement over 2026-04-19, which had only 25 shared benchmarks across all 3 platforms.

## Family Coverage Matrix

Family = benchmark name prefix after `Benchmark` and before first `_` (with `_prefixed_internal` for names like `Benchmark_chunkedIngress_*`).

| Family | Count | Notes |
|--------|------:|-------|
| Alive | 15 | Includes new Alive_Epoch variants |
| AutoExit | 7 | Full family coverage |
| CancelTimer | 6 | New family (timers_1, timers_5, timers_, Individual, Batch, Comparison) |
| CancelTimers_Batch | 3 | timers variants |
| CancelTimers_Comparison | 2 | Individual, Batch |
| ChannelWithMutexQueue | 1 | New benchmark |
| CombinedWorkload | 2 | Old and New variants |
| FastPathExecution | 1 | |
| FastPathSubmit | 1 | |
| GetGoroutineID | 1 | |
| GojaStyleSwap | 1 | |
| HighContention | 1 | |
| HighFrequencyMonitoring | 2 | Old, New |
| IsLoopThread | 2 | True, False |
| LargeTimerHeap | 1 | |
| LatencyAnalysis | 4 | PingPong, SubmitWhileRunning, EndToEnd, +1 |
| LatencyChannel | 2 | BufferedRoundTrip, RoundTrip |
| LatencychunkedIngress | 4 | Pop, Push, PushPop, Push_WithContention |
| LatencymicrotaskRing | 1 | Pop |
| LatencyDeferRecover | 1 | |
| LatencyDirectCall | 1 | |
| LatencyMutexLockUnlock | 1 | |
| LatencyRWMutexRLockRUnlock | 1 | |
| LatencyRecord | 2 | WithPSquare, WithoutPSquare |
| LatencySafeExecute | 1 | |
| LatencySample | 2 | NewPSquare, OldSortBased |
| LatencySimulatedPoll | 1 | |
| LatencySimulatedSubmit | 1 | |
| LatencyStateLoad | 1 | |
| LatencyStateTryTransition | 2 | (NoOp variant) |
| LatencychunkedIngressPush | 1 | |
| LoopDirect | 2 | WithSubmit variant |
| MetricsCollection | 1 | |
| MicroPingPong | 2 | WithCount variant |
| MicrotaskExecution | 1 | |
| MicrotaskLatency | 1 | |
| MicrotaskOverflow | 1 | |
| MicrotaskRingIsEmpty | 2 | WithItems variant |
| MicrotaskSchedule | 2 | Parallel variant |
| MinimalLoop | 1 | |
| MixedWorkload | 1 | |
| NoMetrics | 1 | |
| PromiseAll | 2 | Memory variant |
| PromiseChain | 1 | |
| PromiseCreate | 1 | |
| PromiseCreation | 1 | |
| PromiseGC | 1 | |
| PromiseHandlerTracking | 2 | Optimized, Parallel_Optimized |
| PromiseRace | 1 | |
| PromiseReject | 1 | |
| PromiseRejection | 1 | |
| PromiseResolution | 1 | |
| PromiseResolve | 2 | Memory variant |
| PromiseThen | 1 | |
| PromiseThenChain | 1 | |
| PromiseTry | 1 | |
| PromiseWithResolvers | 1 | |
| PromisifyAllocation | 1 | |
| PureChannelPingPong | 1 | |
| QueueMicrotask | 1 | |
| Quiescing | 2 | ScheduleTimer variants |
| RefUnref | 8 | Full family |
| Regression | 6 | Combined variants (Atomic, Mutex), HasInternalTasks, HasExternalTasks, FastPathWakeup |
| SentinelDrain | 2 | NoWork, WithTimers |
| SentinelIteration | 2 | WithTimers variant |
| SetImmediate | 1 | Optimized |
| SetInterval | 2 | Optimized, Parallel_Optimized |
| SetTimeout | 2 | Optimized, ZeroDelay |
| Submit | 2 | Parallel variant |
| SubmitExecution | 1 | |
| SubmitInternal | 4 | Cost, FastPath_OnLoop, QueuePath_OnLoop |
| SubmitLatency | 1 | |
| Task1_2_ConcurrentSubmissions | 1 | |
| Terminated | 5 | RejectionPath variants, UnrefTimer |
| TimerFire | 1 | |
| TimerHeapOperations | 1 | |
| TimerLatency | 1 | |
| TimerSchedule | 2 | Parallel variant |
| WakeUpDeduplicationIntegration | 1 | |
| _chunkedIngress | 5 | Batch, ParallelWithSync, Pop, Push, PushPop, Sequential |
| _microtaskRing | 3 | Parallel, Push, PushPop |

## New Benchmarks in 2026-04-22 (Not in 2026-04-19)

The following benchmarks are present in 2026-04-22 but absent from 2026-04-19 data:

### CancelTimer Family
- `BenchmarkCancelTimer_Individual/timers_1`
- `BenchmarkCancelTimer_Individual/timers_5`
- `BenchmarkCancelTimer_Individual/timers_:` (unbounded)
- `BenchmarkCancelTimers_Batch/timers_1`
- `BenchmarkCancelTimers_Batch/timers_5`
- `BenchmarkCancelTimers_Batch/timers_:` (unbounded)
- `BenchmarkCancelTimers_Comparison/Individual`
- `BenchmarkCancelTimers_Comparison/Batch`

### Other New Entries
- `BenchmarkChannelWithMutexQueue`
- `BenchmarkCombinedWorkload_New`
- `BenchmarkAlive_Epoch_ConcurrentSubmit` (Windows)
- `BenchmarkAlive_Epoch_FromCallback` (Windows)
- `BenchmarkAlive_Epoch_NoContention` (Windows)

## Win Rate Distribution

### 2-Platform (Darwin vs Linux, 158 benchmarks)

| Metric | Value |
|--------|-------|
| Darwin wins | 99 (62.7%) |
| Linux wins | 59 (37.3%) |
| Ties | 0 |
| Statistically significant | 110 (69.6%) |

### 3-Platform (All 3, 158 benchmarks)

| Platform | Wins | Percentage |
|----------|-----:|-----------:|
| Darwin | 70 | 44.3% |
| Linux | 47 | 29.7% |
| Windows | 41 | 25.9% |

## Allocation Consistency

**Allocs/op match across all platforms**: 148/158 benchmarks

**Allocation mismatches**: 10 benchmarks (AutoExit family and some timer benchmarks)

Total allocations by platform:
- Darwin: 222,075,706
- Linux: 210,113,428
- Windows: 214,141,642

The ~5-6% variation in total allocations across platforms on the same benchmark set suggests platform-specific runtime behavior in object allocation/reclamation patterns.

## Zero-Allocation Benchmarks

77 benchmarks achieve zero allocations on all platforms — these are the gold standard for hot-path performance as they represent no GC pressure:

Core zero-allocation families:
- All `Alive_*` variants (zero allocations)
- All `IsLoopThread_*` variants (zero allocations)
- All `RefUnref_*` variants (zero allocations)
- All `Regression_*` variants (zero allocations)
- Most `Latency*` primitives (zero allocations)

## Benchmark Count Comparison vs 2026-04-19

| Metric | 2026-04-19 | 2026-04-22 | Delta |
|--------|------------|------------|-------|
| Darwin benchmarks | 96 | 158 | +62 |
| Linux benchmarks | 45 | 158 | +113 |
| Windows benchmarks | 25 | 158 | +133 |
| 3-platform shared | 25 | 158 | +133 |
| Pipeline status | Broken | Operational | Fixed |
| JSON reproducibility | Failed | Verified | Fixed |

## Inventory Finding

**Status**: The 2026-04-22 tournament achieves full benchmark parity across all 3 platforms (158 each) — a complete resolution of the coverage skew problem that plagued 2026-04-19.

**Production impact**: Cross-platform conclusions are now based on the complete benchmark surface, not a narrow subset. The prior caveat "timer/promise/latency findings are not cross-platform validated" no longer applies to the same degree.

**Remaining concerns**:
1. GOMAXPROCS differs (Windows=16, Darwin/Linux=10) — potential confound
2. Architecture differs for Windows (AMD64 vs ARM64)
3. Linux runs in Docker container vs Darwin native
4. High variance on some Linux benchmarks (CV > 100% on AutoExit family)
