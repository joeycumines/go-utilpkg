package logiface

import (
	"encoding/base64"
	"time"
)

type (
	ArrayBuilder[E Event, P Parent[E]] refPoolItem

	//lint:ignore U1000 it is or will be used
	contextFieldData[E Event] struct {
		key    string
		shared *loggerShared[E]
		values []func(shared *loggerShared[E], target any) any
	}

	arrayBuilderInterface interface {
		isArrayBuilder() *refPoolItem
		as(key string) any
	}
)

// Array starts building a new array, as a field of a given [Context],
// [Builder] or [ArrayBuilder].
//
// In Go, generic methods are not allowed to introduce cyclic dependencies on
// generic types, and cannot introduce further generic types.
func Array[E Event, P Parent[E]](p P) *ArrayBuilder[E, P] {
	return ArrayWithKey[E, P](p, ``)
}

func ArrayWithKey[E Event, P Parent[E]](p P, key string) (arr *ArrayBuilder[E, P]) {
	if p.Enabled() {
		arr = (*ArrayBuilder[E, P])(refPoolGet())
		arr.a = p
		if arr.jsonMustUseDefault() {
			arr.b = (defaultJSONSupport[E]{}).NewArray()
		} else {
			// note: also takes into account jsonMustUseDefault for p (and it's parent(s))
			arr.b = p.jsonNewArray(key)
		}
	}
	return
}

// Context
// ---
// Uses *contextFieldData[E] as the array, backed by the logger's jsonSupport.

//lint:ignore U1000 it is or will be used
func (x *Context[E]) jsonSupport() iJSONSupport[E] {
	return x.logger.shared.json.iface
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) jsonMustUseDefault() bool {
	return false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) jsonNewArray(key string) any {
	return &contextFieldData[E]{key: key}
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objNewArray(obj any, key string) any {
	return &contextFieldData[E]{key: key}
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrNewArray(arr any) any {
	return &contextFieldData[E]{}
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) jsonWriteArray(key string, arr any) {
	a := arr.(*contextFieldData[E])
	if key != `` && a.key == `` {
		a.key = key
	}
	a.shared = x.logger.shared
	x.Modifiers = append(x.Modifiers, ModifierFunc[E](a.array))
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrField(arr any, val any) any {
	a := arr.(*contextFieldData[E])
	a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
		return shared.json.appendField(arr, val)
	})
	return arr
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrWriteArray(arr, val any) (any, bool) {
	if x.logger.shared.json.iface.CanAppendArray() {
		a := arr.(*contextFieldData[E])
		v := val.(*contextFieldData[E])
		a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
			val := shared.json.appendStartOrNewArray(arr)
			for _, f := range v.values {
				val = f(shared, val)
			}
			return shared.json.appendArray(arr, val)
		})
		return arr, true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrWriteObject(arr, val any) (any, bool) {
	if x.logger.shared.json.iface.CanAppendObject() {
		a := arr.(*contextFieldData[E])
		v := val.(*contextFieldData[E])
		a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
			val := shared.json.appendStartOrNewObject(arr)
			for _, f := range v.values {
				val = f(shared, val)
			}
			return shared.json.appendObject(arr, val)
		})
		return arr, true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrString(arr any, val string) (any, bool) {
	if x.logger.shared.json.iface.CanAppendString() {
		a := arr.(*contextFieldData[E])
		a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
			return shared.json.appendString(arr, val)
		})
		return arr, true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrBool(arr any, val bool) (any, bool) {
	if x.logger.shared.json.iface.CanAppendBool() {
		a := arr.(*contextFieldData[E])
		a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
			return shared.json.appendBool(arr, val)
		})
		return arr, true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrBase64Bytes(arr any, b []byte, enc *base64.Encoding) (any, bool) {
	if x.logger.shared.json.iface.CanAppendBase64Bytes() {
		a := arr.(*contextFieldData[E])
		a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
			return shared.json.appendBase64Bytes(arr, b, enc)
		})
		return arr, true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrDuration(arr any, d time.Duration) (any, bool) {
	if x.logger.shared.json.iface.CanAppendDuration() {
		a := arr.(*contextFieldData[E])
		a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
			return shared.json.appendDuration(arr, d)
		})
		return arr, true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrError(arr any, err error) (any, bool) {
	if x.logger.shared.json.iface.CanAppendError() {
		a := arr.(*contextFieldData[E])
		a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
			return shared.json.appendError(arr, err)
		})
		return arr, true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrInt(arr any, val int) (any, bool) {
	if x.logger.shared.json.iface.CanAppendInt() {
		a := arr.(*contextFieldData[E])
		a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
			return shared.json.appendInt(arr, val)
		})
		return arr, true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrFloat32(arr any, val float32) (any, bool) {
	if x.logger.shared.json.iface.CanAppendFloat32() {
		a := arr.(*contextFieldData[E])
		a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
			return shared.json.appendFloat32(arr, val)
		})
		return arr, true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrTime(arr any, t time.Time) (any, bool) {
	if x.logger.shared.json.iface.CanAppendTime() {
		a := arr.(*contextFieldData[E])
		a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
			return shared.json.appendTime(arr, t)
		})
		return arr, true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrFloat64(arr any, val float64) (any, bool) {
	if x.logger.shared.json.iface.CanAppendFloat64() {
		a := arr.(*contextFieldData[E])
		a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
			return shared.json.appendFloat64(arr, val)
		})
		return arr, true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrInt64(arr any, val int64) (any, bool) {
	if x.logger.shared.json.iface.CanAppendInt64() {
		a := arr.(*contextFieldData[E])
		a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
			return shared.json.appendInt64(arr, val)
		})
		return arr, true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrUint64(arr any, val uint64) (any, bool) {
	if x.logger.shared.json.iface.CanAppendUint64() {
		a := arr.(*contextFieldData[E])
		a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
			return shared.json.appendUint64(arr, val)
		})
		return arr, true
	}
	return arr, false
}

