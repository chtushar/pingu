package evals

import (
	"context"
	"fmt"

	"pingu/evals/mock"
	"pingu/internal/agent"
	"pingu/internal/db"
	"pingu/internal/history"
	"pingu/internal/llm"
	"pingu/internal/tools"
)

// RunCase executes a single eval case and returns grading results.
func RunCase(ctx context.Context, tc EvalCase) ([]EvalResult, error) {
	// 1. Set up in-memory database.
	database, err := db.OpenInMemory()
	if err != nil {
		return nil, fmt.Errorf("open in-memory db: %w", err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	store := history.NewStore(database)

	// 2. Set up LLM provider.
	var provider llm.Provider
	switch tc.Provider {
	case "real":
		return nil, fmt.Errorf("real provider not yet supported in evals")
	default:
		provider = mock.NewScriptedProvider(scriptToMockTurns(tc.Script))
	}

	// 3. Set up memory.
	var mem mock.StaticMemory
	for _, h := range tc.History {
		mem.Items = append(mem.Items, llm.NewMessage(h.Content, llm.Role(h.Role)))
	}

	// 4. Build tool registry.
	registry := agent.NewRegistry()
	registry.Register(&tools.Message{})

	wantTools := make(map[string]bool)
	for _, name := range tc.Tools {
		wantTools[name] = true
	}

	allTools := map[string]agent.Tool{
		"shell": &tools.Shell{},
		"file":  &tools.File{},
	}
	for name, tool := range allTools {
		if len(wantTools) == 0 || wantTools[name] {
			registry.Register(tool)
		}
	}

	// 5. Build runner.
	var opts []agent.ReactOption
	if tc.SystemPrompt != "" {
		opts = append(opts, agent.WithReActSystemPrompt(tc.SystemPrompt))
	}

	runner := agent.NewReactRunner(provider, store, &mem, registry, opts...)

	// 6. Run the agent and collect transcript.
	sessionID := fmt.Sprintf("eval:%s", tc.Name)
	var transcript Transcript

	err = runner.Run(ctx, sessionID, tc.UserMessage, func(e agent.Event) {
		transcript.Events = append(transcript.Events, e)

		switch e.Type {
		case agent.EventToken:
			if s, ok := e.Data.(string); ok {
				transcript.FinalResponse += s
			}
		case agent.EventToolCall:
			if m, ok := e.Data.(map[string]string); ok {
				transcript.ToolCalls = append(transcript.ToolCalls, ToolCallRecord{
					Name:      m["name"],
					Arguments: m["arguments"],
				})
			}
		case agent.EventToolResult:
			if m, ok := e.Data.(map[string]string); ok {
				for i := len(transcript.ToolCalls) - 1; i >= 0; i-- {
					if transcript.ToolCalls[i].Name == m["name"] && transcript.ToolCalls[i].Result == "" {
						transcript.ToolCalls[i].Result = m["content"]
						break
					}
				}
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("agent run failed: %w", err)
	}

	// 7. Grade the transcript.
	var results []EvalResult
	for _, cfg := range tc.Graders {
		g := FromConfig(cfg)
		result := g.Grade(ctx, transcript)
		result.CaseName = tc.Name
		results = append(results, result)
	}

	return results, nil
}

// scriptToMockTurns converts YAML ScriptedTurns to mock.Turn slices.
func scriptToMockTurns(script []ScriptedTurn) []mock.Turn {
	turns := make([]mock.Turn, len(script))
	for i, st := range script {
		turns[i] = mock.Turn{Output: st.ToOutputItems()}
	}
	return turns
}
