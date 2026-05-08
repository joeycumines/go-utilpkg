package gojaeventloop

import (
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/joeycumines/goja"
)

func (a *Adapter) allocateTimerID() (uint64, error) {
	a.timersMu.Lock()
	defer a.timersMu.Unlock()
	if a.nextTimerID >= maxTimerID {
		return 0, errors.New("goja-eventloop: timer ID exhausted")
	}
	a.nextTimerID++
	return a.nextTimerID, nil
}

func (a *Adapter) createTimerHandle(
	state *adapterTimer,
	obj *goja.Object,
	callback goja.Value,
	idleTimeout float64,
	argumentSet goja.Value,
	repeat goja.Value,
	refed goja.Value,
) (*goja.Object, error) {
	if state == nil {
		return nil, errors.New("goja-eventloop: nil timer state")
	}
	if obj == nil {
		obj = a.runtime.NewObject()
		if a.timeoutPrototype != nil {
			if err := obj.SetPrototype(a.timeoutPrototype); err != nil {
				return nil, fmt.Errorf("goja-eventloop: set Timeout prototype: %w", err)
			}
		}
	}
	if callback == nil {
		callback = goja.Undefined()
	}
	if argumentSet == nil {
		argumentSet = goja.Undefined()
	}
	if repeat == nil {
		repeat = goja.Null()
	}
	if refed == nil {
		refed = goja.Undefined()
	}
	if a.timeoutInitializer == nil {
		return nil, errors.New("goja-eventloop: Timeout initializer is unavailable")
	}
	state.payload.Store(&adapterTimerPayload{object: obj})
	a.setHiddenState(a.timerStateStore, obj, state)
	if _, err := a.timeoutInitializer(
		goja.Undefined(),
		obj,
		callback,
		a.runtime.ToValue(idleTimeout),
		argumentSet,
		repeat,
		refed,
	); err != nil {
		retireTimerState(state)
		return nil, wrapRuntimeError("initialize Timeout handle", err)
	}
	a.registerTimerState(state)
	return obj, nil
}

func (a *Adapter) constructTimeoutHandle(call goja.FunctionCall) goja.Value {
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok || obj == nil {
		panic(a.runtime.NewTypeError("Timeout constructor receiver is not an object"))
	}
	id, err := a.allocateTimerID()
	if err != nil {
		panic(a.runtime.NewGoError(err))
	}
	coercion := a.computeTimerDelay(call.Argument(2))
	kind := adapterTimerTimeout
	repeat := goja.Value(goja.Null())
	if call.Argument(4).ToBoolean() {
		kind = adapterTimerInterval
		repeat = a.runtime.ToValue(coercion.idleTimeout)
	}
	state := &adapterTimer{id: id, kind: kind}
	handle, err := a.createTimerHandle(
		state,
		obj,
		call.Argument(1),
		coercion.idleTimeout,
		call.Argument(3),
		repeat,
		call.Argument(5),
	)
	if err != nil {
		a.panicJSException(err)
	}
	return handle
}

func timerArgumentSet(runtime *goja.Runtime, args []goja.Value) goja.Value {
	if len(args) == 0 {
		return goja.Undefined()
	}
	values := make([]any, len(args))
	for index, value := range args {
		values[index] = value
	}
	return runtime.NewArray(values...)
}

