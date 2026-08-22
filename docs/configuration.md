# Configuration

Configuration precedence, highest first:

1. Command flags (`--model`, `--max-turns`, `--timeout`)
2. `agent.toml` in the agent directory
3. `PINGU_*` environment variables
4. Documented defaults

Credentials are environment-only in the first releases.

## Agent directory

```text
my-agent/
  instructions.md   # required; identity and behavior (256 KiB limit)
  agent.toml        # optional
  .pingu/           # runtime state (created at runtime, gitignored)
```

## agent.toml (Phase 1 fields)

```toml
model = "openai/gpt-4o-mini"
```

Model references use `provider/model-id`, split on the first slash. Phase 1
supports the `openai` provider only. Unknown fields are rejected so typos
fail at startup.

## Environment variables

| Variable | Default | Meaning |
|---|---|---|
| `OPENAI_API_KEY` | — | OpenAI credential (required for `openai` models) |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | override for OpenAI-compatible endpoints |
| `PINGU_MODEL` | `openai/gpt-4o-mini` | model reference |
| `PINGU_MAX_MODEL_TURNS` | `32` | model calls per run |
| `PINGU_MAX_TOOL_CALLS` | `64` | tool invocations per run |
| `PINGU_RUN_TIMEOUT` | `10m` | wall-clock budget per run |
| `PINGU_TOOL_TIMEOUT` | `60s` | wall-clock budget per tool call |
| `PINGU_MAX_TOOL_OUTPUT_BYTES` | `65536` | captured tool output per call |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |

`PINGU_STATE_DIR` will relocate runtime state when persistence lands
(Phase 3).

## CLI

```sh
pingu init my-agent                # scaffold an agent directory
pingu run my-agent                 # interactive session
pingu run my-agent -m "hello"      # one-shot; exits when done
pingu run my-agent --model openai/gpt-4o-mini
```

Interactive session: `/exit` or Ctrl-D quits; Ctrl-C interrupts the current
run; a second Ctrl-C exits immediately.

Exit codes: `0` success, `1` runtime/provider failure, `2` usage/config
error, `130` interrupted.
