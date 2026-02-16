package agent

import (
	"context"
	"pingu/internal/llm"

	"github.com/openai/openai-go/v3/responses"
)

type SimpleRunner struct {
	provider llm.Provider
}

func NewSimpleRunner(provider llm.Provider) *SimpleRunner {
	return &SimpleRunner{provider: provider}
}

func (r *SimpleRunner) Run(ctx context.Context, sessionID string, message string, emit func(Event)) error {
	input := []responses.ResponseInputItemUnionParam{
		responses.ResponseInputItemParamOfMessage(message, "user"),
	}

	resp, err := r.provider.ChatStream(ctx, input, nil, func(token string) {
		emit(Event{Type: EventToken, Data: token})
	})
	if err != nil {
		emit(Event{Type: EventError, Data: err.Error()})
		return err
	}

	emit(Event{Type: EventDone, Data: resp.OutputText()})
	return nil
}
