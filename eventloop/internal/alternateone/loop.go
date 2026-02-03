//go:build linux || darwin

package alternateone

import (
	"container/heap"
	"context"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Loop is the "Maximum Safety" event loop implementation.
//
// This implementation prioritizes correctness over performance:
//   - Single lock architecture for ingress subsystem
//   - Full-clear always for chunk management
//   - Strict state transition validation
//   - Conservative check-then-sleep protocol
//   - Write lock for poll operations
//   - Serial shutdown phases
//   - Comprehensive error handling
//
// # Thread Safety
//
// The Loop is thread-safe. External callers use Submit/SubmitInternal/ScheduleTimer.
// The loop goroutine owns exclusive access to most internal state.
type Loop struct { // betteralign:ignore
	// No copying allowed
	_ [0]func()

	// tickAnchor is the immutable start time of the loop.
	// It holds the monotonic clock reference. Read-only after Start(), so thread-safe.
	tickAnchor time.Time

	// Pointer-sized fields (8 bytes each on 64-bit)
	state       *SafeStateMachine // State machine with strict validation
	ingress     *SafeIngress      // Single-lock ingress queue
	shutdownMgr *ShutdownManager  // Shutdown management
	OnError     func(*LoopError)  // Error handler (optional)
	OnPanic     func(*PanicError) // Panic handler (optional)

	// loopDone signals loop termination
	loopDone chan struct{}

	// Timers (slice: 24 bytes)
	timers timerHeap

	// I/O poller (embedded struct)
	poller SafePoller

	// Atomic fields
	tickTimeOffset  atomic.Int64
	loopGoroutineID atomic.Uint64
	tickCount       atomic.Uint64

	// Int fields
	wakePipe      int
	wakePipeWrite int

	// Loop ID for debugging
	id uint64

	// Mutexes together
	shutOnce sync.Once
	timersMu sync.Mutex

	// 4-byte atomic and 4 bytes padding
	wakeUpPending atomic.Uint32

	// 8-byte array
	wakeBuf [8]byte
}

// timer represents a scheduled task
type timer struct {
	when time.Time
	task SafeTask
}

// timerHeap is a min-heap of timers
type timerHeap []timer

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

// New creates a new safety-first event loop.
func New() (*Loop, error) {
	return NewWithObserver(nil)
}

// NewWithObserver creates a new event loop with a state observer.
func NewWithObserver(observer StateObserver) (*Loop, error) {
	// Create wake-up mechanism
	wakeFd, wakeWriteFd, err := createWakeFd(0, EFD_CLOEXEC|EFD_NONBLOCK)
	if err != nil {
		return nil, WrapError("New", err)
	}

	loop := &Loop{
		id:      loopIDCounter.Add(1),
		state:   NewSafeStateMachine(observer),
		ingress: NewSafeIngress(),
		timers:  make(timerHeap, 0),

		wakePipe:      wakeFd,
		wakePipeWrite: wakeWriteFd,

		// Initialize loopDone here to avoid data race with shutdownImpl
		loopDone: make(chan struct{}),
	}

	// Initialize poller
	if err := loop.poller.initPoller(); err != nil {
		_ = unix.Close(wakeFd)
		if wakeWriteFd != wakeFd {
			_ = unix.Close(wakeWriteFd)
		}
		return nil, WrapError("New", err)
	}

	// Register wake pipe for read events
	if err := loop.poller.RegisterFD(wakeFd, EventRead, func(IOEvents) {
		loop.drainWakeUpPipe()
	}); err != nil {
		_ = loop.poller.closePoller()
		_ = unix.Close(wakeFd)
		if wakeWriteFd != wakeFd {
			_ = unix.Close(wakeWriteFd)
		}
		return nil, WrapError("New", err)
	}

	// Initialize shutdown manager
	loop.shutdownMgr = NewShutdownManager(loop)
	//d

	return loop, nil
}

// Run begins running the event loop and blocks until the loop is fully stopped.
//
// Run returns only when:
// 1. The context is canceled (initiates shutdown)
// 2. Shutdown() is called (graceful shutdown)
// 3. Close() is called (immediate termination)
//
// SAFETY: Uses strict state transition validation.
// Returns error on invalid transition or context cancellation.
func (l *Loop) Run(ctx context.Context) error {
	// Re-entrancy check
	if l.isLoopThread() {
		return ErrReentrantRun
	}

	// SAFETY: Strict transition validation - will panic on invalid transition
	if !l.state.Transition(StateAwake, StateRunning) {
		currentState := l.state.Load()
		if currentState == StateTerminated {
			return ErrLoopTerminated
		}
		return ErrLoopAlreadyRunning
	}

	// Close loopDone when run exits to signal completion to Shutdown waiters
	// The channel is created in New() to avoid a data race with shutdownImpl
	defer close(l.loopDone)

	// Initialize monotonic anchor
	l.tickAnchor = time.Now()

	// Run the loop directly (blocking, NOT in goroutine)
	return l.run(ctx)
}

// Shutdown gracefully shuts down the event loop.
//
// Shutdown initiates a graceful shutdown that:
// 1. Rejects new submissions (Submit returns ErrLoopTerminated)
// 2. Processes all queued tasks
// 3. Drains FD registrations
// 4. Closes file descriptors
// 5. Blocks until termination completes
//
// Shutdown is idempotent and safe to call multiple times or concurrently with Close.
//
// SAFETY: Uses sync.Once for idempotence.
func (l *Loop) Shutdown(ctx context.Context) error {
	var result error
	l.shutOnce.Do(func() {
		result = l.shutdownImpl(ctx)
	})
	if result == nil && l.state.Load() != StateTerminated {
		return ErrLoopTerminated // Subsequent callers
	}
	return result
}

// shutdownImpl contains the actual Shutdown implementation.
func (l *Loop) shutdownImpl(ctx context.Context) error {
	// Attempt to transition to Terminating
	for {
		currentState := l.state.Load()
		if currentState == StateTerminated || currentState == StateTerminating {
			return ErrLoopTerminated
		}

		// SAFETY: Will panic on invalid transition
		if l.state.Transition(currentState, StateTerminating) {
			// If never started, clean up directly
			if currentState == StateAwake {
				l.state.ForceTerminated()
				l.closeFDs()
				return nil
			}

			// Wake up if sleeping
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

// run is the main loop. It blocks until termination.
func (l *Loop) run(ctx context.Context) error {
	// Pin to OS thread
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Store goroutine ID for re-entrancy detection
	l.loopGoroutineID.Store(getGoroutineID())
	defer l.loopGoroutineID.Store(0)

	for {
		// Early termination check
		state := l.state.Load()
		if state == StateTerminating || state == StateTerminated {
			if state == StateTerminated {
				return nil
			}
			l.shutdown(ctx)
			return nil
		}

		// Check context
		select {
		case <-ctx.Done():
			current := l.state.Load()
			if current != StateTerminating && current != StateTerminated {
				// Force transition to terminating
				for {
					if l.state.Transition(current, StateTerminating) {
						break
					}
					current = l.state.Load()
					if current == StateTerminating || current == StateTerminated {
						break
					}
				}
				return ctx.Err()
			}
		default:
		}

		l.tick(ctx)
	}
}

// shutdown executes the shutdown sequence.
//
// SAFETY: Serial shutdown phases with logging.
func (l *Loop) shutdown(ctx context.Context) {
	if err := l.shutdownMgr.Execute(ctx); err != nil {
		log.Printf("alternateone: shutdown error: %v", err)
	}

	// Set final state
	l.state.ForceTerminated()

	// Close ingress (if not already closed by Close())
	if !l.ingress.IsClosed() {
		l.ingress.Close()
	}
}

// tick is a single iteration of the event loop.
func (l *Loop) tick(ctx context.Context) {
	// Check for termination early
	if l.state.Load() == StateTerminating || l.state.Load() == StateTerminated {
		return
	}

	l.tickCount.Add(1)

	// Update tick time
	l.tickTimeOffset.Store(int64(time.Since(l.tickAnchor)))

	// Run expired timers
	l.runTimers()

	// Check for termination
	if l.state.Load() == StateTerminating || l.state.Load() == StateTerminated {
		return
	}

	// Process internal tasks (priority)
	l.processInternal()

	// Check for termination
	if l.state.Load() == StateTerminating || l.state.Load() == StateTerminated {
		return
	}

	// Process external tasks
	l.processExternal(ctx)

	// Check for termination
	if l.state.Load() == StateTerminating || l.state.Load() == StateTerminated {
		return
	}

	// Process microtasks
	l.processMicrotasks()

	// Check for termination
	if l.state.Load() == StateTerminating || l.state.Load() == StateTerminated {
		return
	}

	// Poll for I/O with Check-Then-Sleep protocol
	l.poll(ctx)

	// Check for termination
	if l.state.Load() == StateTerminating || l.state.Load() == StateTerminated {
		return
	}

	// Final microtask pass
	l.processMicrotasks()
}

// runTimers executes expired timers.
func (l *Loop) runTimers() {
	now := l.CurrentTickTime()

	l.timersMu.Lock()
	for len(l.timers) > 0 {
		// Check for termination between timer executions
		if l.state.Load() == StateTerminating {
			l.timersMu.Unlock()
			return
		}

		if l.timers[0].when.After(now) {
			break
		}
		t := heap.Pop(&l.timers).(timer)
		l.timersMu.Unlock()

		l.safeExecute(t.task)

		l.timersMu.Lock()
	}
	l.timersMu.Unlock()
}

// processInternal processes internal priority tasks.
func (l *Loop) processInternal() {
	for {
		task, ok := l.ingress.PopInternal()
		if !ok {
			break
		}
		l.safeExecute(task)
	}
}

// processExternal processes external tasks with budget.
func (l *Loop) processExternal(ctx context.Context) {
	const budget = 1024

	for i := 0; i < budget; i++ {
		task, ok := l.ingress.PopExternal()
		if !ok {
			break
		}
		l.safeExecute(task)
	}
}

// processMicrotasks drains the microtask queue.
func (l *Loop) processMicrotasks() {
	const budget = 1024

	for i := 0; i < budget; i++ {
		task, ok := l.ingress.PopMicrotask()
		if !ok {
			break
		}
		l.safeExecute(task)
	}
}

// poll performs the blocking poll with conservative Check-Then-Sleep protocol.
//
// SAFETY: Holds lock through sleep decision.
func (l *Loop) poll(ctx context.Context) {
	// SAFETY: Transition to sleeping using strict validation
	currentState := l.state.Load()
	if currentState != StateRunning {
		return
	}

	if !l.state.Transition(StateRunning, StateSleeping) {
		return
	}

	// SAFETY: Hold lock through sleep decision
	l.ingress.Lock()
	queueLen := l.ingress.Length()
	l.ingress.Unlock()

	if queueLen > 0 {
		// Work pending, abort sleep
		l.state.Transition(StateSleeping, StateRunning)
		return
	}

	// Calculate timeout
	timeout := l.calculateTimeout()

	// Poll for I/O (write lock for safety)
	_, err := l.poller.PollIO(timeout)
	if err != nil {
		log.Printf("alternateone: pollIO error: %v", err)
	}

	// Check for termination (may have been called while polling)
	if l.state.Load() == StateTerminating || l.state.Load() == StateTerminated {
		return
	}

	// Wake up
	l.state.Transition(StateSleeping, StateRunning)
}

// calculateTimeout determines the poll timeout.
func (l *Loop) calculateTimeout() int {
	maxDelay := 10 * time.Second

	l.timersMu.Lock()
	if len(l.timers) > 0 {
		delay := time.Until(l.timers[0].when)
		if delay < 0 {
			delay = 0
		}
		if delay < maxDelay {
			maxDelay = delay
		}
	}
	l.timersMu.Unlock()

	// Ceiling rounding
	if maxDelay > 0 && maxDelay < time.Millisecond {
		return 1
	}

	return int(maxDelay.Milliseconds())
}

// drainWakeUpPipe drains the wake-up pipe.
func (l *Loop) drainWakeUpPipe() {
	for {
		_, err := unix.Read(l.wakePipe, l.wakeBuf[:])
		if err != nil {
			if err == unix.EAGAIN || err == unix.EINTR {
				break
			}
			break
		}
	}
	l.wakeUpPending.Store(0)
}

// submitWakeup writes to the wake-up pipe.
func (l *Loop) submitWakeup() error {
	// Safe cross-architecture write
	var one uint64 = 1
	buf := (*[8]byte)(unsafe.Pointer(&one))[:]

	_, err := unix.Write(l.wakePipeWrite, buf)
	return err
}

// Submit submits a task to the external queue.
//
// SAFETY: Returns error if loop is terminating/terminated.
func (l *Loop) Submit(fn func()) error {
	state := l.state.Load()
	if state == StateTerminating || state == StateTerminated {
		return ErrLoopTerminated
	}

	if err := l.ingress.Push(fn, LaneExternal); err != nil {
		return err
	}

	// Wake if sleeping
	if l.state.Load() == StateSleeping {
		if l.wakeUpPending.CompareAndSwap(0, 1) {
			_ = l.submitWakeup()
		}
	}

	return nil
}

// SubmitInternal submits a task to the internal priority queue.
//
// SAFETY: Accepts during terminating (for in-flight completions).
func (l *Loop) SubmitInternal(fn func()) error {
	state := l.state.Load()
	if state == StateTerminated {
		return ErrLoopTerminated
	}

	if err := l.ingress.Push(fn, LaneInternal); err != nil {
		return err
	}

	// Wake if sleeping
	if l.state.Load() == StateSleeping {
		if l.wakeUpPending.CompareAndSwap(0, 1) {
			_ = l.submitWakeup()
		}
	}

	return nil
}

// ScheduleMicrotask schedules a microtask.
func (l *Loop) ScheduleMicrotask(fn func()) error {
	state := l.state.Load()
	if state == StateTerminated {
		return ErrLoopTerminated
	}

	return l.ingress.Push(fn, LaneMicrotask)
}

// ScheduleTimer schedules a task after a delay.
func (l *Loop) ScheduleTimer(delay time.Duration, fn func()) error {
	when := time.Now().Add(delay)
	t := timer{
		when: when,
		task: SafeTask{Fn: fn},
	}

	return l.SubmitInternal(func() {
		l.timersMu.Lock()
		heap.Push(&l.timers, t)
		l.timersMu.Unlock()
	})
}

// RegisterFD registers a file descriptor for I/O monitoring.
func (l *Loop) RegisterFD(fd int, events IOEvents, callback func(IOEvents)) error {
	return l.poller.RegisterFD(fd, events, callback)
}

// UnregisterFD removes a file descriptor from monitoring.
func (l *Loop) UnregisterFD(fd int) error {
	return l.poller.UnregisterFD(fd)
}

// CurrentTickTime returns the cached time for the current tick.
func (l *Loop) CurrentTickTime() time.Time {
	offset := l.tickTimeOffset.Load()
	if offset == 0 && l.tickAnchor.IsZero() {
		return time.Now()
	}
	return l.tickAnchor.Add(time.Duration(offset))
}

// safeExecute wraps task execution with panic recovery.
//
// SAFETY: Full stack trace on panic.
func (l *Loop) safeExecute(t SafeTask) {
	if t.Fn == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			panicErr := NewPanicError(r, t.ID, l.id)
			log.Printf("alternateone: %v\n%s", panicErr.Error(), panicErr.StackTrace())
			if l.OnPanic != nil {
				l.OnPanic(panicErr)
			}
		}
	}()

	t.Fn()
}

// closeFDs closes file descriptors.
func (l *Loop) closeFDs() {
	_ = l.poller.closePoller()
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

// State returns the current loop state.
func (l *Loop) State() LoopState {
	return l.state.Load()
}

// Close immediately terminates the event loop.
//
// Close is NOT graceful. It:
// 1. Transitions to Terminating immediately
// 2. Wakes up loop if sleeping
// 3. Closes file descriptors without waiting for task completion
// 4. Returns ErrLoopTerminated if already terminated
//
// Close is safe to call multiple times or concurrently with Shutdown.
// Use Shutdown() for graceful termination instead.
//
// Close implements io.Closer semantics.
func (l *Loop) Close() error {
	// Idempotent: check current state first
	currentState := l.state.Load()
	if currentState == StateTerminated {
		return ErrLoopTerminated
	}

	// Attempt transition to Terminating (non-blocking)
	for {
		currentState = l.state.Load()
		if currentState == StateTerminated {
			return ErrLoopTerminated
		}
		if currentState == StateTerminating {
			// Already terminating, just close FDs immediately
			l.closeFDs()
			return nil
		}
		if l.state.Transition(currentState, StateTerminating) {
			// Wake up loop if sleeping to avoid deadlock
			if currentState == StateSleeping {
				_ = l.submitWakeup()
			}
			// Force immediate cleanup - close FDs immediately
			// Note: ingress.Close() is NOT called here to avoid lock contention
			// It will be closed as part of shutdown process when loop exits
			l.state.ForceTerminated()
			l.closeFDs()
			return nil
		}
	}
}
