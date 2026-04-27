# Tournament Autopsy (2026-04-22 Data) — Index

**Scope**: `eventloop/docs/tournament/2026-04-22` tournament artifacts — 3-platform benchmark results (Darwin ARM64, Linux ARM64, Windows AMD64), with cross-tournament comparison versus 2026-04-19.

**One-sentence verdict**: The 2026-04-22 tournament demonstrates statistically significant Darwin performance advantages over Linux (ARM64 vs ARM64) on timer and promise operations, with both showing distinct strengths in different benchmark categories; cross-tournament comparison shows 17 improvements and 6-7 regressions on each platform, confirming active optimization work, but high-variance benchmarks on Linux (CV > 100% on some AutoExit tests) raise reproducibility concerns for production policy decisions.

## Document Index

| # | Document | Purpose |
|---|---|---|
| 01 | `01_core_anatomy.md` | What the tournament pipeline actually is, and where it breaks |
| 02 | `02_inventory.md` | Full benchmark inventory across all 3 platforms |
| 03 | `03_cross_platform_results.md` | Detailed cross-platform results with significance |
| 04 | `04_gap_analysis.md` | Gap analysis between this and 2026-04-19 |
| 05 | `05_debunking.md` | Claim verification (what was claimed vs actual) |
| 06 | `06_kill_conditions.md` | Failure scenarios if these results are used naively |
| 07 | `07_evidence.md` | Evidence and reproducibility |
| 08 | `08_honest_conclusions.md` | True / Uncertain / False synthesis |

## Recommended Reading Order

1. `01_core_anatomy.md`
2. `02_inventory.md`
3. `03_cross_platform_results.md`
4. `08_honest_conclusions.md`
5. Remaining docs as needed for traceability.

## Critical Caveats Up Front

1. **Complete coverage this time**: All 3 platforms have exactly 158 benchmarks — a major improvement over 2026-04-19's 96/45/25 skew.
2. **Linux high variance**: `BenchmarkAutoExit_FastPathExit` (CV=106.3%) and `BenchmarkAutoExit_ImmediateExit` (CV=107.7%) show extreme instability on Linux, questioning the reliability of AutoExit-family conclusions on that platform.
3. **Architecture mixing**: Darwin and Linux are both ARM64 (controlled OS comparison), while Windows is AMD64 (architecture + OS confounded).
4. **Cross-tournament comparison**: Only Darwin and Linux had matching benchmarks for comparison; Windows had 0 significant changes (all "new" benchmarks).
5. **Pipeline integrity**: Analysis scripts (`analyze_2platform.py`, `analyze_3platform.py`) exist and produce output, unlike 2026-04-19 where they were missing.

## Cross-Tournament Summary (2026-04-22 vs 2026-04-19)

| Platform | Improvements | Regressions | New Benchmarks |
|----------|-------------|-------------|----------------|
| Darwin | 17 | 7 | ~15 (CancelTimer family, ChannelWithMutexQueue, CombinedWorkload_New, etc.) |
| Linux | 16 | 6 | ~15 (same new benchmarks) |
| Windows | 0 | 0 | ~5 (Alive_Epoch family — no prior comparison possible) |

## Key Numerical Claims

- Darwin wins 99/158 benchmarks vs Linux (62.7%)
- Linux wins 59/158 benchmarks vs Darwin (37.3%)
- Statistically significant differences: 110/158 (69.6%)
- Mean ratio (Darwin/Linux): 0.92x (Darwin is faster overall)
- 3-platform win distribution: Darwin 70/158 (44.3%), Linux 47/158 (29.7%), Windows 41/158 (25.9%)

## Areas Analyzed

- Full 3-platform benchmark coverage (158 benchmarks each)
- Cross-platform significance testing (Welch t-test, p<0.05)
- Cross-tournament delta (2026-04-22 vs 2026-04-19)
- Allocation consistency across platforms
- High-variance benchmark identification
- Kill condition scenarios for naive consumption

## Areas Not Analyzed

- Benchmark implementation internals in `eventloop` Go source
- Historical trend comparison beyond 2026-04-19
- Statistical power analysis for sample count adequacy
