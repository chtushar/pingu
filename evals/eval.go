package evals

import (
	"context"
	"pingu/internal/agent"
	"pingu/internal/llm"
)

// EvalCase is a single test case loaded from YAML.
type EvalCase struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`

	// Input
	UserMessage string        `yaml:"user_message"`
	History     []HistoryItem `yaml:"history,omitempty"`
	Memories    []MemoryItem  `yaml:"memories,omitempty"`

	// Mock LLM scripted responses — each entry is one LLM turn in the ReAct loop.
	Script []ScriptedTurn `yaml:"script,omitempty"`

	// Agent config
	Tools        []string `yaml:"tools,omitempty"` // tool names to register; empty = all
	SystemPrompt string   `yaml:"system_prompt,omitempty"`

	// Grading
	Graders []GraderConfig `yaml:"graders"`

	// Provider selection: "mock" (default) or "real"
	Provider string `yaml:"provider,omitempty"`
}

type HistoryItem struct {
	Role    string `yaml:"role"`
	Content string `yaml:"content"`
}

type MemoryItem struct {
	Category string `yaml:"category"`
	Content  string `yaml:"content"`
}

// ScriptedTurn defines what the mock LLM returns for one call to ChatStream.
type ScriptedTurn struct {
	Output []ScriptedOutput `yaml:"output"`
}

type ScriptedOutput struct {
	Type      string `yaml:"type"` // "message" or "function_call"
	Content   string `yaml:"content,omitempty"`
	Name      string `yaml:"name,omitempty"`
	CallID    string `yaml:"call_id,omitempty"`
	Arguments string `yaml:"arguments,omitempty"`
}

// GraderConfig selects and configures a grader from YAML.
type GraderConfig struct {
	Type     string   `yaml:"type"` // "match", "contains", "tool_used", "tool_sequence"
	Expected string   `yaml:"expected,omitempty"`
	Tools    []string `yaml:"tools,omitempty"`
	Regex    bool     `yaml:"regex,omitempty"`
}

// ToolCallRecord captures a single tool invocation during an eval run.
type ToolCallRecord struct {
	Name      string
	Arguments string
	Result    string
	Error     string
}

// Transcript captures everything that happened during an eval run.
type Transcript struct {
	FinalResponse string           // concatenated text from EventToken events
	ToolCalls     []ToolCallRecord // ordered list of all tool calls
	Events        []agent.Event    // raw event stream
}

// EvalResult is the outcome of running one case through one grader.
type EvalResult struct {
	CaseName   string
	GraderType string
	Passed     bool
	Score      float64 // 1.0 = pass, 0.0 = fail
	Message    string
}

// Grader scores a transcript against expectations.
type Grader interface {
	Grade(ctx context.Context, transcript Transcript) EvalResult
}

// ToOutputItems converts scripted outputs to native llm.OutputItem slice.
func (t ScriptedTurn) ToOutputItems() []llm.OutputItem {
	items := make([]llm.OutputItem, len(t.Output))
	for i, o := range t.Output {
		items[i] = llm.OutputItem{
			Type:      o.Type,
			Content:   o.Content,
			Name:      o.Name,
			CallID:    o.CallID,
			Arguments: o.Arguments,
		}
	}
	return items
}
