package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"pingu/internal/config"
	"pingu/internal/db"
	"pingu/internal/history"
	"pingu/internal/llm"

	"github.com/openai/openai-go/v3/responses"
)

// Compactor summarizes older conversation turns to keep context windows manageable.
type Compactor struct {
	store    *history.Store
	queries  *db.Queries
	provider llm.Provider
	cfg      config.CompactionConfig
}

func NewCompactor(store *history.Store, database *db.DB, provider llm.Provider, cfg config.CompactionConfig) *Compactor {
	return &Compactor{
		store:   store,
		queries: db.New(database.Conn()),
		provider: provider,
		cfg:     cfg,
	}
}

// MaybeCompact checks if a session has exceeded the turn threshold and
// summarizes older turns if so.
func (c *Compactor) MaybeCompact(ctx context.Context, sessionID string) {
	count, err := c.queries.CountTurnsBySession(ctx, sessionID)
	if err != nil {
		slog.Debug("compaction: count error", "session_id", sessionID, "error", err)
		return
	}

	if int(count) < c.cfg.TurnThreshold {
		return
	}

	session, err := c.queries.GetSession(ctx, sessionID)
	if err != nil {
		slog.Debug("compaction: get session error", "session_id", sessionID, "error", err)
		return
	}

	// Load turns to summarize (everything except the most recent KeepRecent).
	turns, err := c.queries.GetTurnsBySession(ctx, sessionID)
	if err != nil {
		slog.Debug("compaction: get turns error", "session_id", sessionID, "error", err)
		return
	}

	if len(turns) <= c.cfg.KeepRecent {
		return
	}

	toSummarize := turns[:len(turns)-c.cfg.KeepRecent]
	cutoffID := toSummarize[len(toSummarize)-1].ID

	// Build text from turns to summarize.
	var b strings.Builder
	if session.Summary.Valid && session.Summary.String != "" {
		fmt.Fprintf(&b, "Previous summary:\n%s\n\n", session.Summary.String)
	}
	b.WriteString("New turns to incorporate:\n")
	for _, turn := range toSummarize {
		fmt.Fprintf(&b, "User: %s\n", turn.UserMessage)
		// Extract assistant text from response JSON.
		var resp responses.Response
		if err := json.Unmarshal([]byte(turn.ResponseJson), &resp); err == nil {
			for _, item := range resp.Output {
				if item.Type == "message" {
					msg := item.AsMessage()
					for _, c := range msg.Content {
						if c.Type == "output_text" {
							fmt.Fprintf(&b, "Assistant: %s\n", c.AsOutputText().Text)
						}
					}
				}
			}
		}
	}

	summary, err := c.summarize(ctx, b.String())
	if err != nil {
		slog.Debug("compaction: summarize error", "session_id", sessionID, "error", err)
		return
	}

	err = c.queries.UpdateSessionSummary(ctx, db.UpdateSessionSummaryParams{
		Summary:     sql.NullString{String: summary, Valid: true},
		SummaryUpTo: sql.NullString{String: fmt.Sprintf("%d", cutoffID), Valid: true},
		ID:          sessionID,
	})
	if err != nil {
		slog.Debug("compaction: update summary error", "session_id", sessionID, "error", err)
	}

	slog.Info("compaction: summarized turns",
		"session_id", sessionID,
		"turns_summarized", len(toSummarize),
		"cutoff_id", cutoffID,
	)
}

func (c *Compactor) summarize(ctx context.Context, text string) (string, error) {
	prompt := "Summarize the following conversation concisely, preserving key facts, decisions, and context needed for continuity. Output only the summary, no preamble.\n\n" + text

	input := []responses.ResponseInputItemUnionParam{
		responses.ResponseInputItemParamOfMessage(prompt, "user"),
	}

	resp, err := c.provider.ChatStream(ctx, input, nil, func(string) {})
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}

	var summary strings.Builder
	for _, item := range resp.Output {
		if item.Type == "message" {
			msg := item.AsMessage()
			for _, c := range msg.Content {
				if c.Type == "output_text" {
					summary.WriteString(c.AsOutputText().Text)
				}
			}
		}
	}

	return summary.String(), nil
}
