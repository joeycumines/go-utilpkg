package logiface

import (
	"unsafe"
)

type (
	Array[P interface {
		arrayParent[E]
		*V
	}, V any, E Event] refPoolItem

	arrayParent[E Event] interface {
		arrWrite(key string, arr unsafe.Pointer)
		arrAddField(arr unsafe.Pointer, val any)
	}
)

var (
	// compile time assertions

	_ arrayParent[Event] = (*Builder[Event])(nil)
	_ arrayParent[Event] = (*Array[*Builder[Event], Builder[Event], Event])(nil)
	_ arrayParent[Event] = (*Array[*Array[*Builder[Event], Builder[Event], Event], Array[*Builder[Event], Builder[Event], Event], Event])(nil)
)

func (x *Builder[E]) Array() (a *Array[*Builder[E], Builder[E], E]) {
	if x.Enabled() {
		a = (*Array[*Builder[E], Builder[E], E])(refPoolGet())
		a.a = unsafe.Pointer(x)
		a.b = x.shared.array.factory()
	}
	return
}

func (x *Builder[E]) arrWrite(key string, arr unsafe.Pointer) {
	if x.Enabled() {
		x.shared.array.write(x.Event, key, arr)
	}
}

func (x *Builder[E]) arrAddField(arr unsafe.Pointer, val any) {
	if x.Enabled() {
		x.shared.array.addField(arr, val)
	}
}

func (x *Array[P, V, E]) As(key string) (parent P) {
	if x != nil {
		parent = x.p()
		parent.arrWrite(key, x.b)
		*x = Array[P, V, E]{}
		refPoolPut((*refPoolItem)(x))
	}
	return
}

func (x *Array[P, V, E]) Field(val any) *Array[P, V, E] {
	x.arrAddField(x.b, val)
	return x
}

func (x *Array[P, V, E]) p() P {
	if x != nil {
		return (*V)(x.a)
	}
	return nil
}

func (x *Array[P, V, E]) arrWrite(key string, arr unsafe.Pointer) {
	x.p().arrWrite(key, arr)
}

func (x *Array[P, V, E]) arrAddField(arr unsafe.Pointer, val any) {
	x.p().arrAddField(arr, val)
}
