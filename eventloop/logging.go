// logging.go - Structured Logging Interface for Eventloop Module
//
// Package-level configuration for structured logging.
// This design allows external integration with logging frameworks like zerolog, logrus, etc.
// while providing a low-overhead built-in implementation for basic usage.
//
// Usage:
//   // Enable structured logging at package initialization
//   eventloop.SetStructuredLogger(eventloop.NewDefaultLogger(eventloop.LevelInfo))
//
// Design Decision: Package-level global variable is appropriate here because:
//   - Logging is an infrastructure cross-cutting concern
//   - Event loop instances share logging semantics
//   - Zero-allocation configuration at startup
//   - Avoids per-instance logging configuration surface area bloat

package eventloop

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

var (
	// Global structured logger for package-level logging functions
	globalLogger struct {
		sync.RWMutex
		logger Logger
	}
)

// SetStructuredLogger sets the global structured logger
func SetStructuredLogger(logger Logger) {
	globalLogger.Lock()
	defer globalLogger.Unlock()
	globalLogger.logger = logger
}

// getGlobalLogger safely retrieves the global logger
func getGlobalLogger() Logger {
	globalLogger.RLock()
	defer globalLogger.RUnlock()
	if globalLogger.logger != nil {
		return globalLogger.logger
	}
	return NewNoOpLogger()
}

// LogLevel represents the severity of a log message
type LogLevel int32

const (
	// LevelDebug for detailed diagnostic information
	LevelDebug LogLevel = iota

	// LevelInfo for general informational messages
	LevelInfo

	// LevelWarn for warning conditions
	LevelWarn

	// LevelError for error conditions
	LevelError
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", l)
	}
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Level     LogLevel
	Category  string // "timer", "promise", "microtask", "poll", "shutdown"
	LoopID    int64
	TaskID    int64
	TimerID   int64
	Context   map[string]interface{}
	Message   string
	Err       error
	Timestamp time.Time
	// Stack info for debugging
	// File        string
	// Line        int
	// Goroutine   int
}

// Logger is the structured logging interface
type Logger interface {
	Log(entry LogEntry)
	IsEnabled(level LogLevel) bool
}

// DefaultLogger implements Logger using os.Stdout
type DefaultLogger struct {
	level atomic.Int32
	mu    sync.Mutex
	Out   *os.File // Public field for testing
}

// NewDefaultLogger creates a logger with specified minimum level
func NewDefaultLogger(level LogLevel) *DefaultLogger {
	l := &DefaultLogger{
		Out: os.Stdout,
	}
	l.level.Store(int32(level))
	return l
}

// NewFileLogger creates a logger writing to specified file
func NewFileLogger(level LogLevel, filename string) (*DefaultLogger, error) {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	l := &DefaultLogger{
		Out: file,
	}
	l.level.Store(int32(level))
	return l, nil
}

// SetLevel dynamically changes the minimum log level
func (l *DefaultLogger) SetLevel(level LogLevel) {
	l.level.Store(int32(level))
}

// getLevel gets the current log level
func (l *DefaultLogger) getLevel() int32 {
	return l.level.Load()
}

// IsEnabled checks if the specified level would be logged
func (l *DefaultLogger) IsEnabled(level LogLevel) bool {
	return level >= LogLevel(l.getLevel())
}

// Log writes a structured log entry
func (l *DefaultLogger) Log(entry LogEntry) {
	// Lazy evaluation - check level before allocating/formatting
	if !l.IsEnabled(entry.Level) {
		return
	}

	// Set timestamp if not set
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Marshal to JSON (or format as text if in terminal)
	l.mu.Lock()
	defer l.mu.Unlock()

	// Determine if output is a terminal for pretty printing
	isTerminal := isTerminal(l.Out)

	if isTerminal {
		// Pretty-print for terminal
		l.logPretty(entry)
	} else {
		// JSON for file/log aggregation
		l.logJSON(entry)
	}
}

