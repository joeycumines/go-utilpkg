# Emergency Panic Test Failures

## Test Output
```
--- FAIL: Test_TestSuite/TestLoggerLogMethod/enabled_levels_without_modifier/logger=101_arg=emerg (10.00s)
    test_logger_log_method.go:64: expected event
```

## Analysis

Looking at slog.go:429-433:
```go
func (x *Logger) Write(event *Event) error {
    // Emergency level should panic
    if event.lvl == logiface.LevelEmergency {
        panic(logiface.LevelEmergency)
    }
    ...
}
```

The Write() method PANICS when event.lvl is LevelEmergency.

Looking at the testsuite test code (test_logger_log_method.go:36-40):
```go
case logiface.LevelEmergency:
    cfg.HandleEmergencyPanic(t, func() {
        err = tr.Logger.Log(tc.Arg, nil)
    })
```

The test uses `cfg.HandleEmergencyPanic` to handle the panic. Looking at the testsuite code (testsuite.go:238-250):
```go
func (x Config[E]) HandleEmergencyPanic(t *testing.T, fn func()) {
    var ok bool
    if x.EmergencyPanics {
        defer func() {
            if ok {
                t.Fatal(`expected panic`)
            }
            recover()
        }()
    }
    fn()
    ok = true
}
```

The HandleEmergencyPanic function:
1. Sets `ok = false`
2. Runs `fn()` (which calls Logger.Log())
3. If `fn()` panics, `recover()` catches it and returns
4. If no panic occurred, `ok = true` and the test fails with "expected panic"

For logiface-slog, the testSuiteConfig has `EmergencyPanics: true` (slog_test.go:33).

So when Logger.Log(logiface.LevelEmergency, nil) is called:
1. Logger.Log() calls canLog() - which should return true for Emergency
2. Logger.Log() calls newEvent() to create an event with level=Emergency
3. Logger.Log() calls Write(event)
4. Write() panics
5. HandleEmergencyPanic's defer catches the panic and returns normally

But then the test continues (line 63-64):
```go
if ev, ok := tr.ReceiveEvent(); !ok {
    t.Error(`expected event`)
}
```

The test expects an EVENT to be received! But Write() panicked, so no event was written!

This is a TEST SUITE BUG, not an implementation bug!

Wait, let me re-read the HandleEmergencyPanic code...

```go
defer func() {
    if ok {
        t.Fatal(`expected panic`)
    }
    recover()
}()
fn()
ok = true
```

If a panic occurs:
1. defer runs
2. ok is false (not yet set to true)
3. recover() catches the panic
4. Function returns normally

Then the test continues to line 63:
```go
mapped := tr.LevelMapping(tc.Arg)
if !mapped.Enabled() {
    ...
}
if err != nil {
    t.Errorf(`expected no error, got %v`, err)
}

if ev, ok := tr.ReceiveEvent(); !ok {
    t.Error(`expected event`)
}
```

The test expects an event! But Write() panicked, so no event was written!

This is indeed a TEST BUG. The testsuite should NOT expect an event after an Emergency panic!

Or... wait, maybe the panic is supposed to be caught and then an event is still written?

Let me check the logiface specification for Emergency level...

Looking at logiface level.go documentation:
> LevelEmergency is a syslog level.
> Indicates that the system is unusable, a panic condition.
>
> This log level should be used with caution, as it tends to be mapped to
> "panic", which, in at least several logger implementations, will call
> panic().

And the Write() method in slog.go DOES panic for Emergency level.

So the issue is: the testsuite expects an event to be written even for Emergency level!

This is a TEST SUITE BUG. The testsuite should handle Emergency level differently - it should NOT expect an event after a panic.

Actually wait, let me re-read the test code more carefully...

Looking at the test code path:
1. `cfg.HandleEmergencyPanic(t, func() { err = tr.Logger.Log(tc.Arg, nil) })` catches the panic
2. After HandleEmergencyPanic returns, the test checks `if !mapped.Enabled()` - but Emergency is enabled
3. Then it checks `if err != nil` - but err was set inside the panic-handled function
4. Then it expects an event

But the Write() method PANICS before returning an error! So err is never set!

This is the bug: When Write() panics, the error is never captured.

Actually, looking at the Go panic/recover semantics:
- When panic() is called, the function immediately stops
- defer functions run in LIFO order
- If recover() is called in a defer, it catches the panic
- But the original function's return values are not set if panic occurs before return

So in `err = tr.Logger.Log(tc.Arg, nil)`:
- Logger.Log() calls Write()
- Write() panics
- The panic is caught by HandleEmergencyPanic's defer
- But the assignment `err = ...` never completes!
- So err remains nil

Then the test:
1. Checks `if err != nil` - err is nil, so no error
2. Calls `tr.ReceiveEvent()` - expects an event
3. Times out because no event was written (Write() panicked)

This is a TEST SUITE BUG. The testsuite needs to handle the case where Write() panics and no error is set.

Actually, looking at the test more carefully:

```go
case logiface.LevelEmergency:
    cfg.HandleEmergencyPanic(t, func() {
        err = tr.Logger.Log(tc.Arg, nil)
    })
```

The HandleEmergencyPanic catches the panic, so the test continues. But the test expects an event!

I think the issue is: the testsuite expects that Emergency level logs are still written (they just panic after writing).

But the logiface-slog implementation panics BEFORE writing!

Looking at slog.go:429-433:
```go
func (x *Logger) Write(event *Event) error {
    if event.lvl == logiface.LevelEmergency {
        panic(logiface.LevelEmergency)  // <-- PANICS BEFORE WRITING
    }
    ...
}
```

This is the bug! The panic happens BEFORE the slog record is created and written!

Other implementations might:
1. Create the slog record
2. Write it
3. Then panic

But logiface-slog panics immediately when it sees Emergency level.

This is a DESIGN DECISION that differs from what the testsuite expects!

## The Fix

The Write() method should write the record BEFORE panicking for Emergency level:

```go
func (x *Logger) Write(event *Event) error {
    // Check if level is enabled before creating record
    if !x.Handler.Enabled(context.TODO(), toSlogLevel(event.lvl)) {
        return logiface.ErrDisabled
    }
    record := slog.NewRecord(time.Now(), toSlogLevel(event.lvl), event.msg, 0)
    record.AddAttrs(event.attrs...)
    err := x.Handler.Handle(context.TODO(), record)

    // Emergency level should panic AFTER writing
    if event.lvl == logiface.LevelEmergency {
        panic(logiface.LevelEmergency)
    }
    return err
}
```

This way:
1. The log is written
2. Then panic occurs
3. The testsuite receives the event
4. The panic is caught by HandleEmergencyPanic
5. The test passes
