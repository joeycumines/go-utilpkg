package gojaeventloop

import (
	"errors"
	"fmt"
	"slices"

	"github.com/joeycumines/goja"
)

type retainedGlobalKind uint8

const (
	retainedGlobalData retainedGlobalKind = iota
	retainedGlobalAccessor
	retainedGlobalReadonlyAccessor
)

type functionShape struct {
	name   string
	length int
}

type retainedGlobalSpec struct {
	function     *functionShape
	getter       *functionShape
	setter       *functionShape
	name         string
	kind         retainedGlobalKind
	writable     bool
	enumerable   bool
	configurable bool
}

func retainedFunction(name string, length int, enumerable bool) retainedGlobalSpec {
	return retainedGlobalSpec{
		function:     &functionShape{name: name, length: length},
		name:         name,
		kind:         retainedGlobalData,
		writable:     true,
		enumerable:   enumerable,
		configurable: true,
	}
}

// retainedGlobalSurface is the single installation authority for every global
// GEL owns. Data/accessor kind, flags, and callable metadata match Node v26.5.0
// where Node is the declared authority. delay is an explicit package extension
// and intentionally retains the ordinary enumerable global profile.
var retainedGlobalSurface = []retainedGlobalSpec{
	retainedFunction("setTimeout", 2, true),
	retainedFunction("clearTimeout", 1, true),
	retainedFunction("setInterval", 2, true),
	retainedFunction("clearInterval", 1, true),
	retainedFunction("queueMicrotask", 1, true),
	retainedFunction("setImmediate", 1, true),
	retainedFunction("clearImmediate", 1, true),
	retainedFunction("AbortController", 0, false),
	retainedFunction("AbortSignal", 0, false),
	{name: "performance", kind: retainedGlobalAccessor, getter: &functionShape{name: "get performance", length: 0}, setter: &functionShape{name: "set performance", length: 1}, enumerable: true, configurable: true},
	retainedFunction("Performance", 0, false),
	{name: "console", kind: retainedGlobalData, writable: true, enumerable: true, configurable: true},
	{name: "process", kind: retainedGlobalAccessor, getter: &functionShape{name: "get", length: 0}, setter: &functionShape{name: "set", length: 1}, configurable: true},
	retainedFunction("delay", 1, true),
	{name: "crypto", kind: retainedGlobalReadonlyAccessor, getter: &functionShape{name: "get crypto", length: 0}, enumerable: true, configurable: true},
	retainedFunction("Crypto", 0, false),
	retainedFunction("atob", 1, true),
	retainedFunction("btoa", 1, true),
	retainedFunction("EventTarget", 0, false),
	retainedFunction("Event", 1, false),
	retainedFunction("CustomEvent", 1, false),
	retainedFunction("structuredClone", 2, true),
	retainedFunction("DOMException", 0, false),
}

var consolePropertyNames = []string{
	"time",
	"timeEnd",
	"timeLog",
	"count",
	"countReset",
	"assert",
	"table",
	"group",
	"groupCollapsed",
	"groupEnd",
	"trace",
	"clear",
	"dir",
}

var processPropertyNames = []string{
	"_exiting",
	"exitCode",
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
	"emitWarning",
	"exit",
	"nextTick",
}

var promisePropertyNames = []string{"all", "race", "allSettled", "any", "withResolvers", "try"}

type propertySnapshot struct {
	object     *goja.Object
	descriptor goja.Value
	name       string
}

type prototypeSnapshot struct {
	object    *goja.Object
	prototype *goja.Object
}

type preparedPrototypeDefinition struct {
	object    *goja.Object
	prototype *goja.Object
	expected  *goja.Object
}

type installationJournal struct {
	adapter             *Adapter
	global              *goja.Object
	console             *goja.Object
	promise             *goja.Object
	symbol              *goja.Object
	properties          []propertySnapshot
	attempted           []propertySnapshot
	prototypes          []preparedPrototypeDefinition
	attemptedPrototypes []prototypeSnapshot
	propertyDefinitions []preparedPropertyDefinition
	globalDefinitions   []preparedGlobalDefinition
	disposeNeedsCommit  bool
}

