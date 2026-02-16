package gateway

import (
	"context"
	"net/http"
	"pingu/internal/agent"
	"pingu/internal/channels"
)

type Server struct {
	runner   agent.Runner
	mux      *http.ServeMux
	httpSrv  *http.Server
}

func NewServer(runner agent.Runner, chs ...channels.Channel) *Server {
	s := &Server{
		runner: runner,
		mux:    http.NewServeMux(),
	}
	s.routes()
	for _, ch := range chs {
		ch.RegisterRoutes(s.mux)
	}
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /v1/chat", s.handleChat)
	s.mux.HandleFunc("GET /v1/sessions", s.handleListSessions)
	s.mux.HandleFunc("GET /v1/sessions/{id}", s.handleGetSession)
	s.mux.HandleFunc("DELETE /v1/sessions/{id}/run", s.handleCancelRun)
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	s.httpSrv = &http.Server{Addr: addr, Handler: s.mux}

	go func() {
		<-ctx.Done()
		s.httpSrv.Shutdown(context.Background())
	}()

	err := s.httpSrv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
