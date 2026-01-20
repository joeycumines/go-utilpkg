// Copyright 2025 Joseph Cumines
//
// goja-eventloop: Goja adapter for the event loop library
//
// This binds eventloop.JS functionality to Goja JavaScript runtime.

package gojaeventloop

import (
	"fmt"
	"sync"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// Adapter bridges Goja runtime to goeventloop.JS.
// This allows setTimeout/setInterval/queueMicrotask/Promise to work with Goja.
type Adapter struct {
	js      *goeventloop.JS
	runtime *goja.Runtime
	loop    *goeventloop.Loop
	mu      sync.Mutex
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

// ensureLoopThread ensures thread-safe access to eventloop adapter.
// For now, this establishes a memory barrier via mutex lock.
func (a *Adapter) ensureLoopThread() {
	a.mu.Lock()
	defer a.mu.Unlock()
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

	// Bind Promise constructor using native constructor signature
	// Use func(ConstructorCall) *Object to make it work with 'new' operator
	promiseConstructor := a.runtime.ToValue(func(call goja.ConstructorCall) *goja.Object {
		return a.promiseConstructorWrapper(call)
	})
	if err := a.runtime.Set("Promise", promiseConstructor); err != nil {
		return fmt.Errorf("failed to bind Promise: %w", err)
	}

	return nil
}

// setTimeout binding for Goja
func (a *Adapter) setTimeout(call goja.FunctionCall) goja.Value {
	a.ensureLoopThread()
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
	a.ensureLoopThread()
	id := uint64(call.Argument(0).ToInteger())
	_ = a.js.ClearTimeout(id) // Silently ignore if timer not found (matches browser behavior)
	return goja.Undefined()
}

// setInterval binding for Goja
func (a *Adapter) setInterval(call goja.FunctionCall) goja.Value {
	a.ensureLoopThread()
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
	a.ensureLoopThread()
	id := uint64(call.Argument(0).ToInteger())
	_ = a.js.ClearInterval(id) // Silently ignore if timer not found (matches browser behavior)
	return goja.Undefined()
}

// queueMicrotask binding for Goja
func (a *Adapter) queueMicrotask(call goja.FunctionCall) goja.Value {
	a.ensureLoopThread()
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

// promiseConstructorWrapper handles the Goja ConstructorCall for 'new Promise()'
func (a *Adapter) promiseConstructorWrapper(call goja.ConstructorCall) *goja.Object {
	a.ensureLoopThread()
	executor := call.Argument(0)
	if executor.Export() == nil {
		panic(a.runtime.NewTypeError("Promise requires a function as first argument"))
	}

	executorCallable, ok := goja.AssertFunction(executor)
	if !ok {
		panic(a.runtime.NewTypeError("Promise requires a function as first argument"))
	}

	promise, resolve, reject := a.js.NewChainedPromise()

	// call.This is the newly created object when called with 'new'
	promiseObj := call.This

	// Store the internal promise
	_ = promiseObj.Set("_internalPromise", promise)

	// Define then method
	thenMethod := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		thisObj := call.This.ToObject(a.runtime)
		internal := thisObj.Get("_internalPromise")
		if internal.Export() == nil || internal == goja.Undefined() {
			panic(a.runtime.NewTypeError("Promise internal state lost"))
		}

		internalPromise := internal.Export().(*goeventloop.ChainedPromise)
		if internalPromise == nil {
			panic(a.runtime.NewTypeError("Invalid internal promise"))
		}

		onFulfilled := a.gojaFuncToHandler(call.Argument(0))
		onRejected := a.gojaFuncToHandler(call.Argument(1))
		chained := internalPromise.Then(onFulfilled, onRejected)
		if chained == nil {
			panic(a.runtime.NewTypeError("Promise.Then returned nil"))
		}
		return a.gojaWrapPromise(chained)
	})
	_ = promiseObj.Set("then", thenMethod)

	// Define catch method
	catchMethod := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		thisObj := call.This.ToObject(a.runtime)
		internal := thisObj.Get("_internalPromise")
		if internal.Export() == nil {
			panic(a.runtime.NewTypeError("Promise internal state lost"))
		}

		internalPromise := internal.Export().(*goeventloop.ChainedPromise)
		onRejected := a.gojaFuncToHandler(call.Argument(0))
		chained := internalPromise.Catch(onRejected)
		return a.gojaWrapPromise(chained)
	})
	_ = promiseObj.Set("catch", catchMethod)

	// Define finally method
	finallyMethod := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		thisObj := call.This.ToObject(a.runtime)
		internal := thisObj.Get("_internalPromise")
		if internal.Export() == nil {
			panic(a.runtime.NewTypeError("Promise internal state lost"))
		}

		internalPromise := internal.Export().(*goeventloop.ChainedPromise)
		onFinally := a.gojaVoidFuncToHandler(call.Argument(0))
		chained := internalPromise.Finally(onFinally)
		return a.gojaWrapPromise(chained)
	})
	_ = promiseObj.Set("finally", finallyMethod)

	// Call executor with resolve/reject callbacks
	_, err := executorCallable(goja.Undefined(),
		a.runtime.ToValue(func(result goja.Value) {
			var val any
			if result != goja.Undefined() && result.Export() != nil {
				val = result.Export()
			}
			resolve(val)
		}),
		a.runtime.ToValue(func(reason goja.Value) {
			var val any
			if reason != goja.Undefined() && reason.Export() != nil {
				val = reason.Export()
			}
			reject(val)
		}),
	)
	if err != nil {
		// If executor throws, reject the promise
		reject(err)
	}

	return promiseObj
}

