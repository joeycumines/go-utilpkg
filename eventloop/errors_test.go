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

	var typedNil *customTestError
	typedNilPanic := PanicError{Value: typedNil}
	if got := typedNilPanic.Unwrap(); got != nil {
		t.Errorf("Unwrap() with typed nil = %v, want nil", got)
	}
}

func TestPanicErrorError(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "string", value: "test message", want: "eventloop: promise callback panicked: test message"},
		{name: "integer", value: 42, want: "eventloop: promise callback panicked: 42"},
		{name: "nil", value: nil, want: "eventloop: promise callback panicked: <nil>"},
		{name: "error", value: errors.New("inner error"), want: "eventloop: promise callback panicked: inner error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := (PanicError{Value: test.value}).Error(); got != test.want {
				t.Fatalf("PanicError.Error() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestErrGoexitError(t *testing.T) {
	const want = "eventloop: promise callback exited via runtime.Goexit"
	if got := ErrGoexit.Error(); got != want {
		t.Fatalf("ErrGoexit.Error() = %q, want %q", got, want)
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
		Errors: []any{err1, err2},
	}

	unwrapped := aggErr.Unwrap()
	if len(unwrapped) != 2 {
		t.Fatalf("len(Unwrap()) = %d, want 2", len(unwrapped))
	}

	if unwrapped[0] != err1 || unwrapped[1] != err2 {
		t.Error("Unwrap() returned wrong errors")
	}
}

func TestAggregateErrorError(t *testing.T) {
	for _, test := range []struct {
		name    string
		message string
		want    string
	}{
		{name: "default", want: "All promises were rejected"},
		{name: "custom", message: "custom aggregate message", want: "custom aggregate message"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := &AggregateError{Message: test.message, Errors: []any{io.EOF}}
			if got := err.Error(); got != test.want {
				t.Fatalf("AggregateError.Error() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestAggregateError_ErrorsIs tests errors.Is with AggregateError.
func TestAggregateError_ErrorsIs(t *testing.T) {
	aggErr := &AggregateError{
		Errors: []any{io.EOF, io.ErrUnexpectedEOF, io.ErrClosedPipe},
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

func TestNilPromiseError(t *testing.T) {
	err := &NilPromiseError{Index: 3}
	if got := err.Error(); got != "eventloop: nil promise at index 3" {
		t.Fatalf("Error() = %q, want %q", got, "eventloop: nil promise at index 3")
	}
	if !errors.Is(err, &NilPromiseError{Index: 99}) {
		t.Fatal("errors.Is with NilPromiseError target = false, want true")
	}
	if errors.Is(err, io.EOF) {
		t.Fatal("errors.Is with unrelated target = true, want false")
	}
	var target *NilPromiseError
	if !errors.As(err, &target) {
		t.Fatal("errors.As failed to find NilPromiseError")
	}
	if target.Index != 3 {
		t.Fatalf("errors.As target Index = %d, want 3", target.Index)
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

	t.Run("Default message with other reason type", func(t *testing.T) {
		err := &AbortError{Reason: 42}
		if got := err.Error(); got != "AbortError: The operation was aborted" {
			t.Errorf("Error() = %q, want %q", got, "AbortError: The operation was aborted")
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

	t.Run("Is with unrelated target", func(t *testing.T) {
		err := &AbortError{Reason: "test"}
		if err.Is(io.EOF) {
			t.Fatal("Is(io.EOF) = true, want false")
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

func TestUnhandledRejectionDebugInfoReason(t *testing.T) {
	reason := errors.New("test error")
	tests := []struct {
		name       string
		reason     any
		want       string
		wantUnwrap error
	}{
		{name: "error", reason: reason, want: reason.Error(), wantUnwrap: reason},
		{name: "non-error", reason: "string reason", want: "string reason"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := &UnhandledRejectionDebugInfo{Reason: test.reason}
			if got := info.Error(); got != test.want {
				t.Fatalf("Error = %q, want %q", got, test.want)
			}
			if got := info.Unwrap(); got != test.wantUnwrap {
				t.Fatalf("Unwrap = %v, want %v", got, test.wantUnwrap)
			}
		})
	}
}

func TestRetainedErrorNilContracts(t *testing.T) {
	var typedNil *customTestError
	rollbackErr := &FDRegistrationRollbackError{
		cause:      typedNil,
		rollback:   typedNil,
		registered: true,
	}
	if got := rollbackErr.Unwrap(); len(got) != 0 {
		t.Fatalf("FDRegistrationRollbackError typed-nil unwrap = %#v, want empty", got)
	}
	if !rollbackErr.Registered() {
		t.Fatal("FDRegistrationRollbackError Registered = false, want true")
	}
	unregisterErr := &FDUnregisterError{cause: typedNil, released: true}
	if got := unregisterErr.Unwrap(); got != nil {
		t.Fatalf("FDUnregisterError typed-nil unwrap = %#v, want nil", got)
	}
	if !unregisterErr.Released() {
		t.Fatal("FDUnregisterError Released = false, want true")
	}

	timeoutErr := &TimeoutError{Cause: typedNil}
	if got := timeoutErr.Unwrap(); got != nil {
		t.Fatalf("TimeoutError typed-nil cause unwrap = %v, want nil", got)
	}
	abortErr := &AbortError{Reason: typedNil}
	if got := abortErr.Unwrap(); got != nil {
		t.Fatalf("AbortError typed-nil reason unwrap = %v, want nil", got)
	}
	if got, want := abortErr.Error(), "AbortError: The operation was aborted"; got != want {
		t.Fatalf("AbortError typed-nil reason = %q, want %q", got, want)
	}

	aggregateErr := &AggregateError{Errors: []any{typedNil, io.EOF}}
	if got := aggregateErr.Unwrap(); len(got) != 1 || got[0] != io.EOF {
		t.Fatalf("AggregateError typed-nil filtering = %#v, want [io.EOF]", got)
	}

	var nilTimeout *TimeoutError
	if got, want := nilTimeout.Error(), "operation timed out"; got != want || nilTimeout.Unwrap() != nil {
		t.Fatalf("nil TimeoutError = %q, %v; want %q, nil", got, nilTimeout.Unwrap(), want)
	}
	var nilAbort *AbortError
	if got, want := nilAbort.Error(), "AbortError: The operation was aborted"; got != want || nilAbort.Unwrap() != nil {
		t.Fatalf("nil AbortError = %q, %v; want %q, nil", got, nilAbort.Unwrap(), want)
	}
	var nilAggregate *AggregateError
	if got, want := nilAggregate.Error(), "All promises were rejected"; got != want || nilAggregate.Unwrap() != nil {
		t.Fatalf("nil AggregateError = %q, %#v; want %q, nil", got, nilAggregate.Unwrap(), want)
	}
	var nilPromise *NilPromiseError
	if got, want := nilPromise.Error(), "eventloop: nil promise"; got != want {
		t.Fatalf("nil NilPromiseError = %q, want %q", got, want)
	}
	var nilPanic *PanicError
	if errors.Is(PanicError{}, nilPanic) ||
		errors.Is(&AggregateError{}, nilAggregate) ||
		errors.Is(&NilPromiseError{}, nilPromise) ||
		errors.Is(&TimeoutError{}, nilTimeout) ||
		errors.Is(&AbortError{}, nilAbort) {
		t.Fatal("a non-nil category error matched a typed-nil target")
	}

	controller := NewAbortController()
	controller.Abort(typedNil)
	throwErr := controller.Signal().ThrowIfAborted()
	var wrappedTypedNil *AbortError
	if !errors.As(throwErr, &wrappedTypedNil) || wrappedTypedNil.Reason != typedNil {
		t.Fatalf("ThrowIfAborted typed-nil reason = %T %#v, want *AbortError retaining input", throwErr, throwErr)
	}
	debugInfo := &UnhandledRejectionDebugInfo{Reason: typedNil}
	if got, want := debugInfo.Error(), "<nil>"; got != want || debugInfo.Unwrap() != nil {
		t.Fatalf("typed-nil UnhandledRejectionDebugInfo = %q, %v; want %q, nil", got, debugInfo.Unwrap(), want)
	}
	var nilDebugInfo *UnhandledRejectionDebugInfo
	if got, want := nilDebugInfo.Error(), "<nil>"; got != want || nilDebugInfo.Unwrap() != nil {
		t.Fatalf("nil UnhandledRejectionDebugInfo = %q, %v; want %q, nil", got, nilDebugInfo.Unwrap(), want)
	}
}

// TestAggregateError_Is tests the Is method of AggregateError.
func TestAggregateError_Is(t *testing.T) {
	aggErr := &AggregateError{
		Message: "all failed",
		Errors:  []any{io.EOF},
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
