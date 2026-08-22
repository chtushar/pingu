package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chtushar/pingu/internal/config"

	"github.com/spf13/cobra"
)

const starterInstructions = `# Identity

You are a helpful assistant. You answer clearly and concisely, and you ask
for clarification when a request is ambiguous.

Edit this file to describe who your agent is and how it should behave. This
is the only required file in an agent directory.
`

const starterGitignore = ".pingu/\n"

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init PATH",
		Short: "Create a new agent directory",
		Long: `Create a new agent directory at PATH with a starter instructions.md
and a .gitignore for runtime state.

The directory is created only when safe: init refuses to touch a directory
that already contains files.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(args[0])
		},
	}
}

func runInit(path string) error {
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if !info.IsDir() {
			return &config.ConfigError{File: path, Err: errors.New("path exists and is not a directory")}
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return &config.ConfigError{File: path, Err: err}
		}
		if len(entries) > 0 {
			return &config.ConfigError{File: path, Err: errors.New("directory is not empty")}
		}
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(path, 0o755); err != nil {
			return &config.ConfigError{File: path, Err: err}
		}
	default:
		return &config.ConfigError{File: path, Err: err}
	}

	if err := os.WriteFile(filepath.Join(path, "instructions.md"), []byte(starterInstructions), 0o644); err != nil {
		return &config.ConfigError{File: "instructions.md", Err: err}
	}
	if err := os.WriteFile(filepath.Join(path, ".gitignore"), []byte(starterGitignore), 0o644); err != nil {
		return &config.ConfigError{File: ".gitignore", Err: err}
	}

	fmt.Fprintf(os.Stdout, "created agent at %s\n", path)
	fmt.Fprintln(os.Stdout, "run it with: pingu run "+path)
	return nil
}
