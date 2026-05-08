package gojaeventloop

import (
	"errors"
	"fmt"

	"github.com/joeycumines/goja"
)

type processEmitterCore struct {
	constructor       *goja.Object
	addListener       goja.Value
	once              goja.Value
	removeListener    goja.Value
	emit              goja.Value
	listenerCount     goja.Value
	initialize        goja.Callable
	initializeProcess goja.Callable
}

func (a *Adapter) newProcessEmitterCore() (*processEmitterCore, error) {
	if a == nil || a.runtime == nil {
		return nil, errors.New("bind process: runtime is unavailable")
	}
	validateListener := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		listener := call.Argument(0)
		if _, ok := goja.AssertFunction(listener); !ok {
			panic(a.processListenerTypeError(listener))
		}
		return listener
	})
	formatUnhandled := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.runtime.ToValue(a.formatUnhandledErrorReason(call.Argument(0)))
	})
	makeMaxListenerWarning := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return a.maxListenerWarning(
			call.Argument(0),
			call.Argument(1),
			int(call.Argument(2).ToInteger()),
			call.Argument(3),
			call.Argument(4).String(),
		)
	})
	emitMaxListenerWarning := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		process := a.processObj
		if process == nil {
			panic(a.runtime.NewTypeError("process is unavailable"))
		}
		emitWarning, ok := goja.AssertFunction(process.Get("emitWarning"))
		if !ok {
			panic(a.runtime.NewTypeError("process.emitWarning is not callable"))
		}
		if _, err := emitWarning(process, call.Argument(0)); err != nil {
			a.panicJSException(err)
		}
		return goja.Undefined()
	})
	factoryValue, err := a.runtime.RunString(processEmitterFactorySource)
	if err != nil {
		return nil, wrapRuntimeError("compile process EventEmitter core", err)
	}
	factory, ok := goja.AssertFunction(factoryValue)
	if !ok {
		return nil, errors.New("bind process: EventEmitter core factory is not callable")
	}
	primordialSpecs := []struct {
		id   goja.Intrinsic
		name string
	}{
		{id: goja.IntrinsicReflectApply, name: "Reflect.apply"},
		{id: goja.IntrinsicObjectCreate, name: "Object.create"},
		{id: goja.IntrinsicObjectGetPrototypeOf, name: "Object.getPrototypeOf"},
		{id: goja.IntrinsicObjectDefineProperty, name: "Object.defineProperty"},
		{id: goja.IntrinsicObjectSetPrototypeOf, name: "Object.setPrototypeOf"},
		{id: goja.IntrinsicArrayIndexOf, name: "Array.prototype.indexOf"},
		{id: goja.IntrinsicArrayJoin, name: "Array.prototype.join"},
		{id: goja.IntrinsicArraySlice, name: "Array.prototype.slice"},
		{id: goja.IntrinsicArraySplice, name: "Array.prototype.splice"},
		{id: goja.IntrinsicErrorConstructor, name: "Error"},
		{id: goja.IntrinsicErrorPrototype, name: "Error.prototype"},
		{id: goja.IntrinsicFunctionBind, name: "Function.prototype.bind"},
		{id: goja.IntrinsicMathMin, name: "Math.min"},
		{id: goja.IntrinsicStringConstructor, name: "String"},
		{id: goja.IntrinsicStringSplit, name: "String.prototype.split"},
	}
	arguments := []goja.Value{validateListener, formatUnhandled, makeMaxListenerWarning, emitMaxListenerWarning}
	for _, spec := range primordialSpecs {
		value, err := runtimeIntrinsic(a.runtime, spec.id, spec.name)
		if err != nil {
			return nil, err
		}
		arguments = append(arguments, value)
	}
	arguments = append(
		arguments,
		goja.NewSymbol("kCapture"),
		goja.NewSymbol("events.errorMonitor"),
		goja.NewSymbol("kEnhanceStackBeforeInspector"),
		goja.NewSymbol("shapeMode"),
		goja.NewSymbol("events.emitting"),
	)
	coreValue, err := factory(goja.Undefined(), arguments...)
	if err != nil {
		return nil, wrapRuntimeError("initialize process EventEmitter core", err)
	}
	coreObject, ok := coreValue.(*goja.Object)
	if !ok || coreObject == nil {
		return nil, errors.New("bind process: EventEmitter core is not an object")
	}
	constructor, ok := coreObject.Get("EventEmitter").(*goja.Object)
	if !ok || constructor == nil {
		return nil, errors.New("bind process: EventEmitter constructor is not an object")
	}
	if _, ok := goja.AssertConstructor(constructor); !ok {
		return nil, errors.New("bind process: EventEmitter constructor is not constructable")
	}
	initialize, ok := goja.AssertFunction(coreObject.Get("initialize"))
	if !ok {
		return nil, errors.New("bind process: EventEmitter initializer is not callable")
	}
	initializeProcess, ok := goja.AssertFunction(coreObject.Get("initializeProcess"))
	if !ok {
		return nil, errors.New("bind process: process EventEmitter initializer is not callable")
	}
	emitValue := coreObject.Get("emit")
	if _, ok := goja.AssertFunction(emitValue); !ok {
		return nil, errors.New("bind process: EventEmitter emit is not callable")
	}
	core := &processEmitterCore{
		constructor:       constructor,
		addListener:       coreObject.Get("addListener"),
		once:              coreObject.Get("once"),
		removeListener:    coreObject.Get("removeListener"),
		emit:              emitValue,
		listenerCount:     coreObject.Get("listenerCount"),
		initialize:        initialize,
		initializeProcess: initializeProcess,
	}
	for name, value := range map[string]goja.Value{
		"addListener":    core.addListener,
		"once":           core.once,
		"removeListener": core.removeListener,
		"emit":           core.emit,
		"listenerCount":  core.listenerCount,
	} {
		if _, ok := goja.AssertFunction(value); !ok {
			return nil, fmt.Errorf("bind process: EventEmitter %s is not callable", name)
		}
	}
	return core, nil
}

