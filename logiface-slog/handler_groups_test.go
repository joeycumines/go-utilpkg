package slog

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
)

// now returns a consistent time for tests
func now() time.Time {
	return time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
}

// mockHandler is a test handler that captures records
type groupMockHandler struct {
	records []slog.Record
}

func (m *groupMockHandler) Handle(ctx context.Context, r slog.Record) error {
	rec := r.Clone()
	m.records = append(m.records, rec)
	return nil
}

func (m *groupMockHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (m *groupMockHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return m
}

func (m *groupMockHandler) WithGroup(name string) slog.Handler {
	return m
}

// TestGroup_SingleGroup tests single WithGroup call
func TestGroup_SingleGroup(t *testing.T) {
	mock := &groupMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	// Create handler with group
	handlerWithGroup := slogHandler.WithGroup("parent")

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	record.Add(slog.String("child", "value"))

	err := handlerWithGroup.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	// Check that attribute has parent.prefix
	var foundWithGroup bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "parent.child" {
			foundWithGroup = true
		}
		return true
	})

	if !foundWithGroup {
		t.Error("expected attribute to have parent. prefix")
	}
}

// TestGroup_NestedGroups tests nested WithGroup calls
func TestGroup_NestedGroups(t *testing.T) {
	mock := &groupMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	// Create handler with nested groups
	handlerWithGroup1 := slogHandler.WithGroup("level1")
	handlerWithGroup2 := handlerWithGroup1.WithGroup("level2")
	handlerWithGroup3 := handlerWithGroup2.WithGroup("level3")

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	record.Add(slog.String("key", "value"))

	err := handlerWithGroup3.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	var foundNestedGroup bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "level1.level2.level3.key" {
			foundNestedGroup = true
		}
		return true
	})

	if !foundNestedGroup {
		t.Error("expected attribute to have nested level1.level2.level3. prefix")
	}
}

// TestGroup_MixedGroupsWithAttrs tests mixing WithAttrs and WithGroup calls
func TestGroup_MixedGroupsWithAttrs(t *testing.T) {
	mock := &groupMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	// Create handler with mixed groups and attributes
	handler1 := slogHandler.WithGroup("group1")
	handler2 := handler1.WithAttrs([]slog.Attr{slog.String("attr1", "value1")})
	handler3 := handler2.WithGroup("group2")

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	record.Add(slog.String("attr2", "value2"))

	err := handler3.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	// handler3 has groups: ["group1", "group2"], preAttrs: [{attr1}]
	// During Handle, both attr1 (preAttrs) and attr2 (record attrs)
	// get prefix "group1.group2."
	foundAttr1 := false
	foundAttr2 := false
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "group1.group2.attr1" {
			foundAttr1 = true
		}
		if a.Key == "group1.group2.attr2" {
			foundAttr2 = true
		}
		return true
	})

	if !foundAttr1 {
		t.Error("expected attr1 to exist (checking for group1.group2.attr1)")
	}
	if !foundAttr2 {
		t.Error("expected attr2 to exist (checking for group1.group2.attr2)")
	}
}

// TestGroup_KeyPrefixInjection tests that group prefix is correctly injected into attributes
func TestGroup_KeyPrefixInjection(t *testing.T) {
	mock := &groupMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	handlerWithGroup := slogHandler.WithGroup("http")

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	record.Add(
		slog.String("method", "GET"),
		slog.String("path", "/api/users"),
		slog.Int("status", 200),
	)

	err := handlerWithGroup.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	expectedKeys := map[string]bool{
		"http.method": false,
		"http.path":   false,
		"http.status": false,
	}

	mock.records[0].Attrs(func(a slog.Attr) bool {
		if _, ok := expectedKeys[a.Key]; ok {
			expectedKeys[a.Key] = true
		}
		return true
	})

	for key, found := range expectedKeys {
		if !found {
			t.Errorf("expected key %s not found", key)
		}
	}
}

// TestGroup_EmptyGroupName tests handling of group with empty name
func TestGroup_EmptyGroupName(t *testing.T) {
	mock := &groupMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	// Create handler with empty group name
	handlerWithGroup := slogHandler.WithGroup("")

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	record.Add(slog.String("key", "value"))

	err := handlerWithGroup.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	// Empty group should add no prefix
	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "key" {
			found = true
		}
		return true
	})

	if !found {
		t.Error("expected attribute key to be 'key' (no prefix)")
	}
}

// TestGroup_SpecialCharactersInGroupName tests group names with special characters
func TestGroup_SpecialCharactersInGroupName(t *testing.T) {
	mock := &groupMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	// Create handler with special characters in group name
	handlerWithGroup := slogHandler.WithGroup("http.request")

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	record.Add(slog.String("method", "GET"))

	err := handlerWithGroup.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "http.request.method" {
			found = true
		}
		return true
	})

	if !found {
		t.Error("expected attribute key to have 'http.request.' prefix")
	}
}

// TestGroup_IndependentGroupHandlers tests that multiple group handlers are independent
func TestGroup_IndependentGroupHandlers(t *testing.T) {
	mock := &groupMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	// Create two independent handlers with different groups
	handlerGroupA := slogHandler.WithGroup("groupA")
	handlerGroupB := slogHandler.WithGroup("groupB")

	recordA := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	recordA.Add(slog.String("key", "valueA"))

	recordB := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	recordB.Add(slog.String("key", "valueB"))

	err := handlerGroupA.Handle(context.Background(), recordA)
	if err != nil {
		t.Fatalf("Handle groupA failed: %v", err)
	}

	err = handlerGroupB.Handle(context.Background(), recordB)
	if err != nil {
		t.Fatalf("Handle groupB failed: %v", err)
	}

	if len(mock.records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(mock.records))
	}

	// Check first record
	var foundA bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "groupA.key" {
			foundA = true
		}
		return true
	})

	// Check second record
	var foundB bool
	mock.records[1].Attrs(func(a slog.Attr) bool {
		if a.Key == "groupB.key" {
			foundB = true
		}
		return true
	})

	if !foundA {
		t.Error("expected first record to have 'groupA.key' prefix")
	}
	if !foundB {
		t.Error("expected second record to have 'groupB.key' prefix")
	}
}

// TestGroup_SlogValueGroup tests slog.Group Value kind with group prefix
func TestGroup_SlogValueGroup(t *testing.T) {
	mock := &groupMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	handlerWithGroup := slogHandler.WithGroup("outer")

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	record.Add(slog.Group("inner",
		slog.String("key1", "value1"),
		slog.String("key2", "value2"),
	))

	err := handlerWithGroup.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	// slog.Group creates nested attribute, which should have group prefix added
	var foundGroup bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "outer.inner" && a.Value.Kind() == slog.KindGroup {
			foundGroup = true
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

	if !foundGroup {
		t.Error("expected to find 'outer.inner' group attribute")
	}
}

// TestGroup_NoGroup tests behavior with no WithGroup calls
func TestGroup_NoGroup(t *testing.T) {
	mock := &groupMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	// No WithGroup calls
	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)
	record.Add(slog.String("key", "value"))

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	var found bool
	mock.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "key" {
			found = true
		}
		return true
	})

	if !found {
		t.Error("expected attribute key to be 'key' (no prefix)")
	}
}
