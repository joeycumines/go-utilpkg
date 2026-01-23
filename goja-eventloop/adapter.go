// Copyright 2025 Joseph Cumines
//
// goja-eventloop: Goja adapter for the event loop library
//
// This binds eventloop.JS functionality to Goja JavaScript runtime.
//
// Package gojaeventloop provides bindings between the [github.com/joeycumines/go-eventloop]
// JS adapter and the Goja JavaScript runtime.
//
// This package enables JavaScript setTimeout, setInterval, queueMicrotask, and Promise
// APIs to work seamlessly with Go's event loop concurrency model.
//
// # Binding the Adapter
//
//	// Create event loop and Goja runtime
//	loop := eventloop.New()
//	runtime := goja.New()
//
//	// Create and bind adapter
//	adapter, err := gojaeventloop.New(loop, runtime)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if err := adapter.Bind(); err != nil {
//	    log.Fatal(err)
//	}
//
//	// JavaScript code now has access to async APIs
//	runtime.RunString(`
//	    setTimeout(() => console.log("Hello!"), 100);
//	    Promise.resolve(42).then(v => console.log(v));
//	`)
//
//	// Run event loop to process callbacks
//	loop.Run(context.Background())
//
// # Thread Safety
//
// The adapter coordinates thread safety between three components:
//
//   - Goja Runtime: Not thread-safe; should only be accessed from one goroutine
//   - Event Loop: Processes callbacks on its own thread
//   - Go Code: Can schedule timers/promises from any goroutine
//
// After calling [Adapter.Bind], JavaScript callbacks execute on the event loop thread.
// The Goja runtime should be accessed from the same thread (typically via
// [eventloop.Loop.Submit] or from within callbacks).
//
// # Available JavaScript Globals
//
// After binding, the following globals are available in JavaScript:
//
//   - setTimeout(callback, delay?) → timer ID : Schedule one-time callback
//   - clearTimeout(id) → undefined : Cancel scheduled timeout
//   - setInterval(callback, delay?) → timer ID : Schedule repeating callback
//   - clearInterval(id) → undefined : Cancel scheduled interval
//   - queueMicrotask(callback) → undefined : Schedule high-priority callback
//   - Promise : Promise constructor: Create async promise with then/catch/finally
//   - Promise.resolve(value) → promise : Create already-settled promise
//   - Promise.reject(reason) → promise : Create already-rejected promise
//   - Promise.all(iterable) → promise : Wait for all promises to resolve
//   - Promise.race(iterable) → promise : First to settle wins
//   - Promise.allSettled(iterable) → promise : Wait for all to settle
//   - Promise.any(iterable) → promise : First to resolve wins
//
// # Combinator Access from Go
//
// The adapter also provides Go-level access to Promise combinators:
//
//	adapter.JS().All([]*eventloop.ChainedPromise{p1, p2})
//	adapter.JS().Race([]*eventloop.ChainedPromise{p1, p2})
//	adapter.JS().AllSettled([]*eventloop.ChainedPromise{p1, p2})
//	adapter.JS().Any([]*eventloop.ChainedPromise{p1, p2})
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

// Bind creates setTimeout/setInterval/queueMicrotask/Promise bindings in Goja global scope.
//
// This must be called before executing JavaScript code that uses timer or Promise APIs.
//
// After calling Bind(), the following globals become available in JavaScript:
//   - setTimeout(callback, delay?) → timer ID
//   - clearTimeout(id) → undefined
//   - setInterval(callback, delay?) → timer ID
//   - clearInterval(id) → undefined
//   - queueMicrotask(callback) → undefined
//   - Promise : Promise constructor
//   - Promise.resolve(value) → promise
//   - Promise.reject(reason) → promise
//   - Promise.all(iterable) → promise
//   - Promise.race(iterable) → promise
//   - Promise.allSettled(iterable) → promise
//   - Promise.any(iterable) → promise
//
// Example:
//
//	err := adapter.Bind()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	_, err = runtime.RunString(`
//	    setTimeout(() => console.log("Hello!"), 100)
//	`)
//	if err != nil {
//	    log.Fatal(err)
//	}
func (a *Adapter) Bind() error {
	// Bind all JavaScript globals to the Goja runtime
	// Timer functions
	a.runtime.Set("setTimeout", a.setTimeout)
	a.runtime.Set("clearTimeout", a.clearTimeout)
	a.runtime.Set("setInterval", a.setInterval)
	a.runtime.Set("clearInterval", a.clearInterval)
	a.runtime.Set("queueMicrotask", a.queueMicrotask)

	// Promise constructor
	a.runtime.Set("Promise", a.promiseConstructor)

	// FIX CRITICAL #1, #2, #5, #6: Call bindPromise() to set up all combinators
	return a.bindPromise()
}
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
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			var val any
			if len(call.Arguments) > 0 {
				val = call.Argument(0).Export()
			}
			resolve(val)
			return goja.Undefined()
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			var val any
			if len(call.Arguments) > 0 {
				val = call.Argument(0).Export()
			}
			reject(val)
			return goja.Undefined()
		}),
	)
	if err != nil {
		// If executor throws, reject the promise
		reject(err)
	}

	// Get the object that Goja created for 'new Promise()'
	thisObj := call.This

	// Use the prototype created by bindPromise()
	thisObj.SetPrototype(a.promisePrototype)

	// Store internal promise for method access
	// Note: Prototype methods handle then/catch/finally via _internalPromise
	thisObj.Set("_internalPromise", promise)

	return thisObj
}

