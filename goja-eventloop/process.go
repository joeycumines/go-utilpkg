package gojaeventloop

import (
	"fmt"
	"math/big"
	"strconv"
	"sync/atomic"

	"github.com/joeycumines/goja"
)

func (a *Adapter) processListenerTypeError(value goja.Value) *goja.Object {
	message := "The \"listener\" argument must be of type function. " + a.formatReceivedValue(value)
	return a.invalidArgumentTypeError(message)
}

func (a *Adapter) callbackTypeError(value goja.Value) *goja.Object {
	return a.invalidArgumentTypeError("The \"callback\" argument must be of type function. " + a.formatReceivedValue(value))
}

func (a *Adapter) invalidArgumentTypeError(message string) *goja.Object {
	err := a.runtime.NewTypeError(message)
	if defineErr := err.DefineDataProperty("code", a.runtime.ToValue("ERR_INVALID_ARG_TYPE"), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE); defineErr != nil {
		a.panicJSException(wrapRuntimeError("define invalid-argument TypeError code", defineErr))
	}
	return err
}

func (a *Adapter) validateEmitWarningErrorArguments(call goja.FunctionCall) {
	typeValue := call.Argument(1)
	if options, ok := typeValue.(*goja.Object); ok && options != nil {
		if value := options.Get("type"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
			if _, ok := primitiveString(value); !ok {
				panic(a.invalidArgumentTypeError("The \"type\" argument must be of type string. " + a.formatReceivedValue(value)))
			}
		}
		if value := options.Get("code"); value != nil && !goja.IsUndefined(value) {
			if _, ok := primitiveString(value); !ok {
				panic(a.invalidArgumentTypeError("The \"code\" argument must be of type string. " + a.formatReceivedValue(value)))
			}
		}
		return
	}
	if typeValue != nil && !goja.IsUndefined(typeValue) {
		if _, ok := primitiveString(typeValue); !ok {
			panic(a.invalidArgumentTypeError("The \"type\" argument must be of type string. " + a.formatReceivedValue(typeValue)))
		}
		codeValue := call.Argument(2)
		if codeValue != nil && !goja.IsUndefined(codeValue) {
			if _, ok := primitiveString(codeValue); !ok {
				panic(a.invalidArgumentTypeError("The \"code\" argument must be of type string. " + a.formatReceivedValue(codeValue)))
			}
		}
	}
}

func (a *Adapter) formatReceivedValue(value goja.Value) string {
	if value == nil || goja.IsUndefined(value) {
		return "Received undefined"
	}
	if goja.IsNull(value) {
		return "Received null"
	}
	if symbol, ok := value.(*goja.Symbol); ok {
		return "Received type symbol (Symbol(" + symbol.String() + "))"
	}
	if obj, ok := value.(*goja.Object); ok {
		if obj == nil {
			return "Received an instance of Object"
		}
		className := obj.ClassName()
		if className == "Object" && obj.Prototype() == nil {
			return "Received " + a.formatInspectValue(value)
		}
		name := className
		if name == "" || name == "Object" {
			if constructorName := a.objectConstructorName(obj); constructorName != "" {
				name = constructorName
			}
		}
		if name == "" {
			name = "Object"
		}
		return "Received an instance of " + name
	}
	switch exported := value.Export().(type) {
	case string:
		return "Received type string (" + quoteInspectString(exported) + ")"
	case bool:
		return "Received type boolean (" + strconv.FormatBool(exported) + ")"
	case int:
		return "Received type number (" + strconv.Itoa(exported) + ")"
	case int64:
		return "Received type number (" + strconv.FormatInt(exported, 10) + ")"
	case float64:
		return "Received type number (" + strconv.FormatFloat(exported, 'f', -1, 64) + ")"
	case *big.Int:
		if exported == nil {
			return "Received type bigint (0n)"
		}
		return "Received type bigint (" + exported.String() + "n)"
	default:
		return "Received " + value.String()
	}
}

// process.nextTick() Binding

