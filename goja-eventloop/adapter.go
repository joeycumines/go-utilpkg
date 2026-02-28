package gojaeventloop

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// Adapter bridges Goja runtime to goeventloop.JS.
// This allows setTimeout/setInterval/queueMicrotask/Promise to work with Goja.
type Adapter struct { //nolint:govet // betteralign:ignore
	// dispatchJSEvents maps Go Event pointers to their JS wrapper objects during dispatch.
	// This allows event listeners to receive the original JS event object (including CustomEvent.detail).
	dispatchJSEvents sync.Map // map[*goeventloop.Event]goja.Value

	consoleTimersMu   sync.RWMutex // protects consoleTimers
	consoleCountersMu sync.RWMutex // protects consoleCounters
	consoleIndentMu   sync.RWMutex // protects consoleIndent

	js               *goeventloop.JS
	runtime          *goja.Runtime
	loop             *goeventloop.Loop
	promisePrototype *goja.Object         // Promise.prototype for instanceof support
	consoleTimers    map[string]time.Time // label -> start time
	consoleCounters  map[string]int       // label -> count
	getIterator      goja.Callable        // Helper function to get [Symbol.iterator]
	consoleOutput    io.Writer            // output writer for console (defaults to os.Stderr)
	consoleIndent    int                  // current group indentation level
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

	// AbortController and AbortSignal bindings
	a.runtime.Set("AbortController", a.abortControllerConstructor)
	a.runtime.Set("AbortSignal", a.abortSignalConstructor)

	// Add static methods to AbortSignal
	if err := a.bindAbortSignalStatics(); err != nil {
		return fmt.Errorf("failed to bind AbortSignal statics: %w", err)
	}

	// performance API bindings
	if err := a.bindPerformance(); err != nil {
		return fmt.Errorf("failed to bind performance: %w", err)
	}

	// console.time/timeEnd/timeLog bindings
	if err := a.bindConsole(); err != nil {
		return fmt.Errorf("failed to bind console: %w", err)
	}

	// process.nextTick binding
	if err := a.bindProcess(); err != nil {
		return fmt.Errorf("failed to bind process: %w", err)
	}

	// delay() global function
	a.runtime.Set("delay", a.delay)

	// crypto.randomUUID binding
	if err := a.bindCrypto(); err != nil {
		return fmt.Errorf("failed to bind crypto: %w", err)
	}

	// atob/btoa base64 functions
	a.runtime.Set("atob", a.atob)
	a.runtime.Set("btoa", a.btoa)

	// EventTarget and Event bindings
	a.runtime.Set("EventTarget", a.eventTargetConstructor)
	a.runtime.Set("Event", a.eventConstructor)

	// CustomEvent binding
	a.runtime.Set("CustomEvent", a.customEventConstructor)

	// structuredClone global function
	a.runtime.Set("structuredClone", a.structuredClone)

	// URL and URLSearchParams bindings
	a.runtime.Set("URL", a.urlConstructor)
	a.runtime.Set("URLSearchParams", a.urlSearchParamsConstructor)

	// TextEncoder and TextDecoder bindings
	a.runtime.Set("TextEncoder", a.textEncoderConstructor)
	a.runtime.Set("TextDecoder", a.textDecoderConstructor)

	// Blob binding
	a.runtime.Set("Blob", a.blobConstructor)

	// localStorage and sessionStorage (in-memory)
	if err := a.bindStorage(); err != nil {
		return fmt.Errorf("failed to bind storage: %w", err)
	}

	// Headers class (for fetch-like patterns)
	a.runtime.Set("Headers", a.headersConstructor)

	// FormData class (for fetch-like patterns)
	a.runtime.Set("FormData", a.formDataConstructor)

	// FETCH API STATUS: Not Implemented
	// The fetch() API is not currently implemented. If your JavaScript code
	// requires HTTP functionality, use Go's net/http package on the host side
	// and expose it via custom bindings. The Headers and FormData classes
	// are provided to ease future fetch() integration or custom implementations.
	//
	// TODO: Consider implementing fetch() with a configurable http.Client.
	// See: https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API
	a.runtime.Set("fetch", a.fetchNotImplemented)

	// DOMException class
	a.runtime.Set("DOMException", a.domExceptionConstructor)
	if err := a.bindDOMExceptionConstants(); err != nil {
		return fmt.Errorf("failed to bind DOMException constants: %w", err)
	}

	// Symbol.for and Symbol.keyFor utilities
	if err := a.bindSymbol(); err != nil {
		return fmt.Errorf("failed to bind Symbol: %w", err)
	}

	// Call bindPromise() to set up all combinators
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

	// WHATWG HTML Spec Section 8.6: Clamp negative delays to 0
	// https://html.spec.whatwg.org/multipage/timers-and-user-prompts.html
	delayMs := max(int(call.Argument(1).ToInteger()), 0)

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

	// WHATWG HTML Spec Section 8.6: Clamp negative delays to 0
	// https://html.spec.whatwg.org/multipage/timers-and-user-prompts.html
	delayMs := max(int(call.Argument(1).ToInteger()), 0)

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
	// Validate executor FIRST before creating promise to prevent resource leaks
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

// isWrappedPromise checks if a value is a Goja-wrapped
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

// tryExtractWrappedPromise extracts the internal ChainedPromise
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
// Type conversion at Go-native level BEFORE passing to JavaScript
func (a *Adapter) gojaFuncToHandler(fn goja.Value) func(any) any {
	if fn.Export() == nil {
		// No handler provided - return nil to let ChainedPromise handle propagation
		return nil
	}

	fnCallable, ok := goja.AssertFunction(fn)
	if !ok {
		// Not a function - return nil to let ChainedPromise handle propagation
		return nil
	}

	return func(result any) any {
		// Check type at Go-native level, not after Goja conversion
		// Check for already-wrapped promises to preserve identity
		var jsValue goja.Value

		// Extract Go-native type from Result
		goNativeValue := result

		// Check if result is already a wrapped Goja Object before conversion
		// This prevents double-wrapping which breaks Promise identity:
		//   Promise.all([p]).then(r => r[0] === p) should be true
		// Use helper to eliminate duplicated promise wrapper detection
		if obj, ok := goNativeValue.(goja.Value); ok && isWrappedPromise(obj) {
			// Already a wrapped promise - use goja object directly to preserve identity
			jsValue = obj
		} else {
			// Not a Goja Object, proceed with standard conversion
			switch v := goNativeValue.(type) {
			case []any:
				// Convert Go-native slice to JavaScript array
				jsArr := a.runtime.NewArray()
				for i, val := range v {
					// Check each element for already-wrapped promises
					// Use helper to eliminate duplicated promise wrapper detection
					if obj, ok := val.(goja.Value); ok && isWrappedPromise(obj) {
						// Already a wrapped promise - use directly
						_ = jsArr.Set(strconv.Itoa(i), obj)
						continue
					}
					_ = jsArr.Set(strconv.Itoa(i), a.convertToGojaValue(val))
				}
				jsValue = jsArr

			case map[string]any:
				// Convert Go-native map to JavaScript object
				jsObj := a.runtime.NewObject()
				for key, val := range v {
					// Check each value for already-wrapped promises
					// Use helper to eliminate duplicated promise wrapper detection
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
			// Panic so tryCall catches it and rejects the promise
			// Returning error would cause it to be treated as a fulfilled value!
			panic(err)
		}

		// Promise/A+ 2.3.2: If handler returns a promise, adopt its state
		// When we return *goeventloop.ChainedPromise, the framework's resolve()
		// method automatically handles state adoption via Then() (see eventloop/promise.go)
		// This ensures proper chaining: p.then(() => p2) works correctly
		// Use helper to eliminate duplicated promise wrapper detection
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

// GojaWrapPromise wraps a [goeventloop.ChainedPromise] with then/catch/finally
// instance methods, returning a goja.Value suitable for use in JavaScript.
//
// Use this when you create a Go-side promise and need to expose it to JS code:
//
//	promise, resolve, _ := adapter.JS().NewChainedPromise()
//	go func() { resolve(computeResult()) }()
//	return adapter.GojaWrapPromise(promise)
//
// The wrapper holds a strong reference to the native ChainedPromise. When
// JavaScript code no longer references the wrapper, both the wrapper AND the
// native ChainedPromise become eligible for garbage collection.
func (a *Adapter) GojaWrapPromise(promise *goeventloop.ChainedPromise) goja.Value {
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
	if _, ok := iterable.Export().([]any); ok {
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
			cacheSize := min(length, 1000)
			indexCache := make([]string, cacheSize)
			for i := range cacheSize {
				indexCache[i] = strconv.Itoa(i)
			}

			for i := range length {
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
			// Preserve Goja.Error objects directly without Export()
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
			// Preserve Goja.Error objects directly without Export()
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
func (a *Adapter) convertToGojaValue(v any) goja.Value {
	// CRITICAL: Check if this is a wrapper for a preserved Goja Error object
	if wrapper, ok := v.(map[string]any); ok {
		if original, hasOriginal := wrapper["_originalError"]; hasOriginal {
			// This is a wrapped Goja Error - return the original
			if val, ok := original.(goja.Value); ok {
				return val
			}
		}
	}

	// Handle Goja Error objects directly (they're already Goja values)
	// If v is already a Goja value (not a Go-native type), return it directly
	if val, ok := v.(goja.Value); ok {
		return val
	}

	// Handle ChainedPromise objects by wrapping them
	// This preserves referential identity for Promise.reject(p) compliance
	// When a ChainedPromise is passed through (e.g., as rejection reason),
	// we must preserve it as-is, not unwrap its value/reason
	if p, ok := v.(*goeventloop.ChainedPromise); ok {
		// Wrap the ChainedPromise to preserve identity
		return a.GojaWrapPromise(p)
	}

	// Handle slices of Result (from combinators like All, Race, AllSettled, Any)
	if arr, ok := v.([]any); ok {
		jsArr := a.runtime.NewArray()
		for i, val := range arr {
			_ = jsArr.Set(strconv.Itoa(i), a.convertToGojaValue(val))
		}
		return jsArr
	}

	// Handle maps (from allSettled status objects)
	if m, ok := v.(map[string]any); ok {
		jsObj := a.runtime.NewObject()
		for key, val := range m {
			_ = jsObj.Set(key, a.convertToGojaValue(val))
		}
		return jsObj
	}

	// Handle *AggregateError specifically to enable checking err.message/err.errors in JS
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
		return a.GojaWrapPromise(chained)
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
		return a.GojaWrapPromise(chained)
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
		return a.GojaWrapPromise(chained)
	})

	promisePrototype.Set("then", thenFn)
	promisePrototype.Set("catch", catchFn)
	promisePrototype.Set("finally", finallyFn)

	// Get the Promise constructor and ensure it has the prototype
	promiseConstructorVal := a.runtime.Get("Promise")
	promiseConstructorObj := promiseConstructorVal.ToObject(a.runtime)

	// Set the prototype on the constructor (for `Promise.prototype` property)
	promiseConstructorObj.Set("prototype", promisePrototype)

	// Promise.resolve() with defensive null/undefined handling
	promiseConstructorObj.Set("resolve", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0)

		// Skip null/undefined - just return resolved promise
		if goja.IsNull(value) || goja.IsUndefined(value) {
			return a.GojaWrapPromise(a.js.Resolve(nil))
		}

		// Check if value is already our wrapped promise - return unchanged (identity semantics)
		// Promise.resolve(promise) === promise
		// Use helper to eliminate duplicated promise wrapper detection
		if isWrappedPromise(value) {
			return value
		}

		// Check for thenables
		if p := a.resolveThenable(value); p != nil {
			// It was a thenable, return the adopted promise
			return a.GojaWrapPromise(p)
		}

		// Otherwise create new resolved promise
		promise := a.js.Resolve(value.Export())
		return a.GojaWrapPromise(promise)
	}))

	// Promise.reject(reason)
	promiseConstructorObj.Set("reject", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		reason := call.Argument(0)

		// Preserve Goja Error objects without Export() to maintain .message property
		// When Export() converts Error objects, they become opaque wrappers losing .message
		if obj, ok := reason.(*goja.Object); ok && !goja.IsNull(reason) && !goja.IsUndefined(reason) {
			if nameVal := obj.Get("name"); nameVal != nil && !goja.IsUndefined(nameVal) {
				if nameStr, ok := nameVal.Export().(string); ok && (nameStr == "Error" || nameStr == "TypeError" || nameStr == "RangeError" || nameStr == "ReferenceError") {
					// This is an Error object - preserve original Goja.Value
					promise, _, reject := a.js.NewChainedPromise()
					reject(reason) // Reject with Goja.Error (preserves .message property)
					return a.GojaWrapPromise(promise)
				}
			}
		}

		// SPECIFICATION COMPLIANCE (Promise.reject promise object):
		// When reason is a wrapped promise object (with _internalPromise field),
		// we must preserve the wrapper object as the rejection reason per JS spec.
		// Export() on the wrapper returns a map, which would unwrap and lose identity.
		// CRITICAL: We must NOT call a.js.Reject(obj) as it triggers GojaWrapPromise again,
		// causing infinite recursion. Instead, create a new rejected promise directly.

		if isWrappedPromise(reason) {
			// Already a wrapped promise - create NEW rejected promise with wrapper as reason
			// This breaks infinite recursion by avoiding the extract → reject → wrap cycle
			promise, _, reject := a.js.NewChainedPromise()
			reject(reason) // Reject with the Goja Object (wrapper), not extracted promise

			wrapped := a.GojaWrapPromise(promise)
			return wrapped
		}

		// For all other types (primitives, plain objects), use Export()
		// This preserves properties like Error.message and custom fields
		promise := a.js.Reject(reason.Export())
		return a.GojaWrapPromise(promise)
	}))

	// Promise.all(iterable)
	promiseConstructorObj.Set("all", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		iterable := call.Argument(0)

		// Consume iterable using standard protocol
		arr, err := a.consumeIterable(iterable)
		if err != nil {
			// Reject promise on iterable error instead of panic
			// Iterator protocol errors should cause promise rejection, not Go panics
			// Per ES2021 spec: "If iterator.next() throws, the consuming operation should reject"
			return a.GojaWrapPromise(a.js.Reject(err))
		}

		// Extract wrapped promises before passing to All()
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
		return a.GojaWrapPromise(promise)
	}))

	// Promise.race(iterable)
	promiseConstructorObj.Set("race", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		iterable := call.Argument(0)

		// Consume iterable using standard protocol
		arr, err := a.consumeIterable(iterable)
		if err != nil {
			// Reject promise on iterable error instead of panic
			// Iterator protocol errors should cause promise rejection, not Go panics
			// Per ES2021 spec: "If iterator.next() throws, consuming operation should reject"
			return a.GojaWrapPromise(a.js.Reject(err))
		}

		// Extract wrapped promises before passing to Race()
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
		return a.GojaWrapPromise(promise)
	}))

	// Promise.allSettled(iterable)
	promiseConstructorObj.Set("allSettled", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		iterable := call.Argument(0)

		// Consume iterable using standard protocol
		arr, err := a.consumeIterable(iterable)
		if err != nil {
			// Reject promise on iterable error instead of panic
			// Iterator protocol errors should cause promise rejection, not Go panics
			// Per ES2021 spec: "If iterator.next() throws, consuming operation should reject"
			return a.GojaWrapPromise(a.js.Reject(err))
		}

		// Extract wrapped promises before passing to AllSettled()
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
		return a.GojaWrapPromise(promise)
	}))

	// Promise.any(iterable)
	promiseConstructorObj.Set("any", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		iterable := call.Argument(0)

		// Consume iterable using standard protocol
		arr, err := a.consumeIterable(iterable)
		if err != nil {
			// Reject promise on iterable error instead of panic
			// Iterator protocol errors should cause promise rejection, not Go panics
			// Per ES2021 spec: "If iterator.next() throws, consuming operation should reject"
			return a.GojaWrapPromise(a.js.Reject(err))
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
		return a.GojaWrapPromise(promise)
	}))

	// Promise.withResolvers() ES2024 API
	// Returns an object with { promise, resolve, reject } properties
	promiseConstructorObj.Set("withResolvers", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		// Use the Go implementation
		resolvers := a.js.WithResolvers()

		// Create JS object with { promise, resolve, reject }
		obj := a.runtime.NewObject()

		// Wrap the promise
		obj.Set("promise", a.GojaWrapPromise(resolvers.Promise))

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

	// Promise.try() ES2025 API
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

		return a.GojaWrapPromise(promise)
	}))

	return nil
}

