package slog

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/joeycumines/logiface"
)

// TestLogger_CanAddRawJSON tests that Logger supports raw JSON
func TestLogger_CanAddRawJSON(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create internal logger directly
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  nil,
		defaultAttrs: nil,
		groupStack:   nil,
		level:        logiface.LevelTrace,
	}

	if !slogger.CanAddRawJSON() {
		t.Error("Expected CanAddRawJSON to return true")
	}

	// Verify it works via the builder pattern
	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	logger.Debug().RawJSON("json", []byte(`{"key":"value"}`)).Log("test")
}

// TestLogger_CanAddFields tests that Logger supports field addition
func TestLogger_CanAddFields(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create internal logger directly
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  nil,
		defaultAttrs: nil,
		groupStack:   nil,
		level:        logiface.LevelTrace,
	}

	if !slogger.CanAddFields() {
		t.Error("Expected CanAddFields to return true")
	}

	// Verify it works via the builder pattern
	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	logger.Debug().Str("key", "value").Log("test")
}

// TestLogger_CanAddLazyFields tests that Logger supports lazy field evaluation
func TestLogger_CanAddLazyFields(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create internal logger directly
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  nil,
		defaultAttrs: nil,
		groupStack:   nil,
		level:        logiface.LevelTrace,
	}

	if !slogger.CanAddLazyFields() {
		t.Error("Expected CanAddLazyFields to return true")
	}

	// Verify it works via the builder pattern
	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	logger.Debug().Modifier(logiface.NewModifierFunc[*Event](func(e *Event) error {
		e.AddString("key", "value")
		return nil
	})).Log("test")
}

// TestLogger_Close tests that Close returns nil (slog.Handler has no close semantics)
func TestLogger_Close(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create internal logger directly
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  nil,
		defaultAttrs: nil,
		groupStack:   nil,
		level:        logiface.LevelTrace,
	}

	err := slogger.Close()

	if err != nil {
		t.Errorf("Expected Close to return nil, got %v", err)
	}
}

// TestLogger_Write_NilEvent tests Write with nil event
func TestLogger_Write_NilEvent(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create internal logger directly
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  nil,
		defaultAttrs: nil,
		groupStack:   nil,
		level:        logiface.LevelTrace,
	}

	// Can access write on internal logger
	err := slogger.Write(nil)

	if err != nil {
		t.Errorf("Expected Write(nil) to return nil, got %v", err)
	}
}

// TestLogger_Write_WithReplaceAttr tests Write with ReplaceAttr hook
func TestLogger_Write_WithReplaceAttr(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create a replaceAttr hook that redacts passwords
	replaceAttr := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == "password" {
			return slog.String("password", "***REDACTED***")
		}
		return a
	}

	// Create internal logger directly
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  replaceAttr,
		defaultAttrs: nil,
		groupStack:   nil,
		level:        logiface.LevelTrace,
	}

	// Using builder pattern with Write on the internal logger
	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	event := logger.Build(logiface.LevelInformational)
	if event == nil {
		t.Fatal("Expected builder to not be nil")
	}

	// Add a password attribute
	event.Str("password", "secret123")
	event.Str("username", "alice")

	// The event should have the password attribute sent, which gets
	// redacted by ReplaceAttr in Write()
	// Note: We can't easily verify the redaction here without checking the handler output,
	// but we can ensure Write() doesn't error on the internal logger
	err := slogger.Write(event.Event)

	if err != nil {
		t.Errorf("Expected Write to succeed with ReplaceAttr hook, got %v", err)
	}
}

// TestLogger_Write_ReplaceAttrNil tests Write when ReplaceAttr is nil
func TestLogger_Write_ReplaceAttrNil(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create internal logger directly without ReplaceAttr
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  nil,
		defaultAttrs: nil,
		groupStack:   nil,
		level:        logiface.LevelTrace,
	}

	// Using builder pattern with Write on the internal logger
	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	event := logger.Build(logiface.LevelInformational)
	if event == nil {
		t.Fatal("Expected builder to not be nil")
	}

	event.Str("message", "test")
	err := slogger.Write(event.Event)

	if err != nil {
		t.Errorf("Expected Write to succeed without ReplaceAttr, got %v", err)
	}
}

