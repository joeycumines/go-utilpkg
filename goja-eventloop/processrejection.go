package gojaeventloop

import (
	"fmt"
	"strconv"

	"github.com/joeycumines/goja"
)

func (a *Adapter) warningObject(message, name, code string) *goja.Object {
	return a.warningObjectDetailed(message, name, code, code != "", "", false)
}

func (a *Adapter) warningObjectDetailed(message, name, code string, codeSet bool, detail string, detailSet bool) *goja.Object {
	if name == "" {
		name = "Warning"
	}
	warning := a.newCapturedError(message)
	a.defineErrorProperty(warning, "name", a.runtime.ToValue(name))
	if codeSet {
		a.defineErrorProperty(warning, "code", a.runtime.ToValue(code))
	}
	if detailSet {
		a.defineErrorProperty(warning, "detail", a.runtime.ToValue(detail))
	}
	return warning
}

func (a *Adapter) maxListenerWarning(target, event goja.Value, count int, maximum goja.Value, eventName string) *goja.Object {
	label := "Object"
	if targetObject, ok := target.(*goja.Object); ok && targetObject != nil {
		switch {
		case targetObject == a.processObj:
			label = "process"
		case targetObject.ClassName() == "Object":
			if name := a.objectConstructorName(targetObject); name != "" {
				label = name
			}
		default:
			label = targetObject.ClassName()
		}
	}
	message := fmt.Sprintf(
		"Possible EventEmitter memory leak detected. %d %s listeners added to [%s]. MaxListeners is %s. Use emitter.setMaxListeners() to increase limit",
		count,
		eventName,
		label,
		maximum.String(),
	)
	warning := a.newCapturedError(message)
	a.defineErrorProperty(warning, "name", a.runtime.ToValue("MaxListenersExceededWarning"))
	a.defineErrorProperty(warning, "emitter", target)
	a.defineErrorProperty(warning, "type", event)
	a.defineErrorProperty(warning, "count", a.runtime.ToValue(count))
	return warning
}

func (a *Adapter) emitWarningObject(warning *goja.Object) bool {
	if warning == nil {
		return false
	}
	outcome := a.emitProcessOutcome("warning", warning)
	if outcome.exceptionHandled {
		a.yieldMicrotasks()
	}
	if outcome.emitted {
		return outcome.exceptionHandled
	}
	var name, message, codeText, detailText string
	if exception := a.runtime.Try(func() {
		name = warning.Get("name").String()
		message = warning.Get("message").String()
		if code := warning.Get("code"); code != nil && !goja.IsUndefined(code) && !goja.IsNull(code) {
			codeText = code.String()
		}
		if detail := warning.Get("detail"); detail != nil && !goja.IsUndefined(detail) && !goja.IsNull(detail) {
			detailText = detail.String()
		}
	}); exception != nil {
		handled := a.handleHostCallbackError("process.emitWarning", exception, "uncaughtException")
		if handled {
			a.yieldMicrotasks()
		}
		return handled
	}
	if name == "" {
		name = "Warning"
	}
	if output := a.consoleWriter(); output != nil {
		prefix := name
		if codeText != "" {
			prefix = "[" + codeText + "] " + name
		}
		_, _ = fmt.Fprintf(output, "(node) %s: %s\n", prefix, message)
		if detailText != "" {
			_, _ = fmt.Fprintf(output, "%s\n", detailText)
		}
	}
	return outcome.exceptionHandled
}

func (a *Adapter) emitWarningObjectNextTick(warning *goja.Object) {
	if warning == nil || a.exiting.Load() {
		return
	}
	if err := a.js.NextTick(func() {
		if a.exiting.Load() {
			return
		}
		a.emitWarningObject(warning)
	}); err != nil {
		return
	}
}

func (a *Adapter) emitWarningNextTick(message, name, code string) {
	a.emitWarningObjectNextTick(a.warningObject(message, name, code))
}

type uncaughtDisposition uint8

const (
	uncaughtUnhandled uncaughtDisposition = iota
	uncaughtHandled
	uncaughtTerminal
)

func (a *Adapter) dispatchUncaught(value goja.Value, origin string) uncaughtDisposition {
	monitor := a.emitProcessOutcome("uncaughtExceptionMonitor", value, a.runtime.ToValue(origin))
	if monitor.terminal || a.exiting.Load() {
		return uncaughtTerminal
	}
	handler := a.emitProcessOutcome("uncaughtException", value, a.runtime.ToValue(origin))
	if handler.terminal || a.exiting.Load() {
		return uncaughtTerminal
	}
	if handler.emitted {
		return uncaughtHandled
	}
	return uncaughtUnhandled
}

