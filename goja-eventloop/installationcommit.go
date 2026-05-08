package gojaeventloop

import (
	"errors"
	"fmt"

	"github.com/joeycumines/goja"
)

type preparedPropertyDefinition struct {
	object       *goja.Object
	value        goja.Value
	getter       goja.Value
	setter       goja.Value
	name         string
	accessor     bool
	writable     bool
	configurable bool
	enumerable   bool
}

type preparedGlobalDefinition struct {
	property preparedPropertyDefinition
	spec     retainedGlobalSpec
}

func (j *installationJournal) currentDescriptor(object *goja.Object, name string) (goja.Value, error) {
	descriptor, err := j.adapter.objectGetOwnPropertyDesc(
		goja.Undefined(),
		object,
		j.adapter.runtime.ToValue(name),
	)
	if err != nil {
		return nil, wrapRuntimeError(fmt.Sprintf("inspect property %q", name), err)
	}
	if descriptor == nil {
		descriptor = goja.Undefined()
	}
	return descriptor, nil
}

func (j *installationJournal) updateSnapshot(object *goja.Object, name string, descriptor goja.Value) error {
	for index := range j.properties {
		property := &j.properties[index]
		if property.object == object && property.name == name {
			property.descriptor = descriptor
			return nil
		}
	}
	return fmt.Errorf("goja-eventloop: property %q was not journaled", name)
}

func (j *installationJournal) requireExtensible(object *goja.Object, cache map[*goja.Object]bool, name string) error {
	if object == nil {
		return errors.New("goja-eventloop: extensibility preflight is unavailable")
	}
	extensible, ok := cache[object]
	if !ok {
		if exception := j.adapter.runtime.Try(func() {
			extensible = object.IsExtensible()
		}); exception != nil {
			return wrapRuntimeException(fmt.Sprintf("inspect extensibility for property %q", name), exception)
		}
		cache[object] = extensible
	}
	if !extensible {
		return fmt.Errorf("goja-eventloop: property %q is absent on a non-extensible object", name)
	}
	return nil
}

func validatePreparedProperty(definition preparedPropertyDefinition) error {
	if definition.object == nil || definition.name == "" {
		return errors.New("goja-eventloop: prepared property target and name are required")
	}
	if !definition.configurable {
		return fmt.Errorf("goja-eventloop: reversible property %q is not configurable", definition.name)
	}
	if !definition.accessor {
		if definition.value == nil {
			return fmt.Errorf("goja-eventloop: prepared property %q has a nil value", definition.name)
		}
		return nil
	}
	if _, ok := goja.AssertFunction(definition.getter); !ok {
		return fmt.Errorf("goja-eventloop: prepared property %q getter is not callable", definition.name)
	}
	if definition.setter != nil && !goja.IsUndefined(definition.setter) {
		if _, ok := goja.AssertFunction(definition.setter); !ok {
			return fmt.Errorf("goja-eventloop: prepared property %q setter is not callable", definition.name)
		}
	}
	return nil
}

func (j *installationJournal) preflightProperty(definition preparedPropertyDefinition, extensible map[*goja.Object]bool) error {
	if err := validatePreparedProperty(definition); err != nil {
		return err
	}
	descriptor, err := j.currentDescriptor(definition.object, definition.name)
	if err != nil {
		return err
	}
	if err := j.updateSnapshot(definition.object, definition.name, descriptor); err != nil {
		return err
	}
	if goja.IsUndefined(descriptor) {
		return j.requireExtensible(definition.object, extensible, definition.name)
	}
	object, ok := descriptor.(*goja.Object)
	if !ok || object == nil {
		return fmt.Errorf("goja-eventloop: property %q has an invalid descriptor", definition.name)
	}
	if !propertyBoolean(object, "configurable") {
		return fmt.Errorf("goja-eventloop: property %q is not configurable", definition.name)
	}
	return nil
}

