# WIP.md - The Desperate Diary

## Current Session - COVERAGE GAP ANALYSIS COMPLETE

**Updated:** Session continuation after 10.22h elapsed (exceeded 9h mandate)

## Status
- **Current coverage: 83.2%** (up from 73.5%)
- **28 functions below 100% coverage identified**
- **3 critical gaps at 0% coverage**: WithAttributes, WithGroup, WithReplaceAttr
- Test suite passes
- Phase 1 - Coverage gap analysis complete
- **Task: Adding nil Event safety tests - IN PROGRESS**

## Tasks Completed (blueprint.json status):
1. Event.AddError - nil and non-nil error handling ✅
2. Event.AddInt8 - MinInt8, MaxInt8, positive, negative, zero ✅
3. Event.AddInt16 - MinInt16, MaxInt16 boundary values ✅
4. Event.AddInt32 - MinInt32, MaxInt32 boundary values ✅
5. Event.AddUint - MaxUint32 value ✅
6. Event.AddUint8 - zero, MaxUint8 ✅
7. Event.AddUint16 - MaxUint16 ✅
8. Event.AddUint32 - MaxUint32 ✅
9. Event.AddFloat32 - zero, positive, Pi32 ✅
10. Event.AddBase64Bytes - StdEncoding, URLEncoding, empty bytes ✅
11. Event.AddRawJSON - valid object, array, malformed JSON ✅
12. Event.Level - all 9 logiface levels, nil Event ✅
13. Event.Send - error handlers, nil logger, disabled levels ✅
14. Event.AddField - string, int, slice, map, nil values ✅

## Coverage Progress
**Starting:** 55.6% total statements
**Previous:** 73.5% total statements
**Then:** 83.2% total statements
**Current:** 92.5% total statements (+9.3% from previous, +36.9% from start)
**Target:** 100%
**Remaining gap:** 7.5%

## Remaining Coverage Gaps (6 functions below 100%)

### CRITICAL (0% coverage):
1. **options.WithAttributes** - 0.0%
2. **options.WithGroup** - 0.0%
3. **options.WithReplaceAttr** - 0.0%

### MEDIUM PRIORITY (80-95% coverage):
4. **event.AddBase64Bytes** - 83.3% (missing: nil encoding parameter coverage)
5. **event.Reset** - 90.0%
6. **handler.Handle** - 91.7%
7. **level.toLogifaceLevel** - 85.7%
8. **logger.NewLogger** - 83.3%

## Next Steps (In Priority Order)
1. Add nil Event tests for ALL Add* methods (20+ methods at 75%)
2. Add tests for WithAttributes option function - 0% coverage
3. Add tests for WithGroup option function - 0% coverage
4. Add tests for WithReplaceAttr option function - 0% coverage
5. Add tests for Reset method - 90% coverage
6. Add tests for Handle error paths - 91.7% coverage
7. Add tests for toLogifaceLevel edge cases - 85.7% coverage
8. Add tests for NewLogger with options - 83.3% coverage

## Test Files Modified
- event_test.go: Added 20+ new test functions covering Event methods
  - TestEvent_AddError_Nil
  - TestEvent_AddError_NonNil
  - TestEvent_AddInt8_BoundaryValues
  - TestEvent_AddInt8_PositiveNegative
  - TestEvent_AddInt16_BoundaryValues
  - TestEvent_AddInt32_BoundaryValues
  - TestEvent_AddUint_MaxUint32
  - TestEvent_AddUint8_BoundaryValues
  - TestEvent_AddUint16_BoundaryValues
  - TestEvent_AddUint32_BoundaryValues
  - TestEvent_AddFloat32_SmallNumbers
  - TestEvent_AddBase64Bytes_StdEncoding
  - TestEvent_AddBase64Bytes_URLEncoding
  - TestEvent_AddBase64Bytes_Empty
  - TestEvent_AddRawJSON_ValidObject
  - TestEvent_AddRawJSON_Malformed
  - TestEvent_AddRawJSON_ValidArray
  - TestEvent_Level_LogifaceLevels
  - TestEvent_Level_Nil
  - TestEvent_Send_ErrorInHandle
  - TestEvent_Send_NilLogger
  - TestEvent_Send_EnabledBeforeSend
  - TestEvent_AddField_String
  - TestEvent_AddField_Int
  - TestEvent_AddField_Slice
  - TestEvent_AddField_Map
  - TestEvent_AddField_Nil

