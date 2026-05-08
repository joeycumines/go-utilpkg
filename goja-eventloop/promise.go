package gojaeventloop

import (
	"fmt"

	"github.com/joeycumines/goja"
)

// nodePromiseCombinatorElementLimit is the greatest input cardinality accepted
// by Promise.all, Promise.allSettled, and Promise.any in Node.js v26.5.0.
// V8 starts reaction indices at one and rejects at index (1<<21)-1.
const nodePromiseCombinatorElementLimit = (1 << 21) - 2

func (a *Adapter) bindNativePromiseExtensions(promise *goja.Object) error {
	return a.bindNativePromiseExtensionsLimit(promise, nodePromiseCombinatorElementLimit)
}

// bindNativePromiseExtensionsLimit is the pre-Bind seam for exercising the
// combinator boundary without allocating millions of element reactions.
func (a *Adapter) bindNativePromiseExtensionsLimit(promise *goja.Object, elementLimit int) error {
	if promise == nil {
		return fmt.Errorf("failed to bind native Promise extensions: Promise is unavailable")
	}
	if elementLimit < 1 {
		return fmt.Errorf("failed to bind native Promise extensions: element limit must be positive")
	}
	reflectApply, err := runtimeIntrinsic(a.runtime, goja.IntrinsicReflectApply, "Reflect.apply")
	if err != nil {
		return err
	}
	aggregateError, err := runtimeIntrinsic(a.runtime, goja.IntrinsicAggregateErrorConstructor, "AggregateError")
	if err != nil {
		return err
	}
	rangeError, err := runtimeIntrinsic(a.runtime, goja.IntrinsicRangeErrorConstructor, "RangeError")
	if err != nil {
		return err
	}
	typeError, err := runtimeIntrinsic(a.runtime, goja.IntrinsicTypeErrorConstructor, "TypeError")
	if err != nil {
		return err
	}
	defineProperty, err := runtimeIntrinsic(a.runtime, goja.IntrinsicObjectDefineProperty, "Object.defineProperty")
	if err != nil {
		return err
	}
	symbolToString, err := runtimeIntrinsic(a.runtime, goja.IntrinsicSymbolToString, "Symbol.prototype.toString")
	if err != nil {
		return err
	}
	factoryValue, err := a.runtime.RunString(`
		(function (
			Promise,
			isConstructor,
			notConstructorDescription,
			elementLimit,
			reflectApply,
			NativeAggregateError,
			NativeRangeError,
			NativeTypeError,
			objectDefineProperty,
			iteratorSymbol,
			symbolToString
		) {
			"use strict";
			function constructorTypeError(value) {
				return new NativeTypeError(notConstructorDescription(value) + " is not a constructor");
			}
			function nodeValueDescription(value) {
				let received = typeof value;
				if (value === null) received = "object null";
				else if (value === undefined) received = "undefined";
				else if (received === "number" || received === "boolean") received += " " + value;
				else if (received === "bigint") received = "bigint";
				else if (received === "string") received += " \"" + value + "\"";
				return received;
			}
			function notFunctionTypeError(value) {
				return new NativeTypeError(nodeValueDescription(value) + " is not a function");
			}
			function defineDataProperty(target, key, value) {
				objectDefineProperty(target, key, {
					__proto__: null,
					configurable: true,
					enumerable: true,
					writable: true,
					value,
				});
			}
			function argumentList() {
				const result = [];
				for (let index = 0; index < arguments.length; index++) {
					defineDataProperty(result, index, arguments[index]);
				}
				return result;
			}
			const emptyArgumentList = [];
			function promiseThen(nextPromise) {
				if (nextPromise === undefined) {
					throw new NativeTypeError("Cannot read properties of undefined (reading 'then')");
				}
				if (nextPromise === null) {
					throw new NativeTypeError("Cannot read properties of null (reading 'then')");
				}
				const then = nextPromise.then;
				if (typeof then !== "function") throw notFunctionTypeError(then);
				return then;
			}
			function iterableFacade(input) {
				return {
					[iteratorSymbol]() {
						if (input === null || input === undefined) {
							throw new NativeTypeError(nodeValueDescription(input) +
								" is not iterable (cannot read property Symbol(Symbol.iterator))");
						}
						const method = input[iteratorSymbol];
						if (typeof method !== "function") {
							throw new NativeTypeError(nodeValueDescription(input) +
								" is not iterable (cannot read property Symbol(Symbol.iterator))");
						}
						const iterator = reflectApply(method, input, emptyArgumentList);
						if (iterator === null || (typeof iterator !== "object" && typeof iterator !== "function")) {
							throw new NativeTypeError("Result of the Symbol.iterator method is not an object");
						}
						const next = iterator.next;
						if (typeof next !== "function") throw notFunctionTypeError(next);
						const adapter = {
							next() {
								const result = reflectApply(next, iterator, emptyArgumentList);
								if (result === null || (typeof result !== "object" && typeof result !== "function")) {
									const description = typeof result === "symbol"
										? reflectApply(symbolToString, result, emptyArgumentList)
										: "" + result;
									throw new NativeTypeError("Iterator result " + description + " is not an object");
								}
								return result;
							},
						};
						objectDefineProperty(adapter, "return", {
							__proto__: null,
							configurable: true,
							get() {
								const iteratorReturn = iterator.return;
								if (iteratorReturn === undefined) return undefined;
								if (typeof iteratorReturn !== "function") {
									return function () {
										throw new NativeTypeError("The iterator's 'return' method is not callable");
									};
								}
								return function () { return reflectApply(iteratorReturn, iterator, emptyArgumentList); };
							},
						});
						return adapter;
					},
				};
			}
			function internalListIterable(values) {
				return {
					[iteratorSymbol]() {
						let index = 0;
						return {
							next() {
								if (index >= values.length) return { done: true };
								return { done: false, value: values[index++] };
							},
						};
					},
				};
			}
			function promiseAnyAggregateError(errors) {
				const err = new NativeAggregateError(
					internalListIterable(errors),
					"All promises were rejected",
				);
				objectDefineProperty(err, "message", {
					__proto__: null,
					configurable: true,
					writable: true,
					value: "All promises were rejected",
				});
				return err;
			}
			function newPromiseCapability(C, method) {
				if (C === null || (typeof C !== "object" && typeof C !== "function")) {
					throw new NativeTypeError("Promise." + method + " called on non-object");
				}
				if (!isConstructor(C)) throw constructorTypeError(C);
				let resolve, reject;
				const promise = new C((res, rej) => {
					if (resolve !== undefined || reject !== undefined) {
						throw new NativeTypeError("Promise executor has already been invoked with non-undefined arguments");
					}
					resolve = res;
					reject = rej;
				});
				if (typeof resolve !== "function" || typeof reject !== "function") {
					throw new NativeTypeError("Promise resolve or reject function is not callable");
				}
				return { promise, resolve, reject };
			}
			const wrappedAll = ({ all(iterable) {
				const C = this;
				const capability = newPromiseCapability(C, "all");
				let promiseResolve;
				try {
					promiseResolve = C.resolve;
					if (typeof promiseResolve !== "function") throw new NativeTypeError("resolve is not a function");
				} catch (err) {
					reflectApply(capability.reject, undefined, argumentList(err));
					return capability.promise;
				}
				const values = [];
				let remaining = 1;
				let index = 0;
				try {
					for (const nextValue of iterableFacade(iterable)) {
						if (index >= elementLimit) {
							throw new NativeRangeError("Too many elements passed to Promise.all");
						}
						const currentIndex = index++;
						defineDataProperty(values, currentIndex, undefined);
						const nextPromise = reflectApply(promiseResolve, C, argumentList(nextValue));
						const then = promiseThen(nextPromise);
						let alreadyCalled = false;
						remaining++;
						reflectApply(then, nextPromise, argumentList(
							(value) => {
								if (alreadyCalled) return;
								alreadyCalled = true;
								defineDataProperty(values, currentIndex, value);
								remaining--;
								if (remaining === 0) reflectApply(capability.resolve, undefined, argumentList(values));
							},
							capability.reject,
						));
					}
				} catch (err) {
					reflectApply(capability.reject, undefined, argumentList(err));
					return capability.promise;
				}
				try {
					remaining--;
					if (remaining === 0) reflectApply(capability.resolve, undefined, argumentList(values));
				} catch (err) {
					reflectApply(capability.reject, undefined, argumentList(err));
				}
				return capability.promise;
			} }).all;
			const wrappedRace = ({ race(iterable) {
				const C = this;
				const capability = newPromiseCapability(C, "race");
				let promiseResolve;
				try {
					promiseResolve = C.resolve;
					if (typeof promiseResolve !== "function") throw new NativeTypeError("resolve is not a function");
					for (const nextValue of iterableFacade(iterable)) {
						const nextPromise = reflectApply(promiseResolve, C, argumentList(nextValue));
						const then = promiseThen(nextPromise);
						reflectApply(then, nextPromise, argumentList(capability.resolve, capability.reject));
					}
				} catch (err) {
					reflectApply(capability.reject, undefined, argumentList(err));
				}
				return capability.promise;
			} }).race;
			const wrappedAllSettled = ({ allSettled(iterable) {
				const C = this;
				const capability = newPromiseCapability(C, "allSettled");
				let promiseResolve;
				try {
					promiseResolve = C.resolve;
					if (typeof promiseResolve !== "function") throw new NativeTypeError("resolve is not a function");
				} catch (err) {
					reflectApply(capability.reject, undefined, argumentList(err));
					return capability.promise;
				}
				const values = [];
				let remaining = 1;
				let index = 0;
				try {
					for (const nextValue of iterableFacade(iterable)) {
						if (index >= elementLimit) {
							throw new NativeRangeError("Too many elements passed to Promise.all");
						}
						const currentIndex = index++;
						defineDataProperty(values, currentIndex, undefined);
						const nextPromise = reflectApply(promiseResolve, C, argumentList(nextValue));
						const then = promiseThen(nextPromise);
						let alreadyCalled = false;
						remaining++;
						const settle = function (status, key, value) {
							if (alreadyCalled) return;
							alreadyCalled = true;
							const result = { status };
							defineDataProperty(result, key, value);
							defineDataProperty(values, currentIndex, result);
							remaining--;
							if (remaining === 0) reflectApply(capability.resolve, undefined, argumentList(values));
						};
						reflectApply(then, nextPromise, argumentList(
							(value) => { settle("fulfilled", "value", value); },
							(reason) => { settle("rejected", "reason", reason); },
						));
					}
				} catch (err) {
					reflectApply(capability.reject, undefined, argumentList(err));
					return capability.promise;
				}
				try {
					remaining--;
					if (remaining === 0) reflectApply(capability.resolve, undefined, argumentList(values));
				} catch (err) {
					reflectApply(capability.reject, undefined, argumentList(err));
				}
				return capability.promise;
			} }).allSettled;
			const wrappedAny = ({ any(iterable) {
					const C = this;
					const capability = newPromiseCapability(C, "any");
					let promiseResolve;
					try {
						promiseResolve = C.resolve;
						if (typeof promiseResolve !== "function") throw new NativeTypeError("resolve is not a function");
					} catch (err) {
						reflectApply(capability.reject, undefined, argumentList(err));
						return capability.promise;
					}
					const errors = [];
					let remaining = 1;
					let index = 0;
					try {
						for (const nextValue of iterableFacade(iterable)) {
							if (index >= elementLimit) {
								throw new NativeRangeError("Too many elements passed to Promise.any");
							}
							const currentIndex = index;
							index++;
							defineDataProperty(errors, currentIndex, undefined);
							const nextPromise = reflectApply(promiseResolve, C, argumentList(nextValue));
							const then = promiseThen(nextPromise);
							let alreadyCalled = false;
							remaining++;
							reflectApply(then, nextPromise, argumentList(
									capability.resolve,
									(reason) => {
										if (alreadyCalled) return;
										alreadyCalled = true;
										defineDataProperty(errors, currentIndex, reason);
										remaining--;
										if (remaining === 0) reflectApply(capability.reject, undefined, argumentList(promiseAnyAggregateError(errors)));
									},
								));
						}
					} catch (err) {
						reflectApply(capability.reject, undefined, argumentList(err));
						return capability.promise;
					}
					remaining--;
					if (remaining === 0) reflectApply(capability.reject, undefined, argumentList(promiseAnyAggregateError(errors)));
					return capability.promise;
			} }).any;
			objectDefineProperty(Promise, "all", { configurable: true, writable: true, value: wrappedAll });
			objectDefineProperty(Promise, "race", { configurable: true, writable: true, value: wrappedRace });
			objectDefineProperty(Promise, "allSettled", { configurable: true, writable: true, value: wrappedAllSettled });
			objectDefineProperty(Promise, "any", { configurable: true, writable: true, value: wrappedAny });
			if (typeof Promise.withResolvers !== "function") {
				const withResolvers = ({ withResolvers() {
					return newPromiseCapability(this, "withResolvers");
				} }).withResolvers;
				objectDefineProperty(Promise, "withResolvers", {
					configurable: true,
					writable: true,
					value: withResolvers,
				});
			}
			if (typeof Promise.try !== "function") {
				const promiseTry = ({ try(fn) {
					const C = this;
					const args = [];
					for (let i = 1; i < arguments.length; i++) {
						defineDataProperty(args, i - 1, arguments[i]);
					}
					const capability = newPromiseCapability(C, "try");
					if (typeof fn !== "function") {
						reflectApply(capability.reject, undefined, argumentList(notFunctionTypeError(fn)));
						return capability.promise;
					}
					let result;
					try {
						result = reflectApply(fn, undefined, args);
					} catch (err) {
						reflectApply(capability.reject, undefined, argumentList(err));
						return capability.promise;
					}
					reflectApply(capability.resolve, undefined, argumentList(result));
					return capability.promise;
				} }).try;
				objectDefineProperty(Promise, "try", {
					configurable: true,
					writable: true,
					value: promiseTry,
				});
			}
		})
	`)
	if err != nil {
		return wrapRuntimeError("compile native Promise extensions", err)
	}
	factory, ok := goja.AssertFunction(factoryValue)
	if !ok {
		return fmt.Errorf("failed to bind native Promise extensions: factory is not callable")
	}
	isConstructor := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		_, ok := goja.AssertConstructor(call.Argument(0))
		return a.runtime.ToValue(ok)
	})
	notConstructorDescription := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0)
		if _, ok := goja.AssertFunction(value); ok {
			return a.runtime.ToValue(value.String())
		}
		object, ok := value.(*goja.Object)
		if !ok || object == nil {
			return a.runtime.ToValue(value.String())
		}
		if object.Prototype() == nil {
			return a.runtime.ToValue("[object Object]")
		}
		class := object.ClassName()
		if class == "" || class == "Object" {
			return a.runtime.ToValue("#<Object>")
		}
		return a.runtime.ToValue("[object " + class + "]")
	})
	if _, err := factory(
		goja.Undefined(),
		promise,
		isConstructor,
		notConstructorDescription,
		a.runtime.ToValue(elementLimit),
		reflectApply,
		aggregateError,
		rangeError,
		typeError,
		defineProperty,
		goja.SymIterator,
		symbolToString,
	); err != nil {
		return wrapRuntimeError("bind native Promise extensions", err)
	}
	return nil
}

