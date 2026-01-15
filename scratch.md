# Eventloop PR Correctness Analysis: Consolidated Super-Document

> **Status:** DO NOT MERGE
> **Verdict:** CANNOT GUARANTEE CORRECTNESS

This document consolidates findings from multiple independent reviews. Where reviews conflict, reconciliation notes are provided.

---

## Executive Summary

The PR introduces significant architectural improvements—notably a superior `MicrotaskRing` implementation using sequence numbers and fixes for poller initialization. However, **multiple critical defects** have been identified that compromise system stability:

| # | Bug | Severity | Consensus |
|---|-----|----------|-----------|
| 1 | `initPoller()` CAS returns before `Init()` completes | **CRITICAL** | All reviews agree |
| 2 | `ioPoller.closed` is non-atomic bool with concurrent access | **CRITICAL** | Doc 6,7,8 |
| 3 | `MicrotaskRing.Pop()` Write-After-Free race | **CRITICAL** | Doc 4,5 |
| 4 | `MicrotaskRing` FIFO violation (overflow priority inversion) | **CRITICAL** | Doc 2,3 |
| 5 | `PopBatch` doesn't spin like `Pop()` | HIGH | Doc 1,6,7 |
| 6 | `MicrotaskRing.Pop()` infinite loop on `nil` input | HIGH | Doc 1 |
| 7 | `closeFDs()` double-close on poll error | MODERATE | Doc 6,7,8 |
| 8 | Platform error inconsistency (Linux vs Darwin) | MODERATE | Doc 6,7,8 |
| 9 | Platform `pollIO` method location inconsistency | LOW-MODERATE | Doc 6,7,8 |
| 10 | `MicrotaskRing` sequence counter wrap-around | THEORETICAL | Doc 1,4 |

---

## CRITICAL DEFECT #1: `initPoller()` Initialization Race

**Severity:** CRITICAL (Panic/Undefined Behavior)
**Location:** `eventloop/poller_darwin.go` & `eventloop/poller_linux.go`
**Consensus:** UNANIMOUS across all reviews

### The Flaw

The `initPoller` method uses an atomic CAS to ensure only one thread initializes the poller, but it **incorrectly assumes that losing the CAS race means initialization is complete**.

```go
if !p.initialized.CompareAndSwap(false, true) {
    // Another goroutine claimed it...
    return nil // <--- BUG: Returns success immediately!
}
// Actual initialization takes time...
if err := p.p.Init(); err != nil { ... }
```

### The Race Scenario

1. **Goroutine A** calls `initPoller`. Wins CAS. Sets `initialized` to `true`. Begins `p.p.Init()` (syscall, takes non-zero time).
2. **Goroutine B** calls `initPoller`. Loses CAS. Returns `nil` immediately.
3. **Goroutine B** proceeds to call `RegisterFD`.
4. **CRASH:** `RegisterFD` runs against a `FastPoller` struct that hasn't finished `Init()` (e.g., `p.epfd` might still be 0 or map unallocated). Panic or syscall error occurs.

### The Fix (UNANIMOUS)

Replace the `atomic.Bool` + `CompareAndSwap` logic with `sync.Once`. It guarantees that:
1. Only one goroutine executes the function.
2. All other callers **BLOCK** until that function returns.

```go
type ioPoller struct {
    p    FastPoller
    once sync.Once // Use this instead of initialized atomic.Bool
    closed atomic.Bool // Also fix Bug #2
}

func (p *ioPoller) initPoller() error {
    var err error
    p.once.Do(func() {
        if p.closed.Load() {
            err = ErrPollerClosed
            return
        }
        err = p.p.Init()
    })
    return err
}
```

### Proof Test

```go
func TestPoller_Init_Race(t *testing.T) {
    for i := 0; i < 100; i++ {
        p := &ioPoller{}
        start := make(chan struct{})
        var wg sync.WaitGroup
        var failures atomic.Int32

        for g := 0; g < 2; g++ {
            wg.Add(1)
            go func() {
                defer wg.Done()
                <-start
                if err := p.initPoller(); err != nil {
                    t.Error(err)
                    return
                }
                defer func() {
                    if r := recover(); r != nil {
                        failures.Add(1)
                    }
                }()
                _ = p.p.RegisterFD(1, EventRead, func(IOEvents) {})
            }()
        }

        close(start)
        wg.Wait()

        if failures.Load() > 0 {
            t.Fatalf("Iteration %d: Race condition detected!", i)
        }
        p.closePoller()
    }
}
```