## Working Directory
/Users/joeyc/dev/go-utilpkg/logiface-slog

## Command History
```
go test -race ./logiface-slog/...                # PASS
go test -coverprofile=coverage.out               # 83.2% coverage
go tool cover -func=coverage.out | grep -v 100%  # 28 functions below 100%
```

---



## slog.Handler Interface & Record Structure

### slog.Handler Interface:

```go
type Handler interface {
    Enabled(ctx context.Context, level slog.Level) bool
    Handle(ctx context.Context, r slog.Record) error
    WithAttrs(attrs []slog.Attr) slog.Handler
    WithGroup(name string) slog.Handler
}
```

- **Enabled(ctx, level)**: Pre-filtering check. If returns false, Handle() not called
- **Handle(ctx, r)**: Core method - processes/writes the log entry
- **WithAttrs(attrs)**: Returns new Handler with attributes pre-pended to all future logs
- **WithGroup(name)**: Returns new Handler that adds group prefix to all keys

### slog.Record Structure:

```go
type Record struct {
    Time      time.Time      // Timestamp of log
    Level     slog.Level     // Log level
    Message   string         // Log message
    PC        uintptr        // Program counter (call site for source location)
    // Attributes accessed via Attrs(func) iterator
}
```

**Methods:**
- `Add(attr slog.Attr)` - Add attribute to record
- `Attrs(f func(slog.Attr) bool)` - Iterate all attributes
- `NumAttrs() int` - Count attributes
- `Clone() slog.Record` - Return independent copy (important for thread safety)

### slog.Level Values:
- `LevelDebug = -4`
- `LevelInfo = 0`
- `LevelWarn = 4`
- `LevelError = 8`
- Can also have negative values for custom/levels between standard ones

### slog.Value Kind Enum:
- `KindString` - string values
- `KindInt64` - 64-bit signed integers
- `KindUint64` - 64-bit unsigned integers
- `KindFloat64` - 64-bit floating point
- `KindBool` - boolean
- `KindDuration` - time.Duration
- `KindTime` - time.Time
- `KindGroup` - nested attributes (group key -> []slog.Attr)
- `KindAny` - arbitrary Go type
- `KindLogValuer` - value implementing LogValuer interface

### slog.Attr Constructors:
```go
type Attr struct {
    Key   string
    Value Value
}

slog.String(key, value)    slog.Attr
slog.Int64(key, value)    slog.Attr
slog.Uint64(key, value)   slog.Attr
slog.Float64(key, value)  slog.Attr
slog.Bool(key, value)     slog.Attr
slog.Duration(key, value) slog.Attr
slog.Time(key, value)     slog.Attr
slog.Group(key, attrs...) slog.Attr
slog.Any(key, value)      slog.Attr
```

### LogValuer Interface:
```go
type LogValuer interface {
    LogValue() slog.Value
}
```
- Custom types can implement LogValuer to provide their own slog representation
- slog.Handle() calls LogValue() on LogValuer values recursively
- Important for types like `error` which implement LogValuer automatically

### Source Location Extraction:
```go
pc, _, _, _ := runtime.Caller(skip)
frames := runtime.CallersFrames([]uintptr{pc})
frame, _ := frames.Next()
// frame.File, frame.Line, frame.Function are available
```
- PC is program counter pointing to call site
- Extracted to `"source"` field: `{"file": "path/to/file.go", "line": 123, "function": "pkg.Func"}`

### ReplaceAttr Hook:
```go
type HandlerOptions struct {
    AddSource   bool
    Level       slog.Level
    ReplaceAttr func(groups []string, a slog.Attr) slog.Attr
}
```
- ReplaceAttr called for each attribute before output
- `groups` - current group nesting stack (empty means at root)
- `a` - attribute to potentially modify/skip/filter
- Return zero Attr to skip the attribute
- Can modify keys (redaction), values, or filter entirely

