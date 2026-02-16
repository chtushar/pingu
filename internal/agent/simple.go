package agent

import (
	"context"
	"log/slog"
	"pingu/internal/history"
	"pingu/internal/llm"

	"github.com/openai/openai-go/v3/responses"
)

type SimpleRunner struct {
	provider llm.Provider
	store    *history.Store
}

func NewSimpleRunner(provider llm.Provider, store *history.Store) *SimpleRunner {
	return &SimpleRunner{provider: provider, store: store}
}

func (r *SimpleRunner) Run(ctx context.Context, sessionID string, message string, emit func(Event)) error {
	if err := r.store.EnsureSession(ctx, sessionID, "default"); err != nil {
		slog.Warn("failed to ensure session", "session_id", sessionID, "error", err)
	}

	input, err := r.store.LoadInputHistory(ctx, sessionID)
	if err != nil {
		slog.Warn("failed to load history", "session_id", sessionID, "error", err)
		input = nil
	}

	input = append(input, responses.ResponseInputItemParamOfMessage(message, "user"))

	resp, err := r.provider.ChatStream(ctx, input, nil, func(token string) {
		emit(Event{Type: EventToken, Data: token})
	})
	if err != nil {
		emit(Event{Type: EventError, Data: err.Error()})
		return err
	}

	if err := r.store.SaveTurn(ctx, sessionID, message, resp); err != nil {
		slog.Warn("failed to save turn", "session_id", sessionID, "error", err)
	}

	emit(Event{Type: EventDone, Data: resp.OutputText()})
	return nil
}
