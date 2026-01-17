# Super-Document: Event Loop PR Analysis & Verification

## Executive Summary

This document synthesizes analysis of a Go event loop implementation PR that transitions from "best-effort" to "correct-by-construction" concurrency semantics. The analysis reveals **one critical blocker**, **two confirmed bugs**, and multiple verification strategies.

### Critical Issues Identified

1. **CRITICAL BLOCKER**: Fast Path thread affinity violation (Severity: Catastrophic)
2. **CONFIRMED BUG**: `MicrotaskRing.IsEmpty()` logic error (Severity: High)
3. **CONFIRMED BUG**: Data race in `Loop.tick()` accessing `tickAnchor` (Severity: Medium)
4. **MINOR**: Typo "Recomendation" → "Recommendation"

---

## 1. Critical Defect: Fast Path Thread Affinity Violation

### Problem Statement

**Location**: `eventloop/loop.go`, `SubmitInternal()` method, lines 728-735

The "Fast Path" optimization executes tasks immediately on the **caller's goroutine** when:
- Loop state is `StateRunning`
- External queue is empty
- `fastPathEnabled` flag is true

```go
if l.fastPathEnabled.Load() && state == StateRunning {
    if l.external.Length() == 0 {
        l.safeExecute(task) // EXECUTES ON CALLER'S THREAD
        return nil
    }
}
```

### Root Cause

`SubmitInternal()` is a public API callable from any goroutine. When the fast path triggers, it executes the task synchronously on the calling goroutine instead of the event loop's dedicated goroutine, violating the fundamental single-threaded execution guarantee of the reactor pattern.

### Impact

- **Data races** on user state that assumes single-threaded access
- **Memory corruption** in maps, slices, or other non-thread-safe structures
- **Violation of actor model semantics** that the event loop guarantees

### Required Fix

Add thread affinity check:

```go
if l.fastPathEnabled.Load() && state == StateRunning && l.isLoopThread() {
    if l.external.Length() == 0 {
        l.safeExecute(task)
        return nil
    }
}
```

### Proof of Defect

**Test**: `TestLoop_StrictThreadAffinity`

```go
func TestLoop_StrictThreadAffinity(t *testing.T) {
    l, _ := New()
    l.SetFastPathEnabled(true)
    
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    go l.Run(ctx)
    
    time.Sleep(10 * time.Millisecond)
    
    var loopID, taskID uint64
    var wg sync.WaitGroup
    wg.Add(1)
    
    // Capture loop's goroutine ID
    l.Submit(Task{Runnable: func() {
        loopID = getGoroutineID()
    }})
    
    // Submit from external goroutine
    go func() {
        l.SubmitInternal(Task{Runnable: func() {
            taskID = getGoroutineID()
            wg.Done()
        }})
    }()
    
    wg.Wait()
    
    if taskID != loopID {
        t.Fatalf("Thread Affinity Violated: Loop ID=%d, Task ID=%d", 
            loopID, taskID)
    }
}
```

**Expected Results**:
- **Before fix**: FAIL (different goroutine IDs)
- **After fix**: PASS (same goroutine ID)

**Additional verification**: Run with `-race` flag for race detection on shared state.

---

## 2. Confirmed Bug: MicrotaskRing.IsEmpty() Logic Error

### Problem Statement

**Location**: `eventloop/ingress.go`, `IsEmpty()` method

```go
r.overflowMu.Lock()
empty := len(r.overflow) == 0  // INCORRECT
r.overflowMu.Unlock()
```

### Root Cause

The overflow buffer uses a head-index pattern where valid items are `[overflowHead:len]`. After consuming all items, `overflowHead == len(r.overflow)` but the slice length remains non-zero until compaction (which only occurs when `overflowHead > 512`).

`IsEmpty()` checks `len(r.overflow) == 0` but `Length()` correctly uses `len(r.overflow) - r.overflowHead`.

### Impact

- `IsEmpty()` returns `false` when the ring is logically empty
- Inconsistency with `Length()` method
- Affects `poll()` function decisions on blocking behavior
- Performance degradation from unnecessary early returns

### Required Fix

```go
r.overflowMu.Lock()
empty := len(r.overflow) - r.overflowHead == 0  // CORRECT
r.overflowMu.Unlock()
```

