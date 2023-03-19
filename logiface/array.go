package logiface

type (
	ArrayBuilder[E Event, P ArrayParent[E]] refPoolItem

	ArrayParent[E Event] interface {
		Enabled() bool

		root() *Logger[E]

		// these methods are effectively arraySupport, but vary depending on
		// both the top-level parent, and implementation details such as
		// ArraySupport.CanAppendArray

		arrSupport() iArraySupport[E]
		arrNew() any
		arrWrite(key string, arr any)
		arrField(arr any, val any) any
		arrArray(arr, val any) (any, bool)
	}

	//lint:ignore U1000 it is actually used
	contextArray[E Event] struct {
		key    string
		shared *loggerShared[E]
		values []func(shared *loggerShared[E], arr any) any
	}

	arrayBuilderInterface interface {
		arrayBuilderData() any
		as(key string) any
	}
)

// Array starts building a new array, as a field of a given [Context],
// [Builder] or [ArrayBuilder].
//
// In Go, generic methods are not allowed to introduce cyclic dependencies on
// generic types, and cannot introduce further generic types.
func Array[E Event, P ArrayParent[E]](p P) (arr *ArrayBuilder[E, P]) {
	if p.Enabled() {
		arr = (*ArrayBuilder[E, P])(refPoolGet())
		arr.a = p
		// note: takes into account mustUseSliceArray
		arr.b = arr.arrNew()
	}
	return
}

// Context
// ---
// Uses *contextArray[E] as the array, backed by the logger's arraySupport.

//lint:ignore U1000 it is actually used
func (x *Context[E]) arrSupport() iArraySupport[E] {
	return x.logger.shared.array.iface
}

//lint:ignore U1000 it is actually used
func (x *Context[E]) arrNew() any {
	return new(contextArray[E])
}

//lint:ignore U1000 it is actually used
func (x *Context[E]) arrWrite(key string, arr any) {
	if x.Enabled() {
		a := arr.(*contextArray[E])
		a.key = key
		a.shared = x.logger.shared
		x.Modifiers = append(x.Modifiers, ModifierFunc[E](a.modifier))
	}
}

//lint:ignore U1000 it is actually used
func (x *Context[E]) arrField(arr any, val any) any {
	a := arr.(*contextArray[E])
	a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
		return shared.array.appendField(arr, val)
	})
	return arr
}

//lint:ignore U1000 it is actually used
func (x *Context[E]) arrArray(arr, val any) (any, bool) {
	if x.logger.shared.array.iface.CanAppendArray() {
		a := arr.(*contextArray[E])
		v := val.(*contextArray[E])
		a.values = append(a.values, func(shared *loggerShared[E], arr any) any {
			val := shared.array.newArray()
			for _, f := range v.values {
				val = f(shared, val)
			}
			return shared.array.appendArray(arr, val)
		})
		return arr, true
	}
	return nil, false
}

// Builder
// ---
// Uses the arraySupport directly.

//lint:ignore U1000 it is actually used
func (x *Builder[E]) arrSupport() iArraySupport[E] {
	return x.shared.array.iface
}

//lint:ignore U1000 it is actually used
func (x *Builder[E]) arrNew() any {
	return x.shared.array.newArray()
}

//lint:ignore U1000 it is actually used
func (x *Builder[E]) arrWrite(key string, arr any) {
	if x.Enabled() {
		x.shared.array.addArray(x.Event, key, arr)
	}
}

//lint:ignore U1000 it is actually used
func (x *Builder[E]) arrField(arr any, val any) any {
	return x.shared.array.appendField(arr, val)
}

//lint:ignore U1000 it is actually used
func (x *Builder[E]) arrArray(arr, val any) (any, bool) {
	if x.shared.array.iface.CanAppendArray() {
		return x.shared.array.appendArray(arr, val), true
	}
	return nil, false
}

// ArrayBuilder
// ---
// Uses the same array types as it's parent, UNLESS it's immediate parent is an
// ArrayBuilder, and the arraySupport type doesn't support nested arrays.

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
	if x.Enabled() {
		x.b = x.arrField(x.b, val)
	}
	return x
}

func (x *ArrayBuilder[E, P]) as(key string) any {
	return x.As(key)
}

// arrayBuilderData is only implemented by [ArrayBuilder], and returns the
// array data pointer
func (x *ArrayBuilder[E, P]) arrayBuilderData() any {
	return x.b
}

func (x *ArrayBuilder[E, P]) mustUseSliceArray() arrayBuilderInterface {
	if !x.p().arrSupport().CanAppendArray() {
		if v, ok := any(x.p()).(arrayBuilderInterface); ok {
			return v
		}
	}
	return nil
}

//lint:ignore U1000 it is actually used
func (x *ArrayBuilder[E, P]) root() *Logger[E] {
	return x.p().root()
}

//lint:ignore U1000 it is actually used
func (x *ArrayBuilder[E, P]) arrSupport() iArraySupport[E] {
	if x.mustUseSliceArray() != nil {
		return sliceArraySupport[E]{}
	}
	return x.p().arrSupport()
}

func (x *ArrayBuilder[E, P]) arrNew() any {
	if x.mustUseSliceArray() != nil {
		return (sliceArraySupport[E]{}).NewArray()
	}
	return x.p().arrNew()
}

//lint:ignore U1000 it is actually used
func (x *ArrayBuilder[E, P]) arrWrite(key string, arr any) {
	if key != `` {
		x.root().DPanic().Log(`logiface: cannot write to an array with a non-empty key`)
	} else if p := x.mustUseSliceArray(); p != nil {
		x.b = x.p().arrField(p.arrayBuilderData(), arr.([]any))
	} else if v, ok := x.p().arrArray(x.b, arr); !ok {
		x.root().DPanic().Log(`logiface: implementation disallows writing an array to an array`)
	} else {
		x.b = v
	}
}

func (x *ArrayBuilder[E, P]) arrField(arr any, val any) any {
	if x.mustUseSliceArray() != nil {
		return (sliceArraySupport[E]{}).AppendField(arr.([]any), val)
	} else {
		return x.p().arrField(arr, val)
	}
}

//lint:ignore U1000 it is actually used
func (x *ArrayBuilder[E, P]) arrArray(arr, val any) (any, bool) {
	if x.mustUseSliceArray() != nil {
		return (sliceArraySupport[E]{}).AppendArray(arr.([]any), val.([]any)), true
	}
	return x.p().arrArray(arr, val)
}

func (x *contextArray[E]) modifier(event E) error {
	arr := x.shared.array.newArray()
	for _, v := range x.values {
		arr = v(x.shared, arr)
	}
	x.shared.array.addArray(event, x.key, arr)
	return nil
}