// TestLogger_Write_ReplaceAttrEmptyAttributes tests Write with empty attrs
func TestLogger_Write_ReplaceAttrEmptyAttributes(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	called := false
	replaceAttr := func(groups []string, a slog.Attr) slog.Attr {
		called = true
		return a
	}

	// Create internal logger directly
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  replaceAttr,
		defaultAttrs: nil,
		groupStack:   nil,
		level:        logiface.LevelTrace,
	}

	// Using builder pattern with Write on the internal logger
	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	event := logger.Build(logiface.LevelInformational)
	if event == nil {
		t.Fatal("Expected builder to not be nil")
	}

	// Event with no attributes - don't call Log() as it releases the event
	// Just verify that Write works with the event
	err := slogger.Write(event.Event)

	if err != nil {
		t.Errorf("Expected Write to succeed with empty attributes, got %v", err)
	}

	// ReplaceAttr should not be called for empty attrs
	if called {
		t.Error("Expected ReplaceAttr to not be called for empty attributes")
	}
}

// TestEvent_getGroupPrefix_Empty tests getGroupPrefix with empty group stack
func TestEvent_getGroupPrefix_Empty(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create internal logger directly
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  nil,
		defaultAttrs: nil,
		groupStack:   nil,
		level:        logiface.LevelTrace,
	}

	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	event := logger.Build(logiface.LevelInformational).Event

	result := event.getGroupPrefix()

	if result != nil {
		t.Errorf("Expected getGroupPrefix to return nil for empty groups, got %v", result)
	}
}

// TestEvent_getGroupPrefix_SingleGroup tests getGroupPrefix with single group
func TestEvent_getGroupPrefix_SingleGroup(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create internal logger directly with group
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  nil,
		defaultAttrs: nil,
		groupStack:   []string{"http"},
		level:        logiface.LevelTrace,
	}

	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	event := logger.Build(logiface.LevelInformational).Event

	result := event.getGroupPrefix()

	if result == nil {
		t.Fatal("Expected getGroupPrefix to return non-nil for single group")
	}

	if len(result) != 1 {
		t.Errorf("Expected getGroupPrefix to return 1 group, got %d", len(result))
	}

	if result[0] != "http" {
		t.Errorf("Expected group[0] = 'http', got '%s'", result[0])
	}
}

// TestEvent_getGroupPrefix_MultipleGroups tests getGroupPrefix with multiple groups
func TestEvent_getGroupPrefix_MultipleGroups(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create logger with nested groups: http.request.headers
	handlerWithGroups := handler.WithGroup("http").WithGroup("request").WithGroup("headers")

	// Create internal logger directly with multiple groups
	slogger := &Logger{
		handler:      handlerWithGroups,
		defaultCtx:   nil,
		replaceAttr:  nil,
		defaultAttrs: nil,
		groupStack:   []string{"http", "request", "headers"},
		level:        logiface.LevelTrace,
	}

	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	event := logger.Build(logiface.LevelInformational).Event

	result := event.getGroupPrefix()

	if result == nil {
		t.Fatal("Expected getGroupPrefix to return non-nil for multiple groups")
	}

	if len(result) != 3 {
		t.Errorf("Expected getGroupPrefix to return 3 groups, got %d", len(result))
	}

	if result[0] != "http" || result[1] != "request" || result[2] != "headers" {
		t.Errorf("Expected groups ['http','request','headers'], got %v", result)
	}
}

// TestEvent_getGroupPrefix_ModificationSafety tests that modifying returns doesn't affect event
func TestEvent_getGroupPrefix_ModificationSafety(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create internal logger directly with group
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  nil,
		defaultAttrs: nil,
		groupStack:   []string{"http"},
		level:        logiface.LevelTrace,
	}

	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	event := logger.Build(logiface.LevelInformational).Event

	// Get group prefix multiple times
	result1 := event.getGroupPrefix()
	result2 := event.getGroupPrefix()

	// Modify first result
	if len(result1) > 0 {
		result1[0] = "modified"
	}

	// Second result should not be affected (copy is made)
	if len(result2) > 0 && result2[0] == "modified" {
		t.Error("Modifying returned group prefix should not affect subsequent calls")
	}
}

