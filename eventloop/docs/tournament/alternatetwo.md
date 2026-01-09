# Alternate Implementation Two: Maximum Performance Architecture

This specification defines a "Maximum Performance" event loop implementation that prioritizes **throughput**, **zero allocations**, and **minimal latency** over defensive safety measures. Every design decision favors speed over safety margins.

---

## Philosophy: Performance-First Design

1. **Zero Allocations on Hot Paths**: No `make()`, no interface boxing, no closures on critical paths
2. **Lock-Free Where Possible**: Use atomics and CAS loops instead of mutexes
3. **Cache-Line Awareness**: Align data structures to avoid false sharing
4. **Batch Operations**: Amortize overhead across multiple operations
5. **Assume Correct Usage**: Skip validation that slows down correct code

---

## P1. State Machine: Lock-Free with Optimistic Updates

### P1.1. Pure Atomic Operations

No mutex protection, pure CAS loops:

```go
type FastState struct {
    _ [64]byte // Cache line padding
    v atomic.Uint64
    _ [56]byte // Pad to full cache line
}

func (s *FastState) TryTransition(from, to LoopState) bool {
    return s.v.CompareAndSwap(uint64(from), uint64(to))
}
```

### P1.2. Optimistic State Reads

Skip validation on reads (trust the writer):

```go
func (s *FastState) Load() LoopState {
    return LoopState(s.v.Load())
    // No validation - trust the value
}
```

---

## P2. Ingress Queue: Lock-Free MPSC

### P2.1. Lock-Free Multi-Producer Single-Consumer Queue

Use atomic CAS for producers, single-threaded consumer:

```go
type LockFreeIngress struct {
    _       [64]byte
    head    atomic.Pointer[node]
    _       [56]byte
    tail    atomic.Pointer[node]
    _       [56]byte
    stub    node
}

type node struct {
    task Task
    next atomic.Pointer[node]
}

func (q *LockFreeIngress) Push(t Task) {
    n := nodePool.Get().(*node)
    n.task = t
    n.next.Store(nil)
    
    prev := q.tail.Swap(n)
    prev.next.Store(n) // Linearization point
}
```

### P2.2. Batched Pop

Consumer pops in batches to amortize cache misses:

```go
func (q *LockFreeIngress) PopBatch(buf []Task, max int) int {
    count := 0
    head := q.head.Load()
    
    for count < max {
        next := head.next.Load()
        if next == nil {
            break
        }
        buf[count] = next.task
        next.task = Task{} // GC safety
        q.head.Store(next)
        nodePool.Put(head)
        head = next
        count++
    }
    
    return count
}
```

### P2.3. Minimal Chunk Clearing

Only clear used slots:

```go
func returnChunkFast(c *chunk) {
    // Only clear up to pos (not all 128)
    for i := 0; i < c.pos; i++ {
        c.tasks[i] = Task{}
    }
    c.pos = 0
    c.readPos = 0
    c.next = nil
    chunkPool.Put(c)
}
```

---

## P3. Check-Then-Sleep: Minimal Barriers

### P3.1. Single Memory Barrier

Use a single explicit fence instead of mutex:

```go
func (l *Loop) prepareForSleep() bool {
    // Optimistic state transition
    if !l.state.TryTransition(StateRunning, StateSleeping) {
        return false
    }
    
    // Single memory barrier
    atomic.ThreadFence()
    
    // Quick length check (may have false negatives, will retry)
    if atomic.LoadInt64(&l.queueLen) > 0 {
        l.state.TryTransition(StateSleeping, StateRunning)
        return false
    }
    
    return true
}
```

### P3.2. Batched Wake-Up Coalescing

Aggressive wake-up elision:

```go
func (l *Loop) maybeWake() {
    // Only wake if definitely sleeping AND no pending signal
    if l.state.Load() != StateSleeping {
        return
    }
    
    // Try to claim wake responsibility
    if !l.wakePending.CompareAndSwap(0, 1) {
        return // Someone else is waking
    }
    
    // Single write, no retry loop
    unix.Write(l.wakeFD, l.wakeBuf[:])
}
```

---

## P4. Poller: Maximum Throughput

### P4.1. No Lock During Poll

Release all locks and use version counting for safe access:

```go
type FastPoller struct {
    _        [64]byte
    epfd     int32
    _        [60]byte
    version  atomic.Uint64
    _        [56]byte
    eventBuf [256]unix.EpollEvent // Larger buffer, stack allocated
    fds      [65536]fdInfo        // Direct indexing, no map
}

func (p *FastPoller) pollIO(timeoutMs int) int {
    // No lock - version-based consistency
    v := p.version.Load()
    
    n, _ := unix.EpollWait(p.epfd, p.eventBuf[:], timeoutMs)
    
    // Check version after syscall
    if p.version.Load() != v {
        // Poller was modified, results may be stale - discard
        return 0
    }
    
    return n
}
```

### P4.2. Direct FD Indexing

Use array instead of map for O(1) lookup with zero allocation:

```go
func (p *FastPoller) RegisterFD(fd int, events uint32, cb IOCallback) error {
    if fd < 0 || fd >= len(p.fds) {
        return ErrFDOutOfRange
    }
    
    p.fds[fd] = fdInfo{callback: cb, events: events, active: true}
    p.version.Add(1) // Invalidate any in-flight polls
    
    return unix.EpollCtl(p.epfd, unix.EPOLL_CTL_ADD, fd, &unix.EpollEvent{
        Events: events,
        Fd:     int32(fd),
    })
}
```

