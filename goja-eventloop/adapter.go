//go:build linux || darwin

package gojaeventloop

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// Adapter bridges Goja runtime to goeventloop.JS.
// This allows setTimeout/setInterval/queueMicrotask/Promise to work with Goja.
type Adapter struct {
	js               *goeventloop.JS
	runtime          *goja.Runtime
	loop             *goeventloop.Loop
	promisePrototype *goja.Object         // CRITICAL #3: Promise.prototype for instanceof support
	consoleTimers    map[string]time.Time // FEATURE-004: label -> start time
	consoleCounters  map[string]int       // EXPAND-004: label -> count

	getIterator   goja.Callable // Helper function to get [Symbol.iterator]
	consoleOutput io.Writer     // output writer for console (defaults to os.Stderr)

	consoleTimersMu   sync.RWMutex // protects consoleTimers (FEATURE-004)
	consoleCountersMu sync.RWMutex // protects consoleCounters (EXPAND-004)
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
		js:              js,
		runtime:         runtime,
		loop:            loop,
		consoleTimers:   make(map[string]time.Time),
		consoleCounters: make(map[string]int),
		consoleOutput:   os.Stderr, // Default to stderr like browsers/Node.js
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

	// Helper function for consuming iterables (used in promise combinators)
	a.runtime.Set("consumeIterable", a.consumeIterable)

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

	// FEATURE-001: AbortController and AbortSignal bindings
	a.runtime.Set("AbortController", a.abortControllerConstructor)
	a.runtime.Set("AbortSignal", a.abortSignalConstructor)

	// EXPAND-001 & EXPAND-002: Add static methods to AbortSignal
	if err := a.bindAbortSignalStatics(); err != nil {
		return fmt.Errorf("failed to bind AbortSignal statics: %w", err)
	}

	// FEATURE-002/003: performance API bindings
	if err := a.bindPerformance(); err != nil {
		return fmt.Errorf("failed to bind performance: %w", err)
	}

	// FEATURE-004: console.time/timeEnd/timeLog bindings
	if err := a.bindConsole(); err != nil {
		return fmt.Errorf("failed to bind console: %w", err)
	}

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

// R130.6 Helper: isWrappedPromise checks if a value is a Goja-wrapped
// promise object (has _internalPromise field with valid ChainedPromise).
//
// This helper eliminates code duplication across adapter.go, where the promise
// wrapper detection pattern appeared 10+ times.
//
// Returns true if value is a wrapped promise object, false otherwise.
func isWrappedPromise(value goja.Value) bool {
	// Check if value is a Goja Object
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		return false
	}

	// Check for _internalPromise field (indicates wrapped promise)
	internalVal := obj.Get("_internalPromise")
	if internalVal == nil || goja.IsUndefined(internalVal) {
		return false
	}

	// Verify internal value is a valid ChainedPromise
	promise, ok := internalVal.Export().(*goeventloop.ChainedPromise)
	return ok && promise != nil
}

