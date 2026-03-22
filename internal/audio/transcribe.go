package audio

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Transcriber converts an audio WAV file to text.
type Transcriber interface {
	Transcribe(ctx context.Context, wavPath string) (string, error)
}

// WhisperCLI transcribes audio using the local whisper CLI.
type WhisperCLI struct {
	ModelPath string // e.g. "base", "small", "medium"
	ModelDir  string // optional: directory for cached models (avoids download)
}

func (w *WhisperCLI) Transcribe(ctx context.Context, wavPath string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "whisper-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	args := []string{
		wavPath,
		"--model", w.ModelPath,
		"--output_format", "txt",
		"--output_dir", tmpDir,
	}
	if w.ModelDir != "" {
		args = append(args, "--model_dir", w.ModelDir)
	}

	cmd := exec.CommandContext(ctx, "whisper", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("whisper: %s", lastErrorLine(string(out)))
	}

	// Whisper writes <basename>.txt in the output dir.
	base := strings.TrimSuffix(filepath.Base(wavPath), filepath.Ext(wavPath))
	txtPath := filepath.Join(tmpDir, base+".txt")

	data, err := os.ReadFile(txtPath)
	if err != nil {
		return "", fmt.Errorf("read whisper output: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// lastErrorLine extracts the final non-empty line from command output.
// Python tracebacks put the actual error on the last line, so this
// strips the noise while keeping the actionable message.
func lastErrorLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return output
}