func (a *Adapter) trackPromiseRejection(p *goja.Promise, operation goja.PromiseRejectionOperation) {
	if a == nil || p == nil || a.exiting.Load() {
		return
	}
	switch operation {
	case goja.PromiseRejectionReject:
		a.processMu.Lock()
		if _, exists := a.pendingRejections[p]; !exists {
			a.pendingRejectionOrder = append(a.pendingRejectionOrder, p)
			a.nextRejectionID++
			a.rejectionIDMark(p, a.nextRejectionID)
		}
		a.pendingRejections[p] = p.Result()
		a.processMu.Unlock()
		a.scheduleRejectionCheck()
	case goja.PromiseRejectionHandle:
		a.processMu.Lock()
		_, pending := a.pendingRejections[p]
		if pending {
			delete(a.pendingRejections, p)
			a.pendingRejectionOrder = removePendingRejection(a.pendingRejectionOrder, p)
		}
		a.processMu.Unlock()
		if pending {
			return
		}
		id, reported := a.reportedRejectionDelete(p)
		if !reported {
			return
		}
		warning := a.warningObjectDetailed(
			fmt.Sprintf("Promise rejection was handled asynchronously (rejection id: %d)", id),
			"PromiseRejectionHandledWarning",
			"",
			false,
			"",
			false,
		)
		a.rejectionWarningMark(p, warning)
		a.processMu.Lock()
		a.pendingRejectionOrder = append(a.pendingRejectionOrder, p)
		a.processMu.Unlock()
		a.scheduleRejectionCheck()
	}
}

func (a *Adapter) reportedRejectionMark(p *goja.Promise) {
	if a == nil || p == nil || a.reportedRejectionSet == nil || a.weakSetAdd == nil {
		return
	}
	_, _ = a.weakSetAdd(a.reportedRejectionSet, a.runtime.ToValue(p))
}

func (a *Adapter) reportedRejectionDelete(p *goja.Promise) (uint64, bool) {
	if a == nil || p == nil || a.reportedRejectionSet == nil || a.weakSetHas == nil || a.weakSetDelete == nil {
		return 0, false
	}
	promiseValue := a.runtime.ToValue(p)
	has, err := a.weakSetHas(a.reportedRejectionSet, promiseValue)
	if err != nil || !has.ToBoolean() {
		return 0, false
	}
	_, _ = a.weakSetDelete(a.reportedRejectionSet, promiseValue)
	id, ok := a.rejectionID(p)
	return id, ok
}

func (a *Adapter) rejectionIDMark(p *goja.Promise, id uint64) {
	if a == nil || p == nil || a.rejectionIDStore == nil || a.weakMapSet == nil {
		return
	}
	_, _ = a.weakMapSet(a.rejectionIDStore, a.runtime.ToValue(p), a.runtime.ToValue(strconv.FormatUint(id, 10)))
}

func (a *Adapter) rejectionID(p *goja.Promise) (uint64, bool) {
	if a == nil || p == nil || a.rejectionIDStore == nil || a.weakMapGet == nil {
		return 0, false
	}
	value, err := a.weakMapGet(a.rejectionIDStore, a.runtime.ToValue(p))
	if err != nil || value == nil || goja.IsUndefined(value) {
		return 0, false
	}
	id, err := strconv.ParseUint(value.String(), 10, 64)
	return id, err == nil
}

func (a *Adapter) rejectionWarningMark(p *goja.Promise, warning *goja.Object) {
	if a == nil || p == nil || warning == nil || a.rejectionWarningStore == nil || a.weakMapSet == nil {
		return
	}
	_, _ = a.weakMapSet(a.rejectionWarningStore, a.runtime.ToValue(p), warning)
}

func (a *Adapter) rejectionWarning(p *goja.Promise) *goja.Object {
	if a == nil || p == nil || a.rejectionWarningStore == nil || a.weakMapGet == nil {
		return nil
	}
	value, err := a.weakMapGet(a.rejectionWarningStore, a.runtime.ToValue(p))
	if err != nil {
		return nil
	}
	warning, _ := value.(*goja.Object)
	return warning
}

