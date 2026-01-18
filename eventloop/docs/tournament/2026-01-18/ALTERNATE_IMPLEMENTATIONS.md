# Alternate Event Loop Implementations

This document provides a comprehensive guide to all alternate event loop implementations in the `eventloop/internal/` package. These implementations serve as learning references, comparison baselines, and exploration of different design trade-offs.

## Overview

The eventloop package contains **five implementations** with distinct design philosophies:

| Implementation | Location | Philosophy | Target Use Case |
|----------------|----------|------------|-----------------|
| **Main** | `eventloop/` | Balanced Performance | Production workloads |
| **AlternateOne** | `internal/alternateone/` | Maximum Safety | Debugging & Development |
| **AlternateTwo** | `internal/alternatetwo/` | Maximum Performance | Ultra-low latency |
| **AlternateThree** | `internal/alternatethree/` | Balanced (Original Main) | Reference implementation |
| **Baseline** | `internal/gojabaseline/` | External Reference | Tournament baseline |

## Architecture Comparison

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        IMPLEMENTATION SPECTRUM                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Safety ◄────────────────────────────────────────────────────► Performance  │
│                                                                              │
│    AlternateOne        AlternateThree      Main         AlternateTwo        │
│    (Maximum Safety)    (Original Main)   (Balanced)   (Maximum Perf)        │
│         │                    │              │               │               │
│         ▼                    ▼              ▼               ▼               │
│    ┌─────────┐         ┌─────────┐    ┌─────────┐    ┌─────────┐           │
│    │ Single  │         │ RWMutex │    │ Atomic  │    │Lock-Free│           │
│    │ Mutex   │         │ + Mutex │    │ + MPSC  │    │ + Arena │           │
│    └─────────┘         └─────────┘    └─────────┘    └─────────┘           │
│                                                                              │
│    - Full validation    - Full errors   - Smart fast  - Zero alloc         │
│    - Phased shutdown    - Defense-in-   - path mode   - Skip validation    │
│    - Rich errors          depth chunk   - ChunkedIn-  - Direct FD array    │
│    - Lock during poll     clearing        gress       - Inline callbacks   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Main Implementation (Production)

**Location**: `eventloop/`

### Design Philosophy

The Main implementation is the **production-ready** version, achieving an optimal balance between safety and performance. It won the internal tournament against all alternatives including the external Baseline.

### Key Features

| Feature | Implementation |
|---------|----------------|
| **State Machine** | Atomic CAS-based with 5 states |
| **Ingress Queue** | ChunkedIngress with MPSC pattern |
| **Fast Path** | Intelligent auto-switching (`FastPathAuto`) |
| **Poller** | Platform-optimized (kqueue/epoll) |
| **Completion** | `loopDone` channel signaling |

### Smart Fast Path Mode

```go
type FastPathMode int

const (
    FastPathAuto     FastPathMode = 0  // Auto-detect based on IO load (default)
    FastPathForced   FastPathMode = 1  // Always use fast path
    FastPathDisabled FastPathMode = 2  // Never use fast path (debugging)
)
```

The `FastPathAuto` mode intelligently switches between:
- **Fast path**: When no I/O callbacks are registered (pure message-passing)
- **Poll path**: When I/O operations are active

#### FastPath/FD Invariant Enforcement

`FastPathForced` and I/O FD registration are mutually exclusive:

| Operation | When Mode=Forced | When FDs Registered |
|-----------|------------------|---------------------|
| `SetFastPathMode(FastPathForced)` | Succeeds | Returns `ErrFastPathIncompatible` |
| `RegisterFD(...)` | Returns `ErrFastPathIncompatible` | Succeeds (loop uses Auto behavior) |

**Thread Safety:** Both operations use lock-free atomic checks with rollback on conflict. Under concurrent access, exactly one operation will fail with `ErrFastPathIncompatible`. No deadlock or livelock is possible.

**Implementation (Symmetric Optimistic Concurrency):**

Both `RegisterFD` and `SetFastPathMode` use the same Store-Load pattern:

1. **Optimistically Store** primary state
2. **Validate** secondary state
3. **Rollback** if invariant violated

`SetFastPathMode` example:
```go
// STEP 2: Swap Mode FIRST (creates Store-Load barrier, returns previous mode)
prev := FastPathMode(l.fastPathMode.Swap(int32(mode)))

// STEP 3: Validate secondary state
if mode == FastPathForced && l.userIOFDCount.Load() > 0 {
    l.fastPathMode.CompareAndSwap(int32(mode), int32(prev)) // Rollback to PREV
    return ErrFastPathIncompatible
}

// STEP 4: Wake loop for liveness
l.doWakeup()
```

