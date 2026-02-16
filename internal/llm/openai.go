package llm

import (
	"context"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
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
	client := openai.NewClient(opts...)
	return &OpenAIProvider{client: &client, model: model}
}

func (o *OpenAIProvider) ChatStream(ctx context.Context, messages []Message, tools []ToolDef, onToken func(string)) (*Response, error) {
	input := make(responses.ResponseInputParam, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "tool":
			input = append(input, responses.ResponseInputItemParamOfFunctionCallOutput(m.ToolCallID, m.Content))
		case "assistant":
			for _, tc := range m.ToolCalls {
				input = append(input, responses.ResponseInputItemParamOfFunctionCall(tc.Input, tc.ID, tc.Name))
			}
			if m.Content != "" {
				input = append(input, responses.ResponseInputItemParamOfMessage(m.Content, "assistant"))
			}
		default:
			input = append(input, responses.ResponseInputItemParamOfMessage(m.Content, responses.EasyInputMessageRole(m.Role)))
		}
	}

	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(o.model),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
	}

	if len(tools) > 0 {
		for _, t := range tools {
			params.Tools = append(params.Tools, responses.ToolUnionParam{
				OfFunction: &responses.FunctionToolParam{
					Name:        t.Name,
					Description: openai.String(t.Description),
					Parameters:  toFunctionParams(t.InputSchema),
				},
			})
		}
	}

	stream := o.client.Responses.NewStreaming(ctx, params)

	var content string
	var toolCalls []ToolCall

	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "response.output_text.delta":
			if event.Delta != "" {
				onToken(event.Delta)
				content += event.Delta
			}
		case "response.function_call_arguments.done":
			toolCalls = append(toolCalls, ToolCall{
				ID:    event.ItemID,
				Name:  event.Name,
				Input: event.Arguments,
			})
		case "response.failed":
			return nil, fmt.Errorf("response failed: %s", event.Response.Error.Message)
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}

	return &Response{
		Content:   content,
		ToolCalls: toolCalls,
	}, nil
}

func toFunctionParams(schema any) map[string]any {
	if m, ok := schema.(map[string]any); ok {
		return m
	}
	return nil
}
