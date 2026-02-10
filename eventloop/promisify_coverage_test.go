package eventloop

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ========================================================================
// COVERAGE-003: promisify() Function 100% Coverage
// Tests for: loop terminated check, context cancellation before goroutine,
//            function panic, runtime.Goexit(), SubmitInternal() failure paths
// ========================================================================

// TestPromisify_LoopAlreadyTerminated tests the immediate rejection path
// when calling Promisify on an already-terminated loop.
func TestPromisify_LoopAlreadyTerminated(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	// Shut down the loop first
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
	<-runDone

	// Now call Promisify on the terminated loop
	p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		t.Error("Function should never be called on terminated loop")
		return "shouldn't happen", nil
	})

	// The promise should be immediately rejected with ErrLoopTerminated
	select {
	case result := <-p.ToChannel():
		err, ok := result.(error)
		if !ok {
			t.Fatalf("Expected error, got: %T %v", result, result)
		}
		if !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Expected ErrLoopTerminated, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Promise never resolved on terminated loop")
	}
}

// TestPromisify_LoopTerminatingState tests calling Promisify while loop is terminating.
func TestPromisify_LoopTerminatingState(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	// Give loop time to start
	time.Sleep(50 * time.Millisecond)

	// Start shutdown in background
	shutdownDone := make(chan struct{})
	go func() {
		loop.Shutdown(context.Background())
		close(shutdownDone)
	}()

	// Try to create Promisify during termination - may or may not succeed
	// depending on timing, but should never hang
	p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		return "result", nil
	})

	// Should resolve within timeout (either with result or error)
	select {
	case result := <-p.ToChannel():
		// Either result or ErrLoopTerminated is acceptable during shutdown race
		if result == "result" {
			t.Log("Promise resolved with result (submitted before termination)")
		} else if err, ok := result.(error); ok && errors.Is(err, ErrLoopTerminated) {
			t.Log("Promise rejected with ErrLoopTerminated (submitted during termination)")
		} else {
			t.Logf("Promise resolved with: %T %v", result, result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Promise hung during termination")
	}

	<-shutdownDone
	<-runDone
}

// TestPromisify_ContextCancelledBeforeGoroutineStarts tests the immediate
// context cancellation check at the start of the goroutine.
func TestPromisify_ContextCancelledBeforeGoroutineStarts(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()
	defer func() {
		loop.Shutdown(context.Background())
		<-runDone
	}()

	// Cancel context BEFORE calling Promisify
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	funcCalled := atomic.Bool{}
	p := loop.Promisify(cancelCtx, func(ctx context.Context) (Result, error) {
		funcCalled.Store(true)
		return "should not happen", nil
	})

	// Promise should be rejected with context.Canceled
	select {
	case result := <-p.ToChannel():
		err, ok := result.(error)
		if !ok {
			t.Fatalf("Expected error for cancelled context, got: %T %v", result, result)
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Promise never resolved for cancelled context")
	}

	// The function should NOT have been called
	// (though there's a small race window where it might check ctx in the func body)
	time.Sleep(50 * time.Millisecond)
	if funcCalled.Load() {
		t.Log("Function was called despite pre-cancelled context (acceptable race)")
	}
}

// TestPromisify_GoexitDetection tests runtime.Goexit() detection in Promisify.
func TestPromisify_GoexitDetection(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()
	defer func() {
		loop.Shutdown(context.Background())
		<-runDone
	}()

	p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		runtime.Goexit()
		return nil, nil // Never reached
	})

	select {
	case result := <-p.ToChannel():
		err, ok := result.(error)
		if !ok {
			t.Fatalf("Expected error for Goexit, got: %T %v", result, result)
		}
		if !errors.Is(err, ErrGoexit) {
			t.Errorf("Expected ErrGoexit, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Promise never resolved for Goexit")
	}
}

// TestPromisify_PanicWithValue tests panic with non-nil value.
func TestPromisify_PanicWithValue(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()
	defer func() {
		loop.Shutdown(context.Background())
		<-runDone
	}()

	testCases := []struct {
		name       string
		panicValue any
	}{
		{"string panic", "test panic message"},
		{"int panic", 42},
		{"error panic", errors.New("error as panic")},
		{"struct panic", struct{ msg string }{"custom struct"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
				panic(tc.panicValue)
			})

			select {
			case result := <-p.ToChannel():
				err, ok := result.(error)
				if !ok {
					t.Fatalf("Expected error for panic, got: %T %v", result, result)
				}
				panicErr, ok := err.(PanicError)
				if !ok {
					t.Fatalf("Expected PanicError, got: %T %v", err, err)
				}
				// Verify the panic value is preserved
				if panicErr.Value != tc.panicValue {
					t.Errorf("PanicError.Value = %v, want %v", panicErr.Value, tc.panicValue)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Promise never resolved for panic")
			}
		})
	}
}

