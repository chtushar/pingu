package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"pingu/internal/agent"
	"time"
)

const (
	telegramAPIBase      = "https://api.telegram.org/bot%s"
	telegramSendMsg      = "/sendMessage"
	telegramChatAction   = "/sendChatAction"
	telegramGetUpdates   = "/getUpdates"
	telegramGetMe        = "/getMe"
	telegramActionTyping = "typing"
	telegramPollTimeout  = 30 // seconds, Telegram long poll timeout
)

type Telegram struct {
	botToken string
	runner   agent.Runner
	apiURL   string
	offset   int64
}

func NewTelegram(botToken string, runner agent.Runner) *Telegram {
	return &Telegram{
		botToken: botToken,
		runner:   runner,
		apiURL:   fmt.Sprintf(telegramAPIBase, botToken),
	}
}

func (t *Telegram) Name() string { return "telegram" }

func (t *Telegram) RegisterRoutes(mux *http.ServeMux) {}

func (t *Telegram) Start(ctx context.Context) error {
	slog.Info("telegram: starting long poll")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			updates, err := t.getUpdates(ctx)
			if err != nil {
				slog.Error("telegram: poll failed", "error", err)
				time.Sleep(time.Second)
				continue
			}
			for _, u := range updates {
				t.offset = u.UpdateID + 1
				t.handleUpdate(u)
			}
		}
	}
}

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message"`
}

type telegramMessage struct {
	Chat telegramChat `json:"chat"`
	Text string       `json:"text"`
}

type telegramChat struct {
	ID int64 `json:"id"`
}

type telegramSendRequest struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

type telegramGetUpdatesResponse struct {
	OK     bool             `json:"ok"`
	Result []telegramUpdate `json:"result"`
}

func (t *Telegram) getUpdates(ctx context.Context) ([]telegramUpdate, error) {
	body, _ := json.Marshal(map[string]any{
		"offset":  t.offset,
		"timeout": telegramPollTimeout,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.apiURL+telegramGetUpdates, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(telegramPollTimeout+5) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result telegramGetUpdatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram API returned ok=false")
	}
	return result.Result, nil
}

func (t *Telegram) handleUpdate(update telegramUpdate) {
	if update.Message == nil || update.Message.Text == "" {
		return
	}

	chatID := update.Message.Chat.ID
	text := update.Message.Text

	slog.Info("telegram: received message", "chat_id", chatID, "text", text)

	t.sendTyping(chatID)

	// Echo the message back
	if err := t.sendMessage(chatID, text); err != nil {
		slog.Error("telegram: failed to send message", "chat_id", chatID, "error", err)
	}
}

func (t *Telegram) sendTyping(chatID int64) {
	body, _ := json.Marshal(map[string]any{
		"chat_id": chatID,
		"action":  telegramActionTyping,
	})
	resp, err := http.Post(t.apiURL+telegramChatAction, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Warn("telegram: failed to send typing action", "chat_id", chatID, "error", err)
		return
	}
	resp.Body.Close()
}

func (t *Telegram) sendMessage(chatID int64, text string) error {
	body, err := json.Marshal(telegramSendRequest{
		ChatID: chatID,
		Text:   text,
	})
	if err != nil {
		return err
	}

	resp, err := http.Post(t.apiURL+telegramSendMsg, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned %d", resp.StatusCode)
	}
	return nil
}
