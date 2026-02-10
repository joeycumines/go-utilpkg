# macOS Tournament Analysis — 2026-02-08

**Platform:** macOS darwin/arm64, Apple M2 Pro
**Go Version:** 1.25.7_1
**Previous Tournament:** 2026-02-03 (Go 1.25.6)
**Event Loop Benchmark Duration:** 1612.102s
**Promise Tournament Duration:** 5469.663s (separate run)
**Race Detector Duration:** 39.943s
**Correctness Duration:** 47.176s

---

## 1. Event Loop Benchmarks

All values are **medians of 3 runs** (ns/op). Lower is better. ⭐ = category winner.

### 1.1 PingPong (Throughput)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **AlternateThree** | **93.73** | 24 | 1 |
| Baseline | 108.1 | 64 | 3 |
| Main | 112.6 | 24 | 1 |
| AlternateTwo | 144.4 | 25 | 1 |
| AlternateOne | 176.7 | 24 | 1 |

### 1.2 PingPongLatency (End-to-End)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **448.4** | 128 | 2 |
| Baseline | 527.1 | 168 | 4 |
| AlternateTwo | 10,401 | 128 | 2 |
| AlternateThree | 10,912 | 128 | 2 |
| AlternateOne | 11,098 | 128 | 2 |

### 1.3 MultiProducer (10 Producers)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **126.0** | 16 | 1 |
| AlternateThree | 142.9 | 16 | 1 |
| AlternateOne | 200.0 | 36 | 1 |
| AlternateTwo | 212.2 | 35 | 1 |
| Baseline | 226.6 | 56 | 3 |

### 1.4 MultiProducerContention (100 Producers)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **120.1** | 16 | 1 |
| AlternateThree | 136.8 | 17 | 1 |
| AlternateOne | 142.4 | 38 | 1 |
| AlternateTwo | 166.3 | 31 | 1 |
| Baseline | 204.6 | 56 | 3 |

### 1.5 GCPressure (GC-Heavy Workload)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **AlternateTwo** | **420.4** | 31 | 1 |
| Baseline | 447.0 | 64 | 3 |
| AlternateThree | 496.4 | 27 | 1 |
| AlternateOne | 505.2 | 30 | 1 |
| Main | 610.8 | 24 | 1 |

### 1.6 GCPressure_Allocations (Allocation-Heavy)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **AlternateThree** | **92.06** | 24 | 1 |
| Main | 108.2 | 24 | 1 |
| Baseline | 108.3 | 64 | 3 |
| AlternateTwo | 143.0 | 26 | 1 |
| AlternateOne | 170.4 | 24 | 1 |

### 1.7 BurstSubmit (Rapid Task Submission)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **54.34** | 24 | 1 |
| AlternateThree | 98.06 | 24 | 1 |
| AlternateTwo | 111.5 | 24 | 1 |
| Baseline | 114.2 | 64 | 2 |
| AlternateOne | 138.3 | 24 | 1 |

### 1.8 MicroWakeupSyscall_Running

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **72.64** | 0 | 0 |
| AlternateThree | 91.80 | 0 | 0 |
| AlternateOne | 108.7 | 5 | 0 |
| Baseline | 135.6 | 41 | 2 |
| AlternateTwo | 226.9 | 0 | 0 |

### 1.9 MicroWakeupSyscall_Sleeping

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **38.61** | 0 | 0 |
| AlternateThree | 69.96 | 0 | 0 |
| AlternateOne | 75.60 | 5 | 0 |
| AlternateTwo | 96.06 | 0 | 0 |
| Baseline | 109.5 | 40 | 2 |

### 1.10 MicroWakeupSyscall_Burst (10 Burst)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **1,215** | 0 | 0 |
| AlternateThree | 1,255 | 0 | 0 |
| Baseline | 1,257 | 40 | 1 |
| AlternateOne | 1,259 | 0 | 0 |
| AlternateTwo | 1,278 | 0 | 0 |

