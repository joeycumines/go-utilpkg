package islog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"

	"github.com/joeycumines/logiface"
)

// ExampleLogger demonstrates basic usage pattern
func ExampleLogger() {
	// Create a slog handler (JSON in this example)
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create a logger using logiface-slog adapter
	logger := L.New(L.WithSlogHandler(handler))

	// Log an event with structured fields
	logger.Info().
		Str("service", "api").
		Str("method", "GET").
		Int("status", 200).
		Str("path", "/api/users").
		Log("Request completed")
}

// ExampleLogger_levelFiltering demonstrates level filtering configuration
func ExampleLogger_levelFiltering() {
	// Create handler configured for ERROR level only
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	})

	// Create logger with INFO level threshold (will be filtered by handler)
	logger := L.New(
		L.WithSlogHandler(handler),
		logiface.WithLevel[*Event](logiface.LevelInformational),
	)

	// This INFO message will NOT be logged (below handler's ERROR level)
	logger.Info().Str("info", "data").Log("Info message")

	// This ERROR message WILL be logged (at or above handler's ERROR level)
	logger.Err().Str("error", "details").Log("Error message")
}

// ExampleLogger_customHandler demonstrates custom slog handler usage
func ExampleLogger_customHandler() {
	// Create a custom handler that captures logs in memory
	var buf bytes.Buffer
	customHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Custom logic: redact sensitive keys
			if a.Key == "password" {
				return slog.Attr{}
			}
			return a
		},
		AddSource: true,
	})

	// Create logger with custom handler
	logger := L.New(L.WithSlogHandler(customHandler))

	// Log with sensitive data (will be redacted by ReplaceAttr)
	logger.Info().
		Str("username", "alice").
		Str("password", "secret123").
		Log("Login attempt")
}

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const requestIDKey contextKey = "requestID"

// ExampleLogger_contextPropagation demonstrates context usage
func ExampleLogger_contextPropagation() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := L.New(L.WithSlogHandler(handler))

	// Create context with request ID
	ctx := context.Background()
	ctx = context.WithValue(ctx, requestIDKey, "abc-123")

	// Log with additional context fields
	logger.Debug().
		Str("requestID", ctx.Value(requestIDKey).(string)).
		Str("action", "process").
		Log("Processing request")
}

// ExampleLogger_errorHandling demonstrates error field handling
func ExampleLogger_errorHandling() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := L.New(L.WithSlogHandler(handler))

	// Create and log an error
	err := &logEventError{code: 404, message: "Not found"}

	logger.Err().
		Str("error", err.Error()).
		Str("error_code", "404").
		Log("Operation failed")
}

// logEventError is an example error type for demonstration
type logEventError struct {
	code    int
	message string
}

func (e *logEventError) Error() string {
	return e.message
}

// ExampleLogger_poolReuse demonstrates event pool efficiency
func ExampleLogger_poolReuse() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := L.New(L.WithSlogHandler(handler))

	// Pool reuse is automatic: events are returned to pool after Write()
	for i := 0; i < 10; i++ {
		// Each call reuses events from pool after Write completes
		logger.Info().
			Int("iteration", i).
			Str("status", "active").
			Log("Processing loop")
	}

	// Note: Manual event creation with level builders is recommended
	event := logger.Debug().Str("manual", "creation")
	event.Log("Manual event construction")

	// Output will show 10 loop iterations + 1 manual event
}

// ExampleLogger_allLevels demonstrates logging at all log levels
func ExampleLogger_allLevels() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := L.New(
		L.WithSlogHandler(handler),
		logiface.WithLevel[*Event](logiface.LevelTrace),
	)

	logger.Trace().Str("level", "TRACE").Log("Trace message")
	logger.Debug().Str("level", "DEBUG").Log("Debug message")
	logger.Info().Str("level", "INFO").Log("Info message")
	logger.Warning().Str("level", "WARN").Log("Warning message")
	logger.Err().Str("level", "ERROR").Log("Error message")
	logger.Err().Str("level", "FATAL").Log("Fatal message")
}

// ExampleLogger_structuredErrorLogging demonstrates structured error logging with stack traces
func ExampleLogger_structuredErrorLogging() {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := L.New(L.WithSlogHandler(handler))

	// Wrap errors using errors.Is and errors.As patterns
	baseErr := &applicationError{
		code:    "AUTH-001",
		message: "Invalid credentials provided",
		details: map[string]any{
			"attempt_count": 3,
			"username":      "user@example.com",
			"ip_address":    "192.168.1.100",
			"blocked_until": "2026-02-18T12:00:00Z",
		},
	}

	// Create wrapped error chain
	wrappedErr := fmt.Errorf("login failed: %w", baseErr)

	// Log structured error with captured stack trace
	stack := captureStackTrace()
	logger.Err().
		Str("error", wrappedErr.Error()).
		Str("error_code", baseErr.code).
		Str("error_type", "AuthenticationError").
		Interface("stack_trace", stack).
		Interface("error_details", baseErr.details).
		Str("component", "auth-service").
		Str("operation", "validate_credentials").
		Log("Structured error with context")

	// Demonstrate panic/recover pattern
	func() {
		defer func() {
			if r := recover(); r != nil {
				panickedErr := fmt.Errorf("panic recovered: %v", r)
				panicStack := captureStackTrace()

				logger.Err().
					Str("panic", panickedErr.Error()).
					Interface("panic_stack", panicStack).
					Str("recovered_at", "operation_handler").
					Log("Panic caught and recovered in deferred handler")
			}
		}()

		// Simulate a panic condition
		panic("database connection lost")
	}()
}

