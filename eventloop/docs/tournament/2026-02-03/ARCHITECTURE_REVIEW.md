# Eventloop Architecture Review

**Review Date:** 2026-02-03  
**Reviewer:** Architecture Review Subagent  
**Scope:** Main eventloop package and alternate implementations  

---

## 1. Architecture Overview

### 1.1 Core Design Philosophy

The eventloop package implements a high-performance asynchronous event loop for Go, designed with a performance-first philosophy that prioritizes throughput and low latency while maintaining correctness. The architecture follows a multi-layered design with clear separation of concerns across several key subsystems:

- **Event Processing Pipeline**: A tick-based loop that processes external tasks, internal tasks, timers, and microtasks in priority order
- **State Machine**: Lock-free CAS-based state transitions with non-sequential state values to preserve stable serialization
- **Ingress System**: Mutex + chunked queue (ChunkedIngress) that benchmarks showed outperforms lock-free CAS under high contention
- **Poller Abstraction**: Platform-specific I/O polling (epoll on Linux, kqueue on Darwin) with direct FD indexing
- **Promise Implementation**: Promise/A+ compliant implementation with microtask scheduling
- **Wakeup Mechanisms**: Dual-channel wakeup (fast path via channel, I/O path via eventfd/pipe)

### 1.2 State Machine Design

The loop state machine uses five non-sequential states (0, 1, 2, 4, 5) to ensure stable serialization and prevent accidental integer overflow issues. The transitions follow a CAS-based pattern:

```
StateAwake (0) ──────► StateRunning (4)     [Run()]
StateRunning (4) ────► StateSleeping (2)    [poll() via CAS]
StateRunning (4) ────► StateTerminating (5) [Shutdown()]
StateSleeping (2) ──► StateRunning (4)     [poll() wake via CAS]
StateSleeping (2) ──► StateTerminating (5) [Shutdown()]
StateTerminating (5) ► StateTerminated (1)  [shutdown complete]
```

The `FastState` implementation uses cache-line padding to prevent false sharing between cores, achieving pure atomic CAS performance without validation overhead.

### 1.3 Event Processing Pipeline

The tick-based processing follows a clear priority order:

1. **Queue Depth Metrics Recording** - Optional runtime metrics update
2. **Timer Execution** (`runTimers()`) - Binary heap-based timer management
3. **Internal Queue Processing** (`processInternalQueue()`) - High-priority internal tasks
4. **External Task Processing** (`processExternal()`) - Batch processing with 1024-task budget
5. **AuxJobs Drain** (`drainAuxJobs()`) - Cleanup from fast path mode transitions
6. **Microtask Drain** (`drainMicrotasks()`) - Promise handlers and microtasks
7. **I/O Poll** (`poll()`) - Blocking wait for I/O events or wakeup

The microtask budget (1024 per drain) prevents starvation while ensuring bounded latency.

---

## 2. Key Design Patterns and Their Effectiveness

### 2.1 Mutex + Chunked Ingress (Performance-Adaptive)

**Pattern:** Mutex-protected chunked linked-list queue instead of lock-free CAS

**Rationale:** Benchmarks demonstrated that mutex + chunking outperforms lock-free CAS under high contention due to O(N) retry storms inherent in CAS-based approaches.

**Effectiveness:** ✅ **High**
- Fixed 128-task chunks provide cache locality
- `sync.Pool` chunk recycling prevents GC thrashing
- O(1) push/pop without array shifting
- Per-slot nil clearing prevents memory leaks

**Trade-off Note:** The R101 fix (ring buffer sequence zero edge case) revealed that even with chunked queues, under extreme concurrent producer load (>50 producers × 1000 items), sequence number wrap-around could cause infinite spin. This was mitigated by adding explicit validity flags.

### 2.2 Dual-Path Wakeup (Performance-Adaptive)

**Pattern:** Fast path uses channel-based wakeup (~50ns), I/O path uses eventfd/pipe (~10µs)

**Implementation:**
```go
// Fast mode (no user I/O FDs)
select {
case <-l.fastWakeupCh:  // ~50ns latency
}

// I/O mode (user FDs registered)
unix.Write(l.wakePipe, buf)  // ~10µs latency
```

**Effectiveness:** ✅ **High**
- Eliminates unnecessary system calls for task-only workloads
- Buffered channel (size 1) provides automatic wakeup deduplication
- Dual-wakeup during mode transition prevents starvation

### 2.3 Microtask Ring Buffer (Lock-Free MPSC)

**Pattern:** Lock-free ring buffer with Release/Acquire semantics for microtask scheduling

