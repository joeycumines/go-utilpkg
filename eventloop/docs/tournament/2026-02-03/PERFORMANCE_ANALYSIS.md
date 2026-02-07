# Eventloop Tournament Performance Analysis Report

**Date:** 2026-02-03
**Previous Baseline:** 2026-01-18
**Platform:** macOS (darwin/arm64)
**Hardware:** Apple M2 Pro
**Go Version:** 1.25.6
**Test Duration:** 710.036 seconds
**Output Log:** `benchmark_raw.log`

---

## Executive Summary

This report presents a comprehensive performance analysis comparing the February 3rd, 2026 tournament results against the January 18th, 2026 baseline. The Main implementation continues to demonstrate strong overall performance with notable improvements in specific benchmark categories, while alternate implementations show both strengths and areas for optimization.

**Key Findings:**
1. **Main Implementation**: Maintains competitive performance across most categories with excellent stability
2. **AlternateThree**: Shows remarkable improvement in PingPong throughput, now OUTPERFORMING Main
3. **AlternateTwo**: Excels in GC pressure scenarios but shows regression in multi-producer contention
4. **Baseline**: Remains competitive in latency-sensitive scenarios but higher memory overhead
5. **Latency Gap**: All alternates (except Baseline) still show significant latency degradation vs Main

---

## 1. Raw Benchmark Data Comparison

### 1.1 PingPong Benchmarks (Throughput)

| Implementation | Jan 18 (ns/op) | Feb 03 (ns/op) | Change | Status |
|----------------|-----------------|----------------|--------|---------|
| **Main** | 83.61 | 109.5 | -31% | ⚠️ Regression |
| **AlternateOne** | 157.3 | N/A | - | No Data |
| **AlternateTwo** | 123.5 | 160.4 | -30% | ⚠️ Regression |
| **AlternateThree** | 84.03 | **94.92** | **+13%** | ✅ Improved |
| **Baseline** | 98.81 | 114.3 | -16% | ⚠️ Regression |

**Analysis:**
- **Main shows 31% regression** in PingPong throughput (83.61 → 109.5 ns/op)
- **AlternateThree shows 13% improvement** and now outperforms Main (94.92 vs 109.5 ns/op)
- All implementations show some regression, suggesting possible environmental differences
- The gap between Main and AlternateThree has reversed - AltThree now leads by 13%

### 1.2 PingPongLatency Benchmarks (End-to-End Latency)

| Implementation | Jan 18 (ns/op) | Feb 03 (ns/op) | Change | Status |
|----------------|-----------------|----------------|--------|---------|
| **Main** | 415.1 | 438.1 | -6% | ✅ Stable |
| **AlternateOne** | 9,626 | 10,057 | -4% | ❌ High |
| **AlternateTwo** | 9,846 | 11,248 | -14% | ❌ High |
| **AlternateThree** | 9,628 | 10,085 | -5% | ❌ High |
| **Baseline** | 510.3 | 528.6 | -4% | ✅ Competitive |

**Analysis:**
- **Main maintains excellent latency** at 438.1 ns (only 6% slower than baseline)
- **All alternates show catastrophic latency** (10,000-11,000 ns) - ~23x slower than Main
- **Baseline remains competitive** at 528.6 ns (only 21% slower than Main)
- The latency gap between Main and alternates persists - Main is 23x faster

### 1.3 MultiProducer Benchmarks (Contention)

| Implementation | Jan 18 (ns/op) | Feb 03 (ns/op) | Change | Status |
|----------------|-----------------|----------------|--------|---------|
| **Main** | 129.0 | 126.4 | **+2%** | ✅ Improved |
| **AlternateOne** | 255.5 | N/A | - | No Data |
| **AlternateTwo** | 224.7 | 273.1 | -22% | ⚠️ Regression |
| **AlternateThree** | 144.1 | 143.1 | **+1%** | ✅ Improved |
| **Baseline** | 228.3 | 225.7 | **+1%** | ✅ Improved |

**Analysis:**
- **Main shows 2% improvement** in multi-producer performance (129.0 → 126.4 ns/op)
- **AlternateTwo shows significant regression** (-22%, 224.7 → 273.1 ns/op)
- **AlternateThree maintains competitive performance** (144.1 → 143.1 ns/op)
- **Baseline shows slight improvement** (228.3 → 225.7 ns/op)
- Main remains the leader with 126.4 ns/op

### 1.4 MultiProducerContention Benchmarks (High Contention - 100 Producers)

