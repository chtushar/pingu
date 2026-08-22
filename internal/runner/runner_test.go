package runner_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/chtushar/pingu/internal/config"
	"github.com/chtushar/pingu/internal/llm"
	"github.com/chtushar/pingu/internal/runner"
	"github.com/chtushar/pingu/internal/tools"
)

func testLimits() config.Limits {
	return config.Limits{
		MaxModelTurns:      4,
		MaxToolCalls:       8,
		RunTimeout:         5 * time.Second,
		ToolTimeout:        2 * time.Second,
		MaxToolOutputBytes: 1024,
	}
}

func collect(events *[]runner.Event) func(runner.Event) {
	return func(ev runner.Event) { *events = append(*events, ev) }
}

func kinds(events []runner.Event) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, string(e.Kind))
	}
	return out
}

func TestRun_TextOnly(t *testing.T) {
	p := &fakeProvider{next: func(call int, _ llm.Request) ([]llm.Event, error) {
		return textEvents("hello there"), nil
	}}
	r := &runner.Runner{Provider: p, Limits: testLimits()}
	var events []runner.Event
	res, err := r.Run(context.Background(), runner.RunRequest{
		Instructions: "be brief",
		Model:        "openai/test",
		Input:        "hi",
	}, collect(&events))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Turns != 1 {
		t.Errorf("turns = %d, want 1", res.Turns)
	}
	if got := kinds(events); strings.Join(got, ",") != "run_started,text_delta,run_finished" {
		t.Errorf("events = %v", got)
	}
	if p.requestCount() != 1 {
		t.Errorf("provider calls = %d, want 1", p.requestCount())
	}
	req := p.request(0)
	if req.System != "be brief" || len(req.Messages) != 1 || req.Messages[0].Content != "hi" {
		t.Errorf("unexpected request: %+v", req)
	}
	if len(res.Messages) != 1 || res.Messages[0].Content != "hello there" {
		t.Errorf("unexpected result messages: %+v", res.Messages)
	}
}

func TestRun_MultiTurnToolCalls(t *testing.T) {
	echo := &fakeTool{name: "echo", fn: func(_ context.Context, args json.RawMessage) (string, error) {
		var in struct{ Value string }
		_ = json.Unmarshal(args, &in)
		return "echo:" + in.Value, nil
	}}
	reg, err := tools.NewRegistry(echo)
	if err != nil {
		t.Fatal(err)
	}

	p := &fakeProvider{next: func(call int, _ llm.Request) ([]llm.Event, error) {
		switch call {
		case 1:
			return toolCallEvents("call-1", "echo", `{"value":"world"}`), nil
		default:
			return textEvents("done"), nil
		}
	}}
	r := &runner.Runner{Provider: p, Limits: testLimits()}
	var events []runner.Event
	res, err := r.Run(context.Background(), runner.RunRequest{
		Instructions: "use tools",
		Model:        "openai/test",
		Input:        "echo world",
		Tools:        reg,
	}, collect(&events))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Turns != 2 {
		t.Errorf("turns = %d, want 2", res.Turns)
	}
	if echo.calls != 1 || echo.lastArg != "world" {
		t.Errorf("tool calls = %d lastArg = %q", echo.calls, echo.lastArg)
	}
	if p.requestCount() != 2 {
		t.Fatalf("provider calls = %d, want 2", p.requestCount())
	}

	// Second provider call must contain the assistant tool call and result.
	second := p.request(1)
	if len(second.Messages) != 3 {
		t.Fatalf("second call messages = %d, want 3", len(second.Messages))
	}
	assistant, toolResult := second.Messages[1], second.Messages[2]
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].Name != "echo" {
		t.Errorf("assistant tool calls = %+v", assistant.ToolCalls)
	}
	if toolResult.Role != llm.RoleTool || toolResult.ToolCallID != "call-1" || toolResult.Content != "echo:world" {
		t.Errorf("tool result message = %+v", toolResult)
	}

	// Event ordering: tool started/finished before the final text sequence.
	got := kinds(events)
	wantRun := []string{"run_started", "tool_started", "tool_finished", "text_delta", "run_finished"}
	if strings.Join(got, ",") != strings.Join(wantRun, ",") {
		t.Errorf("events = %v, want %v", got, wantRun)
	}
}

func TestRun_MalformedArguments(t *testing.T) {
	echo := &fakeTool{name: "echo", fn: func(_ context.Context, _ json.RawMessage) (string, error) {
		return "should not run", nil
	}}
	reg, _ := tools.NewRegistry(echo)

	p := &fakeProvider{next: func(call int, _ llm.Request) ([]llm.Event, error) {
		switch call {
		case 1:
			return toolCallEvents("call-1", "echo", `{not json`), nil
		default:
			return textEvents("recovered"), nil
		}
	}}
	r := &runner.Runner{Provider: p, Limits: testLimits()}
	var events []runner.Event
	_, err := r.Run(context.Background(), runner.RunRequest{
		Input: "x", Tools: reg,
	}, collect(&events))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if echo.calls != 0 {
		t.Errorf("tool ran despite malformed arguments")
	}
	second := p.request(1)
	if len(second.Messages) != 3 || !strings.HasPrefix(second.Messages[2].Content, "error:") {
		t.Errorf("expected error tool result, got %+v", second.Messages)
	}
}

