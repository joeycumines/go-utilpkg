package alternatethree

import (
	"container/heap"
	"context"
	"errors"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrLoopAlreadyRunning is returned when Run() is called on a loop that is already running.
	ErrLoopAlreadyRunning = errors.New("eventloop: loop is already running")

	// ErrLoopTerminated is returned when operations are attempted on a terminated loop.
	ErrLoopTerminated = errors.New("eventloop: loop has been terminated")

	// ErrLoopNotRunning is returned when operations are attempted on a loop that hasn't been started.
	ErrLoopNotRunning = errors.New("eventloop: loop is not running")

	// ErrLoopOverloaded is returned when the external queue exceeds the tick budget.
	ErrLoopOverloaded = errors.New("eventloop: loop is overloaded")

	// ErrReentrantRun is returned when Run() is called from within the loop itself.
	ErrReentrantRun = errors.New("eventloop: cannot call Run() from within the loop")
)

// loopTestHooks provides injection points for deterministic race testing.
type loopTestHooks struct {
	PrePollSleep func() // Called before CAS to StateSleeping
	PrePollAwake func() // Called before CAS back to StateRunning
}

// Loop is the main event loop structure.
// It manages timers, I/O events, ingress queues, and microtasks in a single goroutine.
//
// # Thread Safety
//
// The Loop routine owns exclusive write access to most Loop fields.
// External callers must use the thread-safe Submit/Schedule APIs.
// The Loop state variable is accessed atomically by both producers and consumers.
type Loop struct { // betteralign:ignore

	// No copying allowed
	_ [0]func()

	// tickAnchor is the immutable start time of the loop.
	// It holds the monotonic clock reference. Read-only after Start(), so thread-safe.
	tickAnchor time.Time

	// Phase 2: Registry
	registry *registry

	// HOOKS: Test hooks for deterministic race testing
	testHooks *loopTestHooks

	// D17: Overload callback
	OnOverload func(error)

	// === Slice fields (24 bytes each: pointer + len + cap) ===

	ingress IngressQueue

	internalQueue    []Task
	internalQueueBuf []Task // H1: Double-buffer for zero-alloc swap

	// Phase 3: Timers
	timers timerHeap

	// Phase 6: Microtasks
	microtasks []Task

	// D14: Reusable buffer to avoid hot loop allocations
	ingressBuffer []Task

	// Phase 9: Poller - T10 FIX: Real I/O registration with epoll/kqueue
	ioPoller ioPoller

	// promisifyWg tracks in-flight Promisify goroutines.
	// Shutdown waits for this to reach zero before calling RejectAll.
	promisifyWg sync.WaitGroup

	// N.B. The atomic.Int64 and atomic.Uint64 types are alignment-safe, irrespective of field order.

	tickTimeOffset  atomic.Int64  // Nanoseconds since tickAnchor (C3: prevent torn reads)
	loopGoroutineID atomic.Uint64 // Goroutine ID for re-entrancy detection (D08)

	// tickCount tracks the number of loop iterations (Task 10.2)
	tickCount uint64

	// Platform specific FDs
	wakePipe      int
	wakePipeWrite int

	// stopOnce ensures Stop() is idempotent across multiple goroutines
	stopOnce sync.Once

	// loopDone signals loop termination to shutdown waiters.
	// Created in New(), closed when run() returns.
	loopDone chan struct{}

	// Mutex fields (start on 8-byte boundary)
	ingressMu  sync.Mutex
	internalMu sync.Mutex

	wakeUpSignalPending atomic.Uint32 // Wake-up deduplication flag (0 = pending, 1 = clear)
	state               atomic.Int32  // LoopState for atomic operations

	forceNonBlockingPoll bool

	// StrictMicrotaskOrdering controls the timing of the microtask barrier.
	//
	// Default (false): Batch Mode. The loop processes a batch of Ingress tasks (e.g., 1024),
	// and then runs the microtask barrier once. This improves throughput but means that
	// if Task A schedules a microtask, that microtask will not run until after Task B (and others)
	// have finished.
	//
	// Strict (true): Per-Task Mode. The loop runs the microtask barrier after EVERY single
	// Ingress task. This ensures that if Task A schedules a microtask, it runs immediately
	// before Task B starts. This guarantees deterministic ordering (Task A -> Microtask A -> Task B)
	// but significantly reduces throughput due to increased checking overhead.
	StrictMicrotaskOrdering bool
}

// timer represents a scheduled task
type timer struct {
	when time.Time
	task Task
}

