package gojaeventloop

import (
	"errors"
	"fmt"
	"weak"

	"github.com/joeycumines/goja"
)

// initHandlePrototypes installs the retained Node.js timer algorithms. Goja
// remains the single authority for timer-list identity, ordering, refresh,
// liveness, and all observable property operations.
func (a *Adapter) initHandlePrototypes() error {
	disposeSymbol, err := a.ensureDisposeSymbol()
	if err != nil {
		return fmt.Errorf("goja-eventloop: initialize timer dispose symbol: %w", err)
	}
	factoryValue, err := a.runtime.RunString(timerListFactorySource)
	if err != nil {
		return wrapRuntimeError("initialize timer-handle factory", err)
	}
	factory, ok := goja.AssertFunction(factoryValue)
	if !ok {
		return errors.New("goja-eventloop: timer-handle factory is not callable")
	}
	apply, err := runtimeIntrinsic(a.runtime, goja.IntrinsicReflectApply, "Reflect.apply")
	if err != nil {
		return err
	}
	mathMax, err := runtimeIntrinsic(a.runtime, goja.IntrinsicMathMax, "Math.max")
	if err != nil {
		return err
	}
	mathTrunc, err := runtimeIntrinsic(a.runtime, goja.IntrinsicMathTrunc, "Math.trunc")
	if err != nil {
		return err
	}
	stringConvert, err := runtimeIntrinsic(a.runtime, goja.IntrinsicStringConstructor, "String")
	if err != nil {
		return err
	}
	typeError, err := runtimeIntrinsic(a.runtime, goja.IntrinsicTypeErrorConstructor, "TypeError")
	if err != nil {
		return err
	}
	values, err := factory(
		goja.Undefined(),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value { return a.constructTimeoutHandle(call) }),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value { return a.constructImmediateHandle(call) }),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			if err := a.scheduleTimerBackend(call.Argument(0).ToFloat()); err != nil && !timerBackendBenign(err) {
				panic(a.runtime.NewGoError(err))
			}
			return goja.Undefined()
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			a.setTimerBackendRef(call.Argument(0).ToBoolean())
			return goja.Undefined()
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			a.nativeTimerInserted(call.Argument(0))
			return goja.Undefined()
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			a.nativeTimerRetired(call.Argument(0))
			return goja.Undefined()
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			a.nativeTimerStateRef(call.Argument(0), call.Argument(1).ToBoolean())
			return goja.Undefined()
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(a.runTimerUserCallback(
				call.Argument(0),
				call.Argument(1),
				call.Argument(2),
				call.Argument(3),
			))
		}),
		a.runtime.ToValue(func(goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(a.timerBackendNow())
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			a.nativeImmediateRef(call.Argument(0), call.Argument(1).ToBoolean())
			return goja.Undefined()
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			a.nativeImmediateClear(call.Argument(0), call.Argument(1).ToBoolean())
			return goja.Undefined()
		}),
		a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			a.nativeImmediateCounted(call.Argument(0))
			return goja.Undefined()
		}),
		a.runtime.ToValue(func(goja.FunctionCall) goja.Value {
			if err := a.loop.AdvanceMicrotaskCheckpoint(); err != nil {
				panic(a.runtime.NewGoError(err))
			}
			return goja.Undefined()
		}),
		disposeSymbol,
		apply,
		mathMax,
		mathTrunc,
		goja.SymIterator,
		goja.SymToPrimitive,
		stringConvert,
		typeError,
		goja.NewSymbol("refed"),
		goja.NewSymbol("kHasPrimitive"),
		goja.NewSymbol("asyncId"),
		goja.NewSymbol("triggerId"),
		goja.NewSymbol("kAsyncContextFrame"),
	)
	if err != nil {
		return wrapRuntimeError("create timer-handle semantics", err)
	}
	result, ok := values.(*goja.Object)
	if !ok || result == nil {
		return errors.New("goja-eventloop: timer-handle factory returned a non-object")
	}
	value := func(index int) goja.Value { return result.Get(fmt.Sprint(index)) }
	callable := func(index int, name string) (goja.Callable, error) {
		fn, callable := goja.AssertFunction(value(index))
		if !callable {
			return nil, fmt.Errorf("goja-eventloop: timer-handle %s helper is not callable", name)
		}
		return fn, nil
	}
	timeoutConstructor := value(0).ToObject(a.runtime)
	immediateConstructor := value(1).ToObject(a.runtime)
	if timeoutConstructor == nil || immediateConstructor == nil {
		return errors.New("goja-eventloop: timer-handle constructors are not objects")
	}
	if err := verifyFunctionShape(a, timeoutConstructor, functionShape{name: "Timeout", length: 5}); err != nil {
		return fmt.Errorf("goja-eventloop: verify Timeout constructor: %w", err)
	}
	if err := verifyFunctionShape(a, immediateConstructor, functionShape{name: "Immediate", length: 2}); err != nil {
		return fmt.Errorf("goja-eventloop: verify Immediate constructor: %w", err)
	}
	a.timeoutPrototype = timeoutConstructor.Get("prototype").ToObject(a.runtime)
	a.immediatePrototype = immediateConstructor.Get("prototype").ToObject(a.runtime)
	a.clearTimeoutFunction = value(2)
	a.clearIntervalFunction = value(3)
	a.clearImmediateFunction = value(4)
	if a.timeoutInitializer, err = callable(5, "Timeout initializer"); err != nil {
		return err
	}
	if a.immediateInitializer, err = callable(6, "Immediate initializer"); err != nil {
		return err
	}
	if a.timerCallbackRunner, err = callable(7, "Timeout callback runner"); err != nil {
		return err
	}
	if a.timerActivator, err = callable(8, "Timeout activator"); err != nil {
		return err
	}
	if a.timerProcessor, err = callable(9, "timer processor"); err != nil {
		return err
	}
	if a.timerTerminator, err = callable(10, "timer terminator"); err != nil {
		return err
	}
	if a.timerSnapshot, err = callable(11, "timer snapshot"); err != nil {
		return err
	}
	if a.immediateInvoker, err = callable(12, "Immediate invoker"); err != nil {
		return err
	}
	if a.timerDelayCoercer, err = callable(13, "delay coercer"); err != nil {
		return err
	}
	if a.immediateCycleEnsurer, err = callable(14, "handled Immediate cycle"); err != nil {
		return err
	}
	return nil
}

