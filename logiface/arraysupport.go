package logiface

import (
	"unsafe"
)

type (
	ArraySupport[E Event, A any] interface {
		NewArray() *A
		WriteArray(evt E, key string, arr *A)
		AddArrayField(arr *A, val any)
		mustEmbedUnimplementedArraySupport()
	}

	// arraySupport is available via loggerShared.array, and models an external
	// array builder implementation.
	arraySupport[E Event] struct {
		factory  func() unsafe.Pointer
		write    func(evt E, key string, arr unsafe.Pointer)
		addField func(arr unsafe.Pointer, val any)
	}

	UnimplementedArraySupport[E Event, A any] struct{}
)

func WithArraySupport[E Event, A any](impl ArraySupport[E, A]) Option[E] {
	return func(c *loggerConfig[E]) {
		if impl == nil {
			c.array = nil
		} else {
			c.array = &arraySupport[E]{
				factory: func() unsafe.Pointer { return unsafe.Pointer(impl.NewArray()) },
				write: func(evt E, key string, arr unsafe.Pointer) {
					impl.WriteArray(evt, key, (*A)(arr))
				},
				addField: func(arr unsafe.Pointer, val any) {
					impl.AddArrayField((*A)(arr), val)
				},
			}
		}
	}
}

func (UnimplementedArraySupport[E, A]) mustEmbedUnimplementedArraySupport() {}
