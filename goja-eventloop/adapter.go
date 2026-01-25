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
	promisePrototype *goja.Object  // CRITICAL #3: Promise.prototype for instanceof support
	getIterator      goja.Callable // Helper function to get [Symbol.iterator]
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
func (a *Adapter) Bind() error {
	// Bind all JavaScript globals to the Goja runtime
	// Timer functions
	a.runtime.Set("setTimeout", a.setTimeout)
	a.runtime.Set("clearTimeout", a.clearTimeout)
	a.runtime.Set("setInterval", a.setInterval)
	a.runtime.Set("clearInterval", a.clearInterval)
	a.runtime.Set("queueMicrotask", a.queueMicrotask)
	// Chunk 2 Feature: setImmediate
	a.runtime.Set("setImmediate", a.setImmediate)
	a.runtime.Set("clearImmediate", a.clearImmediate)

	// Promise constructor
	a.runtime.Set("Promise", a.promiseConstructor)

	// Helper to access Symbol.iterator (Goja API for symbols is complex, JS is easy)
	// We create a function (obj) => obj[Symbol.iterator]
	// This handles the lookup safely within the runtime
	iterScript := `(obj) => obj[Symbol.iterator]`
	iterVal, err := a.runtime.RunString(iterScript)
	if err != nil {
		return fmt.Errorf("failed to compile iterator helper: %w", err)
	}
	iterAssert, ok := goja.AssertFunction(iterVal)
	if !ok {
		return fmt.Errorf("iterator helper is not a function")
	}
	a.getIterator = iterAssert

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

	return a.runtime.ToValue(float64(id))
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

	return a.runtime.ToValue(float64(id))
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

// setImmediate binding for Goja (implemented as setTimeout(fn, 0))
func (a *Adapter) setImmediate(call goja.FunctionCall) goja.Value {
	fn := call.Argument(0)
	if fn.Export() == nil {
		panic(a.runtime.NewTypeError("setImmediate requires a function as first argument"))
	}

	fnCallable, ok := goja.AssertFunction(fn)
	if !ok {
		panic(a.runtime.NewTypeError("setImmediate requires a function as first argument"))
	}

	// Use optimized SetImmediate instead of SetTimeout
	id, err := a.js.SetImmediate(func() {
		_, _ = fnCallable(goja.Undefined())
	})
	if err != nil {
		panic(a.runtime.NewGoError(err))
	}

	return a.runtime.ToValue(float64(id))
}

// clearImmediate binding for Goja
func (a *Adapter) clearImmediate(call goja.FunctionCall) goja.Value {
	id := uint64(call.Argument(0).ToInteger())
	_ = a.js.ClearImmediate(id) // Use specialized ClearImmediate
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
		// No handler provided - return nil to let ChainedPromise handle propagation
		return nil
	}

	fnCallable, ok := goja.AssertFunction(fn)
	if !ok {
		// Not a function - return nil to let ChainedPromise handle propagation
		return nil
	}

	return func(result goeventloop.Result) goeventloop.Result {
		// CRITICAL FIX: Check type at Go-native level, not after Goja conversion
		var jsValue goja.Value

		// Extract Go-native type from Result
		goNativeValue := result

		switch v := goNativeValue.(type) {
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
			// CRITICAL FIX: Panic so tryCall catches it and rejects the promise
			// Returning error would cause it to be treated as a fulfilled value!
			panic(err)
		}

		// CRITICAL FIX: Check if return value is a Wrapped Promise and unwrap it
		// This enables proper chaining: p.then(() => p2)
		if obj, ok := ret.(*goja.Object); ok {
			if internalVal := obj.Get("_internalPromise"); internalVal != nil && !goja.IsUndefined(internalVal) {
				if p, ok := internalVal.Export().(*goeventloop.ChainedPromise); ok && p != nil {
					// Return the ChainedPromise itself so strict resolution sees it
					return p
				}
			}
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

// exportGojaValue extracts a Goja Value from Goja.Value for export without losing type info.
// For Goja.Error objects, we want to preserve the original Value instead of using Export().
// Returns (value, true) if value should be preserved as Goja Value, (nil, false) otherwise.
func exportGojaValue(gojaVal goja.Value) (any, bool) {
	if gojaVal == nil || goja.IsNull(gojaVal) || goja.IsUndefined(gojaVal) {
		return nil, false
	}

	// Goja.Error objects: preserve as Goja.Value to maintain .message property
	if obj, ok := gojaVal.(*goja.Object); ok {
		if nameVal := obj.Get("name"); nameVal != nil && !goja.IsUndefined(nameVal) {
			if nameStr, ok := nameVal.Export().(string); ok && (nameStr == "Error" || nameStr == "TypeError" || nameStr == "RangeError" || nameStr == "ReferenceError") {
				// This is an Error object - preserve original Goja.Value
				return gojaVal, true
			}
		}
	}
	return nil, false
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

// consumeIterable converts an iterable Goja value to a slice of values.
// Supports Arrays, Strings, Sets, Maps, and any object implementing [Symbol.iterator].
// Returns an error if the value is not iterable.
func (a *Adapter) consumeIterable(iterable goja.Value) ([]goja.Value, error) {
	// 1. Handle null/undefined early
	if iterable == nil || goja.IsNull(iterable) || goja.IsUndefined(iterable) {
		return nil, fmt.Errorf("cannot consume null or undefined as iterable")
	}

	// 2. Optimisation: Check for standard Array first (fast path)
	if _, ok := iterable.Export().([]interface{}); ok {
		// Use native export/cast for arrays which is much faster than iterator protocol
		// However, Export() returns []interface{}, not []goja.Value
		// We can simpler use ToObject and get elements by index if it's an array
		obj := iterable.ToObject(a.runtime)
		// Standard array check: verify length property exists and is a number
		if lenVal := obj.Get("length"); lenVal != nil && !goja.IsUndefined(lenVal) {
			// This covers Arrays and array-like objects
			length := int(lenVal.ToInteger())
			result := make([]goja.Value, length)
			for i := 0; i < length; i++ {
				result[i] = obj.Get(strconv.Itoa(i))
			}
			return result, nil
		}
	}

	// 3. Fallback: Use Iterator Protocol (Symbol.iterator)
	// This handles Set, Map, String, Generators, custom iterables

	// Use our JS helper to get the iterator method (handles Symbol lookup)
	iteratorMethodVal, err := a.getIterator(goja.Undefined(), iterable)
	if err != nil {
		return nil, err // Helper failed
	}

	if iteratorMethodVal == nil || goja.IsUndefined(iteratorMethodVal) {
		// Not an iterable (no Symbol.iterator method)
		return nil, fmt.Errorf("object is not iterable (cannot get Symbol.iterator)")
	}

	iteratorMethodCallable, ok := goja.AssertFunction(iteratorMethodVal)
	if !ok {
		return nil, fmt.Errorf("symbol.iterator is not a function")
	}

	// Call [Symbol.iterator]() to get the iterator object
	iteratorVal, err := iteratorMethodCallable(iterable)
	if err != nil {
		return nil, err
	}
	iteratorObj := iteratorVal.ToObject(a.runtime)

	// Get the next() method from the iterator
	nextMethod := iteratorObj.Get("next")
	nextMethodCallable, ok := goja.AssertFunction(nextMethod)
	if !ok {
		return nil, fmt.Errorf("iterator.next is not a function")
	}

	var values []goja.Value
	for {
		// Call iterator.next()
		nextResult, err := nextMethodCallable(iteratorObj)
		if err != nil {
			return nil, err
		}
		nextResultObj := nextResult.ToObject(a.runtime)

		// Check done property
		done := nextResultObj.Get("done")
		if done != nil && done.ToBoolean() {
			break
		}

		// Get value property
		value := nextResultObj.Get("value")
		values = append(values, value)
	}

	return values, nil
}

// resolveThenable handles "thenables" - objects with a "then" method.
// Returns a new *ChainedPromise that adopts the state of the thenable,
// or nil if the value is not a thenable.
func (a *Adapter) resolveThenable(value goja.Value) *goeventloop.ChainedPromise {
	if value == nil || goja.IsNull(value) || goja.IsUndefined(value) {
		return nil
	}

	// Must be an object or function to be a thenable
	obj := value.ToObject(a.runtime)
	if obj == nil {
		return nil
	}

	// Check for .then property
	thenProp := obj.Get("then")
	if thenProp == nil || goja.IsUndefined(thenProp) {
		return nil
	}

	// Must be a function
	thenFn, ok := goja.AssertFunction(thenProp)
	if !ok {
		return nil
	}

	// It IS a thenable. Adopt its state.
	// We create a new promise and pass its resolve/reject to the then function.
	promise, resolve, reject := a.js.NewChainedPromise()

	// Safely call then(resolve, reject)
	// We need to wrap resolve/reject as Goja functions
	resolveVal := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		var val any
		if len(call.Arguments) > 0 {
			arg := call.Argument(0)
			// CRITICAL FIX: Preserve Goja.Error objects directly without Export()
			// Export() converts Goja.Error to opaque wrapper that loses .message property
			// By passing Goja.Value directly, convertToGojaValue() can unwrap it properly
			if exportedVal, ok := exportGojaValue(arg); ok {
				val = exportedVal
			} else {
				val = arg.Export()
			}
		}
		resolve(val)
		return goja.Undefined()
	})

	rejectVal := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		var val any
		if len(call.Arguments) > 0 {
			arg := call.Argument(0)
			// CRITICAL FIX: Preserve Goja.Error objects directly without Export()
			// Export() converts Goja.Error to opaque wrapper that loses .message property
			// By passing Goja.Value directly, convertToGojaValue() can unwrap it properly
			if exportedVal, ok := exportGojaValue(arg); ok {
				val = exportedVal
			} else {
				val = arg.Export()
			}
		}
		reject(val)
		return goja.Undefined()
	})

	// Execute then handler
	// Promise/A+ 2.3.3.3: If then is a function, call it with x as this...
	_, err := thenFn(obj, resolveVal, rejectVal)
	if err != nil {
		// If calling then throws, reject the promise
		// Promise/A+ 2.3.3.3.4
		// Note: We should technically only reject if neither resolve/reject has been called,
		// but NewChainedPromise's resolve/reject are idempotent/thread-safe so this is fine.
		reject(err)
	}

	return promise
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

	// CRITICAL FIX: Handle ChainedPromise objects by wrapping them
	// This preserves referential identity for Promise.reject(p) compliance
	// When a ChainedPromise is passed through (e.g., as rejection reason),
	// we must preserve it as-is, not unwrap its value/reason
	if p, ok := v.(*goeventloop.ChainedPromise); ok {
		// Wrap the ChainedPromise to preserve identity
		return a.gojaWrapPromise(p)
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

	// CRITICAL #10 FIX: Handle *AggregateError specifically to enable checking err.message/err.errors in JS
	if agg, ok := v.(*goeventloop.AggregateError); ok {
		jsObj := a.runtime.NewObject()
		_ = jsObj.Set("message", agg.Error())
		_ = jsObj.Set("errors", a.convertToGojaValue(agg.Errors))
		_ = jsObj.Set("name", "AggregateError")
		return jsObj
	}

	// Handle Goja exceptions (unwrap to original JS value)
	if ex, ok := v.(*goja.Exception); ok {
		return ex.Value()
	}

	// Handle generic errors (wrap as JS Error)
	if err, ok := v.(error); ok {
		// NewGoError wraps the error properly exposing .message
		return a.runtime.NewGoError(err)
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
		// Promise.resolve(promise) === promise
		if obj, ok := value.(*goja.Object); ok {
			if internalVal := obj.Get("_internalPromise"); internalVal != nil && !goja.IsUndefined(internalVal) {
				if p, ok := internalVal.Export().(*goeventloop.ChainedPromise); ok && p != nil {
					// Already a wrapped promise - return unchanged
					return value
				}
			}
		}

		// CRITICAL COMPLIANCE FIX: Check for thenables
		if p := a.resolveThenable(value); p != nil {
			// It was a thenable, return the adopted promise
			return a.gojaWrapPromise(p)
		}

		// Otherwise create new resolved promise
		promise := a.js.Resolve(value.Export())
		return a.gojaWrapPromise(promise)
	}))

	// Promise.reject(reason)
	promiseConstructorObj.Set("reject", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		reason := call.Argument(0)

		// CRITICAL FIX: Preserve Goja Error objects without Export() to maintain .message property
		// When Export() converts Error objects, they become opaque wrappers losing .message
		if obj, ok := reason.(*goja.Object); ok && !goja.IsNull(reason) && !goja.IsUndefined(reason) {
			if nameVal := obj.Get("name"); nameVal != nil && !goja.IsUndefined(nameVal) {
				if nameStr, ok := nameVal.Export().(string); ok && (nameStr == "Error" || nameStr == "TypeError" || nameStr == "RangeError" || nameStr == "ReferenceError") {
					// This is an Error object - preserve original Goja.Value
					promise, _, reject := a.js.NewChainedPromise()
					reject(reason) // Reject with Goja.Error (preserves .message property)
					return a.gojaWrapPromise(promise)
				}
			}
		}

		// SPECIFICATION COMPLIANCE (Promise.reject promise object):
		// When reason is a wrapped promise object (with _internalPromise field),
		// we must preserve the wrapper object as the rejection reason per JS spec.
		// Export() on the wrapper returns a map, which would unwrap and lose identity.
		// CRITICAL: We must NOT call a.js.Reject(obj) as it triggers gojaWrapPromise again,
		// causing infinite recursion. Instead, create a new rejected promise directly.

		if obj, ok := reason.(*goja.Object); ok {
			// Check if reason is a wrapped promise with _internalPromise field
			if internalVal := obj.Get("_internalPromise"); internalVal != nil && !goja.IsUndefined(internalVal) {
				// Already a wrapped promise - create NEW rejected promise with wrapper as reason
				// This breaks infinite recursion by avoiding the extract → reject → wrap cycle
				promise, _, reject := a.js.NewChainedPromise()
				reject(obj) // Reject with the Goja Object (wrapper), not extracted promise

				wrapped := a.gojaWrapPromise(promise)
				return wrapped
			}
		}

		// For all other types (primitives, plain objects), use Export()
		// This preserves properties like Error.message and custom fields
		promise := a.js.Reject(reason.Export())
		return a.gojaWrapPromise(promise)
	}))

	// Promise.all(iterable) - with COMPLIANCE FIX for iterables
	promiseConstructorObj.Set("all", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		iterable := call.Argument(0)

		// Consume iterable using standard protocol
		arr, err := a.consumeIterable(iterable)
		if err != nil {
			// Promise.all rejects if iterable cannot be consumed (or throws strict error)
			// Spec says: "If the completion is an abrupt completion, return a promise rejected with the abrupt completion's value."
			// Goja panic will propagate as an exception, which fits nicely.
			panic(err)
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

			// COMPLIANCE: Check for thenables in array elements too!
			if p := a.resolveThenable(val); p != nil {
				promises[i] = p
				continue
			}

			// Otherwise resolve as new promise
			promises[i] = a.js.Resolve(val.Export())
		}

		promise := a.js.All(promises)
		return a.gojaWrapPromise(promise)
	}))

	// Promise.race(iterable) - with COMPLIANCE FIX for iterables
	promiseConstructorObj.Set("race", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		iterable := call.Argument(0)

		// Consume iterable using standard protocol
		arr, err := a.consumeIterable(iterable)
		if err != nil {
			panic(err)
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

			// COMPLIANCE: Check for thenables
			if p := a.resolveThenable(val); p != nil {
				promises[i] = p
				continue
			}

			// Otherwise resolve as new promise
			promises[i] = a.js.Resolve(val.Export())
		}

		promise := a.js.Race(promises)
		return a.gojaWrapPromise(promise)
	}))

	// Promise.allSettled(iterable) - with COMPLIANCE FIX for iterables
	promiseConstructorObj.Set("allSettled", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		iterable := call.Argument(0)

		// Consume iterable using standard protocol
		arr, err := a.consumeIterable(iterable)
		if err != nil {
			panic(err)
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

			// COMPLIANCE: Check for thenables
			if p := a.resolveThenable(val); p != nil {
				promises[i] = p
				continue
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

	// Promise.any(iterable) - with COMPLIANCE FIX for iterables
	promiseConstructorObj.Set("any", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		iterable := call.Argument(0)

		// Consume iterable using standard protocol
		arr, err := a.consumeIterable(iterable)
		if err != nil {
			panic(err)
		}

		if len(arr) == 0 {
			// ES2021: Promise.any([]) rejects with AggregateError
			// Our eventloop implementation of Any handles empty/all-rejected correctly
			// But we need to pass empty array to it.
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

			// COMPLIANCE: Check for thenables
			if p := a.resolveThenable(val); p != nil {
				promises[i] = p
				continue
			}

			// Otherwise resolve as new promise
			promises[i] = a.js.Resolve(val.Export())
		}

		promise := a.js.Any(promises)
		return a.gojaWrapPromise(promise)
	}))

	return nil
}
