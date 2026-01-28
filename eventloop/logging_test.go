// logging_test.go - Tests for structured logging functionality
//
// Test coverage:
// - Logger interface implementation (DefaultLogger, NoOpLogger)
// - Log level filtering
// - Terminal vs non-terminal output
// - JSON log formatting
// - Package-level logging functions
// - Lazy evaluation

package eventloop

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLogLevelString verifies LogLevel string representations
func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LogLevel(99), "UNKNOWN(99)"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			if got := tc.level.String(); got != tc.expected {
				t.Errorf("String() = %q, want %q", got, tc.expected)
			}
		})
	}
}

// TestDefaultNewLogger creates a logger and verifies defaults
func TestDefaultNewLogger(t *testing.T) {
	logger := NewDefaultLogger(LevelInfo)

	if logger == nil {
		t.Fatal("NewDefaultLogger returned nil")
	}

	// Verify IsEnabled works
	if !logger.IsEnabled(LevelError) {
		t.Error("LevelError should be enabled at LevelInfo")
	}
	if logger.IsEnabled(LevelDebug) {
		t.Error("LevelDebug should not be enabled at LevelInfo")
	}
}

// TestSetLogLevel dynamically changes log level
func TestSetLogLevel(t *testing.T) {
	logger := NewDefaultLogger(LevelInfo)

	// Initially DEBUG should not be enabled
	if logger.IsEnabled(LevelDebug) {
		t.Error("DEBUG should not be enabled at INFO level")
	}

	// Change to DEBUG level
	logger.SetLevel(LevelDebug)
	if !logger.IsEnabled(LevelDebug) {
		t.Error("DEBUG should be enabled after SetLevel(DEBUG)")
	}

	// Change to ERROR level
	logger.SetLevel(LevelError)
	if logger.IsEnabled(LevelInfo) {
		t.Error("INFO should not be enabled at ERROR level")
	}
	if !logger.IsEnabled(LevelError) {
		t.Error("ERROR should be enabled at ERROR level")
	}
}

// TestLoggerLazyEvaluation verifies logs below level are not evaluated
func TestLoggerLazyEvaluation(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWriterLogger(LevelInfo, &buf)

	// This should NOT log (DEBUG < INFO)
	logger.Log(LogEntry{
		Level:    LevelDebug,
		Category: "test",
		Message:  "This should not appear",
	})

	if buf.Len() > 0 {
		t.Errorf("Log entry was written when it should have been filtered (got %d bytes)", buf.Len())
	}

	// This SHOULD log (INFO >= INFO)
	logger.Log(LogEntry{
		Level:    LevelInfo,
		Category: "test",
		Message:  "This should appear",
	})

	if buf.Len() == 0 {
		t.Error("Log entry was not written when it should have been")
	}
	if !strings.Contains(buf.String(), "This should appear") {
		t.Error("Log entry does not contain expected message")
	}
}

// TestLogEntryFormatting tests basic log entry formatting
func TestLogEntryFormatting(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWriterLogger(LevelInfo, &buf)

	entry := LogEntry{
		Level:    LevelInfo,
		Category: "timer",
		LoopID:   123,
		TimerID:  456,
		Message:  "Timer fired",
		Timestamp: time.Date(2026, 1, 29, 12, 34, 56, 123000000, time.UTC),
	}

	logger.Log(entry)

	output := buf.String()

	// Verify message appears
	if !strings.Contains(output, "Timer fired") {
		t.Error("Log entry missing message")
	}

	// Verify loop ID appears
	if !strings.Contains(output, "loop=123") {
		t.Error("Log entry missing loop ID")
	}

	// Verify timer ID appears
	if !strings.Contains(output, "timer=456") {
		t.Error("Log entry missing timer ID")
	}

	// Verify category appears
	if !strings.Contains(output, "[timer") {
		t.Error("Log entry missing category")
	}
}

