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
	Agents   map[string]*AgentConfig      `toml:"agent"`
	DB       DBConfig                     `toml:"db"`
	Services ServicesConfig               `toml:"services"`
	Memory   MemoryConfig                 `toml:"memory"`
}

type MemoryConfig struct {
	Embedding    EmbeddingConfig  `toml:"embedding"`
	VectorWeight float32          `toml:"vector_weight"`
	FTSWeight    float32          `toml:"fts_weight"`
	AutoInject   bool             `toml:"auto_inject"`
	AutoSave     bool             `toml:"auto_save"`
	MaxResults   int              `toml:"max_results"`
	Compaction   CompactionConfig `toml:"compaction"`
}

type EmbeddingConfig struct {
	Enabled    bool   `toml:"enabled"`
	LLM        string `toml:"llm"`
	Model      string `toml:"model"`
	Dimensions int    `toml:"dimensions"`
	CacheSize  int    `toml:"cache_size"`
}

type CompactionConfig struct {
	Enabled       bool `toml:"enabled"`
	TurnThreshold int  `toml:"turn_threshold"`
	KeepRecent    int  `toml:"keep_recent"`
}

type ServicesConfig struct {
	Brave BraveConfig `toml:"brave"`
}

type BraveConfig struct {
	APIKey string `toml:"api_key"`
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

type AgentConfig struct {
	SystemPrompt string   `toml:"system_prompt"`
	Tools        []string `toml:"tools"`
}

type DBConfig struct {
	Path string `toml:"path"`
}

func Load() (*Config, error) {
	cfg := &Config{
		DefaultLLM: "openai",
		LLMs: map[string]*LLMConfig{
			"openai": {
				Model: "gpt-4.1-nano",
			},
		},
		Gateway: GatewayConfig{
			Addr: ":8484",
		},
		DB: DBConfig{
			Path: defaultDBPath(),
		},
		Memory: MemoryConfig{
			VectorWeight: 0.7,
			FTSWeight:    0.3,
			AutoInject:   true,
			AutoSave:     true,
			MaxResults:   5,
			Embedding: EmbeddingConfig{
				CacheSize: 10000,
			},
			Compaction: CompactionConfig{
				TurnThreshold: 20,
				KeepRecent:    5,
			},
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
	dir, _ := os.UserHomeDir()
	return filepath.Join(dir, ".config", "pingu", "config.toml")
}

func defaultDBPath() string {
	dir, _ := os.UserHomeDir()
	return filepath.Join(dir, ".local", "share", "pingu", "pingu.db")
}
