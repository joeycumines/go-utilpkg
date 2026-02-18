# Best Practices for logiface-slog

This guide covers best practices for using `logiface-slog` (islog) adapter in production applications.

## Field Naming Conventions

### Use snake_case for Field Names

**Recommended:**
```go
event.AddField("user_id", "123")
event.AddField("request_id", "abc-123")
event.AddField("operation_name", "create_user")
```

**Avoid:**
```go
event.AddField("userID", "123")       // Inconsistent style
event.AddField("UserId", "123")       // PascalCase (for struct fields)
event.AddField("request-id", "abc-123") // Non-standard separators
```

**Rationale:** snake_case is consistent with slog's JSON output format and most log aggregation systems (ELK, Splunk, Datadog).

### Use Descriptive, Action-Oriented Names

**Recommended:**
```go
event.AddField("error_message", err.Error())
event.AddField("database_host", "db.example.com")
event.AddField("cache_hit", true)
event.AddField("operation_duration_ms", 125)
```

**Avoid:**
```go
event.AddField("msg1", err.Error())           // Vague
event.AddField("db", "db.example.com")         // Too abbreviated
event.AddField("x", true)                    // Meaningless
event.AddField("op_dur", 125)                 // Excessive abbreviation
```

### Group Related Fields with Prefixes

**Recommended:**
```go
// Request metadata
event.AddField("request_id", "123")
event.AddField("request_method", "GET")
event.AddField("request_path", "/api/users")

// Response metadata
event.AddField("response_status_code", 200)
event.AddField("response_duration_ms", 45)
event.AddField("response_size_bytes", 1024)
```

**Alternative:** Use `slog.Handler.WithGroup()` for automatic grouping:

```go
handlerWithGroup := handler.WithGroup("request")
logger := L.New(L.WithSlogHandler(handlerWithGroup))

// Fields automatically grouped under "request"
event.AddField("id", "123")
event.AddField("method", "GET")
// Output: {"request": {"id": "123", "method": "GET"}}
```

## Log Level Selection Guidance

### Level Hierarchy

| Level | Use When | Example |
|--------|-----------|----------|
| Trace | Extremely detailed debugging usually disabled in production | Function entry/exit, variable values inside loops |
| Debug | Detailed information for debugging | Internal state transitions, algorithm steps |
| Informational | Normal, significant events | Request processed, user action, state change |
| Notice | Normal but significant conditions | Startup, shutdown, configuration changes |
| Warning | Unexpected but not error conditions | Retry attempts, degraded functionality, fallback values |
| Error | Error events but application continues | HTTP 5xx errors, database connection failures |
| Critical | Severe error events | Data corruption, complete service outage |
| Alert | Immediate action required | Disk space >95%, memory leak detected |
| Emergency | System is unusable | Kernel panic, hardware failure |

### Usage Examples

**Trace (development only):**
```go
event := logger.NewEvent(logiface.LevelTrace)
event.AddMessage("entering function processUser")
event.AddField("user_id", userID)
event.AddField("parameters_count", len(params))
logger.Write(logger.ReleaseEvent(event))
```

**Informational (production - normal events):**
```go
event := logger.NewEvent(logiface.LevelInformational)
event.AddMessage("request processed successfully")
event.AddField("request_id", reqID)
event.AddField("duration_ms", duration.Milliseconds())
logger.Write(logger.ReleaseEvent(event))
```

**Warning (degraded functionality):**
```go
event := logger.NewEvent(logiface.LevelWarning)
event.AddMessage("using fallback cache after primary cache failure")
event.AddField("cache_type", "redis")
event.AddField("fallback_type", "memory")
event.AddField("primary_error", primaryError.Error())
logger.Write(logger.ReleaseEvent(event))
```

**Error (application continues):**
```go
if err := process(); err != nil {
    event := logger.NewEvent(logiface.LevelError)
    event.AddMessage("failed to process user request")
    event.AddField("user_id", userID)
    event.AddField("request_id", reqID)
    event.AddError(err)
    logger.Write(logger.ReleaseEvent(event))
    // Application continues handling other requests
}
```

**Critical (severe impact):**
```go
if dbConnection == nil {
    event := logger.NewEvent(logiface.LevelCritical)
    event.AddMessage("database connection lost, service degraded")
    event.AddField("database_host", dbHost)
    event.AddField("retry_count", 5)
    logger.Write(logger.ReleaseEvent(event))
    // Alerting system should fire
}
```

### Level Filtering Configuration

```go
// Development - enable all levels including Trace
devHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug, // Maps to logiface LevelDebug
})

// Staging - enable Debug and above
stagHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug, // Maps to logiface LevelDebug
})

// Production - enable Info and above (filter Trace/Debug)
prodHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo, // Maps to logiface LevelInformational
})
```

## Context Logger Pattern

### Propagate Context Through Request Chain

