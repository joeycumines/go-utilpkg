# Event Loop - Pruned & Compacted Requirements Document

Goal: To engineer a "Maximum-Effort," "From-Scratch" event loop in Go that rivals the behavior of Node.js (libuv) or similar reactor patterns—while remaining strictly Go-native.

---

## I. Loop State Machine (state.go)

### I-1. State Values

```go
package example

const (
	StateAwake       LoopState = 0 // Initial state, ready to start
	StateTerminated  LoopState = 1 // Loop has fully stopped (terminal)
	StateSleeping    LoopState = 2 // Loop is blocked in poll()
	StateTerminating LoopState = 3 // Shutdown requested
	StateRunning     LoopState = 4 // Loop is actively executing
)
```

### I-2. Valid State Transitions

```
StateAwake (0)  → StateRunning (4)  [Start()]
StateRunning (4)  → StateSleeping (2)  [poll() via CAS]
StateRunning (4)  → StateTerminating (3)  [Stop()]
StateSleeping (2)  → StateRunning (4)  [poll() wake via CAS]
StateSleeping (2)  → StateTerminating (3)  [Stop()]
StateTerminating (3) → StateTerminated (1)  [shutdown complete]
StateTerminated (1)  → (terminal)
```

### I-3. State Transition Rules

- Use `CompareAndSwap()` for temporary states (Running, Sleeping)
- Use `Store()` ONLY for irreversible states (Terminated, Terminating)
- Using `Store(Running)` or `Store(Sleeping)` is a BUG (breaks CAS logic)

### I-4. Critical State Transition Constraints

**D04 (Critical):** `Run(ctx)` must atomically transition the loop from `StateAwake` → `StateRunning` (CAS) to prevent multiple goroutines from concurrently executing the loop.

**D01 (Critical):** `poll()` must use CAS to restore `StateSleeping` → `StateRunning`, NOT `Store(Awake)`. If CAS fails (state changed to `Terminating`), poll must return immediately without overwriting the termination state.

### I-5. Loop Cycle (Tick Orchestration)

Each tick MUST execute operations in this exact order:

1. **Update Time:** Cache `time.Now()` at tick start (VII-1)
2. **Run Timers:** Execute all expired timer callbacks (VI-B)
3. **Process Ingress:** Internal first, then external (IV-3)
4. **Microtask Barrier:** Drain all microtasks (VI)
5. **Poll:** Block on I/O with calculated timeout (VIII)
6. **Process I/O:** Execute ready I/O callbacks (VIII)
7. **Microtask Barrier:** Drain all microtasks again (VI)

**Failure Modes:**

- Poll before Timers: Timer latency bugs
- Ingress before Time Update: Stale timestamps

---

## II. Memory Safety Invariants

### II-1. IngressQueue (ingress.go)

#### II-1-A. Memory Safety Properties (P1-P6)

**P1 — No Retained References After Chunk Return**

After a chunk is returned to the pool, **no Task, closure, or captured object remains reachable** via that chunk.

**Implementation Requirement:**

```go
package example

func returnChunk(c *chunk) {
	for i := 0; i < len(c.tasks); i++ { // CRITICAL: Clear ALL 128 slots
		c.tasks[i] = Task{}
	}
	c.pos = 0
	c.readPos = 0
	c.next = nil
	chunkPool.Put(c)
}
```

**Rationale:** Clearing all 128 slots (not just `c.pos`) provides defense-in-depth against:

- Future modifications to `popLocked` that skip per-pop zeroing
- Non-standard chunk returns (error handling/shutdown)
- Corrupted `pos` values

**Defense-in-Depth Requirement:** Clearing all 128 slots (not just `c.pos`) is MANDATORY to guard against: (a) corrupted `pos` values, (b) non-standard return paths during errors, (c) future modifications that skip per-pop zeroing.

**P2 — FIFO Semantic Correctness**

Tasks are popped in the same order they are pushed, across chunk boundaries.

**Verification:** Test with ordered task IDs across multiple chunks, verify order preserved.

**P3 — Queue Invariants Always Hold**

At all times (while locked):

- `length == number of unread tasks`
- `head == nil ⇔ tail == nil ⇔ length == 0`
- `readPos ≤ pos ≤ len(tasks)`

**P4 — Pool Reuse Does Not Resurrect Old Tasks**

A chunk obtained from `chunkPool.Get()` behaves identically to a newly allocated chunk.

**Verification:** After returning a chunk with tasks, retrieve from pool and verify all slots are zero.

