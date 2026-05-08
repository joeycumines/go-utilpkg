package gojaeventloop

import (
	"fmt"
	"sync"
	"sync/atomic"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// EventTarget and Event Bindings

// eventTargetListenerInfo tracks adapter-owned listener identity. DOM / Node
// listener identity is the tuple (event type, callback object, capture), not an
// ID stored on the callback; storing state on user functions breaks duplicate
// handling and removal after the same callback is registered for multiple types.
type eventTargetListenerInfo struct {
	callback      goja.Value
	signalCleanup func()
	removed       atomic.Bool
	eventHandler  bool
	capture       bool
	once          bool
	passive       bool
}

func (info *eventTargetListenerInfo) detachSignalCleanup() func() {
	if info == nil {
		return nil
	}
	cleanup := info.signalCleanup
	info.signalCleanup = nil
	return cleanup
}

func removeEventTargetListenerSlot(infos []*eventTargetListenerInfo, index int) []*eventTargetListenerInfo {
	if index < 0 || index >= len(infos) {
		return infos
	}
	copy(infos[index:], infos[index+1:])
	infos[len(infos)-1] = nil
	return infos[:len(infos)-1]
}

// eventTargetWrapper wraps an EventTarget with Goja-specific state.
type eventTargetWrapper struct {
	target        *goeventloop.EventTarget // retained for Go-dispatch fallback tests / APIs
	object        *goja.Object
	abortSignal   *abortSignalState
	listeners     map[string][]*eventTargetListenerInfo // eventType -> listener infos
	goListenerIDs map[string]goeventloop.ListenerID
	mu            sync.Mutex
}

// eventTargetConstructor creates the EventTarget constructor for JavaScript.
func (a *Adapter) eventTargetConstructor(call goja.ConstructorCall) *goja.Object {
	a.initEventTargetObject(call.This)
	return call.This
}

func (a *Adapter) bindEventTargetPrototype(constructor *goja.Object) error {
	if err := defineWebConstructorObject(a.runtime, constructor, "EventTarget", 0); err != nil {
		return err
	}
	prototype, _ := constructor.Get("prototype").(*goja.Object)
	if prototype == nil {
		return fmt.Errorf("eventTarget prototype not found")
	}
	if err := defineWebMethod(a.runtime, prototype, "addEventListener", 2, true, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(a.runtime.NewTypeError("EventTarget.addEventListener requires 2 arguments"))
		}
		wrapper := a.eventTargetThis(call.This)
		a.eventTargetAdd(wrapper, a.webIDLString(call.Argument(0)), call.Argument(1), call.Argument(2))
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind EventTarget.prototype.addEventListener: %w", err)
	}
	if err := defineWebMethod(a.runtime, prototype, "removeEventListener", 2, true, func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(a.runtime.NewTypeError("EventTarget.removeEventListener requires 2 arguments"))
		}
		wrapper := a.eventTargetThis(call.This)
		a.eventTargetRemove(wrapper, a.webIDLString(call.Argument(0)), call.Argument(1), eventListenerCapture(a.runtime, call.Argument(2)))
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind EventTarget.prototype.removeEventListener: %w", err)
	}
	if err := defineWebMethod(a.runtime, prototype, "dispatchEvent", 1, true, func(call goja.FunctionCall) goja.Value {
		wrapper := a.eventTargetThis(call.This)
		eventObj, state := a.eventStateArgument(call.Argument(0))
		if state.dispatching {
			panic(a.throwDOMException("InvalidStateError", "The event is already being dispatched."))
		}
		state.isTrusted = false
		return a.runtime.ToValue(a.dispatchJSEvent(wrapper, eventObj, state))
	}); err != nil {
		return fmt.Errorf("bind EventTarget.prototype.dispatchEvent: %w", err)
	}
	return defineWebTag(a.runtime, prototype, "EventTarget")
}

func (a *Adapter) eventTargetThis(value goja.Value) *eventTargetWrapper {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		panic(a.runtime.NewTypeError("EventTarget method called on incompatible receiver"))
	}
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		panic(a.runtime.NewTypeError("EventTarget method called on incompatible receiver"))
	}
	stateValue := a.hiddenState(a.eventTargetStateStore, obj)
	if stateValue == nil || goja.IsUndefined(stateValue) || goja.IsNull(stateValue) {
		panic(a.runtime.NewTypeError("EventTarget method called on incompatible receiver"))
	}
	wrapper, ok := stateValue.Export().(*eventTargetWrapper)
	if !ok || wrapper == nil {
		panic(a.runtime.NewTypeError("EventTarget method called on incompatible receiver"))
	}
	return wrapper
}

