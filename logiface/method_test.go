package logiface

import (
	"encoding/base64"
	"time"
)

// fieldBuilderObjectInterface is used to ensure standardization of the numerous builder implementations
type fieldBuilderObjectInterface[TEvent Event, TFluent any, TCall any] interface {
	Call(fn func(c TCall)) TFluent
	Field(key string, val any) TFluent
	Any(key string, val any) TFluent
	Base64(key string, b []byte, enc *base64.Encoding) TFluent
	Dur(key string, d time.Duration) TFluent
	Err(err error) TFluent
	Float32(key string, val float32) TFluent
	Int(key string, val int) TFluent
	Interface(key string, val any) TFluent
	Str(key string, val string) TFluent
	Time(key string, t time.Time) TFluent
	Bool(key string, val bool) TFluent
	Float64(key string, val float64) TFluent
	Int64(key string, val int64) TFluent
	Uint64(key string, val uint64) TFluent
}

type fieldBuilderArrayInterface[TEvent Event, TFluent any, TCall any] interface {
	Call(fn func(c TCall)) TFluent
	Field(val any) TFluent
	Any(val any) TFluent
	Base64(b []byte, enc *base64.Encoding) TFluent
	Dur(d time.Duration) TFluent
	Err(err error) TFluent
	Float32(val float32) TFluent
	Int(val int) TFluent
	Interface(val any) TFluent
	Str(val string) TFluent
	Time(t time.Time) TFluent
	Bool(val bool) TFluent
	Float64(val float64) TFluent
	Int64(val int64) TFluent
	Uint64(val uint64) TFluent
}

type fieldBuilderNestedInterface[TParent any] interface {
	As(key string) TParent
	Add() TParent
}

type fieldBuilderFactoryInterface[TEvent Event, TParent interface {
	Parent[TEvent]
	comparable
}] interface {
	Array() *ArrayBuilder[TEvent, *Chain[TEvent, TParent]]
	Object() *ObjectBuilder[TEvent, *Chain[TEvent, TParent]]
}

var (
	// compile time assertions

	_ fieldBuilderObjectInterface[*mockSimpleEvent, *Builder[*mockSimpleEvent], *Builder[*mockSimpleEvent]]                                                                     = (*Builder[*mockSimpleEvent])(nil)
	_ fieldBuilderObjectInterface[*mockSimpleEvent, *Context[*mockSimpleEvent], *Context[*mockSimpleEvent]]                                                                     = (*Context[*mockSimpleEvent])(nil)
	_ fieldBuilderObjectInterface[*mockSimpleEvent, ConditionalBuilder[*mockSimpleEvent], *Builder[*mockSimpleEvent]]                                                           = ConditionalBuilder[*mockSimpleEvent](nil)
	_ fieldBuilderObjectInterface[*mockSimpleEvent, *ObjectBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]], *ObjectBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]]] = (*ObjectBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]])(nil)

	_ fieldBuilderArrayInterface[*mockSimpleEvent, *ArrayBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]], *ArrayBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]]] = (*ArrayBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]])(nil)

	_ fieldBuilderNestedInterface[*Builder[*mockSimpleEvent]]                           = (*ArrayBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]])(nil)
	_ fieldBuilderNestedInterface[*Builder[*mockSimpleEvent]]                           = (*ObjectBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]])(nil)
	_ fieldBuilderNestedInterface[*Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]]] = (*Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]])(nil)

	_ fieldBuilderFactoryInterface[*mockSimpleEvent, *Builder[*mockSimpleEvent]] = (*Builder[*mockSimpleEvent])(nil)
	_ fieldBuilderFactoryInterface[*mockSimpleEvent, *Context[*mockSimpleEvent]] = (*Context[*mockSimpleEvent])(nil)
	_ fieldBuilderFactoryInterface[*mockSimpleEvent, *Builder[*mockSimpleEvent]] = (*Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]])(nil)
	_ fieldBuilderFactoryInterface[*mockSimpleEvent, *Builder[*mockSimpleEvent]] = (*ArrayBuilder[*mockSimpleEvent, *Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]]])(nil)
	_ fieldBuilderFactoryInterface[*mockSimpleEvent, *Builder[*mockSimpleEvent]] = (*ObjectBuilder[*mockSimpleEvent, *Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]]])(nil)
)
