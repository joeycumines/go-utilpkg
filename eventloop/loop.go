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

// Sync pools for zero-allocation hot paths
var (
	taskPool  = sync.Pool{New: func() any { return new(func()) }}
	timerPool = sync.Pool{New: func() any { return new(timer) }}
)

// Standard errors.
var (
	// ErrLoopAlreadyRunning is returned when Run() is called on a loop that is already running.
	ErrLoopAlreadyRunning = errors.New("eventloop: loop is already running")

	// ErrLoopTerminated is returned when operations are attempted on a terminated loop.
	ErrLoopTerminated = errors.New("eventloop: loop has been terminated")

	// ErrLoopOverloaded is returned when the external queue exceeds the tick budget.
	ErrLoopOverloaded = errors.New("eventloop: loop is overloaded")

	// ErrReentrantRun is returned when Run() is called from within the loop itself.
	ErrReentrantRun = errors.New("eventloop: cannot call Run() from within the loop")

	// ErrFastPathIncompatible is returned when fast path mode is forced but I/O FDs are registered.
	ErrFastPathIncompatible = errors.New("eventloop: fast path incompatible with registered I/O FDs")

	// ErrTimerNotFound is returned when attempting to cancel a timer that does not exist.
	ErrTimerNotFound = errors.New("eventloop: timer not found")
)

// loopTestHooks provides injection points for deterministic race testing.
type loopTestHooks struct {
	PrePollSleep           func() // Called before CAS to StateSleeping
	PrePollAwake           func() // Called before CAS back to StateRunning
	OnFastPathEntry        func() // Called when entering fast path (runFastPath or direct exec)
	AfterOptimisticCheck   func() // Called after optimistic check, before Swap
	BeforeFastPathRollback func() // Called before attempting to rollback fast path mode
}

// FastPathMode controls how fast path mode selection works.
type FastPathMode int32

const (
	// FastPathAuto automatically selects mode based on conditions.
	// Default (zero value): uses fast path when userIOFDCount == 0.
	FastPathAuto FastPathMode = iota

	// FastPathForced always uses fast path.
	// Returns ErrFastPathIncompatible if I/O FDs are registered.
	FastPathForced

	// FastPathDisabled always uses poll path (useful for debugging/testing).
	FastPathDisabled
)

// Loop is a high-performance event loop implementation.
//
// It prioritizes throughput and low latency using:
//   - Mutex+chunked ingress queue (ChunkedIngress) which outperforms lock-free under contention
//   - Direct FD indexing
//   - Inline callback execution
//
// Design note: Mutex+chunking is used for ingress because benchmarks showed it outperforms
// lock-free CAS under high contention due to O(N) retry storms.
type Loop struct {
	_ [0]func() // Prevent copying

	// Large pointer-heavy types (all require 8-byte alignment)
	batchBuf     [256]func()
	tickAnchor   time.Time
	registry     *registry
	state        *FastState
	testHooks    *loopTestHooks
	external     *ChunkedIngress
	internal     *ChunkedIngress
	microtasks   *MicrotaskRing
	OnOverload   func(error)
	fastWakeupCh chan struct{}
	loopDone     chan struct{}
	timerMap     map[TimerID]*timer
	timers       timerHeap
	auxJobs      []func()
	auxJobsSpare []func()
	poller       FastPoller
	promisifyWg  sync.WaitGroup

	// Simple primitive types BEFORE anything that requires pointer alignment
	tickCount     uint64
	id            uint64
	wakePipe      int
	wakePipeWrite int

	// Atomic fields (all require 8-byte alignment)
	nextTimerID         atomic.Uint64
	tickElapsedTime     atomic.Int64
	loopGoroutineID     atomic.Uint64
	fastPathEntries     atomic.Int64
	fastPathSubmits     atomic.Int64
	tickAnchorMu        sync.RWMutex
	stopOnce            sync.Once
	closeOnce           sync.Once
	externalMu          sync.Mutex
	internalQueueMu     sync.Mutex
	timerNestingDepth   atomic.Int32 // HTML5 spec: nesting depth for timeout clamping
	userIOFDCount       atomic.Int32
	wakeUpSignalPending atomic.Uint32
	fastPathMode        atomic.Int32

	wakeBuf                 [8]byte
	_                       [2]byte // Align to 8-byte
	_                       [2]byte // Align to 8-byte
	forceNonBlockingPoll    bool
	StrictMicrotaskOrdering bool
}

// TimerID uniquely identifies a scheduled timer and can be used to cancel it.
type TimerID uint64

// timer represents a scheduled task
type timer struct {
	when         time.Time
	task         func()
	id           TimerID
	heapIndex    int
	canceled     atomic.Bool
	nestingLevel int32 // Nesting level at scheduling time for HTML5 clamping
}

// timerHeap is a min-heap of timers
type timerHeap []*timer

// Implement heap.Interface for timerHeap
func (h timerHeap) Len() int           { return len(h) }
func (h timerHeap) Less(i, j int) bool { return h[i].when.Before(h[j].when) }
func (h timerHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIndex = i
	h[j].heapIndex = j
}

