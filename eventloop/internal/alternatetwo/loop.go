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
	done     chan struct{}
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
		done:       make(chan struct{}),

		wakePipe:      wakeFd,
		wakePipeWrite: wakeWriteFd,
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

// Start begins running the event loop in a new goroutine.
func (l *Loop) Start(ctx context.Context) error {
	if l.isLoopThread() {
		return errors.New("alternatetwo: cannot call Start() from within the loop")
	}

	if !l.state.TryTransition(StateAwake, StateRunning) {
		currentState := l.state.Load()
		if currentState == StateTerminated {
			return ErrLoopTerminated
		}
		return ErrLoopAlreadyRunning
	}

	// Initialize timing anchor
	l.tickAnchor = time.Now()

	go l.run(ctx)
	return nil
}

// Stop gracefully shuts down the event loop.
func (l *Loop) Stop(ctx context.Context) error {
	var result error
	l.stopOnce.Do(func() {
		result = l.stopImpl(ctx)
	})
	if result == nil && l.state.Load() != StateTerminated {
		return ErrLoopTerminated
	}
	return result
}

// stopImpl performs the actual stop operation.
func (l *Loop) stopImpl(ctx context.Context) error {
	for {
		currentState := l.state.Load()
		if currentState == StateTerminated || currentState == StateTerminating {
			return ErrLoopTerminated
		}

		if l.state.TryTransition(currentState, StateTerminating) {
			if currentState == StateAwake {
				l.state.Store(StateTerminated)
				l.closeFDs()
				close(l.done)
				return nil
			}

			if currentState == StateSleeping {
				_ = l.submitWakeup()
			}
			break
		}
	}

	select {
	case <-l.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// run is the main loop goroutine.
func (l *Loop) run(ctx context.Context) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	l.loopGoroutineID.Store(getGoroutineID())
	defer l.loopGoroutineID.Store(0)

	for {
		select {
		case <-ctx.Done():
			current := l.state.Load()
			if current != StateTerminating && current != StateTerminated {
				for {
					if l.state.TryTransition(current, StateTerminating) {
						break
					}
					current = l.state.Load()
					if current == StateTerminating || current == StateTerminated {
						break
					}
				}
			}
		default:
		}

		if l.state.Load() == StateTerminating {
			l.shutdown()
			return
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
	close(l.done)
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

	// Default timeout: 10 seconds
	timeout := 10000

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

// Done returns a channel that is closed when the loop terminates.
func (l *Loop) Done() <-chan struct{} {
	return l.done
}

// State returns the current loop state.
func (l *Loop) State() LoopState {
	return l.state.Load()
}
