package agent

import (
	"fmt"
	"pingu/internal/history"
	"pingu/internal/llm"
)

// RunnerFactory builds scoped runners from agent profiles.
type RunnerFactory struct {
	provider       llm.Provider
	store          *history.Store
	globalRegistry *Registry
	profiles       map[string]*AgentProfile
}

func NewRunnerFactory(provider llm.Provider, store *history.Store, registry *Registry, profiles map[string]*AgentProfile) *RunnerFactory {
	return &RunnerFactory{
		provider:       provider,
		store:          store,
		globalRegistry: registry,
		profiles:       profiles,
	}
}

// Build creates a new SimpleRunner scoped to the given profile.
func (f *RunnerFactory) Build(profileName string) (Runner, error) {
	profile, ok := f.profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("unknown agent profile: %s", profileName)
	}

	registry := f.globalRegistry.Scope(profile.Tools)

	var opts []RunnerOption
	if profile.SystemPrompt != "" {
		opts = append(opts, WithSystemPrompt(profile.SystemPrompt))
	}

	return NewSimpleRunner(f.provider, f.store, registry, opts...), nil
}

// Profiles returns the names of all registered profiles.
func (f *RunnerFactory) Profiles() []string {
	names := make([]string, 0, len(f.profiles))
	for name := range f.profiles {
		names = append(names, name)
	}
	return names
}
