# 03 — Cross-Platform Results

## Darwin vs Linux (ARM64 vs ARM64)

### Throughput Benchmarks — Largest Gaps

Darwin shows significant advantages on throughput operations:

| Benchmark | Darwin | Linux | Ratio |
|-----------|--------|-------|-------|
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst=64 | 375.98ns | 18,791ns | 50x |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=256 | 102.32ns | 4,916ns | 48x |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Burst=64 | 312.80ns | 14,807ns | 47x |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=128 | 157.32ns | 7,442ns | 47x |

**Trend**: Linux runs 26-50x slower on throughput benchmarks. The gap is consistent across burst sizes.

### Mixed Batch Benchmarks

| Benchmark | Darwin | Linux | Ratio |
|-----------|--------|-------|-------|
| BenchmarkMicroBatchBudget_Mixed/AlternateThree/Burst=10 | 4,587ns | 35,832ns | 7.8x |
| BenchmarkMicroBatchBudget_Mixed/AlternateThree/Burst=20 | 4,438ns | 30,658ns | 6.9x |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=100 | 2,848ns | 17,126ns | 6.0x |
| BenchmarkMicroBatchBudget_Mixed/Main/Burst=100 | 2,858ns | 14,281ns | 5.0x |

**Trend**: Linux runs 5-8x slower on mixed batch operations. The AlternateThree variant shows the largest gaps.

### Where Linux Is Competitive

Linux performs comparably or better on:

| Benchmark | Darwin | Linux | Ratio |
|-----------|--------|-------|-------|
| BenchmarkMicroBatchBudget_Latency/Main/Burst=1024 | 75.05ns | 60.41ns | 0.80x (Linux faster) |
| BenchmarkMicroBatchBudget_Latency/Main/Burst=2048 | 86.65ns | 61.82ns | 0.71x (Linux faster) |
| BenchmarkMicroBatchBudget_Latency/Main/Burst=4096 | 89.34ns | 61.53ns | 0.69x (Linux faster) |
| BenchmarkMicroCASContention/Main/N=04 | 124.16ns | 78.06ns | 0.63x (Linux faster) |

**Trend**: Linux is faster on large burst sizes with Main concurrency mode. Suggests Linux scheduler handles large batches better.

## Windows Results

### Extreme Slowdowns on Mixed Batch

Windows shows catastrophic performance on `BenchmarkMicroBatchBudget_Mixed/AlternateThree`:

| Burst | Darwin | Windows | Ratio |
|-------|--------|---------|-------|
| 10 | 4,587ns | 236,164ns | 51x |
| 20 | 4,438ns | 484,421ns | 109x |
| 50 | 4,430ns | 278,520ns | 63x |
| 100 | 4,482ns | 459,524ns | 103x |

**Trend**: Windows is 51-109x slower than Darwin on these variants. This is the most significant performance issue in the dataset.

### Where Windows Is Competitive

Windows performs comparably on wakeup syscalls:

| Benchmark | Darwin | Windows | Ratio |
|-----------|--------|---------|-------|
| BenchmarkMicroWakeupSyscall_Sleeping/AlternateThree | 70.62ns | 40.14ns | 0.57x (Windows faster) |
| BenchmarkMicroWakeupSyscall_RapidSubmit/AlternateThree | 71.12ns | 40.24ns | 0.57x (Windows faster) |
| BenchmarkMicroWakeupSyscall_Sleeping/Main | 53.35ns | 44.05ns | 0.83x (Windows faster) |

**Trend**: Windows wakeup syscall performance is competitive or better than Darwin.

## GC Pressure Anomaly

BenchmarkGCPressure/AlternateThree on Darwin shows:
- **CV%**: 184.6% (threshold for "unstable" is 30%)
- This is >6x the instability threshold
- **Mean**: 2,403ns (vs 889ns on Linux, 298ns on Windows)
- **The measurement is unreliable** — any conclusion from this benchmark on Darwin is invalid

## Summary

| Trend | Observation |
|-------|-------------|
| Darwin throughput advantage | Darwin runs 26-50x faster on throughput benchmarks |
| Linux large burst advantage | Linux is faster on large bursts with Main mode |
| Windows mixed batch issue | Windows shows 51-109x slowdown on AlternateThree variants |
| GCPressure anomaly | Darwin's GCPressure/AlternateThree measurement is noise |
| Cross-platform wakeup | Windows wakeup performance is competitive |