**Recommended:**
```go
func handleRequest(ctx context.Context, req Request) error {
    // Use context-based logger for request-scoped logs
    event := loggerFromContext(ctx).NewEvent(logiface.LevelInformational)
    event.AddMessage("processing request")
    event.AddField("request_id", req.ID)
    loggerFromContext(ctx).Write(loggerFromContext(ctx).ReleaseEvent(event))

    // Process request with context propagation
    if err := processUser(ctx, req.UserID); err != nil {
        return err
    }

    return nil
}

func processUser(ctx context.Context, userID string) error {
    event := loggerFromContext(ctx).NewEvent(logiface.LevelInformational)
    event.AddMessage("processing user")
    // Automatically includes request_id from context
    loggerFromContext(ctx).Write(loggerFromContext(ctx).ReleaseEvent(event))

    // ... processing logic ...
    return nil
}
```

### Context Key Pattern

```go
type contextKey string

const (
    loggerKey contextKey = "logger"
)

func withLogger(ctx context.Context, logger *logiface.Logger[*Event]) context.Context {
    return context.WithValue(ctx, loggerKey, logger)
}

func loggerFromContext(ctx context.Context) *logiface.Logger[*Event] {
    if logger, ok := ctx.Value(loggerKey).(*logiface.Logger[*Event]); ok {
        return logger
    }
    // Fallback to default logger
    return defaultLogger
}

// Usage in HTTP middleware
func loggingMiddleware(next http.Handler) http.Handler {
    baseHandler := slog.NewJSONHandler(/* ... */)
    baseLogger := L.New(L.WithSlogHandler(baseHandler))

    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Create per-request logger with request ID
        requestID := generateRequestID()

        // Create logger with request ID field (not using Clone, field accumulation)
        logger := L.New(L.WithSlogHandler(baseHandler))

        ctx := r.Context()
        ctx = withLogger(ctx, logger)

        // Log request start
        event := logger.NewEvent(logiface.LevelInformational)
        event.AddMessage("request started")
        event.AddField("request_id", requestID)
        logger.Write(logger.ReleaseEvent(event))

        // Propagate context to handler
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

## Avoiding Allocations

### Use Pre-allocated Handlers

**Recommended:**
```go
// Create handler once at app startup (not per request)
var globalHandler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})

var globalLogger = L.New(L.WithSlogHandler(globalHandler))

func myFunction() {
    event := globalLogger.NewEvent(logiface.LevelInformational)
    event.AddMessage("message")
    event.AddField("key", "value")
    globalLogger.Write(globalLogger.ReleaseEvent(event))
}
```

**Avoid:**
```go
func myFunction() {
    // Creating handler per function is expensive!
    handler := slog.NewJSONHandler(os.Stdout, nil)
    logger := L.New(L.WithSlogHandler(handler))

    event := logger.NewEvent(logiface.LevelInformational)
    event.AddMessage("message")
    logger.Write(logger.ReleaseEvent(event))
}
```

### Reuse Event Objects

**Recommended:**
```go
event := logger.NewEvent(logiface.LevelInformational)
event.AddMessage("message")
event.AddField("key1", "value1")
event.AddField("key2", "value2")
logger.Write(logger.ReleaseEvent(event)) // Event returned to pool
```

**Avoid:**
```go
event := logger.NewEvent(logiface.LevelInformational)
event.AddMessage("message")
logger.Write(logger.ReleaseEvent(event))
// Missing: Field not accumulated before ReleaseEvent
```

### Use Pooling Effectively

The adapter automatically pools events via `sync.Pool`. To maximize effectiveness:

1. **Always release events:** `logger.ReleaseEvent(event)` returns event to pool
2. **Release promptly:** Don't hold events in variables longer than necessary
3. **Configure pool size (advanced):** Default pools are sized via Go runtime

```go
// High-throughput pattern - rapid event creation and release
func processMultipleRequests(requests []Request) {
    for _, req := range requests {
        event := logger.NewEvent(logiface.LevelInformational)
        event.AddMessage("processing")
        event.AddField("request_id", req.ID)
        logger.Write(logger.ReleaseEvent(event)) // Immediate release
    }
}
```

## Error Logging Patterns

### Structured Error Logging

**Recommended:**
```go
func processUser(userID string) error {
    user, err := database.QueryUser(userID)
    if err != nil {
        event := logger.NewEvent(logiface.LevelError)
        event.AddMessage("database query failed")
        event.AddField("operation", "query_user")
        event.AddField("user_id", userID)
        event.AddField("database", "postgres")
        event.AddError(err)
        logger.Write(logger.ReleaseEvent(event))

        return fmt.Errorf("failed to query user %s: %w", userID, err)
    }

    return nil
}
```

**Avoid:**
```go
func processUser(userID string) error {
    user, err := database.QueryUser(userID)
    if err != nil {
        log.Println("Error:", err) // No context, no structured fields
        return err
    }

    return nil
}
```

### Error Chain Logging

**Recommended:**
```go
func handleRequest(req Request) error {
    token, err := authenticate(req.Token)
    if err != nil {
        event := logger.NewEvent(logiface.LevelWarning)
        event.AddMessage("authentication failed")
        event.AddField("request_id", req.ID)
        event.AddField("auth_error", err.Error())
        event.AddField("auth_type", "jwt")
        logger.Write(logger.ReleaseEvent(event))

        return fmt.Errorf("authentication failed: %w", err) // Wrap error
    }

    // ... process request ...

    return nil
}
```

### Panic/Recover Pattern

**Recommended:**
```go
func serveHTTP() {
    defer func() {
        if r := recover(); r != nil {
            event := logger.NewEvent(logiface.LevelCritical)
            event.AddMessage("panic recovered")
            event.AddField("panic_value", fmt.Sprintf("%v", r))
            event.AddField("stack_trace", captureStackTrace())
            logger.Write(logger.ReleaseEvent(event))
        }
    }()

    // ... HTTP serving logic ...
}

