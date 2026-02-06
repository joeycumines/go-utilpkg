# Performance Tuning Guide

This guide covers performance optimization for the `eventloop` package, including
configuration options, profiling techniques, and platform-specific considerations.

## Table of Contents

1. [FastPathMode Settings](#fastpathmode-settings)
2. [Chunk Size Considerations](#chunk-size-considerations)
3. [Timer Usage Patterns](#timer-usage-patterns)
4. [Microtask vs Timer Tradeoffs](#microtask-vs-timer-tradeoffs)
5. [Profiling with pprof](#profiling-with-pprof)
6. [Benchmark Results Interpretation](#benchmark-results-interpretation)
7. [Memory Allocation Hotspots](#memory-allocation-hotspots)
8. [Platform-Specific Performance Notes](#platform-specific-performance-notes)

---

## FastPathMode Settings

FastPathMode controls how the event loop handles task execution when no I/O file
descriptors are registered. The right setting can significantly impact latency
and throughput.

### Available Modes

```go
type FastPathMode int32

const (
    FastPathAuto    FastPathMode = iota  // Default: automatic selection
    FastPathForced                        // Always use fast path (no I/O)
    FastPathDisabled                      // Always use poll path (debugging)
)
```

### When to Use Each Mode

#### FastPathAuto (Default)
```go
loop, err := New() // FastPathAuto is default
```

**Best for:**
- Most applications
- Mixed workloads (some I/O, some CPU-bound)
- Production environments where conditions vary

**Behavior:**
- Uses fast path (channel-based) when `userIOFDCount == 0`
- Switches to poll path when I/O FDs are registered
- ~50ns wakeup latency in fast path
- ~10µs wakeup latency in poll path

#### FastPathForced
```go
loop, err := New(WithFastPathMode(FastPathForced))
// OR
loop.SetFastPathMode(FastPathForced)
```

**Best for:**
- Pure compute workloads (no file I/O)
- High-frequency task scheduling (>10k tasks/sec)
- Latency-sensitive applications without I/O
- Testing fast path behavior

**Behavior:**
- Always uses channel-based wakeup
- Returns `ErrFastPathIncompatible` if I/O FDs are registered
- Minimum wakeup latency (~50ns)

**⚠️ Constraints:**
- Cannot register I/O file descriptors
- `RegisterFD()` will return error

#### FastPathDisabled
```go
loop, err := New(WithFastPathMode(FastPathDisabled))
```

**Best for:**
- Debugging I/O-related issues
- Testing poll path behavior
- Ensuring consistent behavior across conditions
- Comparing performance between paths

**Behavior:**
- Always uses poll path (kqueue/epoll/IOCP)
- Locks thread to OS thread (required by poll APIs)
- ~10µs wakeup latency

### Runtime Mode Switching

```go
// Switch to forced fast path (fails if I/O FDs registered)
if err := loop.SetFastPathMode(FastPathForced); err != nil {
    // Handle ErrFastPathIncompatible
}

// Switch to disabled (always use poll)
loop.SetFastPathMode(FastPathDisabled)

// Switch back to auto
loop.SetFastPathMode(FastPathAuto)
```

### Performance Comparison

| Mode | Wakeup Latency | Thread Lock | I/O Support | Use Case |
|------|----------------|-------------|-------------|----------|
| FastPathAuto | 50ns-10µs | Dynamic | Yes | General purpose |
| FastPathForced | ~50ns | No | No | Pure compute |
| FastPathDisabled | ~10µs | Yes | Yes | I/O-heavy, debugging |

---

## Chunk Size Considerations

The event loop uses `ChunkedIngress` for external task queues with chunked
allocation strategy.

### Default Configuration

```go
const (
    chunkSize     = 128  // Tasks per chunk (1KB per chunk)
    initialChunks = 1    // Start with one chunk
)
```

### Workload Considerations

#### Low-Volume Workloads (<100 tasks/tick)
- Default chunk size (128) is optimal
- Single chunk sufficient
- No memory waste

#### Medium-Volume Workloads (100-1000 tasks/tick)
- Multiple chunks allocated dynamically
- Chunks recycled via sync.Pool
- Near-zero allocation after warmup

#### High-Volume Workloads (>1000 tasks/tick)
- Consider pre-warming with dummy Submit calls
- Monitor queue depth via Metrics
- External queue budget: 1024 tasks per tick (then overflow)

### Queue Depth Monitoring

```go
loop, _ := New(WithMetrics(true))

// Periodically check metrics
metrics := loop.Metrics()
fmt.Printf("External queue: current=%d, max=%d, EMA=%.2f\n",
    metrics.QueueCurrent.Ingress,
    metrics.QueueMax.Ingress,
    metrics.QueueEMA.Ingress)
```

### Overflow Handling

```go
loop, _ := New(WithOnOverload(func(err error) {
    // Called when external queue exceeds budget
    // Options: log, backpressure, circuit breaker
    log.Printf("Queue overload: %v", err)
}))
```

---

## Timer Usage Patterns

### Efficient Timer Scheduling

#### Single-Shot Timers (setTimeout)
```go
// Preferred: ScheduleTimer returns CancelTimer function
timerID, _ := loop.ScheduleTimer(100*time.Millisecond, func() {
    // Executed once after delay
})

// Cancel if needed
cancel, _ := loop.CancelTimer(timerID)
if cancel != nil {
    <-cancel // Wait for cancellation confirmation
}
```

#### Repeated Timers (setInterval)
```go
js, _ := NewJS(loop)

// SetInterval handles rescheduling internally
intervalID := js.SetInterval(func() {
    // Executed repeatedly
}, 100) // milliseconds

// Clear when done
js.ClearInterval(intervalID)
```

### Timer Pool Optimization

Timers are pooled via `sync.Pool`:
- Zero allocation for timer reuse in steady state
- ~7 allocations per new timer (result channel, closures)
- Pool pre-warmed on first timer creation

### HTML5 Nested Timeout Clamping

Per HTML5 spec, nested timeouts (depth > 5) are clamped to 4ms minimum:

```go
// These timers might fire at different times than expected:
js.SetTimeout(func() {          // depth 1
    js.SetTimeout(func() {      // depth 2
        js.SetTimeout(func() {  // depth 3
            js.SetTimeout(func() { // depth 4
                js.SetTimeout(func() { // depth 5
                    // Depth > 5: 0ms becomes 4ms minimum
                    js.SetTimeout(func() {
                        // This executes after 4ms, not 0ms
                    }, 0)
                }, 0)
            }, 0)
        }, 0)
    }, 0)
}, 0)
```

### Timer Ordering Guarantees

**Guaranteed:**
- Timers with earlier deadlines fire before later ones
- Order is consistent within same execution

**Not Guaranteed:**
- FIFO for same-delay timers (heap-based, not queue-based)
- Microsecond-precision ordering (depends on scheduling jitter)

---

## Microtask vs Timer Tradeoffs

### Microtask Characteristics

```go
loop.ScheduleMicrotask(func() {
    // Executes in current tick, before timers
})
```

**Pros:**
- Minimal latency (~30-80ns per microtask)
- Zero allocation in steady state (ring buffer)
- Executes before next timer/poll
- Guaranteed ordering (FIFO)

**Cons:**
- Blocks event loop until complete
- Can starve timers/I/O if overused
- No delay/scheduling control

**Best For:**
- Promise continuations
- State synchronization
- Immediate callbacks

### Timer Characteristics

```go
loop.ScheduleTimer(delay, func() {
    // Executes after delay
})
```

**Pros:**
- Precise delay control
- Doesn't block current tick
- Cancellable
- Works with HTML5 clamping

**Cons:**
- Higher overhead (~100-500ns per timer)
- ~7 allocations per new timer
- No exact-time guarantee

**Best For:**
- Delayed operations
- Throttling/debouncing
- Periodic tasks
- Animation frames

### When to Use process.nextTick()

```go
loop.ScheduleNextTick(func() {
    // Like microtask but runs BEFORE promise microtasks
})
```

**Use when:**
- Need to run before promise handlers
- Converting callback API to sync-like flow
- Breaking up recursive operations

---

## Profiling with pprof

### Basic CPU Profile

```bash
# Run with CPU profiling
go test -cpuprofile=cpu.prof -bench=. ./eventloop/

# Analyze
go tool pprof cpu.prof

# Interactive commands:
(pprof) top10           # Top 10 functions by CPU
(pprof) list ScheduleTimer  # Source annotation
(pprof) web             # Visual graph (needs graphviz)
```

### Memory Profile

```bash
# Run with memory profiling
go test -memprofile=mem.prof -bench=. ./eventloop/

# Analyze
go tool pprof mem.prof

# Key commands:
(pprof) top10 -cum          # Top by cumulative allocations
(pprof) list MicrotaskRing  # Source annotation
(pprof) alloc_space         # Total allocations (not just in-use)
```

### Block Profile (Contention)

```bash
# Run with block profiling
go test -blockprofile=block.prof -bench=. ./eventloop/

# Analyze
go tool pprof block.prof
(pprof) top10  # Shows where goroutines blocked on sync
```

### Trace Analysis

```bash
# Generate trace
go test -trace=trace.out -bench=BenchmarkSubmitFastPath ./eventloop/

# Analyze (opens web UI)
go tool trace trace.out
```

**Trace UI shows:**
- Goroutine scheduling
- Heap allocations
- Blocking events
- Syscalls

### Live Profiling

```go
import _ "net/http/pprof"

func main() {
    go func() {
        log.Println(http.ListenAndServe(":6060", nil))
    }()
    // Your application...
}
```

Then:
```bash
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
go tool pprof http://localhost:6060/debug/pprof/heap
```

---

## Benchmark Results Interpretation

### Running Benchmarks

```bash
# All benchmarks
go test -bench=. -benchmem ./eventloop/

# Specific benchmark
go test -bench=BenchmarkTimerSchedule -benchmem ./eventloop/

# With count for statistical significance
go test -bench=. -benchmem -count=5 ./eventloop/
```

### Understanding Output

```
BenchmarkTimerSchedule-8    5000000    234 ns/op    56 B/op    2 allocs/op
│                       │        │        │           │            │
│                       │        │        │           │            └─ Heap allocations
│                       │        │        │           └─ Bytes allocated
│                       │        │        └─ Time per operation
│                       │        └─ Number of iterations
│                       └─ GOMAXPROCS
└─ Benchmark name
```

### Key Metrics to Track

| Metric | Good | Warning | Action |
|--------|------|---------|--------|
| Submit ns/op | <100ns | >500ns | Check contention |
| Microtask ns/op | <50ns | >200ns | Check ring buffer |
| Timer ns/op | <300ns | >1µs | Check pool, heap |
| B/op | 0-100 | >500 | Profile allocations |
| allocs/op | 0-5 | >10 | Reduce closures |

### Comparing Benchmarks

```bash
# Install benchstat
go install golang.org/x/perf/cmd/benchstat@latest

# Run baseline
go test -bench=. -count=5 ./eventloop/ > baseline.txt

# Make changes, run again
go test -bench=. -count=5 ./eventloop/ > improved.txt

# Compare
benchstat baseline.txt improved.txt
```

---

## Memory Allocation Hotspots

### Zero-Allocation Paths (Steady State)

| Operation | Allocations | Notes |
|-----------|-------------|-------|
| MicrotaskRing.Push/Pop | 0 | Lock-free ring buffer |
| ChunkedIngress.Push/Pop | 0 | Chunk pool recycling |
| Fast path Submit | 0-2 | Slice growth amortized |
| Timer fire | 0 | Pool return |

### Allocation-Heavy Paths

| Operation | Allocations | Cause |
|-----------|-------------|-------|
| Timer schedule | ~7 | Result channel, closures |
| Promise creation | ~3 | Promise struct, closures |
| Error creation | 1+ | Error message strings |
| First chunk | 1 | Initial chunk allocation |

### Reducing Allocations

1. **Reuse closures where possible:**
```go
// Bad: new closure per call
for i := 0; i < 1000; i++ {
    loop.Submit(func() { process(i) })
}

// Better: capture via parameter
task := func(idx int) func() {
    return func() { process(idx) }
}
for i := 0; i < 1000; i++ {
    loop.Submit(task(i))
}
```

2. **Pre-allocate known sizes:**
```go
// If you know how many promises you need
promises := make([]*ChainedPromise, 0, expectedCount)
```

3. **Use ScheduleMicrotask over Submit for continuations:**
```go
// Lower overhead than Submit
loop.ScheduleMicrotask(continuation)
```

---

## Platform-Specific Performance Notes

### macOS (Darwin) - kqueue

**Characteristics:**
- `kqueue(2)` for I/O notification
- Pipe-based wakeup mechanism
- ~10µs poll wakeup latency
- Thread affinity required for `kevent(2)`

**Optimization Tips:**
- Batch FD registrations when possible
- Use `FastPathForced` if no I/O needed
- Avoid frequent RegisterFD/UnregisterFD

**Known Issues:**
- `kevent` has higher overhead than `epoll`
- Edge-triggered mode requires careful handling

### Linux - epoll

**Characteristics:**
- `epoll_wait(2)` for I/O notification
- `eventfd(2)` for wakeup (more efficient than pipe)
- ~8µs poll wakeup latency
- Thread affinity required for `epoll_wait`

**Optimization Tips:**
- `eventfd` is more efficient than macOS pipe
- Edge-triggered (`EPOLLET`) reduces syscalls
- Can handle more FDs efficiently (O(1) vs O(n))

**Performance Advantage:**
- ~20% lower wakeup latency than macOS
- Better scaling with many FDs

### Windows - IOCP

**Characteristics:**
- I/O Completion Ports for async I/O
- `PostQueuedCompletionStatus` for wakeup
- ~15µs poll wakeup latency
- Thread affinity NOT required

**Optimization Tips:**
- IOCP is fundamentally different (completion-based, not readiness-based)
- Works well with async/await patterns
- Different FD model (handles vs. file descriptors)

**Current Status:**
- Less optimized than Unix implementations
- Consider using `FastPathForced` if possible

### Cross-Platform Recommendations

1. **For consistent behavior:**
   - Use `FastPathAuto` (default)
   - Test on all target platforms during development
   - Use `-race` flag on all platforms

2. **For maximum performance:**
   - Platform-specific tuning may be needed
   - Linux generally has best I/O performance
   - Use benchmarks to compare

3. **For debugging:**
   - Use `FastPathDisabled` to force consistent poll path
   - Check platform-specific error messages
   - Profile on target platform

### Latency Summary by Platform

| Platform | Fast Path | Poll Path | Wake Mechanism |
|----------|-----------|-----------|----------------|
| All | ~50ns | - | chan struct{} |
| Darwin | - | ~10µs | pipe write |
| Linux | - | ~8µs | eventfd write |
| Windows | - | ~15µs | PostQCPS |

---

## Quick Reference

### Configuration Checklist

```go
loop, err := New(
    // Performance options
    WithFastPathMode(FastPathAuto),  // or Forced/Disabled
    WithMetrics(true),               // Enable if monitoring needed
    
    // Reliability options
    WithOnOverload(func(err error) {
        // Handle backpressure
    }),
)
```

### Monitoring Metrics

```go
metrics := loop.Metrics()

// Critical metrics to watch:
fmt.Printf("TPS: %.2f\n", metrics.TPS)
fmt.Printf("P99 Latency: %.2fms\n", metrics.Latency.P99)
fmt.Printf("Queue Depth: %d (max: %d)\n", 
    metrics.QueueCurrent.Ingress,
    metrics.QueueMax.Ingress)
```

### Performance Testing Pattern

```go
func BenchmarkMyWorkload(b *testing.B) {
    loop, _ := New()
    js, _ := NewJS(loop)
    
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    go loop.Run(ctx)
    defer loop.Shutdown(context.Background())
    
    b.ResetTimer()
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        // Your workload here
    }
}
```
