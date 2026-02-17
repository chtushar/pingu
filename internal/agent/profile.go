package agent

// AgentProfile defines a named agent configuration with a scoped toolset.
type AgentProfile struct {
	Name         string
	SystemPrompt string
	Tools        []string // tool names; empty = all tools
}
