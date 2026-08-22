// Package openai implements llm.Provider against the OpenAI-compatible
// Chat Completions API with response streaming. It speaks raw HTTP with the
// standard library; no provider SDK types cross this boundary.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/chtushar/pingu/internal/llm"
)

const (
	// DefaultBaseURL is the public OpenAI API root.
	DefaultBaseURL = "https://api.openai.com/v1"
	providerName   = "openai"
	maxErrorBody   = 4 * 1024
	maxScanLine    = 1024 * 1024
)

// Options configures the adapter.
type Options struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// Provider streams completions from an OpenAI-compatible endpoint.
type Provider struct {
	opts   Options
	client *http.Client
}

// New builds a provider; a missing API key is an error.
func New(opts Options) (*Provider, error) {
	if opts.APIKey == "" {
		return nil, errors.New("OPENAI_API_KEY is not set")
	}
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	return &Provider{opts: opts, client: client}, nil
}

// FromEnv builds a provider from OPENAI_API_KEY and OPENAI_BASE_URL.
func FromEnv(client *http.Client) (*Provider, error) {
	return New(Options{
		APIKey:     os.Getenv("OPENAI_API_KEY"),
		BaseURL:    os.Getenv("OPENAI_BASE_URL"),
		HTTPClient: client,
	})
}

// Stream starts a streaming completion. The context governs the whole HTTP
// exchange: cancelling it aborts the request and unblocks Next.
func (p *Provider) Stream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	body, err := p.buildBody(req)
	if err != nil {
		return nil, llm.NewProviderError(providerName, "request_body", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.opts.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, llm.NewProviderError(providerName, "request_failed", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+p.opts.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, llm.NewProviderError(providerName, "request_failed", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		msg := readBounded(resp.Body)
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return nil, llm.NewProviderError(providerName, "http_"+strconv.Itoa(resp.StatusCode), errors.New(msg))
	}

	s := &stream{resp: resp}
	s.scanner = bufio.NewScanner(resp.Body)
	s.scanner.Buffer(make([]byte, 0, 64*1024), maxScanLine)
	return s, nil
}

func readBounded(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, maxErrorBody))
	return string(bytes.TrimSpace(b))
}

// --- wire format ---

type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireToolFunction `json:"function"`
}

type wireToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolDef struct {
	Type     string          `json:"type"`
	Function wireToolDefFunc `json:"function"`
}

type wireToolDefFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type wireRequest struct {
	Model         string        `json:"model"`
	Stream        bool          `json:"stream"`
	StreamOptions *wireStreamOp `json:"stream_options,omitempty"`
	Messages      []wireMessage `json:"messages"`
	Tools         []wireToolDef `json:"tools,omitempty"`
}

type wireStreamOp struct {
	IncludeUsage bool `json:"include_usage"`
}

func (p *Provider) buildBody(req llm.Request) ([]byte, error) {
	w := wireRequest{
		Model:         req.Model,
		Stream:        true,
		StreamOptions: &wireStreamOp{IncludeUsage: true},
		Messages:      make([]wireMessage, 0, len(req.Messages)+1),
	}
	if req.System != "" {
		w.Messages = append(w.Messages, wireMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		wm := wireMessage{Role: string(m.Role), Content: m.Content, ToolCallID: m.ToolCallID}
		for _, c := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
				ID:       c.ID,
				Type:     "function",
				Function: wireToolFunction{Name: c.Name, Arguments: string(c.Arguments)},
			})
		}
		w.Messages = append(w.Messages, wm)
	}
	for _, t := range req.Tools {
		w.Tools = append(w.Tools, wireToolDef{
			Type:     "function",
			Function: wireToolDefFunc{Name: t.Name, Description: t.Description, Parameters: t.Parameters},
		})
	}
	return json.Marshal(w)
}

// --- stream decoding ---

type chunk struct {
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage"`
}

type stream struct {
	resp    *http.Response
	scanner *bufio.Scanner
	// current tracks the tool-call index being assembled so start/end
	// events can be derived from the wire format.
	current int
	started bool
	closed  bool
}

