package gojaeventloop

import (
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestProcessEmitUnhandledErrorThrows(t *testing.T) {
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
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	value, err := runtime.RunString(`
		const events = [];
		const RealError = Error;
		try {
			events.push("emit:" + process.emit("error", new RealError("boom")));
		} catch (err) {
			events.push("caught:" + err.name + ":" + String(err.code) + ":" + err.message);
		}
		Error = function NotError() {};
		try { process.emit("error", new RealError("mutated")); }
		catch (err) { events.push("mutated:" + err.name + ":" + String(err.code) + ":" + err.message); }
		const oldPrototypeError = new RealError("old-prototype");
		RealError.prototype = {};
		RealError[Symbol.hasInstance] = function() { events.push("hasInstance"); return false; };
		try { process.emit("error", oldPrototypeError); }
		catch (err) { events.push("brand:" + err.name + ":" + String(err.code) + ":" + err.message); }
		const proxyValue = new Proxy({a: 1}, {
			get(target, prop, receiver) { events.push("proxy-get:" + String(prop)); return Reflect.get(target, prop, receiver); },
			ownKeys(target) { events.push("proxy-ownKeys"); return Reflect.ownKeys(target); },
			getOwnPropertyDescriptor(target, prop) { events.push("proxy-desc:" + String(prop)); return Reflect.getOwnPropertyDescriptor(target, prop); },
		});
		class C {}
		for (const value of [undefined, "boom", 5, null, {a: 1}, Object.create(null), function f(){}, new Date(0), { get a() { events.push("getter"); return 1; } }, { nested: { b: 2 } }, [1, { b: 2 }], Symbol("s"), proxyValue, new C(), new Map([[1, 2]]), new Set([1, 2]), new Uint8Array([1, 2])]) {
			try { process.emit("error", value); }
			catch (err) { events.push("nonerror:" + err.name + ":" + err.code + ":" + err.message); }
		}
		process.on("error", function(err) { events.push("listener:" + err.message); });
		events.push("emit2:" + process.emit("error", new RealError("handled")));
		events.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "caught:Error:undefined:boom,mutated:Error:undefined:mutated," +
		"brand:Error:undefined:old-prototype," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. (undefined)," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. ('boom')," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. (5)," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. (null)," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. ({ a: 1 })," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. ([Object: null prototype] {})," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. ([Function: f])," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. (1970-01-01T00:00:00.000Z)," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. ({ a: [Getter] })," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. ({ nested: { b: 2 } })," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. ([ 1, { b: 2 } ])," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. (Symbol(s))," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. (Proxy({ a: 1 }))," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. (C {})," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. (Map(1) { 1 => 2 })," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. (Set(2) { 1, 2 })," +
		"nonerror:Error:ERR_UNHANDLED_ERROR:Unhandled error. (Uint8Array(2) [ 1, 2 ])," +
		"listener:handled,emit2:true"
	if got := value.String(); got != want {
		t.Fatalf("process.emit error events = %q, want %q", got, want)
	}
}

func TestProcessFatalListenerThrowStopsLaterListeners(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name: "exit listener throw",
			script: `
				globalThis.events = [];
				process.on("exit", function() { events.push("exit-a"); throw new Error("exit boom"); });
				process.on("exit", function() { events.push("exit-b"); });
				setTimeout(function() { events.push("timer"); throw new Error("timer boom"); }, 0);
			`,
			want: "timer,exit-a",
		},
		{
			name: "uncaughtException listener throw",
			script: `
				globalThis.events = [];
				process.on("uncaughtException", function() { events.push("uncaught-a"); throw new Error("handler boom"); });
				process.on("uncaughtException", function() { events.push("uncaught-b"); });
				process.on("exit", function() { events.push("exit"); });
				setTimeout(function() { events.push("timer"); throw new Error("timer boom"); }, 0);
			`,
			want: "timer,uncaught-a",
		},
		{
			name: "uncaughtExceptionMonitor listener throw",
			script: `
				globalThis.events = [];
				process.on("uncaughtExceptionMonitor", function() { events.push("monitor"); throw new Error("monitor boom"); });
				process.on("uncaughtException", function() { events.push("uncaught"); });
				process.on("exit", function() { events.push("exit"); });
				setTimeout(function() { events.push("timer"); throw new Error("timer boom"); }, 0);
			`,
			want: "timer,monitor",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runAutoExitProcessScript(t, tt.script)
			if got != tt.want {
				t.Fatalf("process fatal listener events = %q, want %q", got, tt.want)
			}
		})
	}
}
