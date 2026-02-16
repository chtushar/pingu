package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
	"pingu/internal/agent"
	"pingu/internal/channels"
	"pingu/internal/config"
	"pingu/internal/db"
	gw "pingu/internal/gateway"
	"pingu/internal/history"
	"pingu/internal/llm"
	"pingu/internal/tools"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var addr string

var Cmd = &cobra.Command{
	Use:   "gateway",
	Short: "Start the gateway server",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		if addr != "" {
			cfg.Gateway.Addr = addr
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

		registry := agent.NewRegistry()
		registry.Register(&tools.Message{})
		registry.Register(&tools.Shell{})

		runner := agent.NewSimpleRunner(provider, store, registry)

		chs := buildChannels(cfg, runner)

		// Start channel pollers in background
		for _, ch := range chs {
			go func(c channels.Channel) {
				if err := c.Start(ctx); err != nil && ctx.Err() == nil {
					slog.Error("channel stopped", "name", c.Name(), "error", err)
				}
			}(ch)
		}

		srv := gw.NewServer(runner, chs...)
		slog.Info("starting gateway", "addr", cfg.Gateway.Addr, "channels", len(chs))
		return srv.ListenAndServe(ctx, cfg.Gateway.Addr)
	},
}

func init() {
	Cmd.Flags().StringVarP(&addr, "addr", "a", "", "override gateway listen address")
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
