# 04 — Issues

## Issues Ranked by Severity

### ISSUE-001: Windows Mixed Batch AlternateThree (CRITICAL)

**Severity**: CRITICAL

**What**: Windows shows 51-109x slower performance on `BenchmarkMicroBatchBudget_Mixed/AlternateThree` variants compared to Darwin.

**Evidence**:
```
Burst=10:  Windows 236,164ns vs Darwin 4,587ns (51x slower)
Burst=20:  Windows 484,421ns vs Darwin 4,438ns (109x slower)
Burst=50:  Windows 278,520ns vs Darwin 4,430ns (63x slower)
Burst=100: Windows 459,524ns vs Darwin 4,482ns (103x slower)
```

**Why it matters**: This suggests a platform-specific bottleneck in the mixed batch processing path on Windows. If production workloads use this code path, users will experience 50-100x latency.

**Investigation needed**:
- Check for platform-specific build tags in mixed batch implementation
- Examine IOCP vs kqueue/epoll differences in batch processing
- Verify whether this is a code path issue or environment issue

---

### ISSUE-002: Darwin GCPressure/AlternateThree CV 184.6% (HIGH)

**Severity**: HIGH

**What**: BenchmarkGCPressure/AlternateThree on Darwin shows 184.6% coefficient of variation — more than 6x the 30% "unstable" threshold.

**Evidence**: comparison.md line 633, comparison-3platform.md line 159

**Why it matters**: This benchmark's measurement is essentially noise. Any performance conclusion from this benchmark is unreliable.

**Impact**: One of 8 GC pressure benchmarks has unreliable results on Darwin.

**Investigation needed**:
- Run 10+ iterations instead of 5
- Check for background processes or thermal throttling
- Investigate whether this is a measurement issue or actual variance

---

### ISSUE-003: Linux Throughput Gap (MEDIUM)

**Severity**: MEDIUM

**What**: Linux shows 26-50x slower performance on throughput benchmarks compared to Darwin.

**Evidence**:
```
AlternateTwo/Burst=64: Linux 18,791ns vs Darwin 375ns (50x slower)
Main/Burst=256:        Linux 4,916ns vs Darwin 102ns (48x slower)
AlternateThree/Burst=64: Linux 14,807ns vs Darwin 312ns (47x slower)
```

**Why it matters**: The documented Docker container overhead is 1-2%, but observed overhead is 26-50x. This suggests either:
1. Docker configuration differs from expected
2. The container environment is not representative of native Linux
3. Some other environmental factor

**Investigation needed**:
- Verify Docker resource limits match expected configuration
- Test with different container configurations
- Consider running native Linux benchmarks for comparison

---

### ISSUE-004: B/op Mismatch Rate 66.3% (LOW)

**Severity**: LOW

**What**: Only 110/166 (66.3%) benchmarks show matching B/op across platforms. This is lower than expected for identical code.

**Evidence**: comparison.md line 503

**Why it matters**: B/op differences suggest platform-specific memory allocation behavior. This may affect garbage collection patterns.

**Investigation needed**:
- Check which benchmarks have B/op mismatches
- Determine whether mismatches correlate with performance deltas
- Investigate platform-specific allocator behavior

---

### ISSUE-005: GOMAXPROCS Not Recorded (LOW)

**Severity**: LOW

**What**: GOMAXPROCS settings were not recorded in this tournament. Prior tournaments showed Windows=16 vs Darwin/Linux=10.

**Why it matters**: Without knowing GOMAXPROCS, cross-platform comparisons may be confounded.

**Investigation needed**: Add GOMAXPROCS to platform metadata.

---

## Summary Table

| Issue | Severity | Platform | Type |
|-------|----------|----------|------|
| ISSUE-001: Windows Mixed Batch 51-109x slower | CRITICAL | Windows | Performance |
| ISSUE-002: GCPressure/AlternateThree CV 184.6% | HIGH | Darwin | Measurement reliability |
| ISSUE-003: Linux Throughput 26-50x slower | MEDIUM | Linux | Environment/config |
| ISSUE-004: B/op Mismatch 66.3% | LOW | All | Memory behavior |
| ISSUE-005: GOMAXPROCS not recorded | LOW | All | Metadata |

## Production Impact

1. **Windows mixed batch workloads**: Do not deploy without investigation of ISSUE-001
2. **Darwin GC pressure benchmarks**: Do not rely on GCPressure/AlternateThree measurements
3. **Linux throughput**: Verify Docker configuration matches expectations before production use
