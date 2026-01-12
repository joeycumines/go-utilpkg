package alternatetwo

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Standard errors.
var (
	ErrLoopAlreadyRunning = errors.New("alternatetwo: loop is already running")
	ErrLoopTerminated     = errors.New("alternatetwo: loop has been terminated")
	ErrLoopNotRunning     = errors.New("alternatetwo: loop is not running")
	ErrReentrantRun       = errors.New("alternatetwo: cannot call Run() from within the loop")
)

// Loop is the "Maximum Performance" event loop implementation.
//
// PERFORMANCE: Prioritizes throughput and low latency:
//   - Lock-free ingress queue
//   - Direct FD indexing (no map lookups)
//   - Minimal chunk clearing
//   - Inline callback execution
//   - Arena allocation for tasks
//   - Cache-line padding for hot fields
type Loop struct { // betteralign:ignore
	// Prevent copying
	_ [0]func()

	// State machine (cache-line padded internally)
	state *FastState

	// Ingress queues
	external   *LockFreeIngress // External tasks
	internal   *LockFreeIngress // Internal priority tasks
	microtasks *MicrotaskRing   // Microtask ring buffer

	// I/O poller
	poller FastPoller

	// Synchronization
	stopOnce sync.Once

	// Wake-up mechanism
	wakePipe      int
	wakePipeWrite int
	wakeBuf       [8]byte
	wakePending   atomic.Uint32

	// Timing
	tickAnchor     time.Time
	tickTimeOffset atomic.Int64

	// Goroutine tracking
	loopGoroutineID atomic.Uint64
	tickCount       atomic.Uint64

	// Loop ID
	id uint64

	// Loop termination signaling
	loopDone chan struct{}

	// Task batch buffer (avoid allocation)
	batchBuf [256]Task
}

var loopIDCounter atomic.Uint64

// New creates a new performance-first event loop.
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
	// The channel is created in New() to avoid a data race with shutdownImpl
	defer close(l.loopDone)

	// Initialize timing anchor
	l.tickAnchor = time.Now()

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

	for {
		// Early termination check
		if l.state.Load() == StateTerminating || l.state.Load() == StateTerminated {
			l.shutdown()
			return nil
		}

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
					// Wake up if we were sleeping
					if current == StateSleeping {
						_ = l.submitWakeup()
					}
					break
				}
			}
			return ctx.Err()
		default:
		}

		l.tick()
	}
}

// shutdown performs the shutdown sequence.
func (l *Loop) shutdown() {
	// Drain all queues
	for {
		task, ok := l.internal.Pop()
		if !ok {
			break
		}
		l.safeExecute(task.Fn)
	}

	for {
		task, ok := l.external.Pop()
		if !ok {
			break
		}
		l.safeExecute(task.Fn)
	}

	for {
		fn := l.microtasks.Pop()
		if fn == nil {
			break
		}
		l.safeExecute(fn)
	}

	l.state.Store(StateTerminated)
	l.closeFDs()
}

// tick is a single iteration of the event loop.
func (l *Loop) tick() {
	l.tickCount.Add(1)
	l.tickTimeOffset.Store(int64(time.Since(l.tickAnchor)))

	// Process internal tasks (priority)
	l.processInternal()

	// Process external tasks with budget
	l.processExternal()

	// Process microtasks
	l.processMicrotasks()

	// Poll for I/O
	l.poll()

	// Final microtask pass
	l.processMicrotasks()
}

// processInternal processes internal priority tasks.
func (l *Loop) processInternal() {
	for {
		task, ok := l.internal.Pop()
		if !ok {
			break
		}
		l.safeExecute(task.Fn)
	}
}

// processExternal processes external tasks with budget.
func (l *Loop) processExternal() {
	const budget = 1024

	// PERFORMANCE: Batch pop for better cache behavior
	n := l.external.PopBatch(l.batchBuf[:], budget)
	for i := 0; i < n; i++ {
		l.safeExecute(l.batchBuf[i].Fn)
		l.batchBuf[i] = Task{} // Clear for GC
	}
}

// processMicrotasks drains the microtask queue.
func (l *Loop) processMicrotasks() {
	const budget = 1024

	for i := 0; i < budget; i++ {
		fn := l.microtasks.Pop()
		if fn == nil {
			break
		}
		l.safeExecute(fn)
	}
}

// poll performs the blocking poll.
func (l *Loop) poll() {
	currentState := l.state.Load()
	if currentState != StateRunning {
		return
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
	// Critical: Return immediately if terminating was set
	if l.state.Load() == StateTerminating {
		return
	}

	// Use shorter timeout to allow quick response to termination
	// This ensures poll() returns quickly for context cancellation
	timeout := 10

	_, err := l.poller.PollIO(timeout)
	if err != nil {
		l.state.TryTransition(StateSleeping, StateTerminating)
		return
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
func (l *Loop) submitWakeup() error {
	// PERFORMANCE: Native endianness, no binary.LittleEndian overhead
	var one uint64 = 1
	buf := (*[8]byte)(unsafe.Pointer(&one))[:]

	_, err := unix.Write(l.wakePipeWrite, buf)
	return err
}

// Submit submits a task to the external queue.
func (l *Loop) Submit(fn func()) error {
	state := l.state.Load()
	if state == StateTerminating || state == StateTerminated {
		return ErrLoopTerminated
	}

	l.external.Push(fn)

	// Wake if sleeping
	if l.state.Load() == StateSleeping {
		if l.wakePending.CompareAndSwap(0, 1) {
			_ = l.submitWakeup()
		}
	}

	return nil
}

// SubmitInternal submits a task to the internal priority queue.
func (l *Loop) SubmitInternal(fn func()) error {
	state := l.state.Load()
	if state == StateTerminated {
		return ErrLoopTerminated
	}

	l.internal.Push(fn)

	if l.state.Load() == StateSleeping {
		if l.wakePending.CompareAndSwap(0, 1) {
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

	if !l.microtasks.Push(fn) {
		return errors.New("alternatetwo: microtask buffer full")
	}

	return nil
}

// RegisterFD registers a file descriptor for I/O monitoring.
func (l *Loop) RegisterFD(fd int, events IOEvents, callback IOCallback) error {
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

// safeExecute executes a function with panic recovery.
func (l *Loop) safeExecute(fn func()) {
	if fn == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			// PERFORMANCE: Minimal panic handling, just recover
			// In production, consider logging
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

// State returns the current loop state.
func (l *Loop) State() LoopState {
	return l.state.Load()
}

// Close immediately terminates the event loop without waiting for graceful shutdown.
//
// Close implements io.Closer. It transitions to Terminating immediately,
// closes file descriptors, and allows the loop to exit as soon as possible.
//
// If the loop is already terminated, Close() returns ErrLoopTerminated.
func (l *Loop) Close() error {
	for {
		currentState := l.state.Load()
		if currentState == StateTerminated {
			return ErrLoopTerminated
		}

		if l.state.TryTransition(currentState, StateTerminating) {
			if currentState == StateAwake {
				// Never started - fully terminate now
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
