package slog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
)

// mockHandler is a test handler that captures records
type mockHandler struct {
	records []slog.Record
}

func (m *mockHandler) Handle(ctx context.Context, r slog.Record) error {
	rec := r.Clone()
	m.records = append(m.records, rec)
	return nil
}

func (m *mockHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (m *mockHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return m
}

func (m *mockHandler) WithGroup(name string) slog.Handler {
	return m
}

// TestHandle_BasicFields tests that Handle converts basic Record fields correctly
func TestHandle_BasicFields(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	testLevel := slog.LevelInfo
	testMessage := "test message"

	record := slog.NewRecord(time.Now(), testLevel, testMessage, 0)

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	r := mock.records[0]
	if r.Level != testLevel {
		t.Errorf("expected level %v, got %v", testLevel, r.Level)
	}
	if r.Message != testMessage {
		t.Errorf("expected message %q, got %q", testMessage, r.Message)
	}
	// Time is set by Event.Send() so we can't check exact value
}

// TestHandle_ValueKind_String tests String Value kind conversion
func TestHandle_ValueKind_String(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.Add(slog.String("key", "value"))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	// Verify the string attribute was added
	var found bool
	attrs := []slog.Attr{}
	mock.records[0].Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	for _, attr := range attrs {
		if attr.Key == "key" && attr.Value.Kind() == slog.KindString {
			if attr.Value.String() != "value" {
				t.Errorf("expected value 'value', got '%s'", attr.Value.String())
			}
			found = true
		}
	}

	if !found {
		t.Error("string attribute not found")
	}
}

// TestHandle_ValueKind_Int64 tests Int64 Value kind conversion
func TestHandle_ValueKind_Int64(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.Add(slog.Int64("key", 12345))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "key" && a.Value.Kind() == slog.KindInt64 {
			if a.Value.Int64() != 12345 {
				t.Errorf("expected value 12345, got %d", a.Value.Int64())
			}
			found = true
		}
		return true
	})

	if !found {
		t.Error("int64 attribute not found")
	}
}

// TestHandle_ValueKind_Uint64 tests Uint64 Value kind conversion
func TestHandle_ValueKind_Uint64(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.Add(slog.Uint64("key", 54321))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "key" && a.Value.Kind() == slog.KindUint64 {
			if a.Value.Uint64() != 54321 {
				t.Errorf("expected value 54321, got %d", a.Value.Uint64())
			}
			found = true
		}
		return true
	})

	if !found {
		t.Error("uint64 attribute not found")
	}
}

// TestHandle_ValueKind_Float64 tests Float64 Value kind conversion
func TestHandle_ValueKind_Float64(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.Add(slog.Float64("key", 3.14159))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "key" && a.Value.Kind() == slog.KindFloat64 {
			if a.Value.Float64() != 3.14159 {
				t.Errorf("expected value 3.14159, got %f", a.Value.Float64())
			}
			found = true
		}
		return true
	})

	if !found {
		t.Error("float64 attribute not found")
	}
}

// TestHandle_ValueKind_Bool tests Bool Value kind conversion
func TestHandle_ValueKind_Bool(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.Add(slog.Bool("enabled", true))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "enabled" && a.Value.Kind() == slog.KindBool {
			if a.Value.Bool() != true {
				t.Errorf("expected value true, got %v", a.Value.Bool())
			}
			found = true
		}
		return true
	})

	if !found {
		t.Error("bool attribute not found")
	}
}

// TestHandle_ValueKind_Duration tests Duration Value kind conversion
func TestHandle_ValueKind_Duration(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.Add(slog.Duration("elapsed", 5*time.Second))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "elapsed" && a.Value.Kind() == slog.KindDuration {
			if a.Value.Duration() != 5*time.Second {
				t.Errorf("expected value 5s, got %v", a.Value.Duration())
			}
			found = true
		}
		return true
	})

	if !found {
		t.Error("duration attribute not found")
	}
}

// TestHandle_ValueKind_Time tests Time Value kind conversion
func TestHandle_ValueKind_Time(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	testTime := time.Date(2024, time.January, 15, 12, 0, 0, 0, time.UTC)
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.Add(slog.Time("timestamp", testTime))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "timestamp" && a.Value.Kind() == slog.KindTime {
			if !a.Value.Time().Equal(testTime) {
				t.Errorf("expected time %v, got %v", testTime, a.Value.Time())
			}
			found = true
		}
		return true
	})

	if !found {
		t.Error("time attribute not found")
	}
}

// TestHandle_ValueKind_Group tests Group Value kind conversion
func TestHandle_ValueKind_Group(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.Add(slog.Group("parent",
		slog.String("child1", "value1"),
		slog.Int("child2", 42),
	))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "parent" && a.Value.Kind() == slog.KindGroup {
			found = true
			childCount := 0
			for _, child := range a.Value.Group() {
				_ = child
				childCount++
			}
			if childCount != 2 {
				t.Errorf("expected 2 children in group, got %d", childCount)
			}
		}
		return true
	})

	if !found {
		t.Error("group attribute not found")
	}
}