func (a *Adapter) timerStateObject(obj *goja.Object) (*adapterTimer, bool) {
	for obj != nil {
		stateValue := a.hiddenState(a.timerStateStore, obj)
		if stateValue != nil && !goja.IsUndefined(stateValue) && !goja.IsNull(stateValue) {
			state, ok := stateValue.Export().(*adapterTimer)
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

// clearTimer is used only by owner-thread cleanup paths. Observable JS clear
// operations are delegated back to the retained clearTimeout algorithm.
func (a *Adapter) clearTimer(id uint64) {
	a.timersMu.Lock()
	state := a.timers[id]
	a.timersMu.Unlock()
	if state == nil {
		return
	}
	payload := state.payload.Load()
	if payload == nil || payload.object == nil {
		return
	}
	clear, ok := goja.AssertFunction(a.clearTimeoutFunction)
	if !ok {
		return
	}
	if _, err := clear(goja.Undefined(), payload.object); err != nil {
		a.handleHostCallbackResult("clearTimeout", err)
	}
}

const nodeTimerDelayMax = 2147483647

func coerceNodeTimerDelay(value goja.Value) int {
	return computeNodeTimerDelay(value).delay
}

func computeNodeTimerDelay(value goja.Value) timerDelayCoercion {
	if value == nil || goja.IsUndefined(value) {
		return timerDelayCoercion{delay: 1, idleTimeout: 1}
	}
	delay := value.ToFloat()
	if delay >= 1 && delay <= nodeTimerDelayMax {
		return timerDelayCoercion{delay: int(delay), idleTimeout: delay}
	}
	coercion := timerDelayCoercion{delay: 1, idleTimeout: 1}
	switch {
	case delay > nodeTimerDelayMax:
		coercion.name = "TimeoutOverflowWarning"
		coercion.message = formatJSNumber(delay) + " does not fit into a 32-bit signed integer.\nTimeout duration was set to 1."
	case delay < 0:
		coercion.name = "TimeoutNegativeWarning"
		coercion.message = formatJSNumber(delay) + " is a negative number.\nTimeout duration was set to 1."
	case math.IsNaN(delay):
		coercion.name = "TimeoutNaNWarning"
		coercion.message = "NaN is not a number.\nTimeout duration was set to 1."
	}
	return coercion
}

func formatJSNumber(value float64) string {
	switch {
	case math.IsNaN(value):
		return "NaN"
	case math.IsInf(value, 1):
		return "Infinity"
	case math.IsInf(value, -1):
		return "-Infinity"
	default:
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
}

func (a *Adapter) computeTimerDelay(value goja.Value) timerDelayCoercion {
	coerced, err := a.timerDelayCoercer(goja.Undefined(), value)
	if err != nil {
		panic(err)
	}
	coercion := computeNodeTimerDelay(coerced)
	switch coercion.name {
	case "TimeoutOverflowWarning":
		coercion.message = coerced.String() + " does not fit into a 32-bit signed integer.\nTimeout duration was set to 1."
	case "TimeoutNegativeWarning":
		coercion.message = coerced.String() + " is a negative number.\nTimeout duration was set to 1."
	}
	if coercion.name == "" {
		return coercion
	}
	switch coercion.name {
	case "TimeoutNegativeWarning":
		if !a.warnedNegativeDelay.CompareAndSwap(false, true) {
			return coercion
		}
	case "TimeoutNaNWarning":
		if !a.warnedNaNDelay.CompareAndSwap(false, true) {
			return coercion
		}
	}
	a.emitWarningNextTick(coercion.message, coercion.name, "")
	return coercion
}

// nodeIntervalDelay remains a package-level oracle for tests of the retained
// interval arithmetic. Production interval reinsertion executes in Goja.
func nodeIntervalDelay(repeat, elapsed float64) int {
	period := math.Trunc(repeat)
	if math.IsNaN(period) || period < 1 {
		return 1
	}
	if period > nodeTimerDelayMax {
		period = nodeTimerDelayMax
	}
	remaining := period - elapsed
	if remaining < 1 {
		return 1
	}
	return int(math.Ceil(remaining))
}

func (a *Adapter) newTimer(
	kind adapterTimerKind,
	callback goja.Value,
	idleTimeout float64,
	args []goja.Value,
	refed bool,
) (*adapterTimer, *goja.Object, error) {
	id, err := a.allocateTimerID()
	if err != nil {
		return nil, nil, err
	}
	repeat := goja.Value(goja.Null())
	if kind == adapterTimerInterval {
		repeat = a.runtime.ToValue(idleTimeout)
	}
	state := &adapterTimer{id: id, kind: kind}
	var object *goja.Object
	if kind == adapterTimerAbort {
		object = a.runtime.NewObject()
		if err := object.SetPrototype(nil); err != nil {
			return nil, nil, fmt.Errorf("goja-eventloop: isolate AbortSignal timeout handle: %w", err)
		}
	}
	handle, err := a.createTimerHandle(
		state,
		object,
		callback,
		idleTimeout,
		timerArgumentSet(a.runtime, args),
		repeat,
		a.runtime.ToValue(refed),
	)
	if err != nil {
		return nil, nil, err
	}
	return state, handle, nil
}

func (a *Adapter) activateTimer(state *adapterTimer, handle *goja.Object) error {
	if state == nil || handle == nil {
		return errors.New("goja-eventloop: invalid timer activation")
	}
	if a.exiting.Load() {
		clear, ok := goja.AssertFunction(a.clearTimeoutFunction)
		if ok {
			_, _ = clear(goja.Undefined(), handle)
		}
		return nil
	}
	a.markMacrotaskScheduled()
	if _, err := a.timerActivator(goja.Undefined(), handle); err != nil {
		return wrapRuntimeError("activate Timeout handle", err)
	}
	return nil
}

func (a *Adapter) setTimer(call goja.FunctionCall, kind adapterTimerKind) goja.Value {
	callback := call.Argument(0)
	if _, ok := goja.AssertFunction(callback); !ok {
		panic(a.callbackTypeError(callback))
	}
	coercion := a.computeTimerDelay(call.Argument(1))
	args := cloneCallArguments(call.Arguments, 2)
	state, handle, err := a.newTimer(kind, callback, coercion.idleTimeout, args, true)
	if err != nil {
		a.panicJSException(err)
	}
	if err := a.activateTimer(state, handle); err != nil {
		a.panicJSException(err)
	}
	return handle
}

func (a *Adapter) setTimeout(call goja.FunctionCall) goja.Value {
	return a.setTimer(call, adapterTimerTimeout)
}

func (a *Adapter) setInterval(call goja.FunctionCall) goja.Value {
	return a.setTimer(call, adapterTimerInterval)
}

func (a *Adapter) queueMicrotask(call goja.FunctionCall) goja.Value {
	fn := call.Argument(0)
	fnCallable, ok := goja.AssertFunction(fn)
	if !ok {
		panic(a.callbackTypeError(fn))
	}
	if a.exiting.Load() {
		return goja.Undefined()
	}
	if err := a.js.QueueMicrotask(func() {
		a.callHostCallbackGated("queueMicrotask", fnCallable, goja.Undefined(), false)
	}); err != nil {
		panic(a.runtime.NewGoError(err))
	}
	return goja.Undefined()
}
