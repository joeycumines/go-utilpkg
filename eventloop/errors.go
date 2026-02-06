//go:build linux || darwin

// Package eventloop provides ES2022-compatible error types with cause chain support.
package eventloop

import (
	"errors"
	"fmt"
)

// ErrorWithCause represents an error with an optional underlying cause (ES2022).
//
// This type mirrors JavaScript's Error object with a .cause property introduced
// in ES2022. It enables error chaining where high-level errors can wrap lower-level
// causes while maintaining separate error messages.
//
// The Unwrap method enables use with [errors.Is] and [errors.As] for error matching
// and type assertion through the cause chain.
//
// Example:
//
//	// Creating an error with a cause
//	cause := io.ErrUnexpectedEOF
//	err := &ErrorWithCause{
//	    Message: "failed to parse response",
//	    Cause:   cause,
//	}
//
//	// Checking error chain
//	if errors.Is(err, io.ErrUnexpectedEOF) {
//	    // This will match because Cause is io.ErrUnexpectedEOF
//	}
//
//	// Unwrapping to access cause
//	var ioErr *fs.PathError
//	if errors.As(err, &ioErr) {
//	    // Type assertion through cause chain
//	}
type ErrorWithCause struct {
	// Cause is the underlying error that caused this error.
	// Can be nil if there is no underlying cause.
	Cause error

	// Message is the error message describing what went wrong.
	Message string
}

// Error implements the error interface.
// Returns the Message field.
func (e *ErrorWithCause) Error() string {
	if e.Message == "" {
		if e.Cause != nil {
			return e.Cause.Error()
		}
		return "unknown error"
	}
	return e.Message
}

// Unwrap returns the underlying cause for use with [errors.Is] and [errors.As].
// This enables error chain traversal per Go 1.13+ error wrapping conventions.
func (e *ErrorWithCause) Unwrap() error {
	return e.Cause
}

// NewErrorWithCause creates a new ErrorWithCause with the given message and cause.
//
// This is a convenience constructor for creating errors with causes.
//
// Example:
//
//	err := eventloop.NewErrorWithCause("database connection failed", dbErr)
func NewErrorWithCause(message string, cause error) *ErrorWithCause {
	return &ErrorWithCause{
		Message: message,
		Cause:   cause,
	}
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
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}

// AggregateErrorCause returns the first error in the Errors slice, if any.
// This is provided for ES2022 .cause compatibility where you might want
// to access a primary underlying cause.
//
// Returns nil if Errors is empty.
func (e *AggregateError) AggregateErrorCause() error {
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
// Returns true if target is an AggregateError (regardless of contents)
// or if any of the contained errors match target.
func (e *AggregateError) Is(target error) bool {
	// Check if target is an AggregateError type
	var aggTarget *AggregateError
	return errors.As(target, &aggTarget)
}

// TypeError represents a type error, similar to JavaScript's TypeError.
// This is used when a value is not of the expected type.
type TypeError struct {
	Cause   error
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

// RangeError represents a range error, similar to JavaScript's RangeError.
// This is used when a value is not within the expected range.
type RangeError struct {
	Cause   error
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

// TimeoutError represents a timeout error for promise timeouts.
// This is used when an operation times out.
type TimeoutError struct {
	Cause   error
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

// WrapError wraps an error with a message and optional cause chain.
// This is a convenience function for creating wrapped errors with cause.
//
// If the original error should be the cause, pass it as both arguments:
//
//	WrapError("context failed", originalErr)
//
// The result satisfies errors.Is(result, originalErr) == true.
func WrapError(message string, cause error) error {
	return fmt.Errorf("%s: %w", message, cause)
}
