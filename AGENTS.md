# Agents Guide — pingu

This file provides context for agentic coding assistants working in this repository.

---

## Project Overview

`pingu` is a pure-Go AI agent framework with a ReAct loop, supporting multiple LLM providers,
tool execution, conversation memory, and multi-channel delivery (Telegram, HTTP/SSE). The module
name is `pingu` (see `go.mod`).

---

## Build, Lint, and Test Commands

```bash
# Build the binary
make build                      # go build -o bin/pingu ./cmd/pingu

# Run without compiling
make run                        # go run ./cmd/pingu

# Run all tests
make test                       # go test ./...

# Run tests for a single package
go test ./internal/agent/

# Run a single specific test by name
go test ./internal/agent/ -run TestFunctionName

# Run tests with verbose output
go test -v ./...

# Run tests with race detector
go test -race ./...

# Lint (requires golangci-lint installed)
make lint                       # golangci-lint run

# Regenerate sqlc DB code (after editing queries.sql or schema.sql)
sqlc generate

# Run the gateway with debug logging
LOG_LEVEL=debug go run ./cmd/pingu gateway
```

No Node.js, npm, yarn, or other non-Go tooling is used. All tooling is `go` or `make`.

---

## Repository Structure

```
cmd/pingu/            # CLI entry points (cobra subcommands)
  main.go             # wires subcommands + OTel tracing
  agent.go            # `pingu agent` subcommand
  gateway.go          # `pingu gateway` subcommand (main production path)
  setup.go            # `pingu setup` subcommand

internal/
  agent/              # Runner interface, ReAct loop (simple.go), factory, tools registry
  channels/           # Channel interface + Telegram long-poll implementation
  config/             # TOML config loading with safe defaults
  db/                 # SQLite via sqlc — schema.sql, queries.sql, generated code
  gateway/            # HTTP server, handlers, SSE writer
  history/            # Conversation turn persistence
  llm/                # Provider interface + OpenAI Responses API streaming
  logger/             # slog JSON logger init
  memory/             # Memory interface + ConversationMemory (wraps history.Store)
  tools/              # Built-in tools: shell, file, delegate, message, web
  trace/              # OTel OTLP HTTP exporter setup

docs/                 # Architecture and design documentation
```

---

## Code Style Guidelines

### Language and Formatting

- **Go 1.25+**. Use stdlib-first; only add dependencies when the stdlib is clearly insufficient.
- Format all code with `gofmt` / `goimports` before committing. Tabs for indentation (Go standard).
- Opening brace on the same line as the statement (Go standard). No trailing semicolons.
- Numeric literals use underscores for readability: `10_000`, `0o755`.
- Single-line method bodies are acceptable for trivial getters/name methods:
  ```go
  func (s *Shell) Name() string { return "shell" }
  ```

### Import Ordering

Organize imports in two or three groups separated by a blank line:

1. Standard library packages
2. Internal packages (`pingu/internal/...`)
3. Third-party packages

```go
import (
    "context"
    "fmt"
    "log/slog"

    "pingu/internal/agent"
    "pingu/internal/llm"

    "github.com/spf13/cobra"
    oteltrace "go.opentelemetry.io/otel/trace"
)
```

Use import aliases only to resolve conflicts or for well-known abbreviations (e.g., `oteltrace`).

### Naming Conventions

| Entity | Convention | Examples |
|---|---|---|
| Files | `snake_case.go` | `simple.go`, `conn.go`, `queries.sql.go` |
| Packages | Single lowercase word | `agent`, `tools`, `history`, `llm` |
| Interfaces | PascalCase noun, no `I` prefix | `Runner`, `Tool`, `Provider`, `Memory`, `Channel` |
| Structs | PascalCase | `SimpleRunner`, `AgentProfile`, `SSEWriter` |
| Constructors | `New<TypeName>` | `NewSimpleRunner`, `NewRegistry`, `NewDelegate` |
| Unexported types | camelCase | `tracedTool`, `loggingTransport` |
| Exported constants | PascalCase | `EventToken`, `EventDone` |
| Unexported constants | camelCase | `defaultSystemPrompt`, `maxDelegationDepth` |
| Context keys | Unexported typed `int` const with `Key` suffix | `sessionIDKey`, `emitKey` |
| Functional option type | `<Type>Option` | `RunnerOption` |
| Functional option funcs | `With<FieldName>` | `WithSystemPrompt` |
| HTTP handler methods | `handle<Route>` | `handleChat`, `handleHealthz` |
| Cobra command vars (main package) | unexported camelCase ending in `Cmd` | `agentCmd`, `gatewayCmd`, `setupCmd` |

