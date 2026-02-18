package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pingu/internal/agent"
	"pingu/internal/memory"
)

// MemoryRecall is a tool that lets the agent search stored memories.
type MemoryRecall struct {
	searcher *memory.HybridSearcher
}

func NewMemoryRecall(searcher *memory.HybridSearcher) *MemoryRecall {
	return &MemoryRecall{searcher: searcher}
}

func (m *MemoryRecall) Name() string { return "memory_recall" }
func (m *MemoryRecall) Description() string {
	return "Search stored memories by keyword and semantic similarity. Returns the most relevant memories."
}

func (m *MemoryRecall) InputSchema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query to find relevant memories",
			},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
}

func (m *MemoryRecall) Execute(ctx context.Context, input string) (string, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		return "", fmt.Errorf("parsing memory_recall input: %w", err)
	}

	sessionID := agent.SessionIDFromContext(ctx)
	results, err := m.searcher.Search(ctx, args.Query, sessionID, 10)
	if err != nil {
		return "", fmt.Errorf("searching memories: %w", err)
	}

	if len(results) == 0 {
		return "No relevant memories found.", nil
	}

	var b strings.Builder
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n---\n")
		}
		fmt.Fprintf(&b, "[%s] (score=%.2f) %s", r.Category, r.Score, r.Content)
	}
	return b.String(), nil
}