// applicationError demonstrates a custom error type with structured fields
type applicationError struct {
	code    string
	message string
	details map[string]any
}

func (e *applicationError) Error() string {
	return e.message
}

// captureStackTrace captures the current goroutine's stack trace
func captureStackTrace() []string {
	const maxDepth = 32
	var pcs [maxDepth]uintptr
	n := runtime.Callers(2, pcs[:])

	var stack []string
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		fn := frame.Function
		if fn == "" {
			fn = "unknown"
		}
		// Format: filename:line (function)
		stack = append(stack, fmt.Sprintf("%s:%d (%s)", frame.File, frame.Line, fn))
		if !more {
			break
		}
	}
	return stack
}

// ExampleLogger_requestIDPropagation demonstrates request ID propagation pattern
func ExampleLogger_requestIDPropagation() {
	var buf bytes.Buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := L.New(L.WithSlogHandler(jsonHandler))

	// Create context with request ID
	type contextKey string
	const requestIDKey contextKey = "requestID"

	ctx := context.Background()
	ctx = context.WithValue(ctx, requestIDKey, "req-7f3a9c1d-4b2e-8f5a-3d1e")

	// Middleware-style: function that extracts context into fields
	extractRequestID := func(c context.Context) string {
		if val := c.Value(requestIDKey); val != nil {
			return val.(string)
		}
		return ""
	}

	// Log with request ID from context
	requestID := extractRequestID(ctx)
	logger.Info().
		Str("request_id", requestID).
		Str("method", "POST").
		Str("path", "/api/v1/users").
		Str("client_id", "client-12345").
		Log("Incoming request received")

	// Processing function that accepts context
	processRequest := func(c context.Context) error {
		requestID := extractRequestID(c)

		logger.Err().
			Str("request_id", requestID).
			Str("error", "validation failed").
			Str("field", "email").
			Str("value", "invalid-email").
			Log("Validation error")

		return nil
	}

	// Demonstrate processing with request context
	processRequest(ctx)

	// Demonstrate nested context propagation
	ctx2 := context.WithValue(ctx, requestIDKey, "req-7f3a9c1d-4b2e-8f5a-3d1e-CHILD-001")
	processRequest(ctx2)
}

// ExampleLogger_perRequestConfiguration demonstrates per-request logger configuration
func ExampleLogger_perRequestConfiguration() {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	baseLogger := L.New(L.WithSlogHandler(handler))

	// Request-specific fields (these should be included in every log for this request)
	requestID := "req-abc-123"
	userID := "user-456"
	sessionID := "sess-789"

	// Log with per-request context - include request fields in every log
	baseLogger.Info().
		Str("request_id", requestID).
		Str("user_id", userID).
		Str("session_id", sessionID).
		Str("action", "query").
		Str("resource", "products").
		Log("Query operation initiated")

	// Child operation with additional fields
	operationID := "op-xyz-987"
	baseLogger.Info().
		Str("request_id", requestID).
		Str("user_id", userID).
		Str("session_id", sessionID).
		Str("operation_id", operationID).
		Str("step", "validate").
		Bool("valid", true).
		Log("Validation step completed")

	// Another operation with different operation ID but same request fields
	operationID2 := "qrs-555"
	baseLogger.Err().
		Str("request_id", requestID).
		Str("user_id", userID).
		Str("session_id", sessionID).
		Str("operation_id", operationID2).
		Str("error", "query timeout").
		Int("timeout_ms", 5000).
		Log("Operation failed")

	// Final status with original request fields
	baseLogger.Info().
		Str("request_id", requestID).
		Str("user_id", userID).
		Str("session_id", sessionID).
		Str("action", "cleanup").
		Str("status", "completed").
		Log("Request operation completed")
}

// ExampleLogger_testingMockingPattern demonstrates testing logger mocking pattern
func ExampleLogger_testingMockingPattern() {
	// This example shows how to use logiface-testsuite for testing

	// Test function that logs
	testableFunction := func(l *logiface.Logger[*Event]) error {
		l.Info().
			Str("component", "example").
			Str("operation", "test").
			Log("Executing test operation")

		if true { // Simulate error condition
			return errors.New("simulated error")
		}
		return nil
	}

	// In a real test, you would use testing.T and logiface-testsuite
	// For this example, we'll capture logs in a buffer
	var buf bytes.Buffer
	testHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	testLogger := L.New(L.WithSlogHandler(testHandler))

	// Execute testable function
	err := testableFunction(testLogger)

	// Capture and verify log output
	loggedOutput := buf.String()
	testLogger.Err().
		Str("error", err.Error()).
		Str("captured_logs", strings.TrimSpace(loggedOutput)).
		Log("Test execution verification")
}
