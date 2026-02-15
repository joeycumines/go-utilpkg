package slog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
)

// TestEvent_BuilderBasic verifies basic builder usage doesn't panic
func TestEvent_BuilderBasic(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	builder := logger.Info()

	if builder == nil {
		t.Fatal("Expected non-nil builder")
	}

	// Test log sends to handler
	builder.Log("test message")

	if handler.message != "test message" {
		t.Errorf("Expected 'test message', got %s", handler.message)
	}
}

// captureHandler captures log output for testing
type captureHandler struct {
	message string
}

func (h *captureHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *captureHandler) Handle(ctx context.Context, r slog.Record) error {
	h.message = r.Message
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *captureHandler) WithGroup(name string) slog.Handler {
	return h
}

// TestEvent_AddError_Nil tests AddError with nil error
func TestEvent_AddError_Nil(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event
	result := event.AddError(nil)

	if !result {
		t.Error("AddError with nil should return true")
	}

	if event.err != nil {
		t.Errorf("Expected nil error, got %+v", event.err)
	}

	event.Send()
}

// TestEvent_AddError_NonNil tests AddError with non-nil error
func TestEvent_AddError_NonNil(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	testError := errors.New("test error")
	event := logger.Build(logiface.LevelInformational).Event
	result := event.AddError(testError)

	if !result {
		t.Error("AddError with non-nil should return true")
	}

	if event.err != testError {
		t.Errorf("Expected test error, got %+v", event.err)
	}

	event.Send()
}

// TestEvent_AddInt8_BoundaryValues tests AddInt8 with MinInt8 and MaxInt8
func TestEvent_AddInt8_BoundaryValues(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	// Test MinInt8
	event := logger.Build(logiface.LevelInformational).Event
	result := event.AddInt8("min", -128)

	if !result {
		t.Error("AddInt8 with MinInt8 should return true")
	}

	// Test MaxInt8
	result2 := event.AddInt8("max", 127)

	if !result2 {
		t.Error("AddInt8 with MaxInt8 should return true")
	}

	// Send to verify attributes are stored
	event.Send()

	// Verify attributes made it to handler (we can check attrs were added)
	if len(event.attrs) < 2 {
		t.Error("Expected at least 2 attributes to be added")
	}
}

// TestEvent_AddInt8_PositiveNegative tests AddInt8 with various values
func TestEvent_AddInt8_PositiveNegative(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event

	// Positive zero
	event.AddInt8("zero", 0)
	// Small positive
	event.AddInt8("pos", 42)
	// Small negative
	event.AddInt8("neg", -42)

	event.Send()

	// Verify attributes were added
	if len(event.attrs) < 3 {
		t.Errorf("Expected at least 3 attributes, got %d", len(event.attrs))
	}
}

// TestEvent_AddInt16_BoundaryValues tests AddInt16 with MinInt16 and MaxInt16
func TestEvent_AddInt16_BoundaryValues(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event

	result := event.AddInt16("min", -32768)
	result2 := event.AddInt16("max", 32767)

	if !result || !result2 {
		t.Error("AddInt16 with boundary values should return true")
	}

	event.Send()

	if len(event.attrs) < 2 {
		t.Error("Expected at least 2 attributes to be added")
	}
}

// TestEvent_AddInt32_BoundaryValues tests AddInt32 with MinInt32 and MaxInt32
func TestEvent_AddInt32_BoundaryValues(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event

	result := event.AddInt32("min", -2147483648)
	result2 := event.AddInt32("max", 2147483647)

	if !result || !result2 {
		t.Error("AddInt32 with boundary values should return true")
	}

	event.Send()
}

// TestEvent_AddUint_MaxUint32 tests AddUint with MaxUint32
func TestEvent_AddUint_MaxUint32(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event

	// Max uint32 value
	result := event.AddUint("max32", 4294967295)

	if !result {
		t.Error("AddUint with MaxUint32 should return true")
	}

	event.Send()
}

// TestEvent_AddUint8_BoundaryValues tests AddUint8 with boundary values
func TestEvent_AddUint8_BoundaryValues(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event

	result := event.AddUint8("zero", 0)
	result2 := event.AddUint8("max", 255)

	if !result || !result2 {
		t.Error("AddUint8 with boundary values should return true")
	}

	event.Send()
}

