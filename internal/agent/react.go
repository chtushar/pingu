package agent

import (
	"context"
	"log/slog"
	"pingu/internal/history"
	"pingu/internal/llm"
	"pingu/internal/memory"
	"pingu/internal/trace"
	"sync"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const defaultReActSystemPrompt = "You must use the message tool to communicate with the user. Do not produce raw text output."

type ReactOption func(*ReactRunner)

func WithReActSystemPrompt(s string) ReactOption {
	return func(r *ReactRunner) { r.systemPrompt = s }
}

func WithReActCompactor(c *memory.Compactor) ReactOption {
	return func(r *ReactRunner) { r.compactor = c }
}

func WithReActSemanticStore(s *memory.SemanticStore) ReactOption {
	return func(r *ReactRunner) { r.semanticStore = s }
}

// ReactRunner implements a ReAct (Reason + Act) agent loop.
// The agent keeps thinking and acting until it decides the task is done
// (i.e. the LLM returns no more tool calls) or the context is cancelled.
type ReactRunner struct {
	provider      llm.Provider
	store         *history.Store
	memory        memory.Memory
	registry      *Registry
	tools         []responses.ToolUnionParam
	systemPrompt  string
	compactor     *memory.Compactor
	semanticStore *memory.SemanticStore
}

func NewReactRunner(provider llm.Provider, store *history.Store, mem memory.Memory, registry *Registry, opts ...ReactOption) *ReactRunner {
	r := &ReactRunner{
		provider:     provider,
		store:        store,
		memory:       mem,
		registry:     registry,
		systemPrompt: defaultReActSystemPrompt,
	}

	for _, opt := range opts {
		opt(r)
	}

	for _, t := range registry.All() {
		schema, _ := t.InputSchema().(map[string]any)
		r.tools = append(r.tools, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        t.Name(),
				Description: openai.String(t.Description()),
				Parameters:  schema,
				Strict:      openai.Bool(true),
			},
		})
	}

	return r
}

