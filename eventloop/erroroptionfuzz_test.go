package eventloop

import (
	"errors"
	"testing"
)

func FuzzErrorTypesAndOptions(f *testing.F) {
	f.Add("", false, int64(UnhandledRejectionFallbackDisabled))
	f.Add("isolated fallback", false, int64(UnhandledRejectionFallbackIsolated))
	f.Add("message", true, int64(17))
	f.Add("wrapped", false, int64(-42))

	f.Fuzz(func(t *testing.T, message string, useCause bool, raw int64) {
		sentinel := errors.New("sentinel")
		var cause error
		if useCause {
			cause = sentinel
		}

		timeoutErr := &TimeoutError{Message: message, Cause: cause}
		if !errors.Is(timeoutErr, &TimeoutError{}) || (useCause && !errors.Is(timeoutErr, sentinel)) {
			t.Fatalf("TimeoutError matching/unwrapping failed")
		}
		panicErr := PanicError{Value: cause}
		if !errors.Is(panicErr, PanicError{}) || (useCause && !errors.Is(panicErr, sentinel)) {
			t.Fatalf("PanicError matching/unwrapping failed")
		}
		abortErr := &AbortError{Reason: cause}
		if !errors.Is(abortErr, &AbortError{}) || (useCause && !errors.Is(abortErr, sentinel)) {
			t.Fatalf("AbortError matching/unwrapping failed")
		}
		agg := &AggregateError{Errors: []any{sentinel, timeoutErr}, Message: message}
		if !errors.Is(agg, &AggregateError{}) || !errors.Is(agg, sentinel) {
			t.Fatalf("AggregateError matching failed")
		}

		mode := FastPathMode(raw)
		validFastPathMode := mode == FastPathAuto || mode == FastPathForced || mode == FastPathDisabled
		if validFastPathMode {
			loop := New(WithFastPathMode(mode))
			if closeErr := loop.Close(); closeErr != nil && !errors.Is(closeErr, ErrLoopTerminated) {
				t.Fatalf("Close after option fuzzing: %v", closeErr)
			}
		} else if got := captureLoopOptionPanic(func() { _ = New(WithFastPathMode(mode)) }); got == nil {
			t.Fatalf("invalid fast path mode %d did not panic", raw)
		}

		loop := New()
		fallbackMode := UnhandledRejectionFallbackMode(raw)
		validMode := fallbackMode == UnhandledRejectionFallbackIsolated || fallbackMode == UnhandledRejectionFallbackDisabled
		if validMode {
			js := NewJS(loop, WithUnhandledRejectionFallback(fallbackMode))
			if js.unhandledFallback != fallbackMode {
				t.Fatalf("fallback mode = %v, want %v", js.unhandledFallback, fallbackMode)
			}
		} else if got := captureLoopOptionPanic(func() {
			NewJS(loop, WithUnhandledRejectionFallback(fallbackMode))
		}); got == nil {
			t.Fatalf("invalid unhandled rejection fallback mode %d accepted", raw)
		}
		if err := loop.Close(); err != nil {
			t.Fatalf("Close after JS option fuzzing: %v", err)
		}
	})
}
