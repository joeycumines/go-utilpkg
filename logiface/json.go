package logiface

import (
	"encoding/base64"
	"time"
)

type (
	JSONSupport[E Event, O any, A any] interface {
		NewObject() O
		AddObject(evt E, key string, obj O)
		SetField(obj O, key string, val any) O
		CanSetObject() bool
		SetObject(obj O, key string, val O) O
		CanSetArray() bool
		SetArray(obj O, key string, val A) O
		// CanAddStartObject indicates if the JSONSupport implementation can
		// start building an object with the key provided at the beginning when
		// adding to an event.
		CanAddStartObject() bool
		// AddStartObject initializes a new object with the given event and
		// key, and returns the new object.
		AddStartObject(evt E, key string) O
		// CanSetStartObject indicates if the JSONSupport implementation can
		// start building an object with the key provided at the beginning
		// when setting to an object.
		CanSetStartObject() bool
		// SetStartObject initializes a new object with the given parent object
		// and key, and returns the new object.
		SetStartObject(obj O, key string) O
		// CanSetStartArray indicates if the JSONSupport implementation can
		// start building an array with the key provided at the beginning when
		// setting to an object.
		CanSetStartArray() bool
		// SetStartArray initializes a new array with the given parent object
		// and key, and returns the new array.
		SetStartArray(obj O, key string) A
		CanSetString() bool
		SetString(obj O, key string, val string) O
		CanSetBool() bool
		SetBool(obj O, key string, val bool) O
		CanSetBase64Bytes() bool
		SetBase64Bytes(obj O, key string, b []byte, enc *base64.Encoding) O
		CanSetDuration() bool
		SetDuration(obj O, key string, d time.Duration) O
		CanSetError() bool
		SetError(obj O, err error) O
		CanSetInt() bool
		SetInt(obj O, key string, val int) O
		CanSetFloat32() bool
		SetFloat32(obj O, key string, val float32) O
		CanSetTime() bool
		SetTime(obj O, key string, t time.Time) O
		CanSetFloat64() bool
		SetFloat64(obj O, key string, val float64) O
		CanSetInt64() bool
		SetInt64(obj O, key string, val int64) O
		CanSetUint64() bool
		SetUint64(obj O, key string, val uint64) O

		NewArray() A
		AddArray(evt E, key string, arr A)
		AppendField(arr A, val any) A
		CanAppendObject() bool
		AppendObject(arr A, val O) A
		CanAppendArray() bool
		AppendArray(arr A, val A) A
		// CanAddStartArray indicates if the JSONSupport implementation can
		// start building an array with the key provided at the beginning when
		// adding to an event.
		CanAddStartArray() bool
		// AddStartArray initializes a new array with the given event and key,
		// and returns the new array.
		AddStartArray(evt E, key string) A
		// CanAppendStartObject indicates if the JSONSupport implementation can
		// append an object to an array with the key provided at the beginning.
		CanAppendStartObject() bool
		// AppendStartObject appends a new object to the given array and
		// returns the new object.
		AppendStartObject(arr A) O
		// CanAppendStartArray indicates if the JSONSupport implementation can
		// append an array to an array with the key provided at the beginning.
		CanAppendStartArray() bool
		// AppendStartArray appends a new array to the given array and returns
		// the new array.
		AppendStartArray(arr A) A
		CanAppendString() bool
		AppendString(arr A, val string) A
		CanAppendBool() bool
		AppendBool(arr A, val bool) A
		CanAppendBase64Bytes() bool
		AppendBase64Bytes(arr A, b []byte, enc *base64.Encoding) A
		CanAppendDuration() bool
		AppendDuration(arr A, d time.Duration) A
		CanAppendError() bool
		AppendError(arr A, err error) A
		CanAppendInt() bool
		AppendInt(arr A, val int) A
		CanAppendFloat32() bool
		AppendFloat32(arr A, val float32) A
		CanAppendTime() bool
		AppendTime(arr A, t time.Time) A
		CanAppendFloat64() bool
		AppendFloat64(arr A, val float64) A
		CanAppendInt64() bool
		AppendInt64(arr A, val int64) A
		CanAppendUint64() bool
		AppendUint64(arr A, val uint64) A

		mustEmbedUnimplementedJSONSupport()
	}

	// jsonSupport is available via loggerShared.array, and models an external
	// array builder implementation.
	jsonSupport[E Event] struct {
		iface             iJSONSupport[E]
		newObject         func() any
		addObject         func(evt E, key string, obj any)
		setField          func(obj any, key string, val any) any
		setObject         func(obj any, key string, val any) any
		setArray          func(obj any, key string, val any) any
		addStartObject    func(evt E, key string) any
		setStartObject    func(obj any, key string) any
		setStartArray     func(obj any, key string) any
		setString         func(obj any, key string, val string) any
		setBool           func(obj any, key string, val bool) any
		setBase64Bytes    func(obj any, key string, b []byte, enc *base64.Encoding) any
		setDuration       func(obj any, key string, d time.Duration) any
		setError          func(obj any, err error) any
		setInt            func(obj any, key string, val int) any
		setFloat32        func(obj any, key string, val float32) any
		setTime           func(obj any, key string, t time.Time) any
		setFloat64        func(obj any, key string, val float64) any
		setInt64          func(obj any, key string, val int64) any
		setUint64         func(obj any, key string, val uint64) any
		newArray          func() any
		addArray          func(evt E, key string, arr any)
		appendField       func(arr, val any) any
		appendArray       func(arr, val any) any
		appendObject      func(arr, val any) any
		addStartArray     func(evt E, key string) any
		appendStartObject func(arr any) any
		appendStartArray  func(arr any) any
		appendString      func(arr any, val string) any
		appendBool        func(arr any, val bool) any
		appendBase64Bytes func(arr any, b []byte, enc *base64.Encoding) any
		appendDuration    func(arr any, d time.Duration) any
		appendError       func(arr any, err error) any
		appendInt         func(arr any, val int) any
		appendFloat32     func(arr any, val float32) any
		appendTime        func(arr any, t time.Time) any
		appendFloat64     func(arr any, val float64) any
		appendInt64       func(arr any, val int64) any
		appendUint64      func(arr any, val uint64) any
	}

	// iJSONSupport are the [JSONSupport] methods without type-specific behavior
	// (e.g. flags / checking if certain methods can be used)
	iJSONSupport[E Event] interface {
		CanSetObject() bool
		CanSetArray() bool
		CanAddStartObject() bool
		CanSetStartObject() bool
		CanSetStartArray() bool
		CanSetString() bool
		CanSetBool() bool
		CanSetBase64Bytes() bool
		CanSetDuration() bool
		CanSetError() bool
		CanSetInt() bool
		CanSetFloat32() bool
		CanSetTime() bool
		CanSetFloat64() bool
		CanSetInt64() bool
		CanSetUint64() bool
		CanAppendArray() bool
		CanAppendObject() bool
		CanAddStartArray() bool
		CanAppendStartObject() bool
		CanAppendStartArray() bool
		CanAppendString() bool
		CanAppendBool() bool
		CanAppendBase64Bytes() bool
		CanAppendDuration() bool
		CanAppendError() bool
		CanAppendInt() bool
		CanAppendFloat32() bool
		CanAppendTime() bool
		CanAppendFloat64() bool
		CanAppendInt64() bool
		CanAppendUint64() bool
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
	return optionFunc[E](func(c *loggerConfig[E]) {
		if impl == nil {
			c.json = nil
		} else {
			c.json = newJSONSupport(impl)
		}
	})
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
		addStartObject: func(evt E, key string) any {
			return impl.AddStartObject(evt, key)
		},
		setStartObject: func(obj any, key string) any {
			return impl.SetStartObject(obj.(O), key)
		},
		setStartArray: func(obj any, key string) any {
			return impl.SetStartArray(obj.(O), key)
		},
		setString: func(obj any, key string, val string) any {
			return impl.SetString(obj.(O), key, val)
		},
		setBool: func(obj any, key string, val bool) any {
			return impl.SetBool(obj.(O), key, val)
		},
		setBase64Bytes: func(obj any, key string, b []byte, enc *base64.Encoding) any {
			return impl.SetBase64Bytes(obj.(O), key, b, enc)
		},
		setDuration: func(obj any, key string, d time.Duration) any {
			return impl.SetDuration(obj.(O), key, d)
		},
		setError: func(obj any, err error) any {
			return impl.SetError(obj.(O), err)
		},
		setInt: func(obj any, key string, val int) any {
			return impl.SetInt(obj.(O), key, val)
		},
		setFloat32: func(obj any, key string, val float32) any {
			return impl.SetFloat32(obj.(O), key, val)
		},
		setTime: func(obj any, key string, t time.Time) any {
			return impl.SetTime(obj.(O), key, t)
		},
		setFloat64: func(obj any, key string, val float64) any {
			return impl.SetFloat64(obj.(O), key, val)
		},
		setInt64: func(obj any, key string, val int64) any {
			return impl.SetInt64(obj.(O), key, val)
		},
		setUint64: func(obj any, key string, val uint64) any {
			return impl.SetUint64(obj.(O), key, val)
		},
		newArray: func() any {
			return impl.NewArray()
		},
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
		addStartArray: func(evt E, key string) any {
			return impl.AddStartArray(evt, key)
		},
		appendStartObject: func(arr any) any {
			return impl.AppendStartObject(arr.(A))
		},
		appendStartArray: func(arr any) any {
			return impl.AppendStartArray(arr.(A))
		},
		appendString: func(arr any, val string) any {
			return impl.AppendString(arr.(A), val)
		},
		appendBool: func(arr any, val bool) any {
			return impl.AppendBool(arr.(A), val)
		},
		appendBase64Bytes: func(arr any, b []byte, enc *base64.Encoding) any {
			return impl.AppendBase64Bytes(arr.(A), b, enc)
		},
		appendDuration: func(arr any, d time.Duration) any {
			return impl.AppendDuration(arr.(A), d)
		},
		appendError: func(arr any, err error) any {
			return impl.AppendError(arr.(A), err)
		},
		appendInt: func(arr any, val int) any {
			return impl.AppendInt(arr.(A), val)
		},
		appendFloat32: func(arr any, val float32) any {
			return impl.AppendFloat32(arr.(A), val)
		},
		appendTime: func(arr any, t time.Time) any {
			return impl.AppendTime(arr.(A), t)
		},
		appendFloat64: func(arr any, val float64) any {
			return impl.AppendFloat64(arr.(A), val)
		},
		appendInt64: func(arr any, val int64) any {
			return impl.AppendInt64(arr.(A), val)
		},
		appendUint64: func(arr any, val uint64) any {
			return impl.AppendUint64(arr.(A), val)
		},
	}
}

