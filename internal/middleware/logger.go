package middleware

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
)

func Logging(
	baseLogger *slog.Logger,
) func(http.Handler) http.Handler {

	return func(next http.Handler) http.Handler {

		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {

				requestID := uuid.NewString()

				requestLogger := baseLogger.With(
					"request_id", requestID,
					"method", r.Method,
					"path", r.URL.Path,
				)

				ctx := logger.WithLogger(
					r.Context(),
					requestLogger,
				)

				next.ServeHTTP(
					w,
					r.WithContext(ctx),
				)
			},
		)
	}
}
