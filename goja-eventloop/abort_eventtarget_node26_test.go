package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestNode26AbortSignalPrototypeBranding(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		const controller = new AbortController();
		const controllerSignal = controller.signal;
		const abortedSignal = AbortSignal.abort();
		const target = new EventTarget();
		const event = new Event("proto", { cancelable: true });
		const customEvent = new CustomEvent("custom", { detail: { ok: true } });
		let targetCalls = 0;
		EventTarget.prototype.addEventListener.call(target, "x", function () { targetCalls++; });
		EventTarget.prototype.dispatchEvent.call(target, new Event("x"));
		let signalCalls = 0;
		EventTarget.prototype.addEventListener.call(controllerSignal, "abort", function () { signalCalls++; });
		controller.abort();
		[
			controllerSignal instanceof AbortSignal,
			controllerSignal instanceof EventTarget,
			abortedSignal instanceof AbortSignal,
			abortedSignal instanceof EventTarget,
			Object.getPrototypeOf(AbortSignal.prototype) === EventTarget.prototype,
			typeof AbortController.prototype.abort === "function",
			typeof Object.getOwnPropertyDescriptor(AbortController.prototype, "signal").get === "function",
			Object.prototype.hasOwnProperty.call(controller, "abort") === false,
			Object.prototype.hasOwnProperty.call(controller, "signal") === false,
			typeof AbortSignal.prototype.throwIfAborted === "function",
			typeof Object.getOwnPropertyDescriptor(AbortSignal.prototype, "aborted").get === "function",
			typeof Object.getOwnPropertyDescriptor(AbortSignal.prototype, "reason").get === "function",
			typeof Object.getOwnPropertyDescriptor(AbortSignal.prototype, "onabort").get === "function",
			Object.prototype.hasOwnProperty.call(controllerSignal, "throwIfAborted") === false,
			typeof EventTarget.prototype.addEventListener === "function",
			typeof EventTarget.prototype.removeEventListener === "function",
			typeof EventTarget.prototype.dispatchEvent === "function",
			Object.prototype.hasOwnProperty.call(target, "addEventListener") === false,
			Object.prototype.hasOwnProperty.call(controllerSignal, "addEventListener") === false,
			typeof Event.prototype.preventDefault === "function",
			typeof Object.getOwnPropertyDescriptor(Event.prototype, "type").get === "function",
			Object.prototype.hasOwnProperty.call(event, "preventDefault") === false,
			Object.prototype.hasOwnProperty.call(event, "type") === false,
			Event.prototype.preventDefault.call(event) === undefined && event.defaultPrevented === true,
			customEvent instanceof Event,
			customEvent instanceof CustomEvent,
			typeof Object.getOwnPropertyDescriptor(CustomEvent.prototype, "detail").get === "function",
			Object.prototype.hasOwnProperty.call(customEvent, "detail") === false,
			customEvent.detail.ok === true,
			targetCalls === 1,
			signalCalls === 1,
		].join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true"; got != want {
		t.Fatalf("AbortSignal prototype branding = %q, want %q", got, want)
	}
}

func TestNode26PublicObjectsHideLegacyState(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const controller = new AbortController();
			const signal = controller.signal;
			const target = new EventTarget();
			const event = new Event("legacy");
			const customEvent = new CustomEvent("legacy-custom", { detail: 1 });
			const timeout = setTimeout(function () {}, 1);
			const immediate = setImmediate(function () {});
			clearTimeout(timeout);
			clearImmediate(immediate);

			const checks = [
				!("_controller" in controller),
				!("_signal" in signal),
				!("_wrapper" in target),
				!("_event" in event),
				!("_event" in customEvent),
				!("_gojaEventloopTimerID" in timeout),
				!("_gojaEventloopImmediateID" in immediate),
				Object.keys(controller).length === 0,
				Object.keys(signal).length === 0,
				Object.keys(target).length === 0,
				Object.keys(event).join(",") === "isTrusted",
				Object.keys(customEvent).join(",") === "isTrusted",
				Object.getOwnPropertySymbols(controller).length === 0,
				Object.getOwnPropertySymbols(signal).length === 0,
				Object.getOwnPropertySymbols(target).length === 0,
				Object.getOwnPropertySymbols(event).length === 0,
				Object.getOwnPropertySymbols(customEvent).length === 0,
				Object.getOwnPropertySymbols(timeout).every((symbol) => !String(symbol.description).includes("gojaEventloop")),
				Object.getOwnPropertySymbols(immediate).every((symbol) => !String(symbol.description).includes("gojaEventloop")),
			];
			return checks.every(Boolean) ? "ok" : checks.map((ok, index) => ok ? null : index).filter((index) => index !== null).join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "ok"; got != want {
		t.Fatalf("legacy implementation state is public: failed checks %s", got)
	}
}

