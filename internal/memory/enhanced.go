package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"pingu/internal/history"
	"pingu/internal/llm"
)

// EnhancedMemory loads conversation history and auto-injects relevant memories
// from the semantic store as a developer message.
type EnhancedMemory struct {
	store      *history.Store
	searcher   *HybridSearcher
	maxResults int
}

func NewEnhancedMemory(store *history.Store, searcher *HybridSearcher, maxResults int) *EnhancedMemory {
	if maxResults <= 0 {
		maxResults = 5
	}
	return &EnhancedMemory{
		store:      store,
		searcher:   searcher,
		maxResults: maxResults,
	}
}

// Recall returns the full conversation history (backward-compatible).
func (m *EnhancedMemory) Recall(ctx context.Context, sessionID string) ([]llm.InputItem, error) {
	return m.store.LoadInputHistory(ctx, sessionID)
}

// RecallWithContext loads conversation history and prepends relevant memories.
func (m *EnhancedMemory) RecallWithContext(ctx context.Context, sessionID, userMessage string) ([]llm.InputItem, error) {
	items, err := m.store.LoadInputHistory(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	results, err := m.searcher.Search(ctx, userMessage, sessionID, m.maxResults)
	if err != nil {
		slog.Debug("enhanced memory search failed", "error", err)
		return items, nil
	}

	if len(results) == 0 {
		return items, nil
	}

	var b strings.Builder
	b.WriteString("[Relevant memories]\n")
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "- [%s] %s", r.Category, r.Content)
	}

	memoryMsg := llm.NewMessage(b.String(), llm.RoleDeveloper)
	// Prepend memory context before conversation history.
	return append([]llm.InputItem{memoryMsg}, items...), nil
}