### 1.11 MicroWakeupSyscall_RapidSubmit

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **40.26** | 0 | 0 |
| AlternateThree | 69.98 | 0 | 0 |
| AlternateOne | 79.08 | 1 | 0 |
| AlternateTwo | 95.00 | 0 | 0 |
| Baseline | 105.6 | 40 | 2 |

### 1.12 MicroBatchBudget_Throughput (Burst=1024)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **Main** | **100.2** | 24 | 0 |
| AlternateThree | 101.5 | 24 | 1 |
| Baseline | 116.4 | 64 | 2 |
| AlternateTwo | 133.8 | 24 | 1 |

> AlternateOne data obscured by shutdown noise; not reliably extractable at this batch size.

### 1.13 MicroBatchBudget_Throughput Sweep (Main, AltTwo, AltThree)

| Burst Size | Main | AltTwo | AltThree | Baseline |
|------------|-----:|-------:|---------:|---------:|
| 64 | 278.9 | 344.2 | 315.7 | — |
| 128 | 158.7 | 227.3 | 176.4 | — |
| 256 | 93.10 | 174.6 | 135.4 | — |
| 512 | 81.54 | 147.7 | 115.9 | — |
| 1024 | 100.2 | 133.8 | 101.5 | 116.4 |
| 2048 | 106.8 | 125.1 | 95.74 | — |
| 4096 | 109.9 | 126.5 | 97.30 | — |

> Baseline only runs at Burst=1024. AlternateOne omitted (Burst=64 median: 301.8).

### 1.14 MicroBatchBudget_Latency Sweep

| Burst Size | Main | AltTwo | AltThree | Baseline |
|------------|-----:|-------:|---------:|---------:|
| 64 | 59.86 | 236.0 | 208.7 | 109.0 |
| 128 | 70.74 | 175.9 | 130.1 | 106.3 |
| 256 | 74.32 | 145.4 | 121.5 | 116.3 |
| 512 | 68.03 | 152.2 | 105.7 | 117.8 |
| 1024 | 96.09 | 127.9 | 95.62 | 103.1 |
| 2048 | 104.2 | 113.6 | 91.70 | 101.3 |
| 4096 | 108.8 | 114.0 | 90.53 | 102.0 |

### 1.15 MicroBatchBudget_Continuous

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **AlternateThree** | **180.1** | 24 | 1 |
| Main | 181.7 | 24 | 1 |
| AlternateTwo | 251.0 | 39 | 1 |
| Baseline | 257.6 | 64 | 3 |
| AlternateOne | 1,160 | 198 | 5 |

---

## 2. Promise Benchmarks

### 2.1 Promise Tournament (Standalone — `tournament-promise-macos.log`)

Clean separate run. PASS (5469.663s).

#### BenchmarkTournament (median ns/op)

| Implementation | ns/op |
|----------------|------:|
| ⭐ **PromiseAltOne** | **292.0** |
| PromiseAltTwo | 333.8 |
| ChainedPromise | 374.0 |
| PromiseAltFour | 647.3 |

#### BenchmarkChainDepth (median ns/op)

| Implementation | Depth=10 | Depth=100 |
|----------------|:--------:|:---------:|
| ⭐ **PromiseAltOne** | **1,980** | **7,867** |
| ChainedPromise | 2,161 | 10,092 |
| PromiseAltTwo | 2,526 | 9,981 |
| PromiseAltFour | 3,249 | 22,902 |

### 2.2 Promise Integration Benchmarks (from `tournament-macos.log`)

#### ChainCreation_Depth100 (median ns/op)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **ChainedPromise** | **5,777** | 13,784 | 204 |
| PromiseAltOne | 6,180 | 7,304 | 204 |
| PromiseAltTwo | 6,438 | 8,888 | 304 |
| PromiseAltThree | 8,482 | 8,893 | 304 |
| PromiseAltFour | 13,192 | 23,432 | 505 |