func TestTrackAbortSignalCleanupSkipsGojaRuntime(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	signal, err := adapter.runtime.RunString(`
		globalThis.__trackController = new AbortController();
		__trackController.signal;
	`)
	if err != nil {
		t.Fatalf("RunString signal: %v", err)
	}
	callbackCount := 0
	cleanup, aborted, ok := adapter.TrackAbortSignal(signal, func() { callbackCount++ })
	if !ok || aborted || cleanup == nil {
		t.Fatalf("TrackAbortSignal returned cleanup=%v aborted=%v ok=%v", cleanup != nil, aborted, ok)
	}

	cleanupDone := make(chan struct{})
	go func() {
		cleanup()
		close(cleanupDone)
	}()
	select {
	case <-cleanupDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for off-loop cleanup")
	}
	if _, err := adapter.runtime.RunString(`__trackController.abort("late")`); err != nil {
		t.Fatalf("RunString abort after cleanup: %v", err)
	}
	if callbackCount != 0 {
		t.Fatalf("abort callback ran after cleanup: %d", callbackCount)
	}

	signal, err = adapter.runtime.RunString(`
		globalThis.__trackController2 = new AbortController();
		__trackController2.signal;
	`)
	if err != nil {
		t.Fatalf("RunString second signal: %v", err)
	}
	cleanup, aborted, ok = adapter.TrackAbortSignal(signal, func() { callbackCount++ })
	if !ok || aborted || cleanup == nil {
		t.Fatalf("TrackAbortSignal second returned cleanup=%v aborted=%v ok=%v", cleanup != nil, aborted, ok)
	}
	if _, err := adapter.runtime.RunString(`__trackController2.abort("now")`); err != nil {
		t.Fatalf("RunString abort callback: %v", err)
	}
	cleanup()
	if callbackCount != 1 {
		t.Fatalf("abort callback count = %d, want 1", callbackCount)
	}
}

func TestTrackAbortSignalRejectsCopiedAdapter(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	signal, err := adapter.runtime.RunString(`globalThis.__copyController = new AbortController(); __copyController.signal`)
	if err != nil {
		t.Fatalf("RunString signal: %v", err)
	}
	copy := copyAdapterValue(adapter)
	calls := 0
	func() {
		defer assertAdapterPanic(t, "copied Adapter TrackAbortSignal")
		copy.TrackAbortSignal(signal, func() { calls++ })
	}()
	if _, err := adapter.runtime.RunString(`__copyController.abort()`); err != nil {
		t.Fatalf("abort after copied TrackAbortSignal: %v", err)
	}
	if calls != 0 {
		t.Fatalf("copied TrackAbortSignal callback calls = %d, want 0", calls)
	}
}

func TestNode26AbortAlgorithmsIgnoreStopImmediatePropagation(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const controller = new AbortController();
			const target = new EventTarget();
			let calls = 0;
			controller.signal.addEventListener("abort", event => event.stopImmediatePropagation());
			target.addEventListener("x", () => { calls++; }, { signal: controller.signal });
			controller.abort("stop");
			target.dispatchEvent(new Event("x"));

			const source = new AbortController();
			source.signal.addEventListener("abort", event => event.stopImmediatePropagation());
			const composite = AbortSignal.any([source.signal]);
			source.abort("reason");
			return calls + ":" + composite.aborted + ":" + composite.reason;
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "0:true:reason"; got != want {
		t.Fatalf("abort algorithms under stopImmediatePropagation = %q, want %q", got, want)
	}
}