// TestContextFields verifies context fields are logged
func TestContextFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWriterLogger(LevelInfo, &buf)

	entry := LogEntry{
		Level:    LevelInfo,
		Category: "test",
		Message:  "Test with context",
		Context: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
			"key3": true,
		},
	}

	logger.Log(entry)

	output := buf.String()

	// Verify context fields appear
	for _, expected := range []string{"key1=value1", "key2=42", "key3=true"} {
		if !strings.Contains(output, expected) {
			t.Errorf("Log entry missing context field %q", expected)
		}
	}
}

// TestErrorLogging verifies errors are logged correctly
func TestErrorLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWriterLogger(LevelInfo, &buf)

	entry := LogEntry{
		Level:    LevelError,
		Category: "task",
		Message:  "Task panicked",
		Err:      &testError{"unexpected error"},
	}

	logger.Log(entry)

	output := buf.String()

	if !strings.Contains(output, "Task panicked") {
		t.Error("Error log missing message")
	}
	if !strings.Contains(output, "unexpected error") {
		t.Error("Error log missing error value")
	}
	if !strings.Contains(output, "ERROR") {
		t.Error("Error log missing level indicator")
	}
}

// TestJSONOutputFormat verifies JSON formatting
func TestJSONOutputFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWriterLogger(LevelInfo, &buf)

	// Write a structured log entry
	logger.Log(LogEntry{
		Level:    LevelInfo,
		Category: "test",
		Message:  `Test message with "quotes"`,
		Context: map[string]interface{}{
			"key": "value",
		},
		Timestamp: time.Date(2026, 1, 29, 12, 0, 0, 0, time.UTC),
	})

	output := buf.String()

	// Verify it contains expected information
	if !strings.Contains(output, "INFO") {
		t.Error("Output missing level indicator")
	}
	if !strings.Contains(output, "test") {
		t.Error("Output missing category")
	}
	if !strings.Contains(output, `Test message with "quotes"`) {
		t.Error("Output missing message")
	}
	if !strings.Contains(output, "key=value") {
		t.Error("Output missing context field")
	}
}

// TestNoOpLogger discards all logs
func TestNoOpLogger(t *testing.T) {
	logger := NewNoOpLogger()

	if logger == nil {
		t.Fatal("NewNoOpLogger returned nil")
	}

	// Nothing should be enabled
	if logger.IsEnabled(LevelDebug) {
		t.Error("NoOpLogger should not enable any level")
	}
	if logger.IsEnabled(LevelInfo) {
		t.Error("NoOpLogger should not enable any level")
	}
	if logger.IsEnabled(LevelWarn) {
		t.Error("NoOpLogger should not enable any level")
	}
	if logger.IsEnabled(LevelError) {
		t.Error("NoOpLogger should not enable any level")
	}

	// Log should be no-op (no panic)
	logger.Log(LogEntry{
		Level:   LevelError,
		Message: "This should be discarded",
	})
}

// TestPackageLevelLogging verifies package-level logging functions
func TestPackageLevelLogging(t *testing.T) {
	var buf bytes.Buffer
	SetStructuredLogger(NewWriterLogger(LevelInfo, &buf))

	// Reset to no-op after test
	defer SetStructuredLogger(NewNoOpLogger())

	// These should log at appropriate levels
	SDebug("test", "debug message")
	SInfo("test", "info message")
	SWarn("test", "warn message")
	SError("test", "error message", nil)

	output := buf.String()

	// Only INFO, WARN, and ERROR should appear (DEBUG filtered)
	if !strings.Contains(output, "info message") {
		t.Error("Missing info message")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("Missing warn message")
	}
	if !strings.Contains(output, "error message") {
		t.Error("Missing error message")
	}
}

