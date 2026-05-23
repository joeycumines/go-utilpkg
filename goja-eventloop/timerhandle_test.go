package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestNodeTimerAndImmediatePrototypeDescriptorsAndConstructors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	runtime := goja.New()
	installConformingHostSingletons(t, runtime)
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
		const failures = [];
		function check(label, ok) { if (!ok) failures.push(label); }
		function dataDesc(label, proto, key, enumerable) {
			const desc = Object.getOwnPropertyDescriptor(proto, key);
			check(label + ":present", !!desc);
			if (desc) {
				check(label + ":writable", desc.writable === true);
				check(label + ":enumerable", desc.enumerable === enumerable);
				check(label + ":configurable", desc.configurable === true);
			}
		}
		function functionShape(label, fn, name, length) {
			check(label + ":callable", typeof fn === "function");
			if (typeof fn !== "function") return;
			check(label + ":name", fn.name === name);
			check(label + ":length", fn.length === length);
			for (const key of ["name", "length"]) {
				const desc = Object.getOwnPropertyDescriptor(fn, key);
				check(label + ":" + key + ":present", !!desc);
				if (desc) {
					check(label + ":" + key + ":writable", desc.writable === false);
					check(label + ":" + key + ":enumerable", desc.enumerable === false);
					check(label + ":" + key + ":configurable", desc.configurable === true);
				}
			}
		}
		const timeout = setTimeout(function(){}, 1000);
		const immediate = setImmediate(function(){});
		const timeoutProto = Object.getPrototypeOf(timeout);
		const immediateProto = Object.getPrototypeOf(immediate);
		check("timeout keys", Object.keys(timeoutProto).join(",") === "close");
		check("immediate keys", Object.keys(immediateProto).join(",") === "");
		dataDesc("timeout constructor", timeoutProto, "constructor", false);
		dataDesc("timeout ref", timeoutProto, "ref", false);
		dataDesc("timeout unref", timeoutProto, "unref", false);
		dataDesc("timeout hasRef", timeoutProto, "hasRef", false);
		dataDesc("timeout refresh", timeoutProto, "refresh", false);
		dataDesc("timeout close", timeoutProto, "close", true);
		dataDesc("timeout toPrimitive", timeoutProto, Symbol.toPrimitive, true);
		dataDesc("timeout dispose", timeoutProto, Symbol.dispose, true);
		dataDesc("immediate constructor", immediateProto, "constructor", false);
		dataDesc("immediate ref", immediateProto, "ref", false);
		dataDesc("immediate unref", immediateProto, "unref", false);
		dataDesc("immediate hasRef", immediateProto, "hasRef", false);
		dataDesc("immediate dispose", immediateProto, Symbol.dispose, true);
		const inspect = Symbol.for("nodejs.util.inspect.custom");
		check("timeout inspect absent", Object.getOwnPropertyDescriptor(timeoutProto, inspect) === undefined);
		functionShape("Timeout", timeout.constructor, "Timeout", 5);
		functionShape("Timeout ref", timeout.ref, "ref", 0);
		functionShape("Timeout unref", timeout.unref, "unref", 0);
		functionShape("Timeout hasRef", timeout.hasRef, "hasRef", 0);
		functionShape("Timeout refresh", timeout.refresh, "refresh", 0);
		functionShape("Timeout close", timeout.close, "", 0);
		functionShape("Timeout toPrimitive", timeoutProto[Symbol.toPrimitive], "", 0);
		functionShape("Timeout dispose", timeoutProto[Symbol.dispose], "", 0);
		functionShape("Immediate", immediate.constructor, "Immediate", 2);
		functionShape("Immediate ref", immediate.ref, "ref", 0);
		functionShape("Immediate unref", immediate.unref, "unref", 0);
		functionShape("Immediate hasRef", immediate.hasRef, "hasRef", 0);
		functionShape("Immediate dispose", immediateProto[Symbol.dispose], "", 0);
		let constructedTimeoutRan = false;
		let constructedTimeoutRefedRan = false;
		let constructedTimeoutUnrefedRan = false;
		let remainingImmediates = 3;
		function finishImmediate() {
			remainingImmediates -= 1;
			if (remainingImmediates !== 0) return;
			check("constructed timeout did not run", constructedTimeoutRan === false);
			check("constructed refed timeout did not run", constructedTimeoutRefedRan === false);
			check("constructed unrefed timeout did not run", constructedTimeoutUnrefedRan === false);
			clearTimeout(constructedTimeout);
			clearTimeout(constructedTimeoutRefed);
			clearTimeout(constructedTimeoutUnrefed);
			testDone(failures.join(","));
		}
		const constructedTimeout = new timeout.constructor(function() { constructedTimeoutRan = true; }, 1);
		const constructedTimeoutRefed = new timeout.constructor(function() { constructedTimeoutRefedRan = true; }, 1, [], false, true);
		const constructedTimeoutUnrefed = new timeout.constructor(function() { constructedTimeoutUnrefedRan = true; }, 1, [], false, false);
		const constructedImmediate = new immediate.constructor(function(arg) {
			check("constructed immediate this", this === constructedImmediate);
			check("constructed immediate arg", arg === "constructed-immediate");
			finishImmediate();
		}, ["constructed-immediate"]);
		const constructedImmediateString = new immediate.constructor(function(a, b) {
			check("constructed immediate string args", a === "x" && b === "y" && arguments.length === 2);
			finishImmediate();
		}, "xy");
		const constructedImmediateArray = new immediate.constructor(function(a, b) {
			check("constructed immediate array args", a === "x" && b === "y" && arguments.length === 2);
			finishImmediate();
		}, ["x", "y"]);
		check("constructed timeout proto", Object.getPrototypeOf(constructedTimeout) === timeoutProto);
		check("constructed timeout hasRef missing ref", constructedTimeout.hasRef() === undefined);
		check("constructed timeout refed", constructedTimeoutRefed.hasRef() === true);
		check("constructed timeout unrefed", constructedTimeoutUnrefed.hasRef() === false);
		check("constructed immediate proto", Object.getPrototypeOf(constructedImmediate) === immediateProto);
		try { timeout.constructor(function(){}, 1); failures.push("timeout constructor callable"); }
		catch (err) { check("timeout constructor call TypeError", err.name === "TypeError"); }
		try { immediate.constructor(function(){}); failures.push("immediate constructor callable"); }
		catch (err) { check("immediate constructor call TypeError", err.name === "TypeError"); }
		clearTimeout(timeout);
		clearImmediate(immediate);
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case failures := <-done:
		if failures != "" {
			t.Fatalf("handle prototype descriptor failures: %s", failures)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for handle descriptor assertions")
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

func TestNodeTimeoutConstructorRefreshActivatesHandle(t *testing.T) {
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
		const failures = [];
		const events = [];
		function check(label, ok) { if (!ok) failures.push(label); }
		let remaining = 4;
		function record(event) {
			events.push(event);
			remaining -= 1;
			if (remaining !== 0) return;
			clearTimeout(base);
			clearTimeout(short);
			clearTimeout(refed);
			clearTimeout(unrefed);
			clearTimeout(arrayLike);
			testDone(events.sort().join(",") + "|" + failures.join(","));
		}
		const base = setTimeout(function(){}, 1000);
		const Timeout = base.constructor;
		const short = new Timeout(function() { record("short:" + String(short.hasRef())); }, 1);
		check("short before hasRef", short.hasRef() === undefined);
		short.refresh();
		check("short after hasRef", short.hasRef() === false);
		const refed = new Timeout(function(a, b) { record("refed:" + String(refed.hasRef()) + ":" + a + b); }, 1, ["a", "b"], false, true);
		check("refed before hasRef", refed.hasRef() === true);
		refed.refresh();
		check("refed after hasRef", refed.hasRef() === true);
		const unrefed = new Timeout(function() { record("unrefed:" + String(unrefed.hasRef())); }, 1, [], false, false);
		check("unrefed before hasRef", unrefed.hasRef() === false);
		unrefed.refresh();
		check("unrefed after hasRef", unrefed.hasRef() === false);
		const arrayLike = new Timeout(function(arg) { record("arrayLike:" + arg); }, 1, {0: "x", length: 1}, false, true);
		arrayLike.refresh();
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	select {
	case got := <-done:
		want := "arrayLike:x,refed:true:ab,short:false,unrefed:false|"
		if got != want {
			t.Fatalf("constructed Timeout refresh result = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for constructed Timeout refresh")
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

func TestNodeTimerAndImmediateHandleObjects(t *testing.T) {
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

	done := make(chan bool, 1)
	if err := runtime.Set("testDone", func(ok bool) { done <- ok }); err != nil {
		t.Fatalf("set testDone: %v", err)
	}

	_, err = runtime.RunString(`
		const checks = [];
		let remainingCallbacks = 3;
		function callbackDone() {
			remainingCallbacks -= 1;
			if (remainingCallbacks === 0) {
				testDone(checks.length === 27 && checks.every(Boolean));
			}
		}
		const timeout = setTimeout(function(a) {
			checks.push(this === timeout && a === "timeout");
			callbackDone();
		}, 1, "timeout");
		const interval = setInterval(function(a) {
			checks.push(this === interval && a === "interval");
			interval[Symbol.dispose]();
			callbackDone();
		}, 1, "interval");
		const immediate = setImmediate(function(a) {
			checks.push(this === immediate && a === "immediate");
			callbackDone();
		}, "immediate");
		const timeout2 = setTimeout(function(){}, 1000);
		const immediate2 = setImmediate(function(){});

		checks.push(typeof timeout.ref === "function");
		checks.push(typeof timeout.unref === "function");
		checks.push(typeof timeout.hasRef === "function");
		checks.push(typeof timeout.refresh === "function");
		checks.push(typeof timeout.close === "function");
		checks.push(typeof timeout[Symbol.toPrimitive] === "function");
		checks.push(typeof timeout[Symbol.dispose] === "function");
		checks.push(timeout.unref() === timeout && timeout.hasRef() === false);
		checks.push(timeout.ref() === timeout && timeout.hasRef() === true);
		checks.push(Number(timeout) > 0 && String(Number(timeout)) === String(+timeout));
		checks.push(timeout.ref === timeout2.ref);
		checks.push(!Object.prototype.hasOwnProperty.call(timeout, "ref"));
		checks.push(Object.getPrototypeOf(timeout).ref === timeout.ref);
		checks.push(timeout.constructor && timeout.constructor.name === "Timeout");
		checks.push(typeof immediate.ref === "function");
		checks.push(typeof immediate.unref === "function");
		checks.push(typeof immediate.hasRef === "function");
		checks.push(typeof immediate[Symbol.dispose] === "function");
		checks.push(immediate.unref() === immediate && immediate.hasRef() === false);
		checks.push(immediate.ref() === immediate && immediate.hasRef() === true);
		checks.push(immediate.ref === immediate2.ref);
		checks.push(!Object.prototype.hasOwnProperty.call(immediate, "ref"));
		checks.push(Object.getPrototypeOf(immediate).ref === immediate.ref);
		checks.push(immediate.constructor && immediate.constructor.name === "Immediate");
		clearTimeout(timeout2);
		clearImmediate(immediate2);
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("Node timer/immediate handles did not expose required methods, receivers, or primitive behavior")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for handle object assertions")
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

func TestNodeTimerAndImmediateHandleMethodsUseReceiver(t *testing.T) {
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

	value, err := runtime.RunString(`
		const events = [];
		const t1 = setTimeout(function(){}, 1000);
		const t2 = setTimeout(function(){}, 1000);
		t1.unref.call(t2);
		events.push("timer-borrow:" + t1.hasRef() + ":" + t2.hasRef());
		try { const unref = t1.unref; unref(); events.push("timer-detached:ok"); }
		catch (err) { events.push("timer-detached:" + err.name); }
		const i1 = setImmediate(function(){});
		const i2 = setImmediate(function(){});
		i1.unref.call(i2);
		events.push("immediate-borrow:" + i1.hasRef() + ":" + i2.hasRef());
		try { const ref = i1.ref; ref(); events.push("immediate-detached:ok"); }
		catch (err) { events.push("immediate-detached:" + err.name); }
		clearTimeout(t1);
		clearTimeout(t2);
		clearImmediate(i1);
		clearImmediate(i2);
		events.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "timer-borrow:true:false,timer-detached:TypeError,immediate-borrow:true:false,immediate-detached:TypeError"
	if got := value.String(); got != want {
		t.Fatalf("handle method receiver behavior = %q, want %q", got, want)
	}
}

func TestClearTimerAndImmediateHandlesIgnoreUserShadowIDProperties(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New(goeventloop.WithAutoExit(true))
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
	_, err = runtime.RunString(`
		globalThis.timerRan = false;
		const timeout = setTimeout(function() { timerRan = true; }, 1);
		timeout._gojaEventloopTimerID = 999;
		delete timeout._gojaEventloopTimerID;
		clearTimeout(timeout);

		globalThis.immediateRan = false;
		const immediate = setImmediate(function() { immediateRan = true; });
		immediate._gojaEventloopImmediateID = 999;
		delete immediate._gojaEventloopImmediateID;
		clearImmediate(immediate);
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	value, err := runtime.RunString(`"timer-ran:" + timerRan + ",immediate-ran:" + immediateRan`)
	if err != nil {
		t.Fatalf("result script: %v", err)
	}
	want := "timer-ran:false,immediate-ran:false"
	if got := value.String(); got != want {
		t.Fatalf("clear handle after public ID mutation = %q, want %q", got, want)
	}
	adapter.timersMu.Lock()
	timerCount := len(adapter.timers)
	adapter.timersMu.Unlock()
	adapter.immediatesMu.Lock()
	immediateCount := len(adapter.immediates)
	adapter.immediatesMu.Unlock()
	if timerCount != 0 || immediateCount != 0 {
		t.Fatalf("adapter handles after clear = (%d timers, %d immediates), want zero", timerCount, immediateCount)
	}
}

func TestNodeTimerAndImmediateTransparentProxyHandles(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New(goeventloop.WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if _, err := runtime.RunString(`
		globalThis.proxyEvents = [];
		const timeout = setTimeout(function() { proxyEvents.push("timeout-ran"); }, 1);
		const timeoutProxy = new Proxy(new Proxy(timeout, {}), {});
		proxyEvents.push("timeout-unref:" + (timeoutProxy.unref() === timeoutProxy) + ":" + timeoutProxy.hasRef());
		proxyEvents.push("timeout-ref:" + (timeoutProxy.ref() === timeoutProxy) + ":" + timeoutProxy.hasRef());
		proxyEvents.push("timeout-refresh:" + (timeoutProxy.refresh() === timeoutProxy));
		clearTimeout(timeoutProxy);

		const immediate = setImmediate(function() { proxyEvents.push("immediate-ran"); });
		const immediateProxy = new Proxy(new Proxy(immediate, {}), {});
		proxyEvents.push("immediate-unref:" + (immediateProxy.unref() === immediateProxy) + ":" + immediateProxy.hasRef());
		proxyEvents.push("immediate-ref:" + (immediateProxy.ref() === immediateProxy) + ":" + immediateProxy.hasRef());
		clearImmediate(immediateProxy);
	`); err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := runtime.Get("proxyEvents").String(), "timeout-unref:true:false,timeout-ref:true:true,timeout-refresh:true,immediate-unref:true:false,immediate-ref:true:true"; got != want {
		t.Fatalf("transparent proxy handle behavior = %q, want %q", got, want)
	}
}

func TestNodeImmediateDestroyedRefState(t *testing.T) {
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
		const cleared = setImmediate(function() { events.push("cleared-ran"); });
		clearImmediate(cleared);
		events.push("clear:" + cleared.hasRef() + ":" + (cleared.ref() === cleared) + ":" + cleared.hasRef() + ":" + (cleared.unref() === cleared) + ":" + cleared.hasRef());

		const disposed = setImmediate(function() { events.push("disposed-ran"); });
		disposed[Symbol.dispose]();
		events.push("dispose:" + disposed.hasRef() + ":" + (disposed.ref() === disposed) + ":" + disposed.hasRef());

		const executed = setImmediate(function() {
			events.push("inside:" + executed.hasRef());
			executed.ref();
			events.push("inside-ref:" + executed.hasRef());
		});
		setImmediate(function() {
			events.push("after:" + executed.hasRef() + ":" + (executed.ref() === executed) + ":" + executed.hasRef());
			testDone(events.join("|"));
		});
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	select {
	case got := <-done:
		want := "clear:false:true:false:true:false|dispose:false:true:false|inside:false|inside-ref:false|after:false:true:false"
		if got != want {
			t.Fatalf("Immediate destroyed ref state = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for Immediate destroyed ref-state assertion")
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

func TestNodeImmediateRejectsMalformedArgumentIterators(t *testing.T) {
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
		process.on("uncaughtException", function (error, origin) {
			events.push(error.message + ":" + origin);
		});
		const seed = setImmediate(function () {});
		const Immediate = seed.constructor;
		clearImmediate(seed);
		new Immediate(function () { events.push("primitive-iterator-callback"); }, {
			[Symbol.iterator]: function () { return 1; },
		});
		new Immediate(function () { events.push("primitive-step-callback"); }, {
			[Symbol.iterator]: function () {
				return { next: function () { return 1; } };
			},
		});
		setImmediate(function () { testDone(events.join("|")); });
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case got := <-done:
		const want = "Result of the Symbol.iterator method is not an object:uncaughtException|" +
			"Iterator result 1 is not an object:uncaughtException"
		if got != want {
			t.Fatalf("malformed Immediate iterator events = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for malformed Immediate iterator assertions")
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
