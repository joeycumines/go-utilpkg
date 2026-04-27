# 06 — Kill Conditions

These are concrete scenarios that can cause real decision failure if this tournament snapshot is consumed naively.

## KC-001 — GOMAXPROCS Misattributed as Pure OS Performance

**Scenario**: "Darwin is slower than Windows on parallel workloads because Windows wins Benchmark_microtaskRing_Parallel."

**Failure path**:
1. Decision uses Windows win on parallel benchmark without adjusting for GOMAXPROCS=16 (Windows) vs GOMAXPROCS=10 (Darwin).
2. Conclusion is "Windows scheduler is better for parallel workloads."
3. Infrastructure decision prioritizes Windows for parallel eventloop workloads.
4. Actual cause is 60% more goroutine capacity on Windows.

**Probability**: **High**
**Severity**: **High**

**Mitigations present in code**: None (GOMAXPROCS is not tracked in comparison output).

**Required mitigation**:
- Normalize GOMAXPROCS across platforms before comparison
- Or explicitly tag benchmarks where GOMAXPROCS differs

---

## KC-002 — Architecture Confound in Windows Conclusions

**Scenario**: "macOS ARM64 is faster than Windows AMD64, so ARM64 is faster than AMD64 for eventloop."

**Failure path**:
1. Decision compares Darwin (ARM64) vs Windows (AMD64) results.
2. Concludes "ARM64 beats AMD64 for this workload."
3. Ignores that Darwin is also running macOS while Windows is running Windows — the OS difference also matters.
4. Architecture and OS effects cannot be disentangled.

**Probability**: **High**
**Severity**: **High**

**Mitigations present in code**: None (architecture is not isolated in 3-platform comparison).

**Required mitigation**:
- Only compare Darwin vs Linux (both ARM64) for clean OS effects
- Acknowledge Windows comparisons as "AMD64 + Windows" confounded

---

## KC-003 — High-Variance Benchmarks Used for Policy

**Scenario**: "Linux AutoExit_UnrefExit is 2.17x slower than Darwin."

**Failure path**:
1. Policy uses single benchmark comparison for AutoExit behavior.
2. Linux AutoExit benchmarks have CV > 100% (extreme instability).
3. The observed "slower" performance is within noise range.
4. Policy is based on measurement error, not actual performance.

**Probability**: **Medium** (Linux variance is extreme and well-documented)
**Severity**: **Critical**

**Mitigations present in code**: Coefficient of variation is reported, but no exclusion policy is applied.

**Required mitigation**:
- Exclude benchmarks with CV > 20% from cross-platform policy decisions
- Or increase run count to stabilize measurements

---

## KC-004 — Cross-Tournament Comparison Applied to Windows

**Scenario**: "Windows performance is stable since no significant changes detected."

**Failure path**:
1. Cross-tournament section shows "0 improvements, 0 regressions" for Windows.
2. Analyst concludes "Windows performance is consistent."
3. In reality, the Windows 2026-04-19 baseline had only 25 benchmarks, none of which are in the 2026-04-22 158-benchmark set.
4. No actual comparison was possible; the result is an artifact of baseline mismatch.

**Probability**: **High**
**Severity**: **High**

**Mitigations present in code**: None (baseline mismatch is not flagged in comparison output).

**Required mitigation**:
- Maintain benchmark manifest parity across tournaments
- Flag comparisons where baseline coverage differs

---

## KC-005 — Microtask Results Misgeneralized

**Scenario**: "Linux is faster for microtask workloads, so we should use Linux for microtask-heavy production workloads."

**Failure path**:
1. Microtask category shows Linux wins 9/12 benchmarks (75%).
2. Policy recommends Linux for microtask-heavy workloads.
3. But Linux runs in Docker container while Darwin runs natively — the Docker overhead may be artificially helping or hurting Linux microtask results.
4. The "Linux advantage" may be a Docker artifact.

**Probability**: **Medium**
**Severity**: **Medium**

**Mitigations present in code**: Environment metadata is not normalized across platforms.

**Required mitigation**:
- Compare native Linux vs Darwin for production-representative results
- Or explicitly acknowledge Docker as a factor in Linux comparisons

---

## KC-006 — Timer Performance Extrapolated Beyond Data

**Scenario**: "Darwin is faster for all timer operations because it wins 18/26 timer benchmarks."

**Failure path**:
1. Timer category shows Darwin dominates (18/26 wins).
2. Policy extrapolates "Darwin is better for timer workloads globally."
3. Ignores that Linux wins some timer benchmarks (8/26), particularly timer fire and schedule operations.
4. The generalization misses Linux's strengths in specific timer operations.

**Probability**: **Medium**
**Severity**: **Medium**

**Mitigations present in code**: Category breakdowns exist but aren't enforced in summary conclusions.

**Required mitigation**:
- Provide per-category conclusions, not just overall win rates
- Acknowledge that "Darwin wins overall" ≠ "Darwin wins every category"

---

## KC-007 — Promise Performance Hype

**Scenario**: "Darwin is 94% faster on promise operations."

**Failure path**:
1. Promise category shows Darwin wins 17/18 benchmarks (94.4%).
2. Policy infers "use Darwin for promise-heavy JavaScript workloads."
3. The promise results are from synthetic microbenchmarks, not real goja integration workloads.
4. goja-eventloop may show different performance characteristics under actual JS engine load.

**Probability**: **Low** (microbenchmark vs integration gap is understood)
**Severity**: **Medium**

**Mitigations present in code**: None (goja integration testing is separate from eventloop microbenchmarks).

**Required mitigation**:
- Validate microbenchmark conclusions against goja-eventloop integration benchmarks
- Acknowledge that microbenchmark promise performance ≠ goja promise performance

---

## Summary: Kill Conditions by Severity

| ID | Severity | Scenario |
|----|----------|----------|
| KC-001 | HIGH | GOMAXPROCS confound attributed to OS |
| KC-002 | HIGH | Architecture confound in Windows conclusions |
| KC-003 | CRITICAL | High-variance benchmarks used for policy |
| KC-004 | HIGH | Cross-tournament comparison applied where baseline missing |
| KC-005 | MEDIUM | Docker environment confounds Linux microtask conclusions |
| KC-006 | MEDIUM | Timer generalizations miss Linux strengths |
| KC-007 | MEDIUM | Promise microbenchmark hype misapplied to integration |

**The pattern**: Most kill conditions stem from conflating multiple factors (architecture, GOMAXPROCS, environment) that each contribute to observed performance differences. Clean conclusions require isolating these factors, which this tournament data does not fully support for Windows comparisons.
