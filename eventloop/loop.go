package eventloop

import (
	"container/heap"
	"context"
	"errors"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Standard errors.
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

// Loop is the "Maximum Performance" event loop implementation.
//
// PERFORMANCE: Prioritizes throughput and low latency:
//   - Lock-free ingress queue (LockFreeIngress)
//   - Direct FD indexing (no map lookups)
//   - Inline callback execution
//   - Cache-line padding for hot fields
//
// This implementation includes all features from the original Main:
//   - Timer heap (ScheduleTimer)
//   - Promise registry (Promisify support)
//   - StrictMicrotaskOrdering option
//   - OnOverload callback
//   - testHooks for deterministic testing
type Loop struct { // betteralign:ignore
	// Prevent copying
	_ [0]func()

	// Phase 2: Registry
	registry *registry

	// HOOKS: Test hooks for deterministic race testing
	testHooks *loopTestHooks

	// D17: Overload callback
	OnOverload func(error)

	// State machine (cache-line padded internally)
	state *FastState

	// Ingress queues (lock-free)
	external   *LockFreeIngress // External tasks
	internal   *LockFreeIngress // Internal priority tasks
	microtasks *MicrotaskRing   // Microtask ring buffer

	// Legacy compatibility shim for tests accessing l.ingress
	ingress IngressQueue

	// Phase 3: Timers
	timers timerHeap

	// I/O poller (zero-lock FastPoller)
	poller FastPoller

	// Legacy ioPoller shim for test compatibility
	ioPoller ioPoller

	// Synchronization
	stopOnce sync.Once

	// promisifyWg tracks in-flight Promisify goroutines.
	promisifyWg sync.WaitGroup

	// Wake-up mechanism
	wakePipe      int
	wakePipeWrite int
	wakeBuf       [8]byte
	wakePending   atomic.Uint32

	// Timing
	tickAnchor      time.Time    // Reference time for monotonicity (initialized once, never changes)
	tickElapsedTime atomic.Int64 // Nanoseconds offset from anchor (monotonic, atomic for thread safety)

	// Goroutine tracking
	loopGoroutineID atomic.Uint64
	tickCount       uint64

	// Loop ID
	id uint64

	// Loop termination signaling
	loopDone chan struct{}

	// In-flight submit counter for shutdown synchronization
	inflight atomic.Int64

	// Task batch buffer (avoid allocation)
	batchBuf [256]Task

	// Internal queue mutex (for backward compat with SubmitInternal)
	internalMu    sync.Mutex
	internalQueue []Task
	internalBuf   []Task
	ingressMu     sync.Mutex // Legacy - for test compat
	_             []Task     // Legacy ingressBuffer - reserved for processIngress compat

	wakeUpSignalPending atomic.Uint32 // Wake-up deduplication

	forceNonBlockingPoll bool

	// StrictMicrotaskOrdering controls the timing of the microtask barrier.
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

var loopIDCounter atomic.Uint64

// New creates a new performance-first event loop with full feature set.
func New() (*Loop, error) {
	wakeFd, wakeWriteFd, err := createWakeFd(0, EFD_CLOEXEC|EFD_NONBLOCK)
	if err != nil {
		return nil, err
	}

	loop := &Loop{
		id:         loopIDCounter.Add(1),
		state:      NewFastState(),
		external:   NewLockFreeIngress(),
		internal:   NewLockFreeIngress(),
		microtasks: NewMicrotaskRing(),
		registry:   newRegistry(),
		timers:     make(timerHeap, 0),

		wakePipe:      wakeFd,
		wakePipeWrite: wakeWriteFd,

		// Initialize loopDone here to avoid data race with shutdownImpl
		loopDone: make(chan struct{}),
	}

	// Initialize poller
	if err := loop.poller.Init(); err != nil {
		_ = unix.Close(wakeFd)
		if wakeWriteFd != wakeFd {
			_ = unix.Close(wakeWriteFd)
		}
		return nil, err
	}

	// Register wake pipe
	if err := loop.poller.RegisterFD(wakeFd, EventRead, func(IOEvents) {
		loop.drainWakeUpPipe()
	}); err != nil {
		_ = loop.poller.Close()
		_ = unix.Close(wakeFd)
		if wakeWriteFd != wakeFd {
			_ = unix.Close(wakeWriteFd)
		}
		return nil, err
	}

	return loop, nil
}

// Run runs the event loop and blocks until fully stopped.
//
// Run blocks until the loop terminates (via Shutdown(), Close(), or ctx cancellation).
// To run in a separate goroutine, use: `go loop.Run(ctx)`.
func (l *Loop) Run(ctx context.Context) error {
	if l.isLoopThread() {
		return ErrReentrantRun
	}

	if !l.state.TryTransition(StateAwake, StateRunning) {
		currentState := l.state.Load()
		if currentState == StateTerminated {
			return ErrLoopTerminated
		}
		return ErrLoopAlreadyRunning
	}

	// Close loopDone when run exits to signal completion to Shutdown waiters
	defer close(l.loopDone)

	// Initialize timing anchor - this is our reference point for monotonic time
	l.tickAnchor = time.Now()
	l.tickElapsedTime.Store(0)

	// Run the loop directly (blocking)
	return l.run(ctx)
}

// Shutdown gracefully shuts down the event loop.
//
// Shutdown initiates graceful shutdown that waits for all queued tasks to complete.
// It blocks until termination completes or ctx expires.
func (l *Loop) Shutdown(ctx context.Context) error {
	var result error
	l.stopOnce.Do(func() {
		result = l.shutdownImpl(ctx)
	})
	if result == nil && l.state.Load() != StateTerminated {
		return ErrLoopTerminated
	}
	return result
}

// shutdownImpl contains the actual Shutdown implementation.
func (l *Loop) shutdownImpl(ctx context.Context) error {
	for {
		currentState := l.state.Load()
		if currentState == StateTerminated || currentState == StateTerminating {
			return ErrLoopTerminated
		}

		if l.state.TryTransition(currentState, StateTerminating) {
			if currentState == StateAwake {
				l.state.Store(StateTerminated)
				l.closeFDs()
				return nil
			}

			if currentState == StateSleeping {
				_ = l.submitWakeup()
			}
			break
		}
	}

	// Wait for termination via channel, NOT polling
	select {
	case <-l.loopDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// run is the main loop goroutine.
func (l *Loop) run(ctx context.Context) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	l.loopGoroutineID.Store(getGoroutineID())
	defer l.loopGoroutineID.Store(0)

	// Start context watcher goroutine to wake loop on cancellation
	ctxDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = l.submitWakeup()
		case <-ctxDone:
		}
	}()
	defer close(ctxDone)

	for {
		// Check context for external cancellation
		select {
		case <-ctx.Done():
			// Context cancelled, initiate shutdown via CAS
			for {
				current := l.state.Load()
				if current == StateTerminating || current == StateTerminated {
					break
				}
				if l.state.TryTransition(current, StateTerminating) {
					if current == StateSleeping {
						_ = l.submitWakeup()
					}
					break
				}
			}
			l.shutdown()
			return ctx.Err()
		default:
		}

		// Check termination
		if l.state.Load() == StateTerminating || l.state.Load() == StateTerminated {
			l.shutdown()
			return nil
		}

		l.tick()
	}
}

// shutdown performs the shutdown sequence.
func (l *Loop) shutdown() {
	// C4 FIX: Wait briefly for in-flight Promisify goroutines FIRST
	// This ensures their SubmitInternal calls complete before we drain queues
	promisifyDone := make(chan struct{})
	go func() {
		l.promisifyWg.Wait()
		close(promisifyDone)
	}()
	select {
	case <-promisifyDone:
	case <-time.After(100 * time.Millisecond):
	}

	// CRITICAL: Set state to Terminated FIRST to prevent new tasks from being accepted.
	// Any Submit that checked state before this will push a task, and we'll catch it
	// in the drain below. Any Submit that checks state after will be rejected.
	l.state.Store(StateTerminated)

	// Drain loop: continue until BOTH conditions are met:
	// 1. inflight == 0 (no Submit operations in progress)
	// 2. All queues are empty
	// We need to check BOTH because:
	// - A Submit could be between Push and defer (inflight > 0 but queue has task)
	// - A Submit could have just completed defer (inflight == 0 and queue has task)
	emptyChecks := 0
	const requiredEmptyChecks = 3 // Need multiple consecutive empty checks
	for emptyChecks < requiredEmptyChecks {
		// Wait for any in-flight submits to complete
		spinCount := 0
		for l.inflight.Load() > 0 {
			spinCount++
			if spinCount > 1000 {
				time.Sleep(100 * time.Microsecond)
			} else {
				runtime.Gosched()
			}
		}

		drained := false

		// Drain internal queue
		for {
			task, ok := l.internal.Pop()
			if !ok {
				break
			}
			l.safeExecute(task)
			drained = true
		}

		// Drain external queue
		for {
			task, ok := l.external.Pop()
			if !ok {
				break
			}
			l.safeExecute(task)
			drained = true
		}

		// Drain microtasks
		for {
			fn := l.microtasks.Pop()
			if fn == nil {
				break
			}
			l.safeExecuteFn(fn)
			drained = true
		}

		// Also drain legacy internal queue
		l.internalMu.Lock()
		if len(l.internalQueue) > 0 {
			tasks := l.internalQueue
			l.internalQueue = nil
			l.internalMu.Unlock()
			for _, t := range tasks {
				l.safeExecute(t)
			}
			drained = true
		} else {
			l.internalMu.Unlock()
		}

		if drained || l.inflight.Load() > 0 {
			emptyChecks = 0 // Reset if we drained or there are in-flight submits
		} else {
			emptyChecks++
			runtime.Gosched() // Yield to let any racing submits complete
		}
	}

	// Reject all remaining pending promises
	l.registry.RejectAll(ErrLoopTerminated)

	l.closeFDs()
}

// tick is a single iteration of the event loop.

// tick is a single iteration of the event loop.
func (l *Loop) tick() {
	l.tickCount++

	// D6 FIX: Update elapsed monotonic time offset from anchor
	// time.Since(tickAnchor) uses the monotonic clock when available,
	// ensuring accuracy even if wall-clock is adjusted (e.g., NTP)
	elapsed := time.Since(l.tickAnchor)
	l.tickElapsedTime.Store(int64(elapsed))

	// Execute expired timers
	l.runTimers()

	// Process internal tasks (priority)
	l.processInternalQueue()

	// Process external tasks with budget
	l.processExternal()

	// Process microtasks
	l.drainMicrotasks()

	// Poll for I/O
	l.poll()

	// Final microtask pass
	l.drainMicrotasks()

	// Scavenge registry
	l.registry.Scavenge(20)
}

// processInternalQueue drains the internal priority queue.
func (l *Loop) processInternalQueue() bool {
	// First, drain the new lock-free internal queue
	for {
		task, ok := l.internal.Pop()
		if !ok {
			break
		}
		l.safeExecute(task)
	}

	// Then, drain the legacy mutex-protected internal queue
	l.internalMu.Lock()
	if len(l.internalQueue) == 0 {
		l.internalMu.Unlock()
		return false
	}
	tasks := l.internalQueue
	l.internalQueue = l.internalBuf[:0]
	l.internalBuf = tasks[:0]
	l.internalMu.Unlock()

	for i, t := range tasks {
		l.safeExecute(t)
		tasks[i] = Task{}
	}

	l.drainMicrotasks()
	return true
}

// processExternal processes external tasks with budget.
func (l *Loop) processExternal() {
	const budget = 1024

	// PERFORMANCE: Batch pop for better cache behavior
	n := l.external.PopBatch(l.batchBuf[:], budget)

	// D17: Check if more tasks remain after budget exhausted
	remainingTasks := l.external.Length()

	for i := 0; i < n; i++ {
		l.safeExecute(l.batchBuf[i])
		l.batchBuf[i] = Task{} // Clear for GC

		// Strict microtask ordering
		if l.StrictMicrotaskOrdering {
			l.drainMicrotasks()
		}
	}

	// D17: Emit overload signal if more tasks remain after budget
	if remainingTasks > 0 && l.OnOverload != nil {
		l.OnOverload(ErrLoopOverloaded)
	}
}

// drainMicrotasks drains the microtask queue.
func (l *Loop) drainMicrotasks() {
	const budget = 1024

	for i := 0; i < budget; i++ {
		fn := l.microtasks.Pop()
		if fn == nil {
			break
		}
		l.safeExecuteFn(fn)
	}
}

// poll performs the blocking poll.
func (l *Loop) poll() {
	currentState := l.state.Load()
	if currentState != StateRunning {
		return
	}

	// Read and reset forceNonBlockingPoll
	forced := l.forceNonBlockingPoll
	l.forceNonBlockingPoll = false

	// HOOKS: Call test hook before state transition
	if l.testHooks != nil && l.testHooks.PrePollSleep != nil {
		l.testHooks.PrePollSleep()
	}

	// PERFORMANCE: Optimistic state transition
	if !l.state.TryTransition(StateRunning, StateSleeping) {
		return
	}

	// Quick length check (may have false negatives)
	if l.external.Length() > 0 || l.internal.Length() > 0 || !l.microtasks.IsEmpty() {
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// Check for termination before blocking poll
	if l.state.Load() == StateTerminating {
		return
	}

	// Calculate timeout
	timeout := l.calculateTimeout()
	if forced {
		timeout = 0
	}

	_, err := l.poller.PollIO(timeout)
	if err != nil {
		log.Printf("CRITICAL: pollIO failed: %v - terminating loop", err)
		l.state.TryTransition(StateSleeping, StateTerminating)
		return
	}

	// HOOKS: Call test hook after poll
	if l.testHooks != nil && l.testHooks.PrePollAwake != nil {
		l.testHooks.PrePollAwake()
	}

	l.state.TryTransition(StateSleeping, StateRunning)
}

// drainWakeUpPipe drains the wake-up pipe.
func (l *Loop) drainWakeUpPipe() {
	for {
		_, err := unix.Read(l.wakePipe, l.wakeBuf[:])
		if err != nil {
			break
		}
	}
	l.wakePending.Store(0)
}

// submitWakeup writes to the wake-up pipe.
//
// Wake-up Policy:
//   - REJECTS: StateTerminated (fully stopped, no tasks to process)
//   - ALLOWS: StateTerminating (loop needs to wake and drain remaining tasks)
//   - ALLOWS: StateSleeping, StateRunning, StateAwake
//
// Safe to call concurrently during shutdown - pipe write errors during shutdown are
// gracefully handled by callers.
func (l *Loop) submitWakeup() error {
	// D10 FIX: Check state and reject ONLY if fully terminated
	// We MUST allow wake-up during StateTerminating so the loop can
	// drain queued tasks and complete shutdown
	state := l.state.Load()
	if state == StateTerminated {
		// Loop is already fully terminated - no need to wake up
		return ErrLoopTerminated
	}

	// PERFORMANCE: Native endianness, no binary.LittleEndian overhead
	var one uint64 = 1
	buf := (*[8]byte)(unsafe.Pointer(&one))[:]

	_, err := unix.Write(l.wakePipeWrite, buf)
	// Note: Pipe write errors (e.g., "broken pipe") are expected during shutdown
	// when the pipe is being closed. Callers must handle these gracefully.
	return err
}

// Submit submits a task to the external queue.
//
// State Policy during shutdown:
//   - StateTerminated: returns ErrLoopTerminated
//   - StateTerminating: ALLOWS submission (loop needs to drain in-flight work)
//   - StateSleeping/StateRunning: normal operation
func (l *Loop) Submit(task Task) error {
	// Increment inflight counter FIRST, before checking state
	l.inflight.Add(1)
	defer l.inflight.Add(-1)

	// D10 FIX: Only reject if fully terminated, NOT during StateTerminating
	// We must allow submissions during StateTerminating to ensure:
	// 1. In-flight operations (e.g., Promisify) can complete
	// 2. The loop can drain all queued work before shutting down
	state := l.state.Load()
	if state == StateTerminated {
		return ErrLoopTerminated
	}

	// Push the task (this is the linearization point)
	// The push is complete (visible to Pop) after PushTask returns.
	l.external.PushTask(task)

	// Wake if sleeping
	state = l.state.Load()
	if state == StateSleeping {
		if l.wakePending.CompareAndSwap(0, 1) {
			if err := l.submitWakeup(); err != nil {
				// D10 FIX: Gracefully handle wake-up failures during shutdown
				// Expected errors during shutdown:
				//   - EBADF (bad file descriptor): pipe closed
				//   - EPIPE (broken pipe): write end closed
				// These are not fatal - the task is already queued and will be
				// processed if the loop wakes via other means
				l.wakePending.Store(0)
				// Don't return error from Submit - task is already queued
			}
		}
	}

	return nil
}

// SubmitInternal submits a task to the internal priority queue.
//
// State Policy during shutdown:
//   - StateTerminated: returns ErrLoopTerminated
//   - StateTerminating: ALLOWS submission (loop needs to drain in-flight work)
//   - StateSleeping/StateRunning: normal operation
func (l *Loop) SubmitInternal(task Task) error {
	// Increment inflight counter FIRST
	l.inflight.Add(1)
	defer l.inflight.Add(-1)

	// D10 FIX: Only reject if fully terminated, NOT during StateTerminating
	// Consistent with Submit() - allows loop to drain in-flight work
	state := l.state.Load()
	if state == StateTerminated {
		return ErrLoopTerminated
	}

	l.internal.PushTask(task)

	if l.state.Load() == StateSleeping {
		if l.wakePending.CompareAndSwap(0, 1) {
			// D10 FIX: Gracefully handle wake-up failures during shutdown
			if err := l.submitWakeup(); err != nil {
				// Expected errors during shutdown (EBADF, EPIPE)
				// Task is already queued, will be processed if loop wakes
				l.wakePending.Store(0)
				// Don't return error - task is already queued
			}
		}
	}

	return nil
}

// Wake attempts to wake up the loop from a suspended state.
//
// State Policy:
//   - StateSleeping: performs wake-up (if not already pending)
//   - StateTerminated: returns nil (no-op on terminated loop)
//   - StateTerminating/StateRunning/StateAwake: returns nil (loop already active)
//
// Wake-up failures during shutdown are silently ignored (expected when pipe closes).
func (l *Loop) Wake() error {
	state := l.state.Load()

	// Early returns for non-sleeping states
	if state != StateSleeping {
		// If already terminated, sleeping, or transitioning, no wake-up needed
		return nil
	}

	if l.wakeUpSignalPending.CompareAndSwap(0, 1) {
		// D10 FIX: Gracefully handle wake-up failures
		// submitWakeup will reject if StateTerminated, so we don't need
		// to check state again here
		err := l.submitWakeup()
		if err != nil {
			// Reset pending flag on failure so future Wake() can retry
			l.wakeUpSignalPending.Store(0)
			// Don't propagate error - during shutdown, pipe write errors
			// (EBADF, EPIPE) are expected and not fatal
			return nil
		}
	}

	return nil
}

// ScheduleMicrotask schedules a microtask.
//
// D2 FIX: MicrotaskRing now implements dynamic growth, so Push never fails.
// Removed error check for full buffer.
func (l *Loop) ScheduleMicrotask(fn func()) error {
	state := l.state.Load()
	if state == StateTerminated {
		return ErrLoopTerminated
	}

	l.microtasks.Push(fn)
	return nil
}

// scheduleMicrotask adds a task to the microtask queue (internal use).
//
// D2 FIX: MicrotaskRing now implements dynamic growth, so Push never fails.
// Removed fallback to SubmitInternal - always push to microtask ring.
func (l *Loop) scheduleMicrotask(task Task) {
	if task.Runnable != nil {
		l.microtasks.Push(task.Runnable)
	}
}

// RegisterFD registers a file descriptor for I/O monitoring.
func (l *Loop) RegisterFD(fd int, events IOEvents, callback func(events IOEvents)) error {
	return l.poller.RegisterFD(fd, events, callback)
}

// UnregisterFD removes a file descriptor from monitoring.
func (l *Loop) UnregisterFD(fd int) error {
	return l.poller.UnregisterFD(fd)
}

// ModifyFD updates the events being monitored for a file descriptor.
func (l *Loop) ModifyFD(fd int, events IOEvents) error {
	return l.poller.ModifyFD(fd, events)
}

// CurrentTickTime returns the cached time for the current tick.
// The returned value uses the monotonic clock and is safe to use for timer calculations.
func (l *Loop) CurrentTickTime() time.Time {
	// If anchor not initialized (shouldn't happen after Run), return current wall-clock time
	if l.tickAnchor.IsZero() {
		return time.Now()
	}
	// Add elapsed monotonic offset to anchor to get current monotonic time
	// This ensures timer accuracy even if wall-clock is adjusted
	elapsed := time.Duration(l.tickElapsedTime.Load())
	return l.tickAnchor.Add(elapsed)
}

// SetTickAnchor sets the tick anchor time (for testing only).
func (l *Loop) SetTickAnchor(t time.Time) {
	l.tickAnchor = t
	l.tickElapsedTime.Store(0)
}

// TickAnchor returns the tick anchor time (for testing only).
func (l *Loop) TickAnchor() time.Time {
	return l.tickAnchor
}

// State returns the current loop state.
func (l *Loop) State() LoopState {
	return l.state.Load()
}

// calculateTimeout determines how long to block in poll.
func (l *Loop) calculateTimeout() int {
	maxDelay := 10 * time.Second

	// Cap by next timer
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

	// Ceiling rounding: if 0 < delta < 1ms, round up to 1ms
	if maxDelay > 0 && maxDelay < time.Millisecond {
		return 1
	}

	return int(maxDelay.Milliseconds())
}

// runTimers executes all expired timers.
func (l *Loop) runTimers() {
	now := l.CurrentTickTime()
	for len(l.timers) > 0 {
		if l.timers[0].when.After(now) {
			break
		}
		t := heap.Pop(&l.timers).(timer)
		l.safeExecute(t.task)

		if l.StrictMicrotaskOrdering {
			l.drainMicrotasks()
		}
	}
}

// ScheduleTimer schedules a task to be executed after the specified delay.
func (l *Loop) ScheduleTimer(delay time.Duration, fn func()) error {
	// D6 FIX: Use monotonic time by computing when relative to current tick time
	now := l.CurrentTickTime()
	when := now.Add(delay)
	t := timer{
		when: when,
		task: Task{Runnable: fn},
	}

	return l.SubmitInternal(Task{Runnable: func() {
		heap.Push(&l.timers, t)
	}})
}

// safeExecute executes a task with panic recovery.
func (l *Loop) safeExecute(t Task) {
	if t.Runnable == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("ERROR: eventloop: task panicked: %v", r)
		}
	}()

	t.Runnable()
}