func (j *installationJournal) preparePrototype(object, prototype *goja.Object) error {
	if object == nil || prototype == nil {
		return errors.New("goja-eventloop: prepared prototype target and value are required")
	}
	current := object.Prototype()
	if current == prototype {
		return nil
	}
	for _, definition := range j.prototypes {
		if definition.object == object {
			return errors.New("goja-eventloop: object prototype was prepared more than once")
		}
	}
	j.prototypes = append(j.prototypes, preparedPrototypeDefinition{
		object:    object,
		prototype: prototype,
		expected:  current,
	})
	return nil
}

func (a *Adapter) initInstallationHelpers() error {
	if a.objectGetOwnPropertyDesc == nil {
		return errors.New("goja-eventloop: Object.getOwnPropertyDescriptor is not callable")
	}
	defineProperty, err := runtimeIntrinsic(a.runtime, goja.IntrinsicObjectDefineProperty, "Object.defineProperty")
	if err != nil {
		return err
	}
	deleteProperty, err := runtimeIntrinsic(a.runtime, goja.IntrinsicReflectDeleteProperty, "Reflect.deleteProperty")
	if err != nil {
		return err
	}
	create, err := runtimeIntrinsic(a.runtime, goja.IntrinsicObjectCreate, "Object.create")
	if err != nil {
		return err
	}
	descriptors, err := runtimeIntrinsic(a.runtime, goja.IntrinsicObjectGetOwnPropertyDescriptors, "Object.getOwnPropertyDescriptors")
	if err != nil {
		return err
	}
	prototype, err := runtimeIntrinsic(a.runtime, goja.IntrinsicObjectGetPrototypeOf, "Object.getPrototypeOf")
	if err != nil {
		return err
	}
	construct, err := runtimeIntrinsic(a.runtime, goja.IntrinsicReflectConstruct, "Reflect.construct")
	if err != nil {
		return err
	}
	apply, err := runtimeIntrinsic(a.runtime, goja.IntrinsicReflectApply, "Reflect.apply")
	if err != nil {
		return err
	}
	typeError, err := runtimeIntrinsicObject(a.runtime, goja.IntrinsicTypeErrorConstructor, "TypeError")
	if err != nil {
		return err
	}
	functionToString, err := runtimeIntrinsicCallable(a.runtime, goja.IntrinsicFunctionToString, "Function.prototype.toString")
	if err != nil {
		return err
	}
	iteratorFactoryValue, err := a.runtime.RunString(`(iteratorSymbol => object => object[iteratorSymbol])`)
	if err != nil {
		return wrapRuntimeError("initialize iterator helper", err)
	}
	iteratorFactory, ok := goja.AssertFunction(iteratorFactoryValue)
	if !ok {
		return errors.New("goja-eventloop: iterator helper factory is not callable")
	}
	iteratorValue, err := iteratorFactory(goja.Undefined(), goja.SymIterator)
	if err != nil {
		return wrapRuntimeError("create iterator helper", err)
	}
	iterator, ok := goja.AssertFunction(iteratorValue)
	if !ok {
		return errors.New("goja-eventloop: iterator helper is not callable")
	}
	restoreFactoryValue, err := a.runtime.RunString(`
		((defineProperty, deleteProperty, TypeErrorConstructor) => {
			return (object, name, descriptor) => {
				if (descriptor === undefined) {
					if (!deleteProperty(object, name)) {
						throw new TypeErrorConstructor("property rollback failed: " + name);
					}
					return;
				}
				defineProperty(object, name, descriptor);
			};
		})
	`)
	if err != nil {
		return wrapRuntimeError("initialize property restore helper factory", err)
	}
	restoreFactory, ok := goja.AssertFunction(restoreFactoryValue)
	if !ok {
		return errors.New("goja-eventloop: property restore helper factory is not callable")
	}
	restoreValue, err := restoreFactory(goja.Undefined(), defineProperty, deleteProperty, typeError)
	if err != nil {
		return wrapRuntimeError("initialize property restore helper", err)
	}
	restore, ok := goja.AssertFunction(restoreValue)
	if !ok {
		return errors.New("goja-eventloop: property restore helper is not callable")
	}
	processCloneFactoryValue, err := a.runtime.RunString(`
		((create, descriptors, prototype, deleteProperty) => {
			return (source, ownedNames) => {
				const properties = descriptors(source);
				for (let index = 0; index < ownedNames.length; index++) {
					deleteProperty(properties, ownedNames[index]);
				}
				return create(prototype(source), properties);
			};
		})
	`)
	if err != nil {
		return wrapRuntimeError("initialize process clone helper factory", err)
	}
	processCloneFactory, ok := goja.AssertFunction(processCloneFactoryValue)
	if !ok {
		return errors.New("goja-eventloop: process clone helper factory is not callable")
	}
	processCloneValue, err := processCloneFactory(
		goja.Undefined(),
		create,
		descriptors,
		prototype,
		deleteProperty,
	)
	if err != nil {
		return wrapRuntimeError("initialize process clone helper", err)
	}
	processClone, ok := goja.AssertFunction(processCloneValue)
	if !ok {
		return errors.New("goja-eventloop: process clone helper is not callable")
	}
	constructorFactoryFactoryValue, err := a.runtime.RunString(`
		(construct => {
			return target => {
				return class {
					constructor(...args) {
						return construct(target, args, new.target);
					}
				};
			};
		})
	`)
	if err != nil {
		return wrapRuntimeError("initialize Web constructor factory factory", err)
	}
	constructorFactoryFactory, ok := goja.AssertFunction(constructorFactoryFactoryValue)
	if !ok {
		return errors.New("goja-eventloop: Web constructor factory factory is not callable")
	}
	constructorFactoryValue, err := constructorFactoryFactory(goja.Undefined(), construct)
	if err != nil {
		return wrapRuntimeError("initialize Web constructor factory", err)
	}
	constructorFactory, ok := goja.AssertFunction(constructorFactoryValue)
	if !ok {
		return errors.New("goja-eventloop: Web constructor factory is not callable")
	}
	a.getIterator = iterator
	a.functionToString = functionToString
	a.propertyRestore = restore
	a.processClone = processClone
	a.webConstructorFactory = constructorFactory
	ordinaryFactoryFactoryValue, err := a.runtime.RunString(`
		(apply => {
			return callback => function () {
				"use strict";
				return apply(callback, this, arguments);
			};
		})
	`)
	if err != nil {
		return wrapRuntimeError("initialize ordinary function factory factory", err)
	}
	ordinaryFactoryFactory, ok := goja.AssertFunction(ordinaryFactoryFactoryValue)
	if !ok {
		return errors.New("goja-eventloop: ordinary function factory factory is not callable")
	}
	ordinaryFactoryValue, err := ordinaryFactoryFactory(goja.Undefined(), apply)
	if err != nil {
		return wrapRuntimeError("initialize ordinary function factory", err)
	}
	ordinaryFactory, ok := goja.AssertFunction(ordinaryFactoryValue)
	if !ok {
		return errors.New("goja-eventloop: ordinary function factory is not callable")
	}
	a.ordinaryFunctionFactory = ordinaryFactory
	return nil
}

