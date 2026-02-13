package gojaeventloop

import (
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// TestTryExtractWrappedPromise verifies that the tryExtractWrappedPromise
// helper function correctly identifies and extracts wrapped ChainedPromise objects.
func TestTryExtractWrappedPromise(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	// Test 1: Wrapped promise should be extracted
	promise := adapter.JS().Resolve(nil).Then(func(r any) any { return nil }, func(err any) any { return nil })
	wrapped := adapter.GojaWrapPromise(promise)

	extracted, ok := tryExtractWrappedPromise(wrapped)
	if !ok {
		t.Error("Expected to extract wrapped promise, but got ok=false")
	}
	if extracted == nil {
		t.Error("Expected extracted promise to be non-nil")
	}
	if extracted != promise {
		t.Error("Expected extracted promise to be the same as original")
	}

	// Test 2: Non-object value should return (nil, false)
	nonObject := runtime.ToValue(42)
	extracted, ok = tryExtractWrappedPromise(nonObject)
	if ok {
		t.Error("Expected ok=false for non-object value")
	}
	if extracted != nil {
		t.Error("Expected nil promise for non-object value")
	}

	// Test 3: Object without _internalPromise should return (nil, false)
	plainObject := runtime.NewObject()
	extracted, ok = tryExtractWrappedPromise(plainObject)
	if ok {
		t.Error("Expected ok=false for plain object without _internalPromise")
	}
	if extracted != nil {
		t.Error("Expected nil promise for plain object")
	}

	// Test 4: Object with nil _internalPromise should return (nil, false)
	objectWithNilPromise := runtime.NewObject()
	objectWithNilPromise.Set("_internalPromise", runtime.ToValue(nil))
	extracted, ok = tryExtractWrappedPromise(objectWithNilPromise)
	if ok {
		t.Error("Expected ok=false for object with nil _internalPromise")
	}
	if extracted != nil {
		t.Error("Expected nil promise for object with nil _internalPromise")
	}

	// Test 5: Object with undefined _internalPromise should return (nil, false)
	objectWithUndefPromise := runtime.NewObject()
	objectWithUndefPromise.Set("_internalPromise", runtime.ToValue(goja.Undefined()))
	extracted, ok = tryExtractWrappedPromise(objectWithUndefPromise)
	if ok {
		t.Error("Expected ok=false for object with undefined _internalPromise")
	}
	if extracted != nil {
		t.Error("Expected nil promise for object with undefined _internalPromise")
	}
}

// TestTryExtractWrappedPromise_ConcurrentSafety verifies that the helper
// function handles concurrent access correctly when called from multiple goroutines.
func TestTryExtractWrappedPromise_ConcurrentSafety(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	// Create multiple wrapped promises
	promises := make([]goja.Value, 20)
	for i := range promises {
		p := adapter.JS().Resolve(nil).Then(func(r any) any { return nil }, func(err any) any { return nil })
		promises[i] = adapter.GojaWrapPromise(p)
	}

	// Concurrently extract (should be safe - no mutations)
	done := make(chan struct{})
	for i := 0; i < 5; i++ {
		go func() {
			for _, wrapped := range promises {
				extracted, ok := tryExtractWrappedPromise(wrapped)
				_ = extracted
				_ = ok
			}
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	// No assertions needed - test passes if no panic/race
}

// BenchmarkTryExtractWrappedPromise measures the performance of the
// promise extraction helper.
func BenchmarkTryExtractWrappedPromise(b *testing.B) {
	loop, err := goeventloop.New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		b.Fatalf("Failed to create adapter: %v", err)
	}
	promise := adapter.JS().Resolve(nil).Then(func(r any) any { return nil }, func(err any) any { return nil })
	wrapped := adapter.GojaWrapPromise(promise)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tryExtractWrappedPromise(wrapped)
	}
}
