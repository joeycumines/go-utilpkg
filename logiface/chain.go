package logiface

type (
	// Chain wraps a [Parent] implementation in order to support nested
	// data structures.
	Chain[E Event, P comparable] refPoolItem

	Parent[E Event] interface {
		ArrayParent[E]
	}

	chainInterface[E Event] interface {
		isChain() *refPoolItem
		newChain(current Parent[E]) any
	}
)

func (x *Context[E]) Array() *ArrayBuilder[E, *Chain[E, *Context[E]]] {
	if x.Enabled() {
		return Array[E](newChainParent[E](x))
	}
	return nil
}

func (x *Builder[E]) Array() *ArrayBuilder[E, *Chain[E, *Builder[E]]] {
	if x.Enabled() {
		return Array[E](newChainParent[E](x))
	}
	return nil
}

// Array attempts to initialize a sub-array, which will succeed only if the
// receiver is [Chain], otherwise performing [Logger.DPanic] (returning nil
// if in a production configuration).
func (x *ArrayBuilder[E, P]) Array() *ArrayBuilder[E, P] {
	if x.Enabled() {
		if c, ok := any(x.p()).(chainInterface[E]); !ok {
			x.root().DPanic().Log(`logiface: cannot chain a sub-array from a non-chain parent`)
		} else {
			return Array[E](c.newChain(x).(P))
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

func (x *Chain[E, P]) As(key string) *Chain[E, P] {
	if current := x.current(); current != nil {
		switch current := current.(type) {
		case arrayBuilderInterface:
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

//lint:ignore U1000 it is actually used
func (x *Chain[E, P]) arrSupport() iArraySupport[E] {
	return x.current().arrSupport()
}

//lint:ignore U1000 it is actually used
func (x *Chain[E, P]) arrNew() any {
	return x.current().arrNew()
}

//lint:ignore U1000 it is actually used
func (x *Chain[E, P]) arrWrite(key string, arr any) {
	x.current().arrWrite(key, arr)
}

//lint:ignore U1000 it is actually used
func (x *Chain[E, P]) arrField(arr any, val any) any {
	return x.current().arrField(arr, val)
}

//lint:ignore U1000 it is actually used
func (x *Chain[E, P]) arrArray(arr, val any) (any, bool) {
	return x.current().arrArray(arr, val)
}

//lint:ignore U1000 it is actually used
func (x *Chain[E, P]) arrStr(arr any, val string) (any, bool) {
	return x.current().arrStr(arr, val)
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

//lint:ignore U1000 it is actually used
func (x *Chain[E, P]) isChain() *refPoolItem {
	return (*refPoolItem)(x)
}

//lint:ignore U1000 it is actually used
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
