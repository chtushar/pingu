package evals

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// FromConfig creates a Grader from a YAML GraderConfig.
func FromConfig(cfg GraderConfig) Grader {
	switch cfg.Type {
	case "match":
		return &matchGrader{expected: cfg.Expected}
	case "contains":
		return &containsGrader{expected: cfg.Expected, regex: cfg.Regex}
	case "tool_used":
		return &toolUsedGrader{tools: cfg.Tools}
	case "tool_sequence":
		return &toolSequenceGrader{tools: cfg.Tools}
	default:
		return &matchGrader{expected: "UNKNOWN_GRADER_TYPE:" + cfg.Type}
	}
}

// --- Match ---

type matchGrader struct{ expected string }

func (g *matchGrader) Grade(_ context.Context, t Transcript) EvalResult {
	got := strings.TrimSpace(t.FinalResponse)
	want := strings.TrimSpace(g.expected)
	passed := got == want
	msg := ""
	if !passed {
		msg = fmt.Sprintf("expected %q, got %q", want, got)
	}
	return EvalResult{GraderType: "match", Passed: passed, Score: boolScore(passed), Message: msg}
}

// --- Contains ---

type containsGrader struct {
	expected string
	regex    bool
}

func (g *containsGrader) Grade(_ context.Context, t Transcript) EvalResult {
	got := t.FinalResponse
	var passed bool
	if g.regex {
		re, err := regexp.Compile(g.expected)
		if err != nil {
			return EvalResult{GraderType: "contains", Message: fmt.Sprintf("invalid regex: %v", err)}
		}
		passed = re.MatchString(got)
	} else {
		passed = strings.Contains(got, g.expected)
	}
	msg := ""
	if !passed {
		preview := got
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		msg = fmt.Sprintf("response %q does not contain %q", preview, g.expected)
	}
	return EvalResult{GraderType: "contains", Passed: passed, Score: boolScore(passed), Message: msg}
}

// --- ToolUsed ---

type toolUsedGrader struct{ tools []string }

func (g *toolUsedGrader) Grade(_ context.Context, t Transcript) EvalResult {
	called := make(map[string]bool)
	for _, tc := range t.ToolCalls {
		called[tc.Name] = true
	}
	var missing []string
	for _, name := range g.tools {
		if !called[name] {
			missing = append(missing, name)
		}
	}
	passed := len(missing) == 0
	msg := ""
	if !passed {
		msg = fmt.Sprintf("tools not called: %s", strings.Join(missing, ", "))
	}
	return EvalResult{GraderType: "tool_used", Passed: passed, Score: boolScore(passed), Message: msg}
}

// --- ToolSequence ---

type toolSequenceGrader struct{ tools []string }

func (g *toolSequenceGrader) Grade(_ context.Context, t Transcript) EvalResult {
	seqIdx := 0
	for _, tc := range t.ToolCalls {
		if seqIdx < len(g.tools) && tc.Name == g.tools[seqIdx] {
			seqIdx++
		}
	}
	passed := seqIdx == len(g.tools)
	msg := ""
	if !passed {
		var actual []string
		for _, tc := range t.ToolCalls {
			actual = append(actual, tc.Name)
		}
		msg = fmt.Sprintf("expected sequence [%s], got [%s] (matched %d/%d)",
			strings.Join(g.tools, " → "), strings.Join(actual, " → "), seqIdx, len(g.tools))
	}
	return EvalResult{GraderType: "tool_sequence", Passed: passed, Score: boolScore(passed), Message: msg}
}

func boolScore(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}
