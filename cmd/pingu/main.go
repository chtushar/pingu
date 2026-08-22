// Command pingu is the CLI for the pingu agent runtime.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/chtushar/pingu/internal/config"
	"github.com/chtushar/pingu/internal/logging"

	"github.com/spf13/cobra"
)

// Exit codes: 0 success, 1 runtime/provider failure, 2 usage/config error,
// 130 interrupted.
const (
	exitSuccess     = 0
	exitRuntime     = 1
	exitUsageConfig = 2
	exitInterrupted = 130
)

// interrupted is set by the signal handler when the user pressed Ctrl-C; it
// distinguishes "the run was cancelled" from other runtime failures.
var interrupted bool

func main() {
	logging.Init()
	root := newRootCmd()
	root.AddCommand(newInitCmd())
	root.AddCommand(newRunCmd())
	if err := root.Execute(); err != nil {
		var cfgErr *config.ConfigError
		switch {
		case errors.As(err, &cfgErr):
			fmt.Fprintln(os.Stderr, "config error:", err)
			os.Exit(exitUsageConfig)
		case interrupted:
			fmt.Fprintln(os.Stderr, "interrupted")
			os.Exit(exitInterrupted)
		default:
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(exitRuntime)
		}
	}
	if interrupted {
		os.Exit(exitInterrupted)
	}
}

func newRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pingu",
		Short: "The framework for building agents",
		Long: `pingu runs text agents defined by a directory.

An agent directory needs only an instructions.md file. Optional additions:
agent.toml (model and runtime config), tools/ (executable tool plugins),
skills/ (markdown playbooks), and more. See docs/ for details.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
}