func (a *Adapter) ensureDisposeSymbol() (*goja.Symbol, error) {
	if a.disposeSymbol != nil {
		return a.disposeSymbol, nil
	}
	symbolObject, err := runtimeIntrinsicObject(a.runtime, goja.IntrinsicSymbolConstructor, "Symbol")
	if err != nil {
		return nil, err
	}
	value := symbolObject.Get("dispose")
	if value != nil && !goja.IsUndefined(value) {
		symbol, ok := value.(*goja.Symbol)
		if !ok {
			return nil, fmt.Errorf("symbol.dispose is not a symbol")
		}
		a.disposeSymbol = symbol
		return symbol, nil
	}
	symbol := goja.NewSymbol("Symbol.dispose")
	a.disposeSymbol = symbol
	return symbol, nil
}

// consumeIterable converts an iterable Goja value to a slice of values.
// Uses the ECMAScript iterator protocol for every input, including arrays, so
// observable custom iterators and abrupt completions retain their semantics.
// Returns an error if the value is not iterable.
func (a *Adapter) consumeIterable(iterable goja.Value) ([]goja.Value, error) {
	var values []goja.Value
	err := a.iterateValues(iterable, func(_ int, value goja.Value) error {
		values = append(values, value)
		return nil
	})
	return values, err
}

