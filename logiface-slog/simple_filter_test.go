package islog

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestLoggerLevelFiltering(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := L.New(L.WithSlogHandler(handler))

	t.Run("Debug should be filtered by default", func(t *testing.T) {
		buf.Reset()
		logger.Debug().Log("Debug message")
		if s := buf.String(); s != "" {
			t.Errorf("Debug should be filtered (empty output), got: %q", s)
		}
	})

	t.Run("Trace should be filtered by default", func(t *testing.T) {
		buf.Reset()
		logger.Trace().Log("Trace message")
		if s := buf.String(); s != "" {
			t.Errorf("Trace should be filtered (empty output), got: %q", s)
		}
	})

	t.Run("Info should appear", func(t *testing.T) {
		buf.Reset()
		logger.Info().Log("Info message")
		if s := buf.String(); s == "" {
			t.Errorf("Info should produce output, got empty string")
		}
		if s := buf.String(); len(s) < 10 {
			t.Errorf("Info should produce meaningful output, got: %q", s)
		}
	})

	t.Run("Warning should appear", func(t *testing.T) {
		buf.Reset()
		logger.Warning().Log("Warning message")
		if s := buf.String(); s == "" {
			t.Errorf("Warning should produce output, got empty string")
		}
		if s := buf.String(); len(s) < 10 {
			t.Errorf("Warning should produce meaningful output, got: %q", s)
		}
	})

	t.Run("Error should appear", func(t *testing.T) {
		buf.Reset()
		logger.Err().Log("Error message")
		if s := buf.String(); s == "" {
			t.Errorf("Error should produce output, got empty string")
		}
		if s := buf.String(); len(s) < 10 {
			t.Errorf("Error should produce meaningful output, got: %q", s)
		}
	})
}
