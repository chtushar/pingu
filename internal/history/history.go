package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"pingu/internal/db"

	"github.com/openai/openai-go/v3/responses"
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

func (s *Store) SaveTurn(ctx context.Context, sessionID, userMessage string, resp *responses.Response) error {
	raw := resp.RawJSON()
	return s.q.InsertTurn(ctx, db.InsertTurnParams{
		SessionID:    sessionID,
		UserMessage:  userMessage,
		ResponseJson: raw,
		Model:        sql.NullString{String: resp.Model, Valid: resp.Model != ""},
	})
}

func (s *Store) LoadInputHistory(ctx context.Context, sessionID string) ([]responses.ResponseInputItemUnionParam, error) {
	turns, err := s.q.GetTurnsBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	var items []responses.ResponseInputItemUnionParam
	for _, turn := range turns {
		// Add user message.
		items = append(items, responses.ResponseInputItemParamOfMessage(turn.UserMessage, "user"))

		// Deserialize the stored response.
		var resp responses.Response
		if err := json.Unmarshal([]byte(turn.ResponseJson), &resp); err != nil {
			slog.Warn("skipping turn with invalid response JSON", "turn_id", turn.ID, "error", err)
			continue
		}

		// Convert output items to input items.
		items = append(items, OutputToInput(resp.Output)...)
	}

	return items, nil
}

// OutputToInput converts response output items into input item params
// for the next API call. Each output type's ToParam() does a lossless
// round-trip via RawJSON.
func OutputToInput(output []responses.ResponseOutputItemUnion) []responses.ResponseInputItemUnionParam {
	var items []responses.ResponseInputItemUnionParam
	for _, item := range output {
		switch item.Type {
		case "message":
			v := item.AsMessage().ToParam()
			items = append(items, responses.ResponseInputItemUnionParam{OfOutputMessage: &v})
		case "function_call":
			v := item.AsFunctionCall().ToParam()
			items = append(items, responses.ResponseInputItemUnionParam{OfFunctionCall: &v})
		case "reasoning":
			v := item.AsReasoning().ToParam()
			items = append(items, responses.ResponseInputItemUnionParam{OfReasoning: &v})
		case "file_search_call":
			v := item.AsFileSearchCall().ToParam()
			items = append(items, responses.ResponseInputItemUnionParam{OfFileSearchCall: &v})
		case "web_search_call":
			v := item.AsWebSearchCall().ToParam()
			items = append(items, responses.ResponseInputItemUnionParam{OfWebSearchCall: &v})
		case "computer_call":
			v := item.AsComputerCall().ToParam()
			items = append(items, responses.ResponseInputItemUnionParam{OfComputerCall: &v})
		case "code_interpreter_call":
			v := item.AsCodeInterpreterCall().ToParam()
			items = append(items, responses.ResponseInputItemUnionParam{OfCodeInterpreterCall: &v})
		default:
			slog.Debug("skipping unknown output item type", "type", item.Type)
		}
	}
	return items
}