// timerHeap is a min-heap of timers
type timerHeap []timer

// Implement heap.Interface for timerHeap
func (h timerHeap) Len() int           { return len(h) }
func (h timerHeap) Less(i, j int) bool { return h[i].when.Before(h[j].when) }
func (h timerHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *timerHeap) Push(x any) {
	*h = append(*h, x.(timer))
}

func (h *timerHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// New creates a new event loop.
// Initializes the wake-up mechanism (eventfd on Linux, self-pipe on Darwin).
func New() (*Loop, error) {
	// Create wake-up mechanism using platform-specific implementation
	// Linux: eventfd (same for R/W), Darwin: self-pipe (distinct R/W)
	wakeFd, wakeWriteFd, err := createWakeFd(0, EFD_CLOEXEC|EFD_NONBLOCK)
	if err != nil {
		return nil, err
	}

	loop := &Loop{
		wakePipe:      wakeFd,
		wakePipeWrite: wakeWriteFd,
		registry:      newRegistry(),
		// Initialize slices with modest capacity
		microtasks: make([]Task, 0, 64),
		timers:     make(timerHeap, 0),
		// Initialize loopDone here to avoid data race with shutdownImpl
		loopDone: make(chan struct{}),
	}
	loop.state.Store(int32(StateAwake))
	loop.wakeUpSignalPending.Store(0)

	// T10-FIX-3: Initialize the I/O poller and register wakePipe.
	// On Unix, this registers the wakePipe with epoll/kqueue.
	// On Windows, this initializes the IOCP handle.
	if err := loop.initWakeup(); err != nil {
		closeWakeFDs(wakeFd, wakeWriteFd)
		return nil, err
	}

	return loop, nil
}

// Run begins running the event loop and blocks until it fully stops.
// It returns an error if the loop is already running or if called recursively.
//
// # Thread Pinning
//
// The loop goroutine calls runtime.LockOSThread() to prevent scheduler migration
// and maximize cache locality.
//
// # Blocking Semantics
//
// Run() blocks until the loop fully terminates (via Shutdown() or Close()).
// To run the loop in a separate goroutine, wrap in a goroutine:
//
//	go loop.Run(ctx)
//	loop.Shutdown(ctx) // later
func (l *Loop) Run(ctx context.Context) error {
	// Re-entrancy check: ensure we're not calling Run() from within the loop
	if l.isLoopThread() {
		return ErrReentrantRun
	}

	// D04: Use CAS to transition from StateAwake to StateRunning
	// This prevents multiple goroutines from executing run() concurrently
	if !l.state.CompareAndSwap(int32(StateAwake), int32(StateRunning)) {
		// CAS failed - check current state to return appropriate error
		currentState := LoopState(l.state.Load())
		if currentState == StateTerminated {
			return ErrLoopTerminated
		}
		// Loop is already running, sleeping, or terminating
		return ErrLoopAlreadyRunning
	}

	// T5 FIX: Initialize the Monotonic Anchor before running the loop.
	// This captures the monotonic clock reference. Read-only after this point.
	l.tickAnchor = time.Now()

	// Close loopDone when run exits to signal completion to Shutdown waiters
	// The channel is created in New() to avoid a data race with shutdownImpl
	defer close(l.loopDone)

	// Run the loop directly (blocking) - no goroutine spawn
	l.run(ctx)

	return nil
}

// Shutdown gracefully shuts down the event loop.
//
// # Graceful Shutdown Protocol
//
// 1. Stop accepting new Ingress tasks (reject with ErrLoopTerminated)
// 2. Drain existing Microtasks queue
// 3. Drain existing Ingress queues
// 4. Reject all remaining pending promises with ErrLoopTerminated
// 5. Respect ctx.Done() timeout
// 6. Loop goroutine exits gracefully
//
// # Blocking Semantics
//
// Shutdown() blocks until the loop fully terminates. If the loop is running
// in a separate goroutine (via `go loop.Run(ctx)`), Shutdown() will wait for
// that goroutine to complete.
//
// # Idempotence
//
// It is safe to call Shutdown() from multiple goroutines concurrently.
// Only the first caller will perform the actual shutdown; subsequent
// callers will wait for the loop to terminate and return nil.
//
// If ctx cancels before termination completes, ctx.Err() is returned.
// The loop continues shutting down in the background.
func (l *Loop) Shutdown(ctx context.Context) error {
	var shutdownErr error
	l.stopOnce.Do(func() {
		shutdownErr = l.shutdownImpl(ctx)
	})

	if shutdownErr == nil && l.state.Load() != int32(StateTerminated) {
		// Already shutting down by another caller (stopOnce.Do ran but didn't complete)
		return ErrLoopTerminated
	}
	return shutdownErr
}

// shutdownImpl contains the actual Shutdown() implementation.
// Called via stopOnce.Do() to ensure idempotence.
func (l *Loop) shutdownImpl(ctx context.Context) error {
	// Attempt to transition to Terminating state
	for {
		currentState := LoopState(l.state.Load())
		if currentState == StateTerminated || currentState == StateTerminating {
			return ErrLoopTerminated
		}

		if l.state.CompareAndSwap(int32(currentState), int32(StateTerminating)) {
			// Special case: Unstarted loop (StateAwake)
			// If we CAS from Awake -> Terminating, Run() cannot succeed
			if currentState == StateAwake {
				l.state.Store(int32(StateTerminated))
				l.closeFDs()
				return nil
			}

			// If sleeping, wake up to process termination
			if currentState == StateSleeping {
				_ = l.submitWakeup()
			}
			break
		}
	}

	// Handle nil channel edge case: Run() was never called
	// This should already be handled above (StateAwake case), but defensive check
	if l.loopDone == nil {
		return nil
	}

	// Wait for termination via channel, NOT polling
	select {
	case <-l.loopDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// run is the main loop routine.
// Must be called with runtime.LockOSThread() invoked.
func (l *Loop) run(ctx context.Context) {
	// Pin to OS thread to prevent scheduler migration
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// D08: Store goroutine ID for re-entrancy detection
	l.loopGoroutineID.Store(getGoroutineID())
	defer l.loopGoroutineID.Store(0)

	for {
		// Check context for external cancellation
		select {
		case <-ctx.Done():
			// Context cancelled, initiate shutdown
			current := LoopState(l.state.Load())
			if current != StateTerminating && current != StateTerminated {
				l.state.Store(int32(StateTerminating))
			}
		default:
		}

		// Check Terminating state
		if LoopState(l.state.Load()) == StateTerminating {
			l.shutdown()
			return
		}

		l.tick(ctx)
	}
}

// shutdown executes the cleanup sequence.
// SHUTDOWN FIX: Correct ordering to ensure all in-flight work completes.
//
// Order rationale:
// 1. Drain ALL ingress tasks WHILE HOLDING LOCK continuously
// 2. State is already StateTerminating (set by run()), so Submit() is rejected
// 3. Ingress tasks can call SubmitInternal() successfully (only rejects on StateTerminated)
// 4. AFTER ingress is fully drained, set StateTerminated
// 5. Process ALL internal tasks spawned during shutdown
// 6. Drain ALL microtasks spawned during shutdown
// 7. Repeat steps 5-6 to catch chained work (Internal→Microtask)
// 8. Wait briefly for in-flight Promisify goroutines (C4 fix with timeout)
// 9. Final internal drain for any late-arriving resolutions
// 10. Final microtask drain
// 11. Reject all REMAINING pending promises (D16)
// 12. Close FDs and signal done
func (l *Loop) shutdown() {
	// SHUTDOWN DATA LOSS FIX: Single critical section - no unlock/relock cycles
	//
	// CRITICAL: Set StateTerminated BEFORE unlocking, atomically with the drain completion check.
	// Wait... but ingress tasks need to call SubmitInternal() which rejects on StateTerminated!
	//
	// The key insight: SubmitInternal() rejects on StateTerminated, but we're currently StateTerminating.
	// So tasks can SubmitInternal() during drain. After we set StateTerminated, no more tasks can submit.
	//
	// But we still need to hold the lock continuously to prevent tasks from sneaking in after we check
	// but before we drain.
	//
	// SOLUTION: Set StateTerminated WHILE HOLDING LOCK, but AFTER checking ingress is empty.
	// Any tasks that snuck in will have passed StateTerminating check (submitter side) but
	// will still be in the queue when we set StateTerminated. We drain them too.

	l.ingressMu.Lock()

	// SHUTDOWN DATA LOSS FIX: Execute all tasks WHILE HOLDING LOCK
	// This prevents TOCTOU race: Submit() acquires lock, checks state (Terminating), submits.
	// No tasks can sneak in after drain completes because lock is never released.
	//
	// KEY: Tasks call SubmitInternal() which works during StateTerminating (not StateTerminated).
	// So we stay in StateTerminating during drain, then transition to StateTerminated.

	// Phase 1: Drain all ingress tasks WITHOUT RELEASING LOCK
	for {
		t, ok := l.ingress.popLocked()
		if !ok {
			break
		}
		// CRITICAL: Execute WHILE HOLDING LOCK. safeExecute doesn't need ingressMu.
		// This prevents Submit() from sneaking in new tasks.
		l.safeExecute(t)
	}

	// Phase 2: Set StateTerminated - no more SubmitInternal() allowed
	l.state.Store(int32(StateTerminated))
	l.ingressMu.Unlock()

	// Process all work spawned during ingress drain
	// Loop until stable: processInternalQueue spawns microtasks, which can spawn more internal tasks
	for {
		hadInternal := l.processInternalQueue()

		// Drain microtasks (already called by processInternalQueue, but call again to catch chained work)
		l.drainMicrotasks()

		// Check if queue has more work (tasks that might have spawned during the above)
		if !hadInternal && len(l.microtasks) == 0 {
			// No internal or microtask work - stable
			break
		}
	}

	// 5. C4 FIX: Wait for ALL in-flight Promisify goroutines to complete
	// This ensures their SubmitInternal calls happen before we set StateTerminated.
	// Using sync.WaitGroup.Wait() ensures ALL goroutines complete - no timeout to prevent data corruption.
	// Only goroutines that already called promisifyWg.Add() will be waited for.
	// Any new Promisify operations will be rejected by StateTerminated check in Promisify().
	// CRITICAL: This MUST NOT use timeout - timeout causes data corruption.
	l.promisifyWg.Wait()

	// 6. Final drain - catch tasks that snuck in during TOCTOU window
	// Any tasks enqueued between step 3 and step 4 are caught here
	// Also catches any resolutions submitted before StateTerminated was set
	l.processInternalQueue()

	// 7. Final microtask drain
	l.drainMicrotasks()

	// 8. Reject all REMAINING pending promises (D16)
	// These are promises that were never resolved during shutdown
	l.registry.RejectAll(ErrLoopTerminated)

	// 9. Defect 3/7 Fix: Close wake FDs
	l.closeFDs()

	// 10. Loop goroutine will naturally return to Run() caller
}

// Close immediately terminates the event loop without waiting for graceful shutdown.
// This implements the io.Closer interface.
//
// # Immediate Termination
//
// Unlike Shutdown() which gracefully drains queues and respects timeouts,
// Close() transitions to Terminating immediately, closes file descriptors without
// waiting, and allows the loop to exit as soon as possible.
//
// # Use Case
//
// Use Close() in emergency situations where waiting for graceful shutdown
// (via Shutdown()) is not acceptable, such as:
// - Process termination signal handling
// - Fatal error conditions
// - Timeout scenarios
//
// # Error Handling
//
// If the loop is already terminated (StateTerminated), Close() returns ErrLoopTerminated.
// Otherwise, it initiates termination and returns nil.
func (l *Loop) Close() error {
	for {
		currentState := LoopState(l.state.Load())
		if currentState == StateTerminated {
			return ErrLoopTerminated
		}

		if l.state.CompareAndSwap(int32(currentState), int32(StateTerminating)) {
			// CRITICAL: Wake up the loop BEFORE closing FDs
			// On Linux with epoll, if we close the eventfd without waking up,
			// EpollWait will block forever with no way to recover.
			// submitWakeup is safe even if not sleeping - it just writes to the pipe
			_ = l.submitWakeup()

			// If sleeping, also wake up to process termination
			if currentState == StateSleeping {
				_ = l.submitWakeup()
			}
			break
		}
	}

	// FIX: Do NOT close FDs here. Let the loop's shutdown() function close them
	// after the loop has fully exited. This prevents a race where closeFDs() runs
	// while the loop is still trying to drain the wake pipe after epoll returns.
	// The loop's shutdown() → closeFDs() ensures the loop is not using any FDs.

	return nil
}

// tick is a single iteration of the event loop cycle.
func (l *Loop) tick(ctx context.Context) {
	l.tickCount++

	// H2: Update tickTime at START of tick for fresh timestamps in all phases
	// T5 FIX (REVISED): Store monotonic offset from anchor.
	// time.Since() uses the monotonic clock, ensuring this delta is immune to wall-clock jumps.
	// ZERO ALLOCATIONS - time.Since returns Duration (int64), atomic store is int64.
	l.tickTimeOffset.Store(int64(time.Since(l.tickAnchor)))

	// Defect 1 Fix: Execute expired timers
	l.runTimers()

	// D09: Process internal priority queue FIRST (no budget limit)
	l.processInternalQueue()

	// D02: Check forceNonBlockingPoll BEFORE calculating timeout
	// Process ingress first, then poll

	// Phase 2: Process Ingress (before poll, to drain existing work)
	l.processIngress(ctx)
	// After processing ingress, drain microtasks
	l.drainMicrotasks()

	// Phase 3: Poll (Blocking) - with Check-Then-Sleep protocol
	// T10-FIX-3: poll() now uses pollIO internally, which handles both:
	// - Blocking on registered I/O FDs (including wakePipe)
	// - Executing I/O callbacks when events occur
	l.poll(ctx, nil)

	// Drain microtasks after poll (I/O callbacks may have scheduled microtasks)
	l.drainMicrotasks()

	// D15: Scavenge every tick with small batch (not every 10000 ticks)
	l.registry.Scavenge(20)
}

// drainMicrotasks processes tasks from the internal microtask queue.
// It implements Task 6.1 (Yield/Pause) to prevent starvation.
func (l *Loop) drainMicrotasks() {
	// Task 9.5: Unbounded Bypass Warning
	if len(l.microtasks) > 10000 {
		log.Printf("WARNING: Internal Queue > 10k items (%d): potential infinite loop?", len(l.microtasks))
	}

	// Standard microtask budget (Task 6.1)
	const budget = 1024

	executed := 0
	for len(l.microtasks) > 0 {
		if executed >= budget {
			// Budget Breached: Yield
			// We stop processing, leaving remaining tasks in the queue (currentIndex preserved).
			// Task 6.2: Set forceNonBlockingPoll to ensure we come back immediately.
			l.forceNonBlockingPoll = true
			return
		}

		// Pop task
		t := l.microtasks[0]
		// D13: Zero out consumed slot to prevent memory leak
		l.microtasks[0] = Task{}
		l.microtasks = l.microtasks[1:]

		// D06: Use safeExecute for panic isolation
		l.safeExecute(t)
		executed++
	}

	// D13: Compact slice when cap > 1024 && len < cap/4
	if cap(l.microtasks) > 1024 && len(l.microtasks) < cap(l.microtasks)/4 {
		newSlice := make([]Task, len(l.microtasks), len(l.microtasks)*2+64)
		copy(newSlice, l.microtasks)
		l.microtasks = newSlice
	}
}

// processInternalQueue drains the internal priority queue (D09).
// Internal tasks bypass the tick budget and are processed before external tasks.
// H1: Uses double-buffering to avoid allocation thrashing.
// Returns true if any internal tasks were processed.
func (l *Loop) processInternalQueue() bool {
	l.internalMu.Lock()
	if len(l.internalQueue) == 0 {
		l.internalMu.Unlock()
		return false
	}
	// H1: Double-buffer swap - take current queue, swap in the buffer
	// This preserves capacity and avoids allocation on every tick
	tasks := l.internalQueue
	l.internalQueue = l.internalQueueBuf[:0] // Reuse buffer capacity
	l.internalQueueBuf = tasks[:0]           // Save current for next swap
	l.internalMu.Unlock()

	// Execute ALL internal tasks with NO budget limit
	for i, t := range tasks {
		l.safeExecute(t)
		tasks[i] = Task{} // Clear reference for GC
	}

	// Drain microtasks after internal queue
	l.drainMicrotasks()
	return true
}

// processIngress handles tasks from the ingress queue.
// It drains the queue in batches and executes tasks.
func (l *Loop) processIngress(ctx context.Context) {
	// Default budget: 1024 tasks per tick
	const budget = 1024

	// D14: Reuse buffer to avoid hot loop allocations
	if l.ingressBuffer == nil {
		l.ingressBuffer = make([]Task, 0, 128)
	}
	tasks := l.ingressBuffer[:0]

	// Pop a batch of tasks to minimize lock contention
	l.ingressMu.Lock()
	for i := 0; i < budget; i++ {
		t, ok := l.ingress.popLocked()
		if !ok {
			break
		}
		tasks = append(tasks, t)
	}

	// D17: Check if more tasks remain after budget exhausted
	remainingTasks := l.ingress.Length()
	l.ingressMu.Unlock()

	// Execute Tasks
	for i, t := range tasks {
		// D06: Use safeExecute for panic isolation
		l.safeExecute(t)
		// Clear reference for GC
		tasks[i] = Task{}

		// Phase 7.2: Strict Barrier
		// If strict ordering is required, we drain microtasks after EACH ingress task.
		// This guarantees that any microtasks spawned by this task run immediately,
		// before the next ingress task.
		if l.StrictMicrotaskOrdering {
			l.drainMicrotasks()
		}
	}

	// D17: Emit overload signal if more tasks remain after budget
	if remainingTasks > 0 && l.OnOverload != nil {
		l.OnOverload(ErrLoopOverloaded)
	}

	// Store buffer back for reuse
	l.ingressBuffer = tasks[:0]
}

// poll performs the blocking poll with the Check-Then-Sleep protocol.
//
// # Check-Then-Sleep Protocol (Loop Side)
//
// The Mutex-Barrier Pattern ensures we don't sleep when work is pending:
//
//  1. CAS(StateRunning, StateSleeping) - C1 fix: use CAS not Store
//  2. l.ingressMu.Lock() // Acts as StoreLoad barrier
//  3. len := l.ingressQueue.Length()
//  4. l.ingressMu.Unlock()
//  5. If len > 0: CAS(StateSleeping, StateRunning) and abort poll
//     If len == 0: proceed to epoll_wait (or poll with timeout)
//
// IMPORTANT: This is NOT an optimistic check that can be hoisted!
// The length check MUST occur while the mutex is held to prevent TOCTOU races.
func (l *Loop) poll(ctx context.Context, tickTime interface{}) {
	// Check-Then-Sleep Protocol: Commit to sleeping, then verify no work pending

	// Phase 6.2: Read and reset forceNonBlockingPoll at START of poll
	// This ensures the flag is always consumed, even on early-exit paths.
	// If we're not going to block (due to CAS failure or early exit),
	// the flag doesn't matter anyway.
	forced := l.forceNonBlockingPoll
	l.forceNonBlockingPoll = false

	// HOOKS: Call test hook before state transition if configured
	if l.testHooks != nil && l.testHooks.PrePollSleep != nil {
		l.testHooks.PrePollSleep()
	}

	// C1 FIX: Use CAS instead of Store to avoid overwriting StateTerminating
	// If state is not StateRunning (e.g., it's StateTerminating), abort poll immediately
	if !l.state.CompareAndSwap(int32(StateRunning), int32(StateSleeping)) {
		// State changed (likely to Terminating) - abort poll, don't sleep
		return
	}

	// Step 2: Check BOTH ingress and internal queues while holding locks
	// This prevents TOCTOU race where SubmitInternal enqueues after CAS
	// but before we check the queue length.

	// Check ingress queue
	l.ingressMu.Lock()
	ingressLen := l.ingress.Length()
	l.ingressMu.Unlock()

	// Check internal queue - fixes TOCTOU with SubmitInternal
	l.internalMu.Lock()
	internalLen := len(l.internalQueue)
	l.internalMu.Unlock()

	// Step 3: Branch based on queue state
	if ingressLen > 0 || internalLen > 0 {
		// Work arrived! Abort sleep and process immediately
		// C2 FIX: Use CAS(StateSleeping, StateRunning), not Store(StateAwake)
		// Using StateAwake would allow concurrent Start() to succeed
		l.state.CompareAndSwap(int32(StateSleeping), int32(StateRunning))

		// Skip poll and process the queue immediately
		return
	}

	// Both queues are empty - proceed to blocking poll

	// Step 5: Refresh CurrentTime before wait (from requirements)
	// Phase ordering: Refresh Time -> Adaptive Check -> Wait

	// Use calculateTimeout to determine sleep duration (Phase 3)
	timeout := l.calculateTimeout()

	// Apply override from forceNonBlockingPoll (read at start of poll)
	if forced {
		timeout = 0
	}

	// T10-FIX-3: Use pollIO as the unified blocking mechanism.
	// The wakePipe is registered with the ioPoller, so we'll wake on:
	// - Wake-up signals (wakePipe becomes readable)
	// - Any registered I/O FD events
	// - Timeout expiry
	// H-CRITICAL-1 FIX: Handle pollIO error to prevent CPU death spiral
	_, err := l.pollIO(timeout, 128)
	if err != nil {
		// CRITICAL: If pollIO fails, we cannot continue the loop.
		// Common failures: EBADF (FD closed), ENOMEM (out of memory), EINVAL (invalid args)
		// Previously, ignoring this error caused a 100% CPU spin loop.
		// Now we initiate graceful shutdown immediately.
		log.Printf("CRITICAL: pollIO failed: %v - terminating loop", err)
		l.state.Store(int32(StateTerminating))
		_ = l.submitWakeup() // Force wake to process shutdown
		return
	}

	// Reset the wake-up pending flag after poll returns.
	// On Unix, the poller callback already drains the pipe; this is a harmless re-Store(0).
	// On Windows (IOCP), this is the only place the flag is reset, enabling future wakeups.
	l.drainWakeUpPipe()

	// Upon wake-up, buffer events (don't execute callbacks yet)
	// Then update CurrentTime
	// Then iterate and execute callbacks

	// HOOKS: Call test hook before state transition if configured
	if l.testHooks != nil && l.testHooks.PrePollAwake != nil {
		l.testHooks.PrePollAwake()
	}

	// D01/C1: Use CAS to restore active state - prevents zombie loop
	// If state is Terminating, don't overwrite it - let run() handle shutdown
	if !l.state.CompareAndSwap(int32(StateSleeping), int32(StateRunning)) {
		// CAS failed - state was changed (likely to Terminating)
		// Don't overwrite - run() will check and handle appropriately
		return
	}
}

func (l *Loop) calculateTimeout() int {
	maxDelay := 10 * time.Second // Default/Max block time

	// Phase 3.2: Cap by NextTimer
	if len(l.timers) > 0 {
		now := time.Now()
		nextFire := l.timers[0].when
		delay := nextFire.Sub(now)
		if delay < 0 {
			delay = 0
		}
		if delay < maxDelay {
			maxDelay = delay
		}
	}

	// Phase 3.1: Ceiling Rounding
	// If 0 < delta < 1ms, round up to 1ms to prevent truncation to zero (busy spin)
	if maxDelay > 0 && maxDelay < time.Millisecond {
		return 1
	}

	return int(maxDelay.Milliseconds())
}

// Submit enqueues a task to the external ingress queue for execution on the loop.
//
// This method is thread-safe and can be called from any goroutine.
// The task will execute in order relative to other submitted tasks.
//
// **CRITICAL: Shutdown Rejection Policy**
// During shutdown (StateTerminating), Submit() rejects new tasks with ErrLoopTerminated.
// This prevents "task creep" where new work keeps arriving during shutdown.
// Internal tasks (via SubmitInternal) continue to be accepted until StateTerminated.
//
// If the loop is already terminated (StateTerminated), returns ErrLoopTerminated.
func (l *Loop) Submit(fn func()) error {
	task := Task{Runnable: fn}

	// Enqueue first (Write) - Protected by mutex
	// The mutex ensures memory visibility of the task push
	l.ingressMu.Lock()

	// SHUTDOWN DATA LOSS FIX: Check state while holding lock
	// This is atomic with shutdown's state+drain operation
	currentState := LoopState(l.state.Load())
	if currentState == StateTerminating || currentState == StateTerminated {
		l.ingressMu.Unlock()
		return ErrLoopTerminated
	}

	l.ingress.Push(task)
	l.ingressMu.Unlock()

	// D03: After Unlock, unconditionally attempt wake-up if sleeping
	// This fixes the TOCTOU race causing lost wake-ups
	if l.state.Load() == int32(StateSleeping) {
		// Use CompareAndSwap for deduplication to prevent multiple producers
		// from causing multiple wake-up syscalls
		if l.wakeUpSignalPending.CompareAndSwap(0, 1) {
			// We successfully claimed the wake-up responsibility
			// D07: Clear flag on failure to prevent signal loss
			if err := l.submitWakeup(); err != nil {
				l.wakeUpSignalPending.Store(0)
			}
		}
	}

	return nil
}

// Wake attempts to wake up the loop from a suspended state.
// This is the core of the wake-up deduplication system.
//
// Returns nil if wake-up was successful or if wake-up was already pending.
// Returns an error if the loop is not running.
func (l *Loop) Wake() error {
	state := LoopState(l.state.Load())
	if state != StateSleeping {
		// Loop is not sleeping, no wake-up needed
		return nil
	}

	// Use CompareAndSwap for deduplication
	if l.wakeUpSignalPending.CompareAndSwap(0, 1) {
		// D07: Clear flag on failure to prevent signal loss
		if err := l.submitWakeup(); err != nil {
			l.wakeUpSignalPending.Store(0)
			return err
		}
		return nil
	}

	// CAS failed - wake-up signal is already pending
	// We're done - no need to do anything more
	return nil
}

// isLoopThread checks if we're currently executing in the loop routine.
// This is used for re-entrancy protection (D08).
func (l *Loop) isLoopThread() bool {
	loopID := l.loopGoroutineID.Load()
	if loopID == 0 {
		return false // Loop not running
	}
	return getGoroutineID() == loopID
}

// getGoroutineID returns the current goroutine's ID.
// This uses runtime internals and is only for debugging/re-entrancy detection.
func getGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// Stack trace starts with "goroutine NNN ["
	var id uint64
	for i := len("goroutine "); i < n; i++ {
		if buf[i] >= '0' && buf[i] <= '9' {
			id = id*10 + uint64(buf[i]-'0')
		} else {
			break
		}
	}
	return id
}

// SubmitInternal submits a task to the internal priority lane.
// Internal tasks bypass the tick budget and are processed before external tasks.
//
// This is used for Promisify resolution and other system-internal tasks that
// must not be delayed by external queue congestion (D09).
//
// C4: Accepts tasks during StateTerminating to allow in-flight Promisify
// resolutions to complete cleanly.
// M1: Checks state while holding lock to prevent TOCTOU race with shutdown.
func (l *Loop) SubmitInternal(task Task) error {
	// M1 FIX: Acquire lock FIRST, then check state atomically with enqueue
	// This prevents TOCTOU race where shutdown drains between check and enqueue
	l.internalMu.Lock()

	// C4 FIX: Only reject on StateTerminated, NOT StateTerminating
	// In-flight Promisify goroutines need to resolve during shutdown
	currentState := LoopState(l.state.Load())
	if currentState == StateTerminated {
		l.internalMu.Unlock()
		return ErrLoopTerminated
	}

	// Enqueue to internal priority queue (atomic with state check)
	l.internalQueue = append(l.internalQueue, task)
	l.internalMu.Unlock()

	// Wake up if sleeping (same as Submit)
	if l.state.Load() == int32(StateSleeping) {
		if l.wakeUpSignalPending.CompareAndSwap(0, 1) {
			if err := l.submitWakeup(); err != nil {
				l.wakeUpSignalPending.Store(0)
			}
		}
	}

	return nil
}

// CurrentTickTime returns the cached time for the current tick.
// This is refreshed at the start of each tick (H2), ensuring callbacks have fresh time (D18).
// T5 FIX (REVISED): Reconstructs time from Anchor + Offset - ZERO ALLOCATIONS.
// tickAnchor.Add() advances the Monotonic clock inside the struct correctly.
// The result is perfectly valid for Sub/Until comparisons against other monotonic times.
// C3: Atomic access prevents torn reads on 64-bit platforms.
func (l *Loop) CurrentTickTime() time.Time {
	offset := l.tickTimeOffset.Load()
	if offset == 0 && l.tickAnchor.IsZero() {
		// Loop not started yet, return current time as fallback
		return time.Now()
	}
	return l.tickAnchor.Add(time.Duration(offset))
}

// safeExecute wraps task execution with panic recovery (D06).
// This ensures a single panicking task doesn't crash the entire loop.
func (l *Loop) safeExecute(t Task) {
	if t.Runnable == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("ERROR: eventloop: task panicked: %v", r)
			// Loop survives - continue processing
		}
	}()

	t.Runnable()
}

