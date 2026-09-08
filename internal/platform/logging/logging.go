// Package logging configures the process-wide structured logger.
package logging

import (
	"log/slog"
	"os"
	"strings"

	"github.com/syabanf/nuhabit-backend/internal/platform/config"
)

// Setup installs the default logger. JSON in production so logs are
// queryable; text locally so they are readable.
func Setup(cfg config.Log) {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}

	var handler slog.Handler
	if strings.EqualFold(cfg.Format, "text") {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
