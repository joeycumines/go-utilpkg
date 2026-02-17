# logiface-slog Limitations

This document describes known limitations and design constraints of the `github.com/joeycumines/logiface-slog` adapter.

## AddGroup Returns False

The `Event.AddGroup()` method always returns `false` to signal to `slog` that the adapter does not support empty/nested group structures directly.

**Rationale:** The adapter uses a flattened attribute model where all fields are accumulated directly into the `[]slog.Attr` slice. Group nesting is handled by the underlying `slog.Handler` via `Handler.WithGroup()` rather than by the `Event` itself.

**Impact:** When `AddGroup()` returns `false`, `slog` falls back to flattening group keys with dot notation (e.g., `"group.key"`). This does not affect functionality but influences how field names appear in output.

**Alternative:** Use `slog.Handler.WithGroup("group_name")` when creating the logger to establish a group context for all fields.

## Level Mapping Coarseness

The adapter maps the 9 `logiface.Level` values to only 4 `slog.Level` values:

| logiface.Level | slog.Level | Notes |
|----------------|-------------|-------|
| Trace | Debug | Lowest log levels share same output |
| Debug | Debug |  |
| Informational | Info |  |
| Notice | Warn | Notice (5) and Warning (6) both map to Warn |
| Warning | Warn |  |
| Error | Error |  |
| Critical | Error | Critical (7), Alert (8), Emergency (9) all map to Error |
| Alert | Error |  |
| Emergency | Error |  |

**Rationale:** `slog` only defines 4 severity levels (Debug, Info, Warn, Error). The adapter compresses the 9 `logiface.Level` values into these 4 categories following Syslog severity conventions.

**Impact:** Fine-grained distinctness between Notice/Warning or Critical/Alert/Emergency is lost in the slog output. All Emergency-level logs appear identically to Error-level in slog handlers.

**Mitigation:** If you need to distinguish between Critical/Alert/Emergency in downstream processing, encode that information in additional fields (e.g., `event.AddField("severity", "critical")`).

## Handler.Enabled() Governs Output, Not logiface Level

The adapter does **not** implement custom level filtering at the `logiface` level. Instead, it delegates filtering to the underlying `slog.Handler` via the `Enabled()` method.

**Rationale:** This respects the handler's configured level (set via `slog.HandlerOptions.Level`) and ensures consistent filtering behavior whether logging directly via `slog` or via the `logiface` adapter.

**Impact:** If you configure `logiface.WithLevel[*Event](logiface.LevelDebug)` but your `slog.Handler` has `HandlerOptions.Level` set to `Info`, `Debug` and `Trace` logs will still be filtered out by the handler.

**Recommendation:** Set levels consistently:
- For `logiface` logger: `logiface.WithLevel[*Event](logiface.LevelDebug)` or rely on handler defaults
- For `slog.Handler`: `&slog.HandlerOptions{Level: slog.LevelDebug}`

## No Nil Guards on Hot Path Methods

The following methods assume non-nil receivers and do not perform defensive nil checks:

- `Event.AddField()`
- `Event.AddMessage()`
- `Event.AddError()`
- `Event.AddGroup()`
- `Logger.Write()`
- `Logger.NewEvent()`

**Rationale:** The `logiface` framework guarantees non-nil receivers in standard usage through the factory pattern (`LoggerFactory.New()`). Removing nil checks from hot paths reduces overhead (verified by benchmarks: `Disabled` logging costs only 0.5 ns/op with nil check avoidance).

**Impact:** If you manually construct `Event` or `Logger` instances without using the factory, calling these methods on nil receivers may panic.

**Mitigation:** Always create loggers via `islog.L.New()` or `logiface.New[*Event]()` with proper options. The factory ensures all dependencies are initialized.

## Pool Reuse Requires Correct Lifecycle Management

The adapter uses `sync.Pool` for `Event` reuse to minimize allocations. Events MUST be released after use via `Logger.ReleaseEvent(event)`.

**Rationale:** Pooling reduces GC pressure and allocation overhead (benchmarks show ~90% of events are reused in high-throughput scenarios).

**Impact:** If you fail to release events, you will exhaust memory over time. If you use an event after releasing it, you will encounter panics due to field resets.

**Correct Pattern:**
```go
event := logger.NewEvent(logiface.LevelInfo)
event.AddMessage("important message")
logger.Write(event)
logger.ReleaseEvent(event) // REQUIRED
```

**Incorrect Pattern:**
```go
event := logger.NewEvent(logiface.LevelInfo)
event.AddMessage("message")
// Missing ReleaseEvent call - memory leak!
```

## No Automatic Source Code Attribution

The adapter does not automatically add `file`, `line`, or `function` fields to events unless the underlying `slog.Handler` is configured with `AddSource: true`.

**Rationale:** Source attribution has significant overhead and is often unnecessary in production. Delegating to `slog.HandlerOptions.AddSource` keeps the adapter minimal and consistent with slog behavior.

**Impact:** If you need source location, configure your handler:

```go
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    AddSource: true, // Adds source to all events
})
```

## Thread Safety Per-Event

`Event` instances are **NOT** thread-safe. Each `Event` must be confined to a single goroutine from creation through release.

**Rationale:** Removing synchronization from `Event` methods eliminates Lock contention, enabling high-throughput parallel logging (benchmarks show 10x throughput improvement by avoiding per-event locks).

**Impact:** If you share an `Event` between goroutines (e.g., passing as an argument to concurrent workers), you will encounter race conditions and data corruption.

**Correct Pattern:**
```go
// Each goroutine creates its own event
for i := 0; i < 100; i++ {
    go func(id int) {
        event := logger.NewEvent(logiface.LevelInfo)
        event.AddField("id", id)
        logger.Write(event)
        logger.ReleaseEvent(event)
    }(i)
}
```

**Incorrect Pattern:**
```go
event := logger.NewEvent(logiface.LevelInfo)
for i := 0; i < 100; i++ {
    go func(id int) {
        event.AddField("id", id) // RACE CONDITION!
    }(i)
}
```

## Compatibility with slog.LogValuer

The adapter does not directly implement custom `slog.LogValuer` processing. Custom types implementing `LogValuer()` are passed through to the underlying `slog.Handler` unchanged.

**Impact:** If you use `slog.LogValuer` types as field values, the handler receives the original value and may call `LogValue()` depending on the handler implementation. This behavior is consistent with direct `slog` usage.

**Recommendation:** Test your custom `LogValuer` types with your chosen handler(s) to ensure they render correctly.

---

For questions or suggestions about these limitations, open an issue on GitHub.
