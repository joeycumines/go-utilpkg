package gojaeventloop

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	"github.com/joeycumines/logiface"
)

const fatalExceptionExitCode = 7

type processEmitOutcome struct {
	emitted          bool
	exceptionHandled bool
	terminal         bool
}

func (a *Adapter) callHostCallback(name string, fn goja.Callable, this goja.Value, args ...goja.Value) bool {
	return a.callHostCallbackGated(name, fn, this, false, args...)
}

func (a *Adapter) callHostCallbackGated(name string, fn goja.Callable, this goja.Value, allowAfterExit bool, args ...goja.Value) bool {
	if a.exiting.Load() && !allowAfterExit {
		return false
	}
	_, err := fn(this, args...)
	err = wrapRuntimeExceptionError("invoke "+name, err)
	return a.handleHostCallbackResult(name, err)
}

func (a *Adapter) handleHostCallbackResult(name string, err error) bool {
	if err == nil {
		return false
	}
	if _, ok := processExitCode(err); ok {
		a.runtime.ClearInterrupt()
		return false
	}
	return a.handleHostCallbackError(name, err, "uncaughtException")
}

func processExitCode(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	// A raw Goja exception may implement Unwrap by reading its runtime-owned
	// thrown object. Detect it before errors.As can traverse that method while
	// looking for the native process-exit sentinel.
	if _, ok := errors.AsType[*goja.Exception](err); ok {
		return 0, false
	}
	if signal, ok := errors.AsType[processExitSignal](err); ok {
		return signal.code, true
	}
	return 0, false
}

func (a *Adapter) parseProcessExitCode(value goja.Value) (int, bool) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return 0, false
	}
	if symbol, ok := value.(*goja.Symbol); ok {
		panic(a.processCodeTypeError(value, "Received type symbol (Symbol("+symbol.String()+"))"))
	}
	if integer, ok := value.Export().(*big.Int); ok {
		text := "0"
		if integer != nil {
			text = integer.String()
		}
		panic(a.processCodeTypeError(value, "Received type bigint ("+text+"n)"))
	}
	if _, ok := goja.AssertFunction(value); ok {
		name := value.ToObject(a.runtime).Get("name")
		text := ""
		if name != nil && !goja.IsUndefined(name) {
			text = name.String()
		}
		panic(a.processCodeTypeError(value, "Received function "+text))
	}
	if _, ok := value.(*goja.Object); ok {
		object := value.ToObject(a.runtime)
		if object.Prototype() == nil {
			panic(a.processCodeTypeError(value, "Received [Object: null prototype] {}"))
		}
		name := object.ClassName()
		if name == "" || name == "Object" {
			name = "Object"
		}
		panic(a.processCodeTypeError(value, "Received an instance of "+name))
	}
	if exported, ok := value.Export().(bool); ok {
		panic(a.processCodeTypeError(value, fmt.Sprintf("Received type boolean (%t)", exported)))
	}
	var number float64
	switch exported := value.Export().(type) {
	case string:
		text := exported
		if text == "" {
			panic(a.processCodeTypeError(value, "Received type string ("+quoteProcessCodeString(text)+")"))
		}
		number = value.ToFloat()
		if math.IsNaN(number) {
			panic(a.processCodeTypeError(value, "Received type string ("+quoteProcessCodeString(text)+")"))
		}
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		number = value.ToFloat()
	default:
		panic(a.processCodeTypeError(value, fmt.Sprintf("Received type %T", exported)))
	}
	const maxSafeInteger = float64(1<<53 - 1)
	if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number || math.Abs(number) > maxSafeInteger {
		panic(a.processCodeRangeError(number))
	}
	return toProcessExitCode(number), true
}

func quoteProcessCodeString(value string) string {
	units := utf16.Encode([]rune(value))
	if len(units) > 28 {
		units = append(units[:25:25], '.', '.', '.')
		value = string(utf16.Decode(units))
	}
	if strings.ContainsRune(value, '\'') {
		return strconv.Quote(value)
	}
	return "'" + value + "'"
}

func toProcessExitCode(number float64) int {
	const (
		two31 = 1 << 31
		two32 = 1 << 32
	)
	mod := math.Mod(number, two32)
	if mod < 0 {
		mod += two32
	}
	if mod >= two31 {
		mod -= two32
	}
	return int(mod)
}

func (a *Adapter) processCodeTypeError(value goja.Value, received string) *goja.Object {
	message := "The \"code\" argument must be of type number. " + received
	return a.processCodeError("TypeError", message, "ERR_INVALID_ARG_TYPE")
}

func (a *Adapter) processCodeRangeError(value float64) *goja.Object {
	constraint := "an integer"
	received := formatJSNumber(value)
	if !math.IsNaN(value) && !math.IsInf(value, 0) && math.Abs(value) > float64(1<<53-1) {
		constraint = ">= -9007199254740991 && <= 9007199254740991"
		received = formatProcessCodeNumber(a.runtime, value)
	}
	message := "The value of \"code\" is out of range. It must be " + constraint + ". Received " + received
	return a.processCodeError("RangeError", message, "ERR_OUT_OF_RANGE")
}

