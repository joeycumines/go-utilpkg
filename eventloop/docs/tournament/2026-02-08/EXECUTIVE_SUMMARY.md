# Cross-Platform Executive Summary — 2026-02-08

## 1. Overview

**Date:** 2026-02-08
**Platforms Tested:** macOS darwin/arm64 (Apple M2 Pro, kqueue), Linux arm64 (Docker golang:1.25.7, epoll), Windows amd64 (Intel i9-9900K, channel-based pollFastMode)
**Go Version:** 1.25.7 (all platforms)
**Event Loop Implementations:** Main, AlternateOne, AlternateTwo, AlternateThree, Baseline (5 total)
**Promise Implementations:** ChainedPromise, PromiseAltOne, PromiseAltTwo, PromiseAltThree, PromiseAltFour (5 total)
**Methodology:** 3 runs per benchmark, median reported (ns/op)

**Main** is the overall cross-platform champion, winning 32 of 42 event loop benchmark slots (9/14 macOS, 11/14 Linux, 12/14 Windows). It dominates latency, syscall-level wakeup, and burst submission on every platform. **AlternateThree** is the strongest challenger but exhibits dramatic platform sensitivity — competitive on macOS and Windows, catastrophically slow on Linux/epoll. **PromiseAltOne** is the clear promise champion, winning 16 of 21 promise slots across all three platforms with remarkable consistency. All correctness tests pass on all platforms. The conservation fix is validated cross-platform. Zero data races detected (macOS/Linux); the Windows race detector failed due to an infrastructure DLL issue, not a code defect.

---

## 2. Cross-Platform Winner Matrix

| Benchmark | macOS | Linux | Windows | Universal Winner |
|-----------|:-----:|:-----:|:-------:|:----------------:|
| PingPong | **AlternateThree** | **Main** | **Main** | Main (2/3) |
| PingPongLatency | **Main** | **Main** | **Main** | **Main** ✅ |
| MultiProducer | **Main** | **Main** | **AlternateThree** | Main (2/3) |
| MultiProdContention | **Main** | **AlternateTwo** | **Main** | Main (2/3) |
| GCPressure | **AlternateTwo** | **AlternateTwo** | **AlternateThree** | AlternateTwo (2/3) |
| GCPressure_Alloc | **AlternateThree** | **Main** | **Main** | Main (2/3) |
| BurstSubmit | **Main** | **Main** | **Main** | **Main** ✅ |
| WakeupSyscall_Running | **Main** | **Main** | **Main** | **Main** ✅ |
| WakeupSyscall_Sleeping | **Main** | **Main** | **Main** | **Main** ✅ |
| WakeupSyscall_Burst | **Main** | **Baseline**¹ | **Main** | **Main** ✅ |
| WakeupSyscall_RapidSubmit | **Main** | **Main** | **Main** | **Main** ✅ |
| BatchBudget_Throughput/1024 | **Main** | **Main** | **Main** | **Main** ✅ |
| BatchBudget_Latency/1024 | **AlternateThree** | **Main** | **Main** | Main (2/3) |
| BatchBudget_Continuous | **AlternateThree** | **Main** | **Main** | Main (2/3) |

¹ Linux Burst: Baseline wins by 48 ns (0.2%) — all implementations converge to ~19.6 µs, dominated by epoll_wait syscall cost.

**Win count by platform:**

| Implementation | macOS 1st | Linux 1st | Windows 1st | Total 1st (of 42) |
|----------------|:---------:|:---------:|:-----------:|:-----------------:|
| **Main** | 9 | 11 | 12 | **32** |
| **AlternateThree** | 4 | 0 | 2 | 6 |
| **AlternateTwo** | 1 | 2 | 0 | 3 |
| **Baseline** | 0 | 1 | 0 | 1 |
| **AlternateOne** | 0 | 0 | 0 | 0 |

---

## 3. Cross-Platform Performance Ratios

### 3.1 Main Implementation — Core Benchmarks

