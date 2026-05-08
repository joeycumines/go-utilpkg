package gojaeventloop

import (
	"errors"
	"fmt"

	"github.com/joeycumines/goja"
)

func (a *Adapter) allocateImmediateID() (uint64, error) {
	a.immediatesMu.Lock()
	defer a.immediatesMu.Unlock()
	if a.nextImmediateID >= maxTimerID {
		return 0, errors.New("goja-eventloop: immediate ID exhausted")
	}
	a.nextImmediateID++
	return a.nextImmediateID, nil
}

func (a *Adapter) createImmediateHandle(state *adapterImmediate, obj *goja.Object) (*goja.Object, error) {
	if obj == nil {
		obj = a.runtime.NewObject()
		if a.immediatePrototype != nil {
			if err := obj.SetPrototype(a.immediatePrototype); err != nil {
				return nil, fmt.Errorf("goja-eventloop: set Immediate prototype: %w", err)
			}
		}
	}
	argumentSet := state.argumentSet
	if argumentSet == nil {
		argumentSet = goja.Undefined()
	}
	callbackValue := state.callbackValue
	if callbackValue == nil {
		callbackValue = goja.Undefined()
	}
	if a.immediateInitializer == nil {
		return nil, errors.New("goja-eventloop: Immediate initializer is unavailable")
	}
	state.object = obj
	a.setHiddenState(a.immediateStateStore, obj, state)
	state.initializing.Store(true)
	if _, err := a.immediateInitializer(
		goja.Undefined(), obj, callbackValue, argumentSet,
	); err != nil {
		state.initializing.Store(false)
		state.initializerFailed.Store(true)
		accepted := !a.exiting.Load()
		if accepted && state.counted.Load() {
			a.immediatesMu.Lock()
			if !a.exiting.Load() {
				a.immediates[state.id] = state
			} else {
				accepted = false
			}
			a.immediatesMu.Unlock()
			if accepted {
				if scheduleErr := a.enqueueImmediate(state); scheduleErr == nil {
					return nil, wrapRuntimeError("initialize Immediate handle", err)
				}
			}
		}
		if state.refed.Load() && accepted {
			a.setImmediateStateCarrier(state, true)
			a.immediatesMu.Lock()
			a.immediates[state.id] = state
			a.immediatesMu.Unlock()
		} else {
			a.clearImmediateState(state)
		}
		return nil, wrapRuntimeError("initialize Immediate handle", err)
	}
	state.initializing.Store(false)
	return obj, nil
}

func (a *Adapter) constructImmediateHandle(call goja.FunctionCall) goja.Value {
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok || obj == nil {
		panic(a.runtime.NewTypeError("Immediate constructor receiver is not an object"))
	}
	id, err := a.allocateImmediateID()
	if err != nil {
		panic(a.runtime.NewGoError(err))
	}
	callbackValue := call.Argument(1)
	callback, ok := goja.AssertFunction(callbackValue)
	if !ok {
		callback = func(goja.Value, ...goja.Value) (goja.Value, error) {
			panic(a.callbackTypeError(callbackValue))
		}
	}
	state := &adapterImmediate{
		id:            id,
		callback:      callback,
		callbackValue: callbackValue,
		argumentSet:   call.Argument(2),
	}
	handle, err := a.createImmediateHandle(state, obj)
	if err != nil {
		a.panicJSException(err)
	}
	a.immediatesMu.Lock()
	accepted := !a.exiting.Load()
	if accepted {
		a.immediates[id] = state
	}
	a.immediatesMu.Unlock()
	if !accepted {
		a.retireImmediateState(state)
		return handle
	}
	a.markMacrotaskScheduled()
	if err := a.enqueueImmediate(state); err != nil {
		a.clearImmediateState(state)
		panic(a.runtime.NewGoError(err))
	}
	return handle
}

func (a *Adapter) immediateStateObject(obj *goja.Object) (*adapterImmediate, bool) {
	for obj != nil {
		stateValue := a.hiddenState(a.immediateStateStore, obj)
		if stateValue != nil && !goja.IsUndefined(stateValue) && !goja.IsNull(stateValue) {
			state, ok := stateValue.Export().(*adapterImmediate)
			return state, ok && state != nil
		}
		target, proxy := a.proxyTarget(obj)
		if !proxy {
			return nil, false
		}
		obj = target
	}
	return nil, false
}

func (a *Adapter) setImmediate(call goja.FunctionCall) goja.Value {
	fn := call.Argument(0)
	fnCallable, ok := goja.AssertFunction(fn)
	if !ok {
		panic(a.callbackTypeError(fn))
	}
	args := cloneCallArguments(call.Arguments, 1)
	id, err := a.allocateImmediateID()
	if err != nil {
		panic(a.runtime.NewGoError(err))
	}
	state := &adapterImmediate{
		id:            id,
		callback:      fnCallable,
		callbackValue: fn,
		args:          args,
		argumentSet:   timerArgumentSet(a.runtime, args),
	}
	handle, err := a.createImmediateHandle(state, nil)
	if err != nil {
		a.panicJSException(err)
	}
	a.immediatesMu.Lock()
	accepted := !a.exiting.Load()
	if accepted {
		a.immediates[id] = state
	}
	a.immediatesMu.Unlock()
	if !accepted {
		a.retireImmediateState(state)
		return handle
	}
	a.markMacrotaskScheduled()
	if err := a.enqueueImmediate(state); err != nil {
		a.clearImmediateState(state)
		panic(a.runtime.NewGoError(err))
	}
	return handle
}