**R101 Fix:** Added explicit validity flags (`atomic.Bool`) to distinguish empty slots from sequence wrap-around. Previously, `seq==0` was used as sentinel, but under extreme load sequence numbers can legitimately wrap to 0.

**Memory Ordering:**
```go
// Push: Write Data → Write Validity → Store Seq (Release)
// Pop:  Load Seq (Acquire) → Check Validity → Read Data
```

**Effectiveness:** ✅ **High**
- Zero allocations on hot path (pre-allocated 4096 slots)
- Overflow mutex-protected slice maintains FIFO ordering
- Cache-line padding prevents false sharing

### 2.4 Promise/A+ Implementation

**Pattern:** ChainedPromise with Then/Catch/Finally support and JavaScript-compatible semantics

**Key Design Decisions:**
- Handlers execute as microtasks on the event loop thread
- Resolve/reject functions callable from any goroutine
- Cycle detection prevents infinite recursion
- Promise state snapshots before clearing handlers (T27 fix)

**T27 Fix (Critical):**
```go
// CRITICAL FIX: Snapshot handlers BEFORE clearing them
// then() runs concurrently: checks state, then acquires p.mu
// Solution: Don't clear handlers until AFTER processing them
h0 := p.h0
handlers := p.result.([]handler)
// ... process handlers ...
p.h0 = handler{}
p.mu.Unlock()
```

**Effectiveness:** ✅ **High**
- Promise/A+ compliant (verified through test suite)
- Concurrent-safe resolution/rejection
- Handler microtask ordering deterministic

### 2.5 Weak Reference Registry

**Pattern:** Ring buffer + weak pointers for promise lifecycle management

**Design:**
- Weak pointers allow GC of settled promises
- Ring buffer provides deterministic scavenging over time
- Batch scavenging (limited per tick) prevents stalling

**Effectiveness:** ✅ **Moderate**
- Memory-efficient promise tracking
- Prevents promise leaks from unhandled rejections
- Scavenging limit (20 per tick) ensures bounded pause

---

## 3. Concurrency and Safety Assessment

### 3.1 Lock-Free Primitives Assessment

| Component | Safety Model | Assessment |
|-----------|--------------|------------|
| FastState | Pure CAS | ✅ Safe - no validation, trusts writer |
| ChunkedIngress | Mutex-protected | ✅ Safe - single lock, no race windows |
| MicrotaskRing | Release/Acquire | ✅ Safe - R101 validity flags prevent wrap-around bugs |
| TPSCounter | Atomics + periodic mutex | ✅ Safe - bucket rotation protected by mutex |
| Promise | Mutex + state snapshot | ✅ Safe - T27 fix prevents handler loss |

### 3.2 Identified Concurrency Risks

#### Risk 1: Fast Path Mode Transition Race
**Severity:** Medium
**Location:** `loop.go:hasExternalTasks()` and `Submit()`
**Description:** Between checking `canUseFastPath()` and acquiring mutex, mode can change, causing task to go to `auxJobs` instead of `ChunkedIngress`.
**Mitigation:** `drainAuxJobs()` called after every poll to handle stranded tasks.

#### Risk 2: Timer Heap Modification During Iteration
**Severity:** Low
**Location:** `runTimers()`
**Description:** Timer callbacks can register new timers or modify the heap.
**Mitigation:** Binary heap operations are O(log n), concurrent modifications during iteration are tolerated (some timers may fire in same tick).

#### Risk 3: Promisify Goroutine Leak on Goexit
**Severity:** Low
**Location:** `promisify.go`
**Description:** If user goroutine calls `runtime.Goexit()`, promise would hang indefinitely.
**Mitigation:** `completed` flag + defer checks for non-normal return, rejects promise with `ErrGoexit`.

### 3.3 Platform-Specific Considerations

| Platform | Mechanism | Safety Notes |
|----------|-----------|--------------|
| Linux | epoll + eventfd | Safe - eventfd handles wakeup reliably |
| Darwin | kqueue + pipe | Safe - pipe wakeup tested |
| Windows | IOCP + OVERLAPPED | Safe - PostQueuedCompletionStatus for wakeup |

---

## 4. Comparison with Alternate Implementations

### 4.1 Variant Overview

| Implementation | Philosophy | Score | Throughput | P99 Latency |
|----------------|------------|-------|------------|-------------|
| **Main (current)** | Performance-First | 82/100 | ~700K ops/s | ~150µs |
| **AlternateOne** | Safety-First | 65/100 | ~100K ops/s | ~100µs |
| **AlternateTwo** | Maximum Performance | 95/100 | ~1M ops/s | ~30µs |
| **AlternateThree** | Balanced | 76/100 | ~556K ops/s | ~570.5µs |

