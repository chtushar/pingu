package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pingu/internal/agent"
)

const maxDelegationDepth = 3

// Delegate is a tool that spawns a scoped sub-agent to handle a task.
type Delegate struct {
	factory *agent.RunnerFactory
}

func NewDelegate(factory *agent.RunnerFactory) *Delegate {
	return &Delegate{factory: factory}
}

func (d *Delegate) Name() string        { return "delegate" }
func (d *Delegate) Description() string { return "Delegate a task to a specialized sub-agent" }

func (d *Delegate) InputSchema() any {
	// Build enum from registered profiles.
	profiles := d.factory.Profiles()
	profileEnum := make([]any, len(profiles))
	for i, p := range profiles {
		profileEnum[i] = p
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent": map[string]any{
				"type":        "string",
				"description": "Name of the agent profile to delegate to",
				"enum":        profileEnum,
			},
			"task": map[string]any{
				"type":        "string",
				"description": "The task description for the sub-agent",
			},
		},
		"required":             []string{"agent", "task"},
		"additionalProperties": false,
	}
}

func (d *Delegate) Execute(ctx context.Context, input string) (string, error) {
	var args struct {
		Agent string `json:"agent"`
		Task  string `json:"task"`
	}
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		return "", fmt.Errorf("parsing delegate input: %w", err)
	}

	// Recursion guard.
	depth := agent.DelegationDepthFromContext(ctx)
	if depth >= maxDelegationDepth {
		return "", fmt.Errorf("maximum delegation depth (%d) exceeded", maxDelegationDepth)
	}

	runner, err := d.factory.Build(args.Agent)
	if err != nil {
		return "", fmt.Errorf("building sub-agent: %w", err)
	}

	// Derive sub-session ID from parent session.
	parentSession := agent.SessionIDFromContext(ctx)
	subSession := fmt.Sprintf("%s:delegate:%s", parentSession, args.Agent)

	// Increment delegation depth.
	subCtx := agent.ContextWithDelegationDepth(ctx, depth+1)

	// Capture output tokens into a buffer — the sub-agent does NOT emit to the user.
	var buf strings.Builder
	localEmit := func(e agent.Event) {
		if e.Type == agent.EventToken {
			if s, ok := e.Data.(string); ok {
				buf.WriteString(s)
			}
		}
	}

	if err := runner.Run(subCtx, subSession, args.Task, localEmit); err != nil {
		return "", fmt.Errorf("sub-agent %s failed: %w", args.Agent, err)
	}

	result := buf.String()
	if result == "" {
		return "(sub-agent produced no output)", nil
	}
	return result, nil
}