`RegisterFD` rollback handles concurrent unregister:
```go
if FastPathMode(l.fastPathMode.Load()) == FastPathForced {
    // Conditional rollback prevents underflow if concurrent UnregisterFD
    if err := l.poller.UnregisterFD(fd); err != ErrFDNotRegistered {
        l.userIOFDCount.Add(-1)
    }
    return ErrFastPathIncompatible
}
```

**Rationale:** Fast path (`runFastPath`) bypasses the I/O poller entirely, blocking on a channel for task submissions. Registering FDs in forced mode would result in I/O events never being delivered—a silent correctness bug. The bidirectional enforcement prevents this class of error at the API boundary.

**Performance:** The enforcement adds negligible overhead (<1% of RegisterFD latency) and zero overhead to hot paths (Submit, tick, poll).

### Performance Metrics (Tournament Evaluated)

**Complete Evaluation**: See `COMPREHENSIVE_TOURNAMENT_EVALUATION.md` (779 data points, 6 benchmark categories, 2 platforms).

**Summary Results:**

| Platform | Category | Main | 2nd Best | Main Advantage |
|----------|----------|------|----------|----------------|
| **macOS** | PingPong Latency | 415.1 ns | Baseline 510.3 ns | **23% faster** |
| **macOS** | PingPong Throughput | 83.61 ns | AltThree 84.03 ns | **Competitive** (0.5% slower) |
| **macOS** | MultiProducer | 124.6 ns | AltOne 179.8 ns | **44% faster** |
| **macOS** | GCPressure | 453.6 ns | AltThree 348.3 ns | 30% slower |
| **Linux** | PingPong Latency | 503.8 ns | Baseline 597.4 ns | **16% faster** |
| **Linux** | PingPong Throughput | 53.79 ns | Baseline 88.17 ns | **64% faster** |
| **Linux** | MultiProducer | 126.6 ns | Baseline 194.7 ns | **54% faster** |
| **Linux** | GCPressure | 1,355 ns | AltTwo 377.5 ns | 72% slower |

**Weighted Score (Production Criteria):**
- **macOS**: 88.2/100
- **Linux**: 87.6/100
- **Overall**: 87.9/100 (28% margin over 2nd place)
- **Status**: **PARETO OPTIMAL** - cannot be improved on any dimension without degrading another

**See**: `FINAL_RECOMMENDATION_EVALUATION.md` for mathematical proof of superiority.

---

## AlternateOne: Maximum Safety

**Location**: `eventloop/internal/alternateone/`

### Design Philosophy

AlternateOne prioritizes **correctness guarantees** and **defensive programming** over raw performance. Every design decision favors preventing subtle bugs over micro-optimizations.

### Core Principles

1. **Fail-Fast over Fail-Silent**: All error paths are explicit and observable
2. **Lock Coarseness**: Single mutex for entire ingress subsystem
3. **Allocation Tolerance**: Accept allocations for clarity
4. **Extensive Validation**: Runtime invariant checks always enabled
5. **Deterministic Behavior**: No timing assumptions

### Synchronization Design

```
┌─────────────────────────────────────────────────────────────────┐
│                         Loop                                     │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ SafeIngress  │  │ SafePoller   │  │ SafeState    │          │
│  │ (Single Lock)│  │ (Write Lock) │  │ (Validated)  │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────────┐    │
│  │              ShutdownManager                              │    │
│  │   Phase1 → Phase2 → Phase3 → Phase4 → Phase5 → Complete  │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

### Key Design Decisions

| Aspect | AlternateOne | Main |
|--------|--------------|------|
| Lock granularity | Single Mutex | RWMutex + Atomics |
| Invariant checks | Always enabled | Debug builds only |
| Error handling | Rich context (LoopError, PanicError) | Basic errors |
| Callback execution | Inside lock | Outside lock |
| Chunk clearing | Always full (128 slots) | Optimized |
| Poll locking | Lock (write) | RLock (read) |
| Check-then-sleep | Lock held through decision | Optimistic |

### Error Types

```go
// LoopError - Structured error with full context
type LoopError struct {
    Op      string         // Operation that failed
    Phase   string         // Lifecycle phase
    Cause   error          // Underlying error
    Context map[string]any // Additional context
}