// logPretty formats log entry for terminal output
func (l *DefaultLogger) logPretty(entry LogEntry) {
	// Color codes
	colorReset := "\033[0m"
	colorFatal := "\033[31m" // Red
	colorError := "\033[31m"
	colorWarn := "\033[33m"  // Yellow
	colorInfo := "\033[36m"  // Cyan
	colorDebug := "\033[90m" // Dark gray
	colorDim := "\033[2m"    // Dim

	var color string
	switch entry.Level {
	case LevelDebug:
		color = colorDebug
	case LevelInfo:
		color = colorInfo
	case LevelWarn:
		color = colorWarn
	case LevelError:
		color = colorError
	}

	// Format: [LEVEL] [category] message [context fields]
	fmt.Fprintf(l.Out, "%s%s%s %s [%-10s] %s%s",
		color, entry.Level.String(), colorReset,
		entry.Timestamp.Format("15:04:05.000"),
		entry.Category,
		entry.Message,
		colorReset,
	)

	// Print struct fields if present
	if len(entry.Context) > 0 || entry.LoopID != 0 || entry.TaskID != 0 || entry.TimerID != 0 {
		fmt.Fprint(l.Out, colorDim)
		if entry.LoopID != 0 {
			fmt.Fprintf(l.Out, " loop=%d", entry.LoopID)
		}
		if entry.TaskID != 0 {
			fmt.Fprintf(l.Out, " task=%d", entry.TaskID)
		}
		if entry.TimerID != 0 {
			fmt.Fprintf(l.Out, " timer=%d", entry.TimerID)
		}
		for k, v := range entry.Context {
			fmt.Fprintf(l.Out, " %s=%v", k, v)
		}
		fmt.Fprint(l.Out, colorReset)
	}

	// Print error if present
	if entry.Err != nil {
		fmt.Fprintf(l.Out, " %s%v%s\n", colorFatal, entry.Err, colorReset)
	} else {
		fmt.Fprintln(l.Out)
	}
}

// logJSON formats log entry as JSON (for log aggregation)
func (l *DefaultLogger) logJSON(entry LogEntry) {
	fmt.Fprintf(l.Out, "{\"timestamp\":\"%s\",\"level\":%s,\"category\":\"%s\"",
		entry.Timestamp.Format(time.RFC3339Nano),
		entry.Level,
		entry.Category,
	)

	jsonFields := make([]byte, 0, 256)
	jsonFields = append(jsonFields, ',')
	if entry.LoopID != 0 {
		jsonFields = append(jsonFields, fmt.Sprintf("\"loop\":%d", entry.LoopID)...)
	}
	if entry.TaskID != 0 {
		jsonFields = append(jsonFields, fmt.Sprintf("\"task\":%d", entry.TaskID)...)
	}
	if entry.TimerID != 0 {
		jsonFields = append(jsonFields, fmt.Sprintf("\"timer\":%d", entry.TimerID)...)
	}
	for k, v := range entry.Context {
		jsonFields = append(jsonFields, fmt.Sprintf("\"%s\":%v", k, v)...)
	}

	// Escape message and append
	message := escapeJSON(entry.Message)
	fmt.Fprintf(l.Out, ",\"message\":\"%s\"%s}", message, jsonFields)

	if entry.Err != nil {
		fmt.Fprintf(l.Out, ",\"error\":\"%s\"}\n", escapeJSON(entry.Err.Error()))
	} else {
		fmt.Fprintln(l.Out, "}")
	}
}

// escapeJSON escapes special JSON characters
func escapeJSON(s string) string {
	b := make([]byte, 0, len(s)*6) // Each char could expand to \uXXXX
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\', '"', '/', '\b', '\f', '\n', '\r', '\t':
			b = append(b, '\\', c)
		default:
			if c < ' ' {
				// JSON escape for control characters: \u00XX
				b = append(b, '\\', 'u', '0', '0', byte(c>>4)+'0', byte(c&0xF)+'0')
			} else {
				b = append(b, c)
			}
		}
	}
	return *(*string)(unsafe.Pointer(&b))
}

// isTerminal checks if writer is a terminal
func isTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		stat, err := f.Stat()
		if err != nil {
			return false
		}
		return (stat.Mode() & os.ModeCharDevice) != 0
	}
	return false
}

// LogEntryBuilder provides fluent API for building log entries
type LogEntryBuilder struct {
	entry LogEntry
}

