// Package logging configures structured JSON logging to stderr. Secrets are
// never logged by the application; message and tool-output content stay out
// of info-level logs.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// Init installs a JSON slog handler on stderr at the level named by
// LOG_LEVEL (debug, info, warn, error; default info) and returns the
// default logger.
func Init() *slog.Logger {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: levelFromEnv(),
	})))
	return slog.Default()
}

func levelFromEnv() slog.Level {
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
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
