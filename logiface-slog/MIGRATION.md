# Migration Guide: slog.Logger → logiface[*Event] with islog

This guide demonstrates how to migrate code that uses Go's standard `log/slog` package to use the `logiface` framework with the `islog` (logiface-slog) adapter.

## Why Migrate?

The `logiface-slog` adapter enables you to:

1. **Use any slog.Handler** (JSON, text, custom) with the `logiface` framework
2. **Swap handlers at runtime** (e.g., switch from JSON to TextHandler) without changing logging code
3. **Enable consistent logging APIs** across multiple adapters (slog, zerolog, logrus) with shared `logiface` code
4. **Leverage logiface features** like level filtering, context propagation, and structured field APIs

## Quick Reference

| slog | logiface-slog | Notes |
|------|----------------|-------|
| `slog.New(handler)` | `islog.L.New(islog.WithSlogHandler(handler))` | Same handler, factory pattern |
| `logger.Info("msg")` | `logger.Info("msg")` | Same fluent API |
| `logger.Info("msg", "key", "val")` | `event.AddField("key", "val"); logger.Write(event)` | Event accumulation model |
| `slog.NewJSONHandler` | Same (no change) | Handlers are identical |
| `handler.WithLevel(slog.LevelInfo)` | `islog.WithSlogHandler(handler)` | Configured via options |

## Example 1: Basic Logging

**Before (direct slog):**
```go
package main

import (
    "log/slog"
    "os"
)

func main() {
    handler := slog.NewJSONHandler(os.Stdout, nil)
    logger := slog.New(handler)

    logger.Info("application started", "version", "1.0.0")
    logger.Error("something failed", "error", "timeout", "retries", 3)
}
```

**After (logiface-slog):**
```go
package main

import (
    "os"
    "log/slog"

    "github.com/joeycumines/logiface"
    "github.com/joeycumines/logiface-slog"
)

func main() {
    // Step 1: Create slog handler (same as before)
    handler := slog.NewJSONHandler(os.Stdout, nil)

    // Step 2: Wrap handler with islog adapter
    logger := logiface.New[*islog.Event](islog.WithSlogHandler(handler))

    // Step 3: Use logiface accumulation pattern
    event := logger.NewEvent(logiface.LevelInformational)
    event.AddMessage("application started")
    event.AddField("version", "1.0.0")
    logger.Write(logger.ReleaseEvent(event))

    event = logger.NewEvent(logiface.LevelError)
    event.AddMessage("something failed")
    event.AddField("error", "timeout")
    event.AddField("retries", 3)
    logger.Write(logger.ReleaseEvent(event))
}
```

**Output (same for both):**
```json
{"time":"2026-02-18T10:00:00Z","level":"INFO","msg":"application started","version":"1.0.0"}
{"time":"2026-02-18T10:00:01Z","level":"ERROR","msg":"something failed","error":"timeout","retries":3}
```

## Example 2: Using LoggerFactory for Convenience

**Before (direct slog):**
```go
package main

import "log/slog"

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

func main() {
    logger.Info("processing request", "id", 42)
    processData(42)
}

func processData(id int) {
    logger.Info("data processed", "id", id, "status", "success")
}
```

**After (logiface-slog with L):**
```go
package main

import (
    "os"
    "log/slog"

    "github.com/joeycumines/logiface"
    "github.com/joeycumines/logiface-slog"
)

// Create global logger using islog.L (LoggerFactory)
var logger = islog.L.New(
    islog.WithSlogHandler(slog.NewJSONHandler(os.Stdout, nil)),
)

func main() {
    event := logger.NewEvent(logiface.LevelInformational)
    event.AddMessage("processing request")
    event.AddField("id", 42)
    logger.Write(logger.ReleaseEvent(event))

    processData(42)
}

func processData(id int) {
    event := logger.NewEvent(logiface.LevelInformational)
    event.AddMessage("data processed")
    event.AddField("id", id)
    event.AddField("status", "success")
    logger.Write(logger.ReleaseEvent(event))
}
```

