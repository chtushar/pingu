package main

import (
	"log/slog"
	"os"
	"pingu/cmd/pingu/agent"
	"pingu/cmd/pingu/gateway"
	"pingu/cmd/pingu/setup"
	"pingu/internal/logger"

	"github.com/spf13/cobra"
)

func main() {
	logger.Init()
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
