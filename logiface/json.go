package logiface

type (
	JSONSupport[E Event, O any, A any] interface {
		NewObject() O
		AddObject(evt E, key string, obj O)
		SetField(obj O, key string, val any) O
		CanSetObject() bool
		SetObject(obj O, key string, val O) O
		CanSetArray() bool
		SetArray(obj O, key string, val A) O

		NewArray() A
		AddArray(evt E, key string, arr A)
		AppendField(arr A, val any) A
		CanAppendArray() bool
		AppendArray(arr A, val A) A
		CanAppendString() bool
		AppendString(arr A, val string) A
		CanAppendBool() bool
		AppendBool(arr A, val bool) A
		CanAppendObject() bool
		AppendObject(arr A, val O) A

		mustEmbedUnimplementedJSONSupport()
	}

	// jsonSupport is available via loggerShared.array, and models an external
	// array builder implementation.
	jsonSupport[E Event] struct {
		iface        iJSONSupport[E]
		newObject    func() any
		addObject    func(evt E, key string, obj any)
		setField     func(obj any, key string, val any) any
		setObject    func(obj any, key string, val any) any
		setArray     func(obj any, key string, val any) any
		newArray     func() any
		addArray     func(evt E, key string, arr any)
		appendField  func(arr, val any) any
		appendArray  func(arr, val any) any
		appendObject func(arr, val any) any
		appendString func(arr any, val string) any
		appendBool   func(arr any, val bool) any
	}

	// iJSONSupport are the [JSONSupport] methods without type-specific behavior
	// (e.g. flags / checking if certain methods can be used)
	iJSONSupport[E Event] interface {
		CanSetObject() bool
		CanSetArray() bool
		CanAppendArray() bool
		CanAppendObject() bool
		CanAppendString() bool
		CanAppendBool() bool
	}

	UnimplementedJSONSupport[E Event, O any, A any] struct{}

	defaultJSONSupport[E Event] struct{}
)

// WithJSONSupport configures the implementation the logger uses to back
// support for nested data structures.
//
// By default, slices of type `[]any` are used.
//
// See also [LoggerFactory.WithJSONSupport].
func WithJSONSupport[E Event, O any, A any](impl JSONSupport[E, O, A]) Option[E] {
	return func(c *loggerConfig[E]) {
		if impl == nil {
			c.json = nil
		} else {
			c.json = newJSONSupport(impl)
		}
	}
}

// WithJSONSupport configures the implementation the logger uses to back
// support for nested data structures.
//
// By default, maps of type `map[string]any` and slices of type `[]any` are used.
//
// Depending on your implementation, you may need the [WithJSONSupport]
// function, instead.
func (LoggerFactory[E]) WithJSONSupport(impl JSONSupport[E, any, any]) Option[E] {
	return WithJSONSupport(impl)
}

func newJSONSupport[E Event, O any, A any](impl JSONSupport[E, O, A]) *jsonSupport[E] {
	return &jsonSupport[E]{
		iface: impl,
		newObject: func() any {
			return impl.NewObject()
		},
		addObject: func(evt E, key string, obj any) {
			impl.AddObject(evt, key, obj.(O))
		},
		setField: func(obj any, key string, val any) any {
			return impl.SetField(obj.(O), key, val)
		},
		setObject: func(obj any, key string, val any) any {
			return impl.SetObject(obj.(O), key, val.(O))
		},
		setArray: func(obj any, key string, val any) any {
			return impl.SetArray(obj.(O), key, val.(A))
		},
		newArray: func() any { return impl.NewArray() },
		addArray: func(evt E, key string, arr any) {
			impl.AddArray(evt, key, arr.(A))
		},
		appendField: func(arr, val any) any {
			return impl.AppendField(arr.(A), val)
		},
		appendArray: func(arr, val any) any {
			return impl.AppendArray(arr.(A), val.(A))
		},
		appendObject: func(arr, val any) any {
			return impl.AppendObject(arr.(A), val.(O))
		},
		appendString: func(arr any, val string) any {
			return impl.AppendString(arr.(A), val)
		},
		appendBool: func(arr any, val bool) any {
			return impl.AppendBool(arr.(A), val)
		},
	}
}

func generifyJSONSupport[E Event](impl *jsonSupport[E]) *jsonSupport[Event] {
	return &jsonSupport[Event]{
		iface:     impl.iface,
		newObject: impl.newObject,
		addObject: func(evt Event, key string, obj any) {
			impl.addObject(evt.(E), key, obj)
		},
		setField:  impl.setField,
		setObject: impl.setObject,
		setArray:  impl.setArray,
		newArray:  impl.newArray,
		addArray: func(evt Event, key string, arr any) {
			impl.addArray(evt.(E), key, arr)
		},
		appendField:  impl.appendField,
		appendArray:  impl.appendArray,
		appendObject: impl.appendObject,
		appendString: impl.appendString,
		appendBool:   impl.appendBool,
	}
}

func (UnimplementedJSONSupport[E, O, A]) CanSetObject() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) SetObject(obj O, key string, val O) O {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanSetArray() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) SetArray(obj O, key string, val A) O {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanAppendArray() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AppendArray(arr A, val A) A {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanAppendObject() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AppendObject(arr A, val O) A {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanAppendString() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AppendString(arr A, val string) A {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanAppendBool() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AppendBool(arr A, val bool) A {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) mustEmbedUnimplementedJSONSupport() {}

func (x defaultJSONSupport[E]) CanNewObject() bool { return true }

func (x defaultJSONSupport[E]) NewObject() map[string]any { return make(map[string]any) }

func (x defaultJSONSupport[E]) AddObject(evt E, key string, obj map[string]any) {
	evt.AddField(key, obj)
}

func (x defaultJSONSupport[E]) SetField(obj map[string]any, key string, val any) map[string]any {
	obj[key] = val
	return obj
}

func (x defaultJSONSupport[E]) CanSetObject() bool { return true }

func (x defaultJSONSupport[E]) SetObject(obj map[string]any, key string, val map[string]any) map[string]any {
	obj[key] = val
	return obj
}

func (x defaultJSONSupport[E]) CanSetArray() bool { return true }

func (x defaultJSONSupport[E]) SetArray(obj map[string]any, key string, val []any) map[string]any {
	obj[key] = val
	return obj
}

func (x defaultJSONSupport[E]) CanNewArray() bool { return true }

func (x defaultJSONSupport[E]) NewArray() []any { return nil }

func (x defaultJSONSupport[E]) AddArray(evt E, key string, arr []any) {
	evt.AddField(key, arr)
}

func (x defaultJSONSupport[E]) AppendField(arr []any, val any) []any {
	return append(arr, val)
}

func (x defaultJSONSupport[E]) CanAppendArray() bool { return true }

func (x defaultJSONSupport[E]) AppendArray(arr []any, val []any) []any {
	return append(arr, val)
}

func (x defaultJSONSupport[E]) CanAppendObject() bool { return true }

func (x defaultJSONSupport[E]) AppendObject(arr []any, val map[string]any) []any {
	return append(arr, val)
}

func (x defaultJSONSupport[E]) CanAppendString() bool { return true }

func (x defaultJSONSupport[E]) AppendString(arr []any, val string) []any {
	return append(arr, val)
}

func (x defaultJSONSupport[E]) CanAppendBool() bool { return true }

func (x defaultJSONSupport[E]) AppendBool(arr []any, val bool) []any {
	return append(arr, val)
}

func (x defaultJSONSupport[E]) mustEmbedUnimplementedJSONSupport() {}
