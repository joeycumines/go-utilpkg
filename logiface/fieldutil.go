package logiface

import (
	"fmt"
)

// MapFields is a helper function that calls [Builder.Field] or
// [Context.Field], for every element of a map, which must have keys with an
// underlying type of string. The field order is not stable, as it iterates on
// the map, without sorting the keys.
//
// WARNING: The behavior of the [Context.Field] and [Builder.Field] methods may
// change without notice, to facilitate the addition of new field types.
//
// This function is kept to avoid breaking backwards compatibility.
// Prefer using the MapFields method on concrete builder types where available.
func MapFields[K ~string, V any, R interface {
	Enabled() bool
	Field(key string, val any) R
}](r R, m map[K]V) R {
	if r.Enabled() {
		for k, v := range m {
			r = r.Field(string(k), v)
		}
	}
	return r
}

// ArgFields is a helper function that calls [Builder.Field] or
// [Context.Field], for every element of a (varargs) slice.
// If provided, f will be used to ensure that each key is a string, otherwise,
// if not provided, each key will be converted to a string, using fmt.Sprint.
// Passing an odd number of keys will set the last value to any(nil).
//
// WARNING: The behavior of the [Context.Field] and [Builder.Field] methods may
// change without notice, to facilitate the addition of new field types.
//
// This function is kept to avoid breaking backwards compatibility.
// Prefer using the ArgFields method on concrete builder types where available.
func ArgFields[E any, R interface {
	Enabled() bool
	Field(key string, val any) R
}](r R, f func(key E) (string, bool), l ...E) R {
	if r.Enabled() && len(l) != 0 {
		var (
			key string
			ok  bool
		)
		for i := 0; i < len(l); i += 2 {
			key, ok = argFieldKeyConverter(f, l[i])
			if !ok {
				continue
			}
			if i+1 == len(l) {
				r = r.Field(key, nil)
				break
			}
			r = r.Field(key, l[i+1])
		}
	}
	return r
}

func argFieldKeyConverter[E any](f func(key E) (string, bool), key E) (string, bool) {
	if f == nil {
		return fmt.Sprint(key), true
	}
	return f(key)
}

// SliceArray adds a slice as an array field, to the given [Builder],
// [Context], [ArrayBuilder], [ObjectBuilder], or [Chain].
//
// Note that the key must be empty if the parent parameter is an array.
//
// This function is kept to avoid breaking backwards compatibility.
// Prefer using the Slice method on concrete builder types where available.
func SliceArray[E Event, V any, P Parent[E]](parent P, key string, slice []V) P {
	return boundSlice(parent, key, slice)
}

// MapObject adds a map as an object field, to the given [Builder],
// [Context], [ArrayBuilder], [ObjectBuilder], or [Chain].
//
// Note that the key must be empty if the parent parameter is an array.
//
// This function is kept to avoid breaking backwards compatibility.
// Prefer using the Map method on concrete builder types where available.
func MapObject[E Event, K ~string, V any, P Parent[E]](parent P, key string, m map[K]V) P {
	return boundMap(parent, key, m)
}

// boundSlice is the shared implementation for Slice[V] on every concrete Parent.
// It preserves Enabled() guard, DPanic on non-empty key when parent is an array
// (delegated to ArrayWithKey → jsonNewArray → jsonMustUseDefault branch), and
// pooling via As(key) → refPoolPut. No retention after return.
func boundSlice[E Event, P Parent[E], V any](parent P, key string, slice []V) P {
	if !parent.Enabled() {
		return parent
	}
	b := ArrayWithKey[E](parent, key)
	if b == nil {
		return parent
	}
	for _, v := range slice {
		b.Field(v)
	}
	b.As(key)
	return parent
}

// boundMap is the shared implementation for Map[K,V] on every concrete Parent.
func boundMap[E Event, P Parent[E], K ~string, V any](parent P, key string, m map[K]V) P {
	if !parent.Enabled() {
		return parent
	}
	b := ObjectWithKey[E](parent, key)
	if b == nil {
		return parent
	}
	for k, v := range m {
		b.Field(string(k), v)
	}
	b.As(key)
	return parent
}

