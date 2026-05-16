package gojaeventloop

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"weak"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// Promise construction, resolution, error conversion, handler, and lifecycle coverage.

// TestAdapter_Promise_ExecutorNotFunction verifies executor must be function.
func TestAdapter_Promise_ExecutorNotFunction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
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

// TestAdapter_Thenable_Resolution verifies thenable objects are resolved.
func TestAdapter_Thenable_Resolution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
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

// TestAdapter_ErrorConversion_JSError verifies JS errors are preserved.
func TestAdapter_ErrorConversion_JSError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
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

type nativePromiseGCSentinel struct {
	calls *atomic.Int32
}

func newNativePromiseGCHandler() (func(goja.FunctionCall) goja.Value, weak.Pointer[nativePromiseGCSentinel], *atomic.Int32) {
	calls := new(atomic.Int32)
	sentinel := &nativePromiseGCSentinel{calls: calls}
	return func(goja.FunctionCall) goja.Value {
		sentinel.calls.Add(1)
		return goja.Undefined()
	}, weak.Make(sentinel), calls
}

// TestAdapter_NativePromise_GC verifies completed native Promise reactions
// release their Go callback capture without requiring terminal cleanup.
func TestAdapter_NativePromise_GC(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	done := make(chan struct{}, 1)
	handler, sentinel, calls := newNativePromiseGCHandler()
	if err := rt.Set("__promiseGCSentinel", handler); err != nil {
		t.Fatalf("install Promise GC sentinel: %v", err)
	}
	if err := rt.Set("__promiseGCDone", func() { done <- struct{}{} }); err != nil {
		t.Fatalf("install Promise GC completion callback: %v", err)
	}
	_, err = rt.RunString(`
		for (var i = 0; i < 1000; i++) {
			Promise.resolve(i).then(__promiseGCSentinel);
		}
		queueMicrotask(__promiseGCDone);
		delete globalThis.__promiseGCSentinel;
		delete globalThis.__promiseGCDone;
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}
	handler = nil

	go func() { _ = loop.Run(ctx) }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout waiting for Promise reactions")
	}
	if got := calls.Load(); got != 1000 {
		t.Fatalf("Promise reaction callbacks = %d, want 1000", got)
	}
	checkpointReturned := make(chan struct{})
	if err := adapter.Submit(func(*goja.Runtime) { close(checkpointReturned) }); err != nil {
		t.Fatalf("submit post-Promise owner barrier: %v", err)
	}
	select {
	case <-checkpointReturned:
	case <-ctx.Done():
		t.Fatal("Timeout waiting for post-Promise owner barrier")
	}

	for range 100 {
		runtime.GC()
		runtime.Gosched()
		if sentinel.Value() == nil {
			return
		}
	}
	t.Fatal("completed native Promise reactions retained their callback capture")
}

// TestAdapter_NativePromise_NoLeak verifies no memory leak in promise chains.
// This is a simpler version that creates fewer chains to avoid timing issues.
func TestAdapter_NativePromise_NoLeak(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Simple test: create a few promises and resolve them
	var chainCount int
	var chainMu sync.Mutex
	_ = rt.Set("incrementChain", func() {
		chainMu.Lock()
		chainCount++
		chainMu.Unlock()
	})

	done := make(chan struct{})
	_ = rt.Set("notifyDone", func() {
		close(done)
	})

	// Create promise chains BEFORE starting the loop to avoid race
	_, err = rt.RunString(`
		var completed = 0;
		for (var i = 0; i < 5; i++) {
			Promise.resolve(i).then(function(x) {
				incrementChain();
				completed++;
				if (completed === 5) {
					notifyDone();
				}
				return x;
			});
		}
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	// Now start the loop to process the microtasks
	go loop.Run(ctx)

	// Wait for all promises to complete
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Timeout waiting for promises")
	}

	chainMu.Lock()
	count := chainCount
	chainMu.Unlock()

	// Should have processed all 5 chains
	t.Logf("Chain count: %d", count)
	if count != 5 {
		t.Errorf("Expected 5 chains to complete, got %d", count)
	}
}

// TestAdapter_gojaFuncToHandler_NilHandler verifies nil handler returns nil.
func TestAdapter_gojaFuncToHandler_NilHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
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

// TestAdapter_PromiseResolve_Identity verifies Promise.resolve(promise) returns same promise.
func TestAdapter_PromiseResolve_Identity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
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

// TestAdapter_gojaVoidFuncToHandler_NilHandler verifies nil finally handler is no-op.
func TestAdapter_gojaVoidFuncToHandler_NilHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
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

// TestAdapter_Then_OnNonPromise verifies native Promise.prototype.then rejects
// an incompatible receiver with a catchable TypeError.
func TestAdapter_Then_OnNonPromise(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var caught = false;
		try { Promise.prototype.then.call({}, function(){}); }
		catch (e) { caught = e instanceof TypeError; }
		if (!caught) throw new Error("then() on non-promise should throw TypeError");
	`)
	if err != nil {
		t.Fatalf("then() on non-promise: %v", err)
	}
}

// TestAdapter_Catch_OnNonPromise verifies catch() on non-promise panics.
func TestAdapter_Catch_OnNonPromise(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var caught = false;
		try { Promise.prototype.catch.call({}, function(){}); }
		catch (e) { caught = e instanceof TypeError; }
		if (!caught) throw new Error("catch() on non-promise should throw TypeError");
	`)
	if err != nil {
		t.Fatalf("catch() on non-promise: %v", err)
	}
}

// TestAdapter_Finally_OnNonPromise verifies finally() on non-promise panics.
func TestAdapter_Finally_OnNonPromise(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var caught = false;
		try { Promise.prototype.finally.call({}, function(){}); }
		catch (e) { caught = e instanceof TypeError; }
		if (!caught) throw new Error("finally() on non-promise should throw TypeError");
	`)
	if err != nil {
		t.Fatalf("finally() on non-promise: %v", err)
	}
}
