package gojagrpc

import (
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"google.golang.org/grpc/metadata"
)

// metadataObject returns a goja.Object exposing metadata creation
// utilities for gRPC calls.
//
// JavaScript usage:
//
//	const md = grpc.metadata.create();
//	md.set('key', 'value');
//	md.get('key');          // 'value'
//	md.getAll('key');       // ['value']
//	md.delete('key');
//	md.forEach((value, key) => { ... });
//	md.toObject();          // { key: ['value'] }
func (m *Module) metadataObject() *goja.Object {
	obj := m.runtime.NewObject()

	_ = obj.Set("create", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return m.newMetadataWrapper(metadata.New(nil))
	}))

	return obj
}

// newMetadataWrapper wraps a Go [metadata.MD] as a JavaScript object
// with set, get, getAll, delete, forEach, and toObject methods.
func (m *Module) newMetadataWrapper(md metadata.MD) *goja.Object {
	obj := m.runtime.NewObject()

	// set(key, ...values) — sets the key to the given values,
	// replacing any existing values.
	_ = obj.Set("set", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(m.runtime.NewTypeError("metadata.set requires at least 2 arguments"))
		}
		key := strings.ToLower(call.Argument(0).String())
		values := make([]string, 0, len(call.Arguments)-1)
		for i := 1; i < len(call.Arguments); i++ {
			values = append(values, call.Argument(i).String())
		}
		md.Set(key, values...)
		return goja.Undefined()
	}))

	// get(key) → string — returns the first value for the key,
	// or undefined if not set.
	_ = obj.Set("get", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		key := strings.ToLower(call.Argument(0).String())
		vals := md.Get(key)
		if len(vals) == 0 {
			return goja.Undefined()
		}
		return m.runtime.ToValue(vals[0])
	}))

	// getAll(key) → string[] — returns all values for the key.
	_ = obj.Set("getAll", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		key := strings.ToLower(call.Argument(0).String())
		vals := md.Get(key)
		arr := m.runtime.NewArray()
		for i, v := range vals {
			_ = arr.Set(strconv.Itoa(i), m.runtime.ToValue(v))
		}
		return arr
	}))

	// delete(key) — removes all values for the key.
	_ = obj.Set("delete", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		key := strings.ToLower(call.Argument(0).String())
		delete(md, key)
		return goja.Undefined()
	}))

	// forEach(fn) — calls fn(value, key) for each key-value pair.
	// For keys with multiple values, fn is called once per value.
	_ = obj.Set("forEach", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(m.runtime.NewTypeError("metadata.forEach requires a function"))
		}
		for key, vals := range md {
			for _, val := range vals {
				_, _ = fn(goja.Undefined(),
					m.runtime.ToValue(val),
					m.runtime.ToValue(key),
				)
			}
		}
		return goja.Undefined()
	}))

	// toObject() → plain JS object mapping keys to string arrays.
	_ = obj.Set("toObject", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		result := m.runtime.NewObject()
		for key, vals := range md {
			arr := m.runtime.NewArray()
			for i, v := range vals {
				_ = arr.Set(strconv.Itoa(i), m.runtime.ToValue(v))
			}
			_ = result.Set(key, arr)
		}
		return result
	}))

	return obj
}

// metadataToGo extracts a Go [metadata.MD] from a JavaScript metadata
// wrapper object. Returns nil if the value is nil, undefined, or not
// a metadata wrapper.
func (m *Module) metadataToGo(val goja.Value) metadata.MD {
	if val == nil || goja.IsNull(val) || goja.IsUndefined(val) {
		return nil
	}

	// Try calling toObject() to extract the data.
	obj, ok := val.(*goja.Object)
	if !ok {
		return nil
	}

	toObjectVal := obj.Get("toObject")
	if toObjectVal == nil || goja.IsUndefined(toObjectVal) {
		return nil
	}

	toObjectFn, ok := goja.AssertFunction(toObjectVal)
	if !ok {
		return nil
	}

	result, err := toObjectFn(obj)
	if err != nil {
		return nil
	}

	// Convert the plain object to metadata.MD.
	resultObj, ok := result.(*goja.Object)
	if !ok {
		return nil
	}

	md := metadata.MD{}
	for _, key := range resultObj.Keys() {
		arrVal := resultObj.Get(key)
		arrObj, ok := arrVal.(*goja.Object)
		if !ok {
			continue
		}
		lengthVal := arrObj.Get("length")
		if lengthVal == nil || goja.IsUndefined(lengthVal) {
			continue
		}
		length := int(lengthVal.ToInteger())
		vals := make([]string, 0, length)
		for i := 0; i < length; i++ {
			elemVal := arrObj.Get(strconv.Itoa(i))
			if elemVal != nil && !goja.IsUndefined(elemVal) {
				vals = append(vals, elemVal.String())
			}
		}
		if len(vals) > 0 {
			md[key] = vals
		}
	}

	return md
}

// metadataFromGo wraps a Go [metadata.MD] as a JavaScript metadata
// wrapper object. Returns undefined if the metadata is nil.
func (m *Module) metadataFromGo(md metadata.MD) goja.Value {
	if md == nil {
		return goja.Undefined()
	}
	return m.newMetadataWrapper(md)
}
