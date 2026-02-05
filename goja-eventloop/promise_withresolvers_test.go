//go:build linux || darwin

package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// TestPromiseWithResolvers_Basic tests basic Promise.withResolvers() usage.
func TestPromiseWithResolvers_Basic(t *testing.T) {
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
		var r = Promise.withResolvers();
		if (!r) throw new Error("withResolvers returned nothing");
		if (!r.promise) throw new Error("missing promise property");
		if (typeof r.resolve !== 'function') throw new Error("resolve is not a function");
		if (typeof r.reject !== 'function') throw new Error("reject is not a function");
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	loop.Shutdown(context.Background())
}

// TestPromiseWithResolvers_Resolve tests resolving via withResolvers.
func TestPromiseWithResolvers_Resolve(t *testing.T) {
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
		var r = Promise.withResolvers();
		r.promise.then(function(v) {
			result = v;
		});
		r.resolve("test value");
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Run the loop briefly
	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	result := rt.Get("result")
	if result.Export() != "test value" {
		t.Errorf("expected 'test value', got: %v", result.Export())
	}
}

// TestPromiseWithResolvers_Reject tests rejecting via withResolvers.
func TestPromiseWithResolvers_Reject(t *testing.T) {
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
		var r = Promise.withResolvers();
		r.promise.catch(function(r) {
			reason = r;
		});
		r.reject("test error");
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Run the loop briefly
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

// TestPromiseWithResolvers_Chaining tests chaining from withResolvers promise.
func TestPromiseWithResolvers_Chaining(t *testing.T) {
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
		var final = null;
		var r = Promise.withResolvers();
		r.promise
			.then(function(v) { return v * 2; })
			.then(function(v) { final = v; });
		r.resolve(21);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Run the loop briefly
	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	final := rt.Get("final")
	if final.ToInteger() != 42 {
		t.Errorf("expected 42, got: %v", final.Export())
	}
}

// TestPromiseWithResolvers_MultiplePromises tests creating multiple withResolvers.
func TestPromiseWithResolvers_MultiplePromises(t *testing.T) {
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
		var r1 = Promise.withResolvers();
		var r2 = Promise.withResolvers();
		var r3 = Promise.withResolvers();
		
		// Each should be independent
		if (r1.promise === r2.promise) throw new Error("r1 and r2 should be different");
		if (r2.promise === r3.promise) throw new Error("r2 and r3 should be different");
		
		var results = [];
		r1.promise.then(function(v) { results.push("r1:" + v); });
		r2.promise.then(function(v) { results.push("r2:" + v); });
		r3.promise.then(function(v) { results.push("r3:" + v); });
		
		r2.resolve("two");
		r1.resolve("one");
		r3.resolve("three");
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Run the loop briefly
	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	// Check all results arrived
	resultsVal := rt.Get("results")
	results := resultsVal.Export().([]interface{})
	if len(results) != 3 {
		t.Errorf("expected 3 results, got: %d", len(results))
	}
}

// TestPromiseWithResolvers_IdempotentResolve tests that resolve is idempotent.
func TestPromiseWithResolvers_IdempotentResolve(t *testing.T) {
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
		var callCount = 0;
		var r = Promise.withResolvers();
		r.promise.then(function(v) {
			callCount++;
			if (v !== "first") throw new Error("expected 'first', got: " + v);
		});
		r.resolve("first");
		r.resolve("second"); // Should be ignored
		r.resolve("third");  // Should be ignored
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Run the loop briefly
	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	callCount := rt.Get("callCount")
	if callCount.ToInteger() != 1 {
		t.Errorf("expected handler called once, got: %d", callCount.ToInteger())
	}
}

// TestPromiseWithResolvers_IdempotentReject tests that reject is idempotent.
func TestPromiseWithResolvers_IdempotentReject(t *testing.T) {
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
		var callCount = 0;
		var r = Promise.withResolvers();
		r.promise.catch(function(reason) {
			callCount++;
			if (reason !== "first") throw new Error("expected 'first', got: " + reason);
		});
		r.reject("first");
		r.reject("second"); // Should be ignored
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Run the loop briefly
	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	callCount := rt.Get("callCount")
	if callCount.ToInteger() != 1 {
		t.Errorf("expected handler called once, got: %d", callCount.ToInteger())
	}
}

// TestPromiseWithResolvers_ResolveAfterReject tests resolve after reject is ignored.
func TestPromiseWithResolvers_ResolveAfterReject(t *testing.T) {
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
		var thenCalled = false;
		var catchCalled = false;
		var r = Promise.withResolvers();
		r.promise.then(function() { thenCalled = true; });
		r.promise.catch(function() { catchCalled = true; });
		r.reject("error");
		r.resolve("value"); // Should be ignored
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Run the loop briefly
	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	thenCalled := rt.Get("thenCalled")
	catchCalled := rt.Get("catchCalled")

	if thenCalled.ToBoolean() {
		t.Error("then should not have been called")
	}
	if !catchCalled.ToBoolean() {
		t.Error("catch should have been called")
	}
}

// TestPromiseWithResolvers_RejectAfterResolve tests reject after resolve is ignored.
func TestPromiseWithResolvers_RejectAfterResolve(t *testing.T) {
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
		var thenCalled = false;
		var catchCalled = false;
		var r = Promise.withResolvers();
		r.promise.then(function() { thenCalled = true; });
		r.promise.catch(function() { catchCalled = true; });
		r.resolve("value");
		r.reject("error"); // Should be ignored
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Run the loop briefly
	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	thenCalled := rt.Get("thenCalled")
	catchCalled := rt.Get("catchCalled")

	if !thenCalled.ToBoolean() {
		t.Error("then should have been called")
	}
	if catchCalled.ToBoolean() {
		t.Error("catch should not have been called")
	}
}

// TestPromiseWithResolvers_NullUndefined tests resolving/rejecting with null/undefined.
func TestPromiseWithResolvers_NullUndefined(t *testing.T) {
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
		var results = [];
		
		var r1 = Promise.withResolvers();
		r1.promise.then(function(v) { results.push(v === null ? "null" : "not null"); });
		r1.resolve(null);
		
		var r2 = Promise.withResolvers();
		r2.promise.then(function(v) { results.push(v === undefined ? "undefined" : "not undefined"); });
		r2.resolve(undefined);
		
		var r3 = Promise.withResolvers();
		r3.promise.then(function(v) { results.push("no-arg"); });
		r3.resolve(); // No argument = undefined
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Run the loop briefly
	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	resultsVal := rt.Get("results")
	results := resultsVal.Export().([]interface{})
	if len(results) != 3 {
		t.Errorf("expected 3 results, got: %d", len(results))
	}
}

// TestPromiseWithResolvers_WithPromiseAll tests using withResolvers with Promise.all.
func TestPromiseWithResolvers_WithPromiseAll(t *testing.T) {
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
		var allResult = null;
		
		var r1 = Promise.withResolvers();
		var r2 = Promise.withResolvers();
		var r3 = Promise.withResolvers();
		
		Promise.all([r1.promise, r2.promise, r3.promise]).then(function(results) {
			allResult = results;
		});
		
		r1.resolve("a");
		r2.resolve("b");
		r3.resolve("c");
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Run the loop briefly
	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	allResultVal := rt.Get("allResult")
	allResult := allResultVal.Export().([]interface{})
	if len(allResult) != 3 {
		t.Errorf("expected 3 results, got: %d", len(allResult))
	}
	if allResult[0] != "a" || allResult[1] != "b" || allResult[2] != "c" {
		t.Errorf("expected [a, b, c], got: %v", allResult)
	}
}

// TestPromiseWithResolvers_WithPromiseRace tests using withResolvers with Promise.race.
func TestPromiseWithResolvers_WithPromiseRace(t *testing.T) {
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
		var raceResult = null;
		
		var r1 = Promise.withResolvers();
		var r2 = Promise.withResolvers();
		var r3 = Promise.withResolvers();
		
		Promise.race([r1.promise, r2.promise, r3.promise]).then(function(result) {
			raceResult = result;
		});
		
		r2.resolve("winner"); // r2 resolves first
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Run the loop briefly
	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	raceResult := rt.Get("raceResult")
	if raceResult.Export() != "winner" {
		t.Errorf("expected 'winner', got: %v", raceResult.Export())
	}
}

// TestPromiseWithResolvers_TimeoutPattern tests timeout pattern implementation.
func TestPromiseWithResolvers_TimeoutPattern(t *testing.T) {
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
		var rejected = false;
		
		// Create a cancellable delay using withResolvers
		function delay(ms) {
			var r = Promise.withResolvers();
			var id = setTimeout(function() { r.resolve(); }, ms);
			// Return promise with cancel capability
			r.promise.cancel = function() {
				clearTimeout(id);
				r.reject("cancelled");
			};
			return r;
		}
		
		var timer = delay(1000);
		timer.promise.then(function() { result = "completed"; });
		timer.promise.catch(function() { rejected = true; });
		
		// Cancel immediately
		timer.promise.cancel();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Run the loop briefly
	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	rejected := rt.Get("rejected")
	result := rt.Get("result")

	if !rejected.ToBoolean() {
		t.Error("expected promise to be rejected")
	}
	if result.Export() != nil {
		t.Errorf("expected result to be null, got: %v", result.Export())
	}
}

// TestPromiseWithResolvers_Finally tests finally handler with withResolvers.
func TestPromiseWithResolvers_Finally(t *testing.T) {
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
		var finallyCalled = false;
		var thenCalled = false;
		
		var r = Promise.withResolvers();
		r.promise
			.then(function(v) { thenCalled = true; return v; })
			.finally(function() { finallyCalled = true; });
		
		r.resolve("done");
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Run the loop briefly
	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		loop.Shutdown(context.Background())
		close(done)
	}()
	loop.Run(context.Background())
	<-done

	finallyCalled := rt.Get("finallyCalled")
	thenCalled := rt.Get("thenCalled")

	if !finallyCalled.ToBoolean() {
		t.Error("finally should have been called")
	}
	if !thenCalled.ToBoolean() {
		t.Error("then should have been called")
	}
}
