package agent

import (
	"context"
	"log/slog"
	"pingu/internal/history"
	"pingu/internal/llm"
	"pingu/internal/memory"
	"pingu/internal/trace"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const defaultSystemPrompt = "You must use the message tool to communicate with the user. Do not produce raw text output."

type RunnerOption func(*SimpleRunner)

func WithSystemPrompt(s string) RunnerOption {
	return func(r *SimpleRunner) {
		r.systemPrompt = s
	}
}

func WithCompactor(c *memory.Compactor) RunnerOption {
	return func(r *SimpleRunner) {
		r.compactor = c
	}
}

func WithSemanticStore(s *memory.SemanticStore) RunnerOption {
	return func(r *SimpleRunner) {
		r.semanticStore = s
	}
}

type SimpleRunner struct {
	provider      llm.Provider
	store         *history.Store
	memory        memory.Memory
	registry      *Registry
	tools         []llm.ToolDefinition
	systemPrompt  string
	compactor     *memory.Compactor
	semanticStore *memory.SemanticStore
}

func NewSimpleRunner(provider llm.Provider, store *history.Store, mem memory.Memory, registry *Registry, opts ...RunnerOption) *SimpleRunner {
	r := &SimpleRunner{
		provider:     provider,
		store:        store,
		memory:       mem,
		registry:     registry,
		systemPrompt: defaultSystemPrompt,
	}

	for _, opt := range opts {
		opt(r)
	}

	for _, t := range registry.All() {
		schema, _ := t.InputSchema().(map[string]any)
		r.tools = append(r.tools, llm.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  schema,
			Strict:      true,
		})
	}

	return r
}

func (r *SimpleRunner) Run(ctx context.Context, sessionID string, message string, emit func(Event)) error {
	ctx = ContextWithSessionID(ctx, sessionID)
	ctx = ContextWithEmit(ctx, emit)

	truncatedMsg := message
	if len(truncatedMsg) > 200 {
		truncatedMsg = truncatedMsg[:200]
	}
	ctx, span := trace.Tracer().Start(ctx, "agent.run",
		oteltrace.WithAttributes(
			attribute.String("openai.agents.agent.name", "pingu"),
			attribute.String("session.id", sessionID),
			attribute.String("user.message", truncatedMsg),
		),
	)
	defer span.End()

	sc := span.SpanContext()
	slog.Debug("agent.run span started", "trace_id", sc.TraceID(), "span_id", sc.SpanID())

	if err := r.store.EnsureSession(ctx, sessionID, "default"); err != nil {
		slog.Warn("failed to ensure session", "session_id", sessionID, "error", err)
	}

	var input []llm.InputItem
	var err error
	if cm, ok := r.memory.(memory.ContextualMemory); ok {
		slog.Debug("agent: using contextual memory recall", "session_id", sessionID)
		input, err = cm.RecallWithContext(ctx, sessionID, message)
	} else {
		slog.Debug("agent: using basic memory recall", "session_id", sessionID)
		input, err = r.memory.Recall(ctx, sessionID)
	}
	if err != nil {
		slog.Warn("failed to recall memory", "session_id", sessionID, "error", err)
		input = nil
	}
	slog.Debug("agent: memory recalled", "session_id", sessionID, "history_items", len(input))

	input = append(input,
		llm.NewMessage(r.systemPrompt, llm.RoleDeveloper),
		llm.NewMessage(message, llm.RoleUser),
	)
	slog.Debug("agent: input prepared", "total_items", len(input), "tools", len(r.tools))

	var resp *llm.Response
	iteration := 0

	for {
		var llmSpan oteltrace.Span
		var llmCtx context.Context
		llmCtx, llmSpan = trace.Tracer().Start(ctx, "llm.chat",
			oteltrace.WithAttributes(
				attribute.Int("llm.iteration", iteration),
			),
		)

		llmSC := llmSpan.SpanContext()
		slog.Debug("llm.chat span started",
			"trace_id", llmSC.TraceID(),
			"span_id", llmSC.SpanID(),
			"parent_span_id", sc.SpanID(),
			"iteration", iteration,
		)

		resp, err = r.provider.ChatStream(llmCtx, input, r.tools, func(token string) {})
		if err != nil {
			llmSpan.RecordError(err)
			llmSpan.SetStatus(codes.Error, err.Error())
			llmSpan.End()
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			emit(Event{Type: EventError, Data: err.Error()})
			return err
		}

		llmSpan.SetAttributes(
			attribute.String("llm.model", resp.Model),
			attribute.Int64("llm.input_tokens", resp.InputTokens),
			attribute.Int64("llm.output_tokens", resp.OutputTokens),
		)
		llmSpan.End()
		iteration++

		// Convert output items to input items for the next iteration.
		input = append(input, llm.OutputToInput(resp.Output)...)

		// Collect function calls from the response.
		var calls []llm.OutputItem
		for _, item := range resp.Output {
			if item.Type == "function_call" {
				calls = append(calls, item)
			}
		}

		// No function calls means the model is done.
		if len(calls) == 0 {
			break
		}

		// Execute tool calls in parallel.
		var wg sync.WaitGroup
		results := make([]llm.InputItem, len(calls))
		for i, call := range calls {
			wg.Add(1)
			go func(i int, call llm.OutputItem) {
				defer wg.Done()
				tool, ok := r.registry.Get(call.Name)
				if !ok {
					slog.Warn("unknown tool call", "name", call.Name)
					results[i] = llm.NewFunctionCallOutput(call.CallID, "error: unknown tool")
					return
				}

				traced := withTrace(tool)
				result, execErr := traced.Execute(ctx, call.Arguments)
				if execErr != nil {
					slog.Warn("tool execution failed", "name", call.Name, "error", execErr)
					results[i] = llm.NewFunctionCallOutput(call.CallID, "error: "+execErr.Error())
					return
				}
				results[i] = llm.NewFunctionCallOutput(call.CallID, result)
			}(i, call)
		}
		wg.Wait()
		input = append(input, results...)
	}

	if err := r.store.SaveTurn(ctx, sessionID, message, resp); err != nil {
		slog.Warn("failed to save turn", "session_id", sessionID, "error", err)
	}

	if r.semanticStore != nil {
		go func() {
			sid := sessionID
			if _, err := r.semanticStore.Store(context.Background(), &sid, "conversation", message); err != nil {
				slog.Warn("auto-save memory failed", "session_id", sessionID, "error", err)
			}
		}()
	}

	if r.compactor != nil {
		go r.compactor.MaybeCompact(context.Background(), sessionID)
	}

	emit(Event{Type: EventDone})
	return nil
}
