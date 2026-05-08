package gojaeventloop

import (
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func bindProcessTestSurface(t *testing.T, adapter *Adapter) {
	t.Helper()
	process := adapter.runtime.NewObject()
	if err := adapter.runtime.GlobalObject().DefineDataProperty(
		"process",
		process,
		goja.FLAG_TRUE,
		goja.FLAG_TRUE,
		goja.FLAG_FALSE,
	); err != nil {
		t.Fatalf("define process test global: %v", err)
	}
	if err := adapter.bindProcess(process); err != nil {
		t.Fatalf("bind process: %v", err)
	}
}

func TestProcessEmitListenerThrowPropagatesSynchronously(t *testing.T) {
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
		process.on("uncaughtException", function() { events.push("uncaught"); });
		process.on("x", function() { throw new Error("boom"); });
		try {
			events.push("emit:" + process.emit("x"));
		} catch (err) {
			events.push("caught:" + err.message);
		}
		events.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "caught:boom"; got != want {
		t.Fatalf("process.emit throw events = %q, want %q", got, want)
	}
}

func TestProcessOnceListenerSurvivesEarlierThrow(t *testing.T) {
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
		process.on("x", function() { events.push("a"); throw new Error("boom"); });
		process.once("x", function() { events.push("once"); });
		for (let i = 0; i < 2; i++) {
			try {
				process.emit("x");
			} catch (err) {
				events.push("caught" + i);
			}
		}
		events.push("listeners:" + process.listenerCount("x"));
		events.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "a,caught0,a,caught1,listeners:2"; got != want {
		t.Fatalf("process.once throw handling = %q, want %q", got, want)
	}
}

func TestProcessEventEmitterNodeListenerSemantics(t *testing.T) {
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
		const s1 = Symbol("evt");
		const s2 = Symbol("evt");
		process.on(s1, function() { events.push("s1"); });
		process.on(s2, function() { events.push("s2"); });
		events.push("c1:" + process.listenerCount(s1));
		events.push("c2:" + process.listenerCount(s2));
		process.emit(s1);
		process.emit(s2);

		function f() { events.push("f"); }
		process.on("dup", f);
		process.once("dup", f);
		process.off("dup", f);
		events.push("count-before:" + process.listenerCount("dup"));
		process.emit("dup");
		events.push("count-after1:" + process.listenerCount("dup"));
		process.emit("dup");
		events.push("count-after2:" + process.listenerCount("dup"));

		function g() {}
		process.on("filter", f);
		process.on("filter", f);
		process.on("filter", g);
		events.push("all:" + process.listenerCount("filter"));
		events.push("null:" + process.listenerCount("filter", null));
		events.push("f:" + process.listenerCount("filter", f));
		events.push("g:" + process.listenerCount("filter", g));
		events.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "c1:1,c2:1,s1,s2,count-before:1,f,count-after1:1,f,count-after2:1,all:3,null:3,f:2,g:1"
	if got := value.String(); got != want {
		t.Fatalf("process EventEmitter listener semantics = %q, want %q", got, want)
	}
}

func TestProcessEventEmitterTransparentProxySharesRegistry(t *testing.T) {
	loop, runtime, _ := newBoundOwnershipAdapter(t)
	t.Cleanup(func() { _ = loop.Close() })

	value, err := runtime.RunString(`
		const proxy = new Proxy(new Proxy(process, {}), {});
		const events = [];
		function listener(value) {
			events.push(value + ":" + (this === process ? "process" : this === proxy ? "proxy" : "other"));
		}
		const returned = proxy.on("proxy-event", listener) === proxy;
		const before = [process.listenerCount("proxy-event"), proxy.listenerCount("proxy-event")];
		const processResult = process.emit("proxy-event", "direct");
		const proxyResult = proxy.emit("proxy-event", "proxied");
		proxy.off("proxy-event", listener);
		JSON.stringify({ returned, before, processResult, proxyResult, after: process.listenerCount("proxy-event"), events });
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := `{"returned":true,"before":[1,1],"processResult":true,"proxyResult":true,"after":0,"events":["direct:process","proxied:proxy"]}`
	if got := value.String(); got != want {
		t.Fatalf("transparent process proxy semantics = %s, want %s", got, want)
	}
}

func TestProcessEventEmitterCoreCapturedAtConstruction(t *testing.T) {
	loop := goeventloop.New()
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if _, err := runtime.RunString(`
		Reflect.apply = function() { throw new Error("poisoned Reflect.apply"); };
		Object.getPrototypeOf = function() { throw new Error("poisoned Object.getPrototypeOf"); };
		Array.prototype.slice = function() { throw new Error("poisoned Array.prototype.slice"); };
	`); err != nil {
		t.Fatalf("poison userland intrinsics: %v", err)
	}
	bindProcessTestSurface(t, adapter)
	value, err := runtime.RunString(`
		(() => {
			const events = [];
			let mutate = true;
			function first(value) {
				events.push("first:" + value);
				if (mutate) {
					mutate = false;
					process.on("captured", third);
				}
			}
			function second(value) { events.push("second:" + value); }
			function third(value) { events.push("third:" + value); }
			process.on("captured", first);
			process.on("captured", second);
			for (let index = 0; index < 5; index++) process.on("captured", function() {});
			process.emit("captured", 1);
			process.emit("captured", 2);
			return events.join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("run captured EventEmitter core: %v", err)
	}
	want := "first:1,second:1,first:2,second:2,third:2"
	if got := value.String(); got != want {
		t.Fatalf("captured EventEmitter core = %q, want %q", got, want)
	}
}

func TestProcessEventEmitterObservableStateAndKeyCoercion(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			function assert(condition, label) {
				if (!condition) throw new Error(label);
			}
			const on = process.on;
			const off = process.off;
			const emit = process.emit;
			const listenerCount = process.listenerCount;

			for (const name of ["_events", "_eventsCount", "_maxListeners"]) {
				const descriptor = Object.getOwnPropertyDescriptor(process, name);
				assert(descriptor && descriptor.writable && descriptor.enumerable && descriptor.configurable,
					name + " descriptor");
			}
			assert(Object.getPrototypeOf(process._events) === null, "process events prototype");
			assert(process._eventsCount === 2, "initial process event count");
			assert(process._events.newListener.name === "startListeningIfSignal" &&
				process._events.newListener.length === 1 &&
				process._events.removeListener.name === "stopListeningIfSignal" &&
				process._events.removeListener.length === 1,
				"initial process meta-listener shape");
			const EventEmitter = Object.getPrototypeOf(Object.getPrototypeOf(process)).constructor;
			const constructed = new EventEmitter();
			assert(Object.prototype.hasOwnProperty.call(constructed, "_events") &&
				Object.getPrototypeOf(constructed._events) === null && constructed._eventsCount === 0 &&
				Object.prototype.hasOwnProperty.call(constructed, "_maxListeners"),
				"constructed emitter state");
			const shapedEvents = Object.create(null);
			const shaped = { _events: shapedEvents, _eventsCount: 0 };
			Reflect.apply(EventEmitter, shaped, []);
			function shapedListener() {}
			Reflect.apply(on, shaped, ["shaped", shapedListener]);
			Reflect.apply(off, shaped, ["shaped", shapedListener]);
			assert(shaped._events === shapedEvents && shaped._eventsCount === 0 &&
				Object.prototype.hasOwnProperty.call(shapedEvents, "shaped") && shapedEvents.shaped === undefined,
				"shape-mode removal state");

			const idle = {};
			let idleCoercions = 0;
			const idleType = { [Symbol.toPrimitive]() { idleCoercions++; return "idle"; } };
			function idleListener() {}
			assert(Reflect.apply(emit, idle, [idleType]) === false, "idle emit result");
			assert(Reflect.apply(listenerCount, idle, [idleType]) === 0, "idle listener count");
			assert(Reflect.apply(off, idle, [idleType, idleListener]) === idle, "idle off receiver");
			assert(idleCoercions === 0, "idle operations coerced event type");

			const receiver = {};
			let coercions = 0;
			const type = { [Symbol.toPrimitive]() { coercions++; return "x"; } };
			function first() {}
			function second() {}
			function third() {}
			Reflect.apply(on, receiver, [type, first]);
			const firstEvents = receiver._events;
			assert(coercions === 1, "first-listener coercion count");
			assert(Object.getPrototypeOf(firstEvents) === null && receiver._eventsCount === 1,
				"first-listener state");
			Reflect.apply(on, receiver, [type, second]);
			assert(coercions === 3 && Array.isArray(receiver._events.x) &&
				receiver._events.x.length === 2 && receiver._eventsCount === 1,
				"second-listener state");
			Reflect.apply(on, receiver, [type, third]);
			assert(coercions === 4 && receiver._events.x.length === 3 && receiver._eventsCount === 1,
				"third-listener state");
			Reflect.apply(off, receiver, ["x", third]);
			Reflect.apply(off, receiver, ["x", second]);
			assert(receiver._events.x === first && receiver._eventsCount === 1,
				"listener-array collapse");
			Reflect.apply(off, receiver, ["x", first]);
			assert(receiver._events !== firstEvents && Object.getPrototypeOf(receiver._events) === null &&
				receiver._eventsCount === 0 && Reflect.ownKeys(receiver._events).length === 0,
				"last-listener reset");

			let statefulCoercions = 0;
			const statefulType = {
				[Symbol.toPrimitive]() { return ++statefulCoercions === 1 ? "a" : "b"; },
			};
			function a() {}
			function b() {}
			function added() {}
			const statefulEvents = Object.assign(Object.create(null), { a, b });
			const stateful = { _events: statefulEvents, _eventsCount: 2 };
			Reflect.apply(on, stateful, [statefulType, added]);
			assert(statefulCoercions === 2 && statefulEvents.a === a &&
				Array.isArray(statefulEvents.b) && statefulEvents.b.length === 2 &&
				statefulEvents.b[0] === a && statefulEvents.b[1] === added &&
				stateful._eventsCount === 2,
				"stateful get/set key coercion");

			const mutationReceiver = {};
			const mutationLog = [];
			let mutate = true;
			function mutationFirst() {
				mutationLog.push("first");
				if (mutate) {
					mutate = false;
					Reflect.apply(on, mutationReceiver, ["mutation", mutationThird]);
					Reflect.apply(off, mutationReceiver, ["mutation", mutationSecond]);
				}
			}
			function mutationSecond() { mutationLog.push("second"); }
			function mutationThird() { mutationLog.push("third"); }
			Reflect.apply(on, mutationReceiver, ["mutation", mutationFirst]);
			Reflect.apply(on, mutationReceiver, ["mutation", mutationSecond]);
			Reflect.apply(emit, mutationReceiver, ["mutation"]);
			Reflect.apply(emit, mutationReceiver, ["mutation"]);
			assert(mutationLog.join(",") === "first,second,first,third",
				"listener-array mutation during emission");

			const speciesReceiver = {};
			let speciesMutated = false;
			function speciesFirst() {
				if (speciesMutated) return;
				speciesMutated = true;
				Reflect.apply(on, speciesReceiver, ["species", function speciesThird() {}]);
			}
			function speciesSecond() {}
			Reflect.apply(on, speciesReceiver, ["species", speciesFirst]);
			Reflect.apply(on, speciesReceiver, ["species", speciesSecond]);
			Object.defineProperty(speciesReceiver._events.species, "constructor", {
				get() { throw new Error("listener-array constructor was observed"); },
			});
			Reflect.apply(emit, speciesReceiver, ["species"]);
			assert(speciesReceiver._events.species.length === 3,
				"specialized listener-array clone");

			return "ok";
		})()
	`)
	if err != nil {
		t.Fatalf("observable EventEmitter state and coercion: %v", err)
	}
	if got := value.String(); got != "ok" {
		t.Fatalf("observable EventEmitter state and coercion = %q, want ok", got)
	}
}

func TestProcessEventEmitterMetaOnceAndListenerAliases(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			function assert(condition, label) {
				if (!condition) throw new Error(label);
			}
			const on = process.on;
			const off = process.off;
			const once = process.once;
			const emit = process.emit;
			const listenerCount = process.listenerCount;

			let metaCoercions = 0;
			let metaObservation;
			const metaType = {
				[Symbol.toPrimitive]() { metaCoercions++; return "meta-target"; },
			};
			function alias() {}
			function outer() {}
			outer.listener = alias;
			const replacement = Object.create(null);
			const metaReceiver = { emit };
			Reflect.apply(on, metaReceiver, ["newListener", function(type, listener) {
				if (type !== metaType) return;
				metaObservation = [metaCoercions, listener === alias, type === metaType];
				metaReceiver._events = replacement;
				metaReceiver._eventsCount = 0;
			}]);
			Reflect.apply(on, metaReceiver, [metaType, outer]);
			assert(metaObservation.join(",") === "0,true,true", "newListener order and arguments");
			assert(metaCoercions === 2 && metaReceiver._events === replacement &&
				replacement["meta-target"] === outer && metaReceiver._eventsCount === 1,
				"newListener state replacement");

			let onGets = 0;
			let onceCoercions = 0;
			let wrapped;
			let removal;
			const onceLog = [];
			const onceType = {
				[Symbol.toPrimitive]() { onceCoercions++; return "once-target"; },
			};
			function onceListener(value) {
				onceLog.push("listener:" + value + ":" + (this === onceReceiver));
			}
			const onceReceiver = {
				get on() {
					onGets++;
					return function(type, listener) {
						assert(this === onceReceiver && type === onceType, "dynamic once on receiver");
						wrapped = listener;
					};
				},
			};
			let invalid;
			try { Reflect.apply(once, onceReceiver, [onceType, null]); } catch (caught) { invalid = caught; }
			assert(invalid && invalid.code === "ERR_INVALID_ARG_TYPE" && onGets === 0 && onceCoercions === 0,
				"once validation order");
			assert(Reflect.apply(once, onceReceiver, [onceType, onceListener]) === onceReceiver,
				"once receiver return");
			assert(onGets === 1 && onceCoercions === 0 && wrapped.listener === onceListener,
				"once dynamic on and wrapper alias");
			onceReceiver.removeListener = function(type, listener) {
				onceLog.push("remove");
				removal = [this === onceReceiver, type === onceType, listener === wrapped];
			};
			wrapped(7);
			wrapped(8);
			assert(removal.join(",") === "true,true,true" && onceCoercions === 0 &&
				onceLog.join(",") === "remove,listener:7:true",
				"once dynamic removal and firing");

			const aliasLog = [];
			const aliasReceiver = { emit };
			function listenerAlias() {}
			function aliasedListener() {}
			aliasedListener.listener = listenerAlias;
			Reflect.apply(on, aliasReceiver, ["newListener", function(type, listener) {
				if (type === "aliased") aliasLog.push("new:" + (listener === listenerAlias));
			}]);
			Reflect.apply(on, aliasReceiver, ["removeListener", function(type, listener) {
				if (type === "aliased") aliasLog.push("remove:" + (listener === listenerAlias));
			}]);
			Reflect.apply(on, aliasReceiver, ["aliased", aliasedListener]);
			aliasLog.push("count:" + Reflect.apply(listenerCount, aliasReceiver, ["aliased", listenerAlias]));
			Reflect.apply(off, aliasReceiver, ["aliased", listenerAlias]);
			aliasLog.push("after:" + Reflect.apply(listenerCount, aliasReceiver, ["aliased"]));
			assert(aliasLog.join(",") === "new:true,count:1,remove:true,after:0", "listener aliases");

			return "ok";
		})()
	`)
	if err != nil {
		t.Fatalf("EventEmitter meta, once, and listener aliases: %v", err)
	}
	if got := value.String(); got != "ok" {
		t.Fatalf("EventEmitter meta, once, and listener aliases = %q, want ok", got)
	}
}

