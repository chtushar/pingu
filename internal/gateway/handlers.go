package gateway

import "net/http"

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
