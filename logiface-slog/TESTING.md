# Testing Guide for logiface-slog

This guide demonstrates testing strategies for code that uses `logiface-slog` (islog) adapter.

## Overview

Testing logging code requires capturing log output and verifying its contents. This guide shows:

1. **Buffer Handler**: Capture logs in memory for test verification
2. **Assertion Helpers**: Validate log levels, messages, and fields
3. **Mocking Pattern**: Inject test-specific loggers into code
4. **Integration Testing**: Test with real handlers (JSON, text)
5. **Logiface-Testsuite**: Leverage 99,501 parameterized tests

## 1. Buffer Handler Pattern

Capture logs in a `bytes.Buffer` for verification:

```go
package islog_test

import (
    "bytes"
    "encoding/json"
    "log/slog"
    "testing"

    "github.com/joeycumines/logiface"
    "github.com/joeycumines/logiface-slog"
)

func TestBufferHandler_CapturesLogs(t *testing.T) {
    var buf bytes.Buffer
    handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
        Level: slog.LevelDebug,
    })
    logger := logiface.New[*islog.Event](islog.WithSlogHandler(handler))

    // Log some events
    event := logger.NewEvent(logiface.LevelInformational)
    event.AddMessage("test message")
    event.AddField("key", "value")
    logger.Write(logger.ReleaseEvent(event))

    // Parse captured JSON
    output := buf.String()
    var logMap map[string]any
    if err := json.Unmarshal([]byte(output), &logMap); err != nil {
        t.Fatalf("Failed to parse log: %v", err)
    }

    // Verify log content
    if logMap["msg"] != "test message" {
        t.Errorf("Expected message 'test message', got %v", logMap["msg"])
    }
    if logMap["key"] != "value" {
        t.Errorf("Expected key='value', got %v", logMap["key"])
    }
    if logMap["level"] != "INFO" {
        t.Errorf("Expected level INFO, got %v", logMap["level"])
    }
}
```

## 2. Assertion Helpers

Create reusable helper functions for common assertions:

```go

// LogEntry represents a parsed log entry for test assertions
type LogEntry struct {
    Level   string                 `json:"level"`
    Message string                 `json:"msg"`
    Fields  map[string]any        `json:"-"`
    Raw     map[string]any        `json:"-"`
}

// ParseJSONLogs parses JSON log lines into LogEntry structs
func ParseJSONLogs(t *testing.T, buf *bytes.Buffer) []LogEntry {
    lines := bytes.Split(buf.Bytes(), []byte{'\n'})
    entries := make([]LogEntry, 0, len(lines))

    for _, line := range lines {
        if len(line) == 0 {
            continue
        }

        var entry LogEntry
        if err := json.Unmarshal(line, &entry); err != nil {
            t.Fatalf("Failed to parse log line: %v\n%s", err, line)
        }

        // Copy all fields into Fields map
        entry.Fields = make(map[string]any)
        entry.Raw = make(map[string]any)
        json.Unmarshal(line, &entry.Raw) // Keep raw for custom assertions

        // Extract known fields into Fields
        for key, val := range entry.Raw {
            if key != "level" && key != "msg" && key != "time" {
                entry.Fields[key] = val
            }
        }

        entries = append(entries, entry)
    }

    return entries
}

// AssertLogHasField asserts that a log entry contains a field
func AssertLogHasField(t *testing.T, entry LogEntry, key string, expected any) {
    t.Helper()

    actual, ok := entry.Fields[key]
    if !ok {
        t.Errorf("Log entry missing field '%s'. Available fields: %v", key, entry.Fields)
        return
    }

    if actual != expected {
        t.Errorf("Field '%s' = %v, expected %v", key, actual, expected)
    }
}

// AssertLogLevel asserts that a log entry has the expected level
func AssertLogLevel(t *testing.T, entry LogEntry, level string) {
    t.Helper()
    if entry.Level != level {
        t.Errorf("Log level = %s, expected %s", entry.Level, level)
    }
}

// AssertLogMessage asserts that a log entry has the expected message
func AssertLogMessage(t *testing.T, entry LogEntry, message string) {
    t.Helper()
    if entry.Message != message {
        t.Errorf("Log message = %q, expected %q", entry.Message, message)
    }
}
```

**Usage:**

```go
func TestWithAssertionHelpers(t *testing.T) {
    var buf bytes.Buffer
    handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
    logger := logiface.New[*islog.Event](islog.WithSlogHandler(handler))

    // Log some events
    event := logger.NewEvent(logiface.LevelError)
    event.AddMessage("database connection failed")
    event.AddField("database", "postgres")
    event.AddField("host", "db.example.com")
    logger.Write(logger.ReleaseEvent(event))

    // Parse and assert
    entries := ParseJSONLogs(t, &buf)
    if len(entries) != 1 {
        t.Fatalf("Expected 1 log entry, got %d", len(entries))
    }

    entry := entries[0]
    AssertLogLevel(t, entry, "ERROR")
    AssertLogMessage(t, entry, "database connection failed")
    AssertLogHasField(t, entry, "database", "postgres")
    AssertLogHasField(t, entry, "host", "db.example.com")
}
```