// processEmitterFactorySource is a bounded transcription of the retained
// EventEmitter operations in Node.js v26.5.0 lib/events.js. Keeping the core in
// JavaScript preserves native property-key coercion, Proxy trap order, and
// user-visible _events state without maintaining a second listener registry.
const processEmitterFactorySource = `
((validateListener, formatUnhandled, makeMaxListenerWarning, emitMaxListenerWarning,
  ReflectApply,
  ObjectCreate,
  ObjectGetPrototypeOf,
  ObjectDefineProperty,
  ObjectSetPrototypeOf,
  ArrayPrototypeIndexOf,
  ArrayPrototypeJoin,
  ArrayPrototypeSlice,
  ArrayPrototypeSplice,
  ErrorConstructor,
  ErrorPrototype,
  FunctionPrototypeBind,
  MathMin,
  StringConstructor,
  StringPrototypeSplit,
  kCapture,
  kErrorMonitor,
  kEnhanceStackBeforeInspector,
  kShapeMode,
  kEmitting
) => {
  "use strict";

  const unhandledErrorPrototype = ObjectCreate(ErrorPrototype);
  ObjectDefineProperty(unhandledErrorPrototype, "constructor", {
    get() { return ErrorConstructor; },
    configurable: true,
  });
  ObjectDefineProperty(unhandledErrorPrototype, "toString", {
    value: function toString() {
      return this.name + " [" + this.code + "]: " + this.message;
    },
    writable: true,
    configurable: true,
  });

  function makeUnhandled(error) {
    const message = "Unhandled error. (" + formatUnhandled(error) + ")";
    const result = new ErrorConstructor(message);
    delete result.message;
    ObjectDefineProperty(result, "code", {
      value: "ERR_UNHANDLED_ERROR",
      writable: true,
      enumerable: true,
      configurable: true,
    });
    ObjectDefineProperty(result, "message", {
      value: message,
      writable: true,
      configurable: true,
    });
    ObjectDefineProperty(result, "context", {
      value: error,
      writable: true,
      enumerable: true,
      configurable: true,
    });
    ObjectSetPrototypeOf(result, unhandledErrorPrototype);
    return result;
  }

  function initialize(target) {
    if (target._events === undefined ||
        target._events === ObjectGetPrototypeOf(target)._events) {
      target._events = { __proto__: null };
      target._eventsCount = 0;
      target[kShapeMode] = false;
    } else {
      target[kShapeMode] = true;
    }
    target._maxListeners ||= undefined;
    target[kCapture] = EventEmitter.prototype[kCapture];
    return target;
  }

  function EventEmitter(opts) {
    initialize(this, opts);
  }

  // Node installs these internal hooks on process so adding and removing OS
  // signal listeners can update native signal watchers. This retained profile
  // does not expose OS signal delivery, but preserves their observable
  // EventEmitter state and callable shapes.
  function startListeningIfSignal(type) {}
  function stopListeningIfSignal(type) {}

  function initializeProcess(target) {
    initialize(target);
    target._events.newListener = startListeningIfSignal;
    target._events.removeListener = stopListeningIfSignal;
    target._eventsCount = 2;
    return target;
  }

  function arrayClone(array) {
    switch (array.length) {
      case 2: return [array[0], array[1]];
      case 3: return [array[0], array[1], array[2]];
      case 4: return [array[0], array[1], array[2], array[3]];
      case 5: return [array[0], array[1], array[2], array[3], array[4]];
      case 6: return [array[0], array[1], array[2], array[3], array[4], array[5]];
    }
    return ReflectApply(ArrayPrototypeSlice, array, []);
  }

  function cloneEventListenerArray(array) {
    const copy = arrayClone(array);
    copy[kEmitting] = 0;
    if (array.warned) copy.warned = true;
    return copy;
  }

  function ensureMutableListenerArray(events, type, handler) {
    if (handler[kEmitting] > 0) {
      const copy = cloneEventListenerArray(handler);
      events[type] = copy;
      return copy;
    }
    return handler;
  }

  function maxListeners(target) {
    if (target._maxListeners === undefined) return 10;
    return target._maxListeners;
  }

  function addListener(type, listener) {
    validateListener(listener);

    let events = this._events;
    let existing;
    if (events === undefined) {
      events = this._events = { __proto__: null };
      this._eventsCount = 0;
    } else {
      if (events.newListener !== undefined) {
        this.emit("newListener", type, listener.listener ?? listener);
        events = this._events;
      }
      existing = events[type];
    }

    if (existing === undefined) {
      events[type] = listener;
      ++this._eventsCount;
    } else if (typeof existing === "function") {
      existing = [existing, listener];
      existing[kEmitting] = 0;
      events[type] = existing;
    } else {
      existing = ensureMutableListenerArray(events, type, existing);
      existing.push(listener);
    }
    if (existing !== undefined) {
      const maximum = maxListeners(this);
      if (maximum > 0 && existing.length > maximum && !existing.warned) {
        existing.warned = true;
        emitMaxListenerWarning(makeMaxListenerWarning(
          this, type, existing.length, maximum, StringConstructor(type)));
      }
    }
    return this;
  }

  function onceWrapper() {
    if (!this.fired) {
      this.target.removeListener(this.type, this.wrapFn);
      this.fired = true;
      if (arguments.length === 0) return this.listener.call(this.target);
      return ReflectApply(this.listener, this.target, arguments);
    }
  }

  function onceWrap(target, type, listener) {
    const state = { fired: false, wrapFn: undefined, target, type, listener };
    const wrapped = onceWrapper.bind(state);
    wrapped.listener = listener;
    state.wrapFn = wrapped;
    return wrapped;
  }

  function once(type, listener) {
    validateListener(listener);
    this.on(type, onceWrap(this, type, listener));
    return this;
  }

  function spliceOne(list, index) {
    for (; index + 1 < list.length; index++) list[index] = list[index + 1];
    list.pop();
  }

  function removeListener(type, listener) {
    validateListener(listener);

    const events = this._events;
    if (events === undefined) return this;

    let list = events[type];
    if (list === undefined) return this;

    if (list === listener || list.listener === listener) {
      this._eventsCount -= 1;
      if (this[kShapeMode]) {
        events[type] = undefined;
      } else if (this._eventsCount === 0) {
        this._events = { __proto__: null };
      } else {
        delete events[type];
      }
      if (events.removeListener !== undefined) {
        this.emit("removeListener", type, list.listener || listener);
      }
    } else if (typeof list !== "function") {
      list = ensureMutableListenerArray(events, type, list);
      let position = -1;
      for (let index = list.length - 1; index >= 0; index--) {
        if (list[index] === listener || list[index].listener === listener) {
          position = index;
          break;
        }
      }
      if (position < 0) return this;
      if (position === 0) list.shift();
      else spliceOne(list, position);
      if (list.length === 1) events[type] = list[0];
      if (events.removeListener !== undefined) {
        this.emit("removeListener", type, listener);
      }
    }
    return this;
  }

  function emit(type, ...args) {
    let doError = type === "error";
    const events = this._events;
    if (events !== undefined) {
      if (doError && events[kErrorMonitor] !== undefined) {
        this.emit(kErrorMonitor, ...args);
      }
      doError &&= events.error === undefined;
    } else if (!doError) {
      return false;
    }
    if (doError) {
      const error = args[0];
      if (error instanceof ErrorConstructor) {
        try {
          const capture = new ErrorConstructor();
          function enhanceStackTrace() {
            let constructorInfo = "";
            try {
              const name = this.constructor.name;
              if (name !== "EventEmitter") constructorInfo = " on " + name + " instance";
            } catch (_) {}
            const separator = "\nEmitted 'error' event" + constructorInfo + " at:\n";
            const errorStack = ReflectApply(ArrayPrototypeSlice,
              ReflectApply(StringPrototypeSplit, StringConstructor(error.stack), ["\n"]), [1]);
            const ownStack = ReflectApply(ArrayPrototypeSlice,
              ReflectApply(StringPrototypeSplit, StringConstructor(capture.stack), ["\n"]), [1]);
            const range = identicalSequenceRange(ownStack, errorStack);
            if (range[0] > 0) {
              ReflectApply(ArrayPrototypeSplice, ownStack, [
                range[1] + 1,
                range[0] - 2,
                "    [... lines matching original stack trace ...]",
              ]);
            }
            return error.stack + separator + ReflectApply(ArrayPrototypeJoin, ownStack, ["\n"]);
          }
          ObjectDefineProperty(error, kEnhanceStackBeforeInspector, {
            value: ReflectApply(FunctionPrototypeBind, enhanceStackTrace, [this]),
            configurable: true,
          });
        } catch (_) {}
        throw error;
      }
      throw makeUnhandled(error);
    }

    const handler = events[type];
    if (handler === undefined) return false;
    if (typeof handler === "function") {
      const result = ReflectApply(handler, this, args);
      if (result !== undefined && result !== null) void this[kCapture];
    } else {
      handler[kEmitting]++;
      try {
        for (let index = 0; index < handler.length; index++) {
          const result = ReflectApply(handler[index], this, args);
          if (result !== undefined && result !== null) void this[kCapture];
        }
      } finally {
        handler[kEmitting]--;
      }
    }
    return true;
  }

  function identicalSequenceRange(first, second) {
    for (let index = 0; index < first.length - 3; index++) {
      const position = ReflectApply(ArrayPrototypeIndexOf, second, [first[index]]);
      if (position === -1) continue;
      const rest = second.length - position;
      if (rest <= 3) continue;
      let length = 1;
      const maximum = MathMin(first.length - index, rest);
      while (maximum > length && first[index + length] === second[position + length]) length++;
      if (length > 3) return [length, index];
    }
    return [0, 0];
  }

  function listenerCount(type, listener) {
    const events = this._events;
    if (events !== undefined) {
      const eventListener = events[type];
      if (typeof eventListener === "function") {
        if (listener != null) {
          return listener === eventListener || listener === eventListener.listener ? 1 : 0;
        }
        return 1;
      }
      if (eventListener !== undefined) {
        if (listener != null) {
          let matching = 0;
          for (let index = 0, length = eventListener.length; index < length; index++) {
            if (eventListener[index] === listener ||
                eventListener[index].listener === listener) {
              matching++;
            }
          }
          return matching;
        }
        return eventListener.length;
      }
    }
    return 0;
  }

  EventEmitter.prototype._events = undefined;
  EventEmitter.prototype._eventsCount = 0;
  EventEmitter.prototype._maxListeners = undefined;
  ObjectDefineProperty(EventEmitter.prototype, kCapture, {
    value: false,
    writable: true,
    configurable: false,
    enumerable: false,
  });
  EventEmitter.prototype.addListener = addListener;
  EventEmitter.prototype.on = addListener;
  EventEmitter.prototype.once = once;
  EventEmitter.prototype.removeListener = removeListener;
  EventEmitter.prototype.off = removeListener;
  EventEmitter.prototype.emit = emit;
  EventEmitter.prototype.listenerCount = listenerCount;

  return {
    EventEmitter,
    initialize,
		initializeProcess,
    addListener,
    once,
    removeListener,
    emit,
    listenerCount,
  };
})
`