func TestWebEventTargetIgnoresAsyncListenerReturn(t *testing.T) {
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
		process.on("unhandledRejection", reason => events.push("unhandled:" + reason.message));
		process.on("uncaughtException", (err, origin) => events.push("uncaught:" + err.message + ":" + origin));
		const target = new EventTarget();
		target.addEventListener("x", async () => { throw new Error("async listener boom"); });
		target.dispatchEvent(new Event("x"));
		setImmediate(() => testDone(events.join(",")));
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	select {
	case got := <-done:
		if want := "unhandled:async listener boom"; got != want {
			t.Fatalf("async EventTarget rejection events = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for async listener rejection")
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

func TestWebEventTargetDoesNotReadListenerReturnThen(t *testing.T) {
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
		process.on("uncaughtException", (err, origin) => events.push("uncaught:" + err.message + ":" + origin));
		const target = new EventTarget();
		const poisoned = Object.defineProperty({}, "then", {
			get() {
				events.push("getter");
				throw new Error("bad then");
			},
		});
		target.addEventListener("x", function () {
			return poisoned;
		});
		target.addEventListener("x", function () { events.push("second"); });
		target.addEventListener("x", {
			handleEvent() {
				events.push("object");
				return poisoned;
			},
		});
		events.push("before");
		try {
			events.push("after:" + target.dispatchEvent(new Event("x")));
		} catch (err) {
			events.push("sync:" + err.message);
		}
		setImmediate(() => testDone(events.join(",")));
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	select {
	case got := <-done:
		want := "before,second,object,after:true"
		if got != want {
			t.Fatalf("EventTarget throwing then getter events = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for EventTarget throwing then getter")
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

func TestWebEventTargetHandledReturnedPromiseUsesNativeTracker(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() { events.push("uncaught"); });
		process.on("unhandledRejection", function() { events.push("unhandled"); });
		const target = new EventTarget();
		target.addEventListener("x", function() {
			return Promise.reject(new Error("handled")).catch(function() {
				events.push("handled");
			});
		});
		target.dispatchEvent(new Event("x"));
		setImmediate(function() { events.push("checkpoint"); });
	`)
	if want := "handled,checkpoint"; got != want {
		t.Fatalf("handled EventTarget return rejection = %q, want %q", got, want)
	}
}

func TestNode26GoDispatchedEventOnceAndWrapperIdentity(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	_, err := adapter.runtime.RunString(`
		globalThis.__goTarget = new EventTarget();
		globalThis.__goEvents = [];
		globalThis.__firstGoEvent = null;
		__goTarget.addEventListener("go", function(event) {
			__goEvents.push("a:" + event.type);
			__firstGoEvent = event;
		}, { once: true });
		__goTarget.addEventListener("go", function(event) {
			__goEvents.push("b:" + (event === __firstGoEvent));
		});
	`)
	if err != nil {
		t.Fatalf("RunString setup: %v", err)
	}
	target := adapter.eventTargetThis(adapter.runtime.Get("__goTarget"))
	target.target.DispatchEvent(goeventloop.NewEvent("go"))
	target.target.DispatchEvent(goeventloop.NewEvent("go"))
	value, err := adapter.runtime.RunString(`__goEvents.join(",")`)
	if err != nil {
		t.Fatalf("RunString result: %v", err)
	}
	if got, want := value.String(), "a:go,b:true,b:false"; got != want {
		t.Fatalf("Go-dispatched EventTarget events = %q, want %q", got, want)
	}
}

func TestWebIDLAbortSignalTimeoutEnforceRange(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const results = [];
			let coercions = 0;
			const accepted = [null, false, true, "", "5", 1.9, -0.5, 4294967296, 9007199254740991, {
				valueOf() { coercions++; return 7.8; },
			}];
			for (const value of accepted) {
				try {
					const signal = AbortSignal.timeout(value);
					results.push("ok:" + (signal instanceof AbortSignal) + ":" + signal.aborted);
				} catch (err) {
					results.push("unexpected:" + err.name);
				}
			}
			for (const value of [undefined, NaN, -1, Infinity, -Infinity, 9007199254740992, Symbol("delay"), 1n]) {
				try { AbortSignal.timeout(value); results.push("unexpected:ok"); }
				catch (err) { results.push("type:" + (err instanceof TypeError) + ":" + err.code); }
			}
			const abrupt = { marker: "abrupt" };
			try {
				AbortSignal.timeout({ valueOf() { throw abrupt; } });
				results.push("unexpected:coercion");
			} catch (err) {
				results.push("abrupt:" + (err === abrupt));
			}
			results.push("coercions:" + coercions);
			return results.join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "ok:true:false,ok:true:false,ok:true:false,ok:true:false,ok:true:false,ok:true:false,ok:true:false,ok:true:false,ok:true:false,ok:true:false," +
		"type:true:undefined,type:true:undefined,type:true:undefined,type:true:undefined,type:true:undefined,type:true:undefined,type:true:undefined,type:true:undefined,abrupt:true,coercions:1"
	if got := value.String(); got != want {
		t.Fatalf("AbortSignal.timeout Web IDL conversion = %q, want %q", got, want)
	}
}

func TestWebAbortSignalTimeoutDoesNotUseNodeTimerClamp(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		let warnings = 0;
		process.on("warning", function() { warnings++; });
		const signal = AbortSignal.timeout(4294967296);
		setTimeout(function() {
			events.push(String(signal.aborted), String(warnings));
			process.exit(0);
		}, 5);
	`)
	if want := "false,0"; got != want {
		t.Fatalf("large AbortSignal.timeout scheduling = %q, want %q", got, want)
	}
}

func TestWebEventTargetReentrantDispatchError(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const target = new EventTarget();
			const event = new Event("x");
			let observed = "none";
			target.addEventListener("x", function () {
				try { target.dispatchEvent(event); }
				catch (err) { observed = err.name + ":" + err.constructor.name + ":" + err.code + ":" + /event "x"/.test(err.message); }
			});
			target.dispatchEvent(event);
			return observed;
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "InvalidStateError:DOMException:11:false"; got != want {
		t.Fatalf("reentrant dispatch error = %q, want %q", got, want)
	}
}

func TestNode26AbortSignalOnAbortReplacementAndEventShape(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		const events = [];
		const controller = new AbortController();
		function first() { events.push("first"); }
		function second(event) {
			events.push("second:" + (this === controller.signal) + ":" + (event instanceof Event) + ":" + (event.type === "abort") + ":" + (event.target === controller.signal) + ":" + (event.currentTarget === controller.signal));
		}
		controller.signal.onabort = first;
		controller.signal.onabort = second;
		controller.abort("x");
		controller.signal.onabort === second && events.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString replacement: %v", err)
	}
	if got, want := value.String(), "second:true:true:true:true:true"; got != want {
		t.Fatalf("onabort replacement event = %q, want %q", got, want)
	}

	value, err = adapter.runtime.RunString(`
		(() => {
			const removedEvents = [];
			const removedController = new AbortController();
			removedController.signal.onabort = function() { removedEvents.push("removed"); };
			removedController.signal.onabort = null;
			removedController.abort();
			return String(removedController.signal.onabort === null) + ":" + removedEvents.join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString removal: %v", err)
	}
	if got, want := value.String(), "true:"; got != want {
		t.Fatalf("onabort removal = %q, want %q", got, want)
	}

	value, err = adapter.runtime.RunString(`
		(() => {
			const controller = new AbortController();
			const events = [];
			function f() { events.push("f"); }
			controller.signal.addEventListener("abort", f);
			controller.signal.onabort = f;
			controller.abort();
			return events.join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString ordinary/onabort separation: %v", err)
	}
	if got, want := value.String(), "f,f"; got != want {
		t.Fatalf("onabort ordinary listener separation = %q, want %q", got, want)
	}
}

func TestAbortSignalOnAbortKeepsEventHandlerIdentityAndOrder(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const order = [];
			const first = new AbortController();
			first.signal.onabort = () => order.push("a");
			first.signal.addEventListener("abort", () => order.push("b"));
			first.signal.onabort = () => order.push("c");
			first.abort();

			const calls = [];
			const second = new AbortController();
			function shared() { calls.push("shared"); }
			second.signal.onabort = shared;
			second.signal.addEventListener("abort", shared);
			second.signal.removeEventListener("abort", shared);
			const retained = second.signal.onabort === shared;
			second.abort();

			let duplicates = 0;
			const third = new AbortController();
			function duplicated() { duplicates++; }
			third.signal.onabort = duplicated;
			third.signal.addEventListener("abort", duplicated);
			third.abort();
			return order.join(",") + ":" + retained + ":" + calls.join(",") + ":" + duplicates;
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "c,b:true:shared:2"; got != want {
		t.Fatalf("onabort identity and order = %q, want %q", got, want)
	}
}

func TestAbortSignalUsesCapturedIntrinsicsAndTrustedInternalEvents(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const SavedAbortController = AbortController;
			const SavedAbortSignal = AbortSignal;
			const savedSignalPrototype = AbortSignal.prototype;
			const savedEventPrototype = Event.prototype;
			const source = new SavedAbortController();
			const customIterable = {
				[Symbol.iterator]() {
					let done = false;
					return { next() { if (done) return { done: true }; done = true; return { done: false, value: source.signal }; } };
				},
			};
			globalThis.AbortSignal = null;
			globalThis.Event = Object.defineProperty({}, "prototype", {
				get() { throw new Error("mutable Event global was consulted"); },
			});
			globalThis.Symbol = { iterator: "poisoned" };

			const staticallyAborted = SavedAbortSignal.abort("static");
			const dependent = SavedAbortSignal.any([staticallyAborted]);
			const arrayDependent = SavedAbortSignal.any([source.signal]);
			const customDependent = SavedAbortSignal.any(customIterable);
			const controller = new SavedAbortController();
			let observed;
			let captured;
			controller.signal.addEventListener("abort", event => {
				captured = event;
				const before = event.isTrusted;
				let recursion;
				try { controller.signal.dispatchEvent(event); }
				catch (error) { recursion = error.name; }
				observed = before + ":" + event.isTrusted + ":" + recursion + ":" + (Object.getPrototypeOf(event) === savedEventPrototype);
			});
			controller.abort("controller");
			captured.initEvent("reset");
			return [
				Object.getPrototypeOf(staticallyAborted) === savedSignalPrototype,
				Object.getPrototypeOf(dependent) === savedSignalPrototype,
				Object.getPrototypeOf(controller.signal) === savedSignalPrototype,
				dependent.aborted,
				dependent.reason,
				!arrayDependent.aborted,
				!customDependent.aborted,
				observed,
				captured.isTrusted,
			].join(":");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "true:true:true:true:static:true:true:true:true:InvalidStateError:true:false"; got != want {
		t.Fatalf("captured Abort/Event intrinsics = %q, want %q", got, want)
	}
}

func TestEventTrustedStateDistinguishesInternalAndScriptDispatch(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	internal := adapter.wrapEvent(goeventloop.NewEvent("internal"))
	if err := adapter.runtime.Set("__internalEvent", internal); err != nil {
		t.Fatalf("set internal event: %v", err)
	}
	value, err := adapter.runtime.RunString(`
		(() => {
			const own = Object.getOwnPropertyDescriptor(__internalEvent, "isTrusted");
			const before = __internalEvent.isTrusted;
			const constructed = new Event("constructed").isTrusted;
			let during;
			const target = new EventTarget();
			target.addEventListener("internal", event => { during = event.isTrusted; });
			target.dispatchEvent(__internalEvent);
			return [
				before,
				constructed,
				during,
				__internalEvent.isTrusted,
				own.get.name,
				own.get.length,
				own.set === undefined,
				own.enumerable,
				own.configurable,
			].join(":");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "true:false:false:false:get isTrusted:0:true:true:false"; got != want {
		t.Fatalf("Event isTrusted lifecycle = %q, want %q", got, want)
	}
}

func TestNode26AbortSignalAnyValidatesAllInputs(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			try {
				AbortSignal.any([AbortSignal.abort("x"), 1]);
				return "no-throw";
			} catch (err) {
				return err instanceof TypeError;
			}
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !value.ToBoolean() {
		t.Fatalf("AbortSignal.any did not validate invalid later input before returning aborted signal: %s", value.String())
	}
}

func TestAbortSignalAnyValidatesIncrementallyWithoutClosingIterator(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			let nextCount = 0;
			let closeCount = 0;
			const iterable = {
				[Symbol.iterator]() {
					return {
						next() {
							nextCount++;
							if (nextCount === 1) return { value: 1, done: false };
							throw new Error("iteration continued after invalid signal");
						},
						return() {
							closeCount++;
							return { done: true };
						},
					};
				},
			};
			let typeError = false;
			try { AbortSignal.any(iterable); }
			catch (error) { typeError = error instanceof TypeError; }
			return [typeError, nextCount, closeCount].join(":");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "true:1:0"; got != want {
		t.Fatalf("AbortSignal.any incremental validation = %q, want %q", got, want)
	}
}

func TestAbortSignalAnyIteratorAbruptOrderingAndThrowPrecedence(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			function observe(kind, closeKind) {
				const log = [];
				const primary = new Error("primary");
				const close = new Error("close");
				const iterator = {
					next() {
						log.push("next");
						if (kind === "next") throw primary;
						if (kind === "result") return 1;
						return {
							get done() {
								log.push("done-get");
								if (kind === "done") throw primary;
								return false;
							},
							get value() {
								log.push("value-get");
								if (kind === "value") throw primary;
								return 1;
							},
						};
					},
				};
				Object.defineProperty(iterator, "return", {
					get() {
						log.push("return-get");
						if (closeKind === "getter") throw close;
						return function () {
							log.push("return-call");
							if (closeKind === "call") throw close;
							if (closeKind === "primitive") return 1;
							return { done: true };
						};
					},
				});
				let winner = "none";
				try {
					AbortSignal.any({ [Symbol.iterator]() { return iterator; } });
				} catch (error) {
					winner = error === primary ? "primary" :
						error === close ? "close" :
						error instanceof TypeError ? "type" : "other";
				}
				return winner + ":" + log.join(",");
			}
			return [
				observe("invalid", "ok"),
				observe("invalid", "getter"),
				observe("invalid", "call"),
				observe("invalid", "primitive"),
				observe("done", "ok"),
				observe("value", "ok"),
				observe("result", "ok"),
				observe("next", "ok"),
			].join("|");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "type:next,done-get,value-get|" +
		"type:next,done-get,value-get|" +
		"type:next,done-get,value-get|" +
		"type:next,done-get,value-get|" +
		"primary:next,done-get|" +
		"primary:next,done-get,value-get|" +
		"type:next|" +
		"primary:next"
	if got := value.String(); got != want {
		t.Fatalf("AbortSignal.any iterator abrupt ordering = %q, want %q", got, want)
	}
}

func TestNode26EventTargetListenerIdentityDispatchStateAndObjects(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		const target = new EventTarget();
		const events = [];
		function listener(event) {
			events.push("fn:" + (this === target) + ":" + (event.target === target) + ":" + (event.currentTarget === target) + ":" + (event.eventPhase === Event.AT_TARGET));
		}
		target.addEventListener("x", listener);
		target.addEventListener("x", listener);
		target.addEventListener("x", listener, true);
		target.removeEventListener("x", listener, false);
		const objectListener = { handleEvent(event) { events.push("obj:" + (this === objectListener) + ":" + event.type); } };
		target.addEventListener("x", objectListener);
		const event = new Event("x", { cancelable: true });
		target.dispatchEvent(event);
		events.push("after:" + (event.target === target) + ":" + (event.currentTarget === null) + ":" + (event.eventPhase === Event.NONE));
		events.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "fn:true:true:true:true,obj:true:x,after:true:true:true"
	if got := value.String(); got != want {
		t.Fatalf("EventTarget listener semantics = %q, want %q", got, want)
	}
}

func TestNode26EventTimeStampUsesPerformanceOrigin(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const before = performance.now();
			const event = new Event("x");
			const after = performance.now();
			return event.timeStamp >= before - 1 && event.timeStamp <= after + 5 && event.timeStamp < Date.now() / 2;
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !value.ToBoolean() {
		t.Fatalf("Event.timeStamp is not performance-origin based: %s", value.String())
	}
}

func TestNode26EventTargetListenerSignalOption(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const target = new EventTarget();
			const controller = new AbortController();
			let count = 0;
			target.addEventListener("x", () => { count++; }, { signal: controller.signal });
			controller.abort();
			target.dispatchEvent(new Event("x"));

			const alreadyAborted = new EventTarget();
			const aborted = AbortSignal.abort();
			let skipped = 0;
			alreadyAborted.addEventListener("x", () => { skipped++; }, { signal: aborted });
			alreadyAborted.dispatchEvent(new Event("x"));
			return count + ":" + skipped;
		})()
	`)
	if err != nil {
		t.Fatalf("RunString signal option: %v", err)
	}
	if got, want := value.String(), "0:0"; got != want {
		t.Fatalf("EventTarget signal option = %q, want %q", got, want)
	}

	value, err = adapter.runtime.RunString(`
		(() => {
			try {
				new EventTarget().addEventListener("x", function() {}, { signal: 1 });
				return "no-throw";
			} catch (err) {
				return err instanceof TypeError;
			}
		})()
	`)
	if err != nil {
		t.Fatalf("RunString invalid signal option: %v", err)
	}
	if !value.ToBoolean() {
		t.Fatalf("EventTarget invalid signal option did not throw TypeError: %s", value.String())
	}
}