func (r *ReactRunner) Run(ctx context.Context, sessionID string, message string, emit func(Event)) error {
	ctx = ContextWithSessionID(ctx, sessionID)
	ctx = ContextWithEmit(ctx, emit)

	truncatedMsg := message
	if len(truncatedMsg) > 200 {
		truncatedMsg = truncatedMsg[:200]
	}
	ctx, span := trace.Tracer().Start(ctx, "agent.react.run",
		oteltrace.WithAttributes(
			attribute.String("openai.agents.agent.name", "pingu"),
			attribute.String("session.id", sessionID),
			attribute.String("user.message", truncatedMsg),
		),
	)
	defer span.End()

	sc := span.SpanContext()
	slog.Debug("agent.react.run span started", "trace_id", sc.TraceID(), "span_id", sc.SpanID())

	if err := r.store.EnsureSession(ctx, sessionID, "default"); err != nil {
		slog.Warn("failed to ensure session", "session_id", sessionID, "error", err)
	}

	input, err := r.recall(ctx, sessionID, message)
	if err != nil {
		slog.Warn("failed to recall memory", "session_id", sessionID, "error", err)
		input = nil
	}
	slog.Debug("agent.react: memory recalled", "session_id", sessionID, "history_items", len(input))

	input = append(input,
		responses.ResponseInputItemParamOfMessage(r.systemPrompt, "developer"),
		responses.ResponseInputItemParamOfMessage(message, "user"),
	)

	resp, err := r.loop(ctx, span.SpanContext(), input, emit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	r.persist(ctx, sessionID, message, resp)

	emit(Event{Type: EventDone})
	return nil
}

// loop is the core ReAct cycle: Plan → Execute → Observe, repeated until the
// agent decides the task is complete or the context is cancelled.
func (r *ReactRunner) loop(ctx context.Context, parentSC oteltrace.SpanContext, input []responses.ResponseInputItemUnionParam, emit func(Event)) (*responses.Response, error) {
	var resp *responses.Response
	iteration := 0

	for {
		if err := ctx.Err(); err != nil {
			emit(Event{Type: EventError, Data: "request cancelled"})
			return nil, err
		}

		// — Plan: call the LLM without tools to reason about what to do next. —
		planResp, err := r.chat(ctx, parentSC, iteration, "plan", input, nil)
		if err != nil {
			emit(Event{Type: EventError, Data: err.Error()})
			return nil, err
		}
		iteration++

		// Add the plan to the conversation so the execute step can see it.
		input = append(input, history.OutputToInput(planResp.Output)...)

		// — Execute: call the LLM with tools to act on the plan. —
		resp, err = r.chat(ctx, parentSC, iteration, "execute", input, r.tools)
		if err != nil {
			emit(Event{Type: EventError, Data: err.Error()})
			return nil, err
		}
		iteration++

		input = append(input, history.OutputToInput(resp.Output)...)

		// Collect tool calls from the response.
		var calls []responses.ResponseOutputItemUnion
		for _, item := range resp.Output {
			if item.Type == "function_call" {
				calls = append(calls, item)
			}
		}

		// No tool calls — the agent considers the task done.
		if len(calls) == 0 {
			return resp, nil
		}

		// — Observe: execute tools and feed results back. —
		results := r.act(ctx, calls, emit)
		input = append(input, results...)
	}
}

// chat makes a single LLM call with tracing. Pass nil tools for a plan-only call.
func (r *ReactRunner) chat(ctx context.Context, parentSC oteltrace.SpanContext, iteration int, phase string, input []responses.ResponseInputItemUnionParam, tools []responses.ToolUnionParam) (*responses.Response, error) {
	llmCtx, llmSpan := trace.Tracer().Start(ctx, "llm."+phase,
		oteltrace.WithAttributes(attribute.Int("llm.iteration", iteration)),
	)

	slog.Debug("llm."+phase+" span started",
		"trace_id", llmSpan.SpanContext().TraceID(),
		"span_id", llmSpan.SpanContext().SpanID(),
		"parent_span_id", parentSC.SpanID(),
		"iteration", iteration,
	)

	resp, err := r.provider.ChatStream(llmCtx, input, tools, func(token string) {})
	if err != nil {
		llmSpan.RecordError(err)
		llmSpan.SetStatus(codes.Error, err.Error())
		llmSpan.End()
		return nil, err
	}

	llmSpan.SetAttributes(
		attribute.String("llm.model", string(resp.Model)),
		attribute.Int64("llm.input_tokens", resp.Usage.InputTokens),
		attribute.Int64("llm.output_tokens", resp.Usage.OutputTokens),
	)
	llmSpan.End()
	return resp, nil
}

// act executes tool calls in parallel, emitting events for each, and returns
// the results formatted as input items for the next LLM turn.
func (r *ReactRunner) act(ctx context.Context, calls []responses.ResponseOutputItemUnion, emit func(Event)) []responses.ResponseInputItemUnionParam {
	for _, call := range calls {
		fc := call.AsFunctionCall()
		emit(Event{Type: EventToolCall, Data: map[string]string{
			"name":      fc.Name,
			"arguments": fc.Arguments,
		}})
	}

	var wg sync.WaitGroup
	results := make([]responses.ResponseInputItemUnionParam, len(calls))

	for i, call := range calls {
		wg.Add(1)
		go func(i int, call responses.ResponseOutputItemUnion) {
			defer wg.Done()
			fc := call.AsFunctionCall()

			tool, ok := r.registry.Get(fc.Name)
			if !ok {
				slog.Warn("unknown tool call", "name", fc.Name)
				results[i] = responses.ResponseInputItemParamOfFunctionCallOutput(fc.CallID, "error: unknown tool")
				emit(Event{Type: EventToolResult, Data: map[string]string{
					"name":    fc.Name,
					"content": "error: unknown tool",
				}})
				return
			}

			traced := withTrace(tool)
			result, err := traced.Execute(ctx, fc.Arguments)
			if err != nil {
				slog.Warn("tool execution failed", "name", fc.Name, "error", err)
				errMsg := "error: " + err.Error()
				results[i] = responses.ResponseInputItemParamOfFunctionCallOutput(fc.CallID, errMsg)
				emit(Event{Type: EventToolResult, Data: map[string]string{
					"name":    fc.Name,
					"content": errMsg,
				}})
				return
			}

			results[i] = responses.ResponseInputItemParamOfFunctionCallOutput(fc.CallID, result)
			emit(Event{Type: EventToolResult, Data: map[string]string{
				"name":    fc.Name,
				"content": result,
			}})
		}(i, call)
	}

	wg.Wait()
	return results
}

// recall loads conversation history and relevant memories.
func (r *ReactRunner) recall(ctx context.Context, sessionID, message string) ([]responses.ResponseInputItemUnionParam, error) {
	if cm, ok := r.memory.(memory.ContextualMemory); ok {
		slog.Debug("agent.react: using contextual memory recall", "session_id", sessionID)
		return cm.RecallWithContext(ctx, sessionID, message)
	}
	slog.Debug("agent.react: using basic memory recall", "session_id", sessionID)
	return r.memory.Recall(ctx, sessionID)
}

// persist saves the turn and triggers background memory operations.
func (r *ReactRunner) persist(ctx context.Context, sessionID, message string, resp *responses.Response) {
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
}