func (h *timerHeap) Push(x any) {
	n := len(*h)
	t := x.(*timer)
	t.heapIndex = n
	*h = append(*h, t)
}

func (h *timerHeap) Pop() any {
	old := *h
	n := len(old)
	t := old[n-1]
	old[n-1] = nil // Avoid memory leak
	*h = old[:n-1]
	return t
}

var loopIDCounter atomic.Uint64

// New creates a new event loop with optional configuration.
func New(opts ...LoopOption) (*Loop, error) {
	// Apply options
	options, err := resolveLoopOptions(opts)
	if err != nil {
		return nil, err
	}

	wakeFd, wakeWriteFd, err := createWakeFd(0, EFD_CLOEXEC|EFD_NONBLOCK)
	if err != nil {
		return nil, err
	}

	loop := &Loop{
		id:            loopIDCounter.Add(1),
		state:         NewFastState(),
		external:      NewChunkedIngress(),
		internal:      NewChunkedIngress(),
		microtasks:    NewMicrotaskRing(),
		registry:      newRegistry(),
		timers:        make(timerHeap, 0),
		timerMap:      make(map[TimerID]*timer),
		wakePipe:      wakeFd,
		wakePipeWrite: wakeWriteFd,
		// Buffer size 1 prevents blocking on send when channel is full
		fastWakeupCh: make(chan struct{}, 1),
		loopDone:     make(chan struct{}),
	}

	// Apply options to Loop struct
	loop.StrictMicrotaskOrdering = options.strictMicrotaskOrdering
	loop.fastPathMode.Store(int32(options.fastPathMode))

	if err := loop.poller.Init(); err != nil {
		_ = unix.Close(wakeFd)
		if wakeWriteFd != wakeFd {
			_ = unix.Close(wakeWriteFd)
		}
		return nil, err
	}

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

// SetFastPathMode sets the fast path mode for this loop.
//
// Modes:
//   - FastPathAuto (default): Automatically uses fast path when no I/O FDs registered.
//   - FastPathForced: Always uses fast path (returns error if I/O FDs present).
//   - FastPathDisabled: Always uses poll path (for debugging/testing).
//
// Invariant: When mode is FastPathForced, userIOFDCount must be 0.
//
// Thread Safety: Safe to call concurrently with RegisterFD.
// Uses optimistic stores with CAS-based rollback on conflict.
//
// ABA Race Mitigation:
// When SetFastPathMode is called concurrently with RegisterFD, the CAS-based rollback
// ensures a safe final state, though it may result in a transient state where
// one operation returns an error but the final state is safe (not Force + FDs).
func (l *Loop) SetFastPathMode(mode FastPathMode) error {
	// Optimistic Check
	if mode == FastPathForced && l.userIOFDCount.Load() > 0 {
		return ErrFastPathIncompatible
	}

	// Hook to simulate race with RegisterFD
	if l.testHooks != nil && l.testHooks.AfterOptimisticCheck != nil {
		l.testHooks.AfterOptimisticCheck()
	}

	// Store Mode FIRST (Establishes Store-Load barrier)
	prev := FastPathMode(l.fastPathMode.Swap(int32(mode)))

	// Verification/Validation (After Store)
	countAfterSwap := l.userIOFDCount.Load()
	if mode == FastPathForced && countAfterSwap > 0 {
		if l.testHooks != nil && l.testHooks.BeforeFastPathRollback != nil {
			l.testHooks.BeforeFastPathRollback()
		}

		// Invariant violated: We stored Forced but count > 0.
		// Use CompareAndSwap to restore previous mode if it hasn't changed since.
		if !l.fastPathMode.CompareAndSwap(int32(mode), int32(prev)) {
			// CAS failed: another goroutine changed mode after us.
			// Their write wins, so we just return error without rollback.
		}

		return ErrFastPathIncompatible
	}

	// Wake the loop so it immediately re-evaluates the mode.
	l.doWakeup()

	return nil
}

// canUseFastPath returns true if fast path can be used right now.
// This consolidates all conditions into a single check.
func (l *Loop) canUseFastPath() bool {
	mode := FastPathMode(l.fastPathMode.Load())
	switch mode {
	case FastPathForced:
		return true
	case FastPathDisabled:
		return false
	default: // FastPathAuto
		return l.userIOFDCount.Load() == 0
	}
}

// Run runs the event loop and blocks until fully stopped.
//
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

	l.tickAnchorMu.Lock()
	l.tickAnchor = time.Now()
	l.tickAnchorMu.Unlock()
	l.tickElapsedTime.Store(0)

	return l.run(ctx) // blocking
}