func generifyJSONSupport[E Event](impl *jsonSupport[E]) *jsonSupport[Event] {
	return &jsonSupport[Event]{
		iface:             impl.iface,
		newObject:         impl.newObject,
		addObject:         func(evt Event, key string, obj any) { impl.addObject(evt.(E), key, obj) },
		setField:          impl.setField,
		setObject:         impl.setObject,
		setArray:          impl.setArray,
		addStartObject:    func(evt Event, key string) any { return impl.addStartObject(evt.(E), key) },
		setStartObject:    impl.setStartObject,
		setStartArray:     impl.setStartArray,
		setString:         impl.setString,
		setBool:           impl.setBool,
		setBase64Bytes:    impl.setBase64Bytes,
		setDuration:       impl.setDuration,
		setError:          impl.setError,
		setInt:            impl.setInt,
		setFloat32:        impl.setFloat32,
		setTime:           impl.setTime,
		setFloat64:        impl.setFloat64,
		setInt64:          impl.setInt64,
		setUint64:         impl.setUint64,
		newArray:          impl.newArray,
		addArray:          func(evt Event, key string, arr any) { impl.addArray(evt.(E), key, arr) },
		appendField:       impl.appendField,
		appendArray:       impl.appendArray,
		appendObject:      impl.appendObject,
		addStartArray:     func(evt Event, key string) any { return impl.addStartArray(evt.(E), key) },
		appendStartObject: impl.appendStartObject,
		appendStartArray:  impl.appendStartArray,
		appendString:      impl.appendString,
		appendBool:        impl.appendBool,
		appendBase64Bytes: impl.appendBase64Bytes,
		appendDuration:    impl.appendDuration,
		appendError:       impl.appendError,
		appendInt:         impl.appendInt,
		appendFloat32:     impl.appendFloat32,
		appendTime:        impl.appendTime,
		appendFloat64:     impl.appendFloat64,
		appendInt64:       impl.appendInt64,
		appendUint64:      impl.appendUint64,
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

func (UnimplementedJSONSupport[E, O, A]) CanAddStartObject() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AddStartObject(evt E, key string) O {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanSetStartObject() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) SetStartObject(obj O, key string) O {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanSetStartArray() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) SetStartArray(obj O, key string) A {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanSetString() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) SetString(obj O, key string, val string) O {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanSetBool() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) SetBool(obj O, key string, val bool) O {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanSetBase64Bytes() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) SetBase64Bytes(obj O, key string, b []byte, enc *base64.Encoding) O {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanSetDuration() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) SetDuration(obj O, key string, d time.Duration) O {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanSetError() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) SetError(obj O, err error) O {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanSetInt() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) SetInt(obj O, key string, val int) O {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanSetFloat32() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) SetFloat32(obj O, key string, val float32) O {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanSetTime() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) SetTime(obj O, key string, t time.Time) O {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanSetFloat64() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) SetFloat64(obj O, key string, val float64) O {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanSetInt64() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) SetInt64(obj O, key string, val int64) O {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanSetUint64() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) SetUint64(obj O, key string, val uint64) O {
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

func (UnimplementedJSONSupport[E, O, A]) CanAddStartArray() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AddStartArray(evt E, key string) A {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanAppendStartObject() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AppendStartObject(arr A) O {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanAppendStartArray() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AppendStartArray(arr A) A {
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

func (UnimplementedJSONSupport[E, O, A]) CanAppendBase64Bytes() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AppendBase64Bytes(arr A, b []byte, enc *base64.Encoding) A {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanAppendDuration() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AppendDuration(arr A, d time.Duration) A {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanAppendError() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AppendError(arr A, err error) A {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanAppendInt() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AppendInt(arr A, val int) A {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanAppendFloat32() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AppendFloat32(arr A, val float32) A {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanAppendTime() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AppendTime(arr A, t time.Time) A {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanAppendFloat64() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AppendFloat64(arr A, val float64) A {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanAppendInt64() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AppendInt64(arr A, val int64) A {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) CanAppendUint64() bool { return false }

func (UnimplementedJSONSupport[E, O, A]) AppendUint64(arr A, val uint64) A {
	panic("unimplemented")
}

func (UnimplementedJSONSupport[E, O, A]) mustEmbedUnimplementedJSONSupport() {}

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

func (x defaultJSONSupport[E]) CanAddStartObject() bool { return false }

func (x defaultJSONSupport[E]) AddStartObject(evt E, key string) map[string]any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanSetStartObject() bool { return false }

func (x defaultJSONSupport[E]) SetStartObject(obj map[string]any, key string) map[string]any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanSetStartArray() bool { return false }

func (x defaultJSONSupport[E]) SetStartArray(obj map[string]any, key string) []any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanSetString() bool { return false }

func (x defaultJSONSupport[E]) SetString(obj map[string]any, key string, val string) map[string]any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanSetBool() bool { return false }

func (x defaultJSONSupport[E]) SetBool(obj map[string]any, key string, val bool) map[string]any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanSetBase64Bytes() bool { return false }

func (x defaultJSONSupport[E]) SetBase64Bytes(obj map[string]any, key string, b []byte, enc *base64.Encoding) map[string]any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanSetDuration() bool { return false }

func (x defaultJSONSupport[E]) SetDuration(obj map[string]any, key string, d time.Duration) map[string]any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanSetError() bool { return false }

func (x defaultJSONSupport[E]) SetError(obj map[string]any, err error) map[string]any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanSetInt() bool { return false }

func (x defaultJSONSupport[E]) SetInt(obj map[string]any, key string, val int) map[string]any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanSetFloat32() bool { return false }

func (x defaultJSONSupport[E]) SetFloat32(obj map[string]any, key string, val float32) map[string]any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanSetTime() bool { return false }

func (x defaultJSONSupport[E]) SetTime(obj map[string]any, key string, t time.Time) map[string]any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanSetFloat64() bool { return false }

func (x defaultJSONSupport[E]) SetFloat64(obj map[string]any, key string, val float64) map[string]any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanSetInt64() bool { return false }

func (x defaultJSONSupport[E]) SetInt64(obj map[string]any, key string, val int64) map[string]any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanSetUint64() bool { return false }

func (x defaultJSONSupport[E]) SetUint64(obj map[string]any, key string, val uint64) map[string]any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) NewArray() []any { return make([]any, 0) }

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

func (x defaultJSONSupport[E]) CanAddStartArray() bool { return false }

func (x defaultJSONSupport[E]) AddStartArray(evt E, key string) []any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanAppendStartObject() bool { return false }

func (x defaultJSONSupport[E]) AppendStartObject(arr []any) map[string]any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanAppendStartArray() bool { return false }

func (x defaultJSONSupport[E]) AppendStartArray(arr []any) []any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanAppendString() bool { return false }

func (x defaultJSONSupport[E]) AppendString(arr []any, val string) []any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanAppendBool() bool { return false }

func (x defaultJSONSupport[E]) AppendBool(arr []any, val bool) []any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanAppendBase64Bytes() bool { return false }

func (x defaultJSONSupport[E]) AppendBase64Bytes(arr []any, b []byte, enc *base64.Encoding) []any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanAppendDuration() bool { return false }

func (x defaultJSONSupport[E]) AppendDuration(arr []any, d time.Duration) []any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanAppendError() bool { return false }

func (x defaultJSONSupport[E]) AppendError(arr []any, err error) []any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanAppendInt() bool { return false }

func (x defaultJSONSupport[E]) AppendInt(arr []any, val int) []any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanAppendFloat32() bool { return false }

func (x defaultJSONSupport[E]) AppendFloat32(arr []any, val float32) []any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanAppendTime() bool { return false }

func (x defaultJSONSupport[E]) AppendTime(arr []any, t time.Time) []any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanAppendFloat64() bool { return false }

func (x defaultJSONSupport[E]) AppendFloat64(arr []any, val float64) []any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanAppendInt64() bool { return false }

func (x defaultJSONSupport[E]) AppendInt64(arr []any, val int64) []any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) CanAppendUint64() bool { return false }

func (x defaultJSONSupport[E]) AppendUint64(arr []any, val uint64) []any {
	panic("unimplemented")
}

func (x defaultJSONSupport[E]) mustEmbedUnimplementedJSONSupport() {}

func (x *jsonSupport[E]) addStartOrNewObject(event E, key string) any {
	if x.iface.CanAddStartObject() {
		return x.addStartObject(event, key)
	}
	return x.newObject()
}

func (x *jsonSupport[E]) setStartOrNewObject(obj any, key string) any {
	if x.iface.CanSetStartObject() {
		return x.setStartObject(obj, key)
	}
	return x.newObject()
}

func (x *jsonSupport[E]) appendStartOrNewObject(arr any) any {
	if x.iface.CanAppendStartObject() {
		return x.appendStartObject(arr)
	}
	return x.newObject()
}

func (x *jsonSupport[E]) addStartOrNewArray(event E, key string) any {
	if x.iface.CanAddStartArray() {
		return x.addStartArray(event, key)
	}
	return x.newArray()
}

func (x *jsonSupport[E]) setStartOrNewArray(obj any, key string) any {
	if x.iface.CanSetStartArray() {
		return x.setStartArray(obj, key)
	}
	return x.newArray()
}

func (x *jsonSupport[E]) appendStartOrNewArray(arr any) any {
	if x.iface.CanAppendStartArray() {
		return x.appendStartArray(arr)
	}
	return x.newArray()
}