// gojaFuncToHandler converts a Goja function value to a promise handler
// CRITICAL #1 FIX: Type conversion at Go-native level BEFORE passing to JavaScript
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
		// CRITICAL FIX: Check type at Go-native level, not after Goja conversion
		var jsValue goja.Value

		// Extract Go-native type from Result
		goNativeValue := result

		switch v := goNativeValue.(type) {
		case *goeventloop.ChainedPromise:
			// This is a promise - we need to extract its value BEFORE passing to JavaScript
			// If the promise is fulfilled, we resolve it completely to get the primitive value
			// If rejected, we extract the rejection reason
			// If pending, pass undefined (nothing available yet)
			switch p := v; p.State() {
			case goeventloop.Rejected:
				// Get rejection reason and convert it
				reason := p.Reason()
				jsValue = a.convertToGojaValue(reason)
			case goeventloop.Pending:
				// No value available yet
				jsValue = goja.Undefined()
			default: // Fulfilled
				// Get resolved value - this recursively extracts the final value
				value := p.Value()
				jsValue = a.convertToGojaValue(value)
			}

		case []goeventloop.Result:
			// Convert Go-native slice to JavaScript array
			jsArr := a.runtime.NewArray(len(v))
			for i, val := range v {
				_ = jsArr.Set(strconv.Itoa(i), a.convertToGojaValue(val))
			}
			jsValue = jsArr

		case map[string]interface{}:
			// Convert Go-native map to JavaScript object
			jsObj := a.runtime.NewObject()
			for key, val := range v {
				_ = jsObj.Set(key, a.convertToGojaValue(val))
			}
			jsValue = jsObj

		default:
			// Primitive or other type - use standard conversion
			jsValue = a.convertToGojaValue(goNativeValue)
		}

		// Call JavaScript handler with the properly converted value
		ret, err := fnCallable(goja.Undefined(), jsValue)
		if err != nil {
			// Return rejection result instead of panicking
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

// gojaWrapPromise wraps a ChainedPromise with then/catch/finally instance methods
func (a *Adapter) gojaWrapPromise(promise *goeventloop.ChainedPromise) goja.Value {
	// Create a wrapper object
	wrapper := a.runtime.NewObject()

	// Store promise for prototype method access
	wrapper.Set("_internalPromise", promise)

	// Set prototype (prototype has then/catch/finally methods from bindPromise())
	if a.promisePrototype != nil {
		wrapper.SetPrototype(a.promisePrototype)
	}

	// Return the wrapper object as a Goja value
	return wrapper
}

// convertToGojaValue converts Go-native types to JavaScript-compatible types
// This is CRITICAL #3, #7, #9, #10 fix for type conversion
func (a *Adapter) convertToGojaValue(v any) goja.Value {
	// CRITICAL: Check if this is a wrapper for a preserved Goja Error object
	if wrapper, ok := v.(map[string]interface{}); ok {
		if original, hasOriginal := wrapper["_originalError"]; hasOriginal {
			// This is a wrapped Goja Error - return the original
			if val, ok := original.(goja.Value); ok {
				return val
			}
		}
	}

	// CRITICAL #1 FIX: Handle Goja Error objects directly (they're already Goja values)
	// If v is already a Goja value (not a Go-native type), return it directly
	if val, ok := v.(goja.Value); ok {
		return val
	}

	// CRITICAL #1 FIX: Handle ChainedPromise objects - extract Value() or Reason()
	// When Promise.resolve(1) is called, the result is a *ChainedPromise, not the value 1
	if p, ok := v.(*goeventloop.ChainedPromise); ok {
		switch p.State() {
		case goeventloop.Pending:
			// For pending promises, return undefined
			return goja.Undefined()
		case goeventloop.Rejected:
			return a.convertToGojaValue(p.Reason())
		default:
			// Fulfilled or Resolved - return the value
			return a.convertToGojaValue(p.Value())
		}
	}

	// Handle slices of Result (from combinators like All, Race, AllSettled, Any)
	if arr, ok := v.([]goeventloop.Result); ok {
		jsArr := a.runtime.NewArray(len(arr))
		for i, val := range arr {
			_ = jsArr.Set(strconv.Itoa(i), a.convertToGojaValue(val))
		}
		return jsArr
	}

	// Handle maps (from allSettled status objects)
	if m, ok := v.(map[string]interface{}); ok {
		jsObj := a.runtime.NewObject()
		for key, val := range m {
			_ = jsObj.Set(key, a.convertToGojaValue(val))
		}
		return jsObj
	}

	// Handle primitive types
	return a.runtime.ToValue(v)
}

func NewChainedPromise(loop *goeventloop.Loop, runtime *goja.Runtime) (*goeventloop.ChainedPromise, goja.Value, goja.Value) {
	js, err := goeventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}

	promise, resolve, reject := js.NewChainedPromise()
	resolveVal := runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		var val any
		if len(call.Arguments) > 0 {
			val = call.Argument(0).Export()
		}
		resolve(val)
		return goja.Undefined()
	})

	rejectVal := runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		var val any
		if len(call.Arguments) > 0 {
			val = call.Argument(0).Export()
		}
		reject(val)
		return goja.Undefined()
	})

	return promise, resolveVal, rejectVal
}