func TestRun_UnknownTool(t *testing.T) {
	reg, _ := tools.NewRegistry()
	p := &fakeProvider{next: func(call int, _ llm.Request) ([]llm.Event, error) {
		switch call {
		case 1:
			return toolCallEvents("call-1", "nope", `{}`), nil
		default:
			return textEvents("ok"), nil
		}
	}}
	r := &runner.Runner{Provider: p, Limits: testLimits()}
	_, err := r.Run(context.Background(), runner.RunRequest{Input: "x", Tools: reg}, func(runner.Event) {})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	second := p.request(1)
	if !strings.HasPrefix(second.Messages[2].Content, "error: unknown tool") {
		t.Errorf("expected unknown tool error, got %q", second.Messages[2].Content)
	}
}

func TestRun_ToolError(t *testing.T) {
	failing := &fakeTool{name: "fail", fn: func(_ context.Context, _ json.RawMessage) (string, error) {
		return "", errors.New("boom")
	}}
	reg, _ := tools.NewRegistry(failing)
	p := &fakeProvider{next: func(call int, _ llm.Request) ([]llm.Event, error) {
		switch call {
		case 1:
			return toolCallEvents("c1", "fail", `{}`), nil
		default:
			return textEvents("ok"), nil
		}
	}}
	r := &runner.Runner{Provider: p, Limits: testLimits()}
	_, err := r.Run(context.Background(), runner.RunRequest{Input: "x", Tools: reg}, func(runner.Event) {})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := p.request(1).Messages[2].Content; got != "error: boom" {
		t.Errorf("tool error result = %q", got)
	}
}

func TestRun_ProviderFailure(t *testing.T) {
	p := &fakeProvider{next: func(int, llm.Request) ([]llm.Event, error) {
		return nil, llm.NewProviderError("openai", "http_500", errors.New("server exploded"))
	}}
	r := &runner.Runner{Provider: p, Limits: testLimits()}
	var events []runner.Event
	_, err := r.Run(context.Background(), runner.RunRequest{Input: "x"}, collect(&events))
	if err == nil {
		t.Fatal("expected error")
	}
	last := events[len(events)-1]
	if last.Kind != runner.EventRunFinished || last.Err == nil {
		t.Errorf("run_finished must carry the error: %+v", last)
	}
	hasErrorEvent := false
	for _, e := range events {
		if e.Kind == runner.EventError {
			hasErrorEvent = true
		}
	}
	if !hasErrorEvent {
		t.Errorf("expected an error event, got %v", kinds(events))
	}
}

func TestRun_StreamFailure(t *testing.T) {
	p := &fakeProvider{next: func(call int, _ llm.Request) ([]llm.Event, error) {
		return textEvents("partial"), nil
	}}
	p.streamOverride = func(llm.Request) (llm.Stream, error) {
		return &erroringStream{events: textEvents("partial"), err: io.ErrUnexpectedEOF}, nil
	}
	r := &runner.Runner{Provider: p, Limits: testLimits()}
	_, err := r.Run(context.Background(), runner.RunRequest{Input: "x"}, func(runner.Event) {})
	if err == nil || !strings.Contains(err.Error(), "stream failed") {
		t.Fatalf("expected stream failure, got %v", err)
	}
}

func TestRun_Cancellation(t *testing.T) {
	r := &runner.Runner{Provider: hangingProvider{}, Limits: testLimits()}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	done := make(chan error, 1)
	go func() {
		_, err := r.Run(ctx, runner.RunRequest{Input: "x"}, func(runner.Event) {})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run did not observe cancellation")
	}
}

func TestRun_MaxModelTurnsExhausted(t *testing.T) {
	echo := &fakeTool{name: "echo", fn: func(_ context.Context, _ json.RawMessage) (string, error) {
		return "ok", nil
	}}
	reg, _ := tools.NewRegistry(echo)
	p := &fakeProvider{next: func(int, llm.Request) ([]llm.Event, error) {
		return toolCallEvents("c", "echo", `{}`), nil
	}}
	limits := testLimits()
	limits.MaxModelTurns = 2
	r := &runner.Runner{Provider: p, Limits: limits}
	var events []runner.Event
	_, err := r.Run(context.Background(), runner.RunRequest{Input: "x", Tools: reg}, collect(&events))
	if !errors.Is(err, runner.ErrLimitExhausted) {
		t.Fatalf("expected ErrLimitExhausted, got %v", err)
	}
	if p.requestCount() != 2 {
		t.Errorf("provider calls = %d, want 2", p.requestCount())
	}
}