// AbortController/AbortSignal Bindings

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

// AbortSignal Static Methods

// bindAbortSignalStatics adds static methods (any, timeout) to AbortSignal.
func (a *Adapter) bindAbortSignalStatics() error {
	// Get the AbortSignal constructor
	abortSignalVal := a.runtime.Get("AbortSignal")
	if abortSignalVal == nil || goja.IsUndefined(abortSignalVal) {
		return fmt.Errorf("AbortSignal not found")
	}
	abortSignalObj := abortSignalVal.ToObject(a.runtime)

	// AbortSignal.any(signals)
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

	// AbortSignal.timeout(ms)
	// Creates a signal that aborts after the specified timeout
	abortSignalObj.Set("timeout", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		delayMs := max(int(call.Argument(0).ToInteger()), 0)

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

// Performance API Bindings

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

// console.time/timeEnd/timeLog API

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
					strings.Join(dataStrs, " "))
			} else {
				fmt.Fprintf(output, "%s: %.3fms\n", label, float64(elapsed.Nanoseconds())/1e6)
			}
		}

		return goja.Undefined()
	}))

	// console.count(label) - logs count for label
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

	// console.countReset(label) - resets counter for label
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

	// console.assert(condition, ...data) - logs only when falsy
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
				fmt.Fprintf(output, "Assertion failed: %s\n", strings.Join(dataStrs, " "))
			} else {
				fmt.Fprintf(output, "Assertion failed\n")
			}
		}

		return goja.Undefined()
	}))

	// console.table() Implementation

	// console.table(data, columns?) - displays tabular data as an ASCII table
	// If data is an array, each element is a row
	// If data is an object, each property is a row
	// If columns is specified, only those columns are displayed
	consoleObj.Set("table", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		a.consoleTimersMu.RLock()
		output := a.consoleOutput
		a.consoleTimersMu.RUnlock()

		if output == nil {
			return goja.Undefined()
		}

		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}

		data := call.Argument(0)
		if goja.IsNull(data) || goja.IsUndefined(data) {
			fmt.Fprintf(output, "(index)\n")
			return goja.Undefined()
		}

		// Get columns filter (optional second argument)
		var columnFilter []string
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			colArg := call.Argument(1)
			// Try to parse as array of column names
			if arr, err := a.consumeIterable(colArg); err == nil {
				for _, v := range arr {
					columnFilter = append(columnFilter, v.String())
				}
			}
		}

		// Generate table
		tableStr := a.generateConsoleTable(data, columnFilter)
		a.consoleIndentMu.RLock()
		indent := a.consoleIndent
		a.consoleIndentMu.RUnlock()
		indentStr := a.getIndentString(indent)

		// Add indent to each line
		lines := strings.SplitSeq(tableStr, "\n")
		for line := range lines {
			if line != "" {
				fmt.Fprintf(output, "%s%s\n", indentStr, line)
			}
		}

		return goja.Undefined()
	}))

	// console.group/groupEnd/trace/clear/dir

	// console.group(label?) - starts a new indented group
	consoleObj.Set("group", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		a.consoleTimersMu.RLock()
		output := a.consoleOutput
		a.consoleTimersMu.RUnlock()

		// Get current indent for the label
		a.consoleIndentMu.RLock()
		indent := a.consoleIndent
		a.consoleIndentMu.RUnlock()

		// Print label if provided
		if output != nil {
			indentStr := a.getIndentString(indent)
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
				label := call.Argument(0).String()
				fmt.Fprintf(output, "%s▼ %s\n", indentStr, label)
			} else {
				fmt.Fprintf(output, "%s▼ console.group\n", indentStr)
			}
		}

		// Increase indent for subsequent calls
		a.consoleIndentMu.Lock()
		a.consoleIndent++
		a.consoleIndentMu.Unlock()

		return goja.Undefined()
	}))

	// console.groupCollapsed(label?) - same as group (no collapse in terminal)
	consoleObj.Set("groupCollapsed", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		a.consoleTimersMu.RLock()
		output := a.consoleOutput
		a.consoleTimersMu.RUnlock()

		// Get current indent for the label
		a.consoleIndentMu.RLock()
		indent := a.consoleIndent
		a.consoleIndentMu.RUnlock()

		// Print label if provided (same as group, just different visual marker)
		if output != nil {
			indentStr := a.getIndentString(indent)
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
				label := call.Argument(0).String()
				fmt.Fprintf(output, "%s▶ %s\n", indentStr, label)
			} else {
				fmt.Fprintf(output, "%s▶ console.group\n", indentStr)
			}
		}

		// Increase indent for subsequent calls
		a.consoleIndentMu.Lock()
		a.consoleIndent++
		a.consoleIndentMu.Unlock()

		return goja.Undefined()
	}))

	// console.groupEnd() - ends the current group and reduces indent
	consoleObj.Set("groupEnd", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		a.consoleIndentMu.Lock()
		if a.consoleIndent > 0 {
			a.consoleIndent--
		}
		a.consoleIndentMu.Unlock()

		return goja.Undefined()
	}))

	// console.trace(msg?) - prints a stack trace
	consoleObj.Set("trace", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		a.consoleTimersMu.RLock()
		output := a.consoleOutput
		a.consoleTimersMu.RUnlock()

		if output == nil {
			return goja.Undefined()
		}

		a.consoleIndentMu.RLock()
		indent := a.consoleIndent
		a.consoleIndentMu.RUnlock()
		indentStr := a.getIndentString(indent)

		// Print message if provided
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			msg := call.Argument(0).String()
			fmt.Fprintf(output, "%sTrace: %s\n", indentStr, msg)
		} else {
			fmt.Fprintf(output, "%sTrace\n", indentStr)
		}

		// Get stack trace from Goja
		// We create an Error object to capture the stack
		stack := a.runtime.CaptureCallStack(10, nil)
		for _, frame := range stack {
			funcName := frame.FuncName()
			if funcName == "" {
				funcName = "(anonymous)"
			}
			pos := frame.Position()
			// file.Position has Filename and Line but not Col (column is part of String())
			fmt.Fprintf(output, "%s    at %s (%s:%d)\n", indentStr, funcName, pos.Filename, pos.Line)
		}

		return goja.Undefined()
	}))

	// console.clear() - clears the console (prints newlines or ANSI clear)
	consoleObj.Set("clear", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		a.consoleTimersMu.RLock()
		output := a.consoleOutput
		a.consoleTimersMu.RUnlock()

		if output == nil {
			return goja.Undefined()
		}

		// Print a few newlines to simulate clearing
		// In a terminal, we could use ANSI escape codes, but for portability we use newlines
		fmt.Fprintf(output, "\n\n\n")

		return goja.Undefined()
	}))

	// console.dir(obj, options?) - displays an interactive listing of the properties of a specified object
	consoleObj.Set("dir", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		a.consoleTimersMu.RLock()
		output := a.consoleOutput
		a.consoleTimersMu.RUnlock()

		if output == nil {
			return goja.Undefined()
		}

		a.consoleIndentMu.RLock()
		indent := a.consoleIndent
		a.consoleIndentMu.RUnlock()
		indentStr := a.getIndentString(indent)

		if len(call.Arguments) == 0 {
			fmt.Fprintf(output, "%sundefined\n", indentStr)
			return goja.Undefined()
		}

		obj := call.Argument(0)
		if goja.IsUndefined(obj) {
			fmt.Fprintf(output, "%sundefined\n", indentStr)
			return goja.Undefined()
		}
		if goja.IsNull(obj) {
			fmt.Fprintf(output, "%snull\n", indentStr)
			return goja.Undefined()
		}

		// Try to serialize as JSON for object inspection
		exported := obj.Export()
		inspection := a.inspectValue(exported, 0, 2)
		lines := strings.SplitSeq(inspection, "\n")
		for line := range lines {
			if line != "" {
				fmt.Fprintf(output, "%s%s\n", indentStr, line)
			}
		}

		return goja.Undefined()
	}))

	return nil
}

// console.table() Helpers

// getIndentString returns a string of spaces for the current indentation level.
// Each level is 2 spaces.
func (a *Adapter) getIndentString(level int) string {
	if level <= 0 {
		return ""
	}
	var result strings.Builder
	for range level {
		result.WriteString("  ")
	}
	return result.String()
}

// generateConsoleTable generates an ASCII table from the given data.
func (a *Adapter) generateConsoleTable(data goja.Value, columnFilter []string) string {
	if data == nil || goja.IsNull(data) || goja.IsUndefined(data) {
		return "(index)"
	}

	exported := data.Export()

	// Check if it's an array-like structure
	if arr, ok := exported.([]any); ok {
		return a.generateTableFromArray(arr, columnFilter)
	}

	// Check if it's a map (object)
	if obj, ok := exported.(map[string]any); ok {
		return a.generateTableFromObject(obj, columnFilter)
	}

	// For primitives, just return the value as string
	return fmt.Sprintf("%v", exported)
}

// generateTableFromArray creates a table from an array.
func (a *Adapter) generateTableFromArray(arr []any, columnFilter []string) string {
	if len(arr) == 0 {
		return "(index)"
	}

	// Collect all column names from the data
	allColumns := make(map[string]bool)
	rows := make([]map[string]string, len(arr))

	for i, item := range arr {
		rows[i] = make(map[string]string)
		rows[i]["(index)"] = fmt.Sprintf("%d", i)

		if obj, ok := item.(map[string]any); ok {
			for k, v := range obj {
				allColumns[k] = true
				rows[i][k] = a.formatCellValue(v)
			}
		} else {
			// For non-object items, use "Values" as column name
			allColumns["Values"] = true
			rows[i]["Values"] = a.formatCellValue(item)
		}
	}

	// Build ordered column list
	columns := []string{"(index)"}
	if columnFilter != nil {
		// Use specified columns only if they exist
		for _, c := range columnFilter {
			if allColumns[c] {
				columns = append(columns, c)
			}
		}
	} else {
		// Use all collected columns in sorted order
		sortedCols := make([]string, 0, len(allColumns))
		for k := range allColumns {
			sortedCols = append(sortedCols, k)
		}
		// Simple sort for deterministic output
		for i := 0; i < len(sortedCols); i++ {
			for j := i + 1; j < len(sortedCols); j++ {
				if sortedCols[i] > sortedCols[j] {
					sortedCols[i], sortedCols[j] = sortedCols[j], sortedCols[i]
				}
			}
		}
		columns = append(columns, sortedCols...)
	}

	return a.renderTable(columns, rows)
}

// generateTableFromObject creates a table from an object.
func (a *Adapter) generateTableFromObject(obj map[string]any, columnFilter []string) string {
	if len(obj) == 0 {
		return "(index)"
	}

	// Collect all column names from nested objects
	allColumns := make(map[string]bool)
	rows := make([]map[string]string, 0, len(obj))

	// Sort keys for deterministic output
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	for _, k := range keys {
		v := obj[k]
		row := make(map[string]string)
		row["(index)"] = k

		if nested, ok := v.(map[string]any); ok {
			for nk, nv := range nested {
				allColumns[nk] = true
				row[nk] = a.formatCellValue(nv)
			}
		} else {
			// For non-object values, use "Values" as column name
			allColumns["Values"] = true
			row["Values"] = a.formatCellValue(v)
		}
		rows = append(rows, row)
	}

	// Build ordered column list
	columns := []string{"(index)"}
	if columnFilter != nil {
		// Use specified columns only if they exist
		for _, c := range columnFilter {
			if allColumns[c] {
				columns = append(columns, c)
			}
		}
	} else {
		// Use all collected columns in sorted order
		sortedCols := make([]string, 0, len(allColumns))
		for k := range allColumns {
			sortedCols = append(sortedCols, k)
		}
		for i := 0; i < len(sortedCols); i++ {
			for j := i + 1; j < len(sortedCols); j++ {
				if sortedCols[i] > sortedCols[j] {
					sortedCols[i], sortedCols[j] = sortedCols[j], sortedCols[i]
				}
			}
		}
		columns = append(columns, sortedCols...)
	}

	return a.renderTable(columns, rows)
}

// formatCellValue formats a value for display in a table cell.
func (a *Adapter) formatCellValue(v any) string {
	if v == nil {
		return "null"
	}

	switch val := v.(type) {
	case []any:
		return "Array(" + fmt.Sprintf("%d", len(val)) + ")"
	case map[string]any:
		return "Object"
	case string:
		return val
	case bool:
		return fmt.Sprintf("%v", val)
	case float64:
		// Format numbers nicely (no trailing zeros for integers)
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case int:
		return fmt.Sprintf("%d", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// renderTable renders the table as ASCII.
func (a *Adapter) renderTable(columns []string, rows []map[string]string) string {
	if len(columns) == 0 {
		return ""
	}

	// Calculate column widths
	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = len(col)
	}
	for _, row := range rows {
		for i, col := range columns {
			cellLen := len(row[col])
			if cellLen > widths[i] {
				widths[i] = cellLen
			}
		}
	}

	var result strings.Builder

	// Build separator line
	separatorParts := make([]string, len(columns))
	for i, w := range widths {
		separatorParts[i] = strings.Repeat("─", w+2)
	}
	separator := "├" + strings.Join(separatorParts, "┼") + "┤"

	// Build top border
	topParts := make([]string, len(columns))
	for i, w := range widths {
		topParts[i] = strings.Repeat("─", w+2)
	}
	topBorder := "┌" + strings.Join(topParts, "┬") + "┐"

	// Build bottom border
	bottomParts := make([]string, len(columns))
	for i, w := range widths {
		bottomParts[i] = strings.Repeat("─", w+2)
	}
	bottomBorder := "└" + strings.Join(bottomParts, "┴") + "┘"

	// Top border
	result.WriteString(topBorder)
	result.WriteString("\n")

	// Header row
	headerParts := make([]string, len(columns))
	for i, col := range columns {
		headerParts[i] = a.padRight(col, widths[i])
	}
	result.WriteString("│ ")
	result.WriteString(strings.Join(headerParts, " │ "))
	result.WriteString(" │\n")

	// Separator
	result.WriteString(separator)
	result.WriteString("\n")

	// Data rows
	for _, row := range rows {
		cellParts := make([]string, len(columns))
		for i, col := range columns {
			cellParts[i] = a.padRight(row[col], widths[i])
		}
		result.WriteString("│ ")
		result.WriteString(strings.Join(cellParts, " │ "))
		result.WriteString(" │\n")
	}

	// Bottom border
	result.WriteString(bottomBorder)

	return result.String()
}

// padRight pads a string to the right with spaces.
func (a *Adapter) padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// console.dir() Helper

// inspectValue creates a human-readable representation of a value.
// maxDepth controls how deep to inspect nested objects.
func (a *Adapter) inspectValue(v any, depth int, maxDepth int) string {
	if v == nil {
		return "null"
	}

	indent := strings.Repeat("  ", depth)
	nextIndent := strings.Repeat("  ", depth+1)

	switch val := v.(type) {
	case []any:
		if depth >= maxDepth {
			return fmt.Sprintf("Array(%d)", len(val))
		}
		if len(val) == 0 {
			return "[]"
		}
		var parts []string
		for i, item := range val {
			parts = append(parts, fmt.Sprintf("%s%d: %s", nextIndent, i, a.inspectValue(item, depth+1, maxDepth)))
		}
		return "[\n" + strings.Join(parts, ",\n") + "\n" + indent + "]"

	case map[string]any:
		if depth >= maxDepth {
			return "Object"
		}
		if len(val) == 0 {
			return "{}"
		}
		// Sort keys for deterministic output
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		for i := 0; i < len(keys); i++ {
			for j := i + 1; j < len(keys); j++ {
				if keys[i] > keys[j] {
					keys[i], keys[j] = keys[j], keys[i]
				}
			}
		}
		var parts []string
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s%s: %s", nextIndent, k, a.inspectValue(val[k], depth+1, maxDepth)))
		}
		return "{\n" + strings.Join(parts, ",\n") + "\n" + indent + "}"

	case string:
		return fmt.Sprintf("'%s'", val)

	case bool:
		return fmt.Sprintf("%v", val)

	case float64:
		// Format numbers nicely (no trailing zeros for integers)
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)

	case int64:
		return fmt.Sprintf("%d", val)

	case int:
		return fmt.Sprintf("%d", val)

	default:
		return fmt.Sprintf("%v", val)
	}
}

