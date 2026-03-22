package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"pingu/internal/agent"
	"pingu/internal/audio"
	"strings"
	"time"
)

const (
	telegramAPIBase      = "https://api.telegram.org/bot%s"
	telegramFileBase     = "https://api.telegram.org/file/bot%s/%s"
	telegramSendMsg      = "/sendMessage"
	telegramChatAction   = "/sendChatAction"
	telegramGetUpdates   = "/getUpdates"
	telegramGetMe        = "/getMe"
	telegramGetFile      = "/getFile"
	telegramActionTyping = "typing"
	telegramPollTimeout  = 30 // seconds, Telegram long poll timeout
)

type Telegram struct {
	botToken     string
	runner       agent.Runner
	transcriber  audio.Transcriber // nil = audio not supported
	apiURL       string
	offset       int64
	allowedUsers map[int64]bool
}

func NewTelegram(botToken string, allowedUsers []int64, runner agent.Runner, transcriber audio.Transcriber) *Telegram {
	allowed := make(map[int64]bool, len(allowedUsers))
	for _, id := range allowedUsers {
		allowed[id] = true
	}
	return &Telegram{
		botToken:     botToken,
		runner:       runner,
		transcriber:  transcriber,
		apiURL:       fmt.Sprintf(telegramAPIBase, botToken),
		allowedUsers: allowed,
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
				t.handleUpdate(ctx, u)
			}
		}
	}
}

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message"`
}

type telegramMessage struct {
	From  telegramUser   `json:"from"`
	Chat  telegramChat   `json:"chat"`
	Text  string         `json:"text"`
	Voice *telegramVoice `json:"voice"`
	Audio *telegramAudio `json:"audio"`
}

type telegramVoice struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
}

type telegramAudio struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
	MimeType string `json:"mime_type"`
}

