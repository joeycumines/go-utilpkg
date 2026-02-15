package slog

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/joeycumines/logiface"
)

// Logger implements logiface.EventFactory, logiface.Writer, and logiface.EventReleaser.
// It writes log events to a slog.Handler.
type Logger struct {
	// handler is the underlying slog.Handler that receives events
	handler slog.Handler

	// defaultCtx is the default context for events without context
	defaultCtx context.Context

	// replaceAttr is a hook for transforming attributes before output
	replaceAttr func([]string, slog.Attr) slog.Attr

	// defaultAttrs are attributes added to all events
	defaultAttrs []slog.Attr

	// groupStack tracks the current group prefix hierarchy
	groupStack []string

	// level is the minimum enabled log level
	level logiface.Level
}

// eventPool handles reuse of Event instances for performance.
var eventPool = sync.Pool{
	New: func() any {
		return &Event{
			// Pre-allocate slices with reasonable capacity
			attrs:  make([]slog.Attr, 0, 16),
			groups: make([]string, 0, 4),
		}
	},
}

// NewLogger creates a Logger that writes to provided slog.Handler.
// Panics if handler is nil.
//
// Usage:
//
//	handler := slog.NewJSONHandler(os.Stdout, nil)
//	logger := logiface.New[*Event](NewLogger(handler))
func NewLogger(handler slog.Handler, opts ...Option) logiface.Option[*Event] {
	if handler == nil {
		panic("handler cannot be nil")
	}

	slogger := &Logger{
		handler:      handler,
		defaultCtx:   context.Background(),
		defaultAttrs: make([]slog.Attr, 0, 8),
		groupStack:   make([]string, 0, 4),
		level:        logiface.LevelTrace, // Allow all levels by default
	}

	// Apply our custom options to slogger
	for _, opt := range opts {
		opt(slogger)
	}

	// Build composite option using standard logiface helpers
	return logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
		logiface.WithLevel[*Event](slogger.level),
	)
}

// NewEvent creates a new Event from pool with specified logiface level.
// Returns nil if level is disabled.
func (x *Logger) NewEvent(level logiface.Level) *Event {
	// For forward direction (logiface → slog), let the slog.Handler
	// do the level filtering. Don't pre-filter here.
	event := eventPool.Get().(*Event)

	// Initialize event
	event.logger = x
	event.time = time.Now()
	event.slogLevel = toSlogLevel(level)

	// Reset slices to zero length, preserve capacity for reuse
	event.attrs = event.attrs[:0]
	event.groups = event.groups[:0]

	// Copy group stack
	if len(x.groupStack) > 0 {
		event.groups = append(event.groups, x.groupStack...)
	}

	// Set context to default
	event.ctx = x.defaultCtx

	return event
}

// Write emits event via underlying slog.Handler.
func (x *Logger) Write(event *Event) error {
	if event == nil {
		return nil
	}

	// Apply ReplaceAttr hook to all attributes
	if x.replaceAttr != nil {
		groupPrefix := event.getGroupPrefix()
		for i := range event.attrs {
			event.attrs[i] = x.replaceAttr(groupPrefix, event.attrs[i])
		}
	}

	return event.Send()
}

// ReleaseEvent returns Event to pool for reuse.
func (x *Logger) ReleaseEvent(event *Event) {
	if event != nil {
		// Reset clears all fields
		event.Reset()
		eventPool.Put(event)
	}
}

// CanAddRawJSON returns true, indicating raw JSON support.
func (x *Logger) CanAddRawJSON() bool {
	return true
}

// CanAddFields returns true, indicating field addition support.
func (x *Logger) CanAddFields() bool {
	return true
}

// CanAddLazyFields returns true, indicating lazy field evaluation support.
func (x *Logger) CanAddLazyFields() bool {
	return true
}

// Close flushes any buffered state - returns nil as slog.Handler does not have close semantics.
func (x *Logger) Close() error {
	return nil
}

// getGroupPrefix returns the current group prefix as a string slice
// Handles empty group stack gracefully
func (x *Event) getGroupPrefix() []string {
	if len(x.groups) == 0 {
		return nil
	}
	prefix := make([]string, len(x.groups))
	copy(prefix, x.groups)
	return prefix
}
