package memory

import (
	"context"

	"pingu/internal/llm"
)

// Memory recalls prior context for a session to feed into the LLM.
type Memory interface {
	Recall(ctx context.Context, sessionID string) ([]llm.InputItem, error)
}

// ContextualMemory extends Memory with message-aware recall that can inject
// relevant memories based on the current user message.
type ContextualMemory interface {
	Memory
	RecallWithContext(ctx context.Context, sessionID, userMessage string) ([]llm.InputItem, error)
}
