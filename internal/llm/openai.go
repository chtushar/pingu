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

func (o *OpenAIProvider) ChatStream(ctx context.Context, input []InputItem, tools []ToolDefinition, onToken func(string)) (*Response, error) {
	oaiInput := toOpenAIInput(input)
	oaiTools := toOpenAITools(tools)

	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(o.model),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: oaiInput,
		},
		Tools: oaiTools,
	}

	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		reqJSON, _ := json.Marshal(params)
		slog.Debug("llm: request", "model", o.model, "input_items", len(oaiInput), "tools", len(oaiTools), "body_bytes", len(reqJSON))
	}

	// Try streaming first.
	resp, err := o.tryStream(ctx, params, onToken)
	if err != nil {
		slog.Warn("llm: streaming failed, falling back to non-streaming", "error", err)
		resp, err = o.chatNonStream(ctx, params)
		if err != nil {
			return nil, err
		}
	}

	return fromOpenAIResponse(resp), nil
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

// --- Conversion: native → OpenAI ---

func toOpenAIInput(items []InputItem) []responses.ResponseInputItemUnionParam {
	out := make([]responses.ResponseInputItemUnionParam, 0, len(items))
	for _, item := range items {
		switch item.Type {
		case "message":
			out = append(out, responses.ResponseInputItemParamOfMessage(item.Content, responses.EasyInputMessageRole(item.Role)))
		case "function_call":
			out = append(out, responses.ResponseInputItemUnionParam{
				OfFunctionCall: &responses.ResponseFunctionToolCallParam{
					CallID:    item.CallID,
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		case "function_call_output":
			out = append(out, responses.ResponseInputItemParamOfFunctionCallOutput(item.CallID, item.Output))
		default:
			// Round-trip provider-specific items via Raw JSON.
			if item.Raw != nil {
				out = append(out, roundTripInputItem(item)...)
			}
		}
	}
	return out
}

// roundTripInputItem attempts to unmarshal Raw JSON back into an OpenAI input
// item. This handles reasoning, web_search_call, etc. that were captured from
// a previous OpenAI response.
func roundTripInputItem(item InputItem) []responses.ResponseInputItemUnionParam {
	var union responses.ResponseInputItemUnionParam
	if err := json.Unmarshal(item.Raw, &union); err != nil {
		slog.Debug("llm: skipping unrecognized input item", "type", item.Type, "error", err)
		return nil
	}
	return []responses.ResponseInputItemUnionParam{union}
}

func toOpenAITools(tools []ToolDefinition) []responses.ToolUnionParam {
	out := make([]responses.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		out = append(out, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  t.Parameters,
				Strict:      openai.Bool(t.Strict),
			},
		})
	}
	return out
}

// --- Conversion: OpenAI → native ---

func fromOpenAIResponse(resp *responses.Response) *Response {
	output := make([]OutputItem, 0, len(resp.Output))
	for _, item := range resp.Output {
		output = append(output, fromOpenAIOutputItem(item))
	}
	return &Response{
		Model:        resp.Model,
		Output:       output,
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
	}
}

func fromOpenAIOutputItem(item responses.ResponseOutputItemUnion) OutputItem {
	switch item.Type {
	case "message":
		msg := item.AsMessage()
		var text string
		for _, c := range msg.Content {
			if c.Type == "output_text" {
				text += c.AsOutputText().Text
			}
		}
		return OutputItem{Type: "message", Content: text}
	case "function_call":
		fc := item.AsFunctionCall()
		return OutputItem{
			Type:      "function_call",
			CallID:    fc.CallID,
			Name:      fc.Name,
			Arguments: fc.Arguments,
		}
	default:
		// Preserve unknown types (reasoning, web_search, etc.) as raw JSON
		// so they can be round-tripped back to the provider.
		raw, _ := json.Marshal(openAIOutputToInputParam(item))
		return OutputItem{Type: item.Type, Raw: raw}
	}
}

// openAIOutputToInputParam converts an OpenAI output item to its input param
// form for round-tripping.
func openAIOutputToInputParam(item responses.ResponseOutputItemUnion) responses.ResponseInputItemUnionParam {
	switch item.Type {
	case "message":
		v := item.AsMessage().ToParam()
		return responses.ResponseInputItemUnionParam{OfOutputMessage: &v}
	case "function_call":
		v := item.AsFunctionCall().ToParam()
		return responses.ResponseInputItemUnionParam{OfFunctionCall: &v}
	case "reasoning":
		v := item.AsReasoning().ToParam()
		return responses.ResponseInputItemUnionParam{OfReasoning: &v}
	case "file_search_call":
		v := item.AsFileSearchCall().ToParam()
		return responses.ResponseInputItemUnionParam{OfFileSearchCall: &v}
	case "web_search_call":
		v := item.AsWebSearchCall().ToParam()
		return responses.ResponseInputItemUnionParam{OfWebSearchCall: &v}
	case "computer_call":
		v := item.AsComputerCall().ToParam()
		return responses.ResponseInputItemUnionParam{OfComputerCall: &v}
	case "code_interpreter_call":
		v := item.AsCodeInterpreterCall().ToParam()
		return responses.ResponseInputItemUnionParam{OfCodeInterpreterCall: &v}
	default:
		return responses.ResponseInputItemUnionParam{}
	}
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

	// Log body for error responses only.
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