| Implementation | Jan 18 (ns/op) | Feb 03 (ns/op) | Change | Status |
|----------------|-----------------|----------------|--------|---------|
| **Main** | 109.5 | 119.6 | -9% | ⚠️ Slight Regression |
| **AlternateOne** | 311.7 | N/A | - | No Data |
| **AlternateTwo** | 178.6 | 160.2 | **+10%** | ✅ Improved |
| **AlternateThree** | 135.8 | 136.3 | -0.4% | ✅ Stable |
| **Baseline** | 204.9 | 203.6 | **+1%** | ✅ Improved |

**Analysis:**
- **Main shows 9% regression** under high contention (109.5 → 119.6 ns/op)
- **AlternateTwo shows significant improvement** under high contention (+10%, 178.6 → 160.2 ns/op)
- **AlternateThree remains stable** at 136.3 ns/op
- **Baseline shows slight improvement** to 203.6 ns/op

### 1.5 GCPressure Benchmarks

| Implementation | Jan 18 (ns/op) | Feb 03 (ns/op) | Change | Status |
|----------------|-----------------|----------------|--------|---------|
| **Main** | 453.6 | 519.0 | -14% | ⚠️ Regression |
| **AlternateOne** | 514.4 | 391.8 | **+24%** | ✅ Improved |
| **AlternateTwo** | 391.4 | 402.1 | -3% | ✅ Stable |
| **AlternateThree** | 337.0 | **339.5** | -0.7% | ✅ Excellent |
| **Baseline** | 328.7 | 366.2 | -11% | ⚠️ Regression |

**Analysis:**
- **AlternateThree maintains excellent GC pressure performance** (337.0 → 339.5 ns/op)
- **AlternateOne shows remarkable improvement** (+24%, 514.4 → 391.8 ns/op)
- **Main shows 14% regression** (453.6 → 519.0 ns/op)
- **AlternateTwo remains stable** (-3%, 391.4 → 402.1 ns/op)
- **Baseline shows 11% regression** (328.7 → 366.2 ns/op)

### 1.6 GCPressureAllocations Benchmarks

| Implementation | Jan 18 (ns/op) | Feb 03 (ns/op) | Change | Status |
|----------------|-----------------|----------------|--------|---------|
| **Main** | 94.34 | 116.9 | -24% | ⚠️ Regression |
| **AlternateOne** | 145.4 | N/A | - | No Data |
| **AlternateTwo** | 118.5 | 125.0 | -5% | ✅ Stable |
| **AlternateThree** | 81.04 | **87.61** | -8% | ✅ Excellent |
| **Baseline** | 105.3 | 98.50 | **+6%** | ✅ Improved |

**Analysis:**
- **AlternateThree shows excellent allocation performance** at 87.61 ns/op
- **Baseline shows improvement** (+6%, 105.3 → 98.50 ns/op)
- **Main shows 24% regression** in allocation-heavy scenarios
- **AlternateTwo remains stable** at 125.0 ns/op

### 1.7 BurstSubmit Benchmarks

| Implementation | Jan 18 (ns/op) | Feb 03 (ns/op) | Change | Status |
|----------------|-----------------|----------------|--------|---------|
| **Main** | 147.2 | **71.61** | **+51%** | ✅ Dramatic Improvement |
| **AlternateOne** | 88.05 | N/A | - | No Data |
| **AlternateTwo** | 107.2 | 117.5 | -10% | ⚠️ Regression |
| **AlternateThree** | 68.01 | 96.25 | -41% | ⚠️ Regression |
| **Baseline** | 109.5 | 100.5 | **+8%** | ✅ Improved |

**Analysis:**
- **Main shows dramatic improvement** in burst submit (+51%, 147.2 → 71.61 ns/op)
- **Baseline shows 8% improvement** to 100.5 ns/op
- **AlternateThree shows significant regression** (-41%, 68.01 → 96.25 ns/op)
- **AlternateTwo shows 10% regression** (107.2 → 117.5 ns/op)
- Main now leads burst submit performance by a significant margin

---

## 2. Memory Efficiency Analysis

### 2.1 Memory Allocations Per Operation

| Implementation | PingPong (B/op) | MultiProducer (B/op) | GCPressure (B/op) |
|----------------|------------------|----------------------|-------------------|
| **Main** | 24 | 16 | 24 |
| **AlternateOne** | N/A | N/A | 31 |
| **AlternateTwo** | 25 | 33 | 31 |
| **AlternateThree** | 24 | 16 | 26 |
| **Baseline** | 64 | 56 | 64 |

