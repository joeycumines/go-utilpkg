package islog

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

// TestIntegration_SlogNew_Logiface_Wrapping tests bi-directional compatibility
// between slog.Logger and logiface[*Event] with islog adapter.
func TestIntegration_SlogNew_Logiface_Wrapping(t *testing.T) {
	t.Parallel()

	// Create a buffer to capture JSON output
	var buf bytes.Buffer

	// Create slog handler and logger via slog.New (standard slog pattern)
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	slogLogger := slog.New(handler)

	// Create logiface[*Event] logger wrapping the same handler
	logifaceLogger := L.New(L.WithSlogHandler(handler))

	// Log via slog.Logger (baseline)
	buf.Reset()
	slogLogger.Info("slog message", "key", "value")
	slogOutput := buf.String()

	// Log via logiface[*Event] with islog adapter (builder pattern)
	buf.Reset()
	logifaceLogger.Info().
		Str("key", "value").
		Log("logiface message")
	logifaceOutput := buf.String()

	// Both should produce valid JSON, formats may differ but both should log
	if len(slogOutput) == 0 {
		t.Error("slog.Logger produced no output")
	}
	if len(logifaceOutput) == 0 {
		t.Error("logiface[*Event] produced no output")
	}

	// Parse both as JSON to ensure validity
	var slogMap, logifaceMap map[string]any
	if err := json.Unmarshal([]byte(slogOutput), &slogMap); err != nil {
		t.Errorf("slog output invalid JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(logifaceOutput), &logifaceMap); err != nil {
		t.Errorf("logiface output invalid JSON: %v", err)
	}

	// Both should have level and message fields
	if slogMap["level"] == nil {
		t.Error("slog output missing 'level' field")
	}
	if logifaceMap["level"] == nil {
		t.Error("logiface output missing 'level' field")
	}
	if slogMap["msg"] == nil {
		t.Error("slog output missing 'msg' field")
	}
	if logifaceMap["msg"] == nil {
		t.Error("logiface output missing 'msg' field")
	}

	// Verify the key/value pair exists
	if slogMap["key"] == nil {
		t.Error("slog output missing 'key' field")
	}
	if logifaceMap["key"] == nil {
		t.Error("logiface output missing 'key' field")
	}
}

// TestIntegration_WithAttrs_Chaining tests that slog.Handler.WithAttrs()
// pre-configures attributes that appear in all log events via islog.
func TestIntegration_WithAttrs_Chaining(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	// Create handler with pre-configured attributes
	baseHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	handlerWithAttrs := baseHandler.WithAttrs([]slog.Attr{
		slog.String("service", "test-service"),
		slog.Int("version", 1),
	})

	logger := L.New(L.WithSlogHandler(handlerWithAttrs))

	// Log an event - pre-configured attrs should appear automatically
	logger.Info().
		Str("dynamic_field", "value").
		Log("test message")

	output := buf.String()

	// Parse JSON to verify attributes present
	var logMap map[string]any
	if err := json.Unmarshal([]byte(output), &logMap); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Verify pre-configured attributes
	if logMap["service"] != "test-service" {
		t.Errorf("Expected service='test-service', got %v", logMap["service"])
	}
	if logMap["version"] != float64(1) {
		t.Errorf("Expected version=1, got %v", logMap["version"])
	}

	// Verify dynamic field present
	if logMap["dynamic_field"] != "value" {
		t.Errorf("Expected dynamic_field='value', got %v", logMap["dynamic_field"])
	}
}

// TestIntegration_WithGroup_Chaining tests that slog.Handler.WithGroup()
// groups subsequent fields under the specified group name.
func TestIntegration_WithGroup_Chaining(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	// Create handler with group configured
	baseHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	handlerWithGroup := baseHandler.WithGroup("app")

	logger := L.New(L.WithSlogHandler(handlerWithGroup))

	// Log fields - should appear under "app" group
	logger.Info().
		Str("name", "myapp").
		Str("env", "production").
		Log("test message")

	output := buf.String()

	// Parse JSON to verify grouping
	var logMap map[string]any
	if err := json.Unmarshal([]byte(output), &logMap); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Verify "app" group exists and contains the fields
	appGroup, ok := logMap["app"].(map[string]any)
	if !ok {
		t.Errorf("Expected 'app' to be a map, got %T", logMap["app"])
	} else {
		if appGroup["name"] != "myapp" {
			t.Errorf("Expected app.name='myapp', got %v", appGroup["name"])
		}
		if appGroup["env"] != "production" {
			t.Errorf("Expected app.env='production', got %v", appGroup["env"])
		}
	}
}
