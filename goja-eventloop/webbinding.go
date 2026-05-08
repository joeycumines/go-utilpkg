package gojaeventloop

import (
	"fmt"

	"github.com/joeycumines/goja"
)

func defineWebFunction(runtime *goja.Runtime, value goja.Value, name string, length int64, context string) error {
	fn, ok := value.(*goja.Object)
	if !ok || fn == nil {
		return fmt.Errorf("%s: function is not an object", context)
	}
	if err := fn.DefineDataProperty("name", runtime.ToValue(name), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
		return wrapRuntimeError(context+" name", err)
	}
	if err := fn.DefineDataProperty("length", runtime.ToValue(length), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
		return wrapRuntimeError(context+" length", err)
	}
	return nil
}

func defineWebConstructorObject(runtime *goja.Runtime, constructor *goja.Object, name string, length int64) error {
	if constructor == nil {
		return fmt.Errorf("define %s constructor: function is not an object", name)
	}
	if err := defineWebFunction(runtime, constructor, name, length, "define "+name+" constructor"); err != nil {
		return err
	}
	return lockWebConstructorPrototype(runtime, constructor, name)
}

func lockWebConstructorPrototype(runtime *goja.Runtime, constructor *goja.Object, name string) error {
	prototype := constructor.Get("prototype")
	if prototype == nil || goja.IsUndefined(prototype) {
		return fmt.Errorf("define %s constructor: prototype is unavailable", name)
	}
	if err := constructor.DefineDataProperty("prototype", prototype, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE); err != nil {
		return wrapRuntimeError("define "+name+" constructor prototype", err)
	}
	return nil
}

func defineWebMethod(runtime *goja.Runtime, target *goja.Object, name string, length int64, enumerable bool, callback func(goja.FunctionCall) goja.Value) error {
	value := runtime.ToValue(callback)
	if err := defineWebFunction(runtime, value, name, length, "define "+name+" method"); err != nil {
		return err
	}
	enumerableFlag := goja.FLAG_FALSE
	if enumerable {
		enumerableFlag = goja.FLAG_TRUE
	}
	if err := target.DefineDataProperty(name, value, goja.FLAG_TRUE, goja.FLAG_TRUE, enumerableFlag); err != nil {
		return wrapRuntimeError("define "+name+" method property", err)
	}
	return nil
}

func defineWebAccessor(runtime *goja.Runtime, target *goja.Object, name string, enumerable bool, getter func(goja.FunctionCall) goja.Value, setter func(goja.FunctionCall) goja.Value) error {
	getterValue := runtime.ToValue(getter)
	if err := defineWebFunction(runtime, getterValue, "get "+name, 0, "define "+name+" getter"); err != nil {
		return err
	}
	var setterValue goja.Value
	if setter != nil {
		setterValue = runtime.ToValue(setter)
		if err := defineWebFunction(runtime, setterValue, "set "+name, 1, "define "+name+" setter"); err != nil {
			return err
		}
	}
	enumerableFlag := goja.FLAG_FALSE
	if enumerable {
		enumerableFlag = goja.FLAG_TRUE
	}
	if err := target.DefineAccessorProperty(name, getterValue, setterValue, goja.FLAG_TRUE, enumerableFlag); err != nil {
		return wrapRuntimeError("define "+name+" accessor property", err)
	}
	return nil
}

func defineWebTag(runtime *goja.Runtime, target *goja.Object, tag string) error {
	if err := target.DefineDataPropertySymbol(goja.SymToStringTag, runtime.ToValue(tag), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
		return wrapRuntimeError("define "+tag+" toStringTag", err)
	}
	return nil
}
