// Package gojaeventloop provides tests for Promise static methods.
//
// Promise static method tests
//
// Tests cover:
// - Promise.resolve() with value, thenable, and promise
// - Promise.reject()
// - Promise.all([]) empty array
// - Promise.race([]) empty array (never settles)
// - Promise.allSettled([]) empty array
// - Promise.any([]) empty array (AggregateError)
// - Already settled input handling

package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ============================================================================
// Promise.resolve() Tests
// ============================================================================

// TestPromiseResolve_WithValue tests Promise.resolve() with a plain value.
func TestPromiseResolve_WithValue(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var result = null;
		Promise.resolve(42).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	result := rt.Get("result")
	if result.ToInteger() != 42 {
		t.Errorf("expected 42, got: %v", result.Export())
	}
}

// TestPromiseResolve_WithString tests Promise.resolve() with a string value.
func TestPromiseResolve_WithString(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var result = null;
		Promise.resolve("hello world").then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	result := rt.Get("result")
	if result.Export() != "hello world" {
		t.Errorf("expected 'hello world', got: %v", result.Export())
	}
}

// TestPromiseResolve_WithThenable tests Promise.resolve() with a thenable object.
func TestPromiseResolve_WithThenable(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var result = null;
		var thenable = {
			then: function(resolve, reject) {
				resolve("thenable value");
			}
		};
		Promise.resolve(thenable).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	result := rt.Get("result")
	if result.Export() != "thenable value" {
		t.Errorf("expected 'thenable value', got: %v", result.Export())
	}
}

// TestPromiseResolve_WithPromise tests Promise.resolve() with another promise.
func TestPromiseResolve_WithPromise(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var result = null;
		var inner = new Promise(function(resolve) {
			resolve("inner value");
		});
		Promise.resolve(inner).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	result := rt.Get("result")
	if result.Export() != "inner value" {
		t.Errorf("expected 'inner value', got: %v", result.Export())
	}
}

