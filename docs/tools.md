# Tools

Tools are how the agent interacts with the world. Each tool implements the `agent.Tool` interface and is registered on a `Registry` before being passed to the runner.

## Tool Interface

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() any          // must return map[string]any (JSON Schema)
    Execute(ctx context.Context, input string) (string, error)
}
```

- `InputSchema()` returns a JSON Schema object. The LLM uses this to generate valid arguments. Use `"additionalProperties": false` and `"strict": true` is set automatically.
- `Execute` receives the raw JSON string the LLM produced and returns a text result.
- Errors returned from `Execute` are sent back to the LLM as `"error: ..."` so it can retry or adjust.

## Built-in Tools

### `message`

Sends text to the user. This is the agent's only way to communicate — raw LLM text output is suppressed.

```json
{ "text": "Hello, here are your results..." }
```

The message tool reads the `emit` callback from context (`agent.EmitFromContext`) and emits an `EventToken` event. Each runner's `Run` call puts its own emit in context, so sub-agents emit to their local buffer while the parent emits to the channel.

**Source:** `internal/tools/message.go`

### `shell`

Executes a shell command and returns stdout/stderr.

```json
{ "command": "ls -la /tmp", "timeout": 5000 }
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `command` | string | required | Shell command to run via `sh -c` |
| `timeout` | number | 30000 | Timeout in milliseconds |

Output is truncated to 10,000 bytes. Non-zero exit codes are reported but not treated as errors — the output and exit code are returned to the LLM.

**Source:** `internal/tools/shell.go`

### `file`

Reads or writes files.

```json
{ "action": "read", "path": "/etc/hosts" }
{ "action": "write", "path": "/tmp/out.txt", "content": "hello" }
```

| Field | Type | Description |
|-------|------|-------------|
| `action` | `"read"` or `"write"` | Operation to perform |
| `path` | string | Absolute file path |
| `content` | string | Content to write (required for `write`) |

Read output is truncated to 10,000 bytes. Write creates parent directories automatically.

**Source:** `internal/tools/file.go`

### `delegate`

Spawns a scoped sub-agent to handle a task. See [Delegation](delegation.md) for details.

```json
{ "agent": "sysinfo", "task": "Get the current date and hostname" }
```

**Source:** `internal/tools/delegate.go`

### `web`

Placeholder for URL fetching. Not yet implemented.

**Source:** `internal/tools/web.go`

## Writing a Custom Tool

1. Create a file in `internal/tools/`.
2. Implement the `agent.Tool` interface.
3. Register it in `cmd/pingu/gateway/gateway.go`.

Example:

```go
package tools

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
)

type Clock struct{}

func (c *Clock) Name() string        { return "clock" }
func (c *Clock) Description() string { return "Get the current time" }

func (c *Clock) InputSchema() any {
    return map[string]any{
        "type":                 "object",
        "properties":          map[string]any{},
        "required":            []string{},
        "additionalProperties": false,
    }
}

func (c *Clock) Execute(ctx context.Context, input string) (string, error) {
    return time.Now().Format(time.RFC3339), nil
}
```

Register it:

```go
registry.Register(&tools.Clock{})
```

### Accessing the Emit Callback

If your tool needs to stream events to the user (like `message` does), read it from context:

```go
if emit := agent.EmitFromContext(ctx); emit != nil {
    emit(agent.Event{Type: agent.EventToken, Data: "streaming text..."})
}
```

### Output Truncation

Use the `truncate` helper from `common.go` to cap output at 10,000 bytes:

```go
return string(truncate([]byte(largeOutput))), nil
```
