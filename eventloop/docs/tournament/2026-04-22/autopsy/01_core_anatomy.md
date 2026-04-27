# 01 — Core Anatomy

## Scope Boundary

This autopsy inspects the tournament artifacts in `eventloop/docs/tournament/2026-04-22/`:

- `darwin.json` — 158 benchmarks, ARM64 macOS, Apple Silicon
- `linux.json` — 158 benchmarks, ARM64 Linux, Docker container
- `windows.json` — 158 benchmarks, AMD64 Windows via WSL/moo
- `comparison.md` — 2-platform (Darwin vs Linux) analysis
- `comparison-3platform.md` — 3-platform analysis
- `parse_benchmarks.py` — Parser for raw benchmark logs
- `Makefile` — Tournament pipeline orchestration

## Comparison with 2026-04-19 Anatomy

| Aspect | 2026-04-19 | 2026-04-22 |
|--------|------------|------------|
| Benchmark counts | 96/45/25 (Darwin/Linux/Windows) | 158/158/158 (all equal) |
| Cross-platform shared | 25 | 158 |
| Analysis scripts | Missing | Present and functional |
| Pipeline status | Broken at analyze step | Operational |
| JSON provenance | Non-reproducible | Verified |

## What Changed Since 2026-04-19

### 1. Coverage Parity Achieved

2026-04-19 had severe coverage skew: Darwin had 96 benchmarks while Linux had 45 and Windows only 25. The 2026-04-22 tournament achieved complete parity at 158 benchmarks per platform. This eliminates the most critical gap from the prior autopsy (GAP-003 in 2026-04-19).

### 2. Pipeline Integrity Restored

The 2026-04-19 autopsy found that `Makefile` targets referenced `analyze_2platform.py` and `analyze_3platform.py` which did not exist in the directory. The 2026-04-22 artifacts include functional analysis scripts that produce the comparison markdown files.

### 3. New Benchmarks Added

Approximately 15 new benchmarks appeared in the 2026-04-22 run that were absent from 2026-04-19:
- `BenchmarkCancelTimer_Individual` variants (timers_1, timers_5, timers_)
- `BenchmarkCancelTimers_Batch` variants
- `BenchmarkCancelTimers_Comparison` (Individual, Batch)
- `BenchmarkChannelWithMutexQueue`
- `BenchmarkCombinedWorkload_New`
- `BenchmarkAlive_Epoch_*` family (on Windows)

### 4. Cross-Tournament Comparison Data

The comparison documents include statistical comparison versus 2026-04-19 results, showing 16-17 significant improvements and 6-7 regressions per platform.

## The 2026-04-22 Pipeline

### Parser (`parse_benchmarks.py`)

The parser reads benchmark output lines in the format:
```
BenchmarkFoo-10      5000      1234.56 ns/op      0 B/op      0 allocs/op
```

It groups 5 runs per benchmark, computes mean/min/max/stddev for ns/op, B/op, and allocs/op, and emits JSON with statistics.

### Analysis Scripts

- `analyze_2platform.py` — Darwin vs Linux comparison (158 common benchmarks)
- `analyze_3platform.py` — 3-platform triangulation

Both scripts use identical Welch t-test methodology on raw 5-run data for cross-tournament
significance testing. This was verified via strict-review-gate (Rule of Two) with two
contiguous issue-free passes. The scripts also accept `--compare-to <past-dir>` for
cross-tournament comparison against any prior tournament run.

### Makefile Targets

From the 2026-04-22 `Makefile`:
- `all` — runs parse + analyze
- `parse` — parses raw logs to JSON
- `analyze` — generates 3-platform markdown report
- `analyze-2platform` — generates 2-platform markdown report

## Data Shape in 2026-04-22 JSON

All platforms show 158 benchmarks with 5 runs each (verified via statistics sections in the JSON). No non-5-run anomalies like those found in 2026-04-19.

## Platform Specifications

| Platform | Arch | CPU | GOMAXPROCS | Environment |
|----------|------|-----|------------|-------------|
| Darwin | ARM64 | Apple Silicon (local) | 10 | Native macOS |
| Linux | ARM64 | Container (Docker golang:1.26.1) | 10 | Docker container |
| Windows | AMD64 | Intel i9-9900K @ 3.60GHz | 16 | WSL/moo |

**Key observation**: GOMAXPROCS differs — Windows uses 16 while Darwin and Linux use 10. This is a potential confound in cross-platform comparisons.

## Critical Anatomical Finding

**Status**: The pipeline is functionally improved over 2026-04-19, but one structural issue remains:

The `comparison.md` and `comparison-3platform.md` files are pre-existing analysis outputs, not regeneratable from the JSON artifacts alone. The JSON files contain benchmark statistics, but the significance testing, category breakdowns, and triangulation tables require the analysis scripts plus the raw benchmark output format.

The pipeline is operational for fresh runs, but the checked-in markdown reports are static snapshots that cannot be regenerated from the JSON alone without re-running benchmarks or having access to the original raw log files.

## What This Means for Trust

1. **Improved over 2026-04-19**: Coverage is complete and pipeline is functional
2. **Still not self-auditing**: JSON statistics are present but markdown analysis cannot be regenerated from JSON alone
3. **Hardware confound remains**: Windows uses different GOMAXPROCS (16 vs 10) and different architecture (AMD64 vs ARM64)
4. **Linux Docker vs Darwin local**: Containerized Linux may have different performance characteristics than native macOS

## Production Impact

The anatomical improvements mean:
- Cross-platform conclusions are now based on the full benchmark surface (158 vs 25)
- Pipeline can be re-run for fresh data
- But checked-in reports are static snapshots with the same provenance concerns as JSON timestamps
