package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestProcessBeforeExitCanExtendAutoExitAndExitIsTerminal(t *testing.T) {
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

	_, err = runtime.RunString(`
		globalThis.events = [];
		let scheduled = false;
		process.on("beforeExit", function(code) {
			events.push("beforeExit:" + code);
			if (!scheduled) {
				scheduled = true;
				process.nextTick(function() { events.push("nextTick"); });
				Promise.resolve().then(function() { events.push("promise"); });
				setImmediate(function() { events.push("immediate"); });
			}
		});
		process.on("exit", function(code) {
			events.push("exit:" + code + ":" + process._exiting);
			process.on("warning", function(warning) { events.push("warning:" + warning.name); });
			process.nextTick(function() { events.push("lateNextTick"); });
			queueMicrotask(function() { events.push("lateMicrotask"); });
			Promise.resolve().then(function() { events.push("latePromise"); });
			setImmediate(function() { events.push("lateImmediate"); });
			setTimeout(function() { events.push("lateInvalidTimeout"); }, -1);
			setTimeout(function() { events.push("lateTimeout"); }, 0);
		});
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not return after process exit")
	}

	value, err := runtime.RunString(`events.join(",")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	got := value.String()
	want := "beforeExit:0,nextTick,promise,immediate,beforeExit:0,exit:0:true"
	if got != want {
		t.Fatalf("process lifecycle events = %q, want %q", got, want)
	}
}

func TestProcessBeforeExitTimerExtendsAutoExitWithNodeOrdering(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		let count = 0;
		process.on("beforeExit", function() {
			count += 1;
			events.push("beforeExit" + count);
			if (count === 1) {
				process.nextTick(function() { events.push("nextTick-beforeExit"); });
				Promise.resolve().then(function() { events.push("promise-beforeExit"); });
				setTimeout(function() { events.push("timeout-beforeExit"); }, 0);
			}
		});
	`)
	if want := "beforeExit1,nextTick-beforeExit,promise-beforeExit,timeout-beforeExit,beforeExit2"; got != want {
		t.Fatalf("beforeExit timer scheduling order = %q, want %q", got, want)
	}
}

func TestProcessBeforeExitMicrotasksDoNotRepeatBeforeExit(t *testing.T) {
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

	_, err = runtime.RunString(`
		globalThis.events = [];
		let count = 0;
		process.on("beforeExit", function() {
			events.push("before" + count);
			if (count++ < 1) {
				queueMicrotask(function() { events.push("micro"); });
			}
		});
		process.on("exit", function() { events.push("exit"); });
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not return after process exit")
	}

	value, err := runtime.RunString(`events.join(",")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if got, want := value.String(), "before0,micro,exit"; got != want {
		t.Fatalf("process lifecycle events = %q, want %q", got, want)
	}
}

func TestProcessBeforeExitMicrotaskMacrotaskRepeatsBeforeExit(t *testing.T) {
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

	_, err = runtime.RunString(`
		globalThis.events = [];
		let count = 0;
		process.on("beforeExit", function() {
			events.push("before" + count);
			if (count++ < 1) {
				queueMicrotask(function() {
					events.push("micro");
					setImmediate(function() { events.push("immediate"); });
				});
			}
		});
		process.on("exit", function() { events.push("exit"); });
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not return after process exit")
	}

	value, err := runtime.RunString(`events.join(",")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if got, want := value.String(), "before0,micro,immediate,before1,exit"; got != want {
		t.Fatalf("process lifecycle events = %q, want %q", got, want)
	}
}

func TestProcessExitSuppressesAsyncExitWorkAndStopsScript(t *testing.T) {
	loop := goeventloop.New()
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	_, err = runtime.RunString(`
		globalThis.events = [];
		process.exitCode = 4;
		process.on("exit", function(code) {
			events.push("exit:" + code + ":" + process._exiting + ":" + process.exitCode);
			process.nextTick(function() { events.push("tick"); });
			queueMicrotask(function() { events.push("micro"); });
			Promise.resolve().then(function() { events.push("promise"); });
			setImmediate(function() { events.push("immediate"); });
			setTimeout(function() { events.push("timeout"); }, 0);
		});
		process.exit(7);
		events.push("after");
	`)
	if code, ok := processExitCode(err); !ok || code != 7 {
		t.Fatalf("RunString error = %v, want process exit signal code 7", err)
	}
	runtime.ClearInterrupt()

	value, err := runtime.RunString(`events.join(",")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if got, want := value.String(), "exit:7:true:7"; got != want {
		t.Fatalf("process.exit events = %q, want %q", got, want)
	}
}

func TestProcessExitExplicitNullishClearsExitCode(t *testing.T) {
	tests := []struct {
		name string
		exit string
		want string
	}{
		{name: "undefined", exit: "process.exit(undefined);", want: "exit:0:undefined"},
		{name: "null", exit: "process.exit(null);", want: "exit:0:undefined"},
		{name: "no argument", exit: "process.exit();", want: "exit:4:4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop := goeventloop.New()
			runtime := goja.New()
			adapter, err := New(loop, runtime)
			if err != nil {
				t.Fatalf("New adapter: %v", err)
			}
			if err := adapter.Bind(); err != nil {
				t.Fatalf("Bind: %v", err)
			}
			_, err = runtime.RunString(`
				globalThis.events = [];
				process.exitCode = 4;
				process.on("exit", function(code) { events.push("exit:" + code + ":" + String(process.exitCode)); });
				` + tt.exit + `
			`)
			if _, ok := processExitCode(err); !ok {
				t.Fatalf("RunString error = %v, want process exit signal", err)
			}
			runtime.ClearInterrupt()
			value, err := runtime.RunString(`events.join(",")`)
			if err != nil {
				t.Fatalf("read events: %v", err)
			}
			if got := value.String(); got != tt.want {
				t.Fatalf("process.exit nullish events = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProcessExitNoArgumentClearsNullExitCode(t *testing.T) {
	loop := goeventloop.New()
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	_, err = runtime.RunString(`
		globalThis.events = [];
		process.exitCode = null;
		process.on("exit", function(code) { events.push("exit:" + code + ":" + String(process.exitCode)); });
		process.exit();
	`)
	if _, ok := processExitCode(err); !ok {
		t.Fatalf("RunString error = %v, want process exit signal", err)
	}
	runtime.ClearInterrupt()
	value, err := runtime.RunString(`events.join(",")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if got, want := value.String(), "exit:0:undefined"; got != want {
		t.Fatalf("process.exit after null exitCode = %q, want %q", got, want)
	}
}

func TestProcessExitCodeNaturalAutoExit(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name: "pre-set exitCode feeds beforeExit and exit",
			script: `
				globalThis.events = [];
				process.exitCode = 4;
				process.on("beforeExit", function(code) { events.push("before:" + code + ":" + process.exitCode); });
				process.on("exit", function(code) { events.push("exit:" + code + ":" + process.exitCode); });
			`,
			want: "before:4:4,exit:4:4",
		},
		{
			name: "beforeExit can set exitCode",
			script: `
				globalThis.events = [];
				process.on("beforeExit", function(code) {
					events.push("before:" + code + ":" + process.exitCode);
					process.exitCode = 5;
				});
				process.on("exit", function(code) { events.push("exit:" + code + ":" + process.exitCode); });
			`,
			want: "before:0:undefined,exit:5:5",
		},
		{
			name: "unset exitCode remains undefined",
			script: `
				globalThis.events = [];
				process.on("exit", function(code) { events.push("exit:" + code + ":" + process.exitCode); });
			`,
			want: "exit:0:undefined",
		},
		{
			name: "null exitCode clears immediately",
			script: `
				globalThis.events = [];
				process.exitCode = null;
				events.push("assigned:" + String(process.exitCode));
				process.on("beforeExit", function(code) { events.push("before:" + code + ":" + String(process.exitCode)); });
				process.on("exit", function(code) { events.push("exit:" + code + ":" + String(process.exitCode)); });
			`,
			want: "assigned:undefined,before:0:undefined,exit:0:undefined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runAutoExitProcessScript(t, tt.script)
			if got != tt.want {
				t.Fatalf("process lifecycle events = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProcessExitFromBeforeExitIsClean(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function(err) { events.push("uncaught:" + err.message); });
		process.on("beforeExit", function() {
			events.push("before");
			process.exit(7);
			events.push("after-before");
		});
		process.on("exit", function(code) { events.push("exit:" + code + ":" + process._exiting + ":" + process.exitCode); });
	`)
	if want := "before,exit:7:true:7"; got != want {
		t.Fatalf("process lifecycle events = %q, want %q", got, want)
	}
}

func TestProcessExitCodeRejectsInvalidValues(t *testing.T) {
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
			try {
				fn();
				events.push(label + ":ok");
			} catch (err) {
				events.push(label + ":" + err.name + ":" + err.code);
			}
		}
		record("exit-abc", function() { process.exit("abc"); });
		record("exit-fraction", function() { process.exit(1.5); });
		record("exit-unsafe-high", function() { process.exit(9007199254740992); });
		record("exit-unsafe-low-string", function() { process.exit("-9007199254740992"); });
		record("exit-object", function() { process.exit({ valueOf() { return 5; } }); });
		record("exit-boxed-number", function() { process.exit(new Number(5)); });
		record("exitCode-abc", function() { process.exitCode = "abc"; });
		record("exitCode-fraction", function() { process.exitCode = 1.5; });
		record("exitCode-unsafe-high", function() { process.exitCode = 9007199254740992; });
		record("exitCode-unsafe-low-string", function() { process.exitCode = "-9007199254740992"; });
		record("exitCode-object", function() { process.exitCode = {}; });
		record("exitCode-valueOf-object", function() { process.exitCode = { valueOf() { return 5; } }; });
		record("exitCode-boxed-number", function() { process.exitCode = new Number(5); });
		record("exitCode-object-number", function() { process.exitCode = Object(5); });
		record("exitCode-boxed-string", function() { process.exitCode = new String("5"); });
		process.exitCode = 3;
		process.exitCode = null;
		events.push("null:" + String(process.exitCode));
		process.exitCode = "1";
		events.push("numeric-string:" + process.exitCode);
		process.exitCode = " ";
		events.push("whitespace-string:" + process.exitCode);
		process.exitCode = "0x10";
		events.push("hex-string:" + process.exitCode);
		process.exitCode = "0b10";
		events.push("binary-string:" + process.exitCode);
		process.exitCode = "0o10";
		events.push("octal-string:" + process.exitCode);
		process.exitCode = Number.MAX_SAFE_INTEGER;
		events.push("safe-high:" + process.exitCode);
		process.exitCode = Number.MIN_SAFE_INTEGER;
		events.push("safe-low:" + process.exitCode);
		process.exitCode = 4294967296;
		events.push("wrap-zero:" + process.exitCode);
		process.exitCode = 4294967295;
		events.push("wrap-negative-one:" + process.exitCode);
		events.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "exit-abc:TypeError:ERR_INVALID_ARG_TYPE,exit-fraction:RangeError:ERR_OUT_OF_RANGE,exit-unsafe-high:RangeError:ERR_OUT_OF_RANGE,exit-unsafe-low-string:RangeError:ERR_OUT_OF_RANGE,exit-object:TypeError:ERR_INVALID_ARG_TYPE,exit-boxed-number:TypeError:ERR_INVALID_ARG_TYPE,exitCode-abc:TypeError:ERR_INVALID_ARG_TYPE,exitCode-fraction:RangeError:ERR_OUT_OF_RANGE,exitCode-unsafe-high:RangeError:ERR_OUT_OF_RANGE,exitCode-unsafe-low-string:RangeError:ERR_OUT_OF_RANGE,exitCode-object:TypeError:ERR_INVALID_ARG_TYPE,exitCode-valueOf-object:TypeError:ERR_INVALID_ARG_TYPE,exitCode-boxed-number:TypeError:ERR_INVALID_ARG_TYPE,exitCode-object-number:TypeError:ERR_INVALID_ARG_TYPE,exitCode-boxed-string:TypeError:ERR_INVALID_ARG_TYPE,null:undefined,numeric-string:1,whitespace-string:0,hex-string:16,binary-string:2,octal-string:8,safe-high:-1,safe-low:1,wrap-zero:0,wrap-negative-one:-1"
	if got := value.String(); got != want {
		t.Fatalf("process code validation = %q, want %q", got, want)
	}
}

func TestProcessExitWrapsLargeIntegerCode(t *testing.T) {
	loop := goeventloop.New()
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	_, err = runtime.RunString(`
		globalThis.events = [];
		process.on("exit", function(code) { events.push("exit:" + code + ":" + process.exitCode); });
		process.exit(4294967295);
		events.push("after");
	`)
	if code, ok := processExitCode(err); !ok || code != -1 {
		t.Fatalf("RunString error = %v, want process exit signal code -1", err)
	}
	runtime.ClearInterrupt()

	value, err := runtime.RunString(`events.join(",")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if got, want := value.String(), "exit:-1:-1"; got != want {
		t.Fatalf("process.exit events = %q, want %q", got, want)
	}
}

func TestProcessExitAcceptsNodeNumericStrings(t *testing.T) {
	loop := goeventloop.New()
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	_, err = runtime.RunString(`
		globalThis.events = [];
		process.on("exit", function(code) { events.push("exit:" + code + ":" + process.exitCode); });
		process.exit("0x10");
	`)
	if code, ok := processExitCode(err); !ok || code != 16 {
		t.Fatalf("RunString error = %v, want process exit signal code 16", err)
	}
	runtime.ClearInterrupt()

	value, err := runtime.RunString(`events.join(",")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if got, want := value.String(), "exit:16:16"; got != want {
		t.Fatalf("process.exit events = %q, want %q", got, want)
	}
}

func TestProcessExitCodeErrorsIgnoreMutableGlobals(t *testing.T) {
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
		TypeError = function(message) { events.push("type-ctor"); return { name: "FakeType", message }; };
		RangeError = function(message) { events.push("range-ctor"); return { name: "FakeRange", message }; };
		function record(label, fn) {
			try { fn(); events.push(label + ":ok"); }
			catch (err) { events.push(label + ":" + err.name + ":" + err.code + ":" + (err instanceof TypeError)); }
		}
		record("exit-invalid", function() { process.exit("abc"); });
		record("exitCode-fraction", function() { process.exitCode = 1.5; });
		events.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "exit-invalid:TypeError:ERR_INVALID_ARG_TYPE:false,exitCode-fraction:RangeError:ERR_OUT_OF_RANGE:false"
	if got := value.String(); got != want {
		t.Fatalf("process code mutable globals = %q, want %q", got, want)
	}
}

func TestProcessPublicExitingOnlyGatesNextTick(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process._exiting = true;
		if (process._exiting !== true) throw new Error("public exiting getter");
		process.nextTick(function() { events.push("blocked-nextTick"); });
		queueMicrotask(function() { events.push("microtask"); });
		Promise.resolve().then(function() { events.push("promise"); });
		setImmediate(function() { events.push("immediate"); });
		process._exiting = 0;
		if (process._exiting !== false) throw new Error("public exiting Boolean coercion");
		process.nextTick(function() { events.push("nextTick"); });
	`)
	if want := "nextTick,microtask,promise,immediate"; got != want {
		t.Fatalf("public process._exiting gate = %q, want %q", got, want)
	}
}

func runAutoExitProcessScript(t *testing.T, script string) string {
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

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not return after process exit")
	}

	value, err := runtime.RunString(`events.join(",")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	return value.String()
}