### Types and Interfaces

- Define interfaces in their own file named after the concept: `agent.go` → `Runner`, `tools.go` → `Tool`.
- Place implementations in separate files: `simple.go`, `openai.go`, `conversation.go`.
- Use the **functional options** pattern (`type RunnerOption func(*SimpleRunner)`) for extensible constructors.
- Use the **decorator/wrapper pattern** for cross-cutting concerns (e.g., `tracedTool` wraps any `Tool`).
- Context helpers come in pairs in a dedicated `context.go`: `ContextWith<X>` + `<X>FromContext`.
- Context keys use an unexported named integer type to prevent collisions:
  ```go
  type contextKey int
  const sessionIDKey contextKey = iota
  ```

### Error Handling

- **Always wrap errors** with `fmt.Errorf("context: %w", err)` to preserve stack context.
- **Non-fatal errors** (session save failure, memory recall) → `slog.Warn("msg", "error", err)` and continue.
- **Fatal errors** in Cobra `RunE` functions → return the error and let Cobra handle `os.Exit`.
- **Tool errors** → return `"error: " + err.Error()` as a string result to the LLM. Do not propagate as Go errors; let the LLM retry.
- Use `errors.As` for typed error inspection (e.g., `*exec.ExitError`).
- **Never use `panic`**. Defensive error handling throughout.
- In traced code, always record errors on spans:
  ```go
  span.RecordError(err)
  span.SetStatus(codes.Error, err.Error())
  ```

### Logging

- Use `log/slog` (stdlib). Always structured key-value pairs:
  ```go
  slog.Debug("loading config", "path", path)
  slog.Warn("session save failed", "error", err)
  slog.Info("server listening", "addr", addr)
  ```
- Output is JSON to stderr. Log level is controlled by the `LOG_LEVEL` env var (`debug`, `info`, `warn`, `error`).
- No `fmt.Println` or `log.Printf` — use `slog` everywhere.

### Concurrency

- Use `sync.WaitGroup` + pre-allocated indexed slices for safe concurrent writes.
- Pass immutable state via `context.Context`; derive a new context per goroutine rather than sharing mutable state.
- Tools within a single LLM response are executed concurrently (see `internal/agent/simple.go`).

---

## Database (sqlc)

- All SQL lives in `internal/db/queries.sql` and `internal/db/schema.sql`.
- Run `sqlc generate` after any SQL change — **never manually edit** `db.go`, `models.go`, or `queries.sql.go`.
- Generated files are marked with `// Code generated by sqlc. DO NOT EDIT.`
- Schema is embedded via `//go:embed schema.sql` and applied idempotently on startup (`IF NOT EXISTS`).
- Uses `modernc.org/sqlite` (pure Go, no CGo) for cross-compilation support.

---

## Testing Conventions

- Use the standard `testing` package. No external test frameworks (no testify, gomock, etc.).
- Test files are named `<file>_test.go` in the same package; use `package <pkg>_test` for black-box tests.
- Prefer **table-driven tests** for multiple input/output cases.
- Run a single test: `go test ./internal/<pkg>/ -run TestFunctionName`
- Run with the race detector when testing concurrent code: `go test -race ./internal/agent/`

---

## Observability

- Spans follow OpenAI Agents SDK conventions for LLM observability platform compatibility:
  `openai.agents.agent.name`, `gen_ai.tool.name`, `gen_ai.tool.input`, `gen_ai.tool.output`
- Tracing is configured in `internal/trace/trace.go` and wired in `cmd/pingu/main.go`.
- Always add spans at package boundaries and record errors on them.

---

## Miscellaneous

- **Session IDs** are string-based and hierarchical: `telegram:648079060`, `telegram:648079060:delegate:sysinfo`.
- **Config** (`~/.config/pingu/config.toml`) has hardcoded defaults; a missing file is not an error.
- **HTTP routing** uses Go 1.22+ method+path syntax: `"POST /v1/chat"` — no routing library needed.
- **Single binary**: the compiled binary is self-contained (embedded SQL schema, pure-Go SQLite).
- Do not commit the `pingu` binary at the repo root; build outputs go to `bin/` (gitignored).
