package mock

import (
	"context"
	"pingu/internal/llm"
)

// StaticMemory implements memory.Memory by returning pre-configured items.
type StaticMemory struct {
	Items []llm.InputItem
}

func (m *StaticMemory) Recall(ctx context.Context, sessionID string) ([]llm.InputItem, error) {
	return m.Items, nil
}
