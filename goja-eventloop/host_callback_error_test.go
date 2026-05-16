package gojaeventloop

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestNewForwardsJSOptions(t *testing.T) {
	loop, err := goeventloop.New(goeventloop.WithAutoExit(true))
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	reported := make(chan any, 1)
	adapter, err := New(loop, runtime, goeventloop.WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, _, reject := adapter.js.NewChainedPromise()
	reject("forwarded-option")

	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	select {
	case reason := <-reported:
		if reason != "forwarded-option" {
			t.Fatalf("reported reason = %v, want forwarded-option", reason)
		}
	default:
		t.Fatal("unhandled-rejection option was not forwarded to underlying JS adapter")
	}
}

func TestHostCallbackErrorsEmitProcessUncaughtException(t *testing.T) {
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
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	done := make(chan any, 1)
	if err := runtime.Set("testDone", func(value goja.Value) { done <- value.Export() }); err != nil {
		t.Fatalf("set testDone: %v", err)
	}

	_, err = runtime.RunString(`
		const seen = [];
		function record(kind, err, origin) {
			seen.push(kind + ":" + err.message + ":" + origin);
			if (seen.filter((entry) => entry.startsWith("handler:")).length === 5) {
				testDone(seen);
			}
		}
		process.on("uncaughtExceptionMonitor", function (err, origin) { record("monitor", err, origin); });
		process.on("uncaughtException", function (err, origin) { record("handler", err, origin); });
		process.nextTick(function () { throw new Error("tick boom"); });
		queueMicrotask(function () { throw new Error("micro boom"); });
		setImmediate(function () { throw new Error("immediate boom"); });
		setTimeout(function () { throw new Error("timer boom"); }, 0);
		const intervalHandle = setInterval(function () {
			clearInterval(intervalHandle);
			throw new Error("interval boom");
		}, 0);
	`)
	if err != nil {
		t.Fatalf("setup script failed: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	select {
	case raw := <-done:
		entries, ok := raw.([]any)
		if !ok {
			t.Fatalf("testDone value = %T %#v, want []any", raw, raw)
		}
		wantMessages := map[string]int{
			"tick boom":      0,
			"micro boom":     0,
			"immediate boom": 0,
			"timer boom":     0,
			"interval boom":  0,
		}
		for _, entry := range entries {
			text, ok := entry.(string)
			if !ok {
				t.Fatalf("entry = %T %#v, want string", entry, entry)
			}
			if !strings.Contains(text, ":uncaughtException") {
				t.Fatalf("entry %q missing uncaughtException origin", text)
			}
			for message := range wantMessages {
				if strings.Contains(text, message) {
					wantMessages[message]++
				}
			}
		}
		for message, count := range wantMessages {
			if count != 2 {
				t.Fatalf("message %q event count = %d, want monitor+handler entries; all entries %#v", message, count, entries)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for process uncaughtException events")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := loop.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}
}

func TestDefaultFatalHostCallbackStopsLaterJSCallbacks(t *testing.T) {
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
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		globalThis.events = [];
		process.on("exit", function() {
			events.push("exit");
			queueMicrotask(function() { events.push("exitMicrotask"); });
			Promise.resolve().then(function() { events.push("exitPromise"); });
		});
		setTimeout(function() {
			events.push("first");
			throw new Error("boom");
		}, 0);
		setTimeout(function() { events.push("second"); }, 0);
		queueMicrotask(function() { events.push("queuedMicrotask"); });
	`)
	if err != nil {
		t.Fatalf("setup script failed: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not return after fatal callback")
	}

	value, err := runtime.RunString(`events.join(",")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if got, want := value.String(), "queuedMicrotask,first,exit"; got != want {
		t.Fatalf("events after fatal callback = %q, want %q", got, want)
	}
}

func TestAbortSignalDefaultFatalStopsLaterCallbacksAndLogsFirst(t *testing.T) {
	loop, records := newAdapterDiagnosticLoggedLoop(t)
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const controller = new AbortController();
		controller.signal.onabort = function () { throw new Error("onabort boom"); };
		controller.signal.addEventListener("abort", function () { throw new Error("listener boom"); });
		controller.abort("stop");
	`)
	if err != nil {
		t.Fatalf("abort script failed: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	record := receiveAdapterDiagnosticLog(t, records)
	if record.message != "goja host callback failed" {
		t.Fatalf("logged message = %q, want host callback diagnostic", record.message)
	}
	if record.callback != "EventTarget.addEventListener" {
		t.Fatalf("logged callback = %q, want EventTarget.addEventListener", record.callback)
	}
	var exception *goja.Exception
	if record.err == nil || !errors.As(record.err, &exception) || exception == nil {
		t.Fatalf("logged error = %v, want exact Goja exception", record.err)
	}
	if message := exception.Value().ToObject(runtime).Get("message").String(); message != "onabort boom" {
		t.Fatalf("logged exception message = %q, want onabort boom", message)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after fatal abort listener")
	}
	select {
	case extra := <-records:
		t.Fatalf("unexpected callback after default fatal path: %#v", extra)
	default:
	}
}

func TestPromiseFinallyCallbackErrorRejectsDerivedPromise(t *testing.T) {
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
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	settled := make(chan string, 1)
	if err := runtime.Set("settled", func(value string) { settled <- value }); err != nil {
		t.Fatalf("set settled: %v", err)
	}

	_, err = runtime.RunString(`
		Promise.resolve("kept")
			.finally(function () { throw new Error("finally boom"); })
			.then(
				function () { settled("fulfilled"); },
				function (err) { settled(err.message); }
			);
	`)
	if err != nil {
		t.Fatalf("finally script failed: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	select {
	case got := <-settled:
		if got != "finally boom" {
			t.Fatalf("Promise.finally rejection = %q, want finally boom", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Promise.finally chain did not settle")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := loop.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}
}

func TestEventTargetCallbackErrorUsesLoopLogger(t *testing.T) {
	loop, records := newAdapterDiagnosticLoggedLoop(t)
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		var target = new EventTarget();
		target.addEventListener("boom", function () { throw new Error("listener boom"); });
		target.dispatchEvent(new Event("boom"));
	`)
	if err != nil {
		t.Fatalf("dispatch script failed: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	record := receiveAdapterDiagnosticLog(t, records)
	if record.message != "goja host callback failed" {
		t.Fatalf("logged message = %q, want host callback diagnostic", record.message)
	}
	if record.callback != "EventTarget.addEventListener" {
		t.Fatalf("logged callback = %q, want EventTarget.addEventListener", record.callback)
	}
	var exception *goja.Exception
	if record.err == nil || !errors.As(record.err, &exception) || exception == nil {
		t.Fatalf("logged error = %v, want exact Goja exception", record.err)
	}
	if message := exception.Value().ToObject(runtime).Get("message").String(); message != "listener boom" {
		t.Fatalf("logged exception message = %q, want listener boom", message)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after fatal event listener")
	}
}

func TestProcessListenerDiagnosticDoesNotCoerceThrownValue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	loop, records := newAdapterDiagnosticLoggedLoop(t)
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.RunString(`
		globalThis.callbackCoercions = 0;
		globalThis.callbackThrown = {
			[Symbol.toPrimitive]() {
				callbackCoercions++;
				throw new Error("callback exception was coerced");
			}
		};
		process.on("uncaughtException", function () { throw callbackThrown; });
		setImmediate(function () { throw new Error("trigger process listener"); });
	`)
	if err != nil {
		t.Fatal(err)
	}
	thrown := runtime.Get("callbackThrown")
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	record := receiveAdapterDiagnosticLog(t, records)
	if record.callback != "process.uncaughtException" {
		t.Fatalf("logged callback = %q, want process.uncaughtException", record.callback)
	}
	var exception *goja.Exception
	if !errors.As(record.err, &exception) || exception == nil || exception.Value() != thrown {
		t.Fatal("process listener diagnostic did not preserve the exact thrown value")
	}
	if got := record.err.Error(); got != "goja-eventloop: report process.uncaughtException: JavaScript exception" {
		t.Fatalf("logged error text = %q", got)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not return after fatal process listener")
	}
	if got := runtime.Get("callbackCoercions").ToInteger(); got != 0 {
		t.Fatalf("process listener exception was coerced %d times", got)
	}
}

func TestEventTargetThrownDiagnosticDoesNotCoerceReason(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	loop, records := newAdapterDiagnosticLoggedLoop(t)
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.RunString(`
		globalThis.rejectionCoercions = 0;
		globalThis.rejectionReason = {
			[Symbol.toPrimitive]() {
				rejectionCoercions++;
				throw new Error("rejection reason was coerced");
			}
		};
		const target = new EventTarget();
		target.addEventListener("reject", function () {
			throw rejectionReason;
		});
		target.dispatchEvent(new Event("reject"));
	`)
	if err != nil {
		t.Fatal(err)
	}
	thrown := runtime.Get("rejectionReason")
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	record := receiveAdapterDiagnosticLog(t, records)
	if record.callback != "EventTarget.addEventListener" {
		t.Fatalf("logged callback = %q, want EventTarget.addEventListener", record.callback)
	}
	var exception *goja.Exception
	if !errors.As(record.err, &exception) || exception == nil || exception.Value() != thrown {
		t.Fatal("EventTarget thrown diagnostic did not preserve the exact reason")
	}
	if got := record.err.Error(); got != "goja-eventloop: report EventTarget.addEventListener: JavaScript exception" {
		t.Fatalf("logged error text = %q", got)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not return after fatal EventTarget exception")
	}
	if got := runtime.Get("rejectionCoercions").ToInteger(); got != 0 {
		t.Fatalf("EventTarget rejection reason was coerced %d times", got)
	}
}
