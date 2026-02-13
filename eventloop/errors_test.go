package eventloop

import (
	"errors"
	"fmt"
	"io"
	"testing"
)

// customTestError is a test error type used by TestPanicError_ErrorsAs.
type customTestError struct {
	code int
}

func (e *customTestError) Error() string {
	return fmt.Sprintf("custom error: %d", e.code)
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

// TestAggregateError_Cause tests the Cause helper.
func TestAggregateError_Cause(t *testing.T) {
	// With errors
	aggErr := &AggregateError{
		Errors: []error{io.EOF, io.ErrUnexpectedEOF},
	}
	cause := aggErr.Cause()
	if cause != io.EOF {
		t.Errorf("Cause() = %v, want %v", cause, io.EOF)
	}

	// Empty errors
	emptyAgg := &AggregateError{}
	if got := emptyAgg.Cause(); got != nil {
		t.Errorf("Cause() with empty = %v, want nil", got)
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

// TestPanicError_Is tests the Is method of PanicError.
func TestPanicError_Is(t *testing.T) {
	panicErr := PanicError{Value: "something panicked"}

	// Should match another PanicError (value form)
	if !errors.Is(panicErr, PanicError{}) {
		t.Error("errors.Is(panicErr, PanicError{}) = false, want true")
	}

	// Should match pointer form
	if !errors.Is(panicErr, &PanicError{}) {
		t.Error("errors.Is(panicErr, &PanicError{}) = false, want true")
	}

	// Should not match unrelated error
	if errors.Is(panicErr, io.ErrClosedPipe) {
		t.Error("errors.Is(panicErr, io.ErrClosedPipe) = true, want false")
	}
}

// TestTypeError_Is tests the Is method of TypeError.
func TestTypeError_Is(t *testing.T) {
	typeErr := &TypeError{Message: "expected string"}

	// Should match another TypeError
	if !errors.Is(typeErr, &TypeError{}) {
		t.Error("errors.Is(typeErr, &TypeError{}) = false, want true")
	}

	// Should not match unrelated error
	if errors.Is(typeErr, io.EOF) {
		t.Error("errors.Is(typeErr, io.EOF) = true, want false")
	}

	// Should not match different error types
	if errors.Is(typeErr, &RangeError{}) {
		t.Error("errors.Is(typeErr, &RangeError{}) = true, want false")
	}
}

// TestRangeError_Is tests the Is method of RangeError.
func TestRangeError_Is(t *testing.T) {
	rangeErr := &RangeError{Message: "out of bounds"}

	// Should match another RangeError
	if !errors.Is(rangeErr, &RangeError{}) {
		t.Error("errors.Is(rangeErr, &RangeError{}) = false, want true")
	}

	// Should not match unrelated error
	if errors.Is(rangeErr, io.EOF) {
		t.Error("errors.Is(rangeErr, io.EOF) = true, want false")
	}

	// Should not match different error types
	if errors.Is(rangeErr, &TypeError{}) {
		t.Error("errors.Is(rangeErr, &TypeError{}) = true, want false")
	}
}

// TestTimeoutError_Is tests the Is method of TimeoutError.
func TestTimeoutError_Is(t *testing.T) {
	timeoutErr := &TimeoutError{Message: "request timed out"}

	// Should match another TimeoutError
	if !errors.Is(timeoutErr, &TimeoutError{}) {
		t.Error("errors.Is(timeoutErr, &TimeoutError{}) = false, want true")
	}

	// Should not match unrelated error
	if errors.Is(timeoutErr, io.EOF) {
		t.Error("errors.Is(timeoutErr, io.EOF) = true, want false")
	}

	// Should not match different error types
	if errors.Is(timeoutErr, &AbortError{}) {
		t.Error("errors.Is(timeoutErr, &AbortError{}) = true, want false")
	}
}
