# Promise Tournament v1 - Cross-Platform Analysis

**Date**: 2026-02-09  
**Tournament ID**: promise-tournament-v1  
**Session**: Exhaustive Review Session (Takumi)  
**Status**: COMPLETE

## Executive Summary

A comprehensive performance tournament was conducted across three platforms (macOS, Linux, Windows) comparing five promise implementations. **ChainedPromise (the current production implementation) was found to have significant performance and memory efficiency gaps compared to alternatives.**

### Key Results

| Metric                | ChainedPromise Position                     | Best Alternative | Gap                |
|-----------------------|---------------------------------------------|------------------|--------------------|
| Basic Throughput      | 4th of 5                                    | PromiseAltTwo    | 24% slower         |
| Shallow Chains (d=10) | Inconsistent (1st macOS, 3rd-4th elsewhere) | PromiseAltOne    | Platform-dependent |
| Deep Chains (d=100)   | 4th of 5                                    | PromiseAltFive   | 39% slower         |
| Memory Efficiency     | 4th of 5                                    | PromiseAltFive   | 48% more memory    |

### Recommendations

1. **HIGH**: Reduce memory allocations (~635 B/op → ~425 B/op)
2. **MEDIUM**: Improve deep chain performance (39-74% slower than best)
3. **LOW**: Evaluate lock-free approach for throughput (careful tradeoff analysis needed)

---

## Platform Specifications

### macOS

- **Hardware**: Apple M2 Pro
- **OS**: macOS (arm64)
- **Go Version**: 1.25.7
- **Log File**: `promise-tournament-macos.log`

### Linux

- **Environment**: Docker container `golang:1.25.7`
- **Architecture**: arm64 (host: Apple M2 Pro)
- **Log File**: `promise-tournament-linux.log`

### Windows

- **Hardware**: Intel Core i9-9900K @ 3.60GHz (16 logical cores)
- **OS**: Windows (amd64)
- **Go Version**: 1.25.7
- **Log File**: `promise-tournament-windows.log`

---

## Competitors

| Implementation     | Location                             | Notes                                 |
|--------------------|--------------------------------------|---------------------------------------|
| **ChainedPromise** | `eventloop/promise.go`               | Current production implementation     |
| PromiseAltOne      | `eventloop/internal/promisealtone/`  | Mutex-based variant                   |
| PromiseAltTwo      | `eventloop/internal/promisealttwo/`  | Lock-free implementation              |
| PromiseAltFour     | `eventloop/internal/promisealtfour/` | Experimental variant                  |
| PromiseAltFive     | `eventloop/internal/promisealtfive/` | Snapshot of ChainedPromise (baseline) |

---

## Results by Category

### BenchmarkTournament (Basic Throughput)

Tests basic promise creation, resolution, and handler execution.

**macOS (Apple M2 Pro)**:

| Rank | Implementation     | ns/op | B/op | allocs/op |
|------|--------------------|-------|------|-----------|
| 🥇   | PromiseAltTwo      | 368.6 | 430  | 14        |
| 🥈   | PromiseAltOne      | 441.1 | 429  | 12        |
| 🥉   | PromiseAltFive     | 470.8 | 422  | 12        |
| 4    | **ChainedPromise** | 486.4 | 635  | 13        |
| 5    | PromiseAltFour     | 984.9 | 865  | 19        |

**Linux (Docker golang:1.25.7)**:

| Rank | Implementation     | ns/op | B/op | allocs/op |
|------|--------------------|-------|------|-----------|
| 🥇   | PromiseAltTwo      | 809.1 | 429  | 14        |
| 🥈   | PromiseAltOne      | 810.0 | 427  | 12        |
| 🥉   | **ChainedPromise** | 972.9 | 632  | 13        |
| 4    | PromiseAltFive     | 1053  | 416  | 12        |
| 5    | PromiseAltFour     | 1859  | 867  | 19        |

**Windows (Intel i9-9900K)**:

| Rank | Implementation     | ns/op | B/op | allocs/op |
|------|--------------------|-------|------|-----------|
| 🥇   | PromiseAltOne      | 351.8 | 426  | 12        |
| 🥈   | PromiseAltFive     | 389.5 | 423  | 12        |
| 🥉   | PromiseAltTwo      | 411.9 | 431  | 14        |
| 4    | **ChainedPromise** | 484.7 | 640  | 13        |
| 5    | PromiseAltFour     | 622.7 | 870  | 19        |

