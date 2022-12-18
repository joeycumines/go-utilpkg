package zerolog

import (
	"github.com/joeycumines/go-utilpkg/logiface"
	"github.com/rs/zerolog"
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
	r := Event{
		lvl: level,
	}
	switch level {
	case logiface.LevelTrace:
		r.Z = x.Z.Trace()
	case logiface.LevelDebug:
		r.Z = x.Z.Debug()
	case logiface.LevelInformational:
		r.Z = x.Z.Info()
	case logiface.LevelNotice:
		r.Z = x.Z.Warn()
	case logiface.LevelWarning:
		r.Z = x.Z.Warn()
	case logiface.LevelError:
		r.Z = x.Z.Error()
	case logiface.LevelCritical:
		r.Z = x.Z.Fatal()
	case logiface.LevelAlert:
		r.Z = x.Z.Fatal()
	case logiface.LevelEmergency:
		r.Z = x.Z.Panic()
	default:
		// >= 9, translate to numeric levels in zerolog
		// (9 -> -2, 10 -> -3, etc)
		// WARNING: there are 8 levels unaddressable using this mechanism
		r.Z = x.Z.WithLevel(zerolog.Level(7 - level))
	}
	return &r
}

func (x *Logger) Write(event *Event) error {
	event.Z.Msg(event.msg)
	return nil
}
