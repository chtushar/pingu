package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

const maxOutputBytes = 10_000

type Shell struct{}

func (s *Shell) Name() string        { return "shell" }
func (s *Shell) Description() string { return "Execute a shell command" }

func (s *Shell) InputSchema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"timeout": map[string]any{
				"type":        "number",
				"description": "Execution timeout in seconds (default 30)",
			},
		},
		"required":             []string{"command", "timeout"},
		"additionalProperties": false,
	}
}

func (s *Shell) Execute(ctx context.Context, input string) (string, error) {
	var args struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		return "", fmt.Errorf("parsing shell input: %w", err)
	}

	timeout := time.Duration(args.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "sh", "-c", args.Command).CombinedOutput()

	output := truncate(out)

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Sprintf("%s\nexit code: %d", output, exitErr.ExitCode()), nil
		}
		return "", fmt.Errorf("running command: %w", err)
	}

	return output, nil
}

func truncate(b []byte) string {
	if len(b) > maxOutputBytes {
		return string(b[:maxOutputBytes]) + "\n... (truncated)"
	}
	return string(b)
}
