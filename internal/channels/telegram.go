package channels

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"pingu/internal/agent"
)

const (
	telegramAPIBase    = "https://api.telegram.org/bot%s"
	telegramSendMsg      = "/sendMessage"
	telegramChatAction   = "/sendChatAction"
	telegramSetWebhook   = "/setWebhook"
	telegramGetMe        = "/getMe"
	telegramActionTyping = "typing"
)

type Telegram struct {
	botToken string
	runner   agent.Runner
	apiURL   string
}

func NewTelegram(botToken string, runner agent.Runner) *Telegram {
	return &Telegram{
		botToken: botToken,
		runner:   runner,
		apiURL:   fmt.Sprintf(telegramAPIBase, botToken),
	}
}

func (t *Telegram) Name() string { return "telegram" }

func (t *Telegram) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /webhook/telegram", t.handleWebhook)
}

type telegramUpdate struct {
	Message *telegramMessage `json:"message"`
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

func (t *Telegram) handleWebhook(w http.ResponseWriter, r *http.Request) {
	var update telegramUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		slog.Error("telegram: failed to decode update", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if update.Message == nil || update.Message.Text == "" {
		w.WriteHeader(http.StatusOK)
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

	w.WriteHeader(http.StatusOK)
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