// Shutdown gracefully shuts down the event loop.
//
// It waits for all queued tasks to complete.
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

			// Wake up the loop - in fast path mode, the loop may be
			// blocking on fastWakeupCh without transitioning to StateSleeping
			l.doWakeup()
			break
		}
	}

	// Wait for termination
	select {
	case <-l.loopDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// run is the main loop goroutine.
func (l *Loop) run(ctx context.Context) error {
	// Thread locking is only needed when using the I/O poller (kqueue/epoll).
	// In fast path mode (no I/O FDs), we use pure Go channels which don't
	// require thread affinity.
	//
	// Thread locking is deferred to tick() when it actually polls for I/O.
	var osThreadLocked bool

	l.loopGoroutineID.Store(getGoroutineID())
	defer l.loopGoroutineID.Store(0)

	// Start context watcher goroutine to wake loop on cancellation
	ctxDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			l.doWakeup()
		case <-ctxDone:
		}
	}()
	defer close(ctxDone)

	defer func() {
		if osThreadLocked {
			runtime.UnlockOSThread()
		}
	}()

	for {
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
						l.doWakeup()
					}
					break
				}
			}
			l.shutdown()
			return ctx.Err()
		default:
		}

		if l.state.Load() == StateTerminating || l.state.Load() == StateTerminated {
			l.shutdown()
			return nil
		}

		// Use fast-path loop for task-only workloads (~500ns execution vs ~10µs with full tick).
		if l.canUseFastPath() && !l.hasTimersPending() && !l.hasInternalTasks() && !l.hasExternalTasks() {
			if l.runFastPath(ctx) {
				// Fast path completed or needs mode switch - continue to check termination
				continue
			}
			// Fall through to regular tick for feature transition
		}

		// Lock to OS thread before using I/O poller if needed.
		if !osThreadLocked {
			runtime.LockOSThread()
			osThreadLocked = true
		}

		l.tick()
	}
}

// runFastPath is a tight loop for task-only workloads associated with "fast path" mode.
// Returns true if the loop should continue (check termination), false if should use tick.
//
// It uses a simple blocking select (no CAS transitions in hot path) and atomic batch swap,
// achieving very low latency for pure task workloads.
func (l *Loop) runFastPath(ctx context.Context) bool {
	l.fastPathEntries.Add(1)
	if l.testHooks != nil && l.testHooks.OnFastPathEntry != nil {
		l.testHooks.OnFastPathEntry()
	}

	// Initial drain before entering the main select loop
	l.runAux()

	// Check termination after initial drain
	if l.state.Load() >= StateTerminating {
		return true
	}

	for {
		select {
		case <-ctx.Done():
			return true

		case <-l.fastWakeupCh:
			l.runAux()

			// Check for shutdown
			if l.state.Load() >= StateTerminating {
				return true
			}

			// Check if we need to switch to poll path (e.g., I/O FDs registered)
			if !l.canUseFastPath() {
				return false // exit to main loop to switch to poll path
			}

			// Exit fast path if timers or internal tasks need processing.
			// These require tick() which handles runTimers() and processInternalQueue().
			if l.hasTimersPending() || l.hasInternalTasks() {
				return false
			}

			// Exit if external queue has tasks (edge case: Submit() decided on
			// l.external before mode changed back to fast-path-compatible).
			if l.hasExternalTasks() {
				return false
			}
		}
	}
}

// runAux drains the auxJobs queue (fast path submits), internal queue, and microtasks.
//
// It achieves low latency by:
//   - Single lock per batch (not per task)
//   - Simple slice swap
//   - Execution without holding lock
//   - Buffer reuse
func (l *Loop) runAux() {
	// Drain auxJobs (external Submit in fast path mode)
	l.externalMu.Lock()
	jobs := l.auxJobs
	l.auxJobs = l.auxJobsSpare
	l.externalMu.Unlock()

	for i, job := range jobs {
		l.safeExecute(job)
		jobs[i] = nil // Clear for GC

		if l.StrictMicrotaskOrdering {
			l.drainMicrotasks()
		}
	}
	l.auxJobsSpare = jobs[:0]

	// Drain internal queue (SubmitInternal tasks)
	for {
		l.internalQueueMu.Lock()
		task, ok := l.internal.Pop()
		l.internalQueueMu.Unlock()
		if !ok {
			break
		}
		l.safeExecute(task)
	}

	// Drain microtasks (standard pass)
	l.drainMicrotasks()

	// If microtasks remain (budget exceeded), signal the loop to run again immediately.
	// This prevents blocking in the runFastPath select statement.
	if !l.microtasks.IsEmpty() {
		select {
		case l.fastWakeupCh <- struct{}{}:
		default:
			// Channel full means wake-up is already pending
		}
	}
}

// hasTimersPending returns true if there are pending timers.
// NOTE: This is only called from the loop goroutine, so no mutex needed.
func (l *Loop) hasTimersPending() bool {
	return len(l.timers) > 0
}

// hasInternalTasks returns true if there are internal tasks pending.
func (l *Loop) hasInternalTasks() bool {
	l.internalQueueMu.Lock()
	hasInternal := l.internal.Length() > 0
	l.internalQueueMu.Unlock()
	return hasInternal
}

