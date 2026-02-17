package agent

import (
	"context"
	"log/slog"
	"pingu/internal/history"
	"pingu/internal/llm"
	"pingu/internal/trace"
	"sync"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
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

type SimpleRunner struct {
	provider     llm.Provider
	store        *history.Store
	registry     *Registry
	tools        []responses.ToolUnionParam
	systemPrompt string
}

func NewSimpleRunner(provider llm.Provider, store *history.Store, registry *Registry, opts ...RunnerOption) *SimpleRunner {
	r := &SimpleRunner{
		provider:     provider,
		store:        store,
		registry:     registry,
		systemPrompt: defaultSystemPrompt,
	}

	for _, opt := range opts {
		opt(r)
	}

	// Build tool params list from the registry without mutating it.
	// Tracing is applied at execution time, not here.
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

	input, err := r.store.LoadInputHistory(ctx, sessionID)
	if err != nil {
		slog.Warn("failed to load history", "session_id", sessionID, "error", err)
		input = nil
	}

	input = append(input,
		responses.ResponseInputItemParamOfMessage(r.systemPrompt, "developer"),
		responses.ResponseInputItemParamOfMessage(message, "user"),
	)

	var resp *responses.Response
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

		resp, err = r.provider.ChatStream(llmCtx, input, r.tools, func(token string) {
			// Ignore streaming text tokens; output goes through the message tool.
		})
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
			attribute.String("llm.model", string(resp.Model)),
			attribute.Int64("llm.input_tokens", resp.Usage.InputTokens),
			attribute.Int64("llm.output_tokens", resp.Usage.OutputTokens),
		)
		llmSpan.End()
		iteration++

		// Convert output items to input items for the next iteration.
		outputAsInput := history.OutputToInput(resp.Output)
		input = append(input, outputAsInput...)

		// Collect function calls from the response.
		var calls []responses.ResponseOutputItemUnion
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
					return
				}

				// Use traced execution — wrap inline to preserve tracing without mutating registry.
				traced := withTrace(tool)
				result, execErr := traced.Execute(ctx, fc.Arguments)
				if execErr != nil {
					slog.Warn("tool execution failed", "name", fc.Name, "error", execErr)
					results[i] = responses.ResponseInputItemParamOfFunctionCallOutput(fc.CallID, "error: "+execErr.Error())
					return
				}
				results[i] = responses.ResponseInputItemParamOfFunctionCallOutput(fc.CallID, result)
			}(i, call)
		}
		wg.Wait()
		input = append(input, results...)
	}

	if err := r.store.SaveTurn(ctx, sessionID, message, resp); err != nil {
		slog.Warn("failed to save turn", "session_id", sessionID, "error", err)
	}

	emit(Event{Type: EventDone})
	return nil
}
