package logiface

import (
	"encoding/base64"
	"errors"
	"time"
)

type (
	// Event models the integration with the logging framework.
	//
	// The methods Level and AddField are mandatory.
	//
	// Implementations must have a zero value that doesn't panic when calling
	// Level, in which instance it must return LevelDisabled.
	//
	// All implementations must embed UnimplementedEvent, as it provides
	// support for optional methods (and facilitates additional optional
	// methods being added, without breaking changes).
	//
	// # Adding a new log field type
	//
	// In general, types of log fields are added via the implementation of new
	// methods, which should be present in all of [Builder], [Context], and
	// [Event], though the [Event] method will differ.
	//
	// The method name and arguments, for Builder and Context, are typically
	// styled after the equivalent provided by zerolog, though they need not be.
	// The arguments for Event are typically identical, returning a bool (to
	// indicate support), with the method name being somewhat arbitrary, but
	// ideally descriptive and consistent, e.g. Add<full type name>. The Event
	// interface isn't used directly, by end users, making brevity unimportant.
	//
	// These are common development tasks, when a new field type is added:
	//
	//   1. Add the new field type method to the [Event] interface (e.g. AddDuration)
	//   2. Add the new field type method to the [UnimplementedEvent] struct (return false)
	//   3. Add the calling/fallback behavior as a new unexported method of the internal modifierMethods struct (e.g. dur)
	//   4. Update the (internal) modifierMethods.Field method, with type case(s) using 3., for the new field type
	//   5. Add a new (internal) method to the modifierMethods struct, using 3., named per [Builder] and [Context] (e.g. Dur)
	//   6. Add to each of [Builder] and [Context] a method named the same as and using 5. (e.g. Dur)
	//   7. Add the same method to [ConditionalBuilder] (note: passes through to [Builder])
	//   8. Add the Event method to mockComplexEvent in mock_test.go
	//   9. Run make in the root of the git repository, fix any issues
	//   10. Add appropriate Field and specific method calls (e.g. Dur) to fluentCallerTemplate in mock_test.go (note: update the T generic interface)
	//   11. Fix all test cases that fail
	//   12. Update the testsuite module to include the new field type (e.g. throw it on eventTemplate1 in templates.go)
	//   13. Run make in the root of the git repository, everything should still pass
	//   14. Implement new field type in all relevant implementation modules (e.g. logiface/zerolog)
	//   15. Fix any issues with the test harness implementations, which may require adding additional functionality to logiface/testsuite, see also normalizeEvent
	//   16. Consider adding or updating benchmarks, e.g. the comparison (vs direct use) benchmarks in logiface/zerolog
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
		// AddTime adds a field of type time.Time. It's an optional optimisation.
		AddTime(key string, val time.Time) bool
		// AddDuration adds a field of type time.Duration. It's an optional optimisation.
		AddDuration(key string, val time.Duration) bool
		// AddBase64Bytes adds bytes as a field, to be base64 encoded.
		// The enc param will always be non-nil, and is the encoding to use.
		// This abstraction is provided to allow implementations to use the
		// most appropriate method, of the enc param.
		// It's an optional optimisation.
		AddBase64Bytes(key string, val []byte, enc *base64.Encoding) bool
		// AddBool adds a field of type bool. It's an optional optimisation.
		AddBool(key string, val bool) bool
		// AddFloat64 adds a field of type float64. It's an optional optimisation.
		AddFloat64(key string, val float64) bool
		// AddInt64 adds a field of type int64. It's an optional optimisation.
		AddInt64(key string, val int64) bool
		// AddUint64 adds a field of type uint64. It's an optional optimisation.
		AddUint64(key string, val uint64) bool

		mustEmbedUnimplementedEvent()
	}

	// UnimplementedEvent must be embedded in every Event implementation.
	// It provides implementation of methods that are optional.
	UnimplementedEvent struct{}

	// EventFactory initializes a new Event, for Logger instances.
	//
	// As Builder instances are pooled, implementations may need to implement
	// EventReleaser as a way to clear references to objects that should be
	// garbage collected.
	//
	// Note that it may be desirable to use a pool of events, to reduce
	// unnecessary allocations. In this case, EventReleaser should be
	// implemented, to return the event to the pool.
	EventFactory[E Event] interface {
		NewEvent(level Level) E
	}

	// EventFactoryFunc implements EventFactory.
	EventFactoryFunc[E Event] func(level Level) E

	// EventReleaser is an optional implementation that may be used to either
	// "reset" or "release" concrete implementations of Event.
	//
	// Use this interface to, for example, clear references to which should
	// be garbage collected, or return references to pool(s).
	EventReleaser[E Event] interface {
		ReleaseEvent(event E)
	}

	// EventReleaserFunc implements EventReleaser.
	EventReleaserFunc[E Event] func(event E)

	// Writer writes out / finalizes an event.
	//
	// Event MUST NOT be retained or modified after the call.
	Writer[E Event] interface {
		Write(event E) error
	}

	// Modifier is used to model the configuration of a log event, e.g. adding
	// fields, including the message.
	Modifier[E Event] interface {
		Modify(event E) error
	}

	// ModifierFunc implements Modifier.
	ModifierFunc[E Event] func(event E) error

	// WriterFunc implements Writer.
	WriterFunc[E Event] func(event E) error

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
	//
	// It may also return from Logger.Log.
	ErrDisabled = errors.New(`logger disabled`)
)