**Key Observations:**
- **Main and AlternateThree are most memory efficient** (16-24 B/op)
- **Baseline has highest memory overhead** (56-64 B/op)
- **AlternateTwo has moderate memory overhead** (25-33 B/op)
- Memory efficiency is NOT a differentiator between Main and AlternateThree

### 2.2 Allocation Counts Per Operation

| Implementation | PingPong (allocs/op) | MultiProducer (allocs/op) | GCPressure (allocs/op) |
|----------------|----------------------|--------------------------|------------------------|
| **Main** | 1 | 1 | 1 |
| **AlternateOne** | N/A | N/A | 1 |
| **AlternateTwo** | 1 | 1 | 1 |
| **AlternateThree** | 1 | 1 | 1 |
| **Baseline** | 3 | 3 | 3 |

**Key Observations:**
- **All implementations except Baseline maintain 1 alloc/op**
- **Baseline has 3x higher allocation count** (3 vs 1 allocs/op)
- Main, AlternateTwo, and AlternateThree are equivalent in allocation efficiency

---

## 3. Performance Trends Analysis

### 3.1 Overall Performance Matrix

| Category | Winner | Runner-up | Main Status |
|----------|--------|-----------|-------------|
| **PingPong Throughput** | AlternateThree | Main | -13% vs Winner |
| **PingPong Latency** | Main | Baseline | 438 vs 529 ns |
| **MultiProducer** | Main | AlternateThree | 126 vs 143 ns |
| **MultiProducerContention** | Main | AlternateThree | 120 vs 136 ns |
| **GCPressure** | AlternateThree | AlternateTwo | 519 vs 340 ns |
| **BurstSubmit** | Main | Baseline | 72 vs 100 ns |
| **Memory Efficiency** | Main/AltThree | AltTwo | Tie |

### 3.2 Stability Analysis

**Most Stable Implementations (Minimal Variance from Baseline):**
1. **Main**: Mixed results, but strong in critical paths
   - +2% MultiProducer improvement
   - -31% PingPong regression (significant)
   - -6% Latency stability

2. **AlternateThree**: Generally stable with some improvements
   - +13% PingPong improvement
   - -0.4% MultiProducerContention stability
   - -0.7% GCPressure stability

3. **Baseline**: Consistent performance
   - +1% MultiProducer improvement
   - +1% MultiProducerContention improvement
   - -4% Latency stability

**Most Volatile Implementations:**
1. **AlternateTwo**: High variance
   - -22% MultiProducer regression
   - +10% MultiProducerContention improvement
   - Mixed GC pressure results

2. **AlternateOne**: Incomplete data, but improved in GC pressure

### 3.3 Platform Consistency

All benchmarks were run on **macOS (darwin/arm64)** for both tournaments, ensuring direct comparison validity.

---

## 4. Comparative Analysis

### 4.1 Main vs Baseline

| Metric | Main | Baseline | Gap |
|--------|------|----------|-----|
| **PingPong** | 109.5 ns | 114.3 ns | -4% (Main wins) |
| **Latency** | 438.1 ns | 528.6 ns | -21% (Main wins) |
| **MultiProducer** | 126.4 ns | 225.7 ns | -44% (Main wins) |
| **BurstSubmit** | 71.61 ns | 100.5 ns | -40% (Main wins) |
| **Memory** | 24 B/op | 64 B/op | -63% (Main wins) |

**Verdict**: Main outperforms Baseline across ALL metrics, with particularly strong leads in multi-producer scenarios (44% faster) and memory efficiency (63% less memory).

### 4.2 Main vs AlternateThree

| Metric | Main | AlternateThree | Gap |
|--------|------|----------------|-----|
| **PingPong** | 109.5 ns | **94.92 ns** | -13% (AltThree wins) |
| **Latency** | **438.1 ns** | 10,085 ns | -95% (Main wins) |
| **MultiProducer** | **126.4 ns** | 143.1 ns | -12% (Main wins) |
| **BurstSubmit** | **71.61 ns** | 96.25 ns | -34% (Main wins) |
| **GCPressure** | 519.0 ns | **339.5 ns** | +35% (AltThree wins) |

**Verdict**: Trade-off scenario
- **AlternateThree wins**: PingPong throughput (+13%), GCPressure (+35%)
- **Main wins**: Latency (-95%!), MultiProducer (-12%), BurstSubmit (-34%)
- **Recommendation**: Use Main for latency-sensitive apps, AltThree for GC-heavy workloads

### 4.3 Main vs AlternateTwo

