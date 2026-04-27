# 05 — Debunking (Claim Verification)

## Claim Verification Matrix

| Claim | Source | Verdict | Evidence |
|-------|--------|---------|----------|
| Darwin wins overall (99/158 vs Linux) | `comparison.md` | **True** | 62.7% win rate, mean ratio 0.962x |
| Darwin wins on timer operations (18/26) | `comparison.md` category analysis | **True** | 69.2% win rate, up to 2.71x speedup on sentinel |
| Darwin wins on promise operations (17/18) | `comparison.md` category analysis | **True** | 94.4% win rate, only exception is PromiseHandlerTracking_Parallel_Optimized |
| Cross-platform results are complete (158 shared) | `02_inventory.md` | **True** | All 3 platforms have exactly 158 benchmarks |
| Pipeline is reproducible | `Makefile` + scripts | **True** | All targets functional, scripts present |
| Linux has high variance on AutoExit | `comparison.md` statistics | **True** | CV=106.3% (FastPathExit), CV=107.7% (ImmediateExit) |
| Cross-tournament comparison shows improvements | `comparison.md` cross-tournament section | **True** | 17 improvements on Darwin, 16 on Linux |
| Windows has 0 significant changes | `comparison-3platform.md` | **True (but misleading)** | No prior baseline for comparison |
| 3-platform win distribution: Darwin 70, Linux 47, Windows 41 | `comparison-3platform.md` | **True** | 70/158 (44.3%), 47/158 (29.7%), 41/158 (25.9%) |
| Mean ratio Darwin/Linux is 0.92x | `comparison.md` | **True** | 0.962x arithmetic mean |
| Allocations match across platforms | `comparison.md` allocation section | **Partially True** | 148/158 match, 10 have mismatches |
| GOMAXPROCS is consistent | Platform specs | **False** | Windows=16, Darwin/Linux=10 |
| Architecture is consistent | Platform specs | **False** | Windows=AMD64, Darwin/Linux=ARM64 |

## Red Herrings (Looks Right, Is Wrong)

### Red Herring 1: "Darwin wins 70/158 across all 3 platforms, so Darwin is the fastest overall"

1. **Why it looks right**: Darwin has the most wins (70) and highest win percentage.
2. **Why it is wrong**: The 3-platform win count conflates architecture differences. Darwin and Linux are both ARM64, but Windows is AMD64. The "Darwin wins more" partly reflects ARM64 vs AMD64 architecture advantages.
3. **Actual conclusion**: Darwin is faster than Linux on ARM64 (62.7% win rate). Darwin vs Windows comparison is confounded by architecture.

### Red Herring 2: "Windows has no significant changes because performance is stable"

1. **Why it looks right**: The cross-tournament comparison shows "0 improvements, 0 regressions" for Windows.
2. **Why it is wrong**: Windows had only 25 benchmarks in 2026-04-19 vs 158 in 2026-04-22. The new benchmarks have no prior baseline, so the "0 changes" reflects missing comparison data, not stability.
3. **Actual conclusion**: Windows performance change cannot be assessed from this tournament due to baseline mismatch.

### Red Herring 3: "Mean ratio of 0.962x proves Darwin is 3.8% faster on average"

1. **Why it looks right**: The arithmetic mean of Darwin/Linux ratios is presented as 0.962x.
2. **Why it is wrong**: Arithmetic mean is skewed by outliers. Median ratio is also 0.966x, but the distribution is highly non-uniform. Some benchmarks show 3-5x differences that dominate the mean.
3. **Actual conclusion**: Darwin is faster on average, but the distribution is bimodal — Darwin dominates timer/promise operations while Linux dominates microtask operations. The mean obscures this structure.

### Red Herring 4: "GOMAXPROCS=16 should make Windows faster on parallel benchmarks"

1. **Why it looks right**: More goroutines (P) should help concurrent workloads.
2. **Why it is wrong**: While Windows does win some parallel benchmarks (`Benchmark_microtaskRing_Parallel`: Windows 1.81x faster), the relationship is not monotonic. Many parallel benchmarks still favor Darwin or Linux despite lower GOMAXPROCS.
3. **Actual conclusion**: GOMAXPROCS is a confound but not the sole determinant of parallel performance.

### Red Herring 5: "The 2026-04-22 tournament supersedes 2026-04-19 completely"

1. **Why it looks right**: 158 benchmarks vs 25-96, complete pipeline, more improvements than regressions.
2. **Why it is wrong**: The cross-tournament comparison still references 2026-04-19 data. Some benchmarks that existed in 2026-04-19 may have been dropped or renamed in 2026-04-22. The new benchmark set is not strictly a superset.
3. **Actual conclusion**: Some 2026-04-19 benchmarks (particularly on Windows) have no 2026-04-22 equivalent for comparison.

## Findings Summary

**Status**: The 2026-04-22 tournament is structurally sound and the reported statistics are accurate. However, several conclusions in the comparison reports are oversimplified.

**Production impact**: Teams can trust the data but must qualify conclusions by:
1. Specifying "ARM64 vs ARM64" for clean OS comparisons
2. Acknowledging GOMAXPROCS as a factor in parallel benchmarks
3. Recognizing that Windows comparison is architecture-confounded
4. Excluding high-variance benchmarks (Linux AutoExit) from reliable conclusions

**The problem**: Clean headline numbers (Darwin wins 70/158) obscure the multi-dimensional nature of the results (architecture, GOMAXPROCS, variance).
