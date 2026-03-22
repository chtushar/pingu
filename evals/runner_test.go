package evals_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"pingu/evals"
	"pingu/internal/llm"
)

func TestEvals(t *testing.T) {
	suites, err := evals.LoadSuites("testdata")
	if err != nil {
		t.Fatalf("load suites: %v", err)
	}

	if len(suites) == 0 {
		t.Fatal("no eval suites found in testdata/")
	}

	ctx := context.Background()

	for _, suite := range suites {
		t.Run(suite.Name, func(t *testing.T) {
			for _, tc := range suite.Cases {
				t.Run(tc.Name, func(t *testing.T) {
					if tc.Provider == "real" {
						if os.Getenv("PINGU_EVAL_API_KEY") == "" {
							t.Skip("PINGU_EVAL_API_KEY not set")
						}
					}

					results, err := evals.RunCase(ctx, tc)
					if err != nil {
						t.Fatalf("run failed: %v", err)
					}

					for _, r := range results {
						if !r.Passed {
							t.Errorf("[%s] FAIL: %s", r.GraderType, r.Message)
						} else {
							t.Logf("[%s] PASS", r.GraderType)
						}
					}
				})
			}
		})
	}
}

// TestJSONRoundTrip tests that OutputItem and Response correctly unmarshal
// both native and legacy OpenAI JSON formats.
func TestJSONRoundTrip(t *testing.T) {
	t.Run("OutputItem_native_format", func(t *testing.T) {
		data := `{"type":"message","content":"hello"}`
		var item llm.OutputItem
		if err := json.Unmarshal([]byte(data), &item); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if item.Content != "hello" {
			t.Errorf("content = %q, want %q", item.Content, "hello")
		}
	})

	t.Run("OutputItem_legacy_openai_content_array", func(t *testing.T) {
		data := `{"type":"message","content":[{"type":"output_text","text":"hello from legacy"}]}`
		var item llm.OutputItem
		if err := json.Unmarshal([]byte(data), &item); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if item.Content != "hello from legacy" {
			t.Errorf("content = %q, want %q", item.Content, "hello from legacy")
		}
	})

	t.Run("OutputItem_legacy_multiple_content_parts", func(t *testing.T) {
		data := `{"type":"message","content":[{"type":"output_text","text":"part1"},{"type":"output_text","text":" part2"}]}`
		var item llm.OutputItem
		if err := json.Unmarshal([]byte(data), &item); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if item.Content != "part1 part2" {
			t.Errorf("content = %q, want %q", item.Content, "part1 part2")
		}
	})

	t.Run("OutputItem_function_call", func(t *testing.T) {
		data := `{"type":"function_call","call_id":"c1","name":"shell","arguments":"{\"cmd\":\"ls\"}"}`
		var item llm.OutputItem
		if err := json.Unmarshal([]byte(data), &item); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if item.Name != "shell" || item.CallID != "c1" {
			t.Errorf("name=%q call_id=%q", item.Name, item.CallID)
		}
	})

	t.Run("Response_native_format", func(t *testing.T) {
		data := `{"model":"test","output":[{"type":"message","content":"hi"}],"input_tokens":10,"output_tokens":5}`
		var resp llm.Response
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.InputTokens != 10 || resp.OutputTokens != 5 {
			t.Errorf("tokens: in=%d out=%d", resp.InputTokens, resp.OutputTokens)
		}
		if len(resp.Output) != 1 || resp.Output[0].Content != "hi" {
			t.Errorf("output: %+v", resp.Output)
		}
	})

	t.Run("Response_legacy_openai_format", func(t *testing.T) {
		data := `{
			"model":"gpt-4",
			"output":[
				{"type":"message","content":[{"type":"output_text","text":"legacy hello"}]},
				{"type":"function_call","call_id":"c1","name":"message","arguments":"{\"text\":\"hi\"}"}
			],
			"usage":{"input_tokens":100,"output_tokens":50}
		}`
		var resp llm.Response
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.InputTokens != 100 || resp.OutputTokens != 50 {
			t.Errorf("tokens: in=%d out=%d", resp.InputTokens, resp.OutputTokens)
		}
		if len(resp.Output) != 2 {
			t.Fatalf("output len = %d, want 2", len(resp.Output))
		}
		if resp.Output[0].Content != "legacy hello" {
			t.Errorf("output[0].content = %q", resp.Output[0].Content)
		}
		if resp.Output[1].Name != "message" {
			t.Errorf("output[1].name = %q", resp.Output[1].Name)
		}
	})

	t.Run("OutputToInput_roundtrip", func(t *testing.T) {
		output := []llm.OutputItem{
			{Type: "message", Content: "hello"},
			{Type: "function_call", CallID: "c1", Name: "shell", Arguments: `{"cmd":"ls"}`},
			{Type: "reasoning", Raw: json.RawMessage(`{"summary":"thinking"}`)},
		}
		input := llm.OutputToInput(output)
		if len(input) != 3 {
			t.Fatalf("input len = %d, want 3", len(input))
		}
		if input[0].Role != llm.RoleAssistant || input[0].Content != "hello" {
			t.Errorf("input[0] = %+v", input[0])
		}
		if input[1].Name != "shell" || input[1].CallID != "c1" {
			t.Errorf("input[1] = %+v", input[1])
		}
		if input[2].Type != "reasoning" {
			t.Errorf("input[2].type = %q", input[2].Type)
		}
	})
}
