# Linux Tournament Analysis — 2026-02-08

**Platform:** Linux (Docker `golang:1.25.7`), linux/arm64, 10 CPUs, epoll
**Go Version:** 1.25.7
**Previous Linux Tournament:** 2026-01-18 (Go 1.25.5, single-run methodology)
**Event Loop Benchmark Duration:** 1780.021s
**Promise Tournament Duration:** 6144.859s (inline in same run)
**Correctness Duration:** 52.783s
**Race Detector Duration:** 44.409s (FAIL — pre-fix AlternateOne shutdown conservation bug)

---

## 1. Event Loop Benchmarks

All values are **medians of 3 runs** (ns/op). Lower is better. ⭐ = category winner.

### 1.1 PingPong (Throughput)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **74.07** | 24 | 1 |
| Baseline | 108.1 | 64 | 3 |
| AlternateTwo | 162.9 | 25 | 1 |
| AlternateOne | 247.2 | 26 | 1 |
| AlternateThree | 510.7 | 24 | 1 |

### 1.2 PingPongLatency (End-to-End)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **423.3** | 128 | 2 |
| Baseline | 552.1 | 168 | 4 |
| AlternateOne | 41,362 | 128 | 2 |
| AlternateTwo | 41,575 | 128 | 2 |
| AlternateThree | 45,281 | 128 | 2 |

### 1.3 MultiProducer (10 Producers)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **93.87** | 16 | 1 |
| AlternateTwo | 173.4 | 50 | 1 |
| AlternateOne | 184.5 | 31 | 1 |
| Baseline | 187.4 | 56 | 3 |
| AlternateThree | 1,212 | 16 | 1 |

### 1.4 MultiProducerContention (100 Producers)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **AlternateTwo** | **126.6** | 31 | 1 |
| Main | 127.5 | 16 | 1 |
| AlternateOne | 178.2 | 38 | 1 |
| AlternateThree | 195.3 | 16 | 1 |
| Baseline | 215.5 | 56 | 3 |

### 1.5 GCPressure (GC-Heavy Workload)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **AlternateTwo** | **469.0** | 36 | 1 |
| AlternateThree | 753.7 | 27 | 1 |
| Baseline | 1,132 | 64 | 3 |
| Main | 1,465 | 24 | 1 |
| AlternateOne | 1,468 | 31 | 1 |

### 1.6 GCPressure_Allocations (Allocation-Heavy)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **67.39** | 24 | 1 |
| Baseline | 116.7 | 64 | 3 |
| AlternateTwo | 181.5 | 27 | 1 |
| AlternateOne | 292.6 | 25 | 1 |
| AlternateThree | 441.2 | 24 | 1 |

### 1.7 BurstSubmit (Rapid Task Submission)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **54.90** | 24 | 0 |
| Baseline | 114.6 | 63 | 2 |
| AlternateTwo | 179.1 | 24 | 1 |
| AlternateThree | 494.3 | 24 | 1 |
| AlternateOne | 1,338 | 24 | 0 |

### 1.8 MicroWakeupSyscall_Running

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **34.04** | 0 | 0 |
| Baseline | 71.88 | 40 | 2 |
| AlternateOne | 102.1 | 0 | 0 |
| AlternateTwo | 155.8 | 0 | 0 |
| AlternateThree | 985.1 | 0 | 0 |

### 1.9 MicroWakeupSyscall_Sleeping

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **30.32** | 0 | 0 |
| Baseline | 65.39 | 40 | 2 |
| AlternateOne | 76.18 | 1 | 0 |
| AlternateTwo | 84.62 | 0 | 0 |
| AlternateThree | 144.1 | 0 | 0 |

### 1.10 MicroWakeupSyscall_Burst (10 Burst)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Baseline** | **19,610** | 40 | 1 |
| AlternateTwo | 19,630 | 0 | 0 |
| Main | 19,658 | 0 | 0 |
| AlternateThree | 19,661 | 0 | 0 |
| AlternateOne | 19,672 | 0 | 0 |

