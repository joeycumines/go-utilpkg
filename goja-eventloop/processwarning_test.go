package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestProcessEmitWarningIsDeferred(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop := goeventloop.New()
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	done := make(chan string, 1)
	if err := runtime.Set("testDone", func(value string) { done <- value }); err != nil {
		t.Fatalf("set testDone: %v", err)
	}

	value, err := runtime.RunString(`
		const events = [];
		process.on("warning", function(warning) {
			events.push("warning:" + warning.name + ":" + warning.message + ":" +
				warning.constructor.name + ":" + (warning instanceof Error) + ":" +
				(Object.getPrototypeOf(warning) === Error.prototype) + ":" +
				Object.keys(warning).join("|") + ":" +
				Object.getOwnPropertyNames(warning).join("|"));
		});
		process.emitWarning("x", "CustomWarning");
		events.push("after");
		process.nextTick(function() {
			events.push("nextTick");
			testDone(events.join(","));
		});
		events.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "after"; got != want {
		t.Fatalf("process.emitWarning emitted synchronously: events = %q, want %q", got, want)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	select {
	case got := <-done:
		want := "after,warning:CustomWarning:x:Error:true:true:name:stack|message|name,nextTick"
		if got != want {
			t.Fatalf("process.emitWarning ordering = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for deferred warning")
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

func TestProcessEmitWarningValidation(t *testing.T) {
	loop := goeventloop.New()
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	value, err := runtime.RunString(`
		const events = [];
		function record(label, fn) {
			try { fn(); events.push(label + ":ok"); }
			catch (err) { events.push(label + ":" + err.name + ":" + err.code); }
		}
		function recordMessage(label, fn, want) {
			try { fn(); events.push(label + ":ok"); }
			catch (err) { events.push(label + ":" + err.name + ":" + err.code + ":" + (err.message === want)); }
		}
		record("missing", function() { process.emitWarning(); });
		record("undefined", function() { process.emitWarning(undefined); });
		record("null", function() { process.emitWarning(null); });
		record("number", function() { process.emitWarning(1); });
		record("object", function() { process.emitWarning({}); });
		record("error", function() { process.emitWarning(new Error("e")); });
		record("option-type-number", function() { process.emitWarning("x", { type: 1 }); });
		record("option-type-null", function() { process.emitWarning("x", { type: null }); });
		record("option-code-number", function() { process.emitWarning("x", { code: 1 }); });
		record("option-code-null", function() { process.emitWarning("x", { code: null }); });
		record("arg-type-null", function() { process.emitWarning("x", null); });
		record("arg-code-null", function() { process.emitWarning("x", "T", null); });
		record("valid-options", function() { process.emitWarning("x", { type: "T", code: "C" }); });
		record("error-option-type-number", function() { process.emitWarning(new Error("e"), { type: 1 }); });
		record("error-option-code-number", function() { process.emitWarning(new Error("e"), { code: 1 }); });
		record("error-option-code-null", function() { process.emitWarning(new Error("e"), { code: null }); });
		record("error-option-detail-number", function() { process.emitWarning(new Error("e"), { detail: 1 }); });
		record("error-arg-type-null", function() { process.emitWarning(new Error("e"), null); });
		record("error-arg-type-string", function() { process.emitWarning(new Error("e"), "T"); });
		record("error-arg-code-null", function() { process.emitWarning(new Error("e"), "T", null); });
		recordMessage("message-option-type-number", function() { process.emitWarning("x", { type: 1 }); }, 'The "type" argument must be of type string. Received type number (1)');
		recordMessage("message-option-code-number", function() { process.emitWarning("x", { code: 1 }); }, 'The "code" argument must be of type string. Received type number (1)');
		recordMessage("message-option-code-null", function() { process.emitWarning("x", { code: null }); }, 'The "code" argument must be of type string. Received null');
		recordMessage("message-arg-type-null", function() { process.emitWarning("x", null); }, 'The "type" argument must be of type string. Received null');
		recordMessage("message-arg-code-null", function() { process.emitWarning("x", "T", null); }, 'The "code" argument must be of type string. Received null');
		events.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "missing:TypeError:ERR_INVALID_ARG_TYPE," +
		"undefined:TypeError:ERR_INVALID_ARG_TYPE," +
		"null:TypeError:ERR_INVALID_ARG_TYPE," +
		"number:TypeError:ERR_INVALID_ARG_TYPE," +
		"object:TypeError:ERR_INVALID_ARG_TYPE," +
		"error:ok," +
		"option-type-number:TypeError:ERR_INVALID_ARG_TYPE," +
		"option-type-null:ok," +
		"option-code-number:TypeError:ERR_INVALID_ARG_TYPE," +
		"option-code-null:TypeError:ERR_INVALID_ARG_TYPE," +
		"arg-type-null:TypeError:ERR_INVALID_ARG_TYPE," +
		"arg-code-null:TypeError:ERR_INVALID_ARG_TYPE," +
		"valid-options:ok," +
		"error-option-type-number:TypeError:ERR_INVALID_ARG_TYPE," +
		"error-option-code-number:TypeError:ERR_INVALID_ARG_TYPE," +
		"error-option-code-null:TypeError:ERR_INVALID_ARG_TYPE," +
		"error-option-detail-number:ok," +
		"error-arg-type-null:TypeError:ERR_INVALID_ARG_TYPE," +
		"error-arg-type-string:ok," +
		"error-arg-code-null:TypeError:ERR_INVALID_ARG_TYPE," +
		"message-option-type-number:TypeError:ERR_INVALID_ARG_TYPE:true," +
		"message-option-code-number:TypeError:ERR_INVALID_ARG_TYPE:true," +
		"message-option-code-null:TypeError:ERR_INVALID_ARG_TYPE:true," +
		"message-arg-type-null:TypeError:ERR_INVALID_ARG_TYPE:true," +
		"message-arg-code-null:TypeError:ERR_INVALID_ARG_TYPE:true"
	if got := value.String(); got != want {
		t.Fatalf("process.emitWarning validation = %q, want %q", got, want)
	}
}

func TestProcessEmitWarningErrorNameGetterThrowsSynchronously(t *testing.T) {
	loop := goeventloop.New()
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	value, err := runtime.RunString(`
		let thrownCoercions = 0;
		const thrown = {
			[Symbol.toPrimitive]() {
				thrownCoercions++;
				throw new Error("warning exception was coerced");
			}
		};
		const warning = new Error("warning");
		Object.defineProperty(warning, "name", { get() { throw thrown; } });
		let caught;
		try { process.emitWarning(warning); }
		catch (error) { caught = error; }
		[caught === thrown, thrownCoercions].join(":");
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := value.String(), "true:0"; got != want {
		t.Fatalf("synchronous warning getter result = %q, want %q", got, want)
	}
}

func TestProcessEmitWarningFallbackPreservesConversionException(t *testing.T) {
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
		globalThis.warningCodeConversions = 0;
		globalThis.warningThrownCoercions = 0;
		globalThis.warningThrown = {
			[Symbol.toPrimitive]() {
				warningThrownCoercions++;
				throw new Error("warning exception was coerced");
			}
		};
		const code = {
			[Symbol.toPrimitive]() {
				warningCodeConversions++;
				throw warningThrown;
			}
		};
		const warning = new Error("warning");
		warning.code = code;
		process.on("uncaughtException", function(error, origin) {
			testDone([error === warningThrown, origin, warningCodeConversions, warningThrownCoercions].join(":"));
		});
		process.emitWarning(warning);
	`)
	if err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case got := <-done:
		if want := "true:uncaughtException:1:0"; got != want {
			t.Fatalf("warning fallback exception = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for warning fallback exception")
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
	if got := runtime.Get("warningThrownCoercions").ToInteger(); got != 0 {
		t.Fatalf("warning exception was coerced %d times", got)
	}
	select {
	case record := <-records:
		t.Fatalf("unexpected raw core or adapter diagnostic: %#v", record)
	default:
	}
}

func TestProcessEmitWarningDetailAndEmptyCode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop := goeventloop.New()
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
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
		process.on("warning", function(warning) {
			events.push(warning.name + "|" + String(warning.code) + "|" + Object.prototype.hasOwnProperty.call(warning, "code") + "|" + warning.message + "|" + String(warning.detail) + "|" + (typeof warning.stack));
			if (events.length === 2) testDone(events.join("\n"));
		});
		process.emitWarning("msg", { type: "MyWarning", code: "MY_CODE", detail: "details" });
		process.emitWarning("x", { code: "" });
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	select {
	case got := <-done:
		want := "MyWarning|MY_CODE|true|msg|details|string\nWarning||true|x|undefined|string"
		if got != want {
			t.Fatalf("process.emitWarning detail/code = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for warning detail")
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