// hasExternalTasks returns true if there are external tasks pending.
// This is checked before entering fast path to prevent starvation of tasks
// that were queued in l.external while the loop was in poll mode.
func (l *Loop) hasExternalTasks() bool {
	l.externalMu.Lock()
	hasExt := l.external.Length() > 0
	l.externalMu.Unlock()
	return hasExt
}

// shutdown performs the shutdown sequence.
func (l *Loop) shutdown() {
	// Wait briefly for in-flight Promisify goroutines FIRST
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

	// Set state to Terminated FIRST to prevent new tasks from being accepted.
	// Any Submit that checked state before this will push a task, and we'll catch it
	// in the drain below. Any Submit that checks state after will be rejected.
	l.state.Store(StateTerminated)

	// Drain loop: continue until all queues are empty for multiple consecutive checks.
	emptyChecks := 0
	const requiredEmptyChecks = 3 // Need multiple consecutive empty checks
	for emptyChecks < requiredEmptyChecks {
		drained := false

		// Drain internal queue
		for {
			l.internalQueueMu.Lock()
			task, ok := l.internal.Pop()
			l.internalQueueMu.Unlock()
			if !ok {
				break
			}
			l.safeExecute(task)
			drained = true
		}

		// Drain external queue
		for {
			l.externalMu.Lock()
			task, ok := l.external.Pop()
			l.externalMu.Unlock()
			if !ok {
				break
			}
			l.safeExecute(task)
			drained = true
		}

		// Drain fast path queue (auxJobs)
		l.externalMu.Lock()
		jobs := l.auxJobs
		l.auxJobs = l.auxJobsSpare
		l.externalMu.Unlock()
		for i, job := range jobs {
			l.safeExecute(job)
			jobs[i] = nil
			drained = true
		}
		l.auxJobsSpare = jobs[:0]

		// Drain microtasks
		for {
			fn := l.microtasks.Pop()
			if fn == nil {
				break
			}
			l.safeExecuteFn(fn)
			drained = true
		}

		if drained {
			emptyChecks = 0
		} else {
			emptyChecks++
			runtime.Gosched()
		}
	}

	// Reject all remaining pending promises
	l.registry.RejectAll(ErrLoopTerminated)

	l.closeFDs()
}

// tick is a single iteration of the event loop.
func (l *Loop) tick() {
	l.tickCount++

	// Update elapsed monotonic time offset from anchor
	l.tickAnchorMu.RLock()
	anchor := l.tickAnchor
	l.tickAnchorMu.RUnlock()
	elapsed := time.Since(anchor)
	l.tickElapsedTime.Store(int64(elapsed))

	l.runTimers()

	l.processInternalQueue()

	l.processExternal()

	// Drain auxJobs (leftover from fast path mode transitions).
	// This handles the race where Submit() checks canUseFastPath() before lock,
	// mode changes, and task ends up in auxJobs while loop is in poll path.
	l.drainAuxJobs()

	l.drainMicrotasks()

	l.poll()

	l.drainMicrotasks()

	// Scavenge registry - limit per tick to avoid stalling
	const registryScavengeLimit = 20
	l.registry.Scavenge(registryScavengeLimit)
}

// processInternalQueue drains the internal priority queue.
func (l *Loop) processInternalQueue() bool {
	// Drain the chunked internal queue
	processed := false
	for {
		l.internalQueueMu.Lock()
		task, ok := l.internal.Pop()
		l.internalQueueMu.Unlock()
		if !ok {
			break
		}
		l.safeExecute(task)
		processed = true
	}

	if processed {
		l.drainMicrotasks()
	}
	return processed
}

// processExternal processes external tasks with budget.
func (l *Loop) processExternal() {
	const budget = 1024

	// Pop tasks in batch while holding the external mutex
	l.externalMu.Lock()
	n := 0
	for n < budget && n < len(l.batchBuf) {
		task, ok := l.external.Pop()
		if !ok {
			break
		}
		l.batchBuf[n] = task
		n++
	}
	// Check remaining tasks while still holding lock
	remainingTasks := l.external.Length()
	l.externalMu.Unlock()

	// Execute tasks (without holding mutex)
	for i := 0; i < n; i++ {
		l.safeExecute(l.batchBuf[i])
		l.batchBuf[i] = nil // Clear for GC

		if l.StrictMicrotaskOrdering {
			l.drainMicrotasks()
		}
	}

	// Emit overload signal if more tasks remain after budget
	if remainingTasks > 0 && l.OnOverload != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("ERROR: eventloop: OnOverload callback panicked: %v", r)
				}
			}()
			l.OnOverload(ErrLoopOverloaded)
		}()
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