### P4.3. Inline Callback Execution

Execute callbacks inline without collecting:

```go
func (p *FastPoller) dispatchEvents(n int) {
    for i := 0; i < n; i++ {
        fd := int(p.eventBuf[i].Fd)
        info := &p.fds[fd]
        if info.active && info.callback != nil {
            info.callback(IOEvents{
                Fd:     fd,
                Events: EventMask(p.eventBuf[i].Events),
            })
        }
    }
}
```

---

## P5. Timer: Hierarchical Wheel

### P5.1. Four-Level Timer Wheel

O(1) insertion and expiration:

```go
type TimerWheel struct {
    // Level 0: 256 slots, 1ms each = 256ms range
    level0 [256]timerList
    // Level 1: 64 slots, 256ms each = 16.4s range
    level1 [64]timerList
    // Level 2: 64 slots, 16.4s each = 17.5min range
    level2 [64]timerList
    // Level 3: 64 slots, 17.5min each = 18.6hr range
    level3 [64]timerList
    
    cursor [4]int
    tick   int64
}

func (w *TimerWheel) Schedule(delay time.Duration, cb func()) *Timer {
    ticks := int64(delay / time.Millisecond)
    level, slot := w.findSlot(ticks)
    
    t := timerPool.Get().(*Timer)
    t.callback = cb
    t.expiry = w.tick + ticks
    
    w.levels[level][slot].append(t)
    return t
}
```

---

## P6. Memory: Arena Allocation

### P6.1. Pre-Allocated Task Arena

Avoid GC pressure with arena allocation:

```go
type TaskArena struct {
    _      [64]byte
    buffer [65536]Task
    head   atomic.Uint32
    _      [60]byte
}

func (a *TaskArena) Alloc() *Task {
    idx := a.head.Add(1) - 1
    return &a.buffer[idx%65536]
}
```

### P6.2. Object Pooling Everywhere

```go
var (
    nodePool   = sync.Pool{New: func() any { return &node{} }}
    timerPool  = sync.Pool{New: func() any { return &Timer{} }}
    resultPool = sync.Pool{New: func() any { return &Result{} }}
)
```

---

## P7. Microtask: Ring Buffer

### P7.1. Lock-Free Ring Buffer

```go
type MicrotaskRing struct {
    _        [64]byte
    buffer   [4096]func()
    head     atomic.Uint64 // Consumer
    _        [56]byte
    tail     atomic.Uint64 // Producer
    _        [56]byte
}

func (r *MicrotaskRing) Push(fn func()) bool {
    for {
        tail := r.tail.Load()
        head := r.head.Load()
        if tail-head >= 4096 {
            return false // Full
        }
        if r.tail.CompareAndSwap(tail, tail+1) {
            r.buffer[tail%4096] = fn
            return true
        }
    }
}
```

---

## P8. Shutdown: Fast Path

### P8.1. Optimistic Shutdown

Skip draining if queues appear empty:

```go
func (l *Loop) shutdown() {
    // Atomic transition
    l.state.Store(StateTerminating)
    
    // Quick check - skip draining if empty
    if l.queueLen.Load() == 0 && l.microtasks.IsEmpty() {
        goto cleanup
    }
    
    // Drain with timeout
    deadline := time.Now().Add(100 * time.Millisecond)
    for time.Now().Before(deadline) {
        if !l.drainOnce() {
            break
        }
    }
    
cleanup:
    l.state.Store(StateTerminated)
    l.closeFDsFast()
    close(l.done)
}
```

---

## P9. I/O Batching

### P9.1. Vectored I/O

Use `readv`/`writev` for batched I/O:

```go
func (l *Loop) batchRead(fds []int, buffers [][]byte) []int {
    // Batch multiple reads into single syscall
    iovecs := make([]unix.Iovec, len(fds))
    // ... setup iovecs
    unix.Readv(fds[0], iovecs)
}
```

---

## P10. Performance Expectations

| Metric | Target | Notes |
|--------|--------|-------|
| Task latency | <10µs | P99 |
| Lock contention | Near-zero | Lock-free critical paths |
| Allocations | 0 on hot paths | Arena + pools |
| Max throughput | 1M+ tasks/sec | Under ideal conditions |
| Memory overhead | Higher | Pre-allocation trades memory for speed |

---

## P11. Key Differentiators from Main Implementation

| Aspect | Main | AlternateTwo (Performance) |
|--------|------|---------------------------|
| Queue | Mutex + chunked list | Lock-free MPSC |
| Poller FD storage | Map | Direct array indexing |
| Timer | Binary heap | Hierarchical wheel |
| Memory | GC-managed pools | Arena + aggressive pooling |
| Callbacks | Collect-then-execute | Inline execution |
| Validation | Present | Minimal/skipped |
| Error handling | Comprehensive | Fast path only |

---

## P12. Safety Trade-offs (Acknowledged)

This implementation accepts these risks for performance:

1. **No invariant validation**: Bugs manifest as corruption, not panics
2. **Optimistic locking**: Race conditions possible under extreme load
3. **Minimal error handling**: Some errors silently ignored
4. **Direct array indexing**: FDs > 65535 cause undefined behavior
5. **Version-based consistency**: Stale data possible during modifications

**Use only when**: Performance is critical AND usage patterns are well-understood AND extensive testing validates correctness.
