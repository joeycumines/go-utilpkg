package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// Promise.try() Tests
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

	resultCh := make(chan any, 1)
	runtime.Set("captureResult", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0).Export()
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		Promise.try(() => {
			return 42;
		}).then(v => {
			captureResult(v);
		});
	`)
	if err != nil {
		t.Fatalf("Promise.try success test failed: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	select {
	case result := <-resultCh:
		if result != int64(42) {
			t.Errorf("Expected 42, got %v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for result")
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

	caughtCh := make(chan bool, 1)
	runtime.Set("captureCaught", func(call goja.FunctionCall) goja.Value {
		caughtCh <- call.Argument(0).ToBoolean()
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		Promise.try(() => {
			throw new Error("test error");
		}).catch(err => {
			captureCaught(true);
		});
	`)
	if err != nil {
		t.Fatalf("Promise.try throws test failed: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	select {
	case caught := <-caughtCh:
		if !caught {
			t.Error("Error should have been caught")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for catch")
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

	resultCh := make(chan any, 1)
	runtime.Set("captureResult", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0).Export()
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		Promise.try(() => {
			return Promise.resolve(100);
		}).then(v => {
			captureResult(v);
		});
	`)
	if err != nil {
		t.Fatalf("Promise.try returns promise test failed: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	select {
	case result := <-resultCh:
		if result != int64(100) {
			t.Errorf("Expected 100, got %v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for result")
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

	type nullResult struct {
		thenCalled bool
		result     any
	}
	resultCh := make(chan nullResult, 1)
	runtime.Set("captureNullResult", func(call goja.FunctionCall) goja.Value {
		resultCh <- nullResult{
			thenCalled: true,
			result:     call.Argument(0).Export(),
		}
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		Promise.try(() => {
			return null;
		}).then(v => {
			captureNullResult(v);
		});
	`)
	if err != nil {
		t.Fatalf("Promise.try returns null test failed: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	select {
	case nr := <-resultCh:
		if !nr.thenCalled {
			t.Error("Then should have been called")
		}
		if nr.result != nil {
			t.Errorf("Expected null, got %v", nr.result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for result")
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

	resultCh := make(chan any, 1)
	runtime.Set("captureResult", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0).Export()
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		Promise.try(() => {
			return 10;
		}).then(v => {
			return v * 2;
		}).then(v => {
			return v + 5;
		}).then(v => {
			captureResult(v);
		});
	`)
	if err != nil {
		t.Fatalf("Promise.try chaining test failed: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	select {
	case result := <-resultCh:
		if result != int64(25) {
			t.Errorf("Expected 25, got %v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for result")
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

	type finallyResult struct {
		finallyCalled bool
		result        any
	}
	resultCh := make(chan finallyResult, 1)
	runtime.Set("captureFinallyResult", func(call goja.FunctionCall) goja.Value {
		resultCh <- finallyResult{
			finallyCalled: call.Argument(0).ToBoolean(),
			result:        call.Argument(1).Export(),
		}
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		let finallyCalled = false;

		Promise.try(() => {
			return "success";
		}).finally(() => {
			finallyCalled = true;
		}).then(v => {
			captureFinallyResult(finallyCalled, v);
		});
	`)
	if err != nil {
		t.Fatalf("Promise.try finally test failed: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	select {
	case fr := <-resultCh:
		if !fr.finallyCalled {
			t.Error("Finally should have been called")
		}
		if fr.result != "success" {
			t.Errorf("Expected 'success', got %v", fr.result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for result")
	}

	_ = loop.Shutdown(context.Background())
	<-done
}