// R130.6 Helper: tryExtractWrappedPromise extracts the internal ChainedPromise
// from a wrapped promise object.
//
// Returns (promise, true) if value is a wrapped promise, (nil, false) otherwise.
func tryExtractWrappedPromise(value goja.Value) (*goeventloop.ChainedPromise, bool) {
	// Check if value is a Goja Object
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		return nil, false
	}

	// Check for _internalPromise field (indicates wrapped promise)
	internalVal := obj.Get("_internalPromise")
	if internalVal == nil || goja.IsUndefined(internalVal) {
		return nil, false
	}

	// Verify internal value is a valid ChainedPromise
	promise, ok := internalVal.Export().(*goeventloop.ChainedPromise)
	if !ok || promise == nil {
		return nil, false
	}

	return promise, true
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
		// CRITICAL FIX #1: Check type at Go-native level, not after Goja conversion
		// CRITICAL FIX #2: Check for already-wrapped promises to preserve identity
		var jsValue goja.Value

		// Extract Go-native type from Result
		goNativeValue := result

		// CRITICAL #1 FIX: Check if result is already a wrapped Goja Object before conversion
		// This prevents double-wrapping which breaks Promise identity:
		//   Promise.all([p]).then(r => r[0] === p) should be true
		// R130.6: Use helper to eliminate duplicated promise wrapper detection
		if obj, ok := goNativeValue.(goja.Value); ok && isWrappedPromise(obj) {
			// Already a wrapped promise - use goja object directly to preserve identity
			jsValue = obj
		} else {
			// Not a Goja Object, proceed with standard conversion
			switch v := goNativeValue.(type) {
			case []goeventloop.Result:
				// Convert Go-native slice to JavaScript array
				jsArr := a.runtime.NewArray()
				for i, val := range v {
					// CRITICAL #1 FIX: Check each element for already-wrapped promises
					// R130.6: Use helper to eliminate duplicated promise wrapper detection
					if obj, ok := val.(goja.Value); ok && isWrappedPromise(obj) {
						// Already a wrapped promise - use directly
						_ = jsArr.Set(strconv.Itoa(i), obj)
						continue
					}
					_ = jsArr.Set(strconv.Itoa(i), a.convertToGojaValue(val))
				}
				jsValue = jsArr

			case map[string]interface{}:
				// Convert Go-native map to JavaScript object
				jsObj := a.runtime.NewObject()
				for key, val := range v {
					// CRITICAL #1 FIX: Check each value for already-wrapped promises
					// R130.6: Use helper to eliminate duplicated promise wrapper detection
					if obj, ok := val.(goja.Value); ok && isWrappedPromise(obj) {
						// Already a wrapped promise - use directly
						_ = jsObj.Set(key, obj)
						continue
					}
					_ = jsObj.Set(key, a.convertToGojaValue(val))
				}
				jsValue = jsObj

			default:
				// Primitive or other type - use standard conversion
				jsValue = a.convertToGojaValue(goNativeValue)
			}
		}

		// Call JavaScript handler with the properly converted value
		ret, err := fnCallable(goja.Undefined(), jsValue)
		if err != nil {
			// CRITICAL FIX: Panic so tryCall catches it and rejects the promise
			// Returning error would cause it to be treated as a fulfilled value!
			panic(err)
		}

		// Promise/A+ 2.3.2: If handler returns a promise, adopt its state
		// When we return *goeventloop.ChainedPromise, the framework's resolve()
		// method automatically handles state adoption via ThenWithJS() (see eventloop/promise.go)
		// This ensures proper chaining: p.then(() => p2) works correctly
		// R130.6: Use helper to eliminate duplicated promise wrapper detection
		if isWrappedPromise(ret) {
			// Extract the internal ChainedPromise from the wrapped object
			internalVal := ret.ToObject(a.runtime).Get("_internalPromise")
			if promise, ok := internalVal.Export().(*goeventloop.ChainedPromise); ok && promise != nil {
				return promise
			}
		}

		// Default: convert and return normally
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
//
// GARBAGE COLLECTION & LIFECYCLE:
// The wrapper holds a strong reference to the native ChainedPromise via _internalPromise field.
// However, Goja objects are garbage collected by Go's GC, and the wrapper itself
// is a native Goja object. When JavaScript code no longer references the wrapper,
// both the wrapper AND the native ChainedPromise become eligible for GC.
//
// GOJA GC BEHAVIOR:
// - Goja uses Go's garbage collector internally
// - Wrapper objects are reclaimed when no JavaScript references exist
// - Native promises are reclaimed when wrappers are reclaimed (no explicit cleanup needed)
// - In long-running applications, GC will periodically reclaim unreferenced promises
//
// VERIFICATION:
// - Memory leak tests (see TestMemoryLeaks_MicrotaskLoop) verify GC reclaims promises
// - Typical high-frequency microtask loops show no unbounded memory growth
// - If memory growth is observed, ensure promise references are not retained in closures
//
// NOTE: If extremely high-frequency promise creation (>100K/sec) is needed, consider
// pooling or other optimizations. For typical web service workloads, GC is sufficient.
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

			// Optimization: Pre-cache string indices for array access
			// This avoids allocating a new string on each iteration
			// Cache up to 1000 indices; beyond this, fall back to dynamic conversion
			cacheSize := length
			if cacheSize > 1000 {
				cacheSize = 1000
			}
			indexCache := make([]string, cacheSize)
			for i := 0; i < cacheSize; i++ {
				indexCache[i] = strconv.Itoa(i)
			}

			for i := 0; i < length; i++ {
				if i < cacheSize {
					result[i] = obj.Get(indexCache[i])
				} else {
					result[i] = obj.Get(strconv.Itoa(i))
				}
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
		jsArr := a.runtime.NewArray()
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

	// Handle PanicError from tryCall (unwrap and recurse)
	if panicErr, ok := v.(goeventloop.PanicError); ok {
		// Recursively convert the wrapped panic value
		return a.convertToGojaValue(panicErr.Value)
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
			_ = a.js.Resolve(nil)
			return a.gojaWrapPromise(a.js.Resolve(nil))
		}

		// Check if value is already our wrapped promise - return unchanged (identity semantics)
		// Promise.resolve(promise) === promise
		// R130.6: Use helper to eliminate duplicated promise wrapper detection
		if isWrappedPromise(value) {
			return value
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

		if isWrappedPromise(reason) {
			// Already a wrapped promise - create NEW rejected promise with wrapper as reason
			// This breaks infinite recursion by avoiding the extract → reject → wrap cycle
			promise, _, reject := a.js.NewChainedPromise()
			reject(reason) // Reject with the Goja Object (wrapper), not extracted promise

			wrapped := a.gojaWrapPromise(promise)
			return wrapped
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
			// HIGH #1 FIX: Reject promise on iterable error instead of panic
			// Iterator protocol errors should cause promise rejection, not Go panics
			// Per ES2021 spec: "If iterator.next() throws, the consuming operation should reject"
			return a.gojaWrapPromise(a.js.Reject(err))
		}

		// CRITICAL #1 FIX: Extract wrapped promises before passing to All()
		// If val is a wrapped promise (from Promise.resolve(1)), extract internal promise
		promises := make([]*goeventloop.ChainedPromise, len(arr))
		for i, val := range arr {
			// Check if val is our wrapped promise
			if isWrappedPromise(val) {
				if p, extracted := tryExtractWrappedPromise(val); extracted && p != nil {
					// Already our wrapped promise - use directly
					promises[i] = p
					continue
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
			// HIGH #1 FIX: Reject promise on iterable error instead of panic
			// Iterator protocol errors should cause promise rejection, not Go panics
			// Per ES2021 spec: "If iterator.next() throws, consuming operation should reject"
			return a.gojaWrapPromise(a.js.Reject(err))
		}

		// CRITICAL #1 FIX: Extract wrapped promises before passing to Race()
		promises := make([]*goeventloop.ChainedPromise, len(arr))
		for i, val := range arr {
			// Check if val is our wrapped promise
			if isWrappedPromise(val) {
				if p, extracted := tryExtractWrappedPromise(val); extracted && p != nil {
					// Already our wrapped promise - use directly
					promises[i] = p
					continue
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
			// HIGH #1 FIX: Reject promise on iterable error instead of panic
			// Iterator protocol errors should cause promise rejection, not Go panics
			// Per ES2021 spec: "If iterator.next() throws, consuming operation should reject"
			return a.gojaWrapPromise(a.js.Reject(err))
		}

		// CRITICAL #1 FIX: Extract wrapped promises before passing to AllSettled()
		promises := make([]*goeventloop.ChainedPromise, len(arr))
		for i, val := range arr {
			// Check if val is our wrapped promise
			if isWrappedPromise(val) {
				if p, extracted := tryExtractWrappedPromise(val); extracted && p != nil {
					// Already our wrapped promise - use directly
					promises[i] = p
					continue
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
			// HIGH #1 FIX: Reject promise on iterable error instead of panic
			// Iterator protocol errors should cause promise rejection, not Go panics
			// Per ES2021 spec: "If iterator.next() throws, consuming operation should reject"
			return a.gojaWrapPromise(a.js.Reject(err))
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
			if isWrappedPromise(val) {
				if p, extracted := tryExtractWrappedPromise(val); extracted && p != nil {
					// Already our wrapped promise - use directly
					promises[i] = p
					continue
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

	// FEATURE-005: Promise.withResolvers() ES2024 API
	// Returns an object with { promise, resolve, reject } properties
	promiseConstructorObj.Set("withResolvers", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		// Use the Go implementation
		resolvers := a.js.WithResolvers()

		// Create JS object with { promise, resolve, reject }
		obj := a.runtime.NewObject()

		// Wrap the promise
		obj.Set("promise", a.gojaWrapPromise(resolvers.Promise))

		// Create resolve function
		obj.Set("resolve", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			var val any
			if len(call.Arguments) > 0 {
				val = call.Argument(0).Export()
			}
			resolvers.Resolve(val)
			return goja.Undefined()
		}))

		// Create reject function
		obj.Set("reject", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			var val any
			if len(call.Arguments) > 0 {
				val = call.Argument(0).Export()
			}
			resolvers.Reject(val)
			return goja.Undefined()
		}))

		return obj
	}))

	// EXPAND-003: Promise.try() ES2025 API
	// Wraps a function call in a promise, catching synchronous exceptions
	promiseConstructorObj.Set("try", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		fn := call.Argument(0)
		if fn.Export() == nil {
			panic(a.runtime.NewTypeError("Promise.try requires a function"))
		}

		fnCallable, ok := goja.AssertFunction(fn)
		if !ok {
			panic(a.runtime.NewTypeError("Promise.try requires a function"))
		}

		// Use the Go implementation, passing a wrapper that calls the JS function
		promise := a.js.Try(func() any {
			// Call the JS function with no arguments
			result, err := fnCallable(goja.Undefined())
			if err != nil {
				// JS exception - will be caught by Try and rejected
				panic(err)
			}
			// Check if the result is a promise (has _internalPromise)
			// If so, extract the underlying *ChainedPromise so resolution works correctly
			if result != nil && result != goja.Undefined() && result != goja.Null() {
				if obj, ok := result.(*goja.Object); ok {
					if internal := obj.Get("_internalPromise"); internal != nil && internal != goja.Undefined() {
						if internalPromise, ok := internal.Export().(*goeventloop.ChainedPromise); ok {
							return internalPromise
						}
					}
				}
			}
			// Export the result for non-promise values
			return result.Export()
		})

		return a.gojaWrapPromise(promise)
	}))

	return nil
}

// ===============================================
// FEATURE-001: AbortController/AbortSignal Bindings
// ===============================================

// abortControllerConstructor creates the AbortController constructor for JavaScript.
func (a *Adapter) abortControllerConstructor(call goja.ConstructorCall) *goja.Object {
	controller := goeventloop.NewAbortController()

	thisObj := call.This

	// Store the native controller
	thisObj.Set("_controller", controller)

	// Define abort method
	thisObj.Set("abort", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		var reason any
		if len(call.Arguments) > 0 {
			reason = call.Argument(0).Export()
		}
		controller.Abort(reason)
		return goja.Undefined()
	}))

	// Define signal property (getter)
	signal := controller.Signal()
	signalObj := a.wrapAbortSignal(signal)
	thisObj.Set("signal", signalObj)

	return thisObj
}

