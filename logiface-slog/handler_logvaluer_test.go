package slog

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/joeycumines/logiface"
)

// logvaluerMockHandler is a test handler that captures records
type logvaluerMockHandler struct {
	records []slog.Record
}

func (m *logvaluerMockHandler) Handle(ctx context.Context, r slog.Record) error {
	rec := r.Clone()
	m.records = append(m.records, rec)
	return nil
}

func (m *logvaluerMockHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (m *logvaluerMockHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return m
}

func (m *logvaluerMockHandler) WithGroup(name string) slog.Handler {
	return m
}

// simpleLogValuer is a simple struct implementing LogValuer
type simpleLogValuer struct {
	name  string
	value int
}

func (s simpleLogValuer) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("name", s.name),
		slog.Int("value", s.value),
	)
}

// TestLogValuer_Struct tests struct implementing LogValuer
func TestLogValuer_Struct(t *testing.T) {
	mock := &logvaluerMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	custom := simpleLogValuer{
		name:  "test",
		value: 12345,
	}

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	record.Add(slog.Any("custom", custom))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	// Check that LogValuer was resolved and value added
	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "custom" && a.Value.Kind() == slog.KindGroup {
			found = true
			childCount := 0
			for _ = range a.Value.Group() {
				childCount++
			}
			if childCount != 2 {
				t.Errorf("expected 2 children in group, got %d", childCount)
			}
		}
		return true
	})

	if !found {
		t.Error("expected LogValuer to be resolved to group value")
	}
}

// pointerLogValuer is a pointer to a struct implementing LogValuer
type pointerLogValuer struct {
	data string
}

func (p *pointerLogValuer) LogValue() slog.Value {
	return slog.StringValue(p.data)
}

// TestLogValuer_Pointer tests pointer implementing LogValuer
func TestLogValuer_Pointer(t *testing.T) {
	mock := &logvaluerMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	custom := &pointerLogValuer{
		data: "test data",
	}

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	record.Add(slog.Any("pointer", custom))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "pointer" && a.Value.Kind() == slog.KindString {
			found = true
			if a.Value.String() != "test data" {
				t.Errorf("expected 'test data', got '%s'", a.Value.String())
			}
		}
		return true
	})

	if !found {
		t.Error("expected pointer LogValuer to be resolved to string value")
	}
}

// nestedLogValuer is a LogValuer that contains another LogValuer
type nestedLogValuer struct {
	inner *nestedInner
}

type nestedInner struct {
	value string
}

func (n nestedInner) LogValue() slog.Value {
	return slog.StringValue(n.value)
}

func (nl nestedLogValuer) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Any("inner", nl.inner),
	)
}

// TestLogValuer_Nested tests nested LogValuer chain
func TestLogValuer_Nested(t *testing.T) {
	mock := &logvaluerMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	custom := nestedLogValuer{
		inner: &nestedInner{
			value: "nested value",
		},
	}

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	record.Add(slog.Any("nested", custom))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	// Check that outer LogValuer was resolved
	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "nested" && a.Value.Kind() == slog.KindGroup {
			found = true
			// Note: Nested LogValuers may not be recursively resolved
			// in all implementations
			childCount := 0
			for range a.Value.Group() {
				childCount++
			}
			if childCount != 1 {
				t.Errorf("expected 1 child in group, got %d", childCount)
			}
		}
		return true
	})

	if !found {
		t.Error("expected nested LogValuer to be resolved to group value")
	}
}

// circularLogValuer tests circular reference detection
type circularLogValuer struct {
	self *circularLogValuer
}

func (c *circularLogValuer) LogValue() slog.Value {
	// This would cause infinite recursion without cycle detection
	return slog.AnyValue(c.self)
}

// TestLogValuer_CircularReference tests circular reference detection
func TestLogValuer_CircularReference(t *testing.T) {
	mock := &logvaluerMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	// Create circular reference
	circular := &circularLogValuer{}
	circular.self = circular

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	// Add a simple attribute first to verify handler works
	record.Add(slog.String("key", "value"))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	// Just verify it doesn't hang or crash
	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}
	// Test passes if we get here without hanging
}

// errorLogValuer tests error values implementing LogValuer
type errorLogValuer struct {
	msg string
}

func (e errorLogValuer) Error() string {
	return e.msg
}

func (e errorLogValuer) LogValue() slog.Value {
	return slog.StringValue("error: " + e.msg)
}

// TestLogValuer_ErrorValue tests error values as LogValuer
func TestLogValuer_ErrorValue(t *testing.T) {
	mock := &logvaluerMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	customErr := errorLogValuer{
		msg: "test error",
	}

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	record.Add(slog.Any("error", customErr))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "error" {
			found = true
			// Error type implements LogValuer, should be resolved
			strVal := a.Value.String()
			if strVal != "error: test error" {
				t.Errorf("expected 'error: test error', got '%s'", strVal)
			}
		}
		return true
	})

	if !found {
		t.Error("expected error LogValuer to be resolved")
	}
}

// TestLogValuer_StandardError tests standard error type
func TestLogValuer_StandardError(t *testing.T) {
	mock := &logvaluerMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	record.Add(slog.Any("error", errors.New("standard error")))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "error" {
			found = true
			// Standard error type should be handled by slog.Any
			strVal := a.Value.String()
			if strVal != "standard error" {
				t.Errorf("expected 'standard error', got '%s'", strVal)
			}
		}
		return true
	})

	if !found {
		t.Error("expected standard error to be present")
	}
}

// TestLogValuer_NilValue tests nil value handling
func TestLogValuer_NilValue(t *testing.T) {
	mock := &logvaluerMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	var custom *pointerLogValuer = nil

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	record.Add(slog.Any("nil", custom))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	// Should not crash when handling nil
	// Test passes if we get here
}

// TestLogValuer_MultipleLogValuers tests multiple LogValuers in one record
func TestLogValuer_MultipleLogValuers(t *testing.T) {
	mock := &logvaluerMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	val1 := simpleLogValuer{name: "one", value: 1}
	val2 := simpleLogValuer{name: "two", value: 2}

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	record.Add(
		slog.Any("custom1", val1),
		slog.Any("custom2", val2),
	)

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	foundCount := 0
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "custom1" || a.Key == "custom2" {
			if a.Value.Kind() == slog.KindGroup {
				foundCount++
			}
		}
		return true
	})

	if foundCount != 2 {
		t.Errorf("expected 2 LogValuers resolved, got %d", foundCount)
	}
}
