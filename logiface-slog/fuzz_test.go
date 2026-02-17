package islog_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
	islog "github.com/joeycumines/logiface-slog"
)

// FuzzEventAddField tests that AddField() handles arbitrary keys without panicking.
// Fuzzes the key parameter while testing various value types systematically.
func FuzzEventAddField(f *testing.F) {
	f.Add("key")
	f.Add("empty")
	f.Add("unicode_你好_世界")
	f.Add("with\"quote")
	f.Add("with\\backslash")
	f.Add("null\x00byte")
	f.Add("with\ttab")
	f.Add("with\nnewline")
	f.Add("very_long_key_that_tests_slog_field_handling_with_extended_names")

	f.Fuzz(func(t *testing.T, key string) {
		// Test multiple value types with the fuzzed key
		testValues := []any{
			"string_value",
			"",
			42,
			int64(1234567890),
			uint64(9876543210),
			3.14,
			float64(2.718281828459045),
			true,
			false,
			nil,
			[]byte("test_bytes"),
			time.Unix(1609459200, 0).UTC(),
			time.Hour,
			slog.GroupValue(slog.String("group_key", "group_value")),
			slog.Any("raw", map[string]any{"error": "json"}),
		}

		handler := &noopHandler{}
		logger := &islog.Logger{}
		logger.Handler = handler

		for _, val := range testValues {
			event := logger.NewEvent(logiface.LevelDebug)
			if event != nil {
				// This should never panic regardless of key/val combination
				event.AddField(key, val)
				event.AddField("timestamp", time.Now())
				logger.ReleaseEvent(event)
			}
		}
	})
}

// noopHandler is a minimal slog.Handler for fuzzing that records no output.
type noopHandler struct{}

func (h *noopHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (h *noopHandler) Handle(_ context.Context, _ slog.Record) error {
	return nil
}

func (h *noopHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *noopHandler) WithGroup(name string) slog.Handler {
	return h
}