**P5 — Exhausted-Chunk Transitions Are Correct**

Single-chunk and multi-chunk exhaustion paths behave correctly and do not drop or duplicate tasks.

**Critical Requirement:** The early and late exhaustion checks in `popLocked` MUST behave consistently.

**Early Exhaustion Check (Cursor Reset Optimization):**

```go
package example

func example() {
	if q.head.readPos >= q.head.pos {
		if q.head == q.tail {
			// Reset cursors instead of replacing chunk (optimization)
			q.head.pos = 0
			q.head.readPos = 0
			return Task{}, false
		}
		// Multiple chunks: advance and return old
		oldHead := q.head
		q.head = q.head.next
		returnChunk(oldHead)
	}
}
```

This maintains a chunk after first use (~3KB footprint) for simpler state machine and faster push after empty.

**P6 — Changes Remain Correct Under GC, Race Detector, and Stress**

GC cycles, pool reuse, and concurrent producer pressure do not break P1–P5.

#### II-1-B. Ingress Queue Complexity (D10 - Critical)

The ingress queue must be a mutex-protected chunked linked-list where popping is **O(1)** using `readPos`/`writePos` cursors.

**Requirements:**

- Array shifting is **FORBIDDEN**
- Entries cleared for GC safety after removal
- Single-chunk case resets cursors (optimization, prevents chunk thrashing)

### II-2. Registry (registry.go)

#### II-2-A. Hybrid Ring-Renewal Strategy

**Deterministic Discovery (Ring Buffer):**

- Maintain supplementary `[]ID` Circular Buffer mirroring all active Promise IDs
- Maintain persistent `scavengeCursor` index across ticks
- Per tick: Check next N IDs (e.g., 20) by advancing cursor
- If registry slot has nil value OR promise not Pending: delete map entry, swap-remove ID from ring or use null marker
- Wrap cursor when reaching end: ensures every slot checked exactly once per cycle

**Physical Reclamation - Map Renewal (D11):**

**Trigger:** During "Idle Reap" phase (Wait > 1s) or cycle completion

**Threshold (D20):** If `Count < (Capacity * 0.25)` (25%, NOT 12.5%):

- **Action:** Allocate a **NEW** Map
- **Migration:** Iterate current map, copying only live `weak.Pointer` entries to new map
- **Replace:** Atomically replace Global Registry reference

**Result:** Physically releases unused bucket array memory to OS/GC, eliminating "Bucket Ghost" memory leak.

**Map Renewal is MANDATORY (D11 - Critical):** `compact()` **must** allocate new map and migrate live entries; do NOT rely on deleting keys to free hashmap buckets.

#### II-2-B. Scavenge Criteria (D13 - Critical)

Scavenge must remove registry entries where:

- Weak pointer `Value()` is **nil**, OR
- Promise state is **not Pending** (settled promises must also be cleaned up)

#### II-2-C. Scavenge Rate (D15 - Critical)

Scavenge must run per-tick with small batch size (~20 IDs) for deterministic, timely cleanup.

#### II-2-D. Registry Type Safety

Registry must strictly map `ID -> weak.Pointer[Promise]`. Do **not** use `weak.Pointer[*Promise]`.

**Weak Pointer Types:**

- Generic type `T` in `weak.Pointer[T]` must be the underlying `struct` type (e.g., `PromiseStruct`), **not** the interface or the pointer type
- `weak.Make` must be called with the `*PromiseStruct` reference

**Rationale:** Strong references held by Active Systems (Timer Heap, Poller Map, Ingress Queue). Weak Registry is strictly for observability.

---

## III. Check-Then-Sleep Protocol (D03 - Critical)

### III-1. Loop Routine Logic (The "Double-Check Memory Barrier" Pattern)

The loop must strictly order the transition to sleep as follows:

1. **Atomic Store:** Set `LoopState` → `StateSleeping`
2. **Memory Barrier:** Execute a StoreLoad barrier (implied by atomic store on most archs, but logically required)
3. **IngressMu.Lock()**, inspect `IngressQueue.Length()`, **IngressMu.Unlock()**
4. **Branch:**

- **If Queue > 0:** Atomically Set `LoopState` → `StateRunning`, **abort** the transition to `epoll_wait`, and immediately jump to the Queue Processing phase
- **If Queue = 0:** Proceed to `epoll_wait`

This "Commit-and-Verify" pattern ensures that if a Producer enqueues a task between the store and the re-check, the Loop will detect it and abort sleep.

