package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// TestPromiseRejectPreservesPromiseIdentity verifies that Promise.reject(promise)
// returns the promise object itself (not unwrapped value), per JS spec
func TestPromiseRejectPreservesPromiseIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

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
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	// Test: Promise.reject(p1) where p1 is a promise
	testDone := make(chan bool, 1)
	_ = runtime.Set("testDone", func() {
		testDone <- true
	})

	// Set up console for debugging
	console := runtime.NewObject()
	runtime.Set("console", console)
	_ = console.Set("log", func(call goja.FunctionCall) goja.Value {
		for _, arg := range call.Arguments {
			t.Log(arg.ToString())
		}
		return goja.Undefined()
	})
	_ = console.Set("error", func(call goja.FunctionCall) goja.Value {
		for _, arg := range call.Arguments {
			t.Logf("ERROR: %s", arg.ToString())
		}
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		const p1 = Promise.resolve(42);
		const p2 = Promise.reject(p1);

		p2.catch(reason => {
			// SPEC REQUIREMENT: reason should be the promise object, not 42
			console.log("Reason type:", typeof reason);
			console.log("Reason has .then:", typeof reason.then === 'function');
			console.log("Reason has .catch:", typeof reason.catch === 'function');

			// The catch handler verifies it's a promise
			if (typeof reason.then === 'function') {
				testDone();
			} else {
				console.error("FAIL: reason is not a promise!");
			}
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-testDone:
		t.Log("Test passed: Promise.reject(promise) preserved identity")
	case <-ctx.Done():
		t.Fatal("Timeout waiting for promise rejection to complete")
	}
}

// TestPromiseRejectPreservesErrorProperties verifies that Promise.reject(error)
// preserves error properties like .code, .customProperty, etc.
func TestPromiseRejectPreservesErrorProperties(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

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
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	// Test: Promise.reject(error) with custom properties
	testDone := make(chan bool, 1)
	_ = runtime.Set("testDone", func() {
		testDone <- true
	})

	// Set up console for debugging
	console := runtime.NewObject()
	runtime.Set("console", console)
	_ = console.Set("log", func(call goja.FunctionCall) goja.Value {
		for _, arg := range call.Arguments {
			t.Log(arg.ToString())
		}
		return goja.Undefined()
	})
	_ = console.Set("error", func(call goja.FunctionCall) goja.Value {
		for _, arg := range call.Arguments {
			t.Logf("ERROR: %s", arg.ToString())
		}
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		const err = new Error('custom error');
		err.code = 42;
		err.customProperty = 'test data';

		const p = Promise.reject(err);

		p.catch(reason => {
			console.log("Reason type:", typeof reason);
			console.log("Reason instanceof Error:", reason instanceof Error);
			console.log("Reason.message:", reason.message);
			console.log("Reason.code:", reason.code);
			console.log("Reason.customProperty:", reason.customProperty);

			// Verify all properties are preserved
			if (reason instanceof Error &&
			    reason.message === 'custom error' &&
			    reason.code === 42 &&
			    reason.customProperty === 'test data') {
				testDone();
			} else {
				console.error("FAIL: Error properties not preserved!");
			}
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-testDone:
		t.Log("Test passed: Error properties preserved correctly")
	case <-ctx.Done():
		t.Fatal("Timeout waiting for promise rejection to complete")
	}
}