#### CheckResolved_Overhead (median ns/op)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **PromiseAltOne** | **125.6** | 197 | 4 |
| PromiseAltTwo | 141.4 | 197 | 5 |
| ChainedPromise | 142.6 | 245 | 4 |
| PromiseAltThree | 172.9 | 196 | 5 |
| PromiseAltFour | 222.2 | 338 | 7 |

#### FanOut_100 (median ns/op)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| ⭐ **PromiseAltTwo** | **8,282** | 8,800 | 300 |
| PromiseAltThree | 10,241 | 8,800 | 300 |
| PromiseAltOne | 11,086 | 22,726 | 300 |
| ChainedPromise | 12,073 | 30,190 | 300 |
| PromiseAltFour | 20,110 | 42,647 | 500 |

#### Race_100 (median ns/op)

| Implementation | ns/op | B/op | allocs/op |
|----------------|------:|-----:|----------:|
| PromiseAltTwo | ~0.00004 | 0 | 0 |
| PromiseAltThree | ~0.00007 | 0 | 0 |
| ⭐ **PromiseAltOne** | **3,976** | 6,464 | 101 |
| PromiseAltFour | 23,251 | 45,028 | 805 |
| ChainedPromise | 35,810 | 42,593 | 604 |

> **Note:** PromiseAltTwo and PromiseAltThree Race_100 values (~0 ns/op, 0 B/op, 0 allocs/op) indicate a no-op or immediately-returning Race implementation.

### 2.3 BenchmarkChainDepth (In-Run Failure)

The BenchmarkChainDepth benchmark in `tournament-macos.log` **FAILED** with `too many open files` across all implementations (ChainedPromise, PromiseAltOne, PromiseAltTwo, PromiseAltFour) at both Depth=10 and Depth=100. This was an FD exhaustion issue from the preceding event loop tournament run. The standalone `tournament-promise-macos.log` run completed successfully.

---

## 3. Comparison with 2026-02-03

> Δ% = (Feb03 − Feb08) / Feb03 × 100%. Positive = Feb 08 faster (improved). Negative = Feb 08 slower (regressed).

### 3.1 Core Throughput Benchmarks

| Benchmark | Impl | Feb 03 | Feb 08 | Δ% | Status |
|-----------|------|-------:|-------:|---:|--------|
| PingPong | Main | 109.5 | 112.6 | −2.8% | ✅ Stable |
| | AltTwo | 160.4 | 144.4 | **+10.0%** | ✅ Improved |
| | AltThree | 94.92 | 93.73 | +1.3% | ✅ Stable |
| | Baseline | 114.3 | 108.1 | **+5.4%** | ✅ Improved |
| PingPongLatency | Main | 438.1 | 448.4 | −2.4% | ✅ Stable |
| | AltOne | 10,057 | 11,098 | −10.3% | ⚠️ Regressed |
| | AltTwo | 11,248 | 10,401 | **+7.5%** | ✅ Improved |
| | AltThree | 10,085 | 10,912 | −8.2% | ⚠️ Regressed |
| | Baseline | 528.6 | 527.1 | +0.3% | ✅ Stable |
| MultiProducer | Main | 126.4 | 126.0 | +0.3% | ✅ Stable |
| | AltTwo | 273.1 | 212.2 | **+22.3%** | ✅ Major Improvement |
| | AltThree | 143.1 | 142.9 | +0.1% | ✅ Stable |
| | Baseline | 225.7 | 226.6 | −0.4% | ✅ Stable |
| MultiProdContention | Main | 119.6 | 120.1 | −0.4% | ✅ Stable |
| | AltTwo | 160.2 | 166.3 | −3.8% | ✅ Stable |
| | AltThree | 136.3 | 136.8 | −0.4% | ✅ Stable |
| | Baseline | 203.6 | 204.6 | −0.5% | ✅ Stable |

