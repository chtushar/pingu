package llm

import (
	"context"

	"github.com/openai/openai-go/v3/responses"
)

type Provider interface {
	ChatStream(ctx context.Context, input []responses.ResponseInputItemUnionParam, tools []responses.ToolUnionParam, onToken func(string)) (*responses.Response, error)
}
