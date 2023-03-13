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

	contextArray[E Event] struct {
		key    string
		shared *loggerShared[E]
		values []func(shared *loggerShared[E], arr unsafe.Pointer)
	}
)

var (
	// compile time assertions

	_ arrayParent[Event] = (*Context[Event])(nil)
	_ arrayParent[Event] = (*Builder[Event])(nil)
	_ arrayParent[Event] = (*Array[*Builder[Event], Builder[Event], Event])(nil)
	_ arrayParent[Event] = (*Array[*Array[*Builder[Event], Builder[Event], Event], Array[*Builder[Event], Builder[Event], Event], Event])(nil)
)

func (x *Context[E]) Array() (arr *Array[*Context[E], Context[E], E]) {
	if x.Enabled() {
		arr = (*Array[*Context[E], Context[E], E])(refPoolGet())
		arr.a = unsafe.Pointer(x)
		arr.b = unsafe.Pointer(new(contextArray[E]))
	}
	return
}

func (x *Context[E]) arrWrite(key string, arr unsafe.Pointer) {
	if x.Enabled() {
		a := (*contextArray[E])(arr)
		a.key = key
		a.shared = x.logger.shared
		x.Modifiers = append(x.Modifiers, ModifierFunc[E](a.modifier))
	}
}

func (x *Context[E]) arrAddField(arr unsafe.Pointer, val any) {
	if x.Enabled() {
		a := (*contextArray[E])(arr)
		a.values = append(a.values, func(shared *loggerShared[E], arr unsafe.Pointer) {
			shared.array.addField(arr, val)
		})
	}
}

func (x *Builder[E]) Array() (arr *Array[*Builder[E], Builder[E], E]) {
	if x.Enabled() {
		arr = (*Array[*Builder[E], Builder[E], E])(refPoolGet())
		arr.a = unsafe.Pointer(x)
		arr.b = x.shared.array.factory()
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

func (x *Array[P, V, E]) As(key string) (p P) {
	if x != nil {
		p = x.p()
		p.arrWrite(key, x.b)
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

func (x *contextArray[E]) modifier(event E) error {
	arr := x.shared.array.factory()
	for _, v := range x.values {
		v(x.shared, arr)
	}
	x.shared.array.write(event, x.key, arr)
	return nil
}
