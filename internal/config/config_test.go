package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/chtushar/pingu/internal/config"
)

func writeAgentToml(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "agent.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Model.String() != config.DefaultModel {
		t.Errorf("model = %q, want %q", cfg.Model.String(), config.DefaultModel)
	}
}

func TestLoad_TomlOverride(t *testing.T) {
	dir := t.TempDir()
	writeAgentToml(t, dir, "model = \"openai/gpt-test\"\n")
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Model.Model != "gpt-test" {
		t.Errorf("model = %q", cfg.Model.Model)
	}
}

func TestLoad_UnknownField(t *testing.T) {
	dir := t.TempDir()
	writeAgentToml(t, dir, "model = \"openai/gpt-test\"\nmodle = \"typo\"\n")
	_, err := config.Load(dir)
	if err == nil {
		t.Fatal("expected unknown field error")
	}
	var cfgErr *config.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected ConfigError, got %T", err)
	}
	if cfgErr.Field != "modle" {
		t.Errorf("field = %q", cfgErr.Field)
	}
}

func TestLoad_EnvOverridesToml(t *testing.T) {
	dir := t.TempDir()
	writeAgentToml(t, dir, "model = \"openai/from-toml\"\n")
	t.Setenv("PINGU_MODEL", "openai/from-env")
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Model.Model != "from-env" {
		t.Errorf("model = %q, want from-env", cfg.Model.Model)
	}
}

func TestLoad_FlagOverridesAll(t *testing.T) {
	dir := t.TempDir()
	writeAgentToml(t, dir, "model = \"openai/from-toml\"\n")
	t.Setenv("PINGU_MODEL", "openai/from-env")
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ApplyModelFlag("openai/from-flag"); err != nil {
		t.Fatal(err)
	}
	if cfg.Model.Model != "from-flag" {
		t.Errorf("model = %q, want from-flag", cfg.Model.Model)
	}
}

func TestLoad_UnsupportedProvider(t *testing.T) {
	dir := t.TempDir()
	writeAgentToml(t, dir, "model = \"anthropic/claude\"\n")
	_, err := config.Load(dir)
	var cfgErr *config.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected ConfigError, got %v", err)
	}
}

func TestParseModelRef(t *testing.T) {
	tests := []struct {
		in       string
		provider string
		model    string
		wantErr  bool
	}{
		{in: "openai/gpt-4o", provider: "openai", model: "gpt-4o"},
		{in: "openai/o/m/nested", provider: "openai", model: "o/m/nested"}, // split on first slash only
		{in: "", wantErr: true},
		{in: "noprovider", wantErr: true},
		{in: "/model", wantErr: true},
		{in: "provider/", wantErr: true},
	}
	for _, tt := range tests {
		ref, err := config.ParseModelRef(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseModelRef(%q): expected error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseModelRef(%q): %v", tt.in, err)
			continue
		}
		if ref.Provider != tt.provider || ref.Model != tt.model {
			t.Errorf("ParseModelRef(%q) = %q, %q", tt.in, ref.Provider, ref.Model)
		}
	}
}

func TestLimitsEnv(t *testing.T) {
	t.Setenv("PINGU_MAX_MODEL_TURNS", "7")
	t.Setenv("PINGU_RUN_TIMEOUT", "3m")
	t.Setenv("PINGU_MAX_TOOL_OUTPUT_BYTES", "2048")
	l, err := config.DefaultLimits.ApplyEnv()
	if err != nil {
		t.Fatal(err)
	}
	if l.MaxModelTurns != 7 || l.RunTimeout.String() != "3m0s" || l.MaxToolOutputBytes != 2048 {
		t.Errorf("limits = %+v", l)
	}
}

func TestLimitsEnvInvalid(t *testing.T) {
	t.Setenv("PINGU_MAX_MODEL_TURNS", "zero")
	if _, err := config.DefaultLimits.ApplyEnv(); err == nil {
		t.Fatal("expected error")
	}
}
