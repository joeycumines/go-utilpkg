// Package alternateone implements a "Maximum Safety" event loop variant.
//
// This implementation prioritizes **correctness guarantees** and **defensive programming**
// over raw performance. Every design decision favors preventing subtle bugs over
// micro-optimizations. This is a reference implementation that trades throughput for
// verifiable safety, making it ideal for development, debugging, and correctness-critical
// production scenarios.
//
// # Philosophy: Safety-First Design
//
// AlternateOne follows these core principles:
//
//  1. **Fail-Fast over Fail-Silent**: All error paths must be explicit and observable.
//     Panics are used for invariants violations, errors are wrapped with full context.
//
//  2. **Lock Coarseness over Granularity**: Prefer holding locks longer to eliminate
//     race windows. Uses a single mutex for the entire ingress subsystem rather than
//     fine-grained locking.
//
//  3. **Allocation Tolerance**: Accept allocations if they simplify correctness reasoning.
//     No zero-alloc requirements; clarity over optimization.
//
//  4. **Extensive Validation**: Runtime invariant checks always enabled, not just in
//     debug builds. Invalid state transitions panic immediately.
//
//  5. **Deterministic Behavior**: No reliance on timing assumptions. Wake-up retries
//     continue indefinitely for transient errors.
//
// # Architecture Overview
//
// AlternateOne consists of these main components:
//
// ```
// ┌─────────────────────────────────────────────────────────────────┐
// │                         Loop                                      │
// ├─────────────────────────────────────────────────────────────────┤
// │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
// │  │ SafeIngress  │  │ SafePoller   │  │ SafeState    │         │
// │  │ (Single Lock)│  │ (Write Lock) │  │ (Validated)  │         │
// │  └──────────────┘  └──────────────┘  └──────────────┘         │
// ├─────────────────────────────────────────────────────────────────┤
// │  ┌─────────────────────────────────────────────────────────┐   │
// │  │              ShutdownManager                              │   │
// │  │   Phase1 → Phase2 → Phase3 → Phase4 → Phase5 → Complete │   │
// │  └─────────────────────────────────────────────────────────┘   │
// └─────────────────────────────────────────────────────────────────┘
// ```
//
// # Synchronization Design
//
// The synchronization design is intentionally conservative:
//
// ## Completion Signaling
//
// AlternateOne uses a **completion channel** pattern, not polling:
//
//   - Loop creates loopDone channel during initialization (not in Run)
//   - Run() closes the channel before returning (in defer)
//   - Shutdown() waits on loopDone channel, with a context timeout
//   - This eliminates all time.Sleep/Ticker polling patterns
//
// ## Unstarted Loop Safety
//
// Handling of unstarted loops is done via **state machine atomicity**:
//
//   - Shutdown() checks current state atomically
//   - If in Awake state (never started), shutdown performs immediate cleanup
//   - CloseFDs() called directly, loopDone channel closed
//   - State forced to Terminated without waiting (no goroutine to wait for)
//
// ## Check-Then-Sleep Protocol
//
// Conservative approach: lock held through sleep decision:
//
//  1. Transition StateRunning → StateSleeping (atomic CAS)
//  2. Hold ingress lock while checking queue length
//  3. If queue has tasks, transition back to StateRunning
//  4. Otherwise safe to sleep (pollIO with timeout)
//  5. After poll, transition StateSleeping → StateRunning
//
// This eliminates the unlock-check-relock race condition.
//
// # API Contract
//
// ## Lifecycle Methods
//
//   - Run(ctx) error: Starts the event loop, BLOCKS until fully stopped
//     The channel loopDone is closed before returning. Context cancellation
//     triggers graceful shutdown.
//
//   - Shutdown(ctx) error: Graceful shutdown initiated, drains all queues,
//     blocks until loop termination. Uses loopDone channel to wait, with
//     context timeout support.
//
//   - Close() error: Immediate termination. Obeys io.Closer semantics.
//     Closes FDs without waiting for queues to drain.
//
// ## Task Submission
//
//   - Submit(fn func()) error: Submit external task for execution.
//     Returns error during shutdown.
//
//   - SubmitInternal(fn func()) error: Submit internal priority task.
//     Executes before external tasks; also for shutdown-exclusive work.
//
//   - ScheduleMicrotask(fn func()) error: Schedule a microtask for immediate
//     execution in the next iteration.
//
//   - Schedule(fn func(), delay time.Duration) (Canceler, error): Schedule a
//     one-time delayed task.
//
//   - ScheduleRepeating(fn func(), interval time.Duration) (Canceler, error):
//     Schedule a repeating task with fixed interval.
//
// ## I/O Operations
//
//   - RegisterFD(fd int, callback IOCallback) error: Register file descriptor
//     for I/O events.
//
//   - UnregisterFD(fd int) error: Unregister file descriptor from polling.
//
// # Key Design Decisions
//
// ## S1: Strict State Machine
//
// All state transitions are validated against a compile-time transition table.
// Invalid transitions panic immediately. State changes are observable via
// logging.
//
// Valid transitions:
//   - Awake → Running, Terminating
//   - Running → Sleeping, Terminating
//   - Sleeping → Running, Terminating
//   - Terminating → Terminated
//   - Terminated → (terminal state)
//
// ## S2: Single-Lock Ingress Queue
//
// The SafeIngress uses a single sync.Mutex for all three lanes (external,
// internal, microtasks). This simplifies correctness reasoning by eliminating
// lock ordering bugs and reducing race windows.
//
// Defensive validation: Every push/pop operation validates invariants
// (queue length non-negative, head/tail symmetry).
//
// ## S2.3: Full-Clear Chunks
//
// All chunk slots are cleared on return to the pool, regardless of how many
// slots were used. This is defense-in-depth against future modifications that
// might skip per-pop zeroing.
//
// Critical safety property: Tasks are zeroed before returning to pool, never
// leak references.
//
// ```go
//
//	func returnChunk(c *chunk) {
//	    for i := range c.tasks { // Always full iteration (128 slots)
//	        c.tasks[i] = Task{}
//	    }
//	    c.pos = 0
//	    c.readPos = 0
//	    c.next = nil
//	    chunkPool.Put(c)
//	}
//
// ```
//
// ## S3: Conservative Check-Then-Sleep
//
// Unlike the main implementation's unlock-check-relock pattern, AlternateOne
// holds the ingress lock through the sleep decision. This prevents the
// "spurious wake-up" race where a task arrives after unlock but before sleep.
//
// Trade-off: Slightly reduces throughput due to longer lock hold, but eliminates
// a subtle race condition entirely.
//
// ## S4: Write-Lock Polling
//
// SafePoller uses Lock() (write lock) for pollIO instead of RLock() (read lock).
// This blocks RegisterFD during poll but eliminates any risk of zombie poller
// access or concurrent modification bugs.
//
// Callbacks execute under lock. User code must not call RegisterFD/UnregisterFD
// from within a callback (documented limitation).
//
// ## S5: Serial Shutdown
//
// Shutdown executes phases serially with explicit phase markers:
//
//  1. Stop accepting external submissions
//  2. Drain external queue
//  3. Drain internal queue
//  4. Drain microtasks
//  5. Cancel timers
//  6. Reject promises
//  7. Close FDs
//
// Each phase is logged at start and completion, making shutdown behavior
// observable and debuggable.
//
// Stop() uses sync.Once to guarantee single execution. Subsequent callers
// receive ErrLoopTerminated.
//
// ## S7: Comprehensive Error Handling
//
// All errors include context via structured error types:
//
//   - LoopError: Contains operation, phase, cause, and context map
//   - PanicError: Captures panic value, task ID, loop ID, and full stack
//   - TransitionError: Invalid state transition details
//
// Panics are recovered with full stack trace capture and emission as errors,
// ensuring single-task failures don't crash the entire loop.
//
// # Key Differences from Main Implementation
//
// | Aspect | Main | AlternateOne (Safety) |
// |--------|------|----------------------|
// | Lock granularity | Fine (RWMutex per subsystem) | Coarse (single Mutex) |
// | Invariant checks | Disabled in prod | Always enabled |
// | Error handling | Silent drops | Explicit panics |
// | Callback execution | Outside lock | Inside lock |
// | Chunk clearing | Optimizable | Always full (128 slots) |
// | State transitions | CAS only | CAS + validation |
// | Poll locking | RLock during poll | Lock (write lock) |
// | Check-then-sleep | Unlock-check-relock | Lock held through decision |
// | Wake-up errors | Retry on EINTR only | Retry on all transient errors |
// | Shutdown | Immediate | Phased with logging |
// | Completion signaling | loopDone channel (shared) | loopDone channel (shared) |
//
// # Performance Expectations
//
// This implementation intentionally accepts lower performance for correctness.
// Metrics on typical workloads:
//
//   - Task latency: <100µs (acceptable for correctness-critical workloads)
//   - Lock contention: High under load (coarse locking is intentional)
//   - Allocations: Tolerated (no zero-alloc requirement)
//   - Max throughput: ~100k tasks/sec (lower than performance variants)
//   - Memory usage: Slightly higher due to always-clearing 128 slots per chunk
//
// # When to Use AlternateOne
//
// **Choose AlternateOne when:**
//
//   - Correctness is the highest priority
//   - Debugging ease is important (comprehensive logging and validation)
//   - Running in development or testing environments
//   - Need comprehensive error information for troubleshooting
//   - Workload is not extremely latency-sensitive
//   - Want a reference implementation for understanding safety trade-offs
//
// **Avoid AlternateOne when:**
//
//   - Maximum throughput is required
//   - Latency is extremely critical (<10µs)
//   - High contention from many producers is expected
//   - Production use at scale has already been validated with main implementation
//
// # Testing and Verification
//
// The test suite includes:
//
//   - State transition fuzzer: Random transitions, invalid must panic
//   - Shutdown ordering validator: Verify phase execution order
//   - Memory leak detector: Finalizer-based detection for all chunk returns
//   - Deadlock detector: Timeout-based detection for all operations
//   - Invariant stress test: Run invariant checks under heavy load
//   - Concurrent access stress: Race detector coverage
//
// To run tests:
//
//	# Run with race detector
//	go test -v -race ./eventloop/internal/alternateone/...
//
//	# Stress test (100 iterations)
//	go test -v -race -count=100 ./eventloop/internal/alternateone/...
//
// # Error Types
//
// ## LoopError
//
// Structured error with context about where and why an error occurred:
//
//	type LoopError struct {
//	    Op      string      // Operation that failed
//	    Phase   string      // Lifecycle phase
//	    Cause   error       // Underlying error
//	    Context map[string]any  // Additional context
//	}
//
// ## PanicError
//
// Captures full panic information including stack trace:
//
//	type PanicError struct {
//	    Value      any
//	    TaskID     uint64
//	    LoopID     uint64
//	    StackTrace string
//	}
//
// ## TransitionError
//
// Invalid state transition details for debugging state machine issues.
//
// # Implementation Files
//
//   - doc.go: This package documentation
//   - state.go: SafeStateMachine with transition validation
//   - ingress.go: SafeIngress single-lock queue with three lanes
//   - chunk.go: SafeTask, Lane, chunk, taskList, chunkPool
//   - poller_linux.go/darwin.go: SafePoller platform-specific implementations
//   - wakeup_linux.go/darwin.go: Platform-specific wake-up FD creation
//   - shutdown.go: Serial shutdown phases with logging
//   - errors.go: LoopError, PanicError, TransitionError types
//   - loop.go: Main Loop struct integrating all components
package alternateone