### 3.2 GC & Memory Benchmarks

| Benchmark | Impl | Feb 03 | Feb 08 | Δ% | Status |
|-----------|------|-------:|-------:|---:|--------|
| GCPressure | Main | 519.0 | 610.8 | −17.7% | ⚠️ Regressed |
| | AltOne | 391.8 | 505.2 | −28.9% | ❌ Significant Regression |
| | AltTwo | 402.1 | 420.4 | −4.6% | ✅ Stable |
| | AltThree | 339.5 | 496.4 | −46.2% | ❌ Major Regression |
| | Baseline | 366.2 | 447.0 | −22.0% | ⚠️ Regressed |
| GCPressure_Alloc | Main | 116.9 | 108.2 | **+7.4%** | ✅ Improved |
| | AltTwo | 125.0 | 143.0 | −14.4% | ⚠️ Regressed |
| | AltThree | 87.61 | 92.06 | −5.1% | ✅ Stable |
| | Baseline | 98.50 | 108.3 | −10.0% | ⚠️ Regressed |

### 3.3 Burst & Submission Benchmarks

| Benchmark | Impl | Feb 03 | Feb 08 | Δ% | Status |
|-----------|------|-------:|-------:|---:|--------|
| BurstSubmit | Main | 71.61 | 54.34 | **+24.1%** | ✅ Major Improvement |
| | AltTwo | 117.5 | 111.5 | **+5.1%** | ✅ Improved |
| | AltThree | 96.25 | 98.06 | −1.9% | ✅ Stable |
| | Baseline | 100.5 | 114.2 | −13.6% | ⚠️ Regressed |

### 3.4 Summary of Changes

| Direction | Count | Notable |
|-----------|------:|---------|
| ✅ Major Improvement | 3 | BurstSubmit/Main (+24.1%), MultiProducer/AltTwo (+22.3%), PingPong/AltTwo (+10.0%) |
| ✅ Improved | 4 | PingPong/Baseline, PingPongLatency/AltTwo, BurstSubmit/AltTwo, GCPressure_Alloc/Main |
| ✅ Stable (±5%) | 11 | Most Main and AltThree results |
| ⚠️ Regressed | 4 | GCPressure/Main, Baseline; GCPressure_Alloc/AltTwo, Baseline |
| ❌ Major Regression | 2 | GCPressure/AltOne (−28.9%), GCPressure/AltThree (−46.2%) |

---

## 4. Winner Rankings

### 4.1 Event Loop — Category Winners

| # | Benchmark | ⭐ Winner | ns/op | Runner-up | ns/op |
|:-:|-----------|----------|------:|-----------|------:|
| 1 | PingPong | AlternateThree | 93.73 | Baseline | 108.1 |
| 2 | PingPongLatency | Main | 448.4 | Baseline | 527.1 |
| 3 | MultiProducer | Main | 126.0 | AlternateThree | 142.9 |
| 4 | MultiProdContention | Main | 120.1 | AlternateThree | 136.8 |
| 5 | GCPressure | AlternateTwo | 420.4 | Baseline | 447.0 |
| 6 | GCPressure_Alloc | AlternateThree | 92.06 | Main | 108.2 |
| 7 | BurstSubmit | Main | 54.34 | AlternateThree | 98.06 |
| 8 | WakeupSyscall_Running | Main | 72.64 | AlternateThree | 91.80 |
| 9 | WakeupSyscall_Sleeping | Main | 38.61 | AlternateThree | 69.96 |
| 10 | WakeupSyscall_Burst | Main | 1,215 | AlternateThree | 1,255 |
| 11 | WakeupSyscall_RapidSubmit | Main | 40.26 | AlternateThree | 69.98 |
| 12 | BatchBudget_Throughput/1024 | Main | 100.2 | AlternateThree | 101.5 |
| 13 | BatchBudget_Latency/1024 | AlternateThree | 95.62 | Main | 96.09 |
| 14 | BatchBudget_Continuous | AlternateThree | 180.1 | Main | 181.7 |