func (a *Adapter) ordinaryFunction(callback any) (*goja.Object, error) {
	if a == nil || a.runtime == nil || a.ordinaryFunctionFactory == nil {
		return nil, errors.New("goja-eventloop: ordinary function factory is unavailable")
	}
	value, err := a.ordinaryFunctionFactory(goja.Undefined(), a.runtime.ToValue(callback))
	if err != nil {
		return nil, wrapRuntimeError("create ordinary function", err)
	}
	function, ok := value.(*goja.Object)
	if !ok || function == nil {
		return nil, errors.New("goja-eventloop: ordinary function factory returned a non-object")
	}
	if _, ok := goja.AssertFunction(function); !ok {
		return nil, errors.New("goja-eventloop: ordinary function factory returned a non-callable object")
	}
	if _, ok := goja.AssertConstructor(function); !ok {
		return nil, errors.New("goja-eventloop: ordinary function factory returned a non-constructable function")
	}
	return function, nil
}

func (a *Adapter) nonCallableWebConstructor(target any, name string) (*goja.Object, error) {
	if a == nil || a.runtime == nil || a.webConstructorFactory == nil {
		return nil, errors.New("goja-eventloop: Web constructor factory is unavailable")
	}
	value, err := a.webConstructorFactory(goja.Undefined(), a.runtime.ToValue(target))
	if err != nil {
		return nil, wrapRuntimeError(fmt.Sprintf("wrap %s constructor", name), err)
	}
	constructor, ok := value.(*goja.Object)
	if !ok || constructor == nil {
		return nil, fmt.Errorf("goja-eventloop: wrap %s constructor: result is not an object", name)
	}
	if _, ok := goja.AssertConstructor(constructor); !ok {
		return nil, fmt.Errorf("goja-eventloop: wrap %s constructor: result is not constructable", name)
	}
	return constructor, nil
}