func TestProcessEventEmitterProxyTrapOrder(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const on = process.on;
			const off = process.off;
			const emit = process.emit;
			const trace = [];
			const eventTarget = Object.create(null);
			const events = new Proxy(eventTarget, {
				get(target, key) {
					trace.push("events.get:" + (typeof key === "symbol" ? "symbol" : key));
					return target[key];
				},
				set(target, key, value) {
					trace.push("events.set:" + (typeof key === "symbol" ? "symbol" : key));
					target[key] = value;
					return true;
				},
				deleteProperty(target, key) {
					trace.push("events.delete:" + (typeof key === "symbol" ? "symbol" : key));
					return delete target[key];
				},
			});
			const receiverTarget = { _events: events, _eventsCount: 0, emit };
			const receiver = new Proxy(receiverTarget, {
				get(target, key) {
					trace.push("receiver.get:" + (typeof key === "symbol" ? "symbol" : key));
					return target[key];
				},
				set(target, key, value) {
					trace.push("receiver.set:" + (typeof key === "symbol" ? "symbol" : key));
					target[key] = value;
					return true;
				},
			});
			let conversions = 0;
			const type = {
				[Symbol.toPrimitive]() { trace.push("key:" + (++conversions)); return "x"; },
			};
			eventTarget.newListener = function(metaType) { trace.push("meta:" + (metaType === type)); };
			receiverTarget._eventsCount = 1;
			function listener(value) { trace.push("listener:" + value + ":" + (this === receiver)); }
			Reflect.apply(on, receiver, [type, listener]);
			Reflect.apply(emit, receiver, [type, 7]);
			Reflect.apply(off, receiver, [type, listener]);
			return trace.join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("EventEmitter proxy trap order: %v", err)
	}
	want := "receiver.get:_events,events.get:newListener,receiver.get:emit," +
		"receiver.get:_events,events.get:newListener,meta:true,receiver.get:_events," +
		"key:1,events.get:x,key:2,events.set:x,receiver.get:_eventsCount," +
		"receiver.set:_eventsCount,receiver.get:_events,key:3,events.get:x," +
		"listener:7:true,receiver.get:_events,key:4,events.get:x," +
		"receiver.get:_eventsCount,receiver.set:_eventsCount,receiver.get:symbol," +
		"receiver.get:_eventsCount,key:5,events.delete:x,events.get:removeListener"
	if got := value.String(); got != want {
		t.Fatalf("EventEmitter proxy trap order = %q, want %q", got, want)
	}
}