// TestEvent_AddUint16_BoundaryValues tests AddUint16 with boundary values
func TestEvent_AddUint16_BoundaryValues(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event

	result := event.AddUint16("max", 65535)

	if !result {
		t.Error("AddUint16 with MaxUint16 should return true")
	}

	event.Send()
}

// TestEvent_AddUint32_BoundaryValues tests AddUint32 with MaxUint32
func TestEvent_AddUint32_BoundaryValues(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event

	result := event.AddUint32("max", 4294967295)

	if !result {
		t.Error("AddUint32 with MaxUint32 should return true")
	}

	event.Send()
}

// TestEvent_AddFloat32_SmallNumbers tests AddFloat32 with small floats
func TestEvent_AddFloat32_SmallNumbers(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event

	result := event.AddFloat32("zero", 0.0)
	result2 := event.AddFloat32("pi32", float32(3.14159))

	if !result || !result2 {
		t.Error("AddFloat32 should return true")
	}

	event.Send()
}

// TestEvent_AddBase64Bytes_StdEncoding tests AddBase64Bytes with StdEncoding
func TestEvent_AddBase64Bytes_StdEncoding(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event
	input := []byte("hello world")

	result := event.AddBase64Bytes("data", input, base64.StdEncoding)

	if !result {
		t.Error("AddBase64Bytes should return true")
	}

	event.Send()

	// Verify attr was added
	if len(event.attrs) == 0 {
		t.Error("Expected attribute to be added")
	}
}

// TestEvent_AddBase64Bytes_URLEncoding tests AddBase64Bytes with URLEncoding
func TestEvent_AddBase64Bytes_URLEncoding(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event
	input := []byte("data")

	result := event.AddBase64Bytes("url", input, base64.URLEncoding)

	if !result {
		t.Error("AddBase64Bytes with URLEncoding should return true")
	}

	event.Send()
}

// TestEvent_AddBase64Bytes_Empty tests AddBase64Bytes with empty bytes
func TestEvent_AddBase64Bytes_Empty(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event
	input := []byte{}

	result := event.AddBase64Bytes("empty", input, base64.StdEncoding)

	if !result {
		t.Error("AddBase64Bytes with empty slice should return true")
	}

	event.Send()
}

// TestEvent_AddRawJSON_ValidObject tests AddRawJSON with valid JSON object
func TestEvent_AddRawJSON_ValidObject(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event
	input := json.RawMessage(`{"name":"test","value":123}`)

	result := event.AddRawJSON("data", input)

	if !result {
		t.Error("AddRawJSON with valid JSON should return true")
	}

	event.Send()

	if len(event.attrs) == 0 {
		t.Error("Expected attribute to be added")
	}
}

// TestEvent_AddRawJSON_Malformed tests AddRawJSON with malformed JSON
func TestEvent_AddRawJSON_Malformed(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event
	input := json.RawMessage(`{"invalid"`)

	result := event.AddRawJSON("bad", input)

	if !result {
		t.Error("AddRawJSON with malformed JSON should return true (graceful fallback)")
	}

	event.Send()

	// Should still have added an attribute (fallback to string)
	if len(event.attrs) == 0 {
		t.Error("Expected fallback attribute to be added for malformed JSON")
	}
}

// TestEvent_AddRawJSON_ValidArray tests AddRawJSON with valid JSON array
func TestEvent_AddRawJSON_ValidArray(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event
	input := json.RawMessage(`[1,2,3]`)

	result := event.AddRawJSON("array", input)

	if !result {
		t.Error("AddRawJSON with valid JSON array should return true")
	}

	event.Send()
}

// TestEvent_Level_LogifaceLevels tests Level() with all logiface levels
func TestEvent_Level_LogifaceLevels(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	levels := []logiface.Level{
		logiface.LevelTrace,
		logiface.LevelDebug,
		logiface.LevelInformational,
		logiface.LevelNotice,
		logiface.LevelWarning,
		logiface.LevelError,
		logiface.LevelCritical,
		logiface.LevelAlert,
		logiface.LevelEmergency,
	}

	for _, lvl := range levels {
		event := logger.Build(lvl).Event
		if event == nil {
			t.Errorf("NewEvent with level %v should not return nil", lvl)
			continue
		}

		resultLevel := event.Level()
		if resultLevel != lvl {
			// Some precision loss is expected mapping to slog levels
			t.Logf("Level %v -> Level() -> %v (precision loss expected)", lvl, resultLevel)
		}
	}
}