func TestRun_MaxToolCallsExhausted(t *testing.T) {
	echo := &fakeTool{name: "echo", fn: func(_ context.Context, _ json.RawMessage) (string, error) {
		return "ok", nil
	}}
	reg, _ := tools.NewRegistry(echo)
	p := &fakeProvider{next: func(int, llm.Request) ([]llm.Event, error) {
		return toolCallEvents("c", "echo", `{}`), nil
	}}
	limits := testLimits()
	limits.MaxToolCalls = 1
	r := &runner.Runner{Provider: p, Limits: limits}
	_, err := r.Run(context.Background(), runner.RunRequest{Input: "x", Tools: reg}, func(runner.Event) {})
	if !errors.Is(err, runner.ErrLimitExhausted) {
		t.Fatalf("expected ErrLimitExhausted, got %v", err)
	}
}

func TestRun_ToolTimeout(t *testing.T) {
	slow := &fakeTool{name: "slow", fn: func(ctx context.Context, _ json.RawMessage) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}}
	reg, _ := tools.NewRegistry(slow)
	p := &fakeProvider{next: func(call int, _ llm.Request) ([]llm.Event, error) {
		switch call {
		case 1:
			return toolCallEvents("c1", "slow", `{}`), nil
		default:
			return textEvents("done"), nil
		}
	}}
	r := &runner.Runner{Provider: p, Limits: testLimits()}
	_, err := r.Run(context.Background(), runner.RunRequest{Input: "x", Tools: reg}, func(runner.Event) {})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := p.request(1).Messages[2].Content; !strings.Contains(got, "timed out") {
		t.Errorf("expected timeout error result, got %q", got)
	}
}

func TestRun_OutputTruncated(t *testing.T) {
	noisy := &fakeTool{name: "noisy", fn: func(_ context.Context, _ json.RawMessage) (string, error) {
		return strings.Repeat("a", 3000), nil
	}}
	reg, _ := tools.NewRegistry(noisy)
	p := &fakeProvider{next: func(call int, _ llm.Request) ([]llm.Event, error) {
		switch call {
		case 1:
			return toolCallEvents("c1", "noisy", `{}`), nil
		default:
			return textEvents("done"), nil
		}
	}}
	r := &runner.Runner{Provider: p, Limits: testLimits()}
	var events []runner.Event
	_, err := r.Run(context.Background(), runner.RunRequest{Input: "x", Tools: reg}, collect(&events))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	second := p.request(1)
	if got := len(second.Messages[2].Content); got != 1024 {
		t.Errorf("truncated output length = %d, want 1024", got)
	}
	sawWarning := false
	for _, e := range events {
		if e.Kind == runner.EventWarning && strings.Contains(e.Text, "truncated") {
			sawWarning = true
		}
	}
	if !sawWarning {
		t.Errorf("expected truncation warning")
	}
}

func TestRun_UsageAccumulated(t *testing.T) {
	p := &fakeProvider{next: func(call int, _ llm.Request) ([]llm.Event, error) {
		switch call {
		case 1:
			return append(toolCallEvents("c", "echo", `{}`),
				llm.Event{Type: llm.EventUsage, Usage: llm.Usage{InputTokens: 10, OutputTokens: 5}}), nil
		default:
			return []llm.Event{
				{Type: llm.EventTextDelta, Text: "ok"},
				{Type: llm.EventUsage, Usage: llm.Usage{InputTokens: 7, OutputTokens: 3}},
			}, nil
		}
	}}
	echo := &fakeTool{name: "echo", fn: func(_ context.Context, _ json.RawMessage) (string, error) {
		return "ok", nil
	}}
	reg, _ := tools.NewRegistry(echo)
	r := &runner.Runner{Provider: p, Limits: testLimits()}
	res, err := r.Run(context.Background(), runner.RunRequest{Input: "x", Tools: reg}, func(runner.Event) {})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Usage.InputTokens != 17 || res.Usage.OutputTokens != 8 {
		t.Errorf("usage = %+v", res.Usage)
	}
}

func TestRun_HistoryPassedThrough(t *testing.T) {
	p := &fakeProvider{next: func(int, llm.Request) ([]llm.Event, error) {
		return textEvents("hi again"), nil
	}}
	r := &runner.Runner{Provider: p, Limits: testLimits()}
	history := []llm.Message{
		{Role: llm.RoleUser, Content: "hello"},
		{Role: llm.RoleAssistant, Content: "hi"},
	}
	_, err := r.Run(context.Background(), runner.RunRequest{
		Input:   "again",
		History: history,
	}, func(runner.Event) {})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	req := p.request(0)
	if len(req.Messages) != 3 || req.Messages[2].Content != "again" {
		t.Errorf("messages = %+v", req.Messages)
	}
}