func (a *Adapter) nativeTimerInserted(value goja.Value) {
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		return
	}
	state, ok := a.timerStateObject(obj)
	if !ok || state == nil {
		id, err := a.allocateTimerID()
		if err != nil {
			panic(a.runtime.NewGoError(err))
		}
		state = &adapterTimer{id: id, kind: adapterTimerTimeout}
		a.setHiddenState(a.timerStateStore, obj, state)
	}
	if state.payload.Load() == nil {
		state.payload.Store(&adapterTimerPayload{object: obj})
	}
	state.canceled.Store(false)
	state.active.Store(true)
	a.timersMu.Lock()
	if a.exiting.Load() {
		a.timersMu.Unlock()
		retireTimerState(state)
		return
	}
	a.timers[state.id] = state
	a.timersMu.Unlock()
	a.registerTimerState(state)
}

func (a *Adapter) nativeTimerRetired(value goja.Value) {
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		return
	}
	state, ok := a.timerStateObject(obj)
	if !ok || state == nil {
		return
	}
	a.timersMu.Lock()
	if current, exists := a.timers[state.id]; exists && current == state {
		delete(a.timers, state.id)
	}
	delete(a.timerRegistry.states, weak.Make(state))
	a.timersMu.Unlock()
	if payload := retireTimerState(state); payload != nil {
		payload.object = nil
	}
}

func (a *Adapter) nativeTimerStateRef(value goja.Value, refed bool) {
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		return
	}
	if state, ok := a.timerStateObject(obj); ok {
		state.refed.Store(refed)
		state.refKnown.Store(true)
	}
}

func (a *Adapter) runTimerUserCallback(list, timer, start, retiredAsyncID goja.Value) bool {
	obj, ok := timer.(*goja.Object)
	if !ok || obj == nil || a.timerCallbackRunner == nil || a.exiting.Load() {
		return false
	}
	state, ok := a.timerStateObject(obj)
	name := "setTimeout"
	if ok && state != nil {
		state.executing.Store(true)
		defer state.executing.Store(false)
		switch state.kind {
		case adapterTimerInterval:
			name = "setInterval"
		case adapterTimerAbort:
			name = "AbortSignal.timeout"
		}
	}
	var result goja.Value
	var callbackErr error
	runErr := a.loop.RunCallbackDeferredCheckpoint(func() {
		result, callbackErr = a.timerCallbackRunner(goja.Undefined(), list, timer, start, retiredAsyncID)
		if callbackErr != nil {
			if a.handleHostCallbackResult(name, callbackErr) {
				a.yieldMicrotasks()
			}
			return
		}
		outcome, ok := result.(*goja.Object)
		if !ok || outcome == nil || !outcome.Get("0").ToBoolean() {
			return
		}
		a.handleHostCallbackValue(name, outcome.Get("1"), "uncaughtException")
	})
	if runErr != nil && !timerBackendBenign(runErr) {
		a.reportHostCallbackError(name, runErr)
	}
	return !a.exiting.Load()
}