### Proof of Defect

**Test**: `TestMicrotaskRing_IsEmpty_BugWhenOverflowNotCompacted`

```go
func TestMicrotaskRing_IsEmpty_BugWhenOverflowNotCompacted(t *testing.T) {
    r := NewMicrotaskRing()
    
    // Fill ring (4096 items)
    for i := 0; i < 4096; i++ {
        r.Push(func() {})
    }
    
    // Add to overflow (below compaction threshold of 512)
    for i := 0; i < 100; i++ {
        r.Push(func() {})
    }
    
    // Drain all items
    for r.Pop() != nil {}
    
    // Verify inconsistency
    length := r.Length()
    isEmpty := r.IsEmpty()
    
    if length != 0 {
        t.Errorf("Length() = %d after drain, want 0", length)
    }
    
    if !isEmpty {
        t.Errorf("BUG: IsEmpty()=false but Length()=0")
    }
    
    // Invariant check
    if (length == 0) != isEmpty {
        t.Errorf("INVARIANT VIOLATION: (Length()==0)=%v, IsEmpty()=%v",
            length == 0, isEmpty)
    }
}
```

**Expected Results**:
- **Before fix**: FAIL at invariant check
- **After fix**: PASS

---

## 3. Confirmed Bug: Data Race in Loop.tick()

### Problem Statement

**Location**: `eventloop/loop.go`, `tick()` method

```go
elapsed := time.Since(l.tickAnchor)  // UNSYNCHRONIZED READ
```

### Root Cause

- `SetTickAnchor()` writes `l.tickAnchor` under `tickAnchorMu.Lock()`
- `CurrentTickTime()` reads under `tickAnchorMu.RLock()`
- `tick()` reads **without any lock**

This creates a data race when `SetTickAnchor()` is called while the loop is running.

### Impact

- Race detector reports violation
- Potential for reading partially-written time values (though unlikely on 64-bit systems with aligned access)
- Violates Go memory model

### Required Fix

```go
// BEFORE (race):
elapsed := time.Since(l.tickAnchor)

// AFTER (safe):
l.tickAnchorMu.RLock()
anchor := l.tickAnchor
l.tickAnchorMu.RUnlock()
elapsed := time.Since(anchor)
```

### Proof of Defect

**Test**: `TestLoop_TickAnchor_DataRace`

```go
func TestLoop_TickAnchor_DataRace(t *testing.T) {
    loop, _ := New()
    
    ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
    defer cancel()
    
    // Start loop
    go func() { loop.Run(ctx) }()
    time.Sleep(5 * time.Millisecond)
    
    // Concurrent writes to tickAnchor
    var wg sync.WaitGroup
    for i := 0; i < 5; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 50; j++ {
                loop.SetTickAnchor(time.Now())
                time.Sleep(time.Millisecond)
            }
        }()
    }
    
    wg.Wait()
    cancel()
}
```

**Expected Results**:
- **Before fix**: `go test -race` reports `WARNING: DATA RACE`
- **After fix**: No race warning

---

## 4. Verified Correct Implementations

### 4.1 MPSC Queue Acausality Fix

**Location**: `eventloop/ingress.go`, `Pop()` and `PopBatch()`

**Status**: ✅ **CORRECT**

The spin-wait logic correctly handles the window between `tail.Swap()` and `prev.next.Store()`:

```go
if next == nil {
    if q.tail.Load() == head {
        return Task{}, false  // Truly empty
    }
    // Producer swapped tail but hasn't linked yet - spin-wait
    for {
        next = head.next.Load()
        if next != nil { break }
        runtime.Gosched()
    }
}
```

**Analysis**:
- Correctly distinguishes "empty queue" from "producer in-flight"
- Spin-wait preserves linearizability
- Trade-off: Blocking consumer if producer is preempted (acceptable for this design)

### 4.2 Microtask Ring Memory Ordering

**Location**: `eventloop/ingress.go`, `MicrotaskRing` Push/Pop

**Status**: ✅ **CORRECT** (aside from the `IsEmpty()` bug)

**Push ordering**:
```go
buffer[i] = fn              // Data write
seq[i].Store(s)             // Release barrier
```

