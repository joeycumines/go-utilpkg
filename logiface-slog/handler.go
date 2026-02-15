package slog

import (
	"context"
	"log/slog"

	"github.com/joeycumines/logiface"
)

// SlogHandler wraps a logiface.Logger, implementing slog.Handler interface.
// This enables slog.Logger to write to a logiface.Logger.
type SlogHandler struct {
	// logger is the underlying logiface.Logger[*Event]
	logger *logiface.Logger[*Event]

	// preAttrs are accumulated attributes from WithAttrs calls
	preAttrs []slog.Attr

	// groupStack tracks current group prefix hierarchy
	groupStack []string
}

// NewSlogHandler creates a slog.Handler from a logiface.Logger.
func NewSlogHandler(logger *logiface.Logger[*Event]) slog.Handler {
	return &SlogHandler{
		logger:     logger,
		preAttrs:   make([]slog.Attr, 0, 8),
		groupStack: make([]string, 0, 4),
	}
}

// Enabled checks if logging at given level is enabled.
func (x *SlogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	// Convert slog.Level to logiface.Level
	logifaceLevel := toLogifaceLevel(level)

	// Check if logger can write and level is enabled
	return x.logger.Enabled() && logifaceLevel.Enabled()
}

// Handle converts a slog.Record to logiface.Event and emits it.
func (x *SlogHandler) Handle(ctx context.Context, r slog.Record) error {
	// Clone record for thread safety
	// slog.Record is passed by value, but its internal attr slice
	// may be shared. Cloning ensures no data race if same record
	// is used concurrently by multiple goroutines.
	r = r.Clone()

	// Convert slog.Level to logiface.Level
	logifaceLevel := toLogifaceLevel(r.Level)

	// Create builder via logiface.Logger
	builder := x.logger.Build(logifaceLevel)
	if builder == nil {
		// Level disabled
		return nil
	}

	// Apply WithAttrs attributes first
	for _, attr := range x.preAttrs {
		x.applyAttrToBuilder(builder, attr)
	}

	// Iterate Record.Attrs and add to event
	r.Attrs(func(a slog.Attr) bool {
		x.applyAttrToBuilder(builder, a)
		return true
	})

	// Set message and time
	builder.Log(r.Message)

	return nil
}

// applyAttrToBuilder applies a slog.Attr to a logiface.Builder,
// handling group prefixes and Value kinds.
func (x *SlogHandler) applyAttrToBuilder(builder *logiface.Builder[*Event], a slog.Attr) {
	// Build full key with group prefix
	key := x.buildAttrKey(a.Key)

	// Convert slog.Value to appropriate field type
	x.addValueToBuilder(builder, key, a.Value)
}

// buildAttrKey builds the full attribute key including group prefix.
func (x *SlogHandler) buildAttrKey(key string) string {
	if len(x.groupStack) == 0 {
		return key
	}

	// Join group names with "." separator
	fullKey := key
	for i := len(x.groupStack) - 1; i >= 0; i-- {
		fullKey = x.groupStack[i] + "." + fullKey
	}

	return fullKey
}

// addValueToBuilder converts slog.Value to builder field addition.
func (x *SlogHandler) addValueToBuilder(builder *logiface.Builder[*Event], key string, v slog.Value) {
	// Let slog.Value handle its own resolution
	v = v.Resolve()

	// Handle Value.Kind types
	switch v.Kind() {
	case slog.KindString:
		builder.Str(key, v.String())
	case slog.KindInt64:
		builder.Int64(key, v.Int64())
	case slog.KindUint64:
		builder.Uint64(key, v.Uint64())
	case slog.KindFloat64:
		builder.Float64(key, v.Float64())
	case slog.KindBool:
		builder.Bool(key, v.Bool())
	case slog.KindDuration:
		builder.Dur(key, v.Duration())
	case slog.KindTime:
		builder.Time(key, v.Time())
	case slog.KindAny, slog.KindGroup:
		builder.Any(key, v.Any())
	}
}

// WithAttrs creates a new Handler with additional attributes.
func (x *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newPreAttrs := make([]slog.Attr, 0, len(x.preAttrs)+len(attrs))
	newPreAttrs = append(newPreAttrs, x.preAttrs...)
	newPreAttrs = append(newPreAttrs, attrs...)

	return &SlogHandler{
		logger:     x.logger,
		preAttrs:   newPreAttrs,
		groupStack: x.groupStack,
	}
}

// WithGroup adds a group to the Handler's group stack.
func (x *SlogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return x
	}

	newGroupStack := make([]string, 0, len(x.groupStack)+1)
	newGroupStack = append(newGroupStack, x.groupStack...)
	newGroupStack = append(newGroupStack, name)

	return &SlogHandler{
		logger:     x.logger,
		preAttrs:   x.preAttrs,
		groupStack: newGroupStack,
	}
}