// NewLogEntry creates a new log entry builder
func NewLogEntry(level LogLevel, category string, message string) LogEntryBuilder {
	return LogEntryBuilder{
		entry: LogEntry{
			Level:     level,
			Category:  category,
			Message:   message,
			Context:   make(map[string]interface{}),
			Timestamp: time.Now(),
		},
	}
}

// LoopID sets the loop ID for this log entry
func (b LogEntryBuilder) LoopID(id int64) LogEntryBuilder {
	b.entry.LoopID = id
	return b
}

// TaskID sets the task ID for this log entry
func (b LogEntryBuilder) TaskID(id int64) LogEntryBuilder {
	b.entry.TaskID = id
	return b
}

// TimerID sets the timer ID for this log entry
func (b LogEntryBuilder) TimerID(id int64) LogEntryBuilder {
	b.entry.TimerID = id
	return b
}

// Field adds a key-value pair to the context
func (b LogEntryBuilder) Field(key string, value interface{}) LogEntryBuilder {
	b.entry.Context[key] = value
	return b
}

// Fields adds multiple key-value pairs
func (b LogEntryBuilder) Fields(fields map[string]interface{}) LogEntryBuilder {
	for k, v := range fields {
		b.entry.Context[k] = v
	}
	return b
}

// Err sets the error for this log entry
func (b LogEntryBuilder) Err(err error) LogEntryBuilder {
	b.entry.Err = err
	return b
}

// Build constructs the final log entry
func (b LogEntryBuilder) Build() LogEntry {
	return b.entry
}

// ContextFields extracts log fields from context if present
func ContextFields(ctx context.Context) map[string]interface{} {
	fields := make(map[string]interface{})
	fields["correlationID"] = getCorrelationID(ctx)
	fields["traceID"] = getTraceID(ctx)

	// Extract any other log fields from context
	if ctx != nil {
		if requestID, ok := ctx.Value("requestID").(string); ok {
			fields["requestID"] = requestID
		}
		if userID, ok := ctx.Value("userID").(string); ok {
			fields["userID"] = userID
		}
	}

	return fields
}

// getCorrelationID extracts correlation ID from context
func getCorrelationID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if reqID, ok := ctx.Value("correlationID").(string); ok {
		return reqID
	}
	// Generate correlation ID from span context if using tracing
	// This is a placeholder for OpenTelemetry/Jaeger integration
	if spanID, ok := ctx.Value("spanID").(string); ok {
		return spanID
	}
	// Generate a random correlation ID for distributed tracing
	// In production, this would use a UUID such as uuid.NewV4()
	return ""
}

// getTraceID extracts trace ID from context
func getTraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if traceID, ok := ctx.Value("traceID").(string); ok {
		return traceID
	}
	return ""
}

// WithCorrelationID creates a context with correlation ID
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, "correlationID", correlationID)
}

// WithTraceID creates a context with trace ID
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, "traceID", traceID)
}

// No-op logger for when logging is disabled
type NoOpLogger struct{}

func NewNoOpLogger() *NoOpLogger {
	return &NoOpLogger{}
}

func (l *NoOpLogger) Log(entry LogEntry) {
	// No-op
}

func (l *NoOpLogger) IsEnabled(level LogLevel) bool {
	return false
}

// WriterLogger implements Logger using any io.Writer
type WriterLogger struct {
	level atomic.Int32
	mu    sync.Mutex
	out   io.Writer
}

// NewWriterLogger creates a logger writing to any io.Writer
func NewWriterLogger(level LogLevel, out io.Writer) *WriterLogger {
	l := &WriterLogger{
		out: out,
	}
	l.level.Store(int32(level))
	return l
}

// SetLevel dynamically changes the minimum log level
func (l *WriterLogger) SetLevel(level LogLevel) {
	l.level.Store(int32(level))
}

// IsEnabled checks if the specified level would be logged
func (l *WriterLogger) IsEnabled(level LogLevel) bool {
	return level >= LogLevel(l.level.Load())
}

