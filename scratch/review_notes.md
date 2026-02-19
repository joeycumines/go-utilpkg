# HOSTILE REVIEW: logiface-slog PR

## Review Focus Areas
1. API Surface - design, alignment with goals
2. Correctness and Robustness - bugs, issues
3. WithJSONSupport performance question

## ADR Files Found
- 001-goja-require-pattern-for-module-initialization.md
- 002-interface-based-option-pattern.md
- 003-panic-vs-error-contracts.md
- 004-prepositions-banned-in-public-api-names.md
- 005-platform-support-requirements.md
- 006-logiface-slog-adapter-design.md

## Implementation Analysis - slog.go

### Event Structure
```go
type Event struct {
    unimplementedEvent  // embedded (lint:ignore U1000)
    msg string
    attrs []slog.Attr
    lvl logiface.Level
}
```

### Logger Structure
```go
type Logger struct {
    Handler slog.Handler
}
```

### Critical Observations

1. **Event Pool Implementation**:
   - sync.Pool with New func creating Event with attrs capacity 8
   - NewEvent: Get() from pool, reset fields, set level
   - ReleaseEvent: Clear fields (lvl=0, msg="", attrs[:0]), Put() back
   - Preserves slice capacity for reuse

2. **Write Flow**:
   - Emergency level check -> panic
   - Handler.Enabled() check -> return ErrDisabled if false
   - slog.NewRecord creation
   - record.AddAttrs(event.attrs...)
   - Handler.Handle(context.TODO(), record)

3. **Level Mapping** (toSlogLevel):
   - Trace/Debug -> Debug
   - Informational -> Info
   - Notice/Warning -> Warn
   - Error/Critical/Alert/Emergency -> Error

## Issues Found So Far

### POTENTIAL ISSUE 1: Event attrs slice sharing
Location: slog.go:440 `record.AddAttrs(event.attrs...)`

The `...` operator passes a slice reference. If the handler retains
a reference to the attrs (e.g., in a buffer), and the event is returned
to the pool and reused, there could be data corruption.

However: slog.NewRecord.AddAttrs copies attrs into the record's internal
storage, so this is likely safe. Need to verify slog implementation.

### POTENTIAL ISSUE 2: AddGroup returns false
Location: slog.go:338-342

Event.AddGroup returns false, which signals flattening. This is
documented behavior. However, if a handler expects nested groups,
this will produce unexpected output.

This is a DESIGN DECISION documented in ADR 006.

### POTENTIAL ISSUE 3: context.TODO() usage
Location: slog.go:436, 441

Write() uses context.TODO() instead of propagating context from caller.
This is because logiface.Writer doesn't accept context.

This is a LIMITATION documented in ADR 006.

### POTENTIAL ISSUE 4: toSlogLevel default case
Location: slog.go:455-456

Default case returns slog.LevelDebug. This is defensive but could
mask bugs if an invalid Level is passed.

Current tests don't exercise this case.

### POTENTIAL ISSUE 5: NewEvent never returns nil
Location: slog.go:358 (comment)

sync.Pool.Get() can return nil if New returns nil, but here New
always returns &Event{}, so this is safe.

However, ReleaseEvent handles nil events defensively, which is
good practice but suggests there may be edge cases.

### POTENTIAL ISSUE 6: Event attrs capacity growth
Location: slog.go:235 (initial capacity: 8)

If fields > 8 are added, the slice grows. When released to pool,
the capacity is preserved. This could lead to memory bloat if
some events have many fields.

However: This is intentional for performance. Most events have < 8 fields.

### POTENTIAL ISSUE 7: Logger.Handler is public
Location: slog.go:152-155

Logger.Handler is a public field. This allows direct manipulation,
which could break invariants.

No getter/setter - direct access.

### POTENTIAL ISSUE 8: No WithJSONSupport implementation
Review Focus 3: Would implementing logiface.WithJSONSupport help?

Current: All fields go through AddField(key, val any) which creates
slog.Any(key, val). slog then handles type-specific encoding.

WithJSONSupport would allow direct JSON marshaling, potentially
avoiding reflect calls in slog.

## TODO: Further Analysis Needed

1. Verify slog.NewRecord.AddAttrs doesn't retain references
2. Check if Handler.Enabled() can have side effects
3. Analyze objectFields.go and arrayFields.go changes
4. Check logiface-slog changes in logiface core
5. Review test coverage gaps
6. Examine WithJSONSupport performance implications