// PanicError - Full panic capture with stack trace
type PanicError struct {
    Value      any
    TaskID     uint64
    LoopID     uint64
    StackTrace string
}
```

### When to Use

✅ **Choose AlternateOne when:**
- Correctness is the highest priority
- Debugging ease is important
- Running in development/testing environments
- Need comprehensive error information

❌ **Avoid when:**
- Maximum throughput required
- Latency is critical (<10µs)
- High contention expected

### Performance Metrics (Tournament Evaluated)

**Tournament Results (COMPREHENSIVE_TOURNAMENT_EVALUATION.md):**

| Platform | Metric | AlternateOne | vs Main |
|----------|--------|--------------|----------|
| **macOS** | PingPongLatency | 9,626 ns | **22x slower** (Missing fast path) |
| **macOS** | PingPongThroughput | 157.3 ns/op | 88% slower |
| **macOS** | MultiProducer | 179.8 ns/op | 44% slower |
| **macOS** | GCPressure | 405.4 ns | 11% better |
| **macOS** | Memory | 0-144 B/op | Competitive |
| **Linux** | PingPongThroughput | 126.6 ns/op | 135% slower |
| **Linux** | MultiProducer | 165.4 ns/op | 31% slower |
| **Linux** | GCPressure | 843.3 ns | 38% better |

**Weighted Score:**
- **macOS**: 42.1/100 (Latency failure dominates)
- **Linux**: 38.9/100 (Throughput penalty)
- **Overall**: 40.5/100 (47% worse than Main)

**Critical Failure:**
- **Missing fast path**: 22-24x latency degradation (9,626ns vs Main's 415ns)
- **Root cause**: Full Tick() execution (~5-6μs) vs Main's direct/channel tight loop (~400ns)
- **Impact**: Fatal for production workloads requiring responsiveness

**See**: `ANALYSIS_LATENCY_INVESTIGATION.md` for root cause proof.

---

## AlternateTwo: Maximum Performance

**Location**: `eventloop/internal/alternatetwo/`

### Design Philosophy

AlternateTwo prioritizes **throughput**, **zero allocations**, and **minimal latency** over defensive safety measures. Every design decision favors speed.

### Core Principles

1. **Zero Allocations on Hot Paths**: No make(), no boxing, no closures
2. **Lock-Free Where Possible**: CAS loops instead of mutexes
3. **Cache-Line Awareness**: Padding to avoid false sharing
4. **Batch Operations**: Amortize overhead
5. **Assume Correct Usage**: Skip validation

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Loop                                     │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │LockFreeIngress│ │  FastPoller  │  │  FastState   │          │
│  │ (Atomic MPSC)│  │ (Zero Lock)  │  │ (Padded CAS) │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ MicrotaskRing│  │  TaskArena   │  │   loopDone   │          │
│  │ (Lock-Free)  │  │ (Pre-alloc)  │  │  (Channel)   │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
└─────────────────────────────────────────────────────────────────┘
```

### Key Components

#### FastState (Cache-line padded)
```go
type FastState struct {
    _     [64]byte       // Padding before
    state atomic.Int32
    _     [60]byte       // Padding after
}
```

#### LockFreeIngress (MPSC Queue)
```go
type LockFreeIngress struct {
    _    [64]byte
    head atomic.Pointer[node]
    _    [56]byte
    tail atomic.Pointer[node]
    _    [56]byte
    len  atomic.Int64
}
```

#### FastPoller (Direct FD Indexing)
```go
type FastPoller struct {
    epfd     int
    fds      [65536]fdEntry  // Direct indexing, no map
    versions [65536]uint32   // Version for ABA prevention
}
```

### Key Design Decisions

| Aspect | AlternateTwo | Main |
|--------|--------------|------|
| Queue | Lock-free MPSC | Mutex + ChunkedList |
| Poller FD storage | Direct array [65536] | Map |
| Memory | Arena + aggressive pooling | GC-managed pools |
| Callbacks | Inline execution | Collect-then-execute |
| Validation | Minimal/skipped | Present |
| Chunk clearing | Used slots only | Full defense-in-depth |

### Safety Trade-offs (Acknowledged)

⚠️ **This implementation accepts these risks:**
1. No invariant validation - bugs manifest as corruption
2. Optimistic locking - race conditions possible under extreme load
3. Minimal error handling - some errors silently ignored
4. Direct array indexing - FDs > 65535 cause undefined behavior

### When to Use

