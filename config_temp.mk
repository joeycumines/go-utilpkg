.PHONY: fix-adapter-api
fix-adapter-api: SHELL := /bin/bash
fix-adapter-api:
	@echo "=== Rewriting adapter.go with correct goja API ===" && \
	cd /Users/joeyc/dev/go-utilpkg && \
	cat > goja-eventloop/adapter.go << 'ADAPTEREOF'
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
	js       *goeventloop.JS
	runtime  *goja.Runtime
	loop     *goeventloop.Loop
	mu       sync.Mutex
}

// New creates a new Goja adapter for the given event loop and runtime.
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

// Bind creates setTimeout/setInterval/queueMicrotask/Promise bindings in Goja global scope.
func (a *Adapter) Bind() error {
	// Set setTimeout
	err := a.runtime.Set("setTimeout", a.setTimeout)
	if err != nil {
		return fmt.Errorf("failed to bind setTimeout: %w", err)
	}

	// Set clearTimeout
	err = a.runtime.Set("clearTimeout", a.clearTimeout)
	if err != nil {
		return fmt.Errorf("failed to bind clearTimeout: %w", err)
	}

	// Set setInterval
	err = a.runtime.Set("setInterval", a.setInterval)
	if err != nil {
		return fmt.Errorf("failed to bind setInterval: %w", err)
	}

	// Set clearInterval
	err = a.runtime.Set("clearInterval", a.clearInterval)
	if err != nil {
		return fmt.Errorf("failed to bind clearInterval: %w", err)
	}

	// Set queueMicrotask
	err = a.runtime.Set("queueMicrotask", a.queueMicrotask)
	if err != nil {
		return fmt.Errorf("failed to bind queueMicrotask: %w", err)
	}

	// Set Promise (bridge to goeventloop.JS Promise)
	err = a.runtime.Set("Promise", a.promiseConstructor)
	if err != nil {
		return fmt.Errorf("failed to bind Promise: %w", err)
	}

	return nil
}

// setTimeout binding for Goja
func (a *Adapter) setTimeout(call goja.FunctionCall) goja.Value {
	fn := call.Argument(0)
	if fn.IsNull() || fn.IsUndefined() {
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
		a.loopTick()
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
	err := a.js.ClearTimeout(id)
	if err != nil && err != goeventloop.ErrTimerNotFound {
		// Silently ignore if timer not found (matches browser behavior)
	}
	return goja.Undefined()
}

// setInterval binding for Goja
func (a *Adapter) setInterval(call goja.FunctionCall) goja.Value {
	fn := call.Argument(0)
	if fn.IsNull() || fn.IsUndefined() {
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
		a.loopTick()
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
	err := a.js.ClearInterval(id)
	if err != nil && err != goeventloop.ErrTimerNotFound {
		// Silently ignore if timer not found (matches browser behavior)
	}
	return goja.Undefined()
}

// queueMicrotask binding for Goja
func (a *Adapter) queueMicrotask(call goja.FunctionCall) goja.Value {
	fn := call.Argument(0)
	if fn.IsNull() || fn.IsUndefined() {
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
func (a *Adapter) promiseConstructor(call goja.FunctionCall) goja.Value {
	executor := call.Argument(0)
	if executor.IsNull() || executor.IsUndefined() {
		panic(a.runtime.NewTypeError("Promise requires a function as first argument"))
	}

	executorCallable, ok := goja.AssertFunction(executor)
	if !ok {
		panic(a.runtime.NewTypeError("Promise requires a function as first argument"))
	}

	promise, resolve, reject := a.js.NewChainedPromise()

	// Call executor with resolve/reject callbacks wrapped
	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.loopTick()
				reject(r)
			}
		}()

		resolveVal := a.runtime.ToValue(func(reason ...goja.Value) {
			if len(reason) > 0 {
				resolve(reason[0].Export())
			} else {
				resolve(nil)
			}
		})

		rejectVal := a.runtime.ToValue(func(reason ...goja.Value) {
			if len(reason) > 0 {
				reject(reason[0].Export())
			} else {
				reject(nil)
			}
		})

		_, err := executorCallable(goja.Undefined(), resolveVal, rejectVal)
		if err != nil {
			loop := a.js.Loop()
			goeventloop.ScheduleMicrotaskOnLoop(loop, func() {
				reject(fmt.Errorf("executor threw error: %w", err))
			})
			return
		}
	}()

	return a.runtime.ToValue(promise)
}

// loopTick advances the event loop one tick (convenience method)
func (a *Adapter) loopTick() {
	a.loop.RunOneTask()
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

	resolveVal := runtime.ToValue(func(result ...goja.Value) {
		if len(result) > 0 {
			resolve(result[0].Export())
		} else {
			resolve(nil)
		}
	})

	rejectVal := runtime.ToValue(func(reason ...goja.Value) {
		if len(reason) > 0 {
			reject(reason[0].Export())
		} else {
			reject(nil)
		}
	})

	return promise, resolveVal, rejectVal
}
ADAPTEREOF