### III-2. Producer Logic (The "Write-Then-Check" Pattern)

Producers must adhere to strict ordering when enqueuing tasks:

1. **Enqueue:** Push the Task to the `IngressQueue`
2. **Memory Barrier:** Execute a StoreLoad barrier (implied by mutex unlock)
3. **Check:** Inspect `LoopState`
4. **Branch:**

- **If `StateSleeping`:** Perform `write()` syscall to Event Function
- **If `StateAwake` or `StateRunning`:** Elide syscall

This ensures that if the Loop is in the process of transitioning to sleep, the Producer will either:

- See the Loop as `StateSleeping` and wake it up (safe), or
- See the Loop as `StateAwake`/`StateRunning` and skip the syscall because the Loop will process the queue before sleep

### III-3. Critical Implementation Details

**Submit() Wake-Up (D03):**

`Submit()` must check the loop state **after** the enqueue mutex is released and, if the loop is sleeping, atomically request a wake-up (using a wake-up pending flag and platform wake mechanism).

**Wake-Up Signal Safety (D07):**

If submitting a wake-up fails (e.g., syscall error), the `wakeUpSignalPending` flag must be cleared to allow subsequent retry attempts.

**Wake-Up Retry Semantics (D07 Clarification):** `Submit` must **retry indefinitely** on transient errors (`EAGAIN`, `EINTR`) for the wake-up signal. It is UNSAFE to clear the pending flag on failure if other concurrent producers may have elided their signals.

**Wake-Up Implementation:**

- The `Loop Routine` must register the Wake-Up Primitive FD with the Poller
- The Wake-Up Primitive must be platform-appropriate (Linux: `eventfd`; BSD/Darwin: `kqueue EVFILT_USER` or Self-Pipe)

---

## IV. Ingress Queue Architecture

### IV-1. Priority Lanes

The Ingress Queue must support separate submission paths for external and internal tasks.

#### IV-1-A. External Ingress

- Subject to a **Hard Limit** (Tick Budget, e.g., 1024 tasks per tick)
- If budget is exhausted or queue depth exceeds a High-Water Mark (e.g., 100k items), the Loop must emit a "System Overload" signal or return `ErrLoopOverloaded`

**External Ingress Rejection Policy:** `Submit()` MUST reject new tasks with `ErrLoopTerminated` immediately once the loop transitions to `StateTerminating`. This ensures no new external work enters during shutdown while internal cleanup work continues.

#### IV-1-B. Internal Ingress

- Must utilize a **Priority Lane** (Reserved Capacity or Unbounded Bypass) that **never fails**
- Internal system completions (e.g., `Promisify` results) take precedence and must not be rejected due to load to ensure system liveness

### IV-2. SubmitInternal() Method (D09 - Critical)

The loop must expose `SubmitInternal(task Task)` as a priority submission path for internal completions.

**Requirements:**

- State checks that decide acceptance or rejection must be performed **while holding the internal lock** (to avoid TOCTOU races)
- `SubmitInternal` must accept tasks during `StateTerminating` (reject only when `StateTerminated`)
- If the loop is sleeping, it should be woken after enqueue

### IV-3. Processing Order (processIngress)

In `processIngress()`, the loop must:

1. **Drain ALL internal tasks FIRST** with **NO budget limit**
2. Then process external tasks with the tick budget

**Internal Queue Safety Valve:** While internal tasks have no budget limit, implementations SHOULD add a "Panic/Bailout" threshold (e.g., 100k internal tasks per tick) to detect runaway internal recursion and emit a diagnostic error.

### IV-4. Submission Semantics

`Submit()` APIs must **not** block the caller waiting for the consumer to process queued items.

The mutex lock acquisition must be fast and contention-free under normal operation; `Submit()` completes without waiting for queue consumption.

### IV-5. Overload Signal Emission (D17 - Critical)

When the external tick budget is exhausted and tasks remain in the queue, the loop must emit an `ErrLoopOverloaded` signal (e.g., via an `OnOverload` callback).

**Internal ingress must never fail** and always has priority.

---

## V. Shutdown Sequence (D16 - Critical)

The shutdown sequence must strictly follow this order:

**Shutdown() Idempotency (Critical):** The `Shutdown(ctx)` method MUST be idempotent and thread-safe. Multiple concurrent calls MUST all return correctly, utilizing `sync.Once` to ensure shutdown logic runs exactly once while all callers receive appropriate result (first caller gets actual result, subsequent callers get `ErrLoopTerminated`).