func (a *Adapter) clearImmediateState(state *adapterImmediate) {
	if state == nil {
		return
	}
	a.immediatesMu.Lock()
	if current, ok := a.immediates[state.id]; ok && current == state {
		delete(a.immediates, state.id)
	}
	a.immediatesMu.Unlock()
	a.retireImmediateState(state)
}

func (a *Adapter) retireImmediateState(state *adapterImmediate) {
	a.setImmediateStateCarrier(state, false)
	retireImmediateState(state)
}

func retireImmediateState(state *adapterImmediate) {
	if state == nil {
		return
	}
	state.canceled.Store(true)
	state.refed.Store(false)
	state.initializing.Store(false)
	state.counted.Store(false)
	state.initializerFailed.Store(false)
	state.corePending.Store(false)
	state.carrierHeld.Store(false)
	state.object = nil
	state.callback = nil
	state.callbackValue = nil
	clear(state.args)
	state.args = nil
	state.argumentSet = nil
}

func (a *Adapter) enqueueImmediate(state *adapterImmediate) error {
	if state == nil || state.canceled.Load() {
		return nil
	}
	err := a.loop.ScheduleImmediateRef(
		func() { a.runAdapterImmediate(state) },
		func() bool { return !state.canceled.Load() && state.refed.Load() },
	)
	if err != nil {
		return err
	}
	state.corePending.Store(true)
	a.setImmediateStateCarrier(state, false)
	return nil
}

func (a *Adapter) runAdapterImmediate(state *adapterImmediate) {
	if state == nil || state.canceled.Load() || a.exiting.Load() {
		return
	}
	a.immediatesMu.Lock()
	if current, ok := a.immediates[state.id]; !ok || current != state {
		a.immediatesMu.Unlock()
		return
	}
	object := state.object
	a.immediatesMu.Unlock()
	if object == nil || a.exiting.Load() {
		return
	}
	state.corePending.Store(false)
	if a.immediateInvoker == nil {
		a.reportHostCallbackError("setImmediate", errors.New("goja-eventloop: Immediate invoker is unavailable"))
		a.clearImmediateState(state)
		return
	}
	result, callbackErr := a.immediateInvoker(goja.Undefined(), object)
	if callbackErr != nil {
		if state.refed.Load() {
			a.setImmediateStateCarrier(state, true)
		}
		a.handleImmediateCallbackError(callbackErr)
		return
	}
	outcome, ok := result.(*goja.Object)
	if !ok || outcome == nil {
		a.clearImmediateState(state)
		return
	}
	status := outcome.Get("0").ToInteger()
	switch status {
	case 0:
		if state.initializerFailed.Load() && state.refed.Load() {
			a.setImmediateStateCarrier(state, true)
			return
		}
		a.clearImmediateState(state)
	case 1:
		a.clearImmediateState(state)
	case 2:
		if state.refed.Load() {
			a.setImmediateStateCarrier(state, true)
		}
		a.handleImmediateCallbackValue(outcome.Get("1"))
	case 3:
		a.handleImmediateCallbackValue(outcome.Get("1"))
		if !a.exiting.Load() {
			a.clearImmediateState(state)
		}
	case 4:
		if state.refed.Load() {
			a.setImmediateStateCarrier(state, true)
		}
	default:
		a.reportHostCallbackError("setImmediate", errors.New("goja-eventloop: invalid Immediate outcome"))
		a.clearImmediateState(state)
	}
}

func (a *Adapter) handleImmediateCallbackValue(value goja.Value) {
	if value == nil {
		value = goja.Undefined()
	}
	exception := a.runtime.Try(func() { panic(value) })
	if exception == nil {
		a.reportHostCallbackError("setImmediate", errors.New("goja-eventloop: missing Immediate exception"))
		a.requestFatalExit()
		return
	}
	a.handleImmediateCallbackError(exception)
}

func (a *Adapter) handleImmediateCallbackError(err error) {
	if !a.handleHostCallbackResult("setImmediate", err) {
		return
	}
	if a.immediateCycleEnsurer == nil {
		a.reportHostCallbackError("setImmediate", errors.New("goja-eventloop: handled Immediate cycle is unavailable"))
		a.requestFatalExit()
		return
	}
	if _, cycleErr := a.immediateCycleEnsurer(goja.Undefined()); cycleErr != nil {
		a.reportHostCallbackError("setImmediate", cycleErr)
		a.requestFatalExit()
		return
	}
	a.yieldMicrotasks()
}
