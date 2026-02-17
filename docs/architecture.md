# Architecture

Pingu is a personal AI agent that runs as a single Go binary. It connects to LLM providers, exposes tools for the model to call, and communicates with users through pluggable channels (Telegram, HTTP, etc.).

## High-Level Design

```
                    +-----------+
                    |  Channels |
                    | (Telegram,|
                    |  HTTP)    |
                    +-----+-----+
                          |
                    Runner.Run(sessionID, message, emit)
                          |
                    +-----v-----+
                    |   Agent   |
                    | (Runner)  |
                    +-----+-----+
                          |
              +-----------+-----------+
              |                       |
        +-----v-----+          +-----v-----+
        | LLM       |          | Tool      |
        | Provider   |          | Registry  |
        +-----+-----+          +-----+-----+
              |                       |
    OpenAI Responses API    shell, file, message,
                            delegate, ...
```

## Data Flow

1. A **Channel** receives a user message (e.g. Telegram long-poll).
2. The channel calls `Runner.Run(ctx, sessionID, message, emit)`.
3. The **Runner** loads conversation history from the **Store**.
4. It calls the **LLM Provider** with the conversation + tool definitions.
5. The LLM returns tool calls. The runner executes them **in parallel**.
6. Tool results are appended to the conversation and the loop repeats.
7. When the LLM produces no more tool calls, the turn is saved and `EventDone` is emitted.
8. The channel collects `EventToken` events and sends the response back to the user.

## Key Packages

| Package | Path | Purpose |
|---------|------|---------|
| `agent` | `internal/agent/` | Runner interface, SimpleRunner, tool registry, profiles, factory |
| `tools` | `internal/tools/` | Built-in tool implementations |
| `channels` | `internal/channels/` | Channel interface, Telegram implementation |
| `config` | `internal/config/` | TOML configuration loading |
| `llm` | `internal/llm/` | LLM provider interface and OpenAI implementation |
| `history` | `internal/history/` | Conversation history persistence |
| `db` | `internal/db/` | SQLite database, schema, generated queries |
| `gateway` | `internal/gateway/` | HTTP server for API and channel routes |
| `trace` | `internal/trace/` | OpenTelemetry tracing setup |
| `logger` | `internal/logger/` | Structured logging setup |

## Entrypoint

`cmd/pingu/main.go` registers three subcommands via Cobra:

- **`gateway`** — starts the HTTP server, channels, and agent loop. This is the main production command.
- `setup` — placeholder for initial configuration wizard.
- `agent` — placeholder for CLI-based agent interaction.

The `gateway` command (`cmd/pingu/gateway/gateway.go`) wires everything together: config, database, LLM provider, tool registry, agent profiles, channels, and the HTTP server.

## Concurrency Model

Each channel runs in its own goroutine. When a message arrives, the channel calls `Runner.Run()` synchronously — the runner's agentic loop blocks until the LLM conversation completes. Within a single turn, tool calls execute concurrently via goroutines and `sync.WaitGroup`.

There is no internal message queue or worker pool. For a personal assistant, one-request-at-a-time per channel is the right trade-off.