func (a *Adapter) scheduleRejectionCheck() {
	a.processMu.Lock()
	if a.rejectionCheckScheduled || a.exiting.Load() {
		a.processMu.Unlock()
		return
	}
	a.rejectionCheckScheduled = true
	a.processMu.Unlock()
	if err := a.loop.ScheduleMicrotaskCheckpoint(a.flushUnhandledRejections); err != nil {
		a.processMu.Lock()
		a.rejectionCheckScheduled = false
		a.processMu.Unlock()
		a.flushUnhandledRejections()
	}
}

type pendingPromiseRejection struct {
	promise *goja.Promise
	reason  goja.Value
}

func (a *Adapter) takeHandledRejections() []*goja.Promise {
	a.processMu.Lock()
	defer a.processMu.Unlock()
	handled := make([]*goja.Promise, 0)
	pending := a.pendingRejectionOrder[:0]
	for _, promise := range a.pendingRejectionOrder {
		if _, ok := a.pendingRejections[promise]; ok {
			pending = append(pending, promise)
			continue
		}
		if a.rejectionWarning(promise) != nil {
			handled = append(handled, promise)
		}
	}
	clear(a.pendingRejectionOrder[len(pending):])
	a.pendingRejectionOrder = pending
	return handled
}

func (a *Adapter) takePendingRejections() []pendingPromiseRejection {
	a.processMu.Lock()
	defer a.processMu.Unlock()
	rejections := make([]pendingPromiseRejection, 0, len(a.pendingRejections))
	for _, promise := range a.pendingRejectionOrder {
		reason, ok := a.pendingRejections[promise]
		if !ok {
			continue
		}
		rejections = append(rejections, pendingPromiseRejection{promise: promise, reason: reason})
		delete(a.pendingRejections, promise)
	}
	a.pendingRejectionOrder = clearPendingRejectionOrder(a.pendingRejectionOrder)
	a.rejectionCheckScheduled = false
	return rejections
}

func (a *Adapter) deferRejectionCheck(handled []*goja.Promise) {
	a.processMu.Lock()
	if len(handled) != 0 {
		order := make([]*goja.Promise, 0, len(handled)+len(a.pendingRejectionOrder))
		order = append(order, handled...)
		order = append(order, a.pendingRejectionOrder...)
		a.pendingRejectionOrder = order
	}
	pending := len(a.pendingRejectionOrder) != 0 || len(a.pendingRejections) != 0
	a.rejectionCheckScheduled = false
	a.processMu.Unlock()
	a.yieldMicrotasks()
	if pending {
		a.scheduleRejectionCheck()
	}
}

func (a *Adapter) flushUnhandledRejections() {
	for {
		handled := a.takeHandledRejections()
		if len(handled) == 0 {
			break
		}
		for index, promise := range handled {
			if a.exiting.Load() {
				return
			}
			outcome := a.emitProcessOutcome("rejectionHandled", a.runtime.ToValue(promise))
			if outcome.exceptionHandled {
				a.deferRejectionCheck(handled[index+1:])
				return
			}
			if outcome.terminal || a.exiting.Load() {
				return
			}
			if !outcome.emitted {
				a.emitWarningObjectNextTick(a.rejectionWarning(promise))
			}
		}
	}

	rejections := a.takePendingRejections()

	for _, entry := range rejections {
		if a.exiting.Load() {
			break
		}
		a.reportedRejectionMark(entry.promise)
		promiseValue := a.runtime.ToValue(entry.promise)
		outcome := a.emitProcessOutcome("unhandledRejection", entry.reason, promiseValue)
		if outcome.exceptionHandled {
			a.yieldMicrotasks()
			return
		}
		if outcome.terminal || a.exiting.Load() {
			return
		}
		if outcome.emitted {
			continue
		}
		var unhandled goja.Value
		if exception := a.runtime.Try(func() {
			unhandled = a.defaultUnhandledRejection(entry.reason)
		}); exception != nil {
			if a.handleHostCallbackError("process.unhandledRejection", exception, "unhandledRejection") {
				a.yieldMicrotasks()
			}
			return
		}
		switch a.dispatchUncaught(unhandled, "unhandledRejection") {
		case uncaughtHandled:
			continue
		case uncaughtTerminal:
			return
		default:
			a.requestFatalExit()
			return
		}
	}
}

func removePendingRejection(order []*goja.Promise, promise *goja.Promise) []*goja.Promise {
	for index, candidate := range order {
		if candidate != promise {
			continue
		}
		copy(order[index:], order[index+1:])
		order[len(order)-1] = nil
		return order[:len(order)-1]
	}
	return order
}

