//go:build linux || darwin

package eventloop

import (
	"errors"
	"fmt"
	"io"
	"testing"
)

// TestErrorWithCause_Error tests the Error() method of ErrorWithCause.
func TestErrorWithCause_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *ErrorWithCause
		want    string
	}{
		{
			name:    "message only",
			err:     &ErrorWithCause{Message: "something failed"},
			want:    "something failed",
		},
		{
			name:    "message with cause",
			err:     &ErrorWithCause{Message: "top level error", Cause: io.EOF},
			want:    "top level error",
		},
		{
			name:    "empty message with cause",
			err:     &ErrorWithCause{Message: "", Cause: io.EOF},
			want:    "EOF",
		},
		{
			name:    "empty message no cause",
			err:     &ErrorWithCause{Message: "", Cause: nil},
			want:    "unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestErrorWithCause_Unwrap tests the Unwrap() method of ErrorWithCause.
func TestErrorWithCause_Unwrap(t *testing.T) {
	cause := io.ErrUnexpectedEOF
	err := &ErrorWithCause{Message: "read failed", Cause: cause}

	if got := err.Unwrap(); got != cause {
		t.Errorf("Unwrap() = %v, want %v", got, cause)
	}

	// Test nil cause
	nilCauseErr := &ErrorWithCause{Message: "no cause"}
	if got := nilCauseErr.Unwrap(); got != nil {
		t.Errorf("Unwrap() with nil cause = %v, want nil", got)
	}
}

// TestErrorWithCause_ErrorsIs tests errors.Is with ErrorWithCause.
func TestErrorWithCause_ErrorsIs(t *testing.T) {
	ioErr := io.EOF
	wrappedOnce := &ErrorWithCause{Message: "level 1", Cause: ioErr}
	wrappedTwice := &ErrorWithCause{Message: "level 2", Cause: wrappedOnce}

	// Should find io.EOF through chain
	if !errors.Is(wrappedOnce, io.EOF) {
		t.Error("errors.Is(wrappedOnce, io.EOF) = false, want true")
	}

	if !errors.Is(wrappedTwice, io.EOF) {
		t.Error("errors.Is(wrappedTwice, io.EOF) = false, want true")
	}

	// Should not match unrelated error
	if errors.Is(wrappedOnce, io.ErrClosedPipe) {
		t.Error("errors.Is(wrappedOnce, io.ErrClosedPipe) = true, want false")
	}
}

// TestErrorWithCause_ErrorsAs tests errors.As with ErrorWithCause.
func TestErrorWithCause_ErrorsAs(t *testing.T) {
	customErr := &customTestError{code: 42}
	err := &ErrorWithCause{Message: "wrapped custom", Cause: customErr}

	var target *customTestError
	if !errors.As(err, &target) {
		t.Error("errors.As failed to find customTestError")
	}

	if target.code != 42 {
		t.Errorf("target.code = %d, want 42", target.code)
	}
}

type customTestError struct {
	code int
}

func (e *customTestError) Error() string {
	return fmt.Sprintf("custom error: %d", e.code)
}

// TestNewErrorWithCause tests the constructor function.
func TestNewErrorWithCause(t *testing.T) {
	cause := io.EOF
	err := NewErrorWithCause("operation failed", cause)

	if err.Message != "operation failed" {
		t.Errorf("Message = %q, want %q", err.Message, "operation failed")
	}

	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}

	if !errors.Is(err, io.EOF) {
		t.Error("errors.Is(err, io.EOF) = false, want true")
	}
}

// TestPanicError_Unwrap tests the Unwrap() method of PanicError.
func TestPanicError_Unwrap(t *testing.T) {
	// Test with error value
	ioErr := io.ErrUnexpectedEOF
	panicErr := PanicError{Value: ioErr}

	if got := panicErr.Unwrap(); got != ioErr {
		t.Errorf("Unwrap() with error = %v, want %v", got, ioErr)
	}

	// Test with non-error value
	stringPanic := PanicError{Value: "panic string"}
	if got := stringPanic.Unwrap(); got != nil {
		t.Errorf("Unwrap() with string = %v, want nil", got)
	}

	// Test with nil value
	nilPanic := PanicError{Value: nil}
	if got := nilPanic.Unwrap(); got != nil {
		t.Errorf("Unwrap() with nil = %v, want nil", got)
	}
}

// TestPanicError_ErrorsIs tests errors.Is with PanicError.
func TestPanicError_ErrorsIs(t *testing.T) {
	originalErr := io.EOF
	panicErr := PanicError{Value: originalErr}

	// Should find io.EOF through Unwrap
	if !errors.Is(panicErr, io.EOF) {
		t.Error("errors.Is(panicErr, io.EOF) = false, want true")
	}

	// String panic should not match any error
	stringPanic := PanicError{Value: "panic!"}
	if errors.Is(stringPanic, io.EOF) {
		t.Error("errors.Is(stringPanic, io.EOF) = true, want false")
	}
}

// TestPanicError_ErrorsAs tests errors.As with PanicError.
func TestPanicError_ErrorsAs(t *testing.T) {
	customErr := &customTestError{code: 123}
	panicErr := PanicError{Value: customErr}

	var target *customTestError
	if !errors.As(panicErr, &target) {
		t.Error("errors.As failed to find customTestError in PanicError")
	}

	if target.code != 123 {
		t.Errorf("target.code = %d, want 123", target.code)
	}
}

// TestAggregateError_Unwrap tests the Unwrap() method of AggregateError.
func TestAggregateError_Unwrap(t *testing.T) {
	err1 := io.EOF
	err2 := io.ErrUnexpectedEOF

	aggErr := &AggregateError{
		Errors: []error{err1, err2},
	}

	unwrapped := aggErr.Unwrap()
	if len(unwrapped) != 2 {
		t.Errorf("len(Unwrap()) = %d, want 2", len(unwrapped))
	}

	if unwrapped[0] != err1 || unwrapped[1] != err2 {
		t.Error("Unwrap() returned wrong errors")
	}
}

// TestAggregateError_ErrorsIs tests errors.Is with AggregateError.
func TestAggregateError_ErrorsIs(t *testing.T) {
	aggErr := &AggregateError{
		Errors: []error{io.EOF, io.ErrUnexpectedEOF, io.ErrClosedPipe},
	}

	// Should find all contained errors
	if !errors.Is(aggErr, io.EOF) {
		t.Error("errors.Is(aggErr, io.EOF) = false, want true")
	}

	if !errors.Is(aggErr, io.ErrUnexpectedEOF) {
		t.Error("errors.Is(aggErr, io.ErrUnexpectedEOF) = false, want true")
	}

	if !errors.Is(aggErr, io.ErrClosedPipe) {
		t.Error("errors.Is(aggErr, io.ErrClosedPipe) = false, want true")
	}

	// Should not find unrelated error
	if errors.Is(aggErr, io.ErrNoProgress) {
		t.Error("errors.Is(aggErr, io.ErrNoProgress) = true, want false")
	}
}

// TestAggregateError_AggregateErrorCause tests the AggregateErrorCause helper.
func TestAggregateError_AggregateErrorCause(t *testing.T) {
	// With errors
	aggErr := &AggregateError{
		Errors: []error{io.EOF, io.ErrUnexpectedEOF},
	}
	cause := aggErr.AggregateErrorCause()
	if cause != io.EOF {
		t.Errorf("AggregateErrorCause() = %v, want %v", cause, io.EOF)
	}

	// Empty errors
	emptyAgg := &AggregateError{}
	if got := emptyAgg.AggregateErrorCause(); got != nil {
		t.Errorf("AggregateErrorCause() with empty = %v, want nil", got)
	}
}

// TestAbortError tests AbortError functionality.
func TestAbortError(t *testing.T) {
	t.Run("Error message with string reason", func(t *testing.T) {
		err := &AbortError{Reason: "user cancelled"}
		if got := err.Error(); got != "AbortError: user cancelled" {
			t.Errorf("Error() = %q, want %q", got, "AbortError: user cancelled")
		}
	})

	t.Run("Default message with nil reason", func(t *testing.T) {
		err := &AbortError{}
		if got := err.Error(); got != "AbortError: The operation was aborted" {
			t.Errorf("Error() = %q, want %q", got, "AbortError: The operation was aborted")
		}
	})

	t.Run("Error message with error reason", func(t *testing.T) {
		cause := io.EOF
		err := &AbortError{Reason: cause}
		if got := err.Error(); got != "AbortError: EOF" {
			t.Errorf("Error() = %q, want %q", got, "AbortError: EOF")
		}
	})

	t.Run("Unwrap with error reason", func(t *testing.T) {
		cause := io.EOF
		err := &AbortError{Reason: cause}

		if !errors.Is(err, io.EOF) {
			t.Error("errors.Is(err, io.EOF) = false, want true")
		}
	})

	t.Run("Unwrap with non-error reason", func(t *testing.T) {
		err := &AbortError{Reason: "string reason"}

		if err.Unwrap() != nil {
			t.Errorf("Unwrap() with string = %v, want nil", err.Unwrap())
		}
	})

	t.Run("Is with AbortError target", func(t *testing.T) {
		err := &AbortError{Reason: "test"}
		target := &AbortError{}

		if !err.Is(target) {
			t.Error("Is(target) = false, want true for AbortError")
		}
	})
}

// TestTypeError tests TypeError functionality.
func TestTypeError(t *testing.T) {
	t.Run("Error message", func(t *testing.T) {
		err := &TypeError{Message: "expected string, got number"}
		if got := err.Error(); got != "expected string, got number" {
			t.Errorf("Error() = %q, want %q", got, "expected string, got number")
		}
	})

	t.Run("Default message", func(t *testing.T) {
		err := &TypeError{}
		if got := err.Error(); got != "type error" {
			t.Errorf("Error() = %q, want %q", got, "type error")
		}
	})

	t.Run("With cause", func(t *testing.T) {
		cause := io.EOF
		err := &TypeError{Message: "invalid type", Cause: cause}

		if !errors.Is(err, io.EOF) {
			t.Error("errors.Is(err, io.EOF) = false, want true")
		}
	})
}

// TestRangeError tests RangeError functionality.
func TestRangeError(t *testing.T) {
	t.Run("Error message", func(t *testing.T) {
		err := &RangeError{Message: "index out of bounds"}
		if got := err.Error(); got != "index out of bounds" {
			t.Errorf("Error() = %q, want %q", got, "index out of bounds")
		}
	})

	t.Run("Default message", func(t *testing.T) {
		err := &RangeError{}
		if got := err.Error(); got != "range error" {
			t.Errorf("Error() = %q, want %q", got, "range error")
		}
	})

	t.Run("With cause", func(t *testing.T) {
		cause := io.EOF
		err := &RangeError{Message: "out of range", Cause: cause}

		if !errors.Is(err, io.EOF) {
			t.Error("errors.Is(err, io.EOF) = false, want true")
		}
	})
}

// TestTimeoutError tests TimeoutError functionality.
func TestTimeoutError(t *testing.T) {
	t.Run("Error message", func(t *testing.T) {
		err := &TimeoutError{Message: "request timed out after 5s"}
		if got := err.Error(); got != "request timed out after 5s" {
			t.Errorf("Error() = %q, want %q", got, "request timed out after 5s")
		}
	})

	t.Run("Default message", func(t *testing.T) {
		err := &TimeoutError{}
		if got := err.Error(); got != "operation timed out" {
			t.Errorf("Error() = %q, want %q", got, "operation timed out")
		}
	})

	t.Run("With cause", func(t *testing.T) {
		cause := io.EOF
		err := &TimeoutError{Message: "timeout", Cause: cause}

		if !errors.Is(err, io.EOF) {
			t.Error("errors.Is(err, io.EOF) = false, want true")
		}
	})
}

// TestWrapError tests the WrapError convenience function.
func TestWrapError(t *testing.T) {
	original := io.EOF
	wrapped := WrapError("failed to read", original)

	// Should contain message
	if got := wrapped.Error(); got != "failed to read: EOF" {
		t.Errorf("Error() = %q, want %q", got, "failed to read: EOF")
	}

	// Should match original error
	if !errors.Is(wrapped, io.EOF) {
		t.Error("errors.Is(wrapped, io.EOF) = false, want true")
	}
}

// TestDeepErrorChain tests deep error chains with multiple levels.
func TestDeepErrorChain(t *testing.T) {
	// Build a 5-level deep error chain
	level0 := io.EOF
	level1 := &ErrorWithCause{Message: "level 1", Cause: level0}
	level2 := &ErrorWithCause{Message: "level 2", Cause: level1}
	level3 := PanicError{Value: level2}
	level4 := &ErrorWithCause{Message: "level 4", Cause: level3}

	// errors.Is should find io.EOF at the bottom
	if !errors.Is(level4, io.EOF) {
		t.Error("errors.Is failed to find io.EOF in deep chain")
	}

	// errors.As should find ErrorWithCause
	var ewc *ErrorWithCause
	if !errors.As(level4, &ewc) {
		t.Error("errors.As failed to find ErrorWithCause in chain")
	}

	// Should also work with PanicError
	var pe PanicError
	if !errors.As(level4, &pe) {
		t.Error("errors.As failed to find PanicError in chain")
	}
}

// TestErrorWithCause_NilCause tests behavior with nil cause.
func TestErrorWithCause_NilCause(t *testing.T) {
	err := &ErrorWithCause{Message: "no cause error", Cause: nil}

	// Should work with errors.Is
	if errors.Is(err, io.EOF) {
		t.Error("errors.Is(err, io.EOF) = true, want false for nil cause")
	}

	// Unwrap should return nil
	if got := errors.Unwrap(err); got != nil {
		t.Errorf("Unwrap() = %v, want nil", got)
	}
}

// TestAggregateError_Is tests the Is method of AggregateError.
func TestAggregateError_Is(t *testing.T) {
	aggErr := &AggregateError{
		Message: "all failed",
		Errors:  []error{io.EOF},
	}

	// Should match another AggregateError
	targetAgg := &AggregateError{}
	if !aggErr.Is(targetAgg) {
		t.Error("Is(targetAgg) = false, want true for AggregateError type match")
	}

	// Should not match non-AggregateError
	if aggErr.Is(io.EOF) {
		t.Error("Is(io.EOF) = true, want false for non-AggregateError")
	}
}

// TestPanicError_ChainedErrors tests PanicError with chained errors.
func TestPanicError_ChainedErrors(t *testing.T) {
	// Create a chain: PanicError -> ErrorWithCause -> io.EOF
	inner := &ErrorWithCause{Message: "inner", Cause: io.EOF}
	panic := PanicError{Value: inner}

	// Should find io.EOF through the chain
	if !errors.Is(panic, io.EOF) {
		t.Error("errors.Is(panic, io.EOF) = false, want true through chain")
	}

	// Should find ErrorWithCause
	var ewc *ErrorWithCause
	if !errors.As(panic, &ewc) {
		t.Error("errors.As failed to find ErrorWithCause through panic")
	}

	if ewc.Message != "inner" {
		t.Errorf("ewc.Message = %q, want %q", ewc.Message, "inner")
	}
}