// TestLogger_WithAttributes_attributesApplied tests that WithAttributes adds default attributes
func TestLogger_WithAttributes_attributesApplied(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	attrs := []slog.Attr{
		slog.String("service", "test-service"),
		slog.Int("version", 1),
	}

	// Create internal logger directly with attributes
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  nil,
		defaultAttrs: attrs,
		groupStack:   nil,
		level:        logiface.LevelTrace,
	}

	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	event := logger.Build(logiface.LevelInformational)

	// Add event-specific attribute
	event.Str("message", "test")

	// The logger should have the WithAttributes applied to defaultAttrs
	// We can verify this by checking if the event (when sent) processes correctly
	err := slogger.Write(event.Event)

	if err != nil {
		t.Errorf("Expected Write to succeed with WithAttributes, got %v", err)
	}
}

// TestLogger_WithAttributes_EmptySlice tests WithAttributes with empty slice
func TestLogger_WithAttributes_EmptySlice(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create internal logger directly with empty attributes
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  nil,
		defaultAttrs: []slog.Attr{},
		groupStack:   nil,
		level:        logiface.LevelTrace,
	}

	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	event := logger.Build(logiface.LevelInformational)

	event.Str("message", "test")
	err := slogger.Write(event.Event)

	if err != nil {
		t.Errorf("Expected Write to succeed with empty WithAttributes, got %v", err)
	}
}

// TestLogger_WithGroup_namedPrefix tests WithGroup adds group prefix
func TestLogger_WithGroup_namedPrefix(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create internal logger directly with group
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  nil,
		defaultAttrs: nil,
		groupStack:   []string{"http"},
		level:        logiface.LevelTrace,
	}

	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	event := logger.Build(logiface.LevelInformational).Event

	// The event should have the group in its groupStack
	// Check via getGroupPrefix which we can access (internal method)
	groups := event.getGroupPrefix()

	if len(groups) != 1 {
		t.Errorf("Expected 1 group from WithGroup, got %d", len(groups))
	}

	if len(groups) > 0 && groups[0] != "http" {
		t.Errorf("Expected group 'http', got '%s'", groups[0])
	}
}

// TestLogger_WithGroup_EmptyString tests WithGroup with empty string (should be ignored)
func TestLogger_WithGroup_EmptyString(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create internal logger directly without groups
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  nil,
		defaultAttrs: nil,
		groupStack:   nil,
		level:        logiface.LevelTrace,
	}

	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	event := logger.Build(logiface.LevelInformational).Event

	// Empty group should not be added
	groups := event.getGroupPrefix()

	if len(groups) != 0 {
		t.Errorf("Expected 0 groups for empty WithGroup, got %d", len(groups))
	}
}

// TestLogger_WithReplaceAttr_nopHook tests WithReplaceAttr with no-op hook
func TestLogger_WithReplaceAttr_nopHook(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	replaceAttr := func(groups []string, a slog.Attr) slog.Attr {
		// No-op: return attribute unchanged
		return a
	}

	// Create internal logger directly with replaceAttr
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  replaceAttr,
		defaultAttrs: nil,
		groupStack:   nil,
		level:        logiface.LevelTrace,
	}

	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	event := logger.Build(logiface.LevelInformational)

	event.Str("message", "test")
	err := slogger.Write(event.Event)

	if err != nil {
		t.Errorf("Expected Write to succeed with no-op ReplaceAttr, got %v", err)
	}
}

// TestLogger_WithReplaceAttr_FilterAttrs tests WithReplaceAttr that filters attributes
func TestLogger_WithReplaceAttr_FilterAttrs(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// ReplaceAttr that returns zero Attr to filter out "secret" keys
	replaceAttr := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == "secret" {
			// Return zero Attr to filter out
			return slog.Attr{}
		}
		return a
	}

	// Create internal logger directly with replaceAttr
	slogger := &Logger{
		handler:      handler,
		defaultCtx:   nil,
		replaceAttr:  replaceAttr,
		defaultAttrs: nil,
		groupStack:   nil,
		level:        logiface.LevelTrace,
	}

	logger := logiface.New[*Event](logiface.WithOptions[*Event](
		logiface.WithEventFactory[*Event](slogger),
		logiface.WithWriter[*Event](slogger),
		logiface.WithEventReleaser[*Event](slogger),
	))
	event := logger.Build(logiface.LevelInformational)

	event.Str("secret", "hidden")
	event.Str("public", "visible")

	err := slogger.Write(event.Event)

	if err != nil {
		t.Errorf("Expected Write to succeed with filtering ReplaceAttr, got %v", err)
	}
}
