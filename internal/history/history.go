package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"pingu/internal/db"
	"pingu/internal/llm"
)

type Store struct {
	q *db.Queries
}

func NewStore(database *db.DB) *Store {
	return &Store{q: db.New(database.Conn())}
}

func (s *Store) EnsureSession(ctx context.Context, sessionID, channel string) error {
	return s.q.UpsertSession(ctx, db.UpsertSessionParams{
		ID:      sessionID,
		Channel: channel,
	})
}

func (s *Store) SaveTurn(ctx context.Context, sessionID, userMessage string, resp *llm.Response) error {
	raw, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	return s.q.InsertTurn(ctx, db.InsertTurnParams{
		SessionID:    sessionID,
		UserMessage:  userMessage,
		ResponseJson: string(raw),
		Model:        sql.NullString{String: resp.Model, Valid: resp.Model != ""},
	})
}

// LoadCompactedHistory loads history using a session's summary for older turns.
// If a summary exists, it's prepended as a developer message, and only turns
// after the summary cutoff are loaded in full.
func (s *Store) LoadCompactedHistory(ctx context.Context, sessionID string) ([]llm.InputItem, error) {
	session, err := s.q.GetSession(ctx, sessionID)
	if err != nil {
		return s.LoadInputHistory(ctx, sessionID)
	}

	if !session.Summary.Valid || session.Summary.String == "" {
		return s.LoadInputHistory(ctx, sessionID)
	}

	var items []llm.InputItem
	items = append(items, llm.NewMessage(
		"[Conversation summary]\n"+session.Summary.String, llm.RoleDeveloper,
	))

	// Parse summary_up_to as turn ID.
	var cutoffID int64
	if session.SummaryUpTo.Valid {
		fmt.Sscanf(session.SummaryUpTo.String, "%d", &cutoffID)
	}

	turns, err := s.q.GetTurnsBySessionAfterID(ctx, db.GetTurnsBySessionAfterIDParams{
		SessionID: sessionID,
		ID:        cutoffID,
	})
	if err != nil {
		return items, nil
	}

	for _, turn := range turns {
		items = append(items, llm.NewMessage(turn.UserMessage, llm.RoleUser))
		var resp llm.Response
		if err := json.Unmarshal([]byte(turn.ResponseJson), &resp); err != nil {
			slog.Warn("skipping turn with invalid response JSON", "turn_id", turn.ID, "error", err)
			continue
		}
		items = append(items, llm.OutputToInput(resp.Output)...)
	}

	return items, nil
}

func (s *Store) LoadInputHistory(ctx context.Context, sessionID string) ([]llm.InputItem, error) {
	turns, err := s.q.GetTurnsBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	var items []llm.InputItem
	for _, turn := range turns {
		items = append(items, llm.NewMessage(turn.UserMessage, llm.RoleUser))

		var resp llm.Response
		if err := json.Unmarshal([]byte(turn.ResponseJson), &resp); err != nil {
			slog.Warn("skipping turn with invalid response JSON", "turn_id", turn.ID, "error", err)
			continue
		}

		items = append(items, llm.OutputToInput(resp.Output)...)
	}

	return items, nil
}
