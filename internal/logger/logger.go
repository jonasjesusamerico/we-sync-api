package logger

import (
	"log/slog"
	"os"

	"github.com/jonasjesusamerico/we-sync-api/configs"
)

func New(loggerProperties configs.LoggerProperties) *slog.Logger {
	handler := slog.NewJSONHandler(
		os.Stdout,
		&slog.HandlerOptions{
			Level: slog.Level(loggerProperties.Level),
		},
	)

	return slog.New(handler)
}
