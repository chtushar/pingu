package memory

import (
	"context"

	"github.com/openai/openai-go/v3/responses"
)

// Memory recalls prior context for a session to feed into the LLM.
type Memory interface {
	Recall(ctx context.Context, sessionID string) ([]responses.ResponseInputItemUnionParam, error)
}
