// Package agent loads and validates agent directories. An agent root is a
// directory containing a readable, non-empty instructions.md; everything
// else is optional.
package agent

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chtushar/pingu/internal/config"
)

// MaxInstructionsBytes bounds the instructions file.
const MaxInstructionsBytes = 256 * 1024

// InstructionsFile is the only required file in an agent directory.
const InstructionsFile = "instructions.md"

// Agent is a loaded agent directory.
type Agent struct {
	Root         string // absolute, symlink-free path to the agent root
	Instructions string
	Config       config.Config
}

// Load validates the directory at path and resolves its configuration.
func Load(path string) (*Agent, error) {
	root, err := filepath.Abs(path)
	if err != nil {
		return nil, &config.ConfigError{File: path, Err: fmt.Errorf("resolve path: %w", err)}
	}

	instrPath := filepath.Join(root, InstructionsFile)
	info, err := os.Stat(instrPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, &config.ConfigError{File: InstructionsFile, Err: fmt.Errorf("not found in %s", root)}
		}
		return nil, &config.ConfigError{File: InstructionsFile, Err: err}
	}
	if !info.Mode().IsRegular() {
		return nil, &config.ConfigError{File: InstructionsFile, Err: errors.New("not a regular file")}
	}
	if info.Size() > MaxInstructionsBytes {
		return nil, &config.ConfigError{File: InstructionsFile, Err: fmt.Errorf("size %d exceeds limit %d", info.Size(), MaxInstructionsBytes)}
	}

	data, err := os.ReadFile(instrPath)
	if err != nil {
		return nil, &config.ConfigError{File: InstructionsFile, Err: err}
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, &config.ConfigError{File: InstructionsFile, Err: errors.New("file is empty")}
	}

	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}

	return &Agent{
		Root:         root,
		Instructions: strings.TrimRight(string(data), "\n"),
		Config:       cfg,
	}, nil
}