### slog-slog Design Implications:

**Adapter Directionality:**
Primary direction: logiface.Logger[*Event] → slog.Handler
Event accumulates slog.Attr, sends via slog.Handler.Handle()

**Level Mapping:**
- logiface.LevelTrace → slog.LevelDebug
- logiface.LevelDebug → slog.LevelDebug
- logiface.LevelInformational → slog.LevelInfo
- logiface.LevelNotice → slog.LevelWarn
- logiface.LevelWarning → slog.LevelWarn
- logiface.LevelError → slog.LevelError
- logiface.LevelCritical → slog.LevelError
- logiface.LevelAlert → slog.LevelError (mapped to fatal)
- logiface.LevelEmergency → slog.LevelError (mapped to panic)

**Event.Send() Implementation:**
```go
// Construct slog.Record
record := slog.NewRecord(time.Now(), slogLevel, message, pc)
for _, attr := range event.attrs {
    record.Add(attr)
}
// Call Handler
handler.Handle(ctx, record)
```

## Testsuite Requirements

### Required Interfaces and Methods:

**Event Interface Methods:**
- `Level() logiface.Level` - Required
- `AddField(key, val)` - Add generic field
- `AddMessage(msg) bool` - Set message, return true
- `AddError(err) bool` - Set error, return true
- `AddString(key, val) bool`
- `AddInt(key, val) bool`
- `AddFloat32(key, val) bool`
- `AddFloat64(key, val) bool`
- `AddBool(key, val) bool`
- `AddInt64(key, val) bool`
- `AddUint64(key, val) bool`
- `AddTime(key, val time.Time) bool`
- `AddDuration(key, val time.Duration) bool`
- `AddBase64Bytes(key, val []byte, enc base64.Encoding) bool`
- `AddRawJSON(key, val json.RawMessage) bool`

**Logger Interface Methods:**
- `NewEvent(level) *Event` - EventFactory
- `Write(event) error` - Writer
- `ReleaseEvent(event)` - EventReleaser

**JSONSupport (Optional but Recommended):**
- Returns true for CanAddRawJSON, CanAddFields, CanAddLazyFields

### ParseEvent Function:
**CRITICAL:** Must parse JSON from io.Reader and convert to:
```go
type Event struct {
    Message *string                     // May be nil
    Error   *string                     // May be nil
    Fields  map[string]interface{}      // Excludes: message, error, timestamp, level
    Level   logiface.Level
}
```
- **Message**: Extracted from message field (key configurable)
- **Error**: Extracted from error field (key configurable)
- **Fields**: All other fields (but NOT auto-added fields like time, level)
- **EOF**: Return (nil, nil)

### Normalizer Functions:
Testsuite compares event output using custom normalizers:
- `FormatTime(time.Time)` - Convert to comparable format
- `FormatDuration(time.Duration)` - Compare as ns or formatted string
- `FormatBase64Bytes([]byte, base64.Encoding)` - Convert base64 bytes
- `FormatInt64(int64)` - Map logiface.Event.AddInt64 formatting
- `FormatUint64(uint64)` - Map logiface.Event.AddUint64 formatting

### Test Templates:
10 event templates test all Add* methods with various:
- Positive/negative numbers
- min/max values (MaxInt64, MaxFloat64, etc.)
- nil values
- zero values
- RawJSON fragments
- Empty/nil errors
- Field ordering variations

### Level Mapping:
`LevelMapping(func(logiface.Level) logiface.Level)`:
- Some logiface levels may map to `LevelDisabled`
- Testsuite uses mapping to compare expected vs actual levels

### slog-slog Design Decision:

**ParseEvent Implementation:**
Since slog outputs to slog.Handler (JSONHandler/TextHandler):
1. Parse JSON output from underlying slog.Handler
2. Extract slog.Level to logiface.Level
3. Extract message, error fields
4. Remove standard slog fields: time, level, source, msg
5. Return remaining fields in Fields map

