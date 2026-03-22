package llm

import "context"

// Provider is the interface for LLM backends. Implementations convert between
// native Pingu types and their provider-specific wire formats internally.
type Provider interface {
	ChatStream(ctx context.Context, input []InputItem, tools []ToolDefinition, onToken func(string)) (*Response, error)
}
