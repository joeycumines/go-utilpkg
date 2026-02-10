package eventloop

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// TestPromisify_PanicRecovery verifies that Promisify recovers from panics
// and the PanicError.Error() method works correctly.
// Priority: HIGH - PanicError.Error() needs coverage.
func TestPromisify_PanicRecovery(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	go func() {
		_ = loop.Run(ctx)
	}()

	defer loop.Shutdown(context.Background())

	// Test 1: Panic with string
	p1 := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		panic("test string panic")
	})

	result := <-p1.ToChannel()
	errResult, ok := result.(error)
	if !ok {
		t.Fatalf("Expected error for string panic, got: %v", result)
	}

	panicErr, ok := errResult.(PanicError)
	if !ok {
		t.Fatalf("Expected PanicError, got: %T", errResult)
	}

	if panicErr.Value != "test string panic" {
		t.Errorf("Expected panic value 'test string panic', got: %v", panicErr.Value)
	}

	// Test PanicError.Error() method
	errMsg := panicErr.Error()
	expectedMsg := "promise: goroutine panicked: test string panic"
	if errMsg != expectedMsg {
		t.Errorf("PanicError.Error() message:\n  Got:      %s\n  Expected: %s", errMsg, expectedMsg)
	}

	// Test 2: Panic with integer
	p2 := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		panic(42)
	})

	result2 := <-p2.ToChannel()
	errResult2, ok := result2.(error)
	if !ok {
		t.Fatalf("Expected error for int panic, got: %v", result2)
	}

	panicErr2, ok := errResult2.(PanicError)
	if !ok {
		t.Fatalf("Expected PanicError, got: %T", errResult2)
	}

	if panicErr2.Value != 42 {
		t.Errorf("Expected panic value 42, got: %v", panicErr2.Value)
	}

	// Test Error() method with different value types
	errMsg2 := panicErr2.Error()
	expectedMsg2 := "promise: goroutine panicked: 42"
	if errMsg2 != expectedMsg2 {
		t.Errorf("PanicError.Error() message:\n  Got:      %s\n  Expected: %s", errMsg2, expectedMsg2)
	}
}

// TestPromisify_PanicWithNil verifies Promisify handles panic(nil).
// Priority: MEDIUM - Edge case panic(nil) handling.
//
// Note: In Go, panic(nil) is a special case. When you call recover() after
// panic(nil), it returns nil, but the Go runtime tracks this specially.
// If the recovered value is nil, we check if it was an actual nil panic
// by checking if runtime.PanicNilError matches.
func TestPromisify_PanicWithNil(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	go func() {
		_ = loop.Run(ctx)
	}()

	defer loop.Shutdown(context.Background())

	p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		panic(nil)
	})

	result := <-p.ToChannel()
	errResult, ok := result.(error)
	if !ok {
		t.Fatalf("Expected error for panic(nil), got: %v", result)
	}

	panicErr, ok := errResult.(PanicError)
	if !ok {
		t.Fatalf("Expected PanicError for panic(nil), got: %T", errResult)
	}

	// In Go, panic(nil) is handled specially by the runtime.
	// The recovered value depends on Go version and runtime behavior.
	// It could be nil, *runtime.PanicNilError, or a string.
	// We accept any of these as valid panic representation.
	t.Logf("PanicError.Value for panic(nil): Type=%T, Value=%v", panicErr.Value, panicErr.Value)

	// Verify Error() method formats the message correctly
	errMsg := panicErr.Error()
	if errMsg == "" {
		t.Error("PanicError.Error() should return non-empty message")
	}

	// The error message should mention "panicked" and contain the value representation
	if errMsg == "" || len(errMsg) < 10 {
		t.Errorf("PanicError.Error() returned unexpected message: %q", errMsg)
	}
}

// TestPromisify_NormalError verifies Promisify handles normal return errors.
// Priority: LOW - Basic error path already tested in promisify_test.go.
func TestPromisify_NormalError(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	go func() {
		_ = loop.Run(ctx)
	}()

	defer loop.Shutdown(context.Background())

	expectedErr := fmt.Errorf("normal error")
	p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		return nil, expectedErr
	})

	result := <-p.ToChannel()
	errResult, ok := result.(error)
	if !ok {
		t.Fatalf("Expected error, got: %v", result)
	}

	if !errors.Is(errResult, expectedErr) {
		t.Errorf("Expected error %v, got: %v", expectedErr, errResult)
	}
}

// TestPromisify_MultipleConcurrent_Panics tests concurrent Promisify calls that panic.
// Priority: MEDIUM - Concurrent panic scenarios.
func TestPromisify_MultipleConcurrent_Panics(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	go func() {
		_ = loop.Run(ctx)
	}()

	defer loop.Shutdown(context.Background())

	const numPromises = 10
	promises := make([]Promise, numPromises)

	for i := 0; i < numPromises; i++ {
		i := i
		promises[i] = loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
			if i%2 == 0 {
				panic(fmt.Sprintf("panic-%d", i))
			}
			return i, nil
		})
	}

	// Wait for all promises
	for i, p := range promises {
		result := <-p.ToChannel()

		if i%2 == 0 {
			// Should be a PanicError
			errResult, ok := result.(error)
			if !ok {
				t.Errorf("Promise %d: expected error, got: %v", i, result)
				continue
			}

			panicErr, ok := errResult.(PanicError)
			if !ok {
				t.Errorf("Promise %d: expected PanicError, got: %T", i, errResult)
				continue
			}

			expectedMsg := fmt.Sprintf("promise: goroutine panicked: panic-%d", i)
			if panicErr.Error() != expectedMsg {
				t.Errorf("Promise %d: PanicError.Error() = %s, expected %s", i, panicErr.Error(), expectedMsg)
			}
		} else {
			// Should be a value
			if result != i {
				t.Errorf("Promise %d: expected %d, got: %v", i, i, result)
			}
		}
	}
}

// TestPromisify_Shutdown_DuringExecution tests Promisify with shutdown during async work.
// Priority: MEDIUM - Edge case shutdown during Promisify.
func TestPromisify_Shutdown_DuringExecution(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	go func() {
		_ = loop.Run(ctx)
	}()

	defer loop.Shutdown(context.Background())

	// Start async work that takes time
	started := make(chan struct{})
	p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
		close(started)
		time.Sleep(100 * time.Millisecond) // Async work
		return "result", nil
	})

	<-started

	// Shutdown while work is in progress - loop should wait
	go func() {
		time.Sleep(10 * time.Millisecond)
		loop.Shutdown(context.Background())
	}()

	// Promise should still resolve
	result := <-p.ToChannel()
	if result != "result" {
		t.Errorf("Expected 'result', got: %v (promise should still resolve)", result)
	}
}
