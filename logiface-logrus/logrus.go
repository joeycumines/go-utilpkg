// Package ilogrus implements support for using github.com/sirupsen/logrus with github.com/joeycumines/logiface.
package ilogrus

import (
	"github.com/joeycumines/logiface"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
	"sync"
)

type (
	Event struct {
		Entry *logrus.Entry
		lvl   logiface.Level
		//lint:ignore U1000 embedded for it's methods
		unimplementedEvent
	}

	Logger struct {
		Logrus *logrus.Logger
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
	baseLoggerFactory = logiface.LoggerFactory[*Event]
)

var (
	// L is a LoggerFactory, and may be used to configure a
	// logiface.Logger[*Event], using the implementations provided by this
	// package.
	L = LoggerFactory{}

	eventPool = sync.Pool{New: func() any {
		return &Event{Entry: &logrus.Entry{
			Data: make(logrus.Fields, 6),
		}}
	}}
)

// WithLogrus configures a logiface logger to use a logrus logger.
// Will panic if the logger is nil.
//
// See also LoggerFactory.WithLogrus and L (an alias for LoggerFactory{}).
func WithLogrus(logger *logrus.Logger) logiface.Option[*Event] {
	if logger == nil {
		panic(`nil logger`)
	}
	l := Logger{Logrus: logger}
	return L.WithOptions(
		L.WithWriter(&l),
		L.WithEventFactory(&l),
		L.WithEventReleaser(&l),
	)
}

// WithLogrus is an alias of the package function of the same name.
func (LoggerFactory) WithLogrus(logger *logrus.Logger) logiface.Option[*Event] {
	return WithLogrus(logger)
}

func (x *Event) Level() logiface.Level {
	if x != nil {
		return x.lvl
	}
	return logiface.LevelDisabled
}

func (x *Event) AddField(key string, val any) {
	// note: perform logrus.Entry.WithFields later, just prior to logging
	x.Entry.Data[key] = val
}

func (x *Event) AddMessage(msg string) bool {
	// use the entry message storage (it'll be overwritten with itself, later)
	x.Entry.Message = msg
	return true
}

func (x *Event) AddError(err error) bool {
	// consistent with logrus.Entry.WithError
	x.Entry.Data[logrus.ErrorKey] = err
	return true
}

func (x *Logger) NewEvent(level logiface.Level) *Event {
	// we _could_ check if the log level is enabled in the _logrus_ logger,
	// but, unlike the zerolog implementation, the entry could potentially be
	// used in conjunction with an external writer, since the check is only
	// on write (not on builder init)

	event := eventPool.Get().(*Event)
	event.lvl = level
	event.Entry.Logger = x.Logrus

	return event
}

func (x *Logger) ReleaseEvent(event *Event) {
	maps.Clear(event.Entry.Data)
	*event.Entry = logrus.Entry{Data: event.Entry.Data}
	*event = Event{Entry: event.Entry}
	eventPool.Put(event)
}

func (x *Logger) Write(event *Event) error {
	level := event.Level()

	// TODO consider strategy for supporting and/or exposing custom levels
	logrusLevel, ok := toLogrusLevel(level)
	if !ok || !event.Entry.Logger.IsLevelEnabled(logrusLevel) {
		// this lets other writers (e.g. in a logiface.WriterSlice) attempt to
		// handle the event
		return logiface.ErrDisabled
	}

	// normalise the log fields... kinda cooked and allocates but whatever
	fields := event.Entry.Data
	event.Entry.Data = nil
	entry := event.Entry.WithFields(fields)
	event.Entry.Data = fields

	// log the entry
	entry.Log(logrusLevel, event.Entry.Message)

	return nil
}

// toLogrusLevel maps logiface.Level to logrus.Level.
//
// See also the recommended mappings documented on logiface.Level.
func toLogrusLevel(level logiface.Level) (logrus.Level, bool) {
	switch level {
	case logiface.LevelTrace:
		return logrus.TraceLevel, true

	case logiface.LevelDebug:
		return logrus.DebugLevel, true

	case logiface.LevelInformational:
		return logrus.InfoLevel, true

	case logiface.LevelNotice:
		return logrus.WarnLevel, true

	case logiface.LevelWarning:
		return logrus.WarnLevel, true

	case logiface.LevelError:
		return logrus.ErrorLevel, true

	case logiface.LevelCritical:
		return logrus.ErrorLevel, true

	case logiface.LevelAlert:
		return logrus.FatalLevel, true

	case logiface.LevelEmergency:
		return logrus.PanicLevel, true

	default:
		return logrus.PanicLevel, false
	}
}
