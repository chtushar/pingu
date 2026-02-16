package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type File struct{}

func (f *File) Name() string        { return "file" }
func (f *File) Description() string { return "Read or write files" }

func (f *File) InputSchema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"read", "write"},
				"description": "Operation to perform",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File path",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "File content for write; empty string for read",
			},
		},
		"required":             []string{"action", "path", "content"},
		"additionalProperties": false,
	}
}

func (f *File) Execute(ctx context.Context, input string) (string, error) {
	var args struct {
		Action  string `json:"action"`
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		return "", fmt.Errorf("parsing file input: %w", err)
	}

	switch args.Action {
	case "read":
		data, err := os.ReadFile(args.Path)
		if err != nil {
			return "", fmt.Errorf("reading file: %w", err)
		}
		return truncate(data), nil

	case "write":
		if err := os.MkdirAll(filepath.Dir(args.Path), 0755); err != nil {
			return "", fmt.Errorf("creating parent dirs: %w", err)
		}
		content := []byte(args.Content)
		if err := os.WriteFile(args.Path, content, 0644); err != nil {
			return "", fmt.Errorf("writing file: %w", err)
		}
		return fmt.Sprintf("wrote %d bytes to %s", len(content), args.Path), nil

	default:
		return "", fmt.Errorf("unknown action: %s", args.Action)
	}
}
