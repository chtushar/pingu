package openai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chtushar/pingu/internal/llm"
	"github.com/chtushar/pingu/internal/provider/openai"
)

func newProvider(t *testing.T, handler http.HandlerFunc) *openai.Provider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p, err := openai.New(openai.Options{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	return p
}

// sse writes server-sent events.
func sse(w http.ResponseWriter, chunks []string) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher := w.(http.Flusher)
	for _, c := range chunks {
		io.WriteString(w, "data: "+c+"\n\n")
		flusher.Flush()
	}
	io.WriteString(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func collect(t *testing.T, s llm.Stream) []llm.Event {
	t.Helper()
	var events []llm.Event
	for {
		ev, err := s.Next(context.Background())
		if errors.Is(err, io.EOF) {
			return events
		}
		if err != nil {
			t.Fatalf("next: %v", err)
		}
		events = append(events, ev)
	}
}

func TestStream_TextDeltas(t *testing.T) {
	p := newProvider(t, func(w http.ResponseWriter, r *http.Request) {
		sse(w, []string{
			`{"choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
			`{"choices":[{"index":0,"delta":{"content":"Hello"}}]}`,
			`{"choices":[{"index":0,"delta":{"content":" world"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2}}`,
		})
	})
	s, err := p.Stream(context.Background(), llm.Request{Model: "gpt-test", Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer s.Close()
	events := collect(t, s)

	var text strings.Builder
	var usage *llm.Usage
	for _, ev := range events {
		switch ev.Type {
		case llm.EventTextDelta:
			text.WriteString(ev.Text)
		case llm.EventUsage:
			u := ev.Usage
			usage = &u
		}
	}
	if text.String() != "Hello world" {
		t.Errorf("text = %q", text.String())
	}
	if usage == nil || usage.InputTokens != 5 || usage.OutputTokens != 2 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestStream_ToolCallAssembly(t *testing.T) {
	p := newProvider(t, func(w http.ResponseWriter, r *http.Request) {
		sse(w, []string{
			`{"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":""}}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"v"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"alue\":1}"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		})
	})
	s, err := p.Stream(context.Background(), llm.Request{Model: "gpt-test"})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer s.Close()
	events := collect(t, s)

	var sawStart, sawEnd bool
	var args strings.Builder
	for _, ev := range events {
		switch ev.Type {
		case llm.EventToolCallStart:
			sawStart = true
			if ev.ToolCallID != "call_1" || ev.ToolName != "echo" {
				t.Errorf("start = %+v", ev)
			}
		case llm.EventToolCallDelta:
			args.WriteString(ev.ArgumentsDelta)
		case llm.EventToolCallEnd:
			sawEnd = true
		}
	}
	if !sawStart || !sawEnd {
		t.Errorf("start=%v end=%v", sawStart, sawEnd)
	}
	if args.String() != `{"value":1}` {
		t.Errorf("arguments = %q", args.String())
	}
}

func TestStream_HTTPError(t *testing.T) {
	p := newProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":{"message":"server on fire"}}`)
	})
	_, err := p.Stream(context.Background(), llm.Request{Model: "gpt-test"})
	if err == nil {
		t.Fatal("expected error")
	}
	var perr *llm.ProviderError
	if !errors.As(err, &perr) {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if perr.Code != "http_500" || !strings.Contains(err.Error(), "server on fire") {
		t.Errorf("error = %v", err)
	}
}

func TestStream_MalformedLine(t *testing.T) {
	p := newProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "this is not sse\n\n")
	})
	s, err := p.Stream(context.Background(), llm.Request{Model: "gpt-test"})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer s.Close()
	if _, err := s.Next(context.Background()); err == nil {
		t.Fatal("expected malformed stream error")
	} else {
		var perr *llm.ProviderError
		if !errors.As(err, &perr) || perr.Code != "malformed_stream" {
			t.Errorf("expected malformed_stream, got %v", err)
		}
	}
}

func TestStream_BadJSON(t *testing.T) {
	p := newProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {not json\n\n")
	})
	s, err := p.Stream(context.Background(), llm.Request{Model: "gpt-test"})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer s.Close()
	if _, err := s.Next(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestStream_RequestShape(t *testing.T) {
	p := newProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body["model"] != "gpt-test" || body["stream"] != true {
			t.Errorf("body = %v", body)
		}
		msgs := body["messages"].([]any)
		if len(msgs) != 2 { // system + user
			t.Errorf("messages = %v", msgs)
		}
		sse(w, []string{`{"choices":[{"index":0,"delta":{"content":"ok"}}]}`})
	})
	s, err := p.Stream(context.Background(), llm.Request{
		Model:    "gpt-test",
		System:   "sys",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
		Tools: []llm.ToolDef{{
			Name:        "echo",
			Description: "echoes",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer s.Close()
	collect(t, s)
}

func TestNew_MissingAPIKey(t *testing.T) {
	if _, err := openai.New(openai.Options{}); err == nil {
		t.Fatal("expected error for missing API key")
	}
}