func (s *stream) Next(ctx context.Context) (llm.Event, error) {
	if s.closed {
		return llm.Event{}, io.EOF
	}
	for {
		if err := ctx.Err(); err != nil {
			return llm.Event{}, err
		}
		if !s.scanner.Scan() {
			if err := s.scanner.Err(); err != nil {
				return llm.Event{}, s.streamErr(err)
			}
			return llm.Event{}, io.EOF
		}
		line := bytes.TrimSpace(s.scanner.Bytes())
		if len(line) == 0 || line[0] == ':' {
			continue // keep-alive comment or blank separator
		}
		data, ok := bytes.CutPrefix(line, []byte("data:"))
		if !ok {
			return llm.Event{}, llm.NewProviderError(providerName, "malformed_stream", fmt.Errorf("unexpected line %q", string(line)))
		}
		data = bytes.TrimSpace(data)
		if string(data) == "[DONE]" {
			if ev, ok := s.endCurrent(); ok {
				return ev, nil
			}
			return llm.Event{}, io.EOF
		}
		var c chunk
		if err := json.Unmarshal(data, &c); err != nil {
			return llm.Event{}, llm.NewProviderError(providerName, "malformed_stream", err)
		}
		if ev, ok, err := s.decode(&c); err != nil {
			return llm.Event{}, err
		} else if ok {
			return ev, nil
		}
		// Chunk carried no emittable event (e.g. role-only first chunk);
		// keep scanning.
	}
}

func (s *stream) streamErr(err error) error {
	if cerr := s.resp.Request.Context().Err(); cerr != nil {
		return cerr
	}
	return llm.NewProviderError(providerName, "stream_failed", err)
}

func (s *stream) decode(c *chunk) (llm.Event, bool, error) {
	if c.Usage != nil {
		return llm.Event{Type: llm.EventUsage, Usage: llm.Usage{
			InputTokens:  c.Usage.PromptTokens,
			OutputTokens: c.Usage.CompletionTokens,
		}}, true, nil
	}
	if len(c.Choices) == 0 {
		return llm.Event{}, false, nil
	}
	ch := c.Choices[0]
	if ch.FinishReason == "tool_calls" {
		// The wire format has no per-call end marker; finish_reason closes
		// the call being assembled.
		if ev, ok := s.endCurrent(); ok {
			return ev, true, nil
		}
		return llm.Event{}, false, nil
	}
	if ch.Delta.Content != "" {
		return llm.Event{Type: llm.EventTextDelta, Text: ch.Delta.Content}, true, nil
	}
	for _, tc := range ch.Delta.ToolCalls {
		idx := tc.Index
		if s.started && idx != s.current {
			// A new call begins; close the previous one first. The caller
			// re-enters Next for the start event.
			ev, ok := s.endCurrent()
			if ok {
				return ev, true, nil
			}
		}
		if !s.started {
			s.current = idx
			s.started = true
			name := tc.Function.Name
			if name == "" && tc.ID == "" && tc.Function.Arguments != "" {
				// Arguments delta arriving before any start; synthesize an
				// anonymous call start.
			}
			return llm.Event{
				Type:       llm.EventToolCallStart,
				ToolIndex:  idx,
				ToolCallID: tc.ID,
				ToolName:   name,
			}, true, nil
		}
		if tc.Function.Arguments != "" || tc.Function.Name != "" {
			delta := tc.Function.Arguments
			return llm.Event{Type: llm.EventToolCallDelta, ToolIndex: idx, ArgumentsDelta: delta}, true, nil
		}
	}
	return llm.Event{}, false, nil
}

func (s *stream) endCurrent() (llm.Event, bool) {
	if !s.started {
		return llm.Event{}, false
	}
	s.started = false
	return llm.Event{Type: llm.EventToolCallEnd, ToolIndex: s.current}, true
}

func (s *stream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.resp.Body.Close()
}
