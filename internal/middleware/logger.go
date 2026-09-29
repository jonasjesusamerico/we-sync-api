package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
)

// responseWriter wraps http.ResponseWriter to capture the status code written
// by the handler so it can be logged after the response is sent.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures a 200 implicitly set by the first call to Write (when
// WriteHeader was never called explicitly).
func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	return rw.ResponseWriter.Write(b)
}

// Logging returns a middleware that:
//   - Assigns a unique request_id to every request.
//   - Extracts the OTel trace_id and span_id from the active span (set by
//     otelhttp upstream) and injects them into the structured logger so that
//     every log line emitted during the request is automatically correlated
//     with the corresponding distributed trace.
//   - Logs a summary line after the handler returns with status and duration.
func Logging(baseLogger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			requestID := uuid.NewString()

			// Build the per-request logger with fixed fields.
			fields := []any{
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
			}

			// Correlate with the OTel span created by otelhttp (which runs
			// before this middleware). When OTel is disabled or no span is
			// active the span context is zero-valued and IsValid() returns
			// false — the trace fields are simply omitted.
			if sc := trace.SpanFromContext(r.Context()).SpanContext(); sc.IsValid() {
				fields = append(fields,
					"trace_id", sc.TraceID().String(),
					"span_id", sc.SpanID().String(),
				)
			}

			requestLogger := baseLogger.With(fields...)
			ctx := logger.WithLogger(r.Context(), requestLogger)

			rw := &responseWriter{ResponseWriter: w}
			next.ServeHTTP(rw, r.WithContext(ctx))

			status := rw.status
			if status == 0 {
				status = http.StatusOK
			}

			requestLogger.Info("request completed",
				"status", status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}
