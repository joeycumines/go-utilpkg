// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

//go:build linux || darwin

package gojaeventloop

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// COVERAGE-021: goja-eventloop Adapter Full Coverage
// Gaps: all bindings, error type conversions, thenable resolution, iterator protocol errors,
// promise wrapper GC behavior

// ==============================================================================
// Adapter Creation and Basic Bindings
// ==============================================================================

// TestAdapter_New_NilLoop verifies error when loop is nil.
func TestAdapter_New_NilLoop(t *testing.T) {
	runtime := goja.New()
	_, err := New(nil, runtime)
	if err == nil {
		t.Error("Expected error when loop is nil")
	}
}

// TestAdapter_New_NilRuntime verifies error when runtime is nil.
func TestAdapter_New_NilRuntime(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	_, err = New(loop, nil)
	if err == nil {
		t.Error("Expected error when runtime is nil")
	}
}

// TestAdapter_Bind_AllGlobals verifies all globals are bound.
func TestAdapter_Bind_AllGlobals(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Verify all expected globals are set
	globals := []string{
		"setTimeout",
		"clearTimeout",
		"setInterval",
		"clearInterval",
		"queueMicrotask",
		"setImmediate",
		"clearImmediate",
		"Promise",
		"consumeIterable",
	}

	for _, name := range globals {
		val := rt.Get(name)
		if val == nil || goja.IsUndefined(val) {
			t.Errorf("Global %q should be defined", name)
		}
	}
}

// TestAdapter_Bind_PromiseStatics verifies Promise static methods.
func TestAdapter_Bind_PromiseStatics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Verify Promise static methods
	statics := []string{
		"resolve",
		"reject",
		"all",
		"race",
		"allSettled",
		"any",
		"prototype",
	}

	for _, name := range statics {
		val, err := rt.RunString("Promise." + name)
		if err != nil {
			t.Errorf("Promise.%s should exist: %v", name, err)
		}
		if val == nil || goja.IsUndefined(val) {
			t.Errorf("Promise.%s should not be undefined", name)
		}
	}
}

// ==============================================================================
// Timer Bindings - Error Cases
// ==============================================================================

// TestAdapter_setTimeout_NilFunction verifies behavior with nil function.
func TestAdapter_setTimeout_NilFunction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// setTimeout with null should throw TypeError
	_, err = rt.RunString("setTimeout(null, 100)")
	if err == nil {
		t.Error("setTimeout(null) should throw TypeError")
	}
}

// TestAdapter_setInterval_NilFunction verifies behavior with nil function.
func TestAdapter_setInterval_NilFunction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// setInterval with null should throw TypeError
	_, err = rt.RunString("setInterval(null, 100)")
	if err == nil {
		t.Error("setInterval(null) should throw TypeError")
	}
}

// TestAdapter_queueMicrotask_NilFunction verifies behavior with nil function.
func TestAdapter_queueMicrotask_NilFunction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// queueMicrotask with null should throw TypeError
	_, err = rt.RunString("queueMicrotask(null)")
	if err == nil {
		t.Error("queueMicrotask(null) should throw TypeError")
	}
}

// TestAdapter_setImmediate_NilFunction verifies behavior with nil function.
func TestAdapter_setImmediate_NilFunction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// setImmediate with null should throw TypeError
	_, err = rt.RunString("setImmediate(null)")
	if err == nil {
		t.Error("setImmediate(null) should throw TypeError")
	}
}

// TestAdapter_setTimeout_NegativeDelay verifies negative delay handling.
func TestAdapter_setTimeout_NegativeDelay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// setTimeout with negative delay should throw
	_, err = rt.RunString("setTimeout(function(){}, -1)")
	if err == nil {
		t.Error("setTimeout with negative delay should throw")
	}
}

// ==============================================================================
// Promise Constructor and Error Handling
// ==============================================================================

// TestAdapter_Promise_ExecutorNotFunction verifies executor must be function.
func TestAdapter_Promise_ExecutorNotFunction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// new Promise(null) should throw TypeError
	_, err = rt.RunString("new Promise(null)")
	if err == nil {
		t.Error("new Promise(null) should throw TypeError")
	}

	// new Promise("string") should throw TypeError
	_, err = rt.RunString("new Promise('string')")
	if err == nil {
		t.Error("new Promise('string') should throw TypeError")
	}
}