**Pop ordering**:
```go
s := seq[i].Load()          // Acquire barrier
if s == head {
    fn := buffer[i]         // Data read
}
```

**Cleanup ordering**:
```go
buffer[i] = nil
seq[i].Store(0)
head.Add(1)
```

This Release/Acquire pairing ensures consumers never see uninitialized data.

### 4.3 Poller Initialization

**Location**: `eventloop/poller_*.go`

**Status**: ✅ **CORRECT**

Using `sync.Once` correctly ensures:
1. Exactly one initialization execution
2. All concurrent callers block until completion
3. Proper error propagation via `initErr`

```go
once.Do(func() {
    if p.closed.Load() {
        initErr = ErrPollerClosed
        return
    }
    // ... initialization
})
```

### 4.4 Monotonic Time Handling

**Location**: `eventloop/loop.go`, timer scheduling

**Status**: ✅ **CORRECT** (aside from the data race)

```go
func (l *Loop) CurrentTickTime() time.Time {
    l.tickAnchorMu.RLock()
    anchor := l.tickAnchor
    l.tickAnchorMu.RUnlock()
    
    elapsed := time.Duration(l.tickElapsedTime.Load())
    return anchor.Add(elapsed)
}
```

`time.Since(anchor)` correctly uses the monotonic clock, making the system immune to wall-clock adjustments.

### 4.5 Shutdown Drain Logic

**Location**: `eventloop/loop.go`, shutdown implementation

**Status**: ✅ **CORRECT**

The state machine correctly allows `Submit` during `StateTerminating`:

```go
if state == StateTerminated {
    return ErrLoopTerminated
}
// StateTerminating still accepts tasks
```

The `inflight` counter acts as a read-lock on loop liveness:
1. `Submit` increments `inflight` before checking state
2. `shutdown` spins on `inflight > 0`
3. `shutdown` drains queues after `inflight == 0`

This prevents lost tasks during shutdown.

---

## 5. Performance & Design Concerns

### 5.1 Poller Dynamic Growth

**Location**: `eventloop/poller_*.go`, `RegisterFD()`

**Issue**: Naive growth strategy

```go
newSize := fd*2 + 1
newFds := make([]fdInfo, newSize)
copy(newFds, p.fds)  // O(N) copy under lock
```

**Impact**:
- Registering FD 50,000,000 allocates ~1.2GB contiguous memory
- O(N) copy blocks all I/O operations during resize
- Potential OOM or GC pause

**Mitigation Options**:
1. Use paginated/chunked approach: `[][1024]fdInfo`
2. Use sparse map for high FDs: `map[int]fdInfo` with range checks
3. Document that sequential FD allocation is assumed

**Current Status**: Acceptable if FDs are sequential (typical OS behavior), but document this assumption.

### 5.2 Lock-Free Queue Liveness

**Concern**: Producer preemption in critical window

If a producer is preempted between `tail.Swap()` and `next.Store()`, the consumer enters an indefinite spin-loop consuming 100% CPU.

**Analysis**: This is a deliberate trade-off for:
- Wait-free producer (never blocks)
- Zero-allocation push operation
- Acceptable risk in practice (preemption window is nanoseconds)

**Status**: Document this limitation clearly.

---

## 6. Comprehensive Test Suite

### 6.1 Critical Defect Tests

```bash
# Test fast path violation
go test -race -v -run TestLoop_StrictThreadAffinity ./eventloop/

# Test IsEmpty() bug
go test -v -run TestMicrotaskRing_IsEmpty_BugWhenOverflowNotCompacted ./eventloop/

# Test data race
go test -race -v -run TestLoop_TickAnchor_DataRace ./eventloop/
```

### 6.2 Stress Tests

