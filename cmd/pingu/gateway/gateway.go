package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
	"pingu/internal/channels"
	"pingu/internal/config"
	gw "pingu/internal/gateway"
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

		chs := buildChannels(cfg)

		// Start channel pollers in background
		for _, ch := range chs {
			go func(c channels.Channel) {
				if err := c.Start(ctx); err != nil && ctx.Err() == nil {
					slog.Error("channel stopped", "name", c.Name(), "error", err)
				}
			}(ch)
		}

		srv := gw.NewServer(nil, chs...)
		slog.Info("starting gateway", "addr", cfg.Gateway.Addr, "channels", len(chs))
		return srv.ListenAndServe(ctx, cfg.Gateway.Addr)
	},
}

func init() {
	Cmd.Flags().StringVarP(&addr, "addr", "a", "", "override gateway listen address")
}

func buildChannels(cfg *config.Config) []channels.Channel {
	var chs []channels.Channel
	for name, ch := range cfg.Channels {
		if !ch.Enabled {
			continue
		}
		switch ch.Type {
		case "telegram":
			chs = append(chs, channels.NewTelegram(ch.Settings["bot_token"], nil))
			slog.Info("channel registered", "name", name, "type", ch.Type)
		default:
			slog.Warn("unknown channel type", "name", name, "type", ch.Type)
		}
	}
	return chs
}
