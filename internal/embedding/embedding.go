package embedding

import "context"

// Provider generates vector embeddings for text.
type Provider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Model() string
	Dimensions() int
}