// process.nextTick() Binding

// bindProcess creates the process object with nextTick method.
// This emulates Node.js process.nextTick() semantics.
func (a *Adapter) bindProcess() error {
	// Get or create process object
	processVal := a.runtime.Get("process")
	var processObj *goja.Object
	if processVal == nil || goja.IsUndefined(processVal) {
		// No process object exists, create one
		processObj = a.runtime.NewObject()
		a.runtime.Set("process", processObj)
	} else {
		// process object exists, extend it
		processObj = processVal.ToObject(a.runtime)
	}

	// process.nextTick(fn) - schedules fn to run before microtasks
	processObj.Set("nextTick", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		fn := call.Argument(0)
		if fn.Export() == nil {
			panic(a.runtime.NewTypeError("process.nextTick requires a function as first argument"))
		}

		fnCallable, ok := goja.AssertFunction(fn)
		if !ok {
			panic(a.runtime.NewTypeError("process.nextTick requires a function as first argument"))
		}

		// Use the Go NextTick implementation
		err := a.js.NextTick(func() {
			_, _ = fnCallable(goja.Undefined())
		})
		if err != nil {
			panic(a.runtime.NewGoError(err))
		}

		return goja.Undefined()
	}))

	return nil
}

// delay() Promise Helper

// delay returns a promise that resolves after the specified delay.
// This is similar to setTimeout but returns a promise for async/await patterns.
func (a *Adapter) delay(call goja.FunctionCall) goja.Value {
	delayMs := max(int(call.Argument(0).ToInteger()), 0)

	// Use the Go Sleep implementation
	promise := a.js.Sleep(time.Duration(delayMs) * time.Millisecond)
	return a.GojaWrapPromise(promise)
}

// crypto.randomUUID() Binding

// bindCrypto creates the crypto object with randomUUID and getRandomValues methods.
func (a *Adapter) bindCrypto() error {
	// Get or create crypto object
	cryptoVal := a.runtime.Get("crypto")
	var cryptoObj *goja.Object
	if cryptoVal == nil || goja.IsUndefined(cryptoVal) {
		// No crypto object exists, create one
		cryptoObj = a.runtime.NewObject()
		a.runtime.Set("crypto", cryptoObj)
	} else {
		// crypto object exists, extend it
		cryptoObj = cryptoVal.ToObject(a.runtime)
	}

	// crypto.randomUUID() - returns a secure UUID v4 string
	cryptoObj.Set("randomUUID", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		uuid, err := generateUUIDv4()
		if err != nil {
			panic(a.runtime.NewGoError(err))
		}
		return a.runtime.ToValue(uuid)
	}))

	// crypto.getRandomValues(typedArray) - fills TypedArray with cryptographically random values
	// Spec: https://www.w3.org/TR/WebCryptoAPI/#Crypto-method-getRandomValues
	// Accepts any integer TypedArray (Int8Array, Uint8Array, Int16Array, Uint16Array,
	// Int32Array, Uint32Array, Uint8ClampedArray, BigInt64Array, BigUint64Array).
	// Throws TypeError if the argument is not an integer TypedArray.
	// Throws a DOMException with name "QuotaExceededError" if byteLength > 65536.
	// Returns the same TypedArray that was passed in.
	cryptoObj.Set("getRandomValues", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(a.runtime.NewTypeError("Failed to execute 'getRandomValues' on 'Crypto': 1 argument required, but only 0 present."))
		}

		arg := call.Argument(0)
		if goja.IsUndefined(arg) || goja.IsNull(arg) {
			panic(a.runtime.NewTypeError("Failed to execute 'getRandomValues' on 'Crypto': parameter 1 is not of type 'ArrayBufferView'."))
		}

		obj := arg.ToObject(a.runtime)

		// Verify it's a TypedArray: must have 'buffer', 'byteLength', 'byteOffset', and 'BYTES_PER_ELEMENT'
		bufferVal := obj.Get("buffer")
		byteLengthVal := obj.Get("byteLength")
		bytesPerElementVal := obj.Get("BYTES_PER_ELEMENT")
		if bufferVal == nil || goja.IsUndefined(bufferVal) ||
			byteLengthVal == nil || goja.IsUndefined(byteLengthVal) ||
			bytesPerElementVal == nil || goja.IsUndefined(bytesPerElementVal) {
			panic(a.runtime.NewTypeError("Failed to execute 'getRandomValues' on 'Crypto': parameter 1 is not of type 'ArrayBufferView'."))
		}

		// Reject Float32Array and Float64Array (only integer typed arrays allowed)
		bytesPerElement := int(bytesPerElementVal.ToInteger())
		constructorObj := obj.Get("constructor")
		if constructorObj != nil && !goja.IsUndefined(constructorObj) {
			nameVal := constructorObj.ToObject(a.runtime).Get("name")
			if nameVal != nil {
				name := nameVal.String()
				if name == "Float32Array" || name == "Float64Array" {
					panic(a.runtime.NewTypeError("Failed to execute 'getRandomValues' on 'Crypto': parameter 1 is not of type 'ArrayBufferView'."))
				}
			}
		}

		byteLength := int(byteLengthVal.ToInteger())

		// QuotaExceededError if byteLength > 65536
		if byteLength > 65536 {
			panic(a.throwDOMException("QuotaExceededError", "Failed to execute 'getRandomValues' on 'Crypto': The ArrayBufferView's byte length ("+strconv.Itoa(byteLength)+") exceeds the number of bytes of entropy available via this API (65536)."))
		}

		// Get the underlying ArrayBuffer and fill with random bytes
		bufferObj := bufferVal.ToObject(a.runtime)
		exported := bufferObj.Export()

		if ab, ok := exported.(goja.ArrayBuffer); ok {
			// Direct access to backing slice
			backingSlice := ab.Bytes()
			byteOffset := 0
			if offsetVal := obj.Get("byteOffset"); offsetVal != nil && !goja.IsUndefined(offsetVal) {
				byteOffset = int(offsetVal.ToInteger())
			}

			// Fill the relevant portion with random bytes
			if byteOffset+byteLength <= len(backingSlice) {
				_, err := rand.Read(backingSlice[byteOffset : byteOffset+byteLength])
				if err != nil {
					panic(a.runtime.NewGoError(fmt.Errorf("crypto.getRandomValues: failed to generate random bytes: %w", err)))
				}
			}
		} else {
			// Fallback: write random bytes element by element
			randomBytes := make([]byte, byteLength)
			_, err := rand.Read(randomBytes)
			if err != nil {
				panic(a.runtime.NewGoError(fmt.Errorf("crypto.getRandomValues: failed to generate random bytes: %w", err)))
			}

			length := int(obj.Get("length").ToInteger())
			for i := range length {
				// Reconstruct the integer value from the random bytes
				offset := i * bytesPerElement
				var val int64
				for b := 0; b < bytesPerElement && offset+b < len(randomBytes); b++ {
					val |= int64(randomBytes[offset+b]) << (8 * b) // little-endian
				}
				obj.Set(strconv.Itoa(i), val)
			}
		}

		// Return the same typed array
		return arg
	}))

	return nil
}

// generateUUIDv4 generates a cryptographically secure UUID v4 string.
// Format: "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx" where y is 8, 9, a, or b.
func generateUUIDv4() (string, error) {
	var uuid [16]byte
	_, err := rand.Read(uuid[:])
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Set version (4) and variant bits per RFC 4122
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant 1

	// Format as standard UUID string
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		uuid[0], uuid[1], uuid[2], uuid[3],
		uuid[4], uuid[5],
		uuid[6], uuid[7],
		uuid[8], uuid[9],
		uuid[10], uuid[11], uuid[12], uuid[13], uuid[14], uuid[15]), nil
}

// atob/btoa Base64 Functions

// btoa encodes a string to base64.
// This follows the browser's btoa() semantics.
// Each character's code point (0-255) becomes a single byte.
func (a *Adapter) btoa(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		panic(a.runtime.NewTypeError("btoa requires a string argument"))
	}

	input := call.Argument(0).String()

	// btoa in browsers only accepts Latin-1 characters (0x00-0xFF)
	// Each character's code point becomes a single byte
	runes := []rune(input)
	bytes := make([]byte, len(runes))
	for i, r := range runes {
		if r > 0xFF {
			panic(a.runtime.NewTypeError("btoa: The string to be encoded contains characters outside of the Latin1 range"))
		}
		bytes[i] = byte(r)
	}

	encoded := base64.StdEncoding.EncodeToString(bytes)
	return a.runtime.ToValue(encoded)
}

// atob decodes a base64 string.
// This follows the browser's atob() semantics.
// Each byte in the decoded data becomes a character with that code point.
func (a *Adapter) atob(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		panic(a.runtime.NewTypeError("atob requires a string argument"))
	}

	input := call.Argument(0).String()

	decoded, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		// atob throws DOMException with name "InvalidCharacterError"
		// We simulate this with a TypeError since Goja doesn't have DOMException
		panic(a.runtime.NewTypeError("atob: The string to be decoded is not correctly encoded"))
	}

	// Each byte becomes a character with that code point (Latin-1 semantics)
	runes := make([]rune, len(decoded))
	for i, b := range decoded {
		runes[i] = rune(b)
	}

	return a.runtime.ToValue(string(runes))
}

// EventTarget and Event Bindings

// eventTargetListenerInfo tracks listener info for Symbol-based identity removal.
// This enables proper RemoveEventListener implementation in JavaScript where
// the same function reference is used to add and remove listeners.
type eventTargetListenerInfo struct {
	symbol goja.Value // Unique Symbol for listener identity
	id     goeventloop.ListenerID
}

// eventTargetWrapper wraps an EventTarget with Goja-specific state.
type eventTargetWrapper struct {
	target    *goeventloop.EventTarget
	listeners map[string][]eventTargetListenerInfo // eventType -> listener infos
	mu        sync.Mutex
}

// eventTargetConstructor creates the EventTarget constructor for JavaScript.
func (a *Adapter) eventTargetConstructor(call goja.ConstructorCall) *goja.Object {
	target := goeventloop.NewEventTarget()

	wrapper := &eventTargetWrapper{
		target:    target,
		listeners: make(map[string][]eventTargetListenerInfo),
	}

	thisObj := call.This

	// Store the native wrapper
	thisObj.Set("_wrapper", wrapper)

	// addEventListener(type, listener, options?)
	thisObj.Set("addEventListener", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		eventType := call.Argument(0).String()
		listener := call.Argument(1)

		if listener.Export() == nil {
			return goja.Undefined()
		}

		fnCallable, ok := goja.AssertFunction(listener)
		if !ok {
			return goja.Undefined()
		}

		// Check for options object (once)
		once := false
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
			opts := call.Argument(2).ToObject(a.runtime)
			if opts != nil {
				if onceVal := opts.Get("once"); onceVal != nil && onceVal.ToBoolean() {
					once = true
				}
			}
		}

		// Create a unique Symbol for this listener (for RemoveEventListener identity)
		symbolVal, _ := a.runtime.RunString("Symbol()")

		// Add listener and track info
		var id goeventloop.ListenerID
		if once {
			id = target.AddEventListenerOnce(eventType, func(e *goeventloop.Event) {
				// Use the original JS event object if available (preserves CustomEvent.detail, etc.)
				var jsEvent goja.Value
				if stored, ok := a.dispatchJSEvents.Load(e); ok {
					jsEvent = stored.(goja.Value)
				} else {
					// Fallback: create JS event object
					jsEvent = a.wrapEvent(e)
				}
				_, _ = fnCallable(goja.Undefined(), jsEvent)
			})
		} else {
			id = target.AddEventListener(eventType, func(e *goeventloop.Event) {
				// Use the original JS event object if available (preserves CustomEvent.detail, etc.)
				var jsEvent goja.Value
				if stored, ok := a.dispatchJSEvents.Load(e); ok {
					jsEvent = stored.(goja.Value)
				} else {
					// Fallback: create JS event object
					jsEvent = a.wrapEvent(e)
				}
				_, _ = fnCallable(goja.Undefined(), jsEvent)
			})
		}

		wrapper.mu.Lock()
		// Store using the listener's Goja Value identity
		wrapper.listeners[eventType] = append(wrapper.listeners[eventType], eventTargetListenerInfo{
			id:     id,
			symbol: symbolVal,
		})
		// Also store symbol on the listener function for lookup
		if obj, ok := listener.(*goja.Object); ok {
			obj.Set("_eventListenerSymbol_"+eventType, symbolVal)
		}
		wrapper.mu.Unlock()

		return goja.Undefined()
	}))

	// removeEventListener(type, listener, options?)
	thisObj.Set("removeEventListener", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		eventType := call.Argument(0).String()
		listener := call.Argument(1)

		if listener.Export() == nil {
			return goja.Undefined()
		}

		// Get the symbol stored on the listener function
		var symbolVal goja.Value
		if obj, ok := listener.(*goja.Object); ok {
			symbolVal = obj.Get("_eventListenerSymbol_" + eventType)
		}

		if symbolVal == nil || goja.IsUndefined(symbolVal) {
			return goja.Undefined()
		}

		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()

		infos := wrapper.listeners[eventType]
		for i, info := range infos {
			if info.symbol == symbolVal {
				target.RemoveEventListenerByID(eventType, info.id)
				// Remove from tracking
				wrapper.listeners[eventType] = append(infos[:i], infos[i+1:]...)
				break
			}
		}

		return goja.Undefined()
	}))

	// dispatchEvent(event)
	thisObj.Set("dispatchEvent", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		eventArg := call.Argument(0)
		if eventArg.Export() == nil || goja.IsUndefined(eventArg) || goja.IsNull(eventArg) {
			panic(a.runtime.NewTypeError("dispatchEvent requires an Event"))
		}

		eventObj := eventArg.ToObject(a.runtime)
		if eventObj == nil {
			panic(a.runtime.NewTypeError("dispatchEvent requires an Event"))
		}

		// Get the internal event
		internalVal := eventObj.Get("_event")
		if internalVal == nil || goja.IsUndefined(internalVal) {
			panic(a.runtime.NewTypeError("dispatchEvent requires an Event"))
		}

		event, ok := internalVal.Export().(*goeventloop.Event)
		if !ok || event == nil {
			panic(a.runtime.NewTypeError("dispatchEvent requires an Event"))
		}

		// Store the JS event object so listeners can use the original (with CustomEvent.detail, etc.)
		a.dispatchJSEvents.Store(event, eventArg)
		defer a.dispatchJSEvents.Delete(event)

		result := target.DispatchEvent(event)
		return a.runtime.ToValue(result)
	}))

	return thisObj
}

// eventConstructor creates the Event constructor for JavaScript.
func (a *Adapter) eventConstructor(call goja.ConstructorCall) *goja.Object {
	if len(call.Arguments) == 0 {
		panic(a.runtime.NewTypeError("Event requires a type argument"))
	}

	eventType := call.Argument(0).String()

	// Parse options
	bubbles := false
	cancelable := false
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
		opts := call.Argument(1).ToObject(a.runtime)
		if opts != nil {
			if v := opts.Get("bubbles"); v != nil && v.ToBoolean() {
				bubbles = true
			}
			if v := opts.Get("cancelable"); v != nil && v.ToBoolean() {
				cancelable = true
			}
		}
	}

	event := goeventloop.NewEventWithOptions(eventType, bubbles, cancelable)
	a.wrapEventWithObject(event, call.This)
	return call.This
}

// wrapEvent creates a new JS object for an Event.
func (a *Adapter) wrapEvent(event *goeventloop.Event) goja.Value {
	obj := a.runtime.NewObject()
	return a.wrapEventWithObject(event, obj)
}

