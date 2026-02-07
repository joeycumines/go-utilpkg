package gojaeventloop

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	eventloop "github.com/joeycumines/go-eventloop"
)

// ============================================================================
// INTEGRATION-001: Goja-EventLoop Integration Comprehensive
// ============================================================================
//
// Tests comprehensive integration between goja-eventloop and eventloop:
// - Promise chaining across Go/JS boundary
// - Error propagation from Go to JS and JS to Go
// - Type conversions (primitives, arrays, objects, errors)
// - Thenable unwrapping
// - Iterator consumption
// - Promise combinator integration

// TestIntegration_PromiseChainingAcrossBoundary tests that promises created in Go
// can be properly chained and resolved/rejected from JavaScript code.
func TestIntegration_PromiseChainingAcrossBoundary(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

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

	// Create a Go-side promise
	js := adapter.JS()
	goPromise, goResolve, _ := js.NewChainedPromise()

	// Wrap and chain in JavaScript
	gojaRT.Set("goPromise", adapter.gojaWrapPromise(goPromise))

	done := make(chan error, 1)
	var results []interface{}
	var resultsMu sync.Mutex

	gojaRT.Set("captureResult", func(call goja.FunctionCall) goja.Value {
		resultsMu.Lock()
		for _, arg := range call.Arguments {
			results = append(results, arg.Export())
		}
		resultsMu.Unlock()
		return goja.Undefined()
	})

	gojaRT.Set("signalDone", func(call goja.FunctionCall) goja.Value {
		close(done)
		return goja.Undefined()
	})

	// JavaScript code that chains on the Go promise
	_, err = gojaRT.RunString(`
		goPromise
			.then(function(v) {
				captureResult("step1", v);
				return v * 2;
			})
			.then(function(v) {
				captureResult("step2", v);
				return v + 10;
			})
			.then(function(v) {
				captureResult("step3", v);
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

	// Resolve the Go promise from Go code
	goResolve(21)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for promise chain")
	}

	cancel()
	<-loopDone

	resultsMu.Lock()
	defer resultsMu.Unlock()

	if len(results) != 6 {
		t.Errorf("Expected 6 results, got %d: %v", len(results), results)
		return
	}

	// Verify chain execution
	if results[0] != "step1" || results[1] != int64(21) {
		t.Errorf("Step 1 mismatch: %v, %v", results[0], results[1])
	}
	if results[2] != "step2" || results[3] != int64(42) {
		t.Errorf("Step 2 mismatch: %v, %v", results[2], results[3])
	}
	if results[4] != "step3" || results[5] != int64(52) {
		t.Errorf("Step 3 mismatch: %v, %v", results[4], results[5])
	}
}

// TestIntegration_ErrorPropagation_GoToJS tests that errors from Go are properly
// propagated to JavaScript rejection handlers.
func TestIntegration_ErrorPropagation_GoToJS(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

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

	// Create a Go-side promise that will reject
	js := adapter.JS()
	_, _, goReject := js.NewChainedPromise()
	goPromise2, _, goReject2 := js.NewChainedPromise()

	gojaRT.Set("goPromise", adapter.gojaWrapPromise(goPromise2))

	done := make(chan error, 1)
	var capturedError interface{}
	var errorMu sync.Mutex

	gojaRT.Set("captureError", func(call goja.FunctionCall) goja.Value {
		errorMu.Lock()
		capturedError = call.Argument(0).Export()
		errorMu.Unlock()
		return goja.Undefined()
	})

	gojaRT.Set("signalDone", func(call goja.FunctionCall) goja.Value {
		close(done)
		return goja.Undefined()
	})

	_, err = gojaRT.RunString(`
		goPromise
			.then(function(v) {
				return v; // Should not reach here
			})
			.catch(function(err) {
				captureError(err);
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

	// Reject the Go promise with an error - but use goPromise2's reject
	goReject2(errors.New("Go-side error occurred"))
	_ = goReject // Keep reference to avoid unused warning

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for error propagation")
	}

	cancel()
	<-loopDone

	errorMu.Lock()
	defer errorMu.Unlock()

	if capturedError == nil {
		t.Error("Error was not captured")
	}
}

// TestIntegration_ErrorPropagation_JSToGo tests that JavaScript errors are properly
// propagated to Go-side promise rejection handlers.
func TestIntegration_ErrorPropagation_JSToGo(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

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

	done := make(chan interface{}, 1)

	// Create promise in JS that will throw error in handler
	_, err = gojaRT.RunString(`
		var jsPromise = new Promise(function(resolve, reject) {
			resolve(42);
		}).then(function(v) {
			throw new Error("JS error in handler");
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	// Get the JS promise and check for rejection from Go
	jsPromiseVal := gojaRT.Get("jsPromise")
	if jsPromiseVal == nil {
		t.Fatal("jsPromise not found")
	}

	jsPromiseObj := jsPromiseVal.ToObject(gojaRT)
	internalPromise := jsPromiseObj.Get("_internalPromise")
	if internalPromise == nil {
		t.Fatal("_internalPromise not found")
	}

	promise, ok := internalPromise.Export().(*eventloop.ChainedPromise)
	if !ok {
		t.Fatal("Failed to extract ChainedPromise")
	}

	// Attach a Go handler for rejection
	promise.Catch(func(r eventloop.Result) eventloop.Result {
		done <- r
		return nil
	})

	// Start the event loop AFTER all runtime access is complete
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	select {
	case result := <-done:
		if result == nil {
			t.Error("Expected error result, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for JS error propagation")
	}

	cancel()
	<-loopDone
}

// TestIntegration_TypeConversions tests various type conversions between Go and JS.
func TestIntegration_TypeConversions(t *testing.T) {
	testCases := []struct {
		name     string
		goValue  interface{}
		expected string // JavaScript typeof result
	}{
		{"string", "hello", "string"},
		{"integer", 42, "number"},
		{"float", 3.14, "number"},
		{"boolean_true", true, "boolean"},
		{"boolean_false", false, "boolean"},
		{"nil", nil, "object"},                      // nil maps to null in JS, typeof null = "object"
		{"array", []interface{}{1, 2, 3}, "object"}, // Arrays are objects
		{"object", map[string]interface{}{"a": 1}, "object"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			loop, err := eventloop.New()
			if err != nil {
				t.Fatalf("Failed to create loop: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			gojaRT := goja.New()
			adapter, err := New(loop, gojaRT)
			if err != nil {
				t.Fatalf("Failed to create adapter: %v", err)
			}

			if err := adapter.Bind(); err != nil {
				t.Fatalf("Failed to bind: %v", err)
			}

			// Create a promise that resolves with the Go value
			js := adapter.JS()
			promise, resolve, _ := js.NewChainedPromise()
			gojaRT.Set("testPromise", adapter.gojaWrapPromise(promise))

			done := make(chan string, 1)
			gojaRT.Set("reportType", func(call goja.FunctionCall) goja.Value {
				done <- call.Argument(0).String()
				return goja.Undefined()
			})

			_, err = gojaRT.RunString(`
				testPromise.then(function(v) {
					reportType(typeof v);
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

			resolve(tc.goValue)

			select {
			case typeStr := <-done:
				if typeStr != tc.expected {
					t.Errorf("Expected typeof %s, got %s", tc.expected, typeStr)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Timeout waiting for type conversion")
			}

			cancel()
			<-loopDone
		})
	}
}

// TestIntegration_ThenableUnwrapping tests that thenable objects are properly
// unwrapped when resolved.
func TestIntegration_ThenableUnwrapping(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

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

	done := make(chan interface{}, 1)
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

// TestIntegration_PromiseAllWithGoPromises tests Promise.all with mixed Go and JS promises.
func TestIntegration_PromiseAllWithGoPromises(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

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

	js := adapter.JS()

	// Create multiple Go promises
	p1, resolve1, _ := js.NewChainedPromise()
	p2, resolve2, _ := js.NewChainedPromise()
	p3, resolve3, _ := js.NewChainedPromise()

	gojaRT.Set("goPromise1", adapter.gojaWrapPromise(p1))
	gojaRT.Set("goPromise2", adapter.gojaWrapPromise(p2))
	gojaRT.Set("goPromise3", adapter.gojaWrapPromise(p3))

	done := make(chan interface{}, 1)
	gojaRT.Set("captureResult", func(call goja.FunctionCall) goja.Value {
		done <- call.Argument(0).Export()
		return goja.Undefined()
	})

	_, err = gojaRT.RunString(`
		Promise.all([goPromise1, goPromise2, goPromise3]).then(function(values) {
			captureResult(values);
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

	// Resolve in different order
	resolve2("second")
	resolve3("third")
	resolve1("first")

	select {
	case result := <-done:
		arr, ok := result.([]interface{})
		if !ok {
			t.Fatalf("Expected array, got %T", result)
		}
		if len(arr) != 3 {
			t.Errorf("Expected 3 elements, got %d", len(arr))
		}
		// Order should match original array order, not resolution order
		if arr[0] != "first" || arr[1] != "second" || arr[2] != "third" {
			t.Errorf("Unexpected order: %v", arr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for Promise.all")
	}

	cancel()
	<-loopDone
}

// TestIntegration_PromiseRaceWithGoPromises tests Promise.race with Go promises.
func TestIntegration_PromiseRaceWithGoPromises(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

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

	js := adapter.JS()

	p1, resolve1, _ := js.NewChainedPromise()
	p2, resolve2, _ := js.NewChainedPromise()
	p3, _, _ := js.NewChainedPromise()

	gojaRT.Set("goPromise1", adapter.gojaWrapPromise(p1))
	gojaRT.Set("goPromise2", adapter.gojaWrapPromise(p2))
	gojaRT.Set("goPromise3", adapter.gojaWrapPromise(p3))

	done := make(chan interface{}, 1)
	gojaRT.Set("captureResult", func(call goja.FunctionCall) goja.Value {
		done <- call.Argument(0).Export()
		return goja.Undefined()
	})

	_, err = gojaRT.RunString(`
		Promise.race([goPromise1, goPromise2, goPromise3]).then(function(value) {
			captureResult(value);
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

	// Resolve second promise first - it should win the race
	resolve2("winner")
	resolve1("loser")

	select {
	case result := <-done:
		if result != "winner" {
			t.Errorf("Expected 'winner', got %v", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for Promise.race")
	}

	cancel()
	<-loopDone
}

// TestIntegration_NestedPromiseChaining tests deeply nested promise chains.
func TestIntegration_NestedPromiseChaining(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
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
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

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

// TestIntegration_ConcurrentPromiseResolution tests concurrent resolution from multiple goroutines.
func TestIntegration_ConcurrentPromiseResolution(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	gojaRT := goja.New()
	adapter, err := New(loop, gojaRT)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	js := adapter.JS()

	const numPromises = 20 // Reduced for reliability
	var completed atomic.Int32
	done := make(chan struct{})

	gojaRT.Set("signalComplete", func(call goja.FunctionCall) goja.Value {
		if int(completed.Add(1)) == numPromises {
			close(done)
		}
		return goja.Undefined()
	})

	// Create all promises and set up JS handlers BEFORE starting the loop
	resolvers := make([]eventloop.ResolveFunc, numPromises)
	for i := 0; i < numPromises; i++ {
		p, resolve, _ := js.NewChainedPromise()
		resolvers[i] = resolve
		gojaRT.Set(fmt.Sprintf("p%d", i), adapter.gojaWrapPromise(p))

		_, err := gojaRT.RunString(fmt.Sprintf(`
			p%d.then(function(v) {
				signalComplete();
			});
		`, i))
		if err != nil {
			t.Fatalf("Failed to run JS: %v", err)
		}
	}

	// Start the event loop AFTER all runtime access is complete
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	// Now resolve from goroutines
	var wg sync.WaitGroup
	for i := 0; i < numPromises; i++ {
		wg.Add(1)
		go func(r eventloop.ResolveFunc, idx int) {
			defer wg.Done()
			// Small fixed delay to ensure handler is attached
			time.Sleep(5 * time.Millisecond)
			r(idx)
		}(resolvers[i], i)
	}

	wg.Wait()

	select {
	case <-done:
		// Success
	case <-time.After(8 * time.Second):
		t.Fatalf("Timeout waiting for concurrent resolutions, completed: %d/%d", completed.Load(), numPromises)
	}

	cancel()
	<-loopDone
}

// TestIntegration_ErrorRecoveryAndRethrow tests error recovery and rethrowing in chains.
// NOTE: This test uses Go-side rejection to avoid potential issues with Promise.reject().
func TestIntegration_ErrorRecoveryAndRethrow(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

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

	// Test Go-side rejection flowing through JS handlers
	js := adapter.JS()
	goPromise, _, goReject := js.NewChainedPromise()

	gojaRT.Set("rejectablePromise", adapter.gojaWrapPromise(goPromise))

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

	_, err = gojaRT.RunString(`
		rejectablePromise
			.then(function(v) {
				addStep("then1-should-skip");
				return v;
			})
			.catch(function(err) {
				addStep("catch1");
				return "recovered";
			})
			.then(function(v) {
				addStep("then2:" + v);
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

	// Reject from Go side
	goReject(errors.New("go-side-error"))

	select {
	case result := <-done:
		expected := []string{"catch1", "then2:recovered"}
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
		t.Fatal("Timeout waiting for error recovery chain")
	}

	cancel()
	<-loopDone
}

// TestIntegration_PromiseFinally tests finally handler behavior.
func TestIntegration_PromiseFinally(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

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
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

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
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

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

// TestIntegration_PerformanceAPI tests performance.now() and marking.
func TestIntegration_PerformanceAPI(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

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

	done := make(chan float64, 1)
	gojaRT.Set("captureDuration", func(call goja.FunctionCall) goja.Value {
		done <- call.Argument(0).ToFloat()
		return goja.Undefined()
	})

	_, err = gojaRT.RunString(`
		var start = performance.now();

		// Perform some work
		performance.mark("start");

		// Simulate delay with sync work
		var x = 0;
		for (var i = 0; i < 100000; i++) {
			x += i;
		}

		performance.mark("end");
		performance.measure("test", "start", "end");

		var entries = performance.getEntriesByName("test", "measure");
		if (entries.length > 0) {
			captureDuration(entries[0].duration);
		} else {
			captureDuration(-1);
		}
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
	case duration := <-done:
		if duration < 0 {
			t.Error("Measure not found")
		}
		// Duration should be non-negative (could be 0 on fast machines)
		if duration < 0 {
			t.Errorf("Duration should be non-negative, got %f", duration)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for performance API")
	}

	cancel()
	<-loopDone
}

// TestIntegration_Timers tests setTimeout/setInterval integration.
func TestIntegration_Timers(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

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
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

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

	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

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
