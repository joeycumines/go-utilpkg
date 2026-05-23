package gojaeventloop

import (
	"context"
	"fmt"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// Promise iterable protocol coverage.

// TestAdapter_Iterator_NullIterable verifies null/undefined iterable errors.
func TestAdapter_Iterator_NullIterable(t *testing.T) {
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
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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

func TestNodePromiseCombinatorIterableErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = loop.Shutdown(context.Background()) }()
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	done := make(chan string, 1)
	if err := runtime.Set("testDone", func(value string) { done <- value }); err != nil {
		t.Fatalf("set testDone: %v", err)
	}

	_, err = runtime.RunString(`
		const methods = ["all", "race", "allSettled", "any"];
		const cases = [
			["null", function () { return null; }, "object null is not iterable (cannot read property Symbol(Symbol.iterator))"],
			["undefined", function () { return undefined; }, "undefined is not iterable (cannot read property Symbol(Symbol.iterator))"],
			["object", function () { return {}; }, "object is not iterable (cannot read property Symbol(Symbol.iterator))"],
			["number", function () { return 1; }, "number 1 is not iterable (cannot read property Symbol(Symbol.iterator))"],
			["symbol", function () { return Symbol("probe"); }, "symbol is not iterable (cannot read property Symbol(Symbol.iterator))"],
			["method", function () { return { [Symbol.iterator]: 1 }; }, "object is not iterable (cannot read property Symbol(Symbol.iterator))"],
			["iterator", function () { return { [Symbol.iterator]: function () { return 1; } }; }, "Result of the Symbol.iterator method is not an object"],
			["next", function () { return { [Symbol.iterator]: function () { return { next: 1 }; } }; }, "number 1 is not a function"],
			["result", function () { return { [Symbol.iterator]: function () { return { next: function () { return 1; } }; } }; }, "Iterator result 1 is not an object"],
			["null-result", function () { return { [Symbol.iterator]: function () { return { next: function () { return null; } }; } }; }, "Iterator result null is not an object"],
		];
		const pending = [];
		const failures = [];
		for (const method of methods) {
			for (const [name, input, expected] of cases) {
				pending.push(Promise[method](input()).then(
					function () { failures.push(method + "." + name + ":resolved"); },
					function (error) {
						const actual = error.name + ":" + error.message;
						if (actual !== "TypeError:" + expected) {
							failures.push(method + "." + name + ":" + actual);
						}
					},
				));
			}
		}
		Promise.all(pending).then(function () { testDone(failures.join("|")); });
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	select {
	case failures := <-done:
		if failures != "" {
			t.Fatalf("Promise combinator iterable mismatches: %s", failures)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for Promise combinator iterable errors")
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}
}

func TestNodePromiseCombinatorCapabilitiesUseCapturedTypeError(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = loop.Close() }()
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	value, err := runtime.RunString(`
		const NativeTypeError = TypeError;
		let poisonCalls = 0;
		globalThis.TypeError = function PoisonTypeError() { poisonCalls++; };
		function capture(callback) {
			try { callback(); return "ok"; }
			catch (error) {
				return (error instanceof NativeTypeError) + ":" + error.name + ":" + error.message;
			}
		}
		function Twice(executor) {
			executor(function () {}, function () {});
			executor(function () {}, function () {});
			return {};
		}
		function Missing() { return {}; }
		const events = [];
		for (const method of ["all", "race", "allSettled", "any"]) {
			events.push(method + ".null=" + capture(function () { Promise[method].call(null, []); }));
			events.push(method + ".object=" + capture(function () { Promise[method].call({}, []); }));
			events.push(method + ".nullPrototype=" + capture(function () { Promise[method].call(Object.create(null), []); }));
			events.push(method + ".twice=" + capture(function () { Promise[method].call(Twice, []); }));
			events.push(method + ".missing=" + capture(function () { Promise[method].call(Missing, []); }));
		}
		events.push("poison=" + poisonCalls);
		events.join("|");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	const nonObject = "true:TypeError:Promise.%s called on non-object"
	const object = "true:TypeError:#<Object> is not a constructor"
	const nullPrototype = "true:TypeError:[object Object] is not a constructor"
	const twice = "true:TypeError:Promise executor has already been invoked with non-undefined arguments"
	const missing = "true:TypeError:Promise resolve or reject function is not callable"
	want := ""
	for _, method := range []string{"all", "race", "allSettled", "any"} {
		if want != "" {
			want += "|"
		}
		want += method + ".null=" + fmt.Sprintf(nonObject, method) +
			"|" + method + ".object=" + object +
			"|" + method + ".nullPrototype=" + nullPrototype +
			"|" + method + ".twice=" + twice +
			"|" + method + ".missing=" + missing
	}
	want += "|poison=0"
	if got := value.String(); got != want {
		t.Fatalf("Promise combinator capability errors = %q, want %q", got, want)
	}
}

// TestAdapter_Iterator_ArrayFastPath verifies array fast path works.
func TestAdapter_Iterator_ArrayFastPath(t *testing.T) {
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
	arr := result.Export().([]any)
	if len(arr) != 3 || arr[0].(int64) != 1 || arr[1].(int64) != 2 || arr[2].(int64) != 3 {
		t.Errorf("Expected [1,2,3], got %v", arr)
	}
}

// TestAdapter_consumeIterable_Set verifies Set iteration works.
func TestAdapter_consumeIterable_Set(t *testing.T) {
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
	arr := result.Export().([]any)
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
		t.Fatal(err)
	}
	defer loop.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
	arr := result.Export().([]any)
	if len(arr) != 3 {
		t.Errorf("Expected 3 elements from generator, got %d", len(arr))
	}
}
