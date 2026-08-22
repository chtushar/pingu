// Package config resolves agent configuration with the precedence
// flags > agent.toml > PINGU_* environment > documented defaults.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// DefaultModel is the single source of truth for the default model
// reference. Override with agent.toml, PINGU_MODEL, or --model.
const DefaultModel = "openai/gpt-4o-mini"

// Config is the resolved agent configuration.
type Config struct {
	Model ModelRef
}

// ModelRef is a provider/model-id reference split on the first slash.
type ModelRef struct {
	Provider string
	Model    string
}

// String renders the reference as provider/model-id.
func (m ModelRef) String() string { return m.Provider + "/" + m.Model }

// ParseModelRef parses "provider/model-id".
func ParseModelRef(s string) (ModelRef, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return ModelRef{}, errors.New("empty model reference")
	}
	provider, model, found := strings.Cut(s, "/")
	if !found || provider == "" || model == "" {
		return ModelRef{}, fmt.Errorf("invalid model reference %q: want provider/model-id", s)
	}
	return ModelRef{Provider: provider, Model: model}, nil
}

// ConfigError marks validation and resolution failures that must exit with
// code 2 (usage/config) rather than 1 (runtime).
type ConfigError struct {
	File  string // file the error relates to, if any
	Field string // offending field, if known
	Err   error
}

func (e *ConfigError) Error() string {
	loc := e.File
	if e.Field != "" {
		if loc == "" {
			loc = e.Field
		} else {
			loc = e.Field + " in " + loc
		}
	}
	if loc == "" {
		return e.Err.Error()
	}
	return loc + ": " + e.Err.Error()
}

func (e *ConfigError) Unwrap() error { return e.Err }

// agentFile mirrors the agent.toml fields recognized in Phase 1. Unknown
// fields are rejected so typos fail early.
type agentFile struct {
	Model string `toml:"model"`
}

// Load resolves configuration for the agent rooted at root: agent.toml (if
// present), then PINGU_MODEL, then DefaultModel. Flag overrides are applied
// by the caller with ApplyModelFlag.
func Load(root string) (Config, error) {
	var cfg Config
	model := DefaultModel

	path := filepath.Join(root, "agent.toml")
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
	case errors.Is(err, os.ErrNotExist):
		// No agent.toml: defaults and environment apply.
	default:
		return cfg, &ConfigError{File: "agent.toml", Err: err}
	}

	if data != nil {
		var doc agentFile
		md, err := toml.Decode(string(data), &doc)
		if err != nil {
			return cfg, &ConfigError{File: "agent.toml", Err: err}
		}
		if keys := md.Undecoded(); len(keys) > 0 {
			return cfg, &ConfigError{File: "agent.toml", Field: keys[0].String(), Err: errors.New("unknown field")}
		}
		if doc.Model != "" {
			model = doc.Model
		}
	}

	if v := os.Getenv("PINGU_MODEL"); v != "" {
		model = v
	}

	ref, err := ParseModelRef(model)
	if err != nil {
		return cfg, &ConfigError{Field: "model", Err: err}
	}
	if ref.Provider != "openai" {
		return cfg, &ConfigError{Field: "model", Err: fmt.Errorf("unsupported provider %q (supported: openai)", ref.Provider)}
	}
	cfg.Model = ref
	return cfg, nil
}

// ApplyModelFlag applies a --model flag value, which wins over every other
// source. An empty value leaves cfg unchanged.
func (cfg *Config) ApplyModelFlag(v string) error {
	if v == "" {
		return nil
	}
	ref, err := ParseModelRef(v)
	if err != nil {
		return &ConfigError{Field: "--model", Err: err}
	}
	if ref.Provider != "openai" {
		return &ConfigError{Field: "--model", Err: fmt.Errorf("unsupported provider %q (supported: openai)", ref.Provider)}
	}
	cfg.Model = ref
	return nil
}