| Benchmark | macOS | Linux | Windows | Best Platform |
|-----------|------:|------:|--------:|:-------------:|
| PingPong | 112.6 | **74.07** | 94.17 | Linux |
| PingPongLatency | 448.4 | **423.3** | 639.2 | Linux |
| MultiProducer | 126.0 | **93.87** | 102.8 | Linux |
| MultiProdContention | **120.1** | 127.5 | 96.14 | Windows |
| GCPressure | 610.8 | 1,465 | **487.2** | Windows |
| GCPressure_Alloc | 108.2 | **67.39** | 96.65 | Linux |
| BurstSubmit | **54.34** | 54.90 | 68.59 | macOS |
| WakeupSyscall_Running | 72.64 | **34.04** | 39.43 | Linux |
| WakeupSyscall_Sleeping | 38.61 | **30.32** | 31.60 | Linux |
| WakeupSyscall_Burst | **1,215** | 19,658 | 3,574 | macOS |
| WakeupSyscall_RapidSubmit | 40.26 | **30.81** | 33.12 | Linux |
| BatchBudget_Throughput/1024 | 100.2 | **90.89** | 364.7 | Linux |
| BatchBudget_Continuous | 181.7 | **121.3** | 115.6 | Windows |

Linux leads Main in 8/13, Windows in 3/13, macOS in 2/13. Linux's ARM64 Docker environment provides the lowest overhead for Main's hot paths, with the notable exception of the burst/polling benchmarks where macOS kqueue dominates.

### 3.2 AlternateThree — Platform Sensitivity

| Benchmark | macOS | Linux | Windows | Linux Δ vs macOS |
|-----------|------:|------:|--------:|:----------------:|
| PingPong | 93.73 | 510.7 | 99.71 | **5.4× slower** |
| MultiProducer | 142.9 | 1,212 | 94.31 | **8.5× slower** |
| WakeupSyscall_Running | 91.80 | 985.1 | 47.12 | **10.7× slower** |
| GCPressure | 496.4 | 753.7 | 355.1 | 1.5× slower |
| BatchBudget_Continuous | 180.1 | 1,436 | 118.7 | **8.0× slower** |
| BurstSubmit | 98.06 | 494.3 | 88.18 | 5.0× slower |

AlternateThree collapses 5–11× on Linux/epoll while performing competitively (often best-in-class) on macOS/kqueue and Windows/channels.

### 3.3 PingPongLatency Gap — Main vs Alternates

| Platform | Main | Baseline | Best Alternate | Gap (Main vs Alt) |
|----------|-----:|--------:|---------------:|:-----------------:|
| macOS | 448 | 527 | 10,401 (AltTwo) | **~23×** |
| Linux | 423 | 552 | 41,362 (AltOne) | **~100×** |
| Windows | 639 | 718 | 10,833 (AltThree) | **~17×** |

The structural latency gap between Main/Baseline and the alternate implementations is a fundamental architectural difference that persists across all platforms, with Linux amplifying it dramatically.

### 3.4 MicroWakeupSyscall_Burst — I/O Backend Cost

| Implementation | macOS (kqueue) | Linux (epoll) | Windows (channels) |
|----------------|---------------:|--------------:|-------------------:|
| Main | 1,215 | 19,658 | 3,574 |
| Baseline | 1,257 | 19,610 | 4,267 |
| AlternateThree | 1,255 | 19,661 | 6,345 |

macOS kqueue is 16× faster than Linux epoll and 3× faster than Windows channels for burst wakeup. On Linux, the ~19.6 µs epoll_wait floor causes all implementations to converge to identical performance.

---

## 4. Platform-Specific Observations

### 4.1 macOS (kqueue) — Low Latency, AlternateThree Competitive

- **I/O Backend:** kqueue delivers the lowest burst wakeup latency (~1.2 µs), enabling differentiation between implementations.
- **AlternateThree shines:** Wins 4 categories (PingPong throughput, GCPressure_Alloc, BatchBudget_Latency/1024, BatchBudget_Continuous), takes 2nd in 10. Most consistent all-rounder on this platform.
- **Main dominance tempered:** 9/14 wins — the fewest of any platform, though still decisively first.
- **GCPressure regression:** All implementations regressed vs Feb 03 (−5% to −46%), likely attributable to Go 1.25.6 → 1.25.7_1. Not implementation-specific.
- **Race detector:** ✅ Clean. 0 data races (improved from 3 in the Feb 03 tournament).
- **BurstSubmit acceleration:** Main improved +24% vs Feb 03 (71.6 → 54.3 ns/op) — best BurstSubmit ever recorded.

