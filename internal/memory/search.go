package memory

import (
	"context"
	"database/sql"
	"log/slog"
	"sort"
	"strings"

	"pingu/internal/db"
	"pingu/internal/embedding"
)

// SearchResult represents a single memory match from hybrid search.
type SearchResult struct {
	MemoryID int64
	Content  string
	Category string
	Score    float32
}

// HybridSearcher combines FTS5 keyword search with vector cosine similarity.
type HybridSearcher struct {
	conn         *sql.DB
	queries      *db.Queries
	embedder     embedding.Provider // nil = FTS5-only mode
	vectorWeight float32
	ftsWeight    float32
}

func NewHybridSearcher(database *db.DB, embedder embedding.Provider, vectorWeight, ftsWeight float32) *HybridSearcher {
	if embedder == nil {
		// FTS-only mode: all weight on FTS.
		ftsWeight = 1.0
		vectorWeight = 0.0
	}
	return &HybridSearcher{
		conn:         database.Conn(),
		queries:      db.New(database.Conn()),
		embedder:     embedder,
		vectorWeight: vectorWeight,
		ftsWeight:    ftsWeight,
	}
}

// Search performs hybrid FTS5 + vector search, merging results.
func (h *HybridSearcher) Search(ctx context.Context, query, sessionID string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}

	type scored struct {
		id       int64
		content  string
		category string
		fts      float32
		vec      float32
	}
	merged := make(map[int64]*scored)

	// FTS5 keyword search.
	ftsResults, err := h.ftsSearch(ctx, query, sessionID)
	if err != nil {
		slog.Debug("fts search error", "error", err)
	} else {
		for _, r := range ftsResults {
			merged[r.MemoryID] = &scored{
				id:       r.MemoryID,
				content:  r.Content,
				category: r.Category,
				fts:      r.Score,
			}
		}
	}

	// Vector search (if embedder available).
	if h.embedder != nil {
		vecResults, err := h.vectorSearch(ctx, query, sessionID)
		if err != nil {
			slog.Debug("vector search error", "error", err)
		} else {
			for _, r := range vecResults {
				if s, ok := merged[r.MemoryID]; ok {
					s.vec = r.Score
				} else {
					merged[r.MemoryID] = &scored{
						id:       r.MemoryID,
						content:  r.Content,
						category: r.Category,
						vec:      r.Score,
					}
				}
			}
		}
	}

	// Compute final scores and sort.
	results := make([]SearchResult, 0, len(merged))
	for _, s := range merged {
		final := h.vectorWeight*s.vec + h.ftsWeight*s.fts
		results = append(results, SearchResult{
			MemoryID: s.id,
			Content:  s.content,
			Category: s.category,
			Score:    final,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// ftsSearch runs FTS5 MATCH query with BM25 scoring.
func (h *HybridSearcher) ftsSearch(ctx context.Context, query, sessionID string) ([]SearchResult, error) {
	const q = `
		SELECT m.id, m.content, m.category, bm25(memories_fts) AS rank
		FROM memories_fts f
		JOIN memories m ON m.id = f.rowid
		WHERE memories_fts MATCH ?
		  AND (m.session_id IS NULL OR m.session_id = ?)
		ORDER BY rank
		LIMIT 50
	`

	rows, err := h.conn.QueryContext(ctx, q, escapeFTS5Query(query), sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	var minRank, maxRank float32
	first := true

	type raw struct {
		id       int64
		content  string
		category string
		rank     float32
	}
	var raws []raw

	for rows.Next() {
		var r raw
		if err := rows.Scan(&r.id, &r.content, &r.category, &r.rank); err != nil {
			return nil, err
		}
		// BM25 returns negative scores (more negative = better match).
		r.rank = -r.rank
		if first || r.rank < minRank {
			minRank = r.rank
		}
		if first || r.rank > maxRank {
			maxRank = r.rank
		}
		first = false
		raws = append(raws, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Normalize to [0, 1].
	span := maxRank - minRank
	for _, r := range raws {
		var score float32
		if span > 0 {
			score = (r.rank - minRank) / span
		} else if len(raws) > 0 {
			score = 1.0
		}
		results = append(results, SearchResult{
			MemoryID: r.id,
			Content:  r.content,
			Category: r.category,
			Score:    score,
		})
	}

	return results, nil
}

// vectorSearch embeds the query and computes cosine similarity against stored embeddings.
func (h *HybridSearcher) vectorSearch(ctx context.Context, query, sessionID string) ([]SearchResult, error) {
	vecs, err := h.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 || len(vecs[0]) == 0 {
		return nil, nil
	}
	queryVec := vecs[0]

	memories, err := h.queries.GetAllMemoriesWithEmbedding(ctx, sql.NullString{String: sessionID, Valid: sessionID != ""})
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, m := range memories {
		memVec := embedding.BytesToFloat32s(m.Embedding)
		score := embedding.CosineSimilarity(queryVec, memVec)
		if score > 0 {
			results = append(results, SearchResult{
				MemoryID: m.ID,
				Content:  m.Content,
				Category: m.Category,
				Score:    score,
			})
		}
	}

	return results, nil
}

// escapeFTS5Query quotes each term in the query to prevent FTS5 syntax errors
// from special characters like ?, *, AND, OR, NOT, etc.
func escapeFTS5Query(query string) string {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return query
	}
	for i, t := range terms {
		terms[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}
	return strings.Join(terms, " ")
}
