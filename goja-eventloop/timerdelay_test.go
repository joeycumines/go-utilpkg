package gojaeventloop

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestCoerceNodeTimerDelay(t *testing.T) {
	runtime := goja.New()
	tests := []struct {
		name            string
		value           goja.Value
		wantDelay       int
		wantIdleTimeout float64
	}{
		{name: "undefined", value: goja.Undefined(), wantDelay: 1, wantIdleTimeout: 1},
		{name: "null", value: goja.Null(), wantDelay: 1, wantIdleTimeout: 1},
		{name: "nan", value: goja.NaN(), wantDelay: 1, wantIdleTimeout: 1},
		{name: "negative", value: runtime.ToValue(-5), wantDelay: 1, wantIdleTimeout: 1},
		{name: "negative infinity", value: runtime.ToValue(math.Inf(-1)), wantDelay: 1, wantIdleTimeout: 1},
		{name: "zero", value: runtime.ToValue(0), wantDelay: 1, wantIdleTimeout: 1},
		{name: "sub-millisecond", value: runtime.ToValue(0.9), wantDelay: 1, wantIdleTimeout: 1},
		{name: "one fractional visible", value: runtime.ToValue(1.9), wantDelay: 1, wantIdleTimeout: 1.9},
		{name: "string fractional visible", value: runtime.ToValue("5.9"), wantDelay: 5, wantIdleTimeout: 5.9},
		{name: "fraction visible", value: runtime.ToValue(7.9), wantDelay: 7, wantIdleTimeout: 7.9},
		{name: "max", value: runtime.ToValue(2147483647), wantDelay: 2147483647, wantIdleTimeout: 2147483647},
		{name: "fractional above max", value: runtime.ToValue(2147483647.1), wantDelay: 1, wantIdleTimeout: 1},
		{name: "above max", value: runtime.ToValue(int64(2147483648)), wantDelay: 1, wantIdleTimeout: 1},
		{name: "infinity", value: runtime.ToValue(math.Inf(1)), wantDelay: 1, wantIdleTimeout: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := coerceNodeTimerDelay(tt.value); got != tt.wantDelay {
				t.Fatalf("coerceNodeTimerDelay() = %d, want %d", got, tt.wantDelay)
			}
			if got := computeNodeTimerDelay(tt.value).idleTimeout; got != tt.wantIdleTimeout {
				t.Fatalf("idleTimeout = %v, want %v", got, tt.wantIdleTimeout)
			}
		})
	}
}

func TestNodeTimerIdleTimeoutPreservesNodeCoercion(t *testing.T) {
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
		const values = [undefined, null, NaN, -1, 0, 0.9, 1.9, 2147483648, Infinity, "2.9"];
		const events = [];
		for (const value of values) {
			const timeout = setTimeout(function() {}, value);
			const desc = Object.getOwnPropertyDescriptor(timeout, "_idleTimeout");
			events.push(String(value) + "=>" + timeout._idleTimeout + ":" + desc.writable + ":" + desc.enumerable + ":" + desc.configurable);
			clearTimeout(timeout);
		}
		events.join("|");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "undefined=>1:true:true:true|null=>1:true:true:true|NaN=>1:true:true:true|-1=>1:true:true:true|0=>1:true:true:true|0.9=>1:true:true:true|1.9=>1.9:true:true:true|2147483648=>1:true:true:true|Infinity=>1:true:true:true|2.9=>2.9:true:true:true"
	if got := value.String(); got != want {
		t.Fatalf("_idleTimeout coercion = %q, want %q", got, want)
	}
}

func TestNodeTimerDelayUsesArithmeticCoercion(t *testing.T) {
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
		const events = [];
		for (const delay of [1n, Object(1n)]) {
			try {
				setTimeout(function () {}, delay);
			} catch (error) {
				events.push(error.name + ":" + error.message);
			}
		}
		const observed = {
			[Symbol.toPrimitive]: function (hint) {
				events.push("hint:" + hint);
				return 2;
			},
		};
		const handle = setTimeout(function () {}, observed);
		events.push("delay:" + handle._idleTimeout);
		clearTimeout(handle);
		events.join("|");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	const want = "TypeError:Cannot mix BigInt and other types, use explicit conversions|" +
		"TypeError:Cannot mix BigInt and other types, use explicit conversions|hint:number|delay:2"
	if got := value.String(); got != want {
		t.Fatalf("timer delay coercion = %q, want %q", got, want)
	}
}

