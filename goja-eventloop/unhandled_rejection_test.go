package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestProcessUnhandledRejectionAndRejectionHandledSemantics(t *testing.T) {
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
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	done := make(chan string, 1)
	if err := runtime.Set("testDone", func(value string) { done <- value }); err != nil {
		t.Fatalf("set testDone: %v", err)
	}

	_, err = runtime.RunString(`
		const events = [];
		let child;
		process.on("unhandledRejection", function(reason, promise) {
			events.push("unhandled:" + reason + ":" + (promise === child));
		});
		process.on("rejectionHandled", function(promise) {
			events.push("handled:" + (promise === child));
		});

		const sameTurn = Promise.reject("same-turn");
		sameTurn.catch(function() {});

		const microtaskHandled = Promise.reject("microtask-handled");
		queueMicrotask(function() { microtaskHandled.catch(function() {}); });

		const parent = Promise.reject("child-only");
		child = parent.then(function() {});
		parent.catch(function() {});

		setImmediate(function() {
			events.push("immediate1-before-catch");
			child.catch(function() {});
			events.push("immediate1-after-catch");
			setImmediate(function() {
				events.push("immediate2");
				testDone(events.join(","));
			});
		});
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case got := <-done:
		want := "unhandled:child-only:true,immediate1-before-catch,immediate1-after-catch,handled:true,immediate2"
		if got != want {
			t.Fatalf("rejection events = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for rejection events")
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
		t.Fatal("Run did not return")
	}
}

func TestProcessUnhandledRejectionEmissionOrder(t *testing.T) {
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
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	done := make(chan string, 1)
	if err := runtime.Set("testDone", func(value string) { done <- value }); err != nil {
		t.Fatalf("set testDone: %v", err)
	}

	_, err = runtime.RunString(`
		const events = [];
		process.on("unhandledRejection", function(reason) { events.push(String(reason)); });
		for (const reason of ["a", "b", "c", "d", "e", "f", "g", "h", "i", "j"]) {
			Promise.reject(reason);
		}
		setImmediate(function() { testDone(events.join(",")); });
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	select {
	case got := <-done:
		want := "a,b,c,d,e,f,g,h,i,j"
		if got != want {
			t.Fatalf("unhandled rejection order = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for unhandled rejection order")
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
		t.Fatal("Run did not return")
	}
}

func TestDefaultUnhandledRejectionEscalationReason(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   string
	}{
		{
			name:   "primitive reason wraps",
			reason: `"x"`,
			want:   "UnhandledPromiseRejection:ERR_UNHANDLED_REJECTION:true:unhandledRejection",
		},
		{
			name:   "object reason formats like Node",
			reason: `({})`,
			want:   `UnhandledPromiseRejection:ERR_UNHANDLED_REJECTION:true:unhandledRejection:This error originated either by throwing inside of an async function without a catch block, or by rejecting a promise which was not handled with .catch(). The promise rejected with the reason "#<Object>".`,
		},
		{
			name:   "null prototype object reason formats like Node",
			reason: `Object.create(null)`,
			want:   `UnhandledPromiseRejection:ERR_UNHANDLED_REJECTION:true:unhandledRejection:This error originated either by throwing inside of an async function without a catch block, or by rejecting a promise which was not handled with .catch(). The promise rejected with the reason "[object Object]".`,
		},
		{
			name:   "array reason formats like Node",
			reason: `[1, 2]`,
			want:   `UnhandledPromiseRejection:ERR_UNHANDLED_REJECTION:true:unhandledRejection:This error originated either by throwing inside of an async function without a catch block, or by rejecting a promise which was not handled with .catch(). The promise rejected with the reason "[object Array]".`,
		},
		{
			name:   "regexp reason formats like Node",
			reason: `/x/`,
			want:   `UnhandledPromiseRejection:ERR_UNHANDLED_REJECTION:true:unhandledRejection:This error originated either by throwing inside of an async function without a catch block, or by rejecting a promise which was not handled with .catch(). The promise rejected with the reason "[object RegExp]".`,
		},
		{
			name:   "date reason formats like Node",
			reason: `new Date(0)`,
			want:   `UnhandledPromiseRejection:ERR_UNHANDLED_REJECTION:true:unhandledRejection:This error originated either by throwing inside of an async function without a catch block, or by rejecting a promise which was not handled with .catch(). The promise rejected with the reason "[object Date]".`,
		},
		{
			name:   "symbol reason formats like Node",
			reason: `Symbol("s")`,
			want:   `UnhandledPromiseRejection:ERR_UNHANDLED_REJECTION:true:unhandledRejection:This error originated either by throwing inside of an async function without a catch block, or by rejecting a promise which was not handled with .catch(). The promise rejected with the reason "Symbol(s)".`,
		},
		{
			name:   "custom class reason formats like Node",
			reason: `(new (class C {})())`,
			want:   `UnhandledPromiseRejection:ERR_UNHANDLED_REJECTION:true:unhandledRejection:This error originated either by throwing inside of an async function without a catch block, or by rejecting a promise which was not handled with .catch(). The promise rejected with the reason "#<C>".`,
		},
		{
			name:   "Map reason formats like Node",
			reason: `new Map([[1, 2]])`,
			want:   `UnhandledPromiseRejection:ERR_UNHANDLED_REJECTION:true:unhandledRejection:This error originated either by throwing inside of an async function without a catch block, or by rejecting a promise which was not handled with .catch(). The promise rejected with the reason "#<Map>".`,
		},
		{
			name:   "Set reason formats like Node",
			reason: `new Set([1, 2])`,
			want:   `UnhandledPromiseRejection:ERR_UNHANDLED_REJECTION:true:unhandledRejection:This error originated either by throwing inside of an async function without a catch block, or by rejecting a promise which was not handled with .catch(). The promise rejected with the reason "#<Set>".`,
		},
		{
			name:   "typed array reason formats like Node",
			reason: `new Uint8Array([1, 2])`,
			want:   `UnhandledPromiseRejection:ERR_UNHANDLED_REJECTION:true:unhandledRejection:This error originated either by throwing inside of an async function without a catch block, or by rejecting a promise which was not handled with .catch(). The promise rejected with the reason "[object Uint8Array]".`,
		},
		{
			name:   "ArrayBuffer reason formats like Node",
			reason: `new ArrayBuffer(2)`,
			want:   `UnhandledPromiseRejection:ERR_UNHANDLED_REJECTION:true:unhandledRejection:This error originated either by throwing inside of an async function without a catch block, or by rejecting a promise which was not handled with .catch(). The promise rejected with the reason "#<ArrayBuffer>".`,
		},
		{
			name:   "Error reason preserved",
			reason: `new TypeError("boom")`,
			want:   "TypeError:undefined:true:unhandledRejection:boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			if err := adapter.Bind(); err != nil {
				t.Fatalf("Bind: %v", err)
			}
			done := make(chan string, 1)
			if err := runtime.Set("testDone", func(value string) { done <- value }); err != nil {
				t.Fatalf("set testDone: %v", err)
			}

			_, err = runtime.RunString(`
				const events = [];
				process.on("uncaughtException", function(err, origin) {
					events.push(err.name + ":" + String(err.code) + ":" + (err instanceof Error) + ":" + origin + (err.message === undefined ? "" : ":" + err.message));
				});
				Promise.reject(` + tt.reason + `);
				setImmediate(function() { testDone(events.join(",")); });
			`)
			if err != nil {
				t.Fatalf("RunString: %v", err)
			}

			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(ctx) }()
			select {
			case got := <-done:
				if tt.name == "primitive reason wraps" {
					prefix := tt.want + ":"
					if len(got) < len(prefix) || got[:len(prefix)] != prefix {
						t.Fatalf("default unhandled rejection = %q, want prefix %q", got, prefix)
					}
				} else if got != tt.want {
					t.Fatalf("default unhandled rejection = %q, want %q", got, tt.want)
				}
			case <-ctx.Done():
				t.Fatal("timed out waiting for default unhandled rejection")
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
				t.Fatal("Run did not return")
			}
		})
	}
}