---

## CRITICAL DEFECT #2: Data Race on `ioPoller.closed`

**Severity:** CRITICAL
**Location:** Both platforms
**Source:** Doc 6,7,8

### The Flaw

```go
func (p *ioPoller) closePoller() error {
    p.closed = true  // Non-atomic write
    // ...
}

func (p *ioPoller) initPoller() error {
    if p.closed {  // Non-atomic read
        return ...
    }
```

Concurrent calls to `initPoller()` and `closePoller()` race on `p.closed`.

### The Fix

Change `closed bool` to `closed atomic.Bool` and use `Store(true)`/`Load()` for all access.

### Proof Test

```go
// RUN: go test -race -run TestIOPollerClosedDataRace -count=10
func TestIOPollerClosedDataRace(t *testing.T) {
    for i := 0; i < 100; i++ {
        var poller ioPoller
        var wg sync.WaitGroup
        wg.Add(2)

        go func() {
            defer wg.Done()
            for j := 0; j < 10; j++ {
                _ = poller.closePoller()
                poller.closed = false // Reset (also a race!)
            }
        }()

        go func() {
            defer wg.Done()
            for j := 0; j < 10; j++ {
                _ = poller.initPoller()
            }
        }()

        wg.Wait()
    }
}
```

---

## CRITICAL DEFECT #3: `MicrotaskRing.Pop()` Write-After-Free Race

**Severity:** CATASTROPHIC (Deadlock/Data Loss)
**Location:** `eventloop/ingress.go` (Lines 226-228 in `Pop`)
**Source:** Doc 4,5

### The Flaw

The code increments `head` (making the slot available to producers) **before** clearing the sequence guard for that slot.

```go
// Current Implementation (BUGGY)
r.head.Add(1)                   // [A] Slot effectively released to producer
r.seq[(head)%4096].Store(0)     // [B] Guard cleared (using OLD head index)
```

### The Race Scenario

1. **Consumer** reads data at index `i`.
2. **Consumer** executes `r.head.Add(1)` ([A]). The slot `i` is now logically free.
3. *Context Switch*
4. **Producer** claims slot `i`, writes new task, stores new valid sequence `S > 0`.
5. **Consumer** resumes. Executes `r.seq[i].Store(0)` ([B]).
6. **Result:** Slot `i` contains valid data, but sequence guard is `0`.
7. **Impact:** Next `Pop` sees `seq == 0`, enters infinite spin-loop waiting for producer that already finished.

### The Fix

Enforce strict **Release** semantics: clear the sequence guard **before** advancing the head.

```go
// FIXED Implementation
r.seq[(head)%4096].Store(0) // Clear guard FIRST
r.head.Add(1)               // Release slot SECOND
```

### Proof Test (Torture Test)

```go
func TestMicrotaskRing_WriteAfterFree_Race(t *testing.T) {
    ring := NewMicrotaskRing()
    const iterations = 1_000_000
    var wg sync.WaitGroup
    wg.Add(2)

    go func() {
        defer wg.Done()
        runtime.LockOSThread()
        for i := 0; i < iterations; i++ {
            for !ring.Push(func() {}) {
                runtime.Gosched()
            }
        }
    }()

    go func() {
        defer wg.Done()
        runtime.LockOSThread()
        count := 0
        watchdog := time.NewTimer(5 * time.Second)
        defer watchdog.Stop()

        for count < iterations {
            task := ring.Pop()
            if task == nil {
                runtime.Gosched()
                continue
            }
            count++
            if count%1000 == 0 {
                if !watchdog.Stop() {
                    <-watchdog.C
                }
                watchdog.Reset(5 * time.Second)
            }
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
    case <-time.After(10 * time.Second):
        t.Fatal("DEADLOCK DETECTED: Consumer stalled. Write-After-Free race proven.")
    }
}
```

---

## CRITICAL DEFECT #4: `MicrotaskRing` FIFO Violation

**Severity:** CRITICAL (Acausality)
**Location:** `eventloop/ingress.go`
**Source:** Doc 2,3

### The Flaw

The hybrid design of Ring (lock-free) + Overflow (mutex) creates a "Priority Inversion" where **newer tasks can be processed before older tasks**.

