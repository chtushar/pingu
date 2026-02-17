# Agent System

The agent system lives in `internal/agent/` and implements the core agentic loop: call the LLM, execute tool calls, repeat until done.

## Runner Interface

```go
type Runner interface {
    Run(ctx context.Context, sessionID string, message string, emit func(Event)) error
}
```

- `sessionID` — identifies the conversation. History is loaded/saved per session.
- `message` — the user's input text.
- `emit` — callback for streaming events back to the caller.

### Events

```go
type Event struct {
    Type EventType
    Data any
}
```

| EventType | Data | Description |
|-----------|------|-------------|
| `token` | `string` | Text content to show the user |
| `tool_call` | | A tool was called (for observability) |
| `tool_result` | | A tool returned (for observability) |
| `done` | | The agent loop finished |
| `error` | `string` | An error occurred |

## SimpleRunner

`SimpleRunner` is the only `Runner` implementation. It runs a ReAct-style loop:

1. Load conversation history from the store.
2. Prepend the system prompt as a `developer` message.
3. Call the LLM with the conversation and tool definitions.
4. If the LLM returns tool calls, execute them **in parallel** and append results.
5. Repeat from step 3.
6. When no tool calls remain, save the turn and emit `EventDone`.

### Constructor

```go
func NewSimpleRunner(provider llm.Provider, store *history.Store, registry *Registry, opts ...RunnerOption) *SimpleRunner
```

Options:

- `WithSystemPrompt(s string)` — override the default system prompt.

The default system prompt is:
> You must use the message tool to communicate with the user. Do not produce raw text output.

### Parallel Tool Execution

When the LLM returns multiple tool calls in a single response (e.g. two `delegate` calls), they all execute concurrently via goroutines and `sync.WaitGroup`. Results are collected in order and appended to the conversation.

### Context Values

`SimpleRunner.Run` stores values in the context that tools can read:

| Helper | Purpose |
|--------|---------|
| `ContextWithSessionID` / `SessionIDFromContext` | Current session ID |
| `ContextWithEmit` / `EmitFromContext` | The emit callback for this runner |
| `ContextWithDelegationDepth` / `DelegationDepthFromContext` | Recursion depth for delegation |

The emit callback is passed through context (not mutable tool state) so that sub-agents get their own isolated emit without interfering with the parent.

## Tool Registry

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() any
    Execute(ctx context.Context, input string) (string, error)
}
```

The `Registry` holds named tools:

```go
registry := agent.NewRegistry()
registry.Register(&tools.Message{})
registry.Register(&tools.Shell{})
```

### Scoping

`registry.Scope(names []string)` returns a new registry containing only the named tools. If `names` is empty, all tools are copied. This is used to give sub-agents restricted toolsets.

```go
// Sub-agent that can only read files and send messages
scoped := registry.Scope([]string{"file", "message"})
```

`Scope` copies tool references — the same tool instances are shared. This is safe because tools read the emit callback from context rather than from mutable state.

## Tracing

Every tool execution is wrapped with OpenTelemetry tracing at call time (in the runner loop). The `tracedTool` wrapper in `traced.go` creates a span with:

- `gen_ai.tool.name` — tool name
- `gen_ai.tool.input` — raw input JSON
- `gen_ai.tool.output` — result string

The runner also creates spans for `agent.run` (the full loop) and `llm.chat` (each LLM call).

## Agent Profiles

An `AgentProfile` defines a named agent configuration:

```go
type AgentProfile struct {
    Name         string
    SystemPrompt string
    Tools        []string // tool names; empty = all tools
}
```

Profiles are defined in `config.toml` under `[agent.<name>]` and converted to `AgentProfile` structs at startup.

## RunnerFactory

The `RunnerFactory` builds scoped `SimpleRunner` instances from profiles:

```go
factory := agent.NewRunnerFactory(provider, store, registry, profiles)
runner, err := factory.Build("sysinfo")
```

`Build` scopes the registry to the profile's tool list and applies the profile's system prompt.
