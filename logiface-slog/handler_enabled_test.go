package slog

import (
	"context"
	"log/slog"
	"testing"

	"github.com/joeycumines/logiface"
)

// TestSlogHandler_Enabled_NoWriter tests Enabled when logger has no writer
func TestSlogHandler_Enabled_NoWriter(t *testing.T) {
	// Create logger without a writer - this makes logger.Enabled() return false
	logger := logiface.New[*Event]()

	// Wrap with SlogHandler
	slogHandler := NewSlogHandler(logger)

	ctx := context.Background()

	// When logger has no writer, Enabled returns false for any slog level
	levels := []slog.Level{
		slog.LevelDebug,
		slog.LevelInfo,
		slog.LevelWarn,
		slog.LevelError,
		-10, // Custom level
	}

	for _, level := range levels {
		result := slogHandler.Enabled(ctx, level)
		if result {
			t.Errorf("Expected Enabled to return false when logger has no writer (level %v)", level)
		}
	}
}

// TestSlogHandler_Enabled_WithWriter_AllLogifaceLevels tests that all logiface levels return true when logger has writer
func TestSlogHandler_Enabled_WithWriter_AllLogifaceLevels(t *testing.T) {
	handler := slog.NewTextHandler(nil, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create logger with a writer (via NewLogger)
	logger := logiface.New[*Event](NewLogger(handler))

	slogHandler := NewSlogHandler(logger)
	ctx := context.Background()

	// All slog levels map to logiface levels that are not LevelDisabled,
	// and logger has a writer, so Enabled should return true for all
	// (actual filtering happens in Handle(), not Enabled())
	levels := []slog.Level{
		slog.LevelDebug, // Maps to LevelDebug (7)
		slog.LevelInfo,  // Maps to LevelInformational (6)
		slog.LevelWarn,  // Maps to LevelNotice (5)
		slog.LevelError, // Maps to LevelError (3)
		-1,              // Maps to LevelDebug (7)
		-10,             // Maps to LevelDebug (7)
		1,               // Maps to LevelDebug (7)
		2,               // Maps to LevelDebug (7)
		3,               // Maps to LevelDebug (7)
	}

	for _, level := range levels {
		result := slogHandler.Enabled(ctx, level)
		if !result {
			t.Errorf("Expected Enabled to return true for level %v (all logiface levels > LevelDisabled)", level)
		}
	}
}

// TestSlogHandler_Enabled_NoThresholdFiltering confirms Enabled doesn't filter by threshold
func TestSlogHandler_Enabled_NoThresholdFiltering(t *testing.T) {
	handler := slog.NewTextHandler(nil, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create logger with Error level (configures threshold, but Enabled ignores it)
	logger := logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelError)))

	slogHandler := NewSlogHandler(logger)
	ctx := context.Background()

	// Debug and Info are below Error threshold, but Enabled() still returns true
	// because it only checks:
	// 1. logger.Enabled() - returns true since logger has a writer
	// 2. logifaceLevel.Enabled() - returns true for all levels > -1
	// Threshold filtering happens in Handle(), not Enabled()
	result := slogHandler.Enabled(ctx, slog.LevelDebug)
	if !result {
		t.Error("Expected Enabled to return true: level filtering happens in Handle(), not Enabled()")
	}

	result = slogHandler.Enabled(ctx, slog.LevelInfo)
	if !result {
		t.Error("Expected Enabled to return true: level filtering happens in Handle(), not Enabled()")
	}

	// Error should also return true
	result = slogHandler.Enabled(ctx, slog.LevelError)
	if !result {
		t.Error("Expected Enabled to return true for Error level")
	}
}

