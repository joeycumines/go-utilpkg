// Package slog provides a logiface adapter for Go's standard log/slog package.
//
// This package creates a bidirectional bridge between logiface and slog:
//
//  1. **Primary Direction (logiface → slog)**: logiface.Logger writes to slog.Handler
//     Use this when you want logiface's builder API with slog's output mechanisms.
//
//     handler := slog.NewJSONHandler(os.Stdout, nil)
//     logger := slog.NewLogger(handler)
//     logger.Info().Str("key", "value").Msg("message")
//
//  2. **Reverse Direction (slog → logiface)**: slog.Handler wraps logiface.Logger
//     Use this when you have logiface infrastructure and want slog.Logger support.
//
//     ifaceLogger := logiface.New[*Event](...)
//     handler := slog.NewSlogHandler(ifaceLogger)
//     slog.SetDefault(slog.New(handler))
//
// # Key Features
//
// - Event pooling with sync.Pool for reduced allocation under high load
// - Complete slog.Value.Kind support (String, Int64, Uint64, Float64, Bool, Duration, Time, Group, Any, LogValuer)
// - slog.LogValuer interface support with automatic resolution
// - slog.WithAttrs and slog.WithGroup semantics
// - slog.ReplaceAttr hook for attribute transformation
// - Source location (PC) extraction from slog.Record
// - Context propagation through Handle()
// - Level mapping between logiface.Level (9 levels) and slog.Level (4 levels)
//
// # Interface Compliance
//
// The Logger type implements:
//   - logiface.EventFactory[*Event]
//   - logiface.Writer[*Event]
//   - logiface.EventReleaser[*Event]
//   - logiface.JSONSupport[*Event, *slog.Handler, *slog.Handler]
//
// The Event type implements:
//   - logiface.Event (all Add*, Msg, Level methods)
//
// # Level Mapping
//
// Mapping between logiface and slog is lossy but functional:
//
//	logiface.Trace      → slog.LevelDebug
//	logiface.Debug      → slog.LevelDebug
//	logiface.Informational → slog.LevelInfo
//	logiface.Notice      → slog.LevelWarn
//	logiface.Warning     → slog.LevelWarn
//	logiface.Error       → slog.LevelError
//	logiface.Critical    → slog.LevelError
//	logiface.Alert       → slog.LevelError
//	logiface.Emergency   → slog.LevelError
package slog
