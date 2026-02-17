# Tracing

Pingu includes OpenTelemetry tracing at every layer — agent loop, LLM calls, and tool execution.

## Setup

Tracing is configured in `cmd/pingu/main.go` at startup. It reads the trace endpoint and API key from the LLM config and initializes an OTLP HTTP exporter.

The tracer uses a `SimpleSpanProcessor` (synchronous export) so spans are sent immediately, which is appropriate for a low-throughput personal agent.

## Span Hierarchy

```
agent.run (session_id, user.message)
├── llm.chat (iteration=0, model, input_tokens, output_tokens)
├── shell (gen_ai.tool.name, gen_ai.tool.input, gen_ai.tool.output)
├── llm.chat (iteration=1)
├── delegate (gen_ai.tool.name, gen_ai.tool.input, gen_ai.tool.output)
│   └── agent.run (sub-agent)
│       ├── llm.chat (iteration=0)
│       ├── file (...)
│       ├── llm.chat (iteration=1)
│       └── message (...)
├── llm.chat (iteration=2)
└── message (...)
```

## Span Attributes

### `agent.run`

| Attribute | Description |
|-----------|-------------|
| `openai.agents.agent.name` | Always `"pingu"` |
| `session.id` | Session identifier |
| `user.message` | First 200 chars of the user's message |

### `llm.chat`

| Attribute | Description |
|-----------|-------------|
| `llm.iteration` | Loop iteration (0-based) |
| `llm.model` | Model used for this call |
| `llm.input_tokens` | Input token count |
| `llm.output_tokens` | Output token count |

### Tool spans

| Attribute | Description |
|-----------|-------------|
| `openai.agents.span_type` | Always `"function"` |
| `gen_ai.tool.name` | Tool name |
| `gen_ai.tool.input` | Raw JSON input |
| `gen_ai.tool.output` | Result string |
| `gen_ai.tool.output_length` | Result length in bytes |

## Trace Configuration

The OTLP exporter is configured via the `trace.Config` struct:

```go
type Config struct {
    Endpoint string // host:port (e.g. "localhost:5177")
    URLPath  string // OTLP path (e.g. "/api/otlp/v1/traces")
    APIKey   string // Bearer token for authentication
}
```

The exporter uses HTTP (not gRPC) with insecure mode. A custom `loggingTransport` logs all outbound requests when debug logging is enabled.

## Viewing Traces

Any OTLP-compatible backend works. Some options:

- **Langfuse** — LLM-focused observability with prompt tracking
- **Jaeger** — general-purpose trace viewer
- **Grafana Tempo** — if you already use Grafana

Point `Endpoint` and `URLPath` to your backend and set the `APIKey` if required.

## Debug Logging

With `LOG_LEVEL=debug`, the tracer logs:

- Every span export (`otlp exporting spans`)
- HTTP requests/responses to the OTLP endpoint
- Span start events with trace and span IDs

This is useful for verifying spans are being sent and the trace hierarchy is correct.
