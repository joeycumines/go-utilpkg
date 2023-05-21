// Package izerolog implements support for using github.com/rs/zerolog with github.com/joeycumines/logiface.
package izerolog

import (
	"github.com/joeycumines/logiface"
	"github.com/rs/zerolog"
	"sync"
	"time"
)

type (
	Event struct {
		//lint:ignore U1000 embedded for it's methods
		unimplementedEvent
		Z   *zerolog.Event
		msg string
		lvl logiface.Level
	}

	Logger struct {
		//lint:ignore U1000 embedded for it's methods
		unimplementedJSONSupport
		Z zerolog.Logger
	}

	// LoggerFactory is provided as a convenience, embedding
	// logiface.LoggerFactory[*Event], and aliasing the option functions
	// implemented within this package.
	LoggerFactory struct {
		//lint:ignore U1000 embedded for it's methods
		baseLoggerFactory
	}

	//lint:ignore U1000 used to embed without exporting
	unimplementedEvent = logiface.UnimplementedEvent

	//lint:ignore U1000 used to embed without exporting
	unimplementedJSONSupport = logiface.UnimplementedJSONSupport[*Event, *zerolog.Event, *zerolog.Array]

	//lint:ignore U1000 used to embed without exporting
	baseLoggerFactory = logiface.LoggerFactory[*Event]
)

var (
	// L is a LoggerFactory, and may be used to configure a
	// logiface.Logger[*Event], using the implementations provided by this
	// package.
	L = LoggerFactory{}

	eventPool = sync.Pool{New: func() any { return new(Event) }}
)

// WithZerolog configures a logiface logger to use a zerolog logger.
//
// See also LoggerFactory.WithZerolog and L (an alias for LoggerFactory{}).
func WithZerolog(logger zerolog.Logger) logiface.Option[*Event] {
	l := Logger{Z: logger}
	return L.WithOptions(
		L.WithWriter(&l),
		L.WithEventFactory(&l),
		L.WithEventReleaser(&l),
		logiface.WithJSONSupport[*Event, *zerolog.Event, *zerolog.Array](&l),
	)
}

// WithZerolog is an alias of the package function of the same name.
func (LoggerFactory) WithZerolog(logger zerolog.Logger) logiface.Option[*Event] {
	return WithZerolog(logger)
}

func (x *Event) Level() logiface.Level {
	if x != nil {
		return x.lvl
	}
	return logiface.LevelDisabled
}

func (x *Event) AddField(key string, val any) {
	x.Z.Interface(key, val)
}

func (x *Event) AddMessage(msg string) bool {
	x.msg = msg
	return true
}

func (x *Event) AddError(err error) bool {
	x.Z.Err(err)
	return true
}

func (x *Event) AddString(key string, val string) bool {
	x.Z.Str(key, val)
	return true
}

func (x *Event) AddInt(key string, val int) bool {
	x.Z.Int(key, val)
	return true
}

func (x *Event) AddFloat32(key string, val float32) bool {
	x.Z.Float32(key, val)
	return true
}

func (x *Event) AddTime(key string, val time.Time) bool {
	x.Z.Time(key, val)
	return true
}

func (x *Event) AddDuration(key string, val time.Duration) bool {
	x.Z.Dur(key, val)
	return true
}

func (x *Event) AddBool(key string, val bool) bool {
	x.Z.Bool(key, val)
	return true
}

func (x *Event) AddFloat64(key string, val float64) bool {
	x.Z.Float64(key, val)
	return true
}

func (x *Event) AddInt64(key string, val int64) bool {
	x.Z.Int64(key, val)
	return true
}

func (x *Event) AddUint64(key string, val uint64) bool {
	x.Z.Uint64(key, val)
	return true
}

func (x *Logger) NewEvent(level logiface.Level) *Event {
	// map the levels, initialize the zerolog.Event
	z := x.newEvent(level)
	if z == nil {
		// no point in allocating an event, it won't be able to do anything
		// useful, anyway
		return nil
	}

	event := eventPool.Get().(*Event)
	event.lvl = level
	event.Z = z

	return event
}

func (x *Logger) ReleaseEvent(event *Event) {
	// need to be able to handle default values, because NewEvent may return nil
	if event != nil {
		*event = Event{}
		eventPool.Put(event)
	}
}

func (x *Logger) Write(event *Event) error {
	event.Z.Msg(event.msg)
	return nil
}

// newEvent maps the logiface levels to zerolog levels
// see also the recommended mappings documented on logiface.Level
func (x *Logger) newEvent(level logiface.Level) *zerolog.Event {
	switch level {
	case logiface.LevelTrace:
		return x.Z.Trace()

	case logiface.LevelDebug:
		return x.Z.Debug()

	case logiface.LevelInformational:
		return x.Z.Info()

	case logiface.LevelNotice:
		return x.Z.Warn()

	case logiface.LevelWarning:
		return x.Z.Warn()

	case logiface.LevelError:
		return x.Z.Error()

	case logiface.LevelCritical:
		return x.Z.Error()

	case logiface.LevelAlert:
		return x.Z.Fatal()

	case logiface.LevelEmergency:
		return x.Z.Panic()

	default:
		// >= 9, translate to numeric levels in zerolog
		// (9 -> -2, 10 -> -3, etc)
		// WARNING: there are 8 (zerolog) levels unaddressable using this mechanism
		return x.Z.WithLevel(zerolog.Level(7 - level))
	}
}

