package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// EXPAND-003: Promise.try() Tests
// ===============================================

// TestPromiseTry_Success tests Promise.try() with a successful function.
func TestPromiseTry_Success(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	_, err = runtime.RunString(`
		let result;
		Promise.try(() => {
			return 42;
		}).then(v => {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("Promise.try success test failed: %v", err)
	}

	// Allow microtasks to run
	time.Sleep(50 * time.Millisecond)

	resultVal := runtime.Get("result")
	if resultVal.Export() != int64(42) {
		t.Errorf("Expected 42, got %v", resultVal.Export())
	}

	_ = loop.Shutdown(context.Background())
	<-done
}

// TestPromiseTry_Throws tests Promise.try() with a throwing function.
func TestPromiseTry_Throws(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	_, err = runtime.RunString(`
		let caught = false;
		let errorMessage;
		
		Promise.try(() => {
			throw new Error("test error");
		}).catch(err => {
			caught = true;
			errorMessage = err.message || String(err);
		});
	`)
	if err != nil {
		t.Fatalf("Promise.try throws test failed: %v", err)
	}

	// Allow microtasks to run
	time.Sleep(50 * time.Millisecond)

	caughtVal := runtime.Get("caught")
	if caughtVal.Export() != true {
		t.Error("Error should have been caught")
	}

	_ = loop.Shutdown(context.Background())
	<-done
}

// TestPromiseTry_ReturnsPromise tests Promise.try() with a function returning a promise.
func TestPromiseTry_ReturnsPromise(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	_, err = runtime.RunString(`
		let result;
		Promise.try(() => {
			return Promise.resolve(100);
		}).then(v => {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("Promise.try returns promise test failed: %v", err)
	}

	// Allow microtasks to run
	time.Sleep(50 * time.Millisecond)

	resultVal := runtime.Get("result")
	if resultVal.Export() != int64(100) {
		t.Errorf("Expected 100, got %v", resultVal.Export())
	}

	_ = loop.Shutdown(context.Background())
	<-done
}

// TestPromiseTry_ReturnsNull tests Promise.try() with a function returning null.
func TestPromiseTry_ReturnsNull(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	_, err = runtime.RunString(`
		let result = "not set";
		let thenCalled = false;
		
		Promise.try(() => {
			return null;
		}).then(v => {
			thenCalled = true;
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("Promise.try returns null test failed: %v", err)
	}

	// Allow microtasks to run
	time.Sleep(50 * time.Millisecond)

	thenCalled := runtime.Get("thenCalled")
	if thenCalled.Export() != true {
		t.Error("Then should have been called")
	}

	resultVal := runtime.Get("result")
	if resultVal.Export() != nil {
		t.Errorf("Expected null, got %v", resultVal.Export())
	}

	_ = loop.Shutdown(context.Background())
	<-done
}

// TestPromiseTry_NonFunction tests Promise.try() with non-function argument.
func TestPromiseTry_NonFunction(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		try {
			Promise.try(42);
			throw new Error("should have thrown");
		} catch (e) {
			if (!(e instanceof TypeError)) {
				throw new Error("Expected TypeError, got: " + e);
			}
		}
	`)
	if err != nil {
		t.Fatalf("Promise.try non-function test failed: %v", err)
	}
}

// TestPromiseTry_Chaining tests Promise.try() chaining.
func TestPromiseTry_Chaining(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	_, err = runtime.RunString(`
		let result;
		Promise.try(() => {
			return 10;
		}).then(v => {
			return v * 2;
		}).then(v => {
			return v + 5;
		}).then(v => {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("Promise.try chaining test failed: %v", err)
	}

	// Allow microtasks to run
	time.Sleep(50 * time.Millisecond)

	resultVal := runtime.Get("result")
	if resultVal.Export() != int64(25) {
		t.Errorf("Expected 25, got %v", resultVal.Export())
	}

	_ = loop.Shutdown(context.Background())
	<-done
}

// TestPromiseTry_Finally tests Promise.try() with finally.
func TestPromiseTry_Finally(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	_, err = runtime.RunString(`
		let finallyCalled = false;
		let result;
		
		Promise.try(() => {
			return "success";
		}).finally(() => {
			finallyCalled = true;
		}).then(v => {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("Promise.try finally test failed: %v", err)
	}

	// Allow microtasks to run
	time.Sleep(50 * time.Millisecond)

	finallyCalled := runtime.Get("finallyCalled")
	if finallyCalled.Export() != true {
		t.Error("Finally should have been called")
	}

	resultVal := runtime.Get("result")
	if resultVal.Export() != "success" {
		t.Errorf("Expected 'success', got %v", resultVal.Export())
	}

	_ = loop.Shutdown(context.Background())
	<-done
}
