package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chtushar/pingu/internal/tools"
)

type stubTool struct{ name string }

func (s *stubTool) Name() string                { return s.name }
func (s *stubTool) Description() string         { return "stub " + s.name }
func (s *stubTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (s *stubTool) Run(_ context.Context, _ json.RawMessage) (string, error) {
	return "OK", nil
}

func TestRegistry(t *testing.T) {
	r, err := tools.NewRegistry(&stubTool{name: "upper"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Empty() {
		t.Error("registry should not be empty")
	}
	if _, ok := r.Get("upper"); !ok {
		t.Error("Get(upper) missing")
	}
	if _, ok := r.Get("missing"); ok {
		t.Error("Get(missing) should fail")
	}
}

func TestRegistryDuplicateName(t *testing.T) {
	if _, err := tools.NewRegistry(&stubTool{name: "dup"}, &stubTool{name: "dup"}); err == nil {
		t.Fatal("expected duplicate name error")
	}
}

func TestRegistrySorted(t *testing.T) {
	r, _ := tools.NewRegistry()
	for _, n := range []string{"zeta", "alpha", "mid"} {
		if err := r.Add(&stubTool{name: n}); err != nil {
			t.Fatal(err)
		}
	}
	got := r.List()
	want := []string{"alpha", "mid", "zeta"}
	for i, w := range want {
		if got[i].Name() != w {
			t.Errorf("List()[%d] = %q, want %q", i, got[i].Name(), w)
		}
	}
}