### 4.2 AlternateOne (Safety-First) Comparison

**Key Differences:**

| Aspect | Main | AlternateOne |
|--------|------|--------------|
| Lock granularity | Fine (RWMutex per subsystem) | Coarse (single Mutex) |
| Invariant checks | Disabled in prod | Always enabled |
| Error handling | Silent drops | Explicit panics |
| Callback execution | Outside lock | Inside lock |
| Chunk clearing | Optimizable | Always full (128 slots) |
| State transitions | CAS only | CAS + validation |
| Poll locking | RLock during poll | Lock (write lock) |
| Check-then-sleep | Unlock-check-relock | Lock held through decision |

**Architecture Assessment:** AlternateOne trades throughput for debuggability. The coarse locking eliminates race windows but creates contention. The invariant validation provides fail-fast behavior critical for development.

### 4.3 AlternateTwo (Maximum Performance) Comparison

**Key Differences:**

| Aspect | Main | AlternateTwo |
|--------|------|--------------|
| Queue | Mutex + chunked list | Lock-free MPSC |
| Poller FD storage | Dynamic slice | Direct array indexing |
| Timer | Binary heap | Hierarchical wheel |
| Memory | GC-managed pools | Arena + aggressive pooling |
| Callbacks | Collect-then-execute | Inline execution |
| Validation | Present | Minimal/skipped |
| Error handling | Comprehensive | Fast path only |

**Architecture Assessment:** AlternateTwo accepts significant safety trade-offs for performance:
- No invariant validation (bugs manifest as corruption)
- Optimistic locking (race conditions under extreme load)
- Direct array indexing (undefined behavior for FDs > 65535)
- Minimal error handling (some errors silently ignored)

### 4.4 AlternateThree (Balanced) Comparison

**Architecture Assessment:** AlternateThree was the original main implementation before the Phase 18 promotion of AlternateTwo concepts. It provides:
- Mutex-based ingress (simpler, correct)
- RWMutex for poller (concurrent reads)
- Full error handling
- Defense-in-depth chunk clearing
- Balanced performance characteristics

---

## 5. Architectural Strengths

### 5.1 Performance Optimizations

1. **Tick Budgeting**: 1024-task budget per phase prevents starvation
2. **Cache-Line Padding**: `FastState`, `MicrotaskRing` use explicit padding
3. **Batch Processing**: External tasks popped and executed in batches
4. **Pre-allocated Buffers**: Event buffers, task arenas minimize allocations
5. **Fast Path Channel**: ~50ns wakeup vs ~10µs for I/O path
6. **Chunk Recycling**: `sync.Pool` prevents GC thrashing

### 5.2 Correctness Guarantees

1. **State Machine Atomicity**: CAS transitions prevent torn reads/writes
2. **Handler Snapshotting**: T27 fix ensures no handlers lost during concurrent resolution
3. **Sequence Validity**: R101 fix prevents infinite spin under extreme load
4. **Promise Cycle Detection**: Prevents infinite recursion
5. **Shutdown Ordering**: Phased shutdown ensures all tasks complete

### 5.3 Extensibility

1. **LoopOption Pattern**: Configurable behavior via options
2. **Hook Injection**: `loopTestHooks` for deterministic testing
3. **Poller Abstraction**: Platform-specific implementations behind interface
4. **Metrics Integration**: Optional runtime metrics collection
5. **Structured Logging**: `logiface.Logger` integration

---

## 6. Architectural Weaknesses

### 6.1 Design Limitations

1. **Fixed FD Array**: Direct indexing caps at 65536 FDs (Alternatetwo)
2. **Single Consumer Pattern**: MicrotaskRing is MPSC, not MPMC
3. **Binary Heap Timer**: O(log n) operations vs O(1) for hierarchical wheel
4. **No Work Stealing**: Idle loops cannot steal from other loops
5. **No Priority Inversion Protection**: Critical tasks can be starved

### 6.2 Code Quality Issues

1. **TODO/FIXME Markers**: 23 outstanding markers across codebase
2. **TPS Counter Sorting**: O(n log n) percentile computation (IMP-001)
3. **Chunk Slot Clearing**: Incomplete clearing may leak references (IMP-002)
4. **Staticcheck S1040**: Some type assertions not fully DRY-refactored

### 6.3 Testing Gaps

