package llm

import (
	"context"
	"fmt"
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
		Transport: otelhttp.NewTransport(http.DefaultTransport),
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

	stream := o.client.Responses.NewStreaming(ctx, params)

	var completed *responses.Response

	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "response.output_text.delta":
			if event.Delta != "" {
				onToken(event.Delta)
			}
		case "response.completed":
			completed = &event.Response
		case "response.failed":
			return nil, fmt.Errorf("response failed: %s", event.Response.Error.Message)
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}

	return completed, nil
}