> **Note:** All implementations are within 0.3% of each other (~19.6 µs). This benchmark is dominated by the epoll syscall cost, making all implementations effectively equal.

### 1.11 MicroWakeupSyscall_RapidSubmit

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **30.81** | 0 | 0 |
| Baseline | 63.52 | 40 | 2 |
| AlternateTwo | 86.70 | 0 | 0 |
| AlternateOne | 100.1 | 0 | 0 |
| AlternateThree | 156.6 | 0 | 0 |

### 1.12 MicroBatchBudget_Throughput (Burst=1024)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **90.89** | 24 | 0 |
| Baseline | 196.0 | 64 | 2 |
| AlternateTwo | 672.4 | 25 | 1 |
| AlternateThree | 1,275 | 24 | 1 |

> AlternateOne data interleaved with shutdown noise; not reliably extractable at this batch size.

### 1.13 MicroBatchBudget_Throughput Sweep (Main, AltTwo, AltThree)

| Burst Size | Main | AltTwo | AltThree | Baseline |
|------------|-----:|-------:|---------:|---------:|
| 64 | 4,182 | 17,160 | 11,879 | — |
| 128 | 6,436 | 5,462 | 8,742 | — |
| 256 | 2,113 | 2,442 | 3,516 | — |
| 512 | 210.7 | 1,306 | 2,161 | — |
| 1024 | 90.89 | 672.4 | 1,275 | 196.0 |
| 2048 | 70.34 | 260.6 | 854.3 | — |
| 4096 | 63.13 | 179.6 | 685.4 | — |

> Baseline only runs at Burst=1024. AlternateOne omitted (shutdown noise interleaving).

### 1.14 MicroBatchBudget_Latency Sweep

| Burst Size | Main | AltTwo | AltThree | Baseline |
|------------|-----:|-------:|---------:|---------:|
| 64 | 52.86 | 764.7 | 759.1 | 134.9 |
| 128 | 49.63 | 425.2 | 408.6 | 127.3 |
| 256 | 48.16 | 311.7 | 237.6 | 128.4 |
| 512 | 46.95 | 262.9 | 371.8 | 122.9 |
| 1024 | 47.81 | 193.0 | 350.3 | 119.9 |
| 2048 | 46.13 | 144.5 | 392.1 | 122.0 |
| 4096 | 45.97 | 154.6 | 401.1 | 116.2 |

### 1.15 MicroBatchBudget_Continuous

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **121.3** | 24 | 1 |
| Baseline | 188.2 | 64 | 3 |
| AlternateTwo | 226.8 | 38 | 1 |
| AlternateOne | 233.1 | 26 | 1 |
| AlternateThree | 1,436 | 24 | 1 |

---

## 2. Promise Benchmarks

### 2.1 Promise Tournament (Standalone — inline in `tournament-linux.log`)

All promise benchmarks ran within the same log file. PASS (6144.859s).

#### BenchmarkTournament (median ns/op)

| Implementation | ns/op |
|----------------|------:|
| ⭐ **PromiseAltOne** | **375.4** |
| PromiseAltTwo | 431.8 |
| ChainedPromise | 447.2 |
| PromiseAltFour | 755.8 |

#### BenchmarkChainDepth (median ns/op)

| Implementation | Depth=10 | Depth=100 |
|----------------|:--------:|:---------:|
| ⭐ **PromiseAltOne** | **2,820** | **16,858** |
| ChainedPromise | 3,469 | 20,452 |
| PromiseAltTwo | 3,473 | 19,833 |
| PromiseAltFour | 4,204 | 26,079 |

### 2.2 Promise Integration Benchmarks (from `tournament-linux.log`)

