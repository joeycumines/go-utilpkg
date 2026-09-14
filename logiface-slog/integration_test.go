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

// TestIntegration_BuilderMethods exercises the Slice, Map, MapFields, ArgFields
// builder methods with islog adapter.
func TestIntegration_BuilderMethods(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := L.New(L.WithSlogHandler(handler))

	t.Run(`builder methods`, func(t *testing.T) {
		buf.Reset()
		logger.Info().
			Slice("slice_f", []string{"a", "b"}).
			Map("map_f", map[string]string{"k1": "v1"}).
			MapFields(map[string]any{"mf_a": "alpha"}).
			ArgFields[any](nil, "af_a", "beta").
			Log("test builder methods")

		var m map[string]any
		if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}
		if m["msg"] != "test builder methods" {
			t.Errorf("unexpected msg: %v", m["msg"])
		}
		if s, ok := m["slice_f"].([]any); !ok || len(s) != 2 || s[0] != "a" || s[1] != "b" {
			t.Errorf("unexpected slice_f: %v", m["slice_f"])
		}
		if mf, ok := m["map_f"].(map[string]any); !ok || mf["k1"] != "v1" {
			t.Errorf("unexpected map_f: %v", m["map_f"])
		}
		if m["mf_a"] != "alpha" {
			t.Errorf("unexpected mf_a: %v", m["mf_a"])
		}
		if m["af_a"] != "beta" {
			t.Errorf("unexpected af_a: %v", m["af_a"])
		}
	})

	t.Run(`context builder methods`, func(t *testing.T) {
		buf.Reset()
		ctxLogger := logger.Clone().
			Slice("ctx_slice", []string{"c1"}).
			Map("ctx_map", map[string]string{"ck": "cv"}).
			MapFields(map[string]any{"ctx_mf": "cmf"}).
			ArgFields[any](nil, "ctx_af", "caf").
			Logger()

		ctxLogger.Info().Log("test context builder methods")

		var m map[string]any
		if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}
		if m["msg"] != "test context builder methods" {
			t.Errorf("unexpected msg: %v", m["msg"])
		}
		if s, ok := m["ctx_slice"].([]any); !ok || len(s) != 1 || s[0] != "c1" {
			t.Errorf("unexpected ctx_slice: %v", m["ctx_slice"])
		}
		if mf, ok := m["ctx_map"].(map[string]any); !ok || mf["ck"] != "cv" {
			t.Errorf("unexpected ctx_map: %v", m["ctx_map"])
		}
		if m["ctx_mf"] != "cmf" {
			t.Errorf("unexpected ctx_mf: %v", m["ctx_mf"])
		}
		if m["ctx_af"] != "caf" {
			t.Errorf("unexpected ctx_af: %v", m["ctx_af"])
		}
	})
}
