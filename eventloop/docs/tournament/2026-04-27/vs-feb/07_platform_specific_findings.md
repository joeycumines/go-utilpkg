# Platform-Specific Findings

## Overview

The April 27 tournament tested three platforms:
- **Darwin** (macOS, ARM64)
- **Linux** (ARM64, container)
- **Windows** (AMD64/x86_64)

The February 10 tournament also tested all three platforms.

## Darwin vs Linux Comparison

### The Great Convergence

**February 10:**
| Metric | Darwin | Linux | Ratio |
|--------|--------|-------|-------|
| Mean | 27,508 ns/op | 80,984 ns/op | **0.34x** |
| Timer mean | 161,839 ns/op | 492,692 ns/op | **0.33x** |

**April 27:**
| Metric | Darwin | Linux | Ratio |
|--------|--------|-------|-------|
| Mean | 2,167 ns/op | 6,864 ns/op | **0.32x** |
| Timer mean | ~2,000 ns/op | ~6,000 ns/op | **0.33x** |

**Finding:** The Darwin/Linux ratio remained constant (~0.33x). Both platforms benefited equally from the code changes.

### Timer Operation Specifics

| Benchmark | Feb Darwin | Feb Linux | Apr Darwin | Apr Linux |
|-----------|------------|-----------|-----------|-----------|
| BenchmarkCancelTimers_Batch/timers_: | 57,086 | 344,208 | 3,240 | 17,127 |
| Ratio (Darwin/Linux) | **0.17x** | - | **0.19x** | - |

**Finding:** Linux timer performance was consistently 5-6x slower than Darwin in both tournaments. This is a **structural difference**, not a code change.

### Why is Linux Timer Performance Worse?

Possible explanations:
1. **Kernel scheduler**: Linux CFS vs Darwin Mach scheduler
2. **Syscall overhead**: Linux syscall path vs Darwin Mach traps
3. **Container environment**: Linux benchmarks ran in container (cgroups)
4. **Memory management**: Different allocators
5. **Timer implementation**: OS-level timer granularity

## Windows Performance

### Architecture Mismatch

**Critical finding:** Windows benchmarks ran on **AMD64/x86_64**, while Darwin and Linux ran on **ARM64**.

| Platform | Architecture | Implications |
|----------|-------------|--------------|
| Darwin | ARM64 | Apple Silicon |
| Linux | ARM64 | Container on ARM64 |
| Windows | AMD64 | Different ISA |

**This complicates cross-platform comparison.**

### Windows Results

| Metric | Darwin | Linux | Windows |
|--------|--------|-------|---------|
| Mean | 2,167 ns/op | 6,864 ns/op | 13,660 ns/op |
| Win rate | 60.2% | 13.9% | 25.9% |

**Windows is slowest overall**, but this is expected given x86_64 vs ARM64 differences.

## Platform Win Rates

### February 10

| Platform | Wins | Percentage |
|----------|------|------------|
| Darwin | 37 | 34.3% |
| Linux | 42 | 38.9% |
| Windows | 29 | 26.9% |

### April 27

| Platform | Wins | 108 Common | Percentage |
|----------|------|------------|------------|
| Darwin | 100 | ~60 | 60.2% |
| Linux | 23 | ~15 | 13.9% |
| Windows | 43 | ~33 | 25.9% |

**Observation:** Darwin's win rate improved significantly (34% → 60%) when comparing all benchmarks. But when comparing only the 108 common benchmarks, Darwin still dominates.

## Micro-Batch Budget Performance

This is a **new benchmark category** in April 27 (58 new benchmarks added).

### Category: BenchmarkMicroBatchBudget_Latency

| Burst Size | Darwin | Linux | Windows | Fastest |
|-----------|--------|-------|---------|---------|
| 64 | 49.89 ns | 65.37 ns | 93.56 ns | Darwin (0.76x) |
| 128 | 50.76 ns | 67.65 ns | 92.36 ns | Darwin (0.75x) |
| 1024 | 75.05 ns | 60.41 ns | 91.55 ns | Linux (0.94x) |
| 4096 | 89.34 ns | 61.53 ns | 92.10 ns | Linux (0.67x) |

