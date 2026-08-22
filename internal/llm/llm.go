// Package llm defines provider-neutral types for model requests, responses,
// and streaming events. Provider-specific wire formats never leak past the
// provider adapters.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// Role enumerates conversation roles.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall is a tool invocation requested by the model.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// Message is one conversation item.
type Message struct {
	Role       Role
	Content    string
	ToolCalls  []ToolCall // assistant messages only
	ToolCallID string     // tool result messages only
}

// ToolDef advertises a tool to the provider.
type ToolDef struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema; may be nil
}

// Usage reports token accounting.
type Usage struct {
	InputTokens  int64
	OutputTokens int64
}

// Request is a provider-neutral completion request.
type Request struct {
	Model    string
	System   string
	Messages []Message
	Tools    []ToolDef
}

// EventType enumerates provider stream events.
type EventType string

const (
	EventTextDelta     EventType = "text_delta"
	EventToolCallStart EventType = "tool_call_start"
	EventToolCallDelta EventType = "tool_call_arguments_delta"
	EventToolCallEnd   EventType = "tool_call_end"
	EventUsage         EventType = "usage"
)

// Event is one typed provider stream event.
type Event struct {
	Type           EventType
	Text           string // EventTextDelta
	ToolIndex      int    // tool assembly events
	ToolCallID     string // EventToolCallStart
	ToolName       string // EventToolCallStart
	ArgumentsDelta string // EventToolCallDelta
	Usage          Usage  // EventUsage
}

// Provider completes requests and streams typed events.
type Provider interface {
	// Stream starts a completion. It returns an error for request
	// construction or transport setup failures; failures that occur while
	// streaming surface from Stream.Next. The request context governs the
	// entire HTTP exchange: cancelling it cancels the stream.
	Stream(ctx context.Context, req Request) (Stream, error)
}

// Stream is a pull-based event stream. Next blocks until an event is
// available, the stream ends (io.EOF), the context is done, or a provider
// error occurs. Close releases the underlying transport; it is safe to call
// more than once.
type Stream interface {
	Next(ctx context.Context) (Event, error)
	Close() error
}

// ProviderError is a provider-side failure with a stable code.
type ProviderError struct {
	Provider string
	Code     string // e.g. "http_500", "malformed_stream", "request_failed"
	Err      error
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider %s: %s: %v", e.Provider, e.Code, e.Err)
}

func (e *ProviderError) Unwrap() error { return e.Err }

// NewProviderError builds a ProviderError.
func NewProviderError(provider, code string, err error) *ProviderError {
	return &ProviderError{Provider: provider, Code: code, Err: err}
}

// sliceStream adapts a fixed event slice to the Stream interface. It exists
// for tests and providers that produce whole responses.
type SliceStream struct {
	events []Event
	off    int
	closed bool
}

// NewSliceStream builds a stream over events.
func NewSliceStream(events []Event) *SliceStream { return &SliceStream{events: events} }

func (s *SliceStream) Next(_ context.Context) (Event, error) {
	if s.closed || s.off >= len(s.events) {
		return Event{}, io.EOF
	}
	ev := s.events[s.off]
	s.off++
	return ev, nil
}

func (s *SliceStream) Close() error {
	s.closed = true
	return nil
}