func newInstallationJournal(adapter *Adapter) (*installationJournal, error) {
	global := adapter.runtime.GlobalObject()
	journal := &installationJournal{adapter: adapter, global: global}
	for _, spec := range retainedGlobalSurface {
		if err := journal.capture(global, spec.name); err != nil {
			return nil, err
		}
	}
	if err := journal.captureConsole(); err != nil {
		return nil, err
	}
	var err error
	journal.promise, err = journal.captureGlobalObject("Promise", promisePropertyNames)
	if err != nil {
		return nil, err
	}
	canonicalPromise, err := runtimeIntrinsicObject(adapter.runtime, goja.IntrinsicPromiseConstructor, "Promise")
	if err != nil {
		return nil, err
	}
	if journal.promise != canonicalPromise {
		return nil, errors.New("goja-eventloop: global Promise is not the runtime intrinsic")
	}
	journal.symbol, err = journal.captureGlobalObject("Symbol", []string{"dispose"})
	if err != nil {
		return nil, err
	}
	canonicalSymbol, err := runtimeIntrinsicObject(adapter.runtime, goja.IntrinsicSymbolConstructor, "Symbol")
	if err != nil {
		return nil, err
	}
	if journal.symbol != canonicalSymbol {
		return nil, errors.New("goja-eventloop: global Symbol is not the runtime intrinsic")
	}
	return journal, nil
}

func (j *installationJournal) captureConsole() error {
	value := j.adapter.runtime.Get("console")
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil
	}
	console, ok := value.(*goja.Object)
	if !ok || console == nil {
		return errors.New("goja-eventloop: console is not an object")
	}
	j.console = console
	for _, name := range consolePropertyNames {
		if err := j.capture(console, name); err != nil {
			return err
		}
	}
	return nil
}

// detachedProcess creates the adapter-owned process object without mutating a
// host-supplied process. Unrelated own descriptors (including symbol keys) and
// the original prototype are retained; adapter-owned names are deliberately
// omitted so their exact Node-profile descriptors can be installed safely.
func (j *installationJournal) detachedProcess() (*goja.Object, error) {
	value := j.adapter.runtime.Get("process")
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return j.adapter.runtime.NewObject(), nil
	}
	source, ok := value.(*goja.Object)
	if !ok || source == nil {
		return nil, errors.New("goja-eventloop: process is not an object")
	}
	if j.adapter.processClone == nil {
		return nil, errors.New("goja-eventloop: process clone helper is unavailable")
	}
	ownedNames := make([]any, len(processPropertyNames))
	for index, name := range processPropertyNames {
		ownedNames[index] = name
	}
	cloneValue, err := j.adapter.processClone(
		goja.Undefined(),
		source,
		j.adapter.runtime.NewArray(ownedNames...),
	)
	if err != nil {
		return nil, wrapRuntimeError("clone process", err)
	}
	clone, ok := cloneValue.(*goja.Object)
	if !ok || clone == nil {
		return nil, errors.New("goja-eventloop: clone process: result is not an object")
	}
	return clone, nil
}

func (j *installationJournal) captureGlobalObject(globalName string, names []string) (*goja.Object, error) {
	value := j.adapter.runtime.Get(globalName)
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, fmt.Errorf("goja-eventloop: %s is unavailable", globalName)
	}
	object, ok := value.(*goja.Object)
	if !ok || object == nil {
		return nil, fmt.Errorf("goja-eventloop: %s is not an object", globalName)
	}
	for _, name := range names {
		if err := j.capture(object, name); err != nil {
			return nil, err
		}
	}
	return object, nil
}

