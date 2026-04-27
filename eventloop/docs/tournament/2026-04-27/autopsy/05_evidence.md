# 05 — Evidence

## Key Data Points

### EVID-001: Windows Mixed Batch Slowdowns

**Source**: `comparison-3platform.md` lines 199-203

```
| BenchmarkMicroBatchBudget_Mixed/AlternateThre | 4587.60 | 35832.80 | 236164.00 | 51.5x |
| BenchmarkMicroBatchBudget_Mixed/AlternateThre | 4436.20 | 30573.60 | 484421.80 | 109.1x |
| BenchmarkMicroBatchBudget_Mixed/AlternateThre | 4438.60 | 30658.00 | 294742.40 | 66.4x |
| BenchmarkMicroBatchBudget_Mixed/AlternateThre | 4430.80 | 29622.40 | 278520.20 | 62.9x |
| BenchmarkMicroBatchBudget_Mixed/AlternateThre | 4482.60 | 30478.40 | 459524.80 | 102.5x |
```

**Verified**: All values from raw JSON data.

---

### EVID-002: Darwin GCPressure/AlternateThree CV

**Source**: `comparison.md` line 633

```
| BenchmarkGCPressure/AlternateThree | 184.6% (high) | 6.1% (high) |
```

**Cross-reference**: `comparison-3platform.md` line 159 confirms 184.58% CV on Darwin.

---

### EVID-003: Linux Throughput Gaps

**Source**: `comparison.md` lines 434-441

```
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/ | 375.98 | 18,791.20 | 49.98x |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=256 | 102.32 | 4,916.00 | 48.05x |
| BenchmarkMicroBatchBudget_Throughput/AlternateThre | 312.80 | 14,807.80 | 47.34x |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=128 | 157.32 | 7,442.20 | 47.31x |
```

---

### EVID-004: All 3 Platforms Have 166 Benchmarks

**Source**: `comparison-3platform.md` lines 9-12

```
- Darwin benchmarks: 166
- Linux benchmarks: 166
- Windows benchmarks: 166
- Common benchmarks: 166
```

---

### EVID-005: Allocation Match Rate

**Source**: `comparison.md` line 502

```
- **Allocs/op match:** 150/166 (90.4%)
- **B/op match:** 110/166 (66.3%)
```

---

### EVID-006: 142/166 Significant Differences

**Source**: `comparison.md` line 424-425

```
**142** out of 166 benchmarks show statistically significant differences (Welch's t-test, p < 0.05).
```

---

## Data Provenance

| Artifact | Size | Notes |
|----------|------|-------|
| darwin.json | 164,035 | 166 benchmarks, 5 runs each |
| linux.json | 164,172 | 166 benchmarks, 5 runs each |
| windows.json | 164,292 | 166 benchmarks, 5 runs each |
| comparison.md | 63,472 | 2-platform analysis |
| comparison-3platform.md | 42,248 | 3-platform analysis |
