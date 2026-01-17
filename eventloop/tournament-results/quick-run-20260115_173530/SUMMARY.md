# Quick Performance Sense-Check Results

**Date:** 2026-01-15 17:35:30
**Platform:** macOS (Darwin), Apple M2 Pro (arm64)
**Benchmark Suite:** Internal tournament tests for eventloop implementations

## Tested Implementations

1. **Main** - Current main implementation
2. **AlternateOne** - Maximum safety variant
3. **AlternateTwo** - Maximum performance variant
4. **AlternateThree** - Balanced (original Main before Phase 18)
5. **Baseline** - goja_nodejs reference implementation

---

## 1. PingPong Throughput (Single Producer, Single Consumer)
*Measures raw task submission and execution throughput*

| Implementation | Ops/sec | ns/op | % Faster than Baseline |
|----------------|---------|-------|----------------------|
| **AlternateThree** | 11,360,000 | 88.02 ns/op | **+16.3%** ⭐ FASTEST |
| **Baseline** | 9,771,000 | 102.4 ns/op | Baseline |
| **AlternateTwo** | 6,187,000 | 161.7 ns/op | -36.7% |
| **Main** | 5,880,000 | 177.4 ns/op | -42.4% |
| **AlternateOne** | ~Failed to complete | ~No data | Failed |

**Key Finding:** AlternateThree (balanced original) is fastest. Main is ~42% slower than Baseline.

---

## 2. PingPong Latency
*Measures end-to-end latency for single task execution*

| Implementation | ns/op | % Slower than Baseline |
|----------------|-------|----------------------|
| **Baseline** | 530.6 ns/op | Baseline ⭐ FASTEST |
| **AlternateThree** | 11,232 ns/op | **+2016%** 🔴 |
| **Main** | 11,280 ns/op | +2125% 🔴 |
| **AlternateOne** | ~11,300 ns/op | ~+2130% 🔴 |
| **AlternateTwo** | 12,564 ns/op | +2368% 🔴 |

**Key Finding:** Baseline dominates on latency (21x faster!). All variants have terrible latency compared to Baseline.

---

## 3. Multi-Producer Throughput (10 Producers)
*Measures performance under concurrent submission pressure*

| Implementation | Ops/sec | ns/op | % Faster than Baseline |
|----------------|---------|-------|----------------------|
| **AlternateThree** | 712,000 | 140.4 ns/op | **+35.0%** ⭐ FASTEST |
| **AlternateOne** | 586,000 | 170.6 ns/op | +14.1% |
| **AlternateTwo** | 512,000 | 195.4 ns/op | +1.7% |
| **Baseline** | 503,000 | 198.8 ns/op | Baseline |
| **Main** | 468,000 | 213.8 ns/op | -7.0% |

**Key Finding:** AlternateThree shines under contention. Main actually performs WORSE than Baseline by 7.0%!

---

## 4. Burst Submit (1000 tasks per burst)
*Measures throughput when submitting in batches*

| Implementation | Ops/sec | ns/op | % Faster than Baseline |
|----------------|---------|-------|----------------------|
| **AlternateThree** | 1,044,000 | 95.75 ns/op | **+13.1%** ⭐ FASTEST |
| **Baseline** | 923,000 | 108.3 ns/op | Baseline |
| **AlternateTwo** | 753,000 | 132.8 ns/op | -18.5% |
| **AlternateOne** | 722,000 | 138.5 ns/op | -21.8% |
| **Main** | 642,000 | 155.8 ns/op | -30.4% |

**Key Finding:** AlternateThree fastest again. Main is ~30.4% slower than Baseline for bursts.

---

## Summary & Analysis

### Performance Ranking (Overall)

| Rank | Implementation | Strengths | Weaknesses |
|------|----------------|-----------|------------|
| 🥇 **1st** | **AlternateThree** | Fastest throughput in all benchmarks, scales well under contention | Horrible latency (21x slower than Baseline) |
| 🥈 **2nd** | **Baseline** | Exceptional latency (5x-21x faster than others) | Slower throughput than AlternateThree, but competitive |
| 🥉 **3rd** | **Main** | Good multi-producer scaling (beats Baseline!) | Slowest throughput in 3/4 benchmarks, terrible latency |
| 4th | **AlternateTwo** | Designed for max performance but underperforms | Bad latency, mediocre throughput, failed to impress |
| 5th | **AlternateOne** | Maximum safety | Failed most benchmarks, extremely slow |

### Key Comparative Insights: Main vs Baseline

| Benchmark | Main | Baseline | Winner | Gap |
|-----------|------|----------|--------|-----|
| **PingPong Throughput** | 177.4 ns/op | 102.4 ns/op | ✅ Baseline | **Main is 42% slower** |
| **PingPong Latency** | 11,131 ns/op | 530.6 ns/op | ✅ Baseline | **Main is 21x slower** |
| **Multi-Producer (10x)** | 213.8 ns/op | 198.8 ns/op | ✅ Baseline | **Main is 7% slower** |
| **Burst Submit** | 155.8 ns/op | 108.3 ns/op | ✅ Baseline | **Main is 30% slower** |

### Critical Findings

1. **Main vs Baseline Trade-off (CORRECTED):**
   - Main loses in ALL benchmarks - no scenario where it beats Baseline
   - Baseline dominates across the board: 21x faster latency, 7-42% better throughput
   - Main has NO advantages over Baseline based on these benchmarks
   - The "Main beats Baseline in multi-producer" claim was incorrect due to data misattribution

2. **The "Missing" Main Performance:**
   - The current Main implementation is surprisingly slow compared to both Baseline and AlternateThree
   - Main doesn't justify its overhead given its poor performance
   - Main loses to Baseline in every single benchmark measured

3. **AlternateThree is the Performer:**
   - Consistently fastest in ALL throughput benchmarks
   - Destroys Main and Baseline in most scenarios
   - This suggests the "pre-Phase 18" implementation was actually better optimized

4. **Baseline's Latency Advantage is Massive:**
   - 21x faster latency is a dealbreaker for async workloads
   - This explains why goja_nodejs chose this architecture
   - The Baseline implementation has fundamentally better task scheduling

5. **AlternateOne Overhead:**
   - AlternateOne completed all benchmarks (not failed as previously stated)
   - Shows competitive performance (better than Main in some scenarios)
   - Safety features come with measurable performance cost but not catastrophic

### Recommendations

1. **For Production Use:**
   - Use **Baseline** if low latency is critical (most workloads)
   - Consider **AlternateThree** if pure throughput matters more than latency
   - Avoid **Main** in its current state - needs significant optimization

2. **For Main Improvement:**
   - Investigate why AlternateThree beats Main so consistently
   - Adopt Baseline's latency strategies (wakeup mechanism?)
   - Focus on single-producer throughput - that's where Main loses most

3. **Investigate Phase 18:**
   - What changed in Phase 18 that degraded Main's performance?
   - AlternateThree is clearly superior - consider reverting or cherry-picking optimizations

---

## Raw Data

Full benchmark results available in this directory:
- `bench_pingpong.raw`
- `bench_latency.raw`
- `bench_multiproducer.raw`
- `bench_burst.raw`
