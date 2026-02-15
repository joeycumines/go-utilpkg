package slog

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
)

// benchmarkLogger is a helper that creates a test Logger
func benchmarkLogger() *logiface.Logger[*Event] {
	var buf stringBuf
	handler := slog.NewTextHandler(&buf, nil)
	return logiface.New[*Event](NewLogger(handler, WithLevel(logiface.LevelTrace)))
}

// stringBuf is a simple string buffer for benchmarking
type stringBuf struct {
	str string
}

func (b *stringBuf) Write(p []byte) (n int, err error) {
	b.str = string(p)
	return len(p), nil
}

func (b *stringBuf) Reset() {
	b.str = ""
}

// BenchmarkLogger_Debug_String benchmarks Debug() with string field
func BenchmarkLogger_Debug_String(b *testing.B) {
	logger := benchmarkLogger()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.Debug().Str("key", "value").Log("debug message")
	}
}

// BenchmarkLogger_WithoutFields benchmarks Debug() without fields
func BenchmarkLogger_WithoutFields(b *testing.B) {
	logger := benchmarkLogger()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.Debug().Log("debug message")
	}
}

// BenchmarkLogger_AllLevels benchmarks logging at all levels
func BenchmarkLogger_AllLevels(b *testing.B) {
	logger := benchmarkLogger()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.Trace().Log("trace")
		logger.Debug().Log("debug")
		logger.Info().Log("info")
		logger.Notice().Log("notice")
		logger.Warning().Log("warning")
		logger.Err().Log("error")
	}
}

// BenchmarkLogger_MultipleFields benchmarks adding multiple fields
func BenchmarkLogger_MultipleFields(b *testing.B) {
	logger := benchmarkLogger()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.Info().
			Str("str", "value").
			Int64("int64", 123456789012).
			Float64("float", 3.14159).
			Bool("bool", true).
			Log("message")
	}
}

// BenchmarkLogger_AllMethods benchmarks with many field types
func BenchmarkLogger_AllMethods(b *testing.B) {
	logger := benchmarkLogger()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.Info().
			Str("string", "value").
			Int("int", 42).
			Int64("int64", 123456789012).
			Uint64("uint", 42).
			Uint64("uint64", 123456789012).
			Float32("float32", 3.14).
			Float64("float64", 3.14159).
			Bool("bool", true).
			Log("message")
	}
}

// BenchmarkEvent_String benchmarks adding a single string field to Event
func BenchmarkEvent_String(b *testing.B) {
	logger := benchmarkLogger()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.Info().Str("key", "value").Log("message")
	}
}

// BenchmarkEvent_Int benchmarks adding int field
func BenchmarkEvent_Int(b *testing.B) {
	logger := benchmarkLogger()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.Info().Int("key", 42).Log("message")
	}
}

// BenchmarkEvent_Float64 benchmarks adding float64 field
func BenchmarkEvent_Float64(b *testing.B) {
	logger := benchmarkLogger()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.Info().Float64("pi", 3.14159).Log("message")
	}
}

// BenchmarkEvent_Bool benchmarks adding bool field
func BenchmarkEvent_Bool(b *testing.B) {
	logger := benchmarkLogger()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.Info().Bool("enabled", true).Log("message")
	}
}

// BenchmarkLevel_ToSlogLevel benchmarks level conversion to slog
func BenchmarkLevel_ToSlogLevel(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = toSlogLevel(logiface.LevelInformational)
	}
}

// BenchmarkLevel_ToLogifaceLevel benchmarks level conversion to logiface
func BenchmarkLevel_ToLogifaceLevel(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = toLogifaceLevel(slog.LevelInfo)
	}
}

// BenchmarkLevel_AllLevels benchmarks all level conversions
func BenchmarkLevel_AllLevels(b *testing.B) {
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

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for _, lvl := range levels {
			_ = toSlogLevel(lvl)
		}
	}
}

// BenchmarkHandler_Enabled benchmarks Enabled() check
func BenchmarkHandler_Enabled(b *testing.B) {
	var buf stringBuf
	handler := slog.NewTextHandler(&buf, nil)
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(handler)))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = slogHandler.Enabled(context.Background(), slog.LevelInfo)
	}
}

// BenchmarkHandler_Handle benchmarks Handle() with simple Record
func BenchmarkHandler_Handle(b *testing.B) {
	var buf stringBuf
	handler := slog.NewTextHandler(&buf, nil)
	slogHandler := NewSlogHandler(logiface.New[*Event](NewLogger(handler)))

	// Create a simple record
	record := slog.NewRecord(someTime, slog.LevelInfo, "message", 0)
	record.Add(slog.String("key", "value"))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r := record.Clone()
		_ = slogHandler.Handle(context.Background(), r)
	}
}

// BenchmarkHandler_WithAttrs benchmarks creating Handler WithAttrs
func BenchmarkHandler_WithAttrs(b *testing.B) {
	var buf stringBuf
	handler := slog.NewTextHandler(&buf, nil)

	attrs := []slog.Attr{
		slog.String("pre1", "value1"),
		slog.Int("pre2", 42),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = handler.WithAttrs(attrs)
	}
}

// BenchmarkHandler_WithGroup benchmarks creating Handler WithGroup
func BenchmarkHandler_WithGroup(b *testing.B) {
	var buf stringBuf
	handler := slog.NewTextHandler(&buf, nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = handler.WithGroup("group1")
	}
}

var someTime = time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
