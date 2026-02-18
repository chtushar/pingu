package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"

	"pingu/internal/db"
	"pingu/internal/embedding"
)

// SemanticStore stores and deletes semantic memories with optional embeddings.
type SemanticStore struct {
	queries  *db.Queries
	embedder embedding.Provider // nil = no embeddings
}

func NewSemanticStore(database *db.DB, embedder embedding.Provider) *SemanticStore {
	return &SemanticStore{
		queries:  db.New(database.Conn()),
		embedder: embedder,
	}
}

// Store persists a memory with optional embedding. Returns the memory ID.
func (s *SemanticStore) Store(ctx context.Context, sessionID *string, category, content string) (int64, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

	var embBytes []byte
	if s.embedder != nil {
		vecs, err := s.embedder.Embed(ctx, []string{content})
		if err == nil && len(vecs) > 0 {
			embBytes = embedding.Float32sToBytes(vecs[0])
		}
	}

	sid := sql.NullString{}
	if sessionID != nil {
		sid = sql.NullString{String: *sessionID, Valid: true}
	}

	return s.queries.InsertMemory(ctx, db.InsertMemoryParams{
		SessionID:   sid,
		Category:    category,
		Content:     content,
		Embedding:   embBytes,
		ContentHash: sql.NullString{String: hash, Valid: true},
	})
}

// Delete removes a memory by ID.
func (s *SemanticStore) Delete(ctx context.Context, id int64) error {
	return s.queries.DeleteMemory(ctx, id)
}
