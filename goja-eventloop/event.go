package gojaeventloop

import (
	"errors"
	"fmt"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

type eventState struct {
	event         *goeventloop.Event
	target        goja.Value
	currentTarget goja.Value
	detail        goja.Value
	timeStamp     float64
	eventPhase    int
	dispatching   bool
	inPassive     bool
	isTrusted     bool
	composed      bool
	stop          bool
	stopImmediate bool
}

const (
	eventPhaseNone      = 0
	eventPhaseCapturing = 1
	eventPhaseAtTarget  = 2
	eventPhaseBubbling  = 3
)

func (a *Adapter) bindEventPrototype(constructor *goja.Object) error {
	if err := defineWebConstructorObject(a.runtime, constructor, "Event", 1); err != nil {
		return err
	}
	prototype, _ := constructor.Get("prototype").(*goja.Object)
	if prototype == nil {
		return fmt.Errorf("event prototype not found")
	}
	if err := defineWebAccessor(a.runtime, prototype, "type", true, func(call goja.FunctionCall) goja.Value {
		return a.runtime.ToValue(a.eventThis(call.This).event.Type)
	}, nil); err != nil {
		return fmt.Errorf("bind Event.prototype.type: %w", err)
	}
	targetGetter := func(call goja.FunctionCall) goja.Value {
		state := a.eventThis(call.This)
		if state.target == nil {
			return goja.Null()
		}
		return state.target
	}
	if err := defineWebAccessor(a.runtime, prototype, "target", true, targetGetter, nil); err != nil {
		return fmt.Errorf("bind Event.prototype.target: %w", err)
	}
	if err := defineWebAccessor(a.runtime, prototype, "srcElement", true, targetGetter, nil); err != nil {
		return fmt.Errorf("bind Event.prototype.srcElement: %w", err)
	}
	if err := defineWebAccessor(a.runtime, prototype, "currentTarget", true, func(call goja.FunctionCall) goja.Value {
		state := a.eventThis(call.This)
		if state.currentTarget == nil {
			return goja.Null()
		}
		return state.currentTarget
	}, nil); err != nil {
		return fmt.Errorf("bind Event.prototype.currentTarget: %w", err)
	}
	for _, accessor := range []struct {
		name string
		get  func(goja.FunctionCall) goja.Value
	}{
		{"eventPhase", func(call goja.FunctionCall) goja.Value { return a.runtime.ToValue(a.eventThis(call.This).eventPhase) }},
		{"timeStamp", func(call goja.FunctionCall) goja.Value { return a.runtime.ToValue(a.eventThis(call.This).timeStamp) }},
		{"bubbles", func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(a.eventThis(call.This).event.Bubbles)
		}},
		{"cancelable", func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(a.eventThis(call.This).event.Cancelable)
		}},
		{"defaultPrevented", func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(a.eventThis(call.This).event.DefaultPrevented)
		}},
		{"composed", func(call goja.FunctionCall) goja.Value { return a.runtime.ToValue(a.eventThis(call.This).composed) }},
	} {
		if err := defineWebAccessor(a.runtime, prototype, accessor.name, true, accessor.get, nil); err != nil {
			return fmt.Errorf("bind Event.prototype.%s: %w", accessor.name, err)
		}
	}
	if err := defineWebAccessor(a.runtime, prototype, "cancelBubble", true,
		func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(a.eventThis(call.This).stop)
		},
		func(call goja.FunctionCall) goja.Value {
			if call.Argument(0).ToBoolean() {
				state := a.eventThis(call.This)
				state.stop = true
				state.event.StopPropagation()
			}
			return goja.Undefined()
		}); err != nil {
		return fmt.Errorf("bind Event.prototype.cancelBubble: %w", err)
	}
	if err := defineWebAccessor(a.runtime, prototype, "returnValue", true,
		func(call goja.FunctionCall) goja.Value {
			return a.runtime.ToValue(!a.eventThis(call.This).event.DefaultPrevented)
		},
		func(call goja.FunctionCall) goja.Value {
			state := a.eventThis(call.This)
			if !call.Argument(0).ToBoolean() && !state.inPassive {
				state.event.PreventDefault()
			}
			return goja.Undefined()
		}); err != nil {
		return fmt.Errorf("bind Event.prototype.returnValue: %w", err)
	}
	if err := defineWebMethod(a.runtime, prototype, "composedPath", 0, true, func(call goja.FunctionCall) goja.Value {
		state := a.eventThis(call.This)
		if !state.dispatching || state.currentTarget == nil || goja.IsNull(state.currentTarget) {
			return a.runtime.NewArray()
		}
		return a.runtime.NewArray(state.currentTarget)
	}); err != nil {
		return fmt.Errorf("bind Event.prototype.composedPath: %w", err)
	}
	if err := defineWebMethod(a.runtime, prototype, "preventDefault", 0, true, func(call goja.FunctionCall) goja.Value {
		state := a.eventThis(call.This)
		if !state.inPassive {
			state.event.PreventDefault()
		}
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind Event.prototype.preventDefault: %w", err)
	}
	if err := defineWebMethod(a.runtime, prototype, "stopPropagation", 0, true, func(call goja.FunctionCall) goja.Value {
		state := a.eventThis(call.This)
		state.stop = true
		state.event.StopPropagation()
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind Event.prototype.stopPropagation: %w", err)
	}
	if err := defineWebMethod(a.runtime, prototype, "stopImmediatePropagation", 0, true, func(call goja.FunctionCall) goja.Value {
		state := a.eventThis(call.This)
		state.stop = true
		state.stopImmediate = true
		state.event.StopImmediatePropagation()
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind Event.prototype.stopImmediatePropagation: %w", err)
	}
	if err := defineWebMethod(a.runtime, prototype, "initEvent", 1, true, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(a.runtime.NewTypeError("Event.initEvent requires a type argument"))
		}
		eventType := a.webIDLString(call.Argument(0))
		state := a.eventThis(call.This)
		if !state.dispatching {
			a.initializeEvent(state, eventType, call.Argument(1).ToBoolean(), call.Argument(2).ToBoolean())
		}
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind Event.prototype.initEvent: %w", err)
	}
	for _, constant := range []struct {
		name  string
		value int
	}{
		{"NONE", eventPhaseNone},
		{"CAPTURING_PHASE", eventPhaseCapturing},
		{"AT_TARGET", eventPhaseAtTarget},
		{"BUBBLING_PHASE", eventPhaseBubbling},
	} {
		if err := constructor.DefineDataProperty(constant.name, a.runtime.ToValue(constant.value), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
			return fmt.Errorf("define Event.%s: %w", constant.name, err)
		}
		if err := prototype.DefineDataProperty(constant.name, a.runtime.ToValue(constant.value), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
			return fmt.Errorf("define Event.prototype.%s: %w", constant.name, err)
		}
	}
	isTrusted := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.runtime.ToValue(a.eventThis(call.This).isTrusted)
	})
	if err := defineWebFunction(a.runtime, isTrusted, "get isTrusted", 0, "define Event isTrusted getter"); err != nil {
		return err
	}
	if err := defineWebTag(a.runtime, prototype, "Event"); err != nil {
		return err
	}
	a.eventPrototype = prototype
	a.eventIsTrustedGetter = isTrusted
	return nil
}

