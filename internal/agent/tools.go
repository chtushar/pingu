package agent

import "context"

type Tool interface {
	Name() string
	Description() string
	InputSchema() any
	Execute(ctx context.Context, input string) (string, error)
}

// EmitSetter is implemented by tools that need to push events to the user.
// The runner checks for this via type assertion and injects the callback.
type EmitSetter interface {
	SetEmit(func(Event))
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