func captureStackTrace() string {
    buf := make([]byte, 4096)
    n := runtime.Stack(buf, false)
    return string(buf[:n])
}
```

## Structured Logging Principles

### Log What, Not When (Use Structured Fields)

**Recommended:**
```go
event := logger.NewEvent(logiface.LevelInformational)
event.AddMessage("user login attempt")
event.AddField("user_id", "123")
event.AddField("ip_address", "192.168.1.1")
event.AddField("user_agent", "Mozilla/5.0")
logger.Write(logger.ReleaseEvent(event))
```

**Avoid:**
```go
event := logger.NewEvent(logiface.LevelInformational)
// Vague message with no structured context
event.AddMessage("user 123 logged in from 192.168.1.1 using Mozilla/5.0")
logger.Write(logger.ReleaseEvent(event))
```

**Rationale:** Structured fields enable log aggregation, filtering, and analysis. Free-form messages make querying difficult.

### Use Consistent Field Types

**Recommended:**
```go
event.AddField("count", 42)              // Always int for counts
event.AddField("duration_ms", 125)        // Always milliseconds and unit in name
event.AddField("enabled", true)            // Always boolean for enabled/disabled
event.AddField("timestamp", time.Now())     // time.Time for timestamps
event.AddField("error", err)              // Error type for errors (via AddError)
```

**Avoid:**
```go
event.AddField("count", "42")             // String instead of int
event.AddField("duration", 0.125)        // Seconds, unclear unit
event.AddField("flag", "yes")             // String instead of boolean
event.AddField("time", "2024-01-01")    // String instead of time.Time
event.AddField("error", err.Error())     // String message instead of error object
```

### Avoid Redundant Fields in Messages

**Recommended:**
```go
event := logger.NewEvent(logiface.LevelInformational)
event.AddMessage("request completed")
event.AddField("request_id", "123")
event.AddField("duration_ms", 50)
event.AddField("status_code", 200)
logger.Write(logger.ReleaseEvent(event))
```

**Avoid:**
```go
event := logger.NewEvent(logiface.LevelInformational)
// Message repeats information in fields (redundant)
event.AddMessage("request 123 completed with status 200 in 50ms")
event.AddField("request_id", "123")
event.AddField("duration_ms", 50)
event.AddField("status_code", 200)
logger.Write(logger.ReleaseEvent(event))
```

## Concurrency Patterns

### Use Separate Events Per Goroutine

**Recommended:**
```go
func processConcurrently(items []Item) {
    var wg sync.WaitGroup
    for i, item := range items {
        wg.Add(1)
        go func(idx int, data Item) {
            defer wg.Done()

            // Each goroutine creates its own event
            event := logger.NewEvent(logiface.LevelInformational)
            event.AddMessage("processing item")
            event.AddField("item_id", data.ID)
            event.AddField("worker_number", idx)
            logger.Write(logger.ReleaseEvent(event))
        }(i, item)
    }
    wg.Wait()
}
```

**Avoid:**
```go
func processConcurrently(items []Item) {
    event := logger.NewEvent(logiface.LevelInformational) // Single event shared

    var wg sync.WaitGroup
    for i, item := range items {
        wg.Add(1)
        go func(idx int, data Item) {
            defer wg.Done()

            // RACE CONDITION: Multiple goroutines adding to same event!
            event.AddField("item_id", data.ID)
        }(i, item)
    }
    wg.Wait()

    logger.Write(logger.ReleaseEvent(event))
}
```

### Thread Safety Notes

- `Event`: NOT thread-safe (use per goroutine)
- `Logger`: Thread-safe (concurrent Write() calls are safe)
- `LoggerFactory`: Thread-safe (concurrent New() calls are safe)

## Security Considerations

**See [SECURITY.md](SECURITY.md) for:**
- PII handling patterns
- Credential redaction using `ReplaceAttr`
- Audit logging requirements

## Performance Optimization

**See [PERFORMANCE.md](PERFORMANCE.md) for:**
- Benchmark results and interpretation
- Pool effectiveness analysis
- Allocation characteristics
- When to use adapter vs direct slog

---

For additional documentation, see:
- [README.md](README.md) - Installation and usage
- [LIMITATIONS.md](LIMITATIONS.md) - Adapter constraints
- [MIGRATION.md](MIGRATION.md) - Migrating from direct slog
- [TESTING.md](TESTING.md) - Testing strategies
