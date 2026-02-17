package islog

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/joeycumines/logiface"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// traceIDKey is the context key for storing trace IDs in context
type traceIDKey string

const traceIDContextKey traceIDKey = "trace_id"

// ExampleLogger_openTelemetryTraceID demonstrates extracting trace IDs from
// OpenTelemetry context and logging them with every event.
func ExampleLogger_openTelemetryTraceID() {
	// Initialize OpenTelemetry (simplified - in real app, configure properly)
	tracer := otel.Tracer("example")
	ctx := context.Background()
	ctx, span := tracer.Start(ctx, "operation")
	defer span.End()

	// Extract trace ID from span
	spanContext := trace.SpanFromContext(ctx).SpanContext()
	traceID := spanContext.TraceID().String()
	spanID := spanContext.SpanID().String()

	// Create logger with slog handler
	handler := slog.NewJSONHandler(nil, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := L.New(L.WithSlogHandler(handler))

	// Log with trace/span IDs using fluent builder
	logger.Info().
		Str("trace_id", traceID). // OpenTelemetry trace ID
		Str("span_id", spanID).   // OpenTelemetry span ID
		Str("operation", "example").
		Log("processing operation")
}

// withTraceID adds OpenTelemetry trace/span IDs to context
// This enables automatic trace ID propagation through request handlers.
//
//lint:ignore U1000 Example code showing useful pattern for users
func withTraceID(ctx context.Context) context.Context {
	spanContext := trace.SpanFromContext(ctx).SpanContext()

	// Add trace ID to context
	traceID := spanContext.TraceID().String()
	spanID := spanContext.SpanID().String()

	return context.WithValue(ctx, traceIDContextKey, map[string]string{
		"trace_id": traceID,
		"span_id":  spanID,
	})
}

// getTraceIDs safely extracts trace/span IDs from context
func getTraceIDs(ctx context.Context) (traceID, spanID string) {
	if ids, ok := ctx.Value(traceIDContextKey).(map[string]string); ok {
		return ids["trace_id"], ids["span_id"]
	}
	return "", ""
}

// loggerWithTraceIDs adds trace/span IDs to a builder pattern
// extracted from OpenTelemetry context.
func loggerWithTraceIDs(ctx context.Context, builder *logiface.Builder[*Event]) *logiface.Builder[*Event] {
	// Add trace/span IDs to all events
	traceID, spanID := getTraceIDs(ctx)
	if traceID != "" {
		builder.Str("trace_id", traceID)
	}
	if spanID != "" {
		builder.Str("span_id", spanID)
	}

	return builder
}

// ExampleLogger_traceIDInMiddleware shows using trace IDs in HTTP middleware
// This pattern is useful for correlating logs with distributed tracing systems.
func ExampleLogger_traceIDInMiddleware() {
	// Create handler (in real app, this is your application's ServeHTTP)
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, World!"))
	})

	// Wrap with traceID middleware
	server := traceIDMiddleware(httpHandler)
	http.ListenAndServe(":8080", server)
}

// traceIDMiddleware middleware adds trace/span IDs to all logs
// Uses OpenTelemetry's context propagation to extract IDs.
func traceIDMiddleware(next http.Handler) http.Handler {
	// Create base logger with slog handler
	handler := slog.NewJSONHandler(nil, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := L.New(L.WithSlogHandler(handler))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract trace/span IDs from OpenTelemetry context
		traceID, spanID := getTraceIDs(ctx)

		// Log request with trace/span IDs using fluent builder
		builder := logger.Info()
		if traceID != "" {
			builder.Str("trace_id", traceID)
		}
		if spanID != "" {
			builder.Str("span_id", spanID)
		}
		builder.
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Log("handling request")

		// Continue processing
		next.ServeHTTP(w, r)
	})
}

// ExampleLogger_manualTraceID shows manually adding trace IDs
// when OpenTelemetry is not available.
func ExampleLogger_manualTraceID() {
	handler := slog.NewJSONHandler(nil, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := L.New(L.WithSlogHandler(handler))

	// Manually create trace ID (in real app, generate properly)
	ctx := context.Background()
	ctx = context.WithValue(ctx, traceIDContextKey, map[string]string{
		"trace_id": "4bf92f3577b34da6a3ce929d0e0e473",
		"span_id":  "00f067aa0ba902b7",
	})

	// Log with trace/span IDs using fluent builder
	loggerWithTraceIDs(ctx, logger.Info()).
		Str("operation", "manual").
		Log("processing with manual trace ID")
}