// abortSignalConstructor creates the AbortSignal constructor for JavaScript.
// Note: AbortSignal is not typically constructed directly, but we provide it for completeness.
func (a *Adapter) abortSignalConstructor(call goja.ConstructorCall) *goja.Object {
	// AbortSignal should not be constructed directly - throw error
	panic(a.runtime.NewTypeError("AbortSignal cannot be constructed directly"))
}

// wrapAbortSignal wraps a Go AbortSignal in a Goja object.
func (a *Adapter) wrapAbortSignal(signal *goeventloop.AbortSignal) goja.Value {
	obj := a.runtime.NewObject()

	// Store native signal
	obj.Set("_signal", signal)

	// aborted property (getter)
	obj.DefineAccessorProperty("aborted", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.runtime.ToValue(signal.Aborted())
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// reason property (getter)
	obj.DefineAccessorProperty("reason", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		reason := signal.Reason()
		if reason == nil {
			return goja.Undefined()
		}
		return a.convertToGojaValue(reason)
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// onabort property
	var onabortHandler goja.Value
	obj.DefineAccessorProperty("onabort",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return onabortHandler
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			onabortHandler = call.Argument(0)
			if onabortHandler.Export() == nil {
				return goja.Undefined()
			}
			fn, ok := goja.AssertFunction(onabortHandler)
			if ok {
				signal.OnAbort(func(reason any) {
					_, _ = fn(goja.Undefined(), a.convertToGojaValue(reason))
				})
			}
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE)

	// addEventListener method
	obj.Set("addEventListener", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		eventType := call.Argument(0).String()
		if eventType != "abort" {
			return goja.Undefined() // Only abort events supported
		}
		handler := call.Argument(1)
		if handler.Export() == nil {
			return goja.Undefined()
		}
		fn, ok := goja.AssertFunction(handler)
		if ok {
			signal.OnAbort(func(reason any) {
				event := a.runtime.NewObject()
				event.Set("type", "abort")
				event.Set("target", obj)
				_, _ = fn(goja.Undefined(), event)
			})
		}
		return goja.Undefined()
	}))

	// throwIfAborted method
	obj.Set("throwIfAborted", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if err := signal.ThrowIfAborted(); err != nil {
			panic(a.runtime.NewGoError(err))
		}
		return goja.Undefined()
	}))

	return obj
}

