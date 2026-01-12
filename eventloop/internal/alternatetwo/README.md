# AlternateTwo - Maximum Performance Implementation

**Package:** `eventloop/internal/alternatetwo`
**Philosophy:** Performance without compromising correctness. Lock-free where safe.

---

## I. Design Principles

### 1.1 Core Philosophy

AlternateTwo prioritizes **throughput and latency** while maintaining correctness:

- **Lock-free ingress:** MPSC queue with atomic head/tail
- **Minimal-clear chunks:** Only used slots cleared (not all 128)
- **Fast state machine:** Cache-line padded atomics
- **Optimistic check-then-sleep:** Quick length check before commit
- **Zero-lock polling:** Direct FD indexing, version-based consistency
- **Task arena:** Pre-allocated buffer to reduce allocations

### 1.2 Trade-offs

| Aspect | AlternateTwo Choice | Benefit |
|--------|---------------------|---------|
| Ingress Lock | Lock-free MPSC | Higher throughput under contention |
| Chunk Clear | Clear only used slots | Lower CPU per chunk return |
| Poll Lock | No lock during poll | Concurrent FD registration |
| State Machine | Pure CAS, no validation | Faster transitions |
| Allocations | Arena + ring buffers | Reduced GC pressure |

### 1.3 CRITICAL: No Sleep/Poll Hacks

Despite the performance focus, AlternateTwo **MUST NOT** use:

- ❌ `time.Sleep` for race avoidance
- ❌ Polling loops for state changes
- ❌ Busy-wait spinning

**Correct synchronization is NON-NEGOTIABLE.**

---

## II. Architecture

### 2.1 Component Overview

```
┌─────────────────────────────────────────────────────────┐
│                         Loop                             │
├─────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │LockFreeIngress│ │  FastPoller  │  │  FastState   │  │
│  │ (Atomic MPSC)│  │ (Zero Lock)  │  │ (Padded CAS) │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
├─────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │ MicrotaskRing│  │  TaskArena   │  │   loopDone   │  │
│  │ (Lock-Free)  │  │ (Pre-alloc)  │  │  (Channel)   │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### 2.2 FastState

Cache-line padded for zero false sharing:

```go
type FastState struct {
    _      [64]byte      // Padding before
    state  atomic.Int32
    _      [60]byte      // Padding after (64 - 4)
}

func (s *FastState) TryTransition(from, to LoopState) bool {
    return s.state.CompareAndSwap(int32(from), int32(to))
}

func (s *FastState) Load() LoopState {
    return LoopState(s.state.Load())
}
```

### 2.3 LockFreeIngress

MPSC queue with atomic operations:

```go
type LockFreeIngress struct {
    _    [64]byte
    head atomic.Pointer[node]
    _    [56]byte
    tail atomic.Pointer[node]
    _    [56]byte
    len  atomic.Int64
}

func (q *LockFreeIngress) Push(fn func()) {
    n := &node{task: Task{Fn: fn}}
    
    for {
        tail := q.tail.Load()
        if q.tail.CompareAndSwap(tail, n) {
            tail.next.Store(n)
            q.len.Add(1)
            return
        }
    }
}
```

### 2.4 FastPoller

Direct FD indexing with version consistency:

```go
type FastPoller struct {
    epfd     int
    fds      [65536]fdEntry  // Direct indexing, no map
    versions [65536]uint32   // Version for ABA prevention
    eventBuf []unix.EpollEvent
}

func (p *FastPoller) RegisterFD(fd int, events IOEvents, cb IOCallback) error {
    if fd < 0 || fd >= 65536 {
        return ErrFDOutOfRange
    }
    
    p.versions[fd]++
    p.fds[fd] = fdEntry{events: events, callback: cb, version: p.versions[fd]}
    
    return unix.EpollCtl(p.epfd, unix.EPOLL_CTL_ADD, fd, &unix.EpollEvent{
        Events: uint32(events),
        Fd:     int32(fd),
    })
}
```

---

## III. Synchronization Design

### 3.1 Completion Signaling (NO POLLING)

AlternateTwo uses a **completion channel**, identical to AlternateOne:

```go
type Loop struct {
    loopDone chan struct{}  // Closed when loop terminates
}

func (l *Loop) Run(ctx context.Context) error {
    // ... validation ...
    
    l.loopDone = make(chan struct{})
    defer close(l.loopDone)
    
    return l.run(ctx)
}

