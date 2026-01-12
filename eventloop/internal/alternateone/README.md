# AlternateOne - Maximum Safety Implementation

**Package:** `eventloop/internal/alternateone`
**Philosophy:** Correctness over performance. When in doubt, use a mutex.

---

## I. Design Principles

### 1.1 Core Philosophy

AlternateOne prioritizes **deterministic correctness** over raw performance:

- **Single-lock ingress:** One mutex protects all queue operations
- **Full-clear chunks:** All 128 slots cleared on return (defense-in-depth)
- **Strict state validation:** Invalid transitions panic immediately
- **Conservative check-then-sleep:** Lock held through sleep decision
- **Write-lock polling:** Exclusive access during I/O poll
- **Serial shutdown:** Explicit phases with logging

### 1.2 Trade-offs

| Aspect | AlternateOne Choice | Cost |
|--------|---------------------|------|
| Ingress Lock | Single mutex for all lanes | Lower throughput under contention |
| Chunk Clear | Clear all 128 slots always | CPU overhead per chunk return |
| Poll Lock | Write lock during poll | No concurrent registration during poll |
| State Transitions | Panic on invalid | Harder to debug in production |
| Shutdown | Serial phases | Slightly longer shutdown time |

---

## II. Architecture

### 2.1 Component Overview

```
┌─────────────────────────────────────────────────────────┐
│                         Loop                             │
├─────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │ SafeIngress  │  │ SafePoller   │  │ SafeState    │  │
│  │ (Single Lock)│  │ (Write Lock) │  │ (Validated)  │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
├─────────────────────────────────────────────────────────┤
│  ┌──────────────────────────────────────────────────┐   │
│  │              ShutdownManager                       │  │
│  │   Phase1 → Phase2 → Phase3 → Phase4 → Complete   │  │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

### 2.2 SafeStateMachine

Strict state transitions with validation:

```go
type SafeStateMachine struct {
    state    atomic.Int32
    mu       sync.Mutex
    observer StateObserver
}

func (s *SafeStateMachine) Transition(from, to LoopState) bool {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    if !isValidTransition(from, to) {
        panic(fmt.Sprintf("invalid transition: %s -> %s", from, to))
    }
    
    return s.state.CompareAndSwap(int32(from), int32(to))
}
```

### 2.3 SafeIngress

Single-lock ingress with three lanes:

```go
type SafeIngress struct {
    mu         sync.Mutex
    external   *taskList  // External submissions
    internal   *taskList  // Internal priority tasks
    microtasks *taskList  // Microtask queue
    closed     bool
}

func (s *SafeIngress) Push(fn func(), lane Lane) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    if s.closed {
        return ErrIngressClosed
    }
    
    switch lane {
    case LaneExternal:
        s.external.push(fn)
    case LaneInternal:
        s.internal.push(fn)
    case LaneMicrotask:
        s.microtasks.push(fn)
    }
    return nil
}
```

### 2.4 SafePoller

Write-lock during poll for absolute safety:

```go
func (p *SafePoller) PollIO(timeout int) (int, error) {
    p.mu.Lock()  // Write lock, not RLock
    defer p.mu.Unlock()
    
    // Capture FDs
    epfd := p.epfd
    events := p.eventBuf
    
    // Poll (lock held - but this is SafePoller, prioritizing safety)
    n, err := unix.EpollWait(epfd, events, timeout)
    if err != nil {
        return 0, err
    }
    
    // Process events (still under lock)
    for i := 0; i < n; i++ {
        // ...
    }
    return n, nil
}
```

---

## III. Synchronization Design

### 3.1 Completion Signaling (NO POLLING)

AlternateOne uses a **completion channel**, not polling:

```go
type Loop struct {
    loopDone chan struct{}  // Signals loop termination
    loopWg   sync.WaitGroup // Tracks loop goroutine
}

