# logiface-slog Adapter

## Context

Need to create a logiface adapter that works with Go's standard `log/slog` package (introduced in Go 1.21). Existing adapters (logiface-zerolog, logiface-logrus, logiface-stumpy) demonstrate the pattern but slog has unique characteristics.

## Decision

### High-Level Architecture

**Primary Direction: logiface.Logger → slog.Handler**

The adapter enables using slog's output mechanisms with logiface's builder API. Users create a logiface.Logger that writes to any slog.Handler (JSONHandler, TextHandler, custom handlers, etc.)

### Key Components

#### 1. Event Structure

```go
type Event struct {
    unimplementedEvent  // Embed for default implementations
    logger      *Logger         // Back-reference
    slogLevel   slog.Level      // Event's log level
    ctx         context.Context // Context for logging
    message     string          // Log message
    time        time.Time       // Timestamp (optional, defaults to time.Now())
    attrs       []slog.Attr     // Accumulated attributes
    groups      []string        // Active groups (for future slog.Group support)
}
```

**Design Rationale:**
- `attrs []slog.Attr`: Simple accumulation, slog.Attr supports all types (String, Int64, Float64, Group, Any, LogValuer)
- No manual JSON buffer: slog.Handler handles serialization
- Pooling: sync.Pool with Reset() clearing all fields

#### 2. Logger Structure

```go
type Logger struct {
    unimplementedJSONSupport
    handler     slog.Handler       // Target slog.Handler
    defaultCtx  context.Context     // Default context for non-Ctx methods
    replaceAttr func([]string, slog.Attr) slog.Attr  // Optional attr transformation hook
    level       logiface.Level      // Logger's configured level (for logiface interface)
}
```

**Interface Compliance:**
- Implements `logiface.EventFactory[*Event]`: NewEvent(level) creates slog.Event
- Implements `logiface.Writer[*Event]`: Write(event) calls slog.Handler.Handle with Record constructed from Event
- Implements `logiface.EventReleaser[*Event]`: ReleaseEvent(event) returns to sync.Pool
- Implements `logiface.JSONSupport[*Event, ..., ...]`: Can* methods return true

#### 3. Level Mapping

**logiface.Level → slog.Level:**

```go
func toSlogLevel(level logiface.Level) slog.Level {
    switch level {
    case logiface.LevelTrace:
        return slog.LevelDebug
    case logiface.LevelDebug:
        return slog.LevelDebug
    case logiface.LevelInformational:
        return slog.LevelInfo
    case logiface.LevelNotice:
        return slog.LevelWarn
    case logiface.LevelWarning:
        return slog.LevelWarn
    case logiface.LevelError:
        return slog.LevelError
    case logiface.LevelCritical:
        return slog.LevelError
    case logiface.LevelAlert:
        return slog.LevelError      // Fatal in slog
    case logiface.LevelEmergency:
        return slog.LevelError      // Panic in slog
    default:
        return slog.LevelInfo       // Unknown/Disabled levels
    }
}
```

**slog.Level → logiface.Level:**

```go
func toLogifaceLevel(level slog.Level) logiface.Level {
    switch {
    case level <= slog.LevelDebug:
        // Include negative custom levels
        return logiface.LevelDebug
    case level <= slog.LevelInfo:
        return logiface.LevelInformational
    case level <= slog.LevelWarn:
        return logiface.LevelWarning
    default:
        return logiface.LevelError
    }
}
```

**Rationale:**
- slog has only 4 standard levels (Debug, Info, Warn, Error)
- logiface has 9 levels (Trace through Emergency)
- Mapping is lossy but functional: many logiface levels map to same slog level
- Use recommended mappings from logiface.Level documentation

#### 4. Event Pooling

```go
var eventPool = sync.Pool{
    New: func() interface{} { return &Event{} },
}

func (l *Logger) NewEvent(level logiface.Level) *Event {
    event := eventPool.Get().(*Event)
    event.logger = l
    event.slogLevel = toSlogLevel(level)
    event.ctx = l.defaultCtx
    event.message = ""
    event.time = time.Now()  // Could be configurable
    event.attrs = event.attrs[:0]  // Reuse slice capacity
    event.groups = event.groups[:0]
    return event
}

func (l *Logger) ReleaseEvent(event *Event) {
    if event != nil {
        // Clear references
        event.logger = nil
        event.ctx = nil
        event.message = ""
        event.attrs = event.attrs[:0]
        event.groups = event.groups[:0]
        eventPool.Put(event)
    }
}
```

