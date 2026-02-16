package tools

import (
	"context"
	"log/slog"
)

type Web struct{}

func (w *Web) Name() string        { return "web" }
func (w *Web) Description() string { return "Fetch content from a URL" }
func (w *Web) InputSchema() any    { return nil }

func (w *Web) Execute(ctx context.Context, input string) (string, error) {
	slog.Debug("web: execute called (not implemented)")
	return "", nil
}