func (j *installationJournal) preflightGlobal(definition preparedGlobalDefinition, extensible map[*goja.Object]bool) error {
	if definition.property.object != j.global {
		return fmt.Errorf("goja-eventloop: global %q does not target the captured global object", definition.spec.name)
	}
	if err := j.preflightProperty(definition.property, extensible); err != nil {
		return fmt.Errorf("goja-eventloop: prepare global %q: %w", definition.spec.name, err)
	}
	switch definition.spec.kind {
	case retainedGlobalData:
		if definition.property.accessor {
			return fmt.Errorf("goja-eventloop: global %q prepared as an accessor", definition.spec.name)
		}
		if definition.spec.function != nil {
			function, ok := definition.property.value.(*goja.Object)
			if !ok || function == nil {
				return fmt.Errorf("goja-eventloop: global %q function is unavailable", definition.spec.name)
			}
			if _, ok := goja.AssertFunction(function); !ok {
				return fmt.Errorf("goja-eventloop: global %q is not callable", definition.spec.name)
			}
			if err := verifyFunctionShape(j.adapter, function, *definition.spec.function); err != nil {
				return fmt.Errorf("goja-eventloop: global %q: %w", definition.spec.name, err)
			}
		}
	case retainedGlobalAccessor, retainedGlobalReadonlyAccessor:
		if !definition.property.accessor {
			return fmt.Errorf("goja-eventloop: global %q prepared as a data property", definition.spec.name)
		}
		getter, _ := definition.property.getter.(*goja.Object)
		if getter == nil || definition.spec.getter == nil {
			return fmt.Errorf("goja-eventloop: global %q getter shape is unavailable", definition.spec.name)
		}
		if err := verifyFunctionShape(j.adapter, getter, *definition.spec.getter); err != nil {
			return fmt.Errorf("goja-eventloop: global %q getter: %w", definition.spec.name, err)
		}
		if definition.spec.kind == retainedGlobalAccessor {
			setter, _ := definition.property.setter.(*goja.Object)
			if setter == nil || definition.spec.setter == nil {
				return fmt.Errorf("goja-eventloop: global %q setter shape is unavailable", definition.spec.name)
			}
			if err := verifyFunctionShape(j.adapter, setter, *definition.spec.setter); err != nil {
				return fmt.Errorf("goja-eventloop: global %q setter: %w", definition.spec.name, err)
			}
		}
	default:
		return fmt.Errorf("goja-eventloop: global %q has an invalid descriptor kind", definition.spec.name)
	}
	return nil
}

func (j *installationJournal) verifyPinnedGlobal(name string, expected *goja.Object) error {
	if expected == nil {
		return fmt.Errorf("goja-eventloop: captured %s is unavailable", name)
	}
	descriptor, err := j.currentDescriptor(j.global, name)
	if err != nil {
		return err
	}
	object, ok := descriptor.(*goja.Object)
	if !ok || object == nil {
		return fmt.Errorf("goja-eventloop: global %s is not an own data property", name)
	}
	value := object.Get("value")
	if value == nil || !value.SameAs(expected) {
		return fmt.Errorf("goja-eventloop: global %s identity changed during Bind", name)
	}
	return nil
}

func (j *installationJournal) preflightDisposeSymbol(extensible map[*goja.Object]bool) error {
	if j.symbol == nil || j.adapter.disposeSymbol == nil {
		return errors.New("goja-eventloop: prepared Symbol.dispose is unavailable")
	}
	descriptor, err := j.currentDescriptor(j.symbol, "dispose")
	if err != nil {
		return err
	}
	if err := j.updateSnapshot(j.symbol, "dispose", descriptor); err != nil {
		return err
	}
	j.disposeNeedsCommit = false
	if goja.IsUndefined(descriptor) {
		if err := j.requireExtensible(j.symbol, extensible, "dispose"); err != nil {
			return err
		}
		j.disposeNeedsCommit = true
		return nil
	}
	object, ok := descriptor.(*goja.Object)
	if !ok || object == nil {
		return errors.New("goja-eventloop: Symbol.dispose descriptor is invalid")
	}
	current := object.Get("value")
	if current == nil || !current.SameAs(j.adapter.disposeSymbol) {
		return errors.New("goja-eventloop: Symbol.dispose changed after construction")
	}
	if propertyBoolean(object, "configurable") {
		j.disposeNeedsCommit = true
		return nil
	}
	if propertyBoolean(object, "writable") || propertyBoolean(object, "enumerable") {
		return errors.New("goja-eventloop: Symbol.dispose descriptor cannot be normalized")
	}
	return nil
}

func (j *installationJournal) preflightCommit() error {
	if j == nil || j.adapter == nil || j.global == nil {
		return errors.New("goja-eventloop: installation preflight is unavailable")
	}
	if j.adapter.runtime.GlobalObject() != j.global {
		return errors.New("goja-eventloop: global object identity changed during Bind")
	}
	extensible := make(map[*goja.Object]bool)
	for _, definition := range j.propertyDefinitions {
		if err := j.preflightProperty(definition, extensible); err != nil {
			return err
		}
	}
	for _, definition := range j.globalDefinitions {
		if err := j.preflightGlobal(definition, extensible); err != nil {
			return err
		}
	}
	if err := j.preflightDisposeSymbol(extensible); err != nil {
		return err
	}
	if err := j.verifyPinnedGlobal("Promise", j.promise); err != nil {
		return err
	}
	if err := j.verifyPinnedGlobal("Symbol", j.symbol); err != nil {
		return err
	}
	for _, definition := range j.prototypes {
		if definition.object == nil || definition.prototype == nil {
			return errors.New("goja-eventloop: prepared prototype is unavailable")
		}
		if definition.object.Prototype() != definition.expected {
			return errors.New("goja-eventloop: prepared object prototype changed during Bind")
		}
		if err := j.requireExtensible(definition.object, extensible, "[[Prototype]]"); err != nil {
			return fmt.Errorf("goja-eventloop: prepare object prototype: %w", err)
		}
		for current := definition.prototype; current != nil; current = current.Prototype() {
			if current == definition.object {
				return errors.New("goja-eventloop: prepared object prototype would form a cycle")
			}
		}
	}
	if j.adapter.runtime.GlobalObject() != j.global {
		return errors.New("goja-eventloop: global object identity changed during Bind")
	}
	return nil
}

