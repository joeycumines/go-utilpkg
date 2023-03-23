package logiface

const (
	_ parentJSONType = iota
	parentJSONTypeArray
	parentJSONTypeObject
)

type (
	// Chain wraps a [Parent] implementation in order to support nested
	// data structures.
	Chain[E Event, P comparable] refPoolItem

	// Parent models one of the fluent-style builder implementations, including
	// [Builder], [Context], [ArrayBuilder], and others.
	Parent[E Event] interface {
		Enabled() bool

		root() *Logger[E]
		jsonSupport() iJSONSupport[E]

		// these methods are effectively jsonSupport, but vary depending on
		// both the top-level parent, and implementation details such as
		// JSONSupport.CanAppendArray
		//
		// WARNING: The guarded methods must always return the input arr, even
		// when false, in order to avoid allocs within the arrayFields methods.

		jsonObject(key string, obj any)
		jsonArray(key string, arr any)

		objNew() any
		objField(obj any, key string, val any) any
		objObject(obj any, key string, val any) (any, bool)
		objArray(obj any, key string, val any) (any, bool)

		arrNew() any
		arrField(arr any, val any) any
		arrArray(arr, val any) (any, bool)
		arrObject(arr, val any) (any, bool)
		arrStr(arr any, val string) (any, bool)
		arrBool(arr any, val bool) (any, bool)
	}

	chainInterface interface {
		isChain() *refPoolItem
	}

	chainInterfaceFull[E Event] interface {
		chainInterface
		newChain(current Parent[E]) any
	}

	parentJSONType int
)

func (x *Context[E]) Array() *ArrayBuilder[E, *Chain[E, *Context[E]]] {
	if x.Enabled() {
		return Array[E](newChainParent[E](x))
	}
	return nil
}

func (x *Context[E]) Object() *ObjectBuilder[E, *Chain[E, *Context[E]]] {
	if x.Enabled() {
		return Object[E](newChainParent[E](x))
	}
	return nil
}

func (x *Builder[E]) Array() *ArrayBuilder[E, *Chain[E, *Builder[E]]] {
	if x.Enabled() {
		return Array[E](newChainParent[E](x))
	}
	return nil
}

func (x *Builder[E]) Object() *ObjectBuilder[E, *Chain[E, *Builder[E]]] {
	if x.Enabled() {
		return Object[E](newChainParent[E](x))
	}
	return nil
}

// Array attempts to initialize a sub-array, which will succeed only if the
// parent is [Chain], otherwise performing [Logger.DPanic] (returning nil
// if in a production configuration).
func (x *ArrayBuilder[E, P]) Array() *ArrayBuilder[E, P] {
	if x.Enabled() {
		if c, ok := any(x.p()).(chainInterfaceFull[E]); !ok {
			x.root().DPanic().Log(`logiface: cannot chain a sub-array from a non-chain parent`)
		} else {
			return Array[E](c.newChain(x).(P))
		}
	}
	return nil
}

// Object attempts to initialize a sub-object, which will succeed only if the
// receiver is [Chain], otherwise performing [Logger.DPanic] (returning nil
// if in a production configuration).
func (x *ArrayBuilder[E, P]) Object() *ObjectBuilder[E, P] {
	if x.Enabled() {
		if c, ok := any(x.p()).(chainInterfaceFull[E]); !ok {
			x.root().DPanic().Log(`logiface: cannot chain a sub-object from a non-chain parent`)
		} else {
			return Object[E](c.newChain(x).(P))
		}
	}
	return nil
}

// Array attempts to initialize a sub-array, which will succeed only if the
// parent is [Chain], otherwise performing [Logger.DPanic] (returning nil
// if in a production configuration).
func (x *ObjectBuilder[E, P]) Array() *ArrayBuilder[E, P] {
	if x.Enabled() {
		if c, ok := any(x.p()).(chainInterfaceFull[E]); !ok {
			x.root().DPanic().Log(`logiface: cannot chain a sub-array from a non-chain parent`)
		} else {
			return Array[E](c.newChain(x).(P))
		}
	}
	return nil
}

// Object attempts to initialize a sub-object, which will succeed only if the
// parent is [Chain], otherwise performing [Logger.DPanic] (returning nil
// if in a production configuration).
func (x *ObjectBuilder[E, P]) Object() *ObjectBuilder[E, P] {
	if x.Enabled() {
		if c, ok := any(x.p()).(chainInterfaceFull[E]); !ok {
			x.root().DPanic().Log(`logiface: cannot chain a sub-object from a non-chain parent`)
		} else {
			return Object[E](c.newChain(x).(P))
		}
	}
	return nil
}

func (x *Chain[E, P]) Array() *ArrayBuilder[E, *Chain[E, P]] {
	if x.Enabled() {
		return Array[E](x)
	}
	return nil
}

func (x *Chain[E, P]) Object() *ObjectBuilder[E, *Chain[E, P]] {
	if x.Enabled() {
		return Object[E](x)
	}
	return nil
}

