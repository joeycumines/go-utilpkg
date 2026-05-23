package gojaeventloop

import (
	"fmt"
	"math"
	"runtime"
	"weak"

	"github.com/joeycumines/goja"
)

// AbortController/AbortSignal Bindings

// abortControllerConstructor creates the AbortController constructor for JavaScript.
func (a *Adapter) abortControllerConstructor(call goja.ConstructorCall) *goja.Object {
	thisObj := call.This
	signalState, signalObject := a.newAbortSignal()
	controller := &abortControllerState{signal: signalState, signalObject: signalObject}
	a.setHiddenState(a.abortControllerStore, thisObj, controller)

	return thisObj
}

func (a *Adapter) bindAbortControllerPrototype(constructor *goja.Object) error {
	if err := defineWebConstructorObject(a.runtime, constructor, "AbortController", 0); err != nil {
		return err
	}
	prototype, _ := constructor.Get("prototype").(*goja.Object)
	if prototype == nil {
		return fmt.Errorf("abortController prototype not found")
	}
	if err := defineWebAccessor(a.runtime, prototype, "signal", true, func(call goja.FunctionCall) goja.Value {
		state := a.abortControllerThis(call.This)
		if state.signalObject != nil {
			return state.signalObject
		}
		return goja.Undefined()
	}, nil); err != nil {
		return fmt.Errorf("bind AbortController.prototype.signal: %w", err)
	}
	if err := defineWebMethod(a.runtime, prototype, "abort", 0, true, func(call goja.FunctionCall) goja.Value {
		a.abortSignalState(a.abortControllerThis(call.This).signal, a.abortReason(call.Argument(0)))
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind AbortController.prototype.abort: %w", err)
	}
	return defineWebTag(a.runtime, prototype, "AbortController")
}

func (a *Adapter) abortControllerThis(value goja.Value) *abortControllerState {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		panic(a.runtime.NewTypeError("AbortController method called on incompatible receiver"))
	}
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		panic(a.runtime.NewTypeError("AbortController method called on incompatible receiver"))
	}
	stateValue := a.hiddenState(a.abortControllerStore, obj)
	if stateValue == nil || goja.IsUndefined(stateValue) || goja.IsNull(stateValue) {
		panic(a.runtime.NewTypeError("AbortController method called on incompatible receiver"))
	}
	state, ok := stateValue.Export().(*abortControllerState)
	if !ok || state == nil || state.signal == nil {
		panic(a.runtime.NewTypeError("AbortController method called on incompatible receiver"))
	}
	return state
}

// abortSignalConstructor creates the AbortSignal constructor for JavaScript.
// Note: AbortSignal is not typically constructed directly, but we provide it for completeness.
func (a *Adapter) abortSignalConstructor(call goja.ConstructorCall) *goja.Object {
	// AbortSignal should not be constructed directly - throw error
	panic(a.runtime.NewTypeError("AbortSignal cannot be constructed directly"))
}

func (a *Adapter) newAbortSignal() (*abortSignalState, *goja.Object) {
	obj := a.runtime.NewObject()
	if prototype := a.abortSignalPrototype; prototype != nil {
		if err := obj.SetPrototype(prototype); err != nil {
			a.panicJSException(wrapRuntimeError("set AbortSignal prototype", err))
		}
	}
	wrapper := a.initEventTargetObject(obj)
	state := &abortSignalState{target: wrapper, object: weak.Make(obj), reason: goja.Undefined()}
	wrapper.abortSignal = state

	a.setHiddenState(a.abortSignalStateStore, obj, state)

	return state, obj
}

