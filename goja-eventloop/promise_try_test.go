package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// ===============================================
// Promise.try() Tests
// ===============================================

// TestPromiseTry_Success tests Promise.try() with a successful function.
func TestPromiseTry_Success(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	resultCh := make(chan string, 1)
	if err := runtime.Set("capturePromiseTryNonFunction", func(value string) { resultCh <- value }); err != nil {
		t.Fatalf("set capturePromiseTryNonFunction: %v", err)
	}

	_, err = runtime.RunString(`
		const events = [];
		function probe(label, value) {
			try {
				const promise = Promise.try(value);
				const sync = "returned:" + (promise instanceof Promise);
				promise.then(
					() => events.push(label + ":" + sync + "|resolved"),
					(err) => events.push(label + ":" + sync + "|" + err.name + ":" + err.message),
				);
			} catch (err) {
				events.push(label + ":threw:" + err.name + ":" + err.message);
			}
		}
		probe("number", 42);
		probe("null", null);
		probe("bigint", BigInt(1));
		probe("string", "x");
		probe("boolean", true);
		probe("symbol", Symbol("s"));
		probe("object", {});
		probe("array", []);
		setImmediate(function() { capturePromiseTryNonFunction(events.join("\n")); });
	`)
	if err != nil {
		t.Fatalf("Promise.try non-function test failed: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()

	select {
	case got := <-resultCh:
		want := "number:returned:true|TypeError:number 42 is not a function\n" +
			"null:returned:true|TypeError:object null is not a function\n" +
			"bigint:returned:true|TypeError:bigint is not a function\n" +
			"string:returned:true|TypeError:string \"x\" is not a function\n" +
			"boolean:returned:true|TypeError:boolean true is not a function\n" +
			"symbol:returned:true|TypeError:symbol is not a function\n" +
			"object:returned:true|TypeError:object is not a function\n" +
			"array:returned:true|TypeError:object is not a function"
		if got != want {
			t.Fatalf("Promise.try non-function result = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for Promise.try non-function result")
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}
}

func TestPromiseTry_CustomConstructorValidation(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	value, err := runtime.RunString(`
		const events = [];
		let duplicateCalls = 0;
		for (const C of [
			function Fake(executor) {},
			function Fake2(executor) { return {}; },
			function Fake3(executor) { executor(function(){}, function(){}); executor(function(){}, function(){}); },
		]) {
			try {
				Promise.try.call(C, function() { duplicateCalls++; return 7; });
				events.push(C.name + ":ok");
			} catch (err) {
				events.push(C.name + ":" + err.name + ":" + err.message);
			}
		}
		events.push("calls:" + duplicateCalls);
		events.join("\n");
	`)
	if err != nil {
		t.Fatalf("Promise.try custom constructor validation script failed: %v", err)
	}
	want := "Fake:TypeError:Promise resolve or reject function is not callable\n" +
		"Fake2:TypeError:Promise resolve or reject function is not callable\n" +
		"Fake3:TypeError:Promise executor has already been invoked with non-undefined arguments\n" +
		"calls:0"
	if got := value.String(); got != want {
		t.Fatalf("Promise.try custom constructor validation = %q, want %q", got, want)
	}
}

func TestPromiseTryAndAnyIgnoreMutableUserlandIntrinsics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}
	resultCh := make(chan string, 1)
	if err := runtime.Set("captureMutableIntrinsicResult", func(value string) { resultCh <- value }); err != nil {
		t.Fatalf("set captureMutableIntrinsicResult: %v", err)
	}

	_, err = runtime.RunString(`
		const out = [];
		out.push("name:" + Promise.try.name);
		Array.prototype.slice = function() { throw new Error("slice boom"); };
		Function.prototype.apply = function() { throw new Error("apply boom"); };
		globalThis.String = function() { throw new Error("string boom"); };
		globalThis.Symbol = { iterator: "poisoned" };
		Promise.prototype.catch = function() { throw new Error("catch boom"); };
		function fn(a, b) { out.push("fn:" + a + ":" + b); return 7; }
		fn.apply = function() { throw new Error("own apply boom"); };
		try {
			const p = Promise.try(fn, "a", "b");
			out.push("tryReturned:" + (p instanceof Promise));
			p.then(
				(value) => out.push("tryResolved:" + value),
				(err) => out.push("tryRejected:" + err.name + ":" + err.message),
			);
		} catch (err) {
			out.push("tryThrew:" + err.name + ":" + err.message);
		}
		try {
			const p = Promise.try(42);
			out.push("badReturned:" + (p instanceof Promise));
			p.then(
				() => out.push("badResolved"),
				(err) => out.push("badRejected:" + err.name + ":" + err.message),
			);
		} catch (err) {
			out.push("badThrew:" + err.name + ":" + err.message);
		}
		try {
			const p = Promise.any([Promise.reject("x")]);
			out.push("anyReturned:" + (p instanceof Promise));
			p.then(
				() => out.push("anyResolved"),
				(err) => out.push("anyRejected:" + err.name + ":" + err.message + ":" + err.errors[0]),
			);
		} catch (err) {
			out.push("anyThrew:" + err.name + ":" + err.message);
		}
		setImmediate(function() { captureMutableIntrinsicResult(out.join("|")); });
	`)
	if err != nil {
		t.Fatalf("Promise mutable intrinsic script failed: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()
	select {
	case got := <-resultCh:
		want := "name:try|fn:a:b|tryReturned:true|badReturned:true|anyReturned:true|tryResolved:7|badRejected:TypeError:number 42 is not a function|anyRejected:AggregateError:All promises were rejected:x"
		if got != want {
			t.Fatalf("Promise mutable intrinsic behavior = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for Promise mutable intrinsic result")
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}
}

func TestPromiseAnyPreservesGenericReturnSemantics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}
	resultCh := make(chan string, 1)
	if err := runtime.Set("capturePromiseAnyGenericResult", func(value string) { resultCh <- value }); err != nil {
		t.Fatalf("set capturePromiseAnyGenericResult: %v", err)
	}

	_, err = runtime.RunString(`
		const out = [];
		class P extends Promise {
			static get [Symbol.species]() { return Promise; }
		}
		const fulfilled = Promise.any.call(P, [1]);
		out.push("subclassFulfilledReturn:" + (fulfilled instanceof P) + ":" + (fulfilled instanceof Promise));
		fulfilled.then(
			(value) => out.push("subclassFulfilled:" + value),
			(err) => out.push("subclassFulfilledRejected:" + err.name + ":" + err.message),
		);
		const rejected = Promise.any.call(P, []);
		out.push("subclassRejectedReturn:" + (rejected instanceof P) + ":" + (rejected instanceof Promise));
		rejected.then(
			() => out.push("subclassRejectedResolved"),
			(err) => out.push("subclassRejected:" + err.name + ":" + err.message + ":" + err.errors.length),
		);

		function C(executor) {
			out.push("C:ctor");
			executor(
				(value) => out.push("C:resolve:" + value),
				(err) => out.push("C:reject:" + err.name + ":" + err.message),
			);
			return { tag: "fake" };
		}
		const cEmpty = Promise.any.call(C, []);
		out.push("CEmptyReturn:" + cEmpty.tag);
		const cValue = Promise.any.call(C, [1]);
		out.push("CValueReturn:" + cValue.tag);

		function D(executor) {
			out.push("D:ctor");
			executor(
				(value) => out.push("D:resolve:" + value),
				(err) => out.push("D:reject:" + err.name + ":" + err.message),
			);
			return { tag: "fakeD" };
		}
		D.resolve = function(value) {
			out.push("D.resolve:" + value);
			return {
				then(resolve, reject) {
					out.push("D.then");
					resolve("ok-" + value);
				},
			};
		};
		const dValue = Promise.any.call(D, [2]);
		out.push("DValueReturn:" + dValue.tag);

		setImmediate(function() { capturePromiseAnyGenericResult(out.join("|")); });
	`)
	if err != nil {
		t.Fatalf("Promise.any generic semantics script failed: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()
	select {
	case got := <-resultCh:
		want := "subclassFulfilledReturn:true:true|subclassRejectedReturn:true:true|C:ctor|C:reject:TypeError:resolve is not a function|CEmptyReturn:fake|C:ctor|C:reject:TypeError:resolve is not a function|CValueReturn:fake|D:ctor|D.resolve:2|D.then|D:resolve:ok-2|DValueReturn:fakeD|subclassRejected:AggregateError:All promises were rejected:0|subclassFulfilled:1"
		if got != want {
			t.Fatalf("Promise.any generic behavior = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for Promise.any generic result")
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}
}

// TestPromiseTry_Chaining tests Promise.try() chaining.
func TestPromiseTry_Chaining(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
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
