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
func Array[E Event, P Parent[E]](p P) (arr *ArrayBuilder[E, P]) {
	if p.Enabled() {
		arr = (*ArrayBuilder[E, P])(refPoolGet())
		arr.a = p
		// note: takes into account mustUseDefaultJSONSupport
		arr.b = arr.arrNew()
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
func (x *Context[E]) arrNew() any {
	return new(contextFieldData[E])
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) arrWrite(key string, arr any) {
	a := arr.(*contextFieldData[E])
	a.key = key
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
func (x *Context[E]) arrArray(arr, val any) (any, bool) {
	if x.logger.shared.json.iface.CanAppendArray() {
		a := arr.(*contextFieldData[E])
		v := val.(*contextFieldData[E])
		a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
			val := shared.json.newArray()
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
func (x *Context[E]) arrStr(arr any, val string) (any, bool) {
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

// Builder
// ---
// Uses the jsonSupport directly.

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) jsonSupport() iJSONSupport[E] {
	return x.shared.json.iface
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrNew() any {
	return x.shared.json.newArray()
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrWrite(key string, arr any) {
	x.shared.json.addArray(x.Event, key, arr)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrField(arr any, val any) any {
	return x.shared.json.appendField(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrArray(arr, val any) (any, bool) {
	if x.shared.json.iface.CanAppendArray() {
		return x.shared.json.appendArray(arr, val), true
	}
	return arr, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) arrStr(arr any, val string) (any, bool) {
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
		p.arrWrite(key, x.b)
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
func (x *ArrayBuilder[E, P]) Call(fn func(a *ArrayBuilder[E, P])) *ArrayBuilder[E, P] {
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

func (x *ArrayBuilder[E, P]) mustUseDefaultJSONSupport() (ok bool) {
	if !x.p().jsonSupport().CanAppendArray() {
		switch x.a.(type) {
		case arrayBuilderInterface:
			return true
		case chainInterface[E]:
			_, ok = x.a.(chainInterface[E]).isChain().b.(arrayBuilderInterface)
			return ok
		}
	}
	return false
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) root() *Logger[E] {
	return x.p().root()
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) jsonSupport() iJSONSupport[E] {
	if x.mustUseDefaultJSONSupport() {
		return defaultJSONSupport[E]{}
	}
	return x.p().jsonSupport()
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objNew() any {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).NewObject()
	}
	return x.p().objNew()
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objWrite(key string, obj any) {
	x.root().DPanic().Log(`logiface: array builder unexpectedly called objWrite`)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objField(obj any, key string, val any) any {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).SetField(obj.(map[string]any), key, val)
	}
	return x.p().objField(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) objObject(obj any, key string, val any) (any, bool) {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).SetObject(obj.(map[string]any), key, val.(map[string]any)), true
	}
	return x.p().objObject(obj, key, val)
}

func (x *ArrayBuilder[E, P]) arrNew() any {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).NewArray()
	}
	return x.p().arrNew()
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrWrite(key string, arr any) {
	if key != `` {
		x.root().DPanic().Log(`logiface: cannot write to an array with a non-empty key`)
	} else if !x.jsonSupport().CanAppendArray() {
		x.b = x.arrField(x.b, arr.([]any))
	} else if v, ok := x.arrArray(x.b, arr); !ok {
		x.root().DPanic().Log(`logiface: implementation disallows writing an array to an array`)
	} else {
		x.b = v
	}
}

func (x *ArrayBuilder[E, P]) arrField(arr any, val any) any {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).AppendField(arr.([]any), val)
	}
	return x.p().arrField(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrArray(arr, val any) (any, bool) {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).AppendArray(arr.([]any), val.([]any)), true
	}
	return x.p().arrArray(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrStr(arr any, val string) (any, bool) {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).AppendString(arr.([]any), val), true
	}
	return x.p().arrStr(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ArrayBuilder[E, P]) arrBool(arr any, val bool) (any, bool) {
	if x.mustUseDefaultJSONSupport() {
		return (defaultJSONSupport[E]{}).AppendBool(arr.([]any), val), true
	}
	return x.p().arrBool(arr, val)
}

func (x *contextFieldData[E]) array(event E) error {
	arr := x.shared.json.newArray()
	for _, v := range x.values {
		arr = v(x.shared, arr)
	}
	x.shared.json.addArray(event, x.key, arr)
	return nil
}

func (x *contextFieldData[E]) object(event E) error {
	obj := x.shared.json.newObject()
	for _, v := range x.values {
		obj = v(x.shared, obj)
	}
	x.shared.json.addObject(event, x.key, obj)
	return nil
}