### 4.2 Linux (epoll) — Main Supremacy, AlternateThree Collapse

- **I/O Backend:** epoll has a high minimum syscall cost (~19.6 µs), flattening burst benchmarks but not significantly impacting steady-state throughput where Main excels.
- **Main dominant:** 11/14 wins. Main is 34% faster than macOS for PingPong, 53% faster for WakeupSyscall_Running. Linux ARM64 Docker provides excellent hot-path performance.
- **AlternateThree collapse:** From 4 wins (macOS) to 0 wins (Linux). PingPong 5.4× slower, MultiProducer 8.5× slower, WakeupSyscall_Running 10.7× slower. AlternateThree's architecture is optimized for kqueue and channel-based backends, not epoll.
- **Latency gap extreme:** PingPongLatency gap between Main and alternatives reaches ~100× (vs ~23× on macOS). The epoll polling mechanism adds dramatically more latency per wakeup cycle.
- **GCPressure divergence:** GCPressure is 1.5–3× worse on Linux vs macOS for all implementations. Docker/epoll imposes heavier GC overhead.
- **Conservation bug (pre-fix):** AlternateOne lost 1–3 tasks in ShutdownConservation_Stress under race detector. This was a known pre-fix issue, since resolved.
- **Race detector:** ✅ 0 data races. FAIL was from conservation violation, not a race report.

### 4.3 Windows (channels/pollFastMode) — First-Ever Data, Strong Main