func (a *Adapter) initEventTargetObject(obj *goja.Object) *eventTargetWrapper {
	wrapper := &eventTargetWrapper{
		target:        goeventloop.NewEventTarget(),
		object:        obj,
		listeners:     make(map[string][]*eventTargetListenerInfo),
		goListenerIDs: make(map[string]goeventloop.ListenerID),
	}
	a.setHiddenState(a.eventTargetStateStore, obj, wrapper)
	return wrapper
}

func eventListenerCapture(runtime *goja.Runtime, options goja.Value) bool {
	if options == nil || goja.IsUndefined(options) || goja.IsNull(options) {
		return false
	}
	if _, ok := options.(*goja.Object); !ok {
		return options.ToBoolean()
	}
	value := options.ToObject(runtime).Get("capture")
	return value != nil && value.ToBoolean()
}

func eventListenerOnce(runtime *goja.Runtime, options goja.Value) bool {
	if options == nil || goja.IsUndefined(options) || goja.IsNull(options) {
		return false
	}
	obj, ok := options.(*goja.Object)
	if !ok || obj == nil {
		return false
	}
	value := obj.Get("once")
	return value != nil && value.ToBoolean()
}

func eventListenerPassive(options goja.Value) bool {
	if options == nil || goja.IsUndefined(options) || goja.IsNull(options) {
		return false
	}
	obj, ok := options.(*goja.Object)
	if !ok || obj == nil {
		return false
	}
	value := obj.Get("passive")
	return value != nil && value.ToBoolean()
}

func (a *Adapter) eventListenerSignal(options goja.Value) *abortSignalState {
	if options == nil || goja.IsUndefined(options) || goja.IsNull(options) {
		return nil
	}
	obj, ok := options.(*goja.Object)
	if !ok || obj == nil {
		return nil
	}
	value := obj.Get("signal")
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil
	}
	return a.abortSignalValueMessage(value, "addEventListener options.signal must be an AbortSignal")
}

func (a *Adapter) eventTargetAdd(wrapper *eventTargetWrapper, eventType string, callback goja.Value, options goja.Value) *eventTargetListenerInfo {
	if callback != nil && !goja.IsUndefined(callback) && !goja.IsNull(callback) {
		if _, ok := callback.(*goja.Object); !ok {
			panic(a.invalidArgumentTypeError("The \"listener\" argument must be an instance of EventListener. " + a.formatReceivedValue(callback)))
		}
	}
	return a.eventTargetAddConfigured(
		wrapper,
		eventType,
		callback,
		eventListenerCapture(a.runtime, options),
		eventListenerOnce(a.runtime, options),
		eventListenerPassive(options),
		true,
		false,
		a.eventListenerSignal(options),
	)
}

func (a *Adapter) eventTargetAddConfigured(wrapper *eventTargetWrapper, eventType string, callback goja.Value, capture bool, once bool, passive bool, dedupe bool, eventHandler bool, signal *abortSignalState) *eventTargetListenerInfo {
	if wrapper == nil || callback == nil || goja.IsUndefined(callback) || goja.IsNull(callback) {
		return nil
	}
	if _, ok := goja.AssertFunction(callback); !ok {
		obj, ok := callback.(*goja.Object)
		if !ok || obj == nil {
			return nil
		}
	}
	if signal != nil && signal.aborted {
		return nil
	}
	wrapper.mu.Lock()
	if dedupe {
		for _, info := range wrapper.listeners[eventType] {
			if !info.eventHandler && !info.removed.Load() && info.capture == capture && info.callback.SameAs(callback) {
				wrapper.mu.Unlock()
				return info
			}
		}
	}
	info := &eventTargetListenerInfo{callback: callback, eventHandler: eventHandler, capture: capture, once: once, passive: passive}
	if _, ok := wrapper.goListenerIDs[eventType]; !ok {
		wrapper.goListenerIDs[eventType] = wrapper.target.AddEventListener(eventType, func(e *goeventloop.Event) {
			a.dispatchGoEvent(wrapper, e)
		})
	}
	wrapper.listeners[eventType] = append(wrapper.listeners[eventType], info)
	wrapper.mu.Unlock()
	if eventType == "abort" {
		changeAbortSignalObservers(wrapper.abortSignal, 1)
	}
	if signal != nil {
		cleanup, aborted := a.addAbortAlgorithm(signal, func() {
			a.eventTargetRemoveInfo(wrapper, eventType, info)
		})
		if aborted {
			a.eventTargetRemoveInfo(wrapper, eventType, info)
			return nil
		}
		if cleanup != nil {
			wrapper.mu.Lock()
			removed := info.removed.Load()
			if !removed {
				info.signalCleanup = cleanup
			}
			wrapper.mu.Unlock()
			if removed {
				cleanup()
			}
		}
	}
	return info
}

