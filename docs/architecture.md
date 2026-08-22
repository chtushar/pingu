# Architecture

pingu is a local-first runtime and CLI for text agents. One binary; an agent
is a portable directory whose only required file is `instructions.md`.

## Package layout

```text
cmd/pingu/             Cobra wiring only: init, run, (later: validate, serve)
internal/agent/        agent-directory loading and validation
internal/config/       defaults, TOML decoding, env/flag precedence, limits
internal/llm/          provider-neutral request/response/event types
internal/provider/     provider adapters (openai) — the only place wire
                       formats exist
internal/runner/       bounded model/tool loop; owns ordering and termination
internal/tools/        Tool interface and registry (executable tools: Phase 2)
internal/logging/      structured JSON logging to stderr
```

Implementation packages stay under `internal/` until a real external-library
use case exists.

## Core contracts

### Provider (internal/llm)

```go
type Provider interface {
    Stream(ctx context.Context, req Request) (Stream, error)
}
type Stream interface {
    Next(ctx context.Context) (Event, error) // io.EOF at end
    Close() error
}
```

`Request` is provider-neutral: model reference, system prompt, messages,
tool definitions. The event vocabulary is `text_delta`,
`tool_call_start`, `tool_call_arguments_delta`, `tool_call_end`, and
`usage`. Provider SDK types never cross the adapter boundary; the OpenAI
adapter speaks raw HTTP with the standard library.

### Runner (internal/runner)

The runner alone owns loop termination and event ordering. For each run it
emits, synchronously and in order:

| Event | Meaning |
|---|---|
| `run_started` | the run began (carries the run ID) |
| `text_delta` | assistant text chunk |
| `tool_started` / `tool_finished` | tool invocation boundaries |
| `warning` | recoverable issue (truncated output, exhausted budget) |
| `error` | terminal failure detail |
| `run_finished` | final event; carries turns, usage, and terminal error |

The terminal, future channels (HTTP/SSE, Telegram), tracing, and tests are
all consumers of this one stream.

Every run is bounded: maximum model turns, total tool calls, wall-clock run
timeout, per-tool timeout, and captured tool output bytes. Exceeding a limit
fails the run with `ErrLimitExhausted`. Cancellation propagates from the
context into provider streams and tool calls.

Tool errors are conversation content, not Go errors: a failing tool returns
`"error: <message>"` so the model can recover. Unknown tools and malformed
JSON arguments follow the same convention.

### Tool (internal/tools)

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage
    Run(ctx context.Context, args json.RawMessage) (string, error)
}
```

Phase 1 ships no built-in tools. Phase 2 discovers executable plugins from
the agent directory's `tools/` behind this same interface.

## Error handling and exit codes

- Errors are wrapped with `fmt.Errorf("context: %w", err)`.
- `config.ConfigError` marks usage/config failures: malformed agent.toml,
  missing instructions.md, invalid model references, missing credentials.
  The CLI exits **2**.
- Provider/runtime failures exit **1**.
- Interruption (Ctrl-C) exits **130**; the first Ctrl-C cancels the current
  run, a second exits immediately.
- Success exits **0**.

## Logging

`log/slog` with JSON output on stderr; level via `LOG_LEVEL`
(`debug|info|warn|error`, default `info`). Secrets are never logged and
message content stays out of info-level logs.
