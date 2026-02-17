# Channels

Channels are how pingu communicates with users. Each channel is a pluggable input/output adapter that receives messages and sends responses.

## Channel Interface

```go
type Channel interface {
    Name() string
    RegisterRoutes(mux *http.ServeMux)
    Start(ctx context.Context) error
}
```

- `Name()` — unique identifier for the channel (e.g. `"telegram"`).
- `RegisterRoutes(mux)` — optional HTTP routes (e.g. webhooks). Called at server startup.
- `Start(ctx)` — blocks until `ctx` is cancelled. This is where polling loops or listeners run.

Channels are started in background goroutines by the gateway command. Each channel receives a `Runner` and calls `runner.Run()` when a message arrives.

## Telegram

The Telegram channel uses long polling to receive messages and the Bot API to send responses.

### Setup

1. Create a bot via [@BotFather](https://t.me/BotFather).
2. Get your user ID (send a message to [@userinfobot](https://t.me/userinfobot)).
3. Add to `config.toml`:

```toml
[channel.telegram]
enabled = true
type = "telegram"

[channel.telegram.settings]
bot_token = "123456:ABC-DEF..."
allowed_users = "648079060"
```

### Behavior

- **Long polling** with 30-second timeout via `/getUpdates`.
- **Authorization**: only users listed in `allowed_users` can interact. Unauthorized messages are silently ignored.
- **Typing indicator**: sends a `typing` chat action while the agent is processing.
- **Session ID**: `telegram:{chat_id}` — each Telegram chat gets its own conversation history.
- **Response collection**: all `EventToken` events are concatenated into a single reply sent via `/sendMessage`.

### Error Handling

If the agent returns an error, the channel sends `"Sorry, something went wrong."` to the user.

## Writing a Custom Channel

1. Create a file in `internal/channels/`.
2. Implement the `Channel` interface.
3. Add a case in `buildChannels()` in `cmd/pingu/gateway/gateway.go`.

Example skeleton for a Discord channel:

```go
package channels

import (
    "context"
    "pingu/internal/agent"
)

type Discord struct {
    token  string
    runner agent.Runner
}

func NewDiscord(token string, runner agent.Runner) *Discord {
    return &Discord{token: token, runner: runner}
}

func (d *Discord) Name() string { return "discord" }

func (d *Discord) RegisterRoutes(mux *http.ServeMux) {
    // Optional: register webhook endpoints
}

func (d *Discord) Start(ctx context.Context) error {
    // Connect to Discord gateway, listen for messages
    // On message: call d.runner.Run(ctx, sessionID, text, emit)
    // Collect EventToken events, send as Discord reply
    <-ctx.Done()
    return nil
}
```

## Cron as a Channel

The `Channel` interface naturally supports cron jobs — `Start()` runs a ticker loop:

```go
type Cron struct {
    schedule string
    task     string
    runner   agent.Runner
}

func (c *Cron) Name() string { return "cron" }
func (c *Cron) RegisterRoutes(mux *http.ServeMux) {}

func (c *Cron) Start(ctx context.Context) error {
    ticker := time.NewTicker(c.interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            c.runner.Run(ctx, "cron:"+c.Name(), c.task, func(e agent.Event) {})
        }
    }
}
```

This fits the existing config pattern: `[channel.daily_summary]` with `type = "cron"`.