## 3. Mocking Pattern

Pass logger as a parameter to enable test injection:

```go
// Application code - accepts logger interface
type Service struct {
    logger *logiface.Logger[*islog.Event]
}

func NewService(logger *logiface.Logger[*islog.Event]) *Service {
    return &Service{logger: logger}
}

func (s *Service) ProcessRequest(req Request) error {
    event := s.logger.NewEvent(logiface.LevelInformational)
    event.AddMessage("processing request")
    event.AddField("request_id", req.ID)
    s.logger.Write(s.logger.ReleaseEvent(event))

    // ... processing logic

    return nil
}
```

**Test with mock logger:**

```go
func TestService_ProcessRequest_LogsCorrectly(t *testing.T) {
    var buf bytes.Buffer
    handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
    logger := logiface.New[*islog.Event](islog.WithSlogHandler(handler))

    service := NewService(logger)

    req := Request{ID: "req-123"}
    err := service.ProcessRequest(req)

    if err != nil {
        t.Fatalf("ProcessRequest failed: %v", err)
    }

    // Verify log captured
    entries := ParseJSONLogs(t, &buf)
    if len(entries) != 1 {
        t.Fatalf("Expected 1 log, got %d", len(entries))
    }

    entry := entries[0]
    AssertLogMessage(t, entry, "processing request")
    AssertLogHasField(t, entry, "request_id", "req-123")
}
```

## 4. Interface Injection Pattern

For maximum testability, define a logger interface:

```go
// Logger interface abstracts logging for test injection
type Logger interface {
    NewEvent(level logiface.Level) *islog.Event
    Write(event *islog.Event) error
    ReleaseEvent(event *islog.Event)
}

// RealLogger wraps logiface.Logger[*islog.Event]
type RealLogger struct {
    *logiface.Logger[*islog.Event]
}

func NewRealLogger(handler slog.Handler) Logger {
    return &RealLogger{Logger: logiface.New[*islog.Event](islog.WithSlogHandler(handler))}
}

// TestLogger captures logs for testing
type TestLogger struct {
    buf     bytes.Buffer
    handler slog.Handler
    *logiface.Logger[*islog.Event]
}

func NewTestLogger() *TestLogger {
    handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
    return &TestLogger{
        buf:     buf,
        handler: handler,
        Logger:  logiface.New[*islog.Event](islog.WithSlogHandler(handler)),
    }
}

func (tl *TestLogger) GetLogs() []LogEntry {
    return ParseJSONLogs(&testing.T{}, &tl.buf)
}

func (tl *TestLogger) ClearLogs() {
    tl.buf.Reset()
}
```

**Usage in production code:**

```go
type Processor struct {
    log Logger
}

func NewProcessor(log Logger) *Processor {
    return &Processor{log: log}
}

func (p *Processor) Process(data string) {
    event := p.log.NewEvent(logiface.LevelInformational)
    event.AddMessage("processing")
    p.log.Write(p.log.ReleaseEvent(event))
}
```

**Usage in tests:**

```go
func TestProcessor_Process_LogsCorrectly(t *testing.T) {
    testLogger := NewTestLogger()
    processor := NewProcessor(testLogger)

    processor.Process("test data")

    logs := testLogger.GetLogs()
    if len(logs) != 1 {
        t.Fatalf("Expected 1 log, got %d", len(logs))
    }

    AssertLogMessage(t, logs[0], "processing")
}
```

## 5. Testing Error Logging

Verify that errors are logged correctly with structured fields:

```go
func TestErrorLogging_ContainsStructuredDetails(t *testing.T) {
    var buf bytes.Buffer
    handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
    logger := logiface.New[*islog.Event](islog.WithSlogHandler(handler))

    // Log an error with context
    err := errors.New("connection timeout")
    event := logger.NewEvent(logiface.LevelError)
    event.AddMessage("database query failed")
    event.AddField("error", err.Error())
    event.AddField("query", "SELECT * FROM users")
    event.AddField("timeout_ms", 5000)
    logger.Write(logger.ReleaseEvent(event))

    entries := ParseJSONLogs(t, &buf)
    entry := entries[0]

    // Verify error details present
    AssertLogHasField(t, entry, "error", "connection timeout")
    AssertLogHasField(t, entry, "query", "SELECT * FROM users")
    AssertLogHasField(t, entry, "timeout_ms", 5000)
}
```

## 6. Testing Level Filtering

Verify that log filtering works correctly:

