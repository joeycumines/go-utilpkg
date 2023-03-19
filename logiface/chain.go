package logiface

type (
	// Chainable wraps a [Parent] implementation in order to support nested
	// data structures.
	Chainable[E Event, P comparable] refPoolItem

	Parent[E Event] interface {
		ArrayParent[E]
	}

	chainableInterface[E Event] interface {
		newChainable(current Parent[E]) any
	}
)

func (x *Context[E]) Array() *ArrayBuilder[E, *Chainable[E, *Context[E]]] {
	if x.Enabled() {
		return Array[E](newChainableParent[E](x))
	}
	return nil
}

func (x *Builder[E]) Array() *ArrayBuilder[E, *Chainable[E, *Builder[E]]] {
	if x.Enabled() {
		return Array[E](newChainableParent[E](x))
	}
	return nil
}

// Array attempts to initialize a sub-array, which will succeed only if the
// receiver is [Chainable], otherwise performing [Logger.DPanic] (returning nil
// if in a production configuration).
func (x *ArrayBuilder[E, P]) Array() *ArrayBuilder[E, P] {
	if x.Enabled() {
		if c, ok := any(x.p()).(chainableInterface[E]); !ok {
			x.root().DPanic().Log(`logiface: cannot chain a sub-array from a non-chainable parent`)
		} else {
			return Array[E](c.newChainable(x).(P))
		}
	}
	return nil
}

func (x *Chainable[E, P]) Array() *ArrayBuilder[E, *Chainable[E, P]] {
	if x.Enabled() {
		return Array[E](x)
	}
	return nil
}

func (x *Chainable[E, P]) As(key string) *Chainable[E, P] {
	if current := x.current(); current != nil {
		switch current := current.(type) {
		case arrayBuilderInterface:
			if current, ok := current.as(key).(*Chainable[E, P]); ok && current != nil && current.a == x.a {
				x.setCurrent(current.current())
			} else {
				x.setCurrent(nil)
			}
		default:
			x.setCurrent(nil)
		}
		if x.current() == nil {
			x.root().DPanic().Log(`logiface: chainable as failed: called on invalid or terminated parent`)
		}
	}
	return x
}

func (x *Chainable[E, P]) Add() *Chainable[E, P] {
	return x.As(``)
}

// Parent jumps out of chain, returning the parent, and returning the receiver
// to the pool.
func (x *Chainable[E, P]) Parent() (p P) {
	if x != nil {
		if x.a != nil {
			p = x.a.(P)
		}
		refPoolPut((*refPoolItem)(x))
	}
	return
}

func (x *Chainable[E, P]) Enabled() bool {
	if current := x.current(); current != nil {
		return current.Enabled()
	}
	return false
}

func (x *Chainable[E, P]) root() *Logger[E] {
	if current := x.current(); current != nil {
		return current.root()
	}
	return nil
}

//lint:ignore U1000 it is actually used
func (x *Chainable[E, P]) arrSupport() iArraySupport[E] {
	return x.current().arrSupport()
}

//lint:ignore U1000 it is actually used
func (x *Chainable[E, P]) arrNew() any {
	return x.current().arrNew()
}

//lint:ignore U1000 it is actually used
func (x *Chainable[E, P]) arrWrite(key string, arr any) {
	x.current().arrWrite(key, arr)
}

//lint:ignore U1000 it is actually used
func (x *Chainable[E, P]) arrField(arr any, val any) any {
	return x.current().arrField(arr, val)
}

//lint:ignore U1000 it is actually used
func (x *Chainable[E, P]) arrArray(arr, val any) (any, bool) {
	return x.current().arrArray(arr, val)
}

func (x *Chainable[E, P]) current() (p Parent[E]) {
	if x != nil {
		p, _ = x.b.(Parent[E])
	}
	return
}

func (x *Chainable[E, P]) setCurrent(p Parent[E]) {
	x.b = p
}

// newChainable is only implemented by [Chainable]
//
//lint:ignore U1000 it is actually used
func (x *Chainable[E, P]) newChainable(current Parent[E]) any {
	return newChainable[E](x.a.(P), current)
}

func newChainable[E Event, P comparable](parent P, current Parent[E]) (c *Chainable[E, P]) {
	c = (*Chainable[E, P])(refPoolGet())
	c.a = parent
	c.b = current
	return
}

func newChainableParent[E Event, P interface {
	Parent[E]
	comparable
}](parent P) *Chainable[E, P] {
	return newChainable[E, P](parent, parent)
}