// drainAuxJobs drains leftover tasks from the fast path auxJobs queue.
// This handles the race condition where Submit() checks canUseFastPath() before
// acquiring the lock, mode changes (e.g., FD registered), and the task ends up
// in auxJobs while the loop is now in poll path. Without this, such tasks would
// be starved until shutdown or mode reversion.
func (l *Loop) drainAuxJobs() {
	l.externalMu.Lock()
	jobs := l.auxJobs
	l.auxJobs = l.auxJobsSpare
	l.externalMu.Unlock()

	for i, job := range jobs {
		l.safeExecute(job)
		jobs[i] = nil // Clear for GC

		if l.StrictMicrotaskOrdering {
			l.drainMicrotasks()
		}
	}
	l.auxJobsSpare = jobs[:0]
}

// poll performs blocking I/O poll with fast task wakeup optimization.
//
// It uses two wakeup strategies:
// 1. FAST MODE (no user I/O FDs): Blocks on fastWakeupCh channel (~50ns latency)
// 2. I/O MODE (user I/O FDs registered): Blocks on kqueue/epoll (~10µs latency)
func (l *Loop) poll() {
	currentState := l.state.Load()
	if currentState != StateRunning {
		return
	}

	// Read and reset forceNonBlockingPoll
	forced := l.forceNonBlockingPoll
	l.forceNonBlockingPoll = false

	if l.testHooks != nil && l.testHooks.PrePollSleep != nil {
		l.testHooks.PrePollSleep()
	}

	// Optimistic state transition
	if !l.state.TryTransition(StateRunning, StateSleeping) {
		return
	}

	// Quick length check (need to hold mutexes for accurate count)
	l.externalMu.Lock()
	extLen := l.external.Length()
	l.externalMu.Unlock()

	l.internalQueueMu.Lock()
	intLen := l.internal.Length()
	l.internalQueueMu.Unlock()

	if extLen > 0 || intLen > 0 || !l.microtasks.IsEmpty() {
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	if l.state.Load() == StateTerminating {
		return
	}

	// Calculate timeout
	timeout := l.calculateTimeout()
	if forced {
		timeout = 0
	}

	// Check for termination AGAIN after calculating timeout
	// but BEFORE blocking in poll. This prevents racing with Shutdown.
	if l.state.Load() == StateTerminating {
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// FAST MODE: No user I/O FDs registered - use channel-based wakeup
	if l.userIOFDCount.Load() == 0 {
		l.pollFastMode(timeout)
		return
	}

	// I/O MODE: User FDs registered - must use kqueue/epoll
	_, err := l.poller.PollIO(timeout)
	if err != nil {
		l.handlePollError(err)
		return
	}

	if l.testHooks != nil && l.testHooks.PrePollAwake != nil {
		l.testHooks.PrePollAwake()
	}

	l.state.TryTransition(StateSleeping, StateRunning)
}

// pollFastMode is the channel-based fast path for task-only workloads.
// It blocks on fastWakeupCh instead of kqueue, achieving lower latency.
func (l *Loop) pollFastMode(timeoutMs int) {
	// Drain any pending channel signal first (non-blocking)
	select {
	case <-l.fastWakeupCh:
		// Already have a pending wakeup - reset pending flag
		l.wakeUpSignalPending.Store(0)
		if l.testHooks != nil && l.testHooks.PrePollAwake != nil {
			l.testHooks.PrePollAwake()
		}
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	default:
	}

	// Check for termination BEFORE blocking in pollFastMode
	// This prevents a race where shutdown happens after the channel drain
	// but before we block, causing us to sleep indefinitely.
	if l.state.Load() == StateTerminating {
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// Non-blocking case
	if timeoutMs == 0 {
		if l.testHooks != nil && l.testHooks.PrePollAwake != nil {
			l.testHooks.PrePollAwake()
		}
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// For long timeouts (>=1 second), just block indefinitely.
	// This avoids timer allocation overhead.
	if timeoutMs >= 1000 {
		// Check termination before indefinite block
		if l.state.Load() == StateTerminating {
			l.state.TryTransition(StateSleeping, StateRunning)
			return
		}
		// Block indefinitely on channel - no timer allocation
		<-l.fastWakeupCh
		l.wakeUpSignalPending.Store(0)
		if l.testHooks != nil && l.testHooks.PrePollAwake != nil {
			l.testHooks.PrePollAwake()
		}
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// Short timeout - use timer
	// Check termination before timer-protected block
	if l.state.Load() == StateTerminating {
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}
	timer := time.NewTimer(time.Duration(timeoutMs) * time.Millisecond)
	select {
	case <-l.fastWakeupCh:
		timer.Stop()
		l.wakeUpSignalPending.Store(0)
	case <-timer.C:
	}

	if l.testHooks != nil && l.testHooks.PrePollAwake != nil {
		l.testHooks.PrePollAwake()
	}

	l.state.TryTransition(StateSleeping, StateRunning)
}

// handlePollError handles errors from PollIO.
func (l *Loop) handlePollError(err error) {
	log.Printf("CRITICAL: pollIO failed: %v - terminating loop", err)
	if l.state.TryTransition(StateSleeping, StateTerminating) {
		l.shutdown()
	}
}

// drainWakeUpPipe drains the wake-up pipe and resets the wakeup pending flag.
// This is called when the pipe read fd is signaled by kqueue/epoll.
func (l *Loop) drainWakeUpPipe() {
	for {
		_, err := unix.Read(l.wakePipe, l.wakeBuf[:])
		if err != nil {
			break
		}
	}
	// Reset the wakeup pending flag so future Submit/SubmitInternal can wake again
	l.wakeUpSignalPending.Store(0)
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
	// Check state and reject ONLY if fully terminated
	// We MUST allow wake-up during StateTerminating so the loop can
	// drain queued tasks and complete shutdown
	state := l.state.Load()
	if state == StateTerminated {
		// Loop is already fully terminated - no need to wake up
		return ErrLoopTerminated
	}

	// Internal optimization: Native endianness, no binary.LittleEndian overhead
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
//
// Thread Safety: Uses mutex-based atomic state-check-and-push pattern.
func (l *Loop) Submit(task func()) error {
	// Check fast mode conditions BEFORE taking lock
	// This avoids atomic loads inside the critical section.
	fastMode := l.canUseFastPath()

	// Lock external mutex for atomic state-check-and-push
	l.externalMu.Lock()

	// Check state while holding mutex - this is atomic with the push
	state := l.state.Load()
	if state == StateTerminated {
		l.externalMu.Unlock()
		return ErrLoopTerminated
	}

	// Fast path: Simple append to auxJobs slice
	if fastMode {
		l.fastPathSubmits.Add(1)
		l.auxJobs = append(l.auxJobs, task)
		l.externalMu.Unlock()

		// Channel wakeup with automatic deduplication (buffered size 1)
		select {
		case l.fastWakeupCh <- struct{}{}:
		default:
		}
		return nil
	}

	// Normal path: Use ChunkedIngress for I/O mode
	l.external.Push(task)
	l.externalMu.Unlock()

	// I/O Mode: Need more careful wakeup with deduplication
	if l.state.Load() == StateSleeping {
		if l.wakeUpSignalPending.CompareAndSwap(0, 1) {
			l.doWakeup()
		}
	}

	return nil
}

// doWakeup sends the appropriate wakeup signal based on mode.
// In fast mode (no user I/O FDs): sends to channel (~50ns)
// In I/O mode (user I/O FDs registered): writes to pipe (~10µs)
//
// IMPORTANT: We must wake BOTH channels when transitioning from fast to I/O mode.
// The loop might be blocked in runFastPath on fastWakeupCh when I/O FDs are registered.
func (l *Loop) doWakeup() {
	// Always try channel wakeup first (covers fast path mode)
	select {
	case l.fastWakeupCh <- struct{}{}:
	default:
		// Channel already has pending wakeup
	}

	// Also do pipe wakeup if I/O FDs are registered
	if l.userIOFDCount.Load() > 0 {
		_ = l.submitWakeup()
	}
}

// SubmitInternal submits a task to the internal priority queue.
//
// State Policy during shutdown:
//   - StateTerminated: returns ErrLoopTerminated
//   - StateTerminating: ALLOWS submission (loop needs to drain in-flight work)
//   - StateSleeping/StateRunning: normal operation
//
// Thread Safety: Uses mutex-based atomic state-check-and-push pattern.
func (l *Loop) SubmitInternal(task func()) error {
	// Internal fast-path with thread affinity check:
	// If fast path is enabled, loop is running, AND we're ON the loop thread,
	// execute immediately.
	state := l.state.Load()
	if l.canUseFastPath() && state == StateRunning && l.isLoopThread() {
		// Check external queue length (need to lock for this)
		l.externalMu.Lock()
		extLen := l.external.Length()
		l.externalMu.Unlock()
		if extLen == 0 {
			// Re-check state before execution.
			// State could have changed to Terminating/Terminated between checks.
			if l.state.Load() == StateTerminated {
				return ErrLoopTerminated
			}
			l.fastPathEntries.Add(1)
			if l.testHooks != nil && l.testHooks.OnFastPathEntry != nil {
				l.testHooks.OnFastPathEntry()
			}
			// Direct execution - bypasses queue entirely
			l.safeExecute(task)

			// Wake the fast path loop to check for timers/internal tasks.
			// The task we just executed may have added timers or internal tasks
			// (e.g., ScheduleTimer -> heap.Push). Without this wakeup, the
			// runFastPath select would block indefinitely.
			if l.hasTimersPending() || l.hasInternalTasks() {
				select {
				case l.fastWakeupCh <- struct{}{}:
				default:
				}
			}
			return nil
		}
	}

	// Lock internal queue mutex for atomic state-check-and-push
	l.internalQueueMu.Lock()

	// Check state while holding mutex
	state = l.state.Load()
	if state == StateTerminated {
		l.internalQueueMu.Unlock()
		return ErrLoopTerminated
	}

	// Push the task
	l.internal.Push(task)
	l.internalQueueMu.Unlock()

	// In fast mode, runFastPath blocks on fastWakeupCh while state remains StateRunning
	// (no transition to StateSleeping). So we must send to the channel to wake it.
	if l.userIOFDCount.Load() == 0 {
		select {
		case l.fastWakeupCh <- struct{}{}:
		default:
		}
		return nil
	}

	// I/O Mode: Wake up the loop if it's sleeping (with deduplication)
	if l.state.Load() == StateSleeping {
		if l.wakeUpSignalPending.CompareAndSwap(0, 1) {
			l.doWakeup()
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
		l.doWakeup()
	}

	return nil
}

// ScheduleMicrotask schedules a microtask.
//
// MicrotaskRing now implements dynamic growth, so Push never fails.
func (l *Loop) ScheduleMicrotask(fn func()) error {
	state := l.state.Load()
	if state == StateTerminated {
		return ErrLoopTerminated
	}

	// Check fast mode conditions BEFORE taking lock
	fastMode := l.canUseFastPath()

	// Lock external mutex for atomic push
	l.externalMu.Lock()

	// Check state again while holding mutex - this is atomic with push
	state = l.state.Load()
	if state == StateTerminated {
		l.externalMu.Unlock()
		return ErrLoopTerminated
	}

	// Add to microtask queue
	l.microtasks.Push(fn)
	l.externalMu.Unlock()

	// Wake up the loop to process the microtask
	if fastMode {
		// Fast path: Simple channel wakeup with automatic deduplication
		select {
		case l.fastWakeupCh <- struct{}{}:
		default:
		}
	} else if state == StateSleeping {
		// I/O mode: Need proper wakeup with deduplication
		if l.wakeUpSignalPending.CompareAndSwap(0, 1) {
			l.doWakeup()
		}
	}

	return nil
}

// scheduleMicrotask adds a task to the microtask queue (internal use).
//
// MicrotaskRing now implements dynamic growth, so Push never fails.
func (l *Loop) scheduleMicrotask(task func()) {
	if task != nil {
		l.microtasks.Push(task)
	}
}

// RegisterFD registers a file descriptor for I/O event monitoring.
//
// Invariant: RegisterFD is incompatible with FastPathForced mode.
//   - Returns ErrFastPathIncompatible if mode is FastPathForced.
//   - If mode changes to FastPathForced during registration (race),
//     the FD is automatically unregistered and error returned.
//
// When a user FD is registered, the loop switches to poll-based mode
// which has higher latency (~10µs) but supports I/O event notification.
//
// Thread Safety: Safe to call concurrently with SetFastPathMode.
// Uses optimistic increment with validation/rollback on conflict.
func (l *Loop) RegisterFD(fd int, events IOEvents, callback func(events IOEvents)) error {
	// Fast rejection before expensive syscall.
	if FastPathMode(l.fastPathMode.Load()) == FastPathForced {
		return ErrFastPathIncompatible
	}

	// Perform registration with OS/poller (syscall: epoll_ctl/kevent)
	err := l.poller.RegisterFD(fd, events, callback)
	if err != nil {
		return err
	}

	// Increment count (Store our side of the invariant)
	l.userIOFDCount.Add(1)

	// Verify Mode (Load to check secondary state)
	if FastPathMode(l.fastPathMode.Load()) == FastPathForced {
		// ROLLBACK: Mode incompatibility detected.
		// Only decrement if UnregisterFD succeeds (we removed the FD).
		if err := l.poller.UnregisterFD(fd); err != ErrFDNotRegistered {
			l.userIOFDCount.Add(-1)
		}
		return ErrFastPathIncompatible
	}

	// Successfully registered FD in non-Forced mode. Wake the loop
	// to transition from fast-path to poll-path if needed.
	// In fast path, loop blocks on fastWakeupCh while in StateRunning,
	// so we must always send to the channel when in fast mode.
	// In I/O mode, loop blocks on kqueue while in StateSleeping.
	select {
	case l.fastWakeupCh <- struct{}{}:
	default:
	}
	// Also wake via pipe in case loop is in I/O mode
	if l.state.Load() == StateSleeping {
		_ = l.submitWakeup()
	}

	return nil
}

// UnregisterFD removes a file descriptor from monitoring.
//
// When the last user FD is unregistered, the loop switches to channel-based
// wakeup mode which has lower latency (~50ns).
func (l *Loop) UnregisterFD(fd int) error {
	err := l.poller.UnregisterFD(fd)
	if err == nil {
		l.userIOFDCount.Add(-1)
	}
	return err
}

// ModifyFD updates the events being monitored for a file descriptor.
func (l *Loop) ModifyFD(fd int, events IOEvents) error {
	return l.poller.ModifyFD(fd, events)
}

// CurrentTickTime returns the cached time for the current tick.
// The returned value uses the monotonic clock and is safe to use for timer calculations.
func (l *Loop) CurrentTickTime() time.Time {
	l.tickAnchorMu.RLock()
	anchor := l.tickAnchor
	l.tickAnchorMu.RUnlock()

	// If anchor not initialized (shouldn't happen after Run), return current wall-clock time
	if anchor.IsZero() {
		return time.Now()
	}
	// Add elapsed monotonic offset to anchor to get current monotonic time
	// This ensures timer accuracy even if wall-clock is adjusted
	elapsed := time.Duration(l.tickElapsedTime.Load())
	return anchor.Add(elapsed)
}

// SetTickAnchor sets the tick anchor time (for testing only).
func (l *Loop) SetTickAnchor(t time.Time) {
	l.tickAnchorMu.Lock()
	l.tickAnchor = t
	l.tickAnchorMu.Unlock()
	l.tickElapsedTime.Store(0)
}

// TickAnchor returns the tick anchor time (for testing only).
func (l *Loop) TickAnchor() time.Time {
	l.tickAnchorMu.RLock()
	defer l.tickAnchorMu.RUnlock()
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
		t := heap.Pop(&l.timers).(*timer)

		// Handle canceled timer before deletion from timerMap
		if !t.canceled.Load() {
			// HTML5 spec: Set nesting level to timer's scheduled depth + 1 during execution
			// This tracks call stack depth for nested setTimeout calls
			oldDepth := l.timerNestingDepth.Load()
			newDepth := t.nestingLevel + 1
			l.timerNestingDepth.Store(newDepth)

			l.safeExecute(t.task)
			delete(l.timerMap, t.id)

			// Restore nesting level after callback completes
			l.timerNestingDepth.Store(oldDepth)

			// Zero-alloc: Return timer to pool
			t.task = nil // Avoid keeping reference
			timerPool.Put(t)
		} else {
			delete(l.timerMap, t.id)
			// Zero-alloc: Return timer to pool even if canceled
			timerPool.Put(t)
		}

		if l.StrictMicrotaskOrdering {
			l.drainMicrotasks()
		}
	}
}

// ScheduleTimer schedules a task to be executed after the specified delay.
//
// Returns a TimerID that can be used to cancel the timer before it fires.
//
// HTML5 Spec Compliance:
// If this timer is nested deeper than 5 levels, its delay will be clamped to 4ms.
// This matches browser behavior for nested setTimeout/setInterval.
func (l *Loop) ScheduleTimer(delay time.Duration, fn func()) (TimerID, error) {
	// HTML5 spec: Clamp delay to 4ms if nesting depth > 5 and delay < 4ms
	// See: https://html.spec.whatwg.org/multipage/timers-and-user-prompts.html#timers
	// "If nesting level is greater than 5, and timeout is less than 4, then increase timeout to 4."
	currentDepth := l.timerNestingDepth.Load()
	if currentDepth > 5 {
		minDelay := 4 * time.Millisecond
		if delay >= 0 && delay < minDelay {
			delay = minDelay
		}
	}

	// Get timer from pool for zero-alloc in hot path
	t := timerPool.Get().(*timer)
	t.id = TimerID(l.nextTimerID.Add(1))
	t.when = l.CurrentTickTime().Add(delay)
	t.task = fn
	t.nestingLevel = currentDepth
	t.canceled.Store(false)
	t.heapIndex = -1

	// Return timer to pool on error
	id := t.id
	err := l.SubmitInternal(func() {
		l.timerMap[id] = t
		heap.Push(&l.timers, t)
	})
	if err != nil {
		// Put back to pool on error
		t.task = nil // Avoid keeping reference
		timerPool.Put(t)
		return 0, err
	}

	return id, nil
}

// CancelTimer cancels a scheduled timer before it fires.
// Returns ErrTimerNotFound if the timer does not exist.
func (l *Loop) CancelTimer(id TimerID) error {
	result := make(chan error, 1)

	// Submit to loop thread to ensure thread-safe access to timerMap and timer heap
	if err := l.SubmitInternal(func() {
		t, exists := l.timerMap[id]
		if !exists {
			result <- ErrTimerNotFound
			return
		}
		// Mark as canceled
		t.canceled.Store(true)
		// Remove from timerMap
		delete(l.timerMap, id)
		// Remove from heap using heapIndex
		if t.heapIndex < len(l.timers) {
			heap.Remove(&l.timers, t.heapIndex)
		}
		result <- nil
	}); err != nil {
		return err
	}

	return <-result
}

// safeExecute executes a task with panic recovery.
func (l *Loop) safeExecute(t func()) {
	if t == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("ERROR: eventloop: task panicked: %v", r)
		}
	}()

	t()
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
// Uses sync.Once to ensure FDs are only closed once,
// even if called from multiple paths (shutdown + poll error).
func (l *Loop) closeFDs() {
	l.closeOnce.Do(func() {
		_ = l.poller.Close()
		_ = unix.Close(l.wakePipe)
		if l.wakePipeWrite != l.wakePipe {
			_ = unix.Close(l.wakePipeWrite)
		}
	})
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
			// Always wake the loop - could be in poll path (StateSleeping) or
			// fast path (StateRunning but blocked on fastWakeupCh)
			l.doWakeup()
			return nil
		}
	}
}
