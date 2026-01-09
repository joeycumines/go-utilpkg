# Alternate Implementation One: Maximum Safety Architecture

This specification defines a "Maximum Safety" event loop implementation that prioritizes **correctness guarantees** and **defensive programming** over raw performance. Every design decision favors preventing subtle bugs over micro-optimizations.

---

## Philosophy: Safety-First Design

1. **Fail-Fast over Fail-Silent**: All error paths must be explicit and observable
2. **Lock Coarseness over Granularity**: Prefer holding locks longer to eliminate race windows
3. **Allocation Tolerance**: Accept allocations if they simplify correctness reasoning
4. **Extensive Validation**: Runtime invariant checks in all debug builds
5. **Deterministic Behavior**: No reliance on timing assumptions

---

## S1. State Machine: Strict Transition Validation

### S1.1. Transition Table Enforcement

Every state transition MUST be validated against a compile-time transition table. Invalid transitions panic in debug mode.

```go
var validTransitions = map[LoopState][]LoopState{
    StateAwake:       {StateRunning, StateTerminating},
    StateRunning:     {StateSleeping, StateTerminating},
    StateSleeping:    {StateRunning, StateTerminating},
    StateTerminating: {StateTerminated},
    StateTerminated:  {}, // terminal
}

func (sm *SafeStateMachine) transition(from, to LoopState) bool {
    valid := slices.Contains(validTransitions[from], to)
    if !valid {
        panic(fmt.Sprintf("invalid state transition: %v -> %v", from, to))
    }
    return sm.val.CompareAndSwap(uint32(from), uint32(to))
}
```

### S1.2. State Observers

Provide observable state transitions via channel for debugging:

```go
type StateObserver interface {
    OnTransition(from, to LoopState, timestamp time.Time)
}
```

---

## S2. Ingress Queue: Lock-Based with Full Validation

### S2.1. Single Lock Architecture

Use a single `sync.Mutex` for the entire ingress subsystem (external + internal + microtasks). This eliminates lock ordering bugs and simplifies reasoning.

```go
type SafeIngress struct {
    mu         sync.Mutex
    external   *taskList
    internal   *taskList
    microtasks *taskList
    length     int
    closed     bool
}
```

### S2.2. Defensive Chunk Management

Every chunk operation validates invariants:

```go
func (q *SafeIngress) pushLocked(t Task, lane Lane) {
    q.validateInvariants()
    defer q.validateInvariants()
    // ... push logic
}

func (q *SafeIngress) validateInvariants() {
    if q.length < 0 {
        panic("negative queue length")
    }
    if (q.external.head == nil) != (q.external.tail == nil) {
        panic("head/tail asymmetry")
    }
    // ... more checks
}
```

### S2.3. Full-Clear Always

**No optimization** of `returnChunk`. Always clear all 128 slots regardless of `pos`:

```go
func returnChunk(c *chunk) {
    for i := range c.tasks { // Always full iteration
        c.tasks[i] = Task{}
    }
    c.pos = 0
    c.readPos = 0
    c.next = nil
    chunkPool.Put(c)
}
```

---

## S3. Check-Then-Sleep: Conservative Protocol

### S3.1. Hold Lock Through Sleep Decision

Instead of the unlock-check-relock pattern, hold the ingress lock while making the sleep decision:

```go
func (l *Loop) prepareForSleep() (shouldSleep bool) {
    l.ingress.mu.Lock()
    defer l.ingress.mu.Unlock()
    
    if l.ingress.length > 0 {
        return false
    }
    
    // Set state while holding lock
    if !l.state.CAS(StateRunning, StateSleeping) {
        return false
    }
    
    return true
}
```

### S3.2. Wake-Up Retry Loop

Never give up on wake-up syscall:

```go
func (l *Loop) requestWake() {
    for {
        _, err := unix.Write(l.wakeFD, l.wakeBuf[:])
        if err == nil || err == unix.EAGAIN {
            return
        }
        if err == unix.EINTR {
            continue
        }
        // Log but keep retrying for transient errors
        l.logError("wake syscall failed", err)
        runtime.Gosched()
    }
}
```

---

## S4. Poller: Conservative Locking

### S4.1. Write Lock for Poll

Use `Lock()` (write lock) for `pollIO` instead of `RLock()`. This prevents any concurrent modifications during poll processing but simplifies correctness:

```go
func (p *SafePoller) pollIO(timeoutMs int) error {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    if p.closed {
        return ErrPollerClosed
    }
    
    // Blocking syscall under lock (accepts starvation for correctness)
    n, err := unix.EpollWait(p.epfd, p.eventBuf, timeoutMs)
    // ... process events under lock
}
```