**Cross-Platform Summary**:

| Implementation     | macOS | Linux | Windows | Consistency           |
|--------------------|-------|-------|---------|-----------------------|
| PromiseAltTwo      | 🥇    | 🥇    | 🥉      | Excellent on *NIX     |
| PromiseAltOne      | 🥈    | 🥈    | 🥇      | Excellent overall     |
| PromiseAltFive     | 🥉    | 4th   | 🥈      | Very good             |
| **ChainedPromise** | 4th   | 🥉    | 4th     | **Consistently poor** |
| PromiseAltFour     | 5th   | 5th   | 5th     | Worst                 |

---

### BenchmarkChainDepth/Depth=10 (Shallow Chains)

Tests chaining 10 `.Then()` handlers.

**macOS**:

| Rank | Implementation     | ns/op | B/op | allocs/op |
|------|--------------------|-------|------|-----------|
| 🥇   | **ChainedPromise** | 8526  | 1713 | 27        |
| 🥈   | PromiseAltOne      | 9897  | 993  | 26        |
| 🥉   | PromiseAltFive     | 13627 | 984  | 26        |
| 4    | PromiseAltTwo      | 14829 | 1120 | 36        |
| 5    | PromiseAltFour     | 20086 | 2712 | 57        |

**Linux**:

| Rank | Implementation     | ns/op | B/op | allocs/op |
|------|--------------------|-------|------|-----------|
| 🥇   | PromiseAltOne      | 5517  | 993  | 26        |
| 🥈   | PromiseAltFive     | 5578  | 984  | 26        |
| 🥉   | PromiseAltTwo      | 6318  | 1120 | 36        |
| 4    | **ChainedPromise** | 7667  | 1712 | 27        |
| 5    | PromiseAltFour     | 8695  | 2712 | 57        |

**Windows**:

| Rank | Implementation     | ns/op | B/op | allocs/op |
|------|--------------------|-------|------|-----------|
| 🥇   | PromiseAltOne      | 2605  | 996  | 26        |
| 🥈   | PromiseAltFive     | 3018  | 989  | 26        |
| 🥉   | **ChainedPromise** | 3037  | 1717 | 27        |
| 4    | PromiseAltTwo      | 3077  | 1125 | 36        |
| 5    | PromiseAltFour     | 4291  | 2712 | 57        |

**Cross-Platform Summary**:

| Implementation     | macOS | Linux | Windows | Analysis                 |
|--------------------|-------|-------|---------|--------------------------|
| **ChainedPromise** | 🥇    | 4th   | 🥉      | **macOS-only advantage** |
| PromiseAltOne      | 🥈    | 🥇    | 🥇      | Best cross-platform      |
| PromiseAltFive     | 🥉    | 🥈    | 🥈      | Consistent               |
| PromiseAltTwo      | 4th   | 🥉    | 4th     | Inconsistent             |
| PromiseAltFour     | 5th   | 5th   | 5th     | Worst                    |

**Critical Insight**: ChainedPromise's shallow chain advantage is **macOS-specific** and does not transfer to Linux or Windows.

---

### BenchmarkChainDepth/Depth=100 (Deep Chains)

Tests chaining 100 `.Then()` handlers.

**macOS**:

| Rank | Implementation     | ns/op  | B/op  | allocs/op |
|------|--------------------|--------|-------|-----------|
| 🥇   | PromiseAltFive     | 29674  | 8184  | 206       |
| 🥈   | PromiseAltTwo      | 34961  | 9760  | 306       |
| 🥉   | PromiseAltOne      | 41294  | 8192  | 206       |
| 4    | **ChainedPromise** | 48854  | 14672 | 207       |
| 5    | PromiseAltFour     | 125432 | 24313 | 507       |

**Linux**:

| Rank | Implementation     | ns/op | B/op  | allocs/op |
|------|--------------------|-------|-------|-----------|
| 🥇   | PromiseAltFive     | 21628 | 8185  | 206       |
| 🥈   | PromiseAltTwo      | 25324 | 9761  | 306       |
| 🥉   | PromiseAltOne      | 27853 | 8193  | 206       |
| 4    | **ChainedPromise** | 37660 | 14672 | 207       |
| 5    | PromiseAltFour     | 67841 | 24314 | 507       |

