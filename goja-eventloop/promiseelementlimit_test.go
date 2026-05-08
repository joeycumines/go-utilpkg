package gojaeventloop

import (
	"strings"
	"testing"

	"github.com/joeycumines/goja"
)

func TestNodePromiseCombinatorElementLimit(t *testing.T) {
	if got, want := nodePromiseCombinatorElementLimit, 2_097_150; got != want {
		t.Fatalf("nodePromiseCombinatorElementLimit = %d, want %d", got, want)
	}

	runtime := newPromiseElementLimitRuntime(t, 2)
	value, err := runtime.RunString(`
		(() => {
			const NativeAggregateError = AggregateError;
			const NativeRangeError = RangeError;
			const failures = [];
			let poisonCalls = 0;
			globalThis.RangeError = function PoisonRangeError() { poisonCalls++; };

			function check(condition, message) {
				if (!condition) failures.push(message);
			}

			function probe(method, count, closeMode) {
				const closeError = { closeError: true };
				const events = [];
				const resolvedValues = [];
				let nextCalls = 0;
				let valueGets = 0;
				let returnGets = 0;
				let returnCalls = 0;
				let resolveCalls = 0;
				let thenCalls = 0;
				let produced = 0;

				const iterator = {
					next() {
						nextCalls++;
						if (produced >= count) return { done: true };
						produced++;
						const value = produced;
						return {
							done: false,
							get value() {
								valueGets++;
								events.push("value:" + value);
								return value;
							},
						};
					},
				};
				Object.defineProperty(iterator, "return", {
					configurable: true,
					get() {
						returnGets++;
						events.push("return:get");
						if (closeMode === "throwing-getter") throw closeError;
						if (closeMode === "noncallable") return 1;
						if (closeMode === "null") return null;
						if (closeMode === "undefined") return undefined;
						return function () {
							returnCalls++;
							events.push("return:call");
							if (this !== iterator) events.push("return:this");
							if (closeMode === "throwing-call") throw closeError;
							if (closeMode === "primitive") return 1;
							return { done: true };
						};
					},
				});

				const thenable = {
					then() {
						thenCalls++;
						events.push("then");
					},
				};
				let rejectedReason;
				function C(executor) {
					const state = { fulfilled: 0, rejected: 0 };
					executor(
						(value) => {
							state.fulfilled++;
							state.value = value;
							events.push("fulfill");
						},
						(reason) => {
							state.rejected++;
							state.reason = reason;
							rejectedReason = reason;
							events.push("reject");
						},
					);
					return state;
				}
				C.resolve = function (value) {
					resolveCalls++;
					resolvedValues.push(value);
					events.push("resolve:" + value);
					return thenable;
				};

				const state = Promise[method].call(C, {
					[Symbol.iterator]() { return iterator; },
				});
				return {
					closeError,
					events,
					method,
					nextCalls,
					poisonCalls,
					rejectedReason,
					resolvedValues,
					resolveCalls,
					returnCalls,
					returnGets,
					state,
					thenCalls,
					valueGets,
				};
			}

			const messages = {
				all: "Too many elements passed to Promise.all",
				allSettled: "Too many elements passed to Promise.all",
				any: "Too many elements passed to Promise.any",
			};
			for (const method of ["all", "allSettled", "any"]) {
				const accepted = probe(method, 2, "object");
				check(accepted.nextCalls === 3, method + ".accepted.next=" + accepted.nextCalls);
				check(accepted.valueGets === 2, method + ".accepted.value=" + accepted.valueGets);
				check(accepted.resolveCalls === 2, method + ".accepted.resolve=" + accepted.resolveCalls);
				check(accepted.thenCalls === 2, method + ".accepted.then=" + accepted.thenCalls);
				check(accepted.returnGets === 0, method + ".accepted.return-get=" + accepted.returnGets);
				check(accepted.returnCalls === 0, method + ".accepted.return-call=" + accepted.returnCalls);
				check(accepted.state.rejected === 0, method + ".accepted.reject=" + accepted.state.rejected);
				check(accepted.resolvedValues.join(",") === "1,2", method + ".accepted.values=" + accepted.resolvedValues);

				const overflow = probe(method, 3, "object");
				check(overflow.nextCalls === 3, method + ".overflow.next=" + overflow.nextCalls);
				check(overflow.valueGets === 3, method + ".overflow.value=" + overflow.valueGets);
				check(overflow.resolveCalls === 2, method + ".overflow.resolve=" + overflow.resolveCalls);
				check(overflow.thenCalls === 2, method + ".overflow.then=" + overflow.thenCalls);
				check(overflow.returnGets === 1, method + ".overflow.return-get=" + overflow.returnGets);
				check(overflow.returnCalls === 1, method + ".overflow.return-call=" + overflow.returnCalls);
				check(overflow.state.rejected === 1, method + ".overflow.reject=" + overflow.state.rejected);
				check(overflow.state.reason === overflow.rejectedReason, method + ".overflow.reason-identity");
				check(Object.getPrototypeOf(overflow.state.reason) === NativeRangeError.prototype, method + ".overflow.prototype");
				check(overflow.state.reason.name === "RangeError", method + ".overflow.name=" + overflow.state.reason.name);
				check(overflow.state.reason.message === messages[method], method + ".overflow.message=" + overflow.state.reason.message);
				check(!(overflow.state.reason instanceof NativeAggregateError), method + ".overflow.aggregate");
				check(
					overflow.events.join(",") ===
						"value:1,resolve:1,then,value:2,resolve:2,then,value:3,return:get,return:call,reject",
					method + ".overflow.events=" + overflow.events.join(","),
				);
			}

			const race = probe("race", 3, "object");
			check(race.nextCalls === 4, "race.next=" + race.nextCalls);
			check(race.valueGets === 3, "race.value=" + race.valueGets);
			check(race.resolveCalls === 3, "race.resolve=" + race.resolveCalls);
			check(race.thenCalls === 3, "race.then=" + race.thenCalls);
			check(race.returnGets === 0, "race.return-get=" + race.returnGets);
			check(race.returnCalls === 0, "race.return-call=" + race.returnCalls);
			check(race.state.rejected === 0, "race.reject=" + race.state.rejected);
			check(poisonCalls === 0, "poison=" + poisonCalls);
			return failures.join("|");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got := value.String(); got != "" {
		t.Fatalf("Promise combinator element limit mismatches: %s", got)
	}
}

func TestNodePromiseCombinatorElementLimitClosePrecedence(t *testing.T) {
	runtime := newPromiseElementLimitRuntime(t, 2)
	value, err := runtime.RunString(`
		(() => {
			const NativeRangeError = RangeError;
			const failures = [];

			function check(condition, message) {
				if (!condition) failures.push(message);
			}

			function probe(mode) {
				const closeError = { mode };
				let produced = 0;
				let returnGets = 0;
				let returnCalls = 0;
				const iterator = {
					next() {
						produced++;
						return { done: false, value: produced };
					},
				};
				Object.defineProperty(iterator, "return", {
					get() {
						returnGets++;
						if (mode === "throwing-getter") throw closeError;
						if (mode === "noncallable") return 1;
						if (mode === "null") return null;
						if (mode === "undefined") return undefined;
						return function () {
							returnCalls++;
							if (mode === "throwing-call") throw closeError;
							if (mode === "primitive") return 1;
							return { done: true };
						};
					},
				});
				const thenable = { then() {} };
				function C(executor) {
					const state = { rejected: 0 };
					executor(
						function () {},
						function (reason) {
							state.rejected++;
							state.reason = reason;
						},
					);
					return state;
				}
				C.resolve = function () { return thenable; };
				const state = Promise.all.call(C, {
					[Symbol.iterator]() { return iterator; },
				});
				return { closeError, returnCalls, returnGets, state };
			}

			for (const mode of [
				"throwing-getter",
				"throwing-call",
				"primitive",
				"noncallable",
				"null",
				"undefined",
			]) {
				const result = probe(mode);
				check(result.returnGets === 1, mode + ".return-get=" + result.returnGets);
				const expectedCalls = mode === "throwing-call" || mode === "primitive" ? 1 : 0;
				check(result.returnCalls === expectedCalls, mode + ".return-call=" + result.returnCalls);
				check(result.state.rejected === 1, mode + ".reject=" + result.state.rejected);
				check(result.state.reason !== result.closeError, mode + ".close-error-won");
				check(result.state.reason instanceof NativeRangeError, mode + ".range-error");
				check(result.state.reason.message === "Too many elements passed to Promise.all", mode + ".message=" + result.state.reason.message);
			}
			return failures.join("|");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got := value.String(); got != "" {
		t.Fatalf("Promise combinator close precedence mismatches: %s", got)
	}
}

func TestNodePromiseCombinatorElementLimitValidation(t *testing.T) {
	runtime := goja.New()
	promise, ok := runtime.Get("Promise").(*goja.Object)
	if !ok || promise == nil {
		t.Fatal("Promise is unavailable")
	}
	adapter := &Adapter{runtime: runtime}
	err := adapter.bindNativePromiseExtensionsLimit(promise, 0)
	if err == nil || !strings.Contains(err.Error(), "element limit must be positive") {
		t.Fatalf("bindNativePromiseExtensionsLimit error = %v, want positive-limit error", err)
	}
}

func newPromiseElementLimitRuntime(t *testing.T, elementLimit int) *goja.Runtime {
	t.Helper()
	runtime := goja.New()
	promise, ok := runtime.Get("Promise").(*goja.Object)
	if !ok || promise == nil {
		t.Fatal("Promise is unavailable")
	}
	adapter := &Adapter{runtime: runtime}
	if err := adapter.bindNativePromiseExtensionsLimit(promise, elementLimit); err != nil {
		t.Fatalf("bind native Promise extensions: %v", err)
	}
	return runtime
}