// wrapEventWithObject wraps an Event using the provided JS object.
func (a *Adapter) wrapEventWithObject(event *goeventloop.Event, obj *goja.Object) goja.Value {
	// Store internal event
	obj.Set("_event", event)

	// type property (readonly)
	obj.DefineAccessorProperty("type", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.runtime.ToValue(event.Type)
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// target property (readonly)
	obj.DefineAccessorProperty("target", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if event.Target == nil {
			return goja.Null()
		}
		// We could wrap the target, but for simplicity return null
		// (the target is set during dispatch anyway)
		return goja.Null()
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// bubbles property (readonly)
	obj.DefineAccessorProperty("bubbles", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.runtime.ToValue(event.Bubbles)
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// cancelable property (readonly)
	obj.DefineAccessorProperty("cancelable", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.runtime.ToValue(event.Cancelable)
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// defaultPrevented property (readonly)
	obj.DefineAccessorProperty("defaultPrevented", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.runtime.ToValue(event.DefaultPrevented)
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// preventDefault()
	obj.Set("preventDefault", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		event.PreventDefault()
		return goja.Undefined()
	}))

	// stopPropagation()
	obj.Set("stopPropagation", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		event.StopPropagation()
		return goja.Undefined()
	}))

	// stopImmediatePropagation()
	obj.Set("stopImmediatePropagation", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		event.StopImmediatePropagation()
		return goja.Undefined()
	}))

	return obj
}

// CustomEvent Binding

// customEventConstructor creates the CustomEvent constructor for JavaScript.
func (a *Adapter) customEventConstructor(call goja.ConstructorCall) *goja.Object {
	if len(call.Arguments) == 0 {
		panic(a.runtime.NewTypeError("CustomEvent requires a type argument"))
	}

	eventType := call.Argument(0).String()

	// Parse options
	bubbles := false
	cancelable := false
	var detail any

	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
		opts := call.Argument(1).ToObject(a.runtime)
		if opts != nil {
			if v := opts.Get("bubbles"); v != nil && v.ToBoolean() {
				bubbles = true
			}
			if v := opts.Get("cancelable"); v != nil && v.ToBoolean() {
				cancelable = true
			}
			if v := opts.Get("detail"); v != nil && !goja.IsUndefined(v) {
				// Store the Goja value directly for JavaScript access
				detail = v
			}
		}
	}

	customEvent := goeventloop.NewCustomEventWithOptions(eventType, detail, bubbles, cancelable)

	thisObj := call.This

	// Wrap the embedded Event
	a.wrapEventWithObject(customEvent.EventPtr(), thisObj)

	// Override _event to store the CustomEvent's Event pointer
	thisObj.Set("_event", customEvent.EventPtr())

	// detail property (readonly)
	thisObj.DefineAccessorProperty("detail", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		d := customEvent.Detail()
		if d == nil {
			return goja.Null()
		}
		// If detail is already a goja.Value, return it directly
		if v, ok := d.(goja.Value); ok {
			return v
		}
		return a.runtime.ToValue(d)
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	return thisObj
}

// structuredClone() Global Function

// structuredClone implements the HTML structured clone algorithm.
// It performs a deep clone of the value, handling:
// - Primitives (pass-through)
// - Objects (deep clone)
// - Arrays (deep clone)
// - Date (preserve as Date)
// - RegExp (preserve as RegExp)
// - Map (preserve as Map)
// - Set (preserve as Set)
// - null/undefined (pass-through)
// - Throws TypeError for non-cloneable types like functions
func (a *Adapter) structuredClone(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		return goja.Undefined()
	}

	value := call.Argument(0)

	// Create a visited map for circular reference detection
	// Uses object identity (pointer address) as key
	visited := make(map[uintptr]goja.Value)

	return a.structuredCloneValue(value, visited)
}

// structuredCloneValue recursively clones a value.
// The visited map tracks object references to handle circular structures.
func (a *Adapter) structuredCloneValue(value goja.Value, visited map[uintptr]goja.Value) goja.Value {
	// Handle null and undefined (pass-through)
	if value == nil || goja.IsNull(value) {
		return goja.Null()
	}
	if goja.IsUndefined(value) {
		return goja.Undefined()
	}

	// Handle primitives (string, number, boolean, bigint, symbol)
	// primitives are immutable and don't need cloning
	exportType := value.ExportType()
	if exportType == nil {
		// If ExportType() returns nil, it's likely undefined or an opaque type
		return value
	}

	switch exportType.Kind().String() {
	case "string", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "bool":
		// Primitives are immutable - return as-is
		return value
	}

	// For objects, we need to check type and handle specially
	obj, ok := value.(*goja.Object)
	if !ok {
		// Primitive or cannot convert - return as-is
		return value
	}

	// Get object identity for circular reference detection
	// Use the hash code from the object's string representation as a proxy for identity
	// This is a workaround since Goja doesn't expose direct object identity
	objPtr := getObjectIdentity(obj)

	// Check if we've already cloned this object (circular reference)
	if cloned, exists := visited[objPtr]; exists {
		return cloned
	}

	// Check for non-cloneable types first (functions)
	if isFunction(obj) {
		panic(a.runtime.NewTypeError("structuredClone: cannot clone functions"))
	}

	// Handle different object types
	return a.cloneObject(obj, objPtr, visited)
}

// getObjectIdentity returns a unique identifier for a Goja object.
// Since Goja doesn't expose direct object identity, we use the hash of the object pointer.
func getObjectIdentity(obj *goja.Object) uintptr {
	// Use fmt.Sprintf to get a unique string representation of the object's address
	// This is a workaround since Goja objects don't expose their internal identity directly
	// We use the object's pointer address (through reflection-like inspection)
	//
	// Actually, we can use the goja.Object's internal identity by storing it
	// We'll use a simpler approach: use the object's hashCode from JS if available,
	// or fall back to a string hash of the object reference
	addr := fmt.Sprintf("%p", obj)
	// Convert string to uintptr via simple hash
	var hash uintptr
	for _, c := range addr {
		hash = hash*31 + uintptr(c)
	}
	return hash
}

// isFunction checks if a Goja object is a function.
func isFunction(obj *goja.Object) bool {
	// Check if it's callable
	_, ok := goja.AssertFunction(obj)
	return ok
}

// cloneObject clones a Goja object based on its type.
func (a *Adapter) cloneObject(obj *goja.Object, objPtr uintptr, visited map[uintptr]goja.Value) goja.Value {
	// Check for built-in types that need special handling

	// 1. Check for Date
	if a.isDateObject(obj) {
		return a.cloneDate(obj, objPtr, visited)
	}

	// 2. Check for RegExp
	if a.isRegExpObject(obj) {
		return a.cloneRegExp(obj, objPtr, visited)
	}

	// 3. Check for Map
	if a.isMapObject(obj) {
		return a.cloneMap(obj, objPtr, visited)
	}

	// 4. Check for Set
	if a.isSetObject(obj) {
		return a.cloneSet(obj, objPtr, visited)
	}

	// 5. Check for Array
	if a.isArrayObject(obj) {
		return a.cloneArray(obj, objPtr, visited)
	}

	// 6. Check for Error objects - throw TypeError per spec
	if a.isErrorObject(obj) {
		panic(a.runtime.NewTypeError("structuredClone: cannot clone Error objects"))
	}

	// 7. Default: plain object
	return a.clonePlainObject(obj, objPtr, visited)
}

// isDateObject checks if a Goja object is a Date instance.
func (a *Adapter) isDateObject(obj *goja.Object) bool {
	// Check using instanceof-like check via prototype chain
	// We check if obj has getTime method (Date-specific method)
	getTimeVal := obj.Get("getTime")
	if getTimeVal == nil || goja.IsUndefined(getTimeVal) {
		return false
	}
	_, ok := goja.AssertFunction(getTimeVal)
	if !ok {
		return false
	}

	// Also verify constructor name
	constructorVal := obj.Get("constructor")
	if constructorVal == nil || goja.IsUndefined(constructorVal) {
		return false
	}
	constructorObj := constructorVal.ToObject(a.runtime)
	if constructorObj == nil {
		return false
	}
	nameVal := constructorObj.Get("name")
	if nameVal == nil {
		return false
	}
	return nameVal.String() == "Date"
}

// cloneDate clones a Date object.
func (a *Adapter) cloneDate(obj *goja.Object, objPtr uintptr, visited map[uintptr]goja.Value) goja.Value {
	// Get the time value using getTime()
	getTimeFn, _ := goja.AssertFunction(obj.Get("getTime"))
	timeVal, _ := getTimeFn(obj)
	milliseconds := timeVal.ToInteger()

	// Create a new Date with the same time using JS: new Date(ms)
	script := fmt.Sprintf("new Date(%d)", milliseconds)
	newDate, err := a.runtime.RunString(script)
	if err != nil {
		// Fallback: return the time value as-is
		return timeVal
	}

	// Register in visited map
	visited[objPtr] = newDate

	return newDate
}

// isRegExpObject checks if a Goja object is a RegExp instance.
func (a *Adapter) isRegExpObject(obj *goja.Object) bool {
	// Check if obj has test method and source property (RegExp-specific)
	testVal := obj.Get("test")
	if testVal == nil || goja.IsUndefined(testVal) {
		return false
	}
	_, ok := goja.AssertFunction(testVal)
	if !ok {
		return false
	}

	sourceVal := obj.Get("source")
	if sourceVal == nil || goja.IsUndefined(sourceVal) {
		return false
	}

	// Verify constructor name
	constructorVal := obj.Get("constructor")
	if constructorVal == nil || goja.IsUndefined(constructorVal) {
		return false
	}
	constructorObj := constructorVal.ToObject(a.runtime)
	if constructorObj == nil {
		return false
	}
	nameVal := constructorObj.Get("name")
	if nameVal == nil {
		return false
	}
	return nameVal.String() == "RegExp"
}

// cloneRegExp clones a RegExp object.
func (a *Adapter) cloneRegExp(obj *goja.Object, objPtr uintptr, visited map[uintptr]goja.Value) goja.Value {
	// Get source and flags
	source := obj.Get("source").String()
	flags := obj.Get("flags").String()

	// Create new RegExp using JS: new RegExp(source, flags)
	// Escape source for JS string (handle backslashes and quotes)
	escapedSource := escapeJSString(source)
	script := fmt.Sprintf("new RegExp(%q, %q)", escapedSource, flags)
	newRegexp, err := a.runtime.RunString(script)
	if err != nil {
		// Fallback: try without escaping
		script = "new RegExp('" + source + "', '" + flags + "')"
		newRegexp, err = a.runtime.RunString(script)
	}
	if err != nil {
		// Both attempts failed; return the original object to avoid nil
		visited[objPtr] = obj
		return obj
	}

	// Register in visited map
	visited[objPtr] = newRegexp

	return newRegexp
}

// escapeJSString escapes a string for use in a JS string literal.
func escapeJSString(s string) string {
	// Replace backslashes and quotes
	result := strings.ReplaceAll(s, "\\", "\\\\")
	result = strings.ReplaceAll(result, "'", "\\'")
	result = strings.ReplaceAll(result, "\"", "\\\"")
	result = strings.ReplaceAll(result, "\n", "\\n")
	result = strings.ReplaceAll(result, "\r", "\\r")
	result = strings.ReplaceAll(result, "\t", "\\t")
	return result
}

// isMapObject checks if a Goja object is a Map instance.
func (a *Adapter) isMapObject(obj *goja.Object) bool {
	// Check for Map-specific methods: get, set, has, delete
	getVal := obj.Get("get")
	setVal := obj.Get("set")
	hasVal := obj.Get("has")
	deleteVal := obj.Get("delete")

	if getVal == nil || setVal == nil || hasVal == nil || deleteVal == nil {
		return false
	}

	// Verify constructor name
	constructorVal := obj.Get("constructor")
	if constructorVal == nil || goja.IsUndefined(constructorVal) {
		return false
	}
	constructorObj := constructorVal.ToObject(a.runtime)
	if constructorObj == nil {
		return false
	}
	nameVal := constructorObj.Get("name")
	if nameVal == nil {
		return false
	}
	name := nameVal.String()
	return name == "Map"
}

// cloneMap clones a Map object.
func (a *Adapter) cloneMap(obj *goja.Object, objPtr uintptr, visited map[uintptr]goja.Value) goja.Value {
	// Create a new Map using JS: new Map()
	newMapVal, err := a.runtime.RunString("new Map()")
	if err != nil {
		return goja.Undefined()
	}
	newMapObj := newMapVal.ToObject(a.runtime)

	// Register in visited map BEFORE iterating to handle circular references
	visited[objPtr] = newMapVal

	// Get the set method for the new map
	setMethodVal := newMapObj.Get("set")
	if setMethodVal == nil || goja.IsUndefined(setMethodVal) {
		return newMapVal
	}
	setMethod, ok := goja.AssertFunction(setMethodVal)
	if !ok {
		return newMapVal
	}

	// Iterate over entries using forEach
	forEachVal := obj.Get("forEach")
	if forEachVal == nil || goja.IsUndefined(forEachVal) {
		return newMapVal
	}
	forEachFn, ok := goja.AssertFunction(forEachVal)
	if !ok {
		return newMapVal
	}

	_, _ = forEachFn(obj, a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0)
		key := call.Argument(1)

		// Clone key and value
		clonedKey := a.structuredCloneValue(key, visited)
		clonedValue := a.structuredCloneValue(value, visited)

		// Add to new map
		_, _ = setMethod(newMapObj, clonedKey, clonedValue)

		return goja.Undefined()
	}))

	return newMapVal
}

// isSetObject checks if a Goja object is a Set instance.
func (a *Adapter) isSetObject(obj *goja.Object) bool {
	// Check for Set-specific methods: add, has, delete (but not get like Map)
	addVal := obj.Get("add")
	hasVal := obj.Get("has")
	deleteVal := obj.Get("delete")
	getVal := obj.Get("get")

	// Set has add, has, delete but NOT get (Map has get)
	if addVal == nil || hasVal == nil || deleteVal == nil {
		return false
	}
	// If it has get, it's likely a Map
	if getVal != nil && !goja.IsUndefined(getVal) {
		_, ok := goja.AssertFunction(getVal)
		if ok {
			return false // Has get method, so it's a Map
		}
	}

	// Verify constructor name
	constructorVal := obj.Get("constructor")
	if constructorVal == nil || goja.IsUndefined(constructorVal) {
		return false
	}
	constructorObj := constructorVal.ToObject(a.runtime)
	if constructorObj == nil {
		return false
	}
	nameVal := constructorObj.Get("name")
	if nameVal == nil {
		return false
	}
	name := nameVal.String()
	return name == "Set"
}

// cloneSet clones a Set object.
func (a *Adapter) cloneSet(obj *goja.Object, objPtr uintptr, visited map[uintptr]goja.Value) goja.Value {
	// Create a new Set using JS: new Set()
	newSetVal, err := a.runtime.RunString("new Set()")
	if err != nil {
		return goja.Undefined()
	}
	newSetObj := newSetVal.ToObject(a.runtime)

	// Register in visited map BEFORE iterating to handle circular references
	visited[objPtr] = newSetVal

	// Get the add method for the new set
	addMethodVal := newSetObj.Get("add")
	if addMethodVal == nil || goja.IsUndefined(addMethodVal) {
		return newSetVal
	}
	addMethod, ok := goja.AssertFunction(addMethodVal)
	if !ok {
		return newSetVal
	}

	// Iterate over entries using forEach
	forEachVal := obj.Get("forEach")
	if forEachVal == nil || goja.IsUndefined(forEachVal) {
		return newSetVal
	}
	forEachFn, ok := goja.AssertFunction(forEachVal)
	if !ok {
		return newSetVal
	}

	_, _ = forEachFn(obj, a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0)

		// Clone value
		clonedValue := a.structuredCloneValue(value, visited)

		// Add to new set
		_, _ = addMethod(newSetObj, clonedValue)

		return goja.Undefined()
	}))

	return newSetVal
}

// isArrayObject checks if a Goja object is an Array.
func (a *Adapter) isArrayObject(obj *goja.Object) bool {
	// Check for length property and Array constructor
	lengthVal := obj.Get("length")
	if lengthVal == nil || goja.IsUndefined(lengthVal) {
		return false
	}

	// Check with Array.isArray if available
	arrayVal := a.runtime.Get("Array")
	if arrayVal == nil || goja.IsUndefined(arrayVal) {
		return false
	}
	arrayObj := arrayVal.ToObject(a.runtime)
	isArrayFn := arrayObj.Get("isArray")
	if isArrayFn == nil || goja.IsUndefined(isArrayFn) {
		return false
	}
	isArrayCallable, ok := goja.AssertFunction(isArrayFn)
	if !ok {
		return false
	}
	result, _ := isArrayCallable(goja.Undefined(), obj)
	return result.ToBoolean()
}

