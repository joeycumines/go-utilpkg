package zerolog

import (
	"github.com/joeycumines/go-utilpkg/logiface"
	"github.com/rs/zerolog"
	"sync"
)

type (
	Event struct {
		Z   *zerolog.Event
		lvl logiface.Level
		msg string
	}

	Logger struct {
		Z zerolog.Logger
	}
)

var (
	// Pool is provided as a companion to Event.
	// It is used by Logger. If you use multiple writers, you may want to
	// ensure that the Event is returned to the pool.
	Pool = sync.Pool{New: func() any { return new(Event) }}

	// compile time assertions

	_ logiface.Event              = (*Event)(nil)
	_ logiface.LoggerImpl[*Event] = (*Logger)(nil)
)

func (x *Event) Level() logiface.Level {
	if x != nil {
		return x.lvl
	}
	return logiface.LevelDisabled
}

func (x *Event) SetMessage(msg string) {
	x.msg = msg
}

func (x *Event) AddField(key string, val any) {
	x.Z.Interface(key, val)
}

func (x *Logger) NewEvent(level logiface.Level) *Event {
	if !level.Enabled() {
		return nil
	}
	var z *zerolog.Event
	switch level {
	case logiface.LevelTrace:
		z = x.Z.Trace()
	case logiface.LevelDebug:
		z = x.Z.Debug()
	case logiface.LevelInformational:
		z = x.Z.Info()
	case logiface.LevelNotice:
		z = x.Z.Warn()
	case logiface.LevelWarning:
		z = x.Z.Warn()
	case logiface.LevelError:
		z = x.Z.Error()
	case logiface.LevelCritical:
		z = x.Z.Fatal()
	case logiface.LevelAlert:
		z = x.Z.Fatal()
	case logiface.LevelEmergency:
		z = x.Z.Panic()
	default:
		// >= 9, translate to numeric levels in zerolog
		// (9 -> -2, 10 -> -3, etc)
		// WARNING: there are 8 levels unaddressable using this mechanism
		z = x.Z.WithLevel(zerolog.Level(7 - level))
	}
	event := Pool.Get().(*Event)
	event.lvl = level
	event.Z = z
	return event
}

func (x *Logger) Write(event *Event) error {
	event.Z.Msg(event.msg)
	Pool.Put(event)
	return nil
}