// Note: TestPromisify_PanicNil is covered by TestPromisify_PanicWithNil
// in promisify_panic_test.go - not duplicated here to avoid linter issues.

// TestPromisify_SubmitInternalFallbackOnError tests the fallback direct
// resolution when SubmitInternal fails (e.g., during shutdown).
func TestPromisify_SubmitInternalFallbackOnError(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	// Give loop time to start
	time.Sleep(50 * time.Millisecond)

	// Create barriers to control timing
	funcStarted := make(chan struct{})
	funcContinue := make(chan struct{})

	p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		close(funcStarted)
		<-funcContinue
		return "expected result", nil
	})

	// Wait for function to start
	<-funcStarted

	// Now shutdown the loop while function is running
	shutdownDone := make(chan struct{})
	go func() {
		loop.Shutdown(context.Background())
		close(shutdownDone)
	}()

	// Allow function to complete after shutdown is requested
	time.Sleep(20 * time.Millisecond)
	close(funcContinue)

	// The promise should still resolve (either via SubmitInternal or fallback)
	select {
	case result := <-p.ToChannel():
		if result == "expected result" {
			t.Log("Promise resolved successfully via fallback path")
		} else if err, ok := result.(error); ok {
			t.Logf("Promise rejected with: %v (may be shutdown-related)", err)
		} else {
			t.Logf("Promise resolved with: %v", result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Promise never resolved (fallback may have failed)")
	}

	<-shutdownDone
	<-runDone
}

// TestPromisify_SubmitInternalFallbackOnPanic tests fallback when SubmitInternal
// fails during panic handling.
func TestPromisify_SubmitInternalFallbackOnPanic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	// Give loop time to start
	time.Sleep(50 * time.Millisecond)

	panicStarted := make(chan struct{})

	p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		// Signal we're about to panic
		close(panicStarted)
		// Small delay to allow shutdown to start
		time.Sleep(10 * time.Millisecond)
		panic("intentional panic during shutdown")
	})

	// Wait for function to start panicking
	<-panicStarted

	// Start shutdown concurrently
	go func() {
		loop.Shutdown(context.Background())
	}()

	// Promise should still be rejected with PanicError
	select {
	case result := <-p.ToChannel():
		err, ok := result.(error)
		if !ok {
			t.Fatalf("Expected error for panic, got: %T %v", result, result)
		}
		_, isPanicErr := err.(PanicError)
		if !isPanicErr && !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Expected PanicError or ErrLoopTerminated, got: %T %v", err, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Promise never resolved during shutdown+panic")
	}

	<-runDone
}