// cloneArray clones an Array object.
func (a *Adapter) cloneArray(obj *goja.Object, objPtr uintptr, visited map[uintptr]goja.Value) goja.Value {
	length := int(obj.Get("length").ToInteger())

	// Create a new array
	newArr := a.runtime.NewArray()

	// Register in visited map BEFORE iterating to handle circular references
	visited[objPtr] = newArr

	// Clone each element
	for i := range length {
		indexStr := strconv.Itoa(i)
		element := obj.Get(indexStr)
		if element != nil && !goja.IsUndefined(element) {
			clonedElement := a.structuredCloneValue(element, visited)
			_ = newArr.Set(indexStr, clonedElement)
		}
	}

	return newArr
}

// isErrorObject checks if a Goja object is an Error instance.
func (a *Adapter) isErrorObject(obj *goja.Object) bool {
	// Check for Error properties: name, message, stack
	nameVal := obj.Get("name")
	messageVal := obj.Get("message")

	if nameVal == nil || messageVal == nil {
		return false
	}

	name := nameVal.String()
	return name == "Error" || name == "TypeError" || name == "RangeError" ||
		name == "ReferenceError" || name == "SyntaxError" || name == "URIError" ||
		name == "EvalError" || name == "AggregateError"
}

// clonePlainObject clones a plain JavaScript object.
func (a *Adapter) clonePlainObject(obj *goja.Object, objPtr uintptr, visited map[uintptr]goja.Value) goja.Value {
	// Create a new object
	newObj := a.runtime.NewObject()

	// Register in visited map BEFORE iterating to handle circular references
	visited[objPtr] = newObj

	// Get all enumerable own properties using Object.keys
	objectVal := a.runtime.Get("Object")
	if objectVal == nil || goja.IsUndefined(objectVal) {
		return newObj
	}
	objectObj := objectVal.ToObject(a.runtime)
	keysFn := objectObj.Get("keys")
	if keysFn == nil || goja.IsUndefined(keysFn) {
		return newObj
	}
	keysCallable, ok := goja.AssertFunction(keysFn)
	if !ok {
		return newObj
	}

	keysResult, err := keysCallable(goja.Undefined(), obj)
	if err != nil {
		return newObj
	}

	keysArr := keysResult.ToObject(a.runtime)
	keysLength := int(keysArr.Get("length").ToInteger())

	for i := range keysLength {
		keyVal := keysArr.Get(strconv.Itoa(i))
		key := keyVal.String()

		value := obj.Get(key)
		if value != nil && !goja.IsUndefined(value) {
			// Check if value is a function - skip functions
			if valObj, ok := value.(*goja.Object); ok && isFunction(valObj) {
				// Skip functions (they're not cloneable)
				continue
			}
			clonedValue := a.structuredCloneValue(value, visited)
			_ = newObj.Set(key, clonedValue)
		}
	}

	return newObj
}

// URL and URLSearchParams APIs

// urlConstructor creates the URL constructor for JavaScript.
// Implements the WHATWG URL Standard.
func (a *Adapter) urlConstructor(call goja.ConstructorCall) *goja.Object {
	if len(call.Arguments) == 0 {
		panic(a.runtime.NewTypeError("URL constructor requires a URL string"))
	}

	urlStr := call.Argument(0).String()

	// Handle optional base URL
	var baseURL *url.URL
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
		baseStr := call.Argument(1).String()
		var err error
		baseURL, err = url.Parse(baseStr)
		if err != nil {
			panic(a.runtime.NewTypeError("Invalid base URL: " + baseStr))
		}
	}

	// Parse the URL
	var parsedURL *url.URL
	var err error
	if baseURL != nil {
		parsedURL, err = baseURL.Parse(urlStr)
	} else {
		parsedURL, err = url.Parse(urlStr)
	}
	if err != nil {
		panic(a.runtime.NewTypeError("Invalid URL: " + urlStr))
	}

	// Validate that we have a valid URL with scheme
	if parsedURL.Scheme == "" {
		if baseURL == nil {
			panic(a.runtime.NewTypeError("Invalid URL: " + urlStr))
		}
	}

	thisObj := call.This
	a.wrapURLWithObject(parsedURL, thisObj)
	return thisObj
}

// wrapURLWithObject wraps a url.URL in a Goja object.
func (a *Adapter) wrapURLWithObject(parsedURL *url.URL, obj *goja.Object) {
	// Store the parsed URL for mutation
	urlData := &urlWrapper{url: parsedURL}
	obj.Set("_url", urlData)

	// href property (read/write)
	obj.DefineAccessorProperty("href",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(urlData.url.String())
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			newURL, err := url.Parse(call.Argument(0).String())
			if err != nil {
				panic(a.runtime.NewTypeError("Invalid URL"))
			}
			urlData.url = newURL
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE)

	// origin property (readonly)
	obj.DefineAccessorProperty("origin",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			scheme := urlData.url.Scheme
			host := urlData.url.Host
			if scheme == "" || host == "" {
				return a.runtime.ToValue("null")
			}
			return a.runtime.ToValue(scheme + "://" + host)
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// protocol property (read/write)
	obj.DefineAccessorProperty("protocol",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(urlData.url.Scheme + ":")
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			proto := call.Argument(0).String()
			proto = strings.TrimSuffix(proto, ":")
			urlData.url.Scheme = proto
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE)

	// host property (read/write) - hostname:port or just hostname
	obj.DefineAccessorProperty("host",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(urlData.url.Host)
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			urlData.url.Host = call.Argument(0).String()
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE)

	// hostname property (read/write) - just the hostname without port
	obj.DefineAccessorProperty("hostname",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			hostname := urlData.url.Hostname()
			return a.runtime.ToValue(hostname)
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			newHostname := call.Argument(0).String()
			port := urlData.url.Port()
			if port != "" {
				urlData.url.Host = newHostname + ":" + port
			} else {
				urlData.url.Host = newHostname
			}
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE)

	// port property (read/write)
	obj.DefineAccessorProperty("port",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(urlData.url.Port())
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			newPort := call.Argument(0).String()
			hostname := urlData.url.Hostname()
			if newPort != "" {
				urlData.url.Host = hostname + ":" + newPort
			} else {
				urlData.url.Host = hostname
			}
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE)

	// pathname property (read/write)
	obj.DefineAccessorProperty("pathname",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			path := urlData.url.Path
			if path == "" {
				path = "/"
			}
			return a.runtime.ToValue(path)
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			urlData.url.Path = call.Argument(0).String()
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE)

	// search property (read/write) - includes leading ?
	obj.DefineAccessorProperty("search",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			rawQuery := urlData.url.RawQuery
			if rawQuery == "" {
				return a.runtime.ToValue("")
			}
			return a.runtime.ToValue("?" + rawQuery)
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			search := call.Argument(0).String()
			search = strings.TrimPrefix(search, "?")
			urlData.url.RawQuery = search
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE)

	// hash property (read/write) - includes leading #
	obj.DefineAccessorProperty("hash",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			fragment := urlData.url.Fragment
			if fragment == "" {
				return a.runtime.ToValue("")
			}
			return a.runtime.ToValue("#" + fragment)
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			hash := call.Argument(0).String()
			hash = strings.TrimPrefix(hash, "#")
			urlData.url.Fragment = hash
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE)

	// username property (read/write)
	obj.DefineAccessorProperty("username",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			if urlData.url.User == nil {
				return a.runtime.ToValue("")
			}
			return a.runtime.ToValue(urlData.url.User.Username())
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			username := call.Argument(0).String()
			password, hasPassword := "", false
			if urlData.url.User != nil {
				password, hasPassword = urlData.url.User.Password()
			}
			if hasPassword {
				urlData.url.User = url.UserPassword(username, password)
			} else {
				urlData.url.User = url.User(username)
			}
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE)

	// password property (read/write)
	obj.DefineAccessorProperty("password",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			if urlData.url.User == nil {
				return a.runtime.ToValue("")
			}
			password, _ := urlData.url.User.Password()
			return a.runtime.ToValue(password)
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			password := call.Argument(0).String()
			username := ""
			if urlData.url.User != nil {
				username = urlData.url.User.Username()
			}
			urlData.url.User = url.UserPassword(username, password)
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE)

	// searchParams property (readonly) - returns a URLSearchParams object
	obj.DefineAccessorProperty("searchParams",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			// Create URLSearchParams that is linked to this URL
			return a.createLinkedURLSearchParams(urlData)
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// toString() method
	obj.Set("toString", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.runtime.ToValue(urlData.url.String())
	}))

	// toJSON() method
	obj.Set("toJSON", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.runtime.ToValue(urlData.url.String())
	}))
}

// urlWrapper holds a mutable URL for the URL class.
type urlWrapper struct {
	url *url.URL
}

// createLinkedURLSearchParams creates a URLSearchParams linked to a URL.
func (a *Adapter) createLinkedURLSearchParams(urlData *urlWrapper) goja.Value {
	obj := a.runtime.NewObject()

	// Store the linked URL
	obj.Set("_linkedURL", urlData)

	a.addURLSearchParamsMethods(obj, urlData)
	return obj
}

// urlSearchParamsConstructor creates the URLSearchParams constructor for JavaScript.
func (a *Adapter) urlSearchParamsConstructor(call goja.ConstructorCall) *goja.Object {
	thisObj := call.This

	// Initialize with empty or parsed query string
	var params url.Values = make(url.Values)

	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		arg := call.Argument(0)

		// Check if it's a string
		if exportType := arg.ExportType(); exportType != nil && exportType.Kind().String() == "string" {
			queryStr := arg.String()
			queryStr = strings.TrimPrefix(queryStr, "?")
			parsed, err := url.ParseQuery(queryStr)
			if err == nil {
				params = parsed
			}
		} else if obj, ok := arg.(*goja.Object); ok {
			// Check if it's an iterable of pairs or an object
			// Try to iterate as array-like first
			if arr, err := a.consumeIterable(arg); err == nil {
				// Array of [key, value] pairs
				for _, pair := range arr {
					pairObj := pair.ToObject(a.runtime)
					if pairObj != nil {
						length := pairObj.Get("length")
						if length != nil && length.ToInteger() >= 2 {
							key := pairObj.Get("0").String()
							value := pairObj.Get("1").String()
							params.Add(key, value)
						}
					}
				}
			} else {
				// Treat as plain object
				keys := obj.Keys()
				for _, key := range keys {
					val := obj.Get(key)
					if val != nil && !goja.IsUndefined(val) {
						params.Add(key, val.String())
					}
				}
			}
		}
	}

	// Store params wrapper
	paramsWrapper := &urlSearchParamsWrapper{params: params}
	thisObj.Set("_params", paramsWrapper)

	a.addURLSearchParamsMethods(thisObj, nil)
	return thisObj
}

// urlSearchParamsPair represents a key-value pair in URLSearchParams.
type urlSearchParamsPair struct {
	key   string
	value string
}

// urlSearchParamsWrapper holds mutable URL search params with order tracking.
type urlSearchParamsWrapper struct {
	params       url.Values
	orderedPairs []urlSearchParamsPair // nil means use map iteration, non-nil means use this order
}

// addURLSearchParamsMethods adds all URLSearchParams methods to an object.
func (a *Adapter) addURLSearchParamsMethods(obj *goja.Object, linkedURL *urlWrapper) {
	// Helper to get the wrapper
	getWrapper := func() *urlSearchParamsWrapper {
		wrapper := obj.Get("_params")
		if wrapper != nil {
			if w, ok := wrapper.Export().(*urlSearchParamsWrapper); ok {
				return w
			}
		}
		return nil
	}

	// Helper to get/update params
	getParams := func() url.Values {
		if linkedURL != nil {
			return linkedURL.url.Query()
		}
		if w := getWrapper(); w != nil {
			return w.params
		}
		return make(url.Values)
	}

	setParams := func(params url.Values) {
		if linkedURL != nil {
			linkedURL.url.RawQuery = params.Encode()
		} else {
			if w := getWrapper(); w != nil {
				w.params = params
			}
		}
	}

	// Helper to get ordered pairs for iteration.
	// Returns pairs in order (sorted order if sort() was called, otherwise from orderedPairs or map).
	getOrderedPairs := func() []urlSearchParamsPair {
		if linkedURL != nil {
			// For linked URLs, build pairs from query params
			params := linkedURL.url.Query()
			var pairs []urlSearchParamsPair
			for key, values := range params {
				for _, value := range values {
					pairs = append(pairs, urlSearchParamsPair{key, value})
				}
			}
			return pairs
		}
		if w := getWrapper(); w != nil {
			if w.orderedPairs != nil {
				return w.orderedPairs
			}
			// Build pairs from map (order not guaranteed)
			var pairs []urlSearchParamsPair
			for key, values := range w.params {
				for _, value := range values {
					pairs = append(pairs, urlSearchParamsPair{key, value})
				}
			}
			return pairs
		}
		return nil
	}

	// Helper to set ordered pairs (called by sort)
	setOrderedPairs := func(pairs []urlSearchParamsPair) {
		if linkedURL != nil {
			// For linked URLs, rebuild the query string in sorted order
			newParams := make(url.Values)
			for _, pair := range pairs {
				newParams.Add(pair.key, pair.value)
			}
			linkedURL.url.RawQuery = newParams.Encode()
		} else {
			if w := getWrapper(); w != nil {
				w.orderedPairs = pairs
				// Also rebuild the params map
				newParams := make(url.Values)
				for _, pair := range pairs {
					newParams.Add(pair.key, pair.value)
				}
				w.params = newParams
			}
		}
	}

	// Helper to clear ordered pairs (called by mutating operations)
	clearOrderedPairs := func() {
		if linkedURL == nil {
			if w := getWrapper(); w != nil {
				w.orderedPairs = nil
			}
		}
	}

	// append(name, value)
	obj.Set("append", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(a.runtime.NewTypeError("URLSearchParams.append requires 2 arguments"))
		}
		name := call.Argument(0).String()
		value := call.Argument(1).String()
		params := getParams()
		params.Add(name, value)
		setParams(params)
		clearOrderedPairs()
		return goja.Undefined()
	}))

	// delete(name, value?)
	obj.Set("delete", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(a.runtime.NewTypeError("URLSearchParams.delete requires at least 1 argument"))
		}
		name := call.Argument(0).String()
		params := getParams()

		// If value is provided, only delete matching key-value pairs
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			value := call.Argument(1).String()
			values := params[name]
			newValues := make([]string, 0, len(values))
			for _, v := range values {
				if v != value {
					newValues = append(newValues, v)
				}
			}
			if len(newValues) > 0 {
				params[name] = newValues
			} else {
				delete(params, name)
			}
		} else {
			delete(params, name)
		}
		setParams(params)
		clearOrderedPairs()
		return goja.Undefined()
	}))

	// get(name)
	obj.Set("get", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(a.runtime.NewTypeError("URLSearchParams.get requires 1 argument"))
		}
		name := call.Argument(0).String()
		params := getParams()
		value := params.Get(name)
		if value == "" && !params.Has(name) {
			return goja.Null()
		}
		return a.runtime.ToValue(value)
	}))

	// getAll(name)
	obj.Set("getAll", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(a.runtime.NewTypeError("URLSearchParams.getAll requires 1 argument"))
		}
		name := call.Argument(0).String()
		params := getParams()
		values := params[name]
		arr := a.runtime.NewArray()
		for i, v := range values {
			_ = arr.Set(strconv.Itoa(i), v)
		}
		return arr
	}))

	// has(name, value?)
	obj.Set("has", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(a.runtime.NewTypeError("URLSearchParams.has requires at least 1 argument"))
		}
		name := call.Argument(0).String()
		params := getParams()

		if !params.Has(name) {
			return a.runtime.ToValue(false)
		}

		// If value is provided, check for specific value
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			value := call.Argument(1).String()
			if slices.Contains(params[name], value) {
				return a.runtime.ToValue(true)
			}
			return a.runtime.ToValue(false)
		}

		return a.runtime.ToValue(true)
	}))

	// set(name, value)
	obj.Set("set", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(a.runtime.NewTypeError("URLSearchParams.set requires 2 arguments"))
		}
		name := call.Argument(0).String()
		value := call.Argument(1).String()
		params := getParams()
		params.Set(name, value)
		setParams(params)
		clearOrderedPairs()
		return goja.Undefined()
	}))

	// toString()
	obj.Set("toString", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		params := getParams()
		return a.runtime.ToValue(params.Encode())
	}))

	// sort() - sorts all key-value pairs by name
	obj.Set("sort", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		// Get all key-value pairs
		pairs := getOrderedPairs()

		// Sort by key
		sort.SliceStable(pairs, func(i, j int) bool {
			return pairs[i].key < pairs[j].key
		})

		// Store the sorted pairs
		setOrderedPairs(pairs)
		return goja.Undefined()
	}))

	// keys() - returns an iterator over keys
	obj.Set("keys", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		pairs := getOrderedPairs()
		var keys []string
		for _, pair := range pairs {
			keys = append(keys, pair.key)
		}
		return a.createIterator(keys)
	}))

	// values() - returns an iterator over values
	obj.Set("values", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		pairs := getOrderedPairs()
		var values []string
		for _, pair := range pairs {
			values = append(values, pair.value)
		}
		return a.createIterator(values)
	}))

	// entries() - returns an iterator over [key, value] pairs
	obj.Set("entries", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		pairs := getOrderedPairs()
		var entries []goja.Value
		for _, pair := range pairs {
			arr := a.runtime.NewArray()
			_ = arr.Set("0", pair.key)
			_ = arr.Set("1", pair.value)
			entries = append(entries, arr)
		}
		return a.createValueIterator(entries)
	}))

	// forEach(callback, thisArg?)
	obj.Set("forEach", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(a.runtime.NewTypeError("URLSearchParams.forEach requires a callback"))
		}
		callback := call.Argument(0)
		callbackFn, ok := goja.AssertFunction(callback)
		if !ok {
			panic(a.runtime.NewTypeError("URLSearchParams.forEach requires a function"))
		}

		thisArg := goja.Undefined()
		if len(call.Arguments) > 1 {
			thisArg = call.Argument(1)
		}

		pairs := getOrderedPairs()
		for _, pair := range pairs {
			_, _ = callbackFn(thisArg,
				a.runtime.ToValue(pair.value),
				a.runtime.ToValue(pair.key),
				obj)
		}
		return goja.Undefined()
	}))

	// size property (getter)
	obj.DefineAccessorProperty("size",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			params := getParams()
			count := 0
			for _, values := range params {
				count += len(values)
			}
			return a.runtime.ToValue(count)
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
}