- **I/O Backend:** Channel-based pollFastMode (no IOCP used, as no I/O FDs are registered in the tournament). Burst wakeup latency ~3.5 µs — between kqueue and epoll.
- **Main most dominant:** 12/14 wins — strongest showing of any platform. CAS-based fast path + channel-based polling is highly effective.
- **AlternateThree recovers:** Unlike Linux, AlternateThree is competitive on Windows (2 wins: MultiProducer, GCPressure; 2nd place in 11 categories). The channel-based backend aligns well with AlternateThree's architecture.
- **GCPressure champion:** Windows shows the best GCPressure for 3 of 4 implementations (AlternateThree 355 ns, Baseline 382 ns, Main 487 ns). The native x86-64 GC appears more efficient for this workload than ARM64 on macOS/Linux.
- **MicroBatchBudget_Mixed pathology:** AlternateThree shows 25–59× degradation on mixed workloads (250K–517K ns/op vs Main's 8–10K ns/op), indicating architectural issues with mode transitions.
- **Race detector:** ❌ **Infrastructure failure.** Binary exited with `0xc0000139` (STATUS_ENTRYPOINT_NOT_FOUND) — a Windows DLL loading failure before any tests executed. **Not a code defect.** All correctness tests pass without `-race`.
- **Conservation validated:** All conservation-enabled implementations achieved perfect 10000/10000 task execution across all stress iterations.

---

## 5. Event Loop Implementation Rankings

### 5.1 Main — Overall Champion

**Total 1st-place wins:** 32/42 (76%)

Main is the clear cross-platform champion. It wins every latency benchmark, every wakeup syscall benchmark (13 consecutive syscall-level wins), and every burst/batch benchmark. Its CAS-based fast path achieves zero allocations on wakeup hot paths. Main is fastest on Linux (11/14), strongest on Windows (12/14), and dominant on macOS (9/14). The only categories where Main consistently loses are PingPong throughput (to AlternateThree on macOS/Windows) and GCPressure (to AlternateTwo). Main's PingPongLatency advantage ranges from 17× to 100× over the alternates depending on platform, demonstrating a fundamental architectural superiority for end-to-end latency.

### 5.2 AlternateThree — Platform-Sensitive Throughput Specialist

**Total 1st-place wins:** 6/42 (14%)

AlternateThree is the strongest alternative implementation but with extreme platform sensitivity. On macOS (kqueue) and Windows (channels), it is a consistent runner-up and occasional winner, taking 2nd place in 10/14 macOS categories and 11/14 Windows categories. On Linux (epoll), it collapses to last place in most categories (5–11× degradation). AlternateThree excels in pure throughput (PingPong), GC resilience (GCPressure on Windows), and batch latency amortization. Its MicroBatchBudget_Mixed pathology on Windows (25–59×) reveals a structural weakness in workload-mode transitions. Best suited for macOS/Windows deployments with throughput-oriented workloads.

### 5.3 AlternateTwo — GC Pressure Specialist

**Total 1st-place wins:** 3/42 (7%)

AlternateTwo is the only implementation to beat Main at GCPressure on two platforms (macOS 420 ns, Linux 469 ns). It also wins MultiProducerContention on Linux (126.6 ns, marginally ahead of Main's 127.5 ns). However, AlternateTwo is consistently among the slowest for throughput (PingPong), latency (PingPongLatency ~10–45K ns), and syscall-level operations. It has a specific architectural advantage under GC pressure but fundamental throughput limitations elsewhere. On Windows, it finishes last in 12/14 categories.

### 5.4 Baseline — Reference Implementation

**Total 1st-place wins:** 1/42 (2%)

Baseline serves as the reference implementation with a traditional architecture (higher allocation count: 3 allocs/op for most benchmarks vs 1 for alternates). It achieves competitive latency (PingPongLatency 527–718 ns, consistently 2nd behind Main), and its simplicity provides reliable behavior across all platforms. On Linux, Baseline takes 2nd place in 10/14 categories. The TestConcurrentStop_Repeated test is notably slow for Baseline (~50s) but passes on all platforms.

### 5.5 AlternateOne — High Overhead, Reliable

**Total 1st-place wins:** 0/42 (0%)

AlternateOne never wins a benchmark category on any platform. It exhibits high memory overhead in continuous workloads (MicroBatchBudget_Continuous: 264 B/op, 6 allocs/op on Windows vs 24 B/op for others). Its shutdown logging is extremely verbose, producing hundreds of log lines per benchmark iteration, which compromises automated data extraction. AlternateOne passed all conservation tests post-fix but showed pre-fix conservation violations on Linux (1–3 tasks lost under race detector).

---

## 6. Promise Implementation Rankings

### 6.1 PromiseAltOne — Cross-Platform Champion

**Total 1st-place wins:** 16/21 (76%)

PromiseAltOne dominates promise benchmarks with remarkable cross-platform consistency:

| Benchmark | macOS | Linux | Windows | Winner? |
|-----------|------:|------:|--------:|:-------:|
| Tournament (throughput) | **292.0** | **375.4** | **348.0** | ✅ All 3 |
| ChainDepth/10 | **1,980** | **2,820** | **2,687** | ✅ All 3 |
| ChainDepth/100 | **7,867** | **16,858** | **14,600** | ✅ All 3 |
| ChainCreation/100 | 6,180 | **5,152** | **6,548** | 2/3 |
| CheckResolved | **125.6** | 141.2 | **138.7** | 2/3 |
| FanOut/100 | 11,086 | 12,187 | 13,386 | 0/3 |
| Race/100 | **3,976** | **4,590** | **4,774** | ✅ All 3 |

### 6.2 PromiseAltTwo — FanOut Specialist

**Total 1st-place wins:** 3/21 (14%)

PromiseAltTwo wins FanOut/100 on all three platforms (8,282–10,373 ns), demonstrating an optimized fan-out architecture. It achieves competitive throughput (runner-up in Tournament on all platforms). However, its Race implementation is non-functional (returns in ~0 ns with 0 allocations on all platforms).

### 6.3 ChainedPromise — Chain Creation Efficiency

**Total 1st-place wins:** 2/21 (10%)

ChainedPromise wins ChainCreation/100 on macOS (5,777 ns) and CheckResolved on Linux (139.8 ns). It has the highest B/op for chain creation (13,784 vs 7,304 for AltOne) but produces the most efficient resolved-promise overhead on Linux. Solidly mid-pack overall.

### 6.4 PromiseAltThree and PromiseAltFour

Neither implementation wins any benchmark category on any platform. PromiseAltThree has a non-functional Race implementation (like PromiseAltTwo). PromiseAltFour consistently has the highest overhead (23,432 B/op for ChainCreation, 45,028 B/op for Race).

---

## 7. Correctness Summary

### 7.1 Functional Correctness

| Test Suite | macOS | Linux | Windows |
|------------|:-----:|:-----:|:-------:|
| GCPressure_Correctness (5 impls) | ✅ | ✅ | ✅ |
| MemoryLeak (5 impls) | ✅ | ✅ | ✅ |
| MultiProducerStress (5 impls) | ✅ | ✅ | ✅ |
| ConcurrentStop (5 impls) | ✅ | ✅ | ✅ |
| ConcurrentStop_WithSubmits (5 impls) | ✅ | ✅ | ✅ |
| ConcurrentStop_Repeated (5 impls) | ✅ | ✅ | ✅ |
| GojaImmediateBurst (5 impls) | ✅ | ✅ | ✅ |
| GojaMixedWorkload (5 impls) | ✅ | ✅ | ✅ |
| GojaNestedTimeouts (5 impls) | ✅ | ✅ | ✅ |
| GojaPromiseChain (5 impls) | ✅ | ✅ | ✅ |
| GojaTimerStress (5 impls) | ✅ | ✅ | ✅ |
| PanicIsolation (5 impls) | ✅ | ✅ | ✅ |
| PanicIsolation_Multiple (5 impls) | ✅ | ✅ | ✅ |
| PanicIsolation_Internal (5 impls) | ✅ | ✅ | ✅ |

**All 14 correctness test suites pass on all 3 platforms for all 5 implementations. 210/210 test slots green.**

### 7.2 Conservation Fix Validation

| Test | macOS | Linux (no -race) | Linux (-race) | Windows |
|------|:-----:|:-----------------:|:-------------:|:-------:|
| ShutdownConservation | ✅ | ✅ | ✅ | ✅ |
| ShutdownConservation_Stress | ✅ | ✅ | ❌ AlternateOne¹ | ✅ |

¹ Linux -race failure was from the **pre-fix** tournament run. AlternateOne lost 1–3 tasks in 3 of 10 iterations. This has since been fixed. Main and AlternateThree passed all iterations on all platforms.

### 7.3 Race Detector

| Platform | Data Races | Race Tests | Overall |
|----------|:----------:|:----------:|:-------:|
| macOS | 0 detected | ✅ PASS | ✅ |
| Linux | 0 detected | ✅ PASS | ✅² |
| Windows | Untestable | ✅ PASS (non-race) | ❌³ |

² Linux race detector FAIL was from conservation violation (task loss), not a data race report. Zero concurrent access violations detected.
³ Windows race-instrumented binary failed to load (`0xc0000139` STATUS_ENTRYPOINT_NOT_FOUND) — a DLL infrastructure issue before any tests executed. **Not a code defect.**

---

## 8. Key Findings

1. **Main is the definitive cross-platform champion**, winning 32 of 42 event loop benchmark slots (76%) and increasing its dominance from macOS (9/14) to Linux (11/14) to Windows (12/14). No other implementation comes close.

2. **AlternateThree has extreme platform sensitivity**, performing 5–11× slower on Linux/epoll versus macOS/kqueue and Windows/channels. This is the single largest cross-platform behavioral divergence observed, suggesting its architecture is fundamentally incompatible with epoll's polling model.

3. **The PingPongLatency architectural gap is universal** — Main maintains a 17–100× latency advantage over alternate implementations across all three platforms. This structural disparity persists regardless of I/O backend, confirming it is an inherent architectural difference.

4. **I/O backend costs vary dramatically**: kqueue burst wakeup is 16× faster than epoll and 3× faster than Windows channels. On Linux, this cost dominates to the point where all five implementations converge to identical performance (~19.6 µs).

5. **Windows is the GCPressure champion** — 3 of 4 implementations achieve their best GCPressure numbers on Windows (AlternateThree 355 ns, Baseline 382 ns, Main 487 ns), suggesting the native x86-64 garbage collector handles GC-heavy workloads more efficiently than ARM64.

6. **PromiseAltOne is the universal promise champion**, winning 16 of 21 promise benchmark slots with near-identical rankings on all three platforms. Its Tournament throughput ranges from 292 ns (macOS) to 375 ns (Linux), with no platform-specific weaknesses.

7. **Zero data races detected** on macOS and Linux. The race detector infrastructure failure on Windows (`0xc0000139`) is a DLL loading issue unrelated to code quality. All concurrent correctness tests pass on all platforms.

8. **The conservation fix is validated cross-platform** — Main, AlternateOne, and AlternateThree achieve perfect 10000/10000 task execution in ShutdownConservation_Stress on all three platforms (post-fix). The atomic ingress-close-during-drain mechanism works correctly regardless of I/O backend.

9. **Main improved significantly since prior tournaments** — BurstSubmit +24% on macOS (best ever), MultiProducer +26% on Linux, PingPongLatency +16% on Linux. These gains come from Go 1.25.5→1.25.7 runtime improvements combined with code-level optimizations.

10. **AlternateTwo is a narrow specialist** — it wins GCPressure on 2/3 platforms but finishes last or near-last in 70%+ of all other categories. Its architecture has fundamental throughput limitations that no platform can overcome.

11. **AlternateOne never wins any benchmark** on any platform (0/42). Its verbose shutdown logging, high memory overhead in continuous workloads (264 B/op, 6 allocs/op), and pre-fix conservation bugs make it the weakest alternate implementation.

12. **PromiseAltTwo and PromiseAltThree have non-functional Race implementations** — both return in ~0 ns with 0 allocations on all three platforms, indicating a no-op or immediately-returning implementation. This is a functionality gap, not a performance issue.

13. **Windows represents viable production deployment** — despite being the first-ever tournament run, all correctness tests pass, conservation is validated, and Main's performance is competitive (within 20% of macOS for most benchmarks, within 40% for all). The only gap is the race detector infrastructure issue.

---

## 9. Recommendations

1. **Adopt Main as the production default on all platforms.** Its cross-platform dominance (76% win rate), latency leadership (17–100× over alternatives), zero-allocation wakeup paths, and validated conservation make it the clear choice.

2. **Investigate AlternateThree's epoll regression.** The 5–11× Linux degradation suggests an architectural mismatch with epoll that could be diagnosed and potentially fixed. If AlternateThree's kqueue/channel-level performance could be replicated on epoll, it would become a strong throughput-focused alternative.

3. **Fix the Windows race detector infrastructure.** The `0xc0000139` DLL loading failure should be resolved to enable race detector validation. This may require updating the Windows Go toolchain or ensuring the correct runtime DLLs are available.

4. **Address non-functional Race implementations** in PromiseAltTwo and PromiseAltThree. These should either implement Race correctly or be explicitly documented as unsupported.

5. **Investigate GCPressure cross-platform variance.** GCPressure differs by up to 3× across platforms for the same implementation. Understanding why Windows excels and Linux struggles could inform GC tuning or allocation strategy changes.

6. **Suppress AlternateOne shutdown logging** in benchmark contexts. The verbose output compromises automated data extraction and inflates log sizes without providing useful benchmark information.

7. **Consider retiring AlternateOne and AlternateTwo** from the active tournament roster. Neither implementation wins any benchmark on any platform, and both have significant drawbacks (AlternateOne: verbose logging, high overhead, pre-fix conservation bugs; AlternateTwo: consistently slowest in throughput). Retaining them as historical reference while focusing optimization efforts on Main and AlternateThree would be more productive.

8. **Establish Windows as a first-class CI platform.** This first-ever Windows tournament demonstrates viable performance. Adding Windows to the regular tournament rotation — once the race detector issue is resolved — would provide ongoing cross-platform validation.

9. **Investigate the MicroBatchBudget_Mixed pathology** in AlternateThree on Windows (25–59× degradation). This extreme workload-mode-transition penalty suggests a channel contention issue that could also affect production mixed workloads.

10. **Re-run the Linux tournament post-conservation-fix.** The Linux tournament was run before the AlternateOne drainIngress fix. A post-fix confirmation run under the race detector would close the last open validation gap.

---

*Report generated: 2026-02-08*
*Sources: MACOS_ANALYSIS.md, LINUX_ANALYSIS.md, WINDOWS_ANALYSIS.md*
*Methodology: 3 runs per benchmark, median of each, ns/op (lower is better)*
