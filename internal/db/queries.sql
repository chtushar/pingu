-- name: UpsertSession :exec
INSERT INTO sessions (id, channel)
VALUES (?, ?)
ON CONFLICT(id) DO UPDATE SET updated_at = datetime('now');

-- name: GetSession :one
SELECT * FROM sessions WHERE id = ?;

-- name: InsertTurn :exec
INSERT INTO turns (session_id, user_message, response_json, model)
VALUES (?, ?, ?, ?);

-- name: GetTurnsBySession :many
SELECT * FROM turns WHERE session_id = ? ORDER BY created_at ASC;

-- name: InsertMemory :one
INSERT INTO memories (session_id, category, content, embedding, content_hash)
VALUES (?, ?, ?, ?, ?)
RETURNING id;

-- name: DeleteMemory :exec
DELETE FROM memories WHERE id = ?;

-- name: GetMemoriesBySession :many
SELECT * FROM memories
WHERE session_id IS NULL OR session_id = ?
ORDER BY created_at DESC;

-- name: GetAllMemoriesWithEmbedding :many
SELECT id, session_id, category, content, embedding
FROM memories
WHERE embedding IS NOT NULL
  AND (session_id IS NULL OR session_id = ?);

-- name: DeleteExpiredMemories :exec
DELETE FROM memories
WHERE category = 'conversation'
  AND created_at < datetime('now', '-' || sqlc.arg(days_old) || ' days');

-- name: GetEmbeddingCache :one
SELECT * FROM embedding_cache WHERE content_hash = ?;

-- name: UpsertEmbeddingCache :exec
INSERT INTO embedding_cache (content_hash, embed_model, embedding, last_used_at)
VALUES (?, ?, ?, datetime('now'))
ON CONFLICT(content_hash) DO UPDATE SET
    embedding = excluded.embedding,
    last_used_at = datetime('now');

-- name: PruneEmbeddingCache :exec
DELETE FROM embedding_cache
WHERE content_hash NOT IN (
    SELECT content_hash FROM embedding_cache
    ORDER BY last_used_at DESC
    LIMIT sqlc.arg(keep_count)
);

-- name: UpdateSessionSummary :exec
UPDATE sessions
SET summary = ?, summary_up_to = ?, updated_at = datetime('now')
WHERE id = ?;

-- name: CountTurnsBySession :one
SELECT COUNT(*) FROM turns WHERE session_id = ?;

-- name: GetTurnsBySessionAfterID :many
SELECT * FROM turns
WHERE session_id = ? AND id > ?
ORDER BY created_at ASC;