func TestProcessEventEmitterNodeObservableInternals(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const on = process.on;
			const emit = process.emit;
			const listenerCount = process.listenerCount;
			function first() {}
			function second() {}

			const lengthTrace = [];
			const listeners = new Proxy([first, second], {
				get(target, key, receiver) {
					lengthTrace.push(typeof key === "symbol" ? key.description : String(key));
					return Reflect.get(target, key, receiver);
				},
			});
			const matching = Reflect.apply(listenerCount, { _events: { x: listeners } }, ["x", first]);

			const captureTrace = [];
			const captureReceiver = new Proxy({ _events: { x() { return {}; } } }, {
				get(target, key, receiver) {
					captureTrace.push(typeof key === "symbol" ? key.description : String(key));
					return Reflect.get(target, key, receiver);
				},
			});
			const captured = Reflect.apply(emit, captureReceiver, ["x"]);

			const monitorTrace = [];
			const monitorEvents = new Proxy(Object.create(null), {
				get(target, key, receiver) {
					monitorTrace.push(typeof key === "symbol" ? key.description : String(key));
					return Reflect.get(target, key, receiver);
				},
			});
			let monitorError;
			try { Reflect.apply(emit, { _events: monitorEvents, emit }, ["error", 7]); }
			catch (caught) { monitorError = caught; }

			let unhandled;
			try { process.emit("error", 123); } catch (caught) { unhandled = caught; }
			const passthrough = new Error("x");
			try { process.emit("error", passthrough); } catch (_) {}

			const originalEmitWarning = process.emitWarning;
			let warning;
			process.emitWarning = function(value) { warning = value; };
			const warningReceiver = { emit };
			for (let index = 0; index < 11; index++) {
				Reflect.apply(on, warningReceiver, ["leak", function() {}]);
			}
			process.emitWarning = originalEmitWarning;

			const marker = new Error("marker");
			const maxReceiver = { _events: { x: first }, _eventsCount: 1, emit };
			Object.defineProperty(maxReceiver, "_maxListeners", { get() { throw marker; } });
			let maxCaught;
			try { Reflect.apply(on, maxReceiver, ["x", function() {}]); }
			catch (caught) { maxCaught = caught; }

			function errorShape(error) {
				return {
					ctor: error.constructor.name,
					instance: error instanceof Error,
					string: String(error),
					message: error.message,
					context: error.context,
					keys: Object.keys(error),
					ownNames: Object.getOwnPropertyNames(error),
				};
			}
			return JSON.stringify({
				matching,
				lengthTrace,
				captured,
				captureTrace,
				monitorTrace,
				monitorCaught: errorShape(monitorError),
				unhandled: errorShape(unhandled),
				passthrough: Object.getOwnPropertySymbols(passthrough).map(function(symbol) {
					const descriptor = Object.getOwnPropertyDescriptor(passthrough, symbol);
					return [symbol.description, descriptor.value.name, descriptor.value.length,
						descriptor.writable, descriptor.enumerable, descriptor.configurable,
						descriptor.value().includes("Error: x") &&
							descriptor.value().includes("Emitted 'error' event on process instance at:")];
				}),
				warning: {
					ctor: warning.constructor.name,
					instance: warning instanceof Error,
					message: warning.message,
					name: warning.name,
					type: warning.type,
					count: warning.count,
					emitter: warning.emitter === warningReceiver,
					keys: Object.keys(warning),
					ownNames: Object.getOwnPropertyNames(warning),
				},
				max: { same: maxCaught === marker, length: maxReceiver._events.x.length },
			});
		})()
	`)
	if err != nil {
		t.Fatalf("EventEmitter Node-observable internals: %v", err)
	}
	want := `{"matching":1,"lengthTrace":["length","0","1","1"],` +
		`"captured":true,"captureTrace":["_events","kCapture"],` +
		`"monitorTrace":["events.errorMonitor","error"],` +
		`"monitorCaught":{"ctor":"Error","instance":true,"string":"Error [ERR_UNHANDLED_ERROR]: Unhandled error. (7)","message":"Unhandled error. (7)","context":7,"keys":["code","context"],"ownNames":["stack","code","message","context"]},` +
		`"unhandled":{"ctor":"Error","instance":true,"string":"Error [ERR_UNHANDLED_ERROR]: Unhandled error. (123)","message":"Unhandled error. (123)","context":123,"keys":["code","context"],"ownNames":["stack","code","message","context"]},` +
		`"passthrough":[["kEnhanceStackBeforeInspector","bound enhanceStackTrace",0,false,false,true,true]],` +
		`"warning":{"ctor":"Error","instance":true,"message":"Possible EventEmitter memory leak detected. 11 leak listeners added to [Object]. MaxListeners is 10. Use emitter.setMaxListeners() to increase limit","name":"MaxListenersExceededWarning","type":"leak","count":11,"emitter":true,"keys":["name","emitter","type","count"],"ownNames":["stack","message","name","emitter","type","count"]},` +
		`"max":{"same":true,"length":2}}`
	if got := value.String(); got != want {
		t.Fatalf("EventEmitter Node-observable internals = %s, want %s", got, want)
	}
}

func TestProcessHostEmissionUsesVisibleEmitterState(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	if _, err := adapter.runtime.RunString(`
		globalThis.__oldProcessHits = 0;
		globalThis.__visibleProcessHits = [];
		globalThis.__dynamicProcessEmitHits = 0;
		process.on("host-visible", function() { __oldProcessHits++; });
		const replacement = Object.create(null);
		replacement["host-visible"] = function(value) { __visibleProcessHits.push("replaced:" + value); };
		replacement["host-injected"] = function(value) { __visibleProcessHits.push("injected:" + value); };
		process._events = replacement;
		process._eventsCount = 2;
		process.emit = function() { __dynamicProcessEmitHits++; return false; };
	`); err != nil {
		t.Fatalf("replace visible process state: %v", err)
	}
	if adapter.emitProcess("host-visible", adapter.runtime.ToValue(7)) {
		t.Fatal("host-visible emission ignored replacement process.emit result")
	}
	if adapter.emitProcess("host-injected", adapter.runtime.ToValue(9)) {
		t.Fatal("host-injected emission ignored replacement process.emit result")
	}
	value, err := adapter.runtime.RunString(`
		JSON.stringify({ old: __oldProcessHits, visible: __visibleProcessHits, dynamic: __dynamicProcessEmitHits })
	`)
	if err != nil {
		t.Fatalf("read visible process emission result: %v", err)
	}
	want := `{"old":0,"visible":[],"dynamic":2}`
	if got := value.String(); got != want {
		t.Fatalf("host process emission = %s, want %s", got, want)
	}
}

func TestProcessMethodMetadataEventNamesAndReceivers(t *testing.T) {
	loop := goeventloop.New()
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	bindProcessTestSurface(t, adapter)

	_, err = runtime.RunString(`
		const metadata = [
			["on", "addListener", 2], ["addListener", "addListener", 2],
			["once", "once", 2], ["off", "removeListener", 2],
			["removeListener", "removeListener", 2], ["emit", "emit", 1],
			["listenerCount", "listenerCount", 2], ["emitWarning", "emitWarning", 4],
			["exit", "exit", 1], ["nextTick", "nextTick", 1],
		];
		for (const [property, name, length] of metadata) {
			if (process[property].name !== name || process[property].length !== length) {
				throw new Error(property + " metadata");
			}
		}
		if (process.on !== process.addListener || process.off !== process.removeListener) {
			throw new Error("aliases do not share identity");
		}
		function descriptorDepth(object, property) {
			let depth = 0;
			for (let current = object; current !== null; current = Object.getPrototypeOf(current), depth++) {
				if (Object.prototype.hasOwnProperty.call(current, property)) return depth;
			}
			return -1;
		}
		for (const property of ["on", "addListener", "once", "off", "removeListener", "emit", "listenerCount"]) {
			if (descriptorDepth(process, property) !== 2) throw new Error(property + " prototype depth");
		}
		if (descriptorDepth(process, "constructor") !== 1 ||
			process.constructor.name !== "process" || process.constructor.length !== 0) {
			throw new Error("process constructor topology");
		}
		const processPrototype = Object.getPrototypeOf(process);
		const eventEmitterPrototype = Object.getPrototypeOf(processPrototype);
		if (processPrototype.constructor !== process.constructor ||
			eventEmitterPrototype.constructor.name !== "EventEmitter" ||
			eventEmitterPrototype.constructor.length !== 1 ||
			Object.getPrototypeOf(eventEmitterPrototype) !== Object.prototype) {
			throw new Error("process prototype chain");
		}
		for (const property of ["prependListener", "prependOnceListener", "listeners", "rawListeners", "eventNames", "removeAllListeners", "setMaxListeners", "getMaxListeners"]) {
			if (property in process) throw new Error("unexpected EventEmitter method " + property);
		}

		const receiver = {};
		let seen = 0;
		function listener(value) {
			if (this !== receiver || value !== 7) throw new Error("wrong listener receiver/argument");
			seen++;
		}
		if (Reflect.apply(process.on, receiver, ["local", listener]) !== receiver) throw new Error("on return receiver");
		if (process.listenerCount("local") !== 0) throw new Error("borrowed listener leaked to process");
		if (Reflect.apply(process.listenerCount, receiver, ["local"]) !== 1) throw new Error("borrowed listener missing");
		if (Reflect.apply(process.emit, receiver, ["local", 7]) !== true || seen !== 1) throw new Error("borrowed emit");
		if (Reflect.apply(process.off, receiver, ["local", listener]) !== receiver) throw new Error("off return receiver");
		if (Reflect.apply(process.emit, receiver, ["local"]) !== false) throw new Error("borrowed removal");

		const validSymbol = Symbol("event");
		process.on(validSymbol, listener);
		process.off(validSymbol, listener);
		for (const value of [undefined, null, 1, 1n, {}, [], true]) {
			let calls = 0;
			function coercedListener() { calls += 1; }
			if (process.on(value, coercedListener) !== process) throw new Error("coerced on receiver");
			if (process.listenerCount(value) !== 1) throw new Error("coerced listener missing " + String(value));
			if (process.emit(value) !== true || calls !== 1) throw new Error("coerced emit " + String(value));
			if (process.off(value, coercedListener) !== process) throw new Error("coerced off receiver");
			if (process.listenerCount(value) !== 0) throw new Error("coerced listener retained " + String(value));
		}
		let coercions = 0;
		const orderedEvent = { [Symbol.toPrimitive]() { coercions += 1; return "ordered"; } };
		let invalidListenerError;
		try { process.on(orderedEvent, null); } catch (caught) { invalidListenerError = caught; }
		if (!(invalidListenerError instanceof TypeError) || invalidListenerError.code !== "ERR_INVALID_ARG_TYPE" || coercions !== 0) {
			throw new Error("listener validation did not precede event coercion");
		}

		for (const property of ["_exiting", "exitCode"]) {
			const descriptor = Object.getOwnPropertyDescriptor(process, property);
			if (!descriptor || descriptor.get.name !== "get" || descriptor.get.length !== 0 ||
				descriptor.set.name !== "set" || descriptor.set.length !== 1 || !descriptor.enumerable) {
				throw new Error(property + " accessor metadata");
			}
		}
		if (!Object.getOwnPropertyDescriptor(process, "_exiting").configurable) throw new Error("_exiting configurability");
		if (Object.getOwnPropertyDescriptor(process, "exitCode").configurable) throw new Error("exitCode configurability");
	`)
	if err != nil {
		t.Fatalf("process metadata/event-name/receiver contract: %v", err)
	}
}

func TestProcessEventEmitterMetaEventsAndOffValidation(t *testing.T) {
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
		function f() {}
		function onRemove(name, listener) { events.push("remove:" + String(name) + ":" + (listener === f)); }
		process.on("newListener", function(name, listener) {
			if (name !== "newListener") events.push("new:" + String(name) + ":" + (listener === f));
		});
		process.on("removeListener", onRemove);
		process.on("x", f);
		process.off("x", f);

		for (const entry of [
			["missing", ["bad"]],
			["undefined", ["bad", undefined]],
			["null", ["bad", null]],
			["number", ["bad", 1]],
			["string", ["bad", "fn"]],
		]) {
			try { process.off.apply(process, entry[1]); events.push(entry[0] + ":ok"); }
			catch (err) { events.push(entry[0] + ":" + err.name + ":" + err.code); }
		}
		events.push("valid:" + (process.off("bad", function(){}) === process));
		events.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "new:removeListener:false,new:x:true,remove:x:true," +
		"missing:TypeError:ERR_INVALID_ARG_TYPE," +
		"undefined:TypeError:ERR_INVALID_ARG_TYPE," +
		"null:TypeError:ERR_INVALID_ARG_TYPE," +
		"number:TypeError:ERR_INVALID_ARG_TYPE," +
		"string:TypeError:ERR_INVALID_ARG_TYPE," +
		"valid:true"
	if got := value.String(); got != want {
		t.Fatalf("process EventEmitter meta/off semantics = %q, want %q", got, want)
	}
}
