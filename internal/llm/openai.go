package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type OpenAIProvider struct {
	client *openai.Client
	model  string
}

func NewOpenAI(baseURL, apiKey, model string) *OpenAIProvider {
	var opts []option.RequestOption
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	opts = append(opts, option.WithHTTPClient(&http.Client{
		Transport: &errorLoggingTransport{base: otelhttp.NewTransport(http.DefaultTransport)},
	}))
	client := openai.NewClient(opts...)
	return &OpenAIProvider{client: &client, model: model}
}

func (o *OpenAIProvider) ChatStream(ctx context.Context, input []responses.ResponseInputItemUnionParam, tools []responses.ToolUnionParam, onToken func(string)) (*responses.Response, error) {
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(o.model),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
		Tools: tools,
	}

	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		reqJSON, _ := json.Marshal(params)
		slog.Debug("llm: request", "model", o.model, "input_items", len(input), "tools", len(tools), "body_bytes", len(reqJSON))
	}

	// Try streaming first.
	resp, err := o.tryStream(ctx, params, onToken)
	if err != nil {
		slog.Warn("llm: streaming failed, falling back to non-streaming", "error", err)
		return o.chatNonStream(ctx, params)
	}
	return resp, nil
}

func (o *OpenAIProvider) tryStream(ctx context.Context, params responses.ResponseNewParams, onToken func(string)) (*responses.Response, error) {
	stream := o.client.Responses.NewStreaming(ctx, params)

	var completed *responses.Response
	var eventCount int

	for stream.Next() {
		event := stream.Current()
		eventCount++
		slog.Debug("llm: stream event", "type", event.Type, "seq", eventCount)

		switch event.Type {
		case "response.output_text.delta":
			if event.Delta != "" {
				onToken(event.Delta)
			}
		case "response.completed":
			completed = &event.Response
			slog.Debug("llm: response completed",
				"model", completed.Model,
				"output_items", len(completed.Output),
				"raw_json_len", len(completed.RawJSON()),
			)
		case "response.failed":
			slog.Error("llm: response failed", "error", event.Response.Error.Message)
			return nil, fmt.Errorf("response failed: %s", event.Response.Error.Message)
		}
	}

	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("stream error after %d events: %w", eventCount, err)
	}

	if completed == nil {
		return nil, fmt.Errorf("stream ended without completed response (%d events)", eventCount)
	}

	return completed, nil
}

func (o *OpenAIProvider) chatNonStream(ctx context.Context, params responses.ResponseNewParams) (*responses.Response, error) {
	resp, err := o.client.Responses.New(ctx, params)
	if err != nil {
		slog.Error("llm: non-streaming request failed", "error", err)
		return nil, err
	}

	slog.Debug("llm: non-streaming response",
		"model", resp.Model,
		"output_items", len(resp.Output),
		"input_tokens", resp.Usage.InputTokens,
		"output_tokens", resp.Usage.OutputTokens,
	)

	return resp, nil
}

// errorLoggingTransport wraps an http.RoundTripper and logs non-2xx responses
// so that API errors from providers like OpenRouter are visible in debug logs.
type errorLoggingTransport struct {
	base http.RoundTripper
}

func (t *errorLoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	ct := resp.Header.Get("Content-Type")
	slog.Debug("llm: http response", "status", resp.StatusCode, "content_type", ct, "url", req.URL.Path)

	// Log body for error responses only. Read the full body, log a truncated
	// preview, and reconstruct so the caller can still parse it.
	if resp.StatusCode >= 400 {
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			slog.Error("llm: API error (could not read body)", "status", resp.StatusCode, "error", readErr)
		} else {
			preview := string(body)
			if len(preview) > 1024 {
				preview = preview[:1024] + "..."
			}
			slog.Error("llm: API error response", "status", resp.StatusCode, "content_type", ct, "body", preview)
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
	}

	return resp, nil
}