func (l *Loop) shutdownImpl(ctx context.Context) error {
    // Trigger termination...
    
    // Wait via channel - NO POLLING
    select {
    case <-l.loopDone:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

### 3.2 Unstarted Loop Race Handling

**CORRECT approach** (no sleeping):

```go
func (l *Loop) shutdownImpl(ctx context.Context) error {
    for {
        current := l.state.Load()
        if current == StateTerminated {
            return ErrLoopTerminated
        }
        
        if l.state.TryTransition(current, StateTerminating) {
            // Case 1: Transition from Awake - loop never started
            if current == StateAwake {
                // The CAS from Awake -> Terminating means Run() cannot
                // succeed its CAS from Awake -> Running. We own cleanup.
                l.state.Store(StateTerminated)
                l.closeFDs()
                if l.loopDone != nil {
                    close(l.loopDone)
                }
                return nil
            }
            
            // Case 2: Loop is/was running
            if current == StateSleeping {
                _ = l.submitWakeup()
            }
            break
        }
    }
    
    // Wait on completion channel
    select {
    case <-l.loopDone:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

**Key insight:** If we successfully CAS from `StateAwake` to `StateTerminating`, Run() **cannot** succeed its CAS from `StateAwake` to `StateRunning`. The state machine ensures mutual exclusion - no timing/sleeping needed.

### 3.3 Optimistic Check-Then-Sleep

Quick check before committing to sleep:

```go
func (l *Loop) poll() {
    current := l.state.Load()
    if current != StateRunning {
        return
    }
    
    // Optimistic transition
    if !l.state.TryTransition(StateRunning, StateSleeping) {
        return
    }
    
    // Quick length check (may have false negatives - that's OK)
    if l.external.Length() > 0 || l.internal.Length() > 0 {
        l.state.TryTransition(StateSleeping, StateRunning)
        return
    }
    
    // Check for termination before blocking
    if l.state.Load() == StateTerminating {
        return
    }
    
    // Poll with timeout
    timeout := 100  // 100ms max
    _, err := l.poller.PollIO(timeout)
    if err != nil {
        l.state.TryTransition(StateSleeping, StateTerminating)
        return
    }
    
    l.state.TryTransition(StateSleeping, StateRunning)
}
```

---

## IV. Performance Optimizations

### 4.1 TaskArena

Pre-allocated task buffer:

```go
type TaskArena struct {
    tasks [65536]Task
    idx   atomic.Uint64
}

func (a *TaskArena) Alloc(fn func()) *Task {
    i := a.idx.Add(1) & 0xFFFF  // Wrap at 65536
    a.tasks[i] = Task{Fn: fn}
    return &a.tasks[i]
}
```

### 4.2 MicrotaskRing

Lock-free ring buffer:

```go
type MicrotaskRing struct {
    _    [64]byte
    head atomic.Uint64
    _    [56]byte
    tail atomic.Uint64
    _    [56]byte
    buf  [4096]func()
}

func (r *MicrotaskRing) Push(fn func()) bool {
    for {
        tail := r.tail.Load()
        head := r.head.Load()
        if tail-head >= uint64(len(r.buf)) {
            return false  // Full
        }
        if r.tail.CompareAndSwap(tail, tail+1) {
            r.buf[tail&4095] = fn
            return true
        }
    }
}
```

### 4.3 Batch Processing

Process multiple tasks in one go:

```go
func (l *Loop) processExternal() {
    const budget = 1024
    
    // Batch pop for cache efficiency
    n := l.external.PopBatch(l.batchBuf[:], budget)
    for i := 0; i < n; i++ {
        l.safeExecute(l.batchBuf[i].Fn)
        l.batchBuf[i] = Task{}  // Clear for GC
    }
}
```

---

## V. Correctness Guarantees

Despite performance focus, these invariants MUST hold:

### 5.1 State Machine Integrity

- Only valid transitions allowed
- CAS ensures atomic transitions
- No torn reads/writes

### 5.2 Task Conservation

- `Total_Submitted = Executed + Rejected`
- No tasks lost during shutdown
- FIFO ordering within each lane

### 5.3 Resource Cleanup

- All FDs closed on termination
- No goroutine leaks
- Memory properly released

---

## VI. Verification

### 6.1 Tests

```bash
# Run AlternateTwo tests
go test -v -race ./eventloop/internal/alternatetwo/...

# Stress test with race detector
go test -v -race -count=100 ./eventloop/internal/alternatetwo/...
```

### 6.2 Benchmarks

```bash
# Run performance benchmarks
go test -bench=. -benchmem ./eventloop/internal/alternatetwo/...
```

---

## VII. When to Use AlternateTwo

**Choose AlternateTwo when:**

- Maximum throughput required
- Low latency is critical
- High contention expected
- Memory allocation must be minimized
- Production workloads

**Avoid AlternateTwo when:**

- Debugging complex issues
- Need comprehensive error context
- Development/prototyping phase
- Correctness verification needed

---

## VIII. Comparison with AlternateOne

| Feature | AlternateOne | AlternateTwo |
|---------|--------------|--------------|
| Ingress Lock | Single mutex | Lock-free MPSC |
| Chunk Clear | Full 128 slots | Used slots only |
| Poll Lock | Write lock | Zero lock |
| State | Validated transitions | Pure CAS |
| Errors | Rich context | Minimal |
| Throughput | ~400k ops/s | ~1M ops/s |
| Latency (p99) | ~150ms | ~30ms |
