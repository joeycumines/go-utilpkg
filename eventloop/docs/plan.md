# Eventloop Correctness Implementation Plan

> **Generated:** 2026-01-15
> **Source:** Consolidated analysis from [scratch.md](../../scratch.md)
> **Status:** READY FOR IMPLEMENTATION

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Dependency Graph](#dependency-graph)
3. [Phase 1: Critical Defects (Blocking)](#phase-1-critical-defects-blocking)
4. [Phase 2: High Severity Defects](#phase-2-high-severity-defects)
5. [Phase 3: Moderate Defects](#phase-3-moderate-defects)
6. [Phase 4: Low/Theoretical Defects](#phase-4-lowtheoretical-defects)
7. [Test Strategy](#test-strategy)
8. [CI/CD Integration](#cicd-integration)
9. [Rollout & Verification](#rollout--verification)
10. [Risk Assessment](#risk-assessment)

---

## Executive Summary

This plan addresses **10 identified defects** in the eventloop package, ordered by severity and dependencies. The fixes are organized into 4 phases:

| Phase | Defects | Estimated LOC | Risk Level |
|-------|---------|---------------|------------|
| 1 | #1, #2, #3, #4 | ~80 | HIGH |
| 2 | #5, #6 | ~30 | MEDIUM |
| 3 | #7, #8, #9 | ~40 | LOW |
| 4 | #10 | ~5 | MINIMAL |

---

## Dependency Graph

```
Phase 1 (CRITICAL - Must fix first):
┌─────────────────────────────────────────────────────────────┐
│  #1 initPoller Race ──┬──> #2 closed Data Race             │
│                       │    (both require ioPoller refactor)│
│                       │                                     │
│  #3 Pop Write-After-Free ──> (independent)                 │
│                                                             │
│  #4 FIFO Violation ──> (independent, but impacts #3 test)  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
Phase 2 (HIGH - After Phase 1):
┌─────────────────────────────────────────────────────────────┐
│  #5 PopBatch Inconsistency ──> (depends on #3 fix pattern) │
│                                                             │
│  #6 nil Infinite Loop ──> (depends on #3 fix pattern)      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
Phase 3 (MODERATE - After Phase 2):
┌─────────────────────────────────────────────────────────────┐
│  #7 closeFDs Double-Close ──> (independent)                │
│                                                             │
│  #8 Platform Error Inconsistency ──> (depends on #1)       │
│                                                             │
│  #9 pollIO Method Location ──> (cosmetic, after #1/#8)     │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
Phase 4 (THEORETICAL - Optional):
┌─────────────────────────────────────────────────────────────┐
│  #10 Sequence Wrap-Around ──> (independent)                │
└─────────────────────────────────────────────────────────────┘
```

---

## Phase 1: Critical Defects (Blocking)

### Defect #1: `initPoller()` Initialization Race

**Severity:** CRITICAL (Panic/Undefined Behavior)
**Files:** `poller_darwin.go`, `poller_linux.go`
**Consensus:** UNANIMOUS

#### Problem

The `initPoller` method uses `CompareAndSwap` but returns immediately when losing the CAS race—before `Init()` has completed on the winning goroutine.

```go
// CURRENT (BUGGY) - poller_darwin.go:248-260
func (p *ioPoller) initPoller() error {
    if p.closed {
        return errEventLoopClosed
    }
    if p.initialized.Load() {
        return nil
    }
    if !p.initialized.CompareAndSwap(false, true) {
        return nil  // <-- BUG: Returns before Init() completes!
    }
    if err := p.p.Init(); err != nil {
        p.initialized.Store(false)
        return err
    }
    return nil
}
```

#### Race Scenario

1. **Goroutine A** wins CAS, sets `initialized=true`, begins `p.p.Init()` (syscall)
2. **Goroutine B** loses CAS, returns `nil` immediately
3. **Goroutine B** calls `RegisterFD()` on uninitialized `FastPoller`
4. **CRASH:** `epfd=0` or unallocated map access

#### Fix

Replace `atomic.Bool` + `CompareAndSwap` with `sync.Once`:

```go
// FIXED - poller_darwin.go
type ioPoller struct {
    p      FastPoller
    once   sync.Once     // Replace: initialized atomic.Bool
    closed atomic.Bool   // Fix #2: Change from bool
    initErr error        // Store initialization error
}

func (p *ioPoller) initPoller() error {
    p.once.Do(func() {
        if p.closed.Load() {
            p.initErr = ErrPollerClosed
            return
        }
        p.initErr = p.p.Init()
    })
    return p.initErr
}
```

```go
// FIXED - poller_linux.go (same pattern)
type ioPoller struct {
    mu      sync.RWMutex  // Keep for test compatibility
    p       FastPoller
    once    sync.Once     // Replace: initialized atomic.Bool
    closed  atomic.Bool   // Fix #2: Change from bool
    initErr error
}

func (p *ioPoller) initPoller() error {
    p.once.Do(func() {
        if p.closed.Load() {
            p.initErr = ErrPollerClosed
            return
        }
        p.initErr = p.p.Init()
    })
    return p.initErr
}
```

#### Verification Test

Create `poller_init_race_test.go`:

```go
//go:build !race

package eventloop

import (
    "sync"
    "sync/atomic"
    "testing"
)

func TestPoller_InitRace_ConcurrentAccess(t *testing.T) {
    for iteration := 0; iteration < 100; iteration++ {
        var poller ioPoller
        var wg sync.WaitGroup
        var failures atomic.Int32
        start := make(chan struct{})

        for g := 0; g < 10; g++ {
            wg.Add(1)
            go func() {
                defer wg.Done()
                <-start
                if err := poller.initPoller(); err != nil {
                    return
                }
                // Attempt to use the poller
                defer func() {
                    if r := recover(); r != nil {
                        failures.Add(1)
                    }
                }()
                _ = poller.p.RegisterFD(1, EventRead, func(IOEvents) {})
            }()
        }

        close(start)
        wg.Wait()

        if failures.Load() > 0 {
            t.Fatalf("Iteration %d: Race condition - %d goroutines panicked", 
                iteration, failures.Load())
        }
        _ = poller.closePoller()
    }
}
```

---

### Defect #2: Data Race on `ioPoller.closed`

**Severity:** CRITICAL
**Files:** `poller_darwin.go`, `poller_linux.go`
**Note:** Fix is bundled with Defect #1

#### Problem

```go
// CURRENT (BUGGY) - poller_darwin.go
type ioPoller struct {
    // ...
    closed bool  // <-- Non-atomic!
}

func (p *ioPoller) closePoller() error {
    p.closed = true  // Non-atomic write
    // ...
}

func (p *ioPoller) initPoller() error {
    if p.closed {  // Non-atomic read - DATA RACE
        return errEventLoopClosed
    }
```

#### Fix

Already included in Defect #1 fix: change `closed bool` to `closed atomic.Bool`.

```go
// closePoller - both platforms
func (p *ioPoller) closePoller() error {
    p.closed.Store(true)
    // Reset once for potential reinitialization in tests
    p.once = sync.Once{}
    p.initErr = nil
    return p.p.Close()
}
```

#### Verification Test

```go
func TestIOPollerClosedDataRace(t *testing.T) {
    // Run with: go test -race -run TestIOPollerClosedDataRace -count=10
    for i := 0; i < 100; i++ {
        var poller ioPoller
        var wg sync.WaitGroup
        wg.Add(2)

        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                _ = poller.closePoller()
                poller.closed.Store(false)
                poller.once = sync.Once{}
            }
        }()

        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                _ = poller.initPoller()
            }
        }()

        wg.Wait()
    }
}
```

---

### Defect #3: `MicrotaskRing.Pop()` Write-After-Free Race

**Severity:** CATASTROPHIC (Deadlock/Data Loss)
**File:** `ingress.go`
**Lines:** 226-228 (current)

#### Problem

The code increments `head` (releasing the slot to producers) **before** clearing the sequence guard:

```go
// CURRENT (BUGGY) - ingress.go:226-228
r.head.Add(1)                    // [A] Slot released to producer
r.seq[(head)%4096].Store(0)      // [B] Guard cleared AFTER release
r.buffer[(head)%4096] = nil
```

#### Race Scenario

1. **Consumer** reads task from slot `i`, gets `fn`
2. **Consumer** executes `r.head.Add(1)` — slot `i` now "free" to producer
3. *Context switch*
4. **Producer** claims slot `i`, writes new task, stores sequence `S > 0`
5. **Consumer** resumes, executes `r.seq[i].Store(0)` — CLOBBERS producer's sequence
6. **Next Pop:** Sees `seq == 0`, spins forever waiting for "uncommitted" producer

#### Fix

Clear sequence guard **before** advancing head:

```go
// FIXED - ingress.go:Pop()
func (r *MicrotaskRing) Pop() func() {
    head := r.head.Load()
    tail := r.tail.Load()

    for head < tail {
        seq := r.seq[head%4096].Load()

        if seq == 0 {
            head = r.head.Load()
            tail = r.tail.Load()
            runtime.Gosched()
            continue
        }

        fn := r.buffer[head%4096]

        if fn == nil {
            // Fix #6: Must still advance head to avoid infinite loop
            r.buffer[head%4096] = nil
            r.seq[head%4096].Store(0)  // Clear guard FIRST
            r.head.Add(1)               // Release slot SECOND
            head = r.head.Load()
            tail = r.tail.Load()
            continue
        }

        // FIX #3: Correct ordering - clear THEN release
        r.buffer[head%4096] = nil       // Clear for GC
        r.seq[head%4096].Store(0)       // Clear guard FIRST
        r.head.Add(1)                   // Release slot SECOND

        return fn
    }

    // ... overflow handling unchanged ...
}
```

#### Verification Test (Torture)

Create `ingress_writeafterfree_test.go`:

```go
package eventloop

import (
    "runtime"
    "sync"
    "testing"
    "time"
)

func TestMicrotaskRing_WriteAfterFree_Race(t *testing.T) {
    ring := NewMicrotaskRing()
    const iterations = 1_000_000
    var wg sync.WaitGroup
    wg.Add(2)

    go func() {
        defer wg.Done()
        runtime.LockOSThread()
        defer runtime.UnlockOSThread()
        for i := 0; i < iterations; i++ {
            for !ring.Push(func() {}) {
                runtime.Gosched()
            }
        }
    }()

    go func() {
        defer wg.Done()
        runtime.LockOSThread()
        defer runtime.UnlockOSThread()
        count := 0
        for count < iterations {
            task := ring.Pop()
            if task == nil {
                runtime.Gosched()
                continue
            }
            count++
        }
    }()

    done := make(chan struct{})
    go func() {
        wg.Wait()
        close(done)
    }()

    select {
    case <-done:
        t.Logf("Success: %d items processed without deadlock", iterations)
    case <-time.After(30 * time.Second):
        t.Fatal("DEADLOCK DETECTED: Consumer stalled. Write-After-Free race proven.")
    }
}
```

---

### Defect #4: `MicrotaskRing` FIFO Violation

**Severity:** CRITICAL (Acausality)
**File:** `ingress.go`
**Function:** `Push()`

#### Problem

When the ring is full, tasks go to `overflow`. When space opens, new tasks bypass overflow and go directly to the ring—violating FIFO order.

```go
// CURRENT (BUGGY) - ingress.go:Push()
func (r *MicrotaskRing) Push(fn func()) bool {
    for {
        tail := r.tail.Load()
        head := r.head.Load()
        if tail-head >= 4096 {
            break // Ring full, go to overflow
        }
        if r.tail.CompareAndSwap(tail, tail+1) {
            // ... write to ring ...
            return true
        }
    }
    // Slow path: overflow
    r.overflowMu.Lock()
    r.overflow = append(r.overflow, fn)
    r.overflowMu.Unlock()
    return true
}
```

#### Race Scenario

1. **Saturation:** Ring fills to 4096 items
2. **Overflow:** Task A (seq 4097) goes to overflow (ring full)
3. **Drain:** Consumer pops one item, ring now has 4095
4. **Priority Inversion:** Task B (seq 4098) goes to RING (has space!)
5. **Result:** Task B processed before Task A — FIFO violated

#### Fix

If overflow is non-empty, **always append to overflow**:

```go
// FIXED - ingress.go:Push()
func (r *MicrotaskRing) Push(fn func()) bool {
    // FIX #4: Check overflow FIRST - preserve FIFO ordering
    r.overflowMu.Lock()
    if len(r.overflow) > 0 {
        r.overflow = append(r.overflow, fn)
        r.overflowMu.Unlock()
        return true
    }
    r.overflowMu.Unlock()

    // Fast path: try lock-free ring
    for {
        tail := r.tail.Load()
        head := r.head.Load()

        if tail-head >= 4096 {
            break // Ring full
        }

        if r.tail.CompareAndSwap(tail, tail+1) {
            seq := r.tailSeq.Add(1)
            // Fix #10: Skip sequence 0
            if seq == 0 {
                seq = r.tailSeq.Add(1)
            }

            r.buffer[tail%4096] = fn
            r.seq[tail%4096].Store(seq)

            return true
        }
    }

    // Slow path: ring full, use mutex-protected overflow
    r.overflowMu.Lock()
    if r.overflow == nil {
        r.overflow = make([]func(), 0, 1024)
    }
    r.overflow = append(r.overflow, fn)
    r.overflowMu.Unlock()
    return true
}
```

#### Verification Test (Deterministic)

```go
func TestMicrotaskRing_FIFO_Violation(t *testing.T) {
    r := NewMicrotaskRing()

    // 1. Saturate the Ring Buffer (4096 items)
    for i := 0; i < 4096; i++ {
        val := i
        r.Push(func() { _ = val })
    }

    // 2. Force Overflow - This is "Task A"
    var order []string
    r.Push(func() { order = append(order, "A") })

    // 3. Create Space in the Ring
    _ = r.Pop()

    // 4. Push "Task B" - should go to overflow (not ring!)
    r.Push(func() { order = append(order, "B") })

    // 5. Drain the Ring (4095 items)
    for i := 0; i < 4095; i++ {
        fn := r.Pop()
        if fn == nil {
            t.Fatalf("Expected item %d from ring", i)
        }
    }

    // 6. Pop Task A (must be first from overflow)
    fn := r.Pop()
    if fn == nil {
        t.Fatal("Expected Task A")
    }
    fn()

    // 7. Pop Task B
    fn = r.Pop()
    if fn == nil {
        t.Fatal("Expected Task B")
    }
    fn()

    // 8. Verify order
    if len(order) != 2 || order[0] != "A" || order[1] != "B" {
        t.Fatalf("FIFO VIOLATION: expected [A, B], got %v", order)
    }
}
```

---

## Phase 2: High Severity Defects

### Defect #5: `PopBatch` Inconsistency with `Pop`

**Severity:** HIGH
**File:** `ingress.go`
**Function:** `LockFreeIngress.PopBatch()`

#### Problem

`Pop()` correctly spin-waits when a producer has swapped tail but not yet linked `next`. `PopBatch()` does not—it exits immediately.

```go
// CURRENT (BUGGY) - ingress.go:PopBatch()
for count < max {
    next := head.next.Load()
    if next == nil {
        break  // <-- Should spin-wait like Pop() does!
    }
    // ...
}
```

#### Fix

Add spin-wait logic matching `Pop()`:

```go
// FIXED - ingress.go:PopBatch()
func (q *LockFreeIngress) PopBatch(buf []Task, max int) int {
    count := 0
    head := q.head.Load()

    if max > len(buf) {
        max = len(buf)
    }

    for count < max {
        next := head.next.Load()
        if next == nil {
            // FIX #5: Check if producer is in-flight
            if q.tail.Load() == head {
                break // Truly empty
            }
            // Producer swapped tail but hasn't linked next yet - spin
            for {
                next = head.next.Load()
                if next != nil {
                    break
                }
                runtime.Gosched()
            }
        }

        buf[count] = next.task
        next.task = Task{} // Clear for GC
        q.head.Store(next)

        if head != &q.stub {
            putNode(head)
        }

        head = next
        count++
    }

    if count > 0 {
        q.len.Add(int64(-count))
    }
    return count
}
```

#### Verification Test

```go
func TestPopBatch_WaitsForInFlightProducer(t *testing.T) {
    q := NewLockFreeIngress()
    
    // Producer will be blocked mid-push
    var wg sync.WaitGroup
    wg.Add(1)
    
    go func() {
        defer wg.Done()
        q.Push(func() {})
    }()
    
    // Give producer time to start
    time.Sleep(10 * time.Millisecond)
    
    // PopBatch should eventually get the task
    buf := make([]Task, 10)
    done := make(chan int)
    go func() {
        n := q.PopBatch(buf, 10)
        done <- n
    }()
    
    wg.Wait() // Ensure producer completes
    
    select {
    case n := <-done:
        if n != 1 {
            t.Fatalf("Expected 1 task, got %d", n)
        }
    case <-time.After(5 * time.Second):
        t.Fatal("PopBatch deadlocked")
    }
}
```

---

### Defect #6: `MicrotaskRing.Pop()` Infinite Loop on `nil` Input

**Severity:** HIGH (Liveness Failure)
**File:** `ingress.go`
**Function:** `MicrotaskRing.Pop()`

#### Problem

If `Push(nil)` is called, `Pop()` enters infinite loop: reads valid seq, reads `nil` fn, continues without advancing head.

```go
// CURRENT (BUGGY)
if fn == nil {
    head = r.head.Load()
    tail = r.tail.Load()
    continue  // <-- Never advances head!
}
```

#### Fix (Bundled with #3)

The fix in Defect #3 already handles this: when `fn == nil`, we still advance `head`:

```go
if fn == nil {
    r.buffer[head%4096] = nil
    r.seq[head%4096].Store(0)  // Clear guard
    r.head.Add(1)               // Advance head - FIX #6
    head = r.head.Load()
    tail = r.tail.Load()
    continue
}
```

#### Verification Test

```go
func TestMicrotaskRing_NilInput_NoInfiniteLoop(t *testing.T) {
    r := NewMicrotaskRing()
    
    // Push a nil (shouldn't happen, but defensive)
    r.Push(nil)
    r.Push(func() {})
    
    done := make(chan struct{})
    go func() {
        // Pop should skip nil and get the real task
        _ = r.Pop()
        _ = r.Pop()
        close(done)
    }()
    
    select {
    case <-done:
        // Success
    case <-time.After(1 * time.Second):
        t.Fatal("Pop() stuck in infinite loop on nil input")
    }
}
```

---

## Phase 3: Moderate Defects

### Defect #7: `closeFDs()` Double-Close

**Severity:** MODERATE
**File:** `loop.go`
**Function:** `closeFDs()`

#### Problem

`poll()` calls `shutdown()` on error, then `run()` loop also calls `shutdown()` → `closeFDs()` called twice.

```go
// CURRENT - loop.go:poll()
if l.state.TryTransition(StateSleeping, StateTerminating) {
    l.shutdown()  // First close
}
return

// Back in run() - loop.go
if l.state.Load() == StateTerminated {
    l.shutdown()  // Second close!
}
```

#### Fix

Use `sync.Once` for `closeFDs()`:

```go
// ADD to Loop struct - loop.go
type Loop struct {
    // ... existing fields ...
    closeOnce sync.Once
}

// FIXED - loop.go:closeFDs()
func (l *Loop) closeFDs() {
    l.closeOnce.Do(func() {
        _ = l.poller.Close()
        _ = unix.Close(l.wakePipe)
        if l.wakePipeWrite != l.wakePipe {
            _ = unix.Close(l.wakePipeWrite)
        }
    })
}
```

#### Verification Test

```go
func TestCloseFDsInvokedOnce(t *testing.T) {
    loop, err := New()
    if err != nil {
        t.Fatal(err)
    }
    
    var closeCount atomic.Int32
    originalClose := loop.poller.Close
    loop.poller.Close = func() error {
        closeCount.Add(1)
        return originalClose()
    }
    
    loop.closeFDs()
    loop.closeFDs()
    loop.closeFDs()
    
    if closeCount.Load() != 1 {
        t.Fatalf("closeFDs called %d times, expected 1", closeCount.Load())
    }
}
```

---

### Defect #8: Platform Error Inconsistency

**Severity:** MODERATE
**File:** `poller_darwin.go`

#### Problem

Darwin returns `errEventLoopClosed` while Linux returns `ErrPollerClosed`.

```go
// CURRENT - poller_darwin.go
if p.closed {
    return errEventLoopClosed  // <-- Inconsistent!
}

// CURRENT - poller_linux.go
if p.closed {
    return ErrPollerClosed  // <-- Correct
}
```

#### Fix

Already fixed in Defect #1 refactor—both platforms now return `ErrPollerClosed`.

#### Verification Test

```go
func TestInitPollerClosedReturnsConsistentError(t *testing.T) {
    var p ioPoller
    p.closed.Store(true)
    
    err := p.initPoller()
    if err != ErrPollerClosed {
        t.Fatalf("Expected ErrPollerClosed, got %v", err)
    }
}
```

---

### Defect #9: `pollIO` Method Location Inconsistency

**Severity:** LOW-MODERATE
**Files:** `poller_darwin.go`, `poller_linux.go`

#### Problem

| Platform | Current Location |
|----------|------------------|
| Darwin | `func (l *Loop) pollIO(...)` |
| Linux | `func (p *ioPoller) pollIO(...)` |

#### Fix

Standardize: Darwin already has `pollIO` on `Loop` and Linux has it on `ioPoller`. Since `Loop` owns the poller, move Linux's method to match Darwin (or vice versa). The simplest fix is to add a `Loop.pollIO` wrapper on Linux:

```go
// ADD to poller_linux.go
func (l *Loop) pollIO(timeout int, maxEvents int) (int, error) {
    return l.poller.PollIO(timeout)
}
```

**Note:** This is already present on Darwin. Verify both files have this method.

---

## Phase 4: Low/Theoretical Defects

### Defect #10: Sequence Counter Wrap-Around

**Severity:** THEORETICAL (58 years at 10B ops/sec)
**File:** `ingress.go`
**Function:** `MicrotaskRing.Push()`

#### Problem

`tailSeq` is `uint64`. After 2^64 increments, it wraps to 0—which is the sentinel for "empty slot."

#### Fix (Bundled with #4)

Already included in Defect #4 fix:

```go
seq := r.tailSeq.Add(1)
if seq == 0 {
    seq = r.tailSeq.Add(1) // Skip sentinel value
}
```

---

## Test Strategy

### New Test Files to Create

| File | Tests | Phase |
|------|-------|-------|
| `poller_init_race_test.go` | `TestPoller_InitRace_ConcurrentAccess`, `TestIOPollerClosedDataRace` | 1 |
| `ingress_writeafterfree_test.go` | `TestMicrotaskRing_WriteAfterFree_Race` | 1 |
| `ingress_fifo_test.go` | `TestMicrotaskRing_FIFO_Violation` | 1 |
| `ingress_popbatch_test.go` | `TestPopBatch_WaitsForInFlightProducer` | 2 |
| `ingress_nil_test.go` | `TestMicrotaskRing_NilInput_NoInfiniteLoop` | 2 |
| `loop_closeonce_test.go` | `TestCloseFDsInvokedOnce` | 3 |
| `poller_api_test.go` | `TestInitPollerClosedReturnsConsistentError`, `TestPollIOMethodExists` | 3 |

### Testing Commands

```bash
# Phase 1: Race detection (must pass)
go test -race -v -count=5 ./eventloop/... \
    -run "TestPoller_InitRace|TestIOPollerClosedDataRace|TestMicrotaskRing_WriteAfterFree"

# Phase 1: FIFO correctness
go test -v -count=10 ./eventloop/... \
    -run "TestMicrotaskRing_FIFO_Violation"

# Phase 2: Liveness tests
go test -v -timeout=30s ./eventloop/... \
    -run "TestPopBatch_WaitsForInFlightProducer|TestMicrotaskRing_NilInput"

# Full suite
go test -race -v -count=3 ./eventloop/...
```

---

## CI/CD Integration

### GitHub Actions Workflow

Create `.github/workflows/eventloop-correctness.yml`:

```yaml
name: Eventloop Correctness

on:
  push:
    paths:
      - 'eventloop/**'
  pull_request:
    paths:
      - 'eventloop/**'

jobs:
  race-detection:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Race Detection Tests
        run: |
          cd eventloop
          go test -race -v -count=5 \
            -run "TestPoller_Init|TestIOPollerClosed|TestMicrotaskRing"
        timeout-minutes: 15

  cross-platform:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Platform Tests
        run: |
          cd eventloop
          go test -v -count=3 \
            -run "TestInitPollerClosed|TestPollIO"

  stress:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Stress Tests
        run: |
          cd eventloop
          go test -v -count=10 -timeout=10m \
            -run "TestWriteAfterFree|TestFIFO|TestPopBatch"
```

---

## Rollout & Verification

### Pre-Merge Checklist

- [ ] All new tests pass locally on Darwin and Linux
- [ ] `go test -race ./eventloop/...` passes with 0 data races
- [ ] `go vet ./eventloop/...` clean
- [ ] `staticcheck ./eventloop/...` clean
- [ ] Existing tests pass (no regressions)
- [ ] CI/CD pipeline green on all platforms

### Rollout Steps

1. **Commit Phase 1 fixes** (blocking - do not proceed without these)
   - Create branch `fix/eventloop-critical-defects`
   - Commit: `fix(eventloop): resolve initPoller race with sync.Once`
   - Commit: `fix(eventloop): atomic.Bool for ioPoller.closed`
   - Commit: `fix(eventloop): MicrotaskRing Pop ordering (write-after-free)`
   - Commit: `fix(eventloop): MicrotaskRing Push FIFO preservation`
   
2. **Run full test suite** including stress tests

3. **Commit Phase 2 fixes** (high severity)
   - Commit: `fix(eventloop): PopBatch spin-wait for in-flight producers`
   - Commit: `fix(eventloop): Pop handles nil without infinite loop`

4. **Commit Phase 3 fixes** (moderate)
   - Commit: `fix(eventloop): closeFDs uses sync.Once`
   - Commit: `fix(eventloop): consistent ErrPollerClosed on Darwin`

5. **Commit Phase 4** (optional)
   - Commit: `fix(eventloop): skip sequence 0 to prevent wrap-around`

6. **PR Review** - require 2 approvals for critical path changes

7. **Merge** to main with squash

---

## Risk Assessment

### Phase 1 Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| `sync.Once` prevents test reset | Medium | Low | Add `resetForTest()` method |
| Ordering fix causes perf regression | Low | Medium | Benchmark before/after |
| FIFO fix adds latency (mutex check) | Low | Low | Overflow is rare path |

### Phase 2 Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| PopBatch spin-wait causes CPU spike | Low | Medium | Add backoff if needed |

### Phase 3 Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| closeOnce prevents FD reuse in tests | Medium | Low | Add test helper to reset |

### Rollback Plan

If critical issues arise post-merge:
1. Revert PR immediately
2. Investigate root cause with failing test
3. Fix and re-land with additional test coverage

---

## Summary of Code Changes

### File: `eventloop/ingress.go`

| Line Range | Change | Defect |
|------------|--------|--------|
| Push() | Check overflow first | #4 |
| Push() | Skip seq==0 | #10 |
| Pop() | Reorder: clear seq before head.Add | #3 |
| Pop() | Advance head on nil fn | #6 |
| PopBatch() | Add spin-wait for in-flight | #5 |

### File: `eventloop/poller_darwin.go`

| Line Range | Change | Defect |
|------------|--------|--------|
| ioPoller struct | `sync.Once` + `atomic.Bool` | #1, #2 |
| initPoller() | Use `once.Do()` | #1 |
| closePoller() | Use `closed.Store()` | #2 |
| Error return | `ErrPollerClosed` | #8 |

### File: `eventloop/poller_linux.go`

| Line Range | Change | Defect |
|------------|--------|--------|
| ioPoller struct | `sync.Once` + `atomic.Bool` | #1, #2 |
| initPoller() | Use `once.Do()` | #1 |
| closePoller() | Use `closed.Store()` | #2 |

### File: `eventloop/loop.go`

| Line Range | Change | Defect |
|------------|--------|--------|
| Loop struct | Add `closeOnce sync.Once` | #7 |
| closeFDs() | Wrap in `closeOnce.Do()` | #7 |

---

**END OF PLAN**