// Log writes a structured log entry
func (l *WriterLogger) Log(entry LogEntry) {
	// Lazy evaluation - check level before allocating/formatting
	if !l.IsEnabled(entry.Level) {
		return
	}

	// Set timestamp if not set
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Format as text (simpler for WriterLogger, suitable for testing)
	l.mu.Lock()
	defer l.mu.Unlock()

	l.logText(entry)
}

// logText formats log entry as plain text
func (l *WriterLogger) logText(entry LogEntry) {
	// Format: [LEVEL] [timestamp] [category] message [fields]
	fmt.Fprintf(l.out, "[%s] [%s] [%-10s] %s",
		entry.Level.String(),
		entry.Timestamp.Format("15:04:05.000"),
		entry.Category,
		entry.Message,
	)

	// Print struct fields if present
	if len(entry.Context) > 0 || entry.LoopID != 0 || entry.TaskID != 0 || entry.TimerID != 0 {
		if entry.LoopID != 0 {
			fmt.Fprintf(l.out, " loop=%d", entry.LoopID)
		}
		if entry.TaskID != 0 {
			fmt.Fprintf(l.out, " task=%d", entry.TaskID)
		}
		if entry.TimerID != 0 {
			fmt.Fprintf(l.out, " timer=%d", entry.TimerID)
		}
		for k, v := range entry.Context {
			fmt.Fprintf(l.out, " %s=%v", k, v)
		}
	}

	// Print error if present
	if entry.Err != nil {
		fmt.Fprintf(l.out, " err=%v\n", entry.Err)
	} else {
		fmt.Fprintln(l.out)
	}
}

// Helper functions for common logging patterns

// LogDebug logs a debug message using the event loop's logger
func LogDebug(l Logger, category, message string, fields map[string]interface{}) {
	if !l.IsEnabled(LevelDebug) {
		return // Lazy evaluation
	}
	entry := LogEntry{
		Level:     LevelDebug,
		Category:  category,
		Message:   message,
		Context:   fields,
		Timestamp: time.Now(),
	}
	l.Log(entry)
}

// LogInfo logs an info message using the event loop's logger
func LogInfo(l Logger, category, message string, fields map[string]interface{}) {
	if !l.IsEnabled(LevelInfo) {
		return // Lazy evaluation
	}
	entry := LogEntry{
		Level:     LevelInfo,
		Category:  category,
		Message:   message,
		Context:   fields,
		Timestamp: time.Now(),
	}
	l.Log(entry)
}

// LogWarn logs a warning message using the event loop's logger
func LogWarn(l Logger, category, message string, fields map[string]interface{}) {
	if !l.IsEnabled(LevelWarn) {
		return // Lazy evaluation
	}
	entry := LogEntry{
		Level:     LevelWarn,
		Category:  category,
		Message:   message,
		Context:   fields,
		Timestamp: time.Now(),
	}
	l.Log(entry)
}

// LogError logs an error message using the event loop's logger
func LogError(l Logger, category, message string, err error, fields map[string]interface{}) {
	if !l.IsEnabled(LevelError) {
		return // Lazy evaluation
	}
	entry := LogEntry{
		Level:     LevelError,
		Category:  category,
		Message:   message,
		Err:       err,
		Context:   fields,
		Timestamp: time.Now(),
	}
	l.Log(entry)
}

// LogErrorf logs a formatted error message
func LogErrorf(l Logger, category, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	LogError(l, category, message, nil, nil)
}

// Package-level structured logging convenience functions
// These use the global logger and are designed for quick usage

// SDebug logs a debug message using the global logger
func SDebug(category, message string, fields ...map[string]interface{}) {
	logger := getGlobalLogger()
	if !logger.IsEnabled(LevelDebug) {
		return
	}
	var f map[string]interface{}
	if len(fields) > 0 && fields[0] != nil {
		f = fields[0]
	}
	LogDebug(logger, category, message, f)
}

// SInfo logs an info message using the global logger
func SInfo(category, message string, fields ...map[string]interface{}) {
	logger := getGlobalLogger()
	if !logger.IsEnabled(LevelInfo) {
		return
	}
	var f map[string]interface{}
	if len(fields) > 0 && fields[0] != nil {
		f = fields[0]
	}
	LogInfo(logger, category, message, f)
}