The `Pop` method iterates strictly over the Ring. It only checks the Overflow buffer if the Ring is empty.

### The Failure Scenario

1. **Saturation:** Ring fills up (4096 items).
2. **Overflow:** Producer pushes **Task A** (Seq 4097). Since Ring is full, it goes into `overflow`.
3. **Drain:** Consumer pops one item from Ring. Ring now has 4095 items.
4. **Race:** Producer pushes **Task B** (Seq 4098). Since Ring now has space, Task B enters the **Ring**.
5. **Ordering Failure:** Consumer processes **Task B** before **Task A**.

### The Fix

In `Push`: If the overflow buffer is non-empty, **append to overflow**, even if the ring has space.

```go
func (r *MicrotaskRing) Push(fn func()) bool {
    // FIX: Check overflow first
    r.overflowMu.Lock()
    if len(r.overflow) > 0 {
        r.overflow = append(r.overflow, fn)
        r.overflowMu.Unlock()
        return true
    }
    r.overflowMu.Unlock()

    // ... continue with Fast Path CAS loop ...
}
```

### Proof Test (Deterministic)

```go
func TestMicrotaskRing_FIFO_Violation(t *testing.T) {
    r := NewMicrotaskRing()

    // 1. Saturate the Ring Buffer (4096 items)
    for i := 0; i < 4096; i++ {
        val := i
        r.Push(func() { _ = val })
    }

    // 2. Force Overflow - This is "Task A"
    taskA_Executed := false
    r.Push(func() { taskA_Executed = true })

    // 3. Create Space in the Ring
    if fn := r.Pop(); fn == nil {
        t.Fatal("Expected item from ring")
    }

    // 4. Push "Task B" - goes to Ring due to bug
    taskB_Executed := false
    r.Push(func() { taskB_Executed = true })

    // 5. Drain the Ring (4095 items)
    for i := 0; i < 4095; i++ {
        if fn := r.Pop(); fn == nil {
            t.Fatal("Expected item from ring")
        }
    }

    // 6. The Moment of Truth - next item MUST be Task A
    nextFn := r.Pop()
    if nextFn == nil {
        t.Fatal("Queue should not be empty")
    }
    nextFn()

    if taskB_Executed && !taskA_Executed {
        t.Fatalf("CRITICAL: Task B (newer) executed before Task A (older). FIFO violated.")
    }
    if !taskA_Executed {
        t.Fatalf("Expected Task A to execute next")
    }
}
```

---

## HIGH SEVERITY DEFECT #5: `PopBatch` Inconsistency with `Pop`

**Severity:** HIGH
**Location:** `eventloop/ingress.go`
**Source:** Doc 1,6,7

### The Flaw

`Pop()` was correctly patched to spin-wait when a producer has claimed the tail but not yet linked the new node. However, `PopBatch()` was **not given the same fix**.

```go
// Pop(): spins here
for { next = head.next.Load(); if next != nil { break }; runtime.Gosched() }

// PopBatch(): just exits
if next == nil { break }
```

### Impact

A consumer calling `PopBatch` in a loop can fail to retrieve tasks that a subsequent call to `Pop` would have correctly retrieved.

### The Fix

`PopBatch` must perform the same spin-wait logic if `head.next.Load()` is `nil` but `q.tail.Load() != head`.

---

## HIGH SEVERITY DEFECT #6: `MicrotaskRing.Pop()` Infinite Loop on `nil` Input

**Severity:** HIGH (Liveness Failure)
**Location:** `eventloop/ingress.go`
**Source:** Doc 1

### The Flaw

The `Push` method does not prevent `nil` functions from being added. If `Push(nil)` is called, the `Pop` method enters an infinite loop.

1. `Pop` reads a valid sequence number for the slot containing the `nil` task.
2. It reads the task `fn`, which is `nil`.
3. It hits the defensive check: `if fn == nil`.
4. It re-reads `head` and `tail` and `continue`s. It does **not** advance `head`.
5. The next iteration attempts to pop the **exact same `nil` task**, repeating indefinitely.

### The Fix

**Option A (Recommended):** Modify `Pop`. When a `nil` task is encountered, still consume it by advancing `head` and clearing the sequence number, then continue.

**Option B:** Modify `Push` to silently drop or return an error for `nil` functions.

---

