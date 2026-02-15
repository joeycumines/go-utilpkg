package slog

import (
	"log/slog"

	"github.com/joeycumines/logiface"
)

// Option configures a Logger.
type Option func(*Logger)

// WithAttributes adds default attributes that will be added to all log events.
// These attributes are added before any event-specific attributes.
func WithAttributes(attrs []slog.Attr) Option {
	return func(l *Logger) {
		// Append to existing defaultAttrs, don't replace
		l.defaultAttrs = make([]slog.Attr, 0, len(l.defaultAttrs)+len(attrs))
		l.defaultAttrs = append(l.defaultAttrs, l.defaultAttrs...)
		l.defaultAttrs = append(l.defaultAttrs, attrs...)
	}
}

// WithGroup adds a default group prefix that will apply to all log events.
// Group names are prepended to attribute keys with a "." separator.
func WithGroup(name string) Option {
	return func(l *Logger) {
		if name != "" {
			l.groupStack = make([]string, 0, len(l.groupStack)+1)
			l.groupStack = append(l.groupStack, l.groupStack...)
			l.groupStack = append(l.groupStack, name)
		}
	}
}

// WithLevel sets the minimum log level.
// Events below this level will not be emitted.
func WithLevel(lvl logiface.Level) Option {
	return func(l *Logger) {
		l.level = lvl
	}
}

// WithReplaceAttr sets a hook function that is called for each attribute
// before it is added to a log event.
//
// The hook receives the group prefix (e.g., "http.request") and the attribute.
// It can return a modified attribute to transform values, a zero attribute to
// filter it out, or the same attribute to leave it unchanged.
//
// Example (redact passwords):
//
//	replacer := func(groups []string, a slog.Attr) slog.Attr {
//	    if strings.Contains(a.Key, "password") {
//	        return slog.String(a.Key, "***REDACTED***")
//	    }
//	    return a
//	}
//
//	logger := slog.NewLogger(handler, slog.WithReplaceAttr(replacer))
func WithReplaceAttr(fn func([]string, slog.Attr) slog.Attr) Option {
	return func(l *Logger) {
		l.replaceAttr = fn
	}
}