// TestPromiseResolve_NoArgument tests Promise.resolve() with no argument.
func TestPromiseResolve_NoArgument(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var called = false;
		var resultIsEmpty = false;
		Promise.resolve().then(function(v) {
			called = true;
			// Value should be undefined or null (implementation detail)
			resultIsEmpty = (v === undefined || v === null);
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	called := rt.Get("called")
	resultIsEmpty := rt.Get("resultIsEmpty")

	if !called.ToBoolean() {
		t.Error("expected then handler to be called")
	}
	if !resultIsEmpty.ToBoolean() {
		t.Error("expected resolved value to be undefined or null")
	}
}

// ============================================================================
// Promise.reject() Tests
// ============================================================================

// TestPromiseReject_Basic tests basic Promise.reject() usage.
func TestPromiseReject_Basic(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var reason = null;
		Promise.reject("test error").catch(function(r) {
			reason = r;
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	reason := rt.Get("reason")
	if reason.Export() != "test error" {
		t.Errorf("expected 'test error', got: %v", reason.Export())
	}
}

// TestPromiseReject_WithError tests Promise.reject() with an Error object.
func TestPromiseReject_WithError(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var reason = null;
		var err = new Error("custom error");
		Promise.reject(err).catch(function(r) {
			reason = r;
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	// Verify that we got an Error object
	_, err = rt.RunString(`
		if (!(reason instanceof Error)) throw new Error("Expected Error instance");
		if (reason.message !== "custom error") throw new Error("Wrong message");
	`)
	if err != nil {
		t.Fatalf("validation failed: %v", err)
	}
}

// TestPromiseReject_NoArgument tests Promise.reject() with no argument.
func TestPromiseReject_NoArgument(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var called = false;
		var reasonIsEmpty = false;
		Promise.reject().catch(function(r) {
			called = true;
			// Reason should be undefined or null (implementation detail)
			reasonIsEmpty = (r === undefined || r === null);
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	called := rt.Get("called")
	reasonIsEmpty := rt.Get("reasonIsEmpty")

	if !called.ToBoolean() {
		t.Error("expected catch handler to be called")
	}
	if !reasonIsEmpty.ToBoolean() {
		t.Error("expected rejection reason to be undefined or null")
	}
}

// ============================================================================
// Promise.all([]) Empty Array Tests
// ============================================================================

// TestPromiseAll_EmptyArray tests Promise.all with an empty array.
func TestPromiseAll_EmptyArray(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var result = null;
		Promise.all([]).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	result := rt.Get("result")
	exported := result.Export()
	arr, ok := exported.([]any)
	if !ok {
		t.Fatalf("expected array, got: %T", exported)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty array, got length: %d", len(arr))
	}
}

// ============================================================================
// Promise.race([]) Empty Array Tests
// ============================================================================

// TestPromiseRace_EmptyArray tests Promise.race with an empty array (never settles).
func TestPromiseRace_EmptyArray(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var resolved = false;
		var rejected = false;
		Promise.race([]).then(
			function() { resolved = true; },
			function() { rejected = true; }
		);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(100 * time.Millisecond) // Give more time to verify it doesn't settle
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	resolved := rt.Get("resolved")
	rejected := rt.Get("rejected")

	// Promise.race([]) should never settle
	if resolved.ToBoolean() {
		t.Error("Promise.race([]) should never resolve")
	}
	if rejected.ToBoolean() {
		t.Error("Promise.race([]) should never reject")
	}
}

// ============================================================================
// Promise.allSettled([]) Empty Array Tests
// ============================================================================

// TestPromiseAllSettled_EmptyArray tests Promise.allSettled with an empty array.
func TestPromiseAllSettled_EmptyArray(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var result = null;
		Promise.allSettled([]).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	result := rt.Get("result")
	exported := result.Export()
	arr, ok := exported.([]any)
	if !ok {
		t.Fatalf("expected array, got: %T", exported)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty array, got length: %d", len(arr))
	}
}

// ============================================================================
// Promise.any([]) Empty Array Tests (AggregateError)
// ============================================================================

// TestPromiseAny_EmptyArray tests Promise.any with an empty array.
func TestPromiseAny_EmptyArray(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var resolved = false;
		var rejected = false;
		var reason = null;
		Promise.any([]).then(
			function() { resolved = true; },
			function(r) {
				rejected = true;
				reason = r;
			}
		);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	resolved := rt.Get("resolved")
	rejected := rt.Get("rejected")

	if resolved.ToBoolean() {
		t.Error("Promise.any([]) should not resolve")
	}
	if !rejected.ToBoolean() {
		t.Error("Promise.any([]) should reject with AggregateError")
	}

	// Verify it's an AggregateError
	_, err = rt.RunString(`
		if (!reason) throw new Error("reason is null");
		if (reason.name !== "AggregateError") throw new Error("expected AggregateError, got: " + reason.name);
		// Note: Some implementations may have non-empty errors array or no errors array
		// The key thing is that it's an AggregateError for an empty Promise.any([])
	`)
	if err != nil {
		t.Fatalf("AggregateError validation failed: %v", err)
	}
}

// ============================================================================
// Already Settled Input Tests
// ============================================================================

// TestPromiseAll_AlreadySettledResolved tests Promise.all with already resolved promises.
func TestPromiseAll_AlreadySettledResolved(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var result = null;
		var p1 = Promise.resolve(1);
		var p2 = Promise.resolve(2);
		var p3 = Promise.resolve(3);
		Promise.all([p1, p2, p3]).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	result := rt.Get("result")
	exported := result.Export()
	arr, ok := exported.([]any)
	if !ok {
		t.Fatalf("expected array, got: %T", exported)
	}
	if len(arr) != 3 {
		t.Errorf("expected 3 elements, got: %d", len(arr))
	}
	if arr[0].(int64) != 1 || arr[1].(int64) != 2 || arr[2].(int64) != 3 {
		t.Errorf("expected [1, 2, 3], got: %v", arr)
	}
}

// TestPromiseAll_AlreadySettledRejected tests Promise.all with one already rejected.
func TestPromiseAll_AlreadySettledRejected(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var reason = null;
		var p1 = Promise.resolve(1);
		var p2 = Promise.reject("error");
		var p3 = Promise.resolve(3);
		Promise.all([p1, p2, p3]).catch(function(r) {
			reason = r;
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	reason := rt.Get("reason")
	if reason.Export() != "error" {
		t.Errorf("expected 'error', got: %v", reason.Export())
	}
}

// TestPromiseRace_AlreadySettled tests Promise.race with already settled promises.
func TestPromiseRace_AlreadySettled(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var result = null;
		var p1 = Promise.resolve("first");
		var p2 = Promise.resolve("second");
		Promise.race([p1, p2]).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	result := rt.Get("result")
	// First promise wins
	if result.Export() != "first" {
		t.Errorf("expected 'first', got: %v", result.Export())
	}
}

// TestPromiseAny_AlreadySettled tests Promise.any with already settled promises.
func TestPromiseAny_AlreadySettled(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var result = null;
		var p1 = Promise.reject("err1");
		var p2 = Promise.resolve("success");
		var p3 = Promise.reject("err3");
		Promise.any([p1, p2, p3]).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	result := rt.Get("result")
	if result.Export() != "success" {
		t.Errorf("expected 'success', got: %v", result.Export())
	}
}

// TestPromiseAllSettled_AlreadySettled tests Promise.allSettled with already settled.
func TestPromiseAllSettled_AlreadySettled(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var result = null;
		var p1 = Promise.resolve("ok");
		var p2 = Promise.reject("error");
		Promise.allSettled([p1, p2]).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	// Verify the structure of results
	_, err = rt.RunString(`
		if (!result) throw new Error("result is null");
		if (result.length !== 2) throw new Error("expected 2 results, got: " + result.length);
		if (result[0].status !== "fulfilled") throw new Error("expected fulfilled, got: " + result[0].status);
		if (result[0].value !== "ok") throw new Error("expected 'ok', got: " + result[0].value);
		if (result[1].status !== "rejected") throw new Error("expected rejected, got: " + result[1].status);
		if (result[1].reason !== "error") throw new Error("expected 'error', got: " + result[1].reason);
	`)
	if err != nil {
		t.Fatalf("result validation failed: %v", err)
	}
}

// TestPromiseResolve_WithNull tests Promise.resolve() with null.
func TestPromiseResolve_WithNull(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var result = "not null";
		Promise.resolve(null).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	result := rt.Get("result")
	if !goja.IsNull(result) {
		t.Errorf("expected null, got: %v", result.Export())
	}
}

// TestPromiseResolve_WithObject tests Promise.resolve() with a plain object.
func TestPromiseResolve_WithObject(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		var result = null;
		var obj = { name: "test", value: 123 };
		Promise.resolve(obj).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	// Verify the object properties
	_, err = rt.RunString(`
		if (!result) throw new Error("result is null");
		if (result.name !== "test") throw new Error("expected name 'test', got: " + result.name);
		if (result.value !== 123) throw new Error("expected value 123, got: " + result.value);
	`)
	if err != nil {
		t.Fatalf("result validation failed: %v", err)
	}
}
