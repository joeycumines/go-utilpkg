# 01 — Core Anatomy

## Scope

Tournament artifacts in `eventloop/docs/tournament/2026-04-27/`:

- `darwin.json`, `linux.json`, `windows.json` — 166 benchmarks each, 5 runs per benchmark
- `comparison.md` — Darwin vs Linux analysis
- `comparison-3platform.md` — 3-platform triangulation
- `parse_benchmarks.py`, `analyze_2platform.py`, `analyze_3platform.py` — Analysis pipeline

## Benchmark Count

166 benchmarks per platform, complete overlap across all 3 platforms.

## Platform Architectures

| Platform | Architecture | Environment |
|----------|-------------|-------------|
| Darwin | ARM64 | Native macOS |
| Linux | ARM64 | Docker container |
| Windows | AMD64 | WSL/moo |

## Performance Overview

| Platform | Mean ns/op | Relative to Darwin |
|----------|-----------|-------------------|
| Darwin | 2,167.40 | baseline |
| Linux | 6,863.83 | 3.2x slower |
| Windows | 13,659.70 | 6.3x slower |

**Note**: These means are skewed by the Windows mixed-batch outliers. Median would be more representative.

## Key Trends

1. **Darwin vs Linux**: Darwin consistently faster on mixed-batch and throughput operations. Gap is largest on AlternateThree variants.

2. **Windows**: Extreme slowdown on mixed-batch AlternateThree variants (51-109x worse than Darwin). Not uniformly slow elsewhere.

3. **GCPressure anomaly**: BenchmarkGCPressure/AlternateThree on Darwin shows 184.6% CV — measurement unreliable.

## What's Different From Prior Tournaments

- Benchmark count: 158 (2026-04-22) → 166 (2026-04-27), +8 benchmarks
- Linux Docker performance appears worse on throughput benchmarks relative to prior tournament
- Windows slowdowns on mixed-batch were present in prior tournament but not highlighted
