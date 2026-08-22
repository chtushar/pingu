// Package tools defines the tool abstraction and an in-memory registry.
// Phase 2 adds discovery of executable tools from an agent directory; the
// interface and registry are the stable seam.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// Tool is a callable capability exposed to the model. Run receives one JSON
// value as arguments and returns the tool result as text or JSON; a non-nil
// error is surfaced to the model as "error: <msg>" so it can retry or adjust.
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage
	Run(ctx context.Context, args json.RawMessage) (string, error)
}

// Registry holds tools keyed by name with deterministic ordering.
type Registry struct {
	byName map[string]Tool
	names  []string
}

// NewRegistry builds a registry from tools; duplicate names are an error.
func NewRegistry(ts ...Tool) (*Registry, error) {
	r := &Registry{byName: make(map[string]Tool, len(ts))}
	for _, t := range ts {
		if t == nil {
			continue
		}
		if err := r.Add(t); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Add registers one tool.
func (r *Registry) Add(t Tool) error {
	name := t.Name()
	if name == "" {
		return fmt.Errorf("tool with empty name")
	}
	if _, ok := r.byName[name]; ok {
		return fmt.Errorf("duplicate tool name %q", name)
	}
	r.byName[name] = t
	r.names = append(r.names, name)
	sort.Strings(r.names)
	return nil
}

// Get looks a tool up by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.byName[name]
	return t, ok
}

// List returns tools sorted by name.
func (r *Registry) List() []Tool {
	out := make([]Tool, 0, len(r.names))
	for _, n := range r.names {
		out = append(out, r.byName[n])
	}
	return out
}

// Empty reports whether the registry has no tools.
func (r *Registry) Empty() bool { return len(r.names) == 0 }