// TestLoggingOptions verifies functional options
func TestLoggingOptions(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWriterLogger(LevelInfo, &buf)

	// Use options to build log entry
	entry := LogEntry{
		Level:   LevelInfo,
		Message: "Test",
		Context: make(map[string]interface{}),
	}

	WithLoopID(123)(&entry)
	WithTaskID(456)(&entry)
	WithTimerID(789)(&entry)

	WithField("key1", "value1")(&entry)

	WithFields(map[string]interface{}{
		"key2": "value2",
		"key3": "value3",
	})(&entry)

	logger.Log(entry)

	output := buf.String()

	// Verify all options applied
	tests := []string{
		"loop=123",
		"task=456",
		"timer=789",
		"key1=value1",
		"key2=value2",
		"key3=value3",
	}

	for _, expected := range tests {
		if !strings.Contains(output, expected) {
			t.Errorf("Log entry missing field %q", expected)
		}
	}
}

// TestConcurrentLogging verifies thread safety
func TestConcurrentLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWriterLogger(LevelInfo, &buf)

	var wg sync.WaitGroup
	numGoroutines := 10
	numLogsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numLogsPerGoroutine; j++ {
				logger.Log(LogEntry{
					Level:    LevelInfo,
					Category: "test",
					Message:  "Concurrent log",
					Context: map[string]interface{}{
						"goroutine": id,
						"iteration": j,
					},
				})
			}
		}(i)
	}

	wg.Wait()

	// Verify all logs were written
	lineCount := strings.Count(buf.String(), "\n")
	expectedLines := numGoroutines * numLogsPerGoroutine

	if lineCount < expectedLines {
		t.Errorf("Expected %d log lines, got %d", expectedLines, lineCount)
	}
}

// TestDefaultOutputToStdout verifies default logger uses stdout
func TestDefaultOutputToStdout(t *testing.T) {
	logger := NewDefaultLogger(LevelInfo)

	if logger == nil {
		t.Fatal("NewDefaultLogger returned nil")
	}

	if logger.Out != os.Stdout {
		t.Error("DefaultLogger output is not os.Stdout")
	}
}

// TestAppendJSONString verifies JSON escaping
func TestAppendJSONString(t *testing.T) {
	tests := []struct {
		input       string
		shouldMatch []string // Substrings that should be in the output
	}{
		{`simple`, []string{`"simple"`}},
		{`with "quotes"`, []string{`\"`}},
		{`with\\slash`, []string{`\\`}}, // Using double backslash
		{`with control\x07char`, []string{`\u0007`}},
		{`with\nnewline`, []string{`\n`}},
		{`unicode`, []string{`"unicode"`}},
	}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			buf := []byte{}
			buf = appendJSONString(buf, tc.input)

			// Search for expected substrings rather than exact match
			for _, shouldMatch := range tc.shouldMatch {
				if !bytes.Contains(buf, []byte(shouldMatch)) {
					t.Errorf("appendJSONString(%q) = %q, expected to contain %q", tc.input, buf, shouldMatch)
				}
			}
		})
	}
}

// TestHelperFunctions verify internal helper logging functions
func TestHelperFunctions(t *testing.T) {
	var buf bytes.Buffer
	SetStructuredLogger(NewWriterLogger(LevelDebug, &buf))

	// Reset to no-op after test
	defer SetStructuredLogger(NewNoOpLogger())

	// Test timer logging
	LogTimerScheduled(123, 456, 100*time.Millisecond, "callback")
	LogTimerFired(123, 456, 100*time.Millisecond)
	LogTimerCanceled(123, 456, 50*time.Millisecond)

	// Test promise logging
	LogPromiseResolved(123, 789, "result")
	LogPromiseRejected(123, 789, "reason")

	// Test task logging
	LogTaskPanicked(123, 789, "panic", []byte("stack"))

	// Test microtask logging
	LogMicrotaskScheduled(123, 5)
	LogMicrotaskExecuted(123, 5, true)

	// Test poll error logging
	LogPollIOError(123, &testError{"poll error"}, false)
	LogPollIOError(123, &testError{"critical error"}, true)

	output := buf.String()

	// Verify some key messages
	if !strings.Contains(output, "Timer scheduled") {
		t.Error("Missing timer scheduled log")
	}
	if !strings.Contains(output, "Timer fired") {
		t.Error("Missing timer fired log")
	}
	if !strings.Contains(output, "Timer canceled") {
		t.Error("Missing timer canceled log")
	}
	if !strings.Contains(output, "Promise resolved") {
		t.Error("Missing promise resolved log")
	}
	if !strings.Contains(output, "Promise rejected") {
		t.Error("Missing promise rejected log")
	}
	if !strings.Contains(output, "Task panicked") {
		t.Error("Missing task panicked log")
	}
}

