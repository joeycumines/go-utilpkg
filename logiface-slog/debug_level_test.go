package islog

import (
	"bytes"
	"log/slog"
	"testing"
)

// TestSimpleLevelBehavior is a diagnostic test to understand level filtering
func TestSimpleLevelBehavior(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	// Create handler with Debug level (allows all levels)
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create logger with this handler
	logger := L.New(L.WithSlogHandler(handler))

	t.Log("=== Testing Debug level filtering ===")

	t.Run("Debug log", func(t *testing.T) {
		buf.Reset()
		t.Logf("Buffer before: %q", buf.String())

		logger.Debug().Log("Debug message")

		output := buf.String()
		t.Logf("Buffer after: %q", output)
		t.Logf("Output length: %d", len(output))

		if output == "" {
			t.Log("DEBUG: Debug was FILTERED (empty output)")
		} else {
			t.Log("DEBUG: Debug was NOT filtered (has output)")
		}
	})

	t.Run("Info log", func(t *testing.T) {
		buf.Reset()
		t.Logf("Buffer before: %q", buf.String())

		logger.Info().Log("Info message")

		output := buf.String()
		t.Logf("Buffer after: %q", output)
		t.Logf("Output length: %d", len(output))

		if output == "" {
			t.Log("DEBUG: Info was FILTERED (empty output)")
		} else {
			t.Log("DEBUG: Info was NOT filtered (has output)")
		}
	})

	t.Run("Warn log", func(t *testing.T) {
		buf.Reset()
		t.Logf("Buffer before: %q", buf.String())

		logger.Warning().Log("Warning message")

		output := buf.String()
		t.Logf("Buffer after: %q", output)
		t.Logf("Output length: %d", len(output))

		if output == "" {
			t.Log("DEBUG: Warning was FILTERED (empty output)")
		} else {
			t.Log("DEBUG: Warning was NOT filtered (has output)")
		}
	})

	t.Run("Error log", func(t *testing.T) {
		buf.Reset()
		t.Logf("Buffer before: %q", buf.String())

		logger.Err().Log("Error message")

		output := buf.String()
		t.Logf("Buffer after: %q", output)
		t.Logf("Output length: %d", len(output))

		if output == "" {
			t.Log("DEBUG: Error was FILTERED (empty output)")
		} else {
			t.Log("DEBUG: Error was NOT filtered (has output)")
		}
	})

	t.Run("Trace log", func(t *testing.T) {
		buf.Reset()
		t.Logf("Buffer before: %q", buf.String())

		logger.Trace().Log("Trace message")

		output := buf.String()
		t.Logf("Buffer after: %q", output)
		t.Logf("Output length: %d", len(output))

		if output == "" {
			t.Log("DEBUG: Trace was FILTERED (empty output)")
		} else {
			t.Log("DEBUG: Trace was NOT filtered (has output)")
		}
	})
}
