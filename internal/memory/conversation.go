package memory

import (
	"context"
	"pingu/internal/history"

	"pingu/internal/llm"
)

// ConversationMemory recalls the full conversation history for a session.
type ConversationMemory struct {
	store *history.Store
}

func NewConversationMemory(store *history.Store) *ConversationMemory {
	return &ConversationMemory{store: store}
}

func (m *ConversationMemory) Recall(ctx context.Context, sessionID string) ([]llm.InputItem, error) {
	return m.store.LoadInputHistory(ctx, sessionID)
}