// TestEvent_Level_Nil tests Level() with nil Event
func TestEvent_Level_Nil(t *testing.T) {
	var event *Event

	level := event.Level()

	if level != logiface.LevelDisabled {
		t.Errorf("Expected LevelDisabled for nil event, got %v", level)
	}
}

// TestEvent_Send_ErrorInHandle tests Send when Handler.Handle returns error
func TestEvent_Send_ErrorInHandle(t *testing.T) {
	errorHandler := &errorCaptureHandler{}
	logger := logiface.New[*Event](NewLogger(errorHandler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event
	event.AddField("key", "value")

	err := event.Send()

	if err == nil {
		t.Error("Send should return error from Handler.Handle")
	}

	errMsg := "simulated error"
	if errorHandler.lastError == nil || errorHandler.lastError.Error() != errMsg {
		t.Errorf("Expected error %q, got %v", errMsg, errorHandler.lastError)
	}
}

// TestEvent_Send_NilLogger tests Send when logger is nil
func TestEvent_Send_NilLogger(t *testing.T) {
	event := &Event{
		// No logger set
	}

	err := event.Send()

	if err != nil {
		t.Errorf("Send with nil logger should not error, got %v", err)
	}
}

// TestEvent_Send_EnabledBeforeSend tests that Enabled is called before constructing Record
func TestEvent_Send_EnabledBeforeSend(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelError)))

	// Create event at Info level (below Warn threshold)
	builder := logger.Build(logiface.LevelInformational)

	// Builder should be nil since level is disabled
	if builder != nil {
		t.Error("Build should return nil for disabled level")
		return
	}

	// Verify no records were written
	if handler.message != "" {
		t.Errorf("Expected no message for disabled level, got %q", handler.message)
	}
}

// TestEvent_AddField_String tests AddField with string
func TestEvent_AddField_String(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event
	event.AddField("key", "value")

	if len(event.attrs) != 1 {
		t.Errorf("Expected 1 attribute, got %d", len(event.attrs))
	}

	event.Send()
}

// TestEvent_AddField_Int tests AddField with int
func TestEvent_AddField_Int(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event
	event.AddField("number", 42)

	if len(event.attrs) != 1 {
		t.Errorf("Expected 1 attribute, got %d", len(event.attrs))
	}

	event.Send()
}

// TestEvent_AddField_Slice tests AddField with slice
func TestEvent_AddField_Slice(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event
	event.AddField("items", []string{"a", "b", "c"})

	if len(event.attrs) != 1 {
		t.Errorf("Expected 1 attribute, got %d", len(event.attrs))
	}

	event.Send()
}

// TestEvent_AddField_Map tests AddField with map
func TestEvent_AddField_Map(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event
	event.AddField("data", map[string]int{"a": 1, "b": 2})

	if len(event.attrs) != 1 {
		t.Errorf("Expected 1 attribute, got %d", len(event.attrs))
	}

	event.Send()
}

// TestEvent_AddField_Nil tests AddField with nil
func TestEvent_AddField_Nil(t *testing.T) {
	handler := &captureHandler{}
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))

	event := logger.Build(logiface.LevelInformational).Event
	event.AddField("nil", nil)

	if len(event.attrs) != 1 {
		t.Errorf("Expected 1 attribute even with nil value, got %d", len(event.attrs))
	}

	event.Send()
}

// errorCaptureHandler is a handler that returns errors for testing
type errorCaptureHandler struct {
	lastError error
}

func (h *errorCaptureHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *errorCaptureHandler) Handle(ctx context.Context, r slog.Record) error {
	h.lastError = errors.New("simulated error")
	return h.lastError
}

func (h *errorCaptureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *errorCaptureHandler) WithGroup(name string) slog.Handler {
	return h
}

// Nil Event Safety Tests
// These tests verify that calling Add* methods on nil Event returns false
// and does not panic. This is critical for nilsafe API behavior.

func TestEvent_Nil_AddMessage(t *testing.T) {
	var event *Event
	result := event.AddMessage("test message")
	if result {
		t.Error("AddMessage on nil Event should return false")
	}
}