### V-1. Correct Order (Mandatory)

1. **Drain Ingress Tasks:** Stop accepting new and finish existing tasks
2. **Drain Internal Queue:** Process all completions
3. **Drain Microtasks:** Process all queued microtasks
4. **Set StateTerminated:** Atomically set loop state after drains complete
5. **Final Drains:** Perform final drains of internal and ingress queues and microtasks
6. **Reject All Pending Promises:** Via `registry.RejectAll` helper
7. **Close All FDs:** Close wakePipe and wakePipeWrite (poller FIRST, then FDs)
8. **Close Done Channel:** Signal loop completion

### V-2. Close() Immediate Termination (D16 - Critical)

The `Close()` method implements immediate termination semantics, bypassing graceful shutdown:

- ❌ Does NOT wait for in-flight work completion
- ❌ Closes all FDs immediately
- ❌ Drains remaining queues WITHOUT executing tasks
- ❌ Rejects all pending promises with termination error

**Data Loss Scenario (Wrong Order):**

1. Ingress Task A is submitted
2. Task A (when run) submits Internal Task B
3. Task B (when run) submits Microtask C
4. With wrong order: A runs → Microtasks drained (empty) → B runs → C queued → Shutdown completes → **C is lost**

### V-3. Shutdown() on Unstarted Loop (D16 - Critical)

`Shutdown(ctx)` must handle the case where the loop was never started (i.e., `New()` was called but `Run(ctx)` was not):

1. If the loop is in `StateAwake` when `Shutdown(ctx)` is called, `Shutdown()` must **NOT** block
2. `Shutdown()` must perform direct cleanup (close FDs) and return immediately

**Failure Mode:** Without this check, `Shutdown()` on an unstarted loop causes a permanent deadlock.

**Implementation:**

```go
package example

func (l *Loop) Shutdown(ctx context.Context) error {
	oldState := l.state.Swap(StateTerminating)
	if oldState == StateAwake {
		// Loop was never started, clean up directly
		l.closeFDs()
		return nil
	}
	// ... existing wait logic for running loop with context timeout
}
```

### V-4. FD Cleanup Order (D16 - Critical)

Shutdown must close all file descriptors opened by the loop:

**Order (Explicit):**

1. **Stop Event Delivery:** Close poller (`epfd`/`kq`) **FIRST** to stop event emission
2. **Close Wake Pipe FDs:** Close `wakePipe` and `wakePipeWrite` **AFTER** poller closed
3. **Guard Against Double-Close:** Check if `wakePipeWrite != wakePipe` before closing the write FD separately

**FD Initialization Guard:** `closeFDs()` must check if FDs are initialized (e.g., `> 0` or set to `-1` as sentinel) before closing. Alternatively, `New()` must initialize FDs to `-1` on failure paths.

**Implementation:**

```go
package example

func (l *Loop) closeFDs() {
	// Step 1: Stop event delivery
	l.ioPoller.closePoller()

	// Step 2: Close FDs (after event emission stopped)
	unix.Close(l.wakePipe)
	if l.wakePipeWrite != l.wakePipe {
		unix.Close(l.wakePipeWrite)
	}
}
```

**Failure Mode:** Without FD cleanup, repeated loop creation/destruction (e.g., in tests or dynamic worker scaling) will exhaust `ulimit -n`, crashing the process with "too many open files".

---

## VI. Microtask Barrier

### VI-1. Definition

The `DrainMicrotasks()` routine is a **Checkpoint**, not a Phase.

### VI-2. Barrier Requirement

The Loop must drain the Microtask Queue until empty **immediately following** the execution of **any** individual unit of work:

- *After* every Timer callback returns
- *After* every Ingress task executes
- *After* every I/O completion callback returns

### VI-3. Invariant

The Microtask Queue must be empty before the Loop enters the Polling/Idle state.

### VI-4. Re-entrancy

If a microtask schedules another microtask, it is added to the *current* drain cycle (unlike macro-tasks).

### VI-5. Budget Breach Protocol (D02 - Critical)

**MaxMicrotaskBudget Constraint:**

The loop must implement a hard `MaxMicrotaskBudget` (~1024) to prevent infinite loops (e.g., `Promise.resolve().then(...)` loops) from freezing the Loop Routine.

**Breach Handling:**

If `MaxMicrotaskBudget` is exceeded in the current tick:

