package gojaeventloop

import (
	"context"
	"sync"
	"testing"
	"time"

	eventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// Native Promise integration coverage lives here. Owner-safe Go-to-JavaScript
// settlement coverage lives in promisebridge_test.go.

// TestIntegration_ErrorPropagation_JSToGo tests that JavaScript errors are properly
// propagated to Go-side promise rejection handlers.
func TestIntegration_ErrorPropagation_JSToGo(t *testing.T) {
	loop := eventloop.New()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gojaRT := goja.New()
	adapter, err := New(loop, gojaRT)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan string, 1)
	if err := gojaRT.Set("notifyDone", func(message string) { done <- message }); err != nil {
		t.Fatalf("Failed to set notifyDone: %v", err)
	}

	// Create a native JS promise and observe the rejection through a JS catch
	// handler that calls into Go. The adapter no longer replaces Goja's native
	// Promise with the lower-level eventloop.ChainedPromise wrapper.
	_, err = gojaRT.RunString(`
		new Promise(function(resolve, reject) {
			resolve(42);
		}).then(function(v) {
			throw new Error("JS error in handler");
		}).catch(function(err) {
			notifyDone(err.message);
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	select {
	case result := <-done:
		if result != "JS error in handler" {
			t.Errorf("Expected JS error message, got %q", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for JS error propagation")
	}

	cancel()
	<-loopDone
}

// TestIntegration_ThenableUnwrapping tests that thenable objects are properly
// unwrapped when resolved.
func TestIntegration_ThenableUnwrapping(t *testing.T) {
	loop := eventloop.New()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gojaRT := goja.New()
	adapter, err := New(loop, gojaRT)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan any, 1)
	gojaRT.Set("captureResult", func(call goja.FunctionCall) goja.Value {
		done <- call.Argument(0).Export()
		return goja.Undefined()
	})

	// Test with a custom thenable object
	_, err = gojaRT.RunString(`
		// Custom thenable that resolves to a value
		var thenable = {
			then: function(resolve, reject) {
				setTimeout(function() {
					resolve("thenable resolved value");
				}, 10);
			}
		};

		// Promise.resolve should unwrap thenable
		Promise.resolve(thenable).then(function(v) {
			captureResult(v);
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	select {
	case result := <-done:
		if result != "thenable resolved value" {
			t.Errorf("Expected 'thenable resolved value', got %v", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for thenable unwrapping")
	}

	cancel()
	<-loopDone
}

// TestIntegration_NestedPromiseChaining tests deeply nested promise chains.
func TestIntegration_NestedPromiseChaining(t *testing.T) {
	loop := eventloop.New()
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gojaRT := goja.New()
	adapter, err := New(loop, gojaRT)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan int64, 1)
	gojaRT.Set("captureResult", func(call goja.FunctionCall) goja.Value {
		done <- call.Argument(0).ToInteger()
		return goja.Undefined()
	})

	// Create nested promise chains
	// IMPORTANT: All Goja runtime access (RunString, Set, etc.) must complete
	// BEFORE starting the event loop to avoid concurrent access to the runtime.
	// The loop processes callbacks on its own goroutine, which would race with
	// the test goroutine if RunString and loop.Run() execute concurrently.
	_, err = gojaRT.RunString(`
		Promise.resolve(0)
			.then(function(v) {
				return new Promise(function(resolve) {
					setTimeout(function() {
						resolve(v + 1);
					}, 5);
				});
			})
			.then(function(v) {
				return new Promise(function(resolve) {
					setTimeout(function() {
						resolve(v + 1);
					}, 5);
				});
			})
			.then(function(v) {
				return new Promise(function(resolve) {
					setTimeout(function() {
						resolve(v + 1);
					}, 5);
				});
			})
			.then(function(v) {
				captureResult(v);
			});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	// Start the event loop AFTER all JavaScript setup is complete.
	// This ensures the Goja runtime is only accessed from the loop goroutine
	// during callback execution (timer handlers, promise handlers, etc.)
	go func() { _ = loop.Run(ctx) }()

	select {
	case result := <-done:
		if result != 3 {
			t.Errorf("Expected 3, got %d", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for nested chains")
	}
}

// TestIntegration_IteratorConsumption tests that iterables are properly consumed.
func TestIntegration_IteratorConsumption(t *testing.T) {
	loop := eventloop.New()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gojaRT := goja.New()
	adapter, err := New(loop, gojaRT)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan int64, 1)
	gojaRT.Set("captureResult", func(call goja.FunctionCall) goja.Value {
		done <- call.Argument(0).ToInteger()
		return goja.Undefined()
	})

	// Test Promise.all with Set (iterable)
	_, err = gojaRT.RunString(`
		var set = new Set([
			Promise.resolve(1),
			Promise.resolve(2),
			Promise.resolve(3)
		]);
		Promise.all(set).then(function(values) {
			var sum = 0;
			for (var i = 0; i < values.length; i++) {
				sum += values[i];
			}
			captureResult(sum);
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	select {
	case result := <-done:
		if result != 6 {
			t.Errorf("Expected 6, got %d", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for Set iteration")
	}

	cancel()
	<-loopDone
}

// TestIntegration_PromiseFinally tests finally handler behavior.
func TestIntegration_PromiseFinally(t *testing.T) {
	loop := eventloop.New()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gojaRT := goja.New()
	adapter, err := New(loop, gojaRT)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan []string, 1)
	var steps []string
	var stepsMu sync.Mutex

	gojaRT.Set("addStep", func(call goja.FunctionCall) goja.Value {
		stepsMu.Lock()
		steps = append(steps, call.Argument(0).String())
		stepsMu.Unlock()
		return goja.Undefined()
	})

	gojaRT.Set("signalDone", func(call goja.FunctionCall) goja.Value {
		stepsMu.Lock()
		done <- steps
		stepsMu.Unlock()
		return goja.Undefined()
	})

	// Test finally preserves value
	_, err = gojaRT.RunString(`
		Promise.resolve("value")
			.finally(function() {
				addStep("finally1");
			})
			.then(function(v) {
				addStep("then:" + v);
			})
			.finally(function() {
				addStep("finally2");
				signalDone();
			});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	select {
	case result := <-done:
		expected := []string{"finally1", "then:value", "finally2"}
		if len(result) != len(expected) {
			t.Errorf("Expected steps %v, got %v", expected, result)
		} else {
			for i, s := range expected {
				if result[i] != s {
					t.Errorf("Step %d: expected %s, got %s", i, s, result[i])
				}
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for finally")
	}

	cancel()
	<-loopDone
}

// TestIntegration_WithResolvers tests Promise.withResolvers() ES2024 API.
func TestIntegration_WithResolvers(t *testing.T) {
	loop := eventloop.New()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gojaRT := goja.New()
	adapter, err := New(loop, gojaRT)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan int64, 1)
	gojaRT.Set("captureResult", func(call goja.FunctionCall) goja.Value {
		done <- call.Argument(0).ToInteger()
		return goja.Undefined()
	})

	_, err = gojaRT.RunString(`
		var resolvers = Promise.withResolvers();
		resolvers.promise.then(function(v) {
			captureResult(v);
		});
		// Resolve from JavaScript
		resolvers.resolve(42);
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	select {
	case result := <-done:
		if result != 42 {
			t.Errorf("Expected 42, got %d", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for withResolvers")
	}

	cancel()
	<-loopDone
}

// TestIntegration_AbortController tests AbortController/AbortSignal integration.
func TestIntegration_AbortController(t *testing.T) {
	loop := eventloop.New()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gojaRT := goja.New()
	adapter, err := New(loop, gojaRT)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan bool, 1)
	gojaRT.Set("captureAbortState", func(call goja.FunctionCall) goja.Value {
		done <- call.Argument(0).ToBoolean()
		return goja.Undefined()
	})

	_, err = gojaRT.RunString(`
		var controller = new AbortController();
		var signal = controller.signal;

		// Check initial state
		if (signal.aborted) {
			throw new Error("Signal should not be aborted initially");
		}

		// Abort and check state
		controller.abort();
		captureAbortState(signal.aborted);
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	select {
	case aborted := <-done:
		if !aborted {
			t.Error("Expected signal to be aborted")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for AbortController")
	}

	cancel()
	<-loopDone
}

// TestIntegration_Timers tests setTimeout/setInterval integration.
func TestIntegration_Timers(t *testing.T) {
	loop := eventloop.New()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gojaRT := goja.New()
	adapter, err := New(loop, gojaRT)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan int64, 1)
	gojaRT.Set("captureCount", func(call goja.FunctionCall) goja.Value {
		done <- call.Argument(0).ToInteger()
		return goja.Undefined()
	})

	_, err = gojaRT.RunString(`
		var count = 0;
		var id = setInterval(function() {
			count++;
			if (count >= 3) {
				clearInterval(id);
				captureCount(count);
			}
		}, 10);
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	select {
	case count := <-done:
		if count != 3 {
			t.Errorf("Expected count 3, got %d", count)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for interval")
	}

	cancel()
	<-loopDone
}

// TestIntegration_QueueMicrotask tests queueMicrotask integration.
func TestIntegration_QueueMicrotask(t *testing.T) {
	loop := eventloop.New()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gojaRT := goja.New()
	adapter, err := New(loop, gojaRT)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan []string, 1)
	var order []string
	var orderMu sync.Mutex

	gojaRT.Set("addToOrder", func(call goja.FunctionCall) goja.Value {
		orderMu.Lock()
		order = append(order, call.Argument(0).String())
		orderMu.Unlock()
		return goja.Undefined()
	})

	gojaRT.Set("signalDone", func(call goja.FunctionCall) goja.Value {
		orderMu.Lock()
		done <- order
		orderMu.Unlock()
		return goja.Undefined()
	})

	// Microtasks should execute before setTimeout
	_, err = gojaRT.RunString(`
		setTimeout(function() {
			addToOrder("timeout");
			signalDone();
		}, 0);

		queueMicrotask(function() {
			addToOrder("microtask1");
		});

		queueMicrotask(function() {
			addToOrder("microtask2");
		});

		addToOrder("sync");
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	select {
	case result := <-done:
		expected := []string{"sync", "microtask1", "microtask2", "timeout"}
		if len(result) != len(expected) {
			t.Errorf("Expected order %v, got %v", expected, result)
		} else {
			for i, s := range expected {
				if result[i] != s {
					t.Errorf("Order %d: expected %s, got %s", i, s, result[i])
				}
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for microtask ordering")
	}

	cancel()
	<-loopDone
}

// TestIntegration_LargePromiseChain tests that promise chains work correctly.
func TestIntegration_LargePromiseChain(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chain test in short mode")
	}

	loop := eventloop.New()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	gojaRuntime := goja.New()
	adapter, err := New(loop, gojaRuntime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan int64, 1)
	gojaRuntime.Set("captureResult", func(call goja.FunctionCall) goja.Value {
		done <- call.Argument(0).ToInteger()
		return goja.Undefined()
	})

	// Test a simpler chain of 5 promises
	_, err = gojaRuntime.RunString(`
		Promise.resolve(0)
			.then(function(v) { return v + 1; })
			.then(function(v) { return v + 1; })
			.then(function(v) { return v + 1; })
			.then(function(v) { return v + 1; })
			.then(function(v) { return v + 1; })
			.then(function(v) { captureResult(v); });
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	// Start the event loop AFTER all runtime access is complete
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	select {
	case result := <-done:
		if result != 5 {
			t.Errorf("Expected 5, got %d", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for chain")
	}

	cancel()
	<-loopDone
}