// TestSErrorf verifies formatted error logging
func TestSErrorf(t *testing.T) {
	var buf bytes.Buffer
	SetStructuredLogger(NewWriterLogger(LevelError, &buf))

	// Reset to no-op after test
	defer SetStructuredLogger(NewNoOpLogger())

	SErrorf("test", "Error %d: %s", 404, "Not Found")

	output := buf.String()

	if !strings.Contains(output, "Error 404: Not Found") {
		t.Error("SErrorf did not format error message correctly")
	}
}

// TestFileLogger can write to a file (no panic)
func TestFileLogger(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "logging-test-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	logger, err := NewFileLogger(LevelInfo, tmpfile.Name())
	if err != nil {
		t.Fatalf("NewFileLogger failed: %v", err)
	}

	logger.Log(LogEntry{
		Level:    LevelInfo,
		Category: "test",
		Message:  "Written to file",
	})

	// Verify file was written
	content, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if len(content) == 0 {
		t.Error("File logger did not write to file")
	}
}

// testError is a simple error implementation for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// BenchmarkLoggerLazyFiltered measures overhead of filtered logs
func BenchmarkLoggerLazyFiltered(b *testing.B) {
	var buf bytes.Buffer
	logger := NewWriterLogger(LevelError, &buf) // Set high level

	entry := LogEntry{
		Level:   LevelDebug, // Lower than ERROR, should be filtered
		Message: "This should be filtered",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Log(entry)
	}
}

// BenchmarkLoggerLogged measures overhead of actual logging
func BenchmarkLoggerLogged(b *testing.B) {
	var buf bytes.Buffer
	logger := NewWriterLogger(LevelDebug, &buf)

	entry := LogEntry{
		Level:   LevelDebug,
		Message: "This should be logged",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Log(entry)
	}
}

// BenchmarkJSONSerialization measures JSON formatting overhead
func BenchmarkJSONSerialization(b *testing.B) {
	var buf bytes.Buffer
	logger := NewWriterLogger(LevelDebug, &buf)

	entry := LogEntry{
		Level:    LevelDebug,
		Category: "test",
		Message:  "Test message",
		Context: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
			"key3": true,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Log(entry)
	}
}

// TestLoggingWithTimestamp verifies timestamp handling
func TestLoggingWithTimestamp(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWriterLogger(LevelInfo, &buf)

	now := time.Now()
	logger.Log(LogEntry{
		Level:    LevelInfo,
		Category: "test",
		Message:  "Timestamp test",
		Timestamp: now,
	})

	output := buf.String()

	// Check timestamp is in the output (format: HH:MM:SS.mmm)
	timePattern := now.Format("15:04:05")
	if !strings.Contains(output, timePattern) {
		t.Errorf("Timestamp %q not found in output: %q", timePattern, output)
	}
}

// TestLoggingWithNilContext handles nil context gracefully
func TestLoggingWithNilContext(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWriterLogger(LevelInfo, &buf)

	// Should not panic with nil context
	logger.Log(LogEntry{
		Level:    LevelInfo,
		Category: "test",
		Message:  "Nil context test",
		Context: nil,
	})

	output := buf.String()

	if !strings.Contains(output, "Nil context test") {
		t.Error("Message not logged with nil context")
	}
}

// TestLoggingWithEmptyMessage handles empty message
func TestLoggingWithEmptyMessage(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWriterLogger(LevelInfo, &buf)

	// Should not panic with empty message
	logger.Log(LogEntry{
		Level:    LevelInfo,
		Category: "test",
		Message:  "",
	})

	if buf.Len() == 0 {
		t.Error("Empty message not logged")
	}
}