- **Re-queue:** The Loop must **Re-queue** the remaining tasks to the next tick
- **Error Event:** Emit an error event to signal budget exhaustion
- **Non-Blocking Poll:** The Loop must force a **Non-Blocking Poll** (`epoll_wait` with `timeout = 0`) to process network I/O immediately and then return to the Microtask Queue
- **Continue Processing:** Stop processing microtasks for the current tick, allowing the Loop to continue with macro-tasks rather than freezing indefinitely

**Rationale:** A budget breach implies the queue is non-empty. Entering a sleep state would violate the invariant "Queue must be empty before Polling". Forcing a non-blocking poll ensures the Loop checks for I/O events while maintaining responsiveness and preventing indefinite blocking.

**Flag Check Order (Critical):** If a microtask budget breach forces a non-blocking poll, the flag must be checked **before** calling `poll()` and reset after consumption to avoid busy-waiting.

### VI-6. Batching Strategy

**Default:** Execute a batch of I/O callback tasks (e.g., up to 64) before triggering the **[Microtask Barrier]**. This preserves Cache Locality and instruction cache performance.

**Strict Mode:** If `StrictMicrotaskOrdering` is enabled, execute the **[Microtask Barrier]** after *every* individual callback.

### VI-B. Timer Execution

The `tick()` function MUST call `runTimers()` which:

1. Reads current tick time
2. Pops all expired timers from the timer heap
3. Executes callbacks with microtask barriers after each timer callback

**Failure Mode:** Without timer execution, all `setTimeout` equivalents silently fail; high-CPU spin once first timer expires.

---

## VII. Performance Requirements

### VII-1. Cached Time Resolution (D18 - Critical)

**Requirement:** The Loop must cache `time.Now()` result at the start of every Tick.

Internal logic (timer expiration, timeout checks) must use this **Cached Tick Time** to avoid the syscall overhead of `time.Now()` inside tight loops.

#### VII-1-A. Thread Safety (C3 Fix)

`time.Time` is a multi-word struct. Concurrent access without synchronization results in **torn reads** and garbage timestamps.

**Implementation:** Use `atomic.Int64` to store Unix nanoseconds; convert to `time.Time` only on read.

**Anchor+Offset Pattern:**

- Start: Capture `start = time.Now()` (contains monotonic reference)
- Tick: Calculate `delta = time.Since(start)` (uses internal monotonic clock), store `int64(delta)` atomically
- Read: `return start.Add(time.Duration(delta))`

**Benefits:**

- Zero allocations on both write and read paths
- Monotonic clock integrity (NTP jumps ignored)
- Time reconstruction is correct

**Reconstruction:** `CurrentTickTime() = tickAnchor.Add(time.Duration(tickOffset.Load()))`

### VII-2. Zero Allocation Hot Paths (D14 - Critical)

The loop must avoid allocations on hot paths by reusing persistent buffers for poll and ingress processing.

#### VII-2-A. drainWakeUpPipe()

**Requirement:** Must NOT allocate a buffer on every wake-up.

**Implementation:** Use a persistent `[8]byte` buffer stored in the `Loop` struct instead of `make([]byte, 8)` per call.

**Wake Drain Error Handling:** The `wakeUpSignalPending` flag MUST ONLY be cleared if the pipe is successfully and completely drained. Behavior by error:

- `EAGAIN`/`EWOULDBLOCK`: Successfully drained (empty pipe), clear flag
- `EINTR`: Retry the read
- `EBADF`: Shutdown in progress, do NOT clear flag
- Other errors: Log and do NOT clear flag

#### VII-2-B. Poll Event Buffers

**Requirement:** Reuse a persistent slice for `epoll_wait`/`kevent` results.

#### VII-2-C. Ingress Processing

**Requirement:** Avoid per-task allocations during queue traversal.

#### VII-2-D. Microtask Buffers

**Requirement:** Must zero out entries when removing tasks for GC safety and compact periodically when backing capacity becomes disproportionately large relative to length.

---

## VIII. I/O Registration

### VIII-1. Poller Requirements

#### VIII-1-A. Prohibitions

- `reflect.Select` is **Strictly Forbidden** due to scaling and allocation overhead
- Do **not** attempt to link into Go's internal `runtime.netpoll`

#### VIII-1-B. Scope

This Poller manages **Raw File Descriptors** only. Standard Go `net` blocking calls (like `http.Get`) are incompatible and must be handled via `Promisify` (Worker Pool).

#### VIII-1-C. Target Performance

Target wake-up latency should be < 10µs for optimal performance.

#### VIII-1-D. Mechanism

