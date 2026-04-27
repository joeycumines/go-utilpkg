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

	"github.com/joeycumines/go-eventloop/internal/runtimeutil"
	"github.com/joeycumines/logiface"
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

	// ErrTimerIDExhausted is returned when the timer ID would exceed JavaScript's MAX_SAFE_INTEGER.
	// This prevents precision loss when casting timer IDs to float64 for JavaScript interop.
	ErrTimerIDExhausted = errors.New("eventloop: timer ID exceeded MAX_SAFE_INTEGER")

	// ErrLoopNotRunning is returned when an operation requires a running loop.
	ErrLoopNotRunning = errors.New("eventloop: loop is not running")
)

// timerPool for amortized timer allocations.
var timerPool = sync.Pool{New: func() any { return new(timer) }}

// loopTestHooks provides injection points for deterministic race testing.
type loopTestHooks struct {
	PrePollSleep           func()       // Called before CAS to StateSleeping
	PrePollAwake           func()       // Called before CAS back to StateRunning
	OnFastPathEntry        func()       // Called when entering fast path (runFastPath or direct exec)
	AfterOptimisticCheck   func()       // Called after optimistic check, before Swap
	BeforeFastPathRollback func()       // Called before attempting to rollback fast path mode
	BeforeTerminateState   func()       // Called after choosing termination, before StateTerminated is stored
	PollError              func() error // Injects poll error for testing handlePollError
	OnSubmitWakeup         func()       // Called when submitWakeup() is invoked (for testing pipe write optimization)
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
//   - Mutex+chunked ingress queue (chunkedIngress) which outperforms lock-free under contention
//   - Direct FD indexing
//   - Inline callback execution
//
// Design note: Mutex+chunking is used for ingress because benchmarks showed it outperforms
// lock-free CAS under high contention due to O(N) retry storms.
//
// Auto-Exit and the Quiescing Protocol:
//
// When WithAutoExit(true) is configured, the loop monitors its own liveness via [Loop.Alive]
// and terminates itself when no liveness-adding work remains (ref'd timers, I/O FDs, Promisify
// goroutines). The quiescing protocol prevents a race between the decision to exit and
// concurrent API calls that would add liveness:
//
// The loop goroutine sets an atomic quiescing flag before committing to termination. All
// liveness-adding APIs—[Loop.ScheduleTimer], [Loop.RegisterFD], [Loop.RefTimer],
// [Loop.Promisify], and the internal submitToQueue—check this flag and reject work with
// [ErrLoopTerminated] when set. After setting quiescing, the loop re-checks [Loop.Alive]; if
// new work arrived between the first check and the flag (detected via submissionEpoch), the
// termination is aborted and the flag cleared.
//
// The following APIs are intentionally NOT gated by quiescing because they represent
// ephemeral, self-draining work whose arrival is detected by the submissionEpoch mechanism
// inside [Loop.Alive], causing the termination abort:
//   - [Loop.Submit] — enqueues a one-shot task
//   - [Loop.ScheduleMicrotask] — enqueues a microtask
//   - [Loop.ScheduleNextTick] — enqueues a nextTick callback
//
// Adding a quiescing check to these ephemeral APIs would be actively harmful: it would
// reject work that would correctly prevent the (now-invalid) termination.
//
// Thread Safety:
//
// Loop is designed for concurrent use from multiple goroutines. The following
// methods are safe to call concurrently:
//   - [Loop.Submit], [Loop.SubmitInternal], [Loop.ScheduleMicrotask] - task submission
//   - [Loop.ScheduleTimer], [Loop.CancelTimer] - timer management
//   - [Loop.RegisterFD], [Loop.UnregisterFD], [Loop.ModifyFD] - I/O registration
//   - [Loop.SetFastPathMode] - runtime mode configuration
//   - [Loop.Metrics] - metrics retrieval (returns consistent snapshot)
//   - [Loop.State], [Loop.CurrentTickTime] - state inspection
//   - [Loop.Wake] - manual wakeup
//   - [Loop.Shutdown], [Loop.Close] - lifecycle management
//
// The following should only be called once:
//   - [Loop.Run] - blocks until completion; returns error if called again
//
// Callbacks registered via Submit, ScheduleTimer, etc. are always executed
// on the loop goroutine (the goroutine calling Run), never concurrently.
type Loop struct {
	_ [0]func() // Prevent copying

	// Large pointer-heavy types (all require 8-byte alignment)
	batchBuf      [256]func()
	tickAnchor    time.Time
	registry      *registry
	state         *fastState
	testHooks     *loopTestHooks
	external      *chunkedIngress
	internal      *chunkedIngress
	microtasks    *microtaskRing
	nextTickQueue *microtaskRing                   // process.nextTick queue (runs before microtasks)
	metrics       *Metrics                         // Optional runtime metrics
	tpsCounter    *tpsCounter                      // TPS tracking
	logger        *logiface.Logger[logiface.Event] // Optional structured logger
	// OnOverload is called when the loop detects task queue overload.
	// The callback receives [ErrLoopOverloaded] and may be used for
	// backpressure signaling, metrics, or graceful degradation.
	// It is called on the loop goroutine; panics are recovered and logged.
	OnOverload   func(error)
	fastWakeupCh chan struct{}
	loopDone     chan struct{}
	runCh        chan struct{} // closed when Run() is first called
	timerMap     map[TimerID]*timer
	timers       timerHeap
	auxJobs      []func()
	auxJobsSpare []func()
	poller       fastPoller
	promisifyWg  sync.WaitGroup

	// Simple primitive types BEFORE anything that requires pointer alignment
	tickCount     uint64
	id            uint64
	wakePipe      int
	wakePipeWrite int

	// Atomic fields (all require 8-byte alignment).
	// NOTE: These fields do NOT have cache line padding. They share cache lines
	// with each other and with synchronization primitives (sync.Mutex, sync.RWMutex, sync.Once).
	// This can cause false sharing in multi-core scenarios. The fields are grouped here
	// to minimize worst-case sharing, but loopGoroutineID, userIOFDCount, wakeUpSignalPending,
	// and fastPathMode are cross-goroutine accessed and would benefit from cache line isolation.
	// See align_test.go for verification of cache line positions.
	nextTimerID         atomic.Uint64
	tickElapsedTime     atomic.Int64
	loopGoroutineID     atomic.Int64
	fastPathEntries     atomic.Int64
	fastPathSubmits     atomic.Int64
	tickAnchorMu        sync.RWMutex
	stopOnce            sync.Once
	closeOnce           sync.Once
	closeLoopDoneOnce   sync.Once // ensures loopDone is closed exactly once
	runChOnce           sync.Once // ensures runCh is closed exactly once
	externalMu          sync.Mutex
	internalQueueMu     sync.Mutex
	timerNestingDepth   atomic.Int32 // HTML5 spec: nesting depth for timeout clamping
	userIOFDCount       atomic.Int32
	wakeUpSignalPending atomic.Uint32
	fastPathMode        atomic.Int32
	refedTimerCount     atomic.Int32 // ref'd active timers only
	// quiescing is the auto-exit quiescing gate. Set by the loop goroutine in run()/runFastPath()
	// before committing to termination. All liveness-adding APIs (ScheduleTimer, RegisterFD,
	// RefTimer, Promisify, submitToQueue) check this flag and reject work when set. In run(),
	// cleared if the Alive() re-check detects in-flight work (termination abort). In
	// runFastPath(), the flag may remain set on return to run(), which re-evaluates it.
	// Never set when autoExit is false.
	quiescing       atomic.Bool
	promisifyCount  atomic.Int64  // in-flight Promisify goroutines
	submissionEpoch atomic.Uint64 // incremented after each work-adding mutation for Alive() consistency

	wakeBuf                 [8]byte
	_                       [2]byte // Align to 8-byte
	_                       [2]byte // Align to 8-byte
	forceNonBlockingPoll    bool
	strictMicrotaskOrdering bool
	debugMode               bool       // Enable debug features like stack trace capture
	autoExit                bool       // Exit Run() when Alive() returns false
	promisifyMu             sync.Mutex // Protects promisifyWg + state check for Promisify
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
	nestingLevel int32       // Nesting level at scheduling time for HTML5 clamping
	refed        atomic.Bool // default true; when false, timer doesn't keep loop alive
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
	old[n-1] = nil   // Avoid memory leak
	t.heapIndex = -1 // Invalidate index to prevent re-entrant heap corruption
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

	wakeFd, wakeWriteFd, err := createWakeFd(0, efdCloexec|efdNonblock)
	if err != nil {
		return nil, err
	}

	loop := &Loop{
		id:            loopIDCounter.Add(1),
		state:         newFastState(),
		external:      newChunkedIngressWithSize(options.ingressChunkSize), // configurable chunk size
		internal:      newChunkedIngressWithSize(options.ingressChunkSize), // configurable chunk size
		microtasks:    newMicrotaskRing(),
		nextTickQueue: newMicrotaskRing(), // nextTick runs before microtasks
		registry:      newRegistry(),
		timers:        make(timerHeap, 0),
		timerMap:      make(map[TimerID]*timer),
		wakePipe:      wakeFd,
		wakePipeWrite: wakeWriteFd,
		// Buffer size 1 prevents blocking on send when channel is full
		fastWakeupCh: make(chan struct{}, 1),
		loopDone:     make(chan struct{}),
		runCh:        make(chan struct{}),
	}

	// Apply options to Loop struct
	loop.strictMicrotaskOrdering = options.strictMicrotaskOrdering
	loop.fastPathMode.Store(int32(options.fastPathMode))
	loop.logger = options.logger
	loop.debugMode = options.debugMode // Enable debug mode
	loop.autoExit = options.autoExit   // Auto-exit when not alive

	// Phase 5.3: Initialize metrics if enabled
	if options.metricsEnabled {
		loop.metrics = &Metrics{}
		loop.tpsCounter = newTPSCounter(10*time.Second, 100*time.Millisecond)
	}

	if err := loop.poller.Init(); err != nil {
		// Clean up wake FDs on error (if they exist)
		if wakeFd >= 0 {
			_ = closeFD(wakeFd)
			if wakeWriteFd != wakeFd {
				_ = closeFD(wakeWriteFd)
			}
		}
		return nil, err
	}

	// Register wake FD for events (Unix/Linux/Darwin only)
	// On Windows, wakeFd is -1, so skip registration
	if wakeFd >= 0 {
		if err := loop.poller.RegisterFD(wakeFd, EventRead, func(IOEvents) {
			loop.drainWakeUpPipe()
		}); err != nil {
			_ = loop.poller.Close()
			_ = closeFD(wakeFd)
			if wakeWriteFd != wakeFd {
				_ = closeFD(wakeWriteFd)
			}
			return nil, err
		}
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

	// Signal that Run() has been called so that synchronous operations
	// (UnrefTimer, RefTimer, CancelTimer, CancelTimers) can distinguish
	// "Run() called but goroutine hasn't started" from "Run() never called".
	// Uses sync.Once to prevent double-close panic when multiple goroutines
	// call Run() concurrently.
	l.runChOnce.Do(func() { close(l.runCh) })

	if !l.state.TryTransition(StateAwake, StateRunning) {
		currentState := l.state.Load()
		if currentState == StateTerminated || currentState == StateTerminating {
			return ErrLoopTerminated
		}
		return ErrLoopAlreadyRunning
	}

	// Close loopDone when Run() exits — placed AFTER TryTransition so only
	// the goroutine that successfully transitions to StateRunning owns the
	// loopDone lifecycle. A second Run() call that fails TryTransition must
	// NOT close loopDone, as that would poison sync operations (CancelTimer,
	// RefTimer) that monitor loopDone to detect termination.
	defer l.closeLoopDoneOnce.Do(func() { close(l.loopDone) })

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
				// Loop hasn't started running yet - just set state and return
				l.state.Store(StateTerminated)
				l.closeFDs()

				// Close loopDone since Run() was never called and won't close it.
				// Without this, goroutines blocked on <-loopDone in
				// submitTimerRefChange/CancelTimer/CancelTimers would deadlock
				// (or wait 100ms before timing out with ErrLoopNotRunning).
				l.closeLoopDoneOnce.Do(func() { close(l.loopDone) })

				// Wait for in-flight Promisify goroutines before cleanup
				l.promisifyMu.Lock()
				//lint:ignore SA2001 intentional memory barrier
				l.promisifyMu.Unlock()
				l.promisifyWg.Wait()

				l.terminateCleanup()
				l.registry.RejectAll(ErrLoopTerminated)

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
		// Loop goroutine exited cleanly - wait for promisify goroutines
		// This is safe because we're waiting from Shutdown() caller's thread,
		// not from the loop goroutine itself
		l.shutdown()
		return nil
	case <-ctx.Done():
		// Shutdown timed out after waking loop
		// This indicates deadlock - loop should have exited by now
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

	l.loopGoroutineID.Store(runtimeutil.GoroutineID())
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
			// Context cancelled, initiate shutdown sequence and return
			// DO NOT wait for promisifyWg in loop goroutine itself - that causes deadlock
			// if a Promisify goroutine is blocking on something
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
			// Transition state to Terminated so new Promisify operations are rejected
			// Drain queues and clean up - promisifyWg.Wait() is handled by Shutdown() caller
			l.transitionToTerminated()
			l.terminateCleanup() // GAP-AE-06: full cleanup resets all liveness counters
			l.closeFDs()
			return ctx.Err()
		default:
		}

		if l.state.Load() == StateTerminating || l.state.Load() == StateTerminated {
			// State already set by Shutdown caller - just do cleanup and return
			l.closeFDs()
			return nil
		}

		// Auto-exit: if enabled and no ref'd work remains, terminate cleanly.
		// This is analogous to libuv's UV_RUN_DEFAULT mode where the loop exits
		// when there are no more active and referenced handles.
		//
		// Quiescing protocol: set the quiescing flag BEFORE committing termination.
		// This gates all liveness-adding APIs (ScheduleTimer, RegisterFD, RefTimer,
		// Promisify) so no new work can be accepted after this point. Then re-check
		// Alive() to catch any work that was added between the initial !Alive()
		// decision and the flag being set (the epoch-based consistency in Alive()
		// detects concurrent epoch changes). If work was added, abort termination.
		if l.autoExit && !l.Alive() {
			l.quiescing.Store(true)

			// Re-check after gate: catches work added between !Alive() and the flag.
			if l.Alive() {
				l.quiescing.Store(false)
				continue
			}

			l.transitionToTerminated()
			l.terminateCleanup()
			l.closeFDs()
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
	// Fast path must handle: StateTerminated (=1) AND StateTerminating (=5)
	// This is different from main loop which can use >= comparison
	currentState := l.state.Load()
	if currentState == StateTerminated || currentState == StateTerminating {
		return true
	}

	for {
		// Auto-exit check: don't block in fast path if loop should exit.
		// Uses quiescing protocol: set flag, re-check Alive(), abort if work arrived.
		if l.autoExit && !l.Alive() {
			l.quiescing.Store(true)
			if l.Alive() {
				l.quiescing.Store(false)
				continue
			}
			return true // exit to main loop — quiescing flag stays set
		}

		select {
		case <-ctx.Done():
			return true

		case <-l.fastWakeupCh:
			l.runAux()

			// Check for shutdown
			// Fast path must handle both terminated states
			currentState := l.state.Load()
			if currentState == StateTerminated || currentState == StateTerminating {
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

		if l.strictMicrotaskOrdering {
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

// transitionToTerminated performs the quick state transition and queue draining
// that can be safely called from the loop goroutine itself.
// This does NOT wait for promisifyWg to prevent deadlock.
func (l *Loop) transitionToTerminated() {
	if l.testHooks != nil && l.testHooks.BeforeTerminateState != nil {
		l.testHooks.BeforeTerminateState()
	}

	// Lock promisifyMu to prevent new Promisify operations while we shut down
	l.promisifyMu.Lock()
	l.state.Store(StateTerminated)
	l.promisifyMu.Unlock()

	// Drain loop queues quickly (single pass, not exhaustive)
	// This tasks that are already queued will get executed
	// Tasks submitted after this point will be rejected

	// Drain internal queue
	for {
		l.internalQueueMu.Lock()
		task, ok := l.internal.Pop()
		l.internalQueueMu.Unlock()
		if !ok {
			break
		}
		l.safeExecute(task)
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
	}

	// Drain fast path queue (auxJobs)
	l.externalMu.Lock()
	jobs := l.auxJobs
	l.auxJobs = l.auxJobsSpare
	l.externalMu.Unlock()
	for i, job := range jobs {
		l.safeExecute(job)
		jobs[i] = nil
	}
	l.auxJobsSpare = jobs[:0]

	l.drainDeferredQueues()

	// Reject all remaining pending promises
	l.registry.RejectAll(ErrLoopTerminated)
}

// shutdown performs the shutdown sequence.
// IMPORTANT: Do NOT call this from the loop goroutine itself - it will cause deadlock
// if waiting for promisifyWg goroutines that may be blocked.
// This function should only be called from Shutdown() in user's thread.
func (l *Loop) shutdown() {
	// Lock promisifyMu to prevent new Promisify operations while we shut down
	// This ensures atomic transition between StateTerminated and promisifyWg.Wait()
	// NOTE: Loop goroutine may have already set StateTerminated via transitionToTerminated()
	// If so, just wait for promisifyWg
	l.promisifyMu.Lock()
	l.state.Store(StateTerminated)
	l.promisifyMu.Unlock()

	// Wait in-flight Promisify goroutines
	// This ensures their SubmitInternal calls complete before we drain queues
	// Using sync.WaitGroup.Wait() ensures ALL goroutines complete - no timeout to prevent data corruption
	// Only goroutines that already called promisifyWg.Add() will be waited for
	// Any new Promisify operations will see StateTerminated and be rejected before adding to promisifyWg
	// CRITICAL: This MUST NOT be called from the loop goroutine itself
	l.promisifyWg.Wait()

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

		if l.drainDeferredQueues() {
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

	l.terminateCleanup()

	l.closeFDs()
}

func (l *Loop) cleanupTimers() {
	for id, t := range l.timerMap {
		t.task = nil
		t.refed.Store(false)
		t.canceled.Store(true)
		t.nestingLevel = 0
		t.heapIndex = -1
		timerPool.Put(t)
		delete(l.timerMap, id)
	}
	l.timers = l.timers[:0]
	l.refedTimerCount.Store(0)
}

// terminateCleanup clears all remaining loop state after termination.
// It discards timers, resets liveness counters, and drains queues without
// executing callbacks. Must only be called after transitionToTerminated()
// has been called (which sets StateTerminated and drains+executes queues).
// In the Shutdown/Close paths, promisifyWg must be waited for first.
// In the auto-exit and context cancellation paths, it is called from the
// loop goroutine itself (no concurrent access risk since the goroutine is
// the sole consumer of these structures).
func (l *Loop) terminateCleanup() {
	// Clear quiescing flag: the termination decision is complete, so the gate
	// is no longer needed. This maintains the invariant that quiescing is only
	// true during the brief window between !Alive() and transitionToTerminated().
	// While benign in practice (StateTerminated is checked first in all gated APIs),
	// clearing the flag prevents stale state if the code is refactored.
	l.quiescing.Store(false)

	l.cleanupTimers()
	l.userIOFDCount.Store(0)

	// Discard remaining internal queue items
	for {
		l.internalQueueMu.Lock()
		_, ok := l.internal.Pop()
		l.internalQueueMu.Unlock()
		if !ok {
			break
		}
	}

	// Discard remaining external queue items
	for {
		l.externalMu.Lock()
		_, ok := l.external.Pop()
		l.externalMu.Unlock()
		if !ok {
			break
		}
	}

	// Discard remaining auxJobs
	l.externalMu.Lock()
	for i := range l.auxJobs {
		l.auxJobs[i] = nil
	}
	l.auxJobs = l.auxJobs[:0]
	l.externalMu.Unlock()

	// Discard remaining nextTick and microtask items
	for {
		if l.nextTickQueue.Pop() == nil {
			break
		}
	}
	for {
		if l.microtasks.Pop() == nil {
			break
		}
	}
}

// tick is a single iteration of the event loop.
func (l *Loop) tick() {
	l.tickCount++

	// Phase 5.3.5: Track queue depths before processing
	if l.metrics != nil {
		// Update queue depth metrics for all three queues
		l.externalMu.Lock()
		extLen := l.external.Length()
		l.externalMu.Unlock()
		l.metrics.Queue.UpdateIngress(extLen)

		l.internalQueueMu.Lock()
		intLen := l.internal.Length()
		l.internalQueueMu.Unlock()
		l.metrics.Queue.UpdateInternal(intLen)

		microLen := l.microtasks.Length()
		l.metrics.Queue.UpdateMicrotask(microLen)
	}

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

		if l.strictMicrotaskOrdering {
			l.drainMicrotasks()
		}
	}

	// Emit overload signal if more tasks remain after budget
	if remainingTasks > 0 && l.OnOverload != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					l.logError("eventloop: OnOverload callback panicked", r)
				}
			}()
			l.OnOverload(ErrLoopOverloaded)
		}()
	}
}

// drainMicrotasks drains the microtask queue.
// nextTick callbacks run before regular microtasks (like Node.js).
func (l *Loop) drainMicrotasks() {
	const budget = 1024

	for range budget {
		// Priority 1: nextTick queue (process.nextTick runs before Promise microtasks)
		if fn := l.nextTickQueue.Pop(); fn != nil {
			l.safeExecuteFn(fn)
			continue
		}

		// Priority 2: Regular microtasks (Promise reactions, queueMicrotask)
		fn := l.microtasks.Pop()
		if fn == nil {
			break
		}
		l.safeExecuteFn(fn)
	}
}

func (l *Loop) drainDeferredQueues() bool {
	const budget = 4096 // Safety limit: prevents infinite recursion if callbacks re-schedule
	drained := false
	for i := 0; i < budget; i++ {
		if fn := l.nextTickQueue.Pop(); fn != nil {
			l.safeExecuteFn(fn)
			drained = true
			continue
		}
		fn := l.microtasks.Pop()
		if fn == nil {
			break
		}
		l.safeExecuteFn(fn)
		drained = true
	}
	return drained
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

		if l.strictMicrotaskOrdering {
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

	// Auto-exit check: don't block in poll if loop should exit.
	if l.autoExit && !l.Alive() {
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
	if l.testHooks != nil && l.testHooks.PollError != nil {
		err = l.testHooks.PollError()
	}
	if err != nil {
		l.handlePollError(err)
		return
	}

	if l.testHooks != nil && l.testHooks.PrePollAwake != nil {
		l.testHooks.PrePollAwake()
	}

	// Drain auxJobs after returning from poll.
	// This handles the race where tasks raced into auxJobs during mode transition
	// (e.g., Submit() checked canUseFastPath() before lock, mode changed between
	// check and lock acquisition, task went into auxJobs while loop was in poll).
	// Without this, such tasks would starve until next mode change or shutdown.
	l.drainAuxJobs()

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
		// Drain auxJobs after returning from fast mode wakeup
		// Handles race where tasks raced into auxJobs during mode transition
		l.drainAuxJobs()
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

	// Auto-exit check: don't block in pollFastMode if loop should exit.
	if l.autoExit && !l.Alive() {
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// Non-blocking case
	if timeoutMs == 0 {
		if l.testHooks != nil && l.testHooks.PrePollAwake != nil {
			l.testHooks.PrePollAwake()
		}
		// Drain auxJobs after returning from poll
		l.drainAuxJobs()
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
		// Drain auxJobs after returning from poll
		l.drainAuxJobs()
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

	// Drain auxJobs after returning from poll
	l.drainAuxJobs()
	l.state.TryTransition(StateSleeping, StateRunning)
}

// handlePollError handles errors from PollIO.
func (l *Loop) handlePollError(err error) {
	l.logCritical("pollIO failed", err)
	if l.state.TryTransition(StateSleeping, StateTerminating) {
		l.shutdown()
	}
}

// drainWakeUpPipe drains the wake-up pipe and resets the wakeup pending flag.
// This is called when the pipe read fd is signaled by kqueue/epoll.
func (l *Loop) drainWakeUpPipe() {
	if l.wakePipe < 0 {
		// Windows: No wake pipe, nothing to drain
		l.wakeUpSignalPending.Store(0)
		return
	}
	for {
		_, err := readFD(l.wakePipe, l.wakeBuf[:])
		if err != nil {
			break
		}
	}
	// Reset the wakeup pending flag so future Submit/SubmitInternal can wake again
	l.wakeUpSignalPending.Store(0)
}

// submitWakeup writes to the wake-up pipe or calls poller.Wakeup().
//
// Wake-up Policy:
//   - REJECTS: StateTerminated (fully stopped, no tasks to process)
//   - ALLOWS: StateTerminating (loop needs to wake and drain remaining tasks)
//   - ALLOWS: StateSleeping, StateRunning, StateAwake
//
// Safe to call concurrently during shutdown - pipe write errors during shutdown are
// gracefully handled by callers.
//
// IMPLEMENTATION NOTES:
// - Unix/Linux/Darwin: Writes to wake pipe (eventfd or pipe)
// - Windows/IOCP: Calls poller.Wakeup() which uses PostQueuedCompletionStatus
func (l *Loop) submitWakeup() error {
	// Test hook: allow tests to observe submitWakeup calls
	if l.testHooks != nil && l.testHooks.OnSubmitWakeup != nil {
		l.testHooks.OnSubmitWakeup()
	}
	// Check state and reject ONLY if fully terminated
	// We MUST allow wake-up during StateTerminating so the loop can
	// drain queued tasks and complete shutdown
	state := l.state.Load()
	if state == StateTerminated {
		// Loop is already fully terminated - no need to wake up
		return ErrLoopTerminated
	}

	// Platform-specific wake mechanism
	if l.wakePipe < 0 {
		// Windows/IOCP: Use poller.Wakeup() method
		return l.poller.Wakeup()
	}

	// Unix/Linux/Darwin: Write to wake pipe
	// Internal optimization: Native endianness, no binary.LittleEndian overhead
	var one uint64 = 1
	buf := (*[8]byte)(unsafe.Pointer(&one))[:]

	_, err := writeFD(l.wakePipeWrite, buf)
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
// Quiescing Protocol: Submit is intentionally NOT gated by the quiescing flag.
// Submitted tasks are ephemeral work detected by Alive() via the submissionEpoch
// mechanism. If a task is submitted during the quiescing window, the epoch change
// causes the Alive() re-check to abort termination, and the task executes normally
// in the next tick. Adding a quiescing check here would be harmful: it would reject
// work that correctly prevents the (now-invalid) termination.
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
		l.submissionEpoch.Add(1)
		l.externalMu.Unlock()

		// Channel wakeup with automatic deduplication (buffered size 1)
		select {
		case l.fastWakeupCh <- struct{}{}:
		default:
		}

		// Defense in depth: if user I/O FDs are registered, the loop may be
		// in PollIO (not listening on fastWakeupCh) due to transient
		// mode/count disagreement from concurrent SetFastPathMode/RegisterFD.
		// Send pipe/eventfd wakeup to ensure PollIO also returns.
		if l.userIOFDCount.Load() > 0 {
			_ = l.submitWakeup()
		}

		return nil
	}

	// Normal path: Use chunkedIngress for I/O mode
	l.external.Push(task)
	l.submissionEpoch.Add(1)
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
// IMPORTANT: We send BOTH signals unconditionally. This handles the race
// where canUseFastPath() disagrees with the loop's actual poll path
// (e.g., mode=Forced + count>0 due to concurrent RegisterFD/SetFastPathMode).
// Both mechanisms are idempotent, so redundant signals are harmless.
func (l *Loop) doWakeup() {
	// Always try channel wakeup (covers fast path mode)
	select {
	case l.fastWakeupCh <- struct{}{}:
	default:
		// Channel already has pending wakeup
	}

	// Always try pipe/eventfd wakeup (covers I/O poll mode)
	// This is unconditional to prevent lost wakeups when mode and count
	// are transiently inconsistent due to concurrent SetFastPathMode/RegisterFD.
	_ = l.submitWakeup()
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

	return l.submitToQueue(task)
}

// submitToQueue pushes a task to the internal queue and wakes the loop.
// This is the slow path of SubmitInternal, extracted so that callers who
// have already determined they are NOT on the loop thread can skip the
// expensive isLoopThread() fast-path check (which calls runtime.Stack at
// ~1760 ns and 1 alloc per invocation).
func (l *Loop) submitToQueue(task func()) error {
	// Lock internal queue mutex for atomic state-check-and-push
	l.internalQueueMu.Lock()

	// Check state while holding mutex
	state := l.state.Load()
	if state == StateTerminated {
		l.internalQueueMu.Unlock()
		return ErrLoopTerminated
	}

	// Check quiescing flag (auto-exit termination window).
	// This closes the TOCTOU gap: an external goroutine may have passed the
	// API-level quiescing check before the flag was set, but this check (under
	// the same lock that guards the epoch increment) ensures correctness.
	if l.quiescing.Load() {
		l.internalQueueMu.Unlock()
		return ErrLoopTerminated
	}

	// Push the task
	l.internal.Push(task)
	l.submissionEpoch.Add(1)
	l.internalQueueMu.Unlock()

	// In fast mode, runFastPath blocks on fastWakeupCh while state remains StateRunning
	// (no transition to StateSleeping). So we must send to the channel to wake it.
	if l.userIOFDCount.Load() == 0 {
		select {
		case l.fastWakeupCh <- struct{}{}:
		default:
		}

		// No defense-in-depth submitWakeup() needed here. When userIOFDCount
		// transitions from >0 to 0, UnregisterFD's GAP-004 doWakeup() already sent
		// the pipe write that interrupts PollIO. The fastWakeupCh signal above is
		// buffered and consumed when the loop enters fast-mode after PollIO returns.
		// The loop only enters PollIO when userIOFDCount > 0, and the only path from
		// >0 to 0 during normal operation is UnregisterFD (which triggers GAP-004).
		// See docs/eventloop-autopsy-20260419/03_critical_finding.md for proof.

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

// RefTimer marks the timer as keeping the event loop alive.
// Analogous to libuv's uv_ref(). Timers are ref'd by default.
//
// Thread-safe: safe to call from any goroutine.
// When called from the loop goroutine: immediate synchronous effect via
// applyTimerRefChange (timerMap lookup + refedTimerCount update).
// When called from external goroutines: blocks until the loop processes
// the change via SubmitInternal (synchronous channel round-trip).
// Silently ignores timers that have already fired or don't exist.
func (l *Loop) RefTimer(id TimerID) error {
	return l.submitTimerRefChange(id, true)
}

// UnrefTimer marks the timer as NOT keeping the event loop alive.
// Analogous to libuv's uv_unref(). If the only remaining work is
// unref'd timers, the loop is considered idle.
//
// Thread-safe: safe to call from any goroutine.
// When called from the loop goroutine: immediate synchronous effect via
// applyTimerRefChange (timerMap lookup + refedTimerCount update).
// When called from external goroutines: blocks until the loop processes
// the change via SubmitInternal (synchronous channel round-trip).
// Silently ignores timers that have already fired or don't exist.
func (l *Loop) UnrefTimer(id TimerID) error {
	return l.submitTimerRefChange(id, false)
}

func (l *Loop) submitTimerRefChange(id TimerID, ref bool) error {
	if l.state.Load() == StateTerminated {
		return ErrLoopTerminated
	}
	// Gate RefTimer (liveness-adding) during quiescing.
	// UnrefTimer (ref=false) is allowed — it reduces liveness.
	if ref && l.quiescing.Load() {
		return ErrLoopTerminated
	}
	if l.isLoopThread() {
		l.applyTimerRefChange(id, ref)
		return nil
	}
	// Ensure Run() has been called. If Run() was never called, submitting a
	// synchronous closure to the internal queue will never be drained, causing
	// the caller to block forever. Wait for runCh to confirm Run() was called.
	// The select has two paths: non-blocking (already started) and blocking
	// with timeout (just called via go loop.Run(), goroutine hasn't run yet).
	select {
	case <-l.runCh:
		// Run() has been called — proceed
	default:
		// Run() hasn't been called yet. Wait briefly for the goroutine to start.
		select {
		case <-l.runCh:
			// Run() started during our brief wait
		case <-l.loopDone:
			return ErrLoopTerminated
		case <-time.After(time.Second):
			return ErrLoopNotRunning
		}
	}
	// External goroutine: synchronous submission to ensure immediate effect.
	// Matches libuv semantics where uv_ref()/uv_unref() are always immediate.
	result := make(chan struct{}, 1)
	// Uses submitToQueue to skip the redundant isLoopThread() check.
	if err := l.submitToQueue(func() {
		l.applyTimerRefChange(id, ref)
		result <- struct{}{}
		// Note: doWakeup() is NOT called here. applyTimerRefChange already
		// calls doWakeup() conditionally when old != ref (state actually
		// changed). Calling it unconditionally would cause spurious wakeups
		// when old == ref (no-op case, timer already in target state).
	}); err != nil {
		return err
	}
	// Use select with loopDone to prevent deadlock if Close() is called
	// while waiting for the result (Close() sets Terminated and exits without
	// processing queued closures).
	select {
	case <-result:
		return nil
	case <-l.loopDone:
		return ErrLoopTerminated
	}
}

// applyTimerRefChange applies the ref/unref change directly.
// MUST be called on the loop goroutine (timerMap is not thread-safe).
// Silently ignores timers that have already fired, been cancelled, or don't exist.
// When called from external goroutines, FIFO ordering of SubmitInternal ensures
// the timer registration closure runs before the ref change closure.
// When called from the loop thread, ScheduleTimer registers synchronously.
func (l *Loop) applyTimerRefChange(id TimerID, ref bool) {
	t, ok := l.timerMap[id]
	if !ok {
		// Timer already fired, was cancelled, or doesn't exist. Silently ignore.
		return
	}
	old := t.refed.Swap(ref)
	if old != ref {
		if ref {
			l.refedTimerCount.Add(1)
		} else {
			l.refedTimerCount.Add(-1)
		}
		// Increment epoch to ensure Alive() detects the liveness change
		l.submissionEpoch.Add(1)
		// Wake the loop so auto-exit re-checks Alive() after the count changes.
		// Only needed when auto-exit is enabled: the loop may be in PollIO
		// and needs to return so the auto-exit check sees the liveness transition.
		// When auto-exit is disabled, this is pure overhead (pipe write syscall
		// at ~1700 ns per call) with no benefit.
		if l.autoExit {
			l.doWakeup()
		}
	}
}

// Alive reports whether the event loop has ref'd pending work.
// When false, all ref'd timers have fired, all queues are empty,
// no Promisify goroutines are in-flight, and no I/O FDs are registered.
//
// Analogous to libuv's uv_loop_alive().
// Safe to call from any goroutine, including event loop callbacks.
// Uses epoch-based consistency to prevent false negatives under
// concurrent submission: reads submissionEpoch before and after checks;
// if it changed (concurrent work added), retries up to 3 times.
// After max retries, conservatively returns true.
//
// Check ordering: atomic counters are checked first (no lock acquisition)
// to reduce mutex contention under high load. Queue checks require mutex
// acquisition and are performed only when all atomic checks return zero.
func (l *Loop) Alive() bool {
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		epoch := l.submissionEpoch.Load()

		// Fast path: check atomic counters and lock-free ring buffers first.
		// These avoid internalQueueMu/externalMu contention. Note: IsEmpty()
		// on the ring buffers may acquire overflowMu, but this is a separate,
		// low-contention lock distinct from the main queue mutexes.
		if l.refedTimerCount.Load() > 0 {
			return true
		}
		if l.promisifyCount.Load() > 0 {
			return true
		}
		if l.userIOFDCount.Load() > 0 {
			return true
		}
		if !l.microtasks.IsEmpty() || !l.nextTickQueue.IsEmpty() {
			return true
		}

		// Slow path: check queue lengths under mutex.
		l.internalQueueMu.Lock()
		hasInternal := l.internal.Length() > 0
		l.internalQueueMu.Unlock()
		if hasInternal {
			return true
		}
		l.externalMu.Lock()
		hasExternal := l.external.Length() > 0 || len(l.auxJobs) > 0
		l.externalMu.Unlock()
		if hasExternal {
			return true
		}

		// Validate epoch: if unchanged, no concurrent work was added during checks
		if l.submissionEpoch.Load() == epoch {
			return false
		}
		// Epoch changed — concurrent work was added. Retry.
	}
	// Max retries exhausted — conservatively return true (safer to say alive when unsure)
	return true
}

// ScheduleMicrotask schedules a microtask.
//
// microtaskRing now implements dynamic growth, so Push never fails.
//
// Quiescing Protocol: ScheduleMicrotask is intentionally NOT gated by the quiescing
// flag. Microtasks are ephemeral work detected by Alive() via the submissionEpoch
// mechanism. If a microtask is scheduled during the quiescing window, the epoch change
// causes the Alive() re-check to abort termination. Adding a quiescing check here
// would be harmful: it would reject work that correctly prevents termination.
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
	l.submissionEpoch.Add(1)
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
// Used by platform-specific tests (regression_test.go, linux/darwin only).
//
//lint:ignore U1000 Used by platform-specific test files with build constraints.
func (l *Loop) scheduleMicrotask(task func()) {
	if task != nil {
		l.microtasks.Push(task)
	}
}

// ScheduleNextTick schedules a function to run before any microtasks in the current tick.
//
// This emulates Node.js process.nextTick() semantics. NextTick callbacks
// have higher priority than regular microtasks (promises, queueMicrotask), meaning
// they run before any promise handlers in the same tick.
//
// Unlike setTimeout(fn, 0) which schedules for the next tick, NextTick callbacks
// execute immediately after the current synchronous code, before any pending
// promise handlers.
//
// Parameters:
//   - fn: The function to execute. If nil, returns nil without scheduling.
//
// Returns:
//   - ErrLoopTerminated if the loop has been shut down.
//
// Quiescing Protocol: ScheduleNextTick is intentionally NOT gated by the quiescing
// flag. nextTick callbacks are ephemeral work detected by Alive() via the
// submissionEpoch mechanism. If a callback is scheduled during the quiescing window,
// the epoch change causes the Alive() re-check to abort termination. Adding a
// quiescing check here would be harmful: it would reject work that correctly
// prevents termination.
//
// Thread Safety: Safe to call from any goroutine.
func (l *Loop) ScheduleNextTick(fn func()) error {
	if fn == nil {
		return nil
	}

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

	// Add to nextTick queue (higher priority than microtasks)
	l.nextTickQueue.Push(fn)
	l.submissionEpoch.Add(1)
	l.externalMu.Unlock()

	// Wake up the loop to process the nextTick callback
	if fastMode {
		select {
		case l.fastWakeupCh <- struct{}{}:
		default:
		}
	} else if state == StateSleeping {
		if l.wakeUpSignalPending.CompareAndSwap(0, 1) {
			l.doWakeup()
		}
	}

	return nil
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
	// Defense-in-depth: reject on terminated loop (GAP-AE-03).
	if l.state.Load() == StateTerminated {
		return ErrLoopTerminated
	}

	// Reject during auto-exit quiescing window.
	if l.quiescing.Load() {
		return ErrLoopTerminated
	}

	// Fast rejection before expensive syscall.
	if FastPathMode(l.fastPathMode.Load()) == FastPathForced {
		return ErrFastPathIncompatible
	}

	// Perform registration with OS/poller (syscall: epoll_ctl/kevent)
	err := l.poller.RegisterFD(fd, events, callback)
	if err != nil {
		return err
	}

	// Re-check quiescing after expensive syscall.
	// The syscall can take microseconds, during which the loop may have
	// entered quiescing. If so, rollback the registration.
	if l.quiescing.Load() {
		_ = l.poller.UnregisterFD(fd)
		return ErrLoopTerminated
	}

	// Increment count (Store our side of the invariant)
	l.userIOFDCount.Add(1)
	l.submissionEpoch.Add(1) // Alive() consistency: FD registration is work
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
		l.submissionEpoch.Add(1) // Alive() consistency: FD unregistration changes liveness
		// When the last I/O FD is removed, the loop may still be blocked in
		// PollIO() (not listening on fastWakeupCh). Wake it so it transitions
		// to pollFastMode immediately, rather than waiting for the next
		// submitToQueue call or PollIO timeout to trigger the mode change.
		if l.userIOFDCount.Load() == 0 {
			l.doWakeup()
		}
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
//
// Uses time.Now() (wall-clock) rather than CurrentTickTime() (stable tick time).
// This is intentional: the poll timeout must reflect actual remaining time until
// the next timer fires. CurrentTickTime() is set once at tick start and would
// be stale by the time calculateTimeout is called (end of tick). Timers are
// scheduled using CurrentTickTime().Add(delay), so their .when values use the
// stable tick time. The wall-clock comparison here correctly computes the
// remaining real time, and tickElapsedTime catches up at the start of the next
// tick so runTimers sees the timer as due.
func (l *Loop) calculateTimeout() int {
	maxDelay := 10 * time.Second

	// Cap by next timer. Uses time.Now() for wall-clock accuracy (see doc comment).
	if len(l.timers) > 0 {
		now := time.Now()
		nextFire := l.timers[0].when
		delay := max(nextFire.Sub(now), 0)
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

			// Restore nesting depth after callback execution.
			// safeExecute recovers panics, so this always runs normally.
			l.timerNestingDepth.Store(oldDepth)
			delete(l.timerMap, t.id)

			if t.refed.Load() {
				l.refedTimerCount.Add(-1)
			}

			// Zero-alloc: Return timer to pool
			t.heapIndex = -1   // Clear stale heap data
			t.nestingLevel = 0 // Clear stale nesting level
			t.refed.Store(false)
			t.task = nil // Avoid keeping reference
			timerPool.Put(t)
		} else {
			delete(l.timerMap, t.id)

			if t.refed.Load() {
				// Decrement refedTimerCount without incrementing submissionEpoch.
				// This is correct: epoch tracks liveness-*adding* mutations. A
				// timer firing reduces liveness, so Alive() returning false after
				// this decrement is the correct outcome. The epoch-based retry in
				// Alive() only needs to detect concurrent additions, not removals.
				l.refedTimerCount.Add(-1)
			}
			// Zero-alloc: Return timer to pool even if canceled
			t.heapIndex = -1   // Clear stale heap data
			t.nestingLevel = 0 // Clear stale nesting level
			t.refed.Store(false)
			t.task = nil // Clear closure reference to prevent memory leak
			timerPool.Put(t)
		}

		if l.strictMicrotaskOrdering {
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
	// Fast rejection during auto-exit quiescing window.
	// Avoids timer pool allocation when the loop is terminating.
	if l.quiescing.Load() {
		return 0, ErrLoopTerminated
	}

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
	t.refed.Store(true)
	t.heapIndex = -1

	// Return timer to pool on error
	id := t.id

	// Validate ID does not exceed JavaScript's MAX_SAFE_INTEGER
	// This must happen BEFORE SubmitInternal to prevent resource leak
	const maxSafeInteger = 9007199254740991 // 2^53 - 1
	if uint64(id) > maxSafeInteger {
		// Put back to pool - timer was never scheduled.
		// Reset ALL fields per pool hygiene contract (heapIndex, nestingLevel, refed, task).
		t.heapIndex = -1
		t.nestingLevel = 0
		t.refed.Store(false)
		t.task = nil
		timerPool.Put(t)
		return 0, ErrTimerIDExhausted
	}

	// On the loop thread: register synchronously. This bypasses
	// SubmitInternal which may queue the task in I/O mode (when
	// canUseFastPath is false), causing Schedule-then-Unref from a
	// loop callback to race if the unref arrives before the queued
	// registration is processed.
	if l.isLoopThread() {
		// Re-check quiescing on loop thread (defense-in-depth for callbacks
		// executing during termination drain phase).
		if l.quiescing.Load() {
			t.heapIndex = -1
			t.nestingLevel = 0
			t.refed.Store(false)
			t.task = nil
			timerPool.Put(t)
			return 0, ErrLoopTerminated
		}
		l.timerMap[id] = t
		heap.Push(&l.timers, t)
		l.refedTimerCount.Add(1)
		l.submissionEpoch.Add(1)
		return id, nil
	}

	// External goroutine: use submitToQueue (not SubmitInternal) to
	// skip the redundant isLoopThread() check. We already proved
	// we're not on the loop thread above.
	err := l.submitToQueue(func() {
		l.timerMap[id] = t
		heap.Push(&l.timers, t)
		l.refedTimerCount.Add(1)
		l.submissionEpoch.Add(1)
	})
	if err != nil {
		// Put back to pool on error.
		// Reset ALL fields per pool hygiene contract (heapIndex, nestingLevel, refed, task).
		t.heapIndex = -1
		t.nestingLevel = 0
		t.refed.Store(false)
		t.task = nil
		timerPool.Put(t)
		return 0, err
	}

	return id, nil
}

// CancelTimer cancels a scheduled timer before it fires.
// Returns ErrTimerNotFound if the timer does not exist.
// Returns ErrLoopNotRunning if the loop is not in a valid state (not running or terminating or terminated).
//
// Not gated by the quiescing flag: cancellation reduces liveness (opposite of
// ScheduleTimer which IS gated). This asymmetry is intentional — during the
// quiescing window, callers can cancel timers but not schedule new ones.
func (l *Loop) CancelTimer(id TimerID) error {
	// Check if loop is in a valid state for cancellation.
	state := l.state.Load()
	if state == StateTerminated {
		return ErrLoopTerminated
	}

	// If Run() was never called, queueing a synchronous closure will never
	// be drained. Wait for runCh to confirm Run() was called.
	if state == StateAwake {
		select {
		case <-l.runCh:
			// Run() called — proceed
		case <-l.loopDone:
			return ErrLoopTerminated
		case <-time.After(time.Second):
			return ErrLoopNotRunning
		}
	}

	// Loop thread: execute directly to prevent deadlock when I/O FDs are
	// registered (canUseFastPath=false). Without this check, submitToQueue
	// would queue the closure and the caller would block on <-result while
	// the loop thread (the only consumer of the queue) is blocked waiting.
	if l.isLoopThread() {
		return l.applyCancelTimer(id)
	}

	result := make(chan error, 1)

	// Submit to loop thread. Uses submitToQueue to skip the redundant
	// isLoopThread() check — we already proved we're external above.
	if err := l.submitToQueue(func() {
		result <- l.applyCancelTimer(id)
	}); err != nil {
		return err
	}

	// Use select with loopDone to prevent deadlock if Close() is called
	// while this goroutine is waiting for the result. Close() sets
	// StateTerminated and the loop exits without processing queued closures,
	// so the result channel would never be written to.
	select {
	case err := <-result:
		return err
	case <-l.loopDone:
		return ErrLoopTerminated
	}
}

// applyCancelTimer cancels a timer by ID. MUST be called on the loop goroutine.
func (l *Loop) applyCancelTimer(id TimerID) error {
	t, exists := l.timerMap[id]
	if !exists {
		// Timer not in map — already fired or cancelled
		return ErrTimerNotFound
	}
	// Mark as canceled
	t.canceled.Store(true)
	// If timer was already popped from heap (e.g., by runTimers during
	// a callback that calls CancelTimer on its own timer ID), skip cleanup.
	// runTimers will handle counter decrement and pool return.
	if t.heapIndex < 0 || t.heapIndex >= len(l.timers) {
		return nil
	}
	// Timer is still in heap — we own the cleanup
	delete(l.timerMap, id)
	if t.refed.Load() {
		l.refedTimerCount.Add(-1)
	}
	heap.Remove(&l.timers, t.heapIndex)
	// Return timer to pool
	t.heapIndex = -1
	t.nestingLevel = 0
	t.task = nil
	t.refed.Store(false)
	timerPool.Put(t)
	return nil
}

// CancelTimers cancels multiple scheduled timers in a single batch operation.
//
// This is more efficient than calling CancelTimer multiple times because
// it acquires the lock once, removes all matching timers, and returns pool entries once.
//
// Returns a slice of errors corresponding to each timer ID:
//   - nil: Timer was successfully cancelled
//   - ErrTimerNotFound: Timer ID was not found in the timerMap
//
// Returns ErrLoopNotRunning (for all IDs) if the loop is not in a valid state.
// Returns ErrLoopTerminated (for all IDs) if SubmitInternal fails.
//
// Not gated by the quiescing flag: cancellation reduces liveness (opposite of
// ScheduleTimer which IS gated). See CancelTimer for rationale.
//
// Thread Safety: Safe to call from any goroutine.
func (l *Loop) CancelTimers(ids []TimerID) []error {
	if len(ids) == 0 {
		return nil
	}

	// Check if loop is in a valid state for cancellation.
	state := l.state.Load()
	if state == StateTerminated {
		errors := make([]error, len(ids))
		for i := range errors {
			errors[i] = ErrLoopTerminated
		}
		return errors
	}

	// If Run() was never called, queueing a synchronous closure will never
	// be drained. Wait for runCh to confirm Run() was called.
	if state == StateAwake {
		select {
		case <-l.runCh:
			// Run() called — proceed
		case <-l.loopDone:
			errors := make([]error, len(ids))
			for i := range errors {
				errors[i] = ErrLoopTerminated
			}
			return errors
		case <-time.After(time.Second):
			errors := make([]error, len(ids))
			for i := range errors {
				errors[i] = ErrLoopNotRunning
			}
			return errors
		}
	}

	// Loop thread: execute directly to prevent deadlock (same as CancelTimer)
	if l.isLoopThread() {
		return l.applyCancelTimers(ids)
	}

	result := make(chan []error, 1)

	// Submit to loop thread. Uses submitToQueue to skip the redundant
	// isLoopThread() check — we already proved we're external above.
	if err := l.submitToQueue(func() {
		result <- l.applyCancelTimers(ids)
	}); err != nil {
		// submitToQueue failed, return error for all IDs
		errors := make([]error, len(ids))
		for i := range errors {
			errors[i] = err
		}
		return errors
	}

	// Use select with loopDone to prevent deadlock if Close() is called
	select {
	case res := <-result:
		return res
	case <-l.loopDone:
		errors := make([]error, len(ids))
		for i := range errors {
			errors[i] = ErrLoopTerminated
		}
		return errors
	}
}

// applyCancelTimers cancels multiple timers. MUST be called on the loop goroutine.
func (l *Loop) applyCancelTimers(ids []TimerID) []error {
	errors := make([]error, len(ids))

	// Collect timers to remove from heap (use slice to batch heap operations)
	toRemove := make([]*timer, 0, len(ids))

	for i, id := range ids {
		t, exists := l.timerMap[id]
		if !exists {
			// Timer not in map — already fired or cancelled
			errors[i] = ErrTimerNotFound
			continue
		}
		// Mark as canceled
		t.canceled.Store(true)
		// If timer was already popped from heap (e.g., by runTimers during
		// a re-entrant callback), skip cleanup. runTimers handles it.
		if t.heapIndex < 0 || t.heapIndex >= len(l.timers) {
			errors[i] = nil
			continue
		}
		// Timer is still in heap — we own the cleanup
		delete(l.timerMap, id)

		if t.refed.Load() {
			l.refedTimerCount.Add(-1)
		}
		// Collect for batch heap removal
		toRemove = append(toRemove, t)
		errors[i] = nil
	}

	// Note: heap.Remove dynamically updates heapIndex via Swap, so the order of removals
	// doesn't actually matter for correctness - each timer's heapIndex is current at removal time.
	for _, t := range toRemove {
		if t.heapIndex >= 0 && t.heapIndex < len(l.timers) {
			heap.Remove(&l.timers, t.heapIndex)
		}
		// Return timer to pool
		t.heapIndex = -1
		t.nestingLevel = 0
		t.refed.Store(false)
		t.task = nil
		timerPool.Put(t)
	}

	return errors
}

// safeExecute executes a task with panic recovery.
func (l *Loop) safeExecute(t func()) {
	if t == nil {
		return
	}

	// Phase 5.3: Record task execution time if metrics enabled
	var start time.Time
	if l.metrics != nil {
		start = time.Now()
	}

	defer func() {
		if r := recover(); r != nil {
			l.logError("eventloop: task panicked", r)
		}
		// Phase 5.3: Record latency if metrics enabled (even on panic)
		if l.metrics != nil {
			duration := time.Since(start)
			l.metrics.Latency.Record(duration)
		}
	}()

	t()

	// Phase 5.3: Record successful execution for TPS
	if l.tpsCounter != nil {
		l.tpsCounter.Increment()
	}
}

// safeExecuteFn executes a function with panic recovery.
func (l *Loop) safeExecuteFn(fn func()) {
	if fn == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			l.logError("eventloop: task panicked", r)
		}
	}()

	fn()
}

// Structured Error Logging Helpers

// logError logs an error message using structured logging if configured,
// otherwise falls back to log.Printf for backward compatibility.
//
// Defensively recovers from panics in structured logging. This can happen if:
//   - The logger is nil or zero-value (caught by Enabled())
//   - The logger has no event factory (Builder.Err() returns non-nil but with nil Event)
//   - Any other configuration issue with the logger
//
// Logging should never crash the application.
func (l *Loop) logError(msg string, panicVal any) {
	if l.logger.Enabled() {
		logged := func() (ok bool) {
			defer func() {
				if recover() != nil {
					ok = false
				}
			}()
			l.logger.Err().
				Str("component", "eventloop").
				Any("panic", panicVal).
				Log(msg)
			return true
		}()
		if logged {
			return
		}
	}
	log.Printf("ERROR: %s: %v", msg, panicVal)
}

// logCritical logs a critical error message using structured logging if configured,
// otherwise falls back to log.Printf for backward compatibility.
//
// Defensively recovers from panics in structured logging. This can happen if:
//   - The logger is nil or zero-value (caught by Enabled())
//   - The logger has no event factory (Builder.Crit() returns non-nil but with nil Event)
//   - Any other configuration issue with the logger
//
// Logging should never crash the application.
func (l *Loop) logCritical(msg string, err error) {
	if l.logger.Enabled() {
		logged := func() (ok bool) {
			defer func() {
				if recover() != nil {
					ok = false
				}
			}()
			l.logger.Crit().
				Str("component", "eventloop").
				Err(err).
				Log(msg)
			return true
		}()
		if logged {
			return
		}
	}
	log.Printf("CRITICAL: %s: %v - terminating loop", msg, err)
}

// Metrics returns current metrics of the event loop.
//
// This method samples latency percentiles (P50, P90, P95, P99) using the P-Square
// algorithm for O(1) retrieval. For counts >= 5, percentiles are approximate
// (typically <5% relative error); for smaller counts, exact sorting is used.
//
// Recommendation: Call Metrics() no more than once per second for monitoring.
// More frequent calls are safe but provide diminishing value.
//
// Thread Safety:
//
// This method is safe to call concurrently from any goroutine. It returns a
// consistent snapshot of metrics at call time. The returned *Metrics struct
// is a deep copy that can be retained and read without synchronization.
// The caller owns the returned struct and can safely access all fields.
func (l *Loop) Metrics() *Metrics {
	if l.metrics == nil {
		return nil
	}

	// Lock entire Metrics struct for consistent snapshot
	l.metrics.mu.Lock()
	defer l.metrics.mu.Unlock()

	// Update TPS from rolling window counter
	if l.tpsCounter != nil {
		l.metrics.TPS = l.tpsCounter.TPS()
	}

	// Sample latency percentiles (held under LatencyMetrics.mu)
	_ = l.metrics.Latency.Sample()

	snapshot := &Metrics{
		TPS: l.metrics.TPS,
	}

	// Capture Queue metrics with read lock
	// Copy fields individually to avoid "copylocks" warning from vet
	l.metrics.Queue.mu.RLock()
	snapshot.Queue.IngressCurrent = l.metrics.Queue.IngressCurrent
	snapshot.Queue.InternalCurrent = l.metrics.Queue.InternalCurrent
	snapshot.Queue.MicrotaskCurrent = l.metrics.Queue.MicrotaskCurrent
	snapshot.Queue.IngressMax = l.metrics.Queue.IngressMax
	snapshot.Queue.InternalMax = l.metrics.Queue.InternalMax
	snapshot.Queue.MicrotaskMax = l.metrics.Queue.MicrotaskMax
	snapshot.Queue.IngressAvg = l.metrics.Queue.IngressAvg
	snapshot.Queue.InternalAvg = l.metrics.Queue.InternalAvg
	snapshot.Queue.MicrotaskAvg = l.metrics.Queue.MicrotaskAvg
	snapshot.Queue.ingressEMAInitialized = l.metrics.Queue.ingressEMAInitialized
	snapshot.Queue.internalEMAInitialized = l.metrics.Queue.internalEMAInitialized
	snapshot.Queue.microtaskEMAInitialized = l.metrics.Queue.microtaskEMAInitialized
	l.metrics.Queue.mu.RUnlock()

	// Capture Latency metrics with read lock
	l.metrics.Latency.mu.RLock()
	snapshot.Latency.sampleIdx = l.metrics.Latency.sampleIdx
	snapshot.Latency.sampleCount = l.metrics.Latency.sampleCount
	snapshot.Latency.samples = l.metrics.Latency.samples // Array copy
	snapshot.Latency.P50 = l.metrics.Latency.P50
	snapshot.Latency.P90 = l.metrics.Latency.P90
	snapshot.Latency.P95 = l.metrics.Latency.P95
	snapshot.Latency.P99 = l.metrics.Latency.P99
	snapshot.Latency.Max = l.metrics.Latency.Max
	snapshot.Latency.Mean = l.metrics.Latency.Mean
	snapshot.Latency.Sum = l.metrics.Latency.Sum
	l.metrics.Latency.mu.RUnlock()

	return snapshot
}

// closeFDs closes file descriptors.
// Uses sync.Once to ensure FDs are only closed once,
// even if called from multiple paths (shutdown + poll error).
func (l *Loop) closeFDs() {
	l.closeOnce.Do(func() {
		_ = l.poller.Close()
		// Close wake pipe (Unix/Linux/Darwin only)
		if l.wakePipe >= 0 {
			_ = closeFD(l.wakePipe)
			if l.wakePipeWrite != l.wakePipe {
				_ = closeFD(l.wakePipeWrite)
			}
		}
		// On Windows/IOCP, wakePipe is -1, so closeFD is not called
		// poller.Close() handles IOCP handle closure
	})
}

// isLoopThread checks if we're on the loop goroutine.
//
// Performance Note: This implementation uses runtimeutil.GoroutineID() which provides
// ~2-5ns access vs ~1000-2000ns for runtime.Stack parsing.
// Use sparingly in extremely hot paths.
func (l *Loop) isLoopThread() bool {
	loopID := l.loopGoroutineID.Load()
	if loopID == 0 {
		return false
	}
	return runtimeutil.GoroutineID() == loopID
}

// Close immediately terminates the event loop without waiting for graceful shutdown.
//
// NOTE: Close() waits for the loop goroutine to exit before returning.
// This prevents data races where the caller frees resources that the loop
// goroutine might still be accessing.
//
// Concurrent Close()/Shutdown(): Only one of Close() or Shutdown() will
// execute cleanup. The other will see StateTerminating or StateTerminated
// and return ErrLoopTerminated. The TryTransition identity rejection and
// loopDone channel ensure safe concurrent calling.
func (l *Loop) Close() error {
	for {
		currentState := l.state.Load()
		if currentState == StateTerminated {
			return ErrLoopTerminated
		}

		// Another goroutine (Shutdown or Close) is already terminating.
		// Wait for it to complete rather than spinning on the CAS.
		if currentState == StateTerminating {
			<-l.loopDone
			return ErrLoopTerminated
		}

		if l.state.TryTransition(currentState, StateTerminating) {
			if currentState == StateAwake {
				// Loop is in Start() but hasn't entered run() yet
				l.state.Store(StateTerminated)
				l.closeFDs()

				// Close loopDone since Run() was never called and won't close it.
				// Without this, goroutines blocked on <-loopDone in
				// submitTimerRefChange/CancelTimer/CancelTimers would deadlock.
				l.closeLoopDoneOnce.Do(func() { close(l.loopDone) })

				// Wait for in-flight Promisify goroutines before returning.
				// Lock+Unlock acts as a memory barrier so any in-flight
				// Promisify goroutine that passed its initial state check
				// will observe StateTerminated on loopDone. Unlock before
				// Wait() so concurrent Promisify() calls can acquire the
				// mutex, see Terminated state, and reject immediately.
				l.promisifyMu.Lock()
				//lint:ignore SA2001 intentional memory barrier
				l.promisifyMu.Unlock()
				l.promisifyWg.Wait()

				l.terminateCleanup()
				l.registry.RejectAll(ErrLoopTerminated)

				return nil
			}

			// Loop is running (StateRunning or StateSleeping)
			// Set StateTerminated to trigger immediate exit (bypass draining queues)
			// IMPORTANT: Must do this BEFORE closing FDs, otherwise loop goroutine can't wake up
			l.state.Store(StateTerminated)
			// Wake up the loop so it sees the Terminated state and exits
			l.doWakeup()
			// Wait for the loop goroutine to exit
			<-l.loopDone

			// Wait for in-flight Promisify goroutines before cleanup
			l.promisifyMu.Lock()
			//lint:ignore SA2001 intentional memory barrier
			l.promisifyMu.Unlock()
			l.promisifyWg.Wait()

			l.terminateCleanup()
			l.registry.RejectAll(ErrLoopTerminated)

			return nil
		}
	}
}
