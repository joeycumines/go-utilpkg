package eventloop

import "reflect"

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
