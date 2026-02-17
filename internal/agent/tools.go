package agent

import "context"

type Tool interface {
	Name() string
	Description() string
	InputSchema() any
	Execute(ctx context.Context, input string) (string, error)
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// Scope returns a new registry containing only the named tools.
// If names is empty, all tools are copied to the new registry.
func (r *Registry) Scope(names []string) *Registry {
	scoped := NewRegistry()
	if len(names) == 0 {
		for k, v := range r.tools {
			scoped.tools[k] = v
		}
		return scoped
	}
	for _, name := range names {
		if t, ok := r.tools[name]; ok {
			scoped.tools[name] = t
		}
	}
	return scoped
}