**Note:** `islog.L` is a package-level `LoggerFactory` instance providing convenience methods like `.WithSlogHandler()`.

## Example 3: Context Propagation

**Before (direct slog):**
```go
func processRequest(ctx context.Context, req Request) {
    slog.LogAttrs(ctx, slog.LevelInfo, "processing request",
        slog.String("request_id", req.ID),
        slog.Int("user_id", req.UserID),
    )
}
```

**After (logiface-slog):**
```go
func processRequest(ctx context.Context, req Request) {
    event := logger.NewEvent(logiface.LevelInformational)
    event.AddMessage("processing request")
    event.AddField("request_id", req.ID)
    event.AddField("user_id", req.UserID)
    logger.Write(logger.ReleaseEvent(event))
}
```

**Note:** To pass `context` to slog handler with logiface-slog, configure the handler with `slog.HandlerOptions`:

```go
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
})
logger := logiface.New[*islog.Event](islog.WithSlogHandler(handler))

// Handler will use context during Handle() calls
```

## Example 4: Error Handling

**Before (direct slog):**
```go
func doWork() error {
    if err := someOperation(); err != nil {
        slog.Error("operation failed", "error", err)
        return err
    }
    return nil
}
```

**After (logiface-slog):**
```go
func doWork() error {
    if err := someOperation(); err != nil {
        event := logger.NewEvent(logiface.LevelError)
        event.AddMessage("operation failed")
        event.AddError(err) // logiface-specific error handling
        logger.Write(logger.ReleaseEvent(event))
        return err
    }
    return nil
}
```

## Example 5: Swapping Handlers at Runtime

**Before (direct slog):**
```go
// slog requires creating new logger to change handler
var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

func setDebugMode(enabled bool) {
    if enabled {
        debugHandler := slog.NewTextHandler(os.Stdout, nil)
        logger = slog.New(debugHandler) // New logger instance
    } else {
        jsonHandler := slog.NewJSONHandler(os.Stdout, nil)
        logger = slog.New(jsonHandler) // New logger instance
    }
}
```

**After (logiface-slog):**
```go
// Start with JSON handler
var jsonHandler = slog.NewJSONHandler(os.Stdout, nil)
var logger = logiface.New[*islog.Event](islog.WithSlogHandler(jsonHandler))

func setDebugMode(enabled bool) {
    if enabled {
        // Switch to text handler by updating the Logger's Handler field
        debugHandler := slog.NewTextHandler(os.Stdout, nil)
        logger.Handler = debugHandler // Direct handler swap
    } else {
        // Switch back to JSON handler
        logger.Handler = jsonHandler
    }
}
```

**Note:** logiface-slog stores `slog.Handler` directly in the `Logger` struct, allowing runtime handler changes without creating new logger instances.

## Example 6: Pre-configured Attributes (WithAttrs)

**Before (direct slog):**
```go
handler := slog.NewJSONHandler(os.Stdout, nil)
handlerWithAttrs := handler.WithAttrs([]slog.Attr{
    slog.String("service", "myapp"),
    slog.String("version", "1.0.0"),
})
logger := slog.New(handlerWithAttrs)

logger.Info("request received", "path", "/api/users")
// Output includes service and version fields
```

**After (logiface-slog):**
```go
handler := slog.NewJSONHandler(os.Stdout, nil)
handlerWithAttrs := handler.WithAttrs([]slog.Attr{
    slog.String("service", "myapp"),
    slog.String("version", "1.0.0"),
})
logger := logiface.New[*islog.Event](islog.WithSlogHandler(handlerWithAttrs))

event := logger.NewEvent(logiface.LevelInformational)
event.AddMessage("request received")
event.AddField("path", "/api/users")
logger.Write(logger.ReleaseEvent(event))
// Output includes service and version fields
```

**Output (both):**
```json
{"time":"2026-02-18T10:00:00Z","level":"INFO","msg":"request received","service":"myapp","version":"1.0.0","path":"/api/users"}
```

