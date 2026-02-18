package main

import (
	"context"
	"log/slog"
	"os"

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

	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(gatewayCmd)
	rootCmd.AddCommand(agentCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
