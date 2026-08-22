package agent_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chtushar/pingu/internal/agent"
	"github.com/chtushar/pingu/internal/config"
)

func TestLoad_Minimal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "instructions.md"), []byte("# Identity\n\nBe brief."), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := agent.Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !filepath.IsAbs(a.Root) {
		t.Errorf("root %q is not absolute", a.Root)
	}
	if a.Instructions == "" {
		t.Error("instructions empty")
	}
	if a.Config.Model.String() != config.DefaultModel {
		t.Errorf("model = %q", a.Config.Model.String())
	}
}

func TestLoad_MissingInstructions(t *testing.T) {
	_, err := agent.Load(t.TempDir())
	if err == nil {
		t.Fatal("expected error")
	}
	var cfgErr *config.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected ConfigError, got %T", err)
	}
	if !strings.Contains(err.Error(), "instructions.md") {
		t.Errorf("error should mention instructions.md: %v", err)
	}
}

func TestLoad_EmptyInstructions(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "instructions.md"), []byte("   \n\n"), 0o644)
	_, err := agent.Load(dir)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty error, got %v", err)
	}
}

func TestLoad_OversizeInstructions(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("a", agent.MaxInstructionsBytes+1)
	os.WriteFile(filepath.Join(dir, "instructions.md"), []byte(big), 0o644)
	_, err := agent.Load(dir)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected size error, got %v", err)
	}
}

func TestLoad_ConfigErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "instructions.md"), []byte("hi"), 0o644)
	os.WriteFile(filepath.Join(dir, "agent.toml"), []byte("bogus = 1\n"), 0o644)
	_, err := agent.Load(dir)
	var cfgErr *config.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected ConfigError, got %v", err)
	}
}

func TestLoad_ExampleAgent(t *testing.T) {
	a, err := agent.Load("../../examples/hello-agent")
	if err != nil {
		t.Fatalf("example agent must load: %v", err)
	}
	if !strings.Contains(a.Instructions, "hello-agent") {
		t.Errorf("unexpected instructions: %q", a.Instructions)
	}
}
