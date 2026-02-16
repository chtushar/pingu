package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"pingu/internal/agent"
)

type Message struct {
	emit func(agent.Event)
}

func (m *Message) Name() string        { return "message" }
func (m *Message) Description() string { return "Send a message to the user" }

func (m *Message) InputSchema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type": "string",
			},
		},
		"required":             []string{"text"},
		"additionalProperties": false,
	}
}

func (m *Message) SetEmit(emit func(agent.Event)) {
	m.emit = emit
}

func (m *Message) Execute(ctx context.Context, input string) (string, error) {
	var args struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		return "", fmt.Errorf("parsing message input: %w", err)
	}

	slog.Debug("message: sending", "text_len", len(args.Text))

	if m.emit != nil {
		m.emit(agent.Event{Type: agent.EventToken, Data: args.Text})
	}

	slog.Debug("message: sent")
	return "message sent", nil
}