// CurArray returns the current array, calls [Logger.DPanic] if the current
// value is not an array, and returns nil if in a production configuration.
//
// Allows adding fields on the same level as nested object(s) and/or array(s).
func (x *Chain[E, P]) CurArray() *ArrayBuilder[E, *Chain[E, P]] {
	if x.Enabled() {
		if current := x.current(); current != nil {
			if current, ok := current.(*ArrayBuilder[E, *Chain[E, P]]); ok {
				return current
			}
			x.root().DPanic().Log(`logiface: cannot access a non-array as an array`)
		}
	}
	return nil
}

// CurObject returns the current object, calls [Logger.DPanic] if the current
// value is not an array, and returns nil if in a production configuration.
//
// Allows adding fields on the same level as nested object(s) and/or array(s).
func (x *Chain[E, P]) CurObject() *ObjectBuilder[E, *Chain[E, P]] {
	if x.Enabled() {
		if current := x.current(); current != nil {
			if current, ok := current.(*ObjectBuilder[E, *Chain[E, P]]); ok {
				return current
			}
			x.root().DPanic().Log(`logiface: cannot access a non-object as an object`)
		}
	}
	return nil
}

func (x *Chain[E, P]) As(key string) *Chain[E, P] {
	if current := x.current(); current != nil {
		switch current := current.(type) {
		case arrayBuilderInterface:
			if current, ok := current.as(key).(*Chain[E, P]); ok && current != nil && current.a == x.a {
				x.setCurrent(current.current())
			} else {
				x.setCurrent(nil)
			}
		case objectBuilderInterface:
			if current, ok := current.as(key).(*Chain[E, P]); ok && current != nil && current.a == x.a {
				x.setCurrent(current.current())
			} else {
				x.setCurrent(nil)
			}
		default:
			x.setCurrent(nil)
		}
		if x.current() == nil {
			x.root().DPanic().Log(`logiface: chain as failed: called on invalid or terminated parent`)
		}
	}
	return x
}

func (x *Chain[E, P]) Add() *Chain[E, P] {
	return x.As(``)
}

// End jumps out of chain, returning the parent, and returning the receiver to
// the pool.
func (x *Chain[E, P]) End() (p P) {
	if x != nil {
		if x.a != nil {
			p = x.a.(P)
		}
		refPoolPut((*refPoolItem)(x))
	}
	return
}

func (x *Chain[E, P]) Enabled() bool {
	if current := x.current(); current != nil {
		return current.Enabled()
	}
	return false
}

func (x *Chain[E, P]) root() *Logger[E] {
	if current := x.current(); current != nil {
		return current.root()
	}
	return nil
}

//lint:ignore U1000 it is or will be used
func (x *Chain[E, P]) jsonSupport() iJSONSupport[E] {
	return x.current().jsonSupport()
}

//lint:ignore U1000 it is or will be used
func (x *Chain[E, P]) objNew() any {
	return x.current().objNew()
}

//lint:ignore U1000 it is or will be used
func (x *Chain[E, P]) jsonObject(key string, obj any) {
	x.current().jsonObject(key, obj)
}

//lint:ignore U1000 it is or will be used
func (x *Chain[E, P]) objField(obj any, key string, val any) any {
	return x.current().objField(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *Chain[E, P]) objObject(obj any, key string, val any) (any, bool) {
	return x.current().objObject(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *Chain[E, P]) objArray(obj any, key string, val any) (any, bool) {
	return x.current().objArray(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *Chain[E, P]) arrNew() any {
	return x.current().arrNew()
}

//lint:ignore U1000 it is or will be used
func (x *Chain[E, P]) jsonArray(key string, arr any) {
	x.current().jsonArray(key, arr)
}

//lint:ignore U1000 it is or will be used
func (x *Chain[E, P]) arrField(arr any, val any) any {
	return x.current().arrField(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *Chain[E, P]) arrArray(arr, val any) (any, bool) {
	return x.current().arrArray(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *Chain[E, P]) arrObject(arr, val any) (any, bool) {
	return x.current().arrObject(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *Chain[E, P]) arrStr(arr any, val string) (any, bool) {
	return x.current().arrStr(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *Chain[E, P]) arrBool(arr any, val bool) (any, bool) {
	return x.current().arrBool(arr, val)
}

func (x *Chain[E, P]) current() (p Parent[E]) {
	if x != nil {
		p, _ = x.b.(Parent[E])
	}
	return
}

func (x *Chain[E, P]) setCurrent(p Parent[E]) {
	x.b = p
}

//lint:ignore U1000 it is or will be used
func (x *Chain[E, P]) isChain() *refPoolItem {
	return (*refPoolItem)(x)
}

//lint:ignore U1000 it is or will be used
func (x *Chain[E, P]) newChain(current Parent[E]) any {
	return newChain[E](x.a.(P), current)
}

func newChain[E Event, P comparable](parent P, current Parent[E]) (c *Chain[E, P]) {
	c = (*Chain[E, P])(refPoolGet())
	c.a = parent
	c.b = current
	return
}

func newChainParent[E Event, P interface {
	Parent[E]
	comparable
}](parent P) *Chain[E, P] {
	return newChain[E, P](parent, parent)
}

func getParentJSONType(p any) parentJSONType {
	switch p := p.(type) {
	case arrayBuilderInterface:
		return parentJSONTypeArray
	case objectBuilderInterface:
		return parentJSONTypeObject
	case chainInterface:
		return getParentJSONType(p.isChain().b)
	default:
		return 0
	}
}