// ===============================================
// EXPAND-001 & EXPAND-002: AbortSignal Static Methods
// ===============================================

// bindAbortSignalStatics adds static methods (any, timeout) to AbortSignal.
func (a *Adapter) bindAbortSignalStatics() error {
	// Get the AbortSignal constructor
	abortSignalVal := a.runtime.Get("AbortSignal")
	if abortSignalVal == nil || goja.IsUndefined(abortSignalVal) {
		return fmt.Errorf("AbortSignal not found")
	}
	abortSignalObj := abortSignalVal.ToObject(a.runtime)

	// EXPAND-001: AbortSignal.any(signals)
	// Creates a composite signal that aborts when any input signal aborts
	abortSignalObj.Set("any", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		iterable := call.Argument(0)

		// Consume iterable to get signals
		arr, err := a.consumeIterable(iterable)
		if err != nil {
			panic(a.runtime.NewTypeError("AbortSignal.any requires an iterable"))
		}

		// Extract AbortSignal instances from the array
		signals := make([]*goeventloop.AbortSignal, 0, len(arr))
		for _, val := range arr {
			// Check if it's our wrapped AbortSignal
			if val == nil || goja.IsNull(val) || goja.IsUndefined(val) {
				continue
			}

			obj := val.ToObject(a.runtime)
			if obj == nil {
				continue
			}

			// Get the internal signal
			signalVal := obj.Get("_signal")
			if signalVal == nil || goja.IsUndefined(signalVal) {
				// Not a wrapped AbortSignal - throw TypeError
				panic(a.runtime.NewTypeError("AbortSignal.any requires AbortSignal instances"))
			}

			sig, ok := signalVal.Export().(*goeventloop.AbortSignal)
			if !ok || sig == nil {
				panic(a.runtime.NewTypeError("AbortSignal.any requires AbortSignal instances"))
			}

			signals = append(signals, sig)
		}

		// Call Go implementation
		composite := goeventloop.AbortAny(signals)
		return a.wrapAbortSignal(composite)
	}))

	// EXPAND-002: AbortSignal.timeout(ms)
	// Creates a signal that aborts after the specified timeout
	abortSignalObj.Set("timeout", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		delayMs := int(call.Argument(0).ToInteger())
		if delayMs < 0 {
			delayMs = 0
		}

		// Use the Go AbortTimeout function
		controller, err := goeventloop.AbortTimeout(a.loop, delayMs)
		if err != nil {
			panic(a.runtime.NewGoError(err))
		}

		// Return the signal (not the controller)
		return a.wrapAbortSignal(controller.Signal())
	}))

	return nil
}