// NewEventFactoryFunc is an alias provided as a convenience, to make it easier to cast a function to an
// EventFactoryFunc.
//
// It's equivalent to EventFactoryFunc[E](f), which is more verbose, as it cannot infer the type.
//
// See also [LoggerFactory.NewEventFactoryFunc].
func NewEventFactoryFunc[E Event](f func(level Level) E) EventFactoryFunc[E] { return f }

// NewEventReleaserFunc is an alias provided as a convenience, to make it easier to cast a function to an
// EventReleaserFunc.
//
// It's equivalent to EventReleaserFunc[E](f), which is more verbose, as it cannot infer the type.
//
// See also [LoggerFactory.NewEventFactoryFunc].
func NewEventReleaserFunc[E Event](f func(event E)) EventReleaserFunc[E] { return f }

// NewModifierFunc is an alias provided as a convenience, to make it easier to cast a function to a ModifierFunc.
//
// It's equivalent to ModifierFunc[E](f), which is more verbose, as it cannot infer the type.
//
// See also [LoggerFactory.NewModifierFunc].
func NewModifierFunc[E Event](f func(event E) error) ModifierFunc[E] { return f }

// NewWriterFunc is an alias provided as a convenience, to make it easier to cast a function to a WriterFunc.
//
// It's equivalent to WriterFunc[E](f), which is more verbose, as it cannot infer the type.
//
// See also [LoggerFactory.NewWriterFunc].
func NewWriterFunc[E Event](f func(event E) error) WriterFunc[E] { return f }

// NewModifierSlice is an alias provided as a convenience, to make it easier to initialize a ModifierSlice.
//
// See also [LoggerFactory.NewModifierSlice].
func NewModifierSlice[E Event](s ...Modifier[E]) ModifierSlice[E] { return s }

// NewWriterSlice is an alias provided as a convenience, to make it easier to initialize a WriterSlice.
//
// See also [LoggerFactory.NewWriterSlice].
func NewWriterSlice[E Event](s ...Writer[E]) WriterSlice[E] { return s }

func (x EventFactoryFunc[E]) NewEvent(level Level) E {
	return x(level)
}

func (x EventReleaserFunc[E]) ReleaseEvent(event E) {
	x(event)
}

func (x ModifierFunc[E]) Modify(event E) error {
	return x(event)
}

func (x WriterFunc[E]) Write(event E) error {
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

func (UnimplementedEvent) AddMessage(string) bool { return false }

func (UnimplementedEvent) AddError(error) bool { return false }

func (UnimplementedEvent) AddString(string, string) bool { return false }

func (UnimplementedEvent) AddInt(string, int) bool { return false }

func (UnimplementedEvent) AddFloat32(string, float32) bool { return false }

func (UnimplementedEvent) AddTime(string, time.Time) bool { return false }

func (UnimplementedEvent) AddDuration(string, time.Duration) bool { return false }

func (UnimplementedEvent) AddBase64Bytes(string, []byte, *base64.Encoding) bool { return false }

func (UnimplementedEvent) AddBool(string, bool) bool { return false }

func (UnimplementedEvent) AddFloat64(string, float64) bool { return false }

func (UnimplementedEvent) AddInt64(string, int64) bool { return false }

func (UnimplementedEvent) AddUint64(string, uint64) bool { return false }

func (UnimplementedEvent) mustEmbedUnimplementedEvent() {}

// NewEventFactoryFunc is an alias provided as a convenience, to make it easier to cast a function to an
// EventFactoryFunc.
//
// It's equivalent to EventFactoryFunc[E](f), which is more verbose, as it requires specifying the type, which, for
// this method, comes from the receiver.
//
// See also [logiface.NewEventFactoryFunc].
func (LoggerFactory[E]) NewEventFactoryFunc(f func(level Level) E) EventFactoryFunc[E] { return f }

// NewEventReleaserFunc is an alias provided as a convenience, to make it easier to cast a function to an
// EventReleaserFunc.
//
// It's equivalent to EventReleaserFunc[E](f), which is more verbose, as it requires specifying the type, which, for
// this method, comes from the receiver.
//
// See also [logiface.NewEventReleaserFunc].
func (LoggerFactory[E]) NewEventReleaserFunc(f func(event E)) EventReleaserFunc[E] { return f }

// NewModifierFunc is an alias provided as a convenience, to make it easier to cast a function to a ModifierFunc.
//
// It's equivalent to ModifierFunc[E](f), which is more verbose, as it requires specifying the type, which, for
// this method, comes from the receiver.
//
// See also [logiface.NewModifierFunc].
func (LoggerFactory[E]) NewModifierFunc(f func(event E) error) ModifierFunc[E] { return f }

// NewWriterFunc is an alias provided as a convenience, to make it easier to cast a function to a WriterFunc.
//
// It's equivalent to WriterFunc[E](f), which is more verbose, as it requires specifying the type, which, for
// this method, comes from the receiver.
//
// See also [logiface.NewWriterFunc].
func (LoggerFactory[E]) NewWriterFunc(f func(event E) error) WriterFunc[E] { return f }

// NewModifierSlice is an alias provided as a convenience, to make it easier to initialize a ModifierSlice.
//
// See also [logiface.NewModifierSlice].
func (LoggerFactory[E]) NewModifierSlice(s ...Modifier[E]) ModifierSlice[E] { return s }

// NewWriterSlice is an alias provided as a convenience, to make it easier to initialize a WriterSlice.
//
// See also [logiface.NewWriterSlice].
func (LoggerFactory[E]) NewWriterSlice(s ...Writer[E]) WriterSlice[E] { return s }
