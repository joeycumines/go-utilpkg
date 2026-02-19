# CRITICAL: Test Failures Analysis

## Test Output
```
--- FAIL: Test_TestSuite/TestLoggerLogMethod/enabled_levels_without_modifier/logger=-101_arg=101 (0.00s)
    test_logger_log_method.go:54: expected logiface.ErrDisabled, got <nil>
    test_logger_log_method.go:56: unexpected event: level=debug message=""
```

## Analysis

The test expects `logiface.ErrDisabled` when logging at certain levels, but gets `nil` instead.

Looking at slog.go:429-442 (Write method):
```go
func (x *Logger) Write(event *Event) error {
    // Emergency level should panic
    if event.lvl == logiface.LevelEmergency {
        panic(logiface.LevelEmergency)
    }
    // Check if level is enabled before creating record
    if !x.Handler.Enabled(context.TODO(), toSlogLevel(event.lvl)) {
        return logiface.ErrDisabled
    }
    record := slog.NewRecord(time.Now(), toSlogLevel(event.lvl), event.msg, 0)
    record.AddAttrs(event.attrs...)
    return x.Handler.Handle(context.TODO(), record)
}
```

## Issue: Handler.Enabled() returns TRUE for disabled loggers?

Wait - the test is using `WithLevel[*Event](logiface.LevelDebug)` to set the logger level.
But the Write method checks `x.Handler.Enabled()` - the slog handler's level, not the logiface level!

The slog handler in testsuite is created with:
```go
handler := slog.NewJSONHandler(req.Writer, &slog.HandlerOptions{
    Level:       slog.LevelDebug,
    ...
})
```

So the slog handler is set to LevelDebug, which means it will accept ALL logs.

But the test is configured with a different logiface level, and expects that level to be respected!

## The Bug

The Write() method ONLY checks `x.Handler.Enabled()` and does NOT check the logiface-level configuration!

The `WithLevel[*Event]` option sets a level in the logiface logger configuration,
but the Write() method ignores this and only checks the slog handler's level.

This means the logiface.WithLevel option is BROKEN for logiface-slog!

## Expected Behavior

The Write() method should check BOTH:
1. The logiface level configuration (from WithLevel option)
2. The slog handler's Enabled() method

Currently it only checks #2.

## Code Evidence

Looking at slog_test.go:29-53:
```go
func testSuiteLoggerFactory(req testsuite.LoggerRequest[*Event]) testsuite.LoggerResponse[*Event] {
    handler := slog.NewJSONHandler(req.Writer, &slog.HandlerOptions{
        Level:       slog.LevelDebug,  // <-- Always Debug level!
        ...
    })

    var options []logiface.Option[*Event]
    options = append(options, L.WithSlogHandler(handler))
    options = append(options, req.Options...)  // <-- Includes WithLevel from testsuite

    return testsuite.LoggerResponse[*Event]{
        Logger:       L.New(options...),
        ...
    }
}
```

The testsuite passes `req.Options` which includes `WithLevel[*Event](level)`.
But the Write() method doesn't check this - it only checks the slog handler's level!

## Deeper Analysis - The Real Bug

Actually, looking at logger.go:295-318 (Logger.Log method):

```go
func (x *Logger[E]) Log(level Level, modifier Modifier[E]) error {
    if !x.canLog(level) {  // <-- Checks canLog BEFORE calling Write!
        return ErrDisabled
    }
    event := x.newEvent(level)
    ...
    return x.shared.writer.Write(event)
}
```

And canLog() is (logger.go:461-465):
```go
func (x *Logger[E]) canLog(level Level) bool {
    return x.Enabled() &&
        level.Enabled() &&
        (level <= x.shared.level || level > LevelTrace)
}
```

The condition `(level > LevelTrace)` means CUSTOM LEVELS ALWAYS PASS THE LEVEL CHECK!

This is documented in level.go:76-80:
> Custom levels are handled differently than regular levels, in that they
> are not affected by the log level set on the Logger.

So for test case `logger=-101_arg=101`:
- Logger level is -101
- Arg level is 101 (custom level > LevelTrace=8)
- canLog() returns TRUE because 101 > 8
- Write() is called and returns nil

But the testSuiteLevelMapping says custom levels map to LevelDisabled!

## The Mismatch

There's a fundamental mismatch between:

1. **logiface core**: Custom levels (9-127) bypass level filtering
2. **testsuite expectation**: Custom levels should be disabled
3. **islog's LevelMapping**: Returns LevelDisabled for custom levels

## The Actual Bug

The testSuiteLevelMapping is WRONG for islog!

The mapping returns LevelDisabled for custom levels, but islog's Write()
method accepts custom levels (they map to slog.LevelDebug via default case).

So when the test checks `mapped.Enabled()`, it gets false, and expects ErrDisabled.
But Write() was called (because canLog allowed it), and returned nil.

## The Fix

The testSuiteLevelMapping should NOT return LevelDisabled for custom levels.
Instead, it should map custom levels appropriately.

Looking at toSlogLevel:
```go
func toSlogLevel(level logiface.Level) slog.Level {
    switch level {
    case logiface.LevelTrace, logiface.LevelDebug:
        return slog.LevelDebug
    case logiface.LevelInformational:
        return slog.LevelInfo
    case logiface.LevelNotice, logiface.LevelWarning:
        return slog.LevelWarn
    case logiface.LevelError, logiface.LevelCritical, logiface.LevelAlert, logiface.LevelEmergency:
        return slog.LevelError
    default:
        return slog.LevelDebug  // <-- Custom levels map to Debug!
    }
}
```

So custom levels DO work in islog - they map to slog.LevelDebug!

The fix is to update testSuiteLevelMapping to match this behavior.

The logiface.Logger contains a Writer[*Event]. The islog.Logger implements Writer[*Event].
The logiface level is checked BEFORE calling Writer.Write()!

So the testsuite expects the logiface level check to happen in logiface.Logger,
not in the islog.Writer.Write() method.

Let me check the test failures again... The test says it's calling `logger.Log()` which
should check the level before calling Write().

Hmm, but the test is getting an unexpected event with level=debug. This means
Write() WAS called and created a slog record.

Wait - maybe the issue is that the test is passing a custom level that doesn't
map correctly, or the level mapping has an issue?

Let me look at the test more carefully...

The test argument is `logger=-101_arg=101`. -101 and 101 are not standard logiface levels!

This is testing edge cases with invalid/custom level values.

The toSlogLevel function has a default case:
```go
default:
    return slog.LevelDebug
```

So invalid levels map to Debug, which the slog handler accepts!

The issue is: the test expects logiface.ErrDisabled for these invalid levels,
but the Write() method is returning nil (success).

The root cause: logiface level filtering should reject invalid/custom levels BEFORE
calling Write(), but that's not happening.

Actually, looking at the test name "enabled_levels_without_modifier", it seems like
the test is checking that disabled levels return ErrDisabled.

The issue might be in how the testsuite is checking the level, or how the logger
is configured for the test.

Let me trace through what level=-101 means...

Looking at logiface level constants, -101 would be LevelDisabled (since LevelDisabled
has value 0 and Enabled() checks return false for it).

Actually wait - I need to understand the logiface level system better.

Let me look at how Level.Enabled() works...
