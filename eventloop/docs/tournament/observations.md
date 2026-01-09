# Tournament Observations

**Tournament Date:** 2026-01-09  
**Observer:** Automated Analysis  
**Platform:** darwin/arm64, Go 1.25.5

---

## Raw Observations by Event

### T1: Shutdown Conservation

#### Behavioral Patterns Observed

1. **AlternateTwo's Aggressive Processing**
    - Consistently executed ~96% of tasks before shutdown completed
    - Indicates minimal blocking in ingress path
    - Single iteration achieved 100% execution (10,000/10,000)

2. **Main's Balanced Behavior**
    - Executed ~56-60% of tasks during shutdown
    - Shows deliberate pacing between ingress acceptance and shutdown signaling
    - Consistent variance across 10 stress iterations

3. **AlternateOne's Conservative Approach**
    - Executed ~50-51% of tasks
    - Phased shutdown logging visible in output
    - Separate phases: ingress → internal → microtasks → timers → promises → fds

#### Notable Log Patterns (AlternateOne)

```
alternateone: shutdown phase=ingress status=start
alternateone: shutdown phase=ingress status=complete
alternateone: shutdown phase=internal status=start
... (6 phases total)
```

---

### T2: Race Wakeup

#### Concurrency Behavior

1. **Timing Sensitivity**
    - All implementations handled 100 rapid iterations without failure
    - Race detector active throughout—no data races detected

2. **Aggressive Variant**
    - Additional stress test with tighter timing
    - All implementations: 0 failures

#### No Anomalies Detected

- Zero lost wakeups across all test configurations
- Proper memory barriers in place

---

### T3: Ping-Pong Throughput

#### Inferred Characteristics

| Implementation | Implied Overhead                  |
|----------------|-----------------------------------|
| AlternateTwo   | Minimal lock contention           |
| Main           | Moderate synchronization          |
| AlternateOne   | Additional safety checks per task |

*Note: Detailed benchmark numbers require separate benchmark run without race detector.*

---

### T4: Multi-Producer Stress

#### Quantitative Results

| Metric      | Main          | AlternateOne  | AlternateTwo      |
|-------------|---------------|---------------|-------------------|
| Throughput  | 600,225 ops/s | 387,763 ops/s | **975,013 ops/s** |
| Executed    | 100,000       | 100,000       | 100,000           |
| Rejected    | 0             | 0             | 0                 |
| P99 Latency | **2.814ms**   | 147.291ms     | 28.447ms          |

#### Analysis

1. **Throughput Distribution**
    - AlternateTwo: 62.5% faster than Main
    - AlternateTwo: 151% faster than AlternateOne
    - Main: 55% faster than AlternateOne

2. **Latency Characteristics**
    - Main: Excellent P99 (2.8ms) suggests fair scheduling
    - AlternateTwo: Moderate P99 (28ms) suggests batching behavior
    - AlternateOne: High P99 (147ms) reflects safety overhead

3. **Zero Rejections**
    - All implementations handled 100% of submissions
    - No backpressure triggered under test load

---

### T5: Panic Isolation

#### Recovery Behavior

1. **Main**
    - Silent recovery with log: `ERROR: eventloop: task panicked: <message>`
    - Continues processing subsequent tasks

2. **AlternateOne**
    - Full stack trace capture:
   ```
   alternateone: task N panicked: <message>
   goroutine X [running, locked to thread]:
   runtime/debug.Stack()
   ... (full stack)
   ```
    - Panic error wrapping via `NewPanicError()`
    - Diagnostic-rich output

3. **AlternateTwo**
    - Minimal output, fast recovery
    - No visible logging in test output

#### Test Variants Passed

| Variant         | Description                    | All Pass |
|-----------------|--------------------------------|----------|
| Single panic    | One panicking task             | ✅        |
| Multiple panics | 10 panicking tasks interleaved | ✅        |
| Internal panic  | Panic in SubmitInternal path   | ✅        |

---

### T6: GC Pressure

#### Memory Observations

| Implementation | Baseline  | After Load | Delta  |
|----------------|-----------|------------|--------|
| Main           | 239,320 B | 239,720 B  | +400 B |
| AlternateOne   | 240,272 B | 240,752 B  | +480 B |
| AlternateTwo   | 242,264 B | 242,920 B  | +656 B |

#### Interpretation

- All deltas < 1KB indicate no significant leaks
- AlternateTwo's slightly higher delta consistent with higher throughput (more allocations total)
- Stable across 3 samples each

---

### T7: Concurrent Stop

#### Synchronization Behavior

1. **Basic Concurrent Stop**
    - 10 goroutines calling Stop() simultaneously
    - All complete without deadlock
    - Duration: ~0.12s per implementation

2. **Stop With Active Submits**
    - Concurrent Stop() + Submit() calls
    - Proper rejection of post-stop submissions
    - No panics or hangs

3. **Repeated Stops**
    - 10 sequential Stop() calls on same instance
    - Idempotent behavior confirmed
    - No double-free or corruption

---

## Cross-Cutting Observations

### Thread Affinity (AlternateOne)

AlternateOne's panic stack traces show:

```
goroutine X [running, locked to thread]
```

This indicates explicit thread affinity for the main loop goroutine, potentially for:

- Consistent cache locality
- Deterministic scheduling
- Platform-specific optimizations

### Shutdown Phase Verbosity

AlternateOne uniquely logs 6 distinct shutdown phases:

1. `ingress` - External task queue
2. `internal` - Internal task queue
3. `microtasks` - Microtask queue
4. `timers` - Timer heap
5. `promises` - Promise callbacks
6. `fds` - File descriptor cleanup

This provides operational visibility but adds log volume.

### Error Handling Patterns

| Implementation | Panic Handling         | Error Wrapping      |
|----------------|------------------------|---------------------|
| Main           | defer/recover, log     | Simple string       |
| AlternateOne   | defer/recover, stack   | `PanicError` struct |
| AlternateTwo   | defer/recover, minimal | Minimal             |

---

## Anomalies & Edge Cases

### None Detected

All tests passed with:

- Zero race conditions (race detector enabled)
- Zero deadlocks
- Zero data loss
- Zero unexpected panics

### Test Flakiness

- **Not observed**: All tests passed consistently across runs
- Stress tests (10 iterations each) showed stable behavior
- No timing-dependent failures

---

## Recommendations for Future Testing

1. **Add Benchmark Mode**
    - Disable race detector for accurate performance numbers
    - Increase iteration count for statistical significance

2. **Extend GC Pressure Tests**
    - Longer duration (minutes, not seconds)
    - Larger allocation patterns

3. **Add Network I/O Tests**
    - Real poller stress testing
    - File descriptor limits

4. **Cross-Platform Validation**
    - Linux-specific poller tests (epoll vs kqueue)
    - Windows compatibility

---

## Summary

The tournament revealed three well-engineered implementations with distinct characteristics:

| Aspect      | Winner       | Notes       |
|-------------|--------------|-------------|
| Correctness | Tie          | All 100%    |
| Throughput  | AlternateTwo | 975K ops/s  |
| Latency     | Main         | 2.8ms P99   |
| Diagnostics | AlternateOne | Full traces |
| Memory      | Tie          | All stable  |

All implementations are production-viable with appropriate use-case matching.
