// Copyright 2025 Joseph Cumines
//
// goja-eventloop: Goja adapter for the event loop library
//
// This binds eventloop.JS functionality to Goja JavaScript runtime.

package gojaeventloop

import (
	"fmt"
	"strconv"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// Adapter bridges Goja runtime to goeventloop.JS.
// This allows setTimeout/setInterval/queueMicrotask/Promise to work with Goja.
type Adapter struct {
	js               *goeventloop.JS
	runtime          *goja.Runtime
	loop             *goeventloop.Loop
	promisePrototype *goja.Object // CRITICAL #3: Promise.prototype for instanceof support
}

// New creates a new Goja adapter for given event loop and runtime.
func New(loop *goeventloop.Loop, runtime *goja.Runtime) (*Adapter, error) {
	if loop == nil {
		return nil, fmt.Errorf("loop cannot be nil")
	}
	if runtime == nil {
		return nil, fmt.Errorf("runtime cannot be nil")
	}

	js, err := goeventloop.NewJS(loop)
	if err != nil {
		return nil, fmt.Errorf("failed to create JS adapter: %w", err)
	}

	return &Adapter{
		js:      js,
		runtime: runtime,
		loop:    loop,
	}, nil
}

// Loop returns the event loop
func (a *Adapter) Loop() *goeventloop.Loop {
	return a.loop
}

// Runtime returns the Goja runtime
func (a *Adapter) Runtime() *goja.Runtime {
	return a.runtime
}

// JS returns the JS adapter
func (a *Adapter) JS() *goeventloop.JS {
	return a.js
}

// Bind creates setTimeout/setInterval/queueMicrotask bindings in Goja global scope.
func (a *Adapter) Bind() error {
	// Set setTimeout
	if err := a.runtime.Set("setTimeout", a.setTimeout); err != nil {
		return fmt.Errorf("failed to bind setTimeout: %w", err)
	}

	// Set clearTimeout
	if err := a.runtime.Set("clearTimeout", a.clearTimeout); err != nil {
		return fmt.Errorf("failed to bind clearTimeout: %w", err)
	}

	// Set setInterval
	if err := a.runtime.Set("setInterval", a.setInterval); err != nil {
		return fmt.Errorf("failed to bind setInterval: %w", err)
	}

	// Set clearInterval
	if err := a.runtime.Set("clearInterval", a.clearInterval); err != nil {
		return fmt.Errorf("failed to bind clearInterval: %w", err)
	}

	// Set queueMicrotask
	if err := a.runtime.Set("queueMicrotask", a.queueMicrotask); err != nil {
		return fmt.Errorf("failed to bind queueMicrotask: %w", err)
	}

	// Bind Promise constructor - must be set as a callable, not a property
	promiseConstructor := a.runtime.ToValue(a.promiseConstructor)
	if err := a.runtime.GlobalObject().Set("Promise", promiseConstructor); err != nil {
		return fmt.Errorf("failed to bind Promise: %w", err)
	}

	// CRITICAL #1: Bind Promise constructor with combinators, CRITICAL #3: Set up prototype
	if err := a.bindPromise(); err != nil {
		return fmt.Errorf("failed to bind Promise: %w", err)
	}

	return nil
}

// setTimeout binding for Goja
func (a *Adapter) setTimeout(call goja.FunctionCall) goja.Value {
	fn := call.Argument(0)
	if fn.Export() == nil {
		panic(a.runtime.NewTypeError("setTimeout requires a function as first argument"))
	}

	fnCallable, ok := goja.AssertFunction(fn)
	if !ok {
		panic(a.runtime.NewTypeError("setTimeout requires a function as first argument"))
	}

	delayMs := int(call.Argument(1).ToInteger())
	if delayMs < 0 {
		panic(a.runtime.NewTypeError("delay cannot be negative"))
	}

	id, err := a.js.SetTimeout(func() {
		_, _ = fnCallable(goja.Undefined())
	}, delayMs)
	if err != nil {
		panic(a.runtime.NewGoError(err))
	}

	return a.runtime.ToValue(id)
}

// clearTimeout binding for Goja
func (a *Adapter) clearTimeout(call goja.FunctionCall) goja.Value {
	id := uint64(call.Argument(0).ToInteger())
	_ = a.js.ClearTimeout(id) // Silently ignore if timer not found (matches browser behavior)
	return goja.Undefined()
}

// setInterval binding for Goja
func (a *Adapter) setInterval(call goja.FunctionCall) goja.Value {
	fn := call.Argument(0)
	if fn.Export() == nil {
		panic(a.runtime.NewTypeError("setInterval requires a function as first argument"))
	}

	fnCallable, ok := goja.AssertFunction(fn)
	if !ok {
		panic(a.runtime.NewTypeError("setInterval requires a function as first argument"))
	}

	delayMs := int(call.Argument(1).ToInteger())
	if delayMs < 0 {
		panic(a.runtime.NewTypeError("delay cannot be negative"))
	}

	id, err := a.js.SetInterval(func() {
		_, _ = fnCallable(goja.Undefined())
	}, delayMs)
	if err != nil {
		panic(a.runtime.NewGoError(err))
	}

	return a.runtime.ToValue(id)
}

// clearInterval binding for Goja
func (a *Adapter) clearInterval(call goja.FunctionCall) goja.Value {
	id := uint64(call.Argument(0).ToInteger())
	_ = a.js.ClearInterval(id) // Silently ignore if timer not found (matches browser behavior)
	return goja.Undefined()
}

// queueMicrotask binding for Goja
func (a *Adapter) queueMicrotask(call goja.FunctionCall) goja.Value {
	fn := call.Argument(0)
	if fn.Export() == nil {
		panic(a.runtime.NewTypeError("queueMicrotask requires a function as first argument"))
	}

	fnCallable, ok := goja.AssertFunction(fn)
	if !ok {
		panic(a.runtime.NewTypeError("queueMicrotask requires a function as first argument"))
	}

	err := a.js.QueueMicrotask(func() {
		_, _ = fnCallable(goja.Undefined())
	})
	if err != nil {
		panic(a.runtime.NewGoError(err))
	}

	return goja.Undefined()
}

// promiseConstructor binding for Goja
func (a *Adapter) promiseConstructor(call goja.ConstructorCall) *goja.Object {
	// CRITICAL #4: Validate executor FIRST before creating promise to prevent resource leaks
	executor := call.Argument(0)
	if executor.Export() == nil {
		panic(a.runtime.NewTypeError("Promise executor must be a function"))
	}

	executorCallable, ok := goja.AssertFunction(executor)
	if !ok {
		panic(a.runtime.NewTypeError("Promise executor must be a function"))
	}

	// Only create promise after validation to prevent resource leaks
	promise, resolve, reject := a.js.NewChainedPromise()

	_, err := executorCallable(goja.Undefined(),
		a.runtime.ToValue(func(result goja.Value) {
			resolve(result.Export())
		}),
		a.runtime.ToValue(func(reason goja.Value) {
			reject(reason.Export())
		}),
	)
	if err != nil {
		// If executor throws, reject the promise
		reject(err)
	}

	// Get the object that Goja created for 'new Promise()'
	thisObj := call.This

	// Set prototype (for instanceof support)
	if a.promisePrototype != nil {
		thisObj.SetPrototype(a.promisePrototype)
	}

	// Store internal promise and set methods directly on instance
	a.setPromiseMethods(thisObj, promise)

	return thisObj
}

// gojaFuncToHandler converts a Goja function value to a promise handler
func (a *Adapter) gojaFuncToHandler(fn goja.Value) func(goeventloop.Result) goeventloop.Result {
	if fn.Export() == nil {
		// No handler provided - pass through the result
		return func(result goeventloop.Result) goeventloop.Result { return result }
	}

	fnCallable, ok := goja.AssertFunction(fn)
	if !ok {
		// Not a function - pass through the result
		return func(result goeventloop.Result) goeventloop.Result { return result }
	}

	return func(result goeventloop.Result) goeventloop.Result {
		ret, err := fnCallable(goja.Undefined(), a.runtime.ToValue(result))
		if err != nil {
			// CRITICAL #2: Return rejection result instead of panicking
			return err
		}
		return ret.Export()
	}
}

// gojaVoidFuncToHandler converts a Goja function value to a void callback (for Promise.prototype.finally)
func (a *Adapter) gojaVoidFuncToHandler(fn goja.Value) func() {
	if fn.Export() == nil {
		// No handler provided - no-op
		return func() {}
	}

	fnCallable, ok := goja.AssertFunction(fn)
	if !ok {
		// Not a function - no-op
		return func() {}
	}

	return func() {
		_, _ = fnCallable(goja.Undefined())
	}
}

// setPromiseMethods sets then/catch/finally methods directly on a promise object (shared helper)
func (a *Adapter) setPromiseMethods(obj *goja.Object, promise *goeventloop.ChainedPromise) {
	// Store internal promise for method access
	obj.Set("_internalPromise", promise)

	// DEBUG: Mark that we're setting methods
	obj.Set("__DEBUG_SET_PROMISE_METHODS_CALLED", true)

	thenFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		// Extract promise from _internalPromise property
		thisVal := call.This
		thisObj, ok := thisVal.(*goja.Object)
		if !ok || thisObj == nil {
			panic(a.runtime.NewTypeError("then() called on non-Promise object"))
		}
		internalVal := thisObj.Get("_internalPromise")
		p, ok := internalVal.Export().(*goeventloop.ChainedPromise)
		if !ok || p == nil {
			panic(a.runtime.NewTypeError("then() called on non-Promise object"))
		}

		onFulfilled := a.gojaFuncToHandler(call.Argument(0))
		onRejected := a.gojaFuncToHandler(call.Argument(1))
		chained := p.Then(onFulfilled, onRejected)
		result := a.gojaWrapPromise(chained)

		// DEBUG: Check what we're returning
		resultObj := result.ToObject(a.runtime)
		resultObj.Set("__DEBUG_CAME_FROM_THEN", true)

		// DEBUG: Set a property to verify this function was actually called
		resultObj.Set("__DEBUG_THEN_WAS_CALLED", true)

		return result
	})

	catchFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		// Extract promise from _internalPromise property
		thisVal := call.This
		thisObj, ok := thisVal.(*goja.Object)
		if !ok || thisObj == nil {
			panic(a.runtime.NewTypeError("catch() called on non-Promise object"))
		}
		internalVal := thisObj.Get("_internalPromise")
		p, ok := internalVal.Export().(*goeventloop.ChainedPromise)
		if !ok || p == nil {
			panic(a.runtime.NewTypeError("catch() called on non-Promise object"))
		}

		onRejected := a.gojaFuncToHandler(call.Argument(0))
		chained := p.Catch(onRejected)
		return a.gojaWrapPromise(chained)
	})

	finallyFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		// Extract promise from _internalPromise property
		thisVal := call.This
		thisObj, ok := thisVal.(*goja.Object)
		if !ok || thisObj == nil {
			panic(a.runtime.NewTypeError("finally() called on non-Promise object"))
		}
		internalVal := thisObj.Get("_internalPromise")
		p, ok := internalVal.Export().(*goeventloop.ChainedPromise)
		if !ok || p == nil {
			panic(a.runtime.NewTypeError("finally() called on non-Promise object"))
		}

		onFinally := a.gojaVoidFuncToHandler(call.Argument(0))
		chained := p.Finally(onFinally)
		return a.gojaWrapPromise(chained)
	})

	obj.Set("then", thenFn)
	obj.Set("catch", catchFn)
	obj.Set("finally", finallyFn)
}

