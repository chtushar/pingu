# Multi-Agent Delegation

Pingu supports an orchestrator pattern where a parent agent delegates tasks to specialized sub-agents, each with scoped tools and system prompts.

## How It Works

Sub-agents are implemented as a tool. The parent agent calls the `delegate` tool, which:

1. Looks up the agent profile by name.
2. Builds a scoped `SimpleRunner` from the profile (restricted tools, custom system prompt).
3. Runs a full agent loop with a local emit that captures output into a buffer.
4. Returns the captured text as the tool result.

The sub-agent **cannot** message the user directly. All output flows back to the parent as a tool result, and the parent decides what to relay.

```
Parent Agent
  ├── calls delegate(agent="sysinfo", task="...")
  │     └── sysinfo runner.Run(...)
  │           ├── shell("hostname") → "macbook"
  │           └── message("Hostname: macbook") → captured in buffer
  │     returns "Hostname: macbook" as tool output
  │
  ├── calls delegate(agent="fileviewer", task="...")  ← runs in parallel
  │     └── fileviewer runner.Run(...)
  │           ├── file(read, "/etc/hosts") → "127.0.0.1 ..."
  │           └── message("Contents: ...") → captured in buffer
  │     returns "Contents: ..." as tool output
  │
  └── message("Here are the results: ...") → sent to user
```

## Configuration

Define agent profiles in `config.toml`:

```toml
[agent.sysinfo]
system_prompt = "You are a system info agent. Use the shell tool to gather information, then report findings via the message tool."
tools = ["shell", "message"]

[agent.fileviewer]
system_prompt = "You are a file reader agent. Use the file tool to read files, then report findings via the message tool."
tools = ["file", "message"]
```

The `delegate` tool is automatically registered when any agent profiles are configured.

### Orchestrator Profile

If an `[agent.orchestrator]` profile exists, it's used as the top-level agent:

```toml
[agent.orchestrator]
system_prompt = "You are an orchestrator. Delegate specialized tasks to sub-agents. Summarize their results for the user."
tools = ["message", "delegate"]
```

If no orchestrator profile exists, the default runner with all tools is used.

## Parallel Delegation

When the LLM returns multiple `delegate` calls in a single response, they execute concurrently. This is the primary benefit of parallel tool execution — the orchestrator can fan out to multiple specialists simultaneously.

## Session Isolation

Sub-agents get derived session IDs to keep their history separate:

```
Parent:  telegram:648079060
Child:   telegram:648079060:delegate:sysinfo
```

Each sub-agent's conversation is persisted independently.

## Recursion Guard

A delegation depth counter is stored in context. The `delegate` tool refuses to run if the depth exceeds 3, preventing infinite delegation loops.

```
depth 0: orchestrator
depth 1: sysinfo (delegated by orchestrator)
depth 2: helper (delegated by sysinfo)
depth 3: sub-helper (delegated by helper)
depth 4: REFUSED — max depth exceeded
```

## Emit Isolation

The emit callback is passed through context, not through mutable tool state. Each runner's `Run` call stores its own emit in context:

- The parent's context has the Telegram emit (sends messages to the user).
- The sub-agent's context has a local emit (captures into a `strings.Builder`).

This means shared tool instances (from `Registry.Scope`) are safe — the `message` tool reads emit from whatever context it receives.
