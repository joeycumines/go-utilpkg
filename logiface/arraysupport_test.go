package logiface

import (
	"testing"
)

type (
	minimalArraySupportMethods[E Event, A any] interface {
		NewArray() *A
		WriteArray(evt E, key string, arr *A)
		AddArrayField(arr *A, val any)
	}
)

var (
	// compile time assertions

	_ ArraySupport[Event, []any] = struct {
		minimalArraySupportMethods[Event, []any]
		UnimplementedArraySupport[Event, []any]
	}{}
)

func TestUnimplementedArraySupport_mustEmbedUnimplementedArraySupport(t *testing.T) {
	(UnimplementedArraySupport[Event, []any]{}).mustEmbedUnimplementedArraySupport()
}