// SWarn logs a warning message using the global logger
func SWarn(category, message string, fields ...map[string]interface{}) {
	logger := getGlobalLogger()
	if !logger.IsEnabled(LevelWarn) {
		return
	}
	var f map[string]interface{}
	if len(fields) > 0 && fields[0] != nil {
		f = fields[0]
	}
	LogWarn(logger, category, message, f)
}

// SError logs an error message using the global logger
func SError(category, message string, err error, fields ...map[string]interface{}) {
	logger := getGlobalLogger()
	if !logger.IsEnabled(LevelError) {
		return
	}
	var f map[string]interface{}
	if len(fields) > 0 && fields[0] != nil {
		f = fields[0]
	}
	LogError(logger, category, message, err, f)
}

// SErrorf logs a formatted error message using the global logger
func SErrorf(category, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	SError(category, message, nil)
}

// Functional options for Log Entry construction

// LogEntryOption is a function that modifies a log entry
type LogEntryOption func(*LogEntry)

// WithLoopID sets the loop ID for a log entry (functional option)
func WithLoopID(id int64) LogEntryOption {
	return func(e *LogEntry) {
		e.LoopID = id
	}
}

// WithTaskID sets the task ID for a log entry (functional option)
func WithTaskID(id int64) LogEntryOption {
	return func(e *LogEntry) {
		e.TaskID = id
	}
}

// WithTimerID sets the timer ID for a log entry (functional option)
func WithTimerID(id int64) LogEntryOption {
	return func(e *LogEntry) {
		e.TimerID = id
	}
}

// WithField sets a key-value pair in the context (functional option)
func WithField(key string, value interface{}) LogEntryOption {
	return func(e *LogEntry) {
		if e.Context == nil {
			e.Context = make(map[string]interface{})
		}
		e.Context[key] = value
	}
}

// WithFields sets multiple key-value pairs in the context (functional option)
func WithFields(fields map[string]interface{}) LogEntryOption {
	return func(e *LogEntry) {
		if e.Context == nil {
			e.Context = make(map[string]interface{})
		}
		for k, v := range fields {
			e.Context[k] = v
		}
	}
}

// Specialty helper functions for common event loop operations

// LogTimerScheduled logs when a timer is scheduled
func LogTimerScheduled(loopID, timerID int64, duration time.Duration, description string) {
	logger := getGlobalLogger()
	if !logger.IsEnabled(LevelDebug) {
		return
	}
	logger.Log(LogEntry{
		Level:     LevelDebug,
		Category:  "timer",
		LoopID:    loopID,
		TimerID:   timerID,
		Message:   "Timer scheduled",
		Timestamp: time.Now(),
		Context: map[string]interface{}{
			"duration_ms": duration.Milliseconds(),
			"description": description,
		},
	})
}

// LogTimerFired logs when a timer fires
func LogTimerFired(loopID, timerID int64, duration time.Duration) {
	logger := getGlobalLogger()
	if !logger.IsEnabled(LevelDebug) {
		return
	}
	logger.Log(LogEntry{
		Level:     LevelDebug,
		Category:  "timer",
		LoopID:    loopID,
		TimerID:   timerID,
		Message:   "Timer fired",
		Timestamp: time.Now(),
		Context: map[string]interface{}{
			"duration_ms": duration.Milliseconds(),
		},
	})
}

// LogTimerCanceled logs when a timer is canceled
func LogTimerCanceled(loopID, timerID int64, elapsed time.Duration) {
	logger := getGlobalLogger()
	if !logger.IsEnabled(LevelDebug) {
		return
	}
	logger.Log(LogEntry{
		Level:     LevelDebug,
		Category:  "timer",
		LoopID:    loopID,
		TimerID:   timerID,
		Message:   "Timer canceled",
		Timestamp: time.Now(),
		Context: map[string]interface{}{
			"elapsed_ms": elapsed.Milliseconds(),
		},
	})
}