## Example 7: Field Grouping (WithGroup)

**Before (direct slog):**
```go
handler := slog.NewJSONHandler(os.Stdout, nil)
handlerWithGroup := handler.WithGroup("request")
logger := slog.New(handlerWithGroup)

logger.Info("processing", "method", "GET", "path", "/api/users")
```

**After (logiface-slog):**
```go
handler := slog.NewJSONHandler(os.Stdout, nil)
handlerWithGroup := handler.WithGroup("request")
logger := logiface.New[*islog.Event](islog.WithSlogHandler(handlerWithGroup))

event := logger.NewEvent(logiface.LevelInformational)
event.AddMessage("processing")
event.AddField("method", "GET")
event.AddField("path", "/api/users")
logger.Write(logger.ReleaseEvent(event))
```

**Output (both):**
```json
{"time":"2026-02-18T10:00:00Z","level":"INFO","msg":"processing","request":{"method":"GET","path":"/api/users"}}
```

## Migration Checklist

- [ ] Replace `slog.New(handler)` with `logiface.New[*islog.Event](islog.WithSlogHandler(handler))`
- [ ] Replace direct logger calls (`logger.Info()`) with event accumulation pattern:
  - `event := logger.NewEvent(level)`
  - `event.AddMessage(msg)` or `event.AddField("msg", msg)`
  - `event.AddField(key, val)` for each field
  - `logger.Write(event)`
  - `logger.ReleaseEvent(event)` (required for pool reuse)
- [ ] Update level constants from `slog.Level*` to `logiface.Level*`:
  - `slog.LevelDebug` → `logiface.LevelDebug` (same value)
  - `slog.LevelInfo` → `logiface.LevelInformational`
  - `slog.LevelWarn` → `logiface.LevelWarning` or `logiface.LevelNotice`
  - `slog.LevelError` → `logiface.LevelError`
- [ ] Test with existing handlers to ensure output compatibility
- [ ] Review [LIMITATIONS.md](./LIMITATIONS.md) for adapter-specific constraints

## Common Pitfalls

### 1. Forgetting ReleaseEvent

**Incorrect:**
```go
event := logger.NewEvent(logiface.LevelInfo)
event.AddMessage("message")
logger.Write(event)
// Missing: logger.ReleaseEvent(event)
```

**Correct:**
```go
event := logger.NewEvent(logiface.LevelInfo)
event.AddMessage("message")
logger.Write(event)
logger.ReleaseEvent(event) // REQUIRED for pool reuse
```

### 2. Using Wrong Level Constants

**Incorrect:**
```go
event := logger.NewEvent(logiface.Level(4)) // Magic number
```

**Correct:**
```go
event := logger.NewEvent(logiface.LevelNotice) // Named constant
```

### 3. Sharing Events Between Goroutines

**Incorrect:**
```go
event := logger.NewEvent(logiface.LevelInfo)
for i := 0; i < 100; i++ {
    go func() {
        event.AddField("id", i) // RACE CONDITION
    }()
}
```

**Correct:**
```go
for i := 0; i < 100; i++ {
    go func(id int) {
        event := logger.NewEvent(logiface.LevelInfo)
        event.AddField("id", id)
        logger.Write(event)
        logger.ReleaseEvent(event)
    }(i)
}
```

## Performance Considerations

The logiface-slog adapter is optimized for high-throughput scenarios:

- **Event pooling** via `sync.Pool` reduces allocations (zero allocations for disabled logs)
- **Nil checks omitted** from hot paths (~0.5 ns/op overhead for disabled logging)
- **No per-event locks** - thread-safe via event-per-goroutine model

Benchmark results (see [PERFORMANCE.md](./PERFORMANCE.md)):
```
BenchmarkDisabled-8       	2000000000	         0.5000 ns/op	       0 B/op	       0 allocs/op
BenchmarkInfo-8            	 10000000	       85.00 ns/op	      48 B/op	       1 allocs/op
```

---

For additional examples, see [README.md](./README.md) or [example_test.go](./example_test.go).