#### ChainCreation_Depth100 (median ns/op)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **PromiseAltOne** | **5,152** | 7,304 | 204 |
| PromiseAltTwo | 7,227 | 8,888 | 304 |
| ChainedPromise | 8,021 | 13,784 | 204 |
| PromiseAltThree | 9,351 | 8,892 | 304 |
| PromiseAltFour | 15,780 | 23,432 | 505 |

#### CheckResolved_Overhead (median ns/op)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **ChainedPromise** | **139.8** | 242 | 4 |
| PromiseAltOne | 141.2 | 198 | 4 |
| PromiseAltTwo | 167.1 | 199 | 5 |
| PromiseAltThree | 168.4 | 197 | 5 |
| PromiseAltFour | 230.4 | 344 | 7 |

#### FanOut_100 (median ns/op)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **PromiseAltTwo** | **9,220** | 8,800 | 300 |
| PromiseAltThree | 9,961 | 8,800 | 300 |
| ChainedPromise | 11,745 | 29,799 | 300 |
| PromiseAltOne | 12,187 | 22,714 | 300 |
| PromiseAltFour | 22,922 | 42,717 | 500 |

#### Race_100 (median ns/op)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| PromiseAltTwo | ~0.00004 | 0 | 0 |
| PromiseAltThree | ~0.00004 | 0 | 0 |
| ⭐ **PromiseAltOne** | **4,590** | 6,464 | 101 |
| PromiseAltFour | 28,425 | 45,028 | 805 |
| ChainedPromise | 38,885 | 42,073 | 604 |

> **Note:** PromiseAltTwo and PromiseAltThree Race_100 values (~0 ns/op, 0 B/op, 0 allocs/op) indicate a no-op or immediately-returning Race implementation.

---

## 3. Cross-Platform Comparison: macOS vs Linux (Feb 08)

Both runs used Go 1.25.7 and identical benchmarking methodology (3 runs, median of each).

### 3.1 Main Implementation — Key Benchmarks

| Benchmark | macOS (M2 Pro) | Linux (arm64) | Faster | Δ% |
|-----------|---------------:|--------------:|--------:|---:|
| PingPong | 112.6 | 74.07 | **Linux** | +34% |
| PingPongLatency | 448.4 | 423.3 | **Linux** | +6% |
| MultiProducer | 126.0 | 93.87 | **Linux** | +26% |
| MultiProdContention | 120.1 | 127.5 | macOS | −6% |
| GCPressure | 610.8 | 1,465 | macOS | −58% |
| GCPressure_Alloc | 108.2 | 67.39 | **Linux** | +38% |
| BurstSubmit | 54.34 | 54.90 | Tied | <1% |
| WakeupSyscall_Running | 72.64 | 34.04 | **Linux** | +53% |
| WakeupSyscall_Sleeping | 38.61 | 30.32 | **Linux** | +21% |
| WakeupSyscall_Burst | 1,215 | 19,658 | macOS | −94% |
| WakeupSyscall_RapidSubmit | 40.26 | 30.81 | **Linux** | +23% |
| BatchBudget_Continuous | 181.7 | 121.3 | **Linux** | +33% |

### 3.2 Cross-Platform Winner Map

| Benchmark | macOS Winner | Linux Winner | Same? |
|-----------|:-----------:|:-----------:|:-----:|
| PingPong | AlternateThree | **Main** | ❌ |
| PingPongLatency | Main | Main | ✅ |
| MultiProducer | Main | Main | ✅ |
| MultiProdContention | Main | **AlternateTwo** | ❌ |
| GCPressure | AlternateTwo | AlternateTwo | ✅ |
| GCPressure_Alloc | AlternateThree | **Main** | ❌ |
| BurstSubmit | Main | Main | ✅ |
| WakeupSyscall_Running | Main | Main | ✅ |
| WakeupSyscall_Sleeping | Main | Main | ✅ |
| WakeupSyscall_Burst | Main | **Baseline** | ❌ |
| WakeupSyscall_RapidSubmit | Main | Main | ✅ |
| BatchBudget_Continuous | AlternateThree | **Main** | ❌ |

