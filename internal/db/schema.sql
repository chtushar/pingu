-- Pingu database schema (idempotent — safe to run multiple times)

CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT PRIMARY KEY,
    channel    TEXT NOT NULL DEFAULT 'cli',
    channel_id TEXT,
    title      TEXT,
    model           TEXT,
    summary         TEXT,           -- rolling summary of compacted messages
    summary_up_to   TEXT,           -- message id up to which the summary covers
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS messages (
    id           TEXT PRIMARY KEY,
    session_id   TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role         TEXT NOT NULL,  -- 'system', 'user', 'assistant', 'tool'
    content      TEXT,
    tool_calls   TEXT,           -- JSON array of tool calls (for assistant messages)
    tool_call_id TEXT,           -- references the tool_call id (for tool messages)
    name         TEXT,           -- tool name (for tool messages)
    model        TEXT,           -- model that generated this message (for assistant messages)
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_messages_tool_call ON messages(tool_call_id);

CREATE TABLE IF NOT EXISTS turns (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id    TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    user_message  TEXT NOT NULL,
    response_json TEXT NOT NULL,
    model         TEXT,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_turns_session ON turns(session_id, created_at);

-- Semantic memory store
CREATE TABLE IF NOT EXISTS memories (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id   TEXT,                                    -- NULL = global/cross-session
    category     TEXT NOT NULL DEFAULT 'conversation',    -- 'core', 'daily', 'conversation'
    content      TEXT NOT NULL,
    embedding    BLOB,                                    -- float32 vector as raw bytes
    content_hash TEXT,                                    -- SHA-256 for dedup
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_memories_session ON memories(session_id);
CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category);
CREATE INDEX IF NOT EXISTS idx_memories_hash ON memories(content_hash);

-- FTS5 for keyword search
CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
    content, content='memories', content_rowid='id'
);

-- Triggers to keep FTS5 in sync
CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
    INSERT INTO memories_fts(rowid, content) VALUES (new.id, new.content);
END;
CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, content) VALUES ('delete', old.id, old.content);
END;
CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, content) VALUES ('delete', old.id, old.content);
    INSERT INTO memories_fts(rowid, content) VALUES (new.id, new.content);
END;

-- Embedding cache (avoid redundant API calls)
CREATE TABLE IF NOT EXISTS embedding_cache (
    content_hash TEXT PRIMARY KEY,
    embed_model  TEXT NOT NULL,
    embedding    BLOB NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    last_used_at TEXT NOT NULL DEFAULT (datetime('now'))
);