// ===============================================
// FEATURE-002/003: Performance API Bindings
// ===============================================

// bindPerformance creates the performance API bindings for JavaScript.
func (a *Adapter) bindPerformance() error {
	perf := goeventloop.NewLoopPerformance(a.loop)

	perfObj := a.runtime.NewObject()

	// performance.now()
	perfObj.Set("now", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.runtime.ToValue(perf.Now())
	}))

	// performance.timeOrigin
	perfObj.DefineAccessorProperty("timeOrigin", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.runtime.ToValue(perf.TimeOrigin())
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// performance.mark(name, options?)
	perfObj.Set("mark", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()

		var detail any
		if len(call.Arguments) > 1 {
			opts := call.Argument(1).ToObject(a.runtime)
			if opts != nil {
				if detailVal := opts.Get("detail"); detailVal != nil {
					detail = detailVal.Export()
				}
			}
		}

		if detail != nil {
			perf.MarkWithDetail(name, detail)
		} else {
			perf.Mark(name)
		}

		// Return the created entry
		entries := perf.GetEntriesByName(name, "mark")
		if len(entries) > 0 {
			return a.wrapPerformanceEntry(entries[len(entries)-1])
		}
		return goja.Undefined()
	}))

	// performance.measure(name, startMark?, endMark?)
	perfObj.Set("measure", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()

		var startMark, endMark string
		var detail any

		// Handle different argument patterns
		if len(call.Arguments) >= 2 {
			// Check if second arg is options object or string
			arg1 := call.Argument(1)
			// Check for undefined/null first, then check type
			isStringArg := false
			if goja.IsUndefined(arg1) || goja.IsNull(arg1) {
				isStringArg = true // Treat undefined/null as non-options
			} else if exportType := arg1.ExportType(); exportType != nil {
				isStringArg = exportType.Kind().String() == "string"
			}

			if isStringArg {
				// performance.measure(name, startMark, endMark)
				if !goja.IsUndefined(arg1) && !goja.IsNull(arg1) {
					startMark = arg1.String()
				}
				if len(call.Arguments) >= 3 {
					arg2 := call.Argument(2)
					if !goja.IsUndefined(arg2) && !goja.IsNull(arg2) {
						endMark = arg2.String()
					}
				}
			} else {
				// performance.measure(name, options)
				opts := arg1.ToObject(a.runtime)
				if opts != nil {
					if v := opts.Get("start"); v != nil && !goja.IsUndefined(v) {
						startMark = v.String()
					}
					if v := opts.Get("end"); v != nil && !goja.IsUndefined(v) {
						endMark = v.String()
					}
					if v := opts.Get("detail"); v != nil && !goja.IsUndefined(v) {
						detail = v.Export()
					}
				}
			}
		}

		var err error
		if detail != nil {
			err = perf.MeasureWithDetail(name, startMark, endMark, detail)
		} else {
			err = perf.Measure(name, startMark, endMark)
		}

		if err != nil {
			panic(a.runtime.NewGoError(err))
		}

		// Return the created entry
		entries := perf.GetEntriesByName(name, "measure")
		if len(entries) > 0 {
			return a.wrapPerformanceEntry(entries[len(entries)-1])
		}
		return goja.Undefined()
	}))

	// performance.getEntries()
	perfObj.Set("getEntries", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		entries := perf.GetEntries()
		return a.wrapPerformanceEntries(entries)
	}))

	// performance.getEntriesByType(type)
	perfObj.Set("getEntriesByType", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		entryType := call.Argument(0).String()
		entries := perf.GetEntriesByType(entryType)
		return a.wrapPerformanceEntries(entries)
	}))

	// performance.getEntriesByName(name, type?)
	perfObj.Set("getEntriesByName", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		var entries []goeventloop.PerformanceEntry
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			entryType := call.Argument(1).String()
			entries = perf.GetEntriesByName(name, entryType)
		} else {
			entries = perf.GetEntriesByName(name)
		}
		return a.wrapPerformanceEntries(entries)
	}))

	// performance.clearMarks(name?)
	perfObj.Set("clearMarks", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		var name string
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			name = call.Argument(0).String()
		}
		perf.ClearMarks(name)
		return goja.Undefined()
	}))

	// performance.clearMeasures(name?)
	perfObj.Set("clearMeasures", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		var name string
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			name = call.Argument(0).String()
		}
		perf.ClearMeasures(name)
		return goja.Undefined()
	}))

	// performance.clearResourceTimings()
	perfObj.Set("clearResourceTimings", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		perf.ClearResourceTimings()
		return goja.Undefined()
	}))

	// performance.toJSON()
	perfObj.Set("toJSON", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		json := perf.ToJSON()
		jsObj := a.runtime.NewObject()
		for k, v := range json {
			jsObj.Set(k, a.runtime.ToValue(v))
		}
		return jsObj
	}))

	a.runtime.Set("performance", perfObj)
	return nil
}