8/12 categories have the same winner on both platforms. Main is more dominant on Linux (11 wins vs 9 wins on macOS).

### 3.3 Latency Gap: PingPongLatency

| Platform | Main | Baseline | Alt Two/Three | Gap (Main vs Alt) |
|----------|-----:|--------:|-------------:|------------------:|
| macOS | 448 ns | 527 ns | ~10,400–11,100 ns | ~23x |
| Linux | 423 ns | 552 ns | ~41,300–45,300 ns | ~100x |

The latency gap between Main/Baseline and the alternates is **~4x worse on Linux** (100x vs 23x). The alternates' polling-based wakeup mechanism has much higher latency under epoll than under kqueue.

### 3.4 WakeupSyscall_Burst Anomaly

The Burst benchmark shows a dramatic platform difference: **1,215 ns on macOS vs 19,658 ns on Linux** (16x slower). This reflects the minimum cost of an epoll_wait syscall (~19.6 µs) vs a kevent syscall (~1.2 µs). On Linux, this cost so dominates the benchmark that all five implementations converge to identical performance.

### 3.5 GCPressure Platform Divergence

| Implementation | macOS | Linux | Δ |
|----------------|------:|------:|---:|
| AlternateTwo | 420 | 469 | −12% |
| AlternateThree | 496 | 754 | −52% |
| Baseline | 447 | 1,132 | −153% |
| Main | 611 | 1,465 | −140% |
| AlternateOne | 505 | 1,468 | −191% |

GCPressure is significantly worse on Linux for all implementations. Main and AlternateOne are particularly affected, suggesting the GC dynamics under Docker/epoll impose heavier overhead than native macOS/kqueue.

---

## 4. Comparison with 2026-01-18 Linux Tournament

> **Methodology caveat:** The Jan 18 tournament used `benchtime=1s` (single run) on Go 1.25.5. The Feb 08 tournament used `count=3` (median of 3 runs) on Go 1.25.7. Direct comparison should be treated as indicative only.

### 4.1 Main Implementation Progress

| Benchmark | Jan 18 | Feb 08 | Δ% | Status |
|-----------|-------:|-------:|---:|--------|
| PingPong | 73.51 | 74.07 | −0.8% | ✅ Stable |
| PingPongLatency | 503.8 | 423.3 | **+16.0%** | ✅ Improved |
| MultiProducer | 126.6 | 93.87 | **+25.9%** | ✅ Major Improvement |
| MultiProdContention | 168.3 | 127.5 | **+24.3%** | ✅ Major Improvement |
| BurstSubmit | 72.16 | 54.90 | **+23.9%** | ✅ Major Improvement |
| WakeupSyscall_RapidSubmit | 26.36 | 30.81 | −16.9% | ⚠️ Regressed |

### 4.2 Baseline Progress

| Benchmark | Jan 18 | Feb 08 | Δ% | Status |
|-----------|-------:|-------:|---:|--------|
| PingPong | 144.2 | 108.1 | **+25.0%** | ✅ Major Improvement |
| PingPongLatency | 597.4 | 552.1 | **+7.6%** | ✅ Improved |
| MultiProducer | 194.7 | 187.4 | +3.8% | ✅ Stable |
| MultiProdContention | 230.0 | 215.5 | **+6.3%** | ✅ Improved |

### 4.3 Summary

Main shows strong improvement in 4 of 6 comparable benchmarks (up to +26%), with only WakeupSyscall_RapidSubmit showing meaningful regression. Baseline also improved across the board. These gains are likely from Go 1.25.5 → 1.25.7 runtime improvements combined with code-level optimizations.

---

## 5. Winner Rankings

### 5.1 Event Loop — Category Winners