// bindProcess creates the process object with nextTick method.
// This emulates Node.js process.nextTick() semantics.
func (a *Adapter) bindProcess(processObj *goja.Object) error {
	if processObj == nil {
		return fmt.Errorf("bind process: detached process object is unavailable")
	}
	setProcessProperty := func(name string, value goja.Value) error {
		if err := processObj.DefineDataProperty(
			name,
			value,
			goja.FLAG_TRUE,
			goja.FLAG_TRUE,
			goja.FLAG_TRUE,
		); err != nil {
			return wrapRuntimeError("define process."+name, err)
		}
		return nil
	}
	processAccessor := func(name string, length int64, callback func(goja.FunctionCall) goja.Value) (goja.Value, error) {
		value := a.runtime.ToValue(callback)
		if err := defineWebFunction(a.runtime, value, name, length, "define process."+name); err != nil {
			return nil, err
		}
		return value, nil
	}
	processFunction := func(name string, length int64, callback func(goja.FunctionCall) goja.Value) (goja.Value, error) {
		value, err := a.ordinaryFunction(callback)
		if err != nil {
			return nil, fmt.Errorf("define process.%s: %w", name, err)
		}
		if err := defineWebFunction(a.runtime, value, name, length, "define process."+name); err != nil {
			return nil, err
		}
		return value, nil
	}
	publicExiting := new(atomic.Bool)
	publicExiting.Store(a.exiting.Load())
	exitingGetter, err := processAccessor("get", 0, func(goja.FunctionCall) goja.Value {
		return a.runtime.ToValue(publicExiting.Load() || a.exiting.Load())
	})
	if err != nil {
		return err
	}
	exitingSetter, err := processAccessor("set", 1, func(call goja.FunctionCall) goja.Value {
		publicExiting.Store(call.Argument(0).ToBoolean())
		return goja.Undefined()
	})
	if err != nil {
		return err
	}
	if err := processObj.DefineAccessorProperty("_exiting", exitingGetter, exitingSetter, goja.FLAG_TRUE, goja.FLAG_TRUE); err != nil {
		return wrapRuntimeError("define process._exiting", err)
	}
	exitCodeGetter, err := processAccessor("get", 0, func(goja.FunctionCall) goja.Value {
		if !a.exitCodeSet.Load() {
			return goja.Undefined()
		}
		code := a.exitCode.Load()
		return a.runtime.ToValue(int(code))
	})
	if err != nil {
		return err
	}
	exitCodeSetter, err := processAccessor("set", 1, func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0)
		if goja.IsNull(value) {
			a.exitCodeSet.Store(false)
			return goja.Undefined()
		}
		code, ok := a.parseProcessExitCode(value)
		if !ok {
			a.exitCodeSet.Store(false)
			return goja.Undefined()
		}
		a.exitCode.Store(int64(code))
		a.exitCodeSet.Store(true)
		return goja.Undefined()
	})
	if err != nil {
		return err
	}
	if err := processObj.DefineAccessorProperty("exitCode", exitCodeGetter, exitCodeSetter, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return wrapRuntimeError("define process.exitCode", err)
	}

	// Node exposes the EventEmitter methods two links above process itself:
	// process -> process.prototype -> EventEmitter.prototype -> Object.prototype.
	// Build that chain detached so the retained methods have the exact lookup
	// depth without adding the rest of EventEmitter's surface.
	emitterCore := a.processEmitterCore
	if emitterCore == nil {
		return fmt.Errorf("bind process: EventEmitter core is unavailable")
	}
	eventEmitterConstructor := emitterCore.constructor
	if err := defineWebFunction(a.runtime, eventEmitterConstructor, "EventEmitter", 1, "define process EventEmitter constructor"); err != nil {
		return err
	}
	eventEmitterPrototype, ok := eventEmitterConstructor.Get("prototype").(*goja.Object)
	if !ok || eventEmitterPrototype == nil {
		return fmt.Errorf("bind process: EventEmitter prototype is not an object")
	}
	if err := eventEmitterPrototype.SetPrototype(processObj.Prototype()); err != nil {
		return wrapRuntimeError("bind process: set EventEmitter prototype inheritance", err)
	}

	processConstructorValue := a.runtime.ToValue(func(call goja.ConstructorCall) *goja.Object {
		return call.This
	})
	processConstructor, ok := processConstructorValue.(*goja.Object)
	if !ok || processConstructor == nil {
		return fmt.Errorf("bind process: process constructor is not an object")
	}
	if err := defineWebFunction(a.runtime, processConstructor, "process", 0, "define process constructor"); err != nil {
		return err
	}
	processPrototype, ok := processConstructor.Get("prototype").(*goja.Object)
	if !ok || processPrototype == nil {
		return fmt.Errorf("bind process: process prototype is not an object")
	}
	if err := processPrototype.SetPrototype(eventEmitterPrototype); err != nil {
		return wrapRuntimeError("bind process: set process prototype inheritance", err)
	}
	emitWarning, err := processFunction("emitWarning", 4, func(call goja.FunctionCall) goja.Value {
		warning := call.Argument(0)
		if obj, ok := warning.(*goja.Object); ok && a.isErrorValue(warning) {
			a.validateEmitWarningErrorArguments(call)
			// Node reads Error.name before deferring the warning event. Preserve
			// that synchronous getter boundary and its exact thrown value.
			_ = obj.Get("name")
			a.emitWarningObjectNextTick(obj)
			return goja.Undefined()
		}
		message, ok := primitiveString(warning)
		if !ok {
			panic(a.invalidArgumentTypeError("The \"warning\" argument must be of type string or an instance of Error."))
		}
		name := "Warning"
		code := ""
		codeSet := false
		detail := ""
		detailSet := false
		if options, ok := call.Argument(1).(*goja.Object); ok && options != nil {
			if value := options.Get("type"); value != nil && !goja.IsUndefined(value) {
				if !goja.IsNull(value) {
					text, ok := primitiveString(value)
					if !ok {
						panic(a.invalidArgumentTypeError("The \"type\" argument must be of type string. " + a.formatReceivedValue(value)))
					}
					name = text
				}
			}
			if value := options.Get("code"); value != nil && !goja.IsUndefined(value) {
				text, ok := primitiveString(value)
				if !ok {
					panic(a.invalidArgumentTypeError("The \"code\" argument must be of type string. " + a.formatReceivedValue(value)))
				}
				code = text
				codeSet = true
			}
			if value := options.Get("detail"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
				if text, ok := primitiveString(value); ok {
					detail = text
					detailSet = true
				}
			}
		} else if arg := call.Argument(1); arg != nil && !goja.IsUndefined(arg) {
			text, ok := primitiveString(arg)
			if !ok {
				panic(a.invalidArgumentTypeError("The \"type\" argument must be of type string. " + a.formatReceivedValue(arg)))
			}
			name = text
			if codeArg := call.Argument(2); codeArg != nil && !goja.IsUndefined(codeArg) {
				text, ok := primitiveString(codeArg)
				if !ok {
					panic(a.invalidArgumentTypeError("The \"code\" argument must be of type string. " + a.formatReceivedValue(codeArg)))
				}
				code = text
				codeSet = true
			}
		}
		a.emitWarningObjectNextTick(a.warningObjectDetailed(message, name, code, codeSet, detail, detailSet))
		return goja.Undefined()
	})
	if err != nil {
		return err
	}
	if err := setProcessProperty("emitWarning", emitWarning); err != nil {
		return err
	}
	exit, err := processFunction("exit", 1, func(call goja.FunctionCall) goja.Value {
		code := 0
		storeExitCode := false
		if len(call.Arguments) == 0 && a.exitCodeSet.Load() {
			code = int(a.exitCode.Load())
			storeExitCode = true
		}
		if len(call.Arguments) != 0 {
			arg := call.Argument(0)
			if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
				a.exitCodeSet.Store(false)
				code = 0
				storeExitCode = false
			} else {
				var ok bool
				code, ok = a.parseProcessExitCode(arg)
				storeExitCode = ok
			}
		}
		a.requestProcessExit(code, storeExitCode)
		a.runtime.Interrupt(processExitSignal{code: code})
		return goja.Undefined()
	})
	if err != nil {
		return err
	}
	if err := setProcessProperty("exit", exit); err != nil {
		return err
	}

	// process.nextTick(fn) - schedules fn to run before microtasks
	nextTick, err := processFunction("nextTick", 1, func(call goja.FunctionCall) goja.Value {
		fn := call.Argument(0)
		fnCallable, ok := goja.AssertFunction(fn)
		if !ok {
			panic(a.callbackTypeError(fn))
		}
		if publicExiting.Load() || a.exiting.Load() {
			return goja.Undefined()
		}

		args := cloneCallArguments(call.Arguments, 1)

		// Use the Go NextTick implementation
		err := a.js.NextTick(func() {
			if a.callHostCallback("process.nextTick", fnCallable, goja.Undefined(), args...) {
				a.yieldMicrotasks()
			}
		})
		if err != nil {
			panic(a.runtime.NewGoError(err))
		}

		return goja.Undefined()
	})
	if err != nil {
		return err
	}
	if err := setProcessProperty("nextTick", nextTick); err != nil {
		return err
	}

	// Retained EventEmitter names are adapter-owned. Remove any shadowing own
	// properties before publishing the detached prototype chain so their
	// descriptors and depth are deterministic. Bind's mutation journal restores
	// foreign descriptors and the original [[Prototype]] on failure.
	for _, name := range []string{
		"_events",
		"_eventsCount",
		"_maxListeners",
		"on",
		"addListener",
		"once",
		"off",
		"removeListener",
		"emit",
		"listenerCount",
	} {
		if err := processObj.Delete(name); err != nil {
			return wrapRuntimeError("delete shadowing process."+name, err)
		}
	}
	for _, property := range []struct {
		name  string
		value goja.Value
	}{
		{name: "_events", value: goja.Undefined()},
		{name: "_eventsCount", value: a.runtime.ToValue(0)},
		{name: "_maxListeners", value: goja.Undefined()},
	} {
		if err := processObj.DefineDataProperty(
			property.name,
			property.value,
			goja.FLAG_TRUE,
			goja.FLAG_TRUE,
			goja.FLAG_TRUE,
		); err != nil {
			return wrapRuntimeError("define process."+property.name, err)
		}
	}
	if err := processObj.SetPrototype(processPrototype); err != nil {
		return wrapRuntimeError("set process prototype", err)
	}
	if _, err := emitterCore.initializeProcess(goja.Undefined(), processObj); err != nil {
		return wrapRuntimeError("initialize process EventEmitter state", err)
	}

	a.processObj = processObj
	a.processEmitterCore = nil
	return nil
}