The Loop must park on a single synchronization primitive (e.g., `epoll_wait` or a Semaphore) that aggregates both I/O events and Ingress signals.

#### VIII-1-E. I/O Registration API

The Loop must expose:

- `RegisterFD(fd int, events EventMask, callback func(IOEvents)) error`
- `UnregisterFD(fd int) error`
- `ModifyFD(fd int, events EventMask) error`

#### VIII-1-F. Platform Constraint

This architecture targets **POSIX-compliant systems only** (Linux/BSD/macOS). Windows IOCP is out of scope.

---

### VIII-2. Lock Starvation Prevention (T10-C1 - Critical)

**Buffer Immutability Constraint:** The `ioPoller.eventBuf` slice MUST be allocated with a fixed capacity at initialization and NEVER reallocated or swapped during the poller's lifetime.

#### VIII-2-A. Problem

If `pollIO` holds a lock during blocking `EpollWait`/`Kevent`, `RegisterFD` blocks for the entire sleep duration.

#### VIII-2-B. Solution

Acquire `RLock` to capture FD and buffer header, release **BEFORE** blocking syscall, re-acquire to process results.

**Implementation (poll_linux.go & poll_darwin.go):**

```go
package example

func (p *ioPoller) pollIO(timeout int, maxEvents int) error {
	p.mu.RLock()
	// Capture needed data under lock
	epfd := p.epfd
	events := p.eventBuf // Capture slice header
	p.mu.RUnlock()

	// BLOCKING SYSCALL: Lock is released here
	n, err := unix.EpollWait(epfd, events, timeout)
	if err != nil {
		// Handle errors
	}

	p.mu.RLock()
	// Process events under lock
	p.mu.RUnlock()
}
```

**Benefits:**

- `RegisterFD`/`UnregisterFD` can proceed during blocking poll
- Prevents lock starvation

**Post-Syscall Liveness Check (Critical):** After re-acquiring the lock following the blocking syscall, `pollIO` MUST check if the poller has been closed. If closed, it must release the lock and return `ErrPollerClosed` immediately.

---

### VIII-3. Callback Deadlock Prevention (T10-C2 - Critical)

#### VIII-3-A. Problem

If `pollIO` holds `RLock` during callback execution and the user callback calls `UnregisterFD`, a deadlock occurs (RLock held → waiting for Lock).

#### VIII-3-B. Solution

Implement the "Collect-then-Execute" pattern: collect callbacks under lock, execute **outside** lock.

Callbacks must be invoked **after** releasing the `RLock`.

---

## IX. API Requirements

### IX-1. Promisify Loop.Promisify()

**Signature (D12 - Critical):**

```go
package example

func (l *Loop) Promisify(ctx context.Context, fn func(ctx context.Context) (T, error)) *Promise
```

**Behavior:**

- Takes a blocking Go function and a context, runs it in a *separate* worker goroutine
- Constructs a `Result` struct
- Enqueues a job to the Ingress Queue
- The Loop then dequeues this job and resolves the Promise on the Loop thread

#### IX-1-A. Context Propagation

The worker goroutine must monitor `ctx.Done()` and return `ctx.Err()` if triggered, making cancellation possible.

#### IX-1-B. Panic Isolation

Background workers spawned by `Promisify` **must** implement their own `defer/recover` blocks. Panics must be caught and converted into `Result{Err: PanicError}` sent to the Priority Ingress Lane.

**Failure Mode:** Failure to isolate panics causes "Zombie Promises" (hanging on await).

#### IX-1-C. Single-Owner Violation (D05 - Critical)

Worker goroutines spawned by `Promisify` must **not** resolve promises directly. They must submit resolution work to the internal priority lane (e.g., via `SubmitInternal`) so that promise resolution occurs on the loop thread.

---

### IX-2. Promise.ToChannel()

**Requirement:** Provide a bridge to standard Go channels.

**Implementation:**
`Promise.ToChannel()` returns a **buffered** `<-chan Result` with `capacity = 1` so a regular goroutine can block waiting for a loop-managed promise.

#### IX-2-A. Unique Ownership

`Promise.ToChannel()` must allocate and return a **NEW** `make(chan Result, 1)` on *every* invocation. Never reuse channels for multiple calls. Returning a shared channel allows multiple consumers to race or deadlock.

#### IX-2-B. Non-Blocking Loop Send

The Loop must use a non-blocking send mechanism to prevent Trivial Denial-of-Service:

```go
package example

func example() {
	select {
	case ch <- result:
		close(ch)
	default:
		// Critical Liveness Logic
		log.Printf("WARNING: eventloop: dropped promise result, channel full")
	}
}
```

**Failure Strategy (D19):** If the channel buffer is full (impossible on a fresh channel, but required for safety against user interference), the Loop must **DROP** the result and **Log a Warning**: "eventloop: dropped promise result, channel full"

**Prohibition:** The Loop must **NEVER** perform a blocking send. A blocking send on the Loop Thread creates a deadlock vulnerability if a user channel fills up or the consumer hangs.

---

## X. Concurrency & Thread Safety

### X-1. Single-Owner Thread Constraint

To avoid mutex contention and ensure sequential consistency (a requirement for deadlock-free Promise chains):

- The Event Loop must own exactly **one** goroutine (the `Loop Routine`)
- User code (callbacks) must execute inside this `Loop Routine`
- External goroutines must **never** execute logic directly; they must enqueue a closure/job into the loop's thread-safe ingress queue

---

### X-2. Panic Isolation (D06 - Critical)

**Requirement:** Every task (micro or macro) execution must be wrapped in a `defer/recover` block.

**Behavior:**

- A panic in a user callback must **not** crash the Event Loop
- It should be caught, logged/emitted as an `UncaughtException` event
- The loop must proceed to the next tick

**Unhandled Rejections:** If a Promise is rejected without an error handler (`catch` or `then` with rejection callback), the Loop must emit an `UnhandledRejection` event for proper error tracking.

---

### X-3. Re-entrancy Protection (D08 - Critical)

**Requirements:**

- The API must detect if `Loop.Run(ctx)` is called from within the Loop itself and return an error or no-op
- All Promise resolution must be asynchronous and queued to the microtask queue
- Synchronous state mutation bypassing the microtask queue is forbidden to maintain predictable execution order and prevent priority inversion

**Re-entrancy Check Implementation (D08):** `isLoopThread()` must reliably detect the loop thread identity so that `Run()` calls originating from inside the loop can be detected and prevented.

---

### X-4. Wake-Up Safety

**Requirement:** Implementation must prevent `SIGPIPE` crashes during shutdown.

* **Linux:** MUST use `eventfd` (returns `EBADF`/`EINVAL` on write-to-closed, not `SIGPIPE`).
* **Darwin/BSD:** Preferred use of `EVFILT_USER` (no FD write). If using Self-Pipe, `write()` calls must handle `EPIPE` gracefully.

---

## XI. Error Constants

The following sentinel errors must be defined:

```go
package example

var (
	ErrLoopAlreadyRunning = errors.New("eventloop: already running")
	ErrLoopTerminated     = errors.New("eventloop: terminated")
	ErrLoopOverloaded     = errors.New("eventloop: overloaded")
	ErrReentrantRun       = errors.New("eventloop: reentrant Run() call from loop thread")
)
```

---

## XII. Pending Tasks (Not Yet Implemented)

### XII-1. Hierarchical Timer Wheel (Optimization)

**Current:** Binary heap implementation
**Required when Timer count > 1000:** Implement hashed wheel or hierarchical wheel timer for O(1) insertion/cancellation.

**Impact:** High performance impact at scale. Standard binary heaps are acceptable for smaller counts, but hierarchical timer wheels provide O(1) insertion/cancellation for specific granularities, which is critical for thousands of `setTimeout` equivalents.

---

### XII-2. returnChunk Optimization (T11)

**Current:** `for i := 0; i < len(c.tasks); i++` (clears all 128 slots)
**Optimization:** `for i := 0; i < c.pos; i++` (clear only used slots)

**Impact:** CPU waste on every chunk return; low severity (performance optimization, not correctness).

---

### XII-3. TestRegisterFD_Basic Coverage (T10-M2)

**Current:** Uses `t.Skip` when callbacks don't fire within timeout

**Required:** Replace `t.Skip` with hard failure to validate I/O event delivery.

**Criticality:** HIGH - Without this, no verification that pollIO delivers events.

---

### XII-4. Struct Field Alignment (T10-M3)

**Current:** `ioPoller` before `promisifyWg` violates documented "8-byte aligned fields" requirement.

**Required:** Move atomic fields to struct top to guarantee 8-byte alignment on 32-bit architectures.

**Severity:** Medium - Safe on 64-bit architectures, crashes on 32-bit.

---

## XIII. Design Codes Reference

