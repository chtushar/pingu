package gateway

import (
	"encoding/json"
	"net/http"
	"pingu/internal/agent"
)

type chatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
	UserID    string `json:"user_id"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}
	if req.SessionID == "" || req.Message == "" {
		http.Error(w, `{"error":"session_id and message are required"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if req.UserID != "" {
		ctx = agent.ContextWithUserID(ctx, req.UserID)
	}
	ctx = agent.ContextWithPlatform(ctx, "gateway")

	sse := NewSSEWriter(w)
	var sentError bool

	err := s.runner.Run(ctx, req.SessionID, req.Message, func(ev agent.Event) {
		switch ev.Type {
		case agent.EventToken:
			sse.Send("token", map[string]string{"content": ev.Data.(string)})
		case agent.EventToolCall:
			sse.Send("tool_call", ev.Data)
		case agent.EventToolResult:
			sse.Send("tool_result", ev.Data)
		case agent.EventError:
			sentError = true
			sse.Send("error", map[string]string{"error": ev.Data.(string)})
		case agent.EventDone:
			sse.Send("done", map[string]any{})
		}
	})

	if err != nil && !sentError {
		sse.Send("error", map[string]string{"error": err.Error()})
	}
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
