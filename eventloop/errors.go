package eventloop

import (
	"errors"
	"fmt"
	"reflect"
)

// Standard errors.
var (
	// ErrLoopAlreadyRunning is returned when Run() is called on a loop that is already running.
	ErrLoopAlreadyRunning = errors.New("eventloop: loop is already running")

	// ErrLoopTerminated is returned when operations are attempted on a terminated loop.
	ErrLoopTerminated = errors.New("eventloop: loop has been terminated")

	// ErrReentrantRun is returned when Run() is called from within the loop itself.
	ErrReentrantRun = errors.New("eventloop: cannot call Run() from within the loop")

	// ErrReentrantClose is returned when Close() is called from within the loop
	// goroutine or from the goroutine that is draining accepted terminal callbacks.
	ErrReentrantClose = errors.New("eventloop: cannot call Close() from within the loop")

	// ErrFastPathIncompatible is returned when fast path mode is forced but I/O FDs are registered.
	ErrFastPathIncompatible = errors.New("eventloop: fast path incompatible with registered I/O FDs")

	// ErrTimerNotFound is returned when attempting to cancel a timer that does not exist.
	ErrTimerNotFound = errors.New("eventloop: timer not found")

	// ErrTimerIDExhausted is returned when a timer handle namespace has no
	// remaining non-zero identifier.
	ErrTimerIDExhausted = errors.New("eventloop: timer ID exhausted")
)

type terminalErrorBox struct {
	err error
}

func (l *Loop) storeTerminalError(err error) {
	if err != nil {
		l.terminalErr.Store(&terminalErrorBox{err: err})
	}
}

func (l *Loop) terminalError() error {
	var terminalErr error
	value := l.terminalErr.Load()
	if box, ok := value.(*terminalErrorBox); ok && box != nil {
		terminalErr = box.err
	}
	return joinErrors(terminalErr, l.fdResourceCloseError())
}

func joinErrors(primary, secondary error) error {
	if primary == nil {
		return secondary
	}
	if secondary == nil {
		return primary
	}
	return errors.Join(primary, secondary)
}

func (l *Loop) fdResourceCloseError() error {
	value := l.fdCloseErr.Load()
	if box, ok := value.(*terminalErrorBox); ok && box != nil {
		return box.err
	}
	return nil
}

func nonNilError(value any) error {
	err, ok := value.(error)
	if !ok || err == nil {
		return nil
	}
	reflected := reflect.ValueOf(err)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if reflected.IsNil() {
			return nil
		}
	}
	return err
}

// PanicError wraps a panic value recovered from a Go promise callback.
type PanicError struct {
	// Value is the recovered panic value (may be any type, including error).
	Value any
}

// Error implements the error interface.
func (e PanicError) Error() string {
	return fmt.Sprintf("eventloop: promise callback panicked: %v", e.Value)
}

// Unwrap returns the underlying error if the panic value is an error type.
// This enables use with [errors.Is] and [errors.As] for error matching
// through the cause chain.
//
// If the panic Value is not an error (e.g., a string or other type),
// returns nil.
//
// Example:
//
//	// If a function panics with an error
//	panicErr := PanicError{Value: io.EOF}
//
//	// We can check if it wraps a specific error
//	if errors.Is(panicErr, io.EOF) {
//	    // This will match
//	}
func (e PanicError) Unwrap() error {
	return nonNilError(e.Value)
}

// Is implements custom error matching for PanicError.
// Returns true if target is a PanicError (regardless of value).
func (e PanicError) Is(target error) bool {
	_, ok := target.(PanicError)
	if !ok {
		// Also match pointer form
		pointer, pointerOK := target.(*PanicError)
		ok = pointerOK && pointer != nil
	}
	return ok
}

// AggregateError is the rejection reason used when [JS.Any] receives only
// rejected inputs.
//
// The Errors field contains the rejection reasons from all failed promises,
// preserving the order of the input promises array.
//
// Example:
//
//	promise := js.Any(
//	    js.Reject(errors.New("error 1")),
//	    js.Reject(errors.New("error 2")),
//	)
//	promise.Catch(func(r any) any {
//	    if agg, ok := r.(*AggregateError); ok {
//	        fmt.Printf("All failed. Errors:\n")
//	        for i, err := range agg.Errors {
//	            fmt.Printf("  [%d] %v\n", i, err)
//	        }
//	    }
//	    return nil
//	})
type AggregateError struct {
	// Message matches standard JS AggregateError property
	Message string
	// Errors contains all rejection reasons from failed promises.
	// The order matches the input promises array to [JS.Any].
	Errors []any
}

// Error implements the error interface.
// Returns "All promises were rejected" as a generic message.
// Individual rejection reasons can be accessed via the [Errors] field.
func (e *AggregateError) Error() string {
	if e != nil && e.Message != "" {
		return e.Message
	}
	return "All promises were rejected"
}

// NilPromiseError reports a nil promise in a combinator input.
type NilPromiseError struct {
	// Index identifies the zero-based input position.
	Index int
}

// Error implements error.
func (e *NilPromiseError) Error() string {
	if e == nil {
		return "eventloop: nil promise"
	}
	return fmt.Sprintf("eventloop: nil promise at index %d", e.Index)
}

// Unwrap returns the errors slice for multi-error unwrapping (Go 1.20+).
// This enables [errors.Is] and [errors.As] to check against all errors
// in the aggregate.
//
// Example:
//
//	aggErr := &AggregateError{
//	    Errors: []any{io.EOF, io.ErrUnexpectedEOF},
//	}
//
//	// Both of these will return true:
//	errors.Is(aggErr, io.EOF)
//	errors.Is(aggErr, io.ErrUnexpectedEOF)
func (e *AggregateError) Unwrap() []error {
	if e == nil {
		return nil
	}
	errs := make([]error, 0, len(e.Errors))
	for _, err := range e.Errors {
		if err := nonNilError(err); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// Is implements custom error matching for AggregateError.
// Returns true if target is an *AggregateError (regardless of contents).
func (e *AggregateError) Is(target error) bool {
	match, ok := target.(*AggregateError)
	return ok && match != nil
}

// Is implements custom error matching for NilPromiseError.
// Returns true if target is a *NilPromiseError, regardless of input index.
func (e *NilPromiseError) Is(target error) bool {
	match, ok := target.(*NilPromiseError)
	return ok && match != nil
}

// TimeoutError represents an operation timeout. It is used by promise timeout
// helpers and as the exact reason published by [AbortTimeout].
type TimeoutError struct {
	// Cause is the underlying error that triggered this timeout, if any.
	Cause error
	// Message describes the timeout. If empty, defaults to "operation timed out".
	Message string
}

// Error implements the error interface.
func (e *TimeoutError) Error() string {
	if e == nil || e.Message == "" {
		return "operation timed out"
	}
	return e.Message
}

// Unwrap returns the underlying cause for use with [errors.Is] and [errors.As].
func (e *TimeoutError) Unwrap() error {
	if e == nil {
		return nil
	}
	return nonNilError(e.Cause)
}

// Is implements custom error matching for TimeoutError.
// Returns true if target is a *TimeoutError (regardless of message or cause).
func (e *TimeoutError) Is(target error) bool {
	match, ok := target.(*TimeoutError)
	return ok && match != nil
}