func (j *installationJournal) capture(object *goja.Object, name string) error {
	if object == nil {
		return errors.New("goja-eventloop: cannot journal a nil object")
	}
	descriptor, err := j.adapter.objectGetOwnPropertyDesc(
		goja.Undefined(),
		object,
		j.adapter.runtime.ToValue(name),
	)
	if err != nil {
		return wrapRuntimeError(fmt.Sprintf("capture property %q", name), err)
	}
	if descriptor == nil {
		descriptor = goja.Undefined()
	}
	j.properties = append(j.properties, propertySnapshot{object: object, descriptor: descriptor, name: name})
	return nil
}

func (j *installationJournal) property(object *goja.Object, name string) (propertySnapshot, bool) {
	for _, property := range j.properties {
		if property.object == object && property.name == name {
			return property, true
		}
	}
	return propertySnapshot{}, false
}

func (j *installationJournal) verifyPreparedReplacedCallables(target, prepared *goja.Object, names []string) error {
	if err := verifyCallableProperties(prepared, names); err != nil {
		return err
	}
	for _, name := range names {
		property, ok := j.property(target, name)
		if !ok || goja.IsUndefined(property.descriptor) {
			continue
		}
		descriptor, ok := property.descriptor.(*goja.Object)
		if !ok || descriptor == nil {
			continue
		}
		oldValue := descriptor.Get("value")
		current := prepared.Get(name)
		if oldValue != nil && current != nil && current.SameAs(oldValue) {
			return fmt.Errorf("property %q retained the foreign callable", name)
		}
	}
	return nil
}

func (j *installationJournal) verifyOwnProperties(object *goja.Object, names []string) error {
	for _, name := range names {
		descriptor, err := j.adapter.objectGetOwnPropertyDesc(
			goja.Undefined(), object, j.adapter.runtime.ToValue(name),
		)
		if err != nil {
			return wrapRuntimeError(fmt.Sprintf("inspect property %q", name), err)
		}
		if descriptor == nil || goja.IsUndefined(descriptor) {
			return fmt.Errorf("property %q was not installed", name)
		}
	}
	return nil
}

func (j *installationJournal) setGlobal(name string, value any) error {
	spec, ok := retainedGlobal(name)
	if !ok {
		return fmt.Errorf("goja-eventloop: global %q is outside the retained surface", name)
	}
	global := j.global
	if global == nil {
		return errors.New("goja-eventloop: captured global object is unavailable")
	}
	propertyValue := j.adapter.runtime.ToValue(value)
	if spec.function != nil {
		object, ok := propertyValue.(*goja.Object)
		if !ok || object == nil {
			return fmt.Errorf("goja-eventloop: install global %q: value is not a function object", name)
		}
		if _, ok := goja.AssertFunction(propertyValue); !ok {
			return fmt.Errorf("goja-eventloop: install global %q: value is not callable", name)
		}
		if err := defineFunctionShape(j.adapter.runtime, object, *spec.function); err != nil {
			return fmt.Errorf("goja-eventloop: install global %q: %w", name, err)
		}
	}
	if err := j.requireDefinable(global, name); err != nil {
		return fmt.Errorf("goja-eventloop: prepare global %q: %w", name, err)
	}
	for _, existing := range j.globalDefinitions {
		if existing.spec.name == name {
			return fmt.Errorf("goja-eventloop: global %q was prepared more than once", name)
		}
	}
	definition := preparedGlobalDefinition{
		spec: spec,
		property: preparedPropertyDefinition{
			object:       global,
			name:         name,
			configurable: spec.configurable,
			enumerable:   spec.enumerable,
		},
	}
	switch spec.kind {
	case retainedGlobalData:
		definition.property.value = propertyValue
		definition.property.writable = spec.writable
	case retainedGlobalAccessor, retainedGlobalReadonlyAccessor:
		getter, setter, accessorErr := j.globalAccessors(propertyValue, spec)
		if accessorErr != nil {
			return accessorErr
		}
		definition.property.accessor = true
		definition.property.getter = getter
		definition.property.setter = setter
	default:
		return fmt.Errorf("goja-eventloop: install global %q: invalid descriptor kind", name)
	}
	j.globalDefinitions = append(j.globalDefinitions, definition)
	return nil
}