type telegramUser struct {
	ID int64 `json:"id"`
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

type telegramGetFileResponse struct {
	OK     bool `json:"ok"`
	Result struct {
		FilePath string `json:"file_path"`
	} `json:"result"`
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

func (t *Telegram) handleUpdate(ctx context.Context, update telegramUpdate) {
	if update.Message == nil {
		return
	}

	msg := update.Message
	chatID := msg.Chat.ID
	hasText := msg.Text != ""
	hasAudio := msg.Voice != nil || msg.Audio != nil

	if !hasText && !hasAudio {
		return
	}

	if len(t.allowedUsers) > 0 && !t.allowedUsers[msg.From.ID] {
		slog.Warn("telegram: unauthorized user", "user_id", msg.From.ID)
		t.sendMessage(chatID, "Sorry, you are not authorized to use this bot.")
		return
	}

	userID := msg.From.ID

	if hasAudio && t.transcriber != nil {
		fileID := ""
		if msg.Voice != nil {
			fileID = msg.Voice.FileID
		} else {
			fileID = msg.Audio.FileID
		}
		t.processAudio(ctx, chatID, userID, fileID)
		return
	}

	if !hasText {
		return
	}

	t.runText(ctx, chatID, userID, msg.Text)
}

// processAudio downloads, converts, and transcribes an audio file.
// On success the transcribed text is passed to the runner.
// On failure the error and file paths are passed to the runner so the
// agent can use its Shell tool to diagnose and fix the issue.
func (t *Telegram) processAudio(ctx context.Context, chatID, userID int64, fileID string) {
	t.sendTyping(chatID)

	filePath, err := t.getFile(fileID)
	if err != nil {
		slog.Error("telegram: getFile failed", "error", err)
		t.sendMessage(chatID, "Sorry, could not retrieve the audio file.")
		return
	}

	localPath, err := t.downloadFile(filePath)
	if err != nil {
		slog.Error("telegram: download failed", "error", err)
		t.sendMessage(chatID, "Sorry, could not download the audio file.")
		return
	}
	// Don't defer remove — the agent may need the file if processing fails.

	wavPath := localPath + ".wav"
	if err := audio.ConvertToWav(localPath, wavPath); err != nil {
		slog.Error("telegram: ffmpeg convert failed", "error", err)
		prompt := fmt.Sprintf(
			"[Voice message received]\nThe user sent a voice message. I downloaded the audio file to: %s\n"+
				"Converting to WAV failed with error: %s\n"+
				"Please fix the issue (e.g. install ffmpeg via shell), convert the file to WAV, "+
				"transcribe it, and respond to what the user said. "+
				"Clean up any temp files when done.",
			localPath, err,
		)
		t.runText(ctx, chatID, userID, prompt)
		return
	}

	text, err := t.transcriber.Transcribe(ctx, wavPath)
	if err != nil {
		slog.Error("telegram: transcription failed", "error", err)
		prompt := fmt.Sprintf(
			"[Voice message received]\nThe user sent a voice message. I downloaded the audio to: %s and converted it to WAV at: %s\n"+
				"Transcription failed with error: %s\n"+
				"Please fix the issue (e.g. install whisper via `pip install openai-whisper`), "+
				"transcribe the WAV file, and respond to what the user said. "+
				"Clean up any temp files when done.",
			localPath, wavPath, err,
		)
		t.runText(ctx, chatID, userID, prompt)
		return
	}

	// Happy path — clean up temp files ourselves.
	os.Remove(localPath)
	os.Remove(wavPath)

	if text == "" {
		t.sendMessage(chatID, "Could not detect any speech in the audio.")
		return
	}

	slog.Debug("telegram: transcribed audio", "chat_id", chatID, "text", text)
	t.runText(ctx, chatID, userID, text)
}

// runText sends text through the agent runner and replies with the result.
func (t *Telegram) runText(ctx context.Context, chatID int64, userID int64, text string) {
	slog.Debug("telegram: received message", "chat_id", chatID, "text", text)

	t.sendTyping(chatID)

	ctx = agent.ContextWithUserID(ctx, fmt.Sprintf("%d", userID))
	ctx = agent.ContextWithPlatform(ctx, "telegram")

	sessionID := fmt.Sprintf("telegram:%d", chatID)
	var response strings.Builder
	err := t.runner.Run(ctx, sessionID, text, func(e agent.Event) {
		switch e.Type {
		case agent.EventToken:
			if s, ok := e.Data.(string); ok {
				response.WriteString(s)
			}
		case agent.EventToolCall:
			slog.Debug("telegram: tool call", "chat_id", chatID, "data", e.Data)
		case agent.EventToolResult:
			slog.Debug("telegram: tool result", "chat_id", chatID, "data", e.Data)
		case agent.EventError:
			slog.Error("telegram: agent error event", "chat_id", chatID, "data", e.Data)
		}
	})
	if err != nil {
		slog.Error("telegram: runner failed", "chat_id", chatID, "error", err)
		t.sendMessage(chatID, "Sorry, something went wrong.")
		return
	}

	reply := response.String()
	slog.Debug("telegram: run completed", "chat_id", chatID, "reply_len", len(reply))

	if reply != "" {
		if err := t.sendMessage(chatID, reply); err != nil {
			slog.Error("telegram: failed to send message", "chat_id", chatID, "error", err)
		}
	} else {
		slog.Warn("telegram: agent produced no response tokens", "chat_id", chatID)
	}
}

// getFile calls the Telegram getFile API and returns the file_path.
func (t *Telegram) getFile(fileID string) (string, error) {
	body, _ := json.Marshal(map[string]any{"file_id": fileID})
	resp, err := http.Post(t.apiURL+telegramGetFile, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result telegramGetFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if !result.OK {
		return "", fmt.Errorf("telegram getFile returned ok=false")
	}
	return result.Result.FilePath, nil
}

// downloadFile downloads a file from Telegram servers to a temp directory.
func (t *Telegram) downloadFile(filePath string) (string, error) {
	url := fmt.Sprintf(telegramFileBase, t.botToken, filePath)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("telegram file download returned %d", resp.StatusCode)
	}

	ext := filepath.Ext(filePath)
	if ext == "" {
		ext = ".ogg"
	}
	tmp, err := os.CreateTemp("", "tg-audio-*"+ext)
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
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
