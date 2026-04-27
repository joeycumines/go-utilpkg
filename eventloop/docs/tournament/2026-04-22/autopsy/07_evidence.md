# 07 — Evidence

This document contains the direct evidence used for all major conclusions in this autopsy.

## EVID-001: Platform Specification Evidence

From the tournament data headers:

| Platform | Arch | CPU | GOMAXPROCS | Environment |
|----------|------|-----|------------|-------------|
| Darwin | ARM64 | Apple Silicon (local) | 10 | Native macOS |
| Linux | ARM64 | Docker golang:1.26.1 | 10 | Docker container |
| Windows | AMD64 | Intel i9-9900K @ 3.60GHz | 16 | WSL/moo |

**Evidence source**: `comparison.md` and `comparison-3platform.md` platform specification tables.

## EVID-002: Benchmark Count Evidence

All 3 platforms have exactly 158 benchmarks with 5 runs each:

```text
darwin.json: benchmarks=158, runs_per_benchmark=[5,5]
linux.json:  benchmarks=158, runs_per_benchmark=[5,5]
windows.json: benchmarks=158, runs_per_benchmark=[5,5]
```

**Evidence source**: `02_inventory.md` dataset inventory section.

## EVID-003: Cross-Platform Win Rates (Darwin vs Linux)

Darwin vs Linux (ARM64 vs ARM64, clean OS comparison):

```text
common benchmarks = 158
Darwin wins = 99 (62.7%)
Linux wins = 59 (37.3%)
mean ratio (Darwin/Linux) = 0.962x
median ratio (Darwin/Linux) = 0.966x
statistically significant = 110/158 (69.6%)
```

**Evidence source**: `comparison.md` executive summary.

## EVID-004: 3-Platform Win Distribution

All 3 platforms (158 shared benchmarks):

```text
Darwin wins:   70/158 (44.3%)
Linux wins:    47/158 (29.7%)
Windows wins:  41/158 (25.9%)
```

**Evidence source**: `comparison-3platform.md` executive summary.

## EVID-005: Category Analysis — Timer Operations

Timer category (26 benchmarks):
```text
Darwin wins = 18/26 (69.2%)
Linux wins = 8/26 (30.8%)
```

Notable differences:
- `BenchmarkCancelTimer_Individual/timers_:` — Darwin 2.7M ns/op, Linux 5.2M ns/op (1.93x)
- `BenchmarkSentinelIteration_WithTimers` — Darwin 15.8K ns/op, Linux 42.8K ns/op (2.71x)
- `BenchmarkTimerLatency` — Darwin 19.9K ns/op, Linux 49.5K ns/op (2.48x)

**Evidence source**: `comparison.md` "Timer Operations" category section.

## EVID-006: Category Analysis — Promise Operations

Promise category (18 benchmarks):
```text
Darwin wins = 17/18 (94.4%)
Linux wins = 1/18 (5.6%)
```

Linux exception: `BenchmarkPromiseHandlerTracking_Parallel_Optimized` — Linux 2.07x faster

**Evidence source**: `comparison.md` "Promise Operations" category section.

## EVID-007: Category Analysis — Microtask Operations

Microtask category (12 benchmarks):
```text
Darwin wins = 3/12 (25%)
Linux wins = 9/12 (75%)
```

**Evidence source**: `comparison.md` implied microtask analysis and `comparison-3platform.md`.

## EVID-008: High-Variance Benchmarks

Linux extreme variance (CV > 100%):
```text
BenchmarkAutoExit_FastPathExit: CV=106.3%
BenchmarkAutoExit_ImmediateExit: CV=107.7%
BenchmarkSentinelDrain_NoWork: CV=136.1%
```

Darwin variance comparison:
```text
BenchmarkAutoExit_FastPathExit: CV=5.7%
BenchmarkAutoExit_ImmediateExit: CV=1.4%
```

**Evidence source**: `comparison.md` measurement stability section and `comparison-3platform.md` cross-platform triangulation table (CV% columns).

## EVID-009: Cross-Tournament Changes

Darwin (2026-04-22 vs 2026-04-19):
```text
Improvements: 17
Regressions: 7
```

Linux (2026-04-22 vs 2026-04-19):
```text
Improvements: 16
Regressions: 6
```

Windows (2026-04-22 vs 2026-04-19):
```text
Improvements: 0
Regressions: 0
```

**Evidence source**: `comparison.md` cross-tournament sections.

## EVID-010: Allocation Consistency

Allocs/op match across platforms: 148/158 benchmarks

Mismatch benchmarks (10):
- AutoExit family (FastPathExit, ImmediateExit, PollPathExit, TimerFire, UnrefExit)
- PromiseAll
- PromisifyAllocation
- ScheduleTimerWithPool variants
- SetInterval_Parallel_Optimized

Total allocation variation:
```text
Darwin: 222,075,706
Linux: 210,113,428 (-5.4% vs Darwin)
Windows: 214,141,642 (-3.6% vs Darwin)
```

**Evidence source**: `comparison.md` allocation section and `comparison-3platform.md` total allocation summary.

## EVID-011: New Benchmarks in 2026-04-22

Approximately 15 new benchmarks appear in 2026-04-22 not in 2026-04-19:

