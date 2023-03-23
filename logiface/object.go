package logiface

import (
	"encoding/base64"
	"time"
)

type (
	ObjectBuilder[E Event, P Parent[E]] refPoolItem

	objectBuilderInterface interface {
		isObjectBuilder() *refPoolItem
		as(key string) any
	}
)

func Object[E Event, P Parent[E]](p P) (obj *ObjectBuilder[E, P]) {
	if p.Enabled() {
		obj = (*ObjectBuilder[E, P])(refPoolGet())
		obj.a = p
		// note: takes into account mustUseDefaultJSONSupport
		obj.b = p.objNew()
	}
	return
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objNew() any {
	return new(contextFieldData[E])
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) jsonObject(key string, arr any) {
	o := arr.(*contextFieldData[E])
	o.key = key
	o.shared = x.logger.shared
	x.Modifiers = append(x.Modifiers, ModifierFunc[E](o.object))
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objField(obj any, key string, val any) any {
	o := obj.(*contextFieldData[E])
	o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
		return shared.json.setField(obj, key, val)
	})
	return obj
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objObject(obj any, key string, val any) (any, bool) {
	if x.logger.shared.json.iface.CanSetObject() {
		o := obj.(*contextFieldData[E])
		v := val.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], arr any) any {
			val := shared.json.newObject()
			for _, f := range v.values {
				val = f(shared, val)
			}
			return shared.json.setObject(arr, key, val)
		})
		return obj, true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objArray(obj any, key string, val any) (any, bool) {
	if x.logger.shared.json.iface.CanSetArray() {
		o := obj.(*contextFieldData[E])
		v := val.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
			val := shared.json.newArray()
			for _, f := range v.values {
				val = f(shared, val)
			}
			return shared.json.setArray(obj, key, val)
		})
		return obj, true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objNew() any {
	return x.shared.json.newObject()
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) jsonObject(key string, obj any) {
	x.shared.json.addObject(x.Event, key, obj)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objField(obj any, key string, val any) any {
	return x.shared.json.setField(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objObject(obj any, key string, val any) (any, bool) {
	if x.shared.json.iface.CanSetObject() {
		return x.shared.json.setObject(obj, key, val), true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objArray(obj any, key string, val any) (any, bool) {
	if x.shared.json.iface.CanSetArray() {
		return x.shared.json.setArray(obj, key, val), true
	}
	return obj, false
}

// p returns the parent of this object builder
func (x *ObjectBuilder[E, P]) p() (p P) {
	if x != nil && x.a != nil {
		p = x.a.(P)
	}
	return
}

// Enabled returns whether the parent is enabled.
func (x *ObjectBuilder[E, P]) Enabled() bool {
	return x.p().Enabled()
}

// As writes the object to the parent, using the provided key (if relevant),
// and returns the parent.
//
// While you may use this to add an object to an array, providing a non-empty
// key with an array as a parent will result in a [Logger.DPanic]. In such
// cases, prefer [ObjectBuilder.Add].
//
// WARNING: References to the receiver must not be retained.
func (x *ObjectBuilder[E, P]) As(key string) (p P) {
	if x.Enabled() {
		p = x.p()
		p.jsonObject(key, x.b)
		refPoolPut((*refPoolItem)(x))
	}
	return
}

// Add is an alias for [ObjectBuilder.As](""), and is provided for convenience,
// to improve clarity when adding an object to an array (a case where the key
// must be an empty string).
//
// WARNING: References to the receiver must not be retained.
func (x *ObjectBuilder[E, P]) Add() (p P) {
	return x.As(``)
}

// Call is provided as a convenience, to facilitate code which uses the
// receiver explicitly, without breaking out of the fluent-style API.
// The provided fn will not be called if not [ObjectBuilder.Enabled].
func (x *ObjectBuilder[E, P]) Call(fn func(a *ObjectBuilder[E, P])) *ObjectBuilder[E, P] {
	if x.Enabled() {
		fn(x)
	}
	return x
}

func (x *ObjectBuilder[E, P]) Field(key string, val any) *ObjectBuilder[E, P] {
	_ = x.methods().Field(x.fields(), key, val)
	return x
}

func (x *ObjectBuilder[E, P]) Interface(key string, val any) *ObjectBuilder[E, P] {
	_ = x.methods().Interface(x.fields(), key, val)
	return x
}

func (x *ObjectBuilder[E, P]) Any(key string, val any) *ObjectBuilder[E, P] {
	return x.Interface(key, val)
}

func (x *ObjectBuilder[E, P]) Err(val error) *ObjectBuilder[E, P] {
	_ = x.methods().Err(x.fields(), val)
	return x
}

func (x *ObjectBuilder[E, P]) Str(key string, val string) *ObjectBuilder[E, P] {
	_ = x.methods().Str(x.fields(), key, val)
	return x
}

func (x *ObjectBuilder[E, P]) Int(key string, val int) *ObjectBuilder[E, P] {
	_ = x.methods().Int(x.fields(), key, val)
	return x
}

func (x *ObjectBuilder[E, P]) Float32(key string, val float32) *ObjectBuilder[E, P] {
	_ = x.methods().Float32(x.fields(), key, val)
	return x
}

func (x *ObjectBuilder[E, P]) Time(key string, val time.Time) *ObjectBuilder[E, P] {
	_ = x.methods().Time(x.fields(), key, val)
	return x
}

func (x *ObjectBuilder[E, P]) Dur(key string, val time.Duration) *ObjectBuilder[E, P] {
	_ = x.methods().Dur(x.fields(), key, val)
	return x
}

func (x *ObjectBuilder[E, P]) Base64(key string, b []byte, enc *base64.Encoding) *ObjectBuilder[E, P] {
	_ = x.methods().Base64(x.fields(), key, b, enc)
	return x
}

func (x *ObjectBuilder[E, P]) Bool(key string, val bool) *ObjectBuilder[E, P] {
	_ = x.methods().Bool(x.fields(), key, val)
	return x
}

func (x *ObjectBuilder[E, P]) Float64(key string, val float64) *ObjectBuilder[E, P] {
	_ = x.methods().Float64(x.fields(), key, val)
	return x
}

func (x *ObjectBuilder[E, P]) Int64(key string, val int64) *ObjectBuilder[E, P] {
	_ = x.methods().Int64(x.fields(), key, val)
	return x
}

func (x *ObjectBuilder[E, P]) Uint64(key string, val uint64) *ObjectBuilder[E, P] {
	_ = x.methods().Uint64(x.fields(), key, val)
	return x
}

func (x *ObjectBuilder[E, P]) as(key string) any {
	return x.As(key)
}

func (x *ObjectBuilder[E, P]) isObjectBuilder() *refPoolItem {
	return (*refPoolItem)(x)
}

func (x *ObjectBuilder[E, P]) mustUseDefaultJSONSupport() bool {
	switch getParentJSONType(x.a) {
	case parentJSONTypeArray:
		return !x.p().jsonSupport().CanAppendObject()
	case parentJSONTypeObject:
		return !x.p().jsonSupport().CanSetObject()
	default:
		return false
	}
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) root() *Logger[E] {
	return x.p().root()
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) jsonSupport() iJSONSupport[E] {
	if x.mustUseDefaultJSONSupport() {
		return defaultJSONSupport[E]{}
	}
	return x.p().jsonSupport()
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objNew() any {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).NewObject()
	}
	return x.p().objNew()
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) jsonObject(key string, obj any) {
	if !x.jsonSupport().CanSetObject() {
		x.b = x.objField(x.b, key, obj.(map[string]any))
	} else if v, ok := x.objObject(x.b, key, obj); !ok {
		x.root().DPanic().Log(`logiface: implementation disallows writing an object to an object`)
	} else {
		x.b = v
	}
}

func (x *ObjectBuilder[E, P]) objField(obj any, key string, val any) any {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).SetField(obj.(map[string]any), key, val)
	}
	return x.p().objField(obj, key, val)
}

func (x *ObjectBuilder[E, P]) objObject(obj any, key string, val any) (any, bool) {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).SetObject(obj.(map[string]any), key, val.(map[string]any)), true
	}
	return x.p().objObject(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objArray(obj any, key string, val any) (any, bool) {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).SetArray(obj.(map[string]any), key, val.([]any)), true
	}
	return x.p().objArray(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrNew() any {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).NewArray()
	}
	return x.p().arrNew()
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) jsonArray(key string, arr any) {
	if !x.jsonSupport().CanSetArray() {
		x.b = x.objField(x.b, key, arr.([]any))
	} else if v, ok := x.objArray(x.b, key, arr); !ok {
		x.root().DPanic().Log(`logiface: implementation disallows writing an array to an object`)
	} else {
		x.b = v
	}
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrField(arr any, val any) any {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).AppendField(arr.([]any), val)
	}
	return x.p().arrField(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrArray(arr, val any) (any, bool) {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).AppendArray(arr.([]any), val.([]any)), true
	}
	return x.p().arrArray(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrObject(arr, val any) (any, bool) {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).AppendObject(arr.([]any), val.(map[string]any)), true
	}
	return x.p().arrObject(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrStr(arr any, val string) (any, bool) {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).AppendString(arr.([]any), val), true
	}
	return x.p().arrStr(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrBool(arr any, val bool) (any, bool) {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).AppendBool(arr.([]any), val), true
	}
	return x.p().arrBool(arr, val)
}