// Builder
// ---
// Uses the jsonSupport directly.

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) jsonSupport() iJSONSupport[E] {
	return x.shared.json.iface
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) jsonMustUseDefault() bool {
	return false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) jsonNewArray(key string) any {
	return x.shared.json.addStartOrNewArray(x.Event, key)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objNewArray(obj any, key string) any {
	return x.shared.json.setStartOrNewArray(obj, key)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrNewArray(arr any) any {
	return x.shared.json.appendStartOrNewArray(arr)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) jsonWriteArray(key string, arr any) {
	x.shared.json.addArray(x.Event, key, arr)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrField(arr any, val any) any {
	return x.shared.json.appendField(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrWriteArray(arr, val any) (any, bool) {
	if x.shared.json.iface.CanAppendArray() {
		return x.shared.json.appendArray(arr, val), true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrWriteObject(arr, val any) (any, bool) {
	if x.shared.json.iface.CanAppendObject() {
		return x.shared.json.appendObject(arr, val), true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrString(arr any, val string) (any, bool) {
	if x.shared.json.iface.CanAppendString() {
		return x.shared.json.appendString(arr, val), true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrBool(arr any, val bool) (any, bool) {
	if x.shared.json.iface.CanAppendBool() {
		return x.shared.json.appendBool(arr, val), true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrBase64Bytes(arr any, b []byte, enc *base64.Encoding) (any, bool) {
	if x.shared.json.iface.CanAppendBase64Bytes() {
		return x.shared.json.appendBase64Bytes(arr, b, enc), true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrDuration(arr any, d time.Duration) (any, bool) {
	if x.shared.json.iface.CanAppendDuration() {
		return x.shared.json.appendDuration(arr, d), true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrError(arr any, err error) (any, bool) {
	if x.shared.json.iface.CanAppendError() {
		return x.shared.json.appendError(arr, err), true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrInt(arr any, val int) (any, bool) {
	if x.shared.json.iface.CanAppendInt() {
		return x.shared.json.appendInt(arr, val), true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrFloat32(arr any, val float32) (any, bool) {
	if x.shared.json.iface.CanAppendFloat32() {
		return x.shared.json.appendFloat32(arr, val), true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrTime(arr any, t time.Time) (any, bool) {
	if x.shared.json.iface.CanAppendTime() {
		return x.shared.json.appendTime(arr, t), true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrFloat64(arr any, val float64) (any, bool) {
	if x.shared.json.iface.CanAppendFloat64() {
		return x.shared.json.appendFloat64(arr, val), true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrInt64(arr any, val int64) (any, bool) {
	if x.shared.json.iface.CanAppendInt64() {
		return x.shared.json.appendInt64(arr, val), true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrUint64(arr any, val uint64) (any, bool) {
	if x.shared.json.iface.CanAppendUint64() {
		return x.shared.json.appendUint64(arr, val), true
	}
	return arr, false
}

// ArrayBuilder
// ---
// Uses the same array types as its parent, UNLESS its immediate parent is an
// ArrayBuilder, and the jsonSupport type doesn't support nested arrays.

// p returns the parent of this array builder
func (x *ArrayBuilder[E, P]) p() (p P) {
	if x != nil && x.a != nil {
		p = x.a.(P)
	}
	return
}

// Enabled returns whether the parent is enabled.
func (x *ArrayBuilder[E, P]) Enabled() bool {
	return x.p().Enabled()
}

// As writes the array to the parent, using the provided key (if relevant), and
// returns the parent.
//
// While you may use this to add an array to an array, providing a non-empty
// key with an array as a parent will result in a [Logger.DPanic]. In such
// cases, prefer [ArrayBuilder.Add].
//
// WARNING: References to the receiver must not be retained.
func (x *ArrayBuilder[E, P]) As(key string) (p P) {
	if x.Enabled() {
		p = x.p()
		p.jsonWriteArray(key, x.b)
		refPoolPut((*refPoolItem)(x))
	}
	return
}

// Add is an alias for [ArrayBuilder.As](""), and is provided for convenience,
// to improve clarity when adding an array to an array (a case where the key
// must be an empty string).
//
// WARNING: References to the receiver must not be retained.
func (x *ArrayBuilder[E, P]) Add() (p P) {
	return x.As(``)
}

// Call is provided as a convenience, to facilitate code which uses the
// receiver explicitly, without breaking out of the fluent-style API.
// The provided fn will not be called if not [ArrayBuilder.Enabled].
func (x *ArrayBuilder[E, P]) Call(fn func(b *ArrayBuilder[E, P])) *ArrayBuilder[E, P] {
	if x.Enabled() {
		fn(x)
	}
	return x
}

func (x *ArrayBuilder[E, P]) Field(val any) *ArrayBuilder[E, P] {
	_ = x.methods().Field(x.fields(), ``, val)
	return x
}

func (x *ArrayBuilder[E, P]) Interface(val any) *ArrayBuilder[E, P] {
	_ = x.methods().Interface(x.fields(), ``, val)
	return x
}

func (x *ArrayBuilder[E, P]) Any(val any) *ArrayBuilder[E, P] { return x.Interface(val) }

func (x *ArrayBuilder[E, P]) Err(val error) *ArrayBuilder[E, P] {
	_ = x.methods().Err(x.fields(), val)
	return x
}

func (x *ArrayBuilder[E, P]) Str(val string) *ArrayBuilder[E, P] {
	_ = x.methods().Str(x.fields(), ``, val)
	return x
}

func (x *ArrayBuilder[E, P]) Int(val int) *ArrayBuilder[E, P] {
	_ = x.methods().Int(x.fields(), ``, val)
	return x
}

func (x *ArrayBuilder[E, P]) Float32(val float32) *ArrayBuilder[E, P] {
	_ = x.methods().Float32(x.fields(), ``, val)
	return x
}

func (x *ArrayBuilder[E, P]) Time(val time.Time) *ArrayBuilder[E, P] {
	_ = x.methods().Time(x.fields(), ``, val)
	return x
}

func (x *ArrayBuilder[E, P]) Dur(val time.Duration) *ArrayBuilder[E, P] {
	_ = x.methods().Dur(x.fields(), ``, val)
	return x
}

func (x *ArrayBuilder[E, P]) Base64(b []byte, enc *base64.Encoding) *ArrayBuilder[E, P] {
	_ = x.methods().Base64(x.fields(), ``, b, enc)
	return x
}

func (x *ArrayBuilder[E, P]) Bool(val bool) *ArrayBuilder[E, P] {
	_ = x.methods().Bool(x.fields(), ``, val)
	return x
}

func (x *ArrayBuilder[E, P]) Float64(val float64) *ArrayBuilder[E, P] {
	_ = x.methods().Float64(x.fields(), ``, val)
	return x
}

func (x *ArrayBuilder[E, P]) Int64(val int64) *ArrayBuilder[E, P] {
	_ = x.methods().Int64(x.fields(), ``, val)
	return x
}

func (x *ArrayBuilder[E, P]) Uint64(val uint64) *ArrayBuilder[E, P] {
	_ = x.methods().Uint64(x.fields(), ``, val)
	return x
}

func (x *ArrayBuilder[E, P]) as(key string) any {
	return x.As(key)
}

func (x *ArrayBuilder[E, P]) isArrayBuilder() *refPoolItem {
	return (*refPoolItem)(x)
}

func (x *ArrayBuilder[E, P]) jsonMustUseDefault() bool {
	switch getParentJSONType(x.a) {
	case parentJSONTypeArray:
		return !x.p().jsonSupport().CanAppendArray()
	case parentJSONTypeObject:
		return !x.p().jsonSupport().CanSetArray()
	default:
		return false
	}
}

// Root returns the root [Logger] for this instance.
func (x *ArrayBuilder[E, P]) Root() *Logger[E] {
	return x.p().Root()
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) jsonSupport() iJSONSupport[E] {
	if x.jsonMustUseDefault() {
		return defaultJSONSupport[E]{}
	}
	return x.p().jsonSupport()
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) jsonNewObject(key string) any {
	if key != `` {
		x.Root().DPanic().Log(`logiface: cannot start writing to an array with a non-empty key`)
	}
	return x.arrNewObject(x.b)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrNewObject(arr any) any {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).NewObject()
	}
	return x.p().arrNewObject(arr)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objNewObject(obj any, key string) any {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).NewObject()
	}
	return x.p().objNewObject(obj, key)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) jsonWriteObject(key string, obj any) {
	if key != `` {
		x.Root().DPanic().Log(`logiface: cannot write to an array with a non-empty key`)
	} else if !x.jsonSupport().CanAppendObject() {
		x.b = x.arrField(x.b, obj.(map[string]any))
	} else if v, ok := x.arrWriteObject(x.b, obj); !ok {
		x.Root().DPanic().Log(`logiface: implementation disallows writing an object to an array`)
	} else {
		x.b = v
	}
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objField(obj any, key string, val any) any {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).SetField(obj.(map[string]any), key, val)
	}
	return x.p().objField(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objWriteObject(obj any, key string, val any) (any, bool) {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).SetObject(obj.(map[string]any), key, val.(map[string]any)), true
	}
	return x.p().objWriteObject(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objWriteArray(obj any, key string, val any) (any, bool) {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).SetArray(obj.(map[string]any), key, val.([]any)), true
	}
	return x.p().objWriteArray(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objString(obj any, key string, val string) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objString(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objBool(obj any, key string, val bool) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objBool(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objBase64Bytes(obj any, key string, b []byte, enc *base64.Encoding) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objBase64Bytes(obj, key, b, enc)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objDuration(obj any, key string, d time.Duration) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objDuration(obj, key, d)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objError(obj any, err error) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objError(obj, err)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objInt(obj any, key string, val int) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objInt(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objFloat32(obj any, key string, val float32) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objFloat32(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objTime(obj any, key string, t time.Time) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objTime(obj, key, t)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objFloat64(obj any, key string, val float64) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objFloat64(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objInt64(obj any, key string, val int64) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objInt64(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objUint64(obj any, key string, val uint64) (any, bool) {
	if x.jsonMustUseDefault() {
		return obj, false
	}
	return x.p().objUint64(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) jsonNewArray(key string) any {
	if key != `` {
		x.Root().DPanic().Log(`logiface: cannot start writing to an array with a non-empty key`)
	}
	return x.arrNewArray(x.b)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrNewArray(arr any) any {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).NewArray()
	}
	return x.p().arrNewArray(arr)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objNewArray(obj any, key string) any {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).NewArray()
	}
	return x.p().objNewArray(obj, key)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) jsonWriteArray(key string, arr any) {
	if key != `` {
		x.Root().DPanic().Log(`logiface: cannot write to an array with a non-empty key`)
	} else if !x.jsonSupport().CanAppendArray() {
		x.b = x.arrField(x.b, arr.([]any))
	} else if v, ok := x.arrWriteArray(x.b, arr); !ok {
		x.Root().DPanic().Log(`logiface: implementation disallows writing an array to an array`)
	} else {
		x.b = v
	}
}

func (x *ArrayBuilder[E, P]) arrField(arr any, val any) any {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).AppendField(arr.([]any), val)
	}
	return x.p().arrField(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrWriteArray(arr, val any) (any, bool) {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).AppendArray(arr.([]any), val.([]any)), true
	}
	return x.p().arrWriteArray(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrWriteObject(arr, val any) (any, bool) {
	if x.jsonMustUseDefault() {
		return (defaultJSONSupport[E]{}).AppendObject(arr.([]any), val.(map[string]any)), true
	}
	return x.p().arrWriteObject(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrString(arr any, val string) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrString(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrBool(arr any, val bool) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrBool(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrBase64Bytes(arr any, b []byte, enc *base64.Encoding) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrBase64Bytes(arr, b, enc)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrDuration(arr any, d time.Duration) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrDuration(arr, d)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrError(arr any, err error) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrError(arr, err)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrInt(arr any, val int) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrInt(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrFloat32(arr any, val float32) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrFloat32(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrTime(arr any, t time.Time) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrTime(arr, t)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrFloat64(arr any, val float64) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrFloat64(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrInt64(arr any, val int64) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrInt64(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrUint64(arr any, val uint64) (any, bool) {
	if x.jsonMustUseDefault() {
		return arr, false
	}
	return x.p().arrUint64(arr, val)
}

func (x *contextFieldData[E]) array(event E) error {
	arr := x.shared.json.addStartOrNewArray(event, x.key)
	for _, v := range x.values {
		arr = v(x.shared, arr)
	}
	x.shared.json.addArray(event, x.key, arr)
	return nil
}

func (x *contextFieldData[E]) object(event E) error {
	obj := x.shared.json.addStartOrNewObject(event, x.key)
	for _, v := range x.values {
		obj = v(x.shared, obj)
	}
	x.shared.json.addObject(event, x.key, obj)
	return nil
}