// TestHandle_ValueKind_Any tests Any Value kind conversion
func TestHandle_ValueKind_Any(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	customType := struct {
		Name string
		Age  int
	}{
		Name: "Alice",
		Age:  30,
	}

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.Add(slog.Any("person", customType))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "person" && a.Value.Kind() == slog.KindAny {
			found = true
		}
		return true
	})

	if !found {
		t.Error("Any attribute not found")
	}
}

// TestHandle_NestedAttrs tests nested attribute handling
func TestHandle_NestedAttrs(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.Add(
		slog.String("level1.key1", "value1"),
		slog.String("level1.key2", "value2"),
		slog.String("level2.key", "value3"),
	)

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	attrCount := 0
	mock.records[0].Attrs(func(a slog.Attr) bool {
		attrCount++
		return true
	})

	if attrCount != 3 {
		t.Errorf("expected 3 attrs, got %d", attrCount)
	}
}

// TestHandle_EmptyAttrs tests handling of empty attributes
func TestHandle_EmptyAttrs(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	// Call Handle with no attributes

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	attrCount := 0
	mock.records[0].Attrs(func(a slog.Attr) bool {
		attrCount++
		return true
	})

	if attrCount != 0 {
		t.Errorf("expected 0 attrs, got %d", attrCount)
	}
}

// TestHandle_MultipleValueKinds tests multiple Value kinds in one Record
func TestHandle_MultipleValueKinds(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.Add(
		slog.String("string", "value"),
		slog.Int64("int64", 123),
		slog.Float64("float64", 3.14),
		slog.Bool("bool", true),
		slog.Duration("duration", time.Second),
	)

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	expectedKinds := map[string]slog.Kind{
		"string":   slog.KindString,
		"int64":    slog.KindInt64,
		"float64":  slog.KindFloat64,
		"bool":     slog.KindBool,
		"duration": slog.KindDuration,
	}

	mock.records[0].Attrs(func(a slog.Attr) bool {
		if expectedKind, ok := expectedKinds[a.Key]; ok {
			if a.Value.Kind() != expectedKind {
				t.Errorf("attribute %s: expected kind %v, got %v",
					a.Key, expectedKind, a.Value.Kind())
			}
			delete(expectedKinds, a.Key)
		}
		return true
	})

	if len(expectedKinds) > 0 {
		t.Errorf("missing attributes: %v", expectedKinds)
	}
}

// TestHandle_ErrorValue tests handling of error values
func TestHandle_ErrorValue(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	record := slog.NewRecord(time.Now(), slog.LevelError, "test", 0)
	record.Add(slog.Any("error", errors.New("test error")))

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
			strVal := a.Value.String()
			if !strings.Contains(strVal, "test error") {
				t.Errorf("expected error message to contain 'test error', got %s", strVal)
			}
		}
		return true
	})

	if !found {
		t.Error("error attribute not found")
	}
}

// TestHandle_LargeRecord tests handling of records with many attributes
func TestHandle_LargeRecord(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)

	// Add 100 attributes
	for i := range 100 {
		record.Add(slog.String(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i)))
	}

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	attrCount := 0
	mock.records[0].Attrs(func(a slog.Attr) bool {
		attrCount++
		return true
	})

	if attrCount != 100 {
		t.Errorf("expected 100 attrs, got %d", attrCount)
	}
}

// TestHandle_CloneSafety tests that Record cloning works correctly
func TestHandle_CloneSafety(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.Add(slog.String("key", "value"))

	// Handle the same record multiple times
	for i := range 3 {
		err := slogHandler.Handle(context.Background(), record)
		if err != nil {
			t.Fatalf("Handle %d failed: %v", i, err)
		}
	}

	if len(mock.records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(mock.records))
	}

	// Verify all records have the same attributes
	for i, r := range mock.records {
		attrCount := 0
		r.Attrs(func(a slog.Attr) bool {
			attrCount++
			return true
		})
		if attrCount != 1 {
			t.Errorf("record %d: expected 1 attr, got %d", i, attrCount)
		}
	}
}

// TestHandle_AllLevels tests handling Records at all slog levels
func TestHandle_AllLevels(t *testing.T) {
	mock := &mockHandler{}
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(mock)))

	levels := []slog.Level{
		slog.LevelDebug,
		slog.LevelInfo,
		slog.LevelWarn,
		slog.LevelError,
		-8, // custom level
	}

	for i, level := range levels {
		record := slog.NewRecord(time.Now(), level, "test", 0)
		err := slogHandler.Handle(context.Background(), record)
		if err != nil {
			t.Fatalf("Handle with level %v failed: %v", level, err)
		}

		if i != len(mock.records)-1 {
			t.Errorf("expected %d records after handling level %v, got %d",
				i+1, level, len(mock.records))
		}
	}

	if len(mock.records) != len(levels) {
		t.Fatalf("expected %d records, got %d", len(levels), len(mock.records))
	}
}