// gojaWrapPromise wraps a ChainedPromise with then/catch/finally prototype methods
func (a *Adapter) gojaWrapPromise(promise *goeventloop.ChainedPromise) goja.Value {
	wrapped := a.runtime.ToValue(promise)
	wrappedObj := wrapped.ToObject(a.runtime)

	// Store internal promise reference for method access
	_ = wrappedObj.Set("_internalPromise", promise)

	thenFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		// Get internal promise from the object `.then()` was called on
		thisObj := call.This.ToObject(a.runtime)
		internalVal := thisObj.Get("_internalPromise")
		if internalVal == goja.Undefined() || internalVal.Export() == nil {
			panic(a.runtime.NewTypeError("Promise not properly initialized"))
		}
		internal := internalVal.Export().(*goeventloop.ChainedPromise)

		onFulfilled := a.gojaFuncToHandler(call.Argument(0))
		onRejected := a.gojaFuncToHandler(call.Argument(1))
		chained := internal.Then(onFulfilled, onRejected)
		return a.gojaWrapPromise(chained)
	})

	catchFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		// Get internal promise from the object `.catch()` was called on
		thisObj := call.This.ToObject(a.runtime)
		internalVal := thisObj.Get("_internalPromise")
		if internalVal == goja.Undefined() || internalVal.Export() == nil {
			panic(a.runtime.NewTypeError("Promise not properly initialized"))
		}
		internal := internalVal.Export().(*goeventloop.ChainedPromise)

		onRejected := a.gojaFuncToHandler(call.Argument(0))
		chained := internal.Catch(onRejected)
		return a.gojaWrapPromise(chained)
	})

	finallyFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		// Get internal promise from the object `.finally()` was called on
		thisObj := call.This.ToObject(a.runtime)
		internalVal := thisObj.Get("_internalPromise")
		if internalVal == goja.Undefined() || internalVal.Export() == nil {
			panic(a.runtime.NewTypeError("Promise not properly initialized"))
		}
		internal := internalVal.Export().(*goeventloop.ChainedPromise)

		onFinally := a.gojaVoidFuncToHandler(call.Argument(0))
		chained := internal.Finally(onFinally)
		return a.gojaWrapPromise(chained)
	})

	wrappedObj.Set("then", thenFn)
	wrappedObj.Set("catch", catchFn)
	wrappedObj.Set("finally", finallyFn)

	// Return object with methods, not raw value
	return wrappedObj
}

// JS returns the underlying goeventloop.JS adapter
func (a *Adapter) JS() *goeventloop.JS {
	return a.js
}

// Runtime returns the underlying Goja runtime
func (a *Adapter) Runtime() *goja.Runtime {
	return a.runtime
}

// Loop returns the underlying event loop
func (a *Adapter) Loop() *goeventloop.Loop {
	return a.loop
}

// NewChainedPromise creates a new promise with Goja-compatible resolve/reject
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
		// Only convert to Goja value if result is not nil
		var arg goja.Value
		if result != nil {
			arg = a.runtime.ToValue(result)
		} else {
			arg = goja.Undefined()
		}
		ret, err := fnCallable(goja.Undefined(), arg)
		if err != nil {
			panic(err)
		}
		// Safe export that handles nil
		if ret == goja.Undefined() || ret.Export() == nil {
			return nil
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