func (a *Adapter) iterateValues(iterable goja.Value, visit func(int, goja.Value) error) error {
	// 1. Handle null/undefined early
	if iterable == nil || goja.IsNull(iterable) || goja.IsUndefined(iterable) {
		return fmt.Errorf("cannot consume null or undefined as iterable")
	}

	// Use our JS helper to get the iterator method (handles Symbol lookup)
	iteratorMethodVal, err := a.getIterator(goja.Undefined(), iterable)
	if err != nil {
		return err
	}

	if iteratorMethodVal == nil || goja.IsUndefined(iteratorMethodVal) {
		// Not an iterable (no Symbol.iterator method)
		return fmt.Errorf("object is not iterable (cannot get Symbol.iterator)")
	}

	iteratorMethodCallable, ok := goja.AssertFunction(iteratorMethodVal)
	if !ok {
		return fmt.Errorf("symbol.iterator is not a function")
	}

	// Call [Symbol.iterator]() to get the iterator object
	iteratorVal, err := iteratorMethodCallable(iterable)
	if err != nil {
		return err
	}
	iteratorObj, ok := iteratorVal.(*goja.Object)
	if !ok || iteratorObj == nil {
		return fmt.Errorf("symbol.iterator returned a non-object")
	}

	// Get the next() method from the iterator
	nextMethod := iteratorObj.Get("next")
	nextMethodCallable, ok := goja.AssertFunction(nextMethod)
	if !ok {
		return fmt.Errorf("iterator.next is not a function")
	}

	index := 0
	for {
		// Call iterator.next()
		nextResult, err := nextMethodCallable(iteratorObj)
		if err != nil {
			return err
		}
		nextResultObj, ok := nextResult.(*goja.Object)
		if !ok || nextResultObj == nil {
			return fmt.Errorf("iterator.next returned a non-object")
		}

		// Check done property
		done := nextResultObj.Get("done")
		if done != nil && done.ToBoolean() {
			break
		}

		// Get value property
		value := nextResultObj.Get("value")
		if err := visit(index, value); err != nil {
			return err
		}
		index++
	}
	return nil
}
