package islog

import (
	"log/slog"
	"net/http"
	"time"
)

// ExampleLogger_httpMiddleware demonstrates HTTP middleware integration
// that captures request metadata including request ID, method, path,
// duration, and status code using per-request logger configuration.
func ExampleLogger_httpMiddleware() {
	// Create handler (in real app, this is your application's ServeHTTP)
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, World!"))
	})

	// Wrap with requestLogger middleware
	server := requestLogger(httpHandler)
	http.ListenAndServe(":8080", server)
}

// requestLogger middleware adds request metadata to all logs
func requestLogger(next http.Handler) http.Handler {
	// Create base logger with slog handler
	handler := slog.NewJSONHandler(nil /* os.Stdout */, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := L.New(L.WithSlogHandler(handler))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate request ID (in real app, use proper ID generator)
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		// Log request start using fluent builder
		logger.Info().
			Str("request_id", requestID).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Str("user_agent", r.UserAgent()).
			Log("request started")

		// Process request
		next.ServeHTTP(w, r)

		// Log request completion
		duration := time.Since(start)

		// Log request completion using fluent builder
		logger.Info().
			Str("request_id", requestID).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("duration_ms", int(duration.Milliseconds())).
			// In real app, capture status code via response wrapper
			Int("status_code", 200).
			Log("request completed")
	})
}

// generateRequestID creates a simple unique request ID
// In production, use a proper ID generator (UUID, nanoid, etc.)
func generateRequestID() string {
	return "req-" + time.Now().Format("20060102150405.999")
}
