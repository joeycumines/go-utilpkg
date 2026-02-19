package islog

import (
	"bytes"
	"log/slog"
	"os"

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
