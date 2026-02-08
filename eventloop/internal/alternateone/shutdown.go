package alternateone

import (
	"context"
	"log"
	"sync"
)

// shutdownPhase represents a discrete phase in the shutdown sequence.
type shutdownPhase struct {
	fn   func()
	name string
}

// ShutdownManager handles the serial shutdown of the event loop.
//
// SAFETY: Executes shutdown phases sequentially with explicit logging.
// Each phase completes before the next begins.
type ShutdownManager struct {
	loop   *Loop
	logger func(format string, args ...any)
	mu     sync.Mutex
}

// NewShutdownManager creates a new shutdown manager.
func NewShutdownManager(loop *Loop) *ShutdownManager {
	return &ShutdownManager{
		loop:   loop,
		logger: log.Printf,
	}
}

// SetLogger sets a custom logger for shutdown phase logging.
func (sm *ShutdownManager) SetLogger(logger func(format string, args ...any)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.logger = logger
}

// Execute runs the complete shutdown sequence.
//
// SAFETY: Serial execution with explicit phase markers.
// Each phase is logged for debugging.
func (sm *ShutdownManager) Execute(ctx context.Context) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	phases := []shutdownPhase{
		{name: "ingress", fn: sm.drainIngress},
		{name: "internal", fn: sm.drainInternal},
		{name: "microtasks", fn: sm.drainMicrotasks},
		{name: "timers", fn: sm.cancelTimers},
		{name: "promises", fn: sm.rejectPromises},
		{name: "fds", fn: sm.closeFDs},
	}

	for _, phase := range phases {
		// Check context for cancellation
		select {
		case <-ctx.Done():
			sm.logPhase(phase.name, "cancelled")
			return ctx.Err()
		default:
		}

		sm.logPhase(phase.name, "start")
		phase.fn()
		sm.logPhase(phase.name, "complete")
	}

	return nil
}

// logPhase logs a shutdown phase transition.
func (sm *ShutdownManager) logPhase(phase, status string) {
	if sm.logger != nil {
		sm.logger("alternateone: shutdown phase=%s status=%s", phase, status)
	}
}

// drainIngress drains all tasks from the ingress queue.
//
// SAFETY: After draining, closes the ingress while still holding the lock.
// This prevents the TOCTOU race where Submit() passes the state check
// (before StateTerminating is set) but pushes AFTER drainIngress has
// finished draining. Without this atomic drain-and-close, such tasks
// would be accepted (Push returns nil) but never executed, violating
// the shutdown conservation invariant.
func (sm *ShutdownManager) drainIngress() {
	// Hold lock continuously while draining
	sm.loop.ingress.Lock()
	defer sm.loop.ingress.Unlock()
	for {
		task, ok := sm.loop.ingress.popLocked()
		if !ok {
			break
		}
		// Execute task while holding lock
		sm.loop.safeExecute(task)
	}
	// Close ingress atomically with the drain — any Push that acquires the
	// lock after this point will see closed=true and return ErrLoopTerminated.
	sm.loop.ingress.closed = true
}

// drainInternal drains all tasks from the internal queue.
func (sm *ShutdownManager) drainInternal() {
	for {
		task, ok := sm.loop.ingress.PopInternal()
		if !ok {
			break
		}
		sm.loop.safeExecute(task)
	}
}

// drainMicrotasks drains all microtasks.
func (sm *ShutdownManager) drainMicrotasks() {
	for {
		task, ok := sm.loop.ingress.PopMicrotask()
		if !ok {
			break
		}
		sm.loop.safeExecute(task)
	}
}

// cancelTimers cancels all pending timers.
func (sm *ShutdownManager) cancelTimers() {
	// Clear timers
	sm.loop.timersMu.Lock()
	sm.loop.timers = nil
	sm.loop.timersMu.Unlock()
}

// rejectPromises rejects all pending promises.
func (sm *ShutdownManager) rejectPromises() {
	// In a full implementation, this would reject all pending promises
	// For now, this is a placeholder
}

// closeFDs closes all file descriptors.
func (sm *ShutdownManager) closeFDs() {
	sm.loop.closeFDs()
}