// Builder fluent generic helpers.

// Slice adds a slice as an array field. V is inferred from the slice literal.
// The key must be empty if the parent is an array (otherwise DPanic).
func (x *Builder[E]) Slice[V any](key string, slice []V) *Builder[E] {
	return boundSlice(x, key, slice)
}

// Map adds a map as an object field. K and V are inferred from the map literal.
// The key must be empty if the parent is an array (otherwise DPanic).
func (x *Builder[E]) Map[K ~string, V any](key string, m map[K]V) *Builder[E] {
	return boundMap(x, key, m)
}

// MapFields adds each entry of m as a field via [Builder.Field].
func (x *Builder[E]) MapFields[K ~string, V any](m map[K]V) *Builder[E] {
	return MapFields(x, m)
}

// ArgFields adds each key/value pair from args as a field via [Builder.Field].
// If f is nil, keys are converted via fmt.Sprint; if f returns !ok the pair is skipped.
// An odd number of args sets the last value to nil, matching [ArgFields] semantics.
func (x *Builder[E]) ArgFields[E2 any](f func(E2) (string, bool), args ...E2) *Builder[E] {
	return ArgFields(x, f, args...)
}

// Context fluent generic helpers (deferred via Modifiers).

// Slice adds a slice as an array field. For Context the work is deferred via Modifiers
// and applied lazily when the cloned Logger is used, preserving Context.Modifiers semantics.
func (x *Context[E]) Slice[V any](key string, slice []V) *Context[E] {
	return boundSlice(x, key, slice)
}

// Map adds a map as an object field, deferred for Context.
func (x *Context[E]) Map[K ~string, V any](key string, m map[K]V) *Context[E] {
	return boundMap(x, key, m)
}

// MapFields adds each entry of m as a field via [Context.Field] (deferred).
func (x *Context[E]) MapFields[K ~string, V any](m map[K]V) *Context[E] {
	return MapFields(x, m)
}

// ArgFields adds each key/value pair from args as a field via [Context.Field] (deferred).
func (x *Context[E]) ArgFields[E2 any](f func(E2) (string, bool), args ...E2) *Context[E] {
	return ArgFields(x, f, args...)
}

// Chain fluent generic helpers.

func (x *Chain[E, P]) Slice[V any](key string, slice []V) *Chain[E, P] {
	return boundSlice(x, key, slice)
}

func (x *Chain[E, P]) Map[K ~string, V any](key string, m map[K]V) *Chain[E, P] {
	return boundMap(x, key, m)
}

// MapFields adds each entry of m as a field to the current object in the chain.
// If the chain is not currently building an object (e.g. it is an array or nil),
// it logs a DPanic and is otherwise a no-op.
func (x *Chain[E, P]) MapFields[K ~string, V any](m map[K]V) *Chain[E, P] {
	if !x.Enabled() || len(m) == 0 {
		return x
	}
	switch v := any(x.current()).(type) {
	case *Builder[E]:
		MapFields(v, m)
	case *Context[E]:
		MapFields(v, m)
	case *ObjectBuilder[E, *Chain[E, P]]:
		MapFields(v, m)
	default:
		if root := x.Root(); root != nil {
			root.DPanic().Log(`logiface: cannot add object fields while not building an object`)
		}
	}
	return x
}

// ArgFields adds each key/value pair from args as a field to the current object in the chain.
// Semantics match [ArgFields] and [Builder.ArgFields]: nil f uses fmt.Sprint, !ok skips, odd length sets last value to nil.
// If the chain is not currently building an object, it logs a DPanic and is otherwise a no-op.
func (x *Chain[E, P]) ArgFields[E2 any](f func(E2) (string, bool), args ...E2) *Chain[E, P] {
	if !x.Enabled() || len(args) == 0 {
		return x
	}
	switch v := any(x.current()).(type) {
	case *Builder[E]:
		ArgFields(v, f, args...)
	case *Context[E]:
		ArgFields(v, f, args...)
	case *ObjectBuilder[E, *Chain[E, P]]:
		ArgFields(v, f, args...)
	default:
		if root := x.Root(); root != nil {
			root.DPanic().Log(`logiface: cannot add object fields while not building an object`)
		}
	}
	return x
}