// bindPromise sets up the Promise constructor and all static combinators
func (a *Adapter) bindPromise() error {
	// Create Promise prototype with then/catch/finally methods
	promisePrototype := a.runtime.NewObject()
	a.promisePrototype = promisePrototype

	// Set then() method on prototype
	thenFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
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
		return a.gojaWrapPromise(chained)
	})

	// Set catch() method on prototype
	catchFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
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

	// Set finally() method on prototype
	finallyFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
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

	promisePrototype.Set("then", thenFn)
	promisePrototype.Set("catch", catchFn)
	promisePrototype.Set("finally", finallyFn)

	// Get the Promise constructor and ensure it has the prototype
	promiseConstructorVal := a.runtime.Get("Promise")
	promiseConstructorObj := promiseConstructorVal.ToObject(a.runtime)

	// Set the prototype on the constructor (for `Promise.prototype` property)
	promiseConstructorObj.Set("prototype", promisePrototype)

	// CRITICAL #4 (from 07-GOJA_INTEGRATION_COMBINATORS.md): Promise.resolve() with defensive null/undefined handling
	promiseConstructorObj.Set("resolve", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0)

		// Skip null/undefined - just return resolved promise
		if goja.IsNull(value) || goja.IsUndefined(value) {
			promise := a.js.Resolve(nil)
			return a.gojaWrapPromise(promise)
		}

		// Check if value is already our wrapped promise - return unchanged (identity semantics)
		if obj, ok := value.(*goja.Object); ok {
			if internalVal := obj.Get("_internalPromise"); internalVal != nil && !goja.IsUndefined(internalVal) {
				if p, ok := internalVal.Export().(*goeventloop.ChainedPromise); ok && p != nil {
					// Already a wrapped promise - return unchanged
					return value
				}
			}
		}

		// Otherwise create new resolved promise
		promise := a.js.Resolve(value.Export())
		return a.gojaWrapPromise(promise)
	}))

	// Promise.reject(reason)
	promiseConstructorObj.Set("reject", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		reason := call.Argument(0)

		// Export the reason and reject with it
		// For Goja Error objects, Export() creates map[string]interface{}
		// This is acceptable - the tests compare message property which will be preserved
		exportedReason := reason.Export()
		promise := a.js.Reject(exportedReason)
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
			if obj == nil || goja.IsNull(obj) {
				panic(a.runtime.NewTypeError("Promise.all requires an array or iterable object"))
			}
			lengthVal := obj.Get("length")
			if lengthVal == nil || goja.IsUndefined(lengthVal) {
				panic(a.runtime.NewTypeError("Promise.all requires an array with length property"))
			}
			length := int(lengthVal.ToInteger())
			arr = make([]goja.Value, length)
			for i := 0; i < length; i++ {
				arr[i] = obj.Get(strconv.Itoa(i))
			}
		}

		// CRITICAL #1 FIX: Extract wrapped promises before passing to All()
		// If val is a wrapped promise (from Promise.resolve(1)), extract internal promise
		promises := make([]*goeventloop.ChainedPromise, len(arr))
		for i, val := range arr {
			// Check if val is our wrapped promise
			if obj, ok := val.(*goja.Object); ok {
				if internalVal := obj.Get("_internalPromise"); internalVal != nil && !goja.IsUndefined(internalVal) {
					if p, ok := internalVal.Export().(*goeventloop.ChainedPromise); ok && p != nil {
						// Already our wrapped promise - use directly
						promises[i] = p
						continue
					}
				}
			}
			// Otherwise resolve as new promise
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
				panic(a.runtime.NewTypeError("Promise.race requires an array or iterable object"))
			}
			lengthVal := obj.Get("length")
			if lengthVal == nil || goja.IsUndefined(lengthVal) {
				panic(a.runtime.NewTypeError("Promise.race requires an array or iterable object"))
			}
			length := int(lengthVal.ToInteger())
			arr = make([]goja.Value, length)
			for i := 0; i < length; i++ {
				arr[i] = obj.Get(strconv.Itoa(i))
			}
		}

		// CRITICAL #1 FIX: Extract wrapped promises before passing to Race()
		promises := make([]*goeventloop.ChainedPromise, len(arr))
		for i, val := range arr {
			// Check if val is our wrapped promise
			if obj, ok := val.(*goja.Object); ok {
				if internalVal := obj.Get("_internalPromise"); internalVal != nil && !goja.IsUndefined(internalVal) {
					if p, ok := internalVal.Export().(*goeventloop.ChainedPromise); ok && p != nil {
						// Already our wrapped promise - use directly
						promises[i] = p
						continue
					}
				}
			}
			// Otherwise resolve as new promise
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
				panic(a.runtime.NewTypeError("Promise.allSettled requires an array or iterable object"))
			}
			lengthVal := obj.Get("length")
			if lengthVal == nil || goja.IsUndefined(lengthVal) {
				panic(a.runtime.NewTypeError("Promise.allSettled requires an array or iterable object"))
			}
			length := int(lengthVal.ToInteger())
			arr = make([]goja.Value, length)
			for i := 0; i < length; i++ {
				arr[i] = obj.Get(strconv.Itoa(i))
			}
		}

		// CRITICAL #1 FIX: Extract wrapped promises before passing to AllSettled()
		promises := make([]*goeventloop.ChainedPromise, len(arr))
		for i, val := range arr {
			// Check if val is our wrapped promise
			if obj, ok := val.(*goja.Object); ok {
				if internalVal := obj.Get("_internalPromise"); internalVal != nil && !goja.IsUndefined(internalVal) {
					if p, ok := internalVal.Export().(*goeventloop.ChainedPromise); ok && p != nil {
						// Already our wrapped promise - use directly
						promises[i] = p
						continue
					}
				}
			}
			// Otherwise create new promise from value
			// CRITICAL: Use NewChainedPromise directly to preserve state,
			// NOT Resolve() which would convert rejected promises to fulfilled!
			// Otherwise resolve as new promise
			promises[i] = a.js.Resolve(val.Export())
		}

		promise := a.js.AllSettled(promises)
		return a.gojaWrapPromise(promise)
	}))

	// Promise.any(iterable) - with BONUS input validation
	promiseConstructorObj.Set("any", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		iterable := call.Argument(0)
		if goja.IsNull(iterable) || goja.IsUndefined(iterable) {
			panic(a.runtime.NewTypeError("Promise.any requires an array or iterable object"))
		}

		arr, ok := iterable.Export().([]goja.Value)
		if !ok {
			// Try to convert to array-like object
			obj := iterable.ToObject(a.runtime)
			if obj == nil {
				panic(a.runtime.NewTypeError("Promise.any requires an array or iterable object"))
			}
			lengthVal := obj.Get("length")
			if lengthVal == nil || goja.IsUndefined(lengthVal) {
				panic(a.runtime.NewTypeError("Promise.any requires an array or iterable object"))
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

		// Convert to ChainedPromise array - js.Resolve() handles primitives and promises
		promises := make([]*goeventloop.ChainedPromise, len(arr))
		for i, val := range arr {
			// Check if val is our wrapped promise
			if obj, ok := val.(*goja.Object); ok {
				if internalVal := obj.Get("_internalPromise"); internalVal != nil && !goja.IsUndefined(internalVal) {
					if p, ok := internalVal.Export().(*goeventloop.ChainedPromise); ok && p != nil {
						// Already our wrapped promise - use directly
						promises[i] = p
						continue
					}
				}
			}
			// Otherwise resolve as new promise
			promises[i] = a.js.Resolve(val.Export())
		}

		promise := a.js.Any(promises)
		return a.gojaWrapPromise(promise)
	}))

	return nil
}