func (a *Adapter) dispatchGoEvent(wrapper *eventTargetWrapper, event *goeventloop.Event) {
	if wrapper == nil || event == nil {
		return
	}
	var jsEvent goja.Value
	if stored, ok := a.dispatchJSEvents.Load(event); ok {
		jsEvent = stored.(goja.Value)
	} else {
		jsEvent = a.wrapEvent(event)
		a.dispatchJSEvents.Store(event, jsEvent)
		defer a.dispatchJSEvents.Delete(event)
	}
	eventObj, state := a.eventStateArgument(jsEvent)
	if state.dispatching {
		panic(a.throwDOMException("InvalidStateError", "The event is already being dispatched."))
	}
	state.dispatching = true
	state.target = wrapper.object
	state.currentTarget = wrapper.object
	state.eventPhase = eventPhaseAtTarget
	defer func() {
		state.currentTarget = goja.Null()
		state.eventPhase = eventPhaseNone
		state.dispatching = false
		state.stop = false
		state.stopImmediate = false
	}()

	for _, capture := range []bool{true, false} {
		if state.stop {
			return
		}
		wrapper.mu.Lock()
		entries := append([]*eventTargetListenerInfo(nil), wrapper.listeners[event.Type]...)
		wrapper.mu.Unlock()
		for _, info := range entries {
			if info == nil || info.capture != capture || info.removed.Load() {
				continue
			}
			if info.once {
				a.eventTargetRemoveInfo(wrapper, event.Type, info)
			}
			a.invokeEventListener(wrapper, info, eventObj)
			if state.stopImmediate || event.ImmediatePropagationStopped() {
				return
			}
		}
	}
}

func (a *Adapter) eventTargetRemove(wrapper *eventTargetWrapper, eventType string, callback goja.Value, capture bool) bool {
	if wrapper == nil || callback == nil || goja.IsUndefined(callback) || goja.IsNull(callback) {
		return false
	}
	if _, ok := callback.(*goja.Object); !ok {
		panic(a.invalidArgumentTypeError("The \"listener\" argument must be an instance of EventListener. " + a.formatReceivedValue(callback)))
	}
	var cleanup func()
	var removeBridge goeventloop.ListenerID
	removed := false
	wrapper.mu.Lock()
	infos := wrapper.listeners[eventType]
	for i, info := range infos {
		if !info.eventHandler && !info.removed.Load() && info.capture == capture && info.callback.SameAs(callback) {
			info.removed.Store(true)
			cleanup = info.detachSignalCleanup()
			infos = removeEventTargetListenerSlot(infos, i)
			if len(infos) == 0 {
				delete(wrapper.listeners, eventType)
				removeBridge = wrapper.goListenerIDs[eventType]
				delete(wrapper.goListenerIDs, eventType)
			} else {
				wrapper.listeners[eventType] = infos
			}
			removed = true
			break
		}
	}
	wrapper.mu.Unlock()
	if removed && eventType == "abort" {
		changeAbortSignalObservers(wrapper.abortSignal, -1)
	}
	if cleanup != nil {
		cleanup()
	}
	if removeBridge != 0 {
		wrapper.target.RemoveEventListenerByID(eventType, removeBridge)
	}
	return removed
}

func (a *Adapter) eventTargetRemoveInfo(wrapper *eventTargetWrapper, eventType string, target *eventTargetListenerInfo) bool {
	if wrapper == nil || target == nil {
		return false
	}
	var cleanup func()
	var removeBridge goeventloop.ListenerID
	removed := false
	wrapper.mu.Lock()
	infos := wrapper.listeners[eventType]
	for i, info := range infos {
		if info == target && !info.removed.Load() {
			info.removed.Store(true)
			cleanup = info.detachSignalCleanup()
			infos = removeEventTargetListenerSlot(infos, i)
			if len(infos) == 0 {
				delete(wrapper.listeners, eventType)
				removeBridge = wrapper.goListenerIDs[eventType]
				delete(wrapper.goListenerIDs, eventType)
			} else {
				wrapper.listeners[eventType] = infos
			}
			removed = true
			break
		}
	}
	wrapper.mu.Unlock()
	if removed && eventType == "abort" {
		changeAbortSignalObservers(wrapper.abortSignal, -1)
	}
	if cleanup != nil {
		cleanup()
	}
	if removeBridge != 0 {
		wrapper.target.RemoveEventListenerByID(eventType, removeBridge)
	}
	return removed
}

func (a *Adapter) eventStateArgument(value goja.Value) (*goja.Object, *eventState) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		panic(a.runtime.NewTypeError("dispatchEvent requires an Event"))
	}
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		panic(a.runtime.NewTypeError("dispatchEvent requires an Event"))
	}
	stateValue := a.hiddenState(a.eventStateStore, obj)
	if stateValue == nil || goja.IsUndefined(stateValue) || goja.IsNull(stateValue) {
		panic(a.runtime.NewTypeError("dispatchEvent requires an Event"))
	}
	state, ok := stateValue.Export().(*eventState)
	if !ok || state == nil || state.event == nil {
		panic(a.runtime.NewTypeError("dispatchEvent requires an Event"))
	}
	return obj, state
}