// TestSlogHandler_Enabled_AllStandardLevels confirms all standard slog levels return true
func TestSlogHandler_Enabled_AllStandardLevels(t *testing.T) {
	handler := slog.NewTextHandler(nil, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create logger with a writer
	logger := logiface.New[*Event](NewLogger(handler))

	slogHandler := NewSlogHandler(logger)
	ctx := context.Background()

	// All slog levels map to logiface levels > LevelDisabled (-1)
	// Therefore Enabled() returns true for all
	levels := []slog.Level{
		slog.LevelDebug, // Maps to LevelDebug (7) > -1
		slog.LevelInfo,  // Maps to LevelInformational (6) > -1
		slog.LevelWarn,  // Maps to LevelNotice (5) > -1
		slog.LevelError, // Maps to LevelError (3) > -1
	}

	for _, level := range levels {
		result := slogHandler.Enabled(ctx, level)
		if !result {
			t.Errorf("Expected Enabled to return true for level %v (all > LevelDisabled)", level)
		}
	}
}

// TestSlogHandler_Enabled_DebugLevelMapping confirms slog.LevelDebug returns true
func TestSlogHandler_Enabled_DebugLevelMapping(t *testing.T) {
	handler := slog.NewTextHandler(nil, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := logiface.New[*Event](NewLogger(handler))
	slogHandler := NewSlogHandler(logger)
	ctx := context.Background()

	// slog.LevelDebug maps to logiface.LevelDebug (7)
	// LevelDebug > LevelDisabled (-1), so Enabled() returns true
	result := slogHandler.Enabled(ctx, slog.LevelDebug)
	if !result {
		t.Error("Expected Enabled to return true: slog.LevelDebug maps to LevelDebug (7) which is Enabled()")
	}
}

// TestSlogHandler_Enabled_CustomLevels confirms various custom levels return true
func TestSlogHandler_Enabled_CustomLevels(t *testing.T) {
	handler := slog.NewTextHandler(nil, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := logiface.New[*Event](NewLogger(handler))
	slogHandler := NewSlogHandler(logger)
	ctx := context.Background()

	// All levels map to logiface levels > LevelDisabled
	levels := []slog.Level{
		0,                 // Maps to LevelInformational
		4, slog.LevelWarn, // Maps to LevelNotice
		8, slog.LevelError, // Maps to LevelError
		10,              // Custom level > Error
		15,              // Custom level
		100,             // Very large custom level
		slog.LevelDebug, // Maps to LevelDebug (7)
		-1,              // Dynamic level maps to LevelDebug (7)
		-5,              // Maps to LevelDebug (7)
		-100,            // Maps to LevelDebug (7)
	}

	for _, level := range levels {
		result := slogHandler.Enabled(ctx, level)
		if !result {
			t.Errorf("Expected Enabled to return true for level %v", level)
		}
	}
}

// TestSlogHandler_Enabled_AllEmergencToTrace confirms Emergency through Trace all return true
func TestSlogHandler_Enabled_AllEmergencToTrace(t *testing.T) {
	handler := slog.NewTextHandler(nil, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := logiface.New[*Event](NewLogger(handler))
	slogHandler := NewSlogHandler(logger)
	ctx := context.Background()

	// All logiface levels from Emergency (0) through Trace (8) are > LevelDisabled (-1)
	// Therefore Enabled() returns true for all
	logifaceLevels := []struct {
		name  string
		level logiface.Level
		slogL slog.Level
	}{
		{"Emergency", logiface.LevelEmergency, slog.LevelError},
		{"Alert", logiface.LevelAlert, slog.LevelError},
		{"Critical", logiface.LevelCritical, slog.LevelError},
		{"Error", logiface.LevelError, slog.LevelError},
		{"Warning", logiface.LevelWarning, slog.LevelWarn},
		{"Notice", logiface.LevelNotice, slog.LevelWarn},
		{"Informational", logiface.LevelInformational, slog.LevelInfo},
		{"Debug", logiface.LevelDebug, slog.LevelDebug},
	}

	for _, tt := range logifaceLevels {
		result := slogHandler.Enabled(ctx, tt.slogL)
		if !result {
			t.Errorf("Expected Enabled to return true for %s level", tt.name)
		}
	}
}