func (a *Adapter) initializeEvent(state *eventState, eventType string, bubbles, cancelable bool) {
	state.event = goeventloop.NewEventWithOptions(eventType, bubbles, cancelable)
	state.target = goja.Null()
	state.currentTarget = goja.Null()
	state.eventPhase = eventPhaseNone
	state.isTrusted = false
	state.stop = false
	state.stopImmediate = false
}

func (a *Adapter) eventThis(value goja.Value) *eventState {
	_, state := a.eventStateArgument(value)
	return state
}

// eventConstructor creates the Event constructor for JavaScript.
func (a *Adapter) eventConstructor(call goja.ConstructorCall) *goja.Object {
	if len(call.Arguments) == 0 {
		panic(a.runtime.NewTypeError("Event requires a type argument"))
	}

	eventType := a.webIDLString(call.Argument(0))
	bubbles, cancelable, composed := a.eventInit(call.Argument(1))

	event := goeventloop.NewEventWithOptions(eventType, bubbles, cancelable)
	a.wrapEventWithObject(event, call.This, false)
	a.eventThis(call.This).composed = composed
	return call.This
}

func (a *Adapter) webIDLString(value goja.Value) string {
	if value == nil {
		value = goja.Undefined()
	}
	converted := value.ToString()
	if _, symbol := converted.(*goja.Symbol); symbol {
		panic(a.runtime.NewTypeError("Cannot convert a Symbol value to a string"))
	}
	return converted.String()
}

func (a *Adapter) eventInit(value goja.Value) (bubbles, cancelable, composed bool) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return false, false, false
	}
	opts, ok := value.(*goja.Object)
	if !ok || opts == nil {
		panic(a.runtime.NewTypeError("EventInit must be an object"))
	}
	if v := opts.Get("bubbles"); v != nil {
		bubbles = v.ToBoolean()
	}
	if v := opts.Get("cancelable"); v != nil {
		cancelable = v.ToBoolean()
	}
	if v := opts.Get("composed"); v != nil {
		composed = v.ToBoolean()
	}
	return bubbles, cancelable, composed
}

