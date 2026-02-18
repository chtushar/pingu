package main

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"pingu/internal/agent"
	"pingu/internal/channels"
	"pingu/internal/config"
	"pingu/internal/db"
	"pingu/internal/embedding"
	"pingu/internal/gateway"
	"pingu/internal/history"
	"pingu/internal/llm"
	"pingu/internal/memory"
	"pingu/internal/tools"

	"github.com/spf13/cobra"
)

var gatewayAddr string

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Start the gateway server",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		if gatewayAddr != "" {
			cfg.Gateway.Addr = gatewayAddr
		}

		database, err := db.Open(cfg.DB.Path)
		if err != nil {
			return fmt.Errorf("opening database: %w", err)
		}
		defer database.Close()

		if err := database.Migrate(); err != nil {
			return fmt.Errorf("migrating database: %w", err)
		}

		store := history.NewStore(database)

		llmCfg, ok := cfg.LLMs[cfg.DefaultLLM]
		if !ok {
			return fmt.Errorf("default LLM %q not found in config", cfg.DefaultLLM)
		}
		provider := llm.NewOpenAI(llmCfg.BaseURL, llmCfg.APIKey, llmCfg.Model)

		// Embedding provider (optional).
		var embedProvider embedding.Provider
		if cfg.Memory.Embedding.Enabled {
			embLLM, ok := cfg.LLMs[cfg.Memory.Embedding.LLM]
			if !ok {
				return fmt.Errorf("embedding LLM %q not found in config", cfg.Memory.Embedding.LLM)
			}
			raw := embedding.NewOpenAI(embLLM.BaseURL, embLLM.APIKey, cfg.Memory.Embedding.Model, cfg.Memory.Embedding.Dimensions)
			embedProvider = embedding.NewCachedProvider(raw, database, cfg.Memory.Embedding.CacheSize)
			slog.Info("embedding provider enabled", "model", cfg.Memory.Embedding.Model, "dimensions", cfg.Memory.Embedding.Dimensions)
		}

		// Semantic memory store and hybrid searcher.
		semanticStore := memory.NewSemanticStore(database, embedProvider)
		searcher := memory.NewHybridSearcher(database, embedProvider, cfg.Memory.VectorWeight, cfg.Memory.FTSWeight)

		// Memory implementation: enhanced (auto-inject) or plain conversation.
		var mem memory.Memory
		if cfg.Memory.AutoInject {
			mem = memory.NewEnhancedMemory(store, searcher, cfg.Memory.MaxResults)
		} else {
			mem = memory.NewConversationMemory(store)
		}

		// Build global tool registry.
		registry := agent.NewRegistry()
		registry.Register(&tools.Message{})
		registry.Register(&tools.Shell{})
		registry.Register(&tools.File{})
		if cfg.Services.Brave.APIKey != "" {
			registry.Register(tools.NewWeb(cfg.Services.Brave.APIKey))
		}
		registry.Register(tools.NewMemoryStore(semanticStore))
		registry.Register(tools.NewMemoryRecall(searcher))

		// Convert config agent profiles.
		profiles := make(map[string]*agent.AgentProfile)
		for name, ac := range cfg.Agents {
			profiles[name] = &agent.AgentProfile{
				Name:         name,
				SystemPrompt: ac.SystemPrompt,
				Tools:        ac.Tools,
			}
		}

		// Create factory and delegate tool if profiles are configured.
		if len(profiles) > 0 {
			factory := agent.NewRunnerFactory(provider, store, mem, registry, profiles)
			registry.Register(tools.NewDelegate(factory))
		}

		// Runner options.
		var runnerOpts []agent.RunnerOption
		if cfg.Memory.AutoSave {
			runnerOpts = append(runnerOpts, agent.WithSemanticStore(semanticStore))
			slog.Info("memory auto-save enabled")
		}
		if cfg.Memory.Compaction.Enabled {
			compactor := memory.NewCompactor(store, database, provider, cfg.Memory.Compaction)
			runnerOpts = append(runnerOpts, agent.WithCompactor(compactor))
			slog.Info("compaction enabled", "threshold", cfg.Memory.Compaction.TurnThreshold, "keep_recent", cfg.Memory.Compaction.KeepRecent)
		}

		// Build orchestrator runner: use "orchestrator" profile if it exists, else default.
		var runner agent.Runner
		if p, ok := profiles["orchestrator"]; ok {
			if p.SystemPrompt != "" {
				runnerOpts = append(runnerOpts, agent.WithSystemPrompt(p.SystemPrompt))
			}
			orchestratorRegistry := registry.Scope(p.Tools)
			runner = agent.NewSimpleRunner(provider, store, mem, orchestratorRegistry, runnerOpts...)
		} else {
			runner = agent.NewSimpleRunner(provider, store, mem, registry, runnerOpts...)
		}

		chs := buildChannels(cfg, runner)

		// Start channel pollers in background.
		for _, ch := range chs {
			go func(c channels.Channel) {
				if err := c.Start(ctx); err != nil && ctx.Err() == nil {
					slog.Error("channel stopped", "name", c.Name(), "error", err)
				}
			}(ch)
		}

		srv := gateway.NewServer(runner, chs...)
		slog.Info("starting gateway", "addr", cfg.Gateway.Addr, "channels", len(chs))
		return srv.ListenAndServe(ctx, cfg.Gateway.Addr)
	},
}

func init() {
	gatewayCmd.Flags().StringVarP(&gatewayAddr, "addr", "a", "", "override gateway listen address")
}

func buildChannels(cfg *config.Config, runner agent.Runner) []channels.Channel {
	var chs []channels.Channel
	for name, ch := range cfg.Channels {
		if !ch.Enabled {
			continue
		}
		switch ch.Type {
		case "telegram":
			var allowedUsers []int64
			if v, ok := ch.Settings["allowed_users"]; ok {
				for _, s := range strings.Split(v, ",") {
					if id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
						allowedUsers = append(allowedUsers, id)
					}
				}
			}
			chs = append(chs, channels.NewTelegram(ch.Settings["bot_token"], allowedUsers, runner))
			slog.Info("channel registered", "name", name, "type", ch.Type)
		default:
			slog.Warn("unknown channel type", "name", name, "type", ch.Type)
		}
	}
	return chs
}
