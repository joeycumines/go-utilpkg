# 06 — Honest Conclusions

## Honesty Matrix

| # | Finding | Classification | Evidence |
|---|---|---|---|
| 1 | Windows shows 51-109x slowdown on Mixed batch AlternateThree variants | **TRUE** | comparison-3platform.md lines 199-203 |
| 2 | BenchmarkGCPressure/AlternateThree on Darwin has 184.6% CV | **TRUE** | comparison.md line 633 |
| 3 | Linux throughput is 26-50x slower than Darwin | **TRUE** | comparison.md lines 434-441 |
| 4 | Windows wakeup syscall performance is competitive | **TRUE** | comparison-3platform.md lines 259, 266 |
| 5 | Linux is faster on large burst sizes with Main concurrency mode | **TRUE** | comparison.md lines 48, 52, 192, 196 |
| 6 | All 3 platforms have 166 benchmarks with complete overlap | **TRUE** | comparison-3platform.md lines 9-12 |
| 7 | 90.4% allocation match rate | **TRUE** | comparison.md line 502 |
| 8 | Windows slowdowns suggest platform-specific issue | **UNCERTAIN** | Correlation present, causation unproven |
| 9 | Linux Docker overhead explains throughput gap | **UNCERTAIN** | 26-50x gap too large for documented 1-2% overhead |
| 10 | GCPressure/AlternateThree measurement is noise | **TRUE** | CV > 30% threshold per README.md |

## Key Conclusions

### Windows Mixed Batch Issue

Windows shows catastrophic slowdowns (51-109x) on `BenchmarkMicroBatchBudget_Mixed/AlternateThree` variants. This is the most significant performance issue in the dataset and warrants immediate investigation before Windows production deployment with mixed batch workloads.

### Darwin GCPressure Measurement Unreliable

BenchmarkGCPressure/AlternateThree on Darwin shows 184.6% CV. This benchmark's measurement is essentially noise. Any conclusions from this benchmark should be disregarded.

### Linux Throughput Gap Unexplained

Linux shows 26-50x slower throughput than Darwin, but the documented Docker overhead is only 1-2%. The gap is too large to explain with documented overhead. Possible causes:
1. Docker configuration differs from expected
2. Container resource limits in effect
3. Different Go runtime behavior in containerized environment

### Darwin Competitive Across Most Workloads

Darwin performs competitively or better than Linux on:
- Throughput benchmarks (26-50x advantage)
- Mixed batch operations (5-8x advantage)
- Promise operations (consistently faster)

Linux is faster on:
- Large burst sizes with Main concurrency mode (0.69-0.83x ratio)
- Low-contention scenarios (some benchmarks)

## Recommendations

### For Investigation

1. **Windows Mixed Batch**: Investigate the AlternateThree implementation for platform-specific code paths or environmental factors causing 51-109x slowdown.

2. **Linux Throughput**: Verify Docker container configuration (resource limits, cgroup settings) to explain 26-50x gap.

3. **GCPressure/AlternateThree**: Run 10+ iterations or investigate why Darwin shows 184.6% CV.

### For Production

1. **Windows mixed batch**: Test thoroughly before deployment. Consider whether the production code path uses AlternateThree-style processing.

2. **Linux containerized**: Verify container configuration matches benchmark environment if using Docker for production.

3. **Darwin GC pressure results**: Do not use GCPressure/AlternateThree measurements for capacity planning.
