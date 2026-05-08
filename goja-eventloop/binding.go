package gojaeventloop

import (
	"errors"
	"fmt"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// Bind atomically installs the retained JavaScript surface. Bind must be called
// once, while the claimed loop is awake and the caller exclusively owns the
// runtime. The global object and the retained Promise, Symbol, and existing
// console installation targets must be ordinary Goja objects; callback-backed
// Proxy or dynamic-object definition traps are outside the valid host contract.
// Calling Bind on a zero or copied Adapter panics. Concurrent, repeated, and
// post-failure calls return [ErrAdapterBinding], [ErrAdapterBound], and
// [ErrAdapterFailed], respectively.
//
// Bind prepares its reversible Goja transaction, then final-preflights target
// identities, current descriptors, prototype relationships, and extensibility
// while the caller exclusively owns the runtime. The loop lifecycle commit
// applies only callback-free, prevalidated own-property definitions and the
// retained Performance prototype relationship before publishing private hooks.
// Defensive installation errors and JavaScript exceptions are returned after
// every attempted mutation has been restored; the adapter then remains failed
// and its ownership claims are released. An arbitrary Go panic or
// runtime.Goexit propagates after the same rollback attempt. A native panic or
// Goexit raised by rollback itself can interrupt the remaining restoration, but
// the ownership claims are still released.
func (a *Adapter) Bind() (err error) {
	a.mustOriginalReceiver("Bind")
	if !a.ownership.state.CompareAndSwap(uint32(adapterStateReady), uint32(adapterStateBinding)) {
		return a.stateError()
	}
	a.ownership.bindMu.Lock()
	defer a.ownership.bindMu.Unlock()
	if !a.claimed() {
		a.fail()
		return ErrAdapterInvalid
	}

	succeeded := false
	defer func() {
		if !succeeded {
			a.fail()
		}
	}()

	var journal *installationJournal
	committed := false
	defer func() {
		if committed {
			return
		}
		if journal != nil {
			err = errors.Join(err, journal.rollback())
		} else {
			a.js = nil
		}
	}()

	if exception := a.runtime.Try(func() {
		journal, err = newInstallationJournal(a)
		if err == nil {
			err = a.bindRetained(journal)
		}
		if err == nil {
			err = journal.preflightCommit()
		}
	}); exception != nil {
		err = wrapRuntimeException("install runtime surface", exception)
	}
	if err != nil {
		return err
	}

	jsOptions := append([]goeventloop.JSOption(nil), a.ownership.jsOptions...)
	_, err = goeventloop.BindJSLifecycle(a.loop, a.handleQuiescence, a.terminateCleanup, func(js *goeventloop.JS) error {
		// No retained object is published until the loop accepts this adapter.
		// Preparation has already exhausted host getters and other callbacks;
		// this mutable commit uses only preflighted own-property definitions and
		// prototype relationships.
		if err := journal.commitMutable(); err != nil {
			return err
		}
		// Finalizing Symbol.dispose is the last operation that can return an
		// error. After it succeeds, only private Go/Goja hook publication remains.
		if err := journal.commitDisposeSymbol(); err != nil {
			return err
		}
		a.js = js
		a.runtime.SetPromiseJobEnqueuer(newPromiseJobEnqueuerWithGate(a.loop, a.runtime, a.reportPromiseJobError, a.exiting.Load))
		a.runtime.SetPromiseRejectionTracker(a.trackPromiseRejection)
		a.ownership.state.Store(uint32(adapterStateBound))
		committed = true
		return nil
	}, jsOptions...)
	if err != nil {
		if errors.Is(err, goeventloop.ErrJSBindState) {
			return fmt.Errorf("%w: %w", ErrLoopState, err)
		}
		return fmt.Errorf("goja-eventloop: bind JS adapter: %w", err)
	}
	clear(a.ownership.jsOptions)
	a.ownership.jsOptions = nil
	succeeded = true
	return nil
}

func (a *Adapter) bindRetained(journal *installationJournal) error {
	globalBindings := []struct {
		name  string
		value any
	}{
		{name: "setTimeout", value: a.setTimeout},
		{name: "clearTimeout", value: a.clearTimeoutFunction},
		{name: "setInterval", value: a.setInterval},
		{name: "clearInterval", value: a.clearIntervalFunction},
		{name: "queueMicrotask", value: a.queueMicrotask},
		{name: "setImmediate", value: a.setImmediate},
		{name: "clearImmediate", value: a.clearImmediateFunction},
	}
	for _, binding := range globalBindings {
		function := binding.value
		if _, ok := binding.value.(goja.Value); !ok {
			var functionErr error
			function, functionErr = a.ordinaryFunction(binding.value)
			if functionErr != nil {
				return fmt.Errorf("goja-eventloop: wrap global %s: %w", binding.name, functionErr)
			}
		}
		if err := journal.setGlobal(binding.name, function); err != nil {
			return err
		}
	}

	abortControllerConstructor, err := a.nonCallableWebConstructor(a.abortControllerConstructor, "AbortController")
	if err != nil {
		return err
	}
	if err := journal.setGlobal("AbortController", abortControllerConstructor); err != nil {
		return err
	}
	abortSignalValue := a.runtime.ToValue(a.abortSignalConstructor)
	abortSignalObject, ok := abortSignalValue.(*goja.Object)
	if !ok || abortSignalObject == nil {
		return fmt.Errorf("failed to bind AbortSignal: constructor is not an object")
	}
	if err := journal.setGlobal("AbortSignal", abortSignalObject); err != nil {
		return err
	}
	if err := a.bindAbortControllerPrototype(abortControllerConstructor); err != nil {
		return fmt.Errorf("failed to bind AbortController prototype: %w", err)
	}
	if err := a.bindAbortSignalPrototype(abortSignalObject); err != nil {
		return fmt.Errorf("failed to bind AbortSignal prototype: %w", err)
	}
	if err := a.bindAbortSignalStatics(abortSignalObject); err != nil {
		return fmt.Errorf("failed to bind AbortSignal statics: %w", err)
	}
	abortControllerPrototype, _ := abortControllerConstructor.Get("prototype").(*goja.Object)
	if err := verifyCallableProperties(abortControllerPrototype, []string{"abort"}); err != nil {
		return fmt.Errorf("failed to verify AbortController prototype: %w", err)
	}
	if err := journal.verifyOwnProperties(abortControllerPrototype, []string{"signal"}); err != nil {
		return fmt.Errorf("failed to verify AbortController prototype: %w", err)
	}
	if err := verifyCallableProperties(abortSignalObject, []string{"abort", "any", "timeout"}); err != nil {
		return fmt.Errorf("failed to verify AbortSignal statics: %w", err)
	}
	abortSignalPrototype, _ := abortSignalObject.Get("prototype").(*goja.Object)
	if err := verifyCallableProperties(abortSignalPrototype, []string{"throwIfAborted"}); err != nil {
		return fmt.Errorf("failed to verify AbortSignal prototype: %w", err)
	}
	if err := journal.verifyOwnProperties(abortSignalPrototype, []string{"aborted", "reason", "onabort"}); err != nil {
		return fmt.Errorf("failed to verify AbortSignal prototype: %w", err)
	}

	console := journal.console
	if console == nil {
		console = a.runtime.NewObject()
		if err := a.bindConsole(console); err != nil {
			return fmt.Errorf("failed to bind console: %w", err)
		}
		if err := verifyCallableProperties(console, consolePropertyNames); err != nil {
			return fmt.Errorf("failed to bind console: %w", err)
		}
	} else {
		preparedConsole := a.runtime.NewObject()
		if err := a.bindConsole(preparedConsole); err != nil {
			return fmt.Errorf("failed to prepare console: %w", err)
		}
		if err := journal.verifyPreparedReplacedCallables(console, preparedConsole, consolePropertyNames); err != nil {
			return fmt.Errorf("failed to prepare console: %w", err)
		}
		if err := journal.prepareDataProperties(console, preparedConsole, consolePropertyNames); err != nil {
			return fmt.Errorf("failed to prepare console: %w", err)
		}
	}
	if err := journal.setGlobal("console", console); err != nil {
		return err
	}

	process, err := journal.detachedProcess()
	if err != nil {
		return err
	}
	if err := a.bindProcess(process); err != nil {
		return fmt.Errorf("failed to bind process: %w", err)
	}
	if err := verifyCallableProperties(process, []string{
		"on", "addListener", "once", "off", "removeListener", "emit",
		"listenerCount", "emitWarning", "exit", "nextTick",
	}); err != nil {
		return fmt.Errorf("failed to bind process: %w", err)
	}
	if err := journal.verifyOwnProperties(process, []string{"_exiting", "exitCode"}); err != nil {
		return fmt.Errorf("failed to bind process: %w", err)
	}
	if err := journal.setGlobal("process", process); err != nil {
		return err
	}

	if err := journal.setGlobal("delay", a.delay); err != nil {
		return err
	}
	for _, binding := range []struct {
		name  string
		value any
	}{
		{name: "atob", value: a.atob},
		{name: "btoa", value: a.btoa},
	} {
		if err := journal.setGlobal(binding.name, binding.value); err != nil {
			return err
		}
	}
	eventTargetConstructor, err := a.nonCallableWebConstructor(a.eventTargetConstructor, "EventTarget")
	if err != nil {
		return err
	}
	if err := journal.setGlobal("EventTarget", eventTargetConstructor); err != nil {
		return err
	}
	eventConstructor, err := a.nonCallableWebConstructor(a.eventConstructor, "Event")
	if err != nil {
		return err
	}
	if err := journal.setGlobal("Event", eventConstructor); err != nil {
		return err
	}
	if err := a.bindEventTargetPrototype(eventTargetConstructor); err != nil {
		return fmt.Errorf("failed to bind EventTarget prototype: %w", err)
	}
	if err := a.bindEventPrototype(eventConstructor); err != nil {
		return fmt.Errorf("failed to bind Event prototype: %w", err)
	}
	eventTargetPrototype, _ := eventTargetConstructor.Get("prototype").(*goja.Object)
	if err := verifyCallableProperties(eventTargetPrototype, []string{"addEventListener", "removeEventListener", "dispatchEvent"}); err != nil {
		return fmt.Errorf("failed to verify EventTarget prototype: %w", err)
	}
	eventPrototype, _ := eventConstructor.Get("prototype").(*goja.Object)
	if err := verifyCallableProperties(eventPrototype, []string{"composedPath", "preventDefault", "stopPropagation", "stopImmediatePropagation", "initEvent"}); err != nil {
		return fmt.Errorf("failed to verify Event prototype: %w", err)
	}
	if err := journal.verifyOwnProperties(eventPrototype, []string{
		"type", "target", "srcElement", "currentTarget", "eventPhase", "timeStamp",
		"bubbles", "cancelable", "defaultPrevented", "composed", "cancelBubble", "returnValue",
		"NONE", "CAPTURING_PHASE", "AT_TARGET", "BUBBLING_PHASE",
	}); err != nil {
		return fmt.Errorf("failed to verify Event prototype: %w", err)
	}
	if err := journal.verifyOwnProperties(eventConstructor, []string{"NONE", "CAPTURING_PHASE", "AT_TARGET", "BUBBLING_PHASE"}); err != nil {
		return fmt.Errorf("failed to verify Event constructor: %w", err)
	}
	if abortSignalPrototype != nil {
		if err := abortSignalPrototype.SetPrototype(eventTargetPrototype); err != nil {
			return fmt.Errorf("failed to bind AbortSignal prototype inheritance: %w", err)
		}
	}
	customEventConstructor, err := a.nonCallableWebConstructor(a.customEventConstructor, "CustomEvent")
	if err != nil {
		return err
	}
	if err := journal.setGlobal("CustomEvent", customEventConstructor); err != nil {
		return err
	}
	customEventPrototype, _ := customEventConstructor.Get("prototype").(*goja.Object)
	if customEventPrototype != nil {
		if err := customEventPrototype.SetPrototype(eventPrototype); err != nil {
			return fmt.Errorf("failed to bind CustomEvent prototype inheritance: %w", err)
		}
	}
	if err := a.bindCustomEventPrototype(customEventConstructor); err != nil {
		return fmt.Errorf("failed to bind CustomEvent prototype: %w", err)
	}
	if err := verifyCallableProperties(customEventPrototype, []string{"initCustomEvent"}); err != nil {
		return fmt.Errorf("failed to verify CustomEvent prototype: %w", err)
	}
	if err := journal.verifyOwnProperties(customEventPrototype, []string{"detail"}); err != nil {
		return fmt.Errorf("failed to verify CustomEvent prototype: %w", err)
	}
	performance, performanceConstructor, preserved, err := coherentHostSingletonPair(a.runtime, "performance", "Performance")
	if err != nil {
		return fmt.Errorf("failed to preserve performance: %w", err)
	}
	if preserved {
		callables, err := verifyBrandedSingletonObject(a, performance, performanceConstructor, "Performance", []string{"now", "toJSON"}, []string{"timeOrigin"})
		if err != nil {
			return fmt.Errorf("failed to verify preserved performance: %w", err)
		}
		for _, name := range []string{"constructor", "now", "toJSON", "timeOrigin"} {
			if err := verifyPropertyDepth(a, performance, name, 1); err != nil {
				return fmt.Errorf("failed to verify preserved Performance prototype: %w", err)
			}
		}
		performancePrototype, _ := performanceConstructor.Get("prototype").(*goja.Object)
		tagDescriptor, err := a.objectGetOwnPropertyDesc(
			goja.Undefined(),
			performancePrototype,
			goja.SymToStringTag,
		)
		if err != nil {
			return fmt.Errorf("failed to inspect preserved Performance prototype tag: %w", err)
		}
		tagObject, ok := tagDescriptor.(*goja.Object)
		if !ok || tagObject == nil {
			return errors.New("failed to verify preserved Performance prototype: own toStringTag is unavailable")
		}
		tag := tagObject.Get("value")
		if tag == nil || tag.String() != "Performance" {
			return errors.New("failed to verify preserved Performance prototype: own toStringTag differs")
		}
		if err := journal.preparePrototype(performancePrototype, eventTargetPrototype); err != nil {
			return fmt.Errorf("failed to integrate preserved performance: %w", err)
		}
		a.initEventTargetObject(performance)
		a.performance = nil
		a.eventTimeSource = callables["now"]
		a.eventTimeReceiver = performance
	} else {
		if _, err := a.bindPerformance(journal, eventTargetPrototype); err != nil {
			return fmt.Errorf("failed to bind performance: %w", err)
		}
	}
	crypto, cryptoConstructor, preserved, err := coherentHostSingletonPair(a.runtime, "crypto", "Crypto")
	if err != nil {
		return fmt.Errorf("failed to preserve crypto: %w", err)
	}
	if preserved {
		if _, err := verifyBrandedSingletonObject(a, crypto, cryptoConstructor, "Crypto", []string{"randomUUID", "getRandomValues"}, nil); err != nil {
			return fmt.Errorf("failed to verify preserved crypto: %w", err)
		}
		a.setHiddenState(a.uncloneableStateStore, crypto, true)
	} else {
		if _, err := a.bindCrypto(journal); err != nil {
			return fmt.Errorf("failed to bind crypto: %w", err)
		}
	}
	if err := journal.setGlobal("structuredClone", a.structuredClone); err != nil {
		return err
	}
	domExceptionConstructor, err := a.nonCallableWebConstructor(a.domExceptionConstructor, "DOMException")
	if err != nil {
		return err
	}
	errorPrototype, err := runtimeIntrinsicObject(a.runtime, goja.IntrinsicErrorPrototype, "Error.prototype")
	if err != nil {
		return err
	}
	if err := a.bindDOMExceptionConstants(domExceptionConstructor, errorPrototype); err != nil {
		return fmt.Errorf("failed to bind DOMException constants: %w", err)
	}
	if err := journal.verifyOwnProperties(domExceptionConstructor, []string{
		"INDEX_SIZE_ERR", "DOMSTRING_SIZE_ERR", "HIERARCHY_REQUEST_ERR",
		"WRONG_DOCUMENT_ERR", "INVALID_CHARACTER_ERR", "NO_DATA_ALLOWED_ERR",
		"NO_MODIFICATION_ALLOWED_ERR", "NOT_FOUND_ERR", "NOT_SUPPORTED_ERR",
		"INUSE_ATTRIBUTE_ERR", "INVALID_STATE_ERR", "SYNTAX_ERR",
		"INVALID_MODIFICATION_ERR", "NAMESPACE_ERR", "INVALID_ACCESS_ERR",
		"VALIDATION_ERR", "TYPE_MISMATCH_ERR", "SECURITY_ERR", "NETWORK_ERR",
		"ABORT_ERR", "URL_MISMATCH_ERR", "QUOTA_EXCEEDED_ERR", "TIMEOUT_ERR",
		"INVALID_NODE_TYPE_ERR", "DATA_CLONE_ERR",
	}); err != nil {
		return fmt.Errorf("failed to verify DOMException constants: %w", err)
	}
	if err := journal.setGlobal("DOMException", domExceptionConstructor); err != nil {
		return err
	}

	preparedPromise := a.runtime.NewObject()
	promiseProperties := []string{"all", "race", "allSettled", "any"}
	for _, name := range []string{"withResolvers", "try"} {
		value := journal.promise.Get(name)
		if _, ok := goja.AssertFunction(value); ok {
			if err := preparedPromise.DefineDataProperty(name, value, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
				return wrapRuntimeError("prepare native Promise."+name, err)
			}
			continue
		}
		promiseProperties = append(promiseProperties, name)
	}
	if err := a.bindNativePromiseExtensions(preparedPromise); err != nil {
		return err
	}
	if err := verifyCallableProperties(preparedPromise, promisePropertyNames); err != nil {
		return fmt.Errorf("failed to prepare native Promise extensions: %w", err)
	}
	if err := journal.prepareDataProperties(journal.promise, preparedPromise, promiseProperties); err != nil {
		return fmt.Errorf("failed to prepare native Promise extensions: %w", err)
	}
	return nil
}
