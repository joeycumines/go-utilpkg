package logiface

import (
	"github.com/joeycumines/go-utilpkg/logiface/internal/fieldtest"
)

type fieldBuilderCommonInterface[TFluent any, TCall any] interface {
	Call(fn func(c TCall)) TFluent
}

// fieldBuilderObjectInterface is used to ensure standardization of the numerous builder implementations
type fieldBuilderObjectInterface[TFluent any, TCall any] interface {
	fieldBuilderCommonInterface[TFluent, TCall]
	fieldtest.ObjectMethods[TFluent]
}

type fieldBuilderArrayInterface[TFluent any, TCall any] interface {
	fieldBuilderCommonInterface[TFluent, TCall]
	fieldtest.ArrayMethods[TFluent]
}

type fieldBuilderNestedInterface[TParent any] interface {
	As(key string) TParent
	Add() TParent
}

type fieldBuilderFactoryInterface[TEvent Event, TParent interface {
	Parent[TEvent]
	comparable
}, TSelf any] interface {
	Array() *ArrayBuilder[TEvent, *Chain[TEvent, TParent]]
	Object() *ObjectBuilder[TEvent, *Chain[TEvent, TParent]]
}

type fieldBuilderFactoryInterfaceArray[TEvent Event, TParent interface {
	Parent[TEvent]
	comparable
}, TSelf any] interface {
	fieldBuilderFactoryInterface[TEvent, TParent, TSelf]
	ArrayFunc(fn func(b *ArrayBuilder[TEvent, *Chain[TEvent, TParent]])) TSelf
	ObjectFunc(fn func(b *ObjectBuilder[TEvent, *Chain[TEvent, TParent]])) TSelf
}

type fieldBuilderFactoryInterfaceObject[TEvent Event, TParent interface {
	Parent[TEvent]
	comparable
}, TSelf any] interface {
	fieldBuilderFactoryInterface[TEvent, TParent, TSelf]
	ArrayFunc(key string, fn func(b *ArrayBuilder[TEvent, *Chain[TEvent, TParent]])) TSelf
	ObjectFunc(key string, fn func(b *ObjectBuilder[TEvent, *Chain[TEvent, TParent]])) TSelf
}

type eventBuilderInterface[TEvent Event, TFluent any] interface {
	Modifier(val Modifier[TEvent]) TFluent
}

type commonFluentInterface[TEvent Event] interface {
	Enabled() bool
	Root() *Logger[TEvent]
}

var (
	// compile time assertions

	_ fieldBuilderObjectInterface[*Builder[*mockSimpleEvent], *Builder[*mockSimpleEvent]]                                                                     = (*Builder[*mockSimpleEvent])(nil)
	_ fieldBuilderObjectInterface[*Context[*mockSimpleEvent], *Context[*mockSimpleEvent]]                                                                     = (*Context[*mockSimpleEvent])(nil)
	_ fieldBuilderObjectInterface[*ObjectBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]], *ObjectBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]]] = (*ObjectBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]])(nil)

	_ fieldBuilderArrayInterface[*ArrayBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]], *ArrayBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]]] = (*ArrayBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]])(nil)

	_ fieldBuilderNestedInterface[*Builder[*mockSimpleEvent]]                           = (*ArrayBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]])(nil)
	_ fieldBuilderNestedInterface[*Builder[*mockSimpleEvent]]                           = (*ObjectBuilder[*mockSimpleEvent, *Builder[*mockSimpleEvent]])(nil)
	_ fieldBuilderNestedInterface[*Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]]] = (*Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]])(nil)

	_ fieldBuilderFactoryInterfaceObject[*mockSimpleEvent, *Builder[*mockSimpleEvent], *Builder[*mockSimpleEvent]]                                                             = (*Builder[*mockSimpleEvent])(nil)
	_ fieldBuilderFactoryInterfaceObject[*mockSimpleEvent, *Context[*mockSimpleEvent], *Context[*mockSimpleEvent]]                                                             = (*Context[*mockSimpleEvent])(nil)
	_ fieldBuilderFactoryInterfaceObject[*mockSimpleEvent, *Builder[*mockSimpleEvent], *Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]]]                                   = (*Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]])(nil)
	_ fieldBuilderFactoryInterfaceArray[*mockSimpleEvent, *Builder[*mockSimpleEvent], *ArrayBuilder[*mockSimpleEvent, *Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]]]]   = (*ArrayBuilder[*mockSimpleEvent, *Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]]])(nil)
	_ fieldBuilderFactoryInterfaceObject[*mockSimpleEvent, *Builder[*mockSimpleEvent], *ObjectBuilder[*mockSimpleEvent, *Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]]]] = (*ObjectBuilder[*mockSimpleEvent, *Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]]])(nil)

	_ eventBuilderInterface[*mockSimpleEvent, *Builder[*mockSimpleEvent]] = (*Builder[*mockSimpleEvent])(nil)
	_ eventBuilderInterface[*mockSimpleEvent, *Context[*mockSimpleEvent]] = (*Context[*mockSimpleEvent])(nil)

	_ commonFluentInterface[*mockSimpleEvent] = (*Builder[*mockSimpleEvent])(nil)
	_ commonFluentInterface[*mockSimpleEvent] = (*Context[*mockSimpleEvent])(nil)
	_ commonFluentInterface[*mockSimpleEvent] = (*Logger[*mockSimpleEvent])(nil)
	_ commonFluentInterface[*mockSimpleEvent] = (*Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]])(nil)
	_ commonFluentInterface[*mockSimpleEvent] = (*ArrayBuilder[*mockSimpleEvent, *Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]]])(nil)
	_ commonFluentInterface[*mockSimpleEvent] = (*ObjectBuilder[*mockSimpleEvent, *Chain[*mockSimpleEvent, *Builder[*mockSimpleEvent]]])(nil)
)