**Rationale:**
- Pool reuse reduces allocation under high log volume
- Slice reuse via [:0] preserves capacity
- Clear nil references to prevent memory leaks

#### 5. Event.Send() Implementation

```go
func (e *Event) Send() error {
    // Check if level is enabled
    if !e.logger.handler.Enabled(e.ctx, e.slogLevel) {
        return logiface.ErrDisabled
    }

    // Record construction
    record := slog.NewRecord(
        e.time,
        e.slogLevel,
        e.message,
        0,  // PC set below
    )

    // Add all accumulated attributes
    for _, attr := range e.attrs {
        record.Add(attr)
    }

    // Set PC for source location
    runtime.Callers(2, pcs[:1])
    // ... extract source location ...

    // Handle via slog.Handler
    return e.logger.handler.Handle(e.ctx, record)
}
```

**Design Notes:**
- Check Enabled() before Record construction to avoid allocation
- PC extraction needs correct skip depth (2: Send() caller)
- Apply ReplaceAttr hook if configured
- Release event to pool after Send (or on error)

#### 6. slog.Handler to logiface.Logger Adapter (Inverse Direction)

While primary use is logiface.Logger → slog.Handler, some users may need to use slog.Logger with code expecting logiface.Logger. Provide:

```go
type SlogHandler struct {
    logifaceLogger *logiface.Logger[*Event]
}

func (s *SlogHandler) Enabled(ctx context.Context, level slog.Level) bool {
    logLevel := toLogifaceLevel(level)
    return s.logifaceLogger.Enabled(logLevel)
}

func (s *SlogHandler) Handle(ctx context.Context, r slog.Record) error {
    // Create logiface.Event from slog.Record
    event := s.logifaceLogger.NewEvent(toLogifaceLevel(r.Level))

    // Extract message
    if r.Message != "" {
        event.AddMessage(r.Message)
    }

    // Extract PC for source location
    // ...

    // Extract attributes
    r.Attrs(func(a slog.Attr) bool {
        // Convert slog.Attr to logiface field
        // Handle groups, LogValuer, etc.
        return true
    })

    // Send via logiface
    return event.Send()
}

func (s *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    // Return handler with pre-pended attributes
    // ...
}

func (s *SlogHandler) WithGroup(name string) slog.Handler {
    // Return handler with group prefix
    // ...
}
```

**Status:** Optional, may defer to future work if not needed immediately.

### 7. LogValuer Handling

Slog supports `log/slog.LogValuer` interface for custom types:

```go
type LogValuer interface {
    LogValue() slog.Value
}
```

**Implementation Strategy:**
- In Attr conversion, detect if Value.Kind == KindLogValuer
- Call LogValue() recursively to resolve actual value
- Limit resolution depth to prevent infinite recursion (cycle detection)
- Default depth limit: 10 nested LogValuers

### 8. Group Handling

Slog groups create nested attribute structures:

```go
handler.WithGroup("request").WithGroup("user").Info("message",
    slog.String("name", "joey"),
)
// Produces: {"request": {"user": {"name": "joey"}}}
```

**Design Options:**
1. **Support slog.Group in slog.Event**: Event stores groups in slice, adds group prefix to all subsequent keys
2. **Treat groups as Attr with KindGroup**: Convert nested group to slog.Group attr

**Decision:** Support slog.Group via slog.Group() attr constructor in Add* methods. Logiface-slog Event.Add* methods accept slog.Group as parameter (via slog.Any).

### 9. ReplaceAttr Hook

Support the slog.ReplaceAttr hook pattern:

```go
type Logger struct {
    replaceAttr func([]string, slog.Attr) slog.Attr
}

func (l *Logger) applyReplaceAttr(groups []string, attr slog.Attr) slog.Attr {
    if l.replaceAttr != nil {
        return l.replaceAttr(groups, attr)
    }
    return attr
}
```

**Application Points:**
- When converting slog.Record to Event (Handler direction)
- When adding static attributes (WithAttrs)
- When attributes have group prefix (groups parameter)

### 10. testsuite Integration

**Key Challenge:** Parse slog output (from underlying Handler) to build testsuite.Event

