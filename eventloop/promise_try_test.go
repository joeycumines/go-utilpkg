package eventloop

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Promise.try() Tests

// TestTry_Success tests that Try resolves with the function's return value.
func TestTry_Success(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	promise := js.Try(func() any {
		return "success"
	})

	// Start loop briefly
	done := make(chan struct{})
	go func() {
		loop.Run(context.TODO())
		close(done)
	}()

	// Wait for promise
	result := <-promise.ToChannel()

	loop.Shutdown(context.Background())
	<-done

	if result != "success" {
		t.Errorf("Expected 'success', got %v", result)
	}

	if promise.State() != Fulfilled {
		t.Errorf("Expected Fulfilled state, got %v", promise.State())
	}
}

// TestTry_Panic tests that Try catches panics and rejects.
func TestTry_Panic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	promise := js.Try(func() any {
		panic("test panic")
	})

	// The promise should be immediately rejected (panic caught synchronously)
	if promise.State() != Rejected {
		t.Errorf("Expected Rejected state, got %v", promise.State())
	}

	reason := promise.Reason()
	panicErr, ok := reason.(PanicError)
	if !ok {
		t.Fatalf("Expected PanicError, got %T: %v", reason, reason)
	}

	if panicErr.Value != "test panic" {
		t.Errorf("Expected panic value 'test panic', got %v", panicErr.Value)
	}
}

// TestTry_PanicWithError tests panic with an error value.
func TestTry_PanicWithError(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	expectedErr := errors.New("panic error")
	promise := js.Try(func() any {
		panic(expectedErr)
	})

	if promise.State() != Rejected {
		t.Errorf("Expected Rejected state, got %v", promise.State())
	}

	reason := promise.Reason()
	panicErr, ok := reason.(PanicError)
	if !ok {
		t.Fatalf("Expected PanicError, got %T: %v", reason, reason)
	}

	if panicErr.Value != expectedErr {
		t.Errorf("Expected error %v in panic, got %v", expectedErr, panicErr.Value)
	}
}

// TestTry_ReturnsNil tests that Try correctly handles nil return value.
func TestTry_ReturnsNil(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	promise := js.Try(func() any {
		return nil
	})

	if promise.State() != Fulfilled {
		t.Errorf("Expected Fulfilled state, got %v", promise.State())
	}

	if promise.Value() != nil {
		t.Errorf("Expected nil value, got %v", promise.Value())
	}
}

// TestTry_ReturnsError tests that returning an error resolves (not rejects).
func TestTry_ReturnsError(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	expectedErr := errors.New("returned error")
	promise := js.Try(func() any {
		return expectedErr
	})

	// Returning an error should RESOLVE with the error as the value
	// This is different from rejecting
	if promise.State() != Fulfilled {
		t.Errorf("Expected Fulfilled state, got %v", promise.State())
	}

	if promise.Value() != expectedErr {
		t.Errorf("Expected error as value, got %v", promise.Value())
	}
}

// TestTry_Chaining tests that Try result can be chained.
func TestTry_Chaining(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	result := make(chan any, 1)
	promise := js.Try(func() any {
		return 42
	}).Then(func(v any) any {
		return v.(int) * 2
	}, nil).Then(func(v any) any {
		result <- v
		return nil
	}, nil)

	_ = promise // Avoid unused variable warning

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	select {
	case v := <-result:
		if v != 84 {
			t.Errorf("Expected 84, got %v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for result")
	}

	loop.Shutdown(context.Background())
	<-done
}

// TestTry_PanicChaining tests that panic rejection can be caught.
func TestTry_PanicChaining(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	result := make(chan any, 1)
	promise := js.Try(func() any {
		panic("oops")
	}).Catch(func(r any) any {
		panicErr, ok := r.(PanicError)
		if ok {
			result <- panicErr.Value
		} else {
			result <- r
		}
		return nil
	})

	_ = promise

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	select {
	case v := <-result:
		if v != "oops" {
			t.Errorf("Expected 'oops', got %v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for result")
	}

	loop.Shutdown(context.Background())
	<-done
}

// TestTry_ComplexValues tests Try with complex return values.
func TestTry_ComplexValues(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	type ComplexData struct {
		Name  string
		Value int
	}

	expected := ComplexData{Name: "test", Value: 123}
	promise := js.Try(func() any {
		return expected
	})

	if promise.State() != Fulfilled {
		t.Errorf("Expected Fulfilled state, got %v", promise.State())
	}

	result, ok := promise.Value().(ComplexData)
	if !ok {
		t.Fatalf("Expected ComplexData, got %T", promise.Value())
	}

	if result != expected {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

// TestTry_PanicWithNil tests panic with nil value.
func TestTry_PanicWithNil(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	promise := js.Try(func() any {
		panic(nil)
	})

	if promise.State() != Rejected {
		t.Errorf("Expected Rejected state, got %v", promise.State())
	}

	reason := promise.Reason()
	panicErr, ok := reason.(PanicError)
	if !ok {
		t.Fatalf("Expected PanicError, got %T: %v", reason, reason)
	}

	// Go 1.21+ converts panic(nil) to a runtime.PanicNilError with message
	// "panic called with nil argument". We just check the PanicError wrapper works.
	// The actual value might be nil or the runtime.PanicNilError depending on Go version.
	if panicErr.Value == nil {
		// Go <1.21 behavior: panic(nil) passes through as nil
		t.Log("panic(nil) resulted in nil value (pre-Go 1.21 behavior)")
	} else {
		// Go >=1.21 behavior: panic(nil) is wrapped in runtime.PanicNilError
		t.Logf("panic(nil) resulted in: %v (Go 1.21+ behavior)", panicErr.Value)
	}
}