func (a *Adapter) bindAbortSignalPrototype(constructor *goja.Object) error {
	if err := defineWebConstructorObject(a.runtime, constructor, "AbortSignal", 0); err != nil {
		return err
	}
	prototype, _ := constructor.Get("prototype").(*goja.Object)
	if prototype == nil {
		return fmt.Errorf("abortSignal prototype not found")
	}
	if err := defineWebAccessor(a.runtime, prototype, "aborted", true, func(call goja.FunctionCall) goja.Value {
		return a.runtime.ToValue(a.abortSignalValueMessage(call.This, "AbortSignal getter called on incompatible receiver").aborted)
	}, nil); err != nil {
		return fmt.Errorf("bind AbortSignal.prototype.aborted: %w", err)
	}
	if err := defineWebAccessor(a.runtime, prototype, "reason", true, func(call goja.FunctionCall) goja.Value {
		return a.abortSignalValueMessage(call.This, "AbortSignal getter called on incompatible receiver").reason
	}, nil); err != nil {
		return fmt.Errorf("bind AbortSignal.prototype.reason: %w", err)
	}
	if err := defineWebAccessor(a.runtime, prototype, "onabort", true,
		func(call goja.FunctionCall) goja.Value {
			state := a.abortSignalValueMessage(call.This, "AbortSignal getter called on incompatible receiver")
			if state.onabort == nil || state.onabort.removed.Load() {
				return goja.Null()
			}
			return state.onabort.callback
		},
		func(call goja.FunctionCall) goja.Value {
			state := a.abortSignalValueMessage(call.This, "AbortSignal setter called on incompatible receiver")
			value := call.Argument(0)
			if _, ok := goja.AssertFunction(value); ok {
				if state.onabort != nil && !state.onabort.removed.Load() {
					state.onabort.callback = value
					return goja.Undefined()
				}
				state.onabort = a.eventTargetAddConfigured(state.target, "abort", value, false, false, false, false, true, nil)
				return goja.Undefined()
			}
			if state.onabort != nil {
				a.eventTargetRemoveInfo(state.target, "abort", state.onabort)
				state.onabort = nil
			}
			return goja.Undefined()
		}); err != nil {
		return fmt.Errorf("bind AbortSignal.prototype.onabort: %w", err)
	}
	if err := defineWebMethod(a.runtime, prototype, "throwIfAborted", 0, true, func(call goja.FunctionCall) goja.Value {
		state := a.abortSignalValueMessage(call.This, "AbortSignal.throwIfAborted called on incompatible receiver")
		if state.aborted {
			panic(state.reason)
		}
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind AbortSignal.prototype.throwIfAborted: %w", err)
	}
	if err := defineWebTag(a.runtime, prototype, "AbortSignal"); err != nil {
		return err
	}
	a.abortSignalPrototype = prototype
	return nil
}

// AbortSignal Static Methods

// bindAbortSignalStatics adds static methods (any, timeout) to AbortSignal.
func (a *Adapter) bindAbortSignalStatics(abortSignalObj *goja.Object) error {
	if abortSignalObj == nil {
		return fmt.Errorf("AbortSignal not found")
	}

	if err := defineWebMethod(a.runtime, abortSignalObj, "abort", 0, true, func(call goja.FunctionCall) goja.Value {
		state, obj := a.newAbortSignal()
		markAbortSignal(state, a.abortReason(call.Argument(0)))
		return obj
	}); err != nil {
		return fmt.Errorf("bind AbortSignal.abort: %w", err)
	}

	// AbortSignal.any(signals)
	// Creates a composite signal that aborts when any input signal aborts
	if err := defineWebMethod(a.runtime, abortSignalObj, "any", 1, true, func(call goja.FunctionCall) goja.Value {
		signals := a.abortSignalSequence(call.Argument(0))

		composite, compositeObject := a.newAbortSignal()
		for _, sig := range signals {
			if sig.aborted {
				markAbortSignal(composite, sig.reason)
				return compositeObject
			}
		}
		composite.dependent = true
		seen := make(map[*abortSignalState]struct{}, len(signals))
		for _, sig := range signals {
			for _, source := range a.abortOriginalSources(sig) {
				if _, exists := seen[source]; exists {
					continue
				}
				seen[source] = struct{}{}
				a.linkAbortSignal(source, composite)
			}
		}
		return compositeObject
	}); err != nil {
		return fmt.Errorf("bind AbortSignal.any: %w", err)
	}

	// AbortSignal.timeout(ms)
	// Creates a signal that aborts after the specified timeout
	if err := defineWebMethod(a.runtime, abortSignalObj, "timeout", 1, true, func(call goja.FunctionCall) goja.Value {
		delayMs := a.abortTimeoutDelay(call.Argument(0))
		state, obj := a.newAbortSignal()
		stateRef := &abortTimeoutRef{adapter: weak.Make(a), signal: weak.Make(state)}
		state.timeout = stateRef
		callback := a.runtime.ToValue(func(goja.FunctionCall) goja.Value {
			liveState := stateRef.retained.Load()
			if liveState == nil {
				liveState = stateRef.signal.Value()
			}
			if liveState != nil {
				a.abortSignalState(liveState, a.timeoutReason())
			}
			return goja.Undefined()
		})
		timer, handle, err := a.newTimer(adapterTimerAbort, callback, delayMs, nil, false)
		if err != nil {
			a.panicJSException(err)
		}
		stateRef.timerID = timer.id
		if err := a.activateTimer(timer, handle); err != nil {
			a.panicJSException(err)
		}
		cleanup := runtime.AddCleanup(state, cleanupAbortTimeout, abortTimeoutCleanup{
			adapter: weak.Make(a),
			timerID: timer.id,
		})
		stateRef.mu.Lock()
		stateRef.cleanup = cleanup
		stateRef.cleanupSet = true
		stateRef.mu.Unlock()
		return obj
	}); err != nil {
		return fmt.Errorf("bind AbortSignal.timeout: %w", err)
	}

	return nil
}

func (a *Adapter) abortTimeoutDelay(value goja.Value) float64 {
	if value == nil {
		value = goja.Undefined()
	}
	delay := value.ToFloat()
	if math.IsNaN(delay) || math.IsInf(delay, 0) {
		panic(a.runtime.NewTypeError("The \"delay\" argument is outside the accepted range"))
	}
	delay = math.Trunc(delay)
	if delay < 0 || delay > maxTimerID {
		panic(a.runtime.NewTypeError("The \"delay\" argument is outside the accepted range"))
	}
	return delay
}