**Strategy:**
1. Use `slog.NewJSONHandler(writer, &slog.HandlerOptions{})`
2. Parse JSON output to extract:
   - `level` → map slog.Level to logiface.Level
   - `msg` → testsuite.Event.Message
   - `err` (or `error`) → testsuite.Event.Error
   - All other fields → testsuite.Event.Fields (exclude: time, level, source)
3. Parse source location:
   ```json
   "source": {"function": "pkg.Func", "file": "path/to/file.go", "line": 123}
   ```
4. Normalize:
   - timestamp: `FormatTime(t) = t.Format("2006-01-02 15:04:05")`
   - levels: Map to logiface.Level
   - durations: `FormatDuration(d) = d.Nanoseconds()`
   - int64/uint64: slog encodes as strings, normalize to numbers

### 11. Error Handling

**Fatal/Panic Semantics:**
- logiface.LevelAlert and logiface.LevelEmergency map to fatal/panic in slog
- slog doesn't have native fatal/panic - call os.Exit() or panic() directly
- Implementation: Check level after mapping, call os.Exit(1) or panic() if Alert/Emergency

**Handle() Errors:**
- slog.Handler.Handle may return errors
- Logiface.Writer[*Event] interface: Write(event) error
- Propagate errors from Handle to caller
- Consider logging errors to stderr if Handle fails

### 12. Context Propagation

Slog supports context-aware logging:

```go
slog.InfoContext(ctx, "message", ...)
```

**Implementation:**
- Event stores context.Context
- Send() passes context to Handler.Handle(ctx, record)
- Default context from Logger if none provided
- Context availability in Handle() for custom handlers

### 13. Source Location

**Required by testsuite:**
- Extract PC from runtime.Callers
- Convert to slog-compatible source struct
- Format: `{"function": "...", "file": "...", "line": 123}`

**Implementation:**
- Record stores PC
- Use runtime.CallersFrames to extract file, line, function
- Add as `"source"` attribute in Handler direction
- Parse from JSON in testsuite reverse direction

## Alternates Considered

### Alternate 1: Manual JSON Buffer
**Approach:** Like stumpy, manually construct JSON buffer in Event.
**Pros:** Maximum control, zero allocations after initialization.
**Cons:** Duplicate slog serialization logic, inconsistent with slog semantics.

### Alternate 2: Dual-Direction Adapter Only
**Approach:** Only provide slog.Handler → logiface.Logger direction, not Logger → Handler.
**Pros:** Simpler, focuses on making slog output work with logiface consumers.
**Cons:** Primary use case is using slog's output mechanisms with logiface's builder API, not vice versa.

### Alternate 3: No Pooling
**Approach:** Allocate new Event each time.
**Pros:** Simpler code, no Reset() complexity.
**Cons:** Higher GC pressure under high load, demonstrated pattern in other adapters uses pooling.

## Rationale Summary

Chosen design follows established adapter patterns (zerolog, logrus, stumpy) while adapting to slog's unique characteristics:
- slog.Attr accumulation slog-native representation
- Pooling for performance
- Support for slog-specific features (LogValuer, Group, ReplaceAttr)
- testsuite support via JSON parsing
- Both directions possible (primary: Logger → Handler, secondary: Handler → Logger)

## Consequences

### Positive
- Enables using slog's flexible output with logiface's builder API
- Consistent with existing logiface adapters
- Full testsuite compliance
- Support for slog-level features (context, LogValuer, groups)

### Negative
- slog has fewer levels than logiface, mapping is lossy
- JSON parsing required for testsuite (potential fragility)
- Fatal/Panic semantics require direct os.Exit/panic calls

### Risks
- slog.Handler implementation complexity (especially WithAttrs/WithGroup chaining)
- testsuite parsing may break if slog JSON format changes
- PC extraction may be fragile across Go versions

## Future Work

1. Implement slog.Handler → logiface.Logger direction if needed
2. Add benchmark comparisons vs logiface-zerolog
3. Explore optimized Attr storage (maybe reuse slice with capacity)
4. Consider streaming Attr addition for very large events
5. Add fuzz tests for LogValuer resolution and Attr conversion

## References
- Go slog spec: https://pkg.go.dev/log/slog
- logiface adapters: logiface-zerolog, logiface-logrus, logiface-stumpy
- logiface-testsuite: logiface-testsuite package