// wrapEvent creates a new JS object for an Event.
func (a *Adapter) wrapEvent(event *goeventloop.Event) goja.Value {
	obj := a.runtime.NewObject()
	if prototype := a.eventPrototype; prototype != nil {
		if err := obj.SetPrototype(prototype); err != nil {
			a.panicJSException(wrapRuntimeError("set wrapped Event prototype", err))
		}
	}
	return a.wrapEventWithObject(event, obj, true)
}

func (a *Adapter) eventTimeStamp() float64 {
	if a.eventTimeSource != nil {
		value, err := a.eventTimeSource(a.eventTimeReceiver)
		if err != nil {
			a.panicJSException(err)
		}
		return value.ToFloat()
	}
	if a.performance == nil {
		a.performance = goeventloop.NewLoopPerformance(a.loop)
	}
	return a.performance.Now()
}

// wrapEventWithObject wraps an Event using the provided JS object.
func (a *Adapter) wrapEventWithObject(event *goeventloop.Event, obj *goja.Object, trusted bool) goja.Value {
	state := &eventState{event: event, target: goja.Null(), currentTarget: goja.Null(), timeStamp: a.eventTimeStamp(), isTrusted: trusted}
	a.setHiddenState(a.eventStateStore, obj, state)
	if a.eventIsTrustedGetter == nil || goja.IsUndefined(a.eventIsTrustedGetter) || goja.IsNull(a.eventIsTrustedGetter) {
		panic(a.runtime.NewGoError(errors.New("event isTrusted getter is unavailable")))
	}
	if err := obj.DefineAccessorProperty("isTrusted", a.eventIsTrustedGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		a.panicJSException(wrapRuntimeError("define Event isTrusted own property", err))
	}

	return obj
}

func (a *Adapter) bindCustomEventPrototype(constructor *goja.Object) error {
	if err := defineWebConstructorObject(a.runtime, constructor, "CustomEvent", 1); err != nil {
		return err
	}
	prototype, _ := constructor.Get("prototype").(*goja.Object)
	if prototype == nil {
		return fmt.Errorf("customEvent prototype not found")
	}
	if err := defineWebAccessor(a.runtime, prototype, "detail", true, func(call goja.FunctionCall) goja.Value {
		state := a.eventThis(call.This)
		if state.detail == nil {
			return goja.Null()
		}
		return state.detail
	}, nil); err != nil {
		return fmt.Errorf("bind CustomEvent.prototype.detail: %w", err)
	}
	if err := defineWebMethod(a.runtime, prototype, "initCustomEvent", 1, true, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(a.runtime.NewTypeError("CustomEvent.initCustomEvent requires a type argument"))
		}
		eventType := a.webIDLString(call.Argument(0))
		state := a.eventThis(call.This)
		if state.dispatching {
			return goja.Undefined()
		}
		a.initializeEvent(state, eventType, call.Argument(1).ToBoolean(), call.Argument(2).ToBoolean())
		detail := call.Argument(3)
		if len(call.Arguments) < 4 || goja.IsUndefined(detail) {
			detail = goja.Null()
		}
		state.detail = detail
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind CustomEvent.prototype.initCustomEvent: %w", err)
	}
	return defineWebTag(a.runtime, prototype, "CustomEvent")
}

// CustomEvent Binding

// customEventConstructor creates the CustomEvent constructor for JavaScript.
func (a *Adapter) customEventConstructor(call goja.ConstructorCall) *goja.Object {
	if len(call.Arguments) == 0 {
		panic(a.runtime.NewTypeError("CustomEvent requires a type argument"))
	}

	eventType := a.webIDLString(call.Argument(0))
	bubbles, cancelable, composed := a.eventInit(call.Argument(1))
	detail := goja.Null()

	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
		opts := call.Argument(1).(*goja.Object)
		if v := opts.Get("detail"); v != nil && !goja.IsUndefined(v) {
			detail = v
		}
	}

	customEvent := goeventloop.NewCustomEventWithOptions(eventType, detail, bubbles, cancelable)

	thisObj := call.This

	// Wrap the embedded Event
	a.wrapEventWithObject(customEvent.EventPtr(), thisObj, false)
	if stateValue := a.hiddenState(a.eventStateStore, thisObj); stateValue != nil && !goja.IsUndefined(stateValue) && !goja.IsNull(stateValue) {
		if state, ok := stateValue.Export().(*eventState); ok && state != nil {
			state.detail = detail
			state.composed = composed
		}
	}

	return thisObj
}