**Normalizer for slog:**
- `FormatTime`: slog times are nanoseconds, normalize to YYYY-MM-DD HH:MM:SS
- `FormatDuration`: slog uses string duration, normalize to nanoseconds
- `FormatInt64/Uint64`: slog encodes as strings ("12345"), normalize to numbers

## Stumpy Event Implementation Details

### Event Struct Fields:
- `logger *Logger` - back-reference for delegation
- `buf []byte` - incomplete JSON buffer (missing closing "}")
- `off []int` - stack of offset indices for deferred key insertion (negative = key already set)
- `lvl logiface.Level` - log level

### JSON Buffer Management:
1. **Start**: `buf = append(buf[:0], '{')` on NewEvent()
2. **Incomplete**: Always missing closing '}' (added in Write())
3. **No trailing newline**: Custom writers control formatting

### Offset Stack Pattern (Nested Objects):
- `enterKey("")`: Pushes `len(buf)+1` to off when key unknown
- `enterKey("name")`: Pushes `-1` when key known immediately
- `exitKey("name")`: If off >= 0, inserts key at saved offset
- **Purpose**: JSON structure built before key names known (deferred insertion)

### append* Methods (Private):
- `appendString`, `appendInt`, `appendFloat64`, `appendBool` etc.
- Use optimized jsonenc package functions
- Avoid json.Marshal overhead for simple types

### Method Interfaces:
- `Add*`: For Event field addition at current position
- `Set*`: For JSONSupport object field setting
- `Append*`: For JSONSupport array element adding
- Each has `Can*` method returning true

### Pool Reset Warning:
- Comment: "WARNING: if adding fields consider if they may need to be reset"
- stumpy only resets `logger = nil` on ReleaseEvent()
- Slices reused across pool cycles (`buf`, `off`)

### slog-slog Design Decision:
Will NOT use manual buffer approach - will:
1. Store `[]slog.Attr` field accumulator
2. Use `slog.Any()` for lazy evaluation
3. Convert Attr slice to slog.Record on Send()

## NINE HOUR MANDATE TRACKING
**START TIME:** 1771081832
**START_TIME_STR:** Sun Feb 15 01:10:32 AEST 2026
**RECORD FILE:** /tmp/nine_hours_logiface_slog.txt

## Key Patterns Documented from Existing Adapters

### Common Event Design Patterns:
1. **Embed UnimplementedEvent**: All adapters embed `logiface.UnimplementedEvent` for default implementations
2. **Level field**: All store `lvl logiface.Level` for Log() method implementation
3. **Logger reference**: Store back-reference to Logger for delegation

### Common Logger Design Patterns:
1. **sync.Pool for events**: All adapters use `sync.Pool{New: func() any { return new(Event) }}`
2. **NewEvent()**: Acquire from pool, initialize fields (level, logger reference)
3. **ReleaseEvent()**: Reset struct fields, pool.Put(e)
4. **Write()**: Delegate to underlying logger (Z.Msg(), entry.Log(), x.writer.Write())

### Level Mapping Patterns:
- **zerolog**: `newEvent()` switch-case maps logiface.Level to zerolog.Level
- **logrus**: `toLogrusLevel()` function maps types
- **slog design decision**: Create `toSlogLevel()` and `toLogifaceLevel()` functions

### Field Addition Patterns:
- **zerolog/logrus-simple**: Event has Add* methods that delegate to underlying logger's field methods
- **stumpy-manual**: Event manages JSON buffer manually with nested structure tracking (enterKey/exitKey)
- **slog-slog design**: Event will accumulate slog.Attr in slice/field builder, use slog.Any() for lazy evaluation

### JSONSupport Implementation:
- **zerolog**: Full Can* methods returning true, delegates to zerolog.Event
- **stumpy**: Partial implementation for nested JSON objects
- **slog-slog design**: Should implement JSONSupport returning true, use slog.Value struct (supports Group, Any, etc.)

### Pool Reset Patterns:
- **zerolog**: `*event = Event{}` (simple reset)
- **logrus**: `maps.Clear(event.Entry.Data)`, `eventPool.Put(e)`
- **stumpy**: Capacity-based filtering and explicit field clearing