CancelTimer family:
- `BenchmarkCancelTimer_Individual/timers_1`
- `BenchmarkCancelTimer_Individual/timers_5`
- `BenchmarkCancelTimer_Individual/timers_:` (unbounded)
- `BenchmarkCancelTimers_Batch/timers_1`
- `BenchmarkCancelTimers_Batch/timers_5`
- `BenchmarkCancelTimers_Batch/timers_:` (unbounded)
- `BenchmarkCancelTimers_Comparison/Individual`
- `BenchmarkCancelTimers_Comparison/Batch`

Other:
- `BenchmarkChannelWithMutexQueue`
- `BenchmarkCombinedWorkload_New`
- `BenchmarkAlive_Epoch_ConcurrentSubmit` (Windows)
- `BenchmarkAlive_Epoch_FromCallback` (Windows)
- `BenchmarkAlive_Epoch_NoContention` (Windows)

**Evidence source**: `comparison.md` "New Benchmarks" sections for Darwin and Linux.

## EVID-012: GOMAXPROCS and Architecture Confound Evidence

Windows GOMAXPROCS=16 vs Darwin/Linux GOMAXPROCS=10:

Windows wins on parallel benchmarks (higher GOMAXPROCS advantage):
- `Benchmark_microtaskRing_Parallel`: Windows 67.88 ns/op, Darwin 122.68, Linux 99.61 (Windows 1.81x faster than Darwin, 1.47x faster than Linux)

**Evidence source**: `comparison-3platform.md` cross-platform triangulation table.

## EVID-013: Go Version Change Between Tournaments

`eventloop/go.mod` at the February-2026 baseline commit (`506d664`, 2026-02-10):
```
go 1.25.7
```

`eventloop/go.mod` at the April-2026 tournament commit (`ba73276`, 2026-04-22):
```
go 1.26.1
```

The verified fact is that the `go` directive in `eventloop/go.mod` changed from `1.25.7` to `1.26.1` between these two commits.  The causal link between that directive and the specific Docker image used for each Linux run is **not verifiable from the current repository state**: the historical tournament commits do not contain `config.mk`, and the current `config.mk` (line 62–65) derives its Go version from the *root* module's `go.mod` via `go mod edit -print`, not from `eventloop/go.mod` via awk.  How February's and April's Linux container images were actually selected cannot be determined from repository history alone.

Darwin native Go version was not captured in tournament metadata on either date.  Cross-tournament improvements/regressions cannot be attributed solely to `eventloop` code changes.

**Evidence source**: `git show 506d664:eventloop/go.mod`, `git show ba73276:eventloop/go.mod`.

## EVID-014: `_Old` Benchmark Semantic Change (Not a Runtime Improvement)

Three benchmarks appear to have improved dramatically between the February and April tournaments:
- `BenchmarkLatencySample_OldSortBased`
- `BenchmarkCombinedWorkload_Old`
- `BenchmarkHighFrequencyMonitoring_Old`

This is **not** a runtime performance improvement.

Commit `c63934a` (2026-02-19, nine days after the February tournament at `506d664` on 2026-02-10) changed `sampleWithSort` in `eventloop/metrics_psquare_bench_test.go` from `sort.Slice` to `slices.Sort`. Only these three benchmark families call `sampleWithSort`. The February and April numbers for these three benchmarks measure different code paths and are **not directly comparable**.

Do not cite improvements in `BenchmarkLatencySample_OldSortBased`, `BenchmarkCombinedWorkload_Old`, or `BenchmarkHighFrequencyMonitoring_Old` as evidence of runtime progress between the February and April tournaments.  Other `_Old`-suffixed benchmarks in the repository are not affected by this change.

**Evidence source**: `git show c63934a:eventloop/metrics_psquare_bench_test.go` vs `git show 506d664:eventloop/metrics_psquare_bench_test.go`; `git log --oneline 506d664..ba73276 -- eventloop/metrics_psquare_bench_test.go`.

## Evidence Chain for Key Conclusions

### Conclusion: "Darwin is faster for timer operations"

Evidence chain:
1. EVID-005: Timer category Darwin wins 18/26 (69.2%)
2. EVID-005: Largest differences favor Darwin by 1.9-2.7x
3. EVID-008: Linux timer variance is high but not extreme (CV < 20% on timer benchmarks)
4. Source: `comparison.md` "Timer Operations" section

### Conclusion: "Linux has high variance on AutoExit benchmarks"

Evidence chain:
1. EVID-008: AutoExit_FastPathExit CV=106.3% on Linux
2. EVID-008: AutoExit_ImmediateExit CV=107.7% on Linux
3. EVID-008: SentinelDrain_NoWork CV=136.1% on Linux
4. Source: `comparison.md` measurement stability section

### Conclusion: "Cross-tournament comparison is limited for Windows"

Evidence chain:
1. EVID-009: Windows shows 0 improvements, 0 regressions
2. EVID-011: Windows has ~5 new benchmarks without prior baseline
3. 2026-04-19 Windows had only 25 benchmarks vs 158 in 2026-04-22
4. Source: `comparison-3platform.md` cross-tournament comparison for Windows
