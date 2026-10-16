package logger

import (
	"log/slog"
	"os"

	"github.com/jonasjesusamerico/we-sync-api/internal/config"
)

func New(loggerProperties config.LoggerProperties) *slog.Logger {
	handler := slog.NewJSONHandler(
		os.Stdout,
		&slog.HandlerOptions{
			Level: slog.Level(loggerProperties.Level),
		},
	)

	return slog.New(handler)
}