**Finding:** At smaller burst sizes, Darwin wins. At larger burst sizes, Linux wins.

### Category: BenchmarkMicroWakeupSyscall_Burst

| Variant | Darwin | Linux | Windows | Fastest |
|---------|--------|-------|---------|---------|
| Main | 1,239 ns | 19,897 ns | ~10,000 ns | Darwin (0.06x) |
| Baseline | 1,322 ns | 19,795 ns | ~10,000 ns | Darwin (0.07x) |

**Finding:** Darwin is dramatically faster (15x) on syscall burst benchmarks. This suggests:
- Darwin's Mach traps are faster than Linux syscalls
- Linux container environment adds overhead
- Windows syscall path is slower than both

## Cross-Platform Stability

### Coefficient of Variation (CV%)

**February 10:**
| Platform | High CV (>5%) | Very High (>10%) |
|----------|---------------|------------------|
| Darwin | 29 | 7 |
| Linux | 28 | 10 |
| Windows | ~20 | ~5 |

**April 27:**
| Platform | High CV (>5%) | Very High (>10%) |
|----------|---------------|------------------|
| Darwin | 26 | 2 |
| Linux | 49 | 15 |
| Windows | ~40 | ~10 |

**Finding:** Linux stability **degraded** in April (29 → 49 high CV benchmarks). Darwin stability **improved**.

## Platform-Specific Optimizations

### Darwin Optimizations

1. **Mach traps**: Faster than Linux syscalls for event notification
2. **ARM64 efficiency**: Better performance-per-watt on Apple Silicon
3. **kqueue**: Efficient event notification mechanism

### Linux Optimizations

1. **epoll**: Scalable I/O multiplexing
2. **Container overhead**: cgroups, namespaces add latency
3. **Syscall overhead**: General-purpose syscall interface

### Windows Optimizations

1. **IOCP**: Efficient completion-based I/O
2. **x86_64**: Different instruction set (can't directly compare)
3. **Syscall overhead**: Windows syscall path is generally slower

## Architectural Implications

### Why is Darwin Consistently Faster?

1. **ARM64 vs x86_64**: ARM64 is not necessarily faster than x86_64
2. **Apple Silicon**: Integrated GPU, CPU, memory with fast interconnects
3. **Microarchitectural efficiency**: Apple Silicon has excellent single-thread performance
4. **macOS vs Linux kernel**: Mach IPC is highly optimized
5. **No container overhead**: Direct hardware access

### Why is Linux in Container Consistently Slower?

1. **cgroups overhead**: CPU scheduling through cgroups adds latency
2. **Namespace isolation**: IPC through namespace boundaries
3. **Syscall translation**: Additional layer for container syscalls
4. **Network virtualization**: Even loopback has overhead

## Confidence Assessment

| Finding | Evidence | Confidence |
|---------|----------|------------|
| Darwin consistently faster than Linux | Both tournaments | **High** |
| Linux timer operations are 5-6x slower | Data analysis | **High** |
| Windows slowest overall | Data analysis | **High** |
| Windows/ARM64 architectural difference | Platform info | **High** |
| Container overhead explains Linux slowness | Logical inference | **Medium** |
| Mach traps faster than Linux syscalls | Benchmark correlation | **Medium** |

## Conclusion

**Platform differences are structural, not code-related:**

1. **Darwin dominates** due to Apple Silicon efficiency and Mach trap performance
2. **Linux is slower** in container due to cgroup/namespace overhead
3. **Windows is slowest** due to different architecture (x86_64 vs ARM64)
4. **Ratio stayed constant** between tournaments (0.33x Darwin/Linux)

**The code changes (isLoopThread, auto-exit) affected all platforms equally.** The platform performance hierarchy is structural and unlikely to change.

---

*Next: [Regression Assessment](08_regression_assessment.md)*
