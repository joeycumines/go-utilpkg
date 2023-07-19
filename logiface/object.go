package logiface

import (
	"encoding/base64"
	"fmt"
	"time"
)

type (
	ObjectBuilder[E Event, P Parent[E]] refPoolItem

	objectBuilderInterface interface {
		isObjectBuilder() *refPoolItem
		as(key string) any
	}
)

func Object[E Event, P Parent[E]](p P) *ObjectBuilder[E, P] {
	return ObjectWithKey[E, P](p, ``)
}

func ObjectWithKey[E Event, P Parent[E]](p P, key string) (obj *ObjectBuilder[E, P]) {
	if p.Enabled() {
		obj = (*ObjectBuilder[E, P])(refPoolGet())
		obj.a = p
		if obj.jsonMustUseDefault() {
			obj.b = (defaultJSONSupport[E]{}).NewObject()
		} else {
			// note: also takes into account jsonMustUseDefault for p (and it's parent(s))
			obj.b = p.jsonNewObject(key)
		}
	}
	return
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) jsonNewObject(key string) any {
	return &contextFieldData[E]{key: key}
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objNewObject(obj any, key string) any {
	return &contextFieldData[E]{key: key}
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrNewObject(arr any) any {
	return &contextFieldData[E]{}
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) jsonWriteObject(key string, arr any) {
	o := arr.(*contextFieldData[E])
	if key != `` && o.key == `` {
		o.key = key
	}
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
func (x *Context[E]) objWriteObject(obj any, key string, val any) (any, bool) {
	if x.logger.shared.json.iface.CanSetObject() {
		o := obj.(*contextFieldData[E])
		v := val.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
			val := shared.json.setStartOrNewObject(obj, key)
			for _, f := range v.values {
				val = f(shared, val)
			}
			return shared.json.setObject(obj, key, val)
		})
		return obj, true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objWriteArray(obj any, key string, val any) (any, bool) {
	if x.logger.shared.json.iface.CanSetArray() {
		o := obj.(*contextFieldData[E])
		v := val.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
			val := shared.json.setStartOrNewArray(obj, key)
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
func (x *Context[E]) objString(obj any, key string, val string) (any, bool) {
	if x.logger.shared.json.iface.CanSetString() {
		o := obj.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
			return shared.json.setString(obj, key, val)
		})
		return obj, true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objBool(obj any, key string, val bool) (any, bool) {
	if x.logger.shared.json.iface.CanSetBool() {
		o := obj.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
			return shared.json.setBool(obj, key, val)
		})
		return obj, true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objBase64Bytes(obj any, key string, b []byte, enc *base64.Encoding) (any, bool) {
	if x.logger.shared.json.iface.CanSetBase64Bytes() {
		o := obj.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
			return shared.json.setBase64Bytes(obj, key, b, enc)
		})
		return obj, true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objDuration(obj any, key string, d time.Duration) (any, bool) {
	if x.logger.shared.json.iface.CanSetDuration() {
		o := obj.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
			return shared.json.setDuration(obj, key, d)
		})
		return obj, true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objError(obj any, err error) (any, bool) {
	if x.logger.shared.json.iface.CanSetError() {
		o := obj.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
			return shared.json.setError(obj, err)
		})
		return obj, true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objInt(obj any, key string, val int) (any, bool) {
	if x.logger.shared.json.iface.CanSetInt() {
		o := obj.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
			return shared.json.setInt(obj, key, val)
		})
		return obj, true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objFloat32(obj any, key string, val float32) (any, bool) {
	if x.logger.shared.json.iface.CanSetFloat32() {
		o := obj.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
			return shared.json.setFloat32(obj, key, val)
		})
		return obj, true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objTime(obj any, key string, t time.Time) (any, bool) {
	if x.logger.shared.json.iface.CanSetTime() {
		o := obj.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
			return shared.json.setTime(obj, key, t)
		})
		return obj, true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objFloat64(obj any, key string, val float64) (any, bool) {
	if x.logger.shared.json.iface.CanSetFloat64() {
		o := obj.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
			return shared.json.setFloat64(obj, key, val)
		})
		return obj, true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objInt64(obj any, key string, val int64) (any, bool) {
	if x.logger.shared.json.iface.CanSetInt64() {
		o := obj.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
			return shared.json.setInt64(obj, key, val)
		})
		return obj, true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objUint64(obj any, key string, val uint64) (any, bool) {
	if x.logger.shared.json.iface.CanSetUint64() {
		o := obj.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
			return shared.json.setUint64(obj, key, val)
		})
		return obj, true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) jsonNewObject(key string) any {
	return x.shared.json.addStartOrNewObject(x.Event, key)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objNewObject(obj any, key string) any {
	return x.shared.json.setStartOrNewObject(obj, key)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrNewObject(arr any) any {
	return x.shared.json.appendStartOrNewObject(arr)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) jsonWriteObject(key string, obj any) {
	x.shared.json.addObject(x.Event, key, obj)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objField(obj any, key string, val any) any {
	return x.shared.json.setField(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objWriteObject(obj any, key string, val any) (any, bool) {
	if x.shared.json.iface.CanSetObject() {
		return x.shared.json.setObject(obj, key, val), true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objWriteArray(obj any, key string, val any) (any, bool) {
	if x.shared.json.iface.CanSetArray() {
		return x.shared.json.setArray(obj, key, val), true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objString(obj any, key string, val string) (any, bool) {
	if x.shared.json.iface.CanSetString() {
		return x.shared.json.setString(obj, key, val), true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objBool(obj any, key string, val bool) (any, bool) {
	if x.shared.json.iface.CanSetBool() {
		return x.shared.json.setBool(obj, key, val), true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objBase64Bytes(obj any, key string, b []byte, enc *base64.Encoding) (any, bool) {
	if x.shared.json.iface.CanSetBase64Bytes() {
		return x.shared.json.setBase64Bytes(obj, key, b, enc), true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objDuration(obj any, key string, d time.Duration) (any, bool) {
	if x.shared.json.iface.CanSetDuration() {
		return x.shared.json.setDuration(obj, key, d), true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objError(obj any, err error) (any, bool) {
	if x.shared.json.iface.CanSetError() {
		return x.shared.json.setError(obj, err), true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objInt(obj any, key string, val int) (any, bool) {
	if x.shared.json.iface.CanSetInt() {
		return x.shared.json.setInt(obj, key, val), true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objFloat32(obj any, key string, val float32) (any, bool) {
	if x.shared.json.iface.CanSetFloat32() {
		return x.shared.json.setFloat32(obj, key, val), true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objTime(obj any, key string, t time.Time) (any, bool) {
	if x.shared.json.iface.CanSetTime() {
		return x.shared.json.setTime(obj, key, t), true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objFloat64(obj any, key string, val float64) (any, bool) {
	if x.shared.json.iface.CanSetFloat64() {
		return x.shared.json.setFloat64(obj, key, val), true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objInt64(obj any, key string, val int64) (any, bool) {
	if x.shared.json.iface.CanSetInt64() {
		return x.shared.json.setInt64(obj, key, val), true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objUint64(obj any, key string, val uint64) (any, bool) {
	if x.shared.json.iface.CanSetUint64() {
		return x.shared.json.setUint64(obj, key, val), true
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
		p.jsonWriteObject(key, x.b)
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
func (x *ObjectBuilder[E, P]) Call(fn func(b *ObjectBuilder[E, P])) *ObjectBuilder[E, P] {
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

func (x *ObjectBuilder[E, P]) Stringer(key string, val fmt.Stringer) *ObjectBuilder[E, P] {
	_ = x.methods().Stringer(x.fields(), key, val)
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

func (x *ObjectBuilder[E, P]) jsonMustUseDefault() bool {
	switch getParentJSONType(x.a) {
	case parentJSONTypeArray:
		return !x.p().jsonSupport().CanAppendObject()
	case parentJSONTypeObject:
		return !x.p().jsonSupport().CanSetObject()
	default:
		return false
	}
}

// Root returns the root [Logger] for this instance.
func (x *ObjectBuilder[E, P]) Root() *Logger[E] {
	return x.p().Root()
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) jsonSupport() iJSONSupport[E] {
	if x.jsonMustUseDefault() {
		return defaultJSONSupport[E]{}
	}
	return x.p().jsonSupport()
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) jsonNewObject(key string) any {
	return x.objNewObject(x.b, key)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objNewObject(obj any, key string) any {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).NewObject()
	}
	return x.p().objNewObject(obj, key)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrNewObject(arr any) any {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).NewObject()
	}
	return x.p().arrNewObject(arr)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) jsonWriteObject(key string, obj any) {
	if !x.jsonSupport().CanSetObject() {
		x.b = x.objField(x.b, key, obj.(map[string]any))
	} else if v, ok := x.objWriteObject(x.b, key, obj); !ok {
		x.Root().DPanic().Log(`logiface: implementation disallows writing an object to an object`)
	} else {
		x.b = v
	}
}

func (x *ObjectBuilder[E, P]) objField(obj any, key string, val any) any {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).SetField(obj.(map[string]any), key, val)
	}
	return x.p().objField(obj, key, val)
}

func (x *ObjectBuilder[E, P]) objWriteObject(obj any, key string, val any) (any, bool) {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).SetObject(obj.(map[string]any), key, val.(map[string]any)), true
	}
	return x.p().objWriteObject(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objWriteArray(obj any, key string, val any) (any, bool) {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).SetArray(obj.(map[string]any), key, val.([]any)), true
	}
	return x.p().objWriteArray(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objString(obj any, key string, val string) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objString(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objBool(obj any, key string, val bool) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objBool(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objBase64Bytes(obj any, key string, b []byte, enc *base64.Encoding) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objBase64Bytes(obj, key, b, enc)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objDuration(obj any, key string, d time.Duration) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objDuration(obj, key, d)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objError(obj any, err error) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objError(obj, err)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objInt(obj any, key string, val int) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objInt(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objFloat32(obj any, key string, val float32) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objFloat32(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objTime(obj any, key string, t time.Time) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objTime(obj, key, t)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objFloat64(obj any, key string, val float64) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objFloat64(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objInt64(obj any, key string, val int64) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objInt64(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objUint64(obj any, key string, val uint64) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objUint64(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) jsonNewArray(key string) any {
	return x.objNewArray(x.b, key)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objNewArray(obj any, key string) any {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).NewArray()
	}
	return x.p().objNewArray(obj, key)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrNewArray(arr any) any {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).NewArray()
	}
	return x.p().arrNewArray(arr)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) jsonWriteArray(key string, arr any) {
	if !x.jsonSupport().CanSetArray() {
		x.b = x.objField(x.b, key, arr.([]any))
	} else if v, ok := x.objWriteArray(x.b, key, arr); !ok {
		x.Root().DPanic().Log(`logiface: implementation disallows writing an array to an object`)
	} else {
		x.b = v
	}
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrField(arr any, val any) any {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).AppendField(arr.([]any), val)
	}
	return x.p().arrField(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrWriteArray(arr, val any) (any, bool) {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).AppendArray(arr.([]any), val.([]any)), true
	}
	return x.p().arrWriteArray(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrWriteObject(arr, val any) (any, bool) {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).AppendObject(arr.([]any), val.(map[string]any)), true
	}
	return x.p().arrWriteObject(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrString(arr any, val string) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrString(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrBool(arr any, val bool) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrBool(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrBase64Bytes(arr any, b []byte, enc *base64.Encoding) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrBase64Bytes(arr, b, enc)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrDuration(arr any, d time.Duration) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrDuration(arr, d)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrError(arr any, err error) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrError(arr, err)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrInt(arr any, val int) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrInt(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrFloat32(arr any, val float32) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrFloat32(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrTime(arr any, t time.Time) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrTime(arr, t)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrFloat64(arr any, val float64) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrFloat64(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrInt64(arr any, val int64) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrInt64(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrUint64(arr any, val uint64) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrUint64(arr, val)
}