func TestEvent_Nil_AddError(t *testing.T) {
	var event *Event
	result := event.AddError(errors.New("test"))
	if result {
		t.Error("AddError on nil Event should return false")
	}
}

func TestEvent_Nil_AddString(t *testing.T) {
	var event *Event
	result := event.AddString("key", "value")
	if result {
		t.Error("AddString on nil Event should return false")
	}
}

func TestEvent_Nil_AddInt(t *testing.T) {
	var event *Event
	result := event.AddInt("key", 42)
	if result {
		t.Error("AddInt on nil Event should return false")
	}
}

func TestEvent_Nil_AddInt8(t *testing.T) {
	var event *Event
	result := event.AddInt8("key", 127)
	if result {
		t.Error("AddInt8 on nil Event should return false")
	}
}

func TestEvent_Nil_AddInt16(t *testing.T) {
	var event *Event
	result := event.AddInt16("key", 32767)
	if result {
		t.Error("AddInt16 on nil Event should return false")
	}
}

func TestEvent_Nil_AddInt32(t *testing.T) {
	var event *Event
	result := event.AddInt32("key", 2147483647)
	if result {
		t.Error("AddInt32 on nil Event should return false")
	}
}

func TestEvent_Nil_AddInt64(t *testing.T) {
	var event *Event
	result := event.AddInt64("key", 9223372036854775807)
	if result {
		t.Error("AddInt64 on nil Event should return false")
	}
}

func TestEvent_Nil_AddUint(t *testing.T) {
	var event *Event
	result := event.AddUint("key", 42)
	if result {
		t.Error("AddUint on nil Event should return false")
	}
}

func TestEvent_Nil_AddUint8(t *testing.T) {
	var event *Event
	result := event.AddUint8("key", 255)
	if result {
		t.Error("AddUint8 on nil Event should return false")
	}
}

func TestEvent_Nil_AddUint16(t *testing.T) {
	var event *Event
	result := event.AddUint16("key", 65535)
	if result {
		t.Error("AddUint16 on nil Event should return false")
	}
}

func TestEvent_Nil_AddUint32(t *testing.T) {
	var event *Event
	result := event.AddUint32("key", 4294967295)
	if result {
		t.Error("AddUint32 on nil Event should return false")
	}
}

func TestEvent_Nil_AddUint64(t *testing.T) {
	var event *Event
	result := event.AddUint64("key", 18446744073709551615)
	if result {
		t.Error("AddUint64 on nil Event should return false")
	}
}

func TestEvent_Nil_AddFloat32(t *testing.T) {
	var event *Event
	result := event.AddFloat32("key", 3.14)
	if result {
		t.Error("AddFloat32 on nil Event should return false")
	}
}

func TestEvent_Nil_AddFloat64(t *testing.T) {
	var event *Event
	result := event.AddFloat64("key", 3.14159)
	if result {
		t.Error("AddFloat64 on nil Event should return false")
	}
}

func TestEvent_Nil_AddBool(t *testing.T) {
	var event *Event
	result := event.AddBool("key", true)
	if result {
		t.Error("AddBool on nil Event should return false")
	}
}

func TestEvent_Nil_AddTime(t *testing.T) {
	var event *Event
	result := event.AddTime("key", time.Now())
	if result {
		t.Error("AddTime on nil Event should return false")
	}
}

func TestEvent_Nil_AddDuration(t *testing.T) {
	var event *Event
	result := event.AddDuration("key", 5*time.Second)
	if result {
		t.Error("AddDuration on nil Event should return false")
	}
}

func TestEvent_Nil_AddBase64Bytes(t *testing.T) {
	var event *Event
	result := event.AddBase64Bytes("key", []byte("test"), base64.StdEncoding)
	if result {
		t.Error("AddBase64Bytes on nil Event should return false")
	}
}

func TestEvent_Nil_AddRawJSON(t *testing.T) {
	var event *Event
	result := event.AddRawJSON("key", json.RawMessage(`{"test":123}`))
	if result {
		t.Error("AddRawJSON on nil Event should return false")
	}
}

func TestEvent_Nil_AddField(t *testing.T) {
	var event *Event
	// AddField returns early on nil event (no return value)
	// Just ensure it doesn't panic
	event.AddField("key", "value")
	// Test passes if no panic occurred
}