✅ **Choose AlternateTwo when:**
- Maximum throughput required
- Low latency critical (<10µs P99)
- High contention expected
- Memory allocation must be minimized

❌ **Avoid when:**
- Debugging complex issues
- Development/prototyping phase
- Correctness verification needed

### Performance Metrics (Tournament Evaluated)

**Tournament Results (COMPREHENSIVE_TOURNAMENT_EVALUATION.md):**

| Platform | Metric | AlternateTwo | vs Main |
|----------|--------|--------------|----------|
| **macOS** | PingPongLatency | 9,846 ns | **24x slower** (Missing fast path) |
| **macOS** | PingPongThroughput | 123.5 ns/op | 48% slower |
| **macOS** | GCPressure | 391.4 ns | **14% better** (TaskArena advantage) |
| **Linux** | PingPongThroughput | 126.6 ns/op | 135% slower |
| **Linux** | MultiProducer | 179.2 ns/op | 42% slower |
| **Linux** | GCPressure | 377.5 ns | **72% better** (TaskArena advantage) |

**Weighted Score:**
- **macOS**: 53.7/100 (Latency failure)
- **Linux**: 51.2/100 (Latency failure)
- **Overall**: 52.5/100 (40% worse than Main)

**Niche Strength: GC Pressure Resilience**
- **72% Linux GC advantage**: TaskArena pre-allocation + lock-free ingress
- **Root cause**: Zero dynamic allocation + no mutex blocking under GC
- **Trade-off**: Missing fast path causes 24x latency degradation