func formatProcessCodeNumber(runtime *goja.Runtime, value float64) string {
	text := runtime.ToValue(value).String()
	sign := ""
	if len(text) != 0 && text[0] == '-' {
		sign = "-"
		text = text[1:]
	}
	first := len(text) % 3
	if first == 0 {
		first = 3
	}
	var formatted strings.Builder
	formatted.WriteString(sign + text[:first])
	for offset := first; offset < len(text); offset += 3 {
		formatted.WriteString("_" + text[offset:offset+3])
	}
	return formatted.String()
}

func (a *Adapter) processCodeError(name, message, code string) *goja.Object {
	if name == "TypeError" {
		obj := a.runtime.NewTypeError(message)
		a.defineErrorData(obj, "code", a.runtime.ToValue(code), goja.FLAG_TRUE)
		return obj
	}
	if name == "RangeError" && a.rangeErrorConstructor != nil {
		value, err := a.rangeErrorConstructor(goja.Undefined(), a.runtime.ToValue(message))
		if err != nil {
			a.panicJSException(wrapRuntimeError("construct process code RangeError", err))
		}
		obj := value.ToObject(a.runtime)
		a.defineErrorData(obj, "code", a.runtime.ToValue(code), goja.FLAG_TRUE)
		return obj
	}
	obj := a.runtime.NewGoError(errors.New(message))
	a.defineErrorData(obj, "name", a.runtime.ToValue(name), goja.FLAG_FALSE)
	a.defineErrorData(obj, "message", a.runtime.ToValue(message), goja.FLAG_FALSE)
	a.defineErrorData(obj, "code", a.runtime.ToValue(code), goja.FLAG_TRUE)
	return obj
}

func (a *Adapter) markMacrotaskScheduled() {
	if a == nil {
		return
	}
	a.suppressBeforeExit.Store(false)
}

func (a *Adapter) handleHostCallbackError(name string, err error, origin string) bool {
	if err == nil {
		return false
	}
	err = wrapRuntimeExceptionError("invoke "+name, err)
	if origin == "" {
		origin = "uncaughtException"
	}
	value := a.errorValue(err)
	switch a.dispatchUncaught(value, origin) {
	case uncaughtHandled:
		return true
	case uncaughtTerminal:
		return false
	}
	a.reportHostCallbackError(name, err)
	a.requestFatalExit()
	return false
}

func (a *Adapter) errorValue(err error) goja.Value {
	if err == nil {
		return goja.Undefined()
	}
	if exception, ok := errors.AsType[*goja.Exception](err); ok {
		return exception.Value()
	}
	return a.runtime.NewGoError(err)
}

func cloneCallArguments(args []goja.Value, start int) []goja.Value {
	if start >= len(args) {
		return nil
	}
	return slices.Clone(args[start:])
}

func (a *Adapter) reportHostCallbackError(name string, err error) {
	if err == nil {
		return
	}
	if a == nil {
		return
	}
	err = wrapRuntimeExceptionError("report "+name, err)
	logStructuredAdapterError(a.loop, func(event logiface.Event) {
		addAdapterLogString(event, "component", "goja-eventloop")
		addAdapterLogString(event, "callback", name)
		addAdapterLogError(event, err)
		addAdapterLogMessage(event, "goja host callback failed")
	})
}

func (a *Adapter) requestFatalExit() {
	if a == nil || a.loop == nil {
		return
	}
	a.emitExit(1, true)
	state := a.loop.State()
	if state == goeventloop.StateTerminating || state == goeventloop.StateTerminated {
		return
	}
	_ = a.loop.Shutdown(context.Background())
}

func (a *Adapter) requestFatalAbort(code int) {
	if a == nil || a.loop == nil {
		return
	}
	if code == 0 {
		code = 1
	}
	a.exitCode.Store(int64(code))
	a.exitCodeSet.Store(true)
	a.exitEmitted.Store(true)
	a.setExiting()
	state := a.loop.State()
	if state == goeventloop.StateTerminating || state == goeventloop.StateTerminated {
		return
	}
	_ = a.loop.Shutdown(context.Background())
}

func (a *Adapter) setExiting() {
	a.exiting.Store(true)
}

func (a *Adapter) currentExitCode() int {
	if a == nil || !a.exitCodeSet.Load() {
		return 0
	}
	return int(a.exitCode.Load())
}

func (a *Adapter) emitExit(code int, storeExitCode bool) {
	if a == nil || !a.exitEmitted.CompareAndSwap(false, true) {
		return
	}
	if storeExitCode {
		a.exitCode.Store(int64(code))
		a.exitCodeSet.Store(true)
	}
	a.setExiting()
	if a.processObj != nil {
		a.emitProcess("exit", a.runtime.ToValue(code))
	}
}

