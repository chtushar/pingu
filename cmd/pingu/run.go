package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chtushar/pingu/internal/agent"
	"github.com/chtushar/pingu/internal/config"
	"github.com/chtushar/pingu/internal/llm"
	"github.com/chtushar/pingu/internal/provider/openai"
	"github.com/chtushar/pingu/internal/runner"
	"github.com/chtushar/pingu/internal/tools"

	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var (
		message  string
		model    string
		maxTurns int
		timeout  time.Duration
	)
	cmd := &cobra.Command{
		Use:   "run PATH",
		Short: "Run an agent from a directory",
		Long: `Run the agent defined at PATH.

With --message, run a single exchange and exit — useful for scripts and
tests. Without it, start an interactive terminal session: type a message and
press Enter; /exit or Ctrl-D quits. Ctrl-C interrupts the current run; a
second Ctrl-C exits immediately.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := agent.Load(args[0])
			if err != nil {
				return err
			}
			cfg := a.Config
			if err := cfg.ApplyModelFlag(model); err != nil {
				return err
			}
			limits, err := config.DefaultLimits.ApplyEnv()
			if err != nil {
				return err
			}
			if maxTurns > 0 {
				limits.MaxModelTurns = maxTurns
			}
			if timeout > 0 {
				limits.RunTimeout = timeout
			}

			p, err := openai.FromEnv(nil)
			if err != nil {
				return &config.ConfigError{Field: "OPENAI_API_KEY", Err: err}
			}

			slog.Debug("agent loaded", "root", a.Root, "model", cfg.Model.String())

			// Phase 1 ships no built-in tools; executable tools from the
			// agent directory land in Phase 2.
			registry, err := tools.NewRegistry()
			if err != nil {
				return err
			}

			r := &runner.Runner{Provider: p, Limits: limits}
			if message != "" {
				return oneShot(r, registry, a, cfg.Model.String(), message)
			}
			return repl(r, registry, a, cfg.Model.String())
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "send one message and exit")
	cmd.Flags().StringVar(&model, "model", "", "model reference provider/model-id (overrides agent.toml and PINGU_MODEL)")
	cmd.Flags().IntVar(&maxTurns, "max-turns", 0, "maximum model turns per run (overrides PINGU_MAX_MODEL_TURNS)")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "total run timeout (overrides PINGU_RUN_TIMEOUT)")
	return cmd
}

func oneShot(r *runner.Runner, registry *tools.Registry, a *agent.Agent, model, message string) error {
	ctx, cancel, stop := withSignalCancel()
	defer func() {
		cancel()
		stop()
	}()
	_, err := r.Run(ctx, runner.RunRequest{
		RunID:        newRunID(),
		Instructions: a.Instructions,
		Model:        model,
		Input:        message,
		Tools:        registry,
	}, renderEvent)
	if err == nil || errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stdout)
	}
	return err
}

func repl(r *runner.Runner, registry *tools.Registry, a *agent.Agent, model string) error {
	conv := &conversation{}
	reader := bufio.NewScanner(os.Stdin)
	reader.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	fmt.Fprintln(os.Stdout, "pingu — type a message, /exit or Ctrl-D to quit")
	for {
		fmt.Fprint(os.Stdout, "> ")
		if !reader.Scan() {
			fmt.Fprintln(os.Stdout)
			return nil // EOF
		}
		line := strings.TrimSpace(reader.Text())
		if line == "" {
			continue
		}
		if line == "/exit" {
			return nil
		}

		ctx, cancel, stop := withSignalCancel()
		result, err := r.Run(ctx, runner.RunRequest{
			RunID:        newRunID(),
			Instructions: a.Instructions,
			Model:        model,
			Input:        line,
			History:      conv.messages(),
			Tools:        registry,
		}, renderEvent)
		cancel()
		stop()

		if err != nil {
			if errors.Is(err, context.Canceled) {
				fmt.Fprintln(os.Stderr, "(interrupted)")
				continue
			}
			return err
		}
		conv.append(result.Messages)
		fmt.Fprintln(os.Stdout)
	}
}

// renderEvent prints run events: assistant text to stdout, diagnostics to
// stderr.
func renderEvent(ev runner.Event) {
	switch ev.Kind {
	case runner.EventTextDelta:
		fmt.Fprint(os.Stdout, ev.Text)
	case runner.EventToolStarted:
		fmt.Fprintf(os.Stderr, "→ %s\n", ev.ToolName)
	case runner.EventToolFinished:
		fmt.Fprintf(os.Stderr, "✓ %s\n", ev.ToolName)
	case runner.EventWarning:
		fmt.Fprintf(os.Stderr, "warning: %s\n", ev.Text)
	case runner.EventError:
		fmt.Fprintf(os.Stderr, "error: %s\n", ev.Text)
	}
}

// conversation accumulates messages across runs within one process.
type conversation struct {
	msgs []llm.Message
}

func (c *conversation) messages() []llm.Message { return c.msgs }

func (c *conversation) append(msgs []llm.Message) { c.msgs = append(c.msgs, msgs...) }

// withSignalCancel returns a context that is cancelled on the first SIGINT.
// A second SIGINT while the first is being handled exits immediately with
// code 130. stop releases the signal handler.
func withSignalCancel() (context.Context, context.CancelFunc, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt)
	done := make(chan struct{})
	go func() {
		select {
		case <-sig:
			interrupted = true
			cancel()
			select {
			case <-sig:
				fmt.Fprintln(os.Stderr, "interrupted")
				os.Exit(exitInterrupted)
			case <-done:
			}
		case <-done:
		}
	}()
	stop := func() {
		signal.Stop(sig)
		close(done)
	}
	return ctx, cancel, stop
}

var runCounter atomic.Int64

func newRunID() string {
	return fmt.Sprintf("run-%d-%d", time.Now().UnixMilli(), runCounter.Add(1))
}
