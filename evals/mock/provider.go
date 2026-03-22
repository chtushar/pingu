package mock

import (
	"context"
	"pingu/internal/llm"
	"sync"
)

// Turn represents one scripted LLM response.
type Turn struct {
	Output []llm.OutputItem
}

// ScriptedProvider implements llm.Provider by returning pre-scripted responses.
// Each call to ChatStream pops the next Turn from the script.
// When the script is exhausted, it returns an empty response (terminating the agent loop).
type ScriptedProvider struct {
	turns   []Turn
	callIdx int
	mu      sync.Mutex
}

func NewScriptedProvider(turns []Turn) *ScriptedProvider {
	return &ScriptedProvider{turns: turns}
}

func (p *ScriptedProvider) ChatStream(
	ctx context.Context,
	input []llm.InputItem,
	tools []llm.ToolDefinition,
	onToken func(string),
) (*llm.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.callIdx >= len(p.turns) {
		return &llm.Response{Model: "mock"}, nil
	}

	turn := p.turns[p.callIdx]
	p.callIdx++

	// Simulate streaming for message-type outputs.
	for _, item := range turn.Output {
		if item.Type == "message" && item.Content != "" && onToken != nil {
			onToken(item.Content)
		}
	}

	return &llm.Response{
		Model:  "mock",
		Output: turn.Output,
	}, nil
}
