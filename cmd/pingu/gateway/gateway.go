package gateway

import (
	"fmt"
	"log/slog"
	"pingu/internal/channels"
	"pingu/internal/config"
	gw "pingu/internal/gateway"

	"github.com/spf13/cobra"
)

var addr string

var Cmd = &cobra.Command{
	Use:   "gateway",
	Short: "Start the gateway server",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		if addr != "" {
			cfg.Gateway.Addr = addr
		}

		chs := buildChannels(cfg)

		srv := gw.NewServer(nil, chs...)
		slog.Info("starting gateway", "addr", cfg.Gateway.Addr, "channels", len(chs))
		return srv.ListenAndServe(cfg.Gateway.Addr)
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
