package alternateone

import (
	"errors"
	"fmt"
	"runtime/debug"
	"time"
)

// Standard errors returned by the event loop.
var (
	// ErrLoopAlreadyRunning is returned when Run() is called on a loop that is already running.
	ErrLoopAlreadyRunning = errors.New("alternateone: loop is already running")

	// ErrLoopTerminated is returned when operations are attempted on a terminated loop.
	ErrLoopTerminated = errors.New("alternateone: loop has been terminated")

	// ErrLoopNotRunning is returned when operations are attempted on a loop that hasn't been started.
	ErrLoopNotRunning = errors.New("alternateone: loop is not running")

	// ErrPollerClosed is returned when poll operations are attempted on a closed poller.
	ErrPollerClosed = errors.New("alternateone: poller has been closed")

	// ErrInvalidTransition is returned when an invalid state transition is attempted.
	ErrInvalidTransition = errors.New("alternateone: invalid state transition")

	// ErrReentrantRun is returned when Run() is called from within the loop itself.
	ErrReentrantRun = errors.New("alternateone: cannot call Run() from within the loop")

	// ErrNilPromise is returned when a nil promise is registered.
	ErrNilPromise = errors.New("alternateone: nil promise registration")

	// ErrPromiseAlreadyRegistered is returned when a promise is registered twice.
	ErrPromiseAlreadyRegistered = errors.New("alternateone: promise already registered")

	// ErrFDAlreadyRegistered is returned when a file descriptor is already registered.
	ErrFDAlreadyRegistered = errors.New("alternateone: fd already registered")

	// ErrFDNotRegistered is returned when a file descriptor is not registered.
	ErrFDNotRegistered = errors.New("alternateone: fd not registered")

	// ErrPollerNotInitialized is returned when poller operations are attempted before initialization.
	ErrPollerNotInitialized = errors.New("alternateone: poller not initialized")
)

// LoopError represents an error that occurred during event loop operation.
// All errors include comprehensive context for debugging.
type LoopError struct {
	Cause   error          // Underlying error
	Context map[string]any // Additional context
	Op      string         // Operation that failed (e.g., "submit", "poll", "shutdown")
	Phase   string         // Phase of operation (e.g., "ingress", "microtasks")
}

// Error implements the error interface.
func (e *LoopError) Error() string {
	if e.Phase != "" {
		return fmt.Sprintf("alternateone: %s (phase=%s): %v", e.Op, e.Phase, e.Cause)
	}
	return fmt.Sprintf("alternateone: %s: %v", e.Op, e.Cause)
}

// Unwrap returns the underlying error.
func (e *LoopError) Unwrap() error {
	return e.Cause
}

// NewLoopError creates a new LoopError with the given parameters.
func NewLoopError(op, phase string, cause error, context map[string]any) *LoopError {
	return &LoopError{
		Op:      op,
		Phase:   phase,
		Cause:   cause,
		Context: context,
	}
}

// PanicError represents a panic that occurred during task execution.
// Includes full stack trace for debugging.
type PanicError struct {
	Value  any       // Panic value (16 bytes: type ptr, data ptr)
	Time   time.Time // When the panic occurred (24 bytes)
	Stack  []byte    // Stack trace (24 bytes: data ptr, len, cap)
	TaskID uint64    // ID of the task that panicked
	LoopID uint64    // ID of the loop
}

// Error implements the error interface.
func (e *PanicError) Error() string {
	return fmt.Sprintf("alternateone: task %d panicked: %v", e.TaskID, e.Value)
}

// StackTrace returns the formatted stack trace.
func (e *PanicError) StackTrace() string {
	return string(e.Stack)
}

// NewPanicError creates a new PanicError from a recovered panic value.
func NewPanicError(value any, taskID, loopID uint64) *PanicError {
	return &PanicError{
		Value:  value,
		Stack:  debug.Stack(),
		TaskID: taskID,
		LoopID: loopID,
		Time:   time.Now(),
	}
}

// TransitionError represents an invalid state transition attempt.
type TransitionError struct {
	From LoopState
	To   LoopState
}

// Error implements the error interface.
func (e *TransitionError) Error() string {
	return fmt.Sprintf("alternateone: invalid state transition: %v -> %v", e.From, e.To)
}

// WrapError wraps an error with operation context.
func WrapError(op string, err error) error {
	if err == nil {
		return nil
	}
	return NewLoopError(op, "", err, nil)
}

// WrapErrorWithPhase wraps an error with operation and phase context.
func WrapErrorWithPhase(op, phase string, err error) error {
	if err == nil {
		return nil
	}
	return NewLoopError(op, phase, err, nil)
}

// WrapErrorWithContext wraps an error with full context.
func WrapErrorWithContext(op, phase string, err error, ctx map[string]any) error {
	if err == nil {
		return nil
	}
	return NewLoopError(op, phase, err, ctx)
}
