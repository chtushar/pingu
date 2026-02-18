package embedding

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log/slog"

	"pingu/internal/db"
)

// CachedProvider wraps a Provider with SHA-256 content-addressed caching in SQLite.
type CachedProvider struct {
	inner     Provider
	queries   *db.Queries
	cacheSize int
}

func NewCachedProvider(inner Provider, database *db.DB, cacheSize int) *CachedProvider {
	if cacheSize <= 0 {
		cacheSize = 10000
	}
	return &CachedProvider{
		inner:     inner,
		queries:   db.New(database.Conn()),
		cacheSize: cacheSize,
	}
}

func (c *CachedProvider) Model() string  { return c.inner.Model() }
func (c *CachedProvider) Dimensions() int { return c.inner.Dimensions() }

func (c *CachedProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	results := make([][]float32, len(texts))
	var misses []int // indices of texts not found in cache

	// Check cache for each text.
	for i, text := range texts {
		hash := contentHash(text)
		cached, err := c.queries.GetEmbeddingCache(ctx, hash)
		if err == nil {
			results[i] = BytesToFloat32s(cached.Embedding)
			continue
		}
		if err != sql.ErrNoRows {
			slog.Debug("embedding cache lookup error", "error", err)
		}
		misses = append(misses, i)
	}

	if len(misses) == 0 {
		return results, nil
	}

	// Batch embed cache misses.
	missTexts := make([]string, len(misses))
	for i, idx := range misses {
		missTexts[i] = texts[idx]
	}

	embeddings, err := c.inner.Embed(ctx, missTexts)
	if err != nil {
		return nil, err
	}

	// Store results and update cache.
	model := c.inner.Model()
	for i, idx := range misses {
		results[idx] = embeddings[i]
		hash := contentHash(texts[idx])
		if err := c.queries.UpsertEmbeddingCache(ctx, db.UpsertEmbeddingCacheParams{
			ContentHash: hash,
			EmbedModel:  model,
			Embedding:   Float32sToBytes(embeddings[i]),
		}); err != nil {
			slog.Debug("embedding cache store error", "error", err)
		}
	}

	// Prune if needed (best-effort).
	if err := c.queries.PruneEmbeddingCache(ctx, int64(c.cacheSize)); err != nil {
		slog.Debug("embedding cache prune error", "error", err)
	}

	return results, nil
}

func contentHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h)
}