// gojaWrapPromise wraps a ChainedPromise with then/catch/finally instance methods
func (a *Adapter) gojaWrapPromise(promise *goeventloop.ChainedPromise) goja.Value {
	// Create a NEW wrapper object instead of modifying Goja's cached wrapper
	wrapper := a.runtime.NewObject()

	// Store promise as internal data
	wrapper.Set("_internalPromise", promise)

	// Set prototype
	if a.promisePrototype != nil {
		wrapper.SetPrototype(a.promisePrototype)
	}

	// Set methods directly on instance using shared helper
	a.setPromiseMethods(wrapper, promise)

	// Return the wrapper object as a Goja value
	return wrapper
}

func NewChainedPromise(loop *goeventloop.Loop, runtime *goja.Runtime) (*goeventloop.ChainedPromise, goja.Value, goja.Value) {
	js, err := goeventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}

	promise, resolve, reject := js.NewChainedPromise()

	resolveVal := runtime.ToValue(func(result goja.Value) {
		resolve(result.Export())
	})

	rejectVal := runtime.ToValue(func(reason goja.Value) {
		reject(reason.Export())
	})

	return promise, resolveVal, rejectVal
}

// bindPromise sets up the Promise constructor and all static combinators
func (a *Adapter) bindPromise() error {
	// Create Promise prototype
	promisePrototype := a.runtime.NewObject()
	a.promisePrototype = promisePrototype

	// Get the Promise constructor and ensure it has the prototype
	promiseConstructorVal := a.runtime.Get("Promise")
	promiseConstructorObj := promiseConstructorVal.ToObject(a.runtime)

	// Set the prototype on the constructor (for `Promise.prototype` property)
	promiseConstructorObj.Set("prototype", promisePrototype)

	// Promise.resolve(value)
	promiseConstructorObj.Set("resolve", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0)
		promise := a.js.Resolve(value.Export())
		return a.gojaWrapPromise(promise)
	}))

	// Promise.reject(reason)
	promiseConstructorObj.Set("reject", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		reason := call.Argument(0)
		promise := a.js.Reject(reason.Export())
		return a.gojaWrapPromise(promise)
	}))

	// Promise.all(iterable) - with BONUS input validation
	promiseConstructorObj.Set("all", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		iterable := call.Argument(0)
		if goja.IsNull(iterable) || goja.IsUndefined(iterable) {
			promise := a.js.Resolve([]goeventloop.Result{})
			return a.gojaWrapPromise(promise)
		}

		arr, ok := iterable.Export().([]goja.Value)
		if !ok {
			// Try to convert to array-like object
			obj := iterable.ToObject(a.runtime)
			if obj == nil {
				panic(a.runtime.NewTypeError("Promise.all requires an iterable"))
			}
			lengthVal := obj.Get("length")
			if lengthVal == nil {
				panic(a.runtime.NewTypeError("Promise.all requires an iterable"))
			}
			length := int(lengthVal.ToInteger())
			arr = make([]goja.Value, length)
			for i := 0; i < length; i++ {
				arr[i] = obj.Get(strconv.Itoa(i))
			}
		}

		// BONUS: Validate all elements are thenable (have .then method)
		for i, val := range arr {
			if goja.IsNull(val) || goja.IsUndefined(val) {
				continue // Primitive values are auto-resolved
			}
			obj := val.ToObject(a.runtime)
			if obj != nil {
				thenVal := obj.Get("then")
				if !goja.IsUndefined(thenVal) && !goja.IsNull(thenVal) {
					_, isFunc := goja.AssertFunction(thenVal)
					if !isFunc {
						panic(a.runtime.NewTypeError(fmt.Sprintf("Promise.all element %d is not thenable", i)))
					}
				}
			}
		}

		// Convert to ChainedPromise array
		promises := make([]*goeventloop.ChainedPromise, len(arr))
		for i, val := range arr {
			promises[i] = a.js.Resolve(val.Export())
		}

		promise := a.js.All(promises)
		return a.gojaWrapPromise(promise)
	}))

	// Promise.race(iterable) - with BONUS input validation
	promiseConstructorObj.Set("race", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		iterable := call.Argument(0)
		if goja.IsNull(iterable) || goja.IsUndefined(iterable) {
			// Empty iterable returns pending promise
			promise, resolve, _ := a.js.NewChainedPromise()
			_ = resolve // Will never resolve
			return a.gojaWrapPromise(promise)
		}

		arr, ok := iterable.Export().([]goja.Value)
		if !ok {
			// Try to convert to array-like object
			obj := iterable.ToObject(a.runtime)
			if obj == nil {
				panic(a.runtime.NewTypeError("Promise.race requires an iterable"))
			}
			lengthVal := obj.Get("length")
			if lengthVal == nil {
				panic(a.runtime.NewTypeError("Promise.race requires an iterable"))
			}
			length := int(lengthVal.ToInteger())
			arr = make([]goja.Value, length)
			for i := 0; i < length; i++ {
				arr[i] = obj.Get(strconv.Itoa(i))
			}
		}

		// BONUS: Validate all elements are thenable
		for i, val := range arr {
			if goja.IsNull(val) || goja.IsUndefined(val) {
				continue // Primitive values are auto-resolved
			}
			obj := val.ToObject(a.runtime)
			if obj != nil {
				thenVal := obj.Get("then")
				if !goja.IsUndefined(thenVal) && !goja.IsNull(thenVal) {
					_, isFunc := goja.AssertFunction(thenVal)
					if !isFunc {
						panic(a.runtime.NewTypeError(fmt.Sprintf("Promise.race element %d is not thenable", i)))
					}
				}
			}
		}

		// Convert to ChainedPromise array
		promises := make([]*goeventloop.ChainedPromise, len(arr))
		for i, val := range arr {
			promises[i] = a.js.Resolve(val.Export())
		}

		promise := a.js.Race(promises)
		return a.gojaWrapPromise(promise)
	}))

	// Promise.allSettled(iterable) - with BONUS input validation
	promiseConstructorObj.Set("allSettled", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		iterable := call.Argument(0)
		if goja.IsNull(iterable) || goja.IsUndefined(iterable) {
			promise := a.js.Resolve([]goeventloop.Result{})
			return a.gojaWrapPromise(promise)
		}

		arr, ok := iterable.Export().([]goja.Value)
		if !ok {
			// Try to convert to array-like object
			obj := iterable.ToObject(a.runtime)
			if obj == nil {
				panic(a.runtime.NewTypeError("Promise.allSettled requires an iterable"))
			}
			lengthVal := obj.Get("length")
			if lengthVal == nil {
				panic(a.runtime.NewTypeError("Promise.allSettled requires an iterable"))
			}
			length := int(lengthVal.ToInteger())
			arr = make([]goja.Value, length)
			for i := 0; i < length; i++ {
				arr[i] = obj.Get(strconv.Itoa(i))
			}
		}

		// BONUS: Validate all elements are thenable
		for i, val := range arr {
			if goja.IsNull(val) || goja.IsUndefined(val) {
				continue // Primitive values are auto-resolved
			}
			obj := val.ToObject(a.runtime)
			if obj != nil {
				thenVal := obj.Get("then")
				if !goja.IsUndefined(thenVal) && !goja.IsNull(thenVal) {
					_, isFunc := goja.AssertFunction(thenVal)
					if !isFunc {
						panic(a.runtime.NewTypeError(fmt.Sprintf("Promise.allSettled element %d is not thenable", i)))
					}
				}
			}
		}

		// Convert to ChainedPromise array
		promises := make([]*goeventloop.ChainedPromise, len(arr))
		for i, val := range arr {
			promises[i] = a.js.Resolve(val.Export())
		}

		promise := a.js.AllSettled(promises)
		return a.gojaWrapPromise(promise)
	}))

	// Promise.any(iterable) - with BONUS input validation
	promiseConstructorObj.Set("any", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		iterable := call.Argument(0)
		if goja.IsNull(iterable) || goja.IsUndefined(iterable) {
			panic(a.runtime.NewTypeError("Promise.any requires an iterable"))
		}

		arr, ok := iterable.Export().([]goja.Value)
		if !ok {
			// Try to convert to array-like object
			obj := iterable.ToObject(a.runtime)
			if obj == nil {
				panic(a.runtime.NewTypeError("Promise.any requires an iterable"))
			}
			lengthVal := obj.Get("length")
			if lengthVal == nil {
				panic(a.runtime.NewTypeError("Promise.any requires an iterable"))
			}
			length := int(lengthVal.ToInteger())
			arr = make([]goja.Value, length)
			for i := 0; i < length; i++ {
				arr[i] = obj.Get(strconv.Itoa(i))
			}
		}

		if len(arr) == 0 {
			panic(a.runtime.NewTypeError("Promise.any requires at least one element"))
		}

		// BONUS: Validate all elements are thenable
		for i, val := range arr {
			if goja.IsNull(val) || goja.IsUndefined(val) {
				continue // Primitive values are auto-resolved
			}
			obj := val.ToObject(a.runtime)
			if obj != nil {
				thenVal := obj.Get("then")
				if !goja.IsUndefined(thenVal) && !goja.IsNull(thenVal) {
					_, isFunc := goja.AssertFunction(thenVal)
					if !isFunc {
						panic(a.runtime.NewTypeError(fmt.Sprintf("Promise.any element %d is not thenable", i)))
					}
				}
			}
		}

		// Convert to ChainedPromise array
		promises := make([]*goeventloop.ChainedPromise, len(arr))
		for i, val := range arr {
			promises[i] = a.js.Resolve(val.Export())
		}

		promise := a.js.Any(promises)
		return a.gojaWrapPromise(promise)
	}))

	return nil
}

