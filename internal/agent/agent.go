package agent

import "context"

type EventType string

const (
	EventToken      EventType = "token"
	EventToolCall   EventType = "tool_call"
	EventToolResult EventType = "tool_result"
	EventDone       EventType = "done"
	EventError      EventType = "error"
)

type Event struct {
	Type EventType `json:"type"`
	Data any       `json:"data"`
}

type Runner interface {
	Run(ctx context.Context, sessionID string, message string, emit func(Event)) error
}