func clearPendingRejectionOrder(order []*goja.Promise) []*goja.Promise {
	clear(order)
	return order[:0]
}

func (a *Adapter) defaultUnhandledRejection(reason goja.Value) goja.Value {
	if a.isErrorValue(reason) {
		return reason
	}
	message := "This error originated either by throwing inside of an async function without a catch block, " +
		"or by rejecting a promise which was not handled with .catch(). The promise rejected with the reason \"" +
		a.formatUnhandledRejectionReason(reason) + "\"."
	errorObject := a.newCapturedError(message)
	a.defineErrorProperty(errorObject, "name", a.runtime.ToValue("UnhandledPromiseRejection"))
	a.defineErrorProperty(errorObject, "code", a.runtime.ToValue("ERR_UNHANDLED_REJECTION"))
	return errorObject
}

func (a *Adapter) isErrorValue(value goja.Value) bool {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return false
	}
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		return false
	}
	switch obj.ClassName() {
	case "Error", "EvalError", "RangeError", "ReferenceError", "SyntaxError", "TypeError", "URIError", "AggregateError":
		return true
	default:
		return false
	}
}

func (a *Adapter) formatUnhandledRejectionReason(reason goja.Value) string {
	if reason == nil || goja.IsUndefined(reason) {
		return "undefined"
	}
	if symbol, ok := reason.(*goja.Symbol); ok {
		return "Symbol(" + symbol.String() + ")"
	}
	if obj, ok := reason.(*goja.Object); ok && obj != nil {
		if _, isProxy := a.proxyTarget(obj); isProxy {
			return "#<Object>"
		}
		switch className := obj.ClassName(); className {
		case "Object":
			if a.isNullPrototypeObject(reason) {
				return "[object Object]"
			}
			if name := a.objectConstructorName(obj); name != "" && name != "Object" {
				if isTypedArrayClass(name) || name == "DataView" {
					return "[object " + name + "]"
				}
				return "#<" + name + ">"
			}
			return "#<Object>"
		case "Function":
			if a.functionToString != nil {
				value, err := a.functionToString(reason)
				if err == nil {
					if text, ok := primitiveString(value); ok {
						return text
					}
				}
			}
			return "function () { [native code] }"
		case "Map", "Set", "WeakMap", "WeakSet", "ArrayBuffer", "SharedArrayBuffer":
			return "#<" + className + ">"
		case "Array", "RegExp", "Date", "Uint8Array", "Uint8ClampedArray", "Int8Array", "Uint16Array", "Int16Array", "Uint32Array", "Int32Array", "Float32Array", "Float64Array", "BigInt64Array", "BigUint64Array", "DataView":
			return "[object " + className + "]"
		default:
			if className != "" {
				return "#<" + className + ">"
			}
		}
	}
	return reason.String()
}

func isTypedArrayClass(name string) bool {
	switch name {
	case "Uint8Array", "Uint8ClampedArray", "Int8Array", "Uint16Array", "Int16Array", "Uint32Array", "Int32Array", "Float32Array", "Float64Array", "BigInt64Array", "BigUint64Array":
		return true
	default:
		return false
	}
}

func (a *Adapter) objectConstructorName(obj *goja.Object) string {
	if a == nil || obj == nil {
		return ""
	}
	if _, isProxy := a.proxyTarget(obj); isProxy {
		return ""
	}
	prototype := obj.Prototype()
	if prototype == nil {
		return ""
	}
	if _, isProxy := a.proxyTarget(prototype); isProxy {
		return ""
	}
	constructor := a.safeDescriptorProperty(a.safeOwnPropertyDescriptor(prototype, "constructor"), "value")
	constructorObj, ok := constructor.(*goja.Object)
	if !ok || constructorObj == nil {
		return ""
	}
	name := a.safeDescriptorProperty(a.safeOwnPropertyDescriptor(constructorObj, "name"), "value")
	if name == nil || goja.IsUndefined(name) || goja.IsNull(name) {
		return ""
	}
	text, _ := primitiveString(name)
	return text
}

func (a *Adapter) isNullPrototypeObject(value goja.Value) bool {
	if a.objectGetPrototypeOf == nil {
		return false
	}
	prototype, err := a.objectGetPrototypeOf(goja.Undefined(), value)
	if err != nil {
		return false
	}
	return goja.IsNull(prototype)
}