// LogPromiseResolved logs when a promise is resolved
func LogPromiseResolved(loopID, taskID int64, result interface{}) {
	logger := getGlobalLogger()
	if !logger.IsEnabled(LevelDebug) {
		return
	}
	logger.Log(LogEntry{
		Level:     LevelDebug,
		Category:  "promise",
		LoopID:    loopID,
		TaskID:    taskID,
		Message:   "Promise resolved",
		Timestamp: time.Now(),
		Context: map[string]interface{}{
			"result": result,
		},
	})
}

// LogPromiseRejected logs when a promise is rejected
func LogPromiseRejected(loopID, taskID int64, reason interface{}) {
	logger := getGlobalLogger()
	if !logger.IsEnabled(LevelDebug) {
		return
	}
	logger.Log(LogEntry{
		Level:     LevelDebug,
		Category:  "promise",
		LoopID:    loopID,
		TaskID:    taskID,
		Message:   "Promise rejected",
		Timestamp: time.Now(),
		Context: map[string]interface{}{
			"reason": reason,
		},
	})
}

// LogTaskPanicked logs when a task panics
func LogTaskPanicked(loopID, taskID int64, panicMsg interface{}, stack []byte) {
	logger := getGlobalLogger()
	if !logger.IsEnabled(LevelError) {
		return
	}
	logger.Log(LogEntry{
		Level:     LevelError,
		Category:  "task",
		LoopID:    loopID,
		TaskID:    taskID,
		Message:   "Task panicked",
		Timestamp: time.Now(),
		Context: map[string]interface{}{
			"panic": panicMsg,
			"stack": string(stack),
		},
	})
}

// LogMicrotaskScheduled logs when a microtask is scheduled
func LogMicrotaskScheduled(loopID int64, count int) {
	logger := getGlobalLogger()
	if !logger.IsEnabled(LevelDebug) {
		return
	}
	logger.Log(LogEntry{
		Level:     LevelDebug,
		Category:  "microtask",
		LoopID:    loopID,
		Message:   "Microtask scheduled",
		Timestamp: time.Now(),
		Context: map[string]interface{}{
			"queue_size": count,
		},
	})
}

// LogMicrotaskExecuted logs when a microtask is executed
func LogMicrotaskExecuted(loopID int64, remaining int, last bool) {
	logger := getGlobalLogger()
	if !logger.IsEnabled(LevelDebug) {
		return
	}
	logger.Log(LogEntry{
		Level:     LevelDebug,
		Category:  "microtask",
		LoopID:    loopID,
		Message:   "Microtask executed",
		Timestamp: time.Now(),
		Context: map[string]interface{}{
			"remaining": remaining,
			"last":      last,
		},
	})
}

// LogPollIOError logs poll I/O errors
func LogPollIOError(loopID int64, err error, critical bool) {
	logger := getGlobalLogger()
	level := LevelWarn
	if critical {
		level = LevelError
	}
	if !logger.IsEnabled(level) {
		return
	}
	logger.Log(LogEntry{
		Level:     level,
		Category:  "poll",
		LoopID:    loopID,
		Message:   "Poll error",
		Err:       err,
		Timestamp: time.Now(),
		Context: map[string]interface{}{
			"critical": critical,
		},
	})
}

// hexByte converts a nibble (0-15) to its hex character ('0'-'9' or 'A'-'F')
func hexByte(b byte) byte {
	if b < 10 {
		return '0' + b
	}
	return 'A' + b - 10
}

// appendJSONString appends a JSON-escaped string to a byte slice
func appendJSONString(buf []byte, s string) []byte {
	buf = append(buf, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\', '"', '/':
			buf = append(buf, '\\', c)
		case '\b':
			buf = append(buf, '\\', 'b')
		case '\f':
			buf = append(buf, '\\', 'f')
		case '\n':
			buf = append(buf, '\\', 'n')
		case '\r':
			buf = append(buf, '\\', 'r')
		case '\t':
			buf = append(buf, '\\', 't')
		default:
			if c < ' ' {
				// JSON escape for control characters: \u00XX
				buf = append(buf, '\\', 'u', '0', '0')
				high := c >> 4
				low := c & 0xF
				buf = append(buf, hexByte(high), hexByte(low))
			} else {
				buf = append(buf, c)
			}
		}
	}
	return append(buf, '"')
}
