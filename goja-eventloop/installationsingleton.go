package gojaeventloop

import (
	"errors"
	"fmt"

	"github.com/joeycumines/goja"
)

func coherentHostSingleton(runtime *goja.Runtime, name, constructorName string) (*goja.Object, bool, error) {
	object, _, preserved, err := coherentHostSingletonPair(runtime, name, constructorName)
	return object, preserved, err
}

func coherentHostSingletonPair(runtime *goja.Runtime, name, constructorName string) (*goja.Object, *goja.Object, bool, error) {
	var value, constructorValue goja.Value
	if exception := runtime.Try(func() {
		value = runtime.Get(name)
		constructorValue = runtime.Get(constructorName)
	}); exception != nil {
		return nil, nil, false, wrapRuntimeException("read host singleton globals", exception)
	}
	present := value != nil && !goja.IsUndefined(value)
	constructorPresent := constructorValue != nil && !goja.IsUndefined(constructorValue)
	if !present && !constructorPresent {
		return nil, nil, false, nil
	}
	if present != constructorPresent {
		return nil, nil, false, fmt.Errorf("singleton/constructor pair %s/%s is partial", name, constructorName)
	}
	if goja.IsNull(value) || goja.IsNull(constructorValue) {
		return nil, nil, false, errors.New("singleton/constructor pair contains null")
	}
	object, ok := value.(*goja.Object)
	if !ok || object == nil {
		return nil, nil, false, fmt.Errorf("global %s is not an object or function", name)
	}
	constructor, ok := constructorValue.(*goja.Object)
	if !ok || constructor == nil {
		return nil, nil, false, fmt.Errorf("global %s is not a function object", constructorName)
	}
	var inspectErr error
	if exception := runtime.Try(func() {
		if _, ok := goja.AssertFunction(constructor); !ok {
			inspectErr = fmt.Errorf("constructor %q is not callable", constructorName)
			return
		}
		prototype, ok := constructor.Get("prototype").(*goja.Object)
		if !ok || prototype == nil {
			inspectErr = fmt.Errorf("constructor %q prototype is unavailable", constructorName)
			return
		}
		if object.Prototype() != prototype {
			inspectErr = fmt.Errorf("singleton %q has a mismatched prototype", name)
			return
		}
		prototypeConstructor := prototype.Get("constructor")
		if prototypeConstructor == nil || !prototypeConstructor.SameAs(constructor) {
			inspectErr = fmt.Errorf("constructor %q prototype identity differs", constructorName)
			return
		}
	}); exception != nil {
		return nil, nil, false, wrapRuntimeException("inspect host singleton", exception)
	}
	if inspectErr != nil {
		return nil, nil, false, inspectErr
	}
	return object, constructor, true, nil
}

func inheritedPropertyDescriptor(runtime *goja.Runtime, object *goja.Object, name string) (*goja.Object, error) {
	getDescriptor := objectGetOwnPropertyDescriptor(runtime)
	if getDescriptor == nil {
		return nil, errors.New("Object.getOwnPropertyDescriptor is unavailable")
	}
	for current := object; current != nil; current = current.Prototype() {
		value, err := getDescriptor(goja.Undefined(), current, runtime.ToValue(name))
		if err != nil {
			return nil, wrapRuntimeError(fmt.Sprintf("inspect property %q", name), err)
		}
		if value == nil || goja.IsUndefined(value) {
			continue
		}
		descriptor, ok := value.(*goja.Object)
		if !ok || descriptor == nil {
			return nil, fmt.Errorf("inspect property %q: descriptor is unavailable", name)
		}
		return descriptor, nil
	}
	return nil, fmt.Errorf("required property %q is unavailable", name)
}

func verifyBrandedSingletonObject(adapter *Adapter, singleton, constructor *goja.Object, constructorName string, methods, properties []string) (map[string]goja.Callable, error) {
	if adapter == nil || singleton == nil || constructor == nil {
		return nil, errors.New("branded singleton is unavailable")
	}
	if _, ok := goja.AssertFunction(constructor); !ok {
		return nil, fmt.Errorf("constructor %q is not callable", constructorName)
	}
	prototypeValue := constructor.Get("prototype")
	prototype, ok := prototypeValue.(*goja.Object)
	if !ok || prototype == nil {
		return nil, fmt.Errorf("constructor %q prototype is unavailable", constructorName)
	}
	if singleton.Prototype() != prototype {
		return nil, fmt.Errorf("singleton does not carry the %s prototype brand", constructorName)
	}
	if prototypeConstructor := prototype.Get("constructor"); prototypeConstructor == nil || !prototypeConstructor.SameAs(constructor) {
		return nil, fmt.Errorf("%s prototype constructor identity differs", constructorName)
	}
	if tag := singleton.GetSymbol(goja.SymToStringTag); tag == nil || tag.String() != constructorName {
		return nil, fmt.Errorf("singleton does not carry the %s brand tag", constructorName)
	}
	callables := make(map[string]goja.Callable, len(methods))
	for _, method := range methods {
		callable, ok := goja.AssertFunction(singleton.Get(method))
		if !ok {
			return nil, fmt.Errorf("goja-eventloop: installed property %q is not callable", method)
		}
		callables[method] = callable
	}
	for _, property := range properties {
		descriptor, err := inheritedPropertyDescriptor(adapter.runtime, singleton, property)
		if err != nil {
			return nil, err
		}
		if getter := descriptor.Get("get"); getter == nil {
			return nil, fmt.Errorf("required property %q is not an accessor", property)
		} else if _, ok := goja.AssertFunction(getter); !ok {
			return nil, fmt.Errorf("required property %q getter is not callable", property)
		}
	}
	return callables, nil
}