// createIterator creates a simple iterator for strings.
func (a *Adapter) createIterator(items []string) goja.Value {
	idx := 0
	iterator := a.runtime.NewObject()
	iterator.Set("next", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		result := a.runtime.NewObject()
		if idx >= len(items) {
			result.Set("done", true)
			result.Set("value", goja.Undefined())
		} else {
			result.Set("done", false)
			result.Set("value", items[idx])
			idx++
		}
		return result
	}))

	// Add Symbol.iterator that returns the iterator itself
	// Per JS iterator protocol, iterators should also be iterable
	// Use JavaScript to set Symbol-keyed property (Goja Set() only accepts strings)
	a.runtime.Set("__tempIterator", iterator)
	_, _ = a.runtime.RunString(`__tempIterator[Symbol.iterator] = function() { return this; }`)
	a.runtime.Set("__tempIterator", goja.Undefined())

	return iterator
}

// createValueIterator creates an iterator for arbitrary values.
func (a *Adapter) createValueIterator(items []goja.Value) goja.Value {
	idx := 0
	iterator := a.runtime.NewObject()
	iterator.Set("next", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		result := a.runtime.NewObject()
		if idx >= len(items) {
			result.Set("done", true)
			result.Set("value", goja.Undefined())
		} else {
			result.Set("done", false)
			result.Set("value", items[idx])
			idx++
		}
		return result
	}))

	// Add Symbol.iterator that returns the iterator itself
	// Per JS iterator protocol, iterators should also be iterable
	// Use JavaScript to set Symbol-keyed property (Goja Set() only accepts strings)
	a.runtime.Set("__tempIterator", iterator)
	_, _ = a.runtime.RunString(`__tempIterator[Symbol.iterator] = function() { return this; }`)
	a.runtime.Set("__tempIterator", goja.Undefined())

	return iterator
}

// TextEncoder and TextDecoder APIs

// textEncoderConstructor creates the TextEncoder constructor for JavaScript.
// TextEncoder always uses UTF-8 encoding per WHATWG Encoding Standard.
func (a *Adapter) textEncoderConstructor(call goja.ConstructorCall) *goja.Object {
	thisObj := call.This

	// encoding property (readonly) - always "utf-8"
	thisObj.DefineAccessorProperty("encoding",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue("utf-8")
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// encode(string) - returns Uint8Array
	thisObj.Set("encode", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		input := ""
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			input = call.Argument(0).String()
		}

		// Convert string to UTF-8 bytes
		bytes := []byte(input)

		// Create Uint8Array by running JS code to use proper 'new' semantics
		// Create the Uint8Array from the bytes

		// Wrap ArrayBuffer in a Uint8Array view
		uint8ArrayCtorVal := a.runtime.Get("Uint8Array")
		if uint8ArrayCtorVal == nil || goja.IsUndefined(uint8ArrayCtorVal) {
			panic(a.runtime.NewTypeError("Uint8Array not available"))
		}

		// Use NewObject with prototype to construct properly
		script := fmt.Sprintf("new Uint8Array(%d)", len(bytes))
		arr, err := a.runtime.RunString(script)
		if err != nil {
			panic(a.runtime.NewGoError(err))
		}

		arrObjTyped := arr.ToObject(a.runtime)

		// Fill the array with byte values
		for i, b := range bytes {
			_ = arrObjTyped.Set(strconv.Itoa(i), int(b))
		}

		return arr
	}))

	// encodeInto(source, destination) - encodes into existing Uint8Array
	thisObj.Set("encodeInto", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(a.runtime.NewTypeError("TextEncoder.encodeInto requires 2 arguments"))
		}

		source := ""
		if !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			source = call.Argument(0).String()
		}

		dest := call.Argument(1).ToObject(a.runtime)
		if dest == nil {
			panic(a.runtime.NewTypeError("destination must be a Uint8Array"))
		}

		destLength := int(dest.Get("length").ToInteger())

		// Write as much as fits
		written := 0
		runeCount := 0 // Track rune count separately (not byte index)
		for _, r := range source {
			runeBytes := []byte(string(r))
			if written+len(runeBytes) > destLength {
				break
			}
			for j, b := range runeBytes {
				_ = dest.Set(strconv.Itoa(written+j), int(b))
			}
			written += len(runeBytes)
			runeCount++ // Increment rune count after each character
		}

		// Return { read, written }
		result := a.runtime.NewObject()
		result.Set("read", runeCount) // Use rune count, not byte index
		result.Set("written", written)

		return result
	}))

	return thisObj
}

// textDecoderConstructor creates the TextDecoder constructor for JavaScript.
// Supports UTF-8 encoding by default.
func (a *Adapter) textDecoderConstructor(call goja.ConstructorCall) *goja.Object {
	thisObj := call.This

	// Get encoding (default: utf-8)
	encoding := "utf-8"
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		encoding = strings.ToLower(call.Argument(0).String())
	}

	// Normalize encoding name
	switch encoding {
	case "utf8", "utf-8":
		encoding = "utf-8"
	default:
		// Only UTF-8 is supported (per WHATWG, TextDecoder must support UTF-8)
		// Other encodings can be added later
		panic(a.runtime.NewTypeError("TextDecoder: unsupported encoding: " + encoding))
	}

	// Get options
	fatal := false
	ignoreBOM := false
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
		opts := call.Argument(1).ToObject(a.runtime)
		if opts != nil {
			if v := opts.Get("fatal"); v != nil && v.ToBoolean() {
				fatal = true
			}
			if v := opts.Get("ignoreBOM"); v != nil && v.ToBoolean() {
				ignoreBOM = true
			}
		}
	}

	// Store decoder options
	decoder := &textDecoderWrapper{
		encoding:  encoding,
		fatal:     fatal,
		ignoreBOM: ignoreBOM,
	}
	thisObj.Set("_decoder", decoder)

	// encoding property (readonly)
	thisObj.DefineAccessorProperty("encoding",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(decoder.encoding)
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// fatal property (readonly)
	thisObj.DefineAccessorProperty("fatal",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(decoder.fatal)
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// ignoreBOM property (readonly)
	thisObj.DefineAccessorProperty("ignoreBOM",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(decoder.ignoreBOM)
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// decode(input?, options?) - decodes input to string
	thisObj.Set("decode", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		// If no input, return empty string
		if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return a.runtime.ToValue("")
		}

		input := call.Argument(0)

		// Get bytes from typed array or ArrayBuffer
		bytes, err := a.extractBytes(input)
		if err != nil {
			if decoder.fatal {
				panic(a.runtime.NewTypeError("TextDecoder.decode: " + err.Error()))
			}
			return a.runtime.ToValue("")
		}

		// Handle BOM
		if !decoder.ignoreBOM && len(bytes) >= 3 {
			if bytes[0] == 0xEF && bytes[1] == 0xBB && bytes[2] == 0xBF {
				bytes = bytes[3:] // Skip UTF-8 BOM
			}
		}

		// Decode UTF-8
		result := string(bytes)

		return a.runtime.ToValue(result)
	}))

	return thisObj
}

// textDecoderWrapper holds TextDecoder state.
type textDecoderWrapper struct {
	encoding  string
	fatal     bool
	ignoreBOM bool
}

// extractBytes extracts byte slice from Uint8Array, ArrayBuffer, or other typed arrays.
func (a *Adapter) extractBytes(input goja.Value) ([]byte, error) {
	obj := input.ToObject(a.runtime)
	if obj == nil {
		return nil, fmt.Errorf("input must be a BufferSource")
	}

	// Check for ArrayBuffer
	byteLength := obj.Get("byteLength")
	if byteLength != nil && !goja.IsUndefined(byteLength) {
		length := int(byteLength.ToInteger())

		// Check if it's a typed array view (has buffer property)
		buffer := obj.Get("buffer")
		if buffer != nil && !goja.IsUndefined(buffer) {
			// It's a typed array view
			// Read bytes from the view (index access accounts for byteOffset)
			viewLength := length
			if lenVal := obj.Get("length"); lenVal != nil && !goja.IsUndefined(lenVal) {
				viewLength = int(lenVal.ToInteger())
			}

			bytes := make([]byte, viewLength)
			for i := 0; i < viewLength; i++ {
				val := obj.Get(strconv.Itoa(i))
				if val != nil && !goja.IsUndefined(val) {
					bytes[i] = byte(val.ToInteger() & 0xFF)
				}
			}
			return bytes, nil
		}

		// It's an ArrayBuffer - create a Uint8Array view
		uint8ArrayVal := a.runtime.Get("Uint8Array")
		if uint8ArrayVal != nil && !goja.IsUndefined(uint8ArrayVal) {
			constructorFn, ok := goja.AssertFunction(uint8ArrayVal)
			if ok {
				view, err := constructorFn(goja.Undefined(), input)
				if err == nil {
					return a.extractBytes(view)
				}
			}
		}
	}

	// Check for array-like object with numeric indices
	lengthVal := obj.Get("length")
	if lengthVal != nil && !goja.IsUndefined(lengthVal) {
		length := int(lengthVal.ToInteger())
		bytes := make([]byte, length)
		for i := range length {
			val := obj.Get(strconv.Itoa(i))
			if val != nil && !goja.IsUndefined(val) {
				bytes[i] = byte(val.ToInteger() & 0xFF)
			}
		}
		return bytes, nil
	}

	return nil, fmt.Errorf("input must be a BufferSource")
}

// Blob API

// blobWrapper holds internal Blob data.
type blobWrapper struct {
	mimeType string
	data     []byte
}

// blobConstructor creates the Blob constructor for JavaScript.
// Implements the WHATWG File API Blob interface.
// new Blob(blobParts?, options?)
// - blobParts: An optional array of data parts (strings, ArrayBuffer, Uint8Array, Blob)
// - options: { type?: string, endings?: "transparent"|"native" }
func (a *Adapter) blobConstructor(call goja.ConstructorCall) *goja.Object {
	thisObj := call.This

	// Collect data from blobParts
	var data []byte
	mimeType := ""

	// Parse blobParts (first argument)
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		parts, err := a.consumeIterable(call.Argument(0))
		if err != nil {
			panic(a.runtime.NewTypeError("Blob constructor requires an iterable for blobParts"))
		}

		for _, part := range parts {
			// Handle different part types
			partBytes, err := a.blobPartToBytes(part)
			if err != nil {
				panic(a.runtime.NewTypeError(err.Error()))
			}
			data = append(data, partBytes...)
		}
	}

	// Parse options (second argument)
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
		opts := call.Argument(1).ToObject(a.runtime)
		if opts != nil {
			if typeVal := opts.Get("type"); typeVal != nil && !goja.IsUndefined(typeVal) {
				mimeType = strings.ToLower(typeVal.String())
			}
			// endings option is typically "transparent" (default) or "native"
			// We ignore it for now as it's mainly for line ending conversion
		}
	}

	blob := &blobWrapper{
		data:     data,
		mimeType: mimeType,
	}
	thisObj.Set("_blob", blob)

	// size property (readonly)
	thisObj.DefineAccessorProperty("size",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(len(blob.data))
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// type property (readonly)
	thisObj.DefineAccessorProperty("type",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(blob.mimeType)
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// text() - returns Promise<string>
	thisObj.Set("text", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		// Return a resolved promise with the text content
		text := string(blob.data)
		promise := a.js.Resolve(text)
		return a.GojaWrapPromise(promise)
	}))

	// arrayBuffer() - returns Promise<ArrayBuffer>
	thisObj.Set("arrayBuffer", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		// Create an ArrayBuffer containing the blob data
		// We need to return this via a promise
		promise, resolve, _ := a.js.NewChainedPromise()

		// Create ArrayBuffer via Goja
		arrayBuffer := a.runtime.NewArrayBuffer(blob.data)

		// Resolve the promise with the ArrayBuffer
		// We need to wrap this specially to avoid Export() issues
		resolve(arrayBuffer)

		return a.GojaWrapPromise(promise)
	}))

	// slice(start?, end?, contentType?) - returns a new Blob
	thisObj.Set("slice", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		dataLen := len(blob.data)
		start := 0
		end := dataLen
		contentType := ""

		// Parse start
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			start = int(call.Argument(0).ToInteger())
			// Handle negative values
			if start < 0 {
				start = max(dataLen+start, 0)
			}
			if start > dataLen {
				start = dataLen
			}
		}

		// Parse end
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			end = int(call.Argument(1).ToInteger())
			// Handle negative values
			if end < 0 {
				end = max(dataLen+end, 0)
			}
			if end > dataLen {
				end = dataLen
			}
		}

		// Ensure start <= end
		if start > end {
			start = end
		}

		// Parse contentType
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
			contentType = strings.ToLower(call.Argument(2).String())
		}

		// Create sliced data
		var slicedData []byte
		if start < end {
			slicedData = make([]byte, end-start)
			copy(slicedData, blob.data[start:end])
		}

		// Create new Blob object
		newBlob := &blobWrapper{
			data:     slicedData,
			mimeType: contentType,
		}

		// Create new Blob JS object
		blobObj := a.runtime.NewObject()
		a.wrapBlobWithObject(newBlob, blobObj)
		return blobObj
	}))

	// stream() - returns undefined (ReadableStream not implemented)
	//
	// ReadableStream is intentionally NOT implemented. The full Streams API
	// (https://streams.spec.whatwg.org/) requires backpressure management,
	// queueing strategies, and a controller model that would add significant
	// complexity for minimal practical benefit in this embedded JS context.
	//
	// Alternatives for consuming Blob data:
	//   - blob.text()        → returns a Promise<string> of the Blob's UTF-8 content
	//   - blob.arrayBuffer() → returns a Promise<ArrayBuffer> of the raw bytes
	//   - blob.slice()       → returns a new Blob representing a subset of the data
	//
	// These cover the vast majority of Blob consumption use cases.
	thisObj.Set("stream", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	}))

	return thisObj
}

