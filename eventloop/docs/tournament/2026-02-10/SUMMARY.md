# Benchmark Tournament Executive Summary

**Date:** 2026-02-10 | **Module:** eventloop | **Platforms:** 3

---

## Quick Results

| Metric | Value |
|--------|-------|
| Total benchmarks per platform | 108 |
| Total runs collected | 1,620 (108 × 5 × 3) |
| Cross-platform common benchmarks | 108 (100%) |
| Statistically significant differences (Darwin vs Linux) | 70 / 108 |

---

## Platform Performance Rankings

### By Win Rate (who is fastest most often)

| Rank | Platform | Architecture | Wins | Win Rate |
|------|----------|-------------|------|----------|
| 🥇 1 | **Linux** | ARM64 | 42 | 38.9% |
| 🥈 2 | **Darwin** | ARM64 | 37 | 34.3% |
| 🥉 3 | **Windows** | AMD64 | 29 | 26.9% |

### By Mean Performance (ns/op across all benchmarks)

| Rank | Platform | Mean ns/op | Notes |
|------|----------|-----------|-------|
| 🥇 1 | **Windows** | 26,946.48 | Lowest overall mean, strong on primitives |
| 🥈 2 | **Darwin** | 27,508.22 | Close second, dominant in timer ops |
| 🥉 3 | **Linux** | 80,983.90 | Higher mean due to timer/cancel outliers |

> **Note:** Linux's higher mean is driven by significantly slower timer cancellation operations (3–6x slower than Darwin). When excluding timer cancel benchmarks, Linux and Darwin are within ~5% of each other.

---

## Key Metrics Table

### Overall Performance Comparison

| Category | Darwin | Linux | Windows |
|----------|--------|-------|---------|
| **Fastest benchmark** | 0.30 ns/op (DirectCall) | 0.30 ns/op (DirectCall) | 0.21 ns/op (DirectCall) |
| **Timer operations** | ⭐ Dominant (17/18 wins) | 1/18 wins | Competitive |
| **Task submission** | 8/21 wins | ⭐ 13/21 wins | Varies |
| **Promise operations** | ⭐ 11/18 wins | 7/18 wins | Varies |
| **Concurrency** | 1/3 wins | ⭐ 2/3 wins | Varies |
| **Zero-alloc hot paths** | ✅ Submit, FastPathSubmit | ✅ Submit, FastPathSubmit | ✅ Submit, FastPathSubmit |

### Darwin vs Linux (Both ARM64)

| Metric | Value |
|--------|-------|
| Darwin wins | 55 / 108 (50.9%) |
| Linux wins | 53 / 108 (49.1%) |
| Mean ratio (D/L) | 0.980x |
| Median ratio (D/L) | 0.999x |
| Allocs/op match | 97/108 (89.8%) |
| Significant differences | 70/108 (64.8%) |
| Darwin sig. faster | 40 benchmarks |
| Linux sig. faster | 30 benchmarks |

### Top Speedup Factors (Darwin vs Linux)

| Benchmark | Winner | Speedup |
|-----------|--------|---------|
| CancelTimers_Batch/timers_: | 🍎 Darwin | 6.03x |
| CancelTimers_Comparison/Batch | 🍎 Darwin | 4.33x |
| TimerLatency | 🍎 Darwin | 3.42x |
| TimerSchedule_Parallel | 🍎 Darwin | 3.00x |
| LoopDirectWithSubmit | 🍎 Darwin | 2.95x |
| FastPathExecution | 🐧 Linux | 2.49x |
| SubmitExecution | 🐧 Linux | 2.23x |
| chunkedIngress_ParallelWithSync | 🐧 Linux | 2.02x |

---

## Platform Strengths Summary

### 🍎 Darwin ARM64 (macOS)
- **Dominant in timer operations** — wins 17 of 18 timer benchmarks
- Best timer scheduling latency (3.42x faster than Linux)
- Strong promise operation performance
- Lower variance in most benchmarks

### 🐧 Linux ARM64 (Container)
- **Dominant in parallel/concurrent workloads** — wins most submission benchmarks
- FastPathExecution 2.49x faster, Submit Parallel 1.68x faster
- Better channel round-trip latency
- Higher consistency in low-level operations (DirectCall CV: 0.1% vs 0.7%)

### 🪟 Windows AMD64 (i9-9900K)
- **Fastest absolute latency** — DirectCall at 0.21 ns/op
- Strong StateLoad performance (0.26 ns/op)
- Competitive on specific timer heap operations
- Higher GOMAXPROCS (16) benefits some parallel workloads

---

## Allocation Consistency

| Check | Result |
|-------|--------|
| Allocs/op match (Darwin vs Linux) | 97/108 (89.8%) |
| Allocs/op match (all 3 platforms) | ~92% |
| Zero-alloc benchmarks (all platforms) | Submit, FastPathSubmit |
| B/op match (Darwin vs Linux) | 78/108 (72.2%) |

The allocation mismatches are primarily in timer cancellation batching (Darwin uses fewer allocations for batch cancel operations) and minor B/op differences due to runtime scheduling variance.

---

## Recommendations

### High Priority
1. **Investigate Linux timer cancel slowdown** — Linux is 3–6x slower on CancelTimers_Batch. Likely related to Linux kernel timer resolution or container scheduling overhead.
2. **Optimize Darwin parallel submission** — Darwin is 1.5–2.5x slower on parallel task submission paths. May benefit from GOMAXPROCS tuning or lock-free optimization.

### Medium Priority
3. **Implement benchmark regression detection** — Add CI/CD monitoring with 10% variance thresholds based on this baseline data.
4. **Reduce high-CV benchmarks** — 29 benchmarks have CV > 5% on Darwin. Consider increasing `-count` to 10 for future tournaments.

### Low Priority
5. **Investigate Darwin vs Linux allocation differences** — 11 benchmarks show different alloc counts, primarily in timer batch operations.
6. **Windows-specific optimization** — Windows shows highest total allocation counts; investigate Go runtime behavior on AMD64.

---

## Documents

| Document | Description |
|----------|-------------|
| [README.md](README.md) | Full tournament report and methodology |
| [comparison.md](comparison.md) | 2-platform comparison (Darwin vs Linux) |
| [comparison-3platform.md](comparison-3platform.md) | 3-platform comparison (all platforms) |
| [COMPLETE.md](COMPLETE.md) | Tournament completion checklist |
| [FINAL.md](FINAL.md) | Final report |