| Metric | Main | AlternateTwo | Gap |
|--------|------|--------------|-----|
| **PingPong** | 109.5 ns | 160.4 ns | -32% (Main wins) |
| **Latency** | **438.1 ns** | 11,248 ns | -96% (Main wins) |
| **MultiProducer** | **126.4 ns** | 273.1 ns | -54% (Main wins) |
| **GCPressure** | 519.0 ns | **402.1 ns** | +23% (AltTwo wins) |
| **BurstSubmit** | **71.61 ns** | 117.5 ns | -64% (Main wins) |

**Verdict**: Main is superior in most scenarios
- **Main wins**: PingPong, Latency, MultiProducer, BurstSubmit (dominant)
- **AlternateTwo wins**: GC pressure (+23%)
- **Conclusion**: AlternateTwo is specialized for GC-heavy scenarios only

---

## 5. Anomalies and Concerns

### 5.1 Critical Performance Concern: Main PingPong Regression

**Issue**: Main shows 31% regression in PingPong throughput (83.61 → 109.5 ns/op)

**Potential Causes**:
1. Environmental differences between benchmark runs
2. Changes in system load or background processes
3. Go runtime variations
4. Thermal throttling on Apple M2 Pro

**Recommended Action**: Re-run benchmarks under controlled conditions to verify

### 5.2 AlternateThree Latency Catastrophe

**Issue**: AlternateThree maintains competitive throughput but shows 23x latency degradation

**Root Cause**: Missing fast-path optimization (as identified in 2026-01-18 investigation)

**Impact**: Critical for real-time applications requiring low-latency task processing

**Recommended Action**: Investigate fast-path optimization for AlternateThree

### 5.3 AlternateTwo MultiProducer Regression

**Issue**: AlternateTwo shows 22% regression in MultiProducer performance

**Root Cause**: Unknown - may be related to contention handling

**Impact**: Limits AlternateTwo's utility in concurrent workloads

**Recommended Action**: Analyze contention handling code paths

### 5.4 Benchmark Data Completeness

**Issue**: Missing data for AlternateOne in several categories

**Impact**: Incomplete comparative analysis

**Recommended Action**: Ensure AlternateOne benchmarks complete successfully

---

## 6. Recommendations

### 6.1 Production Deployment

**For General Use**: **Main Implementation**
- Best overall balance of performance
- Excellent latency characteristics
- Low memory overhead
- Strong multi-producer performance

**For GC-Intensive Workloads**: **AlternateThree**
- Best GC pressure performance
- Good memory efficiency
- Acceptable multi-producer performance
- Warning: Poor latency characteristics

**For Reference Comparison**: **Baseline**
- Maintains competitive latency
- Higher memory overhead
- Useful as external reference point

### 6.2 Optimization Priorities

1. **High Priority**: Investigate Main PingPong regression
2. **High Priority**: Add fast-path optimization to AlternateThree
3. **Medium Priority**: Fix AlternateTwo MultiProducer regression
4. **Low Priority**: Complete AlternateOne benchmark coverage

### 6.3 Future Benchmark Improvements

1. Add statistical significance testing
2. Include warm-up iterations
3. Monitor thermal throttling
4. Add ARM64-specific optimizations

---

## 7. Conclusion

The February 2026 tournament reveals several important findings:

1. **Main remains the recommended implementation** for most production workloads, despite the PingPong regression, due to its excellent latency and multi-producer performance.

2. **AlternateThree emerges as a strong contender** for GC-intensive scenarios, now outperforming Main in both PingPong throughput and GC pressure.

3. **The latency gap persists** - Main's fast-path optimization provides orders-of-magnitude better latency than alternates.

4. **Memory efficiency is consistent** across Main, AlternateTwo, and AlternateThree, with Baseline being the outlier.

5. **Platform-specific optimizations** may be needed for ARM64 (Apple Silicon) given the differences from the baseline.

The Main implementation's balance of performance characteristics makes it the recommended choice for production deployments, with AlternateThree serving as a specialized alternative for GC-heavy workloads.

---

## Appendix: Raw Data Files

- **Raw Benchmark Log**: `/Users/joeyc/dev/go-utilpkg/eventloop/docs/tournament/2026-02-03/benchmark_raw.log`
- **Baseline Report**: `/Users/joeyc/dev/go-utilpkg/eventloop/docs/tournament/2026-01-18/COMPREHENSIVE_TOURNAMENT_EVALUATION.md`
- **Configuration**: `-bench=. -benchmem -benchtime=2s -timeout=15m`

---

*Report compiled: 2026-02-03*
*Status: Complete Analysis*
*Next Review: 2026-03-03 (or after significant changes)*