func retainedGlobal(name string) (retainedGlobalSpec, bool) {
	index := slices.IndexFunc(retainedGlobalSurface, func(spec retainedGlobalSpec) bool {
		return spec.name == name
	})
	if index < 0 {
		return retainedGlobalSpec{}, false
	}
	return retainedGlobalSurface[index], true
}

func boolFlag(value bool) goja.Flag {
	if value {
		return goja.FLAG_TRUE
	}
	return goja.FLAG_FALSE
}

func (j *installationJournal) globalAccessors(initial goja.Value, spec retainedGlobalSpec) (goja.Value, goja.Value, error) {
	cell := initial
	getterValue := j.adapter.runtime.ToValue(func(goja.FunctionCall) goja.Value { return cell })
	getter, ok := getterValue.(*goja.Object)
	if !ok || getter == nil {
		return nil, nil, fmt.Errorf("goja-eventloop: create global accessor %q: getter is not an object", spec.name)
	}
	if spec.getter == nil {
		return nil, nil, fmt.Errorf("goja-eventloop: create global accessor %q: getter shape is unavailable", spec.name)
	}
	if err := defineFunctionShape(j.adapter.runtime, getter, *spec.getter); err != nil {
		return nil, nil, fmt.Errorf("goja-eventloop: create global accessor %q: %w", spec.name, err)
	}
	if spec.kind == retainedGlobalReadonlyAccessor {
		// A nil setter omits the descriptor field and therefore preserves an
		// existing configurable accessor's setter. Explicit undefined replaces
		// it, which is required for the retained readonly global shape.
		return getter, goja.Undefined(), nil
	}
	if spec.setter == nil {
		return nil, nil, fmt.Errorf("goja-eventloop: create global accessor %q: setter shape is unavailable", spec.name)
	}
	setterValue := j.adapter.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		cell = call.Argument(0)
		return goja.Undefined()
	})
	setter, ok := setterValue.(*goja.Object)
	if !ok || setter == nil {
		return nil, nil, fmt.Errorf("goja-eventloop: create global accessor %q: setter is not an object", spec.name)
	}
	if err := defineFunctionShape(j.adapter.runtime, setter, *spec.setter); err != nil {
		return nil, nil, fmt.Errorf("goja-eventloop: create global accessor %q: %w", spec.name, err)
	}
	return getter, setter, nil
}

func defineFunctionShape(runtime *goja.Runtime, function *goja.Object, shape functionShape) error {
	if runtime == nil || function == nil {
		return errors.New("define function shape: runtime and function are required")
	}
	for name, value := range map[string]any{"name": shape.name, "length": shape.length} {
		if err := function.DefineDataProperty(
			name,
			runtime.ToValue(value),
			goja.FLAG_FALSE,
			goja.FLAG_TRUE,
			goja.FLAG_FALSE,
		); err != nil {
			return wrapRuntimeError(fmt.Sprintf("define function %s", name), err)
		}
	}
	return nil
}

func verifyFunctionShape(adapter *Adapter, function *goja.Object, shape functionShape) error {
	for name, expected := range map[string]any{"name": shape.name, "length": int64(shape.length)} {
		descriptor, err := adapter.objectGetOwnPropertyDesc(
			goja.Undefined(), function, adapter.runtime.ToValue(name),
		)
		if err != nil {
			return wrapRuntimeError(fmt.Sprintf("inspect function %s", name), err)
		}
		object, ok := descriptor.(*goja.Object)
		if !ok || object == nil || propertyBoolean(object, "writable") || propertyBoolean(object, "enumerable") || !propertyBoolean(object, "configurable") {
			return fmt.Errorf("function %s descriptor differs", name)
		}
		value := object.Get("value")
		switch expected := expected.(type) {
		case string:
			if value == nil || value.String() != expected {
				return fmt.Errorf("function name = %v, want %q", value, expected)
			}
		case int64:
			if value == nil || value.ToInteger() != expected {
				return fmt.Errorf("function length = %v, want %d", value, expected)
			}
		}
	}
	return nil
}

func propertyBoolean(object *goja.Object, name string) bool {
	if object == nil {
		return false
	}
	value := object.Get(name)
	return value != nil && value.ToBoolean()
}