**Windows**:

| Rank | Implementation     | ns/op | B/op  | allocs/op |
|------|--------------------|-------|-------|-----------|
| 🥇   | PromiseAltOne      | 13927 | 8193  | 206       |
| 🥈   | PromiseAltFive     | 14087 | 8185  | 206       |
| 🥉   | PromiseAltTwo      | 16299 | 9761  | 306       |
| 4    | **ChainedPromise** | 16401 | 14673 | 207       |
| 5    | PromiseAltFour     | 29213 | 24312 | 507       |

**Cross-Platform Summary**:

| Implementation     | macOS | Linux | Windows | Analysis                 |
|--------------------|-------|-------|---------|--------------------------|
| PromiseAltFive     | 🥇    | 🥇    | 🥈      | **Best for deep chains** |
| PromiseAltTwo      | 🥈    | 🥈    | 🥉      | Consistent 2nd/3rd       |
| PromiseAltOne      | 🥉    | 🥉    | 🥇      | Windows advantage        |
| **ChainedPromise** | 4th   | 4th   | 4th     | **Consistently slow**    |
| PromiseAltFour     | 5th   | 5th   | 5th     | Worst                    |

---

## Memory Efficiency Analysis

**B/op for BenchmarkTournament**:

| Implementation     | macOS | Linux | Windows | Delta vs Best       |
|--------------------|-------|-------|---------|---------------------|
| PromiseAltFive     | 422   | 416   | 423     | **BASELINE (best)** |
| PromiseAltOne      | 429   | 427   | 426     | +2%                 |
| PromiseAltTwo      | 430   | 429   | 431     | +3%                 |
| **ChainedPromise** | 635   | 632   | 640     | **+48%**            |
| PromiseAltFour     | 865   | 867   | 870     | +106%               |

**Conclusion**: ChainedPromise allocates 48% more memory than the most efficient alternatives.

---

## Meta-Analysis

### Why ChainedPromise Underperforms

1. **Memory Overhead**: ChainedPromise maintains additional state for features like unhandled rejection tracking, which adds memory overhead.

2. **Lock Contention**: ChainedPromise uses sync.Mutex while PromiseAltTwo uses lock-free atomics.

3. **Chain Traversal**: Deep chain performance suggests inefficient handler chain traversal compared to PromiseAltFive.

### Platform Variance Analysis

- **macOS Shallow Chain Advantage**: ChainedPromise wins on macOS for shallow chains but loses on other platforms. This suggests macOS-specific scheduler or cache behavior that favors ChainedPromise's memory layout.

- **Windows Intel Advantage**: PromiseAltOne outperforms on Windows, possibly due to Intel CPU cache hierarchy optimization.

- **Linux Container Overhead**: Absolute times ~2x slower in Docker, but relative rankings consistent.

---

## Improvement Roadmap

### Phase 1: Memory Reduction (HIGH PRIORITY)

**Target**: Reduce from ~635 B/op to ~425 B/op

Approach:

1. Profile ChainedPromise allocation sites
2. Compare with PromiseAltFive implementation
3. Identify and eliminate unnecessary allocations
4. Re-run tournament to verify

### Phase 2: Deep Chain Optimization (MEDIUM PRIORITY)

**Target**: Match PromiseAltFive performance (39% improvement needed)

Approach:

1. Profile deep chain traversal
2. Analyze PromiseAltFive's technique
3. Implement optimized chain handling
4. Re-run tournament to verify

### Phase 3: Lock-Free Evaluation (LOW PRIORITY)

**Target**: Evaluate PromiseAltTwo's 24% throughput advantage

Approach:

1. Deep code review of PromiseAltTwo
2. Identify correctness tradeoffs
3. If safe, consider adopting lock-free approach
4. Extensive correctness testing required

---

## Verification Cycle

This tournament requires iterative improvement cycles:

1. ✅ Initial tournament complete
2. ⬜ Implement memory reduction
3. ⬜ Re-run tournament to verify memory improvement
4. ⬜ Implement deep chain optimization
5. ⬜ Re-run tournament to verify chain improvement
6. ⬜ Final comprehensive tournament
7. ⬜ 2x consecutive green subagent reviews
8. ⬜ Commit
