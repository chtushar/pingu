package main

import (
	"context"
	"log/slog"
	"os"
	"pingu/cmd/pingu/agent"
	"pingu/cmd/pingu/gateway"
	"pingu/cmd/pingu/setup"
	"pingu/internal/config"
	"pingu/internal/logger"
	"pingu/internal/trace"

	"github.com/spf13/cobra"
)

func main() {
	logger.Init()

	ctx := context.Background()

	traceCfg := trace.Config{}
	if cfg, err := config.Load(); err == nil {
		if llmCfg, ok := cfg.LLMs[cfg.DefaultLLM]; ok {
			traceCfg.APIKey = llmCfg.APIKey
		}
	}

	shutdown, err := trace.Init(ctx, traceCfg)
	if err != nil {
		slog.Warn("failed to init tracing", "error", err)
	} else {
		defer func() {
			if err := shutdown(ctx); err != nil {
				slog.Warn("failed to shutdown tracing", "error", err)
			}
		}()
	}

	rootCmd := &cobra.Command{
		Use:   "pingu",
		Short: "Pingu is a personal AI agent",
		Run: func(cmd *cobra.Command, args []string) {
			slog.Info("Hello World")
		},
	}

	rootCmd.AddCommand(setup.Cmd)
	rootCmd.AddCommand(gateway.Cmd)
	rootCmd.AddCommand(agent.Cmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
