package gojaeventloop

import (
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func newNodePromiseRuntime(t *testing.T) *goja.Runtime {
	t.Helper()

	loop := goeventloop.New()
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind adapter: %v", err)
	}
	return runtime
}

func TestNodePromiseStaticGenericCallbackShapes(t *testing.T) {
	runtime := newNodePromiseRuntime(t)

	value, err := runtime.RunString(`
		(function () {
			const records = [];
			function constructible(fn) {
				try {
					Reflect.construct(function () {}, [], fn);
					return true;
				} catch (_) {
					return false;
				}
			}
			function shape(fn) {
				return [fn.name, fn.length, Object.hasOwn(fn, "prototype"), constructible(fn)];
			}
			for (const method of ["all", "race", "allSettled", "any", "withResolvers", "try"]) {
				function C(executor) {
					records.push([method + ".executor", shape(executor)]);
					executor(function () {}, function () {});
					return { method };
				}
				C.resolve = function (entry) {
					return { then: function () {} };
				};
				if (method === "withResolvers") Promise.withResolvers.call(C);
				else if (method === "try") Promise.try.call(C, function () { return 1; });
				else Promise[method].call(C, []);
			}

			let capabilityResolve;
			let capabilityReject;
			let activeMethod;
			function H(executor) {
				capabilityResolve = function capabilityResolve(value) {
					if (activeMethod === "all") records.push(["all.result", value[0]]);
					else if (activeMethod === "allSettled") {
						records.push(["allSettled.result", value[0].status, value[0].reason]);
					} else records.push([activeMethod + ".resolve", value]);
					return "resolve-return";
				};
				capabilityReject = function capabilityReject(reason) {
					if (reason instanceof AggregateError) {
						records.push([activeMethod + ".reject", reason.name, reason.message, reason.errors[0]]);
					} else records.push([activeMethod + ".reject", reason]);
					return "reject-return";
				};
				executor(capabilityResolve, capabilityReject);
				return { activeMethod };
			}
			H.resolve = function (entry) {
				return {
					then(onFulfilled, onRejected) {
						if (activeMethod === "all") {
							records.push(["all.handlers", shape(onFulfilled), onRejected === capabilityReject]);
							onFulfilled(entry);
						} else if (activeMethod === "race") {
							records.push(["race.handlers", onFulfilled === capabilityResolve, onRejected === capabilityReject]);
						} else if (activeMethod === "allSettled") {
							records.push(["allSettled.handlers", shape(onFulfilled), shape(onRejected)]);
							onRejected("settled-reason");
						} else {
							records.push(["any.handlers", onFulfilled === capabilityResolve, shape(onRejected)]);
							onRejected("any-reason");
							records.push(["any.resolveReturn", onFulfilled("any-value")]);
						}
					},
				};
			};
			for (activeMethod of ["all", "race", "allSettled", "any"]) {
				Promise[activeMethod].call(H, [activeMethod]);
			}
			return JSON.stringify(records);
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := `[["all.executor",["",2,false,false]],["race.executor",["",2,false,false]],["allSettled.executor",["",2,false,false]],["any.executor",["",2,false,false]],["withResolvers.executor",["",2,false,false]],["try.executor",["",2,false,false]],["all.handlers",["",1,false,false],true],["all.result","all"],["race.handlers",true,true],["allSettled.handlers",["",1,false,false],["",1,false,false]],["allSettled.result","rejected","settled-reason"],["any.handlers",true,["",1,false,false]],["any.resolve","any-value"],["any.resolveReturn","resolve-return"],["any.reject","AggregateError","All promises were rejected","any-reason"]]`
	if got := value.String(); got != want {
		t.Fatalf("generic callback trace = %s, want %s", got, want)
	}
}

func TestNodePromiseAllFinalResolveFailureRejects(t *testing.T) {
	runtime := newNodePromiseRuntime(t)

	value, err := runtime.RunString(`
		(function () {
			const records = [];
			for (const method of ["all", "allSettled"]) {
				const events = [];
				function C(executor) {
					executor(
						function (value) {
							events.push("resolve:" + value.length);
							throw new Error(method + " resolve boom");
						},
						function (reason) { events.push("reject:" + reason.message); },
					);
					return { method };
				}
				C.resolve = function (entry) { return { then: function () {} }; };
				try {
					const result = Promise[method].call(C, []);
					events.push("returned:" + result.method);
				} catch (error) {
					events.push("threw:" + error.message);
				}
				records.push([method, events]);
			}
			return JSON.stringify(records);
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := `[["all",["resolve:0","reject:all resolve boom","returned:all"]],["allSettled",["resolve:0","reject:allSettled resolve boom","returned:allSettled"]]]`
	if got := value.String(); got != want {
		t.Fatalf("final resolve trace = %s, want %s", got, want)
	}
}

func TestNodePromiseStaticPrototypeMutationResistance(t *testing.T) {
	runtime := newNodePromiseRuntime(t)

	value, err := runtime.RunString(`
		(function () {
			let setterCalls = 0;
			let allResult;
			let settledResult;
			let anyError;
			let tryResult;
			function one(entry) {
				return {
					[Symbol.iterator]() {
						let pending = true;
						return {
							next() {
								if (!pending) return { done: true };
								pending = false;
								return { done: false, value: entry };
							},
						};
					},
				};
			}
			Object.defineProperty(Array.prototype, "0", {
				configurable: true,
				set() { setterCalls++; },
			});
			Object.defineProperty(Object.prototype, "reason", {
				configurable: true,
				set() { setterCalls++; },
			});
			Object.defineProperty(Object.prototype, "value", {
				configurable: true,
				set() { setterCalls++; },
			});
			try {
				function All(executor) {
					executor(function (value) { allResult = value; }, function (reason) { allResult = reason; });
					return {};
				}
				All.resolve = function (entry) { return { then(resolve) { resolve(entry); } }; };
				Promise.all.call(All, one(11));

				function Settled(executor) {
					executor(function (value) { settledResult = value; }, function (reason) { settledResult = reason; });
					return {};
				}
				Settled.resolve = function () {
					return { then(resolve, reject) { reject(12); } };
				};
				Promise.allSettled.call(Settled, one(12));

				function Any(executor) {
					executor(function () {}, function (reason) { anyError = reason; });
					return {};
				}
				Any.resolve = function (entry) { return { then(resolve, reject) { reject(entry); } }; };
				Promise.any.call(Any, one(13));

				function Try(executor) {
					executor(function (value) { tryResult = value; }, function (reason) { tryResult = reason; });
					return {};
				}
				Promise.try.call(Try, function (entry) { return entry; }, 14);
			} finally {
				delete Array.prototype[0];
				delete Object.prototype.value;
				delete Object.prototype.reason;
			}
			return JSON.stringify({
				setterCalls,
				all: allResult,
				settled: settledResult,
				any: [anyError.name, anyError.message, anyError.errors],
				tried: tryResult,
			});
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := `{"setterCalls":0,"all":[11],"settled":[{"status":"rejected","reason":12}],"any":["AggregateError","All promises were rejected",[13]],"tried":14}`
	if got := value.String(); got != want {
		t.Fatalf("prototype mutation trace = %s, want %s", got, want)
	}
}

func TestNodePromiseAnyAggregateConstructionIgnoresArrayIterator(t *testing.T) {
	runtime := newNodePromiseRuntime(t)

	value, err := runtime.RunString(`
		(function () {
			const originalIterator = Array.prototype[Symbol.iterator];
			const token = {};
			let rejected;
			let trace;
			function C(executor) {
				executor(function () {}, function (reason) { rejected = reason; });
				return token;
			}
			C.resolve = function (entry) { return entry; };
			const empty = {
				[Symbol.iterator]() {
					return { next() { return { done: true }; } };
				},
			};
			Array.prototype[Symbol.iterator] = function () { throw new Error("array iterator poison"); };
			try {
				trace = Promise.any.call(C, empty) === token ? "returned" : "wrong-return";
			} catch (error) {
				trace = "threw:" + error.message;
			} finally {
				Array.prototype[Symbol.iterator] = originalIterator;
			}
			if (rejected === undefined) return JSON.stringify([trace, "missing-rejection"]);
			return JSON.stringify([trace, rejected.name, rejected.message, rejected.errors.length]);
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := `["returned","AggregateError","All promises were rejected",0]`
	if got := value.String(); got != want {
		t.Fatalf("AggregateError construction trace = %s, want %s", got, want)
	}
}

func TestNodePromiseStaticNullishResolveResultErrors(t *testing.T) {
	runtime := newNodePromiseRuntime(t)

	value, err := runtime.RunString(`
		(function () {
			const records = [];
			for (const method of ["all", "race", "allSettled", "any"]) {
				for (const entry of [undefined, null]) {
					let rejection;
					function C(executor) {
						executor(function () {}, function (reason) { rejection = reason; });
						return {};
					}
					C.resolve = function () { return entry; };
					Promise[method].call(C, [1]);
					records.push([method, String(entry), rejection.name, rejection.message]);
				}
			}
			return JSON.stringify(records);
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := `[["all","undefined","TypeError","Cannot read properties of undefined (reading 'then')"],["all","null","TypeError","Cannot read properties of null (reading 'then')"],["race","undefined","TypeError","Cannot read properties of undefined (reading 'then')"],["race","null","TypeError","Cannot read properties of null (reading 'then')"],["allSettled","undefined","TypeError","Cannot read properties of undefined (reading 'then')"],["allSettled","null","TypeError","Cannot read properties of null (reading 'then')"],["any","undefined","TypeError","Cannot read properties of undefined (reading 'then')"],["any","null","TypeError","Cannot read properties of null (reading 'then')"]]`
	if got := value.String(); got != want {
		t.Fatalf("nullish resolve result trace = %s, want %s", got, want)
	}
}
