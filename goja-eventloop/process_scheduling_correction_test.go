package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestProcessNextTickHandledThrowYieldsCheckpoint(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() {
			events.push("uncaught");
			process.nextTick(function() { events.push("handler-tick"); });
			queueMicrotask(function() { events.push("handler-micro"); });
		});
		process.nextTick(function() {
			events.push("tick1");
			process.nextTick(function() { events.push("tick2"); });
			queueMicrotask(function() { events.push("micro"); });
			throw new Error("boom");
		});
		setImmediate(function() {
			events.push("i1");
			setImmediate(function() { events.push("i2"); });
		});
	`)
	want := "tick1,uncaught,i1,tick2,handler-tick,micro,handler-micro,i2"
	if got != want {
		t.Fatalf("handled nextTick throw order = %q, want %q", got, want)
	}
}

func TestQueueMicrotaskHandledThrowContinuesCheckpoint(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() {
			events.push("uncaught");
			queueMicrotask(function() { events.push("handler-micro"); });
			process.nextTick(function() { events.push("handler-tick"); });
		});
		queueMicrotask(function() {
			events.push("micro1");
			queueMicrotask(function() { events.push("micro2"); });
			throw new Error("boom");
		});
	`)
	want := "micro1,uncaught,micro2,handler-micro,handler-tick"
	if got != want {
		t.Fatalf("handled queueMicrotask throw order = %q, want %q", got, want)
	}
}

func TestTimerHandledThrowYieldsPeerTimerCheckpoint(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() { events.push("u"); });
		setTimeout(function() {
			events.push("t1");
			process.nextTick(function() { events.push("n1"); });
			throw new Error("x");
		}, 0);
		setTimeout(function() {
			events.push("t2");
			process.nextTick(function() { events.push("n2"); });
			Promise.resolve().then(function() { events.push("p"); });
		}, 0);
		setImmediate(function() { events.push("i"); });
	`)
	const timersFirst = "t1,u,t2,n1,n2,p,i"
	const immediateFirst = "i,t1,u,t2,n1,n2,p"
	if got != timersFirst && got != immediateFirst {
		t.Fatalf("handled timer throw order = %q, want %q or %q", got, timersFirst, immediateFirst)
	}
}

func TestTimerNextTickHandledThrowYieldsPeerTimerCheckpoint(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() { events.push("u"); });
		setTimeout(function() {
			events.push("t1");
			process.nextTick(function() {
				events.push("n1");
				throw new Error("x");
			});
			process.nextTick(function() { events.push("n2"); });
			Promise.resolve().then(function() { events.push("p"); });
		}, 0);
		setTimeout(function() { events.push("t2"); }, 0);
		setImmediate(function() { events.push("i"); });
	`)
	const timersFirst = "t1,n1,u,t2,n2,p,i"
	const immediateFirst = "i,t1,n1,u,t2,n2,p"
	if got != timersFirst && got != immediateFirst {
		t.Fatalf("handled timer nextTick throw order = %q, want %q or %q", got, timersFirst, immediateFirst)
	}
}

func TestProcessBeforeExitHandledThrowRepeatsLifecycle(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		let first = true;
		process.on("uncaughtException", function(error) {
			events.push("x:" + error.message);
		});
		process.on("beforeExit", function() {
			events.push("before");
			if (first) {
				first = false;
				throw new Error("boom");
			}
		});
		process.on("exit", function() { events.push("exit"); });
	`)
	want := "before,x:boom,before,exit"
	if got != want {
		t.Fatalf("handled beforeExit throw order = %q, want %q", got, want)
	}
}

func TestUnhandledRejectionHandledThrowDropsPendingSnapshot(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function(error) {
			events.push("x:" + error.message);
		});
		process.on("unhandledRejection", function(reason) {
			events.push("u:" + reason);
			if (reason === "a") throw new Error("listener");
		});
		Promise.reject("a");
		Promise.reject("b");
		setImmediate(function() { events.push("i1"); });
		setImmediate(function() { events.push("i2"); });
	`)
	want := "u:a,x:listener,i1,i2"
	if got != want {
		t.Fatalf("handled unhandledRejection throw order = %q, want %q", got, want)
	}
}

func TestRejectionHandledPrecedesNewUnhandledRejection(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("unhandledRejection", function(reason) {
			events.push("u:" + reason);
		});
		process.on("rejectionHandled", function(promise) {
			events.push("h:" + (promise === old ? "old" : "other"));
		});
		const old = Promise.reject("old");
		setImmediate(function() {
			Promise.reject("new");
			old.catch(function() {});
		});
	`)
	want := "u:old,h:old,u:new"
	if got != want {
		t.Fatalf("rejection checkpoint order = %q, want %q", got, want)
	}
}

func TestRejectionHandledWarningIncludesPromiseID(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("unhandledRejection", function() {
			events.push("unhandled");
		});
		process.on("warning", function(warning) {
			events.push("warning:" + warning.name + ":" + String(warning.code) + ":" + warning.message);
		});
		const promise = Promise.reject("old");
		setImmediate(function() { promise.catch(function() {}); });
	`)
	want := "unhandled,warning:PromiseRejectionHandledWarning:undefined:Promise rejection was handled asynchronously (rejection id: 1)"
	if got != want {
		t.Fatalf("late rejection warning = %q, want %q", got, want)
	}
}

func TestProcessFatalHandlerFailuresUseNodeExitCodes(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   int
	}{
		{
			name: "broken process emitter",
			script: `
				process.emit = 1;
				process.nextTick(function() { throw new Error("boom"); });
			`,
			want: 7,
		},
		{
			name: "throwing process emitter accessor",
			script: `
				Object.defineProperty(process, "emit", {
					get() { throw new Error("getter"); },
				});
				process.nextTick(function() { throw new Error("boom"); });
			`,
			want: 7,
		},
		{
			name: "uncaught exception listener throws",
			script: `
				process.on("uncaughtException", function() { throw new Error("handler"); });
				process.nextTick(function() { throw new Error("boom"); });
			`,
			want: 7,
		},
		{
			name: "explicit nonzero exit survives exit listener throw",
			script: `
				process.on("exit", function() { throw new Error("exit"); });
				setImmediate(function() { process.exit(7); });
			`,
			want: 7,
		},
		{
			name: "natural nonzero exit survives exit listener throw",
			script: `
				process.exitCode = 4;
				process.on("exit", function() { throw new Error("exit"); });
			`,
			want: 4,
		},
		{
			name: "zero exit listener throw becomes failure",
			script: `
				process.on("exit", function() { throw new Error("exit"); });
			`,
			want: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := runFatalProcessScript(t, test.script)
			if got := adapter.currentExitCode(); got != test.want {
				t.Fatalf("exit code = %d, want %d", got, test.want)
			}
		})
	}
}

func TestPromiseJobProcessExitIsConsumed(t *testing.T) {
	adapter := runFatalProcessScript(t, `
		globalThis.events = [];
		process.on("exit", function(code) { events.push("exit:" + code); });
		Promise.resolve().then(function() {
			process.exit(7);
			events.push("late");
		});
		setImmediate(function() { events.push("immediate"); });
	`)
	value, err := adapter.runtime.RunString(`events.join(",")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if got, want := value.String(), "exit:7"; got != want {
		t.Fatalf("Promise process.exit events = %q, want %q", got, want)
	}
	if got := adapter.currentExitCode(); got != 7 {
		t.Fatalf("Promise process.exit code = %d, want 7", got)
	}
}

func runFatalProcessScript(t *testing.T, script string) *Adapter {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	loop := goeventloop.New(goeventloop.WithAutoExit(true))
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return adapter
}
