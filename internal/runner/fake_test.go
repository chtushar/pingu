package runner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/chtushar/pingu/internal/llm"
)

// fakeProvider scripts provider responses per call. It records every
// request for assertions.
type fakeProvider struct {
	mu       sync.Mutex
	requests []llm.Request
	next     func(call int, req llm.Request) ([]llm.Event, error)
	// streamOverride, when set, replaces the scripted event stream.
	streamOverride func(req llm.Request) (llm.Stream, error)
}

func (f *fakeProvider) Stream(_ context.Context, req llm.Request) (llm.Stream, error) {
	f.mu.Lock()
	call := len(f.requests) + 1
	f.requests = append(f.requests, req)
	f.mu.Unlock()
	if f.streamOverride != nil {
		return f.streamOverride(req)
	}
	events, err := f.next(call, req)
	if err != nil {
		return nil, err
	}
	return llm.NewSliceStream(events), nil
}

func (f *fakeProvider) request(i int) llm.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[i]
}

func (f *fakeProvider) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// textEvents is a convenience for a text-only response.
func textEvents(s string) []llm.Event {
	return []llm.Event{{Type: llm.EventTextDelta, Text: s}}
}

// toolCallEvents emits a complete tool call followed by text.
func toolCallEvents(id, name, args string) []llm.Event {
	return []llm.Event{
		{Type: llm.EventToolCallStart, ToolIndex: 0, ToolCallID: id, ToolName: name},
		{Type: llm.EventToolCallDelta, ToolIndex: 0, ArgumentsDelta: args},
		{Type: llm.EventToolCallEnd, ToolIndex: 0},
	}
}

// hangStream blocks until the context is done, then returns the ctx error.
type hangStream struct{}

func (hangStream) Next(ctx context.Context) (llm.Event, error) {
	<-ctx.Done()
	return llm.Event{}, ctx.Err()
}

func (hangStream) Close() error { return nil }

type hangingProvider struct{}

func (hangingProvider) Stream(ctx context.Context, _ llm.Request) (llm.Stream, error) {
	return hangStream{}, nil
}

// fakeTool is a minimal Tool implementation.
type fakeTool struct {
	name    string
	fn      func(ctx context.Context, args json.RawMessage) (string, error)
	calls   int
	lastArg string
}

func (t *fakeTool) Name() string        { return t.name }
func (t *fakeTool) Description() string { return "fake tool " + t.name }
func (t *fakeTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t *fakeTool) Run(ctx context.Context, args json.RawMessage) (string, error) {
	t.calls++
	var m map[string]any
	_ = json.Unmarshal(args, &m)
	t.lastArg = fmt.Sprint(m["value"])
	return t.fn(ctx, args)
}

// erroringStream yields prefix events, then fails.
type erroringStream struct {
	events []llm.Event
	err    error
}

func (s *erroringStream) Next(_ context.Context) (llm.Event, error) {
	if len(s.events) > 0 {
		ev := s.events[0]
		s.events = s.events[1:]
		return ev, nil
	}
	return llm.Event{}, s.err
}

func (s *erroringStream) Close() error { return nil }
