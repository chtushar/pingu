package embedding

import (
	"context"
	"fmt"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// OpenAI implements Provider using the OpenAI-compatible embeddings endpoint.
type OpenAI struct {
	client     *openai.Client
	model      string
	dimensions int
}

func NewOpenAI(baseURL, apiKey, model string, dimensions int) *OpenAI {
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
	return &OpenAI{client: &client, model: model, dimensions: dimensions}
}

func (o *OpenAI) Model() string  { return o.model }
func (o *OpenAI) Dimensions() int { return o.dimensions }

func (o *OpenAI) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	params := openai.EmbeddingNewParams{
		Model: o.model,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: texts,
		},
	}
	if o.dimensions > 0 {
		params.Dimensions = param.NewOpt(int64(o.dimensions))
	}

	resp, err := o.client.Embeddings.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai embedding: %w", err)
	}

	result := make([][]float32, len(resp.Data))
	for _, emb := range resp.Data {
		vec := make([]float32, len(emb.Embedding))
		for j, v := range emb.Embedding {
			vec[j] = float32(v)
		}
		result[emb.Index] = vec
	}
	return result, nil
}