// ArrayBuilder fluent generic helpers.
//
// These do not use boundSlice/boundMap to avoid a vet instantiation cycle:
// ArrayBuilder[E,P] itself satisfies Parent[E], so calling boundSlice[E,*ArrayBuilder[E,P],V]
// which calls ArrayWithKey[E,*ArrayBuilder[E,P]] forms a cycle (go vet: instantiation cycle).
// Instead they delegate via nested builders and Field (mirroring
// ArrayFunc/ObjectFunc), which preserves Enabled guards, DPanic on
// non-empty key when parent is an array, typed field encoding (e.g.
// int64 as string under the default implementation), and JSONSupport
// branching (including Can* checks) without introducing a generic helper
// instantiation cycle.

func (x *ArrayBuilder[E, P]) Slice[V any](key string, slice []V) *ArrayBuilder[E, P] {
	if !x.Enabled() {
		return x
	}
	if c, ok := any(x.p()).(chainInterfaceFull[E]); !ok {
		x.Root().DPanic().Log(`logiface: cannot chain a sub-array from a non-chain parent`)
		return x
	} else if b := Array[E](c.newChain(x).(P)); b != nil {
		for _, v := range slice {
			b.Field(v)
		}
		endChain(b.As(key))
	}
	return x
}

func (x *ArrayBuilder[E, P]) Map[K ~string, V any](key string, m map[K]V) *ArrayBuilder[E, P] {
	if !x.Enabled() {
		return x
	}
	if c, ok := any(x.p()).(chainInterfaceFull[E]); !ok {
		x.Root().DPanic().Log(`logiface: cannot chain a sub-object from a non-chain parent`)
		return x
	} else if b := Object[E](c.newChain(x).(P)); b != nil {
		for k, v := range m {
			b.Field(string(k), v)
		}
		endChain(b.As(key))
	}
	return x
}

// MapFields on an array builder is not a valid JSON operation (arrays have no
// keyed fields). It always DPanic's in debug builds and is otherwise a no-op.
// Prefer building an object ([ArrayBuilder.Object])
// or using a map field via [ArrayBuilder.Map].
func (x *ArrayBuilder[E, P]) MapFields[K ~string, V any](m map[K]V) *ArrayBuilder[E, P] {
	if !x.Enabled() || len(m) == 0 {
		return x
	}
	if root := x.Root(); root != nil {
		root.DPanic().Log(`logiface: cannot add object fields to an array builder`)
	}
	return x
}

// ArgFields on an array builder is not a valid JSON operation. See [ArrayBuilder.MapFields].
func (x *ArrayBuilder[E, P]) ArgFields[E2 any](f func(E2) (string, bool), args ...E2) *ArrayBuilder[E, P] {
	if !x.Enabled() || len(args) == 0 {
		return x
	}
	if root := x.Root(); root != nil {
		root.DPanic().Log(`logiface: cannot add object fields to an array builder`)
	}
	return x
}

// ObjectBuilder fluent generic helpers.

func (x *ObjectBuilder[E, P]) Slice[V any](key string, slice []V) *ObjectBuilder[E, P] {
	if !x.Enabled() {
		return x
	}
	b := x.arrayWithKey(key)
	if b == nil {
		return x
	}
	for _, v := range slice {
		b.Field(v)
	}
	endChain(b.As(key))
	return x
}

func (x *ObjectBuilder[E, P]) Map[K ~string, V any](key string, m map[K]V) *ObjectBuilder[E, P] {
	if !x.Enabled() {
		return x
	}
	b := x.objectWithKey(key)
	if b == nil {
		return x
	}
	for k, v := range m {
		b.Field(string(k), v)
	}
	endChain(b.As(key))
	return x
}

func (x *ObjectBuilder[E, P]) MapFields[K ~string, V any](m map[K]V) *ObjectBuilder[E, P] {
	return MapFields(x, m)
}

func (x *ObjectBuilder[E, P]) ArgFields[E2 any](f func(E2) (string, bool), args ...E2) *ObjectBuilder[E, P] {
	return ArgFields(x, f, args...)
}
