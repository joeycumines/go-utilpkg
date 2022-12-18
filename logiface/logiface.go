package logiface

import (
	"errors"
)

type (
	Event interface {
		Level() Level
		SetMessage(msg string)
		AddField(key string, val any)
	}

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
	// WARNING: ErrDisabled must be returned directly (not wrapped).
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
