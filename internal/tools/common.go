package tools

import (
	"os"
	"path/filepath"
	"strings"
)

const maxOutputBytes = 10_000

func truncate(b []byte) string {
	if len(b) > maxOutputBytes {
		return string(b[:maxOutputBytes]) + "\n... (truncated)"
	}
	return string(b)
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