// wrapBlobWithObject wraps a blobWrapper in a Goja object (for slice()).
func (a *Adapter) wrapBlobWithObject(blob *blobWrapper, obj *goja.Object) {
	obj.Set("_blob", blob)

	// size property (readonly)
	obj.DefineAccessorProperty("size",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(len(blob.data))
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// type property (readonly)
	obj.DefineAccessorProperty("type",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(blob.mimeType)
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// text() - returns Promise<string>
	obj.Set("text", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		text := string(blob.data)
		promise := a.js.Resolve(text)
		return a.GojaWrapPromise(promise)
	}))

	// arrayBuffer() - returns Promise<ArrayBuffer>
	obj.Set("arrayBuffer", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		promise, resolve, _ := a.js.NewChainedPromise()
		arrayBuffer := a.runtime.NewArrayBuffer(blob.data)
		resolve(arrayBuffer)
		return a.GojaWrapPromise(promise)
	}))

	// slice(start?, end?, contentType?) - returns a new Blob
	obj.Set("slice", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		dataLen := len(blob.data)
		start := 0
		end := dataLen
		contentType := ""

		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			start = int(call.Argument(0).ToInteger())
			if start < 0 {
				start = max(dataLen+start, 0)
			}
			if start > dataLen {
				start = dataLen
			}
		}

		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			end = int(call.Argument(1).ToInteger())
			if end < 0 {
				end = max(dataLen+end, 0)
			}
			if end > dataLen {
				end = dataLen
			}
		}

		if start > end {
			start = end
		}

		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
			contentType = strings.ToLower(call.Argument(2).String())
		}

		var slicedData []byte
		if start < end {
			slicedData = make([]byte, end-start)
			copy(slicedData, blob.data[start:end])
		}

		newBlob := &blobWrapper{
			data:     slicedData,
			mimeType: contentType,
		}

		blobObj := a.runtime.NewObject()
		a.wrapBlobWithObject(newBlob, blobObj)
		return blobObj
	}))

	// stream() - stub
	obj.Set("stream", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	}))
}

// Headers class (for fetch-like patterns)

// headersWrapper wraps HTTP headers storage.
// Header names are normalized to lowercase per HTTP/2 and fetch spec.
type headersWrapper struct {
	headers map[string][]string // lowercase key -> values
}

// newHeadersWrapper creates a new headers wrapper.
func newHeadersWrapper() *headersWrapper {
	return &headersWrapper{
		headers: make(map[string][]string),
	}
}

// headersConstructor creates the Headers constructor for JavaScript.
// Usage: new Headers(), new Headers(headersInit)
// headersInit can be: Headers, object, or array of [name, value] pairs.
func (a *Adapter) headersConstructor(call goja.ConstructorCall) *goja.Object {
	wrapper := newHeadersWrapper()

	// If init argument provided, process it
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		init := call.Argument(0)
		a.initHeaders(wrapper, init)
	}

	thisObj := call.This
	thisObj.Set("_headers", wrapper)

	// Define all methods on the instance
	a.defineHeadersMethods(thisObj, wrapper)

	return thisObj
}

// initHeaders initializes headers from various init types.
func (a *Adapter) initHeaders(wrapper *headersWrapper, init goja.Value) {
	if init == nil || goja.IsNull(init) || goja.IsUndefined(init) {
		return
	}

	obj := init.ToObject(a.runtime)
	if obj == nil {
		return
	}

	// Check if it's another Headers instance
	if headersVal := obj.Get("_headers"); headersVal != nil && !goja.IsUndefined(headersVal) {
		if otherWrapper, ok := headersVal.Export().(*headersWrapper); ok {
			// Copy from other Headers
			for name, values := range otherWrapper.headers {
				wrapper.headers[name] = append(wrapper.headers[name], values...)
			}
			return
		}
	}

	// Check if it's an array of [name, value] pairs
	exported := init.Export()
	if arr, ok := exported.([]any); ok {
		for _, item := range arr {
			if pair, ok := item.([]any); ok && len(pair) >= 2 {
				name := strings.ToLower(fmt.Sprintf("%v", pair[0]))
				value := fmt.Sprintf("%v", pair[1])
				wrapper.headers[name] = append(wrapper.headers[name], value)
			}
		}
		return
	}

	// Otherwise treat as plain object
	for _, key := range obj.Keys() {
		val := obj.Get(key)
		if val != nil && !goja.IsUndefined(val) {
			name := strings.ToLower(key)
			wrapper.headers[name] = append(wrapper.headers[name], val.String())
		}
	}
}

// defineHeadersMethods adds all Headers methods to the object.
func (a *Adapter) defineHeadersMethods(obj *goja.Object, wrapper *headersWrapper) {
	// append(name, value) - adds a new value onto an existing header
	obj.Set("append", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(a.runtime.NewTypeError("Headers.append requires 2 arguments"))
		}
		name := strings.ToLower(call.Argument(0).String())
		value := call.Argument(1).String()
		wrapper.headers[name] = append(wrapper.headers[name], value)
		return goja.Undefined()
	}))

	// delete(name) - deletes a header
	obj.Set("delete", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		name := strings.ToLower(call.Argument(0).String())
		delete(wrapper.headers, name)
		return goja.Undefined()
	}))

	// get(name) - returns first value or null
	obj.Set("get", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		name := strings.ToLower(call.Argument(0).String())
		if values, ok := wrapper.headers[name]; ok && len(values) > 0 {
			// Returns all values joined by ", "
			return a.runtime.ToValue(strings.Join(values, ", "))
		}
		return goja.Null()
	}))

	// getSetCookie() - returns all Set-Cookie header values as array
	obj.Set("getSetCookie", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if values, ok := wrapper.headers["set-cookie"]; ok {
			arr := a.runtime.NewArray()
			for i, v := range values {
				_ = arr.Set(strconv.Itoa(i), a.runtime.ToValue(v))
			}
			return arr
		}
		return a.runtime.NewArray()
	}))

	// has(name) - returns true if header exists
	obj.Set("has", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return a.runtime.ToValue(false)
		}
		name := strings.ToLower(call.Argument(0).String())
		_, ok := wrapper.headers[name]
		return a.runtime.ToValue(ok)
	}))

	// set(name, value) - sets header to single value (replaces existing)
	obj.Set("set", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(a.runtime.NewTypeError("Headers.set requires 2 arguments"))
		}
		name := strings.ToLower(call.Argument(0).String())
		value := call.Argument(1).String()
		wrapper.headers[name] = []string{value}
		return goja.Undefined()
	}))

	// entries() - returns iterator of [name, value] pairs
	obj.Set("entries", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		pairs := make([][2]string, 0)
		for name, values := range wrapper.headers {
			pairs = append(pairs, [2]string{name, strings.Join(values, ", ")})
		}
		// Sort for deterministic iteration order
		sort.Slice(pairs, func(i, j int) bool {
			return pairs[i][0] < pairs[j][0]
		})
		return a.createHeadersIterator(pairs, "entries")
	}))

	// keys() - returns iterator of names
	obj.Set("keys", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		pairs := make([][2]string, 0)
		for name, values := range wrapper.headers {
			pairs = append(pairs, [2]string{name, strings.Join(values, ", ")})
		}
		sort.Slice(pairs, func(i, j int) bool {
			return pairs[i][0] < pairs[j][0]
		})
		return a.createHeadersIterator(pairs, "keys")
	}))

	// values() - returns iterator of values
	obj.Set("values", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		pairs := make([][2]string, 0)
		for name, values := range wrapper.headers {
			pairs = append(pairs, [2]string{name, strings.Join(values, ", ")})
		}
		sort.Slice(pairs, func(i, j int) bool {
			return pairs[i][0] < pairs[j][0]
		})
		return a.createHeadersIterator(pairs, "values")
	}))

	// forEach(callback, thisArg?) - iterates over headers
	obj.Set("forEach", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(a.runtime.NewTypeError("Headers.forEach requires a callback"))
		}
		callback, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(a.runtime.NewTypeError("Headers.forEach requires a callback function"))
		}

		thisArg := goja.Undefined()
		if len(call.Arguments) > 1 {
			thisArg = call.Argument(1)
		}

		// Get sorted keys for deterministic order
		pairs := make([][2]string, 0)
		for name, values := range wrapper.headers {
			pairs = append(pairs, [2]string{name, strings.Join(values, ", ")})
		}
		sort.Slice(pairs, func(i, j int) bool {
			return pairs[i][0] < pairs[j][0]
		})

		for _, pair := range pairs {
			_, _ = callback(thisArg,
				a.runtime.ToValue(pair[1]), // value
				a.runtime.ToValue(pair[0]), // key
				obj,                        // Headers object
			)
		}

		return goja.Undefined()
	}))
}

// createHeadersIterator creates an iterator for headers.
func (a *Adapter) createHeadersIterator(pairs [][2]string, mode string) goja.Value {
	index := 0

	iter := a.runtime.NewObject()
	iter.Set("next", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		result := a.runtime.NewObject()
		if index >= len(pairs) {
			result.Set("done", true)
			result.Set("value", goja.Undefined())
			return result
		}

		pair := pairs[index]
		index++

		result.Set("done", false)
		switch mode {
		case "keys":
			result.Set("value", a.runtime.ToValue(pair[0]))
		case "values":
			result.Set("value", a.runtime.ToValue(pair[1]))
		default: // "entries"
			arr := a.runtime.NewArray()
			_ = arr.Set("0", a.runtime.ToValue(pair[0]))
			_ = arr.Set("1", a.runtime.ToValue(pair[1]))
			result.Set("value", arr)
		}

		return result
	}))

	// Make iterator iterable
	_, _ = a.runtime.RunString(`__tempIterator[Symbol.iterator] = function() { return this; }`)
	a.runtime.Set("__tempIterator", iter)
	_, _ = a.runtime.RunString(`__tempIterator[Symbol.iterator] = function() { return this; }`)
	a.runtime.Set("__tempIterator", goja.Undefined())

	return iter
}

// FormData class (for fetch-like patterns)

// formDataEntry represents a form data entry (just strings, no file support).
type formDataEntry struct {
	name  string
	value string
}

// formDataWrapper wraps form data storage.
type formDataWrapper struct {
	entries []formDataEntry
}

// newFormDataWrapper creates a new form data wrapper.
func newFormDataWrapper() *formDataWrapper {
	return &formDataWrapper{
		entries: make([]formDataEntry, 0),
	}
}

// formDataConstructor creates the FormData constructor for JavaScript.
// Usage: new FormData(), new FormData(form) - form is ignored (no DOM)
func (a *Adapter) formDataConstructor(call goja.ConstructorCall) *goja.Object {
	wrapper := newFormDataWrapper()

	thisObj := call.This
	thisObj.Set("_formData", wrapper)

	// Define all methods on the instance
	a.defineFormDataMethods(thisObj, wrapper)

	return thisObj
}

// defineFormDataMethods adds all FormData methods to the object.
func (a *Adapter) defineFormDataMethods(obj *goja.Object, wrapper *formDataWrapper) {
	// append(name, value) - adds a new entry
	obj.Set("append", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(a.runtime.NewTypeError("FormData.append requires at least 2 arguments"))
		}
		name := call.Argument(0).String()
		value := call.Argument(1).String()
		wrapper.entries = append(wrapper.entries, formDataEntry{name: name, value: value})
		return goja.Undefined()
	}))

	// delete(name) - deletes all entries with the name
	obj.Set("delete", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		name := call.Argument(0).String()
		newEntries := make([]formDataEntry, 0)
		for _, e := range wrapper.entries {
			if e.name != name {
				newEntries = append(newEntries, e)
			}
		}
		wrapper.entries = newEntries
		return goja.Undefined()
	}))

	// get(name) - returns first value or null
	obj.Set("get", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		name := call.Argument(0).String()
		for _, e := range wrapper.entries {
			if e.name == name {
				return a.runtime.ToValue(e.value)
			}
		}
		return goja.Null()
	}))

	// getAll(name) - returns all values as array
	obj.Set("getAll", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return a.runtime.NewArray()
		}
		name := call.Argument(0).String()
		arr := a.runtime.NewArray()
		idx := 0
		for _, e := range wrapper.entries {
			if e.name == name {
				_ = arr.Set(strconv.Itoa(idx), a.runtime.ToValue(e.value))
				idx++
			}
		}
		return arr
	}))

	// has(name) - returns true if name exists
	obj.Set("has", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return a.runtime.ToValue(false)
		}
		name := call.Argument(0).String()
		for _, e := range wrapper.entries {
			if e.name == name {
				return a.runtime.ToValue(true)
			}
		}
		return a.runtime.ToValue(false)
	}))

	// set(name, value) - sets value for name (replaces existing or adds)
	obj.Set("set", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(a.runtime.NewTypeError("FormData.set requires at least 2 arguments"))
		}
		name := call.Argument(0).String()
		value := call.Argument(1).String()

		// Remove all existing entries with this name after first occurrence
		found := false
		newEntries := make([]formDataEntry, 0)
		for _, e := range wrapper.entries {
			if e.name == name {
				if !found {
					// First occurrence: replace value
					newEntries = append(newEntries, formDataEntry{name: name, value: value})
					found = true
				}
				// Skip subsequent occurrences
			} else {
				newEntries = append(newEntries, e)
			}
		}
		if !found {
			// Name didn't exist, append
			newEntries = append(newEntries, formDataEntry{name: name, value: value})
		}
		wrapper.entries = newEntries
		return goja.Undefined()
	}))

	// entries() - returns iterator of [name, value] pairs
	obj.Set("entries", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.createFormDataIterator(wrapper.entries, "entries")
	}))

	// keys() - returns iterator of names
	obj.Set("keys", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.createFormDataIterator(wrapper.entries, "keys")
	}))

	// values() - returns iterator of values
	obj.Set("values", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.createFormDataIterator(wrapper.entries, "values")
	}))

	// forEach(callback, thisArg?) - iterates over entries
	obj.Set("forEach", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(a.runtime.NewTypeError("FormData.forEach requires a callback"))
		}
		callback, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(a.runtime.NewTypeError("FormData.forEach requires a callback function"))
		}

		thisArg := goja.Undefined()
		if len(call.Arguments) > 1 {
			thisArg = call.Argument(1)
		}

		for _, e := range wrapper.entries {
			_, _ = callback(thisArg,
				a.runtime.ToValue(e.value), // value
				a.runtime.ToValue(e.name),  // key
				obj,                        // FormData object
			)
		}

		return goja.Undefined()
	}))
}

// createFormDataIterator creates an iterator for form data entries.
func (a *Adapter) createFormDataIterator(entries []formDataEntry, mode string) goja.Value {
	index := 0
	// Make a copy to avoid mutation during iteration
	entriesCopy := make([]formDataEntry, len(entries))
	copy(entriesCopy, entries)

	iter := a.runtime.NewObject()
	iter.Set("next", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		result := a.runtime.NewObject()
		if index >= len(entriesCopy) {
			result.Set("done", true)
			result.Set("value", goja.Undefined())
			return result
		}

		entry := entriesCopy[index]
		index++

		result.Set("done", false)
		switch mode {
		case "keys":
			result.Set("value", a.runtime.ToValue(entry.name))
		case "values":
			result.Set("value", a.runtime.ToValue(entry.value))
		default: // "entries"
			arr := a.runtime.NewArray()
			_ = arr.Set("0", a.runtime.ToValue(entry.name))
			_ = arr.Set("1", a.runtime.ToValue(entry.value))
			result.Set("value", arr)
		}

		return result
	}))

	// Make iterator iterable (using same pattern as Headers)
	a.runtime.Set("__tempIterator", iter)
	_, _ = a.runtime.RunString(`__tempIterator[Symbol.iterator] = function() { return this; }`)
	a.runtime.Set("__tempIterator", goja.Undefined())

	return iter
}

// DOMException class

// DOMException error codes (from the DOM spec)
const (
	DOMExceptionIndexSizeErr             = 1
	DOMExceptionDOMStringSizeErr         = 2 // Deprecated, historical
	DOMExceptionHierarchyRequestErr      = 3
	DOMExceptionWrongDocumentErr         = 4
	DOMExceptionInvalidCharacterErr      = 5
	DOMExceptionNoDataAllowedErr         = 6 // Deprecated, historical
	DOMExceptionNoModificationAllowedErr = 7
	DOMExceptionNotFoundErr              = 8
	DOMExceptionNotSupportedErr          = 9
	DOMExceptionInUseAttributeErr        = 10
	DOMExceptionInvalidStateErr          = 11
	DOMExceptionSyntaxErr                = 12
	DOMExceptionInvalidModificationErr   = 13
	DOMExceptionNamespaceErr             = 14
	DOMExceptionInvalidAccessErr         = 15
	DOMExceptionValidationErr            = 16 // Deprecated, historical
	DOMExceptionTypeMismatchErr          = 17
	DOMExceptionSecurityErr              = 18
	DOMExceptionNetworkErr               = 19
	DOMExceptionAbortErr                 = 20
	DOMExceptionURLMismatchErr           = 21
	DOMExceptionQuotaExceededErr         = 22
	DOMExceptionTimeoutErr               = 23
	DOMExceptionInvalidNodeTypeErr       = 24
	DOMExceptionDataCloneErr             = 25
)