## DEEP, DISTINCT ARCHITECTURAL PLANNING PHASE - COMPLETE
**Session Time:** Exceeded 9 hour mandate (10.22h elapsed)
**Plan Status:** EXHAUSTIVELY COMPLETE blueprint.json with 143+ tasks, 16 phases
**Current Coverage:** 55.6% (target: 100%)
**Next Phase:** Phase 1 - Core Functionality Unit Tests (achieve 100% coverage)

## Blueprint Summary
**Total Tasks:** 143+ tasks across 16 phases
- Phase 1: Core Functionality Unit Tests - Complete Coverage Gaps (34 tasks)
- Phase 2: Advanced Functionality Testing (32+ tasks)
- Phase 3: logiface-testsuite Integration (7 tasks)
- Phase 4: Cross-Platform Verification (6 tasks)
- Phase 5: Static Analysis and Code Quality (7 tasks)
- Phase 6: Documentation Completeness (9 tasks)
- Phase 7: Performance Optimization and Profiling (8 tasks)
- Phase 8: Fuzz Testing and Robustness (6 tasks)
- Phase 9: Integration with Project Build System (6 tasks)
- Phase 10: Final Verification and Validation (18 tasks)
- Phase 11: Continuous Improvement - API Surface Expansion (7 tasks)
- Phase 12: Security and Hardening (3 tasks)
- Phase 13: Observability and Monitoring Integration (3 tasks)
- Phase 14: Developer Experience Improvements (3 tasks)
- Phase 15: Rule of Two Verification (2 tasks)
- Phase 16: Final Comprehensive Re-verification (8 tasks)

## Phase 1 Execution - IN PROGRESS
**Task 1:** Add unit tests for Event.AddError method
**Coverage Target:** 100%
**Current Progress:** Starting Phase 1

## Immediate Next Actions
- Implement test for Event.AddError
- Continue systematically through all uncovered functions
- Verify 100% coverage after Phase 1 completes
- Execute Rule of Two verification before proceeding to Phase 2

## Uncovered Functions (Must be tested in Phase 1)
- event.go: AddError, AddInt8, AddInt16, AddInt32, AddUint, AddUint8, AddUint16, AddUint32, AddFloat32, AddBase64Bytes, AddRawJSON
- event.go: Level (66.7%), Send (84.6%), AddField (66.7%)
- handler.go: Enabled (0% coverage)
- level.go: toSlogLevel (83.3%), toLogifaceLevel (50.0%)
- logger.go: WithAttributes, WithGroup, WithReplaceAttr, CanAddRawJSON, CanAddFields, CanAddLazyFields, Close, getGroupPrefix
- logger.go: Write (42.9%), NewLogger (83.3%), NewEvent (90.0%)

## Current Completed Work (Tasks 1-45)
- ✅ Architecture analysis and design documentation
- ✅ Module initialization (go.mod, doc.go)
- ✅ Core Event struct with pooling (event.go)
- ✅ Logger implementing all logiface interfaces (logger.go)
- ✅ SlogHandler for reverse direction (handler.go)
- ✅ Level mapping functions (level.go)
- ✅ Options pattern implementation (options.go)
- ✅ Comprehensive test suites:
  - Handler record conversion (all Value kinds)
  - Attribute grouping (single, nested, mixed)
  - WithAttrs accumulation and stacking
  - LogValuer resolution (struct, pointer, nested, error)
  - Context propagation (nil, cancelled, timeout)
  - Event pooling (reuse, reset, concurrent)
  - Integration tests (slog.New, slog.SetDefault, all levels)
  - Benchmark tests (basic coverage)

## Current Gaps Identified
- ⏳ Missing unit tests for specific behaviors (Tasks 46-85)
- ⏳ Missing testsuite integration with logiface-testsuite
- ⏳ Missing cross-platform verification
- ⏳ Missing documentation (godoc comments, README, CHANGELOG)
- ⏳ Missing static analysis (vet, staticcheck)
- ⏳ Missing 100% coverage verification
- ⏳ Missing performance optimization and profiling
- ⏳ Missing fuzz testing for robustness
- ⏳ Missing grit publishing configuration
- ⏳ Missing ADR documentation
- ⏳ Missing integration with other modules (goja-eventloop, etc.)
- ⏳ Missing Rule of Two verification cycles

