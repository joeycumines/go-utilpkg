// Package alternateone implements a "Maximum Safety" event loop variant.
//
// This implementation prioritizes correctness guarantees and defensive programming
// over raw performance. Every design decision favors preventing subtle bugs over
// micro-optimizations.
//
// # Philosophy: Safety-First Design
//
//  1. Fail-Fast over Fail-Silent: All error paths must be explicit and observable
//  2. Lock Coarseness over Granularity: Prefer holding locks longer to eliminate race windows
//  3. Allocation Tolerance: Accept allocations if they simplify correctness reasoning
//  4. Extensive Validation: Runtime invariant checks always enabled
//  5. Deterministic Behavior: No reliance on timing assumptions
//
// # API Contract
//
//   - Run(ctx) error: Starts the event loop, BLOCKS until fully stopped (NOT async)
//   - Shutdown(ctx) error: Graceful shutdown, drains queues, blocks until termination
//   - Close() error: Immediate termination, closes FDs without waiting (io.Closer)
//   - Submit(fn func()) error: Submit external task
//   - SubmitInternal(fn func()) error: Submit internal priority task
//   - ScheduleMicrotask, ScheduleTimer, RegisterFD, UnregisterFD: As in main
//
// # Key Differences from Main Implementation
//
//   - Single Mutex: Uses one mutex for entire ingress subsystem (vs fine-grained RWMutex)
//   - Full-Clear Always: Always clears all 128 chunk slots, no optimization
//   - Strict State Transitions: Panics on invalid state transitions
//   - Conservative Check-Then-Sleep: Holds lock through sleep decision
//   - Write Lock for Poll: Uses Lock() not RLock() for pollIO
//   - Serial Shutdown: Executes shutdown phases sequentially with logging
//   - Comprehensive Errors: All errors wrapped with full context
//
// # Performance Expectations
//
// This implementation accepts lower performance for correctness:
//   - Task latency: <100µs (acceptable)
//   - Lock contention: High (coarse locking accepted)
//   - Allocations: Tolerated (no zero-alloc requirement)
//   - Max throughput: ~100k tasks/sec (lower than performance variant)
package alternateone
