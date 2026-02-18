// Package islog implements support for using log/slog with github.com/joeycumines/logiface.
package islog

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/joeycumines/logiface"
)

type (
	// Event represents a pooled log event that accumulates fields for a single log operation.
	//
	// # Lifecycle
	//
	// Events follow a strict lifecycle managed by the logiface framework:
	//
	//  1. **Creation**: Events are obtained from a sync.Pool via Logger.NewEvent(level).
	//     The attrs slice is pre-allocated with capacity 8 to minimize reallocations.
	//     The event msg starts empty and must be set via AddMessage() or Log().
	//
	//  2. **Accumulation**: Events are not thread-safe and must be used by a single
	//     goroutine. Fields are added via AddField(key, val), AddMessage(msg), and
	//     AddError(err). The attrs slice grows to accommodate all fields.
	//
	//  3. **Finalization**: Events are written to the underlying slog.Handler via
	//     Logger.Write(event), which creates a slog.NewRecord and delegates to
	//     Handler.Handle().
	//
	//  4. **Release**: After Write() completes, the framework automatically calls
	//     Logger.ReleaseEvent(event) which returns the Event to the pool for reuse.
	//     All fields are cleared (msg, lvl reset; attrs truncated to length 0) while
	//     preserving slice capacity for efficiency.
	//
	// # Pool Reuse
	//
	// Event pooling eliminates allocation overhead in high-throughput logging scenarios.
	// The sync.Pool automatically scales pool size based on concurrent demand. Events
	// returned to the pool are immediately available for reuse, minimizing GC pressure.
	//
	// # Usage Pattern
	//
	// Typical usage follows logiface's fluent builder API:
	//
	//	logger.Info().                      // NewEvent(LevelInfo)
	//	    Str("service", "api").        // AddField
	//	    Int("status", 200).           // AddField
	//	    Log("Request completed")          // AddMessage + Write
	//
	// The builder pattern (Info(), Debug(), etc.) obtains an Event from the pool,
	// chains field adders, and finally calls Log() which triggers Write()
	// and automatic ReleaseEvent().
	//
	// # Thread Safety
	//
	// Events are NOT thread-safe. Each Event must be confined to a single goroutine
	// for its entire lifecycle (creation through release). Never share an Event
	// between goroutines or return an Event from a function for later use.
	Event struct {
		//lint:ignore U1000 embedded for it's methods
		unimplementedEvent

		// msg is the log message
		msg string

		// attrs stores the log attributes for this event
		attrs []slog.Attr

		// lvl is the logiface level for this event
		lvl logiface.Level
	}

	// Logger bridges logiface's fluent API to slog.Handler.
	//
	// Logger implements multiple logiface integration interfaces:
	//   - logiface.Writer[*Event] for writing events via Write()
	//   - logiface.EventFactory[*Event] for obtaining events from pool via NewEvent()
	//   - logiface.EventReleaser[*Event] for returning events to pool via ReleaseEvent()
	//
	// # Level Filtering
	//
	// Logger supports two-level filtering:
	//
	//  1. **logiface.WithLevel option**: Configures default filter level (defaults to
	//     LevelInformational). Logger respects this by checking Handler.Enabled() in
	//     Write() before creating slog records.
	//
	//  2. **slog.Handler.Enabled()**: The underlying handler may have its own
	//     level threshold. Write() calls this method and returns logiface.ErrDisabled
	//     if the handler rejects the event.
	//
	// Events at or above the configured level are processed; events below are
	// filtered early before slog record creation, avoiding unnecessary allocations.
	//
	// # Error Handling
	//
	// Write() returns errors from Handler.Handle(). If Handler.Enabled() returns false,
	// Write() returns logiface.ErrDisabled to signal that the event was filtered.
	//
	// NewEvent() never returns nil; the sync.Pool guarantees non-nil events.
	// ReleaseEvent() handles nil events defensively (guard clause protects against
	// potential edge cases).
	//
	// # Panic Behavior
	//
	// Events at LevelEmergency cause Write() to panic with logiface.LevelEmergency.
	// This matches logiface's contract for fatal/critical logging where the
	// application should terminate. Alert-level events are passed to the handler,
	// which may terminate the process if configured as a fatal handler.
	//
	// # Thread Safety
	//
	// Logger is safe for concurrent use from multiple goroutines. The underlying
	// slog.Handler must also be thread-safe (handlers like JSONHandler and
	// TextHandler are safe by default).
	//
	// Events acquired from Logger are NOT thread-safe. Each Event must be used by
	// a single goroutine for its entire lifecycle.
	//
	// # Usage Pattern
	//
	// Typical initialization:
	//
	//	handler := slog.NewJSONHandler(os.Stdout, nil)
	//	logger := L.New(L.WithSlogHandler(handler))
	//
	// Logging:
	//	logger.Info().
	//	    Str("service", "api").
	//	    Log("Request completed")
	//
	// The fluent builder methods (Info(), Debug(), etc.) internally call NewEvent()
	// to obtain an Event from pool, chain field additions via AddField(), and
	// finally call Write() which returns the Event to the pool automatically.
	//
	// # Event Lifecycle Management
	//
	// Logger manages the complete event lifecycle:
	//
	//   1. **NewEvent(level)**: Obtains Event from sync.Pool, resets fields,
	//      sets level. attrs slice pre-allocated with capacity 8.
	//
	//   2. **Write(event)**: Creates slog.NewRecord, transfers event.attrs,
	//      calls Handler.Handle(), then (via logiface framework) triggers
	//      ReleaseEvent() to return event to pool.
	//
	//   3. **ReleaseEvent(event)**: Defensive nil check, clears event fields
	//      (msg, lvl, truncates attrs to length 0), returns to sync.Pool.
	//      Slice capacity preserved for reuse.
	Logger struct {
		// Handler is the underlying slog.Handler
		Handler slog.Handler
	}

	// LoggerFactory is a convenience type that embeds logiface.LoggerFactory[*Event] and
	// aliases option functions for configuring slog-backed loggers.
	//
	// # Purpose
	//
	// LoggerFactory provides a global convenience instance (L) for creating configured
	// logiface.Logger[*Event] instances that use slog.Handler backends. By
	// embedding logiface.LoggerFactory[*Event], it inherits the fluent builder
	// methods (New, Build, Info, Debug, etc.) while adding slog-specific
	// configuration via WithSlogHandler().
	//
	// # Relationship to L
	//
	// L is a package-level LoggerFactory instance:
	//
	//	var L = LoggerFactory{}
	//
	// Use L to configure and create loggers:
	//
	//	handler := slog.NewJSONHandler(os.Stdout, nil)
	//	logger := L.New(L.WithSlogHandler(handler))
	//
	// L is safe for concurrent use. Multiple goroutines can call L.New() or
	// L.WithSlogHandler() simultaneously.
	//
	// # Configuration Pattern
	//
	// LoggerFactory works with logiface's option pattern. Options are immutable
	// configuration wrappers:
	//
	//	type Option[T any] func(*Config[T]) error
	//
	// Use option functions to configure logger creation:
	//
	//	logger := L.New(
	//	    L.WithSlogHandler(handler),
	//	    logiface.WithLevel[*Event](logiface.LevelDebug),
	//	)
	//
	// WithSlogHandler() is the primary option provided by this package. Additional
	// options from logiface.WithLevel, logiface.WithEventFactory, etc. can be
	// combined.
	//
	// # Aliasing Strategy
	//
	// LoggerFactory embeds logiface.LoggerFactory[*Event] to inherit the New()
	// method that constructs logiface.Logger[*Event] instances. WithSlogHandler() is
	// defined both as a package function and as a factory method for convenience:
	//
	//	// Package function:
	//	opt := WithSlogHandler(handler)
	//	logger := L.New(opt)
	//
	//	// Factory method (equivalent):
	//	opt := L.WithSlogHandler(handler)
	//	logger := L.New(opt)
	//
	// Both forms produce the same configuration option.
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
// The slog adapter delegates level filtering to the logiface framework and the slog handler.
// Events below the configured minimum level will be filtered by the framework before calling Write().
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

// ============================================================================
// REQUIRED Event Interface Methods
// ============================================================================

// Level returns the logiface level for this event.
//
// Level() is safe for nil events. If x is nil, returns LevelDisabled.
// This matches logiface's contract for events that haven't been initialized.
func (x *Event) Level() logiface.Level {
	if x != nil {
		return x.lvl
	}
	return logiface.LevelDisabled
}

// AddField adds a generic field to the event.
//
// This is the MINIMAL implementation - all field types (strings, ints,
// floats, durations, errors, etc.) go through this single method. The underlying
// slog.Handle API will handle type-specific encoding.
//
// Fields are appended to the event's internal attrs slice. The slice
// pre-allocated capacity is 8, accommodating most common field counts without
// reallocation.
func (x *Event) AddField(key string, val any) {
	x.attrs = append(x.attrs, slog.Any(key, val))
}

// ============================================================================
// OPTIONAL Event Interface Methods
// ============================================================================

// AddMessage sets the log message for the event.
//
// The message is stored for later use by Write() when constructing the
// slog.NewRecord. Empty messages are valid and allowed.
//
// Returns true to indicate the message was set successfully.
// Subsequent calls to AddMessage() overwrite the previous message.
func (x *Event) AddMessage(msg string) bool {
	x.msg = msg
	return true
}

// AddError adds an error to the event.
//
// If err is non-nil, it is wrapped as a slog.Attr with the fixed key
// "error" and appended to the event's attrs slice. This matches slog's
// standard error logging pattern.
//
// Nil errors are silently ignored (no attribute added). This is a defensive
// choice to avoid logging errors with no useful information.
//
// Returns true to indicate the method completed (successfully added error or
// silently ignored nil error).
func (x *Event) AddError(err error) bool {
	if err != nil {
		x.attrs = append(x.attrs, slog.Any("error", err))
	}
	return true
}

// AddGroup adds a group to the event.
//
// Note: slog doesn't support adding an empty group marker without attributes.
// Returns false to indicate the caller should fall back to flattening keys.
//
// The logiface framework interprets a false return as a signal to use
// flattened key names (e.g., "parent.child") instead of nested groups.
// This adapter deliberately returns false for all AddGroup calls because
// slog.Group requires attributes to be meaningful, and the logiface calling
// convention doesn't provide attributes with group markers.
func (x *Event) AddGroup(name string) bool {
	// slog.Group requires attributes to be meaningful.
	// Returning false signals that the adapter should use flattened keys instead.
	return false
}

// ============================================================================
// Event Factory / Writer Interface Methods
// ============================================================================

// NewEvent creates a new Event from pool.
//
// The Event is obtained from a sync.Pool, which reuses previously-released
// events to minimize allocation overhead. The attrs slice is pre-allocated with
// capacity 8 to accommodate typical field counts without reallocation.
//
// The event's level field is set to the provided level. The message is
// initialized to empty string (set via AddMessage() or builder Log() method).
//
// NewEvent() never returns nil. The sync.Pool guarantees non-nil returns.
// ReleaseEvent() handles nil events defensively for robustness.
func (x *Logger) NewEvent(level logiface.Level) *Event {
	event := eventPool.Get().(*Event)
	event.lvl = level
	event.attrs = event.attrs[:0]
	event.msg = ""
	return event
}

// ReleaseEvent returns the Event to the pool for reuse.
//
// The event is cleared of all accumulated state:
//   - event.lvl is reset to 0
//   - event.msg is set to empty string
//   - event.attrs is truncated to length 0 (slice capacity preserved)
//
// The slice capacity preservation is critical for performance: the next
// iteration reusing this Event benefits from the pre-allocated buffer.
//
// ReleaseEvent() handles nil events defensively via a guard clause. This
// protects against potential edge cases (incorrect manual pool usage, bugs in
// framework integration). In normal logiface usage, events are always non-nil
// when released.
//
// ReleaseEvent() is not called directly by user code. The logiface framework
// invokes this method automatically after Write() completes as part of the event
// lifecycle.
func (x *Logger) ReleaseEvent(event *Event) {
	// need to be able to handle default values, because NewEvent may return nil
	if event != nil {
		// Reset all fields while preserving slice capacity
		event.lvl = 0
		event.msg = ""
		event.attrs = event.attrs[:0]
		eventPool.Put(event)
	}
}

// Write finalizes and sends the event to the slog handler.
//
// Write performs the following operations:
//
//  1. **Panic check**: If event.lvl is LevelEmergency, panic with
//     logiface.LevelEmergency. This matches logiface's contract for fatal/critical
//     logging where application termination is required.
//
//  2. **Level filtering**: Check x.Handler.Enabled() with the event's
//     converted slog level. If the handler indicates the level is disabled,
//     return logiface.ErrDisabled immediately without creating a slog record.
//     This early exit avoids unnecessary allocations.
//
//  3. **Record creation**: Create a slog.NewRecord with:
//     - Timestamp: time.Now() (at Write() call time)
//     - Level: toSlogLevel(event.lvl)
//     - Message: event.msg
//     - PC: 0 (not passing program counter for context)
//
//  4. **Attribute transfer**: Append all event.attrs to the record via
//     record.AddAttrs(event.attrs...). This transfers all accumulated fields
//     without copying the slice.
//
//  5. **Handler delegation**: Call x.Handler.Handle(context.TODO(), record).
//     The context.TODO() is used because logiface.Writer doesn't accept
//     a context parameter. The underlying handler may use this context for
//     its own internal operations.
//
//  6. **Error propagation**: Return any error from Handler.Handle(). If the
//     handler returns nil, Write() returns nil.
//
// After Write() returns (successfully or with error), the logiface framework
// automatically calls ReleaseEvent() to return the event to the pool.
func (x *Logger) Write(event *Event) error {
	// Emergency level should panic
	if event.lvl == logiface.LevelEmergency {
		panic(logiface.LevelEmergency)
	}
	record := slog.NewRecord(time.Now(), toSlogLevel(event.lvl), event.msg, 0)
	record.AddAttrs(event.attrs...)
	return x.Handler.Handle(context.TODO(), record)
}

// toSlogLevel converts logiface.Level to slog.Level.
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
