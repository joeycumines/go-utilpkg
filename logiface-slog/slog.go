package islog

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/joeycumines/logiface"
)

type (
	// Event implements logiface.Event using log/slog.
	Event struct {
		//lint:ignore U1000 embedded for it's methods
		unimplementedEvent
		msg   string
		attrs []slog.Attr
		lvl   logiface.Level
	}

	// Logger implements logiface.Writer, EventFactory, and EventReleaser using slog.Handler.
	Logger struct {
		Handler slog.Handler
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
		return &Event{
			attrs: make([]slog.Attr, 0, 8),
		}
	}}
)

// WithSlogHandler configures a logiface logger to use a slog handler.
//
// See also LoggerFactory.WithSlogHandler and L (an alias for LoggerFactory{}).
func WithSlogHandler(handler slog.Handler) logiface.Option[*Event] {
	if handler == nil {
		panic(`handler cannot be nil`)
	}
	l := &Logger{
		Handler: handler,
	}
	return logiface.WithOptions(
		logiface.WithWriter[*Event](l),
		logiface.WithEventFactory[*Event](l),
		logiface.WithEventReleaser[*Event](l),
		logiface.WithLevel[*Event](logiface.LevelInformational), // Default: filter Debug/Trace
	)
}

// WithSlogHandler is an alias of the package function of the same name.
func (LoggerFactory) WithSlogHandler(handler slog.Handler) logiface.Option[*Event] {
	return WithSlogHandler(handler)
}

func (x *Event) Level() logiface.Level {
	if x != nil {
		return x.lvl
	}
	return logiface.LevelDisabled
}

func (x *Event) AddField(key string, val any) {
	x.attrs = append(x.attrs, slog.Any(key, val))
}

func (x *Event) AddMessage(msg string) bool {
	x.msg = msg
	return true
}

func (x *Event) AddError(err error) bool {
	if err != nil {
		x.attrs = append(x.attrs, slog.Any("error", err))
	}
	return true
}

// AddGroup returns false; slog requires attributes with groups, so keys are flattened instead.
func (x *Event) AddGroup(name string) bool {
	return false
}

func (x *Logger) NewEvent(level logiface.Level) *Event {
	event := eventPool.Get().(*Event)
	event.lvl = level
	event.attrs = event.attrs[:0]
	event.msg = ""
	return event
}

func (x *Logger) ReleaseEvent(event *Event) {
	// need to be able to handle default values, because NewEvent may return nil
	if event != nil {
		event.lvl = 0
		event.msg = ""
		event.attrs = event.attrs[:0]
		eventPool.Put(event)
	}
}

func (x *Logger) Write(event *Event) error {
	record := slog.NewRecord(time.Now(), toSlogLevel(event.lvl), event.msg, 0)
	record.AddAttrs(event.attrs...)
	return x.Handler.Handle(context.TODO(), record)
}

func toSlogLevel(level logiface.Level) slog.Level {
	switch level {
	case logiface.LevelTrace, logiface.LevelDebug:
		return slog.LevelDebug
	case logiface.LevelInformational:
		return slog.LevelInfo
	case logiface.LevelNotice, logiface.LevelWarning:
		return slog.LevelWarn
	case logiface.LevelError, logiface.LevelCritical, logiface.LevelAlert, logiface.LevelEmergency:
		return slog.LevelError
	default:
		return slog.LevelDebug
	}
}