// safeExecuteFn executes a function with panic recovery.
func (l *Loop) safeExecuteFn(fn func()) {
	if fn == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("ERROR: eventloop: task panicked: %v", r)
		}
	}()

	fn()
}

// closeFDs closes file descriptors.
func (l *Loop) closeFDs() {
	_ = l.poller.Close()
	_ = unix.Close(l.wakePipe)
	if l.wakePipeWrite != l.wakePipe {
		_ = unix.Close(l.wakePipeWrite)
	}
}

// isLoopThread checks if we're on the loop goroutine.
func (l *Loop) isLoopThread() bool {
	loopID := l.loopGoroutineID.Load()
	if loopID == 0 {
		return false
	}
	return getGoroutineID() == loopID
}

// getGoroutineID returns the current goroutine's ID.
func getGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
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

// Close immediately terminates the event loop without waiting for graceful shutdown.
func (l *Loop) Close() error {
	for {
		currentState := l.state.Load()
		if currentState == StateTerminated {
			return ErrLoopTerminated
		}

		if l.state.TryTransition(currentState, StateTerminating) {
			if currentState == StateAwake {
				l.state.Store(StateTerminated)
				l.closeFDs()
				return nil
			}
			if currentState == StateSleeping {
				_ = l.submitWakeup()
			}
			return nil
		}
	}
}

// ============================================================================
// LEGACY COMPATIBILITY - for existing test access to internal structures
// ============================================================================
// Legacy API compatibility
// ============================================================================

// processIngress handles tasks from the ingress queue (legacy compatibility).
// This method exists for tests that call it directly.
func (l *Loop) processIngress(ctx context.Context) {
	_ = ctx // unused but kept for API compat
	l.processExternal()
}

// Ensure processIngress is not removed by staticcheck
var _ = (*Loop)(nil).processIngress