// wrapPerformanceEntry wraps a Go PerformanceEntry in a Goja object.
func (a *Adapter) wrapPerformanceEntry(entry goeventloop.PerformanceEntry) goja.Value {
	obj := a.runtime.NewObject()
	obj.Set("name", entry.Name)
	obj.Set("entryType", entry.EntryType)
	obj.Set("startTime", entry.StartTime)
	obj.Set("duration", entry.Duration)
	if entry.Detail != nil {
		obj.Set("detail", a.runtime.ToValue(entry.Detail))
	}

	// toJSON method
	obj.Set("toJSON", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return obj
	}))

	return obj
}

// wrapPerformanceEntries wraps a slice of PerformanceEntry in a Goja array.
func (a *Adapter) wrapPerformanceEntries(entries []goeventloop.PerformanceEntry) goja.Value {
	// Create array with wrapped entries
	arr := a.runtime.NewArray()
	for i, entry := range entries {
		arr.Set(strconv.Itoa(i), a.wrapPerformanceEntry(entry))
	}
	return arr
}

// ===============================================
// FEATURE-004: console.time/timeEnd/timeLog API
// ===============================================

// SetConsoleOutput sets the writer for console.time/timeEnd/timeLog output.
// This is useful for testing or redirecting output. Defaults to os.Stderr.
// Setting to nil will disable output (timers still track but no output).
func (a *Adapter) SetConsoleOutput(w io.Writer) {
	a.consoleTimersMu.Lock()
	defer a.consoleTimersMu.Unlock()
	a.consoleOutput = w
}