## SCOPE EXPANSION OPPORTUNITIES
Hana-san demands the wolf of perfection be fed. Beyond the current 122 tasks:
1. **API Surface Refinements**: Add convenience methods, improve ergonomics
2. **Additional Adapters**: Support for other structured logging formats
3. **Integration Patterns**: Better examples and patterns for real-world usage
4. **Performance Optimization**: Hot path optimization, memory reduction
5. **Security Hardening**: Input validation, redaction examples
6. **Observability**: Metrics on adapter performance
7. **Documentation Expansion**: Video-guided tutorials, interactive examples
8. **Testing Expansion**: Property-based testing, chaos engineering
9. **Developer Experience**: Tooling for log analysis, debugging helpers

## Architecture Analysis
**Current Understanding:**
- slog is in Go stdlib (since Go 1.21)
- slog.Handler interface: Enabled(ctx, level), Handle(ctx, r Record), WithAttrs(attrs), WithGroup(name)
- slog.Record: Time, Level, Message, PC, Attrs
- slog.Attr: Key, Value (with many kinds: String, Int64, Uint64, Float64, Bool, Duration, Time, Group, Any, LogValuer)
- blueprint needs bidirectional support: slog.Handler ←→ logiface.Logger

**Adapter Pattern:**
Primary direction appears to be:
- logiface.Logger[*Event] that writes to slog.Handler
- Allows using slog's output mechanisms with logiface's builder API
- Event stores slog.Attr, sends via slog.Handler.Handle()

**Key Differences from Existing Adapters:**
- zerolog/logrus/stumpy: logiface.Logger writes to [zerolog|logrus|stumpy] directly
- logiface-slog: logiface.Logger writes to slog.Handler interface

## Immediate Next Steps
1. Start NINE HOUR time tracking (create file)
2. Mark first blueprint task as In Progress
3. Execute tasks sequentially through blueprint
4. After each task completion, update blueprint status
5. Run Rule of Two verification periodically

## Time Tracking Verification
**Will verify:** Calculate duration between start and end points using date arithmetic
**File:** /tmp/nine_hours_logiface_slog.txt contains timestamps

## PROGRESS - Tasks 1-8 COMPLETE
✓ Task 1-4: Completed (reviews of existing adapters, testsuite, slog API)
✓ Task 5: Created docs/adr/logiface-slog-architecture.md
✓ Task 6: Initialized logiface-slog module directory
✓ Task 7: Created slog.Event struct with pool, Reset(), Send()
✓ Task 8: Created Logger with NewLogger implementing interfaces

## BUILD STATUS
**PASS:** logiface-slog module builds successfully

## Current Task Status
- Tasks 1-8: Done
- Task 9: NEXT - "Implement slog.Logger methods: Debug, Info, Warn, Error"
- Remaining tasks: ~104

## Next Actions
1. Mark Task 9 as In Progress
2. Implement Debug(), Info(), Warn(), Error() methods
3. Update blueprint as tasks complete

## Notes
- Directive mandates: "cycle continually and indefinitely"
- Rule of Two verification required before any task marked complete
- Two contiguous, issue-free verification cycles
- Zero tolerance for test failures
- 100% coverage requirement
- ALL platforms (Darwin, Linux, Windows)

## Critical Warnings from Directive
- Timing dependent tests are BANNED
- Never commit before Rule of Two verification
- Use custom make targets with limited output (tail)
- Use build.log for searching output

## Architectural Questions to Resolve
1. Should logiface-slog also implement slog.Handler directly (inverse direction)?
2. How to handle slog.LogValuer interface?
3. How to handle slog.ReplaceAttr hooks?
4. Event pooling strategy for slog.Attr storage?
5. Level mapping between logiface.Level and slog.Level
6. Source location extraction from PC?
7. Context propagation through Handle?