func (j *installationJournal) commitDisposeSymbol() error {
	if !j.disposeNeedsCommit {
		return nil
	}
	if j.symbol == nil || j.adapter.disposeSymbol == nil {
		return errors.New("goja-eventloop: prepared Symbol.dispose is unavailable")
	}
	if err := j.recordAttempt(j.symbol, "dispose"); err != nil {
		return err
	}
	if err := j.symbol.DefineDataProperty(
		"dispose",
		j.adapter.disposeSymbol,
		goja.FLAG_FALSE,
		goja.FLAG_FALSE,
		goja.FLAG_FALSE,
	); err != nil {
		return wrapRuntimeError("commit Symbol.dispose", err)
	}
	j.disposeNeedsCommit = false
	return nil
}

func (j *installationJournal) rollback() error {
	// Invalidate every adapter-visible publication before running fallible
	// host-object restoration. Even a hostile proxy trap that aborts rollback
	// cannot leave the failed adapter pointing at a usable JS integration.
	j.adapter.processObj = nil
	j.adapter.processEmitterCore = nil
	j.adapter.performance = nil
	j.adapter.eventTimeSource = nil
	j.adapter.eventTimeReceiver = nil
	j.adapter.abortSignalPrototype = nil
	j.adapter.eventPrototype = nil
	j.adapter.eventIsTrustedGetter = nil
	j.adapter.domExceptionPrototype = nil
	j.adapter.js = nil

	var rollbackErr error
	for i := len(j.attempted) - 1; i >= 0; i-- {
		property := j.attempted[i]
		var err error
		if exception := j.adapter.runtime.Try(func() {
			_, err = j.adapter.propertyRestore(
				goja.Undefined(),
				property.object,
				j.adapter.runtime.ToValue(property.name),
				property.descriptor,
			)
		}); exception != nil {
			err = wrapRuntimeException("restore runtime property", exception)
		}
		if err != nil {
			err = wrapRuntimeError("restore runtime property", err)
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore %q: %w", property.name, err))
		}
	}
	for i := len(j.attemptedPrototypes) - 1; i >= 0; i-- {
		prototype := j.attemptedPrototypes[i]
		if err := prototype.object.SetPrototype(prototype.prototype); err != nil {
			rollbackErr = errors.Join(rollbackErr, wrapRuntimeError("restore object prototype", err))
		}
	}
	return rollbackErr
}

var processEventEmitterMethods = []struct {
	property string
	shape    functionShape
}{
	{property: "on", shape: functionShape{name: "addListener", length: 2}},
	{property: "addListener", shape: functionShape{name: "addListener", length: 2}},
	{property: "once", shape: functionShape{name: "once", length: 2}},
	{property: "off", shape: functionShape{name: "removeListener", length: 2}},
	{property: "removeListener", shape: functionShape{name: "removeListener", length: 2}},
	{property: "emit", shape: functionShape{name: "emit", length: 1}},
	{property: "listenerCount", shape: functionShape{name: "listenerCount", length: 2}},
}

func verifyPropertyDepth(adapter *Adapter, object *goja.Object, name string, want int) error {
	depth := 0
	for current := object; current != nil; current, depth = current.Prototype(), depth+1 {
		descriptor, err := adapter.objectGetOwnPropertyDesc(
			goja.Undefined(), current, adapter.runtime.ToValue(name),
		)
		if err != nil {
			return wrapRuntimeError(fmt.Sprintf("inspect property %q depth", name), err)
		}
		if descriptor == nil || goja.IsUndefined(descriptor) {
			continue
		}
		if depth != want {
			return fmt.Errorf("goja-eventloop: property %q depth = %d, want %d", name, depth, want)
		}
		return nil
	}
	return fmt.Errorf("goja-eventloop: property %q is unavailable", name)
}

func verifyCallableProperties(object *goja.Object, names []string) error {
	if object == nil {
		return errors.New("goja-eventloop: installed object is nil")
	}
	for _, name := range names {
		if _, ok := goja.AssertFunction(object.Get(name)); !ok {
			return fmt.Errorf("goja-eventloop: installed property %q is not callable", name)
		}
	}
	return nil
}
