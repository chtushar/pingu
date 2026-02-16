package channels

import (
	"net/http"
	"pingu/internal/agent"
)

type Telegram struct {
	botToken string
	runner   agent.Runner
}

func NewTelegram(botToken string, runner agent.Runner) *Telegram {
	return &Telegram{botToken: botToken, runner: runner}
}

func (t *Telegram) Name() string { return "telegram" }

func (t *Telegram) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /webhook/telegram", t.handleWebhook)
}

func (t *Telegram) handleWebhook(w http.ResponseWriter, r *http.Request) {}