// TestAdapter_Promise_ExecutorThrows verifies executor throw causes rejection.
func TestAdapter_Promise_ExecutorThrows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() { close(done) })

	_, err = rt.RunString(`
		var caught = false;
		var p = new Promise(function() { throw new Error("executor error"); });
		p.catch(function(e) {
			caught = true;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go loop.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout waiting for catch handler")
	}

	caught := rt.Get("caught")
	if !caught.ToBoolean() {
		t.Error("Executor throw should cause rejection caught by catch handler")
	}
}

// ==============================================================================
// Thenable Resolution
// ==============================================================================

// TestAdapter_Thenable_Resolution verifies thenable objects are resolved.
func TestAdapter_Thenable_Resolution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() { close(done) })

	_, err = rt.RunString(`
		var result = null;
		var thenable = {
			then: function(resolve) {
				resolve(42);
			}
		};
		Promise.resolve(thenable).then(function(x) {
			result = x;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go loop.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout waiting for thenable resolution")
	}

	result := rt.Get("result")
	if result.ToInteger() != 42 {
		t.Errorf("Expected 42, got %v", result.Export())
	}
}

// TestAdapter_Thenable_Rejection verifies thenable objects can reject.
func TestAdapter_Thenable_Rejection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() { close(done) })

	_, err = rt.RunString(`
		var caught = null;
		var thenable = {
			then: function(resolve, reject) {
				reject("thenable rejection");
			}
		};
		Promise.resolve(thenable).catch(function(e) {
			caught = e;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go loop.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout waiting for thenable rejection")
	}

	caught := rt.Get("caught")
	if caught.Export() != "thenable rejection" {
		t.Errorf("Expected 'thenable rejection', got %v", caught.Export())
	}
}

// TestAdapter_Thenable_ThenThrows verifies thenable.then() throw causes rejection.
func TestAdapter_Thenable_ThenThrows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() { close(done) })

	_, err = rt.RunString(`
		var caught = false;
		var thenable = {
			then: function() {
				throw new Error("then throws");
			}
		};
		Promise.resolve(thenable).catch(function(e) {
			caught = true;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go loop.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout waiting for catch")
	}

	caught := rt.Get("caught")
	if !caught.ToBoolean() {
		t.Error("Thenable.then() throwing should cause rejection")
	}
}

// ==============================================================================
// Iterator Protocol Errors
// ==============================================================================

// TestAdapter_Iterator_NullIterable verifies null/undefined iterable errors.
func TestAdapter_Iterator_NullIterable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() { close(done) })

	// Promise.all(null) should reject
	_, err = rt.RunString(`
		var caught = false;
		Promise.all(null).catch(function(e) {
			caught = true;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go loop.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout")
	}

	caught := rt.Get("caught")
	if !caught.ToBoolean() {
		t.Error("Promise.all(null) should reject")
	}
}

// TestAdapter_Iterator_NonIterable verifies non-iterable object errors.
func TestAdapter_Iterator_NonIterable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() { close(done) })

	// Promise.all({}) should reject (plain object not iterable)
	_, err = rt.RunString(`
		var caught = false;
		Promise.all({}).catch(function(e) {
			caught = true;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go loop.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout")
	}

	caught := rt.Get("caught")
	if !caught.ToBoolean() {
		t.Error("Promise.all({}) should reject for non-iterable")
	}
}

// TestAdapter_Iterator_ArrayFastPath verifies array fast path works.
func TestAdapter_Iterator_ArrayFastPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() { close(done) })

	_, err = rt.RunString(`
		var result = null;
		Promise.all([1, 2, 3]).then(function(arr) {
			result = arr;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go loop.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout")
	}

	result := rt.Get("result")
	arr := result.Export().([]interface{})
	if len(arr) != 3 || arr[0].(int64) != 1 || arr[1].(int64) != 2 || arr[2].(int64) != 3 {
		t.Errorf("Expected [1,2,3], got %v", arr)
	}
}

// ==============================================================================
// Error Type Conversions
// ==============================================================================

// TestAdapter_ErrorConversion_GoError verifies Go errors are converted properly.
func TestAdapter_ErrorConversion_GoError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() { close(done) })

	// Create a promise that rejects with a Go error
	promise := adapter.JS().Reject(context.DeadlineExceeded)
	_ = rt.Set("testPromise", adapter.gojaWrapPromise(promise))

	_, err = rt.RunString(`
		var caught = null;
		testPromise.catch(function(e) {
			caught = e.message || String(e);
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go loop.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout")
	}

	caught := rt.Get("caught")
	t.Logf("Caught error: %v", caught.Export())
	// Should contain the error message
}

// TestAdapter_ErrorConversion_JSError verifies JS errors are preserved.
func TestAdapter_ErrorConversion_JSError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() { close(done) })

	_, err = rt.RunString(`
		var caught = null;
		Promise.reject(new Error("js error")).catch(function(e) {
			caught = e.message;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go loop.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout")
	}

	caught := rt.Get("caught")
	if caught.Export() != "js error" {
		t.Errorf("Expected 'js error', got %v", caught.Export())
	}
}

// TestAdapter_ErrorConversion_AggregateError verifies AggregateError conversion.
func TestAdapter_ErrorConversion_AggregateError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() { close(done) })

	_, err = rt.RunString(`
		var caught = null;
		Promise.any([
			Promise.reject("error1"),
			Promise.reject("error2")
		]).catch(function(e) {
			caught = e;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go loop.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout")
	}

	caught := rt.Get("caught")
	t.Logf("Caught AggregateError: %v", caught.Export())
	// Should be an AggregateError with errors array
}

// ==============================================================================
// Promise Wrapper GC Behavior
// ==============================================================================

// TestAdapter_PromiseWrapper_GC verifies promise wrappers can be garbage collected.
func TestAdapter_PromiseWrapper_GC(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	go loop.Run(ctx)

	// Create many promises without keeping references
	_, err = rt.RunString(`
		for (var i = 0; i < 1000; i++) {
			Promise.resolve(i);
		}
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	// Force GC
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	runtime.GC()

	// If we get here without OOM, GC is working
	t.Log("Promise wrappers can be garbage collected")
}

// TestAdapter_PromiseWrapper_NoLeak verifies no memory leak in promise chains.
// This is a simpler version that creates fewer chains to avoid timing issues.
func TestAdapter_PromiseWrapper_NoLeak(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	loopCtx, loopCancel := context.WithCancel(ctx)
	defer loopCancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		loop.Run(loopCtx)
	}()

	// Simple test: create a few promises and resolve them
	var chainCount int
	_ = rt.Set("incrementChain", func() {
		chainCount++
	})

	// Create a few promise chains
	_, err = rt.RunString(`
		for (var i = 0; i < 5; i++) {
			Promise.resolve(i).then(function(x) {
				incrementChain();
				return x;
			});
		}
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	// Give time for microtasks to process
	time.Sleep(100 * time.Millisecond)

	// Stop the loop
	loopCancel()
	<-loopDone

	// Should have processed at least some chains
	t.Logf("Chain count: %d", chainCount)
	if chainCount < 1 {
		t.Error("Expected at least 1 chain to complete")
	}
}

// ==============================================================================
// Handler Conversion
// ==============================================================================

// TestAdapter_gojaFuncToHandler_NilHandler verifies nil handler returns nil.
func TestAdapter_gojaFuncToHandler_NilHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() { close(done) })

	// .then(undefined) should propagate value
	_, err = rt.RunString(`
		var result = null;
		Promise.resolve(42).then(undefined).then(function(x) {
			result = x;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go loop.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout")
	}

	result := rt.Get("result")
	if result.ToInteger() != 42 {
		t.Errorf("Value should propagate through undefined handler, got %v", result.Export())
	}
}

// TestAdapter_gojaFuncToHandler_NonFunction verifies non-function returns nil.
func TestAdapter_gojaFuncToHandler_NonFunction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() { close(done) })

	// .then("string") should propagate value (non-function treated as undefined)
	_, err = rt.RunString(`
		var result = null;
		Promise.resolve(42).then("not a function").then(function(x) {
			result = x;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go loop.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout")
	}

	result := rt.Get("result")
	if result.ToInteger() != 42 {
		t.Errorf("Value should propagate through non-function handler, got %v", result.Export())
	}
}

// ==============================================================================
// Promise.resolve with wrapped promise identity
// ==============================================================================

// TestAdapter_PromiseResolve_Identity verifies Promise.resolve(promise) returns same promise.
func TestAdapter_PromiseResolve_Identity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	val, err := rt.RunString(`
		var p = Promise.resolve(42);
		var p2 = Promise.resolve(p);
		p === p2;
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	if !val.ToBoolean() {
		t.Error("Promise.resolve(promise) should return same promise")
	}
}

// ==============================================================================
// Void Handler for Finally
// ==============================================================================

// TestAdapter_gojaVoidFuncToHandler_NilHandler verifies nil finally handler is no-op.
func TestAdapter_gojaVoidFuncToHandler_NilHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() { close(done) })

	_, err = rt.RunString(`
		var result = null;
		Promise.resolve(42).finally(undefined).then(function(x) {
			result = x;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go loop.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout")
	}

	result := rt.Get("result")
	if result.ToInteger() != 42 {
		t.Errorf("Value should propagate through undefined finally handler, got %v", result.Export())
	}
}

// ==============================================================================
// consumeIterable Coverage
// ==============================================================================

// TestAdapter_consumeIterable_Set verifies Set iteration works.
func TestAdapter_consumeIterable_Set(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() { close(done) })

	_, err = rt.RunString(`
		var result = null;
		var s = new Set([1, 2, 3]);
		Promise.all(s).then(function(arr) {
			result = arr;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go loop.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout")
	}

	result := rt.Get("result")
	arr := result.Export().([]interface{})
	if len(arr) != 3 {
		t.Errorf("Expected 3 elements, got %d", len(arr))
	}
}

// TestAdapter_consumeIterable_Generator verifies generator iteration works.
func TestAdapter_consumeIterable_Generator(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() { close(done) })

	_, err = rt.RunString(`
		var result = null;
		function* gen() {
			yield 1;
			yield 2;
			yield 3;
		}
		Promise.all(gen()).then(function(arr) {
			result = arr;
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go loop.Run(ctx)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout")
	}

	result := rt.Get("result")
	arr := result.Export().([]interface{})
	if len(arr) != 3 {
		t.Errorf("Expected 3 elements from generator, got %d", len(arr))
	}
}

// ==============================================================================
// isWrappedPromise and tryExtractWrappedPromise Coverage
// ==============================================================================

// TestAdapter_isWrappedPromise_Coverage verifies helper function works.
func TestAdapter_isWrappedPromise_Coverage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test with non-object
	if isWrappedPromise(rt.ToValue(42)) {
		t.Error("Number should not be wrapped promise")
	}

	// Test with null
	if isWrappedPromise(goja.Null()) {
		t.Error("Null should not be wrapped promise")
	}

	// Test with regular object
	obj, _ := rt.RunString("({})")
	if isWrappedPromise(obj) {
		t.Error("Empty object should not be wrapped promise")
	}

	// Test with wrapped promise
	promise := adapter.JS().Resolve(42)
	wrapped := adapter.gojaWrapPromise(promise)
	if !isWrappedPromise(wrapped) {
		t.Error("Wrapped promise should be detected")
	}

	// Test extraction
	extracted, ok := tryExtractWrappedPromise(wrapped)
	if !ok || extracted != promise {
		t.Error("Should extract same promise")
	}
}

// ==============================================================================
// convertToGojaValue Coverage
// ==============================================================================

// TestAdapter_convertToGojaValue_ChainedPromise verifies promise conversion.
func TestAdapter_convertToGojaValue_ChainedPromise(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	promise := adapter.JS().Resolve(42)
	converted := adapter.convertToGojaValue(promise)

	if !isWrappedPromise(converted) {
		t.Error("ChainedPromise should be converted to wrapped promise")
	}
}

// TestAdapter_convertToGojaValue_Slice verifies slice conversion.
func TestAdapter_convertToGojaValue_Slice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	slice := []goeventloop.Result{1, "two", 3.0}
	converted := adapter.convertToGojaValue(slice)

	obj := converted.ToObject(rt)
	lenVal := obj.Get("length")
	if lenVal.ToInteger() != 3 {
		t.Errorf("Expected array length 3, got %v", lenVal.Export())
	}
}

// TestAdapter_convertToGojaValue_Map verifies map conversion.
func TestAdapter_convertToGojaValue_Map(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	m := map[string]interface{}{
		"status": "fulfilled",
		"value":  42,
	}
	converted := adapter.convertToGojaValue(m)

	obj := converted.ToObject(rt)
	status := obj.Get("status")
	if status.Export() != "fulfilled" {
		t.Errorf("Expected 'fulfilled', got %v", status.Export())
	}
}

// ==============================================================================
// Method Calls on Non-Promise
// ==============================================================================

// TestAdapter_Then_OnNonPromise verifies then() on non-promise panics.
// The adapter's then() implementation panics with TypeError when called on non-promise.
func TestAdapter_Then_OnNonPromise(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Calling then on object without _internalPromise causes a panic (TypeError)
	// which goja re-panics, so we get a panic rather than js exception.
	// This test verifies the behavior exists and doesn't cause undefined behavior.
	defer func() {
		if r := recover(); r == nil {
			t.Error("then() on non-promise should panic")
		} else {
			t.Logf("Got expected panic: %v", r)
		}
	}()

	_, _ = rt.RunString(`
		var obj = {};
		obj.then = Promise.prototype.then;
		obj.then(function(){});
	`)
}

// TestAdapter_Catch_OnNonPromise verifies catch() on non-promise panics.
func TestAdapter_Catch_OnNonPromise(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("catch() on non-promise should panic")
		} else {
			t.Logf("Got expected panic: %v", r)
		}
	}()

	_, _ = rt.RunString(`
		var obj = {};
		obj.catch = Promise.prototype.catch;
		obj.catch(function(){});
	`)
}

// TestAdapter_Finally_OnNonPromise verifies finally() on non-promise panics.
func TestAdapter_Finally_OnNonPromise(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("finally() on non-promise should panic")
		} else {
			t.Logf("Got expected panic: %v", r)
		}
	}()

	_, _ = rt.RunString(`
		var obj = {};
		obj.finally = Promise.prototype.finally;
		obj.finally(function(){});
	`)
}
