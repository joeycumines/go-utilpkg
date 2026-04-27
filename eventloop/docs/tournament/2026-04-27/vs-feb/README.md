# Adversarial Codebase Autopsy: April 27 vs February 10 Tournament Execution

**Date:** 2026-04-27
**Analysis Date:** 2026-04-27
**Scope:** Cross-tournament performance comparison of the eventloop module

## Executive Verdict — CORRECTED

**The April tournament shows MIXED results:**

| Operation Type | Feb | Apr | Change | Cause |
|---------------|-----|-----|--------|-------|
| Timer operations | ~11,700 ns | ~100-3,000 ns | **-90-99%** | isLoopThread savings dominate |
| Submit/Microtask | ~40-80 ns | ~100-210 ns | **+150-165%** | Auto-exit overhead (no isLoopThread benefit) |

**Key findings:**
- **Timer operations improved 10-250x** due to isLoopThread() optimization (1760ns → 5ns)
- **Submit/Microtask operations regressed 2-3x** because they do NOT use isLoopThread()
- **The original narrative was backwards:** timers did NOT regress; submit/microtask DID

## Document Index

| # | Title | Status |
|---|-------|--------|
| 01 | [Core Code Changes](01_core_code_changes.md) | What actually changed |
| 02 | [Benchmark Delta Analysis](02_benchmark_delta_analysis.md) | Quantified differences |
| 03 | [isLoopThread Impact](03_isloopthread_impact.md) | **CORRECTED** — code diff shows it only helps timers |
| 04 | [Go Version GC Effects](04_go_version_gc_effects.md) | Green tea GC assessment |
| 05 | [Auto-Exit Feature Analysis](05_auto_exit_feature_analysis.md) | **CORRECTED** — actual overhead quantified |
| 06 | [Unexplained Gains](06_unexplained_gains.md) | Obsolete |
| 07 | [Platform-Specific Findings](07_platform_specific_findings.md) | Cross-platform patterns |
| 08 | [Regression Assessment](08_regression_assessment.md) | **CORRECTED** — root causes found |
| 09 | [Honest Conclusions](09_honest_conclusions.md) | **CORRECTED** — true/false synthesis |

## What Was Wrong

### Original Claim: "Auto-exit caused regression in timers"

**Reality:** Timer benchmarks improved 10-250x. The isLoopThread() optimization savings (~1755ns per call) far exceeded the auto-exit overhead (~50ns per operation).

### Original Claim: "Fast-path Submit would benefit from isLoopThread"

**Reality:** `Submit()` does NOT call `isLoopThread()`. This path only sees auto-exit overhead, not isLoopThread() savings.

## Root Causes Found

### BenchmarkFastPathSubmit (+157% regression)

**NOT** from benchmark changes (benchmark code is identical).

**FROM:** `submissionEpoch.Add(1)` added to Submit() fast path in Apr 27.

### BenchmarkMicrotaskSchedule (+165% regression)

**NOT** from benchmark changes (benchmark code is identical).

**FROM:** `submissionEpoch.Add(1)` added to ScheduleMicrotask() in Apr 27.

## The Actual Code Diff

### isLoopThread() — Feb 10
```go
func getGoroutineID() uint64 {
    var buf [64]byte
    n := runtime.Stack(buf[:], false)  // ~1760ns
    // ... parsing loop
}
```

### isLoopThread() — Apr 27
```go
func GoroutineID() (ID int64) {
    ID = goroutineid.Fast()  // ~5ns via assembly
    // ...
}
```

**350x faster** per call, but only called in timer hot paths.

---

*This analysis is adversarial by design. The original narrative was backwards. This document reflects the truth found in the code.*