func (x *Logger) NewObject() *zerolog.Event {
	return zerolog.Dict()
}

func (x *Logger) AddObject(evt *Event, key string, obj *zerolog.Event) {
	evt.Z.Dict(key, obj)
}

func (x *Logger) SetField(obj *zerolog.Event, key string, val any) *zerolog.Event {
	return obj.Interface(key, val)
}

func (x *Logger) CanSetObject() bool { return true }

func (x *Logger) SetObject(obj *zerolog.Event, key string, val *zerolog.Event) *zerolog.Event {
	return obj.Dict(key, val)
}

func (x *Logger) CanSetArray() bool { return true }

func (x *Logger) SetArray(obj *zerolog.Event, key string, val *zerolog.Array) *zerolog.Event {
	return obj.Array(key, val)
}

func (x *Logger) CanSetString() bool { return true }

func (x *Logger) SetString(obj *zerolog.Event, key string, val string) *zerolog.Event {
	return obj.Str(key, val)
}

func (x *Logger) CanSetBool() bool { return true }

func (x *Logger) SetBool(obj *zerolog.Event, key string, val bool) *zerolog.Event {
	return obj.Bool(key, val)
}

func (x *Logger) CanSetDuration() bool { return true }

func (x *Logger) SetDuration(obj *zerolog.Event, key string, d time.Duration) *zerolog.Event {
	return obj.Dur(key, d)
}

func (x *Logger) CanSetError() bool { return true }

func (x *Logger) SetError(obj *zerolog.Event, err error) *zerolog.Event {
	return obj.Err(err)
}

func (x *Logger) CanSetInt() bool { return true }

func (x *Logger) SetInt(obj *zerolog.Event, key string, val int) *zerolog.Event {
	return obj.Int(key, val)
}

func (x *Logger) CanSetFloat32() bool { return true }

func (x *Logger) SetFloat32(obj *zerolog.Event, key string, val float32) *zerolog.Event {
	return obj.Float32(key, val)
}

func (x *Logger) CanSetTime() bool { return true }

func (x *Logger) SetTime(obj *zerolog.Event, key string, t time.Time) *zerolog.Event {
	return obj.Time(key, t)
}

func (x *Logger) CanSetFloat64() bool { return true }

func (x *Logger) SetFloat64(obj *zerolog.Event, key string, val float64) *zerolog.Event {
	return obj.Float64(key, val)
}

func (x *Logger) CanSetInt64() bool { return true }

func (x *Logger) SetInt64(obj *zerolog.Event, key string, val int64) *zerolog.Event {
	return obj.Int64(key, val)
}

func (x *Logger) CanSetUint64() bool { return true }

func (x *Logger) SetUint64(obj *zerolog.Event, key string, val uint64) *zerolog.Event {
	return obj.Uint64(key, val)
}

func (x *Logger) NewArray() *zerolog.Array { return zerolog.Arr() }

func (x *Logger) AddArray(evt *Event, key string, arr *zerolog.Array) {
	evt.Z.Array(key, arr)
}

func (x *Logger) AppendField(arr *zerolog.Array, val any) *zerolog.Array {
	return arr.Interface(val)
}

func (x *Logger) CanAppendObject() bool { return true }

func (x *Logger) AppendObject(arr *zerolog.Array, val *zerolog.Event) *zerolog.Array {
	return arr.Dict(val)
}

func (x *Logger) CanAppendString() bool { return true }

func (x *Logger) AppendString(arr *zerolog.Array, val string) *zerolog.Array {
	return arr.Str(val)
}

func (x *Logger) CanAppendBool() bool { return true }

func (x *Logger) AppendBool(arr *zerolog.Array, val bool) *zerolog.Array {
	return arr.Bool(val)
}

func (x *Logger) CanAppendDuration() bool { return true }

func (x *Logger) AppendDuration(arr *zerolog.Array, d time.Duration) *zerolog.Array {
	return arr.Dur(d)
}

func (x *Logger) CanAppendError() bool { return true }

func (x *Logger) AppendError(arr *zerolog.Array, err error) *zerolog.Array {
	return arr.Err(err)
}

func (x *Logger) CanAppendInt() bool { return true }

func (x *Logger) AppendInt(arr *zerolog.Array, val int) *zerolog.Array {
	return arr.Int(val)
}

func (x *Logger) CanAppendFloat32() bool { return true }

func (x *Logger) AppendFloat32(arr *zerolog.Array, val float32) *zerolog.Array {
	return arr.Float32(val)
}

func (x *Logger) CanAppendTime() bool { return true }

func (x *Logger) AppendTime(arr *zerolog.Array, t time.Time) *zerolog.Array {
	return arr.Time(t)
}

func (x *Logger) CanAppendFloat64() bool { return true }

func (x *Logger) AppendFloat64(arr *zerolog.Array, val float64) *zerolog.Array {
	return arr.Float64(val)
}

func (x *Logger) CanAppendInt64() bool { return true }

func (x *Logger) AppendInt64(arr *zerolog.Array, val int64) *zerolog.Array {
	return arr.Int64(val)
}

func (x *Logger) CanAppendUint64() bool { return true }

func (x *Logger) AppendUint64(arr *zerolog.Array, val uint64) *zerolog.Array {
	return arr.Uint64(val)
}
