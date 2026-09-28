package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
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

func Logging(
	baseLogger *slog.Logger,
) func(http.Handler) http.Handler {

	return func(next http.Handler) http.Handler {

		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {

				start := time.Now()
				requestID := uuid.NewString()

				requestLogger := baseLogger.With(
					"request_id", requestID,
					"method", r.Method,
					"path", r.URL.Path,
				)

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
			},
		)
	}
}
