package agent

import (
	"context"
	"log/slog"
	"pingu/internal/history"
	"pingu/internal/llm"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

type SimpleRunner struct {
	provider llm.Provider
	store    *history.Store
	registry *Registry
	tools    []responses.ToolUnionParam
}

func NewSimpleRunner(provider llm.Provider, store *history.Store, registry *Registry) *SimpleRunner {
	var tools []responses.ToolUnionParam
	for _, t := range registry.All() {
		schema, _ := t.InputSchema().(map[string]any)
		tools = append(tools, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        t.Name(),
				Description: openai.String(t.Description()),
				Parameters:  schema,
				Strict:      openai.Bool(true),
			},
		})
	}

	return &SimpleRunner{
		provider: provider,
		store:    store,
		registry: registry,
		tools:    tools,
	}
}

func (r *SimpleRunner) Run(ctx context.Context, sessionID string, message string, emit func(Event)) error {
	// Inject emit callback into tools that need it.
	for _, t := range r.registry.All() {
		if es, ok := t.(EmitSetter); ok {
			es.SetEmit(emit)
		}
	}

	if err := r.store.EnsureSession(ctx, sessionID, "default"); err != nil {
		slog.Warn("failed to ensure session", "session_id", sessionID, "error", err)
	}

	input, err := r.store.LoadInputHistory(ctx, sessionID)
	if err != nil {
		slog.Warn("failed to load history", "session_id", sessionID, "error", err)
		input = nil
	}

	input = append(input,
		responses.ResponseInputItemParamOfMessage(
			"You must use the message tool to communicate with the user. Do not produce raw text output.",
			"developer",
		),
		responses.ResponseInputItemParamOfMessage(message, "user"),
	)

	var resp *responses.Response

	for {
		resp, err = r.provider.ChatStream(ctx, input, r.tools, func(token string) {
			// Ignore streaming text tokens; output goes through the message tool.
		})
		if err != nil {
			emit(Event{Type: EventError, Data: err.Error()})
			return err
		}

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

		// Execute each function call and append results.
		for _, call := range calls {
			fc := call.AsFunctionCall()
			tool, ok := r.registry.Get(fc.Name)
			if !ok {
				slog.Warn("unknown tool call", "name", fc.Name)
				input = append(input, responses.ResponseInputItemParamOfFunctionCallOutput(fc.CallID, "error: unknown tool"))
				continue
			}

			result, execErr := tool.Execute(ctx, fc.Arguments)
			if execErr != nil {
				slog.Warn("tool execution failed", "name", fc.Name, "error", execErr)
				input = append(input, responses.ResponseInputItemParamOfFunctionCallOutput(fc.CallID, "error: "+execErr.Error()))
				continue
			}

			input = append(input, responses.ResponseInputItemParamOfFunctionCallOutput(fc.CallID, result))
		}
	}

	if err := r.store.SaveTurn(ctx, sessionID, message, resp); err != nil {
		slog.Warn("failed to save turn", "session_id", sessionID, "error", err)
	}

	emit(Event{Type: EventDone})
	return nil
}
