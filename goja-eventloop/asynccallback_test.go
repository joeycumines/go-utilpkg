package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestTimerImmediateAndNextTickArguments(t *testing.T) {
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
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	done := make(chan string, 1)
	if err := runtime.Set("testDone", func(result string) { done <- result }); err != nil {
		t.Fatalf("set testDone: %v", err)
	}

	_, err = runtime.RunString(`
		const results = [];
		function mark(name, ok) {
			results.push([name, ok]);
			if (results.length === 4) {
				testDone(results.map((entry) => entry[0] + ":" + entry[1]).sort().join(","));
			}
		}

		setTimeout(function(a, b, c) {
			mark("timeout", arguments.length === 3 && a === "timeout" && b === 42 && c === true);
		}, 0, "timeout", 42, true);

		const intervalID = setInterval(function(a, b) {
			clearInterval(intervalID);
			mark("interval", arguments.length === 2 && a === "interval" && b === 7);
		}, 0, "interval", 7);

		setImmediate(function(a, b) {
			mark("immediate", arguments.length === 2 && a === "immediate" && b === 13);
		}, "immediate", 13);

		process.nextTick(function(a, b) {
			mark("nextTick", arguments.length === 2 && a === "nextTick" && b === 99);
		}, "nextTick", 99);
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case got := <-done:
		want := "immediate:true,interval:true,nextTick:true,timeout:true"
		if got != want {
			t.Fatalf("scheduled callback argument results = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for scheduled callbacks")
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

func TestNodeAsyncCallbackValidationErrors(t *testing.T) {
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
		const values = [
			["undefined", undefined, 'The "callback" argument must be of type function. Received undefined'],
			["null", null, 'The "callback" argument must be of type function. Received null'],
			["number", 1, 'The "callback" argument must be of type function. Received type number (1)'],
			["string", "x", 'The "callback" argument must be of type function. Received type string (\'x\')'],
			["object", {}, 'The "callback" argument must be of type function. Received an instance of Object'],
			["array", [], 'The "callback" argument must be of type function. Received an instance of Array'],
			["date", new Date(0), 'The "callback" argument must be of type function. Received an instance of Date'],
			["regexp", /x/, 'The "callback" argument must be of type function. Received an instance of RegExp'],
			["map", new Map(), 'The "callback" argument must be of type function. Received an instance of Map'],
			["nullproto", Object.create(null), 'The "callback" argument must be of type function. Received [Object: null prototype] {}'],
			["bigint", BigInt(1), 'The "callback" argument must be of type function. Received type bigint (1n)'],
			["symbol", Symbol("x"), 'The "callback" argument must be of type function. Received type symbol (Symbol(x))'],
		];
		const failures = [];
		function check(api, call) {
			for (const [label, value, want] of values) {
				try { call(value); failures.push(api + ":" + label + ":accepted"); }
				catch (err) {
					if (err.name !== "TypeError" || err.code !== "ERR_INVALID_ARG_TYPE" || err.message !== want) {
						failures.push(api + ":" + label + ":" + err.name + ":" + err.code + ":" + err.message);
					}
				}
			}
		}
		check("setTimeout", function(value) { setTimeout(value, 1); });
		check("setInterval", function(value) { setInterval(value, 1); });
		check("setImmediate", function(value) { setImmediate(value); });
		check("queueMicrotask", function(value) { queueMicrotask(value); });
		check("nextTick", function(value) { process.nextTick(value); });
		failures.join("\n");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got := value.String(); got != "" {
		t.Fatalf("async callback validation errors were not Node-shaped:\n%s", got)
	}
}

func TestProcessNextTickValidationDuringExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New(goeventloop.WithAutoExit(true))
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

	_, err = runtime.RunString(`
		globalThis.nextTickExitResults = [];
		process.on("exit", function() {
			const values = [
				["undefined", undefined, 'The "callback" argument must be of type function. Received undefined'],
				["null", null, 'The "callback" argument must be of type function. Received null'],
				["number", 1, 'The "callback" argument must be of type function. Received type number (1)'],
				["string", "x", 'The "callback" argument must be of type function. Received type string (\'x\')'],
				["object", {}, 'The "callback" argument must be of type function. Received an instance of Object'],
			];
			for (const [label, value, want] of values) {
				try { process.nextTick(value); nextTickExitResults.push(label + ":accepted"); }
				catch (err) { nextTickExitResults.push(label + ":" + err.name + ":" + err.code + ":" + (err.message === want)); }
			}
		});
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	value, err := runtime.RunString(`nextTickExitResults.join("\n")`)
	if err != nil {
		t.Fatalf("result script: %v", err)
	}
	want := "undefined:TypeError:ERR_INVALID_ARG_TYPE:true\nnull:TypeError:ERR_INVALID_ARG_TYPE:true\nnumber:TypeError:ERR_INVALID_ARG_TYPE:true\nstring:TypeError:ERR_INVALID_ARG_TYPE:true\nobject:TypeError:ERR_INVALID_ARG_TYPE:true"
	if got := value.String(); got != want {
		t.Fatalf("exit-time process.nextTick validation = %q, want %q", got, want)
	}
}