| Code | Description                                            |
|------|--------------------------------------------------------|
| D01  | poll() Zombie Prevention (CAS-based state transitions) |
| D02  | Budget Breach Non-Blocking Poll                        |
| D03  | Check-Then-Sleep Protocol / Lost Wake-up Fix           |
| D04  | Atomic State Machine (Run(ctx) CAS)                    |
| D05  | Promisify Single-Owner (SubmitInternal resolution)     |
| D06  | Panic Isolation (safeExecute wrapper)                  |
| D07  | Wake-Up Signal Safety (clear on failure)               |
| D08  | Re-entrancy Guard (goroutine ID tracking)              |
| D09  | Priority Lanes (SubmitInternal bypass)                 |
| D10  | O(1) Ingress Queue (readPos/writePos cursors)          |
| D11  | Registry Map Compaction (Bucket Ghost fix)             |
| D12  | Context Propagation in Promisify                       |
| D13  | Task Slot Zeroing (scavenge settled + nil)             |
| D14  | Zero Allocation Hot Paths (buffer reuse)               |
| D15  | Scavenge Rate (per-tick with batch)                    |
| D16  | Graceful Shutdown (Shutdown(ctx) + correct order)      |
| D17  | Ingress Budget / ErrLoopOverloaded                     |
| D18  | Time Freshness (start-of-tick update)                  |
| D19  | ToChannel Non-Blocking Send                            |
| D20  | Bucket Ghost Prevention (25% threshold)                |

---

## XIV. Summary Checklist

| Component                 | Critical Requirement                                                     | Failure Mode if Ignored                                                     |
|---------------------------|--------------------------------------------------------------------------|-----------------------------------------------------------------------------|
| **State Machine**         | Exact numeric values (0-4), CAS for temporary states                     | Contract violation / State corruption                                       |
| **Microtasks**            | Drain after **every** callback (not just phases).                        | Priority Inversion / Latency Spikes.                                        |
| **Ingress**               | Chunked MPSC + **Backpressure** / High-Water Mark.                       | OOM Crash or Deadlock.                                                      |
| **Ingress**               | **Check-Then-Sleep Protocol** (Double-Check Memory Barrier)              | **TOCTOU Race** (Lost wake-ups / Indefinite sleep).                         |
| **Ingress**               | Priority Lanes (Internal Bypass)                                         | **Deadlock** (Internal results dropped on load).                            |
| **GC**                    | Use **`weak.Pointer`** for Registry/Cycles.                              | Memory Leaks (Zombies).                                                     |
| **GC**                    | **Hybrid Ring-Renewal Strategy** (Deterministic Discovery + Map Renewal) | **Memory Leak** (Scavenger Lottery + Bucket Ghost).                         |
| **I/O Barrier**           | **Batching Strategy** (Default batch, Strict mode per-callback)          | **Cache Locality Loss** (Chatty Barrier) or **Starvation** (Over-batching). |
| **Budget Breach**         | **Non-Blocking Poll** (timeout=0) when exceeded                          | **Liveness Hazard** (Queue non-empty during sleep).                         |
| **Pooling**               | Pool **Internals Only** (Nodes/Tasks).                                   | Data Corruption (Race on Stale Ptrs).                                       |
| **Poller Lock**           | Release RLock before blocking syscall                                    | **Lock starvation** (RegisterFD blocks during poll).                        |
| **Poller Callback**       | Collect then Execute (re-entrancy safe)                                  | **Deadlock** (Callback calls UnregisterFD).                                 |
| **Hot Paths**             | **Zero Allocation** (persistent buffers for poll/ingress)                | **GC Pressure** (Allocation every tick).                                    |
| **Await**                 | **Unique Channel Ownership** (NEW channel per call)                      | **Race Conditions** (Multiple consumers deadlock).                          |
| **Await**                 | **Non-Blocking Send** (Drop on full + Warn)                              | **Deadlock** (Loop blocks on slow consumer).                                |
| **Promisify**             | **Context Injection** + **Weak Pointer Precision**                       | **Cancellation Impossible** / **Type Safety Violation**.                    |
| **Shutdown Order**        | **Ingress → Internal → Microtasks → RejectAll**                          | **Data Loss** (Microtasks from internal tasks lost).                        |
| **Graceful vs Immediate** | **Shutdown(ctx) for graceful, Close() for immediate**                    | **Resource Leak** (FDs not closed on timeout).                              |
| **Workers**               | **Panic Recovery** -> Result                                             | **Hanging Promise** (User await never returns).                             |