| # | Benchmark | ⭐ Winner | ns/op | Runner-up | ns/op |
|:-:|-----------|----------|------:|-----------|------:|
| 1 | PingPong | Main | 74.07 | Baseline | 108.1 |
| 2 | PingPongLatency | Main | 423.3 | Baseline | 552.1 |
| 3 | MultiProducer | Main | 93.87 | AlternateTwo | 173.4 |
| 4 | MultiProdContention | AlternateTwo | 126.6 | Main | 127.5 |
| 5 | GCPressure | AlternateTwo | 469.0 | AlternateThree | 753.7 |
| 6 | GCPressure_Alloc | Main | 67.39 | Baseline | 116.7 |
| 7 | BurstSubmit | Main | 54.90 | Baseline | 114.6 |
| 8 | WakeupSyscall_Running | Main | 34.04 | Baseline | 71.88 |
| 9 | WakeupSyscall_Sleeping | Main | 30.32 | Baseline | 65.39 |
| 10 | WakeupSyscall_Burst | Baseline | 19,610 | Main | 19,658 |
| 11 | WakeupSyscall_RapidSubmit | Main | 30.81 | Baseline | 63.52 |
| 12 | BatchBudget_Throughput/1024 | Main | 90.89 | Baseline | 196.0 |
| 13 | BatchBudget_Latency/1024 | Main | 47.81 | Baseline | 119.9 |
| 14 | BatchBudget_Continuous | Main | 121.3 | Baseline | 188.2 |

### 5.2 Win Count Summary

| Implementation | 1st Place | 2nd Place | Total Podiums |
|----------------|:---------:|:---------:|:-------------:|
| **Main** | **11** | 2 | 13 |
| **Baseline** | 1 | 10 | 11 |
| **AlternateTwo** | 2 | 1 | 3 |
| **AlternateThree** | 0 | 1 | 1 |
| **AlternateOne** | 0 | 0 | 0 |

### 5.3 Promise — Category Winners

| Benchmark | ⭐ Winner | Runner-up |
|-----------|----------|-----------|
| Tournament (throughput) | PromiseAltOne (375.4) | PromiseAltTwo (431.8) |
| ChainDepth/10 | PromiseAltOne (2,820) | ChainedPromise (3,469) |
| ChainDepth/100 | PromiseAltOne (16,858) | PromiseAltTwo (19,833) |
| ChainCreation/100 | PromiseAltOne (5,152) | PromiseAltTwo (7,227) |
| CheckResolved | ChainedPromise (139.8) | PromiseAltOne (141.2) |
| FanOut/100 | PromiseAltTwo (9,220) | PromiseAltThree (9,961) |
| Race/100 *(functional impls only)* | PromiseAltOne (4,590) | PromiseAltFour (28,425) |

### 5.4 Promise Win Count

| Implementation | 1st Place | 2nd Place |
|----------------|:---------:|:---------:|
| **PromiseAltOne** | **5** | 1 |
| **PromiseAltTwo** | 1 | 2 |
| **ChainedPromise** | 1 | 1 |
| **PromiseAltFour** | 0 | 1 |
| **PromiseAltThree** | 0 | 1 |

---

## 6. Correctness & Race Detector Results

### 6.1 Correctness Tests (without -race, 52.783s)

| Test | Result | Notes |
|------|--------|-------|
| TestGCPressure_Correctness | ✅ PASS (5/5) | All impls 0.00s |
| TestMemoryLeak | ✅ PASS (5/5) | All impls ≤0.01s |
| TestMultiProducerStress | ✅ PASS (5/5) | All impls ≤0.03s |
| TestConcurrentStop | ✅ PASS (5/5) | All impls ≤0.12s |
| TestConcurrentStop_WithSubmits | ✅ PASS (5/5) | All impls ≤0.01s |
| TestConcurrentStop_Repeated | ✅ PASS (5/5) | Baseline: 50.09s (slow but passes) |
| TestGojaImmediateBurst | ✅ PASS (5/5) | All impls pass |
| TestGojaMixedWorkload | ✅ PASS (5/5) | All impls pass |
| TestGojaNestedTimeouts | ✅ PASS (5/5) | All impls pass |
| TestGojaPromiseChain | ✅ PASS (5/5) | All impls pass |
| TestGojaTimerStress | ✅ PASS (5/5) | All impls pass |
| TestPanicIsolation | ✅ PASS (5/5) | All impls 0.01s |
| TestPanicIsolation_Multiple | ✅ PASS (5/5) | All impls 0.00s |
| TestPanicIsolation_Internal | ✅ PASS (5/5) | All impls 0.01s |