1. **HandlePollError Path**: 0% coverage, difficult to trigger
2. **Darwin Wakeup Functions**: Platform-specific gaps
3. **ID Exhaustion Tests**: Skipped (require mocking)
4. **Timer ID Exhaustion**: No handling for MAX_SAFE_INTEGER overflow

---

## 7. Recommendations for Architectural Improvements

### 7.1 High Priority

#### 7.1.1 Timer Wheel Implementation
**Issue:** Binary heap has O(log n) operations; hierarchical wheel provides O(1)
**Recommendation:** Implement hierarchical wheel timer (Alternatetwo pattern):
```
struct TimerWheel {
    levels [4]RingBuffer  // ms, s, min, hour precision
    currentLevel int
    currentTick uint64
}
```

#### 7.1.2 TPS Counter Optimization
**Issue:** O(n log n) sorting in `Sample()` method
**Recommendation:** Implement P-Square or t-digest algorithm for O(1) percentiles
**Impact:** Enables sub-second metric resolution without performance impact

#### 7.1.3 Chunk Slot Clearing
**Issue:** `returnChunk()` doesn't clear all 128 slots
**Recommendation:** Match AlternateTwo's `returnChunkFast()` pattern:
```go
func returnChunk(c *chunk) {
    for i := 0; i < c.pos; i++ {
        c.tasks[i] = nil  // Clear only used slots
    }
    c.pos = 0
    // ...
}
```

### 7.2 Medium Priority

#### 7.2.1 Promise Handler Tracking Improvement
**Issue:** Promise handlers tracked separately from promises themselves
**Recommendation:** Embed handler list directly in promise structure to eliminate:
- `promiseHandlers` map maintenance
- Handler snapshot race complexity (T27 fix)
- Promise ID exhaustion concerns

#### 7.2.2 Structured Logging Enhancement
**Issue:** 6 `log.Printf` calls for error/panic cases
**Recommendation:** Complete structured logging migration:
- Add correlation ID tracking
- Context propagation for async operations
- Log level filtering

#### 7.2.3 Error Path Coverage
**Issue:** `handlePollError` has 0% test coverage
**Recommendation:** Add platform-specific error injection tests:
```go
func TestHandlePollError_PlatformSpecific(t *testing.T) {
    // Inject errno via testHooks.PollError
    // Verify graceful degradation
}
```

### 7.3 Low Priority

#### 7.3.1 FD Allocation Improvement
**Issue:** FD allocation uses map lookup
**Recommendation:** Implement slot-based allocator for O(1) registration:
```go
type FDAllocator struct {
    freeList []int
    bitmap [65536]atomic.Bool
}
```

#### 7.3.2 Multi-Loop Work Stealing
**Issue:** No work stealing between multiple event loops
**Recommendation:** Design work stealing queue for MP/MC scenarios:
```
Loop A ──────► StealQueue ◄────── Loop B
                ▲
                │ Steal
           Loop C ──────►
```

#### 7.3.3 Priority Queue Enhancement
**Issue:** No priority levels for timer tasks
**Recommendation:** Add priority tiers for timer callbacks:
```go
type Timer struct {
    priority uint8  // 0=low, 255=critical
    when    time.Time
    task    func()
}
```

---

## 8. Summary Assessment

### Overall Architecture Grade: **A-** (Strong)

The eventloop package demonstrates a well-engineered architecture that successfully balances performance, correctness, and maintainability. The key strengths are:

1. **Adaptive Performance**: Dual-path design (fast path vs I/O path) adapts to workload characteristics
2. **Battle-Tested Concurrency**: Multiple concurrent fixes (T27, R101, RV08-12) demonstrate real-world validation
3. **Clear Abstraction Boundaries**: Separated ingress, timer, promise, and poller concerns
4. **Comprehensive Testing**: 89%+ coverage with race detector validation

The main implementation represents a mature, production-grade event loop suitable for high-throughput JavaScript runtime integration (Goja) and general-purpose async programming.

### Key Files Reference

| File | Purpose | Lines |
|------|---------|-------|
| `loop.go` | Core event loop, tick processing | 1838 |
| `ingress.go` | ChunkedIngress + MicrotaskRing | 400 |
| `promise.go` | Promise/A+ implementation | 1187 |
| `js.go` | JavaScript timer/promise adapter | 557 |
| `metrics.go` | Runtime metrics collection | 383 |
| `state.go` | FastState machine | 116 |
| `poller_*.go` | Platform-specific polling | 281+ |
| `registry.go` | Weak promise registry | 209 |

---

*Generated by Architecture Review Subagent*  
*2026-02-03*
