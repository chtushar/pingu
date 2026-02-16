package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	DefaultLLM string                `toml:"default_llm"`
	LLMs       map[string]*LLMConfig `toml:"llm"`
	Gateway  GatewayConfig                `toml:"gateway"`
	Channels map[string]*ChannelConfig    `toml:"channel"`
	DB       DBConfig                     `toml:"db"`
}

type LLMConfig struct {
	Model   string `toml:"model"`
	BaseURL string `toml:"base_url"`
	APIKey  string `toml:"api_key"`
}

type GatewayConfig struct {
	Addr  string `toml:"addr"`
	Token string `toml:"token"`
}

type ChannelConfig struct {
	Enabled    bool              `toml:"enabled"`
	Type       string            `toml:"type"`
	Settings   map[string]string `toml:"settings"`
}

type DBConfig struct {
	Path string `toml:"path"`
}

func Load() (*Config, error) {
	cfg := &Config{
		DefaultLLM: "anthropic",
		LLMs: map[string]*LLMConfig{
			"anthropic": {
				Model:   "claude-sonnet-4-20250514",
				BaseURL: "https://api.anthropic.com/v1",
			},
		},
		Gateway: GatewayConfig{
			Addr: ":8484",
		},
		DB: DBConfig{
			Path: defaultDBPath(),
		},
	}

	path := configPath()
	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, cfg); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func configPath() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "pingu", "config.toml")
}

func defaultDBPath() string {
	dir, _ := os.UserHomeDir()
	return filepath.Join(dir, ".local", "share", "pingu", "pingu.db")
}
