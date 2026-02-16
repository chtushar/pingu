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