func (j *installationJournal) requireDefinable(object *goja.Object, name string) error {
	property, ok := j.property(object, name)
	if !ok {
		return fmt.Errorf("property %q was not journaled", name)
	}
	if goja.IsUndefined(property.descriptor) {
		return nil
	}
	descriptor, ok := property.descriptor.(*goja.Object)
	if !ok || descriptor == nil {
		return fmt.Errorf("property %q has an invalid descriptor", name)
	}
	if !propertyBoolean(descriptor, "configurable") {
		return fmt.Errorf("property %q is not configurable", name)
	}
	return nil
}

func (j *installationJournal) queueProperty(definition preparedPropertyDefinition) error {
	if definition.object == nil || definition.name == "" {
		return errors.New("goja-eventloop: prepared property target and name are required")
	}
	if err := j.requireDefinable(definition.object, definition.name); err != nil {
		return err
	}
	for _, existing := range j.propertyDefinitions {
		if existing.object == definition.object && existing.name == definition.name {
			return fmt.Errorf("goja-eventloop: property %q was prepared more than once", definition.name)
		}
	}
	j.propertyDefinitions = append(j.propertyDefinitions, definition)
	return nil
}

func (j *installationJournal) prepareDataProperties(target, source *goja.Object, names []string) error {
	if target == nil || source == nil {
		return errors.New("goja-eventloop: prepared property source and target are required")
	}
	for _, name := range names {
		descriptorValue, err := j.adapter.objectGetOwnPropertyDesc(
			goja.Undefined(),
			source,
			j.adapter.runtime.ToValue(name),
		)
		if err != nil {
			return wrapRuntimeError(fmt.Sprintf("inspect prepared property %q", name), err)
		}
		descriptor, ok := descriptorValue.(*goja.Object)
		if !ok || descriptor == nil {
			continue
		}
		if getter := descriptor.Get("get"); getter != nil && !goja.IsUndefined(getter) {
			return fmt.Errorf("goja-eventloop: prepared property %q is not a data property", name)
		}
		if setter := descriptor.Get("set"); setter != nil && !goja.IsUndefined(setter) {
			return fmt.Errorf("goja-eventloop: prepared property %q is not a data property", name)
		}
		if err := j.queueProperty(preparedPropertyDefinition{
			object:       target,
			name:         name,
			value:        descriptor.Get("value"),
			writable:     propertyBoolean(descriptor, "writable"),
			configurable: propertyBoolean(descriptor, "configurable"),
			enumerable:   propertyBoolean(descriptor, "enumerable"),
		}); err != nil {
			return fmt.Errorf("goja-eventloop: prepare property %q: %w", name, err)
		}
	}
	return nil
}

func (definition preparedPropertyDefinition) apply() error {
	if definition.object == nil {
		return errors.New("prepared property target is nil")
	}
	if definition.accessor {
		return definition.object.DefineAccessorProperty(
			definition.name,
			definition.getter,
			definition.setter,
			boolFlag(definition.configurable),
			boolFlag(definition.enumerable),
		)
	}
	return definition.object.DefineDataProperty(
		definition.name,
		definition.value,
		boolFlag(definition.writable),
		boolFlag(definition.configurable),
		boolFlag(definition.enumerable),
	)
}

func (j *installationJournal) recordAttempt(object *goja.Object, name string) error {
	property, ok := j.property(object, name)
	if !ok {
		return fmt.Errorf("goja-eventloop: property %q was not journaled", name)
	}
	j.attempted = append(j.attempted, property)
	return nil
}

func (j *installationJournal) commitMutable() error {
	for _, definition := range j.prototypes {
		j.attemptedPrototypes = append(j.attemptedPrototypes, prototypeSnapshot{
			object:    definition.object,
			prototype: definition.expected,
		})
		if err := definition.object.SetPrototype(definition.prototype); err != nil {
			return wrapRuntimeError("install object prototype", err)
		}
	}
	for _, definition := range j.propertyDefinitions {
		if err := j.recordAttempt(definition.object, definition.name); err != nil {
			return err
		}
		if err := definition.apply(); err != nil {
			return wrapRuntimeError(fmt.Sprintf("install property %q", definition.name), err)
		}
	}
	for _, definition := range j.globalDefinitions {
		if err := j.recordAttempt(definition.property.object, definition.spec.name); err != nil {
			return err
		}
		if err := definition.property.apply(); err != nil {
			return wrapRuntimeError(fmt.Sprintf("install global %q", definition.spec.name), err)
		}
	}
	return nil
}