**Critical Failure:**
- **Missing fast path**: 24x latency degradation (9,846ns vs Main's 415ns)
- **Throughput penalty**: 48-135% slower (123.5-126.6ns vs Main's 83.6-53.8ns)

**Use Only If:**
1. **GC bottleneck confirmed**: Profiling shows GC pauses >50% of runtime
2. **Latency tolerant**: Accepting 24x degradation, P99 >10ms acceptable
3. **Single-platform deployment**: Variance acceptable (macOS/Linux 2.5pt score diff)

**See**:
- `ANALYSIS_LATENCY_INVESTIGATION.md` - Latency failure root cause
- `ANALYSIS_GC_PRESSURE_INVESTIGATION.md` - GC strength root cause

---

## AlternateThree: Balanced (Original Main)

**Location**: `eventloop/internal/alternatethree/`

### Design Philosophy

AlternateThree was the **original Main implementation** before the Phase 18 promotion of the optimized variant. It provides a balanced trade-off between safety and performance.

### Key Features

- Mutex-based ingress queue (simple, correct)
- RWMutex for poller (allows concurrent reads)
- Full error handling and validation
- Defense-in-depth chunk clearing
- `loopDone` channel completion signaling

### When to Use

✅ **Choose AlternateThree when:**
- P99 latency is critical (excellent at 570.5µs)
- Moderate throughput acceptable (~556K ops/s)
- Full error handling needed
- Prefer simpler debugging (mutex-based)

### Performance Metrics (Tournament Evaluated)

**Tournament Results (COMPREHENSIVE_TOURNAMENT_EVALUATION.md):**

| Platform | Metric | AlternateThree | vs Main |
|----------|--------|----------------|----------|
| **macOS** | PingPongLatency | 9,628 ns | **22x slower** (Missing fast path) |
| **macOS** | PingPongThroughput | 84.03 ns/op | Competitive (0.5% faster) |
| **macOS** | GCPressure | 348.3 ns | **23% better** |
| **Linux** | MultiProducer | 308.3 ns/op | **2.4x slower** |
| **Linux** | MultiProducerLatency | 1,846 ns | **14.6x slower** (**CATASTROPHIC**) |
| **Linux** | PingPongThroughput | 350.4 ns/op | **6.5x slower** |
| **Linux** | GCPressure | 799.6 ns | Competitive |
| **Overall** | Weighted Score | 40.5/100 | 54% worse than Main |

**Critical Failures:**

1. **Catastrophic Linux MultiProducer**:
   - 1,846ns latency vs Main's 126.6ns (14.6x worse)
   - **Root cause**: Missing channel-based fast path + catastrophic eventfd overhead
   - **Impact**: Non-viable for production on Linux with multiple producers
   - **See**: `ANALYSIS_ALTERNATETHREE_LINUX_INVESTIGATION.md`

2. **Missing fast path**:
   - 22-24x latency degradation (9,628ns vs Main's 415ns)
   - Same root cause as other alternates: full Tick() execution

3. **Platform variance**:
   - macOS: 51.8/100 score
   - Linux: 29.1/100 score
   - Variance: **22.7 points** (unacceptable for production)

**Conclusion: OBSOLETE - Not Recommended**

AlternateThree has **unacceptable platform variance** (22.7 point score difference) and **catastrophic performance failures** (14.6x MultiProducer latency degradation). This implementation is non-viable for production use.

---

## Baseline: External Reference (goja-nodejs)

**Location**: `eventloop/internal/gojabaseline/`

### Design Philosophy

The Baseline wraps `github.com/dop251/goja_nodejs/eventloop` to serve as an **external reference implementation**. Our custom implementations must outperform this to be considered viable.

### Implementation Details

```go
type Loop struct {
    inner        *gojaloop.EventLoop
    loopDone     chan struct{}
    shutdownOnce sync.Once
    running      atomic.Bool
    stopped      atomic.Bool
}
```

### Semantic Bridging

goja's eventloop uses Node.js semantics (auto-exit when idle), but our tournament interface requires blocking until explicit shutdown. The adapter bridges this gap using `StartInForeground()`.

### Performance Metrics (Tournament Evaluated)

**Tournament Results (COMPREHENSIVE_TOURNAMENT_EVALUATION.md):**

| Platform | Metric | Baseline | vs Main |
|----------|--------|----------|----------|
| **macOS** | PingPongLatency | 510.3 ns | 23% slower (2nd best) |
| **macOS** | PingPongThroughput | 98.81 ns/op | 18% slower |
| **macOS** | MultiProducer | 494.8 ns/op | **4x slower** |
| **macOS** | GCPressure | 595.9 ns | 31% slower |
| **Linux** | PingPongLatency | 597.4 ns | 19% slower (2nd best) |
| **Linux** | PingPongThroughput | 88.17 ns/op | 64% slower |
| **Linux** | MultiProducer | 194.7 ns/op | 54% slower |
| **Linux** | GCPressure | 2,347 ns | **73% slower** |

**Weighted Score:**
- **macOS**: 65.4/100 (Limited by MultiProducer)
- **Linux**: 71.8/100 (Limited by GCPressure)
- **Overall**: 68.6/100 (28% worse than Main, 2nd place overall)

**Why Competitive on Latency:**

See `ANALYSIS_BASELINE_LATENCY_INVESTIGATION.md`:
- goja_nodejs internally uses **channel-based tight loop** (same pattern as Main's fast path)
- Result: 510ns latency vs Main's 415ns (only 23% worse)
- **Proof**: Baseline's `RunOnLoop()` implements channel select pattern with batch drain

**Why Fails on Other Benchmarks:**
- **MultiProducer**: 4x slower under contention (494.8ns vs 124.6ns on macOS)
- **GCPressure**: 73% slower (2,347ns vs 1,355ns on Linux)
- **Throughput**: 18-64% slower (88-98ns vs 53-83ns)

### When to Use

This implementation is used only for **tournament benchmarking** as the baseline reference. Not recommended for production use.

---

## Tournament Interface

All implementations satisfy the common tournament interface:

```go
type EventLoop interface {
    // Run begins the event loop and BLOCKS until fully stopped
    Run(ctx context.Context) error

    // Shutdown gracefully shuts down and BLOCKS until complete
    Shutdown(ctx context.Context) error

    // Submit submits a task to the external queue
    Submit(fn func()) error

    // SubmitInternal submits a task to the internal priority queue
    SubmitInternal(fn func()) error

    // Close immediately terminates without graceful shutdown
    Close() error
}
```

---

## Performance Comparison Matrix

### macOS (kqueue)

| Implementation | PingPong Latency | vs Main | vs Baseline |
|----------------|------------------|---------|-------------|
| **Main** | 407.4 ns/op | — | +18.7% |
| Baseline | 500.9 ns/op | -19% | — |
| AlternateThree | 9,552 ns/op | -2,243% | -1,808% |
| AlternateOne | 9,634 ns/op | -2,264% | -1,824% |
| AlternateTwo | 9,731 ns/op | -2,288% | -1,843% |

### Linux (epoll)

| Implementation | PingPong Latency | vs Main | vs Baseline |
|----------------|------------------|---------|-------------|
| **Main** | 503.8 ns/op | — | +15.7% |
| Baseline | 597.4 ns/op | -15.7% | — |
| AlternateOne | ~86.07 ns/op* | — | — |
| AlternateTwo | ~112.3 ns/op* | — | — |
| AlternateThree | ~394.0 ns/op* | — | — |

*Different benchmark mode (throughput vs latency)

### Multi-Producer Throughput (Lower = Better)

| Implementation | macOS | Linux |
|----------------|-------|-------|
| **Main** | ~125 ns/op | 126.6 ns/op |
| AlternateOne | ~180 ns/op | 165.4 ns/op |
| AlternateTwo | — | 179.2 ns/op |
| AlternateThree | — | 308.3 ns/op |
| Baseline | ~495 ns/op | 194.7 ns/op |

---

## Decision Matrix: Which Implementation to Use?

```
                    ┌──────────────────────────────────────────────┐
                    │           IMPLEMENTATION SELECTOR            │
                    └──────────────────────────────────────────────┘
                                        │
                                        ▼
                    ┌──────────────────────────────────────────────┐
                    │  Are you debugging or developing a feature?  │
                    └──────────────────────────────────────────────┘
                            │                    │
                          Yes                   No
                            │                    │
                            ▼                    ▼
               ┌───────────────────┐  ┌──────────────────────────┐
               │   AlternateOne    │  │ Is maximum latency the   │
               │  (Maximum Safety) │  │ absolute top priority?   │
               └───────────────────┘  └──────────────────────────┘
                                              │           │
                                            Yes          No
                                              │           │
                                              ▼           ▼
                                 ┌─────────────────┐ ┌─────────────┐
                                 │  AlternateTwo   │ │    Main     │
                                 │ (Max Perf)*     │ │ (Balanced)  │
                                 └─────────────────┘ └─────────────┘

* AlternateTwo has known trade-offs - use only if you accept the risks
```

---

## Testing All Implementations

### Run All Tests

```bash
# Main implementation
go test -v -race ./eventloop/...

# All alternates
go test -v -race ./eventloop/internal/alternateone/...
go test -v -race ./eventloop/internal/alternatetwo/...
go test -v -race ./eventloop/internal/alternatethree/...
go test -v -race ./eventloop/internal/tournament/...
```

### Run Tournament Benchmarks

```bash
# Full tournament
make tournament-benchmark

# Linux-specific (via Docker)
make bench-linux-docker
```

### Stress Testing

```bash
# 100 iterations with race detector
go test -v -race -count=100 ./eventloop/internal/...
```

---

## Conclusion

The eventloop package provides a spectrum of implementations for different use cases:

| Priority | Recommended |
|----------|-------------|
| **Production** | Main |
| **Debugging** | AlternateOne |
| **Ultra-Low Latency** | AlternateTwo (with caution) |
| **Reference/Learning** | AlternateThree, Baseline |

The **Main implementation** is recommended for all production workloads. It provides the best balance of performance, safety, and maintainability, having won the tournament against all alternatives.

---

## Appendix: Investigation References

This document summarizes findings from the comprehensive tournament evaluation. For detailed analysis:

**Primary Documents:**
1. **FINAL_RECOMMENDATION_EVALUATION.md** - Mathematical proof of Main's superiority (87.9/100 score)
2. **COMPREHENSIVE_TOURNAMENT_EVALUATION.md** - Full tournament results (779 data points, 6 benchmarks, 2 platforms)
3. **TOURNAMENT_REPORT_2026-01-18.md** - Initial tournament report summary

**Investigation Documents:**
4. **ANALYSIS_LATENCY_INVESTIGATION.md** - Why alternates have 22-24x worse latency (missing fast path)
5. **ANALYSIS_ALTERNATETHREE_LINUX_INVESTIGATION.md** - Why AlternateThree fails catastrophically on Linux (eventfd overhead)
6. **ANALYSIS_GC_PRESSURE_INVESTIGATION.md** - Why AlternateTwo shines under GC pressure (TaskArena + lock-free)
7. **ANALYSIS_BASELINE_LATENCY_INVESTIGATION.md** - Why Baseline is competitive (goja_nodejs uses channel tight loop)
8. **ANALYSIS_RUNNING_VS_SLEEPING.md** - Running vs Sleeping anomaly (background goroutine contention)

**Platform-Specific Reports:**
9. **LINUX_BENCHMARK_REPORT_2026-01-18.md** - Linux-specific results and analysis
10. **FINAL_SUMMARY_FOR_HANA.md** - Executive summary document

---

*Document version: 2.0 (Updated with tournament evaluation findings)*
*Last updated: 2026-01-19*
