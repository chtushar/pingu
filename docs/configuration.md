# Configuration

Pingu reads its configuration from `~/.config/pingu/config.toml`.

## Full Example

```toml
default_llm = "openai"

[llm.openai]
model = "gpt-4.1-nano"
base_url = "https://api.openai.com/v1"
api_key = "sk-..."

[gateway]
addr = ":8484"
token = "my-secret-token"

[db]
path = "~/.local/share/pingu/pingu.db"

[channel.telegram]
enabled = true
type = "telegram"

[channel.telegram.settings]
bot_token = "123456:ABC-DEF..."
allowed_users = "648079060,123456789"

[agent.sysinfo]
system_prompt = "You are a system info agent. Use the shell tool to gather information."
tools = ["shell", "message"]

[agent.fileviewer]
system_prompt = "You are a file reader agent. Use the file tool to read files."
tools = ["file", "message"]

[agent.orchestrator]
system_prompt = "You are an orchestrator. Delegate tasks to specialized agents when appropriate."
tools = ["message", "delegate"]
```

## Sections

### `default_llm`

Name of the LLM configuration to use. Must match a key in `[llm.*]`. Default: `"openai"`.

### `[llm.<name>]`

| Field | Type | Description |
|-------|------|-------------|
| `model` | string | Model identifier (e.g. `"gpt-4.1-nano"`) |
| `base_url` | string | API base URL. Empty uses the provider's default. |
| `api_key` | string | API key for authentication |

### `[gateway]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `addr` | string | `":8484"` | HTTP listen address. Override with `--addr` flag. |
| `token` | string | | Auth token for the HTTP API |

### `[db]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `path` | string | `~/.local/share/pingu/pingu.db` | SQLite database path. `~/` is expanded. |

### `[channel.<name>]`

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Whether this channel is active |
| `type` | string | Channel type. Currently only `"telegram"` is supported. |

#### `[channel.<name>.settings]`

Key-value pairs specific to the channel type.

**Telegram settings:**

| Key | Description |
|-----|-------------|
| `bot_token` | Telegram Bot API token from @BotFather |
| `allowed_users` | Comma-separated list of authorized Telegram user IDs |

### `[agent.<name>]`

Defines a named agent profile for multi-agent delegation. See [Delegation](delegation.md).

| Field | Type | Description |
|-------|------|-------------|
| `system_prompt` | string | System prompt for this agent |
| `tools` | string[] | List of tool names this agent can use. Empty = all tools. |

The special profile name `"orchestrator"` is used as the top-level agent when it exists. If no orchestrator profile is defined, pingu uses a default runner with all tools.

## Defaults

If the config file doesn't exist, pingu runs with these defaults:

- LLM: OpenAI with `gpt-4.1-nano`
- Gateway: `:8484`
- Database: `~/.local/share/pingu/pingu.db`
- No channels enabled
- No agent profiles (single agent with all tools)