// runTimers executes all expired timers.
// Defect 1 Fix: This was completely missing - timers were calculated but never executed.
func (l *Loop) runTimers() {
	now := l.CurrentTickTime()
	for len(l.timers) > 0 {
		if l.timers[0].when.After(now) {
			break
		}
		// Pop and execute
		t := heap.Pop(&l.timers).(timer)
		l.safeExecute(t.task)

		if l.StrictMicrotaskOrdering {
			l.drainMicrotasks()
		}
	}
}

// ScheduleTimer schedules a task to be executed after the specified delay.
// The task will be executed on the loop thread after at least `delay` has elapsed.
// Thread-safe: can be called from any goroutine.
func (l *Loop) ScheduleTimer(delay time.Duration, fn func()) error {
	when := time.Now().Add(delay)
	t := timer{
		when: when,
		task: Task{Runnable: fn},
	}

	// Route through SubmitInternal to ensure heap access is on loop thread
	// This avoids data race on l.timers
	return l.SubmitInternal(Task{Runnable: func() {
		heap.Push(&l.timers, t)
	}})
}

// scheduleMicrotask adds a task to the microtask queue.
// Microtasks are processed after each phase, before the next ingress batch.
func (l *Loop) scheduleMicrotask(task Task) {
	l.microtasks = append(l.microtasks, task)
}

// Ensure scheduleMicrotask is not removed by staticcheck
var _ = (*Loop)(nil).scheduleMicrotask
