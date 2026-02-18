// Package islog implements support for using log/slog with github.com/joeycumines/logiface.
//
// # Purpose
//
// islog is an adapter that bridges logiface's fluent builder API to Go's slog
// (structured logging) handlers. It allows you to use any slog.Handler (JSON,
// text, custom handlers) with logiface's ergonomic field builder methods.
//
// The adapter implements logiface interfaces:
//   - logiface.Writer[*Event] for writing events
//   - logiface.EventFactory[*Event] for creating events from a pool
//   - logiface.EventReleaser[*Event] for returning events to the pool
//
// # Quick Start
//
// Basic usage with aJSON handler:
//
//	import (
//	    "log/slog"
//	    "os"
//	    "github.com/joeycumines/logiface-slog/islog"
//	)
//
//	handler := slog.NewJSONHandler(os.Stdout, nil)
//	logger := islog.L.New(islog.L.WithSlogHandler(handler))
//
//	logger.Info().
//	    Str("service", "api").
//	    Str("method", "GET").
//	    Int("status", 200).
//	    Log("Request completed")
//
// # Integration with slog
//
// islog adapts logiface's type-safe builder API to slog's structured logging model.
//
// # Level Mapping
//
// logiface levels map to slog levels as follows:
//
//	logiface.LevelTrace    → slog.LevelDebug
//	logiface.LevelDebug    → slog.LevelDebug
//	logiface.LevelInfo     → slog.LevelInfo
//	logiface.LevelNotice   → slog.LevelWarn
//	logiface.LevelWarning  → slog.LevelWarn
//	logiface.LevelError    → slog.LevelError
//	logiface.LevelCritical → slog.LevelError
//	logiface.LevelAlert    → slog.LevelError
//	logiface.LevelEmergency→ panic (terminates application)
//
// Notice that slog has fewer levels than logiface. Multiple logiface levels
// map to the same slog level (e.g., Notice and Warning both map to Warn).
//
// # Performance Characteristics
//
// islog is designed for high-throughput logging with minimal allocation:
//
// # Pool Reuse
//
// Events are obtained from a sync.Pool and reused across log calls. The attrs
// slice is pre-allocated with capacity 8, accommodating most common field counts
// without reallocation. Pool reuse significantly reduces GC pressure in high-volume
// logging scenarios.
//
// # Early Filter
//
// Logger.Write() checks Handler.Enabled() before creating a slog.NewRecord.
// Disabled logs return logiface.ErrDisabled immediately, avoiding unnecessary allocations.
//
// # Minimal Overhead
//
// The adapter layer is thin:
//   - No reflection in hot path (slog handles type encoding)
//   - Inline-friendly method signatures
//   - Struct alignment optimized for cache locality
//
// # Usage Patterns
//
// ## Basic Logging
//
//	logger := islog.L.New(islog.L.WithSlogHandler(handler))
//	logger.Info().Str("key", "value").Log("message")
//
// ## Level Configuration
//
//	logger := islog.L.New(
//	    islog.L.WithSlogHandler(handler),
//	    logiface.WithLevel[*islog.Event](logiface.LevelDebug),
//	)
//
// ## Concurrent Logging
//
// Logger is safe for concurrent use. Share a single instance across goroutines:
//
//	var globalLogger = islog.L.New(islog.L.WithSlogHandler(handler))
//
//	func handleRequest() {
//	    globalLogger.Info().
//	        Str("request_id", reqID).
//	        Log("Processing request")
//	}
//
// ## Custom Handlers
//
// Use any slog.Handler, including community handlers:
//
//	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
//	    Level: slog.LevelDebug,
//	    ReplaceAttr: customRedaction,
//	})
//	logger := islog.L.New(islog.L.WithSlogHandler(handler))
//
// # Thread Safety
//
//   - **Logger**: Safe for concurrent use from multiple goroutines
//   - **Event**: Not thread-safe. Each Event must be confined to a single goroutine
//     for its entire lifecycle (creation → Write → Release)
//   - **LoggerFactory (L)**: Safe for concurrent use
//
// # Limitations
//
// islog deliberately omits some logiface features to align with slog's model:
//
//   - **AddGroup**: Always returns false. slog.Group requires attributes to be
//     meaningful; the adapter signals the framework to use flattened keys instead.
//
//   - **Context propagation**: logiface.Writer doesn't accept context; the adapter
//     uses context.TODO() when calling Handler.Handle(). This is a known limitation.
//
// # See Also
//
//   - https://pkg.go.dev/log/slog
//   - https://github.com/joeycumines/logiface
package islog
