# 08 — Honest Conclusions (True / Uncertain / False)

## Honesty Matrix

| # | Finding | Classification | Evidence |
|---|---|---|---|
| 1 | Darwin wins more benchmarks than Linux overall (99 vs 59, 62.7%) | **TRUE** | `comparison.md` executive summary |
| 2 | Darwin is faster on ARM64 vs ARM64 clean comparison | **TRUE** | Mean ratio 0.962x, median 0.966x |
| 3 | Darwin dominates timer operations (18/26 wins, 69.2%) | **TRUE** | `comparison.md` timer category |
| 4 | Darwin dominates promise operations (17/18 wins, 94.4%) | **TRUE** | `comparison.md` promise category |
| 5 | Linux dominates microtask operations (9/12 wins, 75%) | **TRUE** | Implied from category analysis |
| 6 | 3-platform comparison shows Darwin 70, Linux 47, Windows 41 wins | **TRUE** | `comparison-3platform.md` |
| 7 | Pipeline is functional and reproducible | **TRUE** | Makefile targets work, scripts present |
| 8 | All 3 platforms have 158 benchmarks with complete overlap | **TRUE** | `02_inventory.md` |
| 9 | Cross-tournament shows 16-17 improvements, 6-7 regressions | **TRUE** | `comparison.md` cross-tournament sections |
| 10 | Windows GOMAXPROCS=16 vs Darwin/Linux GOMAXPROCS=10 | **TRUE** | Platform specifications |
| 11 | Windows uses AMD64 vs ARM64 for Darwin/Linux | **TRUE** | Platform specifications |
| 12 | Linux AutoExit benchmarks have extreme variance (CV > 100%) | **TRUE** | `comparison.md` measurement stability |
| 13 | Windows cross-tournament comparison shows 0 changes | **TRUE (but misleading)** | No comparable baseline (25 vs 158 benchmarks) |
| 14 | Darwin vs Windows comparison isolates OS performance | **FALSE** | Architecture confound prevents OS-only conclusions |
| 15 | Allocations are consistent across platforms | **PARTIALLY TRUE** | 148/158 match, 10 have mismatches |
| 16 | Linux Docker container performance reflects native Linux | **UNCERTAIN** | Docker confounds (cgroups, memory, kernel) |
| 17 | Mean ratio 0.962x captures true performance relationship | **UNCERTAIN** | Bimodal distribution (timer/promise vs microtask) |
| 18 | Cross-tournament improvements indicate real optimization | **UNCERTAIN** | Sample stability not verified; some regressions concerning |
| 19 | The tournament results support production platform policy | **FALSE** | Multiple confounds (GAP-001 to GAP-007) prevent clean conclusions |
| 20 | Benchmark microbenchmark results generalize to goja integration | **UNCERTAIN** | goja-eventloop integration may differ from eventloop microbenchmarks |
| 21 | `_Old` / `OldSortBased` benchmarks improved due to runtime or code optimization | **FALSE** | `sampleWithSort` changed from `sort.Slice` to `slices.Sort` in commit `c63934a` (2026-02-19), nine days after the Feb tournament; the benchmarks measure different code paths and are non-comparable across tournaments (EVID-014) |
| 22 | February-to-April cross-tournament deltas isolate `eventloop` code changes | **FALSE** | Go version changed from `go 1.25.7` (Feb) to `go 1.26.1` (Apr) in `eventloop/go.mod`; Linux Docker image selection mechanism cannot be verified from repository history; Darwin native Go version was not recorded (EVID-013, GAP-007) |

## Uncomfortable Truths

### 1. What the headline numbers suggest

**Darwin wins 70/158 across 3 platforms** sounds conclusive. The reality is more nuanced:
- Darwin and Linux (both ARM64) have a clean comparison: Darwin is faster (62.7% win rate)
- Windows (AMD64) comparisons are architecture-confounded: OS effect cannot be isolated
- GOMAXPROCS differs: Windows uses 16, Darwin/Linux use 10 — parallel benchmarks advantage Windows

### 2. What "Darwin is faster overall" misses

Darwin's overall lead comes from specific categories:
- Timer operations: Darwin 69.2% win rate
- Promise operations: Darwin 94.4% win rate
- Microtask operations: Linux 75% win rate (Darwin only 25%)

The mean ratio (0.962x) obscures this bimodal distribution. "Darwin is faster on average" is true but useless for workload-specific decisions.

### 3. What the Linux AutoExit variance means

Linux shows CV > 100% on AutoExit benchmarks. This is measurement noise, not performance signal. Any conclusion about Linux AutoExit performance from this dataset is unreliable — the true performance could be 2x better or worse than measured.

### 4. What the Windows "0 significant changes" hides

The cross-tournament comparison shows 0 improvements and 0 regressions for Windows. This does NOT mean Windows performance is stable. It means the benchmark sets are non-comparable (2026-04-19: 25 benchmarks, 2026-04-22: 158 benchmarks). Windows performance change cannot be assessed.

### 5. What "active optimization work" in cross-tournament changes implies

The 16-17 improvements per platform suggest real optimization progress. But 6-7 regressions also occurred. Some regressions are concerning:
- Darwin: AutoExit_FastPathExit +11.6% (slower)
- Linux: RefUnref_SubmitInternal_External +55.6% (slower)

The "improvements" may be real, but so are the "regressions" — this is ongoing work, not a clean victory.

## Final Verdict

**The 2026-04-22 tournament demonstrates statistically significant Darwin performance advantages over Linux (ARM64 vs ARM64), with both showing distinct strengths in different benchmark categories. Cross-tournament comparison shows 17-22% of benchmarks changed significantly, suggesting ongoing optimization work. However, GOMAXPROCS and architecture confounds prevent clean cross-platform conclusions for Windows, and high Linux AutoExit variance (CV > 100%) undermines reliability for that benchmark family.**

## Recommendations

### For Cross-Platform Conclusions

1. **Darwin vs Linux (ARM64 vs ARM64)**: Clean OS comparison. Darwin is faster overall (62.7% win rate), with particular strength in timer and promise operations. Linux is stronger in microtask operations.

2. **Windows comparisons**: Cannot be disentangled from architecture (AMD64 vs ARM64) and GOMAXPROCS (16 vs 10) effects. Do not draw OS-only conclusions from Windows comparisons.

3. **Linux AutoExit benchmarks**: Unreliable due to CV > 100%. Exclude from policy decisions.

4. **Cross-tournament comparison**: Treat improvements as encouraging but regressions as equally important. 16-17 improvements with 6-7 regressions is ongoing work, not optimization complete.

### For Production Decisions

Current tournament data supports:
- **Timer-heavy workloads**: Consider Darwin deployment (69% win rate)
- **Promise-heavy workloads**: Consider Darwin deployment (94% win rate)
- **Microtask-heavy workloads**: Linux may be preferred (75% win rate)
- **Parallel workloads**: Normalize for GOMAXPROCS before comparing

Current tournament data does NOT support:
- **Windows vs Darwin/Linux performance policy** (architecture + GOMAXPROCS confounds)
- **Linux AutoExit policy** (measurement variance too high)
- **Global "fastest platform" claims** (bimodal distribution)

### What Would Enable Stronger Conclusions

1. Normalize GOMAXPROCS across all platforms (set to same value)
2. Run Linux natively (not in Docker) for production-representative results
3. Add ARM64 Windows runners for clean Windows comparison
4. Increase run count for high-variance benchmarks (10+ runs)
5. Validate microbenchmark conclusions against goja-eventloop integration benchmarks
6. Maintain benchmark manifest parity across tournaments