**Ingress Queue Torture Test**:
```go
func TestLockFreeIngress_Acausality(t *testing.T) {
    q := NewLockFreeIngress()
    
    const producers = 50
    const itemsPerProducer = 10000
    const totalItems = producers * itemsPerProducer
    
    var processedCount atomic.Int64
    
    // Single consumer (hot loop)
    doneCh := make(chan struct{})
    go func() {
        defer close(doneCh)
        for {
            task, ok := q.Pop()
            if ok {
                task.Runnable()
                processedCount.Add(1)
            }
            if processedCount.Load() == int64(totalItems) {
                return
            }
        }
    }()
    
    // Multiple producers
    var wg sync.WaitGroup
    wg.Add(producers)
    for i := 0; i < producers; i++ {
        go func() {
            defer wg.Done()
            for j := 0; j < itemsPerProducer; j++ {
                q.Push(func() {})
            }
        }()
    }
    
    wg.Wait()
    
    select {
    case <-doneCh:
        // Success
    case <-time.After(5 * time.Second):
        t.Fatalf("Timeout: Processed %d/%d items", 
            processedCount.Load(), totalItems)
    }
}
```

**Shutdown Drain Test**:
```go
func TestLoop_ShutdownDrain(t *testing.T) {
    l, _ := New()
    ctx, cancel := context.WithCancel(context.Background())
    
    go l.Run(ctx)
    time.Sleep(10 * time.Millisecond)
    
    // Submit blocking task
    startedBlocking := make(chan struct{})
    l.Submit(Task{Runnable: func() {
        close(startedBlocking)
        time.Sleep(50 * time.Millisecond)
    }})
    <-startedBlocking
    
    // Trigger shutdown while loop is busy
    cancel()
    
    // Submit task during StateTerminating
    drainTaskExecuted := make(chan struct{})
    err := l.Submit(Task{Runnable: func() {
        close(drainTaskExecuted)
    }})
    
    if err != nil {
        t.Fatalf("Submit rejected during drain: %v", err)
    }
    
    // Verify task executed
    select {
    case <-drainTaskExecuted:
        // Success
    case <-time.After(2 * time.Second):
        t.Fatal("Task not executed during drain")
    }
}
```

### 6.3 Regression Suite

```go
func TestRegression_AllDefects(t *testing.T) {
    t.Run("FastPath_ThreadAffinity", TestLoop_StrictThreadAffinity)
    t.Run("IsEmpty_OverflowBug", TestMicrotaskRing_IsEmpty_BugWhenOverflowNotCompacted)
    t.Run("IsEmpty_Consistency", TestMicrotaskRing_IsEmpty_LengthConsistency)
    t.Run("TickAnchor_Race", TestLoop_TickAnchor_DataRace)
    t.Run("Ingress_Acausality", TestLockFreeIngress_Acausality)
    t.Run("Shutdown_Drain", TestLoop_ShutdownDrain)
}
```

---

## 7. Required Actions Before Merge

### BLOCKER (Must Fix)

1. **Fix Fast Path thread affinity**:
   - Add `l.isLoopThread()` check to fast path condition
   - OR disable fast path entirely if thread affinity cannot be guaranteed

### HIGH Priority (Should Fix)

2. **Fix `MicrotaskRing.IsEmpty()`**:
   ```go
   empty := len(r.overflow) - r.overflowHead == 0
   ```

3. **Fix `Loop.tick()` data race**:
   ```go
   l.tickAnchorMu.RLock()
   anchor := l.tickAnchor
   l.tickAnchorMu.RUnlock()
   elapsed := time.Since(anchor)
   ```

### LOW Priority (Nice to Have)

4. **Fix typo**: "Recomendation" → "Recommendation"

5. **Add documentation**:
   - Document fast path thread affinity requirement
   - Document lock-free queue blocking behavior
   - Document poller growth assumptions (sequential FDs)

---

## 8. Verification Commands

```bash
# Full verification suite
go test -race -count=100 -timeout=10m ./eventloop/...

# Specific defect tests
go test -v -run TestRegression_AllDefects ./eventloop/

# Race detection
go test -race -v -run "TestLoop_TickAnchor|TestLoop_StrictThreadAffinity" ./eventloop/

# Stress tests
go test -v -run "TestLockFreeIngress_Acausality|TestIntegrity" ./eventloop/
```

---

## 9. Certification Status

**CANNOT CERTIFY FOR MERGE** due to critical fast path violation.

**After applying required fixes**:
- All tests in Section 6 must pass
- No race warnings with `-race` flag
- Stress tests pass with `-count=100`

**Then certification becomes**: ✅ **APPROVED FOR MERGE**