## MODERATE DEFECT #7: `closeFDs()` Double-Close

**Severity:** MODERATE
**Location:** `eventloop/loop.go`
**Source:** Doc 6,7,8

### The Flaw (Flow when `poll()` errors)

```go
// In poll():
if l.state.TryTransition(StateSleeping, StateTerminating) {
    l.shutdown()  // First call - closes FDs
}
return

// Back in run() loop:
if l.state.Load() == StateTerminated {
    l.shutdown()  // Second call - closes same FDs again!
    return nil
}
```

### The Fix

Use `sync.Once` for `closeFDs()`:

```go
// In Loop struct:
closeOnce sync.Once

func (l *Loop) closeFDs() {
    l.closeOnce.Do(func() {
        // ... existing close logic ...
    })
}
```

---

## MODERATE DEFECT #8: Platform Error Inconsistency

**Severity:** MODERATE
**Location:** `poller_darwin.go` vs `poller_linux.go`
**Source:** Doc 6,7,8

| Condition | Linux | Darwin |
|-----------|-------|--------|
| `initPoller()` when closed | `ErrPollerClosed` | `errEventLoopClosed` |

### The Fix

Change Darwin `errEventLoopClosed` to `ErrPollerClosed` for consistency.

---

## LOW-MODERATE DEFECT #9: `pollIO` Method Location Inconsistency

**Severity:** LOW-MODERATE
**Source:** Doc 6,7,8

| Platform | Method Receiver |
|----------|-----------------|
| Linux | `func (p *ioPoller) pollIO(...)` |
| Darwin | `func (l *Loop) pollIO(...)` |

Code calling `loop.pollIO()` compiles on Darwin but fails on Linux.

### The Fix

Standardize method location across platforms.

---

## THEORETICAL DEFECT #10: Sequence Counter Wrap-Around

**Severity:** LOW (Theoretical)
**Source:** Doc 1,4

### The Flaw

`tailSeq` is a `uint64` starting at 0, always incremented. After 2^64 operations (~58 years at 10B ops/sec), it wraps to `0`. Since `0` is the sentinel for "empty slot", the consumer will incorrectly assume the slot is empty and spin forever.

### The Fix

In `Push`, if `tailSeq.Add(1)` returns `0`, increment again to `1`.

---

## VERIFIED CORRECT COMPONENTS

| Component | Status | Notes |
|-----------|--------|-------|
| `LockFreeIngress.Pop()` spin logic | ✓ Correct | Properly handles in-flight producers |
| `MicrotaskRing` memory ordering | ✓ Correct | Release-acquire semantics valid (Go 1.19+) |
| Race-Free Poller Initialization Reset | ✓ Correct | Resets `initialized` on failure |
| `UnregisterFD` Callback Lifetime Docs | ✓ Correct | Critical documentation improvement |
| Error Handling in `poll()` | ✓ Correct | Now calls `shutdown()` on `PollIO` failure |

---

## LATENT ISSUES (Documented Constraints)

### `MicrotaskRing.Pop()` Single-Consumer Only

The implementation uses `r.head.Add(1)` without CAS, meaning concurrent consumers can consume the same slot. This is **mitigated by** the event loop's single-tick-goroutine architecture, but the requirement is **undocumented**.

### `getGoroutineID()` Fragility

The `isLoopThread()` check relies on `getGoroutineID()`, which parses `runtime.Stack` output. The Go team explicitly warns against this as the format is not a guaranteed API.

---

## COMPREHENSIVE FIX SUMMARY

### File: `eventloop/ingress.go`

#### Fix #3 (Write-After-Free)
```go
func (r *MicrotaskRing) Pop() func() {
    // ... (seq check logic) ...
    fn := r.buffer[head%4096]
    r.buffer[(head)%4096] = nil // Optional: GC friendliness
    
    // FIX: Clear sequence guard BEFORE making slot available
    r.seq[(head)%4096].Store(0) 
    
    // FIX: Advance head AFTER clearing guard
    r.head.Add(1)
    
    return fn
}
```

#### Fix #4 (FIFO Violation)
```go
func (r *MicrotaskRing) Push(fn func()) bool {
    r.overflowMu.Lock()
    if len(r.overflow) > 0 {
        r.overflow = append(r.overflow, fn)
        r.overflowMu.Unlock()
        return true
    }
    r.overflowMu.Unlock()
    // ... continue with Fast Path ...
}
```

