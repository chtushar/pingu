# Gateway

The gateway is the HTTP server that hosts the API and channel routes. It's started via the `pingu gateway` command.

## Starting

```bash
pingu gateway              # listen on :8484 (default)
pingu gateway --addr :9090 # override listen address
```

The gateway command (`cmd/pingu/gateway/gateway.go`) wires everything together:

1. Loads config from `~/.config/pingu/config.toml`
2. Opens and migrates the SQLite database
3. Creates the history store
4. Initializes the LLM provider
5. Builds the global tool registry (message, shell, file)
6. Loads agent profiles and creates the delegate tool if any exist
7. Builds the orchestrator runner
8. Starts channel pollers in background goroutines
9. Starts the HTTP server

## HTTP API

The HTTP server (`internal/gateway/server.go`) exposes these routes:

| Method | Path | Status | Description |
|--------|------|--------|-------------|
| `GET` | `/healthz` | Implemented | Health check (returns 200) |
| `POST` | `/v1/chat` | Stub | Chat endpoint |
| `GET` | `/v1/sessions` | Stub | List sessions |
| `GET` | `/v1/sessions/{id}` | Stub | Get session details |
| `DELETE` | `/v1/sessions/{id}/run` | Stub | Cancel a running agent |

Channel routes are also registered on the same mux (e.g. Telegram webhook endpoints if configured).

## SSE Streaming

The `SSEWriter` (`internal/gateway/sse.go`) provides Server-Sent Events support for streaming agent responses over HTTP:

```go
writer := gw.NewSSEWriter(w)
writer.Send("token", tokenData)
writer.Send("done", nil)
```

It sets the appropriate headers (`text/event-stream`, `no-cache`, `keep-alive`) and flushes after each event.

## Graceful Shutdown

The gateway listens for `SIGINT` and `SIGTERM`. When received, the context is cancelled, which:

1. Stops channel pollers (their `Start(ctx)` returns)
2. Shuts down the HTTP server via `server.Shutdown(ctx)`
3. Closes the database connection (deferred in the command)
