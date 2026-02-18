package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"pingu/internal/agent"
	"pingu/internal/memory"
)

// MemoryStore is a tool that lets the agent persist memories across sessions.
type MemoryStore struct {
	store *memory.SemanticStore
}

func NewMemoryStore(store *memory.SemanticStore) *MemoryStore {
	return &MemoryStore{store: store}
}

func (m *MemoryStore) Name() string { return "memory_store" }
func (m *MemoryStore) Description() string {
	return "Store a memory for later recall. Use category 'core' for cross-session facts (preferences, identity), 'daily' for daily context, or 'conversation' for session-scoped notes."
}

func (m *MemoryStore) InputSchema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "The content to remember",
			},
			"category": map[string]any{
				"type":        "string",
				"enum":        []string{"core", "daily", "conversation"},
				"description": "Memory category: 'core' for persistent cross-session, 'daily' for daily context, 'conversation' for session-scoped",
			},
		},
		"required":             []string{"content", "category"},
		"additionalProperties": false,
	}
}

func (m *MemoryStore) Execute(ctx context.Context, input string) (string, error) {
	var args struct {
		Content  string `json:"content"`
		Category string `json:"category"`
	}
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		return "", fmt.Errorf("parsing memory_store input: %w", err)
	}

	// 'core' and 'daily' are global (no session_id); 'conversation' is session-scoped.
	var sessionID *string
	if args.Category == "conversation" {
		sid := agent.SessionIDFromContext(ctx)
		if sid != "" {
			sessionID = &sid
		}
	}

	id, err := m.store.Store(ctx, sessionID, args.Category, args.Content)
	if err != nil {
		return "", fmt.Errorf("storing memory: %w", err)
	}

	return fmt.Sprintf("Memory stored (id=%d, category=%s)", id, args.Category), nil
}