// bindConsole creates console.time/timeEnd/timeLog bindings in Goja global scope.
// This creates or extends the console object with timer methods.
func (a *Adapter) bindConsole() error {
	// Get or create console object
	consoleVal := a.runtime.Get("console")
	var consoleObj *goja.Object
	if consoleVal == nil || goja.IsUndefined(consoleVal) {
		// No console object exists, create one
		consoleObj = a.runtime.NewObject()
		a.runtime.Set("console", consoleObj)
	} else {
		// Console object exists, extend it
		consoleObj = consoleVal.ToObject(a.runtime)
	}

	// console.time(label) - starts a timer with given label
	// If label is omitted, "default" is used.
	// If a timer with the same label already exists, a warning is logged.
	consoleObj.Set("time", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		label := "default"
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			label = call.Argument(0).String()
		}

		a.consoleTimersMu.Lock()
		defer a.consoleTimersMu.Unlock()

		if _, exists := a.consoleTimers[label]; exists {
			// Timer already exists - log warning (matches Node.js behavior)
			if a.consoleOutput != nil {
				fmt.Fprintf(a.consoleOutput, "Warning: Timer '%s' already exists\n", label)
			}
			return goja.Undefined()
		}

		a.consoleTimers[label] = time.Now()
		return goja.Undefined()
	}))

	// console.timeEnd(label) - stops timer and logs duration
	// If label is omitted, "default" is used.
	// If no timer with the label exists, a warning is logged.
	// Output format matches Node.js: "label: X.XXXms"
	consoleObj.Set("timeEnd", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		label := "default"
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			label = call.Argument(0).String()
		}

		a.consoleTimersMu.Lock()
		defer a.consoleTimersMu.Unlock()

		startTime, exists := a.consoleTimers[label]
		if !exists {
			// Timer doesn't exist - log warning (matches Node.js behavior)
			if a.consoleOutput != nil {
				fmt.Fprintf(a.consoleOutput, "Warning: Timer '%s' does not exist\n", label)
			}
			return goja.Undefined()
		}

		elapsed := time.Since(startTime)
		delete(a.consoleTimers, label)

		// Output format matches Node.js: "label: X.XXXms"
		if a.consoleOutput != nil {
			fmt.Fprintf(a.consoleOutput, "%s: %.3fms\n", label, float64(elapsed.Nanoseconds())/1e6)
		}

		return goja.Undefined()
	}))

	// console.timeLog(label, ...data) - logs elapsed time without stopping timer
	// If label is omitted, "default" is used.
	// If no timer with the label exists, a warning is logged.
	// Additional arguments are logged after the time.
	consoleObj.Set("timeLog", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		label := "default"
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			label = call.Argument(0).String()
		}

		a.consoleTimersMu.RLock()
		startTime, exists := a.consoleTimers[label]
		a.consoleTimersMu.RUnlock()

		if !exists {
			// Timer doesn't exist - log warning (matches Node.js behavior)
			a.consoleTimersMu.RLock()
			output := a.consoleOutput
			a.consoleTimersMu.RUnlock()
			if output != nil {
				fmt.Fprintf(output, "Warning: Timer '%s' does not exist\n", label)
			}
			return goja.Undefined()
		}

		elapsed := time.Since(startTime)

		// Build output with optional additional data
		a.consoleTimersMu.RLock()
		output := a.consoleOutput
		a.consoleTimersMu.RUnlock()

		if output != nil {
			// Output format matches Node.js: "label: X.XXXms [data...]"
			if len(call.Arguments) > 1 {
				// Additional arguments to log
				dataStrs := make([]string, 0, len(call.Arguments)-1)
				for i := 1; i < len(call.Arguments); i++ {
					dataStrs = append(dataStrs, fmt.Sprintf("%v", call.Argument(i).Export()))
				}
				fmt.Fprintf(output, "%s: %.3fms %s\n", label, float64(elapsed.Nanoseconds())/1e6,
					joinStrings(dataStrs, " "))
			} else {
				fmt.Fprintf(output, "%s: %.3fms\n", label, float64(elapsed.Nanoseconds())/1e6)
			}
		}

		return goja.Undefined()
	}))

	// EXPAND-004: console.count(label) - logs count for label
	// If label is omitted, "default" is used.
	// Increments count and logs "label: count"
	consoleObj.Set("count", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		label := "default"
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			label = call.Argument(0).String()
		}

		a.consoleCountersMu.Lock()
		a.consoleCounters[label]++
		count := a.consoleCounters[label]
		a.consoleCountersMu.Unlock()

		// Output format matches Node.js: "label: count"
		a.consoleTimersMu.RLock()
		output := a.consoleOutput
		a.consoleTimersMu.RUnlock()

		if output != nil {
			fmt.Fprintf(output, "%s: %d\n", label, count)
		}

		return goja.Undefined()
	}))

	// EXPAND-004: console.countReset(label) - resets counter for label
	// If label is omitted, "default" is used.
	// If no counter with the label exists, a warning is logged.
	consoleObj.Set("countReset", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		label := "default"
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			label = call.Argument(0).String()
		}

		a.consoleCountersMu.Lock()
		_, exists := a.consoleCounters[label]
		if !exists {
			a.consoleCountersMu.Unlock()
			// Counter doesn't exist - log warning (matches Node.js behavior)
			a.consoleTimersMu.RLock()
			output := a.consoleOutput
			a.consoleTimersMu.RUnlock()
			if output != nil {
				fmt.Fprintf(output, "Warning: Count for '%s' does not exist\n", label)
			}
			return goja.Undefined()
		}
		delete(a.consoleCounters, label)
		a.consoleCountersMu.Unlock()

		return goja.Undefined()
	}))

	// EXPAND-005: console.assert(condition, ...data) - logs only when falsy
	// If the condition is falsy, logs "Assertion failed: data..."
	// If condition is truthy, does nothing.
	consoleObj.Set("assert", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		// Get condition - defaults to undefined (falsy)
		condition := false
		if len(call.Arguments) > 0 {
			condition = call.Argument(0).ToBoolean()
		}

		// If condition is truthy, do nothing
		if condition {
			return goja.Undefined()
		}

		// Condition is falsy - log assertion failure
		a.consoleTimersMu.RLock()
		output := a.consoleOutput
		a.consoleTimersMu.RUnlock()

		if output != nil {
			if len(call.Arguments) > 1 {
				// Additional data to log
				dataStrs := make([]string, 0, len(call.Arguments)-1)
				for i := 1; i < len(call.Arguments); i++ {
					dataStrs = append(dataStrs, fmt.Sprintf("%v", call.Argument(i).Export()))
				}
				fmt.Fprintf(output, "Assertion failed: %s\n", joinStrings(dataStrs, " "))
			} else {
				fmt.Fprintf(output, "Assertion failed\n")
			}
		}

		return goja.Undefined()
	}))

	return nil
}

// joinStrings joins a slice of strings with a separator.
// This is a simple helper to avoid importing strings package.
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
