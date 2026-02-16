package tools

import "context"

type Shell struct{}

func (s *Shell) Name() string        { return "shell" }
func (s *Shell) Description() string { return "Execute a shell command" }
func (s *Shell) InputSchema() any    { return nil }

func (s *Shell) Execute(ctx context.Context, input string) (string, error) {
	return "", nil
}
