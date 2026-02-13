package eventloop

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
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}

// Is implements custom error matching for PanicError.
// Returns true if target is a PanicError (regardless of value).
func (e PanicError) Is(target error) bool {
	_, ok := target.(PanicError)
	if !ok {
		// Also match pointer form
		_, ok = target.(*PanicError)
	}
	return ok
}

// Cause returns the first error in the Errors slice, if any.
// This is provided for ES2022 .cause compatibility where you might want
// to access a primary underlying cause.
//
// Returns nil if Errors is empty.
func (e *AggregateError) Cause() error {
	if len(e.Errors) > 0 {
		return e.Errors[0]
	}
	return nil
}

// Unwrap returns the errors slice for multi-error unwrapping (Go 1.20+).
// This enables [errors.Is] and [errors.As] to check against all errors
// in the aggregate.
//
// Example:
//
//	aggErr := &AggregateError{
//	    Errors: []error{io.EOF, io.ErrUnexpectedEOF},
//	}
//
//	// Both of these will return true:
//	errors.Is(aggErr, io.EOF)
//	errors.Is(aggErr, io.ErrUnexpectedEOF)
func (e *AggregateError) Unwrap() []error {
	return e.Errors
}

// Is implements custom error matching for AggregateError.
// Returns true if target is an *AggregateError (regardless of contents).
func (e *AggregateError) Is(target error) bool {
	_, ok := target.(*AggregateError)
	return ok
}

// TypeError represents a type error, similar to JavaScript's TypeError.
// This is used when a value is not of the expected type.
type TypeError struct {
	// Cause is the underlying error that triggered this type error, if any.
	Cause error
	// Message describes the type error. If empty, defaults to "type error".
	Message string
}

// Error implements the error interface.
func (e *TypeError) Error() string {
	if e.Message == "" {
		return "type error"
	}
	return e.Message
}

// Unwrap returns the underlying cause for use with [errors.Is] and [errors.As].
func (e *TypeError) Unwrap() error {
	return e.Cause
}

// Is implements custom error matching for TypeError.
// Returns true if target is a *TypeError (regardless of message or cause).
func (e *TypeError) Is(target error) bool {
	_, ok := target.(*TypeError)
	return ok
}

// RangeError represents a range error, similar to JavaScript's RangeError.
// This is used when a value is not within the expected range.
type RangeError struct {
	// Cause is the underlying error that triggered this range error, if any.
	Cause error
	// Message describes the range error. If empty, defaults to "range error".
	Message string
}

// Error implements the error interface.
func (e *RangeError) Error() string {
	if e.Message == "" {
		return "range error"
	}
	return e.Message
}

// Unwrap returns the underlying cause for use with [errors.Is] and [errors.As].
func (e *RangeError) Unwrap() error {
	return e.Cause
}

// Is implements custom error matching for RangeError.
// Returns true if target is a *RangeError (regardless of message or cause).
func (e *RangeError) Is(target error) bool {
	_, ok := target.(*RangeError)
	return ok
}

// TimeoutError represents a timeout error for promise timeouts.
// This is used when an operation times out.
type TimeoutError struct {
	// Cause is the underlying error that triggered this timeout, if any.
	Cause error
	// Message describes the timeout. If empty, defaults to "operation timed out".
	Message string
}

// Error implements the error interface.
func (e *TimeoutError) Error() string {
	if e.Message == "" {
		return "operation timed out"
	}
	return e.Message
}

// Unwrap returns the underlying cause for use with [errors.Is] and [errors.As].
func (e *TimeoutError) Unwrap() error {
	return e.Cause
}

// Is implements custom error matching for TimeoutError.
// Returns true if target is a *TimeoutError (regardless of message or cause).
func (e *TimeoutError) Is(target error) bool {
	_, ok := target.(*TimeoutError)
	return ok
}