// domExceptionNameToCode maps error names to legacy codes.
var domExceptionNameToCode = map[string]int{
	"IndexSizeError":             DOMExceptionIndexSizeErr,
	"HierarchyRequestError":      DOMExceptionHierarchyRequestErr,
	"WrongDocumentError":         DOMExceptionWrongDocumentErr,
	"InvalidCharacterError":      DOMExceptionInvalidCharacterErr,
	"NoModificationAllowedError": DOMExceptionNoModificationAllowedErr,
	"NotFoundError":              DOMExceptionNotFoundErr,
	"NotSupportedError":          DOMExceptionNotSupportedErr,
	"InUseAttributeError":        DOMExceptionInUseAttributeErr,
	"InvalidStateError":          DOMExceptionInvalidStateErr,
	"SyntaxError":                DOMExceptionSyntaxErr,
	"InvalidModificationError":   DOMExceptionInvalidModificationErr,
	"NamespaceError":             DOMExceptionNamespaceErr,
	"InvalidAccessError":         DOMExceptionInvalidAccessErr,
	"TypeMismatchError":          DOMExceptionTypeMismatchErr,
	"SecurityError":              DOMExceptionSecurityErr,
	"NetworkError":               DOMExceptionNetworkErr,
	"AbortError":                 DOMExceptionAbortErr,
	"URLMismatchError":           DOMExceptionURLMismatchErr,
	"QuotaExceededError":         DOMExceptionQuotaExceededErr,
	"TimeoutError":               DOMExceptionTimeoutErr,
	"InvalidNodeTypeError":       DOMExceptionInvalidNodeTypeErr,
	"DataCloneError":             DOMExceptionDataCloneErr,
	// New error names (code 0)
	"EncodingError":    0,
	"NotReadableError": 0,
	"UnknownError":     0,
	"ConstraintError":  0,
	"DataError":        0,
	"TransactionError": 0, // Deprecated
	"ReadOnlyError":    0,
	"VersionError":     0,
	"OperationError":   0,
	"NotAllowedError":  0,
	"OptOutError":      0, // Deprecated
}

// domExceptionWrapper wraps DOMException data.
type domExceptionWrapper struct {
	message string
	name    string
	code    int
}

// throwDOMException creates a DOMException object and returns it as a goja.Value
// suitable for use with panic() to throw it as a JS exception.
func (a *Adapter) throwDOMException(name, message string) goja.Value {
	domExCtor := a.runtime.Get("DOMException")
	if domExCtor != nil && !goja.IsUndefined(domExCtor) {
		if ctor, ok := goja.AssertConstructor(domExCtor); ok {
			obj, err := ctor(nil, a.runtime.ToValue(message), a.runtime.ToValue(name))
			if err == nil {
				return obj
			}
		}
	}
	// Fallback if DOMException constructor is not available
	return a.runtime.NewTypeError(name + ": " + message)
}

// domExceptionConstructor creates the DOMException constructor for JavaScript.
// Usage: new DOMException(message?, name?)
// message defaults to empty string, name defaults to "Error"
func (a *Adapter) domExceptionConstructor(call goja.ConstructorCall) *goja.Object {
	message := ""
	name := "Error"

	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
		message = call.Argument(0).String()
	}
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
		name = call.Argument(1).String()
	}

	// Look up legacy code for the name
	code := 0
	if c, ok := domExceptionNameToCode[name]; ok {
		code = c
	}

	wrapper := &domExceptionWrapper{
		message: message,
		name:    name,
		code:    code,
	}

	thisObj := call.This
	thisObj.Set("_domException", wrapper)

	// Set properties
	thisObj.Set("message", message)
	thisObj.Set("name", name)
	thisObj.Set("code", code)

	// toString() method
	thisObj.Set("toString", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.runtime.ToValue(fmt.Sprintf("%s: %s", wrapper.name, wrapper.message))
	}))

	return thisObj
}

// bindDOMExceptionConstants adds all constant properties to DOMException.
func (a *Adapter) bindDOMExceptionConstants() error {
	// Get the DOMException constructor
	domExceptionVal := a.runtime.Get("DOMException")
	if domExceptionVal == nil || goja.IsUndefined(domExceptionVal) {
		return fmt.Errorf("DOMException not found")
	}
	domExceptionObj := domExceptionVal.ToObject(a.runtime)

	// Add static constants
	domExceptionObj.Set("INDEX_SIZE_ERR", DOMExceptionIndexSizeErr)
	domExceptionObj.Set("DOMSTRING_SIZE_ERR", DOMExceptionDOMStringSizeErr)
	domExceptionObj.Set("HIERARCHY_REQUEST_ERR", DOMExceptionHierarchyRequestErr)
	domExceptionObj.Set("WRONG_DOCUMENT_ERR", DOMExceptionWrongDocumentErr)
	domExceptionObj.Set("INVALID_CHARACTER_ERR", DOMExceptionInvalidCharacterErr)
	domExceptionObj.Set("NO_DATA_ALLOWED_ERR", DOMExceptionNoDataAllowedErr)
	domExceptionObj.Set("NO_MODIFICATION_ALLOWED_ERR", DOMExceptionNoModificationAllowedErr)
	domExceptionObj.Set("NOT_FOUND_ERR", DOMExceptionNotFoundErr)
	domExceptionObj.Set("NOT_SUPPORTED_ERR", DOMExceptionNotSupportedErr)
	domExceptionObj.Set("INUSE_ATTRIBUTE_ERR", DOMExceptionInUseAttributeErr)
	domExceptionObj.Set("INVALID_STATE_ERR", DOMExceptionInvalidStateErr)
	domExceptionObj.Set("SYNTAX_ERR", DOMExceptionSyntaxErr)
	domExceptionObj.Set("INVALID_MODIFICATION_ERR", DOMExceptionInvalidModificationErr)
	domExceptionObj.Set("NAMESPACE_ERR", DOMExceptionNamespaceErr)
	domExceptionObj.Set("INVALID_ACCESS_ERR", DOMExceptionInvalidAccessErr)
	domExceptionObj.Set("VALIDATION_ERR", DOMExceptionValidationErr)
	domExceptionObj.Set("TYPE_MISMATCH_ERR", DOMExceptionTypeMismatchErr)
	domExceptionObj.Set("SECURITY_ERR", DOMExceptionSecurityErr)
	domExceptionObj.Set("NETWORK_ERR", DOMExceptionNetworkErr)
	domExceptionObj.Set("ABORT_ERR", DOMExceptionAbortErr)
	domExceptionObj.Set("URL_MISMATCH_ERR", DOMExceptionURLMismatchErr)
	domExceptionObj.Set("QUOTA_EXCEEDED_ERR", DOMExceptionQuotaExceededErr)
	domExceptionObj.Set("TIMEOUT_ERR", DOMExceptionTimeoutErr)
	domExceptionObj.Set("INVALID_NODE_TYPE_ERR", DOMExceptionInvalidNodeTypeErr)
	domExceptionObj.Set("DATA_CLONE_ERR", DOMExceptionDataCloneErr)

	return nil
}

// Symbol.for and Symbol.keyFor utilities

// bindSymbol adds Symbol.for and Symbol.keyFor utilities to the global Symbol object.
// These implement the global symbol registry per ECMAScript specification.
// Note: Goja already provides native Symbol.for and Symbol.keyFor implementations.
// This function verifies they exist and are properly accessible. If Goja's native
// implementation is incomplete, we provide a polyfill.
func (a *Adapter) bindSymbol() error {
	// Get the Symbol constructor
	symbolVal := a.runtime.Get("Symbol")
	if symbolVal == nil || goja.IsUndefined(symbolVal) {
		// Symbol doesn't exist - this shouldn't happen in Goja, but return no error
		return nil
	}
	symbolObj := symbolVal.ToObject(a.runtime)

	// Check if Symbol.for already exists (Goja provides native implementation)
	symbolFor := symbolObj.Get("for")
	if symbolFor != nil && !goja.IsUndefined(symbolFor) {
		// Goja already provides Symbol.for - verify it works and return
		_, ok := goja.AssertFunction(symbolFor)
		if ok {
			// Also check Symbol.keyFor
			symbolKeyFor := symbolObj.Get("keyFor")
			if symbolKeyFor != nil && !goja.IsUndefined(symbolKeyFor) {
				if _, ok := goja.AssertFunction(symbolKeyFor); ok {
					// Both exist and are functions - Goja's native implementation is fine
					return nil
				}
			}
		}
	}

	// If we reach here, we need to provide a polyfill
	// Create a JavaScript-based registry since Symbol values are difficult to
	// work with directly in Go
	polyfill := `
		(function() {
			var symbolRegistry = {};
			var keyRegistry = new Map(); // Symbol -> key mapping

			Symbol.for = function(key) {
				if (key === undefined) {
					throw new TypeError("Symbol.for requires a key argument");
				}
				key = String(key);
				if (symbolRegistry.hasOwnProperty(key)) {
					return symbolRegistry[key];
				}
				var sym = Symbol(key);
				symbolRegistry[key] = sym;
				keyRegistry.set(sym, key);
				return sym;
			};

			Symbol.keyFor = function(sym) {
				if (typeof sym !== 'symbol') {
					throw new TypeError("Symbol.keyFor requires a Symbol argument");
				}
				return keyRegistry.get(sym);
			};
		})();
	`

	_, err := a.runtime.RunString(polyfill)
	return err
}

// blobPartToBytes converts a Blob part (string, ArrayBuffer, Uint8Array, Blob) to bytes.
func (a *Adapter) blobPartToBytes(part goja.Value) ([]byte, error) {
	if part == nil || goja.IsUndefined(part) || goja.IsNull(part) {
		return nil, nil
	}

	// Check for string
	if exportType := part.ExportType(); exportType != nil && exportType.Kind().String() == "string" {
		return []byte(part.String()), nil
	}

	obj := part.ToObject(a.runtime)
	if obj == nil {
		// Try to convert to string
		return []byte(part.String()), nil
	}

	// Check if it's a Blob (has _blob property)
	if blobVal := obj.Get("_blob"); blobVal != nil && !goja.IsUndefined(blobVal) {
		if blob, ok := blobVal.Export().(*blobWrapper); ok {
			// Return a copy of the blob data
			result := make([]byte, len(blob.data))
			copy(result, blob.data)
			return result, nil
		}
	}

	// Check for typed array or ArrayBuffer using extractBytes
	bytes, err := a.extractBytes(part)
	if err == nil {
		return bytes, nil
	}

	// Fallback: convert to string
	return []byte(part.String()), nil
}

// FETCH API: Not Implemented
//
// The fetch() API is intentionally not implemented in this package because:
//
//   1. HTTP client configuration varies significantly between use cases
//      (timeouts, TLS settings, proxies, authentication, etc.)
//
//   2. Server-side JavaScript environments (like this one) often need
//      different network semantics than browser environments
//
//   3. Using Go's net/http package directly from host code provides
//      better control and type safety
//
// If you need HTTP functionality in your JavaScript code, consider:
//
//   - Exposing custom Go functions that use net/http
//   - Creating a fetch-like API tailored to your specific needs
//   - Using the provided Headers and FormData classes for compatibility
//
// The fetchNotImplemented function provides a clear error message when
// JavaScript code attempts to use fetch().

// fetchNotImplemented returns a rejected promise with a clear error message
// explaining that fetch() is not implemented and suggesting alternatives.
func (a *Adapter) fetchNotImplemented(call goja.FunctionCall) goja.Value {
	// Return a rejected promise with a descriptive error
	err := fmt.Errorf("fetch() is not implemented in goja-eventloop. " +
		"For HTTP operations, expose custom Go functions using net/http " +
		"or implement a fetch wrapper with your preferred http.Client configuration")
	return a.GojaWrapPromise(a.js.Reject(err))
}

// localStorage and sessionStorage (in-memory)
//
// ⚠️ IMPORTANT LIMITATION: In-Memory Storage Only
//
// Unlike browser implementations, localStorage and sessionStorage in this package
// are ephemeral and NOT persisted to disk:
//
//   - localStorage: Persists only for the lifetime of the Adapter instance.
//     Data is lost when the Go process terminates or the Adapter is garbage collected.
//
//   - sessionStorage: Same behavior as localStorage in this implementation.
//     There is no concept of browser tabs or windows to isolate storage.
//
// This in-memory implementation is suitable for:
//   - Unit testing JavaScript code that uses Web Storage APIs
//   - Short-lived scripts that need temporary key-value storage
//   - Applications that don't require persistence
//
// If you need persistent storage, consider implementing a custom storage adapter
// that writes to a database or filesystem.
//
// Additionally, the following browser-specific features are NOT implemented:
//   - Storage events (storage event is not fired on changes)
//   - Size limits (no quota enforcement)
//   - Cross-origin isolation

// storageWrapper implements an in-memory Storage interface.
type storageWrapper struct {
	items map[string]string
	keys  []string // Maintain insertion order for key()
	mu    sync.RWMutex
}

// newStorageWrapper creates a new storage wrapper.
func newStorageWrapper() *storageWrapper {
	return &storageWrapper{
		items: make(map[string]string),
		keys:  make([]string, 0),
	}
}

// bindStorage creates localStorage and sessionStorage bindings.
func (a *Adapter) bindStorage() error {
	// Create localStorage (persists for the lifetime of the Adapter)
	localStorage := newStorageWrapper()
	localStorageObj := a.createStorageObject(localStorage)
	a.runtime.Set("localStorage", localStorageObj)

	// Create sessionStorage (same behavior for our in-memory implementation)
	sessionStorage := newStorageWrapper()
	sessionStorageObj := a.createStorageObject(sessionStorage)
	a.runtime.Set("sessionStorage", sessionStorageObj)

	return nil
}

// createStorageObject creates a Storage object with all Web Storage API methods.
func (a *Adapter) createStorageObject(storage *storageWrapper) goja.Value {
	obj := a.runtime.NewObject()

	// Store internal reference
	obj.Set("_storage", storage)

	// length property (getter)
	obj.DefineAccessorProperty("length",
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			storage.mu.RLock()
			defer storage.mu.RUnlock()
			return a.runtime.ToValue(len(storage.items))
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// getItem(key) - returns value or null
	obj.Set("getItem", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		key := call.Argument(0).String()

		storage.mu.RLock()
		defer storage.mu.RUnlock()

		if value, exists := storage.items[key]; exists {
			return a.runtime.ToValue(value)
		}
		return goja.Null()
	}))

	// setItem(key, value) - stores value
	obj.Set("setItem", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(a.runtime.NewTypeError("Storage.setItem requires 2 arguments"))
		}
		key := call.Argument(0).String()
		value := call.Argument(1).String()

		storage.mu.Lock()
		defer storage.mu.Unlock()

		// Check if key is new
		_, exists := storage.items[key]
		storage.items[key] = value

		// Track key order for key() method
		if !exists {
			storage.keys = append(storage.keys, key)
		}

		return goja.Undefined()
	}))

	// removeItem(key) - removes value
	obj.Set("removeItem", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		key := call.Argument(0).String()

		storage.mu.Lock()
		defer storage.mu.Unlock()

		if _, exists := storage.items[key]; exists {
			delete(storage.items, key)
			// Remove from keys slice
			for i, k := range storage.keys {
				if k == key {
					storage.keys = append(storage.keys[:i], storage.keys[i+1:]...)
					break
				}
			}
		}

		return goja.Undefined()
	}))

	// clear() - removes all items
	obj.Set("clear", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		storage.mu.Lock()
		defer storage.mu.Unlock()

		storage.items = make(map[string]string)
		storage.keys = make([]string, 0)

		return goja.Undefined()
	}))

	// key(index) - returns key at index or null
	obj.Set("key", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		index := int(call.Argument(0).ToInteger())

		storage.mu.RLock()
		defer storage.mu.RUnlock()

		if index < 0 || index >= len(storage.keys) {
			return goja.Null()
		}
		return a.runtime.ToValue(storage.keys[index])
	}))

	return obj
}
