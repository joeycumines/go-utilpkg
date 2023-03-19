package logiface

import (
	"testing"
)

type (
	minimalArraySupportMethods[E Event, A any] interface {
		NewArray() A
		AddArray(evt E, key string, arr A)
		AppendField(arr A, val any) A
	}
)

var (
	// compile time assertions

	_ iArraySupport[Event]       = (*UnimplementedArraySupport[Event, any])(nil)
	_ ArraySupport[Event, []any] = (*sliceArraySupport[Event])(nil)
	_ ArraySupport[Event, []any] = struct {
		minimalArraySupportMethods[Event, []any]
		UnimplementedArraySupport[Event, []any]
	}{}
)

func TestUnimplementedArraySupport_mustEmbedUnimplementedArraySupport(t *testing.T) {
	(UnimplementedArraySupport[Event, []any]{}).mustEmbedUnimplementedArraySupport()
}

func TestSliceArraySupport_mustEmbedUnimplementedArraySupport(t *testing.T) {
	sliceArraySupport[Event]{}.mustEmbedUnimplementedArraySupport()
}
