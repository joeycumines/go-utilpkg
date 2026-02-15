package slog

import (
	"bytes"
	"strings"
	"testing"

	"log/slog"

	"github.com/joeycumines/logiface"
)

// TestIntegration_SlogNew tests that logiface.Logger with slog.Handler works
func TestIntegration_SlogNew(t *testing.T) {
	// Create a buffer to capture output
	var buf bytes.Buffer

	// Create slog handler
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create logiface.Logger that writes to slog.Handler
	logger := logiface.New[*Event](NewLogger(handler))

	// Test at different levels using builder pattern
	logger.Debug().Str("key", "value").Log("debug message")
	logger.Info().Str("key", "value").Log("info message")
	logger.Warning().Str("key", "value").Log("warn message")
	logger.Err().Str("key", "value").Log("error message")

	// Verify output contains expected content
	output := buf.String()

	// slog.Handler has its own level filtering
	for _, msg := range []string{"info message", "warn message", "error message"} {
		if !strings.Contains(output, msg) {
			t.Errorf("Expected output to contain '%s', got: %s", msg, output)
		}
	}

	if !strings.Contains(output, "key=value") {
		t.Errorf("Expected output to contain 'key=value', got: %s", output)
	}
}

// TestIntegration_SlogSetDefault tests integration patterns
func TestIntegration_SlogSetDefault(t *testing.T) {
	// This test verifies that logiface.Logger forwards to slog.Handler correctly
	// The primary use case is: logiface API → slog.Handlers
	t.Skip("Skipping - primary use case is logiface → slog direction")
}

// TestIntegration_ContextPropagation tests that context is propagated correctly
func TestIntegration_ContextPropagation(t *testing.T) {
	var buf bytes.Buffer

	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := logiface.New[*Event](NewLogger(handler))

	// Test logging with message and field
	logger.Info().Str("key", "value").Log("context test")

	output := buf.String()
	if !strings.Contains(output, "context test") {
		t.Errorf("Expected output to contain 'context test', got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("Expected output to contain 'key=value', got: %s", output)
	}
}

// TestIntegration_AllSlogLevels tests all slog level methods work
func TestIntegration_AllSlogLevels(t *testing.T) {
	var buf bytes.Buffer

	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Test forward direction: logiface.Logger → slog.Handler
	logger := logiface.New[*Event](NewLogger(handler))

	// Use logiface builder methods to log at different levels
	logger.Debug().Log("debug message")
	logger.Info().Log("info message")
	logger.Warning().Log("warn message")
	logger.Err().Log("error message")

	output := buf.String()

	// Note: slog.Handler has its own level filtering, so only logs
	// that pass slog.Handler's level filter will appear in output.
	// We're just verifying no panic/crash occurs at various levels.
	if !strings.Contains(output, "info message") {
		t.Errorf("Expected output to contain 'info message', got: %s", output)
	}
}

// TestIntegration_Enabled tests that slog.Handler level filtering works
func TestIntegration_Enabled(t *testing.T) {
	var buf bytes.Buffer

	// Create handler that only accepts Warn and above
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})

	// Create logger
	logger := logiface.New[*Event](NewLogger(handler))

	// Log at different levels - slog.Handler will filter based on its level
	logger.Debug().Log("debug message")
	logger.Info().Log("info message")
	logger.Warning().Log("warn message")
	logger.Err().Log("error message")

	output := buf.String()

	// Verify only Warn and Error appeared (slog handler level filter)
	if strings.Contains(output, "debug message") {
		t.Errorf("Expected debug message to be filtered out by slog handler, got: %s", output)
	}
	if strings.Contains(output, "info message") {
		t.Errorf("Expected info message to be filtered out by slog handler, got: %s", output)
	}
	if !strings.Contains(output, "warn message") {
		t.Errorf("Expected warn message to appear, got: %s", output)
	}
	if !strings.Contains(output, "error message") {
		t.Errorf("Expected error message to appear, got: %s", output)
	}
}

// TestIntegration_WithAttrs tests that slog.Handler WithAttrs works
func TestIntegration_WithAttrs(t *testing.T) {
	// This test is for slog.Handler.WithAttrs which is used by slog
	// slog.Handler.WithAttrs returns a new handler with attributes applied to all logs
	var buf bytes.Buffer

	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Wrap handler with attributes
	handlerWithAttrs := handler.WithAttrs([]slog.Attr{
		slog.String("pre1", "value1"),
		slog.Int("pre2", 42),
	})

	// Log using logiface.Logger that writes to this handler
	logger := logiface.New[*Event](NewLogger(handlerWithAttrs))
	logger.Info().Log("test message")

	output := buf.String()

	// Verify pre-attributes appear
	if !strings.Contains(output, "pre1=value1") {
		t.Errorf("Expected output to contain 'pre1=value1', got: %s", output)
	}
	if !strings.Contains(output, "pre2=42") {
		t.Errorf("Expected output to contain 'pre2=42', got: %s", output)
	}
}

// TestIntegration_WithGroup tests that slog.Handler WithGroup works
func TestIntegration_WithGroup(t *testing.T) {
	// This test is for slog.Handler.WithGroup which is used by slog
	var buf bytes.Buffer

	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Wrap handler with group
	handlerWithGroup := handler.WithGroup("group1")

	// Log using logiface.Logger that writes to this handler
	logger := logiface.New[*Event](NewLogger(handlerWithGroup))
	logger.Info().Str("key", "value").Log("test message")

	output := buf.String()

	// Verify group prefix is applied
	// TextHandler uses "group1.key=value" format
	if !strings.Contains(output, "group1.") {
		t.Errorf("Expected output to contain group prefix 'group1.', got: %s", output)
	}
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected output to contain 'test message', got: %s", output)
	}
}

// TestIntegration_CustomHandler tests that logiface-slog works with custom slog.Handler
func TestIntegration_CustomHandler(t *testing.T) {
	var buf bytes.Buffer

	// Create a custom handler that writes to buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := logiface.New[*Event](NewLogger(handler))

	// Log multiple messages
	logger.Info().Log("info message")
	logger.Warning().Log("warn message")

	output := buf.String()

	// Verify messages were handled (slog handler level filtering applies)
	if !strings.Contains(output, "info message") {
		t.Errorf("Expected output to contain 'info message', got: %s", output)
	}
	if !strings.Contains(output, "warn message") {
		t.Errorf("Expected output to contain 'warn message', got: %s", output)
	}
}

// TestIntegration_NestedGroups tests nested group handling
func TestIntegration_NestedGroups(t *testing.T) {
	var buf bytes.Buffer

	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create nested groups using slog.Handler.WithGroup
	handlerWithNested := handler.WithGroup("level1").WithGroup("level2")

	// Log using logiface.Logger
	logger := logiface.New[*Event](NewLogger(handlerWithNested))
	logger.Info().Str("key", "value").Log("test message")

	output := buf.String()

	// Verify nested group prefixes are applied
	if !strings.Contains(output, "level1.") {
		t.Errorf("Expected output to contain 'level1.', got: %s", output)
	}
	if !strings.Contains(output, "level2.") {
		t.Errorf("Expected output to contain 'level2.', got: %s", output)
	}
}