func (a *Adapter) requestProcessExit(code int, storeExitCode bool) {
	if a == nil || a.loop == nil {
		return
	}
	// Node's explicit process.exit path first publishes exitCode, then writes
	// the public _exiting property. The write is observable when user code has
	// replaced that configurable property; natural and fatal exit paths do not
	// invoke such a replacement setter.
	if storeExitCode {
		a.exitCode.Store(int64(code))
		a.exitCodeSet.Store(true)
	}
	if a.processObj != nil {
		if err := a.processObj.Set("_exiting", true); err != nil {
			a.panicJSException(wrapRuntimeError("set process._exiting", err))
		}
	}
	a.emitExit(code, false)
	state := a.loop.State()
	if state == goeventloop.StateTerminating || state == goeventloop.StateTerminated {
		return
	}
	_ = a.loop.Shutdown(context.Background())
}

func (a *Adapter) handleQuiescence() bool {
	if a == nil {
		return false
	}
	if a.exiting.Load() {
		a.emitExit(a.currentExitCode(), false)
		return false
	}
	if a.suppressBeforeExit.Load() {
		a.emitExit(a.currentExitCode(), false)
		return false
	}
	if a.processObj != nil {
		outcome := a.emitProcessOutcome("beforeExit", a.runtime.ToValue(a.currentExitCode()))
		if outcome.exceptionHandled {
			a.yieldMicrotasks()
		}
	}
	if a.loop != nil && a.loop.HasMacrotaskWork() {
		a.suppressBeforeExit.Store(false)
		return true
	}
	if a.loop != nil && a.loop.Alive() {
		a.suppressBeforeExit.Store(true)
		return true
	}
	a.emitExit(a.currentExitCode(), false)
	return false
}

func (a *Adapter) emitProcess(event string, args ...goja.Value) bool {
	return a.emitProcessOutcome(event, args...).emitted
}

func (a *Adapter) emitProcessOutcome(event string, args ...goja.Value) processEmitOutcome {
	if a == nil || a.runtime == nil || a.processObj == nil {
		return processEmitOutcome{}
	}
	callArgs := make([]goja.Value, len(args)+1)
	callArgs[0] = a.runtime.ToValue(event)
	copy(callArgs[1:], args)

	var (
		result      goja.Value
		err         error
		notCallable bool
	)
	if exception := a.runtime.Try(func() {
		emit, callable := goja.AssertFunction(a.processObj.Get("emit"))
		if !callable {
			notCallable = true
			return
		}
		result, err = emit(a.processObj, callArgs...)
	}); exception != nil {
		err = wrapRuntimeException("emit process event", exception)
	} else {
		err = wrapRuntimeError("emit process event", err)
	}
	if notCallable {
		err := errors.New("process.emit is not a function")
		a.abortProcessEmitter(event, err)
		return processEmitOutcome{emitted: true, terminal: true}
	}
	if err == nil {
		return processEmitOutcome{emitted: result != nil && result.ToBoolean()}
	}
	if _, ok := processExitCode(err); ok {
		a.runtime.ClearInterrupt()
		return processEmitOutcome{emitted: true, terminal: true}
	}
	if event == "exit" || event == "uncaughtException" || event == "uncaughtExceptionMonitor" {
		a.abortProcessEmitter(event, err)
		return processEmitOutcome{emitted: true, terminal: true}
	}
	handled := a.handleHostCallbackError("process."+event, err, "uncaughtException")
	return processEmitOutcome{
		emitted:          true,
		exceptionHandled: handled,
		terminal:         a.exiting.Load(),
	}
}

func (a *Adapter) abortProcessEmitter(event string, err error) {
	code := fatalExceptionExitCode
	if event == "exit" {
		code = a.currentExitCode()
		if code == 0 {
			code = 1
		}
	}
	a.reportHostCallbackError("process."+event, err)
	a.requestFatalAbort(code)
}

func (a *Adapter) yieldMicrotasks() {
	if a == nil || a.loop == nil {
		return
	}
	_ = a.loop.YieldMicrotasks()
}

func (a *Adapter) newCapturedError(message string) *goja.Object {
	constructor, ok := goja.AssertConstructor(a.errorConstructor)
	if !ok {
		panic(a.runtime.NewTypeError("captured Error constructor is unavailable"))
	}
	object, err := constructor(nil, a.runtime.ToValue(message))
	if err != nil {
		a.panicJSException(err)
	}
	return object
}

func (a *Adapter) defineErrorProperty(object *goja.Object, name string, value goja.Value) {
	a.defineErrorData(object, name, value, goja.FLAG_TRUE)
}

func (a *Adapter) defineErrorData(object *goja.Object, name string, value goja.Value, enumerable goja.Flag) {
	if err := object.DefineDataProperty(name, value, goja.FLAG_TRUE, goja.FLAG_TRUE, enumerable); err != nil {
		a.panicJSException(wrapRuntimeError("define Error."+name, err))
	}
}

func (a *Adapter) formatUnhandledErrorReason(reason goja.Value) string {
	if reason == nil || goja.IsUndefined(reason) {
		return "undefined"
	}
	return a.formatInspectValue(reason)
}