```go
func TestLevelFiltering_RespectsHandlerLevel(t *testing.T) {
    var buf bytes.Buffer

    // Configure handler to only allow INFO and above
    handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
        Level: slog.LevelInfo, // Filters out Debug/Trace
    })
    logger := logiface.New[*islog.Event](islog.WithSlogHandler(handler))

    // Try to log at different levels
    levels := []struct {
        level    logiface.Level
        shouldLog bool
    }{
        {logiface.LevelTrace, false},    // Filtered
        {logiface.LevelDebug, false},    // Filtered
        {logiface.LevelInformational, true},  // Logged
        {logiface.LevelNotice, true},  // Logged
        {logiface.LevelWarning, true},  // Logged
        {logiface.LevelError, true},   // Logged
        {logiface.LevelCritical, true}, // Logged
    }

    for _, tc := range levels {
        buf.Reset()
        event := logger.NewEvent(tc.level)
        event.AddMessage("test")
        logger.Write(logger.ReleaseEvent(event))

        entries := ParseJSONLogs(t, &buf)
        logged := len(entries) > 0

        if logged != tc.shouldLog {
            t.Errorf("Level %v: logged=%v, expected=%v", tc.level, logged, tc.shouldLog)
        }
    }
}
```

## 7. Integration Testing with Real Handlers

Test with production-like handlers (JSON, text):

```go
func TestWithRealHandlers_VerifyOutput(t *testing.T) {
    tests := []struct {
        name    string
        handler slog.Handler
    }{
        {
            name:    "JSONHandler",
            handler: slog.NewJSONHandler(&bytes.Buffer{}, nil),
        },
        {
            name:    "TextHandler",
            handler: slog.NewTextHandler(&bytes.Buffer{}, nil),
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            var buf bytes.Buffer
            handler := tc.handler

            // For handlers that already have buffering, wrap them
            if jsonHandler, ok := handler.(*slog.JSONHandler); ok {
                jsonHandler = handler
            } else if textHandler, ok := handler.(*slog.TextHandler); ok {
                textHandler = handler
            }

            // Note: In practice, you'd need to capture output differently
            // This is a simplified example showing handler compatibility

            logger := logiface.New[*islog.Event](islog.WithSlogHandler(handler))
            event := logger.NewEvent(logiface.LevelInformational)
            event.AddMessage("test message")
            event.AddField("key", "value")
            logger.Write(logger.ReleaseEvent(event))

            // Verify handler received event (implementation-specific)
            // In real tests, inspect the handler's output buffer
        })
    }
}
```

## 8. Logiface-Testsuite Integration

Leverage the 99,501 parameterized tests from logiface-testsuite:

```go
import (
    testsuite "github.com/joeycumines/logiface-testsuite"
)

var testSuiteConfig = testsuite.Config[*islog.Event]{
    LoggerFactory:    testSuiteLoggerFactory,
    WriteTimeout:     time.Second * 10,
    AlertCallsOsExit: true,
    EmergencyPanics:  true,
    LogsEmptyMessage: true,
}

func testSuiteLoggerFactory(req testsuite.LoggerRequest[*islog.Event]) testsuite.LoggerResponse[*islog.Event] {
    handler := slog.NewJSONHandler(req.Writer, &slog.HandlerOptions{
        Level:       slog.LevelDebug,
        ReplaceAttr: nil,
    })

    var options []logiface.Option[*islog.Event]
    options = append(options, islog.L.WithSlogHandler(handler))
    options = append(options, req.Options...)

    return testsuite.LoggerResponse[*islog.Event]{
        Logger:       islog.L.New(options...),
        LevelMapping: testSuiteLevelMapping,
        ParseEvent:   testSuiteParseEvent,
    }
}

func Test_TestSuite(t *testing.T) {
    testsuite.Run(t, testSuiteConfig)
}
```

This runs comprehensive tests covering:
- All field types (string, int, float64, bool, time, duration, errors, etc.)
- All log levels
- Nil/disabled logger behavior
- Event pooling effectiveness
- Context propagation
- And 99,493 more scenarios

## Best Practices

1. **Always Release Events**
   ```go
   event := logger.NewEvent(level)
   // ... use event ...
   logger.ReleaseEvent(event) // REQUIRED in tests too
   ```

2. **Reset Buffer Between Tests**
   ```go
   func (tl *TestLogger) ClearLogs() {
       tl.buf.Reset()
   }
   ```

3. **Use t.Helper() in Assertions**
   ```go
   func AssertLogField(t *testing.T, entry LogEntry, key, expected string) {
       t.Helper() // marks this as helper for better error traces
       // ... assertion code ...
   }
   ```

4. **Parse JSON Once Per Test**
   ```go
   entries := ParseJSONLogs(t, &buf) // Parse once
   for _, entry := range entries {
       // Multiple assertions on same parsed data
       AssertLogLevel(t, entry, "INFO")
       AssertLogMessage(t, entry, "msg")
   }
   ```

5. **Test Concurrency when Applicable**
   ```go
   go func() {
       logger.Info().Log("goroutine 1")
   }()
   go func() {
       logger.Info().Log("goroutine 2")
   }()
   // Verify both logs appear (no race conditions)
   ```

---

For additional examples, see [example_test.go](./example_test.go) and [simple_filter_test.go](./simple_filter_test.go).