**Trade-off**: `RegisterFD` blocks during poll, but there's zero risk of zombie poller access.

### S4.2. Callback Execution Under Lock

Execute callbacks under lock to prevent re-entrancy issues:

```go
func (p *SafePoller) processEvents(n int) {
    // Still under p.mu.Lock()
    for i := 0; i < n; i++ {
        fd := int(p.eventBuf[i].Fd)
        if info, ok := p.fds[fd]; ok {
            // Callback executes under lock
            // User must not call RegisterFD from callback (documented)
            p.safeCallback(info.callback, IOEvents{Fd: fd})
        }
    }
}
```

---

## S5. Shutdown: Strict Sequential Ordering

### S5.1. Serial Shutdown Phases

Execute shutdown phases serially with explicit phase markers:

```go
func (l *Loop) shutdown() {
    phases := []shutdownPhase{
        {name: "ingress", fn: l.drainIngress},
        {name: "internal", fn: l.drainInternal},
        {name: "microtasks", fn: l.drainMicrotasks},
        {name: "timers", fn: l.cancelTimers},
        {name: "promises", fn: l.rejectPromises},
        {name: "fds", fn: l.closeFDs},
    }
    
    for _, phase := range phases {
        l.logPhase("shutdown", phase.name, "start")
        phase.fn()
        l.logPhase("shutdown", phase.name, "complete")
    }
}
```

### S5.2. Stop() with sync.Once

Guaranteed single execution:

```go
func (l *Loop) Stop(ctx context.Context) error {
    var result error
    l.stopOnce.Do(func() {
        result = l.stopImpl(ctx)
    })
    if result == nil {
        return ErrLoopTerminated // Subsequent callers
    }
    return result
}
```

---

## S6. Memory Safety: Explicit Ownership

### S6.1. Task Ownership Tracking

Track task lifecycle explicitly:

```go
type SafeTask struct {
    Fn        func()
    CreatedAt time.Time
    Lane      Lane
    State     TaskState // Queued, Executing, Completed
    ID        uint64
}
```

### S6.2. Registry with Strong Validation

Validate all registry operations:

```go
func (r *SafeRegistry) Register(p *Promise) ID {
    if p == nil {
        panic("nil promise registration")
    }
    if p.id != 0 {
        panic("promise already registered")
    }
    // ... registration
}
```

---

## S7. Error Handling: Comprehensive

### S7.1. Error Wrapping

All errors include context:

```go
type LoopError struct {
    Op      string
    Phase   string
    Cause   error
    Context map[string]any
}
```

### S7.2. Panic Recovery with Full Stack

```go
func (l *Loop) safeExecute(t SafeTask) {
    defer func() {
        if r := recover(); r != nil {
            stack := debug.Stack()
            l.emitError(&PanicError{
                Value:   r,
                Stack:   stack,
                Task:    t,
                LoopID:  l.id,
            })
        }
    }()
    t.Fn()
}
```

---

## S8. Testing Requirements

### S8.1. Mandatory Tests

1. **State Transition Fuzzer**: Random state transitions, all invalid must panic
2. **Shutdown Ordering Validator**: Verify phase order via observer
3. **Memory Leak Detector**: Finalizer-based leak detection for all chunk returns
4. **Deadlock Detector**: Timeout-based deadlock detection for all operations
5. **Invariant Stress Test**: Run with invariant checks under heavy load

### S8.2. Debug Mode Features

```go
// Build with -tags=eventloop_debug
const DebugMode = true

func init() {
    if DebugMode {
        enableInvariantChecks()
        enableTransitionLogging()
        enableMemoryTracking()
    }
}
```

---

## S9. Performance Expectations

This implementation prioritizes correctness. Expected performance:

| Metric | Target | Notes |
|--------|--------|-------|
| Task latency | <100µs | Acceptable for correctness |
| Lock contention | High | Coarse locking accepted |
| Allocations | Tolerated | No zero-alloc requirement |
| Max throughput | 100k tasks/sec | Lower than performance variant |

---

## S10. Key Differentiators from Main Implementation

| Aspect | Main | AlternateOne (Safety) |
|--------|------|----------------------|
| Lock granularity | Fine (RWMutex per subsystem) | Coarse (single Mutex) |
| Invariant checks | Disabled in prod | Always enabled |
| Error handling | Silent drops | Explicit panics |
| Callback execution | Outside lock | Inside lock |
| Chunk clearing | Optimizable | Always full |
| State transitions | CAS only | CAS + validation |