### 6.2 Race Detector Tests

| Test | Result | Notes |
|------|--------|-------|
| TestRaceWakeup | ✅ PASS (5/5) | All impls ≤0.29s |
| TestRaceWakeup_Aggressive | ✅ PASS (5/5) | All impls ≤0.01s |

### 6.3 Shutdown Conservation Tests

#### Correctness Run (without -race)

| Test | Result | Notes |
|------|--------|-------|
| TestShutdownConservation | ✅ PASS | Main, AltOne, AltThree: PASS. AltTwo, Baseline: SKIPPED |
| TestShutdownConservation_Stress | ✅ PASS | Main, AltOne (10/10), AltThree (10/10): PASS. AltTwo: SKIPPED. Baseline: SKIPPED |

#### Race Detector Run (with -race) — ❌ FAIL

| Test | Result | Notes |
|------|--------|-------|
| TestShutdownConservation | ✅ PASS | Main, AltOne, AltThree: PASS. AltTwo, Baseline: SKIPPED |
| TestShutdownConservation_Stress | ❌ **FAIL** | AlternateOne lost tasks (see below) |

**ShutdownConservation_Stress Failures (Race Detector Run):**

| Sub-test | Submitted | Executed | Lost |
|----------|----------:|---------:|-----:|
| AlternateOne/Iteration | 6,222 | 6,221 | 1 |
| AlternateOne/Iteration#03 | 5,426 | 5,425 | 1 |
| AlternateOne/Iteration#09 | 6,659 | 6,656 | 3 |

> **Known issue (pre-fix):** This tournament was run BEFORE the fix that atomically closes ingress during drain. The AlternateOne implementation could lose 1–3 tasks during concurrent shutdown under the race detector. This has since been fixed. Main and AlternateThree passed all 10 iterations.

### 6.4 Race Detector Summary

| Suite | Result | Duration |
|-------|--------|----------|
| Correctness (without -race) | ✅ **PASS** | 52.783s |
| Race Detector (with -race) | ❌ **FAIL** | 44.409s |

**0 data races detected.** The FAIL is from a conservation violation (task loss), not a data race report. The race detector did not flag any concurrent access violations.

### 6.5 Promise Tournament Run

| Log File | Result | Duration | Notes |
|----------|--------|----------|-------|
| tournament-linux.log | ✅ PASS | 6,144.859s | All benchmarks completed successfully (inline run) |

---

## 7. Key Findings

### 7.1 Main Dominates Linux Even More Than macOS

Main wins **11 of 14** event loop categories on Linux (vs 9/14 on macOS). On Linux, Main is the clear overall winner with no close challengers. Baseline takes second place but trails Main by 30–60% in most categories.

### 7.2 AlternateThree Collapse on Linux

AlternateThree, a strong performer on macOS (4 wins, 10 second places), shows severe degradation on Linux:
- **PingPong**: 93.73 ns (macOS, 1st) → 510.7 ns (Linux, 5th) — 5.4x slower
- **MultiProducer**: 142.9 ns (macOS, 2nd) → 1,212 ns (Linux, 5th) — 8.5x slower
- **MicroWakeupSyscall_Running**: 91.80 ns (macOS, 2nd) → 985.1 ns (Linux, 5th) — 10.7x slower
- **BatchBudget_Continuous**: 180.1 ns (macOS, 1st) → 1,436 ns (Linux, 5th) — 8.0x slower

