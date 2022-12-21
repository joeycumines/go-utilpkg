package logiface

import (
	"errors"
)

type (
	// Event models the integration with the logging framework.
	// The methods Level, SetMessage, and AddField are mandatory.
	// Implementations must have a zero value that doesn't panic when calling
	// Level, in which instance it must return LevelDisabled.
	// All implementations must embed UnimplementedEvent, as it provides
	// support for all optional methods.
	Event interface {
		// required methods

		// Level returns the level of the event.
		// It must be the same as originally provided to the factory.
		Level() Level
		// AddField adds a field to the event, for structured logging.
		// How fields are handled is implementation specific.
		AddField(key string, val any)

		// optional methods

		// AddMessage sets the log message for the event, returning false if unimplemented.
		// The field or output structure of the log message is implementation specific.
		AddMessage(msg string) bool
		// AddError adds an error to the event, returning false if unimplemented.
		// The field or output structure of the log message is implementation specific.
		AddError(err error) bool
		// AddString adds a field of type string. It's an optional optimisation.
		AddString(key string, val string) bool
		// AddInt adds a field of type int. It's an optional optimisation.
		AddInt(key string, val int) bool
		// AddFloat32 adds a field of type float32. It's an optional optimisation.
		AddFloat32(key string, val float32) bool

		mustEmbedUnimplementedEvent()
	}

	// UnimplementedEvent must be embedded in every Event implementation.
	// It provides implementation of methods that are optional.
	UnimplementedEvent struct{}

	LoggerImpl[E Event] interface {
		EventFactory[E]
		Writer[E]
	}

	EventFactory[E Event] interface {
		NewEvent(level Level) E
	}

	EventFactoryFunc[E Event] func(level Level) E

	Writer[E Event] interface {
		Write(event E) error
	}

	Modifier[E Event] interface {
		Modify(event E) error
	}

	ModifyFunc[E Event] func(event E) error

	WriteFunc[E Event] func(event E) error

	// ModifierSlice combines Modifier values, calling each in turn, returning
	// the first non-nil error.
	ModifierSlice[E Event] []Modifier[E]

	// WriterSlice combines writers, returning success on the first writer
	// that succeeds, returning the first error that isn't ErrDisabled, or
	// ErrDisabled if every writer returns ErrDisabled (or if empty).
	//
	// IMPL. WARNING: ErrDisabled must be returned directly (not wrapped).
	// USAGE WARNING: May complicate use of loggers that use sync.Pool.
	WriterSlice[E Event] []Writer[E]
)

var (
	// ErrDisabled is a sentinel value that can be returned by a Modifier to
	// disable logging of the event, or by a Writer to indicate that the event
	// was not written.
	// It may also return from Logger.Log.
	ErrDisabled = errors.New(`logger disabled`)

	// compile time assertions

	_ EventFactory[Event] = EventFactoryFunc[Event](nil)
	_ Modifier[Event]     = ModifyFunc[Event](nil)
	_ Writer[Event]       = WriteFunc[Event](nil)
	_ Modifier[Event]     = ModifierSlice[Event](nil)
	_ Writer[Event]       = WriterSlice[Event](nil)
)

func (x EventFactoryFunc[E]) NewEvent(level Level) E {
	return x(level)
}

func (x ModifyFunc[E]) Modify(event E) error {
	return x(event)
}

func (x WriteFunc[E]) Write(event E) error {
	return x(event)
}

func (x ModifierSlice[E]) Modify(event E) (err error) {
	for _, m := range x {
		err = m.Modify(event)
		if err != nil {
			break
		}
	}
	return
}

func (x WriterSlice[E]) Write(event E) (err error) {
	for _, w := range x {
		err = w.Write(event)
		if err != ErrDisabled {
			return
		}
	}
	return ErrDisabled
}

func (UnimplementedEvent) AddMessage(string) bool          { return false }
func (UnimplementedEvent) AddError(error) bool             { return false }
func (UnimplementedEvent) AddString(string, string) bool   { return false }
func (UnimplementedEvent) AddInt(string, int) bool         { return false }
func (UnimplementedEvent) AddFloat32(string, float32) bool { return false }
func (UnimplementedEvent) mustEmbedUnimplementedEvent()    {}
