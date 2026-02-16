package tools

import "context"

type Web struct{}

func (w *Web) Name() string        { return "web" }
func (w *Web) Description() string { return "Fetch content from a URL" }
func (w *Web) InputSchema() any    { return nil }

func (w *Web) Execute(ctx context.Context, input string) (string, error) {
	return "", nil
}