func (a *Adapter) dispatchJSEvent(wrapper *eventTargetWrapper, eventObj *goja.Object, state *eventState) bool {
	if wrapper == nil || eventObj == nil || state == nil || state.event == nil {
		return true
	}
	if state.dispatching {
		panic(a.throwDOMException("InvalidStateError", "The event is already being dispatched."))
	}
	state.dispatching = true
	state.target = wrapper.object
	state.currentTarget = wrapper.object
	state.eventPhase = eventPhaseAtTarget
	state.event.Target = wrapper.target
	a.dispatchJSEvents.Store(state.event, eventObj)
	defer func() {
		a.dispatchJSEvents.Delete(state.event)
		state.currentTarget = goja.Null()
		state.eventPhase = eventPhaseNone
		state.dispatching = false
		state.stop = false
		state.stopImmediate = false
	}()

	for _, capture := range []bool{true, false} {
		if state.stop {
			return !state.event.Cancelable || !state.event.DefaultPrevented
		}
		wrapper.mu.Lock()
		entries := append([]*eventTargetListenerInfo(nil), wrapper.listeners[state.event.Type]...)
		wrapper.mu.Unlock()
		for _, info := range entries {
			if info == nil || info.capture != capture || info.removed.Load() {
				continue
			}
			if info.once {
				a.eventTargetRemoveInfo(wrapper, state.event.Type, info)
			}
			a.invokeEventListener(wrapper, info, eventObj)
			if state.stopImmediate {
				return !state.event.Cancelable || !state.event.DefaultPrevented
			}
		}
	}
	return !state.event.Cancelable || !state.event.DefaultPrevented
}

func (a *Adapter) invokeEventListener(wrapper *eventTargetWrapper, info *eventTargetListenerInfo, eventObj *goja.Object) {
	if wrapper == nil || info == nil || eventObj == nil {
		return
	}
	var (
		fn   goja.Callable
		this goja.Value
	)
	if f, ok := goja.AssertFunction(info.callback); ok {
		fn = f
		this = wrapper.object
	} else if obj, ok := info.callback.(*goja.Object); ok && obj != nil {
		var handleEvent goja.Value
		if exception := a.runtime.Try(func() { handleEvent = obj.Get("handleEvent") }); exception != nil {
			a.queueEventTargetError(exception)
			return
		}
		var ok bool
		fn, ok = goja.AssertFunction(handleEvent)
		if !ok {
			a.queueEventTargetErrorValue(a.runtime.NewTypeError("EventListener.handleEvent is not callable"))
			return
		}
		this = obj
	}
	if fn == nil {
		return
	}
	_, state := a.eventStateArgument(eventObj)
	previousPassive := state.inPassive
	state.inPassive = info.passive
	defer func() { state.inPassive = previousPassive }()
	_, err := fn(this, eventObj)
	if err != nil {
		a.queueEventTargetError(err)
	}
}

func (a *Adapter) queueEventTargetError(err error) {
	if err == nil || a.exiting.Load() {
		return
	}
	err = wrapRuntimeExceptionError("invoke EventTarget listener", err)
	if qErr := a.js.NextTick(func() {
		if a.handleHostCallbackError("EventTarget.addEventListener", err, "uncaughtException") {
			a.yieldMicrotasks()
		}
	}); qErr != nil {
		if a.handleHostCallbackError("EventTarget.addEventListener", err, "uncaughtException") {
			a.yieldMicrotasks()
		}
	}
}

func (a *Adapter) queueEventTargetErrorValue(value goja.Value) {
	if a.exiting.Load() {
		return
	}
	if value == nil {
		value = goja.Undefined()
	}
	if qErr := a.js.NextTick(func() {
		a.handleHostCallbackValue("EventTarget.addEventListener", value, "uncaughtException")
	}); qErr != nil {
		a.handleHostCallbackValue("EventTarget.addEventListener", value, "uncaughtException")
	}
}

func (a *Adapter) handleHostCallbackValue(name string, value goja.Value, origin string) {
	if origin == "" {
		origin = "uncaughtException"
	}
	if value == nil {
		value = goja.Undefined()
	}
	switch a.dispatchUncaught(value, origin) {
	case uncaughtHandled:
		a.yieldMicrotasks()
		return
	case uncaughtTerminal:
		return
	}
	a.reportHostCallbackError(name, wrapRuntimeValue(a.runtime, "invoke "+name, value))
	a.requestFatalExit()
}