#### Fix #6 (nil Infinite Loop)
```go
// In Pop(), when fn == nil is encountered:
if fn == nil {
    r.seq[(head)%4096].Store(0)
    r.head.Add(1) // MUST advance head to avoid infinite loop
    continue
}
```

#### Fix #10 (Sequence Wrap-Around)
```go
// In Push():
seq := r.tailSeq.Add(1)
if seq == 0 {
    seq = r.tailSeq.Add(1) // Skip 0
}
```

### File: `eventloop/poller_darwin.go` & `eventloop/poller_linux.go`

#### Fix #1 & #2 (initPoller Race + closed Data Race)
```go
type ioPoller struct {
    p      FastPoller
    once   sync.Once   // Replace initialized atomic.Bool
    closed atomic.Bool // Replace bool
}

func (p *ioPoller) initPoller() error {
    var err error
    p.once.Do(func() {
        if p.closed.Load() {
            err = ErrPollerClosed
            return
        }
        err = p.p.Init()
    })
    return err
}

func (p *ioPoller) closePoller() error {
    p.closed.Store(true)
    // ...
}
```

#### Fix #8 (Platform Error Inconsistency)
```go
// In poller_darwin.go, change:
// return errEventLoopClosed
// to:
return ErrPollerClosed
```

### File: `eventloop/loop.go`

#### Fix #7 (closeFDs Double-Close)
```go
type Loop struct {
    // ...
    closeOnce sync.Once
}

func (l *Loop) closeFDs() {
    l.closeOnce.Do(func() {
        // ... existing close logic ...
    })
}
```

---

## REQUIRED TEST FILES

### `eventloop/ingress_torture_test.go`
- `TestMicrotaskRing_WriteAfterFree_Race`
- `TestMicrotaskRing_FIFO_Violation`
- `TestMicrotaskRing_NilInput_Liveness`

### `eventloop/poller_race_test.go`
- `TestPoller_Init_Race`
- `TestIOPollerClosedDataRace`

### `eventloop/correctness_test.go`
- `TestCloseFDsInvokedOnce`
- `TestInitPollerClosedReturnsConsistentError`
- `TestMicrotaskRingMultiConsumerCorruption`
- `TestLockFreeIngressPopWaitsForProducer`

### `eventloop/popbatch_test.go`
- `TestPopBatchInconsistencyWithPop`

### `eventloop/poller_api_test.go`
- `TestPollerAPI_InitOnClosedReturnsStandardError`
- `TestLoopAPI_PollIOMethodExists`

---

## CI INTEGRATION

```yaml
name: Correctness Tests

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
          go test -race -v -count=5 ./eventloop/... \
            -run "TestIOPollerClosedDataRace|TestMicrotaskRingMultiConsumer"
        timeout-minutes: 10

  cross-platform-consistency:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Platform Consistency Tests
        run: |
          go test -v ./eventloop/... \
            -run "TestInitPollerClosedReturnsConsistentError|TestPollIOCompiles"

  stress-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Stress Tests
        run: |
          go test -v -count=10 -timeout=10m ./eventloop/... \
            -run "TestLockFreeIngressPopWaitsForProducer|TestPopBatchInconsistencyWithPop"
```

---

## RECONCILIATION NOTES

### Conflict: MicrotaskRing Primary Bug

- **Doc 1,3** emphasize FIFO violation (overflow priority inversion)
- **Doc 4,5** emphasize Write-After-Free (sequence clobbering)

**Resolution:** Both bugs exist. They are independent defects. The Write-After-Free is in `Pop()` ordering. The FIFO violation is in `Push()` overflow logic. **Both must be fixed.**

### Conflict: PopBatch Inconsistency Severity

- **Doc 1** calls it a BUG
- **Doc 6** calls it LATENT (mitigated by tick loop)

**Resolution:** Classify as HIGH severity. While the tick loop mitigates impact, the inconsistency violates principle of least astonishment and can cause task starvation under specific timing.

### Conflict: initPoller Fix Approach

- **Doc 1,4** suggest manual CAS + spin-wait with `initializing` flag
- **Doc 2,3,5,6,7,8** recommend `sync.Once`

**Resolution:** Use `sync.Once`. It is the idiomatic Go solution, requires less code, and is guaranteed correct by the standard library.