func (l *Loop) Run(ctx context.Context) error {
    l.loopDone = make(chan struct{})
    l.loopWg.Add(1)
    
    defer func() {
        l.loopWg.Done()
        close(l.loopDone)
    }()
    
    return l.run(ctx)
}

func (l *Loop) Shutdown(ctx context.Context) error {
    // ... transition to terminating ...
    
    // Wait via channel, NOT polling
    select {
    case <-l.loopDone:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

### 3.2 Unstarted Loop Handling

Handled via **state machine atomicity**, not sleeping:

```go
func (l *Loop) shutdownImpl(ctx context.Context) error {
    for {
        current := l.state.Load()
        if current == StateTerminated {
            return ErrLoopTerminated
        }
        
        if l.state.Transition(current, StateTerminating) {
            // Did we transition from Awake?
            if current == StateAwake {
                // Loop was never started - we own cleanup
                // Run()'s CAS will fail if it races here
                l.closeFDs()
                l.state.ForceTerminated()
                if l.loopDone != nil {
                    close(l.loopDone)
                }
                return nil
            }
            break
        }
    }
    
    // Loop is running - wait for it
    // ...
}
```

### 3.3 Check-Then-Sleep Protocol

Conservative approach: lock held through decision:

```go
func (l *Loop) poll(ctx context.Context) {
    // Transition to sleeping
    if !l.state.Transition(StateRunning, StateSleeping) {
        return
    }
    
    // SAFETY: Hold lock through sleep decision
    l.ingress.Lock()
    queueLen := l.ingress.Length()
    l.ingress.Unlock()
    
    if queueLen > 0 {
        l.state.Transition(StateSleeping, StateRunning)
        return
    }
    
    // Safe to sleep now
    _, _ = l.poller.PollIO(timeout)
    
    l.state.Transition(StateSleeping, StateRunning)
}
```

---

## IV. Error Handling

### 4.1 LoopError Type

Structured errors with context:

```go
type LoopError struct {
    Op      string      // Operation that failed
    Phase   string      // Lifecycle phase
    Cause   error       // Underlying error
    Context map[string]any  // Additional context
}

func (e *LoopError) Error() string {
    return fmt.Sprintf("alternateone: %s during %s: %v", e.Op, e.Phase, e.Cause)
}
```

### 4.2 PanicError Type

Full panic information:

```go
type PanicError struct {
    Value      any
    TaskID     uint64
    LoopID     uint64
    stackTrace string
}

func (e *PanicError) StackTrace() string {
    return e.stackTrace
}
```

---

## V. Shutdown Sequence

### 5.1 ShutdownManager

Serial phases with explicit boundaries:

```go
type ShutdownManager struct {
    loop   *Loop
    phases []ShutdownPhase
}

func (s *ShutdownManager) Execute(ctx context.Context) error {
    for _, phase := range s.phases {
        log.Printf("alternateone: shutdown phase: %s", phase.Name())
        if err := phase.Execute(ctx); err != nil {
            return &LoopError{Op: "shutdown", Phase: phase.Name(), Cause: err}
        }
    }
    return nil
}
```

### 5.2 Phase Order

1. **Phase 1:** Stop accepting external submissions
2. **Phase 2:** Drain external queue
3. **Phase 3:** Drain internal queue
4. **Phase 4:** Drain microtasks
5. **Phase 5:** Close FDs

---

## VI. Verification

### 6.1 Tests

```bash
# Run AlternateOne tests
go test -v -race ./eventloop/internal/alternateone/...

# Stress test
go test -v -race -count=100 ./eventloop/internal/alternateone/...
```

### 6.2 Coverage Requirements

- All state transitions tested
- All error paths covered
- Panic recovery verified
- Concurrent access stress tested

---

## VII. When to Use AlternateOne

**Choose AlternateOne when:**

- Correctness is paramount
- Debugging ease is important
- Performance is not critical
- Running in development/testing
- Need comprehensive error information

**Avoid AlternateOne when:**

- Maximum throughput required
- Latency-sensitive workload
- High contention expected