func TestDefaultUnhandledRejectionIgnoresMutableGlobals(t *testing.T) {
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
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	done := make(chan string, 1)
	if err := runtime.Set("testDone", func(value string) { done <- value }); err != nil {
		t.Fatalf("set testDone: %v", err)
	}

	_, err = runtime.RunString(`
		const events = [];
		const RealError = Error;
		const oldPrototypeError = new RealError("old-prototype");
		Error = function NotError() {};
		RealError.prototype = {};
		RealError[Symbol.hasInstance] = function() { events.push("hasInstance"); return false; };
		Object.getPrototypeOf = function() { events.push("getPrototypeOf"); return null; };
		process.on("uncaughtException", function(err, origin) {
			events.push(err.name + ":" + String(err.code) + ":" + origin + ":" + err.message);
		});
		Promise.reject(new RealError("boom"));
		Promise.reject(oldPrototypeError);
		Promise.reject({});
		setImmediate(function() { testDone(events.join("\n---\n")); });
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	select {
	case got := <-done:
		want := "Error:undefined:unhandledRejection:boom\n---\n" +
			"Error:undefined:unhandledRejection:old-prototype\n---\n" +
			`UnhandledPromiseRejection:ERR_UNHANDLED_REJECTION:unhandledRejection:This error originated either by throwing inside of an async function without a catch block, or by rejecting a promise which was not handled with .catch(). The promise rejected with the reason "#<Object>".`
		if got != want {
			t.Fatalf("default unhandled rejection mutable globals = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for default unhandled rejection")
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
		t.Fatal("Run did not return")
	}
}

func TestDefaultUnhandledRejectionFunctionDoesNotInvokePrimitiveHook(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	loop, records := newAdapterDiagnosticLoggedLoop(t)
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	done := make(chan string, 1)
	if err := runtime.Set("testDone", func(value string) { done <- value }); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.RunString(`
		globalThis.rejectionPrimitiveCalls = 0;
		globalThis.mutableFunctionToStringCalls = 0;
		function rejected(){}
		Object.defineProperty(rejected, Symbol.toPrimitive, {
			value() {
				rejectionPrimitiveCalls++;
				throw new Error("rejection function was coerced");
			}
		});
		Function.prototype.toString = function () {
			mutableFunctionToStringCalls++;
			throw new Error("mutable Function.prototype.toString ran");
		};
		process.on("uncaughtException", function(error, origin) {
			testDone(error.name + "|" + error.code + "|" + origin + "|" + error.message + "|" + rejectionPrimitiveCalls + "|" + mutableFunctionToStringCalls);
		});
		Promise.reject(rejected);
	`)
	if err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case got := <-done:
		want := `UnhandledPromiseRejection|ERR_UNHANDLED_REJECTION|unhandledRejection|This error originated either by throwing inside of an async function without a catch block, or by rejecting a promise which was not handled with .catch(). The promise rejected with the reason "function rejected(){}".|0|0`
		if got != want {
			t.Fatalf("unhandled function rejection = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for unhandled function rejection")
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not return")
	}
	if got := runtime.Get("rejectionPrimitiveCalls").ToInteger(); got != 0 {
		t.Fatalf("rejection function primitive hook ran %d times", got)
	}
	if got := runtime.Get("mutableFunctionToStringCalls").ToInteger(); got != 0 {
		t.Fatalf("mutable Function.prototype.toString ran %d times", got)
	}
	select {
	case record := <-records:
		t.Fatalf("unexpected core or adapter diagnostic: %#v", record)
	default:
	}
}