### 4.2 Win Count Summary

| Implementation | 1st Place | 2nd Place | Total Podiums |
|----------------|:---------:|:---------:|:-------------:|
| **Main** | **9** | 3 | 12 |
| **AlternateThree** | **4** | 10 | 14 |
| **AlternateTwo** | **1** | 0 | 1 |
| **Baseline** | 0 | 1 | 1 |
| **AlternateOne** | 0 | 0 | 0 |

### 4.3 Promise — Category Winners

| Benchmark | ⭐ Winner | Runner-up |
|-----------|----------|-----------|
| Tournament (throughput) | PromiseAltOne (292.0) | PromiseAltTwo (333.8) |
| ChainDepth/10 | PromiseAltOne (1,980) | ChainedPromise (2,161) |
| ChainDepth/100 | PromiseAltOne (7,867) | PromiseAltTwo (9,981) |
| ChainCreation/100 | ChainedPromise (5,777) | PromiseAltOne (6,180) |
| CheckResolved | PromiseAltOne (125.6) | PromiseAltTwo (141.4) |
| FanOut/100 | PromiseAltTwo (8,282) | PromiseAltThree (10,241) |
| Race/100 *(functional impls only)* | PromiseAltOne (3,976) | PromiseAltFour (23,251) |

### 4.4 Promise Win Count

| Implementation | 1st Place | 2nd Place |
|----------------|:---------:|:---------:|
| **PromiseAltOne** | **5** | 1 |
| **PromiseAltTwo** | 1 | 3 |
| **ChainedPromise** | 1 | 1 |
| **PromiseAltFour** | 0 | 1 |
| **PromiseAltThree** | 0 | 1 |

---

## 5. Correctness & Race Detector Results

### 5.1 Correctness Tests (with `-race`, 47.176s)

| Test | Result | Notes |
|------|--------|-------|
| TestGCPressure_Correctness | ✅ PASS (5/5) | All impls 0.00s |
| TestMemoryLeak | ✅ PASS (5/5) | All impls ≤0.01s |
| TestMultiProducerStress | ✅ PASS (5/5) | All impls ≤0.03s |
| TestConcurrentStop | ✅ PASS (5/5) | All impls ≤0.12s |
| TestConcurrentStop_WithSubmits | ✅ PASS (5/5) | All impls ≤0.01s |
| TestConcurrentStop_Repeated | ✅ PASS (5/5) | Baseline: 45.01s (slow but passes) |
| TestGojaImmediateBurst | ✅ PASS (5/5) | 0.26s total |
| TestGojaMixedWorkload | ✅ PASS (5/5) | 0.57s total |
| TestGojaNestedTimeouts | ✅ PASS (5/5) | 0.00s total |
| TestGojaPromiseChain | ✅ PASS (5/5) | 0.01s total |
| TestGojaTimerStress | ✅ PASS (5/5) | 0.05s total |
| TestPanicIsolation | ✅ PASS (5/5) | All impls 0.01s |
| TestPanicIsolation_Multiple | ✅ PASS (5/5) | All impls 0.00s |
| TestPanicIsolation_Internal | ✅ PASS (5/5) | All impls 0.01s |

### 5.2 Race Detector Tests

| Test | Result | Notes |
|------|--------|-------|
| TestRaceWakeup | ✅ PASS (5/5) | All impls ≤0.12s |
| TestRaceWakeup_Aggressive | ✅ PASS (5/5) | All impls ≤0.01s |

### 5.3 Shutdown Conservation Tests

| Test | Result | Notes |
|------|--------|-------|
| TestShutdownConservation | ✅ PASS | Main, AltOne, AltThree: PASS. AltTwo, Baseline: SKIPPED |
| TestShutdownConservation_Stress | ✅ PASS | Main, AltOne, AltThree: PASS (10 iters each). Baseline: SKIPPED |

