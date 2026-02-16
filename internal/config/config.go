package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	LLM      LLMConfig      `toml:"llm"`
	Gateway  GatewayConfig  `toml:"gateway"`
	Telegram TelegramConfig `toml:"telegram"`
	DB       DBConfig       `toml:"db"`
}

type LLMConfig struct {
	Provider  string `toml:"provider"`
	Model     string `toml:"model"`
	APIKeyEnv string `toml:"api_key_env"`
}

type GatewayConfig struct {
	Addr  string `toml:"addr"`
	Token string `toml:"token"`
}

type TelegramConfig struct {
	Enabled     bool   `toml:"enabled"`
	BotTokenEnv string `toml:"bot_token_env"`
	WebhookURL  string `toml:"webhook_url"`
}

type DBConfig struct {
	Path string `toml:"path"`
}

func Load() (*Config, error) {
	cfg := &Config{
		LLM: LLMConfig{
			Provider:  "anthropic",
			Model:     "claude-sonnet-4-20250514",
			APIKeyEnv: "ANTHROPIC_API_KEY",
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
