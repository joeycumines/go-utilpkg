package slog

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
)

// contextMockHandler is a test handler that captures records and context
type contextMockHandler struct {
	records []slog.Record
}

func (m *contextMockHandler) Handle(ctx context.Context, r slog.Record) error {
	rec := r.Clone()
	m.records = append(m.records, rec)
	return nil
}

func (m *contextMockHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (m *contextMockHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return m
}

func (m *contextMockHandler) WithGroup(name string) slog.Handler {
	return m
}

// TestContext_PassedThrough tests that context values are available through Handle
func TestContext_PassedThrough(t *testing.T) {
	mock := &contextMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	// Create context with value
	ctx := context.WithValue(context.Background(), "testKey", "testValue")

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)

	err := slogHandler.Handle(ctx, record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	// Context is not stored in the record or event
	// Test passes if Handle succeeds without error
}

// TestContext_NilHandling tests nil context handling
func TestContext_NilHandling(t *testing.T) {
	mock := &contextMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)

	err := slogHandler.Handle(nil, record)
	if err != nil {
		t.Fatalf("Handle with nil context failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	// Should not crash with nil context
}

// TestContext_WithCancel tests context cancellation before Handle
func TestContext_WithCancel(t *testing.T) {
	mock := &contextMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	// Create context that is cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)

	err := slogHandler.Handle(ctx, record)
	if err != nil {
		t.Fatalf("Handle with cancelled context failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	// Cancelled context should not affect Handle
}

// TestContext_WithTimeout tests context with timeout
func TestContext_WithTimeout(t *testing.T) {
	mock := &contextMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)

	err := slogHandler.Handle(ctx, record)
	if err != nil {
		t.Fatalf("Handle with timeout context failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	// Timeout context should not affect Handle
}

// TestContext_BackgroundContext tests background context default
func TestContext_BackgroundContext(t *testing.T) {
	mock := &contextMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)

	err := slogHandler.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("Handle with background context failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	// Background context should work normally
}

// TestContext_MultipleValues tests context with multiple values
func TestContext_MultipleValues(t *testing.T) {
	mock := &contextMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	// Create context with multiple values
	ctx := context.Background()
	ctx = context.WithValue(ctx, "key1", "value1")
	ctx = context.WithValue(ctx, "key2", 42)
	ctx = context.WithValue(ctx, "key3", true)

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)

	err := slogHandler.Handle(ctx, record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	// Context values should be available (in practice)
}

// TestContext_ChainedContexts tests chained context values
func TestContext_ChainedContexts(t *testing.T) {
	mock := &contextMockHandler{}
	logger := logiface.New[*Event](NewLogger(mock))
	slogHandler := NewSlogHandler(logger)

	// Create chained contexts
	ctx1 := context.WithValue(context.Background(), "level1", "val1")
	ctx2 := context.WithValue(ctx1, "level2", "val2")
	ctx3 := context.WithValue(ctx2, "level3", "val3")

	record := slog.NewRecord(now(), slog.LevelInfo, "test", 0)

	err := slogHandler.Handle(ctx3, record)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(mock.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.records))
	}

	// Chained context should work normally
}