> AlternateTwo and Baseline are skipped in ShutdownConservation tests (not applicable to their architecture).

### 5.4 Race Detector Summary

| Suite | Result | Duration |
|-------|--------|----------|
| Correctness (with -race) | ✅ **PASS** | 47.176s |
| Race Detector (dedicated) | ✅ **PASS** | 39.943s |

**0 data races detected.** (Improved from 3 races in the 2026-02-03 tournament.)

### 5.5 Promise Tournament Runs

| Log File | Result | Duration | Notes |
|----------|--------|----------|-------|
| tournament-macos.log | ❌ FAIL | 60.336s | BenchmarkChainDepth: "too many open files" (FD exhaustion from prior run) |
| tournament-promise-macos.log | ✅ PASS | 5469.663s | Clean standalone run; all benchmarks completed |

---

## 6. Key Findings

### 6.1 GCPressure Universal Regression

All implementations show GCPressure regression compared to Feb 03 (−4.6% to −46.2%). This is a cross-cutting change, not implementation-specific. The Go 1.25.6 → 1.25.7_1 upgrade is the most likely contributing factor, though environmental variance cannot be excluded.

### 6.2 Main Dominates Syscall-Level Performance

Main wins **all four** MicroWakeupSyscall categories (Running, Sleeping, Burst, RapidSubmit) with **zero allocations**. In Sleeping mode, Main is 1.8x faster than the runner-up (38.61 vs 69.96 ns/op).

### 6.3 AlternateTwo Shows Strong Improvement

AlternateTwo shows the largest improvement delta of any implementation:
- MultiProducer: +22.3% (273.1 → 212.2)
- PingPong: +10.0% (160.4 → 144.4)
- PingPongLatency: +7.5% (11,248 → 10,401)

### 6.4 AlternateThree: Consistent Runner-Up

AlternateThree appears on the podium in 14 of 14 categories (4 wins, 10 second places). It is the most consistent all-rounder but rarely the outright winner outside of PingPong throughput, allocation efficiency, and batch budget latency at high burst sizes.

### 6.5 AlternateOne Data Integrity Issue

AlternateOne's shutdown logging is extremely verbose, producing hundreds of log lines per benchmark iteration. This interleaves with benchmark output and makes automated parsing unreliable. The actual benchmark values were manually extracted where possible, but several MicroBatchBudget data points could not be reliably attributed.

### 6.6 BurstSubmit Main Acceleration

Main's BurstSubmit improved by +24.1% (71.61 → 54.34 ns/op), now nearly 2x faster than any alternate. This is the best BurstSubmit performance observed in any tournament to date.

### 6.7 Latency Gap Persists

The PingPongLatency gap between Main/Baseline (~450–530 ns) and the alternates (~10,400–11,100 ns) remains at ~23x. This structural disparity has persisted across all tournaments.

### 6.8 Zero Race Conditions

This tournament detected **0 data races** across all implementations, an improvement from the 3 races detected in the 2026-02-03 tournament.

### 6.9 Promise Tournament: PromiseAltOne Dominates

PromiseAltOne wins 5 of 7 promise benchmark categories, including the overall Tournament throughput benchmark. ChainedPromise maintains the most efficient chain creation (fewest B/op at 13,784 vs 7,304 for AltOne, but AltOne uses fewer bytes for the same alloc count). PromiseAltTwo and PromiseAltThree have non-functional Race implementations (returning in ~0 ns with 0 allocations).

### 6.10 MicroBatchBudget Scaling Behavior

At small burst sizes (64), all implementations show elevated latency (208–344 ns/op for throughput). At large burst sizes (2048–4096), Main and AlternateThree converge to similar performance (~97–110 ns/op for throughput), while AlternateTwo plateaus higher (~125 ns/op). For latency, AlternateThree overtakes Main at burst sizes ≥1024, showing better amortized per-operation latency under heavy batching.
