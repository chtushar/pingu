package memory

import (
	"context"
	"pingu/internal/history"

	"github.com/openai/openai-go/v3/responses"
)

// ConversationMemory recalls the full conversation history for a session.
type ConversationMemory struct {
	store *history.Store
}

func NewConversationMemory(store *history.Store) *ConversationMemory {
	return &ConversationMemory{store: store}
}

func (m *ConversationMemory) Recall(ctx context.Context, sessionID string) ([]responses.ResponseInputItemUnionParam, error) {
	return m.store.LoadInputHistory(ctx, sessionID)
}