func (a *Adapter) nativeImmediateRef(value goja.Value, refed bool) {
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		return
	}
	state, ok := a.immediateStateObject(obj)
	if !ok {
		a.setGenericImmediateRef(obj, refed)
		return
	}
	if state.canceled.Load() {
		a.setGenericImmediateRef(obj, refed)
		return
	}
	previous := state.refed.Swap(refed)
	if previous == refed || state.initializing.Load() || state.corePending.Load() {
		return
	}
	a.setImmediateStateCarrier(state, refed)
}

func (a *Adapter) nativeImmediateClear(value goja.Value, outstanding bool) {
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		return
	}
	a.setGenericImmediateRef(obj, false)
	if state, ok := a.immediateStateObject(obj); ok {
		if outstanding {
			state.callback = nil
			state.callbackValue = nil
			clear(state.args)
			state.args = nil
			state.argumentSet = nil
			if !state.corePending.Load() {
				_ = a.enqueueImmediate(state)
			}
			return
		}
		a.clearImmediateStateNative(state)
	}
}

func (a *Adapter) nativeImmediateCounted(value goja.Value) {
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		return
	}
	if state, ok := a.immediateStateObject(obj); ok {
		state.counted.Store(true)
	}
}

func (a *Adapter) setGenericImmediateRef(obj *goja.Object, refed bool) {
	if a == nil || obj == nil || a.genericImmediateStore == nil {
		return
	}
	value := a.hiddenState(a.genericImmediateStore, obj)
	var state *genericImmediateState
	if value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
		state, _ = value.Export().(*genericImmediateState)
	}
	if state == nil {
		if !refed {
			return
		}
		state = new(genericImmediateState)
		a.setHiddenState(a.genericImmediateStore, obj, state)
	}
	if refed {
		if state.held.CompareAndSwap(false, true) {
			a.setGenericImmediateCarrier(true)
		}
		return
	}
	if state.held.CompareAndSwap(true, false) {
		a.setGenericImmediateCarrier(false)
	}
}

func (a *Adapter) clearImmediateStateNative(state *adapterImmediate) {
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

func (a *Adapter) setImmediateStateCarrier(state *adapterImmediate, held bool) {
	if a == nil || state == nil {
		return
	}
	if held {
		if state.carrierHeld.CompareAndSwap(false, true) {
			a.setGenericImmediateCarrier(true)
		}
		return
	}
	if state.carrierHeld.CompareAndSwap(true, false) {
		a.setGenericImmediateCarrier(false)
	}
}

func (a *Adapter) setGenericImmediateCarrier(refed bool) {
	if a == nil || a.js == nil {
		return
	}
	if refed {
		a.timersMu.Lock()
		if a.exiting.Load() {
			a.timersMu.Unlock()
			return
		}
		if a.genericImmediateRefs > 0 {
			a.genericImmediateRefs++
			a.timersMu.Unlock()
			return
		}
		a.genericImmediateRefs = 1
		a.timersMu.Unlock()
		id, err := a.js.SetInterval(func() {}, nodeTimerDelayMax)
		if err != nil {
			a.timersMu.Lock()
			a.genericImmediateRefs = 0
			a.timersMu.Unlock()
			return
		}
		a.timersMu.Lock()
		if a.exiting.Load() || a.genericImmediateRefs == 0 {
			a.timersMu.Unlock()
			_ = a.js.ClearInterval(id)
			return
		}
		a.genericImmediateRefID = id
		a.timersMu.Unlock()
		return
	}
	a.timersMu.Lock()
	if a.genericImmediateRefs == 0 {
		a.timersMu.Unlock()
		return
	}
	a.genericImmediateRefs--
	if a.genericImmediateRefs != 0 {
		a.timersMu.Unlock()
		return
	}
	id := a.genericImmediateRefID
	a.genericImmediateRefID = 0
	a.timersMu.Unlock()
	if id != 0 {
		_ = a.js.ClearInterval(id)
	}
}
