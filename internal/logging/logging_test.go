package logging_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/chtushar/pingu/internal/logging"
)

func TestInitLevelFromEnv(t *testing.T) {
	tests := []struct {
		env  string
		want slog.Level
	}{
		{"", slog.LevelInfo},
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"bogus", slog.LevelInfo},
	}
	for _, tt := range tests {
		t.Setenv("LOG_LEVEL", tt.env)
		logger := logging.Init()
		if !logger.Enabled(nil, tt.want) {
			t.Errorf("LOG_LEVEL=%q: level %v not enabled", tt.env, tt.want)
		}
		if logger.Enabled(nil, tt.want-1) {
			t.Errorf("LOG_LEVEL=%q: level below %v enabled", tt.env, tt.want)
		}
	}
	_ = os.Stderr // keep os import for symmetry with future assertions
}