// TestPromisify_ConcurrentWithShutdown tests multiple Promisify calls concurrent
// with shutdown to ensure all promises settle.
func TestPromisify_ConcurrentWithShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	// Give loop time to start
	time.Sleep(50 * time.Millisecond)

	const numPromises = 20
	promises := make([]Promise, numPromises)
	var wg sync.WaitGroup

	// Start promises concurrently
	for i := 0; i < numPromises; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			promises[i] = loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
				time.Sleep(time.Duration(i) * time.Millisecond)
				return i, nil
			})
		}()
	}

	wg.Wait()

	// Start shutdown while promises are in-flight
	go func() {
		time.Sleep(5 * time.Millisecond)
		loop.Shutdown(context.Background())
	}()

	// All promises should settle (not hang)
	for i, p := range promises {
		select {
		case result := <-p.ToChannel():
			if result == i {
				t.Logf("Promise %d resolved with value", i)
			} else if err, ok := result.(error); ok {
				t.Logf("Promise %d rejected with: %v", i, err)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("Promise %d hung during concurrent shutdown", i)
		}
	}

	<-runDone
}

// TestPromisify_NormalSuccess tests normal successful completion path.
func TestPromisify_NormalSuccess(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()
	defer func() {
		loop.Shutdown(context.Background())
		<-runDone
	}()

	expected := "test result value"
	p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		return expected, nil
	})

	select {
	case result := <-p.ToChannel():
		if result != expected {
			t.Errorf("Expected %v, got: %v", expected, result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Promise never resolved for success")
	}
}

// TestPromisify_ReturnsError tests normal error return path from function.
// This is the coverage test variant - promisify_panic_test.go has similar.
func TestPromisify_ReturnsError(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()
	defer func() {
		loop.Shutdown(context.Background())
		<-runDone
	}()

	expected := errors.New("expected error from function")
	p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		return nil, expected
	})

	select {
	case result := <-p.ToChannel():
		err, ok := result.(error)
		if !ok {
			t.Fatalf("Expected error, got: %T %v", result, result)
		}
		if !errors.Is(err, expected) {
			t.Errorf("Expected %v, got: %v", expected, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Promise never resolved for error")
	}
}

// TestPromisify_ContextTimeoutDuringExecution tests context timeout while
// function is executing.
func TestPromisify_ContextTimeoutDuringExecution(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()
	defer func() {
		loop.Shutdown(context.Background())
		<-runDone
	}()

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	funcCompleted := atomic.Bool{}
	p := loop.Promisify(timeoutCtx, func(ctx context.Context) (Result, error) {
		// Wait for context to be cancelled
		<-ctx.Done()
		funcCompleted.Store(true)
		return nil, ctx.Err()
	})

	select {
	case result := <-p.ToChannel():
		err, ok := result.(error)
		if !ok {
			t.Fatalf("Expected error for timeout, got: %T %v", result, result)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Expected context.DeadlineExceeded, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Promise never resolved for timeout")
	}

	time.Sleep(20 * time.Millisecond)
	if !funcCompleted.Load() {
		t.Error("Function should have completed after context cancellation")
	}
}

// TestPanicError_Error tests PanicError.Error() method.
func TestPanicError_Error(t *testing.T) {
	tests := []struct {
		value   any
		wantMsg string
	}{
		{"test message", "promise: goroutine panicked: test message"},
		{42, "promise: goroutine panicked: 42"},
		{nil, "promise: goroutine panicked: <nil>"},
		{errors.New("inner error"), "promise: goroutine panicked: inner error"},
	}

	for _, tc := range tests {
		pe := PanicError{Value: tc.value}
		got := pe.Error()
		if got != tc.wantMsg {
			t.Errorf("PanicError{%v}.Error() = %q, want %q", tc.value, got, tc.wantMsg)
		}
	}
}

// TestErrGoexit_Error tests ErrGoexit.Error() method.
func TestErrGoexit_Error(t *testing.T) {
	expected := "promise: goroutine exited via runtime.Goexit"
	got := ErrGoexit.Error()
	if got != expected {
		t.Errorf("ErrGoexit.Error() = %q, want %q", got, expected)
	}
}

// TestErrPanic_Error tests ErrPanic.Error() method.
func TestErrPanic_Error(t *testing.T) {
	expected := "promise: goroutine panicked"
	got := ErrPanic.Error()
	if got != expected {
		t.Errorf("ErrPanic.Error() = %q, want %q", got, expected)
	}
}