func TestNodeTimerWarningUsesECMAScriptNumberFormatting(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("warning", function(warning) {
			events.push(warning.name + ":" + warning.message);
		});
		clearTimeout(setTimeout(function() {}, 1e21));
		clearTimeout(setTimeout(function() {}, -1e-7));
		setImmediate(function() {});
	`)
	const want = "TimeoutOverflowWarning:1e+21 does not fit into a 32-bit signed integer.\n" +
		"Timeout duration was set to 1.," +
		"TimeoutNegativeWarning:-1e-7 is a negative number.\n" +
		"Timeout duration was set to 1."
	if got != want {
		t.Fatalf("timer warning number formatting = %q, want %q", got, want)
	}
}

func TestNodeTimerIdleTimeoutRefreshAndClearSemantics(t *testing.T) {
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
		const events = [];
		const active = setTimeout(function() { events.push("active-fired"); }, 1000);
		clearTimeout(active);
		events.push("active-clear:" + active._idleTimeout);
		const closed = setTimeout(function() { events.push("closed-fired"); }, 1000);
		closed.close();
		events.push("close:" + closed._idleTimeout);
		const disposed = setTimeout(function() { events.push("disposed-fired"); }, 1000);
		disposed[Symbol.dispose]();
		events.push("dispose:" + disposed._idleTimeout);
		const base = setTimeout(function() { events.push("base-fired"); }, 1000);
		const Timeout = base.constructor;
		clearTimeout(base);
		const constructedClear = new Timeout(function() { events.push("constructed-clear-fired"); }, 1000);
		clearTimeout(constructedClear);
		events.push("constructed-clear:" + constructedClear._idleTimeout);
		const constructedClose = new Timeout(function() { events.push("constructed-close-fired"); }, 1000);
		constructedClose.close();
		events.push("constructed-close:" + constructedClose._idleTimeout);
		const constructedDispose = new Timeout(function() { events.push("constructed-dispose-fired"); }, 1000);
		constructedDispose[Symbol.dispose]();
		events.push("constructed-dispose:" + constructedDispose._idleTimeout);

		let shortRan = false;
		const short = setTimeout(function() {
			shortRan = true;
			setImmediate(function() {
				const shortBeforeClear = short._idleTimeout;
				clearTimeout(short);
				events.push("short:" + shortRan + ":" + shortBeforeClear + ":" + short._idleTimeout);
				const longBeforeClear = long._idleTimeout;
				clearTimeout(long);
				events.push("long:" + longRan + ":" + longBeforeClear + ":" + long._idleTimeout);
				const undefinedBeforeClear = String(undefinedDelay._idleTimeout);
				clearTimeout(undefinedDelay);
				events.push("undefined:" + undefinedRan + ":" + undefinedBeforeClear + ":" + undefinedDelay._idleTimeout);
				const negativeBeforeClear = negative._idleTimeout;
				clearTimeout(negative);
				events.push("negative:" + negativeRan + ":" + negativeBeforeClear + ":" + negative._idleTimeout);
				const nanBeforeClear = String(nan._idleTimeout);
				clearTimeout(nan);
				events.push("nan:" + nanRan + ":" + nanBeforeClear + ":" + nan._idleTimeout);
				testDone(events.join("|"));
			});
		}, 1000);
		short._idleTimeout = 1;
		short.refresh();

		let longRan = false;
		const long = setTimeout(function() { longRan = true; }, 1);
		long._idleTimeout = 1000;
		long.refresh();

		let undefinedRan = false;
		const undefinedDelay = setTimeout(function() { undefinedRan = true; }, 1000);
		undefinedDelay._idleTimeout = undefined;
		undefinedDelay.refresh();

		let negativeRan = false;
		const negative = setTimeout(function() { negativeRan = true; }, 1000);
		negative._idleTimeout = -2;
		negative.refresh();

		let nanRan = false;
		const nan = setTimeout(function() { nanRan = true; }, 1000);
		nan._idleTimeout = NaN;
		nan.refresh();

	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	var got string
	select {
	case got = <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for _idleTimeout refresh/clear assertions")
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
	want := "active-clear:-1|close:-1|dispose:-1|constructed-clear:-1|constructed-close:-1|constructed-dispose:-1|short:true:1:1|long:false:1000:-1|undefined:false:undefined:-1|negative:false:-2:-1|nan:true:NaN:NaN"
	if got != want {
		t.Fatalf("_idleTimeout refresh/clear semantics = %q, want %q", got, want)
	}
}

func TestNodeTimerIdleTimeoutCallbackClearSemantics(t *testing.T) {
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
		const events = [];
		let remaining = 4;
		function finish() {
			remaining -= 1;
			if (remaining === 0) {
				testDone(events.sort().join("|"));
			}
		}
		const callbackClear = setTimeout(function() {
			clearTimeout(callbackClear);
			events.push("callback-clear:" + callbackClear._idleTimeout);
			finish();
		}, 1);
		const callbackClose = setTimeout(function() {
			callbackClose.close();
			events.push("callback-close:" + callbackClose._idleTimeout);
			finish();
		}, 1);
		const callbackDispose = setTimeout(function() {
			callbackDispose[Symbol.dispose]();
			events.push("callback-dispose:" + callbackDispose._idleTimeout);
			finish();
		}, 1);
		const afterCallback = setTimeout(function() {
			setImmediate(function() {
				const beforeClear = afterCallback._idleTimeout;
				clearTimeout(afterCallback);
				events.push("after-callback-clear:" + beforeClear + ":" + afterCallback._idleTimeout);
				finish();
			});
		}, 1);
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	var got string
	select {
	case got = <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for callback clear assertions")
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
	want := "after-callback-clear:1:1|callback-clear:-1|callback-close:-1|callback-dispose:-1"
	if got != want {
		t.Fatalf("callback _idleTimeout clear semantics = %q, want %q", got, want)
	}
}

func TestNodeTimerDelayWarnings(t *testing.T) {
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

	value, err := runtime.RunString(`
		const warnings = [];
		const events = [];
		process.on("warning", function(warning) {
			warnings.push(warning.name + "\n" + warning.message);
			events.push("warning:" + warning.name);
		});

		const overflow = setTimeout(function() {}, 2147483648);
		const infinity = setTimeout(function() {}, Infinity);
		const negative = setTimeout(function() {}, -5);
		const negativeAgain = setInterval(function() {}, -10);
		const nan = setTimeout(function() {}, "not-a-number");
		const nanAgain = setInterval(function() {}, NaN);
		const zero = setTimeout(function() {}, 0);
		clearTimeout(overflow);
		clearTimeout(infinity);
		clearTimeout(negative);
		clearInterval(negativeAgain);
		clearTimeout(nan);
		clearInterval(nanAgain);
		clearTimeout(zero);
		events.push("after-setTimeout");
		process.nextTick(function() {
			events.push("nextTick");
			testDone(events.join(",") + "\n===\n" + warnings.join("\n---\n"));
		});
		events.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "after-setTimeout"; got != want {
		t.Fatalf("warnings emitted synchronously: events = %q, want %q", got, want)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	var got string
	select {
	case got = <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for warning events")
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
	parts := strings.SplitN(got, "\n===\n", 2)
	if len(parts) != 2 {
		t.Fatalf("warning result missing separator: %q", got)
	}
	if gotEvents, want := parts[0], "after-setTimeout,warning:TimeoutOverflowWarning,warning:TimeoutOverflowWarning,warning:TimeoutNegativeWarning,warning:TimeoutNaNWarning,nextTick"; gotEvents != want {
		t.Fatalf("warning ordering = %q, want %q", gotEvents, want)
	}
	got = parts[1]
	wantSubstrings := []string{
		"TimeoutOverflowWarning\n2147483648 does not fit into a 32-bit signed integer.\nTimeout duration was set to 1.",
		"TimeoutOverflowWarning\nInfinity does not fit into a 32-bit signed integer.\nTimeout duration was set to 1.",
		"TimeoutNegativeWarning\n-5 is a negative number.\nTimeout duration was set to 1.",
		"TimeoutNaNWarning\nNaN is not a number.\nTimeout duration was set to 1.",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Fatalf("warning output missing %q; got:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"-10 is a negative number", "Warning\n0"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("warning output unexpectedly contained %q; got:\n%s", forbidden, got)
		}
	}
	if count := strings.Count(got, "TimeoutNaNWarning"); count != 1 {
		t.Fatalf("TimeoutNaNWarning count = %d, want 1; got:\n%s", count, got)
	}
	if count := strings.Count(got, "TimeoutNegativeWarning"); count != 1 {
		t.Fatalf("TimeoutNegativeWarning count = %d, want 1; got:\n%s", count, got)
	}
	if count := strings.Count(got, "TimeoutOverflowWarning"); count != 2 {
		t.Fatalf("TimeoutOverflowWarning count = %d, want 2; got:\n%s", count, got)
	}
}