AlternateThree's architecture appears optimized for kqueue (macOS) and performs poorly under epoll (Linux).

### 7.3 epoll vs kqueue: MicroWakeupSyscall_Burst

The Burst benchmark exposes a fundamental platform difference:
- **macOS (kqueue)**: ~1,215 ns — kevent batches I/O notifications efficiently
- **Linux (epoll)**: ~19,660 ns — epoll_wait has higher minimum latency

This 16x difference causes all implementations to converge to identical performance on Linux (~19.6 µs), as the syscall cost completely dominates.

### 7.4 PingPongLatency Gap Is Extreme on Linux

The alternates' PingPongLatency is ~100x worse than Main on Linux (41,000–45,000 ns vs 423 ns), compared to ~23x on macOS. The epoll-based polling mechanism adds significantly more latency per wakeup cycle than kqueue.

### 7.5 AlternateTwo: Consistent GC Champion

AlternateTwo wins GCPressure on both platforms (420 ns macOS, 469 ns Linux). It also wins MultiProducerContention on Linux (126.6 ns), marginally beating Main (127.5 ns). AlternateTwo's architecture handles GC pressure and contention well across platforms.

### 7.6 Main's Syscall-Level Dominance

Main wins all four MicroWakeupSyscall categories (Running, Sleeping, Burst*, RapidSubmit) with **zero allocations** and sub-35 ns performance in the non-burst modes. On Linux, Main's wakeup path is 2.1–4.6x faster than Baseline, and 2.5–29x faster than the alternates.

*Burst: technically Baseline wins by 48 ns (0.2%), but all implementations are within noise.

### 7.7 AlternateOne Data Integrity & Conservation Issues

AlternateOne produces hundreds of shutdown log lines per benchmark iteration, interleaving with benchmark output. While most benchmark values could be manually extracted, MicroBatchBudget sweep data is unreliably parseable.

Additionally, AlternateOne failed ShutdownConservation_Stress under the race detector, losing 1–3 tasks across 3 of 10 iterations. This was a known pre-fix bug.

### 7.8 Linux Shows Stronger Main Performance Than macOS

Main is faster on Linux in 8 of 12 cross-platform comparisons. Key Linux advantages:
- **MicroWakeupSyscall_Running**: 34 ns vs 73 ns (53% faster)
- **GCPressure_Allocations**: 67 ns vs 108 ns (38% faster)
- **PingPong**: 74 ns vs 113 ns (34% faster)
- **MultiProducer**: 94 ns vs 126 ns (26% faster)

The Linux ARM64 environment (Docker) provides lower overhead for Main's hot paths.

### 7.9 Promise Tournament: PromiseAltOne Dominates (Consistent Across Platforms)

PromiseAltOne wins 5 of 7 promise categories on both macOS and Linux. Results are remarkably consistent:
- Tournament throughput: 292 ns (macOS) vs 375 ns (Linux)
- ChainDepth/100: 7,867 ns (macOS) vs 16,858 ns (Linux)

Linux is generally ~1.5–2x slower for promise operations, likely due to Docker overhead and ARM64 scheduling differences.

### 7.10 MicroBatchBudget Scaling: Extreme Throughput Gap on Linux

At Burst=4096, Main achieves 63 ns/op throughput while AlternateThree is at 685 ns/op (10.8x slower). On macOS, the gap at the same burst size was only 1.1x (110 ns vs 97 ns). Linux amplifies the throughput advantage of Main's architecture at high batch sizes.

For latency, Main maintains remarkably stable ~46–53 ns across all burst sizes, while AlternateThree shows *inverse scaling* on Linux — latency **increases** from 759 ns (Burst=64) to only 401 ns (Burst=4096), and actually *worsens* again above Burst=512. This suggests an architectural issue with AlternateThree's batching under epoll.

---

*Report generated: 2026-02-08*
*Platform: Docker golang:1.25.7 (Linux arm64, epoll)*
*Source: tournament-linux.log*
